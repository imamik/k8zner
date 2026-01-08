package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithExponentialBackoff_Success(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return nil
	}

	ctx := context.Background()
	err := WithExponentialBackoff(ctx, operation)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got: %d", attempts)
	}
}

func TestWithExponentialBackoff_SuccessAfterRetries(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	ctx := context.Background()
	err := WithExponentialBackoff(ctx, operation, WithInitialDelay(10*time.Millisecond))

	if err != nil {
		t.Errorf("Expected no error after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got: %d", attempts)
	}
}

func TestWithExponentialBackoff_MaxRetries(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return errors.New("persistent error")
	}

	ctx := context.Background()
	maxRetries := 3
	err := WithExponentialBackoff(ctx, operation,
		WithMaxRetries(maxRetries),
		WithInitialDelay(10*time.Millisecond))

	if err == nil {
		t.Error("Expected error after max retries, got nil")
	}
	// MaxRetries is the number of retries after the first attempt
	// So total attempts = maxRetries + 1
	expectedAttempts := maxRetries + 1
	if attempts != expectedAttempts {
		t.Errorf("Expected %d attempts (1 + %d retries), got: %d", expectedAttempts, maxRetries, attempts)
	}
}

func TestWithExponentialBackoff_ContextCancellation(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return errors.New("error")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := WithExponentialBackoff(ctx, operation, WithInitialDelay(10*time.Millisecond))

	if err == nil {
		t.Error("Expected error due to context cancellation, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt before context check, got: %d", attempts)
	}
}

func TestWithExponentialBackoff_ContextTimeout(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return errors.New("error")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := WithExponentialBackoff(ctx, operation,
		WithInitialDelay(100*time.Millisecond),
		WithMaxRetries(10))

	if err == nil {
		t.Error("Expected error due to context timeout, got nil")
	}
	// Should timeout after first retry attempt (waiting 100ms but timeout is 50ms)
	if attempts > 2 {
		t.Errorf("Expected at most 2 attempts before timeout, got: %d", attempts)
	}
}

func TestWithExponentialBackoff_FatalError(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return Fatal(errors.New("fatal error"))
	}

	ctx := context.Background()
	err := WithExponentialBackoff(ctx, operation, WithInitialDelay(10*time.Millisecond))

	if err == nil {
		t.Error("Expected fatal error, got nil")
	}
	if !IsFatal(err) {
		t.Errorf("Expected fatal error, got: %v", err)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retries for fatal error), got: %d", attempts)
	}
}

func TestWithExponentialBackoff_BackoffTiming(t *testing.T) {
	attempts := 0
	var delays []time.Duration
	lastTime := time.Now()

	operation := func() error {
		attempts++
		now := time.Now()
		if attempts > 1 {
			delays = append(delays, now.Sub(lastTime))
		}
		lastTime = now
		if attempts < 4 {
			return errors.New("error")
		}
		return nil
	}

	ctx := context.Background()
	initialDelay := 50 * time.Millisecond
	err := WithExponentialBackoff(ctx, operation,
		WithInitialDelay(initialDelay),
		WithMaxDelay(200*time.Millisecond),
		WithMultiplier(2.0))

	if err != nil {
		t.Errorf("Expected success after retries, got: %v", err)
	}

	// We should have 3 delays (between 4 attempts)
	if len(delays) != 3 {
		t.Errorf("Expected 3 delays, got: %d", len(delays))
	}

	// Check that delays are exponentially increasing
	// Allow 20ms tolerance for timing variations
	tolerance := 20 * time.Millisecond

	expectedDelays := []time.Duration{
		50 * time.Millisecond,  // Initial
		100 * time.Millisecond, // 2x
		200 * time.Millisecond, // 2x (capped at max)
	}

	for i, delay := range delays {
		expected := expectedDelays[i]
		if delay < expected-tolerance || delay > expected+tolerance {
			t.Errorf("Delay %d: expected ~%v, got %v", i+1, expected, delay)
		}
	}
}

func TestFatal(t *testing.T) {
	t.Run("Nil error", func(t *testing.T) {
		err := Fatal(nil)
		if err != nil {
			t.Errorf("Expected nil, got: %v", err)
		}
	})

	t.Run("Non-nil error", func(t *testing.T) {
		originalErr := errors.New("test error")
		err := Fatal(originalErr)

		if err == nil {
			t.Error("Expected non-nil error")
		}
		if !IsFatal(err) {
			t.Error("Expected error to be fatal")
		}
		if err.Error() != originalErr.Error() {
			t.Errorf("Expected error message %q, got %q", originalErr.Error(), err.Error())
		}
	})
}

func TestIsFatal(t *testing.T) {
	t.Run("Non-fatal error", func(t *testing.T) {
		err := errors.New("regular error")
		if IsFatal(err) {
			t.Error("Expected non-fatal error")
		}
	})

	t.Run("Fatal error", func(t *testing.T) {
		err := Fatal(errors.New("fatal error"))
		if !IsFatal(err) {
			t.Error("Expected fatal error")
		}
	})

	t.Run("Wrapped fatal error", func(t *testing.T) {
		err := Fatal(errors.New("base error"))
		wrapped := errors.Join(err, errors.New("additional context"))
		if !IsFatal(wrapped) {
			t.Error("Expected wrapped fatal error to be detected")
		}
	})
}

func TestWithOptions(t *testing.T) {
	t.Run("WithMaxRetries", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return errors.New("error")
		}

		ctx := context.Background()
		_ = WithExponentialBackoff(ctx, operation,
			WithMaxRetries(2),
			WithInitialDelay(10*time.Millisecond))

		if attempts != 3 { // 1 + 2 retries
			t.Errorf("Expected 3 attempts, got: %d", attempts)
		}
	})

	t.Run("WithInitialDelay", func(t *testing.T) {
		start := time.Now()
		attempts := 0
		operation := func() error {
			attempts++
			if attempts < 2 {
				return errors.New("error")
			}
			return nil
		}

		ctx := context.Background()
		_ = WithExponentialBackoff(ctx, operation,
			WithInitialDelay(100*time.Millisecond),
			WithMaxRetries(2))

		duration := time.Since(start)
		// Should wait at least 100ms for the first retry
		if duration < 100*time.Millisecond {
			t.Errorf("Expected at least 100ms delay, got: %v", duration)
		}
	})

	t.Run("WithMaxDelay", func(t *testing.T) {
		attempts := 0
		var delays []time.Duration
		lastTime := time.Now()

		operation := func() error {
			attempts++
			now := time.Now()
			if attempts > 1 {
				delays = append(delays, now.Sub(lastTime))
			}
			lastTime = now
			if attempts < 5 {
				return errors.New("error")
			}
			return nil
		}

		ctx := context.Background()
		_ = WithExponentialBackoff(ctx, operation,
			WithInitialDelay(10*time.Millisecond),
			WithMaxDelay(20*time.Millisecond),
			WithMultiplier(2.0))

		// Check that no delay exceeds max delay (with tolerance)
		maxDelay := 20 * time.Millisecond
		tolerance := 10 * time.Millisecond
		for i, delay := range delays {
			if delay > maxDelay+tolerance {
				t.Errorf("Delay %d exceeded max: %v > %v", i+1, delay, maxDelay)
			}
		}
	})

	t.Run("WithMultiplier", func(t *testing.T) {
		attempts := 0
		var delays []time.Duration
		lastTime := time.Now()

		operation := func() error {
			attempts++
			now := time.Now()
			if attempts > 1 {
				delays = append(delays, now.Sub(lastTime))
			}
			lastTime = now
			if attempts < 3 {
				return errors.New("error")
			}
			return nil
		}

		ctx := context.Background()
		_ = WithExponentialBackoff(ctx, operation,
			WithInitialDelay(50*time.Millisecond),
			WithMultiplier(3.0),
			WithMaxDelay(1*time.Second))

		if len(delays) != 2 {
			t.Fatalf("Expected 2 delays, got: %d", len(delays))
		}

		// First delay should be ~50ms, second should be ~150ms (3x)
		tolerance := 20 * time.Millisecond
		if delays[0] < 50*time.Millisecond-tolerance || delays[0] > 50*time.Millisecond+tolerance {
			t.Errorf("First delay expected ~50ms, got: %v", delays[0])
		}
		if delays[1] < 150*time.Millisecond-tolerance || delays[1] > 150*time.Millisecond+tolerance {
			t.Errorf("Second delay expected ~150ms (3x), got: %v", delays[1])
		}
	})
}
