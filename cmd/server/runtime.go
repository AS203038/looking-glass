package main

import (
	"fmt"
	"log/slog"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"go.uber.org/automaxprocs/maxprocs"
)

// configureRuntime adjusts GOMAXPROCS and GOMEMLIMIT to match container cgroup
// limits and returns a function that restores the prior GOMAXPROCS.
func configureRuntime(log *slog.Logger) func() {
	if log == nil {
		log = slog.Default()
	}

	undo, err := maxprocs.Set(maxprocs.Logger(func(format string, args ...any) {
		log.Info(fmt.Sprintf(format, args...))
	}))
	if err != nil {
		log.Warn("automaxprocs failed", slog.Any("err", err))
	}
	if undo == nil {
		undo = func() {}
	}

	_, _ = memlimit.Set(memlimit.WithLogger(log))

	return undo
}
