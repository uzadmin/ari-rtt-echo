#!/bin/bash

# Script to clean up all processes and reset the environment

echo "=== Cleaning Up Environment ==="

# Kill any running processes
echo "Killing running processes..."
pkill -f "ari-service" 2>/dev/null || true
pkill -f "echo-server" 2>/dev/null || true
pkill -f "load-test" 2>/dev/null || true
pkill -f "find_zombies_and_ports" 2>/dev/null || true
pkill -f "run_extended_test" 2>/dev/null || true
pkill -f "monitor_resources" 2>/dev/null || true

# Wait a moment for processes to terminate
echo "Waiting for processes to terminate..."
sleep 3

# Check if any processes are still running
echo "Checking for remaining processes..."
remaining_ari=$(ps aux | grep "ari-service" | grep -v grep | wc -l)
remaining_echo=$(ps aux | grep "echo-server" | grep -v grep | wc -l)
remaining_load=$(ps aux | grep "load-test" | grep -v grep | wc -l)

if [ "$remaining_ari" -gt 0 ] || [ "$remaining_echo" -gt 0 ] || [ "$remaining_load" -gt 0 ]; then
    echo "⚠️  Found remaining processes, force killing..."
    pkill -9 -f "ari-service" 2>/dev/null || true
    pkill -9 -f "echo-server" 2>/dev/null || true
    pkill -9 -f "load-test" 2>/dev/null || true
    pkill -9 -f "find_zombies_and_ports" 2>/dev/null || true
    pkill -9 -f "run_extended_test" 2>/dev/null || true
    pkill -9 -f "monitor_resources" 2>/dev/null || true
    sleep 2
fi

# Check port usage
echo "Checking port usage..."
ports_in_use=$(lsof -i :21000-31000 2>/dev/null | grep -c "UDP")
if [ "$ports_in_use" -gt 0 ]; then
    echo "⚠️  Found $ports_in_use ports still in use, force closing..."
    lsof -t -i :21000-31000 2>/dev/null | xargs kill -9 2>/dev/null || true
    sleep 2
fi

# Clean up log files
echo "Cleaning up log files..."
rm -f logs/*.log
rm -f /tmp/metrics.json
rm -f /tmp/final_metrics.json
rm -f /tmp/metrics2.json

# Clean up report files
echo "Cleaning up report files..."
rm -f reports/*.json
rm -f reports/extended_test_summary_*.txt

# Clean up temporary files
echo "Cleaning up temporary files..."
rm -f /tmp/*.json

echo "✅ Environment cleaned up successfully!"

# Show current status
echo ""
echo "Current status:"
echo "==============="
echo "Running ARI service processes: $(ps aux | grep "ari-service" | grep -v grep | wc -l)"
echo "Running echo server processes: $(ps aux | grep "echo-server" | grep -v grep | wc -l)"
echo "Running load test processes: $(ps aux | grep "load-test" | grep -v grep | wc -l)"
echo "Ports in use (21000-31000): $(lsof -i :21000-31000 2>/dev/null | grep -c "UDP")"

echo ""
echo "You can now safely run your extended test."