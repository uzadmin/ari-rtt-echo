#!/bin/bash

# Test script to originate calls and check metrics

set -e

echo "=== Testing Call Origination ==="

# Check if services are accessible
echo "Checking service connectivity..."
curl -s http://localhost:9090/health && echo " - ARI service OK"
curl -s http://localhost:8088/ari/api-docs/resources.json | head -1 && echo " - Asterisk ARI OK"

# Try to originate a single test call using the load test
echo "Originating test calls..."
docker exec asterisk-ari /app/bin/load-test-new --count=3 --duration-ms=10000 --delay-between-ms=100

# Wait a moment for calls to process
sleep 3

# Check metrics
echo "Checking metrics after calls..."
curl -s http://localhost:9090/metrics | jq '.'

# Check Asterisk channels
echo "Checking Asterisk channels..."
docker exec asterisk-ari asterisk -rx "core show channels"

echo "Test completed."