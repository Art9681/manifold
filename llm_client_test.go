// llm_client_test.go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLocalLLMClient(t *testing.T) {
	client := NewLocalLLMClient("http://localhost:8080", "test-model", "test-api-key")
	require.NotNil(t, client, "NewLocalLLMClient should not return nil")

	localClient, ok := client.(*Client)
	assert.True(t, ok, "Client should be of type *Client")
	assert.Equal(t, "http://localhost:8080", localClient.BaseURL)
	assert.Equal(t, "test-model", localClient.Model)
	assert.Equal(t, "test-api-key", localClient.APIKey)
}
