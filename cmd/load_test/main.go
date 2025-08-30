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
	ConcurrentCalls     int
	DurationSeconds     int
	CallDurationSeconds int
	ReportFile          string
}

// LoadTest runs the load test
type LoadTest struct {
	config    *Config
	ariClient *ARIClient
}

// CallResult represents the result of a single call
type CallResult struct {
	CallID    string    `json:"call_id"`
	ChannelID string    `json:"channel_id,omitempty"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// LoadTestResults holds the results of the load test
type LoadTestResults struct {
	StartTime       time.Time    `json:"start_time"`
	EndTime         time.Time    `json:"end_time"`
	Duration        float64      `json:"duration_seconds"`
	ConcurrentCalls int          `json:"concurrent_calls"`
	TotalCalls      int          `json:"total_calls"`
	SuccessfulCalls int          `json:"successful_calls"`
	FailedCalls     int          `json:"failed_calls"`
	SuccessRate     float64      `json:"success_rate"`
	CallsPerSecond  float64      `json:"calls_per_second"`
	CallDetails     []CallResult `json:"call_details"`
}

// LoadConfig loads configuration from command-line flags
func LoadConfig() *Config {
	var (
		ariURL              = flag.String("ari-url", "localhost:8088", "ARI server URL")
		ariUser             = flag.String("ari-user", "ari", "ARI username")
		ariPass             = flag.String("ari-pass", "ari", "ARI password")
		appName             = flag.String("app-name", "ari-app", "ARI application name")
		endpoint            = flag.String("endpoint", "Local/echo@ari-context", "Endpoint to call")
		concurrentCalls     = flag.Int("concurrent", 10, "Number of concurrent calls")
		durationSeconds     = flag.Int("duration", 60, "Test duration in seconds")
		callDurationSeconds = flag.Int("call-duration", 30, "Call duration in seconds")
		reportFile          = flag.String("report-file", "reports/load_test_report.json", "Report file path")
	)

	flag.Parse()

	config := &Config{
		ARIURL:              *ariURL,
		ARIUser:             *ariUser,
		ARIPass:             *ariPass,
		AppName:             *appName,
		Endpoint:            *endpoint,
		ConcurrentCalls:     *concurrentCalls,
		DurationSeconds:     *durationSeconds,
		CallDurationSeconds: *callDurationSeconds,
		ReportFile:          *reportFile,
	}

	return config
}

// NewLoadTest creates a new load test
func NewLoadTest(config *Config) *LoadTest {
	return &LoadTest{
		config:    config,
		ariClient: NewARIClient(config.ARIURL, config.ARIUser, config.ARIPass),
	}
}

// Run executes the load test
func (lt *LoadTest) Run() error {
	log.Printf("Starting load test: %d concurrent calls for %d seconds",
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
			if callCount >= lt.config.ConcurrentCalls*10 { // Limit total calls
				goto waitForCalls
			}

			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				// Acquire semaphore
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				callID := fmt.Sprintf("load_test_call_%d", idx)
				result := lt.makeCall(ctx, callID)

				mu.Lock()
				callResults = append(callResults, result)
				mu.Unlock()
			}(callCount)

			callCount++
			time.Sleep(100 * time.Millisecond) // Small delay between call starts
		}
	}

waitForCalls:
	// Wait for all calls to complete
	wg.Wait()

	results.EndTime = time.Now()
	results.CallDetails = callResults
	lt.calculateStatistics(results)

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

	// Create originate request
	originateReq := OriginateRequest{
		Endpoint: lt.config.Endpoint,
		App:      lt.config.AppName,
		AppArgs:  []string{callID},
		CallerId: callID,
		Timeout:  30,
		Variables: map[string]string{
			"LOAD_TEST_CALL_ID": callID,
		},
	}

	channel, err := lt.ariClient.Originate(originateReq)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to originate: %v", err)
		result.EndTime = time.Now()
		result.Success = false
		return result
	}

	result.ChannelID = channel.ID

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
	result.Success = result.Error == ""

	return result
}

// calculateStatistics calculates statistics from call results
func (lt *LoadTest) calculateStatistics(results *LoadTestResults) {
	results.TotalCalls = len(results.CallDetails)
	results.Duration = results.EndTime.Sub(results.StartTime).Seconds()

	var successfulCalls int
	for _, call := range results.CallDetails {
		if call.Success {
			successfulCalls++
		}
	}

	results.SuccessfulCalls = successfulCalls
	results.FailedCalls = results.TotalCalls - successfulCalls
	results.SuccessRate = float64(successfulCalls) / float64(results.TotalCalls) * 100
	results.CallsPerSecond = float64(results.TotalCalls) / results.Duration
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
	fmt.Printf("Duration: %.2f seconds\n", results.Duration)
	fmt.Printf("Concurrent Calls: %d\n", results.ConcurrentCalls)
	fmt.Printf("Total Calls: %d\n", results.TotalCalls)
	fmt.Printf("Successful Calls: %d\n", results.SuccessfulCalls)
	fmt.Printf("Failed Calls: %d\n", results.FailedCalls)
	fmt.Printf("Success Rate: %.2f%%\n", results.SuccessRate)
	fmt.Printf("Calls Per Second: %.2f\n", results.CallsPerSecond)

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
