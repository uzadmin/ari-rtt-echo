#!/bin/bash

# Simple connectivity test

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

echo "=== Connectivity Test ==="

# Check if we can connect to Asterisk
echo "Checking Asterisk connection..."
if nc -z localhost 8088; then
    echo "✅ Asterisk is accessible on port 8088"
else
    echo "❌ Asterisk is not accessible on port 8088"
fi

# Check if we can connect to ARI service
echo "Checking ARI service connection..."
if nc -z localhost 9090; then
    echo "✅ ARI service is accessible on port 9090"
else
    echo "❌ ARI service is not accessible on port 9090"
fi

# Check if we can connect to echo server
echo "Checking echo server connection..."
if nc -z localhost 4000; then
    echo "✅ Echo server is accessible on port 4000"
else
    echo "❌ Echo server is not accessible on port 4000"
fi

# Try to get metrics
echo "Checking metrics endpoint..."
if curl -s http://localhost:9090/metrics > /dev/null; then
    echo "✅ Metrics endpoint is accessible"
else
    echo "❌ Metrics endpoint is not accessible"
fi

echo "Test completed."