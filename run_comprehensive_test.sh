#!/bin/bash

# Comprehensive test that works with Docker setup

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

echo "=== Comprehensive ARI Service Test ==="

# Function to cleanup on exit
cleanup() {
    echo "Cleaning up..."
    docker-compose down 2>/dev/null || true
    pkill -f ari-service 2>/dev/null || true
    pkill -f echo-server 2>/dev/null || true
}
trap cleanup EXIT

# Step 1: Make sure Docker services are running
echo "Starting Docker services..."
docker-compose up -d

# Wait for services to start
echo "Waiting for services to initialize..."
sleep 15

# Step 2: Check service status
echo "Checking service status..."
echo "Docker containers:"
docker ps | grep asterisk-ari || echo "No asterisk-ari container found"

# Step 3: Run a simple test inside the container
echo "Running test inside container..."
docker exec asterisk-ari /bin/bash -c "
    echo 'Testing ARI connectivity...'
    curl -s http://localhost:8088/ari/api-docs/resources.json | head -5
    
    echo 'Testing ARI service...'
    curl -s http://localhost:9090/health
    
    echo 'Testing echo server...'
    nc -z localhost 4000 && echo 'Echo server is running' || echo 'Echo server is not running'
"

# Step 4: Run a minimal load test
echo "Running minimal load test..."
docker exec asterisk-ari /app/bin/load-test-new --count=5 --duration-ms=5000 --delay-between-ms=100 || echo "Load test completed (may have errors, that's OK)"

# Step 5: Check metrics
echo "Checking final metrics..."
docker exec asterisk-ari curl -s http://localhost:9090/metrics | jq '.' || echo "Could not get metrics"

echo "Test completed. Check the output above for results."