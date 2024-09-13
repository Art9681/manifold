// completions_test.go
package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockLLMClient is a mock implementation of the LLMClient interface
type MockLLMClient struct {
	mock.Mock
}

func (m *MockLLMClient) SendCompletionRequest(payload *CompletionRequest) (*http.Response, error) {
	args := m.Called(payload)
	return args.Get(0).(*http.Response), args.Error(1)
}

func TestSendCompletionRequest(t *testing.T) {
	mockClient := new(MockLLMClient)

	payload := &CompletionRequest{
		Model:       "test-model",
		Messages:    []Message{{Role: "user", Content: "Hello"}},
		Temperature: 0.5,
		MaxTokens:   100,
		Stream:      false,
	}

	// Mock response
	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
	}

	mockClient.On("SendCompletionRequest", payload).Return(mockResp, nil)

	client := NewLocalLLMClient("http://localhost:8080", "test-model", "test-api-key").(*Client)
	client.LLMClient = mockClient

	resp, err := client.SendCompletionRequest(payload)
	require.NoError(t, err, "SendCompletionRequest should not return an error")
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response status code should be 200")
	mockClient.AssertExpectations(t)
}

func TestSendCompletionRequestError(t *testing.T) {
	mockClient := new(MockLLMClient)

	payload := &CompletionRequest{
		Model:       "test-model",
		Messages:    []Message{{Role: "user", Content: "Hello"}},
		Temperature: 0.5,
		MaxTokens:   100,
		Stream:      false,
	}

	// Mock error
	mockClient.On("SendCompletionRequest", payload).Return(nil, errors.New("network error"))

	client := NewLocalLLMClient("http://localhost:8080", "test-model", "test-api-key").(*Client)
	client.LLMClient = mockClient

	resp, err := client.SendCompletionRequest(payload)
	assert.Error(t, err, "SendCompletionRequest should return an error")
	assert.Nil(t, resp, "Response should be nil on error")
	mockClient.AssertExpectations(t)
}

func TestStreamCompletionToWebSocket(t *testing.T) {
	// Mock LLMClient
	mockClient := new(MockLLMClient)

	payload := &CompletionRequest{
		Model:       "test-model",
		Messages:    []Message{{Role: "user", Content: "Hello"}},
		Temperature: 0.5,
		MaxTokens:   100,
		Stream:      true,
	}

	// Mock response body with SSE data
	sseData := "data: {\"choices\":[{\"finish_reason\":\"\",\"delta\":{\"content\":\"Hi there!\"}}]}\n" +
		"data: {\"choices\":[{\"finish_reason\":\"stop\",\"delta\":{\"content\":\" Goodbye!\"}}]}\n"

	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(sseData)),
	}

	mockClient.On("SendCompletionRequest", payload).Return(mockResp, nil)

	// Create a WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err, "WebSocket upgrade should not return an error")
		defer conn.Close()

		// Read the stream
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			// Echo the message back for testing
			err = conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	// Connect to the WebSocket server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "WebSocket dial should not return an error")
	defer ws.Close()

	responseBuffer := &bytes.Buffer{}

	err = StreamCompletionToWebSocket(ws, mockClient, 1, "test-model", payload, responseBuffer)
	require.Error(t, err, "StreamCompletionToWebSocket should return an error when finish_reason is 'stop'")
	assert.Contains(t, err.Error(), responseBuffer.String(), "Error message should contain the response buffer")
	mockClient.AssertExpectations(t)
}

// MockTool is a mock implementation of the Tool interface
type MockTool struct {
	mock.Mock
}

func (m *MockTool) Process(ctx context.Context, input string) (string, error) {
	args := m.Called(ctx, input)
	return args.String(0), args.Error(1)
}

func (m *MockTool) Enabled() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockTool) SetParams(params map[string]interface{}) error {
	args := m.Called(params)
	return args.Error(0)
}
