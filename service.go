// manifold/service.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"
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

// UpdateWorkflowManagerForToolToggle handles enabling or disabling tools, including starting/stopping services.
func UpdateWorkflowManagerForToolToggle(toolName string, enabled bool, config *Config) {
	wm := GetGlobalWorkflowManager()
	if wm == nil {
		log.Println("WorkflowManager is not initialized")
		return
	}

	switch toolName {
	case "teams":
		// Handle the Teams tool specifically
		var teamsTool *TeamsTool
		for _, wrapper := range wm.tools {
			if wrapper.Name == "teams" {
				var ok bool
				teamsTool, ok = wrapper.Tool.(*TeamsTool)
				if !ok {
					log.Println("Failed to cast to TeamsTool")
					return
				}
				break
			}
		}

		// if teamsTool == nil {
		// 	log.Println("TeamsTool not found in WorkflowManager")
		// 	return
		// }

		if enabled {
			args := config.Services[5].Args

			// Create a new ServiceConfig for the Teams tool
			teamsTool.serviceConfig = ServiceConfig{
				Name:    "teams",
				Command: config.Services[5].Command,
				Args:    args,
			}

			// Start the ExternalService
			if teamsTool.service == nil {
				teamsTool.service = NewExternalService(teamsTool.serviceConfig, false) // Set verbose as needed
				if err := teamsTool.service.Start(context.Background()); err != nil {
					log.Printf("Failed to start Teams ExternalService: %v", err)
					return
				}

				// Initialize the LLMClient pointing to the Teams service
				baseURL := fmt.Sprintf("http://%s:%d/v1", teamsTool.serviceConfig.Host, teamsTool.serviceConfig.Port)
				teamsTool.client = NewLocalLLMClient(baseURL, "", "") // Adjust APIKey if needed

				log.Printf("Teams tool '%s' has been enabled and service started", toolName)

				// Logic to register the tool with the WorkflowManager
				tool, err := CreateToolByName(toolName)
				if err != nil {
					log.Printf("Failed to create tool '%s': %v", toolName, err)
					return
				}
				err = wm.AddTool(tool, toolName)
				if err != nil {
					log.Printf("Failed to add tool '%s' to WorkflowManager: %v", toolName, err)
				}
				log.Printf("Tool '%s' has been enabled and added to WorkflowManager", toolName)
			}
		} else {
			// Stop the ExternalService
			if teamsTool.service != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := teamsTool.service.Stop(ctx); err != nil {
					log.Printf("Failed to stop Teams ExternalService: %v", err)
				} else {
					teamsTool.service = nil
					teamsTool.client = nil
					log.Printf("Teams tool '%s' has been disabled and service stopped", toolName)
				}
			}
		}
	default:
		if enabled {
			// Logic to register the tool with the WorkflowManager
			tool, err := CreateToolByName(toolName)
			if err != nil {
				log.Printf("Failed to create tool '%s': %v", toolName, err)
				return
			}
			err = wm.AddTool(tool, toolName)
			if err != nil {
				log.Printf("Failed to add tool '%s' to WorkflowManager: %v", toolName, err)
			}
			log.Printf("Tool '%s' has been enabled and added to WorkflowManager", toolName)
		} else {
			// Logic to unregister the tool from the WorkflowManager
			err := wm.RemoveTool(toolName)
			if err != nil {
				log.Printf("Failed to remove tool '%s' from WorkflowManager: %v", toolName, err)
			} else {
				log.Printf("Tool '%s' has been disabled and removed from WorkflowManager", toolName)
			}
		}
	}

}
