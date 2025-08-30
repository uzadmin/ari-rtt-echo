package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
}

// ARIServer handles ARI events and WebSocket connections
type ARIServer struct {
	config      *Config
	upgrader    websocket.Upgrader
	clients     *sync.Map // WebSocket connections
	channels    *sync.Map // Channel metrics
	metricsLock sync.RWMutex
}

// ChannelMetrics tracks metrics for a channel
type ChannelMetrics struct {
	ChannelID       string
	StartTime       time.Time
	LastEventTime   time.Time
	RTTMeasurements []RTTMeasurement
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
}

// StasisEndEvent represents a StasisEnd ARI event
type StasisEndEvent struct {
	Type    string `json:"type"`
	Channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel"`
}

// ARIEvent represents a generic ARI event
type ARIEvent struct {
	Type        string                 `json:"type"`
	Application string                 `json:"application"`
	Args        []string               `json:"args"`
	Channel     map[string]interface{} `json:"channel"`
	Timestamp   string                 `json:"timestamp"`
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

	// Collect metrics from all channels
	var allMetrics []ChannelMetrics
	s.channels.Range(func(key, value interface{}) bool {
		if metrics, ok := value.(*ChannelMetrics); ok {
			allMetrics = append(allMetrics, *metrics)
		}
		return true
	})

	json.NewEncoder(w).Encode(allMetrics)
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

	// Send a test StasisStart event to verify the connection
	s.sendTestEvent(clientID, conn)

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

		// Echo the message back (for testing purposes)
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Printf("Error writing message to client %s: %v", clientID, err)
			return
		}
	}
}

// processEvent processes an ARI event and updates metrics
func (s *ARIServer) processEvent(message []byte) {
	var event ARIEvent
	if err := json.Unmarshal(message, &event); err != nil {
		log.Printf("Error unmarshaling event: %v", err)
		return
	}

	switch event.Type {
	case "StasisStart":
		s.handleStasisStart(event)
	case "StasisEnd":
		s.handleStasisEnd(event)
	case "ChannelStateChange":
		s.handleChannelStateChange(event)
	default:
		log.Printf("Unhandled event type: %s", event.Type)
	}
}

// handleStasisStart handles StasisStart events
func (s *ARIServer) handleStasisStart(event ARIEvent) {
	channelID, ok := event.Channel["id"].(string)
	if !ok {
		log.Printf("Could not extract channel ID from StasisStart event")
		return
	}

	log.Printf("StasisStart event for channel %s", channelID)

	// Create new metrics for this channel
	metrics := &ChannelMetrics{
		ChannelID:       channelID,
		StartTime:       time.Now(),
		LastEventTime:   time.Now(),
		RTTMeasurements: make([]RTTMeasurement, 0),
	}

	s.channels.Store(channelID, metrics)
}

// handleStasisEnd handles StasisEnd events
func (s *ARIServer) handleStasisEnd(event ARIEvent) {
	channelID, ok := event.Channel["id"].(string)
	if !ok {
		log.Printf("Could not extract channel ID from StasisEnd event")
		return
	}

	log.Printf("StasisEnd event for channel %s", channelID)

	// Remove metrics for this channel
	s.channels.Delete(channelID)
}

// handleChannelStateChange handles ChannelStateChange events
func (s *ARIServer) handleChannelStateChange(event ARIEvent) {
	channelID, ok := event.Channel["id"].(string)
	if !ok {
		log.Printf("Could not extract channel ID from ChannelStateChange event")
		return
	}

	// Update last event time for RTT calculation
	if value, ok := s.channels.Load(channelID); ok {
		if metrics, ok := value.(*ChannelMetrics); ok {
			now := time.Now()
			rtt := now.Sub(metrics.LastEventTime)

			// Add RTT measurement
			measurement := RTTMeasurement{
				Timestamp: now,
				RTT:       rtt,
			}

			metrics.RTTMeasurements = append(metrics.RTTMeasurements, measurement)
			metrics.LastEventTime = now

			// Update the stored metrics
			s.channels.Store(channelID, metrics)

			log.Printf("Channel %s RTT: %v", channelID, rtt)
		}
	}
}

// sendTestEvent sends a test StasisStart event to a client
func (s *ARIServer) sendTestEvent(clientID string, conn *websocket.Conn) {
	// Create a test StasisStart event
	event := StasisStartEvent{
		Type:        "StasisStart",
		Application: s.config.AppName,
		Args:        []string{},
	}
	event.Channel.ID = "test-channel-123"
	event.Channel.Name = "Local/echo@ari-context"

	// Convert to JSON
	eventBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling test event: %v", err)
		return
	}

	// Send the event
	if err := conn.WriteMessage(websocket.TextMessage, eventBytes); err != nil {
		log.Printf("Error sending test event to client %s: %v", clientID, err)
		return
	}

	log.Printf("Sent test StasisStart event to client %s", clientID)
}

// getAverageRTT calculates the average RTT for all channels
func (s *ARIServer) getAverageRTT() (time.Duration, int) {
	var totalRTT time.Duration
	var count int

	s.channels.Range(func(key, value interface{}) bool {
		if metrics, ok := value.(*ChannelMetrics); ok {
			for _, measurement := range metrics.RTTMeasurements {
				totalRTT += measurement.RTT
				count++
			}
		}
		return true
	})

	if count == 0 {
		return 0, 0
	}

	return totalRTT / time.Duration(count), count
}

// Start begins the ARI server
func (s *ARIServer) Start(ctx context.Context) error {
	log.Printf("Starting Enhanced ARI Server on %s:%d", s.config.BindIP, s.config.ServicePort)

	// Create HTTP server with routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/metrics", s.metricsHandler)
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

	log.Printf("Enhanced ARI Server listening on %s:%d", s.config.BindIP, s.config.ServicePort)

	// Start metrics reporter
	go s.startMetricsReporter(ctx)

	// Wait for shutdown signal
	<-ctx.Done()

	log.Println("Shutting down Enhanced ARI Server...")

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
			avgRTT, count := s.getAverageRTT()
			if count > 0 {
				log.Printf("Average RTT across %d measurements: %v", count, avgRTT)
			}
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

	log.Println("Enhanced ARI Server stopped")
}
