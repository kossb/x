# SOCKS Connector Retry Feature

## Overview

The SOCKS (v4 and v5) connectors now support automatic retry functionality for transient connection failures. This feature helps improve reliability in unstable network environments by automatically retrying failed connections with exponential backoff and jitter.

## Supported Error Types

### Retriable Errors

The following error types will trigger automatic retries:

#### SOCKS5 Errors
- `0x01` - General SOCKS server failure (temporary server issues)
- `0x03` - Network unreachable (routing problems)
- `0x04` - Host unreachable (network connectivity issues)
- `0x05` - Connection refused (target service restarting)
- `0x06` - TTL expired (temporary routing loops)

#### SOCKS4 Errors
- Request rejected or failed (temporary server issues)
- Request failed (could be temporary)

#### Network-Level Errors
- Timeout errors (`net.Error.Timeout()`)
- Temporary network errors (`net.Error.Temporary()`)

### Non-Retriable Errors

The following errors will NOT trigger retries:

#### SOCKS5 Errors
- `0x02` - Connection not allowed by ruleset (configuration issue)
- `0x07` - Command not supported (server capability issue)
- `0x08` - Address type not supported (server configuration issue)

#### SOCKS4 Errors
- Request rejected due to user ID mismatch (authentication issue)

## Configuration

### Metadata Configuration

Configure retry behavior using metadata parameters:

```yaml
connectors:
  - name: socks5-connector
    type: socks5
    metadata:
      retry.maxAttempts: 3          # Maximum number of retry attempts (default: 3)
      retry.initialDelay: 100ms     # Initial delay before first retry (default: 100ms)
      retry.maxDelay: 5s           # Maximum delay between retries (default: 5s)
```

### Alternative Configuration Keys

For backward compatibility, you can also use:

```yaml
connectors:
  - name: socks5-connector
    type: socks5
    metadata:
      retryMaxAttempts: 3
      retryInitialDelay: 100ms
      retryMaxDelay: 5s
```

## Retry Algorithm

### Exponential Backoff with Jitter

The retry mechanism uses exponential backoff with jitter to prevent thundering herd problems:

1. **Base Delay**: Starts with `initialDelay`
2. **Exponential Growth**: Each retry doubles the delay (factor: 2.0)
3. **Maximum Cap**: Delays are capped at `maxDelay`
4. **Jitter**: ±10% random variation to prevent synchronized retries
5. **Context Awareness**: Respects context cancellation and timeouts

### Retry Sequence Example

```
Attempt 1: Immediate
Attempt 2: ~100ms ± 10ms jitter
Attempt 3: ~200ms ± 20ms jitter
Attempt 4: ~400ms ± 40ms jitter (if maxAttempts > 3)
```

## Usage Examples

### Basic Configuration

```yaml
services:
  - name: socks5-service
    addr: ":1080"
    handler:
      type: socks5
    listener:
      type: tcp

connectors:
  - name: socks5-retry
    type: socks5
    metadata:
      retry.maxAttempts: 5
      retry.initialDelay: 200ms
      retry.maxDelay: 10s
```

### With SOCKS Authentication

```yaml
connectors:
  - name: authenticated-socks5
    type: socks5
    auth:
      username: user
      password: pass
    metadata:
      retry.maxAttempts: 3
      retry.initialDelay: 100ms
      retry.maxDelay: 5s
```

### SOCKS4 Configuration

```yaml
connectors:
  - name: socks4-retry
    type: socks4
    metadata:
      retry.maxAttempts: 3
      retry.initialDelay: 150ms
      retry.maxDelay: 3s
```

## Logging and Monitoring

### Log Messages

The retry feature provides detailed logging:

```
DEBUG: connect example.com/tcp
WARN:  connection attempt 1/3 failed, retrying in 100ms: host unreachable
DEBUG: retrying connection attempt 2/3
WARN:  connection attempt 2/3 failed, retrying in 200ms: host unreachable  
DEBUG: retrying connection attempt 3/3
INFO:  connection succeeded on attempt 3/3
```

### Log Fields

Each retry attempt includes structured log fields:

- `attempt`: Current attempt number
- `max_attempts`: Maximum configured attempts
- `error`: The error that triggered the retry
- `delay`: Calculated backoff delay
- `reply_code`: SOCKS protocol error code (hex format)
- `target`: Target address being connected to

### Error Classification

Non-retriable errors are logged with explanatory messages:

```
DEBUG: error is not retriable, aborting retries: connection not allowed by ruleset
```

## Best Practices

### Configuration Guidelines

1. **Conservative Defaults**: Start with default values (3 attempts, 100ms initial delay)
2. **Network Latency**: Adjust `initialDelay` based on expected network latency
3. **Timeout Coordination**: Ensure `connectTimeout` > `maxDelay` * `maxAttempts`
4. **Resource Limits**: Consider server load when setting `maxAttempts`

### Monitoring Recommendations

1. **Track Retry Rates**: Monitor ratio of retried vs. successful connections
2. **Alert on High Retry Rates**: High retry rates may indicate network issues
3. **Measure Total Latency**: Include retry delays in latency metrics
4. **Log Analysis**: Use structured logs for retry pattern analysis

### Performance Considerations

1. **Memory Usage**: Each retry attempt reuses the same connection
2. **Context Cancellation**: Retries respect context deadlines
3. **Concurrent Connections**: Retry logic is per-connection, not global
4. **Jitter Benefits**: Prevents synchronized retry storms

## Troubleshooting

### Common Issues

#### High Retry Rates
- **Cause**: Network instability or server overload
- **Solution**: Investigate network path, check server capacity

#### No Retries Occurring
- **Cause**: Errors are classified as non-retriable
- **Solution**: Check error types, verify configuration

#### Excessive Delays
- **Cause**: `maxDelay` set too high or too many attempts
- **Solution**: Reduce `maxDelay` or `maxAttempts`

### Diagnostic Tools

Use the built-in diagnostic functionality:

```go
// Test SOCKS5 connectivity
err := TestSocks5Connectivity(ctx, "127.0.0.1:1080", 5*time.Second, logger)
if err != nil {
    log.Errorf("SOCKS5 connectivity test failed: %v", err)
}
```

### Debug Configuration

Enable debug logging to see retry behavior:

```yaml
log:
  level: debug
  format: json
```

## Migration Guide

### From Non-Retry Configuration

Existing configurations will continue to work with default retry behavior:

- Default: 3 retry attempts
- Default: 100ms initial delay  
- Default: 5s maximum delay

### Disabling Retries

To disable retry functionality:

```yaml
metadata:
  retry.maxAttempts: 1  # Only one attempt, no retries
```

## Performance Impact

### Latency
- **Success Case**: No additional latency
- **Retry Case**: Additional latency = sum of backoff delays
- **Failure Case**: Total time = `maxDelay` * `maxAttempts`

### Resources
- **CPU**: Minimal overhead for retry logic
- **Memory**: No additional memory per retry
- **Network**: Only failed attempts generate additional traffic

## Version Compatibility

- **SOCKS4**: Retry support added
- **SOCKS5**: Retry support added  
- **UDP**: Retry support for UDP tunnel setup
- **Backward Compatibility**: Full compatibility with existing configurations 