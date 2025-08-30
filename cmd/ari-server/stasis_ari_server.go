package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

// Config holds service configuration
type Config struct {
	BindIP      string
	ServicePort int
	ARIUser     string
	ARIPass     string
	AppName     string
	EchoHost    string
	EchoPort    string
}

// ARIServer handles ARI events and WebSocket connections
type ARIServer struct {
	config       *Config
	upgrader     websocket.Upgrader
	clients      *sync.Map // WebSocket connections
	channels     *sync.Map // Active channels
	metrics      *Metrics
	metricsMutex sync.RWMutex
}

// Metrics tracks overall system metrics
type Metrics struct {
	TotalChannels     int64
	ActiveChannels    int64
	CompletedChannels int64
	TotalRTT          time.Duration
	RTTMeasurements   int64
	PacketLossCount   int64
	LatePacketCount   int64
	TotalPackets      int64
	SequenceNumbers   map[string]uint16 // channelID -> last sequence number
	ConsecutiveMisses map[string]int    // channelID -> consecutive misses
}

// ChannelInfo tracks information about an active channel
type ChannelInfo struct {
	ChannelID         string
	StartTime         time.Time
	LastEventTime     time.Time
	RTTMeasurements   []RTTMeasurement
	PacketCount       int64
	LostPackets       int64
	LatePackets       int64
	LastSeqNumber     uint16
	ConsecutiveMisses int
}

// RTTMeasurement tracks a single RTT measurement
type RTTMeasurement struct {
	Timestamp time.Time
	RTT       time.Duration
}

// StasisStartEvent represents a StasisStart ARI event
type StasisStartEvent struct {
	Type        string   `json:"type"`
	Application string   `json:"application"`
	Args        []string `json:"args"`
	Channel     struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel"`
	Timestamp string `json:"timestamp"`
}

// StasisEndEvent represents a StasisEnd ARI event
type StasisEndEvent struct {
	Type    string `json:"type"`
	Channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel"`
	Timestamp string `json:"timestamp"`
}

// ChannelStateChangeEvent represents a ChannelStateChange ARI event
type ChannelStateChangeEvent struct {
	Type    string `json:"type"`
	Channel struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"channel"`
	Timestamp string `json:"timestamp"`
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	config := &Config{
		BindIP:      getEnv("BIND_IP", "0.0.0.0"),
		ServicePort: getEnvAsInt("SERVICE_PORT", 9091),
		ARIUser:     getEnv("ARI_USER", "ari"),
		ARIPass:     getEnv("ARI_PASS", "ari"),
		AppName:     getEnv("APP_NAME", "ari-app"),
		EchoHost:    getEnv("ECHO_HOST", "127.0.0.1"),
		EchoPort:    getEnv("ECHO_PORT", "4000"),
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// NewARIServer creates a new ARI server
func NewARIServer(config *Config) *ARIServer {
	return &ARIServer{
		config: config,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow connections from any origin
			},
		},
		clients:  &sync.Map{},
		channels: &sync.Map{},
		metrics: &Metrics{
			SequenceNumbers:   make(map[string]uint16),
			ConsecutiveMisses: make(map[string]int),
		},
	}
}

// healthHandler handles health check requests
func (s *ARIServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// metricsHandler handles metrics requests
func (s *ARIServer) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.metricsMutex.RLock()
	defer s.metricsMutex.RUnlock()

	var avgRTT time.Duration
	if s.metrics.RTTMeasurements > 0 {
		avgRTT = s.metrics.TotalRTT / time.Duration(s.metrics.RTTMeasurements)
	}

	lateRatio := 0.0
	if s.metrics.TotalPackets > 0 {
		lateRatio = float64(s.metrics.LatePacketCount) / float64(s.metrics.TotalPackets) * 100
	}

	packetLoss := 0.0
	if s.metrics.TotalPackets > 0 {
		packetLoss = float64(s.metrics.PacketLossCount) / float64(s.metrics.TotalPackets) * 100
	}

	metrics := map[string]interface{}{
		"total_channels":      s.metrics.TotalChannels,
		"active_channels":     s.metrics.ActiveChannels,
		"completed_channels":  s.metrics.CompletedChannels,
		"average_rtt":         avgRTT.String(),
		"rtt_measurements":    s.metrics.RTTMeasurements,
		"packet_loss_percent": packetLoss,
		"late_ratio_percent":  lateRatio,
		"total_packets":       s.metrics.TotalPackets,
		"lost_packets":        s.metrics.PacketLossCount,
		"late_packets":        s.metrics.LatePacketCount,
	}

	json.NewEncoder(w).Encode(metrics)
}

// slaMetricsHandler provides SLA-specific metrics
func (s *ARIServer) slaMetricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.metricsMutex.RLock()
	defer s.metricsMutex.RUnlock()

	// Collect RTT measurements from all channels
	var allRTTs []time.Duration
	s.channels.Range(func(key, value interface{}) bool {
		if channelInfo, ok := value.(*ChannelInfo); ok {
			for _, measurement := range channelInfo.RTTMeasurements {
				allRTTs = append(allRTTs, measurement.RTT)
			}
		}
		return true
	})

	// Sort RTTs for percentile calculations
	sort.Slice(allRTTs, func(i, j int) bool {
		return allRTTs[i] < allRTTs[j]
	})

	var p50, p95, p99, max time.Duration
	if len(allRTTs) > 0 {
		p50 = allRTTs[len(allRTTs)*50/100]
		p95 = allRTTs[len(allRTTs)*95/100]
		p99 = allRTTs[len(allRTTs)*99/100]
		max = allRTTs[len(allRTTs)-1]
	}

	metrics := map[string]interface{}{
		"rtt_p50":  p50.String(),
		"rtt_p95":  p95.String(),
		"rtt_p99":  p99.String(),
		"rtt_max":  max.String(),
		"channels": s.metrics.ActiveChannels,
	}

	json.NewEncoder(w).Encode(metrics)
}

// ariHandler handles ARI WebSocket connections
func (s *ARIServer) ariHandler(w http.ResponseWriter, r *http.Request) {
	// Check authentication
	username, password, ok := r.BasicAuth()
	if !ok {
		// Try to get from query parameters
		if r.URL.Query().Get("api_key") != "" {
			// Parse api_key=user:pass
			apiKey := r.URL.Query().Get("api_key")
			// Simple parsing for demo purposes
			if apiKey != fmt.Sprintf("%s:%s", s.config.ARIUser, s.config.ARIPass) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else {
		// Check basic auth credentials
		if username != s.config.ARIUser || password != s.config.ARIPass {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Upgrade to WebSocket connection
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// Store client connection
	clientID := fmt.Sprintf("%s-%d", r.RemoteAddr, time.Now().Unix())
	s.clients.Store(clientID, conn)

	log.Printf("New WebSocket connection from %s (ID: %s)", r.RemoteAddr, clientID)

	// Handle the connection in a separate goroutine
	go s.handleClient(clientID, conn)
}

// handleClient handles messages from a WebSocket client
func (s *ARIServer) handleClient(clientID string, conn *websocket.Conn) {
	defer func() {
		// Clean up connection
		s.clients.Delete(clientID)
		conn.Close()
		log.Printf("WebSocket connection closed (ID: %s)", clientID)
	}()

	// Send initial ARI protocol message
	initMessage := map[string]interface{}{
		"type":         "AriConnected",
		"applications": []string{s.config.AppName},
	}
	initBytes, _ := json.Marshal(initMessage)
	if err := conn.WriteMessage(websocket.TextMessage, initBytes); err != nil {
		log.Printf("Error writing initial message to client %s: %v", clientID, err)
		return
	}

	// Read messages from the client
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message from client %s: %v", clientID, err)
			return
		}

		log.Printf("Received message from client %s: %s", clientID, string(message))

		// Process the event
		s.processEvent(message)

		// Don't echo the message back - handle ARI protocol properly
	}
}

// processEvent processes an ARI event
func (s *ARIServer) processEvent(message []byte) {
	var genericEvent map[string]interface{}
	if err := json.Unmarshal(message, &genericEvent); err != nil {
		log.Printf("Error unmarshaling event: %v", err)
		return
	}

	eventType, ok := genericEvent["type"].(string)
	if !ok {
		log.Printf("Could not determine event type")
		return
	}

	switch eventType {
	case "StasisStart":
		var event StasisStartEvent
		if err := json.Unmarshal(message, &event); err != nil {
			log.Printf("Error unmarshaling StasisStart event: %v", err)
			return
		}
		s.handleStasisStart(event)
	case "StasisEnd":
		var event StasisEndEvent
		if err := json.Unmarshal(message, &event); err != nil {
			log.Printf("Error unmarshaling StasisEnd event: %v", err)
			return
		}
		s.handleStasisEnd(event)
	case "ChannelStateChange":
		var event ChannelStateChangeEvent
		if err := json.Unmarshal(message, &event); err != nil {
			log.Printf("Error unmarshaling ChannelStateChange event: %v", err)
			return
		}
		s.handleChannelStateChange(event)
	default:
		log.Printf("Unhandled event type: %s", eventType)
	}
}

// handleStasisStart handles StasisStart events
func (s *ARIServer) handleStasisStart(event StasisStartEvent) {
	channelID := event.Channel.ID
	log.Printf("StasisStart event for channel %s", channelID)

	// Create new channel info
	channelInfo := &ChannelInfo{
		ChannelID:         channelID,
		StartTime:         time.Now(),
		LastEventTime:     time.Now(),
		RTTMeasurements:   make([]RTTMeasurement, 0),
		PacketCount:       0,
		LostPackets:       0,
		LatePackets:       0,
		LastSeqNumber:     0,
		ConsecutiveMisses: 0,
	}

	s.channels.Store(channelID, channelInfo)

	s.metricsMutex.Lock()
	s.metrics.TotalChannels++
	s.metrics.ActiveChannels++
	s.metricsMutex.Unlock()
}

// handleStasisEnd handles StasisEnd events
func (s *ARIServer) handleStasisEnd(event StasisEndEvent) {
	channelID := event.Channel.ID
	log.Printf("StasisEnd event for channel %s", channelID)

	// Remove channel info
	if value, ok := s.channels.Load(channelID); ok {
		if channelInfo, ok := value.(*ChannelInfo); ok {
			s.metricsMutex.Lock()
			s.metrics.ActiveChannels--
			s.metrics.CompletedChannels++
			s.metrics.PacketLossCount += channelInfo.LostPackets
			s.metrics.LatePacketCount += channelInfo.LatePackets
			s.metrics.TotalPackets += channelInfo.PacketCount
			s.metricsMutex.Unlock()
		}
	}

	s.channels.Delete(channelID)
}

// handleChannelStateChange handles ChannelStateChange events
func (s *ARIServer) handleChannelStateChange(event ChannelStateChangeEvent) {
	channelID := event.Channel.ID
	log.Printf("ChannelStateChange event for channel %s, state: %s", channelID, event.Channel.State)

	// Update last event time for RTT calculation
	if value, ok := s.channels.Load(channelID); ok {
		if channelInfo, ok := value.(*ChannelInfo); ok {
			now := time.Now()
			rtt := now.Sub(channelInfo.LastEventTime)

			// Add RTT measurement
			measurement := RTTMeasurement{
				Timestamp: now,
				RTT:       rtt,
			}

			channelInfo.RTTMeasurements = append(channelInfo.RTTMeasurements, measurement)
			channelInfo.LastEventTime = now

			// Update the stored channel info
			s.channels.Store(channelID, channelInfo)

			log.Printf("Channel %s RTT: %v", channelID, rtt)
		}
	}
}

// recordRTT records an RTT measurement
func (s *ARIServer) recordRTT(rtt time.Duration) {
	s.metricsMutex.Lock()
	defer s.metricsMutex.Unlock()

	s.metrics.TotalRTT += rtt
	s.metrics.RTTMeasurements++
}

// simulatePacketProcessing simulates packet processing for SLA testing
func (s *ARIServer) simulatePacketProcessing(channelID string, seqNumber uint16) {
	s.metricsMutex.Lock()
	defer s.metricsMutex.Unlock()

	// Check for packet loss
	lastSeq, exists := s.metrics.SequenceNumbers[channelID]
	if exists {
		expectedSeq := lastSeq + 1
		if seqNumber != expectedSeq {
			// Packet loss detected
			lostPackets := int(seqNumber - expectedSeq)
			s.metrics.PacketLossCount += int64(lostPackets)

			// Update consecutive misses
			s.metrics.ConsecutiveMisses[channelID] += lostPackets
			if s.metrics.ConsecutiveMisses[channelID] > 2 {
				log.Printf("WARNING: Channel %s has %d consecutive packet misses (exceeds SLA limit of 2)",
					channelID, s.metrics.ConsecutiveMisses[channelID])
			}
		} else {
			// Reset consecutive misses
			s.metrics.ConsecutiveMisses[channelID] = 0
		}
	}

	// Update sequence number
	s.metrics.SequenceNumbers[channelID] = seqNumber

	// Check for late packets (simplified check)
	// In a real implementation, we would compare against deadlines
	now := time.Now()
	deadline := now.Add(-22 * time.Millisecond) // 22ms deadline
	if now.After(deadline) {
		s.metrics.LatePacketCount++
		lateRatio := float64(s.metrics.LatePacketCount) / float64(s.metrics.TotalPackets+1) * 100
		if lateRatio > 0.2 { // 0.2% threshold
			log.Printf("WARNING: Late packet ratio is %f%% (exceeds SLA threshold)", lateRatio)
		}
	}

	s.metrics.TotalPackets++
}

// Start begins the ARI server
func (s *ARIServer) Start(ctx context.Context) error {
	log.Printf("Starting Stasis ARI Server on %s:%d", s.config.BindIP, s.config.ServicePort)

	// Create HTTP server with routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/metrics", s.metricsHandler)
	mux.HandleFunc("/sla-metrics", s.slaMetricsHandler)
	mux.HandleFunc("/ari", s.ariHandler)        // For WebSocket connections
	mux.HandleFunc("/ari/events", s.ariHandler) // For WebSocket connections

	// Create server
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.config.BindIP, s.config.ServicePort),
		Handler: mux,
	}

	// Start HTTP server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	log.Printf("Stasis ARI Server listening on %s:%d", s.config.BindIP, s.config.ServicePort)

	// Start metrics reporter
	go s.startMetricsReporter(ctx)

	// Wait for shutdown signal
	<-ctx.Done()

	log.Println("Shutting down Stasis ARI Server...")

	return server.Shutdown(context.Background())
}

// startMetricsReporter periodically reports metrics
func (s *ARIServer) startMetricsReporter(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.metricsMutex.RLock()
			var avgRTT time.Duration
			if s.metrics.RTTMeasurements > 0 {
				avgRTT = s.metrics.TotalRTT / time.Duration(s.metrics.RTTMeasurements)
			}

			lateRatio := 0.0
			if s.metrics.TotalPackets > 0 {
				lateRatio = float64(s.metrics.LatePacketCount) / float64(s.metrics.TotalPackets) * 100
			}

			packetLoss := 0.0
			if s.metrics.TotalPackets > 0 {
				packetLoss = float64(s.metrics.PacketLossCount) / float64(s.metrics.TotalPackets) * 100
			}

			log.Printf("Metrics - Active: %d, Completed: %d, Avg RTT: %v, Packet Loss: %.4f%%, Late Ratio: %.4f%%",
				s.metrics.ActiveChannels, s.metrics.CompletedChannels, avgRTT, packetLoss, lateRatio)
			s.metricsMutex.RUnlock()
		}
	}
}

func main() {
	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create ARI server
	server := NewARIServer(config)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received shutdown signal")
		cancel()
	}()

	// Start the server
	if err := server.Start(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Stasis ARI Server stopped")
}
