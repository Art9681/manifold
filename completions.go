package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"manifold/internal/web"
)

const (
	baseURL             = "http://192.168.0.110:32182/v1"
	completionsEndpoint = "/chat/completions"
	ttsEndpoint         = "/audio/speech"
)

var (
	// TurnCounter is a counter for the number of turns in a chat.
	TurnCounter int
)

type CompletionsRole struct {
	ID           uint   `gorm:"primaryKey" yaml:"-"`
	Name         string `gorm:"uniqueIndex" yaml:"name"`
	Instructions string `gorm:"type:text" yaml:"instructions"`
}

type ChatRole interface {
	GetName() string
	GetInstructions() string
}

func (r *CompletionsRole) GetName() string {
	return r.Name
}

func (r *CompletionsRole) GetInstructions() string {
	return r.Instructions
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PromptTemplate represents a template for generating string prompts.
type PromptTemplate struct {
	Template string
}

// ChatPromptTemplate represents a template for generating chat prompts.
type ChatPromptTemplate struct {
	Messages []Message
}

// Model represents an AI model from the OpenAI API with its ID, name, and description.
type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CompletionRequest represents the payload for the completion API.
type CompletionRequest struct {
	//Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
	Stream      bool      `json:"stream"`
}

// Choice represents a choice for the completion response.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	Logprobs     *bool   `json:"logprobs"` // Pointer to a boolean or nil
	FinishReason string  `json:"finish_reason"`
}

// Usage contains information about token usage in the completion response.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CompletionResponse represents the response from the completion API.
type CompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint"`
	Choices           []Choice `json:"choices"`
	Usage             Usage    `json:"usage"`
}

// ErrorData represents the structure of an error response from the OpenAI API.
type ErrorData struct {
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
}

// ErrorResponse wraps the structure of an error when an API request fails.
type ErrorResponse struct {
	Error ErrorData `json:"error"`
}

// UsageMetrics details the token usage of the Embeddings API request.
type UsageMetrics struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// GetSystemTemplate returns the system template.
func GetSystemTemplate(userPrompt string) ChatPromptTemplate {
	userPrompt = fmt.Sprintf("{%s}", userPrompt)
	template := NewChatPromptTemplate([]Message{
		{
			Role:    "system",
			Content: "You are a helpful AI assistant.",
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	})

	return *template
}

// NewChatPromptTemplate creates a new ChatPromptTemplate.
func NewChatPromptTemplate(messages []Message) *ChatPromptTemplate {
	return &ChatPromptTemplate{Messages: messages}
}

// Format formats the template with the provided variables.
func (pt *PromptTemplate) Format(vars map[string]string) string {
	result := pt.Template
	for k, v := range vars {
		placeholder := fmt.Sprintf("{%s}", k)
		result = strings.ReplaceAll(result, placeholder, v)
	}
	return result
}

// FormatMessages formats the chat messages with the provided variables.
func (cpt *ChatPromptTemplate) FormatMessages(vars map[string]string) []Message {
	var formattedMessages []Message
	for _, msg := range cpt.Messages {
		formattedContent := msg.Content
		for k, v := range vars {
			placeholder := fmt.Sprintf("{%s}", k)
			formattedContent = strings.ReplaceAll(formattedContent, placeholder, v)
		}
		formattedMessages = append(formattedMessages, Message{Role: msg.Role, Content: formattedContent})
	}
	return formattedMessages
}

// SendRequest sends a request to the OpenAI API and decodes the response.
func SendRequest(endpoint string, payload *CompletionRequest) (*http.Response, error) {
	// Convert the layload to json
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	fmt.Println(string(jsonPayload))

	req, err := http.NewRequest("POST", "http://192.168.0.110:32182/v1/chat/completions", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	//req.Header.Set("Authorization", "Bearer "+apiKey) // Used for public services

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func StreamCompletionToWebSocket(c *websocket.Conn, chatID int, model string, payload *CompletionRequest, responseBuffer *bytes.Buffer) error {

	resp, err := SendRequest(completionsEndpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			jsonStr := line[6:] // Strip the "data: " prefix
			var data struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
				FinishReason string `json:"finish_reason"`
			}

			if strings.Contains(jsonStr, "[DONE]") {
				if err := c.WriteMessage(websocket.TextMessage, []byte("EOS")); err != nil {
					return err
				}

				// Close the socket with a message
				// c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				// c.Close()

				return c.Close()
			}

			if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
				return fmt.Errorf("%s", responseBuffer.String())
			}

			for _, choice := range data.Choices {
				responseBuffer.WriteString(choice.Delta.Content)
			}

			htmlMsg := web.MarkdownToHTML(responseBuffer.Bytes())
			turnIDStr := fmt.Sprint(chatID + TurnCounter)
			formattedContent := fmt.Sprintf("<div id='response-content-%s' class='mx-1' hx-trigger='load'>%s</div>\n<codapi-snippet engine='browser' sandbox='javascript' editor='basic'></codapi-snippet>", turnIDStr, htmlMsg)

			if err := c.WriteMessage(websocket.TextMessage, []byte(formattedContent)); err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

// IncrementTurn increments the turn counter.
func IncrementTurn() int {
	TurnCounter++
	return TurnCounter
}

// handleChatSubmit handles the submission of chat messages.
// func handleChatSubmit(c echo.Context) error {
// 	userPrompt := c.FormValue("userprompt")
// 	role := c.FormValue("role")

// 	// Get the role instructions
// 	// This is a hack to get the role instructions
// 	// This needs to be set by the frontend dropdown
// 	//config := c.Get("config").(*Config)
// 	//roleInstructions := config.CompletionsRole[0].Instructions

// 	// Set the chat role
// 	chatRole := &CompletionsRole{
// 		Name:         role,
// 		Instructions: "You are a helpful AI assistant.",
// 	}

// 	turnID := IncrementTurn()

// 	// render map into echo chat.html template
// 	return c.Render(http.StatusOK, "chat", echo.Map{
// 		"username":  "User",
// 		"message":   userPrompt,
// 		"assistant": "Assistant",
// 		"model":     "Local",
// 		"turnID":    turnID,
// 		"wsRoute":   "",
// 		"hosts":     "http://192.168.0.110:32182", // Do not hard code this
// 		"role":      chatRole.GetInstructions(),
// 	})
// }

// handleChatSubmit handles the submission of chat messages.
func handleChatSubmit(c echo.Context) error {
	userPrompt := c.FormValue("userprompt")

	// Create a new CompletionRequest using the chat message
	payload := &CompletionRequest{
		Messages:    []Message{{Role: "user", Content: userPrompt}},
		Temperature: 0.3,
		MaxTokens:   128000,
		Stream:      true,
	}

	// Stream the JSON response back to the client
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c.Response().WriteHeader(http.StatusOK)

	resp, err := SendRequest(completionsEndpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			jsonStr := line[6:] // Strip the "data: " prefix

			// Stream the data directly to the response
			if _, err := c.Response().Write([]byte(jsonStr + "\n")); err != nil {
				return err
			}

			// Flush the buffer to ensure the data is sent immediately
			c.Response().Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

// handleSetChatRole handles the setting of the chat role.
func handleSetChatRole(c echo.Context, config *Config) error {
	role := c.FormValue("role")
	var instructions string

	// Use case to retrieve the role instructions by name
	for _, r := range config.Roles {
		if r.Name == role {
			instructions = r.Instructions
			break
		}
	}

	return c.JSON(http.StatusOK, map[string]string{
		role: instructions,
	})
}

func handleGetAllChatRoles(c echo.Context, config *Config) error {
	rolesMap := make(map[string]string)

	for _, r := range config.Roles {
		rolesMap[r.Name] = r.Instructions
	}

	return c.JSON(http.StatusOK, rolesMap)
}

// handleGetRoleInstructions handles the retrieval of the role instructions.
func handleGetRoleInstructions(c echo.Context) error {
	role := c.Param("role")

	// Get the role instructions
	// This is a hack to get the role instructions
	// This needs to be set by the frontend dropdown
	//config := c.Get("config").(*Config)
	//roleInstructions := config.CompletionsRole[0].Instructions

	// Retrieve the role instructions from the app config

	// Set the chat role
	chatRole := &CompletionsRole{
		Name:         role,
		Instructions: "You are a helpful AI assistant.",
	}

	return c.JSON(http.StatusOK, map[string]string{
		"role": chatRole.GetInstructions(),
	})
}
