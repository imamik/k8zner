// Package testing provides test utilities, builders, and fixtures for unit and integration tests.
//
// This package centralizes common testing patterns to avoid duplication across test files:
//   - ConfigBuilder: Fluent builder for creating test configurations
//   - InfraFixture: Pre-configured mock infrastructure for common scenarios
//   - MockTalosProducer: Shared mock for Talos configuration generation
//
// Usage:
//
//	cfg := testing.NewConfigBuilder().
//	    WithClusterName("test").
//	    WithLocation("nbg1").
//	    Build()
//
//	fixture := testing.NewInfraFixture()
//	mockInfra := fixture.SuccessfulProvisioning()
package testing
