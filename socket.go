package main

import (
	"bytes"
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var (
	upgrader = websocket.Upgrader{}
)

// WebSocketMessage represents a message sent over WebSocket.
type WebSocketMessage struct {
	ChatMessage      string                 `json:"chat_message"`
	RoleInstructions string                 `json:"role_instructions"`
	Model            string                 `json:"model"`
	Headers          map[string]interface{} `json:"HEADERS"`
}

func handleWebSocketConnection(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	var responseBuffer bytes.Buffer
	var wsMessage WebSocketMessage

	// Read and unmarshal the initial WebSocket message
	wsMessage, err = readAndUnmarshalMessage(ws)
	if err != nil {
		return err
	}

	cpt := GetSystemTemplate(wsMessage.RoleInstructions, wsMessage.ChatMessage)

	// Create a new CompletionRequest using the chat message
	payload := &CompletionRequest{
		Messages:    cpt.Messages,
		Temperature: 0.6,
		MaxTokens:   8192,
		Stream:      true,
		TopP:        0.9,
	}

	for {
		err = StreamCompletionToWebSocket(ws, 0, wsMessage.Model, payload, &responseBuffer)
		if err != nil {
			return err
		}

		// Clear the completion request messages so when a new one is submitted it sends the correct one
		payload.Messages = nil

		_, _, err := ws.ReadMessage()
		if err != nil {
			return err
		}
	}
}

// readAndUnmarshalMessage reads and unmarshals a WebSocket message.
func readAndUnmarshalMessage(c *websocket.Conn) (WebSocketMessage, error) {
	// Read the message from the WebSocket.
	_, messageBytes, err := c.ReadMessage()
	if err != nil {
		return WebSocketMessage{}, err
	}

	// Unmarshal the JSON message.
	var wsMessage WebSocketMessage
	err = json.Unmarshal(messageBytes, &wsMessage)
	if err != nil {
		return WebSocketMessage{}, err
	}

	return wsMessage, nil
}
