#!/bin/bash

# Script to run 100-call test with monitoring for zombie channels and unclosed ports

echo "=== Starting 100-Call Test with Monitoring ==="

# Clean up any existing processes
pkill -f "monitor_resources.sh" 2>/dev/null || true
pkill -f "ari-service" 2>/dev/null || true
pkill -f "echo-server" 2>/dev/null || true

# Wait a moment for processes to terminate
sleep 2

# Clean up logs
rm -f logs/*.log
rm -f reports/*.json

# Create necessary directories
mkdir -p logs reports

# Build components
echo "Building components..."
go build -o bin/ari-service ./cmd/ari-service
go build -o bin/echo-server ./cmd/echo
go build -o bin/load-test ./cmd/load_test

# Start monitoring in background
echo "Starting resource monitoring..."
./monitor_resources.sh > logs/monitor.log 2>&1 &
MONITOR_PID=$!

# Start ARI service
echo "Starting ARI service..."
./bin/ari-service > logs/ari-service.log 2>&1 &
ARI_PID=$!

# Wait for ARI service to start
sleep 3

# Check if ARI service is running
if ! kill -0 $ARI_PID 2>/dev/null; then
    echo "❌ ARI service failed to start"
    cat logs/ari-service.log
    kill $MONITOR_PID 2>/dev/null || true
    exit 1
fi

echo "✅ ARI service started"

# Start Echo server
echo "Starting Echo server..."
./bin/echo-server > logs/echo-server.log 2>&1 &
ECHO_PID=$!

# Wait for echo server to start
sleep 2

# Check if Echo server is running
if ! kill -0 $ECHO_PID 2>/dev/null; then
    echo "❌ Echo server failed to start"
    cat logs/echo-server.log
    kill $ARI_PID 2>/dev/null || true
    kill $MONITOR_PID 2>/dev/null || true
    exit 1
fi

echo "✅ Echo server started"

# Function to display real-time metrics
display_metrics() {
    echo ""
    echo "Real-time Metrics:"
    echo "=================="
    while kill -0 $ARI_PID 2>/dev/null; do
        if curl -s http://localhost:9090/metrics > /tmp/metrics.json 2>/dev/null; then
            echo -n "Channels: $(jq -r '.active_channels' /tmp/metrics.json) | "
            echo -n "RTT p50: $(jq -r '.p50_latency' /tmp/metrics.json)ms | "
            echo -n "RTT p95: $(jq -r '.p95_latency' /tmp/metrics.json)ms | "
            echo -n "RTT p99: $(jq -r '.p99_latency' /tmp/metrics.json)ms | "
            echo -n "Max RTT: $(jq -r '.max_latency' /tmp/metrics.json)ms"
            echo ""
            echo -n "Packet Loss: $(jq -r '.packet_loss_ratio' /tmp/metrics.json) | "
            echo -n "Late Packets: $(jq -r '.late_ratio' /tmp/metrics.json)"
            echo -e "\n"
        fi
        sleep 5
    done
}

# Run load test in background and display metrics
echo "Running 100-call test (100 concurrent calls for 10 minutes with 30-minute call duration)..."
echo "This will take approximately 10 minutes to complete..."
timeout 700s ./bin/load-test -concurrent=100 -duration=600 -call-duration=1800 > logs/load-test.log 2>&1 &
LOAD_TEST_PID=$!

# Display metrics while load test is running
display_metrics &

# Wait for load test to complete
if wait $LOAD_TEST_PID; then
    echo "Load test completed successfully"
    LOAD_TEST_SUCCESS=true
else
    echo "Load test failed or timed out"
    LOAD_TEST_SUCCESS=false
fi

# Kill the metrics display process
pkill -P $$ display_metrics 2>/dev/null || true

# Generate test report
echo "Generating test report..."

# Get final metrics
if curl -s http://localhost:9090/metrics > /tmp/final_metrics.json 2>/dev/null; then
    FINAL_METRICS_AVAILABLE=true
else
    FINAL_METRICS_AVAILABLE=false
fi

REPORT_FILE="reports/hundred_call_test_summary.txt"
cat > "$REPORT_FILE" << EOF
100-Call Test Summary Report
===========================
Generated: $(date)
Project: ARI Service with RTP Latency Measurement

Test Configuration:
- Concurrent Calls: 100
- Test Duration: 600 seconds (10 minutes)
- Call Duration: 1800 seconds (30 minutes)
- Echo Server: 127.0.0.1:4000
- ARI Service: 0.0.0.0:21000-31000

Test Result: $(if [ "$LOAD_TEST_SUCCESS" = true ]; then echo "SUCCESS"; else echo "FAILED"; fi)

Files Generated:
- ARI Service Log: logs/ari-service.log
- Echo Server Log: logs/echo-server.log
- Load Test Results: logs/load-test.log
- Monitoring Log: logs/monitor.log

Final Metrics:
$(if [ "$FINAL_METRICS_AVAILABLE" = true ]; then
    cat << METRICS
- Total Channels: $(jq -r '.total_channels' /tmp/final_metrics.json)
- Active Channels: $(jq -r '.active_channels' /tmp/final_metrics.json)
- Total Latencies: $(jq -r '.total_latencies' /tmp/final_metrics.json)
- RTT p50: $(jq -r '.p50_latency' /tmp/final_metrics.json) ms
- RTT p95: $(jq -r '.p95_latency' /tmp/final_metrics.json) ms
- RTT p99: $(jq -r '.p99_latency' /tmp/final_metrics.json) ms
- Max RTT: $(jq -r '.max_latency' /tmp/final_metrics.json) ms
- Average RTT: $(jq -r '.avg_latency' /tmp/final_metrics.json) ms
- Packet Loss Ratio: $(jq -r '.packet_loss_ratio' /tmp/final_metrics.json)
- Late Packet Ratio: $(jq -r '.late_ratio' /tmp/final_metrics.json)
METRICS
else
    echo "No metrics available"
fi)

Resource Monitoring Summary:
- Check logs/monitor.log for detailed resource usage
- Look for warnings about zombie channels or unclosed ports

To view detailed results:
- Load test log: cat logs/load-test.log
- Service logs: cat logs/ari-service.log
- Echo server log: cat logs/echo-server.log
- Monitoring log: cat logs/monitor.log

Health Check:
- ARI Service: curl http://localhost:9090/health
- Metrics: curl http://localhost:9090/metrics
EOF

# Stop services
echo "Stopping services..."
kill $ECHO_PID 2>/dev/null || true
kill $ARI_PID 2>/dev/null || true
kill $MONITOR_PID 2>/dev/null || true

# Wait for services to stop
sleep 2

echo ""
echo "=== TEST COMPLETE ==="
echo "Summary report: $REPORT_FILE"
echo ""

if [ "$LOAD_TEST_SUCCESS" = true ]; then
    echo "✅ Test completed successfully!"
    echo ""
    echo "Quick Results:"
    echo "Final RTT Metrics:"
    if [ "$FINAL_METRICS_AVAILABLE" = true ]; then
        echo "RTT p50: $(jq -r '.p50_latency' /tmp/final_metrics.json) ms"
        echo "RTT p95: $(jq -r '.p95_latency' /tmp/final_metrics.json) ms"
        echo "RTT p99: $(jq -r '.p99_latency' /tmp/final_metrics.json) ms"
        echo "Max RTT: $(jq -r '.max_latency' /tmp/final_metrics.json) ms"
        echo "Packet Loss: $(jq -r '.packet_loss_ratio' /tmp/final_metrics.json)"
        echo "Late Packets: $(jq -r '.late_ratio' /tmp/final_metrics.json)"
    fi
else
    echo "❌ Test failed. Check logs for details."
    exit 1
fi

echo ""
echo "To view full results:"
echo "cat $REPORT_FILE"