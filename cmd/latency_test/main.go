package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pion/rtp"
)

func main() {
	// Create UDP connection to echo server
	addr, err := net.ResolveUDPAddr("udp", "localhost:4000")
	if err != nil {
		log.Fatal("Failed to resolve UDP address:", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatal("Failed to dial UDP:", err)
	}
	defer conn.Close()

	fmt.Println("Connected to echo server on port 4000")

	// Test round-trip time
	latencies := make([]time.Duration, 10)

	for i := 0; i < 10; i++ {
		// Create RTP packet
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Padding:        false,
				Extension:      false,
				Marker:         false,
				PayloadType:    0, // PCMU
				SequenceNumber: uint16(2000 + i),
				Timestamp:      uint32(i * 160), // 20ms * 8000Hz
				SSRC:           0x87654321,
			},
			Payload: []byte("latency test payload"),
		}

		// Marshal packet
		packetBytes, err := packet.Marshal()
		if err != nil {
			log.Printf("Failed to marshal packet %d: %v", i, err)
			continue
		}

		// Send packet and measure round-trip time
		start := time.Now()
		_, err = conn.Write(packetBytes)
		if err != nil {
			log.Printf("Failed to send packet %d: %v", i, err)
			continue
		}

		// Read echoed packet
		buffer := make([]byte, 1500)
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, err := conn.Read(buffer)
		if err != nil {
			log.Printf("Failed to read echoed packet %d: %v", i, err)
			continue
		}

		elapsed := time.Since(start)
		latencies[i] = elapsed

		// Parse echoed packet
		echoedPacket := &rtp.Packet{}
		if err := echoedPacket.Unmarshal(buffer[:n]); err != nil {
			log.Printf("Failed to parse echoed packet %d: %v", i, err)
			continue
		}

		fmt.Printf("Packet %d: RTT=%v, Seq=%d, TS=%d\n",
			i, elapsed, echoedPacket.SequenceNumber, echoedPacket.Timestamp)
	}

	// Calculate statistics
	var total time.Duration
	min := time.Hour
	max := time.Duration(0)

	for _, latency := range latencies {
		total += latency
		if latency < min {
			min = latency
		}
		if latency > max {
			max = latency
		}
	}

	avg := total / time.Duration(len(latencies))

	fmt.Printf("\n=== Echo Server Latency Results ===\n")
	fmt.Printf("Min: %v\n", min)
	fmt.Printf("Max: %v\n", max)
	fmt.Printf("Avg: %v\n", avg)
	fmt.Printf("Measurements: %d\n", len(latencies))
}
