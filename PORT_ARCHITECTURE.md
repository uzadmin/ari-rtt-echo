# RTP Port Architecture

## Overview

The system implements a dual-port architecture where:
1. **Echo Server** uses a single port for all operations
2. **ARI Service** uses a dynamic port range for RTP traffic

## Port Usage Breakdown

### 1. Echo Server (Single Port)
- **Port**: 4000/UDP
- **Purpose**: Stateless packet echoing
- **Behavior**: 
  - Listens on single port for incoming RTP packets
  - Echoes all packets back on the same port
  - No dynamic port allocation needed
  - Handles all channels through the same port

### 2. ARI Service (Port Range)
- **Port Range**: 10000-10100/UDP
- **Purpose**: Dynamic RTP channel management
- **Behavior**:
  - Dynamically allocates ports from range for each channel
  - Each SIP call gets unique RTP port
  - Forwards packets to echo server on port 4000
  - Receives echoes and calculates latency metrics

## System Flow

### Call Setup
1. ARI service receives call request
2. PortManager allocates RTP port from 10000-10100 range
3. RTP Worker created listening on allocated port
4. Channel established for RTP traffic

### RTP Packet Flow
1. Asterisk sends RTP packet to ARI service on allocated port (e.g., 10005)
2. ARI service forwards packet to echo server on port 4000
3. Echo server echoes packet back on port 4000
4. ARI service receives echo and sends back to Asterisk on allocated port
5. Latency calculated and metrics updated

## Configuration

### Docker Compose
```yaml
ports:
  - "4000:4000/udp"           # Echo server port
  - "10000-10100:10000-10100/udp"  # RTP port range
```

### Environment Variables
```env
PORT_RANGE=10000-10100
ECHO_HOST=127.0.0.1
ECHO_PORT=4000
```

## Benefits of This Design

### Echo Server (Single Port)
- **Simplicity**: Stateless operation requires no port management
- **Efficiency**: Single socket handles all echo operations
- **Reliability**: No port allocation failures possible

### ARI Service (Port Range)
- **Scalability**: Supports multiple concurrent calls
- **Isolation**: Each channel has dedicated RTP port
- **Flexibility**: Dynamic allocation adapts to call volume
- **Compatibility**: Standard RTP port range practices

## Technical Implementation

### PortManager
The ARI service uses a `PortManager` to handle dynamic allocation:

```go
type PortManager struct {
    minPort int
    maxPort int
    ports   []bool // bitset for port usage
    mu      sync.Mutex
}
```

### RTP Worker
Each channel gets an RTP worker with its own port:

```go
worker := rtp.NewWorker(
    channelID,
    bindIP,
    rtpPort,      // Allocated from range
    echoHost,     // Fixed: 127.0.0.1
    echoPort,     // Fixed: 4000
    metrics,
    t0,
    asteriskIP,
)
```

## Testing Verification

All tests confirm the system works correctly:
- Echo server responds on single port 4000
- ARI service communicates on ports 8088 and 9090
- SIP service listens on port 5060
- Port range 10000-10100 is properly mapped through Docker