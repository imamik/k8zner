package wizard

import "errors"

// Validation errors for the interactive wizard.
var (
	errClusterNameRequired = errors.New("cluster name is required")
	errClusterNameInvalid  = errors.New("cluster name must be 1-32 lowercase alphanumeric characters or hyphens, starting and ending with alphanumeric")
	errSSHKeysRequired     = errors.New("at least one SSH key name is required")
	errCIDRRequired        = errors.New("CIDR is required")
	errCIDRInvalid         = errors.New("invalid CIDR format (expected: x.x.x.x/xx)")
)
