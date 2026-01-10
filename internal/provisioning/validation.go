package provisioning

import (
	"fmt"
	"net"
	"strings"
)

// ValidationError represents a configuration validation error or warning.
type ValidationError struct {
	Field    string // Configuration field that failed validation
	Message  string // Human-readable error message
	Severity string // "error" or "warning"
}

// Error implements the error interface.
func (ve ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", ve.Severity, ve.Field, ve.Message)
}

// IsError returns true if this is an error (not a warning).
func (ve ValidationError) IsError() bool {
	return ve.Severity == "error"
}

// Validator defines the interface for configuration validators.
type Validator interface {
	Validate(ctx *Context) []ValidationError
}

// ValidationPhase implements the Phase interface for pre-flight validation.
type ValidationPhase struct {
	validators []Validator
}

// NewValidationPhase creates a new validation phase with standard validators.
func NewValidationPhase() *ValidationPhase {
	return &ValidationPhase{
		validators: []Validator{
			&RequiredFieldsValidator{},
			&NetworkValidator{},
			&ServerTypeValidator{},
			&SSHKeyValidator{},
			&VersionValidator{},
		},
	}
}

// Provision implements the Phase interface.
func (vp *ValidationPhase) Provision(ctx *Context) error {
	ctx.Logger.Printf("[Validation] Running pre-flight validation...")

	var allErrors []ValidationError
	for _, validator := range vp.validators {
		errors := validator.Validate(ctx)
		allErrors = append(allErrors, errors...)
	}

	// Separate errors and warnings
	var errors []ValidationError
	var warnings []ValidationError
	for _, ve := range allErrors {
		if ve.IsError() {
			errors = append(errors, ve)
		} else {
			warnings = append(warnings, ve)
		}
	}

	// Log warnings
	for _, warning := range warnings {
		ctx.Logger.Printf("[Validation] WARNING: %s", warning.Message)
	}

	// Apply defaults
	if err := ctx.Config.ApplyDefaults(); err != nil {
		return fmt.Errorf("failed to apply defaults: %w", err)
	}

	// Calculate subnets (moved from infrastructure phase)
	if err := ctx.Config.CalculateSubnets(); err != nil {
		return fmt.Errorf("failed to calculate subnets: %w", err)
	}

	// Fail if we have errors
	if len(errors) > 0 {
		var errMsgs []string
		for _, e := range errors {
			errMsgs = append(errMsgs, e.Error())
		}
		return fmt.Errorf("configuration validation failed:\n  %s", strings.Join(errMsgs, "\n  "))
	}

	ctx.Logger.Printf("[Validation] Validation passed")
	return nil
}

// RequiredFieldsValidator validates that required configuration fields are set.
type RequiredFieldsValidator struct{}

// Validate implements Validator interface for RequiredFieldsValidator.
func (v *RequiredFieldsValidator) Validate(ctx *Context) []ValidationError {
	var errors []ValidationError
	cfg := ctx.Config

	if cfg.ClusterName == "" {
		errors = append(errors, ValidationError{
			Field:    "ClusterName",
			Message:  "cluster name is required",
			Severity: "error",
		})
	}

	if cfg.Location == "" {
		errors = append(errors, ValidationError{
			Field:    "Location",
			Message:  "location is required (e.g., 'nbg1', 'fsn1')",
			Severity: "error",
		})
	}

	if cfg.Network.Zone == "" {
		errors = append(errors, ValidationError{
			Field:    "Network.Zone",
			Message:  "network zone is required (e.g., 'eu-central')",
			Severity: "error",
		})
	}

	return errors
}

// NetworkValidator validates network configuration.
type NetworkValidator struct{}

// Validate implements Validator interface for NetworkValidator.
func (v *NetworkValidator) Validate(ctx *Context) []ValidationError {
	var errors []ValidationError
	cfg := ctx.Config

	// Validate IPv4 CIDR
	if cfg.Network.IPv4CIDR == "" {
		errors = append(errors, ValidationError{
			Field:    "Network.IPv4CIDR",
			Message:  "network IPv4 CIDR is required",
			Severity: "error",
		})
	} else {
		_, ipNet, err := net.ParseCIDR(cfg.Network.IPv4CIDR)
		if err != nil {
			errors = append(errors, ValidationError{
				Field:    "Network.IPv4CIDR",
				Message:  fmt.Sprintf("invalid IPv4 CIDR: %v", err),
				Severity: "error",
			})
		} else {
			// Check if CIDR is large enough for subnets
			ones, bits := ipNet.Mask.Size()
			if ones > 16 {
				errors = append(errors, ValidationError{
					Field:    "Network.IPv4CIDR",
					Message:  fmt.Sprintf("CIDR prefix /%d is too small, recommended /16 or larger", ones),
					Severity: "warning",
				})
			}
			if bits != 32 {
				errors = append(errors, ValidationError{
					Field:    "Network.IPv4CIDR",
					Message:  "only IPv4 CIDRs are supported",
					Severity: "error",
				})
			}
		}
	}

	return errors
}

// ServerTypeValidator validates server type configurations.
type ServerTypeValidator struct{}

// Validate implements Validator interface for ServerTypeValidator.
func (v *ServerTypeValidator) Validate(ctx *Context) []ValidationError {
	var errors []ValidationError
	cfg := ctx.Config

	// Validate control plane server types
	for i, pool := range cfg.ControlPlane.NodePools {
		if pool.ServerType == "" {
			errors = append(errors, ValidationError{
				Field:    fmt.Sprintf("ControlPlane.NodePools[%d].ServerType", i),
				Message:  "server type is required",
				Severity: "error",
			})
		}
		if pool.Count <= 0 {
			errors = append(errors, ValidationError{
				Field:    fmt.Sprintf("ControlPlane.NodePools[%d].Count", i),
				Message:  "count must be greater than 0",
				Severity: "error",
			})
		}
	}

	// Validate worker server types
	for i, pool := range cfg.Workers {
		if pool.ServerType == "" {
			errors = append(errors, ValidationError{
				Field:    fmt.Sprintf("Workers[%d].ServerType", i),
				Message:  "server type is required",
				Severity: "error",
			})
		}
		if pool.Count < 0 {
			errors = append(errors, ValidationError{
				Field:    fmt.Sprintf("Workers[%d].Count", i),
				Message:  "count must be non-negative",
				Severity: "error",
			})
		}
	}

	return errors
}

// SSHKeyValidator validates SSH key configuration.
type SSHKeyValidator struct{}

// Validate implements Validator interface for SSHKeyValidator.
func (v *SSHKeyValidator) Validate(ctx *Context) []ValidationError {
	var errors []ValidationError
	cfg := ctx.Config

	if len(cfg.SSHKeys) == 0 {
		errors = append(errors, ValidationError{
			Field:    "SSHKeys",
			Message:  "at least one SSH key is recommended for server access",
			Severity: "warning",
		})
	}

	return errors
}

// VersionValidator validates Talos and Kubernetes version formats.
type VersionValidator struct{}

// Validate implements Validator interface for VersionValidator.
func (v *VersionValidator) Validate(ctx *Context) []ValidationError {
	var errors []ValidationError
	cfg := ctx.Config

	// Validate Talos version format
	if cfg.Talos.Version != "" && !strings.HasPrefix(cfg.Talos.Version, "v") {
		errors = append(errors, ValidationError{
			Field:    "Talos.Version",
			Message:  "version should start with 'v' (e.g., 'v1.8.3')",
			Severity: "warning",
		})
	}

	// Validate Kubernetes version format
	if cfg.Kubernetes.Version != "" && !strings.HasPrefix(cfg.Kubernetes.Version, "v") {
		errors = append(errors, ValidationError{
			Field:    "Kubernetes.Version",
			Message:  "version should start with 'v' (e.g., 'v1.31.0')",
			Severity: "warning",
		})
	}

	return errors
}
