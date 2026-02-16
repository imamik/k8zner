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

// ValidationPhase implements the Phase interface for pre-flight validation.
type ValidationPhase struct{}

// NewValidationPhase creates a new validation phase.
func NewValidationPhase() *ValidationPhase {
	return &ValidationPhase{}
}

// Name implements the Phase interface.
func (vp *ValidationPhase) Name() string {
	return "validation"
}

// Provision implements the Phase interface.
func (vp *ValidationPhase) Provision(ctx *Context) error {
	ctx.Observer.Printf("[Validation] Running pre-flight validation...")

	allErrors := validate(ctx)

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
		ctx.Observer.Printf("[Validation] WARNING: %s", warning.Message)
	}

	// Calculate subnets if not already set (v2 configs have them pre-set)
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

	ctx.Observer.Printf("[Validation] Validation passed")
	return nil
}

// validate runs all validation checks and returns any errors or warnings.
func validate(ctx *Context) []ValidationError {
	var errs []ValidationError
	cfg := ctx.Config

	// --- Required fields ---

	if cfg.ClusterName == "" {
		errs = append(errs, ValidationError{
			Field:    "ClusterName",
			Message:  "cluster name is required",
			Severity: "error",
		})
	}

	if cfg.Location == "" {
		errs = append(errs, ValidationError{
			Field:    "Location",
			Message:  "location is required (e.g., 'nbg1', 'fsn1')",
			Severity: "error",
		})
	}

	if cfg.Network.Zone == "" {
		errs = append(errs, ValidationError{
			Field:    "Network.Zone",
			Message:  "network zone is required (e.g., 'eu-central')",
			Severity: "error",
		})
	}

	// --- Network ---

	if cfg.Network.IPv4CIDR == "" {
		errs = append(errs, ValidationError{
			Field:    "Network.IPv4CIDR",
			Message:  "network IPv4 CIDR is required",
			Severity: "error",
		})
	} else {
		_, ipNet, err := net.ParseCIDR(cfg.Network.IPv4CIDR)
		if err != nil {
			errs = append(errs, ValidationError{
				Field:    "Network.IPv4CIDR",
				Message:  fmt.Sprintf("invalid IPv4 CIDR: %v", err),
				Severity: "error",
			})
		} else {
			ones, bits := ipNet.Mask.Size()
			if ones > 16 {
				errs = append(errs, ValidationError{
					Field:    "Network.IPv4CIDR",
					Message:  fmt.Sprintf("CIDR prefix /%d is too small, recommended /16 or larger", ones),
					Severity: "warning",
				})
			}
			if bits != 32 {
				errs = append(errs, ValidationError{
					Field:    "Network.IPv4CIDR",
					Message:  "only IPv4 CIDRs are supported",
					Severity: "error",
				})
			}
		}
	}

	// --- Server types ---

	for i, pool := range cfg.ControlPlane.NodePools {
		if pool.ServerType == "" {
			errs = append(errs, ValidationError{
				Field:    fmt.Sprintf("ControlPlane.NodePools[%d].ServerType", i),
				Message:  "server type is required",
				Severity: "error",
			})
		}
		if pool.Count <= 0 {
			errs = append(errs, ValidationError{
				Field:    fmt.Sprintf("ControlPlane.NodePools[%d].Count", i),
				Message:  "count must be greater than 0",
				Severity: "error",
			})
		}
	}

	for i, pool := range cfg.Workers {
		if pool.ServerType == "" {
			errs = append(errs, ValidationError{
				Field:    fmt.Sprintf("Workers[%d].ServerType", i),
				Message:  "server type is required",
				Severity: "error",
			})
		}
		if pool.Count < 0 {
			errs = append(errs, ValidationError{
				Field:    fmt.Sprintf("Workers[%d].Count", i),
				Message:  "count must be non-negative",
				Severity: "error",
			})
		}
	}

	// --- SSH keys ---

	if len(cfg.SSHKeys) == 0 {
		errs = append(errs, ValidationError{
			Field:    "SSHKeys",
			Message:  "at least one SSH key is recommended for server access",
			Severity: "warning",
		})
	}

	// --- Version formats ---

	if cfg.Talos.Version != "" && !strings.HasPrefix(cfg.Talos.Version, "v") {
		errs = append(errs, ValidationError{
			Field:    "Talos.Version",
			Message:  "version should start with 'v' (e.g., 'v1.8.3')",
			Severity: "warning",
		})
	}

	if cfg.Kubernetes.Version != "" && !strings.HasPrefix(cfg.Kubernetes.Version, "v") {
		errs = append(errs, ValidationError{
			Field:    "Kubernetes.Version",
			Message:  "version should start with 'v' (e.g., 'v1.31.0')",
			Severity: "warning",
		})
	}

	return errs
}
