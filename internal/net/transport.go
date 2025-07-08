package net

import (
	"errors"
	"io"
	"log"
	"time"

	"github.com/go-gost/core/common/bufpool"
	"github.com/go-gost/core/metrics"
	xmetrics "github.com/go-gost/x/metrics"
)

const (
	// Default buffer size optimized for high-performance proxy operations
	// 256KB provides excellent throughput for 1Gbps+ networks while maintaining
	// reasonable memory usage with buffer pooling
	bufferSize = 256 * 1024 // 256KB (was 64KB)
)

func Transport(rw1, rw2 io.ReadWriter) error {
	errc := make(chan error, 1)
	go func() {
		errc <- CopyBufferWithTTFB(rw1, rw2, bufferSize, "client_to_server")
	}()

	go func() {
		errc <- CopyBufferWithTTFB(rw2, rw1, bufferSize, "server_to_client")
	}()

	if err := <-errc; err != nil && err != io.EOF {
		return err
	}

	return nil
}

func CopyBuffer(dst io.Writer, src io.Reader, bufSize int) error {
	buf := bufpool.Get(bufSize)
	defer bufpool.Put(buf)

	_, err := io.CopyBuffer(dst, src, buf)
	return err
}

// CopyBufferWithTTFB copies data with TTFB measurement
func CopyBufferWithTTFB(dst io.Writer, src io.Reader, bufSize int, direction string) error {
	buf := bufpool.Get(bufSize)
	defer bufpool.Put(buf)

	// Measure time to first byte
	start := time.Now()
	firstByteRead := false

	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			// Record TTFB on first successful read
			if !firstByteRead && xmetrics.IsEnabled() {
				if observer := xmetrics.GetObserver(
					xmetrics.MetricTransportTTFBObserver,
					metrics.Labels{
						"direction": direction,
					}); observer != nil {
					observer.Observe(time.Since(start).Seconds())
					log.Printf("TTFB: %v, direction: %v", time.Since(start).Seconds(), direction)
				}
				firstByteRead = true
			}

			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			if ew != nil {
				return ew
			}
			if nr != nw {
				return io.ErrShortWrite
			}
		}
		if er != nil {
			if er != io.EOF {
				return er
			}
			break
		}
	}
	return nil
}
