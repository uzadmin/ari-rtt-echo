# Extended Testing Guide

This guide explains how to run the extended 100-call test with monitoring for zombie channels and unclosed ports.

## Test Configuration

The extended test runs with the following configuration:
- **Concurrent calls**: 100
- **Test duration**: 600 seconds (10 minutes)
- **Call duration**: 1800 seconds (30 minutes)
- **Port range**: 21000-31000 (10,001 ports available)

## Running the Extended Test

### 1. Execute the Test Script

```bash
./run_extended_test.sh
```

This script will:
1. Build all components
2. Start the ARI service and echo server
3. Start monitoring for zombie channels and unclosed ports
4. Run the 100-call test
5. Generate a comprehensive report

### 2. Monitor Progress

During the test, you'll see real-time metrics displayed every 10 seconds:
- Active channel count
- RTT percentiles (p50, p95, p99, max)
- Packet loss ratio
- Late packet ratio

### 3. Test Duration

The test will run for approximately 10 minutes. The script includes a 20-minute timeout to prevent indefinite hanging.

## Monitoring for Issues

### Real-time Monitoring

The test automatically monitors for:
1. **Zombie channels**: Active channels that are no longer processing latencies
2. **Unclosed ports**: Ports that remain allocated after channels are terminated
3. **Resource usage**: Memory and CPU consumption

### Manual Monitoring

You can also manually check for issues during the test:

```bash
# Check ARI service metrics
curl http://localhost:9090/metrics

# Check port usage in our range
lsof -i :21000-31000

# Check running processes
ps aux | grep -E "(ari-service|echo-server)"
```

## Analyzing Results

### 1. Automatic Analysis

After the test completes, run the analysis script:

```bash
./analyze_test_results.sh
```

This script will check:
- Zombie channel warnings
- Unclosed port warnings
- Port allocation errors
- Channel cleanup statistics
- Current port usage
- Load test errors

### 2. Manual Analysis

Key log files to examine:

1. **Main Report**: `reports/extended_test_summary_*.txt`
2. **ARI Service Log**: `logs/ari-service.log`
3. **Echo Server Log**: `logs/echo-server.log`
4. **Load Test Log**: `logs/load-test.log`
5. **Monitoring Log**: `logs/monitoring.log`

### 3. Key Metrics to Check

In the final metrics, look for:
- **Active Channels**: Should be 0 after test completion
- **Used RTP Ports**: Should be 0 after test completion
- **Packet Loss Ratio**: Should be low (< 1%)
- **Late Packet Ratio**: Should be low (< 1%)

## Troubleshooting Common Issues

### 1. Port Exhaustion

**Symptoms**: 
- "No ports available in range 21000-31000" errors
- Test fails early

**Solutions**:
- Ensure the port range is correctly configured (21000-31000)
- Check for processes holding onto ports: `lsof -i :21000-31000`
- Kill any lingering processes: `pkill -f "ari-service"`

### 2. Zombie Channels

**Symptoms**:
- Active channels remain after test completion
- Metrics show channels but no new latencies

**Solutions**:
- The system automatically cleans up zombie channels every 2 minutes
- Manually check channels: `curl http://localhost:9090/metrics`
- Restart services if needed

### 3. High Packet Loss or Late Packets

**Symptoms**:
- Packet loss ratio > 1%
- Late packet ratio > 1%
- Poor RTT metrics

**Solutions**:
- Check system resources (CPU, memory)
- Verify network connectivity
- Reduce concurrent call count if system is overloaded

## Best Practices

### Before Running Tests

1. **Clean up previous runs**:
   ```bash
   pkill -f "ari-service"
   pkill -f "echo-server"
   rm -f logs/*.log
   ```

2. **Verify port availability**:
   ```bash
   lsof -i :21000-31000 | grep LISTEN
   ```

3. **Check system resources**:
   ```bash
   top -l 1 | head -20
   ```

### During Tests

1. **Monitor resource usage**:
   ```bash
   top -l 1 | grep -E "(ari-service|echo-server)"
   ```

2. **Check for errors**:
   ```bash
   tail -f logs/ari-service.log | grep -i error
   ```

### After Tests

1. **Verify clean shutdown**:
   ```bash
   ps aux | grep -E "(ari-service|echo-server)"
   ```

2. **Check port cleanup**:
   ```bash
   lsof -i :21000-31000
   ```

3. **Analyze results**:
   ```bash
   ./analyze_test_results.sh
   ```

## Scaling Considerations

For tests with higher concurrent call counts:

1. **Increase port range**: Modify [.env](file:///Users/3knet3knet/4/v3/.env) and [docker-compose.yml](file:///Users/3knet3knet/4/v3/docker-compose.yml) to use a larger port range
2. **Monitor system resources**: Higher call counts require more CPU and memory
3. **Adjust timeouts**: Longer tests may require adjusted timeouts in scripts
4. **Consider hardware limitations**: Very high concurrent calls may require multiple machines

## Emergency Procedures

### If Test Hangs

1. **Kill the test**:
   ```bash
   pkill -f "load-test"
   ```

2. **Stop services**:
   ```bash
   pkill -f "ari-service"
   pkill -f "echo-server"
   ```

3. **Check for lingering processes**:
   ```bash
   ps aux | grep -E "(ari-service|echo-server|load-test)"
   ```

### If Ports Are Not Released

1. **Check port usage**:
   ```bash
   lsof -i :21000-31000
   ```

2. **Kill processes using ports**:
   ```bash
   lsof -t -i :21000-31000 | xargs kill -9
   ```

3. **Verify ports are free**:
   ```bash
   lsof -i :21000-31000
   ```

This comprehensive testing approach will help you identify and resolve any issues with zombie channels or unclosed ports during your extended load testing.