#!/bin/bash

# ARI Service Runner
# This script starts all components and runs the load test
#
# Usage:
#   ./run.sh          - Run regular load test
#   ./run.sh prod     - Run production test (50 calls over 5 minutes)

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

echo "=== ARI Service Runner ==="
echo "Project root: $PROJECT_ROOT"

# Step 1: Environment Setup
echo ""
echo "Step 1: Environment Setup"
echo "========================"

# Create necessary directories
mkdir -p bin logs reports

# Export environment variables
export ARI_URL=${ARI_URL:-localhost:8088}
export ARI_USER=${ARI_USER:-ari}
export ARI_PASS=${ARI_PASS:-ari}
export APP_NAME=${APP_NAME:-ari-app}
export BIND_IP=${BIND_IP:-0.0.0.0}
export PORT_RANGE=${PORT_RANGE:-4500-50000}
export ECHO_HOST=${ECHO_HOST:-127.0.0.1}
export ECHO_PORT=${ECHO_PORT:-8080}
export METRICS_INTERVAL_SEC=${METRICS_INTERVAL_SEC:-5}

# Load test configuration
export LOAD_TEST_CONCURRENT_CALLS=${LOAD_TEST_CONCURRENT_CALLS:-10}
export LOAD_TEST_DURATION_SECONDS=${LOAD_TEST_DURATION_SECONDS:-60}
export LOAD_TEST_CALL_DURATION_SECONDS=${LOAD_TEST_CALL_DURATION_SECONDS:-30}
export LOAD_TEST_ENDPOINT=${LOAD_TEST_ENDPOINT:-Local/echo@ari-context}
export LOAD_TEST_REPORT=${LOAD_TEST_REPORT:-reports/load_test_report.json}

# Production test configuration (50 calls over 5 minutes)
export PROD_TEST_CONCURRENT_CALLS=50
export PROD_TEST_DURATION_SECONDS=300
export PROD_TEST_CALL_DURATION_SECONDS=60
export PROD_TEST_REPORT=reports/prod_test_report.json

# 100 calls over 10 minutes test configuration
export HUNDRED_CALL_TEST_CONCURRENT_CALLS=100
export HUNDRED_CALL_TEST_DURATION_SECONDS=600
export HUNDRED_CALL_TEST_CALL_DURATION_SECONDS=1800
export HUNDRED_CALL_TEST_REPORT=reports/hundred_call_test_report.json

echo "Environment variables set"

# Function to run production test
run_production_test() {
    echo ""
    echo "=== RUNNING PRODUCTION TEST ==="
    echo "50 calls over 5 minutes (300 seconds)"
    echo "================================"
    
    # Set production test parameters
    export LOAD_TEST_CONCURRENT_CALLS=50
    export LOAD_TEST_DURATION_SECONDS=300
    export LOAD_TEST_CALL_DURATION_SECONDS=60
    export LOAD_TEST_REPORT=reports/prod_test_report.json
    
    # Run the test
    run_load_test
}

# Function to run 100 calls over 10 minutes test
run_hundred_call_test() {
    echo ""
    echo "=== RUNNING 100 CALLS OVER 10 MINUTES TEST ==="
    echo "100 calls over 10 minutes (600 seconds) with 30-minute call duration"
    echo "=================================================================="
    
    # Set test parameters
    export LOAD_TEST_CONCURRENT_CALLS=100
    export LOAD_TEST_DURATION_SECONDS=600
    export LOAD_TEST_CALL_DURATION_SECONDS=1800
    export LOAD_TEST_REPORT=reports/hundred_call_test_report.json
    
    # Run the test
    run_load_test
}

# Function to run load test
run_load_test() {
    echo "Starting load test..."
    echo "Concurrent calls: $LOAD_TEST_CONCURRENT_CALLS"
    echo "Test duration: $LOAD_TEST_DURATION_SECONDS seconds"
    echo "Call duration: $LOAD_TEST_CALL_DURATION_SECONDS seconds"

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
    echo "Running load test in background..."
    timeout $((LOAD_TEST_DURATION_SECONDS + 60)) ./bin/load-test > logs/load-test.log 2>&1 &
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
}

# Step 2: Build Components
echo ""
echo "Step 2: Building Components"
echo "=========================="

go build -o bin/ari-service ./cmd/ari-service
go build -o bin/echo-server ./cmd/echo
go build -o bin/load-test ./cmd/load_test

echo "All components built successfully"

# Step 3: Start ARI Service
echo ""
echo "Step 3: Starting ARI Service"
echo "==========================="

echo "Starting ARI service in background..."
./bin/ari-service > logs/ari-service.log 2>&1 &
ARI_PID=$!
echo "ARI service started with PID: $ARI_PID"

# Wait for service to initialize
sleep 3

# Check if ARI service is running
if ! kill -0 $ARI_PID 2>/dev/null; then
    echo "Error: ARI service failed to start"
    exit 1
fi

echo "ARI service is running"

# Step 4: Start Echo Server
echo ""
echo "Step 4: Starting Echo Server"
echo "==========================="

echo "Starting Echo server in background..."
./bin/echo-server > logs/echo-server.log 2>&1 &
ECHO_PID=$!
echo "Echo server started with PID: $ECHO_PID"

# Wait for server to initialize
sleep 2

# Check if Echo server is running
if ! kill -0 $ECHO_PID 2>/dev/null; then
    echo "Error: Echo server failed to start"
    kill $ARI_PID 2>/dev/null || true
    exit 1
fi

echo "Echo server is running"

# Step 5: Run Load Test
echo ""
echo "Step 5: Running Load Test"
echo "========================"

# Check if production test is requested
if [ "$1" = "prod" ]; then
    run_production_test
elif [ "$1" = "100call" ]; then
    run_hundred_call_test
else
    run_load_test
fi

# Step 6: Generate Report
echo ""
echo "Step 6: Generating Report"
echo "========================"

# Stop services
echo "Stopping services..."
kill $ECHO_PID 2>/dev/null || true
kill $ARI_PID 2>/dev/null || true

# Wait for services to stop
sleep 2

# Generate summary report
REPORT_FILE="reports/summary_report.txt"
echo "Generating summary report: $REPORT_FILE"

# Get final metrics
if curl -s http://localhost:9090/metrics > /tmp/final_metrics.json 2>/dev/null; then
    FINAL_METRICS_AVAILABLE=true
else
    FINAL_METRICS_AVAILABLE=false
fi

cat > "$REPORT_FILE" << EOF
ARI Service Load Test Summary Report
===================================
Generated: $(date)
Project: ARI Service with RTP Latency Measurement
Location: $PROJECT_ROOT

Test Configuration:
- Test Type: $(if [ "$1" = "prod" ]; then echo "PRODUCTION TEST"; else echo "REGULAR TEST"; fi)
- Concurrent Calls: $LOAD_TEST_CONCURRENT_CALLS
- Test Duration: $LOAD_TEST_DURATION_SECONDS seconds
- Call Duration: $LOAD_TEST_CALL_DURATION_SECONDS seconds
- Echo Server: $ECHO_HOST:$ECHO_PORT
- ARI Service: $BIND_IP:$PORT_RANGE

Test Result: $(if [ "$LOAD_TEST_SUCCESS" = true ]; then echo "SUCCESS"; else echo "FAILED"; fi)

Files Generated:
- ARI Service Log: logs/ari-service.log
- Echo Server Log: logs/echo-server.log
- Load Test Results: $LOAD_TEST_REPORT

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

To view detailed results:
- Load test: cat $LOAD_TEST_REPORT
- Service logs: cat logs/ari-service.log

To restart services:
- ARI Service: ./bin/ari-service
- Echo Server: ./bin/echo-server
- Load Test: ./bin/load-test

Health Check:
- ARI Service: curl http://localhost:9090/health
- Metrics: curl http://localhost:9090/metrics
EOF

echo ""
echo "=== EXECUTION COMPLETE ==="
echo "Summary report: $REPORT_FILE"
echo ""

if [ "$LOAD_TEST_SUCCESS" = true ]; then
    echo "✅ All steps completed successfully!"
    echo ""
    echo "Quick Results:"
    if [ -f "$LOAD_TEST_REPORT" ]; then
        echo "Success Rate: $(cat $LOAD_TEST_REPORT | grep -o '"success_rate":[^,]*' | cut -d':' -f2)%"
        echo "Average Latency: $(cat $LOAD_TEST_REPORT | grep -o '"average_latency_ms":[^,]*' | cut -d':' -f2) ms"
    fi
    echo ""
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
    echo "❌ Load test failed. Check logs for details."
    exit 1
fi

echo ""
echo "To view full results:"
echo "cat $REPORT_FILE"
echo "cat $LOAD_TEST_REPORT"