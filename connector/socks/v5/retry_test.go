package v5

import (
	"errors"
	"testing"
	"time"
)

func TestIsRetriableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "general SOCKS server failure",
			err:      errors.New("general SOCKS server failure"),
			expected: true,
		},
		{
			name:     "network unreachable",
			err:      errors.New("network unreachable"),
			expected: true,
		},
		{
			name:     "host unreachable",
			err:      errors.New("host unreachable"),
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "TTL expired",
			err:      errors.New("TTL expired"),
			expected: true,
		},
		{
			name:     "connection not allowed by ruleset",
			err:      errors.New("connection not allowed by ruleset"),
			expected: false,
		},
		{
			name:     "command not supported",
			err:      errors.New("command not supported"),
			expected: false,
		},
		{
			name:     "address type not supported",
			err:      errors.New("address type not supported"),
			expected: false,
		},
		{
			name:     "unknown error",
			err:      errors.New("some random error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetriableError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetriableError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsRetriableSocks5ReplyCode(t *testing.T) {
	tests := []struct {
		name     string
		code     uint8
		expected bool
	}{
		{
			name:     "succeeded",
			code:     0x00,
			expected: false,
		},
		{
			name:     "general SOCKS server failure",
			code:     0x01,
			expected: true,
		},
		{
			name:     "connection not allowed by ruleset",
			code:     0x02,
			expected: false,
		},
		{
			name:     "network unreachable",
			code:     0x03,
			expected: true,
		},
		{
			name:     "host unreachable",
			code:     0x04,
			expected: true,
		},
		{
			name:     "connection refused",
			code:     0x05,
			expected: true,
		},
		{
			name:     "TTL expired",
			code:     0x06,
			expected: true,
		},
		{
			name:     "command not supported",
			code:     0x07,
			expected: false,
		},
		{
			name:     "address type not supported",
			code:     0x08,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetriableSocks5ReplyCode(tt.code)
			if result != tt.expected {
				t.Errorf("isRetriableSocks5ReplyCode(0x%02x) = %v, expected %v", tt.code, result, tt.expected)
			}
		})
	}
}

func TestCalculateBackoffDelay(t *testing.T) {
	config := RetryConfig{
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
	}

	tests := []struct {
		name     string
		attempt  int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{
			name:     "attempt 0",
			attempt:  0,
			minDelay: 90 * time.Millisecond,  // -10% jitter
			maxDelay: 110 * time.Millisecond, // +10% jitter
		},
		{
			name:     "attempt 1",
			attempt:  1,
			minDelay: 90 * time.Millisecond,  // -10% jitter
			maxDelay: 110 * time.Millisecond, // +10% jitter
		},
		{
			name:     "attempt 2",
			attempt:  2,
			minDelay: 180 * time.Millisecond, // -10% jitter
			maxDelay: 220 * time.Millisecond, // +10% jitter
		},
		{
			name:     "attempt 3",
			attempt:  3,
			minDelay: 360 * time.Millisecond, // -10% jitter
			maxDelay: 440 * time.Millisecond, // +10% jitter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := calculateBackoffDelay(tt.attempt, config)

			if delay < tt.minDelay || delay > tt.maxDelay {
				t.Errorf("calculateBackoffDelay(%d) = %v, expected between %v and %v",
					tt.attempt, delay, tt.minDelay, tt.maxDelay)
			}

			// Ensure delay never exceeds maxDelay
			if delay > config.MaxDelay {
				t.Errorf("calculateBackoffDelay(%d) = %v, exceeded maxDelay %v",
					tt.attempt, delay, config.MaxDelay)
			}

			// Ensure delay is never less than initialDelay
			if delay < config.InitialDelay {
				t.Errorf("calculateBackoffDelay(%d) = %v, less than initialDelay %v",
					tt.attempt, delay, config.InitialDelay)
			}
		})
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxAttempts != 3 {
		t.Errorf("DefaultRetryConfig().MaxAttempts = %d, expected 3", config.MaxAttempts)
	}

	if config.InitialDelay != 100*time.Millisecond {
		t.Errorf("DefaultRetryConfig().InitialDelay = %v, expected 100ms", config.InitialDelay)
	}

	if config.MaxDelay != 5*time.Second {
		t.Errorf("DefaultRetryConfig().MaxDelay = %v, expected 5s", config.MaxDelay)
	}

	if config.BackoffFactor != 2.0 {
		t.Errorf("DefaultRetryConfig().BackoffFactor = %f, expected 2.0", config.BackoffFactor)
	}

	if config.JitterFactor != 0.1 {
		t.Errorf("DefaultRetryConfig().JitterFactor = %f, expected 0.1", config.JitterFactor)
	}
}

func TestSocks5ReplyError(t *testing.T) {
	tests := []struct {
		name        string
		code        uint8
		expectedMsg string
	}{
		{
			name:        "succeeded",
			code:        0x00,
			expectedMsg: "",
		},
		{
			name:        "general SOCKS server failure",
			code:        0x01,
			expectedMsg: "general SOCKS server failure",
		},
		{
			name:        "connection not allowed by ruleset",
			code:        0x02,
			expectedMsg: "connection not allowed by ruleset",
		},
		{
			name:        "network unreachable",
			code:        0x03,
			expectedMsg: "network unreachable",
		},
		{
			name:        "host unreachable",
			code:        0x04,
			expectedMsg: "host unreachable",
		},
		{
			name:        "connection refused",
			code:        0x05,
			expectedMsg: "connection refused",
		},
		{
			name:        "TTL expired",
			code:        0x06,
			expectedMsg: "TTL expired",
		},
		{
			name:        "command not supported",
			code:        0x07,
			expectedMsg: "command not supported",
		},
		{
			name:        "address type not supported",
			code:        0x08,
			expectedMsg: "address type not supported",
		},
		{
			name:        "unknown reply code",
			code:        0xFF,
			expectedMsg: "unknown SOCKS5 reply code: 0xff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := socks5ReplyError(tt.code)

			if tt.expectedMsg == "" {
				if err != nil {
					t.Errorf("socks5ReplyError(0x%02x) = %v, expected nil", tt.code, err)
				}
			} else {
				if err == nil {
					t.Errorf("socks5ReplyError(0x%02x) = nil, expected error", tt.code)
				} else if err.Error() != tt.expectedMsg {
					t.Errorf("socks5ReplyError(0x%02x) = %q, expected %q", tt.code, err.Error(), tt.expectedMsg)
				}
			}
		})
	}
}

// Example test showing retry behavior simulation
func TestRetryBehaviorSimulation(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  50 * time.Millisecond,
		MaxDelay:      1 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
	}

	// Simulate retry attempts
	var totalDelay time.Duration

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		if attempt > 1 {
			delay := calculateBackoffDelay(attempt-1, config)
			totalDelay += delay
			t.Logf("Attempt %d: would wait %v (total delay so far: %v)",
				attempt, delay, totalDelay)
		} else {
			t.Logf("Attempt %d: immediate", attempt)
		}
	}

	// Verify total delay is reasonable
	expectedMinDelay := config.InitialDelay * time.Duration(config.MaxAttempts-1)
	if totalDelay < expectedMinDelay {
		t.Errorf("Total delay %v is less than expected minimum %v", totalDelay, expectedMinDelay)
	}

	// Verify total delay doesn't exceed reasonable bounds
	maxPossibleDelay := config.MaxDelay * time.Duration(config.MaxAttempts-1) * 2 // 2x factor for jitter
	if totalDelay > maxPossibleDelay {
		t.Errorf("Total delay %v exceeds reasonable maximum %v", totalDelay, maxPossibleDelay)
	}
}
