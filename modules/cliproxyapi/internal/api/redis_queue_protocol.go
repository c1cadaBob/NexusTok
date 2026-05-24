// api - redis_queue_protocol.go
// 嵌入式 Redis RESP 协议服务器实现，用于提供使用统计数据的实时订阅和队列消费。
// 该模块实现了一个轻量级的 Redis 协议子集，支持以下命令：
//   - AUTH: 客户端认证（复用管理 API 的认证机制）
//   - SUBSCRIBE: 订阅使用统计频道，实时接收新的使用记录
//   - LPOP/RPOP: 从使用统计队列中弹出记录
//
// 这使得现有的 Redis 客户端库可以直接连接到本服务获取使用统计数据，
// 无需引入额外的消息队列中间件。
package api

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/redisqueue"
	log "github.com/sirupsen/logrus"
)

// redisUsageChannel 是使用统计数据的 Redis Pub/Sub 频道名称。
const redisUsageChannel = "usage"

// redisSubscriptionCommand 表示在 Redis 订阅模式下接收到的命令。
// 包含解析后的参数列表和可能的读取错误。
type redisSubscriptionCommand struct {
	args []string // 命令参数列表
	err  error    // 读取命令时发生的错误（如连接断开）
}

// isRedisRESPPrefix 检查给定字节是否为 Redis RESP 协议的有效前缀。
// RESP 协议定义了以下类型前缀：
//   - '*': 数组（Array）
//   - '$': 批量字符串（Bulk String）
//   - '+': 简单字符串（Simple String）
//   - '-': 错误（Error）
//   - ':': 整数（Integer）
func isRedisRESPPrefix(prefix byte) bool {
	switch prefix {
	case '*', '$', '+', '-', ':':
		return true
	default:
		return false
	}
}

// handleRedisConnection 处理单个 Redis 客户端连接的完整生命周期。
// 该方法实现了 Redis 协议的命令处理循环，支持以下命令：
//   - AUTH: 使用管理 API 的密钥进行认证
//   - SUBSCRIBE: 订阅使用统计频道，进入消息推送模式
//   - LPOP/RPOP: 从使用统计队列中弹出指定数量的记录
//
// 连接需要先通过 AUTH 认证后才能执行其他命令。
// 在 Home 模式下，Redis 使用统计输出被禁用。
func (s *Server) handleRedisConnection(conn net.Conn, reader *bufio.Reader) {
	if s == nil || conn == nil {
		return
	}
	if reader == nil {
		reader = bufio.NewReader(conn)
	}

	clientIP, localClient := resolveRemoteIP(conn.RemoteAddr())
	authed := false
	writer := bufio.NewWriter(conn)
	defer func() {
		if errClose := conn.Close(); errClose != nil {
			log.Errorf("redis connection close error: %v", errClose)
		}
	}()

	flush := func() bool {
		if errFlush := writer.Flush(); errFlush != nil {
			log.Errorf("redis protocol flush error: %v", errFlush)
			return false
		}
		return true
	}

	if s.cfg != nil && s.cfg.Home.Enabled {
		_ = writeRedisError(writer, "ERR redis usage output disabled in home mode")
		_ = writer.Flush()
		return
	}

	for {
		if !s.managementRoutesEnabled.Load() {
			return
		}

		args, errRead := readRESPArray(reader)
		if errRead != nil {
			if !errors.Is(errRead, io.EOF) {
				_ = writeRedisError(writer, "ERR "+errRead.Error())
				_ = writer.Flush()
			}
			return
		}
		if len(args) == 0 {
			_ = writeRedisError(writer, "ERR empty command")
			if !flush() {
				return
			}
			continue
		}

		cmd := strings.ToUpper(strings.TrimSpace(args[0]))

		if cmd != "AUTH" && !authed {
			if s.mgmt != nil {
				_, statusCode, errMsg := s.mgmt.AuthenticateManagementKey(clientIP, localClient, "")
				if statusCode == http.StatusForbidden && strings.HasPrefix(errMsg, "IP banned due to too many failed attempts") {
					_ = writeRedisError(writer, "ERR "+errMsg)
				} else {
					_ = writeRedisError(writer, "NOAUTH Authentication required.")
				}
			} else {
				_ = writeRedisError(writer, "NOAUTH Authentication required.")
			}
			if !flush() {
				return
			}
			continue
		}

		switch cmd {
		case "AUTH":
			password, ok := parseAuthPassword(args)
			if !ok {
				if s.mgmt != nil {
					_, statusCode, errMsg := s.mgmt.AuthenticateManagementKey(clientIP, localClient, "")
					if statusCode == http.StatusForbidden && strings.HasPrefix(errMsg, "IP banned due to too many failed attempts") {
						_ = writeRedisError(writer, "ERR "+errMsg)
						if !flush() {
							return
						}
						continue
					}
				}
				_ = writeRedisError(writer, "ERR wrong number of arguments for 'auth' command")
				if !flush() {
					return
				}
				continue
			}
			if s.mgmt == nil {
				_ = writeRedisError(writer, "ERR remote management disabled")
				if !flush() {
					return
				}
				continue
			}
			allowed, _, errMsg := s.mgmt.AuthenticateManagementKey(clientIP, localClient, password)
			if !allowed {
				_ = writeRedisError(writer, "ERR "+errMsg)
				if !flush() {
					return
				}
				continue
			}
			authed = true
			_ = writeRedisSimpleString(writer, "OK")
			if !flush() {
				return
			}
		case "SUBSCRIBE":
			channel, ok := parseSubscribeChannel(args)
			if !ok {
				_ = writeRedisError(writer, "ERR wrong number of arguments for 'subscribe' command")
				if !flush() {
					return
				}
				continue
			}
			if !strings.EqualFold(channel, redisUsageChannel) {
				_ = writeRedisError(writer, fmt.Sprintf("ERR unsupported channel '%s'", channel))
				if !flush() {
					return
				}
				continue
			}
			messages, unsubscribe := redisqueue.SubscribeUsage()
			if errWrite := writeRedisPubSubSubscribe(writer, redisUsageChannel, 1); errWrite != nil {
				unsubscribe()
				log.Errorf("redis protocol subscribe response error: %v", errWrite)
				return
			}
			if !flush() {
				unsubscribe()
				return
			}
			s.streamRedisUsageSubscription(reader, writer, messages, unsubscribe)
			return
		case "LPOP", "RPOP":
			count, hasCount, ok := parsePopCount(args)
			if !ok {
				_ = writeRedisError(writer, "ERR wrong number of arguments for '"+strings.ToLower(cmd)+"' command")
				if !flush() {
					return
				}
				continue
			}
			if count <= 0 {
				_ = writeRedisError(writer, "ERR value is not an integer or out of range")
				if !flush() {
					return
				}
				continue
			}
			items := redisqueue.PopOldest(count)
			if hasCount {
				_ = writeRedisArrayOfBulkStrings(writer, items)
				if !flush() {
					return
				}
				continue
			}
			if len(items) == 0 {
				_ = writeRedisNilBulkString(writer)
				if !flush() {
					return
				}
				continue
			}
			_ = writeRedisBulkString(writer, items[0])
			if !flush() {
				return
			}
		default:
			_ = writeRedisError(writer, fmt.Sprintf("ERR unknown command '%s'", strings.ToLower(cmd)))
			if !flush() {
				return
			}
		}
	}
}

// streamRedisUsageSubscription 处理 Redis Pub/Sub 订阅模式下的消息推送。
// 该方法在 SUBSCRIBE 命令成功后被调用，负责：
//  1. 持续监听使用统计消息通道，将新消息推送给客户端
//  2. 处理客户端在订阅期间发送的命令（PING、UNSUBSCRIBE、QUIT）
//  3. 在连接关闭或取消订阅时清理资源
//
// 使用独立的 goroutine 读取客户端命令，避免消息推送和命令读取之间的阻塞。
func (s *Server) streamRedisUsageSubscription(reader *bufio.Reader, writer *bufio.Writer, messages <-chan []byte, unsubscribe func()) {
	if unsubscribe == nil {
		return
	}
	defer unsubscribe()

	done := make(chan struct{})
	defer close(done)

	commands := make(chan redisSubscriptionCommand, 1)
	go readRedisSubscriptionCommands(reader, commands, done)

	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				return
			}
			if errWrite := writeRedisPubSubMessage(writer, redisUsageChannel, msg); errWrite != nil {
				log.Errorf("redis protocol publish message error: %v", errWrite)
				return
			}
			if errFlush := writer.Flush(); errFlush != nil {
				log.Errorf("redis protocol flush error: %v", errFlush)
				return
			}
		case command, ok := <-commands:
			if !ok {
				return
			}
			keepOpen := handleRedisSubscriptionCommand(writer, command)
			if errFlush := writer.Flush(); errFlush != nil {
				log.Errorf("redis protocol flush error: %v", errFlush)
				return
			}
			if !keepOpen {
				return
			}
		}
	}
}

// readRedisSubscriptionCommands 在独立 goroutine 中持续读取客户端在订阅模式下发送的命令。
// 读取到的命令通过 commands 通道发送给主循环处理。
// 当连接关闭或 done 通道被关闭时，goroutine 退出并关闭 commands 通道。
func readRedisSubscriptionCommands(reader *bufio.Reader, commands chan<- redisSubscriptionCommand, done <-chan struct{}) {
	defer close(commands)

	for {
		args, errRead := readRESPArray(reader)
		if errRead != nil {
			if !errors.Is(errRead, io.EOF) {
				select {
				case commands <- redisSubscriptionCommand{err: errRead}:
				case <-done:
				}
			}
			return
		}
		select {
		case commands <- redisSubscriptionCommand{args: args}:
		case <-done:
			return
		}
	}
}

// handleRedisSubscriptionCommand 处理订阅模式下接收到的客户端命令。
// 支持的命令：
//   - PING: 返回 PONG 响应，用于心跳检测
//   - UNSUBSCRIBE: 取消订阅，返回 false 以终止订阅循环
//   - QUIT: 关闭连接，返回 false 以终止订阅循环
//
// 返回值表示是否继续保持订阅连接。
func handleRedisSubscriptionCommand(writer *bufio.Writer, command redisSubscriptionCommand) bool {
	if command.err != nil {
		_ = writeRedisError(writer, "ERR "+command.err.Error())
		return false
	}
	if len(command.args) == 0 {
		_ = writeRedisError(writer, "ERR empty command")
		return true
	}

	cmd := strings.ToUpper(strings.TrimSpace(command.args[0]))
	switch cmd {
	case "PING":
		payload := []byte(nil)
		if len(command.args) > 1 {
			payload = []byte(command.args[1])
		}
		_ = writeRedisPubSubPong(writer, payload)
		return true
	case "UNSUBSCRIBE":
		_ = writeRedisPubSubUnsubscribe(writer, redisUsageChannel, 0)
		return false
	case "QUIT":
		_ = writeRedisSimpleString(writer, "OK")
		return false
	default:
		_ = writeRedisError(writer, fmt.Sprintf("ERR unknown command '%s'", strings.ToLower(cmd)))
		return true
	}
}

// resolveRemoteIP 从网络地址中解析出客户端 IP 地址。
// 支持 TCPAddr 类型的直接解析和其他地址类型的字符串解析。
// 同时判断客户端是否为本地连接（127.0.0.1 或 ::1）。
// 返回解析后的 IP 字符串和是否为本地客户端的标志。
func resolveRemoteIP(addr net.Addr) (ip string, localClient bool) {
	if addr == nil {
		return "", false
	}

	var host string
	switch a := addr.(type) {
	case *net.TCPAddr:
		if a != nil && a.IP != nil {
			if ip4 := a.IP.To4(); ip4 != nil {
				host = ip4.String()
			} else {
				host = a.IP.String()
			}
		}
	default:
		host = addr.String()
		if h, _, errSplit := net.SplitHostPort(host); errSplit == nil {
			host = h
		}
		host = strings.TrimSpace(host)
		if raw, _, ok := strings.Cut(host, "%"); ok {
			host = raw
		}
		if parsed := net.ParseIP(host); parsed != nil {
			if ip4 := parsed.To4(); ip4 != nil {
				host = ip4.String()
			} else {
				host = parsed.String()
			}
		}
	}

	host = strings.TrimSpace(host)
	localClient = host == "127.0.0.1" || host == "::1"
	return host, localClient
}

// parseAuthPassword 从 AUTH 命令参数中提取密码。
// 支持两种格式：AUTH password（2个参数）和 AUTH username password（3个参数）。
// 返回密码字符串和解析是否成功。
func parseAuthPassword(args []string) (string, bool) {
	switch len(args) {
	case 2:
		return args[1], true
	case 3:
		return args[2], true
	default:
		return "", false
	}
}

// parseSubscribeChannel 从 SUBSCRIBE 命令参数中提取频道名称。
// 要求恰好有 2 个参数（命令名 + 频道名），否则返回解析失败。
func parseSubscribeChannel(args []string) (string, bool) {
	if len(args) != 2 {
		return "", false
	}
	return strings.TrimSpace(args[1]), true
}

// parsePopCount 从 LPOP/RPOP 命令参数中提取弹出数量。
// 支持两种格式：LPOP key（无数量参数，默认弹出1个）和 LPOP key count（指定数量）。
// 返回弹出数量、是否有数量参数、以及解析是否成功。
func parsePopCount(args []string) (count int, hasCount bool, ok bool) {
	if len(args) != 2 && len(args) != 3 {
		return 0, false, false
	}
	if len(args) == 2 {
		return 1, false, true
	}
	parsed, errParse := strconv.Atoi(strings.TrimSpace(args[2]))
	if errParse != nil {
		return 0, true, true
	}
	return parsed, true, true
}

// readRESPArray 从 RESP 数据流中读取一个数组。
// RESP 数组格式为：*<count>\r\n<element1>...<elementN>
// 返回解析后的字符串数组，如果格式不正确则返回错误。
func readRESPArray(reader *bufio.Reader) ([]string, error) {
	prefix, errRead := reader.ReadByte()
	if errRead != nil {
		return nil, errRead
	}
	if prefix != '*' {
		return nil, fmt.Errorf("protocol error")
	}
	line, errLine := readRESPLine(reader)
	if errLine != nil {
		return nil, errLine
	}
	count, errParse := strconv.Atoi(line)
	if errParse != nil || count < 0 {
		return nil, fmt.Errorf("protocol error")
	}
	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		value, errString := readRESPString(reader)
		if errString != nil {
			return nil, errString
		}
		args = append(args, value)
	}
	return args, nil
}

// readRESPString 从 RESP 数据流中读取一个字符串。
// 根据前缀字节判断类型并分派到对应的读取函数：
//   - '$': 批量字符串（Bulk String）
//   - '+' 或 ':': 简单字符串或整数
//
// 其他前缀返回协议错误。
func readRESPString(reader *bufio.Reader) (string, error) {
	prefix, errRead := reader.ReadByte()
	if errRead != nil {
		return "", errRead
	}
	switch prefix {
	case '$':
		return readRESPBulkString(reader)
	case '+', ':':
		return readRESPLine(reader)
	default:
		return "", fmt.Errorf("protocol error")
	}
}

// readRESPBulkString 从 RESP 数据流中读取一个批量字符串。
// 格式为：$<length>\r\n<data>\r\n
// 长度为 -1 表示空值（nil bulk string），返回空字符串。
// 读取时会验证结尾的 \r\n 分隔符，格式不正确则返回错误。
func readRESPBulkString(reader *bufio.Reader) (string, error) {
	line, errLine := readRESPLine(reader)
	if errLine != nil {
		return "", errLine
	}
	length, errParse := strconv.Atoi(line)
	if errParse != nil {
		return "", fmt.Errorf("protocol error")
	}
	if length < 0 {
		return "", nil
	}
	buf := make([]byte, length+2)
	if _, errRead := io.ReadFull(reader, buf); errRead != nil {
		return "", errRead
	}
	if length+2 < 2 || buf[length] != '\r' || buf[length+1] != '\n' {
		return "", fmt.Errorf("protocol error")
	}
	return string(buf[:length]), nil
}

// readRESPLine 从 RESP 数据流中读取一行（以 \r\n 结尾）。
// 去除行尾的 \n 和 \r 字符后返回内容字符串。
func readRESPLine(reader *bufio.Reader) (string, error) {
	line, errRead := reader.ReadString('\n')
	if errRead != nil {
		return "", errRead
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, nil
}

// writeRedisSimpleString 向 RESP 数据流写入一个简单字符串。
// 格式为：+<value>\r\n
// 简单字符串用于表示状态响应，如 "OK"。
func writeRedisSimpleString(writer *bufio.Writer, value string) error {
	if writer == nil {
		return net.ErrClosed
	}
	_, errWrite := writer.WriteString("+" + value + "\r\n")
	return errWrite
}

// writeRedisError 向 RESP 数据流写入一个错误响应。
// 格式为：-<message>\r\n
// 错误响应用于表示命令执行失败或协议错误。
func writeRedisError(writer *bufio.Writer, message string) error {
	if writer == nil {
		return net.ErrClosed
	}
	_, errWrite := writer.WriteString("-" + message + "\r\n")
	return errWrite
}

// writeRedisNilBulkString 向 RESP 数据流写入一个空批量字符串。
// 格式为：$-1\r\n
// 用于表示空值或不存在的键。
func writeRedisNilBulkString(writer *bufio.Writer) error {
	if writer == nil {
		return net.ErrClosed
	}
	_, errWrite := writer.WriteString("$-1\r\n")
	return errWrite
}

// writeRedisBulkString 向 RESP 数据流写入一个批量字符串。
// 格式为：$<length>\r\n<data>\r\n
// 如果 payload 为 nil，则写入空批量字符串。
func writeRedisBulkString(writer *bufio.Writer, payload []byte) error {
	if writer == nil {
		return net.ErrClosed
	}
	if payload == nil {
		return writeRedisNilBulkString(writer)
	}
	if _, errWrite := writer.WriteString("$" + strconv.Itoa(len(payload)) + "\r\n"); errWrite != nil {
		return errWrite
	}
	if _, errWrite := writer.Write(payload); errWrite != nil {
		return errWrite
	}
	_, errWrite := writer.WriteString("\r\n")
	return errWrite
}

// writeRedisArrayOfBulkStrings 向 RESP 数据流写入一个批量字符串数组。
// 格式为：*<count>\r\n$<len1>\r\n<data1>\r\n...$<lenN>\r\n<dataN>\r\n
// 用于 LPOP/RPOP 命令返回多条记录的场景。
func writeRedisArrayOfBulkStrings(writer *bufio.Writer, items [][]byte) error {
	if writer == nil {
		return net.ErrClosed
	}
	if _, errWrite := writer.WriteString("*" + strconv.Itoa(len(items)) + "\r\n"); errWrite != nil {
		return errWrite
	}
	for i := range items {
		if errWrite := writeRedisBulkString(writer, items[i]); errWrite != nil {
			return errWrite
		}
	}
	return nil
}

// writeRedisInteger 向 RESP 数据流写入一个整数。
// 格式为：:<value>\r\n
// 用于返回计数或状态码等整数值。
func writeRedisInteger(writer *bufio.Writer, value int) error {
	if writer == nil {
		return net.ErrClosed
	}
	_, errWrite := writer.WriteString(":" + strconv.Itoa(value) + "\r\n")
	return errWrite
}

// writeRedisArrayHeader 向 RESP 数据流写入数组头部。
// 格式为：*<count>\r\n
// 后续需要写入 count 个元素来完成数组。
func writeRedisArrayHeader(writer *bufio.Writer, count int) error {
	if writer == nil {
		return net.ErrClosed
	}
	_, errWrite := writer.WriteString("*" + strconv.Itoa(count) + "\r\n")
	return errWrite
}

// writeRedisPubSubSubscribe 向 RESP 数据流写入 SUBSCRIBE 命令的响应。
// 格式为数组：["subscribe", <channel>, <count>]
// 这是 Redis Pub/Sub 协议的标准订阅确认响应。
func writeRedisPubSubSubscribe(writer *bufio.Writer, channel string, count int) error {
	if errWrite := writeRedisArrayHeader(writer, 3); errWrite != nil {
		return errWrite
	}
	if errWrite := writeRedisBulkString(writer, []byte("subscribe")); errWrite != nil {
		return errWrite
	}
	if errWrite := writeRedisBulkString(writer, []byte(channel)); errWrite != nil {
		return errWrite
	}
	return writeRedisInteger(writer, count)
}

// writeRedisPubSubUnsubscribe 向 RESP 数据流写入 UNSUBSCRIBE 命令的响应。
// 格式为数组：["unsubscribe", <channel>, <count>]
// 这是 Redis Pub/Sub 协议的标准取消订阅确认响应。
func writeRedisPubSubUnsubscribe(writer *bufio.Writer, channel string, count int) error {
	if errWrite := writeRedisArrayHeader(writer, 3); errWrite != nil {
		return errWrite
	}
	if errWrite := writeRedisBulkString(writer, []byte("unsubscribe")); errWrite != nil {
		return errWrite
	}
	if errWrite := writeRedisBulkString(writer, []byte(channel)); errWrite != nil {
		return errWrite
	}
	return writeRedisInteger(writer, count)
}

// writeRedisPubSubMessage 向 RESP 数据流写入一条 Pub/Sub 消息。
// 格式为数组：["message", <channel>, <payload>]
// 当订阅的频道有新消息时，服务器通过此格式推送给客户端。
func writeRedisPubSubMessage(writer *bufio.Writer, channel string, payload []byte) error {
	if errWrite := writeRedisArrayHeader(writer, 3); errWrite != nil {
		return errWrite
	}
	if errWrite := writeRedisBulkString(writer, []byte("message")); errWrite != nil {
		return errWrite
	}
	if errWrite := writeRedisBulkString(writer, []byte(channel)); errWrite != nil {
		return errWrite
	}
	return writeRedisBulkString(writer, payload)
}

// writeRedisPubSubPong 向 RESP 数据流写入 PING 命令的 PONG 响应。
// 格式为数组：["pong", <payload>]
// 在订阅模式下，客户端发送 PING 时服务器返回此响应用于心跳检测。
func writeRedisPubSubPong(writer *bufio.Writer, payload []byte) error {
	if errWrite := writeRedisArrayHeader(writer, 2); errWrite != nil {
		return errWrite
	}
	if errWrite := writeRedisBulkString(writer, []byte("pong")); errWrite != nil {
		return errWrite
	}
	return writeRedisBulkString(writer, payload)
}
