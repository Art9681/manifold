// service_test.go
package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockCmd is a mock for exec.Cmd
type MockCmd struct {
	mock.Mock
}

func (m *MockCmd) Start() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockCmd) Process() *os.Process {
	args := m.Called()
	return args.Get(0).(*os.Process)
}

func (m *MockCmd) Wait() error {
	args := m.Called()
	return args.Error(0)
}

func TestExternalServiceStartAndStop(t *testing.T) {
	// Since exec.Cmd cannot be easily mocked, we can test the logic without actually starting a process
	// Alternatively, use a real command like "echo" for demonstration purposes

	// Define a simple service configuration
	serviceConfig := ServiceConfig{
		Name:    "echo_service",
		Command: "echo",
		Args:    []string{"Hello, World!"},
		Host:    "localhost",
		Port:    12345,
	}

	service := NewExternalService(serviceConfig, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := service.Start(ctx)
	require.NoError(t, err, "Service.Start should not return an error")

	// Since "echo" exits immediately, stopping it should not cause issues
	err = service.Stop(ctx)
	require.NoError(t, err, "Service.Stop should not return an error")
}

func TestExternalServiceStartFailure(t *testing.T) {
	// Define a service with an invalid command
	serviceConfig := ServiceConfig{
		Name:    "invalid_service",
		Command: "nonexistent_command",
		Args:    []string{},
		Host:    "localhost",
		Port:    12345,
	}

	service := NewExternalService(serviceConfig, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := service.Start(ctx)
	require.Error(t, err, "Service.Start should return an error for invalid command")
	assert.Contains(t, err.Error(), "failed to start")
}

func TestExternalServiceStopWithoutStart(t *testing.T) {
	// Define a simple service configuration
	serviceConfig := ServiceConfig{
		Name:    "echo_service",
		Command: "echo",
		Args:    []string{"Hello, World!"},
		Host:    "localhost",
		Port:    12345,
	}

	service := NewExternalService(serviceConfig, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt to stop without starting
	err := service.Stop(ctx)
	assert.Error(t, err, "Service.Stop should return an error when service is not running")
	assert.Contains(t, err.Error(), "is not running")
}
