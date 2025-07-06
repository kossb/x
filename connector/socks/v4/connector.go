package v4

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-gost/core/connector"
	md "github.com/go-gost/core/metadata"
	"github.com/go-gost/gosocks4"
	ctxvalue "github.com/go-gost/x/ctx"
	"github.com/go-gost/x/registry"
)

func init() {
	registry.ConnectorRegistry().Register("socks4", NewConnector)
	registry.ConnectorRegistry().Register("socks4a", NewConnector)
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
	log := c.options.Logger.WithFields(map[string]any{
		"remote":  conn.RemoteAddr().String(),
		"local":   conn.LocalAddr().String(),
		"network": network,
		"address": address,
		"sid":     string(ctxvalue.SidFromContext(ctx)),
	})
	log.Debugf("connect %s/%s", address, network)

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
