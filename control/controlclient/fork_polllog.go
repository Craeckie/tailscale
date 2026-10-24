// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Fork-only diagnostics for the map long-poll. They make the reason behind a
// "Network map response timeout" health warning visible in the daemon log at
// the default verbosity, without turning on TS_DEBUG_NETMAP (which also dumps
// whole netmaps) or --verbose.

package controlclient

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"tailscale.com/tstime"
	"tailscale.com/types/logger"
)

// pollLogf returns logf while ctx is still live and vlogf once it is done.
//
// Errors after a deliberate cancel (pause on sleep or link change, re-auth,
// logout, shutdown, or the watchdog, which logs on its own) only say
// "context canceled" and stay verbose; errors on a live context are the
// actual network or server failures and are logged at normal level.
func pollLogf(ctx context.Context, logf, vlogf logger.Logf) logger.Logf {
	if ctx.Err() == nil {
		return logf
	}
	return vlogf
}

// v1UnlessLive returns the "[v1] " log level prefix if ctx is done, and the
// empty string while it is still live. See [pollLogf].
func v1UnlessLive(ctx context.Context) string {
	if ctx.Err() == nil {
		return ""
	}
	return "[v1] "
}

// mapPollTrace records how far a single map long-poll got, so the watchdog
// timeout can say whether the poll died waiting for the first response or
// went silent mid-stream. It is safe for concurrent use: the read loop writes
// it and the watchdog goroutine reads it.
type mapPollTrace struct {
	clock tstime.Clock
	t0    time.Time

	lastMsgUnixNano atomic.Int64 // time of the last message read; 0 if none
	msgs            atomic.Int64 // number of messages read
}

func newMapPollTrace(clock tstime.Clock, t0 time.Time) *mapPollTrace {
	return &mapPollTrace{clock: clock, t0: t0}
}

// noteMsg records that a complete map response message was read.
func (t *mapPollTrace) noteMsg() {
	t.lastMsgUnixNano.Store(t.clock.Now().UnixNano())
	t.msgs.Add(1)
}

// String describes the poll's progress, for log lines.
func (t *mapPollTrace) String() string {
	elapsed := t.clock.Since(t.t0).Round(time.Millisecond)
	last := t.lastMsgUnixNano.Load()
	if last == 0 {
		return fmt.Sprintf("after %v; no message received yet", elapsed)
	}
	ago := t.clock.Since(time.Unix(0, last)).Round(time.Millisecond)
	return fmt.Sprintf("after %v; %d messages received, last one %v ago", elapsed, t.msgs.Load(), ago)
}
