package main
package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

func main() {
	fmt.Println("Simple Echo Server Test")
	fmt.Println("======================")

	// Test basic UDP connectivity to echo server
	testUDPEcho()
}

func testUDPEcho() {
	fmt.Println("\nTesting UDP Echo on port 4000...")
	
	// Resolve address
	addr, err := net.ResolveUDPAddr("udp", "localhost:4000")
	if err != nil {
		log.Fatal("Failed to resolve address:", err)
	}

	// Create connection
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatal("Failed to dial UDP:", err)
	}
	defer conn.Close()

	fmt.Println("✓ Connected to echo server")

	// Send test message
	message := "Hello Echo Server!"
	_, err = conn.Write([]byte(message))
	if err != nil {
		log.Fatal("Failed to send message:", err)
	}

	fmt.Printf("✓ Sent message: %s\n", message)

	// Read response
	buffer := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buffer)
	if err != nil {
		log.Fatal("Failed to read response:", err)
	}

	response := string(buffer[:n])
	fmt.Printf("✓ Received echo: %s\n", response)

	if response == message {
		fmt.Println("✓ Echo server working correctly!")
	} else {
		fmt.Println("⚠ Echo response doesn't match sent message")
	}
}