// 火山引擎 WebSocket 二进制协议实现文件。
// 实现了火山引擎 TTS（语音合成）服务的自定义二进制消息协议，
// 包含消息的序列化/反序列化、事件类型定义、WebSocket 消息收发等功能。
// 协议格式为：头部（版本、消息类型、序列化方式、压缩方式）+ 事件/会话/序列号 + 载荷。
package volcengine

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	// 第三方依赖
	"github.com/gorilla/websocket"
)

// 以下是协议中使用的位域类型定义，用于二进制消息头的解析和构建。

type (
	EventType         int32  // 事件类型，标识消息的业务语义
	MsgType           uint8  // 消息类型，标识消息的整体类别（如客户端请求、服务端响应、错误等）
	MsgTypeFlagBits   uint8  // 消息类型标志位，标识是否有事件头、是否有序列号等
	VersionBits       uint8  // 协议版本号
	HeaderSizeBits    uint8  // 头部大小（以 4 字节为单位）
	SerializationBits uint8  // 序列化方式（JSON 等）
	CompressionBits   uint8  // 压缩方式（无压缩等）
)

// 消息类型标志位常量，用于 MsgTypeFlagBits。
const (
	MsgTypeFlagNoSeq       MsgTypeFlagBits = 0      // 无序列号
	MsgTypeFlagPositiveSeq MsgTypeFlagBits = 0b1    // 正序列号（有序消息）
	MsgTypeFlagNegativeSeq MsgTypeFlagBits = 0b11   // 负序列号（用于标识最后一条消息）
	MsgTypeFlagWithEvent   MsgTypeFlagBits = 0b100  // 包含事件头
)

// 协议版本常量。
const (
	Version1 VersionBits = iota + 1 // 版本 1
)

// 头部大小常量（以 4 字节为单位）。
const (
	HeaderSize4 HeaderSizeBits = iota + 1 // 头部大小为 4 字节
)

// 序列化方式常量。
const (
	SerializationJSON SerializationBits = 0b1 // JSON 序列化
)

// 压缩方式常量。
const (
	CompressionNone CompressionBits = 0 // 无压缩
)

// 消息类型常量，标识消息的方向和用途。
const (
	MsgTypeFullClientRequest    MsgType = 0b1    // 客户端完整请求
	MsgTypeAudioOnlyClient      MsgType = 0b10   // 客户端纯音频数据
	MsgTypeFullServerResponse   MsgType = 0b1001 // 服务端完整响应
	MsgTypeAudioOnlyServer      MsgType = 0b1011 // 服务端纯音频数据
	MsgTypeFrontEndResultServer MsgType = 0b1100 // 服务端前端结果（如文字转写）
	MsgTypeError                MsgType = 0b1111 // 错误消息
)

// String 返回消息类型的可读字符串表示，用于日志和调试。
// 返回:
//   - string: 消息类型名称
	switch t {
	case MsgTypeFullClientRequest:
		return "MsgType_FullClientRequest"
	case MsgTypeAudioOnlyClient:
		return "MsgType_AudioOnlyClient"
	case MsgTypeFullServerResponse:
		return "MsgType_FullServerResponse"
	case MsgTypeAudioOnlyServer:
		return "MsgType_AudioOnlyServer"
	case MsgTypeError:
		return "MsgType_Error"
	case MsgTypeFrontEndResultServer:
		return "MsgType_FrontEndResultServer"
	default:
		return fmt.Sprintf("MsgType_(%d)", t)
	}
}

// 事件类型常量，定义了火山引擎协议中所有支持的事件。
// 包括连接管理、会话管理、TTS 语音合成、ASR 语音识别、对话等事件。
const (
	EventType_None EventType = 0 // 空事件

	// 连接生命周期事件
	EventType_StartConnection  EventType = 1   // 开始连接
	EventType_FinishConnection EventType = 2   // 结束连接
	EventType_ConnectionStarted  EventType = 50 // 连接已建立
	EventType_ConnectionFailed   EventType = 51 // 连接失败
	EventType_ConnectionFinished EventType = 52 // 连接已完成

	// 会话生命周期事件
	EventType_StartSession  EventType = 100 // 开始会话
	EventType_CancelSession EventType = 101 // 取消会话
	EventType_FinishSession EventType = 102 // 结束会话
	EventType_SessionStarted  EventType = 150 // 会话已开始
	EventType_SessionCanceled EventType = 151 // 会话已取消
	EventType_SessionFinished EventType = 152 // 会话已完成
	EventType_SessionFailed   EventType = 153 // 会话失败
	EventType_UsageResponse   EventType = 154 // 使用量响应

	// 任务和配置事件
	EventType_TaskRequest  EventType = 200 // 任务请求
	EventType_UpdateConfig EventType = 201 // 更新配置
	EventType_AudioMuted   EventType = 250 // 音频静音
	EventType_SayHello     EventType = 300 // 打招呼

	// TTS 语音合成事件
	EventType_TTSSentenceStart     EventType = 350 // TTS 句子开始
	EventType_TTSSentenceEnd       EventType = 351 // TTS 句子结束
	EventType_TTSResponse          EventType = 352 // TTS 响应数据
	EventType_TTSEnded             EventType = 359 // TTS 结束
	EventType_PodcastRoundStart    EventType = 360 // 播客轮次开始
	EventType_PodcastRoundResponse EventType = 361 // 播客轮次响应
	EventType_PodcastRoundEnd      EventType = 362 // 播客轮次结束

	// ASR 语音识别事件
	EventType_ASRInfo     EventType = 450 // ASR 信息
	EventType_ASRResponse EventType = 451 // ASR 响应
	EventType_ASREnded    EventType = 459 // ASR 结束

	// 对话事件
	EventType_ChatTTSText EventType = 500 // 对话 TTS 文本
	EventType_ChatResponse EventType = 550 // 对话响应
	EventType_ChatEnded    EventType = 559 // 对话结束

	// 字幕事件
	EventType_SourceSubtitleStart    EventType = 650 // 源字幕开始
	EventType_SourceSubtitleResponse EventType = 651 // 源字幕响应
	EventType_SourceSubtitleEnd      EventType = 652 // 源字幕结束
	EventType_TranslationSubtitleStart    EventType = 653 // 翻译字幕开始
	EventType_TranslationSubtitleResponse EventType = 654 // 翻译字幕响应
	EventType_TranslationSubtitleEnd      EventType = 655 // 翻译字幕结束
)

// String 返回事件类型的可读字符串表示，用于日志和调试。
// 返回:
//   - string: 事件类型名称
	switch t {
	case EventType_None:
		return "EventType_None"
	case EventType_StartConnection:
		return "EventType_StartConnection"
	case EventType_FinishConnection:
		return "EventType_FinishConnection"
	case EventType_ConnectionStarted:
		return "EventType_ConnectionStarted"
	case EventType_ConnectionFailed:
		return "EventType_ConnectionFailed"
	case EventType_ConnectionFinished:
		return "EventType_ConnectionFinished"
	case EventType_StartSession:
		return "EventType_StartSession"
	case EventType_CancelSession:
		return "EventType_CancelSession"
	case EventType_FinishSession:
		return "EventType_FinishSession"
	case EventType_SessionStarted:
		return "EventType_SessionStarted"
	case EventType_SessionCanceled:
		return "EventType_SessionCanceled"
	case EventType_SessionFinished:
		return "EventType_SessionFinished"
	case EventType_SessionFailed:
		return "EventType_SessionFailed"
	case EventType_UsageResponse:
		return "EventType_UsageResponse"
	case EventType_TaskRequest:
		return "EventType_TaskRequest"
	case EventType_UpdateConfig:
		return "EventType_UpdateConfig"
	case EventType_AudioMuted:
		return "EventType_AudioMuted"
	case EventType_SayHello:
		return "EventType_SayHello"
	case EventType_TTSSentenceStart:
		return "EventType_TTSSentenceStart"
	case EventType_TTSSentenceEnd:
		return "EventType_TTSSentenceEnd"
	case EventType_TTSResponse:
		return "EventType_TTSResponse"
	case EventType_TTSEnded:
		return "EventType_TTSEnded"
	case EventType_PodcastRoundStart:
		return "EventType_PodcastRoundStart"
	case EventType_PodcastRoundResponse:
		return "EventType_PodcastRoundResponse"
	case EventType_PodcastRoundEnd:
		return "EventType_PodcastRoundEnd"
	case EventType_ASRInfo:
		return "EventType_ASRInfo"
	case EventType_ASRResponse:
		return "EventType_ASRResponse"
	case EventType_ASREnded:
		return "EventType_ASREnded"
	case EventType_ChatTTSText:
		return "EventType_ChatTTSText"
	case EventType_ChatResponse:
		return "EventType_ChatResponse"
	case EventType_ChatEnded:
		return "EventType_ChatEnded"
	case EventType_SourceSubtitleStart:
		return "EventType_SourceSubtitleStart"
	case EventType_SourceSubtitleResponse:
		return "EventType_SourceSubtitleResponse"
	case EventType_SourceSubtitleEnd:
		return "EventType_SourceSubtitleEnd"
	case EventType_TranslationSubtitleStart:
		return "EventType_TranslationSubtitleStart"
	case EventType_TranslationSubtitleResponse:
		return "EventType_TranslationSubtitleResponse"
	case EventType_TranslationSubtitleEnd:
		return "EventType_TranslationSubtitleEnd"
	default:
		return fmt.Sprintf("EventType_(%d)", t)
	}
}

// Message 是火山引擎二进制协议的核心消息结构体。
// 包含协议头信息（版本、类型、序列化、压缩）和业务数据（事件、会话、序列号、载荷）。
type Message struct {
	Version       VersionBits       // 协议版本
	HeaderSize    HeaderSizeBits    // 头部大小（4 字节为单位）
	MsgType       MsgType           // 消息类型
	MsgTypeFlag   MsgTypeFlagBits   // 消息类型标志
	Serialization SerializationBits // 序列化方式
	Compression   CompressionBits   // 压缩方式

	EventType EventType // 事件类型
	SessionID string    // 会话 ID
	ConnectID string    // 连接 ID
	Sequence  int32     // 消息序列号
	ErrorCode uint32    // 错误码（仅 MsgTypeError 类型时使用）

	Payload []byte // 消息载荷数据
}

// NewMessageFromBytes 从原始字节数据解析消息。
// 先从第二个字节提取消息类型和标志位，创建 Message 实例，再进行完整反序列化。
// 参数:
//   - data: 原始字节数据
// 返回:
//   - *Message: 解析后的消息实例
//   - error: 数据过短或解析失败时返回错误
	if len(data) < 3 {
		return nil, fmt.Errorf("data too short: expected at least 3 bytes, got %d", len(data))
	}

	typeAndFlag := data[1]

	msg, err := NewMessage(MsgType(typeAndFlag>>4), MsgTypeFlagBits(typeAndFlag&0b00001111))
	if err != nil {
		return nil, err
	}

	if err := msg.Unmarshal(data); err != nil {
		return nil, err
	}

	return msg, nil
}

// NewMessage 创建一个新的消息实例，使用默认的协议配置。
// 默认使用版本 1、4 字节头部、JSON 序列化、无压缩。
// 参数:
//   - msgType: 消息类型
//   - flag: 消息类型标志
// 返回:
//   - *Message: 新创建的消息实例
//   - error: 始终返回 nil
	return &Message{
		MsgType:       msgType,
		MsgTypeFlag:   flag,
		Version:       Version1,
		HeaderSize:    HeaderSize4,
		Serialization: SerializationJSON,
		Compression:   CompressionNone,
	}, nil
}

// String 返回消息的可读字符串表示，用于日志和调试。
// 根据消息类型和标志位，展示不同的信息（如序列号、载荷大小、错误码等）。
// 返回:
//   - string: 消息的格式化描述
	switch m.MsgType {
	case MsgTypeAudioOnlyServer, MsgTypeAudioOnlyClient:
		if m.MsgTypeFlag == MsgTypeFlagPositiveSeq || m.MsgTypeFlag == MsgTypeFlagNegativeSeq {
			return fmt.Sprintf("%s, %s, Sequence: %d, PayloadSize: %d", m.MsgType, m.EventType, m.Sequence, len(m.Payload))
		}
		return fmt.Sprintf("%s, %s, PayloadSize: %d", m.MsgType, m.EventType, len(m.Payload))
	case MsgTypeError:
		return fmt.Sprintf("%s, %s, ErrorCode: %d, Payload: %s", m.MsgType, m.EventType, m.ErrorCode, string(m.Payload))
	default:
		if m.MsgTypeFlag == MsgTypeFlagPositiveSeq || m.MsgTypeFlag == MsgTypeFlagNegativeSeq {
			return fmt.Sprintf("%s, %s, Sequence: %d, Payload: %s",
				m.MsgType, m.EventType, m.Sequence, string(m.Payload))
		}
		return fmt.Sprintf("%s, %s, Payload: %s", m.MsgType, m.EventType, string(m.Payload))
	}
}

// Marshal 将消息序列化为二进制字节数组。
// 序列化顺序：头部 -> 事件/会话ID（可选）-> 序列号/错误码（可选）-> 载荷。
// 返回:
//   - []byte: 序列化后的字节数组
//   - error: 序列化失败时返回错误
	buf := new(bytes.Buffer)

	header := []uint8{
		uint8(m.Version)<<4 | uint8(m.HeaderSize),
		uint8(m.MsgType)<<4 | uint8(m.MsgTypeFlag),
		uint8(m.Serialization)<<4 | uint8(m.Compression),
	}

	headerSize := 4 * int(m.HeaderSize)
	if padding := headerSize - len(header); padding > 0 {
		header = append(header, make([]uint8, padding)...)
	}

	if err := binary.Write(buf, binary.BigEndian, header); err != nil {
		return nil, err
	}

	writers, err := m.writers()
	if err != nil {
		return nil, err
	}

	for _, write := range writers {
		if err := write(buf); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// Unmarshal 从二进制字节数据反序列化消息。
// 解析顺序：头部 -> 读取器列表（根据消息类型动态确定）-> 校验无剩余数据。
// 参数:
//   - data: 原始字节数据
// 返回:
//   - error: 反序列化失败时返回错误
	buf := bytes.NewBuffer(data)

	versionAndHeaderSize, err := buf.ReadByte()
	if err != nil {
		return err
	}

	m.Version = VersionBits(versionAndHeaderSize >> 4)
	m.HeaderSize = HeaderSizeBits(versionAndHeaderSize & 0b00001111)

	_, err = buf.ReadByte()
	if err != nil {
		return err
	}

	serializationCompression, err := buf.ReadByte()
	if err != nil {
		return err
	}

	m.Serialization = SerializationBits(serializationCompression & 0b11110000)
	m.Compression = CompressionBits(serializationCompression & 0b00001111)

	headerSize := 4 * int(m.HeaderSize)
	readSize := 3
	if paddingSize := headerSize - readSize; paddingSize > 0 {
		if n, err := buf.Read(make([]byte, paddingSize)); err != nil || n < paddingSize {
			return fmt.Errorf("insufficient header bytes: expected %d, got %d", paddingSize, n)
		}
	}

	readers, err := m.readers()
	if err != nil {
		return err
	}

	for _, read := range readers {
		if err := read(buf); err != nil {
			return err
		}
	}

	if _, err := buf.ReadByte(); err != io.EOF {
		return fmt.Errorf("unexpected data after message: %v", err)
	}

	return nil
}

// writers 根据消息类型和标志位返回序列化时需要调用的写入函数列表。
// 写入顺序由协议规范决定：事件头 -> 会话ID -> 序列号/错误码 -> 载荷。
// 返回:
//   - writers: 写入函数列表
//   - error: 不支持的消息类型时返回错误
	if m.MsgTypeFlag == MsgTypeFlagWithEvent {
		writers = append(writers, m.writeEvent, m.writeSessionID)
	}

	switch m.MsgType {
	case MsgTypeFullClientRequest, MsgTypeFullServerResponse, MsgTypeFrontEndResultServer, MsgTypeAudioOnlyClient, MsgTypeAudioOnlyServer:
		if m.MsgTypeFlag == MsgTypeFlagPositiveSeq || m.MsgTypeFlag == MsgTypeFlagNegativeSeq {
			writers = append(writers, m.writeSequence)
		}
	case MsgTypeError:
		writers = append(writers, m.writeErrorCode)
	default:
		return nil, fmt.Errorf("unsupported message type: %d", m.MsgType)
	}

	writers = append(writers, m.writePayload)
	return writers, nil
}

// writeEvent 将事件类型写入缓冲区（int32 大端序）。
func (m *Message) writeEvent(buf *bytes.Buffer) error {
	return binary.Write(buf, binary.BigEndian, m.EventType)
}

// writeSessionID 将会话 ID 写入缓冲区。
// 连接管理类事件（Start/Finish/Started/Failed）不需要会话 ID。
// 格式：4 字节长度前缀 + 会话 ID 字符串。
func (m *Message) writeSessionID(buf *bytes.Buffer) error {
	switch m.EventType {
	case EventType_StartConnection, EventType_FinishConnection,
		EventType_ConnectionStarted, EventType_ConnectionFailed:
		return nil
	}

	size := len(m.SessionID)
	if int64(size) > math.MaxUint32 {
		return fmt.Errorf("session ID size (%d) exceeds max(uint32)", size)
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(size)); err != nil {
		return err
	}

	buf.WriteString(m.SessionID)
	return nil
}

// writeSequence 将消息序列号写入缓冲区（int32 大端序）。
func (m *Message) writeSequence(buf *bytes.Buffer) error {
	return binary.Write(buf, binary.BigEndian, m.Sequence)
}

// writeErrorCode 将错误码写入缓冲区（uint32 大端序）。
func (m *Message) writeErrorCode(buf *bytes.Buffer) error {
	return binary.Write(buf, binary.BigEndian, m.ErrorCode)
}

// writePayload 将载荷数据写入缓冲区。
// 格式：4 字节长度前缀 + 载荷原始字节。
func (m *Message) writePayload(buf *bytes.Buffer) error {
	size := len(m.Payload)
	if int64(size) > math.MaxUint32 {
		return fmt.Errorf("payload size (%d) exceeds max(uint32)", size)
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(size)); err != nil {
		return err
	}

	buf.Write(m.Payload)
	return nil
}

// readers 根据消息类型和标志位返回反序列化时需要调用的读取函数列表。
// 读取顺序与写入顺序对应，但需注意事件头在反序列化时可能包含连接 ID。
// 返回:
//   - readers: 读取函数列表
//   - error: 不支持的消息类型时返回错误
	switch m.MsgType {
	case MsgTypeFullClientRequest, MsgTypeFullServerResponse, MsgTypeFrontEndResultServer, MsgTypeAudioOnlyClient, MsgTypeAudioOnlyServer:
		if m.MsgTypeFlag == MsgTypeFlagPositiveSeq || m.MsgTypeFlag == MsgTypeFlagNegativeSeq {
			readers = append(readers, m.readSequence)
		}
	case MsgTypeError:
		readers = append(readers, m.readErrorCode)
	default:
		return nil, fmt.Errorf("unsupported message type: %d", m.MsgType)
	}

	if m.MsgTypeFlag == MsgTypeFlagWithEvent {
		readers = append(readers, m.readEvent, m.readSessionID, m.readConnectID)
	}

	readers = append(readers, m.readPayload)
	return readers, nil
}

// readEvent 从缓冲区读取事件类型（int32 大端序）。
func (m *Message) readEvent(buf *bytes.Buffer) error {
	return binary.Read(buf, binary.BigEndian, &m.EventType)
}

// readSessionID 从缓冲区读取会话 ID。
// 连接管理类事件不需要会话 ID，直接跳过。
// 格式：4 字节长度前缀 + 会话 ID 字符串。
func (m *Message) readSessionID(buf *bytes.Buffer) error {
	switch m.EventType {
	case EventType_StartConnection, EventType_FinishConnection,
		EventType_ConnectionStarted, EventType_ConnectionFailed,
		EventType_ConnectionFinished:
		return nil
	}

	var size uint32
	if err := binary.Read(buf, binary.BigEndian, &size); err != nil {
		return err
	}

	if size > 0 {
		m.SessionID = string(buf.Next(int(size)))
	}

	return nil
}

// readConnectID 从缓冲区读取连接 ID。
// 仅连接管理类事件（ConnectionStarted/Failed/Finished）包含连接 ID。
// 格式：4 字节长度前缀 + 连接 ID 字符串。
func (m *Message) readConnectID(buf *bytes.Buffer) error {
	switch m.EventType {
	case EventType_ConnectionStarted, EventType_ConnectionFailed,
		EventType_ConnectionFinished:
	default:
		return nil
	}

	var size uint32
	if err := binary.Read(buf, binary.BigEndian, &size); err != nil {
		return err
	}

	if size > 0 {
		m.ConnectID = string(buf.Next(int(size)))
	}

	return nil
}

// readSequence 从缓冲区读取消息序列号（int32 大端序）。
func (m *Message) readSequence(buf *bytes.Buffer) error {
	return binary.Read(buf, binary.BigEndian, &m.Sequence)
}

// readErrorCode 从缓冲区读取错误码（uint32 大端序）。
func (m *Message) readErrorCode(buf *bytes.Buffer) error {
	return binary.Read(buf, binary.BigEndian, &m.ErrorCode)
}

// readPayload 从缓冲区读取载荷数据。
// 格式：4 字节长度前缀 + 载荷原始字节。
func (m *Message) readPayload(buf *bytes.Buffer) error {
	var size uint32
	if err := binary.Read(buf, binary.BigEndian, &size); err != nil {
		return err
	}

	if size > 0 {
		m.Payload = buf.Next(int(size))
	}

	return nil
}

// ReceiveMessage 从 WebSocket 连接接收一条消息并解析为 Message 对象。
// 仅接受二进制或文本类型的 WebSocket 消息。
// 参数:
//   - conn: WebSocket 连接
// 返回:
//   - *Message: 解析后的消息实例
//   - error: 接收或解析失败时返回错误
	mt, frame, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return nil, fmt.Errorf("unexpected Websocket message type: %d", mt)
	}
	msg, err := NewMessageFromBytes(frame)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// FullClientRequest 通过 WebSocket 发送一条完整的客户端请求消息。
// 创建 MsgTypeFullClientRequest 类型的消息，设置载荷后序列化并以二进制帧发送。
// 参数:
//   - conn: WebSocket 连接
//   - payload: 请求载荷数据（通常是 JSON 序列化的请求体）
// 返回:
//   - error: 创建、序列化或发送失败时返回错误
	msg, err := NewMessage(MsgTypeFullClientRequest, MsgTypeFlagNoSeq)
	if err != nil {
		return err
	}
	msg.Payload = payload
	frame, err := msg.Marshal()
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, frame)
}
