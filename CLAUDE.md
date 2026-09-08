# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

This is a fork of [tailscale/tailscale](https://github.com/tailscale/tailscale) (`upstream` remote);
`origin` is `Craeckie/tailscale`. Sync with `git fetch upstream && git merge upstream/main`.

## Use `./tool/go`, never the system `go`

`tool/go` execs `tool/gocross/gocross-wrapper.sh`, which downloads and pins Tailscale's *own* Go
fork (`github.com/tailscale/go`, rev in `go.toolchain.rev`, version in `go.toolchain.version`) into
`~/.cache/tsgo/<rev>` and re-execs through `gocross`. Every Makefile target and CI workflow uses it.
A binary built with a mismatched toolchain panics at startup (`assert_ts_toolchain_match.go`, build
tag `tailscale_go`) unless `TS_PERMIT_TOOLCHAIN_MISMATCH=1`. `TS_GO_NEXT=1` selects
`go.toolchain.next.rev` instead, for testing toolchain bumps.

Tools that shell back out to a plain `go` (notably `depaware`) need the pinned toolchain on `PATH`
first — the Makefile does `PATH="$(./tool/go env GOROOT)/bin:$PATH"`. Copy that pattern in new tooling.

## Commands

```bash
./tool/go build ./cmd/...                       # compile everything
./build_dist.sh tailscale.com/cmd/tailscale     # distributable binary WITH version stamped in
./tool/go test ./ipn/ipnlocal/... -run TestFoo -v   # single test / package (normal dev loop)
make check                # staticcheck + vet + depaware + windows/386/arm/wasm cross-compiles
make vet                  # ./tool/go vet ./...
make staticcheck          # honnef.co/go/tools, config in staticcheck.conf
make depaware             # verify the per-binary dependency allowlists
make updatedeps           # regenerate them after a legitimate dependency change
make generate             # ./tool/go generate ./... (cloner/viewer codegen)
./tool/go test . -run TestLicenseHeaders   # the license-header check (a test, not a linter)
```

A plain `go build`/`go install` produces a binary with no real version string — `build_dist.sh` sets
the `-X tailscale.com/version.*Stamp` ldflags via `cmd/mkversion`. Use it for anything you'll run
and report bugs against.

CI (`.github/workflows/test.yml`) does not run `go test ./...` directly: it builds
`cmd/testwrapper` and runs `/tmp/testwrapper ./...`, which retries failing tests to distinguish
flakes from real failures and supports sharding (`TS_TEST_SHARD`). You don't need it locally.

## What CI will reject

- **A dirty tree after build+test.** `git diff --exit-code` plus an untracked-file check. This is
  also how stale generated files and stray build artifacts get caught.
- **A missing/wrong license header.** Exactly the two lines in `header.txt`, on every git-tracked
  `.go` file; enforced by `TestLicenseHeaders` in `license_test.go` with a short hardcoded exception
  list.
- **Any `replace` directive in `go.mod`** — `gomod_test.go` requires zero.
- **A depaware diff.** Adding an import that changes a tracked binary's transitive graph fails
  `make depaware` until the relevant `depaware.txt` is regenerated and committed. These files
  (`cmd/tailscaled/depaware{,-min,-minbox}.txt`, `tsnet/depaware.txt`, `cmd/{tailscale,derper,k8s-operator,stund,tsidp}/depaware.txt`)
  are a reviewed audit trail of the *whole* dependency graph, not a blocklist. Never hand-edit them.
- **Custom vet analyzers** (`cmd/vet`: `jsontags`, `lowerell`, `subtestnames`), run over
  `./tool/go run ./tool/listpkgs --ignore-3p ./...` — `--ignore-3p` skips `tempfork/`.
- **golangci-lint** (PRs only, `only-new-issues`). `.golangci.yml` enables only `bidichk`, `govet`,
  `importas`, `misspell`, `revive`; `importas` forces the alias `gliderssh`. `tempfork/*.go` and
  `util/singleflight/*.go` are deliberately gofmt-exempt — they're kept close to upstream for re-syncing.
- **Missing `Signed-off-by`.** DCO is required: `git commit -s`.

Commit messages follow `docs/commit-messages.md` — Go/Linux style, not Conventional Commits:
`pkg/subpkg: lowercase imperative summary`, no trailing period, body wrapped ~76 cols, and a
`Fixes #nnnn` / `Updates #nnnn` trailer (`Updates #cleanup` for genuine cleanups). `RELNOTE:` for
release-notes-worthy changes. Not machine-enforced, but strongly held.

## Architecture

### The daemon pipeline

`cmd/tailscaled/tailscaled.go` `run()` opens the control socket (`safesocket.Listen`), builds an
`ipnserver`, and calls `getLocalBackend`, which wires everything in order: a `tsdial.Dialer`, then
`createEngine` (either a real kernel TUN via `net/tstun` or, with `--tun=userspace-networking`,
netstack), then the state store, then `ipnlocal.NewLocalBackend`.

```
cmd/tailscaled -> ipn/ipnlocal.LocalBackend -> control/controlclient -> wgengine -> net/tstun
                                                                          |
                                                                     magicsock -> disco / DERP
```

- **`tsd.System`** (the `sys` value threaded through bring-up) is the DI container holding `NetMon`,
  `Bus`, `Engine`, `Tun`, `MagicSock`, `HealthTracker`, `Store`. Subsystems find each other through it.
- **`ipn/ipnlocal.LocalBackend`** is the hub — prefs, the netmap state machine, starting/stopping the
  control client, pushing peer/DNS/route config into `wgengine`, Taildrop, SSH, serve/funnel. It is a
  very large package; `local.go` alone is ~23k lines.
- **`control/controlclient`** polls the control plane over HTTPS+noise and delivers
  `tailcfg.MapResponse` → `types/netmap.NetworkMap` updates back to `LocalBackend` by callback.
  `NetworkMap` fields are read-only and may alias parts of the previous netmap.
- **`wgengine`** wraps wireguard-go; `wgengine/magicsock` is the path-switching transport that uses
  `disco` messages to probe for a direct UDP path and falls back to `derp`/`derphttp` relaying,
  transparently to WireGuard.

### netstack vs kernel TUN

`net/tstun` binds a real OS TUN device (excluded entirely on wasm/tamago/aix/solaris/illumos).
`wgengine/netstack` is gVisor's userspace TCP/IP stack, used for `--tun=userspace-networking`
(no root, the normal local-dev mode), for platforms where a TUN driver is awkward, and for
subnet-router SNAT — see `handleSubnetsInNetstack()` in `tailscaled.go` for exactly which platforms
and why. `tsnet` always runs this way.

### CLI ↔ daemon

`cmd/tailscale` (+ `cli/`) → `client/local.Client` → HTTP over `safesocket` → `ipn/localapi.Handler`
(a string-keyed dispatch table) → `LocalBackend` methods. Nothing in the CLI touches the backend
directly.

### The `feature/` modular-build system — read `feature/README.md`

The biggest architectural gotcha. New functionality goes in `feature/<name>/` and registers itself
via hooks from `init()` rather than being imported by callers, so that `ts_omit_<name>` build tags
actually remove the code and its dependencies.

- `feature/featuretags/featuretags.go` is the single source of truth (a DAG of feature deps). After
  editing it, regenerate with `./tool/go generate ./feature/buildfeatures`.
- `feature/condregister` is the one package `tailscaled` blank-imports; its `maybe_<name>.go` shims
  (`//go:build !ts_omit_<name>`) are what pull default-on features in.
- **`tsnet` does not go through `condregister`** — it has its own smaller default set and its own
  `depaware.txt`. So `buildfeatures.HasFoo` only means "not omitted by build tag", not "linked in".
  The correct idiom is `buildfeatures.HasFoo && feature.IsRegistered("foo")`.
- Hooks in `feature/hooks.go` must not reference heavy package types in their signatures — every
  `feature` consumer loads them, and that is what keeps `cmd/tailscaled/depaware-min.txt` small.
- `ipn/ipnext.RegisterExtension` is the parallel mechanism for features needing per-`LocalBackend`
  state, since one process (e.g. a `tsnet` host) can run several backends.

### Generated code — never hand-edit

`*_clone.go` and `*_view.go` come from `cmd/cloner` / `cmd/viewer` `//go:generate` directives (43
files carry them); `k8s-operator/apis/v1alpha1/zz_generated.deepcopy.go` comes from
`make kube-generate-deepcopy`; `feature/buildfeatures/feature_*_{enabled,disabled}.go` from the
featuretags generator. The View pattern wraps a pointer in a struct whose unexported field is named
`ж` — deliberately ugly, to make escaping it to callers feel dangerous.

Also worth knowing: `types/structs.Incomparable` (`[0]func()`, placed first in a struct to forbid
`==`/map-key use at zero size) and `types/opt` (`opt.Bool` is tri-state true/false/unset).

## Running it locally

`tailscaled` needs root unless you use `--tun=userspace-networking`, which forces the netstack path
and is the normal way to run a dev node. For multi-node and control-plane testing, use
`tstest/integration/testcontrol` (a minimal fake control server) — the integration suite in
`tstest/integration/` builds on it, and `cmd/testcontrol` is a standalone runnable version.
