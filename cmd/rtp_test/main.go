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

	// Create RTP packet
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         true,
			PayloadType:    0, // PCMU
			SequenceNumber: 1000,
			Timestamp:      4294967295, // Start with a high timestamp
			SSRC:           123456789,  // Some SSRC
		},
		Payload: []byte("Hello, RTP Echo Server!"),
	}

	// Send multiple packets to generate metrics
	for i := 0; i < 100; i++ {
		// Update sequence number and timestamp
		packet.SequenceNumber = uint16(1000 + i)
		packet.Timestamp = 4294967295 + uint32(i*160) // Increment by 160 samples (20ms at 8kHz)

		// Marshal packet
		packetBytes, err := packet.Marshal()
		if err != nil {
			log.Printf("Failed to marshal packet %d: %v", i, err)
			continue
		}

		// Send packet
		_, err = conn.Write(packetBytes)
		if err != nil {
			log.Printf("Failed to send packet %d: %v", i, err)
			continue
		}

		fmt.Printf("Sent packet %d: Seq=%d, TS=%d\n", i, packet.SequenceNumber, packet.Timestamp)

		// Wait a bit between packets
		time.Sleep(20 * time.Millisecond)
	}

	fmt.Println("Sent 100 RTP packets to echo server")

	// Wait a bit for processing
	time.Sleep(2 * time.Second)

	// Check metrics
	fmt.Println("Checking metrics...")
	metricsResp, err := net.Dial("tcp", "localhost:9090")
	if err != nil {
		log.Fatal("Failed to connect to metrics server:", err)
	}
	defer metricsResp.Close()

	_, err = metricsResp.Write([]byte("GET /metrics HTTP/1.0\r\n\r\n"))
	if err != nil {
		log.Fatal("Failed to send metrics request:", err)
	}

	buf := make([]byte, 4096)
	n, err := metricsResp.Read(buf)
	if err != nil {
		log.Fatal("Failed to read metrics response:", err)
	}

	fmt.Printf("Metrics response:\n%s\n", string(buf[:n]))
}
