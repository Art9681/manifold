// manifold/service.go

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// ManagedService defines the interface for external services
type ManagedService interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// ExternalService implements the ManagedService interface
type ExternalService struct {
	config  ServiceConfig
	cmd     *exec.Cmd
	verbose bool
}

// NewExternalService creates a new ExternalService instance
func NewExternalService(config ServiceConfig, verbose bool) *ExternalService {
	return &ExternalService{
		config:  config,
		verbose: verbose,
	}
}

// Start launches the external service process
func (es *ExternalService) Start(ctx context.Context) error {
	es.cmd = exec.CommandContext(ctx, es.config.Command, es.config.Args...)

	if es.verbose {
		es.cmd.Stdout = os.Stdout
		es.cmd.Stderr = os.Stderr
	} else {
		es.cmd.Stdout = nil
		es.cmd.Stderr = nil
	}

	if err := es.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", es.config.Name, err)
	}

	fmt.Printf("%s started with PID %d\n", es.config.Name, es.cmd.Process.Pid)
	return nil
}

// Stop terminates the external service process
func (es *ExternalService) Stop(ctx context.Context) error {
	if es.cmd == nil || es.cmd.Process == nil {
		return fmt.Errorf("%s is not running", es.config.Name)
	}

	err := es.cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		return fmt.Errorf("failed to stop %s: %w", es.config.Name, err)
	}

	// Create a channel to receive the result of Wait()
	done := make(chan error, 1)
	go func() {
		_, err := es.cmd.Process.Wait()
		done <- err
	}()

	// Wait for the process to exit or for the context to be canceled
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("error waiting for %s to stop: %w", es.config.Name, err)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	fmt.Printf("%s stopped\n", es.config.Name)
	return nil
}
