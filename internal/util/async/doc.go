// Package async provides utilities for parallel task execution with
// error collection.
//
// The [Run] function executes multiple operations concurrently and
// returns all errors. It is used throughout provisioning to parallelize
// independent infrastructure operations.
package async
