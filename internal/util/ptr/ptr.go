// Package ptr provides helper functions for creating pointers to primitive types.
package ptr

// Bool returns a pointer to the given bool value.
func Bool(b bool) *bool { return &b }
