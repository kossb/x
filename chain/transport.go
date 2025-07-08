package chain

import (
	"context"
	"net"
	"reflect"
	"strings"

	"github.com/go-gost/core/chain"
	"github.com/go-gost/core/connector"
	"github.com/go-gost/core/dialer"
	"github.com/go-gost/core/logger"
	net_dialer "github.com/go-gost/x/internal/net/dialer"
	proxy_sniffer "github.com/go-gost/x/internal/util/proxy"
)

type Transport struct {
	dialer    dialer.Dialer
	connector connector.Connector
	options   chain.TransportOptions
}

func NewTransport(d dialer.Dialer, c connector.Connector, opts ...chain.TransportOption) *Transport {
	tr := &Transport{
		dialer:    d,
		connector: c,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&tr.options)
		}
	}

	return tr
}

func (tr *Transport) Dial(ctx context.Context, addr string) (net.Conn, error) {
	netd := &net_dialer.Dialer{
		Interface: tr.options.IfceName,
		Netns:     tr.options.Netns,
	}
	if tr.options.SockOpts != nil {
		netd.Mark = tr.options.SockOpts.Mark
	}
	if tr.options.Route != nil && len(tr.options.Route.Nodes()) > 0 {
		netd.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return tr.options.Route.Dial(ctx, network, addr)
		}
	}
	opts := []dialer.DialOption{
		dialer.HostDialOption(tr.options.Addr),
		dialer.NetDialerDialOption(netd),
	}
	return tr.dialer.Dial(ctx, addr, opts...)
}

func (tr *Transport) Handshake(ctx context.Context, conn net.Conn) (net.Conn, error) {
	var err error
	if hs, ok := tr.dialer.(dialer.Handshaker); ok {
		conn, err = hs.Handshake(ctx, conn,
			dialer.AddrHandshakeOption(tr.options.Addr))
		if err != nil {
			return nil, err
		}
	}
	if hs, ok := tr.connector.(connector.Handshaker); ok {
		return hs.Handshake(ctx, conn)
	}
	return conn, nil
}

func (tr *Transport) Connect(ctx context.Context, conn net.Conn, network, address string) (net.Conn, error) {
	netd := &net_dialer.Dialer{
		Interface: tr.options.IfceName,
		Netns:     tr.options.Netns,
	}
	if tr.options.SockOpts != nil {
		netd.Mark = tr.options.SockOpts.Mark
	}

	// Determine connector type for TTFB measurement using reflection
	connectorType := "unknown"
	if tr.connector != nil {
		typeName := reflect.TypeOf(tr.connector).String()
		// Extract the package and type name (e.g., "*http.httpConnector" -> "http")
		if idx := strings.LastIndex(typeName, "."); idx > 0 {
			if idx2 := strings.LastIndex(typeName[:idx], "."); idx2 > 0 {
				connectorType = typeName[idx2+1 : idx]
			}
		}
	}

	// Wrap connection with TTFB sniffer for proxy node measurement
	wrappedConn := proxy_sniffer.WrapConnection(conn, connectorType, logger.Default())

	result, err := tr.connector.Connect(ctx, wrappedConn, network, address,
		connector.DialerConnectOption(netd),
	)

	// If connection failed, return the original error
	if err != nil {
		return nil, err
	}

	// Return the result connection (which may be further wrapped by the connector)
	return result, nil
}

func (tr *Transport) Bind(ctx context.Context, conn net.Conn, network, address string, opts ...connector.BindOption) (net.Listener, error) {
	if binder, ok := tr.connector.(connector.Binder); ok {
		return binder.Bind(ctx, conn, network, address, opts...)
	}
	return nil, connector.ErrBindUnsupported
}

func (tr *Transport) Multiplex() bool {
	if mux, ok := tr.dialer.(dialer.Multiplexer); ok {
		return mux.Multiplex()
	}
	return false
}

func (tr *Transport) Options() *chain.TransportOptions {
	if tr != nil {
		return &tr.options
	}
	return nil
}

func (tr *Transport) Copy() chain.Transporter {
	tr2 := &Transport{}
	*tr2 = *tr
	return tr
}
