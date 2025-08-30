#!/bin/bash

# Docker Test Runner for Enhanced Load Testing
# This script runs all enhanced tests within the Docker environment

echo "=========================================="
echo "Docker Test Runner for Enhanced Load Testing"
echo "=========================================="

# Check if we're in the right directory
if [ ! -f "docker-compose.yml" ]; then
    echo "Error: docker-compose.yml not found. Please run this script from the project root."
    exit 1
fi

# Function to check if services are running
check_services() {
    echo "Checking if required services are running..."
    if docker-compose ps | grep -q "asterisk-ari"; then
        echo "✓ Services are running"
        return 0
    else
        echo "✗ Services are not running. Starting them now..."
        docker-compose up -d
        echo "Waiting for services to start..."
        sleep 10
        return 1
    fi
}

# Function to run enhanced load test
run_enhanced_load_test() {
    echo "=========================================="
    echo "Running Enhanced Load Test"
    echo "=========================================="
    
    # Build the test binary
    echo "Building enhanced load test..."
    docker-compose exec asterisk go build -o /tmp/enhanced_load_test cmd/enhanced_load_test/main.go cmd/enhanced_load_test/ari_client.go
    
    if [ $? -ne 0 ]; then
        echo "Failed to build enhanced load test"
        return 1
    fi
    
    echo "Running enhanced load test (5 concurrent calls, 120 seconds)..."
    docker-compose exec asterisk /tmp/enhanced_load_test \
        -concurrent=5 \
        -duration=120 \
        -call-duration=60 \
        -report-file=reports/docker_enhanced_load_test_report.json
    
    if [ $? -eq 0 ]; then
        echo "✓ Enhanced load test completed successfully"
        echo "Report saved to reports/docker_enhanced_load_test_report.json"
    else
        echo "✗ Enhanced load test failed"
        return 1
    fi
}

# Function to run packet reordering test
run_packet_reordering_test() {
    echo "=========================================="
    echo "Running Packet Reordering Test"
    echo "=========================================="
    
    # Build the test binary
    echo "Building packet reordering test..."
    docker-compose exec asterisk go build -o /tmp/packet_reordering_test cmd/packet_reordering_test/main.go
    
    if [ $? -ne 0 ]; then
        echo "Failed to build packet reordering test"
        return 1
    fi
    
    echo "Running packet reordering test..."
    docker-compose exec asterisk /tmp/packet_reordering_test
    
    if [ $? -eq 0 ]; then
        echo "✓ Packet reordering test completed successfully"
    else
        echo "✗ Packet reordering test failed"
        return 1
    fi
}

# Function to run integration tests
run_integration_tests() {
    echo "=========================================="
    echo "Running Integration Tests"
    echo "=========================================="
    
    echo "Running existing integration test..."
    docker-compose exec asterisk go run cmd/integration_test/main.go
    
    if [ $? -eq 0 ]; then
        echo "✓ Integration test completed successfully"
    else
        echo "✗ Integration test failed"
        return 1
    fi
}

# Function to collect and display metrics
show_metrics() {
    echo "=========================================="
    echo "Collecting System Metrics"
    echo "=========================================="
    
    echo "Current system metrics:"
    docker-compose exec asterisk curl -s http://localhost:9090/metrics | jq '.'
    
    echo ""
    echo "Echo server logs (last 10 lines):"
    docker-compose exec asterisk tail -10 /var/log/asterisk/full.log | grep "Echo Server"
}

# Main execution
main() {
    # Check and start services if needed
    check_services
    
    # Run all tests
    echo "Starting test execution..."
    
    # Run integration tests first
    run_integration_tests
    if [ $? -ne 0 ]; then
        echo "Integration tests failed. Stopping execution."
        exit 1
    fi
    
    # Run packet reordering test
    run_packet_reordering_test
    if [ $? -ne 0 ]; then
        echo "Packet reordering test failed. Continuing with other tests."
    fi
    
    # Run enhanced load test
    run_enhanced_load_test
    if [ $? -ne 0 ]; then
        echo "Enhanced load test failed."
    fi
    
    # Show final metrics
    show_metrics
    
    echo "=========================================="
    echo "All tests completed!"
    echo "=========================================="
}

# Run main function
main