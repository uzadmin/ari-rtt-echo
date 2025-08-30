#!/bin/bash

# Script to analyze test results for zombie channels and unclosed ports

echo "=== Analyzing Test Results ==="

# Check if logs directory exists
if [ ! -d "logs" ]; then
    echo "❌ Logs directory not found"
    exit 1
fi

echo "Checking for zombie channels..."
echo "=============================="

# Look for zombie channel warnings in monitoring log
if [ -f "logs/monitoring.log" ]; then
    zombie_warnings=$(grep -c "WARNING: Possible zombie channels" logs/monitoring.log)
    if [ "$zombie_warnings" -gt 0 ]; then
        echo "⚠️  Found $zombie_warnings zombie channel warnings"
        echo "Details:"
        grep "WARNING: Possible zombie channels" logs/monitoring.log
    else
        echo "✅ No zombie channel warnings found"
    fi
else
    echo "⚠️  Monitoring log not found"
fi

echo ""
echo "Checking for unclosed ports..."
echo "============================="

# Look for unclosed port warnings in monitoring log
if [ -f "logs/monitoring.log" ]; then
    port_warnings=$(grep -c "WARNING: Found.*unclosed ports" logs/monitoring.log)
    if [ "$port_warnings" -gt 0 ]; then
        echo "⚠️  Found $port_warnings unclosed port warnings"
        echo "Details:"
        grep "WARNING: Found.*unclosed ports" logs/monitoring.log
    else
        echo "✅ No unclosed port warnings found"
    fi
else
    echo "⚠️  Monitoring log not found"
fi

echo ""
echo "Checking ARI service logs..."
echo "==========================="

if [ -f "logs/ari-service.log" ]; then
    # Look for port allocation errors
    port_errors=$(grep -c "No ports available" logs/ari-service.log)
    if [ "$port_errors" -gt 0 ]; then
        echo "❌ Found $port_errors port allocation errors"
        echo "Details:"
        grep "No ports available" logs/ari-service.log
    else
        echo "✅ No port allocation errors found"
    fi
    
    # Look for channel cleanup messages
    cleanup_count=$(grep -c "Cleaned up channel" logs/ari-service.log)
    echo "✅ $cleanup_count channels cleaned up properly"
    
    # Look for zombie cleanup messages
    zombie_cleanup=$(grep -c "zombie channel" logs/ari-service.log)
    if [ "$zombie_cleanup" -gt 0 ]; then
        echo "⚠️  Found $zombie_cleanup zombie channel cleanups"
        grep "zombie channel" logs/ari-service.log
    fi
else
    echo "⚠️  ARI service log not found"
fi

echo ""
echo "Checking current port usage..."
echo "============================="

# Check current port usage in our range
current_ports=$(lsof -i :21000-31000 2>/dev/null | grep -c "UDP")
echo "Currently used ports in range 21000-31000: $current_ports"

if [ "$current_ports" -gt 0 ]; then
    echo "Active ports:"
    lsof -i :21000-31000 2>/dev/null | grep "UDP" | awk '{print $2, $9}' | while read pid port; do
        process_name=$(ps -p $pid -o comm= 2>/dev/null || echo "unknown")
        echo "  $port (PID: $pid, Process: $process_name)"
    done
fi

echo ""
echo "Checking load test results..."
echo "============================"

if [ -f "logs/load-test.log" ]; then
    # Check for errors in load test
    load_test_errors=$(grep -c "Error\|Failed\|failed" logs/load-test.log)
    if [ "$load_test_errors" -gt 0 ]; then
        echo "⚠️  Found $load_test_errors errors in load test"
        echo "Sample errors:"
        grep -i "Error\|Failed\|failed" logs/load-test.log | head -5
    else
        echo "✅ No errors found in load test"
    fi
else
    echo "⚠️  Load test log not found"
fi

echo ""
echo "=== Analysis Complete ==="