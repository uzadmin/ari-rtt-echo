package main
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/pion/rtp"
)

func main() {
	// Test the full RTP round-trip flow with metrics collection
	fmt.Println("Starting metrics test...")

	// Create a UDP connection to simulate RTP traffic
	echoConn, err := net.Dial("udp", "localhost:4000")
	if err != nil {
		log.Fatalf("Failed to dial echo server: %v", err)
	}
	defer echoConn.Close()

	// Send RTP packets
	sequenceNumber := uint16(200)
	for i := 0; i < 10; i++ {
		// Create RTP packet
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Padding:        false,
				Extension:      false,
				Marker:         false,
				PayloadType:    0, // PCMU
				SequenceNumber: sequenceNumber + uint16(i),
				Timestamp:      uint32(i * 160), // 20ms * 8000Hz
				SSRC:           0x87654321,
			},
			Payload: []byte("metrics test payload data"),
		}

		// Marshal packet
		packetData, err := packet.Marshal()
		if err != nil {
			log.Printf("Failed to marshal RTP packet: %v", err)
			continue
		}

		// Send to echo server
		_, err = echoConn.Write(packetData)
		if err != nil {
			log.Printf("Failed to send RTP packet: %v", err)
			continue
		}

		fmt.Printf("Sent packet: Seq=%d, TS=%d\n", packet.SequenceNumber, packet.Timestamp)
		time.Sleep(20 * time.Millisecond) // 20ms interval
	}

	// Wait a moment for processing
	time.Sleep(100 * time.Millisecond)

	// Check metrics endpoint
	fmt.Println("Checking metrics...")
	resp, err := http.Get("http://localhost:9090/metrics")
	if err != nil {
		log.Printf("Failed to get metrics: %v", err)
	} else {
		defer resp.Body.Close()
		fmt.Printf("Metrics endpoint status: %d\n", resp.StatusCode)
		if resp.StatusCode == 200 {
			fmt.Println("Metrics endpoint is working!")
		}
	}

	fmt.Println("Metrics test completed")
}