package v4

import (
	"time"

	mdata "github.com/go-gost/core/metadata"
	mdutil "github.com/go-gost/x/metadata/util"
)

type metadata struct {
	connectTimeout    time.Duration
	disable4a         bool
	retryMaxAttempts  int
	retryInitialDelay time.Duration
	retryMaxDelay     time.Duration
}

func (c *socks4Connector) parseMetadata(md mdata.Metadata) (err error) {
	const (
		connectTimeout = "timeout"
		disable4a      = "disable4a"
	)

	c.md.connectTimeout = mdutil.GetDuration(md, connectTimeout)
	c.md.disable4a = mdutil.GetBool(md, disable4a)

	// Parse retry configuration
	c.md.retryMaxAttempts = mdutil.GetInt(md, "retry.maxAttempts", "retryMaxAttempts")
	c.md.retryInitialDelay = mdutil.GetDuration(md, "retry.initialDelay", "retryInitialDelay")
	c.md.retryMaxDelay = mdutil.GetDuration(md, "retry.maxDelay", "retryMaxDelay")

	return
}
