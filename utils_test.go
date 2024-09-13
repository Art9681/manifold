// utils_test.go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHostInfoProvider(t *testing.T) {
	host := NewHostInfoProvider()
	require.NotNil(t, host, "NewHostInfoProvider should not return nil")

	assert.NotEmpty(t, host.GetOS(), "OS should not be empty")
	assert.NotEmpty(t, host.GetArch(), "Architecture should not be empty")
	assert.Greater(t, host.GetCPUs(), 0, "Number of CPUs should be greater than 0")
	assert.Greater(t, host.GetMemory(), uint64(0), "Memory should be greater than 0")
}

func TestGetGPUs(t *testing.T) {
	host := NewHostInfoProvider()
	gpus, err := host.GetGPUs()

	// Depending on the test environment, there may or may not be GPUs
	// We just ensure that the function doesn't return an unexpected error
	if host.GetOS() == "darwin" || host.GetOS() == "linux" || host.GetOS() == "windows" {
		require.NoError(t, err, "GetGPUs should not return an error on supported OS")
	} else {
		require.Error(t, err, "GetGPUs should return an error on unsupported OS")
	}

	// Further assertions can be made based on the actual GPU data
	if err == nil {
		for _, gpu := range gpus {
			assert.NotEmpty(t, gpu.model, "GPU model should not be empty")
		}
	}
}

func TestPrintHostInfo(t *testing.T) {
	// Capture the output
	var buf bytes.Buffer
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	host := NewHostInfoProvider()
	PrintHostInfo(host)

	w.Close()
	buf.ReadFrom(r)
	os.Stdout = originalStdout

	output := buf.String()
	assert.Contains(t, output, "OS: ", "Output should contain OS information")
	assert.Contains(t, output, "Architecture: ", "Output should contain Architecture information")
	assert.Contains(t, output, "CPUs: ", "Output should contain CPU information")
	assert.Contains(t, output, "Memory: ", "Output should contain Memory information")
}

func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()
	existingFile := filepath.Join(tempDir, "existing.txt")
	nonExistingFile := filepath.Join(tempDir, "nonexisting.txt")

	// Create an existing file
	err := os.WriteFile(existingFile, []byte("test"), 0644)
	assert.NoError(t, err, "Writing to existingFile should not return an error")

	// Test existing file
	exists := fileExists(existingFile)
	assert.True(t, exists, "fileExists should return true for existing file")

	// Test non-existing file
	exists = fileExists(nonExistingFile)
	assert.False(t, exists, "fileExists should return false for non-existing file")
}
