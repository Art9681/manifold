package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type LLMClient interface {
	SendCompletionRequest(payload *CompletionRequest) (*http.Response, error)
	SendEmbeddingRequest(payload *EmbeddingRequest) (*http.Response, error)
	SetModel(model string)
}

type Client struct {
	BaseURL string
	Model   string
	APIKey  string
	LLMClient
}

func NewLocalLLMClient(baseURL string, model string, apiKey string) LLMClient {
	return &Client{BaseURL: baseURL, Model: model, APIKey: apiKey}
}

func (client *Client) SetModel(model string) {
	client.Model = model
}

func (client *Client) SendEmbeddingRequest(payload *EmbeddingRequest) (*http.Response, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := "http://localhost:32184/v1/embeddings"

	fmt.Println("Sending embedding request to:", url)

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
