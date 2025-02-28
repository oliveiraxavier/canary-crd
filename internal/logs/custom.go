package logs

import (
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	Custom = ctrl.Log.WithName("")
)
