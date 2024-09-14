package main

import (
	"context"
	"errors"
	"fmt"
)

type Tool interface {
	Process(ctx context.Context, input string) (string, error)
	Enabled() bool
	SetParams(params map[string]interface{}) error
}

type ToolWrapper struct {
	Tool Tool
	Name string
}

type WorkflowManager struct {
	tools []ToolWrapper
}

func (wm *WorkflowManager) AddTool(tool Tool, name string) error {
	if tool.Enabled() {
		wm.tools = append(wm.tools, ToolWrapper{Tool: tool, Name: name})
		return nil
	}
	return fmt.Errorf("tool %s is not enabled and cannot be added", name)
}

func (wm *WorkflowManager) RemoveTool(name string) error {
	for i, wrapper := range wm.tools {
		if wrapper.Name == name {
			wm.tools = append(wm.tools[:i], wm.tools[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("tool %s not found", name)
}

func (wm *WorkflowManager) ListTools() []string {
	var toolNames []string
	for _, wrapper := range wm.tools {
		toolNames = append(toolNames, wrapper.Name)
	}
	return toolNames
}

func (wm *WorkflowManager) Run(ctx context.Context, prompt string) (string, error) {
	var result string
	for _, wrapper := range wm.tools {
		processed, err := wrapper.Tool.Process(ctx, prompt)
		if err != nil {
			return "", fmt.Errorf("error processing with tool %s: %w", wrapper.Name, err)
		}
		result += processed
	}
	if result == "" {
		return "", errors.New("no tools processed the input or no result generated")
	}
	return result, nil
}

type WebSearchTool struct {
	enabled      bool
	SearchEngine string
	Endpoint     string
	TopN         int
}

func (t *WebSearchTool) Process(ctx context.Context, input string) (string, error) {
	return fmt.Sprintf("WebSearchTool processed (SearchEngine=%s, TopN=%d): %s\n", t.SearchEngine, t.TopN, input), nil
}

func (t *WebSearchTool) Enabled() bool {
	return t.enabled
}

func (t *WebSearchTool) SetParams(params map[string]interface{}) error {
	if enabled, ok := params["enabled"].(bool); ok {
		t.enabled = enabled
	}
	if searchEngine, ok := params["search_engine"].(string); ok {
		t.SearchEngine = searchEngine
	}
	if endpoint, ok := params["endpoint"].(string); ok {
		t.Endpoint = endpoint
	}
	if topN, ok := params["top_n"].(int); ok {
		t.TopN = topN
	}
	return nil
}
