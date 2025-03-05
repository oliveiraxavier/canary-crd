package logs

import (
	"context"

	defaultlog "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	ctx    context.Context
	Custom = defaultlog.FromContext(ctx)
)
