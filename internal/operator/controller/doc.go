// Package controller implements the Kubernetes controller for K8znerCluster
// custom resources.
//
// The controller uses a phase-based state machine to drive cluster lifecycle:
// Infrastructure -> Image -> Compute -> Bootstrap -> CNI -> Addons -> Running.
//
// Once running, the controller continuously monitors node health, replaces
// unhealthy nodes (respecting etcd quorum for control planes), and scales
// workers to match the desired count.
package controller
