package websocket

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// In production, this should be more restrictive
		return true
	},
}

// ServeWS handles WebSocket upgrade requests
func ServeWS(hub *Hub, c echo.Context) error {
	fmt.Printf("[WebSocket] New connection request from %s\n", c.Request().RemoteAddr)
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		fmt.Printf("[WebSocket] Upgrade failed: %v\n", err)
		return err
	}

	client := newClient(hub, conn)
	hub.register <- client
	fmt.Printf("[WebSocket] Client registered\n")

	// Start read and write pumps in goroutines
	go client.writePump()
	go client.readPump()

	return nil
}
