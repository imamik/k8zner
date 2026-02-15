package retry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestWithExponentialBackoff_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
		WithMaxDelay(200*time.Millisecond))

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
	t.Parallel()
	t.Run("Nil error", func(t *testing.T) {
		t.Parallel()
		err := Fatal(nil)
		if err != nil {
			t.Errorf("Expected nil, got: %v", err)
		}
	})

	t.Run("Non-nil error", func(t *testing.T) {
		t.Parallel()
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
	t.Parallel()
	t.Run("Non-fatal error", func(t *testing.T) {
		t.Parallel()
		err := errors.New("regular error")
		if IsFatal(err) {
			t.Error("Expected non-fatal error")
		}
	})

	t.Run("Fatal error", func(t *testing.T) {
		t.Parallel()
		err := Fatal(errors.New("fatal error"))
		if !IsFatal(err) {
			t.Error("Expected fatal error")
		}
	})

	t.Run("Wrapped fatal error", func(t *testing.T) {
		t.Parallel()
		err := Fatal(errors.New("base error"))
		wrapped := errors.Join(err, errors.New("additional context"))
		if !IsFatal(wrapped) {
			t.Error("Expected wrapped fatal error to be detected")
		}
	})
}

func TestFatalError_Unwrap(t *testing.T) {
	t.Parallel()

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		t.Parallel()
		originalErr := errors.New("original error")
		fatalErr := &FatalError{Err: originalErr}

		unwrapped := fatalErr.Unwrap()
		if unwrapped != originalErr {
			t.Errorf("Unwrap() returned %v, want %v", unwrapped, originalErr)
		}
	})

	t.Run("errors.Unwrap returns underlying error", func(t *testing.T) {
		t.Parallel()
		originalErr := errors.New("original error")
		fatalErr := Fatal(originalErr)

		unwrapped := errors.Unwrap(fatalErr)
		if unwrapped != originalErr {
			t.Errorf("errors.Unwrap() returned %v, want %v", unwrapped, originalErr)
		}
	})

	t.Run("errors.Is traverses Unwrap chain", func(t *testing.T) {
		t.Parallel()
		sentinel := errors.New("sentinel error")
		fatalErr := Fatal(sentinel)

		if !errors.Is(fatalErr, sentinel) {
			t.Error("errors.Is should find sentinel through FatalError.Unwrap()")
		}
	})

	t.Run("errors.Is with fmt.Errorf wrapped fatal", func(t *testing.T) {
		t.Parallel()
		sentinel := errors.New("sentinel error")
		fatalErr := Fatal(sentinel)
		doubleWrapped := fmt.Errorf("context: %w", fatalErr)

		if !errors.Is(doubleWrapped, sentinel) {
			t.Error("errors.Is should find sentinel through double-wrapped FatalError")
		}
		if !IsFatal(doubleWrapped) {
			t.Error("IsFatal should detect FatalError through fmt.Errorf wrapping")
		}
	})
}

func TestWithOptions(t *testing.T) {
	t.Parallel()
	t.Run("WithMaxRetries", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
			WithMaxDelay(20*time.Millisecond))

		// Check that no delay exceeds max delay (with tolerance)
		maxDelay := 20 * time.Millisecond
		tolerance := 10 * time.Millisecond
		for i, delay := range delays {
			if delay > maxDelay+tolerance {
				t.Errorf("Delay %d exceeded max: %v > %v", i+1, delay, maxDelay)
			}
		}
	})

}
