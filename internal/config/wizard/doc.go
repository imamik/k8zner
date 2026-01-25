// Package wizard provides an interactive configuration wizard for k8zner.
//
// This package implements a TUI-based wizard that guides users through
// creating a cluster configuration file. It uses charmbracelet/huh for
// form-based input collection.
//
// The main entry point is RunWizard, which orchestrates question groups
// and returns a WizardResult. Use BuildConfig to convert results to a
// Config struct, and WriteConfig to generate the YAML output file.
package wizard
