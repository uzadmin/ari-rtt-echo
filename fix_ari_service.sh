#!/bin/bash

# Script to fix ARI service port binding issues and enable RTT metrics collection

echo "=== Fixing ARI Service Port Binding Issues ==="

# Stop any running ARI service instances
echo "Stopping existing ARI service instances..."
docker-compose exec asterisk pkill -f ari-service 2>/dev/null || true

# Wait for services to stop
sleep 5

# Restart the container to get a clean start
echo "Restarting Docker container..."
docker-compose restart

# Wait for container to restart
sleep 10

# Check if services are running properly
echo "Checking service status..."
docker-compose exec asterisk ps aux | grep -E "(asterisk|ari|echo)"

# Check port usage
echo "Checking port usage..."
docker-compose exec asterisk netstat -tulpn | grep -E "(8088|9090|4000)"

# Test metrics endpoint
echo "Testing metrics endpoint..."
curl -s http://localhost:9090/metrics | jq '.'

echo ""
echo "=== ARI Service Fix Complete ==="
echo "The ARI service should now be running without port conflicts."
echo "You can now run the production test with RTT metrics collection enabled."

# Instructions for running the test
echo ""
echo "To run the production test with RTT metrics:"
echo "  docker-compose exec asterisk /app/bin/load-test -concurrent=50 -duration=300 -call-duration=60 -report-file=reports/prod_test_with_rtt.json"
echo ""
echo "To monitor RTT metrics during the test:"
echo "  watch -n 5 'curl -s http://localhost:9090/metrics | jq \"{p50: .p50_latency, p95: .p95_latency, p99: .p99_latency, max: .max_latency, active_channels: .active_channels}\"'"