// manifold/utils.go

package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/jaypipes/ghw"        // Package for hardware information
	"github.com/shirou/gopsutil/mem" // Package for system memory information
)

type HostInfo struct {
	os     string
	arch   string
	cpus   int
	memory uint64
}

type GPUInfo struct {
	model              string
	totalNumberOfCores string
	metalSupport       string
}

type HostInfoProvider interface {
	GetOS() string
	GetArch() string
	GetCPUs() int
	GetMemory() uint64
	GetGPUs() ([]GPUInfo, error)
}

type GPUInfoProvider interface {
	GetModel() string
	GetTotalNumberOfCores() string
	GetMetalSupport() string
}

func (h *HostInfo) GetOS() string     { return h.os }
func (h *HostInfo) GetArch() string   { return h.arch }
func (h *HostInfo) GetCPUs() int      { return h.cpus }
func (h *HostInfo) GetMemory() uint64 { return h.memory }

func (g *GPUInfo) GetModel() string              { return g.model }
func (g *GPUInfo) GetTotalNumberOfCores() string { return g.totalNumberOfCores }
func (g *GPUInfo) GetMetalSupport() string       { return g.metalSupport }

func NewHostInfoProvider() HostInfoProvider {
	host := &HostInfo{
		os:     runtime.GOOS,
		arch:   runtime.GOARCH,
		cpus:   runtime.NumCPU(),
		memory: getMemoryTotal(), // in GB
	}

	return host
}

func getMemoryTotal() uint64 {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("Error getting memory info: %v", err)
		return 0
	}

	// Convert to GB
	totalMemory := vmStat.Total / 1024 / 1024 / 1024

	return totalMemory
}

func (h *HostInfo) GetGPUs() ([]GPUInfo, error) {
	switch runtime.GOOS {
	case "darwin":
		return getMacOSGPUInfo()
	case "linux", "windows":
		return getLinuxWindowsGPUInfo()
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func getMacOSGPUInfo() ([]GPUInfo, error) {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Printf("Error running system_profiler: %v", err)
		return nil, err
	}

	return parseMacOSGPUInfo(out.String())
}

func parseMacOSGPUInfo(input string) ([]GPUInfo, error) {
	var gpus []GPUInfo
	gpu := &GPUInfo{}

	for _, line := range strings.Split(input, "\n") {
		if strings.Contains(line, "Chipset Model") {
			gpu.model = strings.TrimSpace(strings.Split(line, ":")[1])
		}
		if strings.Contains(line, "Total Number of Cores") {
			gpu.totalNumberOfCores = strings.TrimSpace(strings.Split(line, ":")[1])
		}
		if strings.Contains(line, "Metal") {
			gpu.metalSupport = strings.TrimSpace(strings.Split(line, ":")[1])
		}
	}

	if gpu.model != "" {
		gpus = append(gpus, *gpu)
	}

	return gpus, nil
}

func getLinuxWindowsGPUInfo() ([]GPUInfo, error) {
	gpu, err := ghw.GPU()
	if err != nil {
		log.Printf("Error getting GPU info: %v", err)
		return nil, err
	}

	var gpus []GPUInfo
	for _, card := range gpu.GraphicsCards {
		gpus = append(gpus, GPUInfo{
			model: card.DeviceInfo.Product.Name,
		})
	}

	return gpus, nil
}

func GetHostInfo() (HostInfoProvider, error) {
	host := NewHostInfoProvider()
	_, err := host.GetGPUs() // We're calling this just to populate GPU info
	if err != nil {
		log.Printf("Error getting GPU info: %v", err)
	}
	return host, nil
}

func PrintHostInfo(host HostInfoProvider) {
	fmt.Printf("OS: %s\n", host.GetOS())
	fmt.Printf("Architecture: %s\n", host.GetArch())
	fmt.Printf("CPUs: %d\n", host.GetCPUs())
	fmt.Printf("Memory: %d GB\n", host.GetMemory())

	gpus, err := host.GetGPUs()
	if err != nil {
		log.Printf("Error getting GPU info: %v", err)
		return
	}

	for i, gpu := range gpus {
		fmt.Printf("GPU #%d\n", i+1)
		fmt.Printf("  Model: %s\n", gpu.GetModel())
		fmt.Printf("  Total Number of Cores: %s\n", gpu.GetTotalNumberOfCores())
		fmt.Printf("  Metal Support: %s\n", gpu.GetMetalSupport())
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
