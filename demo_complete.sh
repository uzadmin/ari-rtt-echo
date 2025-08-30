#!/bin/bash

# Complete System Demo Script
# This script demonstrates the complete ARI service with all components

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

echo "=== Complete ARI Service Demo ==="
echo "Project root: $PROJECT_ROOT"

# Step 1: Clean up any existing processes
echo ""
echo "Step 1: Cleaning up existing processes"
echo "====================================="
pkill -f ari-service 2>/dev/null || true
pkill -f echo-server 2>/dev/null || true
sleep 2

# Step 2: Build all components
echo ""
echo "Step 2: Building all components"
echo "==============================="
go build -o bin/ari-service ./cmd/ari-service
go build -o bin/echo-server ./cmd/echo
go build -o bin/load-test-new ./cmd/load_test_new

echo "✅ All components built successfully"

# Step 3: Show component help
echo ""
echo "Step 3: Component information"
echo "============================"
echo "ARI Service help:"
./bin/ari-service --help | head -10

echo ""
echo "Echo Server help:"
./bin/echo-server --help

echo ""
echo "Load Test help:"
./bin/load-test-new --help | head -10

# Step 4: Demonstrate simple run
echo ""
echo "Step 4: Demonstrating simple run (10 calls for 10 seconds)"
echo "========================================================"
timeout 30s ./run_simple.sh --count=10 --duration-ms=10000 --delay-between-ms=100 || true

# Step 5: Show generated files
echo ""
echo "Step 5: Generated files"
echo "======================"
echo "Binaries:"
ls -la bin/

echo ""
echo "Logs (if any):"
ls -la logs/ 2>/dev/null || echo "No logs directory"

echo ""
echo "Reports (if any):"
ls -la reports/ 2>/dev/null || echo "No reports directory"

# Step 6: Clean up
echo ""
echo "Step 6: Cleaning up"
echo "=================="
pkill -f ari-service 2>/dev/null || true
pkill -f echo-server 2>/dev/null || true

echo ""
echo "✅ Demo completed successfully!"
echo ""
echo "To run full SLA tests:"
echo "  ./test_sla.sh"
echo ""
echo "To run custom load tests:"
echo "  ./run_simple.sh --count=N --duration-ms=N --delay-between-ms=N"
echo ""
echo "Documentation is available in:"
echo "  README_SIMPLE.md"
echo "  FINAL_IMPLEMENTATION_SUMMARY.md"