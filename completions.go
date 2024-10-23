// completions.go

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"manifold/internal/web"
)

const (
	//baseURL = "https://api.openai.com/v1"
	//completionsEndpoint = "/chat/completions"
	ttsEndpoint = "/audio/speech"
)

var (
	// TurnCounter is a counter for the number of turns in a chat.
	TurnCounter int
)

type ChatDocument struct {
	ID       string `json:"id"`
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
}

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
	Model       string    `json:"model,omitempty"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	//TopP        float64   `json:"top_p,omitempty"`
	MaxTokens int  `json:"max_tokens,omitempty"`
	Stream    bool `json:"stream,omitempty"`
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

type AnthropicResponse struct {
	ID           string                `json:"id"`
	Type         string                `json:"type"`
	Role         string                `json:"role"`
	Content      []ContentBlock        `json:"content"`
	Model        string                `json:"model"`
	StopReason   string                `json:"stop_reason"`
	StopSequence interface{}           `json:"stop_sequence"`
	Usage        AnthropicUsageMetrics `json:"usage"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AnthropicUsageMetrics struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// GetSystemTemplate returns the system template.
func GetSystemTemplate(systemPrompt string, userPrompt string) ChatPromptTemplate {
	userPrompt = fmt.Sprintf("{%s}", userPrompt)
	template := NewChatPromptTemplate([]Message{
		{
			Role:    "system",
			Content: systemPrompt,
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

func StreamCompletionToWebSocket(c *websocket.Conn, llmClient LLMClient, chatID int, model string, payload *CompletionRequest, responseBuffer *bytes.Buffer) error {
	// Check if the payload contains at least one message
	if len(payload.Messages) == 0 {
		log.Printf("Error: No messages found in the payload")
		return fmt.Errorf("no messages found in the payload")
	}

	userPrompt := payload.Messages[0].Content

	// Print the user prompt
	fmt.Println("USER PROMPT:", userPrompt)

	// Process the user prompt through the WorkflowManager
	processedPrompt, err := globalWM.Run(context.Background(), userPrompt, c)
	if err != nil {
		log.Printf("Error processing prompt through WorkflowManager: %v", err)
	}

	// Prepend the processed prompt to the messages
	payload.Messages[0].Content = processedPrompt

	timestamp := time.Now().Format(time.RFC3339)

	statusMsg := "Thinking..."
	formattedContent := fmt.Sprintf("<div id='progress' class='progress-bar placeholder-wave fs-5' style='width: 100%%;'>%s</div>", statusMsg)
	c.WriteMessage(websocket.TextMessage, []byte(formattedContent))

	// Use llmClient to send the request
	resp, err := llmClient.SendCompletionRequest(payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Process the streamed response
	var accumulatedText string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Check if the line has "data: " prefix
		if strings.HasPrefix(line, "data: ") {
			jsonStr := line[6:] // Strip the "data: " prefix

			// Parse the event
			var rawEvent map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &rawEvent); err != nil {
				log.Printf("Error unmarshalling event: %v", err)
				continue
			}

			// Handle specific types of events
			eventType, ok := rawEvent["type"].(string)
			if !ok {
				continue
			}

			switch eventType {
			case "content_block_delta":
				// Extract the text from the delta
				delta, ok := rawEvent["delta"].(map[string]interface{})
				if ok {
					text, textOk := delta["text"].(string)
					if textOk {
						accumulatedText += text
						responseBuffer.WriteString(text)

						htmlMsg := web.MarkdownToHTML(responseBuffer.Bytes())
						turnIDStr := fmt.Sprint(chatID + TurnCounter)
						formattedContent := fmt.Sprintf("<div id='response-content-%s' class='mx-1' hx-trigger='load'>%s</div>\n<codapi-snippet engine='browser' sandbox='javascript' editor='basic'></codapi-snippet>", turnIDStr, htmlMsg)

						if err := c.WriteMessage(websocket.TextMessage, []byte(formattedContent)); err != nil {
							return err
						}
					}
				}
			case "content_block_stop":
				// Finalize the content block processing
				log.Printf("Completed content block: %s", accumulatedText)

				// Convert accumulated text to HTML and send as a response update
				htmlMsg := web.MarkdownToHTML([]byte(accumulatedText))
				turnIDStr := fmt.Sprint(chatID + TurnCounter)
				formattedContent := fmt.Sprintf("<div id='response-content-%s' class='mx-1' hx-trigger='load'>%s</div>\n<codapi-snippet engine='browser' sandbox='javascript' editor='basic'></codapi-snippet>", turnIDStr, htmlMsg)

				if err := c.WriteMessage(websocket.TextMessage, []byte(formattedContent)); err != nil {
					return err
				}

				// Clear the accumulated text after sending
				accumulatedText = ""
				responseBuffer.Reset()

				// Save the chat turn
				if err := SaveChatTurn(userPrompt, accumulatedText, timestamp); err != nil {
					log.Printf("Error saving chat turn: %v", err)
				}

				// Return an error to close the connection
				return fmt.Errorf("content block stop")

			case "message_delta":
				// Handle message completion, stop reasons, or usage updates
				log.Printf("Completed message received.")
			// Add cases for other event types if needed
			default:
				// Ignore other event types like "ping", "message_start", etc.
				continue
			}

			// Log accumulated response to see the progress
			log.Printf("Accumulated Response: %s", accumulatedText)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Save the final chat turn if anything remains
	if accumulatedText != "" {
		htmlMsg := web.MarkdownToHTML([]byte(accumulatedText))
		turnIDStr := fmt.Sprint(chatID + TurnCounter)
		finalFormattedContent := fmt.Sprintf("<div id='response-content-%s' class='mx-1' hx-trigger='load'>%s</div>\n<codapi-snippet engine='browser' sandbox='javascript' editor='basic'></codapi-snippet>", turnIDStr, htmlMsg)

		if err := c.WriteMessage(websocket.TextMessage, []byte(finalFormattedContent)); err != nil {
			return err
		}

		// Save the final chat turn
		if err := SaveChatTurn(userPrompt, accumulatedText, timestamp); err != nil {
			log.Printf("Error saving chat turn: %v", err)
		}
	}

	return nil
}

// IncrementTurn increments the turn counter.
func IncrementTurn() int {
	TurnCounter++
	return TurnCounter
}

// handleChatSubmit handles the submission of chat messages.
func handleChatSubmit(c echo.Context) error {
	//model := c.FormValue("model")
	userPrompt := c.FormValue("userprompt")
	roleInstructions := c.FormValue("role_instructions")
	endpoint := c.FormValue("endpoint")

	// Stream the completion response to the client

	turnID := IncrementTurn()

	// render map into echo chat.html template
	return c.Render(http.StatusOK, "chat", echo.Map{
		"username":  "User",
		"message":   userPrompt,
		"assistant": "Assistant",
		//"model":            model,
		"turnID":           turnID,
		"wsRoute":          "",
		"endpoint":         endpoint,
		"roleInstructions": roleInstructions,
	})
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
