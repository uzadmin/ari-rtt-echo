package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// Config holds load test configuration
type Config struct {
	ARIURL         string
	ARIUser        string
	ARIPass        string
	AppName        string
	Endpoint       string
	Count          int
	DurationMs     int
	DelayBetweenMs int
	ReportFile     string
	ARIServerURL   string // URL for ARI server metrics
}

// LoadTest runs the load test
type LoadTest struct {
	config    *Config
	ariClient *ARIClient
}

// CallMetrics tracks detailed metrics for a call
type CallMetrics struct {
	CallID           string
	ChannelID        string
	OriginateTime    time.Time
	AnswerTime       time.Time
	FirstPacketTime  time.Time
	LastPacketTime   time.Time
	EndTime          time.Time
	PacketTimestamps []time.Time
	SequenceNumbers  []uint16
	RTTMeasurements  []time.Duration
	LatePackets      int
	LostPackets      int
	TotalPackets     int
}

// CallResult represents the result of a single call
type CallResult struct {
	CallID       string    `json:"call_id"`
	ChannelID    string    `json:"channel_id,omitempty"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Success      bool      `json:"success"`
	Error        string    `json:"error,omitempty"`
	LatencyP50   float64   `json:"latency_p50,omitempty"`
	LatencyP95   float64   `json:"latency_p95,omitempty"`
	LatencyP99   float64   `json:"latency_p99,omitempty"`
	LatencyMax   float64   `json:"latency_max,omitempty"`
	LateRatio    float64   `json:"late_ratio,omitempty"`
	PacketLoss   float64   `json:"packet_loss,omitempty"`
	RoundTripP95 float64   `json:"round_trip_p95,omitempty"`
	RoundTripP99 float64   `json:"round_trip_p99,omitempty"`
}

// LoadTestResults holds the results of the load test
type LoadTestResults struct {
	StartTime       time.Time    `json:"start_time"`
	EndTime         time.Time    `json:"end_time"`
	DurationMs      int          `json:"duration_ms"`
	ConcurrentCalls int          `json:"concurrent_calls"`
	TotalCalls      int          `json:"total_calls"`
	SuccessfulCalls int          `json:"successful_calls"`
	FailedCalls     int          `json:"failed_calls"`
	SuccessRate     float64      `json:"success_rate"`
	CallsPerSecond  float64      `json:"calls_per_second"`
	CallDetails     []CallResult `json:"call_details"`
	AvgLatencyP50   float64      `json:"avg_latency_p50"`
	AvgLatencyP95   float64      `json:"avg_latency_p95"`
	AvgLatencyP99   float64      `json:"avg_latency_p99"`
	AvgLateRatio    float64      `json:"avg_late_ratio"`
	AvgPacketLoss   float64      `json:"avg_packet_loss"`
}

// ARIClient handles communication with the ARI API
type ARIClient struct {
	ARIURL  string
	ARIUser string
	ARIPass string
}

// Channel represents an ARI channel
type Channel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// OriginateRequest represents a request to originate a call
type OriginateRequest struct {
	Endpoint  string            `json:"endpoint"`
	App       string            `json:"app"`
	AppArgs   []string          `json:"appArgs,omitempty"`
	CallerId  string            `json:"callerId,omitempty"`
	Timeout   int               `json:"timeout,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

// ARIServerMetrics represents metrics from the ARI server
type ARIServerMetrics struct {
	RTTP50            string  `json:"rtt_p50"`
	RTTP95            string  `json:"rtt_p95"`
	RTTP99            string  `json:"rtt_p99"`
	RTTMax            string  `json:"rtt_max"`
	Channels          int64   `json:"channels"`
	PacketLossPercent float64 `json:"packet_loss_percent"`
	LateRatioPercent  float64 `json:"late_ratio_percent"`
}

// LoadConfig loads configuration from command-line flags and environment variables
func LoadConfig() *Config {
	// Load .env file if it exists
	_ = godotenv.Load()

	var (
		ariURL         = flag.String("ari-url", getEnv("ASTERISK_HOST", "localhost")+":"+getEnv("ASTERISK_PORT", "8088"), "ARI server URL")
		ariUser        = flag.String("ari-user", getEnv("ASTERISK_USERNAME", "ari"), "ARI username")
		ariPass        = flag.String("ari-pass", getEnv("ASTERISK_PASSWORD", "ari"), "ARI password")
		appName        = flag.String("app-name", getEnv("ASTERISK_APP_NAME", "ari-app"), "ARI application name")
		endpoint       = flag.String("endpoint", getEnv("LOAD_TEST_ENDPOINT", "Local/echo@ari-context"), "Endpoint to call")
		count          = flag.Int("count", getEnvAsInt("LOAD_TEST_CONCURRENT_CALLS", 30), "Number of calls to make")
		durationMs     = flag.Int("duration-ms", getEnvAsInt("LOAD_TEST_DURATION_SECONDS", 30)*1000, "Test duration in milliseconds")
		delayBetweenMs = flag.Int("delay-between-ms", 100, "Delay between call starts in milliseconds")
		reportFile     = flag.String("report-file", getEnv("LOAD_TEST_REPORT", "reports/load_test_new_report.json"), "Report file path")
		ariServerURL   = flag.String("ari-server-url", getEnv("ARI_SERVER_URL", "http://localhost:9091"), "ARI server metrics URL")
	)

	flag.Parse()

	config := &Config{
		ARIURL:         *ariURL,
		ARIUser:        *ariUser,
		ARIPass:        *ariPass,
		AppName:        *appName,
		Endpoint:       *endpoint,
		Count:          *count,
		DurationMs:     *durationMs,
		DelayBetweenMs: *delayBetweenMs,
		ReportFile:     *reportFile,
		ARIServerURL:   *ariServerURL,
	}

	return config
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

// NewLoadTest creates a new load test
func NewLoadTest(config *Config) *LoadTest {
	return &LoadTest{
		config:    config,
		ariClient: &ARIClient{ARIURL: config.ARIURL, ARIUser: config.ARIUser, ARIPass: config.ARIPass},
	}
}

// Run executes the load test
func (lt *LoadTest) Run() error {
	log.Printf("Starting load test: %d calls for %d ms with %d ms delay between calls",
		lt.config.Count, lt.config.DurationMs, lt.config.DelayBetweenMs)

	results := &LoadTestResults{
		StartTime:       time.Now(),
		ConcurrentCalls: lt.config.Count,
		DurationMs:      lt.config.DurationMs,
		CallDetails:     make([]CallResult, 0),
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(lt.config.DurationMs)*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	callResults := make([]CallResult, 0)

	// Start all calls
	for i := 0; i < lt.config.Count; i++ {
		select {
		case <-ctx.Done():
			// Time's up, stop starting new calls
			break
		default:
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				callID := fmt.Sprintf("load_test_call_%d", idx)
				result := lt.makeCall(ctx, callID)

				mu.Lock()
				callResults = append(callResults, result)
				mu.Unlock()
			}(i)

			// Delay between call starts
			time.Sleep(time.Duration(lt.config.DelayBetweenMs) * time.Millisecond)
		}
	}

	// Wait for all calls to complete
	wg.Wait()

	results.EndTime = time.Now()
	results.CallDetails = callResults
	lt.calculateStatistics(results)

	// Get final metrics from ARI server
	ariMetrics, err := lt.getARIServerMetrics()
	if err == nil {
		log.Printf("Final ARI Server Metrics: RTT p50=%s, p95=%s, p99=%s, Max=%s, Packet Loss=%.4f%%, Late Ratio=%.4f%%",
			ariMetrics.RTTP50, ariMetrics.RTTP95, ariMetrics.RTTP99, ariMetrics.RTTMax,
			ariMetrics.PacketLossPercent, ariMetrics.LateRatioPercent)
	}

	// Save results
	if err := lt.saveResults(results); err != nil {
		return err
	}

	// Print summary
	lt.printSummary(results)

	return nil
}

// makeCall originates a single call
func (lt *LoadTest) makeCall(ctx context.Context, callID string) CallResult {
	result := CallResult{
		CallID:    callID,
		StartTime: time.Now(),
	}

	log.Printf("Making call with ID: %s", callID)

	// Record originate time
	originateTime := time.Now()

	// Create originate request
	originateReq := OriginateRequest{
		Endpoint: lt.config.Endpoint,
		App:      lt.config.AppName,
		AppArgs:  []string{callID},
		CallerId: callID,
		Timeout:  30,
		Variables: map[string]string{
			"LOAD_TEST_CALL_ID": callID,
			"TIMESTAMP":         originateTime.Format(time.RFC3339Nano),
		},
	}

	channel, err := lt.ariClient.Originate(originateReq)
	if err != nil {
		log.Printf("Failed to originate call %s: %v", callID, err)
		result.Error = fmt.Sprintf("Failed to originate: %v", err)
		result.EndTime = time.Now()
		result.Success = false
		return result
	}

	log.Printf("Successfully originated call %s with channel ID: %s", callID, channel.ID)
	result.ChannelID = channel.ID

	// Wait for call duration or context cancellation
	callDuration := time.Duration(lt.config.DurationMs) * time.Millisecond
	callCtx, callCancel := context.WithTimeout(ctx, callDuration)
	defer callCancel()

	// In a real implementation, we would wait for call events
	// For now, we'll just wait for the duration
	<-callCtx.Done()

	// Hang up the channel
	// hangupStart := time.Now()  // Commenting out unused variable
	if err := lt.ariClient.HangupChannel(channel.ID); err != nil {
		log.Printf("Failed to hangup channel %s: %v", channel.ID, err)
	}
	// hangupEnd := time.Now()  // Commenting out unused variable

	// Record end time
	endTime := time.Now()

	// Calculate metrics
	result.EndTime = endTime
	result.Success = result.Error == ""

	// Get actual metrics from ARI server
	ariMetrics, err := lt.getARIServerMetrics()
	if err == nil {
		// Parse the time.Duration strings to get numeric values
		// For simplicity, we'll convert to milliseconds
		result.LatencyP50 = lt.parseDurationToMs(ariMetrics.RTTP50)
		result.LatencyP95 = lt.parseDurationToMs(ariMetrics.RTTP95)
		result.LatencyP99 = lt.parseDurationToMs(ariMetrics.RTTP99)
		result.LatencyMax = lt.parseDurationToMs(ariMetrics.RTTMax)
		result.LateRatio = ariMetrics.LateRatioPercent
		result.PacketLoss = ariMetrics.PacketLossPercent
	} else {
		log.Printf("Failed to get ARI server metrics: %v", err)
	}

	// Calculate round-trip metrics (originate to hangup)
	roundTripTime := time.Since(originateTime)
	result.RoundTripP95 = float64(roundTripTime) / float64(time.Millisecond)
	result.RoundTripP99 = float64(roundTripTime) / float64(time.Millisecond)

	return result
}

// getARIServerMetrics fetches metrics from the ARI server
func (lt *LoadTest) getARIServerMetrics() (*ARIServerMetrics, error) {
	url := fmt.Sprintf("%s/sla-metrics", lt.config.ARIServerURL)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ARI server returned status %d", resp.StatusCode)
	}

	var metrics ARIServerMetrics
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return nil, err
	}

	return &metrics, nil
}

// parseDurationToMs converts a duration string to milliseconds
func (lt *LoadTest) parseDurationToMs(durationStr string) float64 {
	// Parse duration string like "5.234ms" or "1.234µs"
	d, err := time.ParseDuration(durationStr)
	if err != nil {
		log.Printf("Failed to parse duration %s: %v", durationStr, err)
		return 0
	}

	return float64(d) / float64(time.Millisecond)
}

// Originate originates a call
func (c *ARIClient) Originate(request OriginateRequest) (*Channel, error) {
	url := fmt.Sprintf("http://%s/ari/channels", c.ARIURL)
	log.Printf("Making ARI request to: %s", url)

	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	log.Printf("Request body: %s", string(body))

	// Create HTTP client
	client := &http.Client{}

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Set basic auth
	req.SetBasicAuth(c.ARIUser, c.ARIPass)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HTTP request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body: %v", err)
		return nil, err
	}
	log.Printf("Response status: %d, body: %s", resp.StatusCode, string(respBody))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ARI API error: %d - %s", resp.StatusCode, string(respBody))
	}

	var channel Channel
	if err := json.Unmarshal(respBody, &channel); err != nil {
		return nil, err
	}

	return &channel, nil
}

// HangupChannel hangs up a channel
func (c *ARIClient) HangupChannel(channelID string) error {
	url := fmt.Sprintf("http://%s/ari/channels/%s", c.ARIURL, channelID)

	// Create HTTP client
	client := &http.Client{}

	// Create request
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	// Set basic auth
	req.SetBasicAuth(c.ARIUser, c.ARIPass)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("ARI API error: %d - %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// calculateStatistics calculates statistics from call results
func (lt *LoadTest) calculateStatistics(results *LoadTestResults) {
	results.TotalCalls = len(results.CallDetails)
	results.DurationMs = int(results.EndTime.Sub(results.StartTime).Milliseconds())

	var successfulCalls int
	var totalLatencyP50, totalLatencyP95, totalLatencyP99, totalLateRatio, totalPacketLoss float64
	count := 0

	for _, call := range results.CallDetails {
		if call.Success {
			successfulCalls++
			// Accumulate metrics from actual ARI server data
			totalLatencyP50 += call.LatencyP50
			totalLatencyP95 += call.LatencyP95
			totalLatencyP99 += call.LatencyP99
			totalLateRatio += call.LateRatio
			totalPacketLoss += call.PacketLoss
			count++
		}
	}

	results.SuccessfulCalls = successfulCalls
	results.FailedCalls = results.TotalCalls - successfulCalls
	results.SuccessRate = float64(successfulCalls) / float64(results.TotalCalls) * 100
	results.CallsPerSecond = float64(results.TotalCalls) / (float64(results.DurationMs) / 1000.0)

	if count > 0 {
		results.AvgLatencyP50 = totalLatencyP50 / float64(count)
		results.AvgLatencyP95 = totalLatencyP95 / float64(count)
		results.AvgLatencyP99 = totalLatencyP99 / float64(count)
		results.AvgLateRatio = totalLateRatio / float64(count)
		results.AvgPacketLoss = totalPacketLoss / float64(count)
	}
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
	fmt.Println("\n=== LOAD TEST RESULTS ===")
	fmt.Printf("Start Time: %s\n", results.StartTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("End Time: %s\n", results.EndTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("Duration: %d ms\n", results.DurationMs)
	fmt.Printf("Total Calls: %d\n", results.TotalCalls)
	fmt.Printf("Successful Calls: %d\n", results.SuccessfulCalls)
	fmt.Printf("Failed Calls: %d\n", results.FailedCalls)
	fmt.Printf("Success Rate: %.2f%%\n", results.SuccessRate)
	fmt.Printf("Calls Per Second: %.2f\n", results.CallsPerSecond)

	if results.TotalCalls > 0 {
		fmt.Printf("\nLatency Metrics:\n")
		fmt.Printf("  Avg RTT p50: %.2f ms\n", results.AvgLatencyP50)
		fmt.Printf("  Avg RTT p95: %.2f ms\n", results.AvgLatencyP95)
		fmt.Printf("  Avg RTT p99: %.2f ms\n", results.AvgLatencyP99)
		fmt.Printf("  Avg Late Ratio: %.4f%%\n", results.AvgLateRatio)
		fmt.Printf("  Avg Packet Loss: %.4f%%\n", results.AvgPacketLoss)
	}

	fmt.Printf("\nResults saved to: %s\n", lt.config.ReportFile)
}

func main() {
	config := LoadConfig()

	loadTest := NewLoadTest(config)

	// Run the load test
	if err := loadTest.Run(); err != nil {
		log.Fatalf("Load test failed: %v", err)
	}
}
