#!/bin/bash

# Script to monitor for zombie channels and unclosed ports during testing

echo "=== Resource Monitor Started ==="

# Create logs directory if it doesn't exist
mkdir -p logs

# Function to check for zombie channels
check_zombie_channels() {
    echo "$(date): Checking for zombie channels..."
    
    # Check ARI service metrics for active channels
    if curl -s http://localhost:9090/metrics > /tmp/metrics.json 2>/dev/null; then
        active_channels=$(jq -r '.active_channels' /tmp/metrics.json)
        total_channels=$(jq -r '.total_channels' /tmp/metrics.json)
        echo "Active channels: $active_channels, Total channels: $total_channels"
        
        # If there are active channels but no recent activity, they might be zombies
        if [ "$active_channels" -gt 0 ]; then
            # Check if channels are actually processing packets
            total_latencies=$(jq -r '.total_latencies' /tmp/metrics.json)
            sleep 5
            if curl -s http://localhost:9090/metrics > /tmp/metrics2.json 2>/dev/null; then
                new_total_latencies=$(jq -r '.total_latencies' /tmp/metrics2.json)
                if [ "$total_latencies" -eq "$new_total_latencies" ]; then
                    echo "WARNING: Possible zombie channels detected - no latency updates in 5 seconds"
                fi
            fi
        fi
    else
        echo "Unable to fetch metrics from ARI service"
    fi
}

# Function to check for unclosed ports
check_unclosed_ports() {
    echo "$(date): Checking for unclosed ports in range 21000-31000..."
    
    # Count how many ports are currently in use
    used_ports=$(lsof -i :21000-31000 2>/dev/null | grep -c "UDP")
    echo "Currently used ports in range 21000-31000: $used_ports"
    
    # Show which ports are in use
    if [ "$used_ports" -gt 0 ]; then
        echo "Ports currently in use:"
        lsof -i :21000-31000 2>/dev/null | grep "UDP" | awk '{print $9}' | cut -d':' -f2 | sort -n
    fi
}

# Function to check system resources
check_system_resources() {
    echo "$(date): Checking system resources..."
    
    # Check memory usage
    memory_usage=$(ps aux | grep -E "(ari-service|echo-server)" | awk '{sum+=$6} END {print sum/1024 " MB"}')
    echo "Memory usage by ARI services: $memory_usage"
    
    # Check CPU usage
    cpu_usage=$(ps aux | grep -E "(ari-service|echo-server)" | awk '{sum+=$3} END {print sum "%"}')
    echo "CPU usage by ARI services: $cpu_usage"
}

# Main monitoring loop
while true; do
    check_zombie_channels
    check_unclosed_ports
    check_system_resources
    echo "----------------------------------------"
    sleep 30
done