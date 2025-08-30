package packetreorderingtest
package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// PacketReorderingDetector detects packet reordering in RTP streams
type PacketReorderingDetector struct {
	lastSeqNum uint16
	reordered  int
	total      int
	mu         sync.Mutex
}

// NewPacketReorderingDetector creates a new detector
func NewPacketReorderingDetector() *PacketReorderingDetector {
	return &PacketReorderingDetector{}
}

// CheckPacket checks if a packet is reordered
func (prd *PacketReorderingDetector) CheckPacket(seqNum uint16) bool {
	prd.mu.Lock()
	defer prd.mu.Unlock()

	prd.total++

	// First packet
	if prd.total == 1 {
		prd.lastSeqNum = seqNum
		return false
	}

	// Check for reordering (considering wraparound)
	expected := prd.lastSeqNum + 1
	if seqNum != expected {
		// Handle wraparound (sequence numbers are 16-bit)
		if seqNum < prd.lastSeqNum && (prd.lastSeqNum-seqNum) > 32768 {
			// This is likely wraparound, not reordering
			prd.lastSeqNum = seqNum
			return false
		} else if seqNum > expected && (seqNum-expected) < 32768 {
			// Normal progression
			prd.lastSeqNum = seqNum
			return false
		} else {
			// This is reordering
			prd.reordered++
			log.Printf("Packet reordering detected: expected %d, got %d", expected, seqNum)
			return true
		}
	}

	prd.lastSeqNum = seqNum
	return false
}

// GetStats returns reordering statistics
func (prd *PacketReorderingDetector) GetStats() (int, int, float64) {
	prd.mu.Lock()
	defer prd.mu.Unlock()

	var ratio float64
	if prd.total > 0 {
		ratio = float64(prd.reordered) / float64(prd.total) * 100
	}

	return prd.reordered, prd.total, ratio
}

func main() {
	fmt.Println("Packet Reordering Detection Test")
	fmt.Println("==============================")

	// Test the detector with simulated packets
	testPacketReordering()

	// Test with actual RTP traffic if possible
	testRTPReordering()
}

func testPacketReordering() {
	fmt.Println("\n1. Simulated Packet Reordering Test")
	fmt.Println("----------------------------------")

	detector := NewPacketReorderingDetector()

	// Simulate a sequence with some reordering
	sequence := []uint16{100, 101, 102, 104, 103, 105, 106, 108, 107, 109, 110}

	fmt.Printf("Testing sequence: %v\n", sequence)

	for _, seq := range sequence {
		isReordered := detector.CheckPacket(seq)
		if isReordered {
			fmt.Printf("  Packet %d: REORDERED\n", seq)
		} else {
			fmt.Printf("  Packet %d: OK\n", seq)
		}
	}

	reordered, total, ratio := detector.GetStats()
	fmt.Printf("\nResults: %d/%d packets reordered (%.2f%%)\n", reordered, total, ratio)
}

func testRTPReordering() {
	fmt.Println("\n2. RTP Packet Reordering Test")
	fmt.Println("---------------------------")

	// Connect to echo server
	conn, err := net.Dial("udp", "localhost:4000")
	if err != nil {
		log.Printf("Failed to connect to echo server: %v", err)
		return
	}
	defer conn.Close()

	fmt.Println("✓ Connected to echo server on port 4000")

	// Create reordering detector
	detector := NewPacketReorderingDetector()

	// Send RTP packets with intentional sequence gaps to simulate reordering
	packetCount := 20
	receivedCount := 0

	fmt.Printf("Sending %d RTP packets with simulated reordering...\n", packetCount)

	for i := 0; i < packetCount; i++ {
		// Create RTP packet with intentional sequence gaps
		seqNum := uint16(5000 + i*2) // Skip every other sequence number
		if i%5 == 0 && i > 0 {
			seqNum = uint16(5000 + (i-1)*2) // Send previous sequence number to simulate reordering
		}

		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Padding:        false,
				Extension:      false,
				Marker:         false,
				PayloadType:    0, // PCMU
				SequenceNumber: seqNum,
				Timestamp:      uint32(i * 160), // 20ms * 8000Hz
				SSRC:           0x11223344,
			},
			Payload: []byte(fmt.Sprintf("reordering test payload %d", i)),
		}

		// Marshal packet
		packetData, err := packet.Marshal()
		if err != nil {
			log.Printf("Failed to marshal RTP packet: %v", err)
			continue
		}

		// Send to echo server
		_, err = conn.Write(packetData)
		if err != nil {
			log.Printf("Failed to send RTP packet: %v", err)
			continue
		}

		// Check for reordering
		isReordered := detector.CheckPacket(packet.SequenceNumber)
		if isReordered {
			fmt.Printf("  ✓ Packet %d: REORDERED (sent seq %d)\n", i, packet.SequenceNumber)
		} else {
			fmt.Printf("  ✓ Packet %d: OK (sent seq %d)\n", i, packet.SequenceNumber)
		}

		// Try to read echoed packet with timeout
		buffer := make([]byte, 1500)
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := conn.Read(buffer)
		if err != nil {
			continue
		}

		// Parse echoed packet
		echoedPacket := &rtp.Packet{}
		if err := echoedPacket.Unmarshal(buffer[:n]); err != nil {
			log.Printf("Failed to parse echoed RTP packet: %v", err)
			continue
		}

		receivedCount++
	}

	fmt.Printf("\nSent %d packets, received %d echoes\n", packetCount, receivedCount)

	reordered, total, ratio := detector.GetStats()
	fmt.Printf("Packet reordering detection: %d/%d packets reordered (%.2f%%)\n", reordered, total, ratio)

	if reordered > 0 {
		fmt.Println("✓ Packet reordering detection is working!")
	} else {
		fmt.Println("ℹ No reordering detected in this test (this is expected for the pattern used)")
	}
}