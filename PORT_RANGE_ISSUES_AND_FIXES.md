# Port Range Issues and Fixes

## Problem Analysis

Based on the logs provided, we identified two critical issues:

1. **Port Exhaustion**: The error "No ports available in range 21000-21100" indicates that the current port range only provides 101 ports (21000-21100 inclusive), which is insufficient for the load test with 50 concurrent calls.

2. **Channel Not Found Errors**: Multiple "Channel not found" errors when trying to answer channels, indicating timing issues between when Asterisk creates a channel and when the ARI service tries to answer it.

## Root Causes

### Port Exhaustion
- The port range was configured as `21000-21100` (101 ports)
- Each concurrent call requires a unique RTP port
- With 50 concurrent calls in the production test, we need at least 50 ports
- Under high load or with port cleanup delays, more ports may be needed temporarily

### Channel Not Found Errors
- Timing issues between Asterisk channel creation and ARI service processing
- Asterisk sends the StasisStart event, but the channel might not be fully initialized yet
- When the ARI service immediately tries to answer the channel, it may not exist yet in Asterisk's channel registry

## Solutions Implemented

### 1. Expanded Port Range
- Updated [.env](file:///Users/3knet3knet/4/v3/.env) file to use `PORT_RANGE=21000-31000` (10,001 ports)
- Updated [docker-compose.yml](file:///Users/3knet3knet/4/v3/docker-compose.yml) to reflect the new port range
- This provides sufficient ports for even heavy load testing scenarios

### 2. Improved Error Handling
- Added retry logic for answering channels with exponential backoff
- Added specific handling for "Channel not found" errors
- Implemented logging to track retry attempts

### 3. Separated Port Ranges
- Confirmed that Asterisk RTP ports (10000-20000) are separate from ARI service ports (21000-31000)
- This prevents conflicts between Asterisk's internal RTP handling and our service's RTP handling

## Files Modified

1. [.env](file:///Users/3knet3knet/4/v3/.env) - Updated PORT_RANGE from `21000-21100` to `21000-31000`
2. [docker-compose.yml](file:///Users/3knet3knet/4/v3/docker-compose.yml) - Updated PORT_RANGE environment variable
3. [cmd/ari-service/main.go](file:///Users/3knet3knet/4/v3/cmd/ari-service/main.go) - Added retry logic for channel answering

## Verification Steps

1. Run the fix script:
   ```bash
   ./fix_port_range.sh
   ```

2. Run the production test:
   ```bash
   ./run.sh prod
   ```

3. Monitor logs for any remaining issues:
   ```bash
   docker-compose logs -f asterisk
   ```

## Expected Results

- Port exhaustion errors should be eliminated
- Channel not found errors should be significantly reduced
- Production test with 50 concurrent calls over 5 minutes should complete successfully
- RTT metrics should be properly collected and reported

## Additional Considerations

1. **Resource Usage**: The expanded port range uses more memory but provides better reliability
2. **Network Configuration**: Ensure firewall rules allow traffic on the expanded port range
3. **Load Testing**: The system should now handle the intended load test scenarios without port exhaustion

## Future Improvements

1. **Dynamic Port Management**: Implement more sophisticated port allocation that can dynamically adjust based on load
2. **Enhanced Error Recovery**: Add more robust error handling for various ARI API errors
3. **Monitoring and Alerts**: Implement monitoring for port usage to detect potential issues before they cause failures