package main

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WebSocketMessage represents a message sent over WebSocket.
type WebSocketMessage struct {
	ChatMessage      string                 `json:"chat_message"`
	RoleInstructions string                 `json:"role_instructions"`
	Model            string                 `json:"model"`
	Headers          map[string]interface{} `json:"HEADERS"`
}

func handleWebSocketConnection(c echo.Context) error {

	// Upgrade the HTTP connection to a WebSocket connection.
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		c.Logger().Error("WebSocket upgrade failed:", err)
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

	// Process the user prompt through the WorkflowManager
	// processedPrompt, err := globalWM.Run(context.Background(), wsMessage.ChatMessage)
	// if err != nil {
	// 	log.Printf("Error processing prompt through WorkflowManager: %v", err)
	// }

	// Get the system instructions (assuming they are part of the message)
	cpt := GetSystemTemplate(wsMessage.RoleInstructions, wsMessage.ChatMessage)

	// Create a new CompletionRequest using the processed prompt
	payload := &CompletionRequest{
		Model:       wsMessage.Model,
		Messages:    cpt.FormatMessages(nil), // Assuming no additional variables
		Temperature: 0.3,
		MaxTokens:   64000, // Ensure this is set by the backend LLM config in a future commit.
		Stream:      true,
		//TopP:        0.9,
	}

	for {
		// Pass llmClient as an argument
		err = StreamCompletionToWebSocket(ws, llmClient, 0, wsMessage.Model, payload, &responseBuffer)
		if err != nil {
			return err
		}

		// Clear the completion request messages for the next submission
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
