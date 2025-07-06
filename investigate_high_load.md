# Investigation Guide: "Host Unreachable" Errors Under High Load

## 1. Immediate Diagnostics

### Check Current Configuration
```bash
# Check current connection limits and queue sizes
grep -r "backlog.*=" config/ || echo "backlog not configured"
grep -r "timeout.*=" config/ || echo "timeout not configured"
grep -r "retries.*=" config/ || echo "retries not configured"
```

### Monitor Queue Overflow
```bash
# Check for queue overflow logs
tail -f logs/*.log | grep -i "queue is full\|discarded"
```

### Check SOCKS Proxy Health
```bash
# Test SOCKS proxy directly
curl --socks5 your-socks-proxy:port https://httpbin.org/ip -v
```

## 2. Configuration Tuning (Apply These Fixes)

### A. Increase Connection Queue Sizes
Add to your service configuration:
```yaml
services:
  - name: your-service
    addr: ":8080"
    metadata:
      backlog: 1024              # Increase from default 128
      readQueueSize: 4096        # For UDP services
      readBufferSize: 65536      # Increase buffer sizes
```

### B. Increase Timeouts and Retries
Add to your chain/router configuration:
```yaml
chains:
  - name: your-chain
    hops:
      - name: your-hop
        metadata:
          timeout: 60s           # Increase from 15s default
          retries: 5             # Increase from 1 default
          connectTimeout: 30s    # SOCKS connection timeout
```

### C. Tune Traffic Limiters
```yaml
limiters:
  - name: conn-limiter
    type: conn
    limits:
      - "$": 10000             # Global connection limit
      - "$$": 1000             # Per-IP connection limit
  
  - name: traffic-limiter
    type: traffic
    limits:
      - "$": "1GB"             # Global traffic limit
      - "$$": "100MB"          # Per-IP traffic limit
```

## 3. Code-Level Optimizations

### A. Increase Default Backlog in Listeners
For immediate relief, modify these default values:

**File: `listener/*/metadata.go`**
```go
const (
    defaultBacklog = 1024  // Change from 128
)
```

### B. Make Tunnel Dialer Timeout Configurable
**File: `handler/tunnel/dialer.go`** (around line 78):
```go
dialer := net.Dialer{
    Timeout: d.timeout, // Already configurable
}
```
**File: `handler/tunnel/connect.go`** (around line 82):
```go
// Make retry and timeout configurable
d := Dialer{
    node:    h.id,
    pool:    h.pool,
    sd:      h.md.sd,
    retry:   h.md.tunnelRetries,    // Make configurable
    timeout: h.md.tunnelTimeout,    // Make configurable
    log:     log,
}
```

### C. Improve Connection Pool TTL Management
**File: `handler/tunnel/tunnel.go`** (around line 292):
```go
// Reduce cleanup interval under high load
go t.clean() // Make interval configurable
```

## 4. Monitoring and Alerting

### A. Enable Detailed Metrics
Ensure metrics are enabled and monitoring:
```bash
# Check metrics endpoint
curl http://localhost:9090/metrics | grep -E "gost_(service|chain|conn)"
```

### B. Key Metrics to Monitor
- `gost_service_requests_in_flight` - Active connections
- `gost_service_handler_errors_total` - Error rates
- `gost_chain_errors_total` - Chain-level failures
- `gost_service_requests_total` - Request volume

### C. Set Up Alerts
```yaml
# Prometheus alert rules
- alert: HighErrorRate
  expr: rate(gost_service_handler_errors_total[5m]) > 0.1
  for: 2m
  
- alert: QueueSaturation
  expr: gost_service_requests_in_flight > 900  # 90% of 1024 queue
  for: 1m
```

## 5. Load Testing

### A. Reproduce the Issue
```bash
# Use a load testing tool to simulate high load
hey -n 10000 -c 100 -t 30 http://your-proxy:port/test
```

### B. Monitor During Load Test
```bash
# Watch metrics and logs during load test
watch -n 1 'curl -s http://localhost:9090/metrics | grep gost_service_requests_in_flight'
```

## 6. Emergency Mitigation

### A. Scale Horizontally
```bash
# Add more proxy instances behind a load balancer
docker-compose scale gost=3
```

### B. Connection Draining
```bash
# Gracefully restart services to clear connection pools
pkill -TERM gost  # Graceful shutdown
```

### C. Circuit Breaker Pattern
Consider implementing circuit breakers for failing SOCKS proxies:
```yaml
chains:
  - name: your-chain
    hops:
      - name: socks-proxy-1
        selector:
          strategy: round_robin
          maxFails: 3
          failTimeout: 30s
```

## 7. Long-term Solutions

### A. Connection Pooling Optimization
- Implement connection reuse for SOCKS connections
- Add connection health checks
- Dynamic pool sizing based on load

### B. Auto-scaling Configuration
- Configure auto-scaling based on queue depth
- Implement back-pressure mechanisms
- Add adaptive timeout adjustments

### C. Distributed Architecture
- Consider splitting traffic across multiple proxy clusters
- Implement proper load balancing with health checks
- Add circuit breakers and fallback mechanisms

## 8. Configuration Template

Here's a production-ready configuration template:

```yaml
services:
  - name: http-proxy
    addr: ":8080"
    handler:
      type: http
      metadata:
        readTimeout: 30s
        keepalive: true
    listener:
      type: tcp
      metadata:
        backlog: 2048
        mptcp: true
    
chains:
  - name: socks-chain
    hops:
      - name: socks-proxy
        addr: socks-proxy:1080
        connector:
          type: socks5
          metadata:
            connectTimeout: 30s
        dialer:
          type: tcp
          metadata:
            timeout: 60s
            retries: 3

limiters:
  - name: conn-limiter
    type: conn
    limits:
      - "$": 50000
      - "$$": 1000
  
  - name: traffic-limiter
    type: traffic
    limits:
      - "$": "10GB"
      - "$$": "500MB"

api:
  addr: ":18080"
  pathPrefix: /api
  accesslog: true

metrics:
  addr: ":9090"
  path: /metrics
``` 