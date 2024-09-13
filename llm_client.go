package main

import "net/http"

type LLMClient interface {
	SendCompletionRequest(payload *CompletionRequest) (*http.Response, error)
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
