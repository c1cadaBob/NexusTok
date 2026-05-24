// 包 home - certificate.go
// 该文件实现了 Home 控制平面的 mTLS 证书管理功能。
// 包括从 JWT 解析证书请求、创建 CSR、向 Home 服务器请求客户端证书、
// 验证 CA 证书指纹等功能。
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

// homeCertificateRequestTimeout 是证书请求的超时时间
const homeCertificateRequestTimeout = 30 * time.Second

// homeJWTClaims 包含从 Home JWT 中解析的证书请求声明。
type homeJWTClaims struct {
	// CertificateID 是证书标识符
	CertificateID string `json:"certificate_id"`
	// ClusterID 是集群标识符
	ClusterID string `json:"cluster_id"`
	// CAFingerprint 是 CA 证书的 SHA256 指纹
	CAFingerprint string `json:"ca_fingerprint"`
	// EnrollmentSecret 是注册密钥
	EnrollmentSecret string `json:"enrollment_secret"`
	// IP 是 Home 服务器的 IP 地址
	IP string `json:"ip"`
	// Port 是 Home 服务器的端口
	Port int `json:"port"`
	// IssuedAt 是 JWT 签发时间
	IssuedAt int64 `json:"iat"`
}

// certificateRequestResponse 是证书请求的响应结构。
type certificateRequestResponse struct {
	// OK 指示请求是否成功
	OK bool `json:"ok"`
	// Certificate 是客户端证书 PEM
	Certificate string `json:"certificate"`
	// CA 是 CA 证书 PEM
	CA string `json:"ca"`
}

// certificatePaths 存储证书文件的路径。
type certificatePaths struct {
	// Dir 是证书目录
	Dir string
	// ClientCert 是客户端证书文件路径
	ClientCert string
	// ClientKey 是客户端私钥文件路径
	ClientKey string
	// CACert 是 CA 证书文件路径
	CACert string
}

// ConfigFromJWT 从 JWT 准备 Home 配置并确保本地 mTLS 文件存在。
//
// 参数：
//   - ctx: 请求的上下文
//   - rawJWT: 原始 JWT 字符串
//
// 返回：
//   - config.HomeConfig: Home 配置
//   - error: 操作失败时返回的错误
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

// parseHomeJWTClaims 从原始 JWT 字符串解析 Home JWT 声明。
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

// decodeJWTPart 解码 JWT 部分，支持标准和原始 URL 编码。
func decodeJWTPart(part string) ([]byte, error) {
	if decoded, errDecode := base64.RawURLEncoding.DecodeString(part); errDecode == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(part)
}

// defaultCertificatePaths 返回默认的证书文件路径。
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

// ensureHomeCertificateFiles 确保 Home 证书文件存在，如果不存在则创建。
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

// verifyCACertificateFile 验证 CA 证书文件的指纹。
func verifyCACertificateFile(path string, expectedFingerprint string) error {
	raw, errRead := os.ReadFile(path)
	if errRead != nil {
		return errRead
	}
	return verifyCACertificatePEM(raw, expectedFingerprint)
}

// verifyCACertificatePEM 验证 CA 证书 PEM 的指纹。
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

// certificateFingerprintPEM 计算 PEM 编码证书的 SHA256 指纹。
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

// normalizeFingerprint 规范化指纹字符串，移除冒号和空格并转换为小写。
func normalizeFingerprint(fingerprint string) string {
	fingerprint = strings.TrimSpace(strings.ToLower(fingerprint))
	fingerprint = strings.ReplaceAll(fingerprint, ":", "")
	fingerprint = strings.ReplaceAll(fingerprint, " ", "")
	return fingerprint
}

// loadOrCreateClientKey 加载或创建客户端 RSA 私钥。
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

// writeFile0600 以 0600 权限写入文件。
func writeFile0600(path string, raw []byte) error {
	if errWrite := os.WriteFile(path, raw, 0o600); errWrite != nil {
		return errWrite
	}
	return os.Chmod(path, 0o600)
}

// chmodCertificateFiles 设置证书文件的权限为 0600。
func chmodCertificateFiles(paths certificatePaths) error {
	for _, path := range []string{paths.ClientCert, paths.ClientKey, paths.CACert} {
		if errChmod := os.Chmod(path, 0o600); errChmod != nil {
			return errChmod
		}
	}
	return nil
}

// parseRSAPrivateKeyPEM 解析 PEM 编码的 RSA 私钥。
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

// createClientCSR 创建客户端证书签名请求。
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

// requestClientCertificate 向 Home 服务器请求客户端证书。
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

// encodeRESPArray 编码 RESP 数组格式。
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

// readRESPBulk 读取 RESP 批量字符串响应。
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

// fileExists 检查文件是否存在。
func fileExists(path string) bool {
	info, errStat := os.Stat(path)
	return errStat == nil && !info.IsDir()
}
