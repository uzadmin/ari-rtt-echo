# ARI Service with RTP Latency Measurement

This is a minimal ARI service that creates externalMedia channels for each call and transparently processes audio through an external UDP echo processor. The system measures round-trip latency, packet loss, and late packet ratios.

## Architecture

1. **ARI Service** - Handles Stasis events, creates externalMedia channels, and manages RTP workers
2. **Echo Server** - Simple UDP echo processor that loops back RTP packets with proper pacing
3. **Load Test Script** - Configurable load testing tool with specified parameters

## Requirements

- Go 1.16+
- Docker (for containerized deployment)
- Asterisk 20 with ARI enabled

## Environment Variables

The following environment variables can be configured:

```
ARI_URL=localhost:8088          # ARI server URL
ARI_USER=ari                     # ARI username
ARI_PASS=ari                     # ARI password
APP_NAME=ari-app                 # ARI application name
BIND_IP=0.0.0.0                  # IP to bind services
PORT_RANGE=4500-50000            # RTP port range for ARI service
ECHO_HOST=127.0.0.1              # Echo server host
ECHO_PORT=4000                   # Echo server port
METRICS_INTERVAL_SEC=5           # Metrics reporting interval
```

## Quick Start

1. **Build the components:**
   ```bash
   go build -o bin/ari-service ./cmd/ari-service
   go build -o bin/echo-server ./cmd/echo
   go build -o bin/load-test-new ./cmd/load_test_new
   ```

2. **Start the services:**
   ```bash
   ./bin/ari-service &
   ./bin/echo-server &
   ```

3. **Run the load test:**
   ```bash
   ./bin/load-test-new --count=30 --duration-ms=30000 --delay-between-ms=100
   ```

## Using the Simple Runner Script

For easier execution, use the provided runner script:

```bash
# Run with default parameters (30 calls)
./run_simple.sh

# Run with custom parameters
./run_simple.sh --count=100 --duration-ms=30000 --delay-between-ms=100
./run_simple.sh --count=150 --duration-ms=30000 --delay-between-ms=100
```

## SLA Requirements

The system is designed to meet the following SLA requirements:

### Base Invariants (any load level):
1. No packet with latency > 22ms relative to its deadline
2. No more than 2 consecutive deadline misses per channel
3. late_ratio (percentage of packets arriving/sent later than deadline) ≤ 0.1%
4. Application-level packet loss (sequence gaps) ≤ 0.2%

### 30 Concurrent Calls (Nominal Load):
- p50 ≤ 8 ms, p95 ≤ 12 ms, p99 ≤ 18 ms, max ≤ 22 ms
- Round-trip (echo) p95 ≤ 18 ms, p99 ≤ 22 ms
- late_ratio ≤ 0.05%

### 100 Concurrent Calls (High Load):
- p50 ≤ 10 ms, p95 ≤ 15 ms, p99 ≤ 20 ms, max ≤ 22 ms
- Round-trip p95 ≤ 20 ms, p99 ≤ 24 ms
- late_ratio ≤ 0.1%

### 150 Concurrent Calls (Stress):
- p50 ≤ 12 ms, p95 ≤ 18 ms, p99 ≤ 22 ms, max ≤ 22 ms
- Round-trip p95 ≤ 22 ms, p99 ≤ 26 ms
- late_ratio ≤ 0.2%
- No "zombie" ports/workers after test completion

## Monitoring

Real-time metrics are available at:
- Health check: `curl http://localhost:9090/health`
- Metrics: `curl http://localhost:9090/metrics`

Metrics include:
- Active channels count
- RTT percentiles (p50, p95, p99, max)
- Packet loss ratio
- Late packet ratio

## Logs

All components log to the `logs/` directory:
- `logs/ari-service.log` - ARI service logs with timestamps and channel IDs
- `logs/echo-server.log` - Echo server logs
- `logs/load-test-new.log` - Load test execution logs

## Output

The system generates reports in the `reports/` directory:
- `reports/load_test_new_report.json` - Detailed load test results
- `reports/simple_summary_report_*.txt` - Summary reports with final metrics

## Cleanup

To stop all services:
```bash
pkill -f ari-service
pkill -f echo-server
```