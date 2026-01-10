package ws

import "time"

type MessageType string

const (
	MessagePing   MessageType = "ping"
	MessagePong   MessageType = "pong"
	MessageStatus MessageType = "status"
	MessageError  MessageType = "error"
	MessageEcho   MessageType = "echo"
)

type Message struct {
	Type      MessageType `json:"type"`
	Payload   any         `json:"payload"`
	TimeStamp time.Time   `json:"timestamp"`
}

func NewMessage(t MessageType, payload any) Message {
	return Message{
		Type:      t,
		Payload:   payload,
		TimeStamp: time.Now().UTC(),
	}
}
