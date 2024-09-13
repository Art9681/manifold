// tool_test.go
package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowManagerAddAndRemoveTool(t *testing.T) {
	wm := &WorkflowManager{}

	// Create mock tools
	webTool := &WebGetTool{enabled: true, TopN: 5}
	memoryTool := &MemoryTool{enabled: true, MaxMemory: 1024}

	// Add tools
	err := wm.AddTool(webTool, "WebGetTool")
	require.NoError(t, err, "Adding enabled WebGetTool should not return an error")

	err = wm.AddTool(memoryTool, "MemoryTool")
	require.NoError(t, err, "Adding enabled MemoryTool should not return an error")

	// List tools
	tools := wm.ListTools()
	assert.Len(t, tools, 2, "WorkflowManager should have two tools")
	assert.Contains(t, tools, "WebGetTool")
	assert.Contains(t, tools, "MemoryTool")

	// Remove a tool
	err = wm.RemoveTool("WebGetTool")
	require.NoError(t, err, "Removing existing WebGetTool should not return an error")

	// List tools again
	tools = wm.ListTools()
	assert.Len(t, tools, 1, "WorkflowManager should have one tool after removal")
	assert.Contains(t, tools, "MemoryTool")
	assert.NotContains(t, tools, "WebGetTool")

	// Attempt to remove a non-existent tool
	err = wm.RemoveTool("NonExistentTool")
	assert.Error(t, err, "Removing non-existent tool should return an error")
}

func TestWebGetToolProcess(t *testing.T) {
	tool := &WebGetTool{enabled: true, TopN: 3}
	ctx := context.Background()
	input := "Test input for WebGetTool"

	output, err := tool.Process(ctx, input)
	require.NoError(t, err, "WebGetTool.Process should not return an error")
	expectedOutput := "WebGetTool processed (TopN=3): Test input for WebGetTool\n"
	assert.Equal(t, expectedOutput, output, "WebGetTool.Process output should match expected")
}

func TestMemoryToolSetParams(t *testing.T) {
	tool := &MemoryTool{enabled: true, MaxMemory: 512}

	// Valid parameters
	params := map[string]interface{}{
		"MaxMemory": 2048,
	}

	err := tool.SetParams(params)
	require.NoError(t, err, "Setting valid parameters should not return an error")
	assert.Equal(t, 2048, tool.MaxMemory, "MaxMemory should be updated to 2048")

	// Invalid parameters
	invalidParams := map[string]interface{}{
		"InvalidParam": 100,
	}

	err = tool.SetParams(invalidParams)
	assert.Error(t, err, "Setting invalid parameters should return an error")
	assert.Equal(t, "invalid parameter: MaxMemory", err.Error())
}

func TestWorkflowManagerRun(t *testing.T) {
	wm := &WorkflowManager{}

	// Add tools
	webTool := &WebGetTool{enabled: true, TopN: 2}
	memoryTool := &MemoryTool{enabled: true, MaxMemory: 1024}

	err := wm.AddTool(webTool, "WebGetTool")
	require.NoError(t, err, "Adding WebGetTool should not return an error")

	err = wm.AddTool(memoryTool, "MemoryTool")
	require.NoError(t, err, "Adding MemoryTool should not return an error")

	ctx := context.Background()
	input := "Initial prompt"

	output, err := wm.Run(ctx, input)
	require.NoError(t, err, "WorkflowManager.Run should not return an error")

	expectedOutput := "WebGetTool processed (TopN=2): Initial prompt\nMemoryTool processed (MaxMemory=1024): Initial prompt\n"
	assert.Equal(t, expectedOutput, output, "WorkflowManager.Run output should match expected")
}

func TestWorkflowManagerRunNoTools(t *testing.T) {
	wm := &WorkflowManager{}

	ctx := context.Background()
	input := "Initial prompt"

	output, err := wm.Run(ctx, input)
	assert.Error(t, err, "WorkflowManager.Run should return an error when no tools are present")
	assert.Equal(t, "no tools processed the input or no result generated", err.Error())
	assert.Equal(t, "", output, "Output should be empty when no tools are present")
}
