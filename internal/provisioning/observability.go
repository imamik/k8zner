package provisioning

import (
	"log"
)

// Observer defines the interface for observability during provisioning.
type Observer interface {
	Printf(format string, v ...interface{})
}

// ConsoleObserver implements Observer using the standard log package.
type ConsoleObserver struct{}

// NewConsoleObserver creates a new console-based observer.
func NewConsoleObserver() *ConsoleObserver {
	return &ConsoleObserver{}
}

// Printf implements Observer.
func (o *ConsoleObserver) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}
