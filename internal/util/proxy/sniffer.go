package proxy

import (
	"net"
	"sync"
	"time"

	"github.com/go-gost/core/logger"
	"github.com/go-gost/core/metrics"
	xmetrics "github.com/go-gost/x/metrics"
)

// TTFBConn wraps a connection to measure time to first byte from proxy nodes
type TTFBConn struct {
	net.Conn
	connectorType string
	logger        logger.Logger

	// State for TTFB measurement
	firstReadDone sync.Once
	startTime     time.Time
	ttfbMeasured  bool
	mu            sync.RWMutex
}

// WrapConnection wraps a connection for TTFB measurement
func WrapConnection(conn net.Conn, connectorType string, logger logger.Logger) net.Conn {
	if !xmetrics.IsEnabled() || conn == nil {
		return conn
	}

	return &TTFBConn{
		Conn:          conn,
		connectorType: connectorType,
		logger:        logger,
		startTime:     time.Now(),
	}
}

func (c *TTFBConn) Read(b []byte) (n int, err error) {
	c.firstReadDone.Do(func() {
		// Record time to first byte from proxy node
		if observer := xmetrics.GetObserver(
			xmetrics.MetricProxyNodeTTFBObserver,
			metrics.Labels{
				"connector_type": c.connectorType,
			}); observer != nil {
			observer.Observe(time.Since(c.startTime).Seconds())
		}

		c.mu.Lock()
		c.ttfbMeasured = true
		c.mu.Unlock()

		if c.logger != nil {
			c.logger.Debugf("TTFB connection from proxy %s (%s): %v",
				c.Conn.RemoteAddr().String(),
				c.connectorType,
				time.Since(c.startTime))
		}
	})

	return c.Conn.Read(b)
}

func (c *TTFBConn) Write(b []byte) (n int, err error) {
	// Reset start time on first write (when we send the request)
	c.mu.Lock()
	if !c.ttfbMeasured {
		c.startTime = time.Now()
	}
	c.mu.Unlock()

	return c.Conn.Write(b)
}

func (c *TTFBConn) Close() error {
	// Record round trip time from connection start to close
	if observer := xmetrics.GetObserver(
		xmetrics.MetricProxyNodeRoundTripObserver,
		metrics.Labels{
			"connector_type": c.connectorType,
		}); observer != nil {
		observer.Observe(time.Since(c.startTime).Seconds())
	}

	if c.logger != nil {
		c.logger.Debugf("Round trip to proxy %s (%s): %v",
			c.Conn.RemoteAddr().String(),
			c.connectorType,
			time.Since(c.startTime))
	}

	return c.Conn.Close()
}

// IsTTFBMeasured returns true if TTFB has been measured
func (c *TTFBConn) IsTTFBMeasured() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ttfbMeasured
}

// GetConnectorType returns the connector type
func (c *TTFBConn) GetConnectorType() string {
	return c.connectorType
}
