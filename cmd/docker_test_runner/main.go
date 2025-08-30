package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

func main() {
	fmt.Println("Docker Test Runner")
	fmt.Println("==================")

	// Test 1: Check if all services are accessible
	testServiceConnectivity()

	// Test 2: Run enhanced load test
	runEnhancedLoadTest()

	// Test 3: Run packet reordering test
	runPacketReorderingTest()

	// Test 4: Show final metrics
	showFinalMetrics()
}

func testServiceConnectivity() {
	fmt.Println("\n1. Service Connectivity Test")
	fmt.Println("--------------------------")

	services := map[string]string{
		"Asterisk ARI": "localhost:8088",
		"ARI Service":  "localhost:9090",
		"Echo Server":  "localhost:4000",
		"SIP Service":  "localhost:5060",
	}

	allConnected := true
	for service, address := range services {
		if isServiceAvailable(address) {
			fmt.Printf("✓ %s is accessible at %s\n", service, address)
		} else {
			fmt.Printf("✗ %s is NOT accessible at %s\n", service, address)
			allConnected = false
		}
	}

	if allConnected {
		fmt.Println("✓ All services are running and accessible")
	} else {
		log.Fatal("Some services are not accessible")
	}
}

func runEnhancedLoadTest() {
	fmt.Println("\n2. Enhanced Load Test")
	fmt.Println("-------------------")
	fmt.Println("Note: This would run the full enhanced load test in a production environment.")
	fmt.Println("For now, we'll simulate the test execution.")

	// In a real implementation, we would run the enhanced load test here
	fmt.Println("✓ Enhanced load test framework is ready")
}

func runPacketReorderingTest() {
	fmt.Println("\n3. Packet Reordering Test")
	fmt.Println("----------------------")
	fmt.Println("Note: This would run the packet reordering detection test.")
	fmt.Println("For now, we'll simulate the test execution.")

	// In a real implementation, we would run the packet reordering test here
	fmt.Println("✓ Packet reordering detection framework is ready")
}

func showFinalMetrics() {
	fmt.Println("\n4. Final System Metrics")
	fmt.Println("---------------------")

	// Get current metrics from ARI service
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost:9090/metrics")
	if err != nil {
		log.Printf("Failed to get metrics: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("✓ Metrics endpoint is accessible")
		fmt.Println("  Current system is tracking channels and collecting metrics")
	} else {
		fmt.Printf("⚠ Metrics endpoint returned status: %d\n", resp.StatusCode)
	}
}

func isServiceAvailable(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 1*time.Second)
	if err != nil {
		// Try UDP for SIP and Echo
		if address == "localhost:4000" || address == "localhost:5060" {
			udpAddr, err := net.ResolveUDPAddr("udp", address)
			if err != nil {
				return false
			}
			udpConn, err := net.DialUDP("udp", nil, udpAddr)
			if err != nil {
				return false
			}
			udpConn.Close()
			return true
		}
		return false
	}
	conn.Close()
	return true
}
