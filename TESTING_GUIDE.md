# Asterisk RTP Latency Measurement System - Testing Guide

## Overview
This guide provides instructions for running various tests on the Asterisk RTP Latency Measurement system, including production tests with 50 concurrent calls over 5 minutes.

## Prerequisites
- Docker and Docker Compose installed
- jq tool for JSON processing (optional but recommended)

## Quick Start

### 1. Start the System
```bash
# Build and start all services
docker-compose up -d

# Check if services are running
docker-compose ps
```

### 2. Run Production Test (50 calls over 5 minutes)
```bash
# Run the production test
docker-compose exec asterisk /app/bin/load-test -concurrent=50 -duration=300 -call-duration=60 -report-file=reports/prod_test_50_calls.json

# Monitor real-time metrics (in a separate terminal)
watch -n 5 'curl -s http://localhost:9090/metrics | jq "."'
```

### 3. Check Results
```bash
# View the test report
docker-compose exec asterisk cat reports/prod_test_50_calls.json | jq '.'

# View system health
curl http://localhost:9090/health

# View detailed metrics
curl http://localhost:9090/metrics | jq '.'
```

## Test Configurations

### Regular Load Test
```bash
# Default test: 10 concurrent calls for 60 seconds
docker-compose exec asterisk /app/bin/load-test

# Custom test parameters
docker-compose exec asterisk /app/bin/load-test \
  -concurrent=20 \
  -duration=120 \
  -call-duration=30 \
  -report-file=reports/custom_test.json
```

### Production Test (50 calls over 5 minutes)
```bash
docker-compose exec asterisk /app/bin/load-test \
  -concurrent=50 \
  -duration=300 \
  -call-duration=60 \
  -report-file=reports/prod_test_50_calls.json
```

## Monitoring RTT Metrics

### Real-time Metrics
```bash
# Watch RTT metrics in real-time
watch -n 5 'curl -s http://localhost:9090/metrics | jq "{p50: .p50_latency, p95: .p95_latency, p99: .p99_latency, max: .max_latency, active_channels: .active_channels}"'
```

### Key RTT Metrics
- **p50_latency**: 50th percentile latency (median)
- **p95_latency**: 95th percentile latency
- **p99_latency**: 99th percentile latency
- **max_latency**: Maximum observed latency
- **avg_latency**: Average latency
- **packet_loss_ratio**: Packet loss percentage
- **late_ratio**: Late packet percentage

## Troubleshooting

### Port Conflicts
If you encounter port binding issues:
```bash
# Run the fix script
./fix_ari_service.sh
```

### Services Not Starting
```bash
# Check container logs
docker-compose logs asterisk

# Restart services
docker-compose restart

# Check service status
docker-compose exec asterisk ps aux
```

### No RTT Metrics
If RTT metrics show 0 values:
1. Ensure the ARI service is running without port conflicts
2. Check that calls are being processed in Asterisk
3. Verify the WebSocket connection between ARI service and Asterisk

## Test Reports

### Report Locations
- **Load Test Reports**: `reports/` directory in the container
- **Service Logs**: `logs/` directory in the container
- **Asterisk Logs**: `asterisk-logs/` directory on the host

### Report Format
The load test generates JSON reports with:
- Test start/end times
- Call success/failure statistics
- Performance metrics
- Individual call details

## Advanced Testing

### Packet Reordering Detection
The system includes packet reordering detection capabilities. To test this feature:
```bash
# Run packet reordering test
docker-compose exec asterisk go run cmd/packet_reordering_test/main.go
```

### Enhanced Load Testing
For longer duration tests with more comprehensive metrics:
```bash
# Run enhanced load test
docker-compose exec asterisk go run cmd/enhanced_load_test/main.go cmd/enhanced_load_test/ari_client.go
```

## System Architecture

### Components
1. **Asterisk**: Core telephony engine
2. **ARI Service**: RTP latency measurement and metrics collection
3. **Echo Server**: Test endpoint for RTP traffic
4. **Load Test Client**: Generates test calls

### Port Mappings
- **8088**: Asterisk HTTP/Ari
- **9090**: ARI Service metrics
- **5060**: SIP signaling
- **4000**: Echo server
- **10000-20000**: Asterisk RTP ports
- **21000-31000**: ARI service RTP ports

## Performance Considerations

### Recommended Test Durations
- **Short Tests**: 60-120 seconds for quick validation
- **Medium Tests**: 300-600 seconds for performance analysis
- **Long Tests**: 900+ seconds for stability testing

### Concurrent Call Limits
- **Light Load**: 10-20 concurrent calls
- **Medium Load**: 30-50 concurrent calls
- **Heavy Load**: 50+ concurrent calls (system dependent)

## Health Checks

### Service Health
```bash
# ARI Service health
curl http://localhost:9090/health

# Asterisk status
docker-compose exec asterisk asterisk -rx "core show version"
```

### System Resources
```bash
# Container resource usage
docker stats asterisk-ari

# Process list
docker-compose exec asterisk ps aux
```