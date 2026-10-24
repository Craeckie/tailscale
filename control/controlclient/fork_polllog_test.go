// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package controlclient

import (
	"context"
	"fmt"
	"testing"
	"time"

	"tailscale.com/tstest"
)

func TestMapPollTrace(t *testing.T) {
	clock := tstest.NewClock(tstest.ClockOpts{Start: time.Unix(1000, 0)})
	tr := newMapPollTrace(clock, clock.Now())

	clock.Advance(2 * time.Minute)
	if got, want := tr.String(), "after 2m0s; no message received yet"; got != want {
		t.Errorf("before any message: got %q, want %q", got, want)
	}

	tr.noteMsg()
	clock.Advance(time.Second)
	tr.noteMsg()
	clock.Advance(2 * time.Minute)
	if got, want := tr.String(), "after 4m1s; 2 messages received, last one 2m0s ago"; got != want {
		t.Errorf("after messages: got %q, want %q", got, want)
	}
}

func TestPollLogf(t *testing.T) {
	var got string
	logf := func(format string, args ...any) { got = "logf: " + fmt.Sprintf(format, args...) }
	vlogf := func(format string, args ...any) { got = "vlogf: " + fmt.Sprintf(format, args...) }

	ctx, cancel := context.WithCancel(context.Background())
	pollLogf(ctx, logf, vlogf)("x %d", 1)
	if want := "logf: x 1"; got != want {
		t.Errorf("live ctx: got %q, want %q", got, want)
	}
	if p := v1UnlessLive(ctx); p != "" {
		t.Errorf("live ctx: v1UnlessLive = %q, want empty", p)
	}

	cancel()
	pollLogf(ctx, logf, vlogf)("x %d", 2)
	if want := "vlogf: x 2"; got != want {
		t.Errorf("canceled ctx: got %q, want %q", got, want)
	}
	if p, want := v1UnlessLive(ctx), "[v1] "; p != want {
		t.Errorf("canceled ctx: v1UnlessLive = %q, want %q", p, want)
	}
}
