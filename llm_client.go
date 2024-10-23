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
	Backend string
	LLMClient
}

func NewLocalLLMClient(baseURL string, model string, apiKey string, backend string) LLMClient {
	return &Client{BaseURL: baseURL, Model: model, APIKey: apiKey, Backend: backend}
}

func (client *Client) SetModel(model string) {
	client.Model = model
}

func (client *Client) SendCompletionRequest(payload *CompletionRequest) (*http.Response, error) {
	// Set model based on the client URL
	if client.BaseURL == "https://api.openai.com/v1" {
		fmt.Println(client.BaseURL)
		payload.Model = "chatgpt-4o-latest"
		payload = &CompletionRequest{
			Model:       payload.Model,
			Messages:    payload.Messages,
			Temperature: payload.Temperature,
			Stream:      true,
		}
	} else if client.BaseURL == "https://api.anthropic.com/v1" {
		fmt.Println(client.BaseURL)
		payload.Model = "claude-3-5-sonnet-20241022"
		// No need to create a new payload object, just modify the existing one
		payload.MaxTokens = 1024 // Default max tokens, if needed
	}

	// Convert the payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// Adjust the endpoint based on the backend
	var endpoint string
	if client.BaseURL == "https://api.openai.com/v1" {
		endpoint = "/chat/completions"
	} else if client.BaseURL == "https://api.anthropic.com/v1" {
		endpoint = "/messages"
	}

	// Create a new HTTP request
	req, err := http.NewRequest("POST", client.BaseURL+endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}

	// Default headers for all requests
	req.Header.Set("Content-Type", "application/json")

	// Set specific headers based on the backend
	if client.BaseURL == "https://api.openai.com/v1" {
		req.Header.Set("Authorization", "Bearer "+client.APIKey)
	} else if client.BaseURL == "https://api.anthropic.com/v1" {
		req.Header.Set("x-api-key", client.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	// Execute the HTTP request
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	return res, nil
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
