package metrics

import (
	"math"
	"sort"
	"sync"
	"time"
)

// Metrics provides latency and packet statistics
type Metrics struct {
	// Channel metrics
	channelMetrics sync.Map // channelID -> *ChannelMetrics

	// Global counters
	totalChannels  int64
	totalLatencies int64
	mu             sync.RWMutex // Mutex for global counters
}

// ChannelMetrics stores metrics for a single channel
type ChannelMetrics struct {
	ChannelID       string    `json:"channel_id"`
	StartTime       time.Time `json:"start_time"`
	Latencies       []float64 `json:"latencies"`
	OutgoingPackets int64     `json:"outgoing_packets"`
	DroppedPackets  int64     `json:"dropped_packets"`
	LatePackets     int64     `json:"late_packets"`

	mu sync.RWMutex
}

// GlobalStats provides aggregated statistics
type GlobalStats struct {
	TotalChannels   int       `json:"total_channels"`
	ActiveChannels  int       `json:"active_channels"`
	TotalLatencies  int64     `json:"total_latencies"`
	P50Latency      float64   `json:"p50_latency"`
	P95Latency      float64   `json:"p95_latency"`
	P99Latency      float64   `json:"p99_latency"`
	MaxLatency      float64   `json:"max_latency"`
	AvgLatency      float64   `json:"avg_latency"`
	LateRatio       float64   `json:"late_ratio"`
	PacketLossRatio float64   `json:"packet_loss_ratio"`
	Timestamp       time.Time `json:"timestamp"`
}

// NewMetrics creates a new Metrics instance
func NewMetrics() *Metrics {
	return &Metrics{}
}

// RecordLatency records a latency measurement for a channel
func (m *Metrics) RecordLatency(channelID string, latencyMs float64) {
	// Get or create channel metrics
	metricsInterface, _ := m.channelMetrics.LoadOrStore(channelID, &ChannelMetrics{
		ChannelID: channelID,
		StartTime: time.Now(),
		Latencies: make([]float64, 0, 10000), // Buffer for up to 10k latencies
	})

	metrics := metricsInterface.(*ChannelMetrics)

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	// Add latency to buffer
	metrics.Latencies = append(metrics.Latencies, latencyMs)

	// Limit buffer size
	if len(metrics.Latencies) > 10000 {
		// Create a new slice to allow old memory to be garbage collected
		metrics.Latencies = append([]float64(nil), metrics.Latencies[1000:]...)
	}

	// Update global counters
	m.mu.Lock()
	m.totalLatencies++
	m.mu.Unlock()
}

// RecordDroppedPackets records dropped packets for a channel
func (m *Metrics) RecordDroppedPackets(channelID string, count int64) {
	metricsInterface, _ := m.channelMetrics.LoadOrStore(channelID, &ChannelMetrics{
		ChannelID: channelID,
		StartTime: time.Now(),
		Latencies: make([]float64, 0, 10000),
	})

	metrics := metricsInterface.(*ChannelMetrics)
	metrics.mu.Lock()
	metrics.DroppedPackets += count
	metrics.mu.Unlock()
}

// RecordOutgoingPacket records an outgoing packet for a channel
func (m *Metrics) RecordOutgoingPacket(channelID string) {
	metricsInterface, _ := m.channelMetrics.LoadOrStore(channelID, &ChannelMetrics{
		ChannelID: channelID,
		StartTime: time.Now(),
		Latencies: make([]float64, 0, 10000),
	})

	metrics := metricsInterface.(*ChannelMetrics)
	metrics.mu.Lock()
	metrics.OutgoingPackets++
	metrics.mu.Unlock()
}

// RecordLatePacket records a late packet for a channel
func (m *Metrics) RecordLatePacket(channelID string) {
	metricsInterface, _ := m.channelMetrics.LoadOrStore(channelID, &ChannelMetrics{
		ChannelID: channelID,
		StartTime: time.Now(),
		Latencies: make([]float64, 0, 10000),
	})

	metrics := metricsInterface.(*ChannelMetrics)
	metrics.mu.Lock()
	metrics.LatePackets++
	metrics.mu.Unlock()
}

// GetGlobalStats calculates global statistics
func (m *Metrics) GetGlobalStats() *GlobalStats {
	now := time.Now()
	allLatencies := make([]float64, 0)
	activeChannels := 0
	totalDroppedPackets := int64(0)
	totalLatePackets := int64(0)
	totalPackets := int64(0)

	// Collect data from all channels
	m.channelMetrics.Range(func(key, value interface{}) bool {
		if metrics, ok := value.(*ChannelMetrics); ok {
			metrics.mu.Lock()

			activeChannels++
			allLatencies = append(allLatencies, metrics.Latencies...)

			totalDroppedPackets += metrics.DroppedPackets
			totalLatePackets += metrics.LatePackets
			totalPackets += metrics.OutgoingPackets

			metrics.mu.Unlock()
		}
		return true
	})

	m.mu.RLock()
	totalLatencies := m.totalLatencies
	totalChannels := m.totalChannels
	m.mu.RUnlock()

	stats := &GlobalStats{
		TotalChannels:  int(totalChannels),
		ActiveChannels: activeChannels,
		TotalLatencies: totalLatencies,
		Timestamp:      now,
	}

	// Calculate percentiles and statistics
	if len(allLatencies) > 0 {
		sort.Float64s(allLatencies)

		n := len(allLatencies)
		stats.P50Latency = allLatencies[int(float64(n)*0.5)]
		stats.P95Latency = allLatencies[int(math.Min(float64(n)*0.95, float64(n-1)))]
		stats.P99Latency = allLatencies[int(math.Min(float64(n)*0.99, float64(n-1)))]
		stats.MaxLatency = allLatencies[n-1]

		// Calculate average
		sum := 0.0
		for _, lat := range allLatencies {
			sum += lat
		}
		stats.AvgLatency = sum / float64(n)

		// Calculate ratios
		if totalPackets > 0 {
			stats.LateRatio = float64(totalLatePackets) / float64(totalPackets)
			stats.PacketLossRatio = float64(totalDroppedPackets) / float64(totalPackets)
		}
	}

	return stats
}

// MarkChannelStarted marks a channel as started
func (m *Metrics) MarkChannelStarted(channelID string) {
	// Increment the total channels counter when a channel starts
	m.mu.Lock()
	m.totalChannels++
	m.mu.Unlock()
}
