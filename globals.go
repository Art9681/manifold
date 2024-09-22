// globals.go

package main

import (
	"sync"
)

var (
	globalWM     *WorkflowManager
	globalWMLock sync.RWMutex
)

// SetGlobalWorkflowManager sets the global WorkflowManager instance.
func SetGlobalWorkflowManager(wm *WorkflowManager) {
	globalWMLock.Lock()
	defer globalWMLock.Unlock()
	globalWM = wm
}

// GetGlobalWorkflowManager retrieves the global WorkflowManager instance.
func GetGlobalWorkflowManager() *WorkflowManager {
	globalWMLock.RLock()
	defer globalWMLock.RUnlock()
	return globalWM
}
