package main

import (
	"context"
	"errors"
	"fmt"
)

type Tool interface {
	Process(ctx context.Context, input string) (string, error) // Process the input prompt
	Enabled() bool
	SetParams(params map[string]interface{}) error // Set tool-specific parameters
}

type ToolWrapper struct {
	Tool Tool
	Name string
}

// WorkflowManager manages the execution of enabled tools
type WorkflowManager struct {
	tools []ToolWrapper
}

// AddTool adds a new tool to the workflow manager only if it is enabled
func (wm *WorkflowManager) AddTool(tool Tool, name string) error {
	if tool.Enabled() {
		wm.tools = append(wm.tools, ToolWrapper{Tool: tool, Name: name})
		return nil
	}
	return fmt.Errorf("tool %s is not enabled and cannot be added", name)
}

// RemoveTool removes a tool from the workflow by name
func (wm *WorkflowManager) RemoveTool(name string) error {
	for i, wrapper := range wm.tools {
		if wrapper.Name == name {
			wm.tools = append(wm.tools[:i], wm.tools[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("tool %s not found", name)
}

// ListTools lists all tools in the workflow manager
func (wm *WorkflowManager) ListTools() []string {
	var toolNames []string
	for _, wrapper := range wm.tools {
		toolNames = append(toolNames, wrapper.Name)
	}
	return toolNames
}

// Run runs the input prompt through all enabled tools and returns the final result
func (wm *WorkflowManager) Run(ctx context.Context, prompt string) (string, error) {
	var result string
	for _, wrapper := range wm.tools {
		processed, err := wrapper.Tool.Process(ctx, prompt)
		if err != nil {
			return "", fmt.Errorf("error processing with tool %s: %w", wrapper.Name, err)
		}
		result += processed // Accumulate results from each tool
	}
	if result == "" {
		return "", errors.New("no tools processed the input or no result generated")
	}
	return result, nil
}

// WebGetTool with configurable parameters
type WebGetTool struct {
	enabled bool
	TopN    int // Parameter: Top N search results to retrieve
}

func (t *WebGetTool) Process(ctx context.Context, input string) (string, error) {
	// Simulate web scraping or processing logic
	return fmt.Sprintf("WebGetTool processed (TopN=%d): %s\n", t.TopN, input), nil
}

func (t *WebGetTool) Enabled() bool {
	return t.enabled
}

func (t *WebGetTool) SetParams(params map[string]interface{}) error {
	if topN, ok := params["TopN"].(int); ok {
		t.TopN = topN
		return nil
	}
	return errors.New("invalid parameter: TopN")
}

// MemoryTool with configurable parameters
type MemoryTool struct {
	enabled   bool
	MaxMemory int // Parameter: Maximum memory to use
}

func (t *MemoryTool) Process(ctx context.Context, input string) (string, error) {
	// Simulate memory interaction logic
	return fmt.Sprintf("MemoryTool processed (MaxMemory=%d): %s\n", t.MaxMemory, input), nil
}

func (t *MemoryTool) Enabled() bool {
	return t.enabled
}

func (t *MemoryTool) SetParams(params map[string]interface{}) error {
	if maxMemory, ok := params["MaxMemory"].(int); ok {
		t.MaxMemory = maxMemory
		return nil
	}
	return errors.New("invalid parameter: MaxMemory")
}
