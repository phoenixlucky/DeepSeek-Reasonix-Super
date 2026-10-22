package main

import (
	"strings"
	"testing"
)

func TestAScheduleChildIsRecognisedByItsArguments(t *testing.T) {
	for args, want := range map[string]bool{"schedule exec sch_x/1": true, "schedule exec": true, "schedule list": false, "exec schedule": false, "": false} {
		if got := scheduleChildArgs(strings.Fields(args)); got != want {
			t.Errorf("%q = %v, want %v", args, got, want)
		}
	}
}

// An orphan, or a kernel nobody can see, never starts a scheduled run.
func TestSchedulesStartOnlyForASupervisedKernelThatCanBeSeen(t *testing.T) {
	cases := []struct {
		supervised, icon bool
		panes            int
		want             bool
	}{
		{true, true, 0, true}, {true, false, 1, true}, {true, true, 3, true},
		{true, false, 0, false}, {false, true, 2, false}, {false, false, 0, false},
	}
	for _, c := range cases {
		if got := scheduleEligible(c.supervised, c.icon, c.panes); got != c.want {
			t.Errorf("%+v = %v", c, got)
		}
	}
}
