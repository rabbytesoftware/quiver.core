package ws

import "github.com/gorilla/websocket"

type WebSocketHandler interface {
	HandleConnection(conn *websocket.Conn)
	Broadcast(message Message)
	Send(conn *websocket.Conn, message Message)
}
