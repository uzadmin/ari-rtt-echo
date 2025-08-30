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
	config   *Config
	upgrader websocket.Upgrader
	clients  *sync.Map // WebSocket connections
	rttStats *RTTStats
}

// RTTStats tracks RTT statistics
type RTTStats struct {
	mu           sync.RWMutex
	measurements []RTTMeasurement
	totalRTT     time.Duration
	count        int
}

// RTTMeasurement tracks a single RTT measurement
type RTTMeasurement struct {
	Timestamp time.Time
	RTT       time.Duration
}

// RESTResponse represents a REST response from Asterisk
type RESTResponse struct {
	Type          string `json:"type"`
	TransactionID string `json:"transaction_id"`
	RequestID     string `json:"request_id"`
	StatusCode    int    `json:"status_code"`
	ReasonPhrase  string `json:"reason_phrase"`
	URI           string `json:"uri"`
	ContentType   string `json:"content_type"`
	MessageBody   string `json:"message_body"`
	Application   string `json:"application"`
	Timestamp     string `json:"timestamp"`
	AsteriskID    string `json:"asterisk_id"`
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
		rttStats: &RTTStats{},
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

	s.rttStats.mu.RLock()
	defer s.rttStats.mu.RUnlock()

	var avgRTT time.Duration
	if s.rttStats.count > 0 {
		avgRTT = s.rttStats.totalRTT / time.Duration(s.rttStats.count)
	}

	metrics := map[string]interface{}{
		"average_rtt":       avgRTT.String(),
		"measurement_count": s.rttStats.count,
		"total_rtt":         s.rttStats.totalRTT.String(),
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

	// Track message timestamps for RTT calculation
	messageTimestamps := make(map[string]time.Time)

	// Read messages from the client
	for {
		// Record time before reading message
		readStart := time.Now()

		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message from client %s: %v", clientID, err)
			return
		}

		// Calculate RTT based on time to read message
		rtt := time.Since(readStart)
		s.recordRTT(rtt)

		log.Printf("Received message from client %s (RTT: %v): %s", clientID, rtt, string(message))

		// Try to parse as RESTResponse to get timestamp
		var response RESTResponse
		if err := json.Unmarshal(message, &response); err == nil && response.Type == "RESTResponse" {
			// Record timestamp for potential future correlation
			messageTimestamps[response.RequestID] = time.Now()
		}

		// Echo the message back (for testing purposes)
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Printf("Error writing message to client %s: %v", clientID, err)
			return
		}
	}
}

// recordRTT records an RTT measurement
func (s *ARIServer) recordRTT(rtt time.Duration) {
	s.rttStats.mu.Lock()
	defer s.rttStats.mu.Unlock()

	measurement := RTTMeasurement{
		Timestamp: time.Now(),
		RTT:       rtt,
	}

	s.rttStats.measurements = append(s.rttStats.measurements, measurement)
	s.rttStats.totalRTT += rtt
	s.rttStats.count++

	// Keep only the last 1000 measurements to prevent memory issues
	if len(s.rttStats.measurements) > 1000 {
		// Remove oldest measurements
		removed := s.rttStats.measurements[0]
		s.rttStats.measurements = s.rttStats.measurements[1:]
		s.rttStats.totalRTT -= removed.RTT
		s.rttStats.count--
	}
}

// getAverageRTT calculates the average RTT
func (s *ARIServer) getAverageRTT() time.Duration {
	s.rttStats.mu.RLock()
	defer s.rttStats.mu.RUnlock()

	if s.rttStats.count == 0 {
		return 0
	}

	return s.rttStats.totalRTT / time.Duration(s.rttStats.count)
}

// Start begins the ARI server
func (s *ARIServer) Start(ctx context.Context) error {
	log.Printf("Starting Simple RTT ARI Server on %s:%d", s.config.BindIP, s.config.ServicePort)

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

	log.Printf("Simple RTT ARI Server listening on %s:%d", s.config.BindIP, s.config.ServicePort)

	// Start metrics reporter
	go s.startMetricsReporter(ctx)

	// Wait for shutdown signal
	<-ctx.Done()

	log.Println("Shutting down Simple RTT ARI Server...")

	return server.Shutdown(context.Background())
}

// startMetricsReporter periodically reports metrics
func (s *ARIServer) startMetricsReporter(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			avgRTT := s.getAverageRTT()
			log.Printf("Current average RTT: %v", avgRTT)
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

	log.Println("Simple RTT ARI Server stopped")
}
