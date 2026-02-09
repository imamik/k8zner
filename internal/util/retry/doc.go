// Package retry provides exponential backoff retry logic for transient failures.
//
// The [Do] function retries an operation with configurable max attempts,
// initial delay, and maximum delay. It is used for Hetzner Cloud API calls
// and other operations that may fail transiently.
package retry
