// home - certificate.go
// 该文件实现 Home 控制平面的 mTLS 证书管理功能，包括从 JWT 提取配置信息、
// 生成客户端密钥和 CSR、通过 RESP 协议向 Home 服务端请求客户端证书、
// 验证 CA 证书指纹、以及本地证书文件的读写和权限管理。
package home

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

// homeCertificateRequestTimeout 是向 Home 服务端请求证书的网络超时时间。
const homeCertificateRequestTimeout = 30 * time.Second

// homeJWTClaims 表示 Home 注册 JWT 中的声明字段，包含证书 ID、集群 ID、
// CA 指纹、注册密钥、目标地址等信息。
type homeJWTClaims struct {
	CertificateID    string `json:"certificate_id"`    // 证书唯一标识
	ClusterID        string `json:"cluster_id"`        // 集群标识
	CAFingerprint    string `json:"ca_fingerprint"`    // CA 证书的 SHA-256 指纹
	EnrollmentSecret string `json:"enrollment_secret"` // 注册密钥
	IP               string `json:"ip"`                // Home 服务端 IP 地址
	Port             int    `json:"port"`              // Home 服务端端口
	IssuedAt         int64  `json:"iat"`               // JWT 签发时间戳
}

// certificateRequestResponse 表示 Home 服务端返回的证书请求响应。
type certificateRequestResponse struct {
	OK          bool   `json:"ok"`          // 请求是否成功
	Certificate string `json:"certificate"` // PEM 格式的客户端证书
	CA          string `json:"ca"`          // PEM 格式的 CA 证书
}

// certificatePaths 存储本地证书文件的路径信息。
type certificatePaths struct {
	Dir        string // 证书目录
	ClientCert string // 客户端证书文件路径
	ClientKey  string // 客户端私钥文件路径
	CACert     string // CA 证书文件路径
}

// ConfigFromJWT prepares a Home config from the JWT and ensures local mTLS files exist.
func ConfigFromJWT(ctx context.Context, rawJWT string) (config.HomeConfig, error) {
	claims, errClaims := parseHomeJWTClaims(rawJWT)
	if errClaims != nil {
		return config.HomeConfig{}, errClaims
	}
	paths, errPaths := defaultCertificatePaths()
	if errPaths != nil {
		return config.HomeConfig{}, errPaths
	}
	if errEnsure := ensureHomeCertificateFiles(ctx, claims, paths); errEnsure != nil {
		return config.HomeConfig{}, errEnsure
	}
	return config.HomeConfig{
		Enabled: true,
		Host:    strings.TrimSpace(claims.IP),
		Port:    claims.Port,
		TLS: config.HomeTLSConfig{
			Enable:              true,
			CACert:              paths.CACert,
			ClientCert:          paths.ClientCert,
			ClientKey:           paths.ClientKey,
			UseTargetServerName: true,
		},
	}, nil
}

// parseHomeJWTClaims 解析 Home 注册 JWT 的载荷部分，提取并验证所有必需的声明字段。
func parseHomeJWTClaims(rawJWT string) (homeJWTClaims, error) {
	var claims homeJWTClaims
	parts := strings.Split(strings.TrimSpace(rawJWT), ".")
	if len(parts) != 3 {
		return claims, fmt.Errorf("home jwt is invalid")
	}
	payload, errDecode := decodeJWTPart(parts[1])
	if errDecode != nil {
		return claims, errDecode
	}
	if errUnmarshal := json.Unmarshal(payload, &claims); errUnmarshal != nil {
		return claims, errUnmarshal
	}
	if strings.TrimSpace(claims.CertificateID) == "" {
		return claims, fmt.Errorf("home jwt certificate_id is required")
	}
	if strings.TrimSpace(claims.ClusterID) == "" {
		return claims, fmt.Errorf("home jwt cluster_id is required")
	}
	if normalizeFingerprint(claims.CAFingerprint) == "" {
		return claims, fmt.Errorf("home jwt ca_fingerprint is required")
	}
	if strings.TrimSpace(claims.EnrollmentSecret) == "" {
		return claims, fmt.Errorf("home jwt enrollment_secret is required")
	}
	if strings.TrimSpace(claims.IP) == "" || claims.Port <= 0 {
		return claims, fmt.Errorf("home jwt target address is invalid")
	}
	return claims, nil
}

// decodeJWTPart 解码 JWT 的 Base64 编码部分，先尝试 RawURL 编码，失败后回退到 URL 编码。
func decodeJWTPart(part string) ([]byte, error) {
	if decoded, errDecode := base64.RawURLEncoding.DecodeString(part); errDecode == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(part)
}

// defaultCertificatePaths 返回默认的证书文件路径，位于用户主目录下的 .cli-proxy-api 目录。
func defaultCertificatePaths() (certificatePaths, error) {
	homeDir, errHome := os.UserHomeDir()
	if errHome != nil {
		return certificatePaths{}, errHome
	}
	dir := filepath.Join(homeDir, ".cli-proxy-api")
	return certificatePaths{
		Dir:        dir,
		ClientCert: filepath.Join(dir, "client-crt.pem"),
		ClientKey:  filepath.Join(dir, "client-key.pem"),
		CACert:     filepath.Join(dir, "home-ca-crt.pem"),
	}, nil
}

// ensureHomeCertificateFiles 确保本地 mTLS 证书文件存在且有效。
// 若证书已存在则验证 CA 指纹；否则生成新的密钥对和 CSR 并向 Home 服务端请求签发证书。
func ensureHomeCertificateFiles(ctx context.Context, claims homeJWTClaims, paths certificatePaths) error {
	if fileExists(paths.ClientCert) && fileExists(paths.ClientKey) {
		if !fileExists(paths.CACert) {
			return fmt.Errorf("home ca certificate file is missing")
		}
		if errVerify := verifyCACertificateFile(paths.CACert, claims.CAFingerprint); errVerify != nil {
			return errVerify
		}
		if errChmod := chmodCertificateFiles(paths); errChmod != nil {
			return errChmod
		}
		return nil
	}
	if errMkdir := os.MkdirAll(paths.Dir, 0o700); errMkdir != nil {
		return errMkdir
	}
	key, errKey := loadOrCreateClientKey(paths.ClientKey)
	if errKey != nil {
		return errKey
	}
	csrPEM, errCSR := createClientCSR(claims.CertificateID, key)
	if errCSR != nil {
		return errCSR
	}
	response, errRequest := requestClientCertificate(ctx, claims, csrPEM)
	if errRequest != nil {
		return errRequest
	}
	if strings.TrimSpace(response.Certificate) == "" || strings.TrimSpace(response.CA) == "" {
		return fmt.Errorf("home certificate response is incomplete")
	}
	if errVerify := verifyCACertificatePEM([]byte(response.CA), claims.CAFingerprint); errVerify != nil {
		return errVerify
	}
	if errWrite := writeFile0600(paths.ClientCert, []byte(response.Certificate)); errWrite != nil {
		return errWrite
	}
	if errWrite := writeFile0600(paths.CACert, []byte(response.CA)); errWrite != nil {
		return errWrite
	}
	return nil
}

// verifyCACertificateFile 从文件读取 CA 证书并验证其指纹是否与期望值匹配。
func verifyCACertificateFile(path string, expectedFingerprint string) error {
	raw, errRead := os.ReadFile(path)
	if errRead != nil {
		return errRead
	}
	return verifyCACertificatePEM(raw, expectedFingerprint)
}

// verifyCACertificatePEM 验证 PEM 格式 CA 证书的 SHA-256 指纹是否与期望值匹配。
func verifyCACertificatePEM(raw []byte, expectedFingerprint string) error {
	actual, errFingerprint := certificateFingerprintPEM(raw)
	if errFingerprint != nil {
		return errFingerprint
	}
	expected := normalizeFingerprint(expectedFingerprint)
	if expected == "" {
		return fmt.Errorf("home ca fingerprint is required")
	}
	if actual != expected {
		return fmt.Errorf("home ca fingerprint mismatch")
	}
	return nil
}

// certificateFingerprintPEM 计算 PEM 格式证书的 SHA-256 指纹，返回十六进制编码的字符串。
func certificateFingerprintPEM(raw []byte) (string, error) {
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("home ca certificate pem is invalid")
	}
	cert, errParse := x509.ParseCertificate(block.Bytes)
	if errParse != nil {
		return "", errParse
	}
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:]), nil
}

// normalizeFingerprint 规范化指纹字符串，转为小写并移除冒号和空格分隔符。
func normalizeFingerprint(fingerprint string) string {
	fingerprint = strings.TrimSpace(strings.ToLower(fingerprint))
	fingerprint = strings.ReplaceAll(fingerprint, ":", "")
	fingerprint = strings.ReplaceAll(fingerprint, " ", "")
	return fingerprint
}

// loadOrCreateClientKey 加载已有的 RSA 私钥文件，若不存在则生成新的 2048 位 RSA 密钥对并保存。
func loadOrCreateClientKey(path string) (*rsa.PrivateKey, error) {
	if fileExists(path) {
		raw, errRead := os.ReadFile(path)
		if errRead != nil {
			return nil, errRead
		}
		key, errParse := parseRSAPrivateKeyPEM(raw)
		if errParse != nil {
			return nil, errParse
		}
		if errChmod := os.Chmod(path, 0o600); errChmod != nil {
			return nil, errChmod
		}
		return key, nil
	}
	key, errKey := rsa.GenerateKey(rand.Reader, 2048)
	if errKey != nil {
		return nil, errKey
	}
	raw := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if errWrite := writeFile0600(path, raw); errWrite != nil {
		return nil, errWrite
	}
	return key, nil
}

// writeFile0600 以 0600 权限写入文件，确保只有文件所有者可读写。
func writeFile0600(path string, raw []byte) error {
	if errWrite := os.WriteFile(path, raw, 0o600); errWrite != nil {
		return errWrite
	}
	return os.Chmod(path, 0o600)
}

// chmodCertificateFiles 将所有证书文件的权限设置为 0600。
func chmodCertificateFiles(paths certificatePaths) error {
	for _, path := range []string{paths.ClientCert, paths.ClientKey, paths.CACert} {
		if errChmod := os.Chmod(path, 0o600); errChmod != nil {
			return errChmod
		}
	}
	return nil
}

// parseRSAPrivateKeyPEM 解析 PEM 格式的 RSA 私钥，支持 PKCS1 和 PKCS8 两种格式。
func parseRSAPrivateKeyPEM(raw []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("client key pem is invalid")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, errParse := x509.ParsePKCS8PrivateKey(block.Bytes)
		if errParse != nil {
			return nil, errParse
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("client key is not rsa")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("client key pem type %q is unsupported", block.Type)
	}
}

// createClientCSR 使用证书 ID 作为 Common Name 创建 PKCS#10 证书签名请求。
func createClientCSR(certificateID string, key *rsa.PrivateKey) ([]byte, error) {
	certificateID = strings.TrimSpace(certificateID)
	if certificateID == "" {
		return nil, fmt.Errorf("certificate id is required")
	}
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: certificateID,
		},
	}
	der, errCreate := x509.CreateCertificateRequest(rand.Reader, template, key)
	if errCreate != nil {
		return nil, errCreate
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

// requestClientCertificate 通过 TCP 连接向 Home 服务端发送 RESP 格式的证书请求，
// 使用注册密钥和 CSR 获取签发的客户端证书。
func requestClientCertificate(ctx context.Context, claims homeJWTClaims, csrPEM []byte) (certificateRequestResponse, error) {
	var response certificateRequestResponse
	if ctx == nil {
		ctx = context.Background()
	}
	dialCtx, cancel := context.WithTimeout(ctx, homeCertificateRequestTimeout)
	defer cancel()
	addr := net.JoinHostPort(strings.TrimSpace(claims.IP), strconv.Itoa(claims.Port))
	conn, errDial := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if errDial != nil {
		return response, errDial
	}
	defer func() {
		_ = conn.Close()
	}()
	if deadline, ok := dialCtx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, errWrite := conn.Write(encodeRESPArray("CERTIFICATE", "REQUEST", claims.CertificateID, claims.EnrollmentSecret, string(csrPEM))); errWrite != nil {
		return response, errWrite
	}
	raw, errRead := readRESPBulk(bufio.NewReader(conn))
	if errRead != nil {
		return response, errRead
	}
	if errUnmarshal := json.Unmarshal(raw, &response); errUnmarshal != nil {
		return response, errUnmarshal
	}
	if !response.OK {
		return response, fmt.Errorf("home certificate request failed")
	}
	return response, nil
}

// encodeRESPArray 将字符串参数编码为 RESP（Redis 序列化协议）数组格式。
func encodeRESPArray(args ...string) []byte {
	var buf bytes.Buffer
	buf.WriteString("*")
	buf.WriteString(strconv.Itoa(len(args)))
	buf.WriteString("\r\n")
	for _, arg := range args {
		buf.WriteString("$")
		buf.WriteString(strconv.Itoa(len(arg)))
		buf.WriteString("\r\n")
		buf.WriteString(arg)
		buf.WriteString("\r\n")
	}
	return buf.Bytes()
}

// readRESPBulk 从 RESP 流中读取一个 Bulk String 响应，支持正常数据和错误响应。
func readRESPBulk(reader *bufio.Reader) ([]byte, error) {
	prefix, errRead := reader.ReadByte()
	if errRead != nil {
		return nil, errRead
	}
	switch prefix {
	case '$':
		line, errLine := reader.ReadString('\n')
		if errLine != nil {
			return nil, errLine
		}
		size, errSize := strconv.Atoi(strings.TrimSpace(line))
		if errSize != nil {
			return nil, errSize
		}
		if size < 0 {
			return nil, fmt.Errorf("home certificate request returned nil")
		}
		payload := make([]byte, size+2)
		if _, errFull := io.ReadFull(reader, payload); errFull != nil {
			return nil, errFull
		}
		return payload[:size], nil
	case '-':
		line, errLine := reader.ReadString('\n')
		if errLine != nil {
			return nil, errLine
		}
		return nil, fmt.Errorf("%s", strings.TrimSpace(line))
	default:
		return nil, fmt.Errorf("home certificate request returned unsupported resp prefix %q", prefix)
	}
}

// fileExists 检查指定路径的文件是否存在且不是目录。
func fileExists(path string) bool {
	info, errStat := os.Stat(path)
	return errStat == nil && !info.IsDir()
}
