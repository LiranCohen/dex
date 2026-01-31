package main

import (
	"crypto/subtle"
	"errors"
	"sync"
	"time"
)

var (
	ErrRateLimited   = errors.New("too many attempts, please wait")
	ErrInvalidPIN    = errors.New("invalid PIN")
	ErrPINNotSet     = errors.New("PIN not configured")
)

// PINVerifier handles PIN verification with rate limiting
type PINVerifier struct {
	pin         string
	attempts    int
	lastAttempt time.Time
	maxAttempts int
	windowDur   time.Duration
	mu          sync.Mutex
}

// NewPINVerifier creates a new PIN verifier
func NewPINVerifier(pin string) *PINVerifier {
	return &PINVerifier{
		pin:         pin,
		maxAttempts: 5,
		windowDur:   time.Minute,
	}
}

// Verify checks if the provided PIN is correct
// Returns nil on success, or an appropriate error
func (p *PINVerifier) Verify(input string) error {
	if p.pin == "" {
		return ErrPINNotSet
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Reset attempts if window has passed
	if time.Since(p.lastAttempt) >= p.windowDur {
		p.attempts = 0
	}

	// Check rate limit
	if p.attempts >= p.maxAttempts {
		return ErrRateLimited
	}

	// Record attempt
	p.attempts++
	p.lastAttempt = time.Now()

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(input), []byte(p.pin)) != 1 {
		return ErrInvalidPIN
	}

	// Success - reset attempts
	p.attempts = 0
	return nil
}

// AttemptsRemaining returns how many attempts are left before rate limiting
func (p *PINVerifier) AttemptsRemaining() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Reset if window has passed
	if time.Since(p.lastAttempt) >= p.windowDur {
		p.attempts = 0
	}

	remaining := p.maxAttempts - p.attempts
	if remaining < 0 {
		return 0
	}
	return remaining
}

// TimeUntilReset returns how long until the rate limit resets
func (p *PINVerifier) TimeUntilReset() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.attempts < p.maxAttempts {
		return 0
	}

	elapsed := time.Since(p.lastAttempt)
	if elapsed >= p.windowDur {
		return 0
	}

	return p.windowDur - elapsed
}
