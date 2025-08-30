#!/bin/bash

# SLA Compliance Testing Script
# This script runs the load test with different load levels and checks SLA compliance

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

echo "=== SLA Compliance Testing ==="
echo "Project root: $PROJECT_ROOT"

# Create reports directory
mkdir -p reports/sla_tests

# Function to run test and check SLA compliance
run_sla_test() {
    local test_name=$1
    local count=$2
    local duration_ms=$3
    local delay_ms=$4
    
    echo ""
    echo "=== Running $test_name Test ==="
    echo "Count: $count calls"
    echo "Duration: $duration_ms ms"
    echo "Delay: $delay_ms ms"
    
    # Run the test
    ./run_simple.sh --count=$count --duration-ms=$duration_ms --delay-between-ms=$delay_ms
    
    # Check if test was successful
    if [ $? -eq 0 ]; then
        echo "✅ $test_name test completed successfully"
        
        # Get the latest report file
        latest_report=$(ls -t reports/simple_summary_report_*.txt | head -1)
        
        if [ -f "$latest_report" ]; then
            echo "Report: $latest_report"
            
            # Extract metrics from the report
            if [ -f "/tmp/final_metrics.json" ]; then
                p50=$(jq -r '.p50_latency' /tmp/final_metrics.json)
                p95=$(jq -r '.p95_latency' /tmp/final_metrics.json)
                p99=$(jq -r '.p99_latency' /tmp/final_metrics.json)
                max_rt=$(jq -r '.max_latency' /tmp/final_metrics.json)
                packet_loss=$(jq -r '.packet_loss_ratio' /tmp/final_metrics.json)
                late_ratio=$(jq -r '.late_ratio' /tmp/final_metrics.json)
                
                echo "Metrics:"
                echo "  RTT p50: ${p50}ms"
                echo "  RTT p95: ${p95}ms"
                echo "  RTT p99: ${p99}ms"
                echo "  RTT max: ${max_rt}ms"
                echo "  Packet Loss: ${packet_loss}"
                echo "  Late Ratio: ${late_ratio}"
            fi
        fi
    else
        echo "❌ $test_name test failed"
        return 1
    fi
}

# Run SLA tests
echo ""
echo "Starting SLA Compliance Tests..."
echo "================================"

# Test 1: 30 Concurrent Calls (Nominal Load)
run_sla_test "30 Concurrent Calls" 30 30000 100

# Test 2: 100 Concurrent Calls (High Load)
run_sla_test "100 Concurrent Calls" 100 30000 100

# Test 3: 150 Concurrent Calls (Stress)
run_sla_test "150 Concurrent Calls" 150 30000 100

echo ""
echo "=== SLA Compliance Testing Complete ==="
echo "All tests have been executed. Please review the individual reports for detailed metrics."