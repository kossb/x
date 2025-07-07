package auth

import (
	"context"
	"io"
	"time"

	"github.com/go-gost/core/auth"
	"github.com/go-gost/core/logger"
	"github.com/go-gost/core/metrics"
	"github.com/go-gost/plugin/auth/proto"
	ctxvalue "github.com/go-gost/x/ctx"
	"github.com/go-gost/x/internal/plugin"
	xmetrics "github.com/go-gost/x/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type grpcPlugin struct {
	conn   grpc.ClientConnInterface
	client proto.AuthenticatorClient
	log    logger.Logger
}

// NewGRPCPlugin creates an Authenticator plugin based on gRPC.
func NewGRPCPlugin(name string, addr string, opts ...plugin.Option) auth.Authenticator {
	var options plugin.Options
	for _, opt := range opts {
		opt(&options)
	}

	log := logger.Default().WithFields(map[string]any{
		"kind":   "auther",
		"auther": name,
	})
	conn, err := plugin.NewGRPCConn(addr, &options)
	if err != nil {
		log.Error(err)
	}

	p := &grpcPlugin{
		conn: conn,
		log:  log,
	}

	if conn != nil {
		p.client = proto.NewAuthenticatorClient(conn)
	}
	return p
}

// Authenticate checks the validity of the provided user-password pair.
func (p *grpcPlugin) Authenticate(ctx context.Context, user, password string, opts ...auth.Option) (string, bool) {
	if p.client == nil {
		return "", false
	}

	handler := ctxvalue.HandlerFromContext(ctx)
	if handler != "" {
		md := metadata.New(map[string]string{
			"handler-type": string(handler),
		})
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	// Start timing
	start := time.Now()

	r, err := p.client.Authenticate(ctx,
		&proto.AuthenticateRequest{
			Username: user,
			Password: password,
			Client:   string(ctxvalue.ClientAddrFromContext(ctx)),
		})

	// Record metrics
	duration := time.Since(start).Seconds()
	statusCode := "OK"
	if err != nil {
		if st, ok := status.FromError(err); ok {
			statusCode = st.Code().String()
		} else {
			statusCode = "UNKNOWN"
		}
	}

	// Record duration
	if observer := xmetrics.GetObserver(
		xmetrics.MetricGRPCRequestsDurationObserver,
		metrics.Labels{
			"service": "auther",
			"method":  "Authenticate",
		}); observer != nil {
		observer.Observe(duration)
	}

	// Record request count with status
	if counter := xmetrics.GetCounter(
		xmetrics.MetricGRPCRequestsCounter,
		metrics.Labels{
			"service":     "auther",
			"method":      "Authenticate",
			"status_code": statusCode,
		}); counter != nil {
		counter.Inc()
	}

	if err != nil {
		p.log.Error(err)
		return "", false
	}
	return r.Id, r.Ok
}

func (p *grpcPlugin) Close() error {
	if closer, ok := p.conn.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
