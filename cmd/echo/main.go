package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pion/rtp"
)

const (
	// DefaultEchoPort is the default port for the echo server
	DefaultEchoPort = 4000
)

// Config holds echo server configuration
type Config struct {
	BindPort   int
	SampleRate uint32
}

// EchoServer implements an RTP echo server with proper pacing
type EchoServer struct {
	config     *Config
	conn       *net.UDPConn
	packetPool *sync.Pool
	metrics    *Metrics
	pacer      *PacketPacer
}

// Metrics tracks echo server performance
type Metrics struct {
	packetsEchoed  int64
	bytesEchoed    int64
	echoErrors     int64
	lastReportTime time.Time
	mu             sync.RWMutex
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

// ParseFlags parses command-line flags
func ParseFlags() *Config {
	var (
		bindPort   = flag.Int("port", DefaultEchoPort, "Echo server port")
		sampleRate = flag.Uint("sample-rate", 8000, "RTP sample rate")
	)

	flag.Parse()

	config := &Config{
		BindPort:   *bindPort,
		SampleRate: uint32(*sampleRate),
	}

	return config
}

// NewEchoServer creates a new echo server
func NewEchoServer(config *Config) *EchoServer {
	return &EchoServer{
		config: config,
		packetPool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, 1500) // MTU size
			},
		},
		metrics: &Metrics{
			lastReportTime: time.Now(),
		},
		pacer: NewPacketPacer(config.SampleRate),
	}
}

// Start begins the echo server
func (s *EchoServer) Start(ctx context.Context) error {
	// Create UDP listener
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", s.config.BindPort))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %v", err)
	}

	s.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP port %d: %v", s.config.BindPort, err)
	}
	defer s.conn.Close()

	// Set large buffer sizes for high throughput
	s.conn.SetReadBuffer(2 * 1024 * 1024)  // 2MB
	s.conn.SetWriteBuffer(2 * 1024 * 1024) // 2MB

	log.Printf("Echo server listening on port %d with 2MB buffers", s.config.BindPort)

	// Start metrics reporter
	go s.startMetricsReporter(ctx)

	buffer := make([]byte, 1500) // MTU size

	for {
		select {
		case <-ctx.Done():
			log.Println("Echo server shutting down...")
			return nil
		default:
		}

		// Set read deadline for responsive handling
		s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, clientAddr, err := s.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Continue loop
				continue
			}
			log.Printf("Error reading UDP packet: %v", err)
			continue
		}

		// Process the echo with proper pacing
		s.processEchoWithPacing(buffer[:n], clientAddr)
	}
}

// processEchoWithPacing handles echoing an RTP packet back to the client with proper timing
func (s *EchoServer) processEchoWithPacing(data []byte, clientAddr *net.UDPAddr) {
	startTime := time.Now()

	// Parse RTP packet
	packet := &rtp.Packet{}
	if err := packet.Unmarshal(data); err != nil {
		atomic.AddInt64(&s.metrics.echoErrors, 1)
		return
	}

	// Calculate delay based on RTP timestamp
	delay := s.pacer.CalculateDelay(packet.Timestamp)

	// Apply pacing delay if needed
	if delay > 0 {
		time.Sleep(delay)
	}

	// Echo the same packet back with the same SSRC, Seq, TS, PT
	// This maintains the exact packet for proper RTT measurement
	_, err := s.conn.WriteToUDP(data, clientAddr)
	if err != nil {
		atomic.AddInt64(&s.metrics.echoErrors, 1)
		return
	}

	// Update metrics
	atomic.AddInt64(&s.metrics.packetsEchoed, 1)
	atomic.AddInt64(&s.metrics.bytesEchoed, int64(len(data)))

	// Log processing time occasionally
	if atomic.LoadInt64(&s.metrics.packetsEchoed)%10000 == 0 {
		processingTime := time.Since(startTime)
		log.Printf("Echo processing time: %v", processingTime)
	}
}

// startMetricsReporter periodically reports metrics
func (s *EchoServer) startMetricsReporter(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reportMetrics()
		}
	}
}

// reportMetrics logs current metrics
func (s *EchoServer) reportMetrics() {
	packets := atomic.LoadInt64(&s.metrics.packetsEchoed)
	bytes := atomic.LoadInt64(&s.metrics.bytesEchoed)
	errors := atomic.LoadInt64(&s.metrics.echoErrors)

	log.Printf("Echo Server Metrics: packets=%d bytes=%d errors=%d", packets, bytes, errors)
}

func main() {
	config := ParseFlags()

	server := NewEchoServer(config)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		log.Fatalf("Echo server failed: %v", err)
	}

	log.Println("Echo server stopped")
}
