// Package types는 서버 전반에서 사용되는 데이터 구조를 정의합니다.
package types

import (
	"io"
	"strconv"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
)

// SSEEvent는 서버 전송 이벤트(SSE)의 유형을 정의합니다.
type SSEEvent string

const (
	// SSEEventHeartbeat는 연결 유지를 위한 하트비트 이벤트입니다.
	SSEEventHeartbeat   SSEEvent = "heartbeat"
	// SSEEventSubscribe는 채널 구독 시 발생하는 이벤트입니다.
	SSEEventSubscribe   SSEEvent = "subscribe"
	// SSEEventUnsubscribe는 채널 구독 해제 시 발생하는 이벤트입니다.
	SSEEventUnsubscribe SSEEvent = "unsubscribe"
	// SSEEventPublish는 데이터를 발행할 때 발생하는 이벤트입니다.
	SSEEventPublish     SSEEvent = "publish"
	// SSEEventError는 에러 발생 시 전달되는 이벤트입니다.
	SSEEventError       SSEEvent = "error"
)

func (e SSEEvent) String() string {
	return string(e)
}

// SSEMeta는 SSE 이벤트와 관련된 부가적인 메타데이터를 담고 있는 구조체입니다.
type SSEMeta struct {
	ID     uuid.UUID `json:"id,omitempty"`     // 관련 리소스 ID
	Path   string    `json:"path,omitempty"`   // 관련 리소스 경로
	Action string    `json:"action,omitempty"` // 수행된 액션
}

func (m SSEMeta) Zero() bool {
	return m.ID == uuid.Nil && m.Path == "" && m.Action == ""
}

// SSEData는 SSE 이벤트를 통해 전달되는 실제 데이터 페이로드 구조체입니다.
type SSEData struct {
	Timestamp time.Time `json:"timestamp"`        // 이벤트 발생 시간
	Status    bool      `json:"status"`           // 성공 여부
	Message   string    `json:"message,omitempty"` // 메시지 내용
	Meta      SSEMeta   `json:"meta"`              // 메타데이터
}

func (data *SSEData) MarshalJSON() ([]byte, error) {
	if data.Meta.Zero() {
		return json.Marshal(map[string]any{
			"timestamp": data.Timestamp.Format(time.RFC3339),
			"status":    data.Status,
			"message":   data.Message,
		})
	}

	return json.Marshal(map[string]any{
		"timestamp": data.Timestamp.Format(time.RFC3339),
		"status":    data.Status,
		"message":   data.Message,
		"meta":      data.Meta,
	})
}

// SSEMessage는 SSE 스트림으로 전송되는 전체 메시지 패킷을 정의합니다.
type SSEMessage struct {
	ID    uuid.UUID     `json:"id"`    // 메시지 고유 ID
	Event SSEEvent      `json:"event"` // 이벤트 유형
	Data  SSEData       `json:"data"`  // 데이터 페이로드
	Retry time.Duration `json:"retry"` // 재연결 시도 주기
}

func (msg *SSEMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":    msg.ID.String(),
		"event": msg.Event,
		"data":  msg.Data,
		"retry": msg.Retry.Milliseconds(),
	})
}

// WriteTo는 메시지를 SSE 프로토콜 형식에 맞춰 Writer에 기록합니다.
func (msg *SSEMessage) WriteTo(w io.Writer) (int64, error) {
	var total int64
	var n int
	var err error

	// ID 필드
	if msg.ID != uuid.Nil {
		n, err = writeTo(w, "id", []byte(msg.ID.String()))
		total += int64(n)
		if err != nil {
			return total, err
		}
	}

	// Event 필드
	if msg.Event != "" {
		n, err = writeTo(w, "event", []byte(msg.Event.String()))
		total += int64(n)
		if err != nil {
			return total, err
		}
	}

	// Data 필드는 JSON으로 직렬화
	jsonData, err := json.Marshal(msg.Data)
	if err != nil {
		return total, err
	}

	n, err = writeTo(w, "data", jsonData)
	total += int64(n)
	if err != nil {
		return total, err
	}

	// Retry 필드
	if msg.Retry > 0 {
		n, err = writeTo(w, "retry", []byte(strconv.FormatInt(msg.Retry.Microseconds(), 10)))
		total += int64(n)
		if err != nil {
			return total, err
		}
	}

	// 이벤트 메시지 완료
	n, err = io.WriteString(w, "\n\n")
	total += int64(n)
	if err != nil {
		return total, err
	}

	return total, nil
}

func writeTo(w io.Writer, k string, v []byte) (int, error) {
	var total int
	var n int
	var err error

	n, err = io.WriteString(w, k+":")
	total += n
	if err != nil {
		return total, err
	}

	n, err = w.Write(v)
	total += n
	if err != nil {
		return total, err
	}

	n, err = io.WriteString(w, "\n")
	total += n
	if err != nil {
		return total, err
	}

	return total, nil
}
