# ARI Service Architecture

## Component Diagram

```mermaid
graph TD
    A[Asterisk] -->|SIP/RTP| B[ARI Service]
    B -->|RTP| C[Echo Server]
    C -->|RTP| B
    B -->|ARI Events| D[Load Test]
    
    subgraph ARI_Service["ARI Service (localhost:9090)"]
        B1[RTP Worker] -->|T0/T1| B2[Metrics Collector]
        B2 -->|p50/p95/p99| B3[HTTP Endpoint]
    end
    
    subgraph Echo["Echo Server (ECHO_PORT)"]
        C1[Packet Receiver] --> C2[Packet Echo]
    end
    
    subgraph Load["Load Test"]
        D1[Call Generator] -->|Originate| A
        D2[Result Collector] --> D3[Report Generator]
    end
```

## Data Flow

1. **Call Setup**:
   ```
   Load Test → ARI Originate → Asterisk → ARI StasisStart Event → ARI Service
   ```

2. **RTP Processing**:
   ```
   Asterisk → RTP Packet → ARI Service (T0) → Echo Server → ARI Service (T1) → Asterisk
   ```

3. **Latency Calculation**:
   ```
   RTT = T1 - T0 for each (SSRC, Seq) pair
   ```

4. **Metrics Collection**:
   ```
   RTT values → Percentile calculation → Periodic reporting
   ```

5. **Teardown**:
   ```
   ARI StasisEnd Event → Stop Worker → Close Socket → Release Port
   ```

## Key Implementation Details

### Packet Flow
```
┌─────────────┐    RTP     ┌─────────────┐    RTP     ┌─────────────┐
│   Asterisk  │ ────────► │ ARI Service │ ────────► │ Echo Server │
│             │           │             │           │             │
│             │           │ ├── Parse   │           │ ├── Echo    │
│             │           │ ├── T0      │           │ ├── TS      │
│             │           │ └── Forward │           │ └── Pacing  │
└─────────────┘           └─────────────┘           └─────────────┘
       ▲                         │                         │
       │                         │                         │
       │                         ▼                         │
       │                  ┌─────────────┐                  │
       │                  │   Return    │                  │
       │                  │   Path      │                  │
       │                  └─────────────┘                  │
       │                         │                         │
       │                         ▼                         │
       │                  ┌─────────────┐                  │
       │                  │  T1 Calc    │                  │
       │                  │  RTT = T1-T0│                  │
       │                  └─────────────┘                  │
       │                         │                         │
       └─────────────────────────┼─────────────────────────┘
                            Round-trip
                            Measurement
```

### Per-Channel Architecture
```
Channel Handler
├── RTP Worker (goroutine)
│   ├── UDP Socket
│   ├── Latency Tracker
│   └── Sequence Tracker
├── Bridge
├── External Media
└── Cleanup Function
```

### Metrics Flow
```
RTP Packet
    ↓
Latency Tracker (T0/T1)
    ↓
RTT Value
    ↓
Metrics Collector
    ↓
Percentile Calculation
    ↓
Periodic Reporting (5s)
    ↓
stdout + HTTP Endpoint
```