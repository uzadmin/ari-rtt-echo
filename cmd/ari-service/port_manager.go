package main

import (
	"log"
	"sync"
	"time"
)

// PortManager manages allocation and release of RTP ports
type PortManager struct {
	minPort int
	maxPort int
	ports   []bool // bitset for port usage (index = port - minPort)
	mu      sync.Mutex
}

// NewPortManager creates a new port manager
func NewPortManager(minPort, maxPort int) *PortManager {
	size := maxPort - minPort + 1
	ports := make([]bool, size)

	return &PortManager{
		minPort: minPort,
		maxPort: maxPort,
		ports:   ports,
	}
}

// GetPort allocates an available port
func (pm *PortManager) GetPort() (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for port := pm.minPort; port <= pm.maxPort; port++ {
		idx := port - pm.minPort
		if !pm.ports[idx] {
			pm.ports[idx] = true
			log.Printf("%s [PortManager] INFO: Allocated port %d", time.Now().Format("2006-01-02 15:04:05"), port)
			return port, nil
		}
	}

	log.Printf("%s [PortManager] ERROR: No ports available in range %d-%d", time.Now().Format("2006-01-02 15:04:05"), pm.minPort, pm.maxPort)
	return 0, ErrNoPortsAvailable
}

// ReleasePort releases a port back to the pool
func (pm *PortManager) ReleasePort(port int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if port >= pm.minPort && port <= pm.maxPort {
		idx := port - pm.minPort
		pm.ports[idx] = false
		log.Printf("%s [PortManager] INFO: Released port %d", time.Now().Format("2006-01-02 15:04:05"), port)
	}
}

// ErrNoPortsAvailable is returned when no ports are available
var ErrNoPortsAvailable = &PortError{"no ports available"}

// PortError represents a port management error
type PortError struct {
	msg string
}

func (e *PortError) Error() string {
	return e.msg
}
