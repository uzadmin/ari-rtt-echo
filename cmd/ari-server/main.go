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

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	config := &Config{
		BindIP:      getEnv("BIND_IP", "0.0.0.0"),
		ServicePort: getEnvAsInt("SERVICE_PORT", 9090),
		ARIUser:     getEnv("ARI_USER", "ari_user"),
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
		if intValue, err := fmt.Sscanf(value, "%d", &defaultValue); err == nil && intValue == 1 {
			return defaultValue
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
		clients: &sync.Map{},
	}
}

// healthHandler handles health check requests
func (s *ARIServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
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

		// Echo the message back (for testing purposes)
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Printf("Error writing message to client %s: %v", clientID, err)
			return
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
	}
}

// Start begins the ARI server
func (s *ARIServer) Start(ctx context.Context) error {
	log.Printf("Starting ARI Server on %s:%d", s.config.BindIP, s.config.ServicePort)

	// Create HTTP server with routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthHandler)
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

	log.Printf("ARI Server listening on %s:%d", s.config.BindIP, s.config.ServicePort)

	// Wait for shutdown signal
	<-ctx.Done()

	log.Println("Shutting down ARI Server...")

	return server.Shutdown(context.Background())
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

	log.Println("ARI Server stopped")
}