// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package health

import (
	"strings"
	"testing"
	"time"

	"tailscale.com/util/eventbus/eventbustest"
)

func TestMapResponseTimeoutLastMapRequestHeard(t *testing.T) {
	ht := NewTracker(eventbustest.NewBus(t))
	ht.SetIPNState("Running", true)

	ht.mu.Lock()
	defer ht.mu.Unlock()
	now := ht.now()
	ht.ipnWantRunningLastTrue = now.Add(-time.Minute)
	ht.inMapPoll = true
	ht.lastStreamedMapResponse = now.Add(-5 * time.Minute)

	check := func(wantArg, wantText string) {
		t.Helper()
		ht.updateBuiltinWarnablesLocked()
		ws, ok := ht.warnableVal[mapResponseTimeoutWarnable]
		if !ok {
			t.Fatalf("mapResponseTimeoutWarnable not unhealthy; warnables: %v", ht.warnableVal)
		}
		if got := ws.Args[ArgLastMapRequestHeard]; got != wantArg {
			t.Errorf("%s = %q, want %q", ArgLastMapRequestHeard, got, wantArg)
		}
		if got := mapResponseTimeoutWarnable.Text(ws.Args); !strings.HasSuffix(got, wantText) {
			t.Errorf("Text = %q, want suffix %q", got, wantText)
		}
	}

	check("never", "has not accepted a map request since Tailscale started.")

	ht.lastMapRequestHeard = now.Add(-4*time.Minute - 12*time.Second)
	check("4m12s", "last accepted a map request 4m12s ago.")
}

func TestMapResponseTimeoutTextWithoutDetail(t *testing.T) {
	got := mapResponseTimeoutWarnable.Text(Args{ArgDuration: "3m0s"})
	want := "Tailscale hasn't received a network map from the coordination server in 3m0s."
	if got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
}
