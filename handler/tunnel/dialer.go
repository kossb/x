package tunnel

import (
	"context"
	"net"
	"time"

	"github.com/go-gost/core/logger"
	"github.com/go-gost/core/sd"
)

type Dialer struct {
	node    string
	pool    *ConnectorPool
	sd      sd.SD
	retry   int
	timeout time.Duration
	log     logger.Logger
}

func (d *Dialer) Dial(ctx context.Context, network string, tid string) (conn net.Conn, node string, cid string, err error) {
	retry := d.retry
	if retry <= 0 {
		retry = 3 // Increased default from 1 to 3 for better reliability under load
	}

	for i := 0; i < retry; i++ {
		c := d.pool.Get(network, tid)
		if c == nil {
			break
		}

		conn, err = c.GetConn()
		if err != nil {
			d.log.Error(err)
			continue
		}
		node = d.node
		cid = c.id.String()

		break
	}
	if conn != nil || err != nil {
		return
	}

	if d.sd == nil {
		err = ErrTunnelNotAvailable
		return
	}

	ss, err := d.sd.Get(ctx, tid)
	if err != nil {
		return
	}

	var service *sd.Service
	for _, s := range ss {
		d.log.Debugf("%+v", s)
		if s.Node != d.node && s.Network == network {
			service = s
			break
		}
	}
	if service == nil || service.Address == "" {
		err = ErrTunnelNotAvailable
		return
	}

	node = service.Node
	cid = service.ID

	timeout := d.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second // Increased default from 15s to 30s for high load
	}

	dialer := net.Dialer{
		Timeout: timeout,
	}
	conn, err = dialer.DialContext(ctx, "tcp", service.Address)
	return
}
