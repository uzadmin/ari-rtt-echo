#!/bin/bash

# Docker-based ARI Service Test
# This script runs the complete system in Docker

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

echo "=== Docker-based ARI Service Test ==="
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

# Step 1: Build Docker image
echo ""
echo "Step 1: Building Docker image"
echo "============================"
docker build -t ari-service:latest .

# Step 2: Start Docker services
echo ""
echo "Step 2: Starting Docker services"
echo "==============================="
docker-compose up -d

# Wait for services to start
echo "Waiting for services to initialize..."
sleep 10

# Step 3: Check if services are running
echo ""
echo "Step 3: Checking service status"
echo "=============================="
echo "Docker containers:"
docker ps | grep asterisk-ari

echo ""
echo "Checking Asterisk status:"
docker exec asterisk-ari asterisk -rx "core show version" || echo "Asterisk not ready yet"

echo ""
echo "Checking ARI service status:"
curl -s http://localhost:9090/health || echo "ARI service not ready yet"

# Step 4: Run load test inside Docker
echo ""
echo "Step 4: Running load test"
echo "========================"
echo "Starting load test with count=$COUNT, duration=$DURATION_MS ms, delay=$DELAY_BETWEEN_MS ms..."

# Run load test inside the container
docker exec asterisk-ari /app/bin/load-test-new \
    --count=$COUNT \
    --duration-ms=$DURATION_MS \
    --delay-between-ms=$DELAY_BETWEEN_MS

# Step 5: Monitor metrics
echo ""
echo "Step 5: Monitoring metrics"
echo "========================="
for i in {1..10}; do
    echo "Metrics check #$i:"
    curl -s http://localhost:9090/metrics | jq '.'
    sleep 3
done

# Step 6: Clean up
echo ""
echo "Step 6: Cleaning up"
echo "=================="
echo "Stopping services..."
docker-compose down

echo ""
echo "✅ Docker test completed!"