package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"reasonix/internal/base/filelock"
	"reasonix/internal/frontend/serve"
	"reasonix/internal/runtime/schedrun"
	"reasonix/internal/runtime/scheduler"
)

// scheduleChildArgs says argv is a supervised scheduled run: the supervisor
// starts this binary as `schedule exec <id>`, and this host has no CLI of its own.
func scheduleChildArgs(args []string) bool {
	return len(args) >= 2 && args[0] == "schedule" && args[1] == "exec"
}

// scheduleEligible is when this kernel may start scheduled runs: it has a
// supervised lifetime (a parent that ends it) and something the person can see.
// An orphan, or a kernel nobody can see, never spends on anyone's behalf.
func scheduleEligible(supervised, iconLive bool, panes int) bool {
	return supervised && (iconLive || panes > 0)
}

// startScheduleTicker runs the in-kernel driver of scheduled runs. Every kernel
// on the machine may run it; the ticker lock lets one tick at a time.
func startScheduleTicker(ctx context.Context, supervised bool, hub *serve.Hub, logs io.Writer) {
	if !supervised {
		return
	}
	loop := &scheduler.Loop{
		Eligible: func() bool { return scheduleEligible(supervised, (&studioTray{}).IconLive(), len(hub.Runtimes())) },
		Lock: func() (func(), error) {
			store, _, err := scheduler.OpenUserStore(nil)
			if err != nil {
				return nil, err
			}
			return filelock.TryAcquire(filepath.Join(store.Dir(), "scheduler.lock"))
		},
		Tick: func(ctx context.Context) (scheduler.Result, error) {
			store, policy, err := scheduler.OpenUserStore(nil)
			if err != nil {
				return scheduler.Result{}, err
			}
			return (&scheduler.Scheduler{Store: store, Policy: policy, Runner: &schedrun.Supervisor{Store: store, Policy: policy}}).Tick(ctx)
		},
		Done: func(res scheduler.Result, err error) {
			for _, r := range res.Reports {
				fmt.Fprintf(logs, "schedule: ran %s: %s %s\n", r.TriggerID, r.State, r.Code)
			}
			for _, s := range res.Skipped {
				fmt.Fprintf(logs, "schedule: skipped %s: %s\n", s.ScheduleID, s.Reason)
			}
			if err != nil {
				fmt.Fprintln(logs, "schedule:", err)
			}
		},
	}
	go loop.Run(ctx)
}
