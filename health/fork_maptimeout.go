// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Fork-only detail for the "Network map response timeout" warning: when the
// coordination server last answered a map request with a 200. That tells a
// poll that never reached the server apart from one that reached it and then
// went silent mid-stream.

package health

import (
	"fmt"
	"time"
)

// ArgLastMapRequestHeard provides the map response timeout Warnable with how
// long ago the coordination server last accepted a map request, as a
// duration string, or "never".
const ArgLastMapRequestHeard Arg = "last-map-request-heard"

// lastMapRequestHeardArgLocked returns the value for [ArgLastMapRequestHeard].
// t.mu must be held.
func (t *Tracker) lastMapRequestHeardArgLocked(now time.Time) string {
	if t.lastMapRequestHeard.IsZero() {
		return "never"
	}
	return now.Sub(t.lastMapRequestHeard).Round(time.Second).String()
}

// mapResponseTimeoutDetail returns the sentence the map response timeout
// Warnable appends to its text, or "" if args carry no such detail.
func mapResponseTimeoutDetail(args Args) string {
	switch v := args[ArgLastMapRequestHeard]; v {
	case "":
		return ""
	case "never":
		return " The coordination server has not accepted a map request since Tailscale started."
	default:
		return fmt.Sprintf(" The coordination server last accepted a map request %s ago.", v)
	}
}
