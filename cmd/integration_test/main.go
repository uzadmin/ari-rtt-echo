package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/rtp"
)

func main() {
	// Test the full RTP round-trip flow
	fmt.Println("Starting integration test...")

	// Create a UDP connection to simulate Asterisk sending RTP to our system
	conn, err := net.Dial("udp", "localhost:4500")
	if err != nil {
		log.Fatalf("Failed to dial UDP: %v", err)
	}
	defer conn.Close()

	// Send RTP packets to the echo server port (simulating the flow through our system)
	echoConn, err := net.Dial("udp", "localhost:4000")
	if err != nil {
		log.Fatalf("Failed to dial echo server: %v", err)
	}
	defer echoConn.Close()

	// Send a few RTP packets and measure round-trip time
	var wg sync.WaitGroup
	sequenceNumber := uint16(100)

	// Listen for echoed packets
	wg.Add(1)
	go func() {
		defer wg.Done()
		buffer := make([]byte, 1500)
		for i := 0; i < 5; i++ {
			n, err := echoConn.Read(buffer)
			if err != nil {
				log.Printf("Error reading from echo server: %v", err)
				return
			}

			// Parse the echoed RTP packet
			packet := &rtp.Packet{}
			if err := packet.Unmarshal(buffer[:n]); err != nil {
				log.Printf("Failed to parse echoed RTP packet: %v", err)
				continue
			}

			fmt.Printf("Received echoed packet: Seq=%d, TS=%d, SSRC=%d\n",
				packet.SequenceNumber, packet.Timestamp, packet.SSRC)
		}
	}()

	// Send RTP packets
	for i := 0; i < 5; i++ {
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
				SSRC:           0x12345678,
			},
			Payload: []byte("test payload data"),
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

	wg.Wait()
	fmt.Println("Integration test completed")
}
