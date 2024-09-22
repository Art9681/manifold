// completions.go

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"manifold/internal/documents"
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
	Temperature float64   `json:"temperature"`
	TopP        float64   `json:"top_p"`
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

func (client *Client) SendCompletionRequest(payload *CompletionRequest) (*http.Response, error) {

	// TODO: Add a better way to handle the model selection using the frontend
	// Jank way to set the model to gpt-4o-mini if the client url is openai
	// if the client url is openai, set the payload model to gpt-4o-mini
	// this should not be hard coded but loaded from app config instead
	if client.BaseURL == "https://api.openai.com/v1" {
		payload.Model = "gpt-4o-mini"
	}

	// Convert the payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := client.BaseURL + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	if client.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+client.APIKey)
	}

	return http.DefaultClient.Do(req)
}

func StreamCompletionToWebSocket(c *websocket.Conn, llmClient LLMClient, chatID int, model string, payload *CompletionRequest, responseBuffer *bytes.Buffer) error {
	timestamp := time.Now().Format(time.RFC3339)

	// Use llmClient to send the request
	resp, err := llmClient.SendCompletionRequest(payload)
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
					FinishReason string `json:"finish_reason"`
					Delta        struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
				err := SaveChatTurn(payload.Messages[1].Content, responseBuffer.String(), timestamp)
				if err != nil {
					log.Printf("Error saving chat turn: %v", err)
				}

				return fmt.Errorf("%s", responseBuffer.String())
			}

			for _, choice := range data.Choices {
				// If the finish reason is "stop", then stop streaming
				if choice.FinishReason == "stop" {
					err := SaveChatTurn(payload.Messages[1].Content, responseBuffer.String(), timestamp)
					if err != nil {
						log.Printf("Error saving chat turn: %v", err)
					}

					// Clear all buffers and prompts
					responseBuffer.Reset()

					return fmt.Errorf("%s", responseBuffer.String())
				}

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
func handleChatSubmit(c echo.Context) error {
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
		//"model":            "/Users/arturoaquino/.eternal-v1/models/gemma-2-27b-it/gemma-2-27b-it-Q8_0.gguf",
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

// SaveChatTurn stores the chat turn's prompt and response in the Badger KV store.
func SaveChatTurn(prompt, response, timestamp string) error {
	// Concatenate the prompt and response
	concatenatedText := fmt.Sprintf("User: %s\nAssistant: %s", prompt, response)

	// Split the concatenated text into chunks of 1000 tokens
	chunks := documents.SplitTextByCount(concatenatedText, 500)

	// Create a unique chat turn ID
	chatTurnID := fmt.Sprintf("chatturn-%d", time.Now().UnixNano())

	// Find the RetrievalTool within the WorkflowManager
	var retrievalTool *RetrievalTool
	for _, tool := range globalWM.tools {
		if tool.Name == "retrieval" {
			var ok bool
			retrievalTool, ok = tool.Tool.(*RetrievalTool)
			if !ok {
				return fmt.Errorf("retrieval tool type assertion failed")
			}
			break
		}
	}

	if retrievalTool == nil {
		return fmt.Errorf("retrieval tool not found in WorkflowManager")
	}

	// Iterate over each chunk, generate embeddings, and store in Badger
	for _, chunk := range chunks {
		embeddings, err := GenerateEmbedding(chunk)
		if err != nil {
			log.Printf("Error generating embeddings for chunk: %v", err)
			continue
		}

		docID := fmt.Sprintf("%s-%d", chatTurnID, time.Now().UnixNano())

		// Store the document and its embedding in Badger
		err = retrievalTool.StoreDocument(docID, chunk, embeddings)
		if err != nil {
			log.Printf("Error storing document in Badger: %v", err)
			continue
		}
	}

	if retrievalTool == nil {
		return fmt.Errorf("retrieval tool not found in WorkflowManager")
	}

	return nil
}

func GenerateEmbedding(text string) ([]float64, error) {
	// Invoke the embeddings API
	textArr := []string{text}
	embeddingRequest := EmbeddingRequest{
		Input:          textArr,
		Model:          "eternal",
		EncodingFormat: "float",
	}

	resp, err := llmClient.SendEmbeddingRequest(&embeddingRequest)
	if err != nil {
		log.Printf("Error sending embedding request: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	var embeddingResponse EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResponse); err != nil {
		log.Printf("Error decoding embedding response: %v", err)
		return nil, err
	}

	if len(embeddingResponse.Data) == 0 {
		return nil, fmt.Errorf("no embeddings found")
	}

	// Concatenate the embeddings into a single slice
	var embeddings []float64
	for _, emb := range embeddingResponse.Data {
		embeddings = append(embeddings, emb.Embedding...)
	}

	return embeddings, nil
}
