package ctx

import (
	"bytes"
	"context"

	"github.com/go-gost/core/logger"
)

// clientAddrKey saves the client address.
type clientAddrKey struct{}

type ClientAddr string

var (
	keyClientAddr clientAddrKey
)

func ContextWithClientAddr(ctx context.Context, addr ClientAddr) context.Context {
	return context.WithValue(ctx, keyClientAddr, addr)
}

func ClientAddrFromContext(ctx context.Context) ClientAddr {
	v, _ := ctx.Value(keyClientAddr).(ClientAddr)
	return v
}

// sidKey saves the session ID.
type sidKey struct{}
type Sid string

var (
	keySid sidKey
)

func ContextWithSid(ctx context.Context, sid Sid) context.Context {
	return context.WithValue(ctx, keySid, sid)
}

func SidFromContext(ctx context.Context) Sid {
	v, _ := ctx.Value(keySid).(Sid)
	return v
}

// hashKey saves the hash source for Selector.
type hashKey struct{}

type Hash struct {
	Source string
}

var (
	clientHashKey = &hashKey{}
)

func ContextWithHash(ctx context.Context, hash *Hash) context.Context {
	return context.WithValue(ctx, clientHashKey, hash)
}

func HashFromContext(ctx context.Context) *Hash {
	if v, _ := ctx.Value(clientHashKey).(*Hash); v != nil {
		return v
	}
	return nil
}

type handlerKey struct{}
type Handler string

var (
	keyHandler = &handlerKey{}
)

func ContextWithHandler(ctx context.Context, handler Handler) context.Context {
	return context.WithValue(ctx, keyHandler, handler)
}

func HandlerFromContext(ctx context.Context) Handler {
	v, _ := ctx.Value(keyHandler).(Handler)
	return v
}

type localAddrKey struct{}
type LocalAddr string

var (
	keyLocalAddr = &localAddrKey{}
)

func ContextWithLocalAddr(ctx context.Context, addr LocalAddr) context.Context {
	return context.WithValue(ctx, keyLocalAddr, addr)
}

func LocalAddrFromContext(ctx context.Context) LocalAddr {
	v, _ := ctx.Value(keyLocalAddr).(LocalAddr)
	return v
}

type serviceKey struct{}
type Service string

var (
	keyService = &serviceKey{}
)

func ContextWithService(ctx context.Context, service Service) context.Context {
	return context.WithValue(ctx, keyService, service)
}
func ServiceFromContext(ctx context.Context) Service {
	v, _ := ctx.Value(keyService).(Service)
	return v
}

type clientIDKey struct{}
type ClientID string

var (
	keyClientID = &clientIDKey{}
)

func ContextWithClientID(ctx context.Context, clientID ClientID) context.Context {
	return context.WithValue(ctx, keyClientID, clientID)
}

func ClientIDFromContext(ctx context.Context) ClientID {
	v, _ := ctx.Value(keyClientID).(ClientID)
	return v
}

type bufferKey struct{}

var (
	keyBuffer = &bufferKey{}
)

func ContextWithBuffer(ctx context.Context, buffer *bytes.Buffer) context.Context {
	return context.WithValue(ctx, keyBuffer, buffer)
}

func BufferFromContext(ctx context.Context) *bytes.Buffer {
	v, _ := ctx.Value(keyBuffer).(*bytes.Buffer)
	return v
}

type loggerKey struct{}

var (
	keyLogger = &loggerKey{}
)

func ContextWithLogger(ctx context.Context, log logger.Logger) context.Context {
	return context.WithValue(ctx, keyLogger, log)
}

func LoggerFromContext(ctx context.Context) logger.Logger {
	v, _ := ctx.Value(keyLogger).(logger.Logger)
	return v
}
