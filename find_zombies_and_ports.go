package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type Metrics struct {
	TotalChannels   int     `json:"total_channels"`
	ActiveChannels  int     `json:"active_channels"`
	TotalLatencies  int64   `json:"total_latencies"`
	P50Latency      float64 `json:"p50_latency"`
	P95Latency      float64 `json:"p95_latency"`
	P99Latency      float64 `json:"p99_latency"`
	MaxLatency      float64 `json:"max_latency"`
	AvgLatency      float64 `json:"avg_latency"`
	LateRatio       float64 `json:"late_ratio"`
	PacketLossRatio float64 `json:"packet_loss_ratio"`
	Timestamp       string  `json:"timestamp"`
}

func main() {
	fmt.Println("=== Zombie Channel and Port Monitor ===")
	fmt.Println("Monitoring for zombie channels and unclosed ports...")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	// Track metrics over time to detect zombies
	var previousLatencies int64
	var zombieCheckCount int

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Check ARI service metrics
		metrics, err := getMetrics()
		if err != nil {
			fmt.Printf("Error getting metrics: %v\n", err)
			continue
		}

		fmt.Printf("[%s] Active Channels: %d, Total Channels: %d, Latencies: %d\n",
			time.Now().Format("15:04:05"), metrics.ActiveChannels, metrics.TotalChannels, metrics.TotalLatencies)

		// Check for zombie channels - active channels with no new latencies
		if metrics.ActiveChannels > 0 {
			if previousLatencies == metrics.TotalLatencies && metrics.TotalLatencies > 0 {
				zombieCheckCount++
				if zombieCheckCount >= 3 {
					fmt.Printf("⚠️  WARNING: Possible zombie channels detected! No new latencies in %d checks\n", zombieCheckCount)
					fmt.Printf("   Active channels: %d, but no new latencies recorded\n", metrics.ActiveChannels)
				}
			} else {
				zombieCheckCount = 0 // Reset counter if we see new latencies
			}
		} else {
			zombieCheckCount = 0 // Reset counter if no active channels
		}

		previousLatencies = metrics.TotalLatencies

		// Check for unclosed ports
		unclosedPorts, err := getUnclosedPorts()
		if err != nil {
			fmt.Printf("Error checking ports: %v\n", err)
		} else if len(unclosedPorts) > 0 {
			fmt.Printf("⚠️  WARNING: Found %d potentially unclosed ports in range 21000-31000\n", len(unclosedPorts))
			if len(unclosedPorts) <= 10 {
				fmt.Printf("   Ports: %v\n", unclosedPorts)
			} else {
				fmt.Printf("   First 10 ports: %v\n", unclosedPorts[:10])
			}
		}

		fmt.Println("---")
	}
}

func getMetrics() (*Metrics, error) {
	resp, err := http.Get("http://localhost:9090/metrics")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to metrics endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var metrics Metrics
	if err := json.Unmarshal(body, &metrics); err != nil {
		return nil, fmt.Errorf("failed to parse metrics JSON: %v", err)
	}

	return &metrics, nil
}

func getUnclosedPorts() ([]string, error) {
	// Use lsof to find UDP ports in our range
	cmd := exec.Command("lsof", "-i", ":21000-31000", "-P", "-n")
	output, err := cmd.Output()
	if err != nil {
		// lsof returns exit code 1 when no matches found, which is not an error for us
		if _, ok := err.(*exec.ExitError); ok {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to run lsof: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	var ports []string

	for _, line := range lines {
		if strings.Contains(line, "UDP") && strings.Contains(line, ":") {
			// Extract port number
			fields := strings.Fields(line)
			for _, field := range fields {
				if strings.Contains(field, ":") && strings.Contains(field, "->") {
					// This is a connection field, skip it
					continue
				}
				if strings.Contains(field, ":") {
					parts := strings.Split(field, ":")
					if len(parts) == 2 {
						port := parts[1]
						// Check if it's in our range
						ports = append(ports, port)
					}
				}
			}
		}
	}

	return ports, nil
}
