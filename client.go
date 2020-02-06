package main

import (
	"bytes"
	"log"
	"net/http"
	"time"
	"github.com/gorilla/websocket"
)

const (
	// Max time to write message to client
	messageTimeout = 10 * time.Second

	// Max time to wait for message response from client
	responseTimeout = 60 * time.Second

	// Polling interval for querying message * response timeouts
	queryInterval = (responseTimeout * 9) / 10

	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize: 1024,
	WriteBufferSize: 1024,
}

type Client struct {
	// Server instance
	server *Server

	// Websocket connection
	conn *websocket.Conn

	// Outbound messages
	send chan []byte
}

func (c * Client) receiveMessage() {
	defer func() {
		c.server.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(responseTimeout))
	c.conn.SetPongHandler(func (string) error { c.conn.SetReadDeadline(time.Now().Add(responseTimeout)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))
		c.server.transmission <- message
	}
}

func (c *Client) sendMessage() {
	ticker := time.NewTicker(queryInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <- c.send:
			c.conn.SetWriteDeadline(time.Now().Add(messageTimeout))
			if !ok {
				// Server closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <- ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(messageTimeout))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func serveWebSocket(s *Server, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{server: s, conn: conn, send: make(chan []byte, 256)}
	client.server.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.receiveMessage()
	go client.sendMessage()
}