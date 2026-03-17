package types

import (
	"io"
	"strconv"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
)

type SSEEvent string

const (
	SSEEventHeartbeat   SSEEvent = "heartbeat"
	SSEEventSubscribe   SSEEvent = "subscribe"
	SSEEventUnsubscribe SSEEvent = "unsubscribe"
	SSEEventPublish     SSEEvent = "publish"
	SSEEventError       SSEEvent = "error"
)

func (e SSEEvent) String() string {
	return string(e)
}

type SSEMeta struct {
	ID     uuid.UUID `json:"id,omitempty"`
	Path   string    `json:"path,omitempty"`
	Action string    `json:"action,omitempty"`
}

func (m SSEMeta) Zero() bool {
	return m.ID == uuid.Nil && m.Path == "" && m.Action == ""
}

type SSEData struct {
	Timestamp time.Time `json:"timestamp"`
	Status    bool      `json:"status"`
	Message   string    `json:"message,omitempty"`
	Meta      SSEMeta   `json:"meta"`
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

type SSEMessage struct {
	ID    uuid.UUID     `json:"id"`
	Event SSEEvent      `json:"event"`
	Data  SSEData       `json:"data"`
	Retry time.Duration `json:"retry"`
}

func (msg *SSEMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":    msg.ID.String(),
		"event": msg.Event,
		"data":  msg.Data,
		"retry": msg.Retry.Milliseconds(),
	})
}

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
