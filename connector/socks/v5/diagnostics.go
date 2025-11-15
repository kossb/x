package v5

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/go-gost/core/logger"
	"github.com/go-gost/gosocks5"
)

// DiagnosticInfo provides detailed information about a SOCKS5 connection failure
type DiagnosticInfo struct {
	Target       string
	Network      string
	ReplyCode    uint8
	ReplyCodeHex string
	ErrorMessage string
	Suggestions  []string
}

// DiagnoseSocks5Error provides detailed diagnostic information for SOCKS5 errors
func DiagnoseSocks5Error(replyCode uint8, target, network string) *DiagnosticInfo {
	info := &DiagnosticInfo{
		Target:       target,
		Network:      network,
		ReplyCode:    replyCode,
		ReplyCodeHex: fmt.Sprintf("0x%02x", replyCode),
	}

	switch replyCode {
	case 0x01: // General SOCKS server failure
		info.Suggestions = []string{
			"Check if the SOCKS5 server is running and accessible",
			"Verify SOCKS5 server configuration",
			"Check server logs for internal errors",
			"Ensure the server has sufficient resources",
		}
	case 0x02: // Connection not allowed by ruleset
		info.Suggestions = []string{
			"Check SOCKS5 server access control rules",
			"Verify your client IP is allowed",
			"Check if authentication credentials are correct",
			"Review server firewall/ACL configuration",
		}
	case 0x03: // Network unreachable
		info.Suggestions = []string{
			"Check network connectivity from SOCKS5 server to target",
			"Verify routing tables on the SOCKS5 server",
			"Check if target network is accessible",
			"Verify DNS resolution is working",
		}
	case 0x04: // Host unreachable
		info.Suggestions = []string{
			"Verify the target host/IP address is correct",
			"Check if the target host is online and reachable",
			"Verify DNS resolution for the target hostname",
			"Check for network connectivity issues to the target",
			"Ensure there are no firewall blocks to the target",
		}
	case 0x05: // Connection refused
		info.Suggestions = []string{
			"Check if the target service is running on the specified port",
			"Verify the target port number is correct",
			"Check if the target service is accepting connections",
			"Review target host firewall configuration",
		}
	case 0x06: // TTL expired
		info.Suggestions = []string{
			"Check for routing loops in the network path",
			"Verify network routing configuration",
			"Check if the network path has too many hops",
			"Consider increasing TTL values if configurable",
		}
	case 0x07: // Command not supported
		info.Suggestions = []string{
			"Verify the SOCKS5 server supports the requested command",
			"Check if UDP ASSOCIATE is supported (if using UDP)",
			"Check if BIND command is supported (if using BIND)",
			"Review SOCKS5 server capabilities and configuration",
		}
	case 0x08: // Address type not supported
		info.Suggestions = []string{
			"Verify the SOCKS5 server supports the address type (IPv4/IPv6/domain)",
			"Check if IPv6 is properly configured if using IPv6 addresses",
			"Try using an IP address instead of a domain name",
			"Review SOCKS5 server address type support",
		}
	default:
		info.Suggestions = []string{
			"Check SOCKS5 server logs for more details",
			"Verify SOCKS5 protocol version compatibility",
			"Contact SOCKS5 server administrator",
		}
	}

	return info
}

// LogDiagnosticInfo logs comprehensive diagnostic information
func LogDiagnosticInfo(log logger.Logger, info *DiagnosticInfo) {
	log.WithFields(map[string]any{
		"target":        info.Target,
		"network":       info.Network,
		"reply_code":    info.ReplyCodeHex,
		"error_message": info.ErrorMessage,
	}).Error("SOCKS5 connection failed")

	log.Info("SOCKS5 Diagnostic Information:")
	log.Infof("  Target: %s (%s)", info.Target, info.Network)
	log.Infof("  Reply Code: %s (%d)", info.ReplyCodeHex, info.ReplyCode)
	log.Infof("  Error: %s", info.ErrorMessage)
	log.Info("  Troubleshooting suggestions:")
	for i, suggestion := range info.Suggestions {
		log.Infof("    %d. %s", i+1, suggestion)
	}
}

// TestSocks5Connectivity performs basic connectivity tests
func TestSocks5Connectivity(ctx context.Context, serverAddr string, timeout time.Duration, log logger.Logger) error {
	log.Infof("Testing SOCKS5 connectivity to %s...", serverAddr)

	// Test basic TCP connection
	conn, err := net.DialTimeout("tcp", serverAddr, timeout)
	if err != nil {
		log.Errorf("Failed to connect to SOCKS5 server: %v", err)
		return fmt.Errorf("SOCKS5 server connection failed: %w", err)
	}
	defer conn.Close()

	log.Infof("Successfully connected to SOCKS5 server at %s", serverAddr)

	// Test SOCKS5 handshake
	selector := &clientSelector{
		methods: []uint8{gosocks5.MethodNoAuth},
		logger:  log,
	}

	cc := gosocks5.ClientConn(conn, selector)
	if err := cc.Handleshake(); err != nil {
		log.Errorf("SOCKS5 handshake failed: %v", err)
		return fmt.Errorf("SOCKS5 handshake failed: %w", err)
	}

	log.Info("SOCKS5 handshake successful")
	return nil
}
