// Package retry provides utilities for retrying operations with exponential backoff.
package retry

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Config holds retry configuration.
type Config struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// Option is a functional option for retry configuration.
type Option func(*Config)

// WithExponentialBackoff executes the operation with exponential backoff retry.
// It retries the operation up to MaxRetries times, with exponentially increasing
// delays between attempts. Context cancellation is respected throughout.
//
// Errors wrapped with Fatal() are not retried.
func WithExponentialBackoff(ctx context.Context, operation func() error, opts ...Option) error {
	cfg := &Config{
		MaxRetries:   5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	delay := cfg.InitialDelay
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is fatal (non-retryable)
		if IsFatal(err) {
			return fmt.Errorf("fatal error (not retrying): %w", err)
		}

		if attempt < cfg.MaxRetries {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled after %d attempts: %w", attempt+1, ctx.Err())
			case <-time.After(delay):
				delay = time.Duration(float64(delay) * cfg.Multiplier)
				if delay > cfg.MaxDelay {
					delay = cfg.MaxDelay
				}
			}
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", cfg.MaxRetries+1, lastErr)
}

// WithMaxRetries sets the maximum number of retries.
func WithMaxRetries(n int) Option {
	return func(c *Config) {
		c.MaxRetries = n
	}
}

// WithInitialDelay sets the initial delay between retries.
func WithInitialDelay(d time.Duration) Option {
	return func(c *Config) {
		c.InitialDelay = d
	}
}

// WithMaxDelay sets the maximum delay between retries.
func WithMaxDelay(d time.Duration) Option {
	return func(c *Config) {
		c.MaxDelay = d
	}
}

// WithMultiplier sets the backoff multiplier.
func WithMultiplier(m float64) Option {
	return func(c *Config) {
		c.Multiplier = m
	}
}

// FatalError wraps an error to mark it as fatal (non-retryable).
type FatalError struct {
	Err error
}

func (e *FatalError) Error() string {
	return e.Err.Error()
}

func (e *FatalError) Unwrap() error {
	return e.Err
}

// Fatal marks an error as fatal (non-retryable).
// Operations that encounter fatal errors will not be retried.
func Fatal(err error) error {
	if err == nil {
		return nil
	}
	return &FatalError{Err: err}
}

// IsFatal checks if an error is fatal (non-retryable).
func IsFatal(err error) bool {
	var fatalErr *FatalError
	return errors.As(err, &fatalErr)
}
