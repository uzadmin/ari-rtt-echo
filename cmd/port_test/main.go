package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

func main() {
	// Test if we can bind to ports in the range 4500-5000
	fmt.Println("Testing port availability in range 4500-5000...")

	successCount := 0
	failureCount := 0

	for port := 4500; port <= 4510; port++ {
		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
		if err != nil {
			log.Printf("Failed to resolve address for port %d: %v", port, err)
			failureCount++
			continue
		}

		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			log.Printf("Failed to bind to port %d: %v", port, err)
			failureCount++
			continue
		}

		log.Printf("Successfully bound to port %d", port)
		successCount++
		conn.Close()

		// Small delay to avoid overwhelming the system
		time.Sleep(10 * time.Millisecond)
	}

	fmt.Printf("Results: %d successful bindings, %d failures\n", successCount, failureCount)

	// Now test if we can connect to the echo server
	fmt.Println("Testing connection to echo server on port 4000...")

	echoAddr, err := net.ResolveUDPAddr("udp", "localhost:4000")
	if err != nil {
		log.Fatalf("Failed to resolve echo server address: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, echoAddr)
	if err != nil {
		log.Fatalf("Failed to connect to echo server: %v", err)
	}
	defer conn.Close()

	// Send a simple test packet
	testData := []byte("PORT_TEST")
	_, err = conn.Write(testData)
	if err != nil {
		log.Printf("Failed to send test packet: %v", err)
	} else {
		log.Println("Successfully sent test packet to echo server")
	}

	// Try to read a response
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buffer := make([]byte, 1500)
	n, err := conn.Read(buffer)
	if err != nil {
		log.Printf("Failed to read response from echo server: %v", err)
	} else {
		log.Printf("Received response from echo server: %s", string(buffer[:n]))
	}
}
