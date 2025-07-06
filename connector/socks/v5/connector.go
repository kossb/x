package v5

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"time"

	"github.com/go-gost/core/connector"
	"github.com/go-gost/core/logger"
	md "github.com/go-gost/core/metadata"
	"github.com/go-gost/gosocks5"
	ctxvalue "github.com/go-gost/x/ctx"
	"github.com/go-gost/x/internal/util/socks"
	"github.com/go-gost/x/registry"
)

func init() {
	registry.ConnectorRegistry().Register("socks5", NewConnector)
	registry.ConnectorRegistry().Register("socks", NewConnector)
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts   int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
	JitterFactor  float64
}

// DefaultRetryConfig returns sensible defaults for retry behavior
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
	}
}

// isRetriableError determines if a SOCKS5 error should be retried
func isRetriableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network-level errors that might be transient
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() || netErr.Temporary() {
			return true
		}
	}

	// SOCKS5 reply errors that might be transient
	switch errStr {
	case "general SOCKS server failure":
		return true // Server might be temporarily overloaded
	case "network unreachable":
		return true // Routing might be temporarily broken
	case "host unreachable":
		return true // Network path might be temporarily down
	case "connection refused":
		return true // Target service might be restarting
	case "TTL expired":
		return true // Routing loops might be temporary
	default:
		return false
	}
}

// isRetriableSocks5ReplyCode determines if a SOCKS5 reply code indicates a retriable error
func isRetriableSocks5ReplyCode(code uint8) bool {
	switch code {
	case 0x01: // General SOCKS server failure
		return true
	case 0x03: // Network unreachable
		return true
	case 0x04: // Host unreachable
		return true
	case 0x05: // Connection refused
		return true
	case 0x06: // TTL expired
		return true
	default:
		return false
	}
}

// calculateBackoffDelay calculates the delay for the next retry attempt
func calculateBackoffDelay(attempt int, config RetryConfig) time.Duration {
	if attempt <= 0 {
		return config.InitialDelay
	}

	// Exponential backoff
	delay := float64(config.InitialDelay) * math.Pow(config.BackoffFactor, float64(attempt-1))

	// Apply maximum delay cap
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	// Add jitter to prevent thundering herd
	jitter := delay * config.JitterFactor * (rand.Float64()*2 - 1) // ±jitterFactor
	finalDelay := time.Duration(delay + jitter)

	// Ensure minimum delay
	if finalDelay < config.InitialDelay {
		finalDelay = config.InitialDelay
	}

	return finalDelay
}

type socks5Connector struct {
	selector gosocks5.Selector
	md       metadata
	options  connector.Options
}

func NewConnector(opts ...connector.Option) connector.Connector {
	options := connector.Options{}
	for _, opt := range opts {
		opt(&options)
	}

	return &socks5Connector{
		options: options,
	}
}

func (c *socks5Connector) Init(md md.Metadata) (err error) {
	if err = c.parseMetadata(md); err != nil {
		return
	}

	selector := &clientSelector{
		methods: []uint8{
			gosocks5.MethodNoAuth,
		},
		User:      c.options.Auth,
		TLSConfig: c.options.TLSConfig,
		logger:    c.options.Logger,
	}
	if selector.User != nil {
		selector.methods = append(selector.methods, gosocks5.MethodUserPass)
	}
	if !c.md.noTLS {
		selector.methods = append(selector.methods, socks.MethodTLS)
		if selector.TLSConfig == nil {
			selector.TLSConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
		}
		if selector.User != nil {
			selector.methods = append(selector.methods, socks.MethodTLSAuth)
		}
	}
	c.selector = selector

	return
}

// Handshake implements connector.Handshaker.
func (c *socks5Connector) Handshake(ctx context.Context, conn net.Conn) (net.Conn, error) {
	log := c.options.Logger.WithFields(map[string]any{
		"remote": conn.RemoteAddr().String(),
		"local":  conn.LocalAddr().String(),
	})

	if c.md.connectTimeout > 0 {
		conn.SetDeadline(time.Now().Add(c.md.connectTimeout))
		defer conn.SetDeadline(time.Time{})
	}

	cc := gosocks5.ClientConn(conn, c.selector)
	if err := cc.Handleshake(); err != nil {
		log.Error(err)
		return nil, err
	}

	return cc, nil
}

// socks5ReplyError maps SOCKS5 reply codes to meaningful error messages
func socks5ReplyError(code uint8) error {
	switch code {
	case gosocks5.Succeeded:
		return nil
	case 0x01:
		return errors.New("general SOCKS server failure")
	case 0x02:
		return errors.New("connection not allowed by ruleset")
	case 0x03:
		return errors.New("network unreachable")
	case 0x04:
		return errors.New("host unreachable")
	case 0x05:
		return errors.New("connection refused")
	case 0x06:
		return errors.New("TTL expired")
	case 0x07:
		return errors.New("command not supported")
	case 0x08:
		return errors.New("address type not supported")
	default:
		return fmt.Errorf("unknown SOCKS5 reply code: 0x%02x", code)
	}
}

func (c *socks5Connector) Connect(ctx context.Context, conn net.Conn, network, address string, opts ...connector.ConnectOption) (net.Conn, error) {
	return c.connectWithRetry(ctx, conn, network, address, opts...)
}

func (c *socks5Connector) connectWithRetry(ctx context.Context, conn net.Conn, network, address string, opts ...connector.ConnectOption) (net.Conn, error) {
	retryConfig := DefaultRetryConfig()

	// Override with metadata configuration if available
	if c.md.retryMaxAttempts > 0 {
		retryConfig.MaxAttempts = c.md.retryMaxAttempts
	}
	if c.md.retryInitialDelay > 0 {
		retryConfig.InitialDelay = c.md.retryInitialDelay
	}
	if c.md.retryMaxDelay > 0 {
		retryConfig.MaxDelay = c.md.retryMaxDelay
	}

	log := c.options.Logger.WithFields(map[string]any{
		"remote":  conn.RemoteAddr().String(),
		"local":   conn.LocalAddr().String(),
		"network": network,
		"address": address,
		"sid":     string(ctxvalue.SidFromContext(ctx)),
	})

	var lastErr error

	for attempt := 1; attempt <= retryConfig.MaxAttempts; attempt++ {
		attemptLog := log.WithFields(map[string]any{
			"attempt":      attempt,
			"max_attempts": retryConfig.MaxAttempts,
		})

		if attempt > 1 {
			attemptLog.Debugf("retrying connection attempt %d/%d", attempt, retryConfig.MaxAttempts)
		} else {
			attemptLog.Debugf("connect %s/%s", address, network)
		}

		result, err := c.connectAttempt(ctx, conn, network, address, attemptLog, opts...)
		if err == nil {
			if attempt > 1 {
				attemptLog.Infof("connection succeeded on attempt %d/%d", attempt, retryConfig.MaxAttempts)
			}
			return result, nil
		}

		lastErr = err

		// Check if error is retriable
		if !isRetriableError(err) {
			attemptLog.WithFields(map[string]any{
				"error": err.Error(),
			}).Debug("error is not retriable, aborting retries")
			break
		}

		// Don't retry if this is the last attempt
		if attempt >= retryConfig.MaxAttempts {
			break
		}

		// Check if context is cancelled
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Calculate backoff delay
		delay := calculateBackoffDelay(attempt, retryConfig)
		attemptLog.WithFields(map[string]any{
			"error": err.Error(),
			"delay": delay.String(),
		}).Warnf("connection attempt %d/%d failed, retrying in %v", attempt, retryConfig.MaxAttempts, delay)

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return nil, fmt.Errorf("all %d connection attempts failed, last error: %w", retryConfig.MaxAttempts, lastErr)
}

func (c *socks5Connector) connectAttempt(ctx context.Context, conn net.Conn, network, address string, log logger.Logger, opts ...connector.ConnectOption) (net.Conn, error) {
	if c.md.connectTimeout > 0 {
		conn.SetDeadline(time.Now().Add(c.md.connectTimeout))
		defer conn.SetDeadline(time.Time{})
	}

	var cOpts connector.ConnectOptions
	for _, opt := range opts {
		opt(&cOpts)
	}

	switch network {
	case "udp", "udp4", "udp6":
		return c.connectUDPWithRetry(ctx, conn, network, address, log, &cOpts)
	case "tcp", "tcp4", "tcp6":
		if _, ok := conn.(net.PacketConn); ok {
			err := fmt.Errorf("tcp over udp is unsupported")
			log.Error(err)
			return nil, err
		}
	default:
		err := fmt.Errorf("network %s is unsupported", network)
		log.Error(err)
		return nil, err
	}

	addr := gosocks5.Addr{}
	if err := addr.ParseFrom(address); err != nil {
		log.Error(err)
		return nil, err
	}
	if addr.Host == "" {
		addr.Type = gosocks5.AddrIPv4
		addr.Host = "127.0.0.1"
	}

	req := gosocks5.NewRequest(gosocks5.CmdConnect, &addr)
	log.Trace(req)
	if err := req.Write(conn); err != nil {
		log.Error(err)
		return nil, err
	}

	reply, err := gosocks5.ReadReply(conn)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	log.Trace(reply)

	if reply.Rep != gosocks5.Succeeded {
		err = socks5ReplyError(reply.Rep)

		// Enhanced diagnostic logging
		diagnosticInfo := DiagnoseSocks5Error(reply.Rep, address, network)
		LogDiagnosticInfo(log, diagnosticInfo)

		return nil, err
	}

	return conn, nil
}

func (c *socks5Connector) connectUDPWithRetry(ctx context.Context, conn net.Conn, network, address string, log logger.Logger, opts *connector.ConnectOptions) (net.Conn, error) {
	retryConfig := DefaultRetryConfig()

	// Override with metadata configuration if available
	if c.md.retryMaxAttempts > 0 {
		retryConfig.MaxAttempts = c.md.retryMaxAttempts
	}
	if c.md.retryInitialDelay > 0 {
		retryConfig.InitialDelay = c.md.retryInitialDelay
	}
	if c.md.retryMaxDelay > 0 {
		retryConfig.MaxDelay = c.md.retryMaxDelay
	}

	var lastErr error

	for attempt := 1; attempt <= retryConfig.MaxAttempts; attempt++ {
		attemptLog := log.WithFields(map[string]any{
			"attempt":      attempt,
			"max_attempts": retryConfig.MaxAttempts,
		})

		if attempt > 1 {
			attemptLog.Debugf("retrying UDP connection attempt %d/%d", attempt, retryConfig.MaxAttempts)
		}

		result, err := c.connectUDPAttempt(ctx, conn, network, address, attemptLog, opts)
		if err == nil {
			if attempt > 1 {
				attemptLog.Infof("UDP connection succeeded on attempt %d/%d", attempt, retryConfig.MaxAttempts)
			}
			return result, nil
		}

		lastErr = err

		// Check if error is retriable
		if !isRetriableError(err) {
			attemptLog.WithFields(map[string]any{
				"error": err.Error(),
			}).Debug("UDP error is not retriable, aborting retries")
			break
		}

		// Don't retry if this is the last attempt
		if attempt >= retryConfig.MaxAttempts {
			break
		}

		// Check if context is cancelled
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Calculate backoff delay
		delay := calculateBackoffDelay(attempt, retryConfig)
		attemptLog.WithFields(map[string]any{
			"error": err.Error(),
			"delay": delay.String(),
		}).Warnf("UDP connection attempt %d/%d failed, retrying in %v", attempt, retryConfig.MaxAttempts, delay)

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return nil, fmt.Errorf("all %d UDP connection attempts failed, last error: %w", retryConfig.MaxAttempts, lastErr)
}

func (c *socks5Connector) connectUDPAttempt(ctx context.Context, conn net.Conn, network, address string, log logger.Logger, opts *connector.ConnectOptions) (net.Conn, error) {
	addr, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	if c.md.relay == "udp" {
		return c.relayUDP(ctx, conn, addr, log, opts)
	}

	req := gosocks5.NewRequest(socks.CmdUDPTun, nil)
	log.Trace(req)
	if err := req.Write(conn); err != nil {
		log.Error(err)
		return nil, err
	}

	reply, err := gosocks5.ReadReply(conn)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	log.Trace(reply)

	if reply.Rep != gosocks5.Succeeded {
		err = socks5ReplyError(reply.Rep)
		log.WithFields(map[string]any{
			"reply_code": fmt.Sprintf("0x%02x", reply.Rep),
			"target":     address,
		}).Error(err)
		return nil, fmt.Errorf("UDP tunnel setup failed: %w", err)
	}

	return socks.UDPTunClientConn(conn, addr), nil
}

func (c *socks5Connector) relayUDP(ctx context.Context, conn net.Conn, addr net.Addr, log logger.Logger, opts *connector.ConnectOptions) (net.Conn, error) {
	req := gosocks5.NewRequest(gosocks5.CmdUdp, nil)
	log.Trace(req)
	if err := req.Write(conn); err != nil {
		log.Error(err)
		return nil, err
	}

	reply, err := gosocks5.ReadReply(conn)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	log.Trace(reply)

	if reply.Rep != gosocks5.Succeeded {
		err = socks5ReplyError(reply.Rep)
		log.WithFields(map[string]any{
			"reply_code": fmt.Sprintf("0x%02x", reply.Rep),
			"target":     addr.String(),
		}).Error(err)
		return nil, fmt.Errorf("UDP relay setup failed: %w", err)
	}

	log.Debugf("bind on: %v", reply.Addr)

	cc, err := opts.Dialer.Dial(ctx, "udp", reply.Addr.String())
	if err != nil {
		c.options.Logger.Error(err)
		return nil, err
	}
	log.Debugf("%s <- %s -> %s", cc.LocalAddr(), cc.RemoteAddr(), addr)

	if c.md.udpTimeout > 0 {
		cc.SetReadDeadline(time.Now().Add(c.md.udpTimeout))
	}

	return &udpRelayConn{
		udpConn:    cc.(*net.UDPConn),
		tcpConn:    conn,
		taddr:      addr,
		bufferSize: c.md.udpBufferSize,
		logger:     log,
	}, nil
}
