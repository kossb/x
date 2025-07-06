package v4

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/go-gost/core/connector"
	"github.com/go-gost/core/logger"
	md "github.com/go-gost/core/metadata"
	"github.com/go-gost/gosocks4"
	ctxvalue "github.com/go-gost/x/ctx"
	"github.com/go-gost/x/registry"
)

func init() {
	registry.ConnectorRegistry().Register("socks4", NewConnector)
	registry.ConnectorRegistry().Register("socks4a", NewConnector)
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

// isRetriableError determines if a SOCKS4 error should be retried
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

	// SOCKS4 reply errors that might be transient
	switch errStr {
	case "request rejected or failed":
		return true // Might be temporary server issue
	case "request failed":
		return true // Could be temporary
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

// socks4ReplyError maps SOCKS4 reply codes to meaningful error messages
func socks4ReplyError(code uint8) error {
	switch code {
	case gosocks4.Granted:
		return nil
	case gosocks4.Rejected:
		return errors.New("request rejected or failed")
	case gosocks4.RejectedUserid:
		return errors.New("request rejected because the client program and identd report different user-ids")
	case gosocks4.Failed:
		return errors.New("request failed")
	default:
		return fmt.Errorf("unknown SOCKS4 reply code: 0x%02x", code)
	}
}

type socks4Connector struct {
	md      metadata
	options connector.Options
}

func NewConnector(opts ...connector.Option) connector.Connector {
	options := connector.Options{}
	for _, opt := range opts {
		opt(&options)
	}

	return &socks4Connector{
		options: options,
	}
}

func (c *socks4Connector) Init(md md.Metadata) (err error) {
	return c.parseMetadata(md)
}

func (c *socks4Connector) Connect(ctx context.Context, conn net.Conn, network, address string, opts ...connector.ConnectOption) (net.Conn, error) {
	return c.connectWithRetry(ctx, conn, network, address, opts...)
}

func (c *socks4Connector) connectWithRetry(ctx context.Context, conn net.Conn, network, address string, opts ...connector.ConnectOption) (net.Conn, error) {
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
			attemptLog.Debugf("retrying SOCKS4 connection attempt %d/%d", attempt, retryConfig.MaxAttempts)
		} else {
			attemptLog.Debugf("connect %s/%s", address, network)
		}

		result, err := c.connectAttempt(ctx, conn, network, address, attemptLog, opts...)
		if err == nil {
			if attempt > 1 {
				attemptLog.Infof("SOCKS4 connection succeeded on attempt %d/%d", attempt, retryConfig.MaxAttempts)
			}
			return result, nil
		}

		lastErr = err

		// Check if error is retriable
		if !isRetriableError(err) {
			attemptLog.WithFields(map[string]any{
				"error": err.Error(),
			}).Debug("SOCKS4 error is not retriable, aborting retries")
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
		}).Warnf("SOCKS4 connection attempt %d/%d failed, retrying in %v", attempt, retryConfig.MaxAttempts, delay)

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return nil, fmt.Errorf("all %d SOCKS4 connection attempts failed, last error: %w", retryConfig.MaxAttempts, lastErr)
}

func (c *socks4Connector) connectAttempt(ctx context.Context, conn net.Conn, network, address string, log logger.Logger, opts ...connector.ConnectOption) (net.Conn, error) {
	switch network {
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

	var addr *gosocks4.Addr

	if c.md.disable4a {
		taddr, err := net.ResolveTCPAddr("tcp4", address)
		if err != nil {
			log.Error("resolve: ", err)
			return nil, err
		}
		if len(taddr.IP) == 0 {
			taddr.IP = net.IPv4zero
		}
		addr = &gosocks4.Addr{
			Type: gosocks4.AddrIPv4,
			Host: taddr.IP.String(),
			Port: uint16(taddr.Port),
		}
	} else {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		p, _ := strconv.Atoi(port)
		addr = &gosocks4.Addr{
			Type: gosocks4.AddrDomain,
			Host: host,
			Port: uint16(p),
		}
	}

	if c.md.connectTimeout > 0 {
		conn.SetDeadline(time.Now().Add(c.md.connectTimeout))
		defer conn.SetDeadline(time.Time{})
	}

	var userid []byte
	if c.options.Auth != nil {
		userid = []byte(c.options.Auth.Username())
	}
	req := gosocks4.NewRequest(gosocks4.CmdConnect, addr, userid)
	log.Trace(req)
	if err := req.Write(conn); err != nil {
		log.Error(err)
		return nil, err
	}

	reply, err := gosocks4.ReadReply(conn)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	log.Trace(reply)

	if reply.Code != gosocks4.Granted {
		err = socks4ReplyError(reply.Code)
		log.WithFields(map[string]any{
			"reply_code": fmt.Sprintf("0x%02x", reply.Code),
			"target":     address,
		}).Error(err)
		return nil, err
	}

	return conn, nil
}
