# Enhanced Testing Plan for Asterisk RTP Latency Measurement System

## Overview

This document outlines the enhanced testing capabilities implemented for the Asterisk RTP Latency Measurement system, including longer-duration tests, load testing, packet reordering detection, and Docker-based testing.

## Test Components

### 1. Enhanced Load Testing (`cmd/enhanced_load_test`)

#### Features
- **Longer Duration Tests**: Extended test duration (120 seconds vs 60 seconds)
- **Extended Call Duration**: Longer individual call duration (60 seconds vs 30 seconds)
- **Concurrent Call Support**: Configurable concurrent calls (default 5)
- **Periodic Metrics Collection**: Real-time metrics monitoring during tests
- **Enhanced Reporting**: Detailed JSON reports with latency metrics

#### Configuration Options
```bash
-ari-url string
    ARI server URL (default "localhost:8088")
-ari-user string
    ARI username (default "ari")
-ari-pass string
    ARI password (default "ari")
-app-name string
    ARI application name (default "ari-app")
-call-duration int
    Call duration in seconds (longer) (default 60)
-concurrent int
    Number of concurrent calls (default 5)
-context string
    Dialplan context (default "ari-context")
-duration int
    Test duration in seconds (enhanced) (default 120)
-endpoint string
    Endpoint to call (default "Local/echo@ari-context")
-extension string
    Dialplan extension (default "echo")
-metrics-interval int
    Metrics collection interval in seconds (default 10)
-packet-reordering
    Enable packet reordering detection (default true)
-priority int
    Dialplan priority (default 1)
-report-file string
    Report file path (default "reports/enhanced_load_test_report.json")
```

### 2. Packet Reordering Detection (`cmd/packet_reordering_test`)

#### Features
- **Sequence Number Monitoring**: Tracks RTP packet sequence numbers
- **Reordering Detection Algorithm**: Detects out-of-order packet arrival
- **Wraparound Handling**: Properly handles 16-bit sequence number wraparound
- **Statistics Collection**: Provides reordering statistics and ratios

#### Detection Logic
1. Monitors sequence number progression
2. Identifies gaps and out-of-order arrivals
3. Handles sequence number wraparound (0xFFFF → 0x0000)
4. Calculates reordering ratio and statistics

### 3. Docker Test Runner (`run_docker_tests.sh`)

#### Features
- **Automated Service Management**: Starts/stops Docker services as needed
- **Integrated Test Execution**: Runs all tests in Docker environment
- **Metrics Collection**: Gathers and displays system metrics
- **Error Handling**: Graceful error handling and reporting

#### Execution Flow
1. Check if services are running, start if needed
2. Run integration tests
3. Run packet reordering tests
4. Run enhanced load tests
5. Collect and display final metrics

## Test Execution

### Running Enhanced Load Test
```bash
# Build and run enhanced load test
go build -o enhanced_load_test cmd/enhanced_load_test/main.go cmd/enhanced_load_test/ari_client.go
./enhanced_load_test -concurrent=5 -duration=120 -call-duration=60

# Or using Docker
docker-compose exec asterisk go run cmd/enhanced_load_test/main.go
```

### Running Packet Reordering Test
```bash
# Build and run packet reordering test
go build -o packet_reordering_test cmd/packet_reordering_test/main.go
./packet_reordering_test

# Or using Docker
docker-compose exec asterisk go run cmd/packet_reordering_test/main.go
```

### Running Docker Test Suite
```bash
# Make script executable
chmod +x run_docker_tests.sh

# Run all tests
./run_docker_tests.sh
```

## Metrics Collection

### Enhanced Load Test Metrics
- **Call Success Rate**: Percentage of successful calls
- **Calls Per Second**: Throughput measurement
- **Latency Metrics**: p50, p95, p99, max, average
- **Packet Loss Ratio**: Percentage of lost packets
- **Late Packet Ratio**: Percentage of late packets
- **Packet Reordering**: Number of reordered packets detected

### Real-time Monitoring
- Periodic metrics collection during test execution
- Live logging of key performance indicators
- Immediate error detection and reporting

## Test Scenarios

### Scenario 1: Baseline Performance Test
- **Concurrent Calls**: 5
- **Test Duration**: 120 seconds
- **Call Duration**: 60 seconds
- **Purpose**: Establish baseline performance metrics

### Scenario 2: Stress Test
- **Concurrent Calls**: 10
- **Test Duration**: 180 seconds
- **Call Duration**: 90 seconds
- **Purpose**: Test system under higher load

### Scenario 3: Long Duration Test
- **Concurrent Calls**: 3
- **Test Duration**: 300 seconds
- **Call Duration**: 150 seconds
- **Purpose**: Test system stability over extended periods

## Reporting

### JSON Report Format
```json
{
  "start_time": "2025-08-30T10:00:00Z",
  "end_time": "2025-08-30T10:02:00Z",
  "duration_seconds": 120.0,
  "concurrent_calls": 5,
  "total_calls": 25,
  "successful_calls": 23,
  "failed_calls": 2,
  "success_rate": 92.0,
  "calls_per_second": 0.21,
  "call_details": [...],
  "final_metrics": {
    "p50_latency_ms": 15.2,
    "p95_latency_ms": 28.7,
    "p99_latency_ms": 35.1,
    "max_latency_ms": 42.3,
    "avg_latency_ms": 18.4,
    "late_ratio": 0.02,
    "packet_loss_ratio": 0.01
  },
  "packet_reordering_detected": 1
}
```

## Docker Integration

### Container Requirements
- **Asterisk**: Running with ARI enabled
- **ARI Service**: Listening on port 9090
- **Echo Server**: Listening on port 4000
- **SIP Service**: Listening on port 5060
- **Port Range**: 10000-10100 for RTP traffic

### Volume Mounts
- `/app/reports`: Test reports directory
- `/app/logs`: Application logs directory
- `/etc/asterisk`: Asterisk configuration
- `/var/log/asterisk`: Asterisk logs

## Future Enhancements

### Planned Improvements
1. **Advanced Load Patterns**: Variable call rates and patterns
2. **Network Condition Simulation**: Introduce artificial latency/packet loss
3. **Real-time Dashboard**: Web-based metrics visualization
4. **Automated Alerting**: SLA violation notifications
5. **Distributed Testing**: Multi-node test execution

### Integration Opportunities
1. **CI/CD Pipeline**: Automated testing in deployment pipeline
2. **Prometheus Export**: Metrics export for monitoring systems
3. **Grafana Dashboard**: Visual metrics display
4. **Kubernetes Deployment**: Container orchestration support

## Conclusion

The enhanced testing framework provides comprehensive testing capabilities for the Asterisk RTP Latency Measurement system, enabling thorough validation of system performance, reliability, and scalability under various load conditions.