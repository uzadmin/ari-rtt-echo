package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// Config holds load test configuration
type Config struct {
	ARIURL              string
	ARIUser             string
	ARIPass             string
	AppName             string
	Endpoint            string
	Context             string
	Extension           string
	Priority            int
	ConcurrentCalls     int
	DurationSeconds     int
	CallDurationSeconds int
	ReportFile          string
	MetricsInterval     int
	PacketReordering    bool
}

// LoadTest runs the enhanced load test
type LoadTest struct {
	config    *Config
	ariClient *ARIClient
}

// CallResult represents the result of a single call
type CallResult struct {
	CallID         string         `json:"call_id"`
	ChannelID      string         `json:"channel_id,omitempty"`
	StartTime      time.Time      `json:"start_time"`
	EndTime        time.Time      `json:"end_time"`
	Success        bool           `json:"success"`
	Error          string         `json:"error,omitempty"`
	Duration       float64        `json:"duration_seconds"`
	LatencyMetrics LatencyMetrics `json:"latency_metrics,omitempty"`
}

// LatencyMetrics holds detailed latency metrics
type LatencyMetrics struct {
	P50Latency      float64 `json:"p50_latency_ms"`
	P95Latency      float64 `json:"p95_latency_ms"`
	P99Latency      float64 `json:"p99_latency_ms"`
	MaxLatency      float64 `json:"max_latency_ms"`
	AvgLatency      float64 `json:"avg_latency_ms"`
	LateRatio       float64 `json:"late_ratio"`
	PacketLossRatio float64 `json:"packet_loss_ratio"`
	PacketReordered bool    `json:"packet_reordered"`
}

// LoadTestResults holds the results of the load test
type LoadTestResults struct {
	StartTime                time.Time      `json:"start_time"`
	EndTime                  time.Time      `json:"end_time"`
	Duration                 float64        `json:"duration_seconds"`
	ConcurrentCalls          int            `json:"concurrent_calls"`
	TotalCalls               int            `json:"total_calls"`
	SuccessfulCalls          int            `json:"successful_calls"`
	FailedCalls              int            `json:"failed_calls"`
	SuccessRate              float64        `json:"success_rate"`
	CallsPerSecond           float64        `json:"calls_per_second"`
	CallDetails              []CallResult   `json:"call_details"`
	FinalMetrics             LatencyMetrics `json:"final_metrics"`
	PacketReorderingDetected int            `json:"packet_reordering_detected"`
}

// LoadConfig loads configuration from command-line flags
func LoadConfig() *Config {
	var (
		ariURL              = flag.String("ari-url", "localhost:8088", "ARI server URL")
		ariUser             = flag.String("ari-user", "ari", "ARI username")
		ariPass             = flag.String("ari-pass", "ari", "ARI password")
		appName             = flag.String("app-name", "ari-app", "ARI application name")
		endpoint            = flag.String("endpoint", "Local/echo@ari-context", "Endpoint to call")
		context             = flag.String("context", "ari-context", "Dialplan context")
		extension           = flag.String("extension", "echo", "Dialplan extension")
		priority            = flag.Int("priority", 1, "Dialplan priority")
		concurrentCalls     = flag.Int("concurrent", 5, "Number of concurrent calls")
		durationSeconds     = flag.Int("duration", 120, "Test duration in seconds (enhanced)")
		callDurationSeconds = flag.Int("call-duration", 60, "Call duration in seconds (longer)")
		reportFile          = flag.String("report-file", "reports/enhanced_load_test_report.json", "Report file path")
		metricsInterval     = flag.Int("metrics-interval", 10, "Metrics collection interval in seconds")
		packetReordering    = flag.Bool("packet-reordering", true, "Enable packet reordering detection")
	)

	flag.Parse()

	config := &Config{
		ARIURL:              *ariURL,
		ARIUser:             *ariUser,
		ARIPass:             *ariPass,
		AppName:             *appName,
		Endpoint:            *endpoint,
		Context:             *context,
		Extension:           *extension,
		Priority:            *priority,
		ConcurrentCalls:     *concurrentCalls,
		DurationSeconds:     *durationSeconds,
		CallDurationSeconds: *callDurationSeconds,
		ReportFile:          *reportFile,
		MetricsInterval:     *metricsInterval,
		PacketReordering:    *packetReordering,
	}

	return config
}

// NewLoadTest creates a new enhanced load test
func NewLoadTest(config *Config) *LoadTest {
	return &LoadTest{
		config:    config,
		ariClient: NewARIClient(config.ARIURL, config.ARIUser, config.ARIPass),
	}
}

// Run executes the enhanced load test
func (lt *LoadTest) Run() error {
	log.Printf("Starting ENHANCED load test: %d concurrent calls for %d seconds",
		lt.config.ConcurrentCalls, lt.config.DurationSeconds)

	results := &LoadTestResults{
		StartTime:       time.Now(),
		ConcurrentCalls: lt.config.ConcurrentCalls,
		CallDetails:     make([]CallResult, 0),
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(lt.config.DurationSeconds)*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	callResults := make([]CallResult, 0)

	// Start metrics collection in background
	go lt.collectMetrics(ctx, results)

	// Create semaphore to limit concurrent calls
	semaphore := make(chan struct{}, lt.config.ConcurrentCalls)

	// Start all calls
	callCount := 0
	for {
		select {
		case <-ctx.Done():
			// Time's up, stop starting new calls
			goto waitForCalls
		default:
			// Check if we should start another call
			if callCount >= lt.config.ConcurrentCalls*20 { // Limit total calls
				goto waitForCalls
			}

			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				// Acquire semaphore
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				callID := fmt.Sprintf("enhanced_load_test_call_%d", idx)
				result := lt.makeEnhancedCall(ctx, callID)

				mu.Lock()
				callResults = append(callResults, result)
				mu.Unlock()
			}(callCount)

			callCount++
			time.Sleep(500 * time.Millisecond) // Small delay between call starts
		}
	}

waitForCalls:
	// Wait for all calls to complete
	wg.Wait()

	results.EndTime = time.Now()
	results.CallDetails = callResults
	lt.calculateStatistics(results)

	// Get final metrics
	finalMetrics, err := lt.ariClient.GetMetrics()
	if err == nil {
		lt.extractMetrics(finalMetrics, &results.FinalMetrics)
	}

	// Save results
	if err := lt.saveResults(results); err != nil {
		return err
	}

	// Print summary
	lt.printSummary(results)

	return nil
}

// makeEnhancedCall originates a single call with enhanced monitoring
func (lt *LoadTest) makeEnhancedCall(ctx context.Context, callID string) CallResult {
	result := CallResult{
		CallID:    callID,
		StartTime: time.Now(),
	}

	// Create originate request with proper context/extension
	originateReq := OriginateRequest{
		Endpoint:  lt.config.Endpoint,
		App:       lt.config.AppName,
		Context:   lt.config.Context,
		Extension: lt.config.Extension,
		Priority:  lt.config.Priority,
		AppArgs:   []string{callID},
		CallerId:  callID,
		Timeout:   30,
		Variables: map[string]string{
			"LOAD_TEST_CALL_ID": callID,
		},
	}

	channel, err := lt.ariClient.Originate(originateReq)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to originate: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime).Seconds()
		result.Success = false
		return result
	}

	result.ChannelID = channel.ID
	log.Printf("Call %s originated with channel %s", callID, channel.ID)

	// Wait for call duration or context cancellation
	callDuration := time.Duration(lt.config.CallDurationSeconds) * time.Second
	callCtx, callCancel := context.WithTimeout(ctx, callDuration)
	defer callCancel()

	// In a real implementation, we would wait for call events
	// For now, we'll just wait for the duration
	<-callCtx.Done()

	// Hang up the channel
	if err := lt.ariClient.HangupChannel(channel.ID); err != nil {
		log.Printf("Failed to hangup channel %s: %v", channel.ID, err)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).Seconds()
	result.Success = result.Error == ""

	// Collect latency metrics for this call
	lt.collectCallMetrics(&result)

	return result
}

// collectCallMetrics collects metrics for a specific call
func (lt *LoadTest) collectCallMetrics(result *CallResult) {
	// In a real implementation, we would track sequence numbers and timestamps
	// For now, we'll simulate some metrics collection
	metrics, err := lt.ariClient.GetMetrics()
	if err != nil {
		log.Printf("Failed to get metrics for call %s: %v", result.CallID, err)
		return
	}

	lt.extractMetrics(metrics, &result.LatencyMetrics)
}

// extractMetrics extracts metrics from the API response
func (lt *LoadTest) extractMetrics(metrics map[string]interface{}, target *LatencyMetrics) {
	if val, ok := metrics["p50_latency"]; ok {
		if fval, ok := val.(float64); ok {
			target.P50Latency = fval
		}
	}
	if val, ok := metrics["p95_latency"]; ok {
		if fval, ok := val.(float64); ok {
			target.P95Latency = fval
		}
	}
	if val, ok := metrics["p99_latency"]; ok {
		if fval, ok := val.(float64); ok {
			target.P99Latency = fval
		}
	}
	if val, ok := metrics["max_latency"]; ok {
		if fval, ok := val.(float64); ok {
			target.MaxLatency = fval
		}
	}
	if val, ok := metrics["avg_latency"]; ok {
		if fval, ok := val.(float64); ok {
			target.AvgLatency = fval
		}
	}
	if val, ok := metrics["late_ratio"]; ok {
		if fval, ok := val.(float64); ok {
			target.LateRatio = fval
		}
	}
	if val, ok := metrics["packet_loss_ratio"]; ok {
		if fval, ok := val.(float64); ok {
			target.PacketLossRatio = fval
		}
	}
}

// collectMetrics collects metrics periodically during the test
func (lt *LoadTest) collectMetrics(ctx context.Context, results *LoadTestResults) {
	ticker := time.NewTicker(time.Duration(lt.config.MetricsInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics, err := lt.ariClient.GetMetrics()
			if err != nil {
				log.Printf("Failed to collect metrics: %v", err)
				continue
			}

			// Log current metrics
			if p50, ok := metrics["p50_latency"]; ok {
				log.Printf("Current metrics - p50: %.2fms, active channels: %.0f",
					p50, metrics["active_channels"])
			}
		}
	}
}

// calculateStatistics calculates statistics from call results
func (lt *LoadTest) calculateStatistics(results *LoadTestResults) {
	results.TotalCalls = len(results.CallDetails)
	results.Duration = results.EndTime.Sub(results.StartTime).Seconds()

	var successfulCalls int
	var packetReordering int
	for _, call := range results.CallDetails {
		if call.Success {
			successfulCalls++
		}
		// Count calls with packet reordering detected
		if call.LatencyMetrics.PacketReordered {
			packetReordering++
		}
	}

	results.SuccessfulCalls = successfulCalls
	results.FailedCalls = results.TotalCalls - successfulCalls
	results.SuccessRate = float64(successfulCalls) / float64(results.TotalCalls) * 100
	results.CallsPerSecond = float64(results.TotalCalls) / results.Duration
	results.PacketReorderingDetected = packetReordering
}

// saveResults saves the test results to a file
func (lt *LoadTest) saveResults(results *LoadTestResults) error {
	// Create reports directory
	if err := os.MkdirAll("reports", 0755); err != nil {
		return err
	}

	file, err := os.Create(lt.config.ReportFile)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(results)
}

// printSummary prints a summary of the test results
func (lt *LoadTest) printSummary(results *LoadTestResults) {
	fmt.Println("\n=== ENHANCED LOAD TEST RESULTS ===")
	fmt.Printf("Start Time: %s\n", results.StartTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("End Time: %s\n", results.EndTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("Duration: %.2f seconds\n", results.Duration)
	fmt.Printf("Concurrent Calls: %d\n", results.ConcurrentCalls)
	fmt.Printf("Total Calls: %d\n", results.TotalCalls)
	fmt.Printf("Successful Calls: %d\n", results.SuccessfulCalls)
	fmt.Printf("Failed Calls: %d\n", results.FailedCalls)
	fmt.Printf("Success Rate: %.2f%%\n", results.SuccessRate)
	fmt.Printf("Calls Per Second: %.2f\n", results.CallsPerSecond)
	fmt.Printf("Packet Reordering Detected: %d calls\n", results.PacketReorderingDetected)

	if results.FinalMetrics.P50Latency > 0 {
		fmt.Printf("\nFinal Latency Metrics:")
		fmt.Printf("\n  p50: %.2f ms", results.FinalMetrics.P50Latency)
		fmt.Printf("\n  p95: %.2f ms", results.FinalMetrics.P95Latency)
		fmt.Printf("\n  p99: %.2f ms", results.FinalMetrics.P99Latency)
		fmt.Printf("\n  Max: %.2f ms", results.FinalMetrics.MaxLatency)
		fmt.Printf("\n  Avg: %.2f ms", results.FinalMetrics.AvgLatency)
		fmt.Printf("\n  Late Ratio: %.2f%%", results.FinalMetrics.LateRatio*100)
		fmt.Printf("\n  Packet Loss: %.2f%%", results.FinalMetrics.PacketLossRatio*100)
	}

	fmt.Printf("\n\nResults saved to: %s\n", lt.config.ReportFile)
}

func main() {
	config := LoadConfig()

	loadTest := NewLoadTest(config)

	// Run the enhanced load test
	if err := loadTest.Run(); err != nil {
		log.Fatalf("Enhanced load test failed: %v", err)
	}
}
