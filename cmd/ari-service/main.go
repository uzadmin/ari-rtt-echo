package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"ari-service/internal/metrics"
	"ari-service/internal/rtp"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

// Config holds service configuration
type Config struct {
	ARIURL             string
	ARIUser            string
	ARIPass            string
	AsteriskUser       string // For REST API calls to Asterisk
	AsteriskPass       string // For REST API calls to Asterisk
	AppName            string
	BindIP             string
	PortRange          string
	EchoHost           string
	EchoPort           string
	MetricsIntervalSec int
	ServicePort        int // Add service port for WebSocket server
}

// ARIService handles ARI events and RTP processing
type ARIService struct {
	config      *Config
	client      *ARIClient
	portManager *PortManager
	channels    *sync.Map // channelID -> *ChannelHandler
	metrics     *metrics.Metrics
	conn        *websocket.Conn
	server      *http.Server
}

// ChannelHandler manages a single channel's resources
type ChannelHandler struct {
	channelID       string
	externalMediaID string
	bridgeID        string
	rtpPort         int
	worker          *rtp.Worker
	startTime       time.Time
	cleanup         func()
}

// StasisStartEvent represents a StasisStart ARI event
type StasisStartEvent struct {
	Type    string `json:"type"`
	Channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel"`
	Application string   `json:"application"`
	Args        []string `json:"args"`
}

// StasisEndEvent represents a StasisEnd ARI event
type StasisEndEvent struct {
	Type    string `json:"type"`
	Channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel"`
}

// WebSocket upgrader for handling WebSocket connections
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin
	},
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	config := &Config{
		ARIURL:             getEnv("ARI_URL", "localhost:8088"),
		ARIUser:            getEnv("ARI_USER", "asterisk"),     // For WebSocket auth (Asterisk connects to us)
		ARIPass:            getEnv("ARI_PASS", "asterisk"),     // For WebSocket auth (Asterisk connects to us)
		AsteriskUser:       getEnv("ASTERISK_USERNAME", "ari"), // For REST API calls (We connect to Asterisk)
		AsteriskPass:       getEnv("ASTERISK_PASSWORD", "ari"), // For REST API calls (We connect to Asterisk)
		AppName:            getEnv("APP_NAME", "ari-app"),
		BindIP:             getEnv("BIND_IP", "0.0.0.0"),
		PortRange:          getEnv("PORT_RANGE", "4500-50000"),
		EchoHost:           getEnv("ECHO_HOST", "127.0.0.1"),
		EchoPort:           getEnv("ECHO_PORT", "4000"),
		MetricsIntervalSec: getEnvAsInt("METRICS_INTERVAL_SEC", 5),
		ServicePort:        getEnvAsInt("SERVICE_PORT", 9090), // Add service port
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
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// NewARIService creates a new ARI service
func NewARIService(config *Config) *ARIService {
	// Parse port range
	ports := strings.Split(config.PortRange, "-")
	if len(ports) != 2 {
		log.Fatal("Invalid PORT_RANGE format, expected MIN-MAX")
	}
	minPort, err := strconv.Atoi(ports[0])
	if err != nil {
		log.Fatal("Invalid PORT_RANGE min value")
	}
	maxPort, err := strconv.Atoi(ports[1])
	if err != nil {
		log.Fatal("Invalid PORT_RANGE max value")
	}

	return &ARIService{
		config:      config,
		client:      NewARIClient(config.ARIURL, config.AsteriskUser, config.AsteriskPass), // Use correct credentials for REST API
		portManager: NewPortManager(minPort, maxPort),
		channels:    &sync.Map{},
		metrics:     metrics.NewMetrics(),
	}
}

// WebSocket handler for Asterisk outbound connections
func (s *ARIService) ariWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("Asterisk connected via WebSocket from %s", r.RemoteAddr)

	// Store connection
	s.conn = conn

	// Create context for this connection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start processing WebSocket messages
	go s.processWebSocketMessages(ctx)

	// Keep connection alive
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Read messages to keep connection alive
			_, _, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				cancel()
				return
			}
		}
	}
}

// Start begins the ARI service
func (s *ARIService) Start(ctx context.Context) error {
	log.Printf("Starting ARI Service on %s:%d", s.config.BindIP, s.config.ServicePort)

	// Register the ARI application
	if err := s.registerApplication(); err != nil {
		log.Printf("Warning: Failed to register ARI application: %v", err)
	}

	// Try to establish WebSocket connection to Asterisk
	// If that fails, fall back to HTTP polling
	if err := s.connectToAsterisk(ctx); err != nil {
		log.Printf("Failed to establish WebSocket connection to Asterisk: %v", err)
		log.Println("Falling back to HTTP polling mode")
		// Start event polling
		go s.startEventPolling(ctx)
	} else {
		log.Println("WebSocket connection established successfully")
	}

	// Start metrics reporter
	go s.startMetricsReporter(ctx)

	// Create HTTP server with routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/metrics", s.metricsHandler)
	// Remove the WebSocket handler since we're using HTTP polling

	// Create server
	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.config.BindIP, s.config.ServicePort),
		Handler: mux,
	}

	// Start HTTP server in a goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start zombie cleanup goroutine
	go s.startZombieCleanup(ctx)

	// Wait for shutdown signal
	<-ctx.Done()

	log.Println("Shutting down ARI Service...")

	// Print final SLA report
	s.printSLAReport()

	return s.server.Shutdown(context.Background())
}

// registerApplication registers the ARI application with Asterisk
func (s *ARIService) registerApplication() error {
	// Check if the application already exists
	_, err := s.client.GetApplication(s.config.AppName)
	if err != nil {
		// If the application doesn't exist, that's expected
		log.Printf("Application %s not found, will be registered when channels connect", s.config.AppName)
		return nil
	}

	log.Printf("Application %s already registered", s.config.AppName)
	return nil
}

// startEventPolling polls for ARI events using HTTP
func (s *ARIService) startEventPolling(ctx context.Context) {
	log.Println("Starting event polling")

	ticker := time.NewTicker(100 * time.Millisecond) // Poll every 100ms
	defer ticker.Stop()

	// Keep track of known channels to detect new and ended channels
	knownChannels := make(map[string]bool)

	for {
		select {
		case <-ctx.Done():
			log.Println("Event polling stopped")
			return
		case <-ticker.C:
			// Poll for channels
			if err := s.pollChannels(knownChannels); err != nil {
				log.Printf("Error polling channels: %v", err)
			}
		}
	}
}

// pollChannels polls for active channels and detects new and ended channels
func (s *ARIService) pollChannels(knownChannels map[string]bool) error {
	log.Println("Polling channels...")
	channels, err := s.client.GetChannels()
	if err != nil {
		return fmt.Errorf("failed to get channels: %v", err)
	}

	log.Printf("Polled %d channels", len(channels))

	// Log the channels for debugging
	for _, channel := range channels {
		log.Printf("Channel: ID=%s, Name=%s, State=%s", channel.ID, channel.Name, channel.State)
	}

	// Convert to map for easier lookup
	currentChannels := make(map[string]bool)
	for _, channel := range channels {
		currentChannels[channel.ID] = true
	}

	// Detect new channels (StasisStart events)
	for channelID := range currentChannels {
		if !knownChannels[channelID] {
			log.Printf("Detected new channel: %s", channelID)
			// This is a new channel, simulate StasisStart event
			event := map[string]interface{}{
				"type": "StasisStart",
				"channel": map[string]interface{}{
					"id":   channelID,
					"name": "Local/echo@ari-context", // Default name for testing
				},
				"application": s.config.AppName,
				"args":        []string{},
			}
			// Convert event to JSON bytes to match handleWebSocketMessage signature
			eventBytes, err := json.Marshal(event)
			if err != nil {
				log.Printf("Failed to marshal StasisStart event: %v", err)
				continue
			}
			s.handleWebSocketMessage(eventBytes)
		}
	}

	// Detect ended channels (StasisEnd events)
	for channelID := range knownChannels {
		if !currentChannels[channelID] {
			log.Printf("Detected ended channel: %s", channelID)
			// This channel has ended, simulate StasisEnd event
			event := map[string]interface{}{
				"type": "StasisEnd",
				"channel": map[string]interface{}{
					"id":   channelID,
					"name": "Local/echo@ari-context", // Default name for testing
				},
			}
			// Convert event to JSON bytes to match handleWebSocketMessage signature
			eventBytes, err := json.Marshal(event)
			if err != nil {
				log.Printf("Failed to marshal StasisEnd event: %v", err)
				continue
			}
			s.handleWebSocketMessage(eventBytes)
		}
	}

	// Update known channels
	for k, v := range currentChannels {
		knownChannels[k] = v
	}

	// Remove ended channels from known channels
	for channelID := range knownChannels {
		if !currentChannels[channelID] {
			delete(knownChannels, channelID)
		}
	}

	return nil
}

// connectToAsterisk establishes a WebSocket connection to Asterisk
func (s *ARIService) connectToAsterisk(ctx context.Context) error {
	// Construct WebSocket URL for Asterisk ARI
	wsURL := fmt.Sprintf("ws://%s/ari/events?api_key=%s:%s&app=%s",
		s.config.ARIURL, s.config.ARIUser, s.config.ARIPass, s.config.AppName)

	log.Printf("Connecting to Asterisk ARI at %s", wsURL)

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Asterisk: %v", err)
	}

	// Store connection
	s.conn = conn

	// Start processing WebSocket messages
	go s.processWebSocketMessages(ctx)

	log.Println("Connected to Asterisk ARI")
	return nil
}

func (s *ARIService) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func (s *ARIService) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stats := s.metrics.GetGlobalStats()
	json.NewEncoder(w).Encode(stats)
}

func (s *ARIService) startMetricsReporter(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(s.config.MetricsIntervalSec) * time.Second)
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

func (s *ARIService) reportMetrics() {
	stats := s.metrics.GetGlobalStats()
	log.Printf("\rSTATUS: Channels=%d Latency=%.1fms Loss=%.2f%% Late=%.2f%%",
		stats.ActiveChannels, stats.AvgLatency, stats.PacketLossRatio*100, stats.LateRatio*100)
}

func (s *ARIService) processWebSocketMessages(ctx context.Context) {
	if s.conn == nil {
		log.Println("No WebSocket connection available")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, message, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			return
		}

		s.handleWebSocketMessage(message)
	}
}

func (s *ARIService) handleWebSocketMessage(message []byte) {
	var event map[string]interface{}
	if err := json.Unmarshal(message, &event); err != nil {
		log.Printf("Error unmarshaling event: %v", err)
		return
	}

	eventType, ok := event["type"].(string)
	if !ok {
		return
	}

	switch eventType {
	case "StasisStart":
		s.handleStasisStart(event)
	case "StasisEnd":
		s.handleStasisEnd(event)
	}
}

func (s *ARIService) handleStasisStart(event map[string]interface{}) {
	// Parse the StasisStart event
	channelData, ok := event["channel"].(map[string]interface{})
	if !ok {
		log.Println("Invalid channel data in StasisStart event")
		return
	}

	channelID, ok := channelData["id"].(string)
	if !ok {
		log.Println("Missing channel id in StasisStart event")
		return
	}

	log.Printf("StasisStart event received for channel %s", channelID)

	// Answer the channel with retry logic
	if err := s.answerChannelWithRetry(channelID, 3, 100*time.Millisecond); err != nil {
		log.Printf("Failed to answer channel %s after retries: %v", channelID, err)
		return
	}

	// Create channel with proper VoIP logic
	if err := s.createChannel(channelID); err != nil {
		log.Printf("Failed to create channel %s: %v", channelID, err)
		return
	}
}

// answerChannelWithRetry attempts to answer a channel with retry logic
func (s *ARIService) answerChannelWithRetry(channelID string, maxRetries int, retryDelay time.Duration) error {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		err := s.client.AnswerChannel(channelID)
		if err == nil {
			// Success
			log.Printf("Successfully answered channel %s", channelID)
			return nil
		}

		// Check if it's a "Channel not found" error
		if strings.Contains(err.Error(), "Channel not found") {
			log.Printf("Channel %s not found, waiting before retry %d/%d", channelID, i+1, maxRetries)
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}

		// For other errors, don't retry
		return fmt.Errorf("failed to answer channel %s: %v", channelID, err)
	}

	return fmt.Errorf("failed to answer channel %s after %d retries: %v", channelID, maxRetries, lastErr)
}

func (s *ARIService) handleStasisEnd(event map[string]interface{}) {
	channelData, ok := event["channel"].(map[string]interface{})
	if !ok {
		log.Println("Invalid channel data in StasisEnd event")
		return
	}

	channelID, ok := channelData["id"].(string)
	if !ok {
		log.Println("Missing channel id in StasisEnd event")
		return
	}

	log.Printf("StasisEnd event received for channel %s", channelID)

	// Clean up channel resources
	s.cleanupChannel(channelID)
}

func (s *ARIService) createChannel(channelID string) error {
	// Get RTP port
	rtpPort, err := s.portManager.GetPort()
	if err != nil {
		return fmt.Errorf("failed to get RTP port: %v", err)
	}

	// Get echo server port
	echoPort, err := strconv.Atoi(s.config.EchoPort)
	if err != nil {
		s.portManager.ReleasePort(rtpPort)
		return fmt.Errorf("invalid echo port: %v", err)
	}

	// Create bridge
	bridgeReq := map[string]interface{}{
		"type": "mixing",
	}
	bridge, err := s.client.CreateBridge(bridgeReq)
	if err != nil {
		s.portManager.ReleasePort(rtpPort)
		return fmt.Errorf("failed to create bridge: %v", err)
	}

	// Create external media with proper parameters
	// direction=both, format=ulaw, transport=udp, encapsulation=rtp
	externalMediaReq := map[string]interface{}{
		"app":           s.config.AppName,
		"format":        "ulaw",
		"direction":     "both",
		"encapsulation": "rtp",
		"external_host": fmt.Sprintf("%s:%d", s.config.BindIP, rtpPort),
		"channelId":     fmt.Sprintf("external-media-%s", channelID),
	}

	externalMediaID, err := s.client.CreateExternalMedia(externalMediaReq)
	if err != nil {
		s.portManager.ReleasePort(rtpPort)
		return fmt.Errorf("failed to create external media: %v", err)
	}

	// Add channels to bridge
	if err := s.client.AddChannelToBridge(bridge.ID, channelID); err != nil {
		s.portManager.ReleasePort(rtpPort)
		return fmt.Errorf("failed to add channel to bridge: %v", err)
	}

	if err := s.client.AddChannelToBridge(bridge.ID, externalMediaID); err != nil {
		s.portManager.ReleasePort(rtpPort)
		return fmt.Errorf("failed to add external media to bridge: %v", err)
	}

	// Record T0 - time when we create the channel (call creation time)
	t0 := time.Now()

	// When creating externalMedia, we know the Asterisk IP should be the same as our bind IP
	// since they're on the same machine in this setup
	asteriskIP := s.config.BindIP

	// Create RTP worker
	worker := rtp.NewWorker(
		channelID,
		s.config.BindIP,
		rtpPort,
		s.config.EchoHost,
		echoPort,
		s.metrics,
		t0,         // Pass T0 time to worker
		asteriskIP, // Pass Asterisk IP to worker
	)

	// Start the worker
	go worker.Start()

	// Create channel handler
	handler := &ChannelHandler{
		channelID:       channelID,
		externalMediaID: externalMediaID,
		bridgeID:        bridge.ID,
		rtpPort:         rtpPort,
		worker:          worker,
		startTime:       time.Now(),
	}

	// Set cleanup function
	handler.cleanup = func() {
		worker.Stop()
		s.portManager.ReleasePort(rtpPort)

		// Hangup external media channel
		if externalMediaID != "" {
			s.client.HangupChannel(externalMediaID)
		}

		// Delete bridge
		if bridge.ID != "" {
			s.client.DeleteBridge(bridge.ID)
		}
	}

	// Save handler
	s.channels.Store(channelID, handler)
	s.metrics.MarkChannelStarted(channelID) // Mark as started

	log.Printf("Created channel %s with RTP port %d", channelID, rtpPort)

	return nil
}

func (s *ARIService) cleanupChannel(channelID string) {
	// Remove channel from map
	handlerInterface, found := s.channels.LoadAndDelete(channelID)
	if !found {
		return
	}

	handler, ok := handlerInterface.(*ChannelHandler)
	if !ok {
		return
	}

	// Call cleanup function
	if handler.cleanup != nil {
		handler.cleanup()
	}

	log.Printf("Cleaned up channel %s", channelID)
}

// startZombieCleanup periodically checks for zombie channels and cleans them up
func (s *ARIService) startZombieCleanup(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute) // Check every 2 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupZombieChannels()
		}
	}
}

// cleanupZombieChannels checks all channels in s.channels and removes those not active in Asterisk
func (s *ARIService) cleanupZombieChannels() {
	log.Println("Starting zombie channel cleanup")

	var zombieCount int
	var totalCount int

	s.channels.Range(func(key, value interface{}) bool {
		totalCount++
		channelID := key.(string)

		// Check if channel exists in Asterisk
		_, err := s.client.GetChannel(channelID)
		if err != nil {
			// Check if this is a "channel not found" error
			errStr := err.Error()
			if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") || strings.Contains(errStr, "Channel not found") {
				// Channel doesn't exist in Asterisk, clean it up
				log.Printf("Found zombie channel %s (404 error), cleaning up", channelID)
				s.cleanupChannel(channelID)
				zombieCount++
			} else {
				// Some other error occurred, log it but don't clean up yet
				log.Printf("Error checking channel %s: %v (not cleaning up yet)", channelID, err)
			}
		}

		return true
	})

	if zombieCount > 0 {
		log.Printf("Cleaned up %d zombie channels out of %d total tracked channels", zombieCount, totalCount)
	} else if totalCount > 0 {
		log.Printf("No zombie channels found among %d tracked channels", totalCount)
	} else {
		log.Println("No channels to check for zombie status")
	}
}

// printSLAReport prints a final SLA-compliant report
func (s *ARIService) printSLAReport() {
	stats := s.metrics.GetGlobalStats()
	log.Printf("\n=== FINAL SLA REPORT ===")
	log.Printf("p50=%.1fms p95=%.1fms p99=%.1fms max=%.1fms",
		stats.P50Latency, stats.P95Latency, stats.P99Latency, stats.MaxLatency)
	log.Printf("late_ratio=%.2f%% drops=%.2f%%",
		stats.LateRatio*100, stats.PacketLossRatio*100)
	log.Printf("Total Channels: %d Active Channels: %d",
		stats.TotalChannels, stats.ActiveChannels)

	// Log port manager status
	usedPorts := s.getUsedPortCount()
	log.Printf("Used RTP Ports: %d", usedPorts)

	log.Printf("========================\n")
}

// getUsedPortCount returns the number of currently allocated ports
func (s *ARIService) getUsedPortCount() int {
	s.portManager.mu.Lock()
	defer s.portManager.mu.Unlock()

	used := 0
	for i := 0; i < len(s.portManager.ports); i++ {
		if s.portManager.ports[i] {
			used++
		}
	}
	return used
}

func main() {
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	service := NewARIService(config)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := service.Start(ctx); err != nil {
		log.Fatalf("Service failed: %v", err)
	}
}
