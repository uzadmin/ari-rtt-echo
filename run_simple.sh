#!/bin/bash

# Simple ARI Service Runner
# This script starts all components and runs the load test with specified parameters
#
# Usage:
#   ./run_simple.sh --count=30 --duration-ms=30000 --delay-between-ms=100
#   ./run_simple.sh --count=100 --duration-ms=30000 --delay-between-ms=100
#   ./run_simple.sh --count=150 --duration-ms=30000 --delay-between-ms=100

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

echo "=== Simple ARI Service Runner ==="
echo "Project root: $PROJECT_ROOT"

# Parse command line arguments
COUNT=30
DURATION_MS=30000
DELAY_BETWEEN_MS=100

for arg in "$@"; do
    case $arg in
        --count=*)
            COUNT="${arg#*=}"
            ;;
        --duration-ms=*)
            DURATION_MS="${arg#*=}"
            ;;
        --delay-between-ms=*)
            DELAY_BETWEEN_MS="${arg#*=}"
            ;;
        *)
            echo "Unknown argument: $arg"
            echo "Usage: $0 [--count=N] [--duration-ms=N] [--delay-between-ms=N]"
            exit 1
            ;;
    esac
done

echo ""
echo "Configuration:"
echo "  Count: $COUNT"
echo "  Duration: $DURATION_MS ms"
echo "  Delay between calls: $DELAY_BETWEEN_MS ms"

# Step 1: Environment Setup
echo ""
echo "Step 1: Environment Setup"
echo "========================"

# Load environment variables from .env file
if [ -f ".env" ]; then
    echo "Loading environment variables from .env file"
    source .env
else
    echo "No .env file found, using defaults"
fi

# Create necessary directories
mkdir -p bin logs reports

# Export environment variables with fallbacks
export ARI_URL=${ASTERISK_HOST:-localhost}:${ASTERISK_PORT:-8088}
export ARI_USER=${ASTERISK_USERNAME:-ari}
export ARI_PASS=${ASTERISK_PASSWORD:-ari}
export APP_NAME=${ASTERISK_APP_NAME:-ari-app}
export BIND_IP=${BIND_IP:-0.0.0.0}
export PORT_RANGE=${PORT_RANGE:-21000-31000}
export ECHO_HOST=${ECHO_HOST:-127.0.0.1}
export ECHO_PORT=${ECHO_PORT:-4000}
export METRICS_INTERVAL_SEC=${METRICS_INTERVAL_SEC:-5}

echo "Environment variables set"
echo "  ARI_URL: $ARI_URL"
echo "  ARI_USER: $ARI_USER"
echo "  APP_NAME: $APP_NAME"
echo "  BIND_IP: $BIND_IP"
echo "  PORT_RANGE: $PORT_RANGE"
echo "  ECHO_HOST: $ECHO_HOST"
echo "  ECHO_PORT: $ECHO_PORT"

# Step 2: Build Components
echo ""
echo "Step 2: Building Components"
echo "=========================="

go build -o bin/ari-service ./cmd/ari-service
go build -o bin/echo-server ./cmd/echo
go build -o bin/load-test-new ./cmd/load_test_new

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

echo "Starting load test with count=$COUNT, duration=$DURATION_MS ms, delay=$DELAY_BETWEEN_MS ms..."

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
timeout $((DURATION_MS/1000 + 60)) ./bin/load-test-new \
    --count=$COUNT \
    --duration-ms=$DURATION_MS \
    --delay-between-ms=$DELAY_BETWEEN_MS > logs/load-test-new.log 2>&1 &
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
REPORT_FILE="reports/simple_summary_report_$(date +%Y%m%d_%H%M%S).txt"
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
- Count: $COUNT
- Duration: $DURATION_MS ms
- Delay Between Calls: $DELAY_BETWEEN_MS ms
- Echo Server: $ECHO_HOST:$ECHO_PORT
- ARI Service: $BIND_IP:$PORT_RANGE

Test Result: $(if [ "$LOAD_TEST_SUCCESS" = true ]; then echo "SUCCESS"; else echo "FAILED"; fi)

Files Generated:
- ARI Service Log: logs/ari-service.log
- Echo Server Log: logs/echo-server.log
- Load Test Results: reports/load_test_new_report.json

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
- Load test: cat reports/load_test_new_report.json
- Service logs: cat logs/ari-service.log

To restart services:
- ARI Service: ./bin/ari-service
- Echo Server: ./bin/echo-server
- Load Test: ./bin/load-test-new

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
echo "cat reports/load_test_new_report.json"