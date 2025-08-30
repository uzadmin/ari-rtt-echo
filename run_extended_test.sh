#!/bin/bash

# Script to run extended 100-call test with zombie/unclosed port monitoring

echo "=== Extended 100-Call Test with Monitoring ==="

# Clean up any existing processes
pkill -f "find_zombies_and_ports" 2>/dev/null || true
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
go build -o bin/find_zombies_and_ports find_zombies_and_ports.go

if [ $? -ne 0 ]; then
    echo "❌ Failed to build components"
    exit 1
fi

echo "✅ All components built successfully"

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
    exit 1
fi

echo "✅ Echo server started"

# Start monitoring in background
echo "Starting zombie and port monitoring..."
./bin/find_zombies_and_ports > logs/monitoring.log 2>&1 &
MONITOR_PID=$!

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
        sleep 10
    done
}

# Run load test in background and display metrics
echo ""
echo "=== STARTING 100-CALL TEST ==="
echo "Configuration:"
echo "  - Concurrent calls: 100"
echo "  - Test duration: 600 seconds (10 minutes)"
echo "  - Call duration: 1800 seconds (30 minutes)"
echo ""
echo "This test will run for approximately 10 minutes..."
echo ""

# Start metrics display
display_metrics &

# Run the load test
echo "Starting load test..."
./bin/load-test -concurrent=100 -duration=600 -call-duration=1800 > logs/load-test.log 2>&1 &
LOAD_TEST_PID=$!

# Wait for load test to complete (with timeout)
echo "Waiting for test to complete (timeout: 20 minutes)..."
if timeout 1200 wait $LOAD_TEST_PID 2>/dev/null; then
    echo "✅ Load test completed successfully"
    LOAD_TEST_SUCCESS=true
else
    echo "⚠️  Load test timed out or failed"
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

REPORT_FILE="reports/extended_test_summary_$(date +%Y%m%d_%H%M%S).txt"
cat > "$REPORT_FILE" << EOF
Extended 100-Call Test Summary Report
===================================
Generated: $(date)
Project: ARI Service with RTP Latency Measurement

Test Configuration:
- Concurrent Calls: 100
- Test Duration: 600 seconds (10 minutes)
- Call Duration: 1800 seconds (30 minutes)
- Echo Server: 127.0.0.1:4000
- ARI Service: 0.0.0.0:21000-31000

Test Result: $(if [ "$LOAD_TEST_SUCCESS" = true ]; then echo "SUCCESS"; else echo "FAILED/INCOMPLETE"; fi)

Files Generated:
- ARI Service Log: logs/ari-service.log
- Echo Server Log: logs/echo-server.log
- Load Test Results: logs/load-test.log
- Monitoring Log: logs/monitoring.log

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
- Check logs/monitoring.log for detailed zombie and port monitoring
- Look for warnings about zombie channels or unclosed ports

To view detailed results:
- Load test log: cat logs/load-test.log
- Service logs: cat logs/ari-service.log
- Echo server log: cat logs/echo-server.log
- Monitoring log: cat logs/monitoring.log

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
    echo "⚠️  Test may have failed or timed out. Check logs for details."
fi

echo ""
echo "To view full results:"
echo "cat $REPORT_FILE"
echo ""
echo "To check for zombie channels and unclosed ports:"
echo "cat logs/monitoring.log"