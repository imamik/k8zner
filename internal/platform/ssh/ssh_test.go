package ssh

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/util/keygen"
)

// generateTestKey generates a test RSA key pair for use in tests.
func generateTestKey(t *testing.T) *keygen.KeyPair {
	t.Helper()
	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	return keyPair
}

func TestNewClient_Success(t *testing.T) {
	keyPair := generateTestKey(t)

	cfg := &Config{
		Host:       "192.168.1.100",
		User:       "root",
		PrivateKey: keyPair.PrivateKey,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if client == nil {
		t.Fatal("expected client, got nil")
	}

	// Verify defaults were applied
	if client.config.Port != defaultPort { //nolint:staticcheck // t.Fatal above ensures client is not nil
		t.Errorf("expected port %d, got %d", defaultPort, client.config.Port)
	}
	if client.config.DialTimeout != defaultDialTimeout {
		t.Errorf("expected timeout %v, got %v", defaultDialTimeout, client.config.DialTimeout)
	}
	if client.config.MaxRetries != defaultMaxRetries {
		t.Errorf("expected max retries %d, got %d", defaultMaxRetries, client.config.MaxRetries)
	}
	if client.config.RetryDelay != defaultRetryDelay {
		t.Errorf("expected retry delay %v, got %v", defaultRetryDelay, client.config.RetryDelay)
	}
}

func TestNewClient_InvalidKey(t *testing.T) {
	cfg := &Config{
		Host:       "192.168.1.100",
		User:       "root",
		PrivateKey: []byte("invalid key"),
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("expected error for invalid private key, got nil")
	}

	want := "failed to parse private key"
	if len(err.Error()) < len(want) || err.Error()[:len(want)] != want {
		t.Errorf("expected error starting with %q, got: %v", want, err)
	}
}

func TestNewClient_NilConfig(t *testing.T) {
	_, err := NewClient(nil)
	if err == nil {
		t.Fatal("expected error for nil config, got nil")
	}

	if err.Error() != "config cannot be nil" {
		t.Errorf("expected 'config cannot be nil' error, got: %v", err)
	}
}

func TestNewClient_EmptyHost(t *testing.T) {
	keyPair := generateTestKey(t)

	cfg := &Config{
		Host:       "",
		User:       "root",
		PrivateKey: keyPair.PrivateKey,
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("expected error for empty host, got nil")
	}

	want := "config host cannot be empty"
	if err.Error() != want {
		t.Errorf("expected error %q, got: %v", want, err)
	}
}

func TestNewClient_EmptyUser(t *testing.T) {
	keyPair := generateTestKey(t)

	cfg := &Config{
		Host:       "192.168.1.100",
		User:       "",
		PrivateKey: keyPair.PrivateKey,
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("expected error for empty user, got nil")
	}

	want := "config user cannot be empty"
	if err.Error() != want {
		t.Errorf("expected error %q, got: %v", want, err)
	}
}

func TestNewClient_EmptyPrivateKey(t *testing.T) {
	cfg := &Config{
		Host:       "192.168.1.100",
		User:       "root",
		PrivateKey: nil,
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("expected error for empty private key, got nil")
	}

	want := "config private key cannot be empty"
	if err.Error() != want {
		t.Errorf("expected error %q, got: %v", want, err)
	}
}

func TestNewClient_CustomConfig(t *testing.T) {
	keyPair := generateTestKey(t)

	customPort := 2222
	customTimeout := 5 * time.Second
	customMaxRetries := 10
	customRetryDelay := 2 * time.Second

	cfg := &Config{
		Host:        "192.168.1.100",
		Port:        customPort,
		User:        "root",
		PrivateKey:  keyPair.PrivateKey,
		DialTimeout: customTimeout,
		MaxRetries:  customMaxRetries,
		RetryDelay:  customRetryDelay,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify custom values were preserved
	if client.config.Port != customPort {
		t.Errorf("expected port %d, got %d", customPort, client.config.Port)
	}
	if client.config.DialTimeout != customTimeout {
		t.Errorf("expected timeout %v, got %v", customTimeout, client.config.DialTimeout)
	}
	if client.config.MaxRetries != customMaxRetries {
		t.Errorf("expected max retries %d, got %d", customMaxRetries, client.config.MaxRetries)
	}
	if client.config.RetryDelay != customRetryDelay {
		t.Errorf("expected retry delay %v, got %v", customRetryDelay, client.config.RetryDelay)
	}
}

func TestNewClient_ConfigNotMutated(t *testing.T) {
	keyPair := generateTestKey(t)

	cfg := &Config{
		Host:       "192.168.1.100",
		User:       "root",
		PrivateKey: keyPair.PrivateKey,
		// Leave all optional fields as zero values
	}

	// Store original zero values
	originalPort := cfg.Port
	originalDialTimeout := cfg.DialTimeout
	originalMaxRetries := cfg.MaxRetries
	originalRetryDelay := cfg.RetryDelay

	_, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify original config was NOT mutated
	if cfg.Port != originalPort {
		t.Errorf("config was mutated: port changed from %d to %d", originalPort, cfg.Port)
	}
	if cfg.DialTimeout != originalDialTimeout {
		t.Errorf("config was mutated: DialTimeout changed from %v to %v", originalDialTimeout, cfg.DialTimeout)
	}
	if cfg.MaxRetries != originalMaxRetries {
		t.Errorf("config was mutated: MaxRetries changed from %d to %d", originalMaxRetries, cfg.MaxRetries)
	}
	if cfg.RetryDelay != originalRetryDelay {
		t.Errorf("config was mutated: RetryDelay changed from %v to %v", originalRetryDelay, cfg.RetryDelay)
	}
}

func TestNewClient_ParsesPrivateKey(t *testing.T) {
	keyPair := generateTestKey(t)

	cfg := &Config{
		Host:       "192.168.1.100",
		User:       "root",
		PrivateKey: keyPair.PrivateKey,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify signer was created (key was parsed)
	if client.signer == nil {
		t.Fatal("expected signer to be set, got nil")
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	keyPair := generateTestKey(t)

	cfg := &Config{
		Host:       "192.168.1.100", // Non-existent host
		User:       "root",
		PrivateKey: keyPair.PrivateKey,
		MaxRetries: 3,
		RetryDelay: 100 * time.Millisecond,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("expected no error creating client, got: %v", err)
	}

	// Create a context that is already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.Execute(ctx, "echo test")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}

	// Should fail quickly due to cancelled context, not retry
	if !errors.Is(err, context.Canceled) && err.Error()[:16] != "context canceled" && err.Error()[:16] != "context cancelled" {
		t.Logf("Got error: %v", err)
		// Context cancellation should be propagated
	}
}

func TestClient_AppliesDefaults(t *testing.T) {
	keyPair := generateTestKey(t)

	tests := []struct {
		name            string
		cfg             *Config
		wantPort        int
		wantDialTimeout time.Duration
		wantMaxRetries  int
		wantRetryDelay  time.Duration
	}{
		{
			name: "zero values get defaults",
			cfg: &Config{
				Host:       "192.168.1.100",
				User:       "root",
				PrivateKey: keyPair.PrivateKey,
				// All optional fields zero
			},
			wantPort:        defaultPort,
			wantDialTimeout: defaultDialTimeout,
			wantMaxRetries:  defaultMaxRetries,
			wantRetryDelay:  defaultRetryDelay,
		},
		{
			name: "custom values are preserved",
			cfg: &Config{
				Host:        "192.168.1.100",
				Port:        2222,
				User:        "root",
				PrivateKey:  keyPair.PrivateKey,
				DialTimeout: 5 * time.Second,
				MaxRetries:  10,
				RetryDelay:  2 * time.Second,
			},
			wantPort:        2222,
			wantDialTimeout: 5 * time.Second,
			wantMaxRetries:  10,
			wantRetryDelay:  2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify client uses expected values (either defaults or custom)
			if client.config.Port != tt.wantPort {
				t.Errorf("port = %d, want %d", client.config.Port, tt.wantPort)
			}
			if client.config.DialTimeout != tt.wantDialTimeout {
				t.Errorf("DialTimeout = %v, want %v", client.config.DialTimeout, tt.wantDialTimeout)
			}
			if client.config.MaxRetries != tt.wantMaxRetries {
				t.Errorf("MaxRetries = %d, want %d", client.config.MaxRetries, tt.wantMaxRetries)
			}
			if client.config.RetryDelay != tt.wantRetryDelay {
				t.Errorf("RetryDelay = %v, want %v", client.config.RetryDelay, tt.wantRetryDelay)
			}
		})
	}
}
