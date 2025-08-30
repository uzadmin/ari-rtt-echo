package rtp

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"ari-service/internal/metrics"

	"github.com/pion/rtp"
)

// Packet represents an RTP packet with metadata
type Packet struct {
	Data           []byte
	ClientAddr     *net.UDPAddr
	IsFromAsterisk bool
	ReceiveTime    time.Time
}

// Worker handles RTP packet processing for a single channel
type Worker struct {
	channelID  string
	bindIP     string
	rtpPort    int
	echoHost   string
	echoPort   int
	metrics    *metrics.Metrics
	t0         time.Time // Time when channel was created (call creation time)
	asteriskIP string    // IP address of Asterisk server

	// Network connections
	conn         *net.UDPConn
	echoAddr     *net.UDPAddr
	asteriskAddr *net.UDPAddr

	// Packet processing
	packetChan      chan *Packet
	latencyTracker  *LatencyTracker
	sequenceTracker *SequenceTracker
	pacer           *PacketPacer

	// Late packet detection
	baseTimestamp uint32
	baseTime      time.Time
	mu            sync.Mutex

	// Control
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// PacketPacer implements RTP packet pacing based on timestamps
type PacketPacer struct {
	baseTimestamp uint32
	baseTime      time.Time
	sampleRate    uint32
	mu            sync.Mutex
}

// NewPacketPacer creates a new packet pacer
func NewPacketPacer(sampleRate uint32) *PacketPacer {
	return &PacketPacer{
		sampleRate: sampleRate,
	}
}

// CalculateDelay calculates the delay needed before sending a packet based on its timestamp
func (p *PacketPacer) CalculateDelay(timestamp uint32) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.baseTimestamp == 0 {
		// First packet - set base time
		p.baseTimestamp = timestamp
		p.baseTime = time.Now()
		return 0
	}

	// Calculate expected time based on timestamp difference
	// For 8kHz sample rate: each timestamp unit = 1/8000 seconds = 125 microseconds
	timestampDiff := timestamp - p.baseTimestamp
	expectedTime := p.baseTime.Add(time.Duration(timestampDiff) * time.Second / time.Duration(p.sampleRate))

	// Calculate delay needed
	now := time.Now()
	if expectedTime.After(now) {
		return expectedTime.Sub(now)
	}

	return 0
}

// NewWorker creates a new RTP worker for a channel
func NewWorker(channelID, bindIP string, rtpPort int, echoHost string, echoPort int, metrics *metrics.Metrics, t0 time.Time, asteriskIP string) *Worker {
	log.Printf("%s [%s] INFO: Creating new RTP worker on port %d", time.Now().Format("2006-01-02 15:04:05"), channelID, rtpPort)
	return &Worker{
		channelID:       channelID,
		bindIP:          bindIP,
		rtpPort:         rtpPort,
		echoHost:        echoHost,
		echoPort:        echoPort,
		metrics:         metrics,
		t0:              t0,                       // Store the call creation time as T0
		asteriskIP:      asteriskIP,               // Store Asterisk IP
		packetChan:      make(chan *Packet, 1000), // Buffered channel for packet processing
		latencyTracker:  NewLatencyTracker(),
		sequenceTracker: NewSequenceTracker(),
		pacer:           NewPacketPacer(8000), // 8kHz sample rate
		stopChan:        make(chan struct{}),
	}
}

// Start begins processing RTP packets using a concurrent model
func (w *Worker) Start() {
	// Resolve echo server address
	var err error
	w.echoAddr, err = net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", w.echoHost, w.echoPort))
	if err != nil {
		log.Printf("Failed to resolve echo server address: %v", err)
		return
	}

	// Create UDP listener
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", w.bindIP, w.rtpPort))
	if err != nil {
		log.Printf("Failed to resolve UDP address: %v", err)
		return
	}

	w.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("Failed to listen on UDP port %d: %v", w.rtpPort, err)
		return
	}

	// Set large buffer sizes for high throughput (2MB as recommended)
	w.conn.SetReadBuffer(2 * 1024 * 1024)  // 2MB
	w.conn.SetWriteBuffer(2 * 1024 * 1024) // 2MB

	// Socket buffer sizes are already set above with SetReadBuffer/SetWriteBuffer
	// No additional socket options needed

	log.Printf("%s [RTP Worker] INFO: Started on port %d", time.Now().Format("2006-01-02 15:04:05"), w.rtpPort)

	// Start packet reader goroutine
	w.wg.Add(1)
	go w.packetReader()

	// Start packet processor goroutine
	w.wg.Add(1)
	go w.packetProcessor()
}

// packetReader reads packets from UDP connection and sends them to packetChan
func (w *Worker) packetReader() {
	defer w.wg.Done()
	defer w.conn.Close()

	buffer := make([]byte, 1500) // MTU size

	for {
		select {
		case <-w.stopChan:
			log.Printf("%s [%s] INFO: Packet reader stopped", time.Now().Format("2006-01-02 15:04:05"), w.channelID)
			return
		default:
		}

		// Set read deadline for responsive handling
		w.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, clientAddr, err := w.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Continue loop
				continue
			}
			log.Printf("%s [%s] ERROR: Error reading UDP packet: %v", time.Now().Format("2006-01-02 15:04:05"), w.channelID, err)
			break
		}

		// Create packet copy to avoid data races
		packetData := make([]byte, n)
		copy(packetData, buffer[:n])

		// Determine packet source
		isFromAsterisk := w.isFromAsterisk(clientAddr)

		// Send packet to processing channel
		select {
		case w.packetChan <- &Packet{
			Data:           packetData,
			ClientAddr:     clientAddr,
			IsFromAsterisk: isFromAsterisk,
			ReceiveTime:    time.Now(),
		}:
		case <-w.stopChan:
			return
		default:
			// Channel is full, drop packet
			log.Printf("%s [%s] WARN: Packet channel full, dropping packet", time.Now().Format("2006-01-02 15:04:05"), w.channelID)
		}
	}
}

// packetProcessor processes packets from packetChan
func (w *Worker) packetProcessor() {
	defer w.wg.Done()

	for {
		select {
		case packet := <-w.packetChan:
			if packet.IsFromAsterisk {
				// Packet from Asterisk -> send to echo server (with pacing)
				w.handleOutgoingPacket(packet)
			} else {
				// Packet from echo server -> send back to Asterisk (calculate latency)
				w.handleIncomingPacket(packet)
			}
		case <-w.stopChan:
			log.Printf("%s [%s] INFO: Packet processor stopped", time.Now().Format("2006-01-02 15:04:05"), w.channelID)
			return
		}
	}
}

// isFromAsterisk determines if a packet is from Asterisk
func (w *Worker) isFromAsterisk(addr *net.UDPAddr) bool {
	// Use the known Asterisk IP to identify packets from Asterisk
	// This is more reliable than trying to identify it from the first packet
	return addr.IP.String() == w.asteriskIP
}

// handleOutgoingPacket processes packets from Asterisk to be sent to echo server with pacing
func (w *Worker) handleOutgoingPacket(packet *Packet) {
	// Parse RTP header
	rtpPacket := &rtp.Packet{}
	if err := rtpPacket.Unmarshal(packet.Data); err != nil {
		log.Printf("%s [%s] ERROR: Failed to parse RTP packet: %v", time.Now().Format("2006-01-02 15:04:05"), w.channelID, err)
		return
	}

	// Log RTP packet details for debugging (only every 100th packet to avoid log spam)
	if rtpPacket.SequenceNumber%100 == 0 {
		log.Printf("%s [%s] RTP OUT: Seq=%d, TS=%d, SSRC=%d, Payload=%d, Len=%d",
			time.Now().Format("2006-01-02 15:04:05"),
			w.channelID,
			rtpPacket.SequenceNumber,
			rtpPacket.Timestamp,
			rtpPacket.SSRC,
			rtpPacket.PayloadType,
			len(packet.Data))
	}

	// Initialize baseTimestamp and baseTime for late packet detection on first packet
	w.mu.Lock()
	if w.baseTimestamp == 0 {
		w.baseTimestamp = rtpPacket.Timestamp
		w.baseTime = time.Now()
	}
	w.mu.Unlock()

	// Record send time for latency tracking
	w.latencyTracker.RecordSent(rtpPacket.SequenceNumber, time.Now())

	// Track sequence numbers for drop detection
	w.sequenceTracker.TrackOutgoing(rtpPacket.SequenceNumber)

	// Record outgoing packet for metrics
	w.metrics.RecordOutgoingPacket(w.channelID)

	// Calculate delay based on RTP timestamp (pacing)
	delay := w.pacer.CalculateDelay(rtpPacket.Timestamp)

	// Apply pacing delay if needed
	if delay > 0 {
		time.Sleep(delay)
	}

	// Forward packet to echo server
	_, err := w.conn.WriteToUDP(packet.Data, w.echoAddr)
	if err != nil {
		log.Printf("%s [%s] ERROR: Failed to send packet to echo server: %v", time.Now().Format("2006-01-02 15:04:05"), w.channelID, err)
	}
}

// handleIncomingPacket processes packets from echo server to be sent back to Asterisk
func (w *Worker) handleIncomingPacket(packet *Packet) {
	// Parse RTP header
	rtpPacket := &rtp.Packet{}
	if err := rtpPacket.Unmarshal(packet.Data); err != nil {
		log.Printf("%s [%s] ERROR: Failed to parse RTP packet: %v", time.Now().Format("2006-01-02 15:04:05"), w.channelID, err)
		return
	}

	// Log RTP packet details for debugging (only every 100th packet to avoid log spam)
	if rtpPacket.SequenceNumber%100 == 0 {
		log.Printf("%s [%s] RTP IN: Seq=%d, TS=%d, SSRC=%d, Payload=%d, Len=%d",
			time.Now().Format("2006-01-02 15:04:05"),
			w.channelID,
			rtpPacket.SequenceNumber,
			rtpPacket.Timestamp,
			rtpPacket.SSRC,
			rtpPacket.PayloadType,
			len(packet.Data))
	}

	// Calculate latency using sequence number correlation
	if latency, found := w.latencyTracker.GetLatency(rtpPacket.SequenceNumber); found {
		// Record the latency in metrics
		w.metrics.RecordLatency(w.channelID, float64(latency.Milliseconds()))

		// Track sequence numbers for drop detection
		dropped := w.sequenceTracker.TrackIncoming(rtpPacket.SequenceNumber)
		if dropped > 0 {
			w.metrics.RecordDroppedPackets(w.channelID, int64(dropped))
		}

		// Check for late packets based on RTP timestamp
		if isLate := w.checkLatePacket(rtpPacket, time.Now()); isLate {
			w.metrics.RecordLatePacket(w.channelID)
		}
	} else {
		// Packet not found in tracker - could be duplicate, late, or lost packet
		log.Printf("%s [%s] WARN: Packet with sequence number %d not found in tracker", time.Now().Format("2006-01-02 15:04:05"), w.channelID, rtpPacket.SequenceNumber)
	}

	// Send packet back to Asterisk
	w.mu.Lock()
	asteriskAddr := w.asteriskAddr
	w.mu.Unlock()

	if asteriskAddr != nil {
		_, err := w.conn.WriteToUDP(packet.Data, asteriskAddr)
		if err != nil {
			log.Printf("%s [%s] ERROR: Failed to send packet to Asterisk: %v", time.Now().Format("2006-01-02 15:04:05"), w.channelID, err)
		}
	}
}

// checkLatePacket determines if a packet is late based on RTP timestamp
// Implements the exact formula: t_expected(ts)=t0+(ts−ts0)/8000; late если t_actual>t_expected+3мс
func (w *Worker) checkLatePacket(packet *rtp.Packet, receiveTime time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.baseTimestamp == 0 {
		w.baseTimestamp = packet.Timestamp
		w.baseTime = time.Now()
		return false
	}

	// Calculate expected time using the exact formula:
	// t_expected(ts) = t0 + (ts - ts0) / 8000
	// where ts0 is baseTimestamp and t0 is baseTime
	timestampDiff := int64(packet.Timestamp - w.baseTimestamp)
	expectedTime := w.baseTime.Add(time.Duration(timestampDiff) * time.Second / 8000)

	// Late if actual time is more than expected time + 3ms
	// Using 3ms as the exact threshold as specified in requirements
	deadline := expectedTime.Add(3 * time.Millisecond)
	return receiveTime.After(deadline)
}

// Stop stops the RTP worker gracefully
func (w *Worker) Stop() {
	log.Printf("%s [%s] INFO: Stopping RTP worker", time.Now().Format("2006-01-02 15:04:05"), w.channelID)
	close(w.stopChan)
	w.wg.Wait() // Wait for all goroutines to finish
	log.Printf("%s [%s] INFO: RTP worker stopped", time.Now().Format("2006-01-02 15:04:05"), w.channelID)
}

// setSocketOptions is deprecated - using SetReadBuffer/SetWriteBuffer instead
