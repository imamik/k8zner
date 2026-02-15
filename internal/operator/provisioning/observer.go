package provisioning

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// OperatorObserver implements provisioning.Observer for operator context.
type OperatorObserver struct {
	ctx context.Context
}

// NewOperatorObserver creates a new operator observer.
func NewOperatorObserver(ctx context.Context) *OperatorObserver {
	return &OperatorObserver{ctx: ctx}
}

// Printf implements provisioning.Observer.
func (o *OperatorObserver) Printf(format string, v ...interface{}) {
	logger := log.FromContext(o.ctx)
	logger.Info(fmt.Sprintf(format, v...))
}
