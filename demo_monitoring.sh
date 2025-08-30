#!/bin/bash

# Script to demonstrate monitoring capabilities

echo "=== Monitoring Demo ==="

# Clean up any existing processes
./cleanup.sh > /dev/null 2>&1

# Build components
echo "Building components..."
go build -o bin/ari-service ./cmd/ari-service
go build -o bin/echo-server ./cmd/echo
go build -o bin/find_zombies_and_ports find_zombies_and_ports.go

echo "✅ Components built successfully"

# Start services
echo "Starting services..."
./bin/echo-server > logs/echo-server.log 2>&1 &
ECHO_PID=$!

./bin/ari-service > logs/ari-service.log 2>&1 &
ARI_PID=$!

# Wait for services to start
sleep 3

# Verify services are running
if ! kill -0 $ECHO_PID 2>/dev/null || ! kill -0 $ARI_PID 2>/dev/null; then
    echo "❌ Failed to start services"
    exit 1
fi

echo "✅ Services started successfully"

# Start monitoring in background
echo "Starting monitoring..."
./bin/find_zombies_and_ports > logs/monitoring.log 2>&1 &
MONITOR_PID=$!

echo "Monitoring started. Let's simulate some activity..."

# Simulate activity by making requests to the ARI service
for i in {1..10}; do
    echo "Making request $i..."
    curl -s http://localhost:9090/health > /dev/null
    curl -s http://localhost:9090/metrics > /dev/null
    sleep 2
done

echo "Activity simulation complete."

# Check monitoring output
echo ""
echo "=== Monitoring Results ==="
echo "Last 10 lines of monitoring output:"
tail -10 logs/monitoring.log

# Clean up
echo ""
echo "Cleaning up..."
kill $ECHO_PID $ARI_PID $MONITOR_PID 2>/dev/null || true
wait $ECHO_PID $ARI_PID $MONITOR_PID 2>/dev/null || true

echo "✅ Demo completed successfully"

echo ""
echo "This demo shows how the monitoring system works:"
echo "1. It tracks active channels and latencies"
echo "2. It detects zombie channels (channels with no new latencies)"
echo "3. It monitors port usage in the range 21000-31000"
echo "4. It reports warnings when issues are detected"