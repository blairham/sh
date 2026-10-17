# `gated`: a `/bin/sh` for distroless and scratch images that cannot run anything

A distroless or `scratch` image has no `/bin/sh`, no coreutils and no package
manager, so `kubectl exec` has nothing to run and "exec in and look at `/proc`"
is dead. The usual fixes — add busybox, or attach an ephemeral debug container —
put an executable shell where an attacker who reaches code execution can find
it, and leave no record of what anyone did.

`github.com/blairham/sh/gated` puts a shell back that **cannot execute
anything**. It is this module's interpreter in the `dash` dialect with a
deny-by-default gate in front of every action it takes, and a record of every
action it took. It is a way to *read* a container, not to run things in it.

It is not the same thing as this repository's `ghcr.io/blairham/sh/scratch/*`
images. Those carry a whole shell, which can exec whatever the image holds; this
is a diagnostic shell whose answer to exec is always no.

`gated.Main` is the front end: install it as your image's `/bin/sh` and reach
it with `kubectl exec <pod> -- /bin/sh -c '<script>'` or
`kubectl exec -i <pod> -- /bin/sh < probe.sh`. `gated.Shell` is the same shell
as a function call, for a front end of your own. Localiam was its first home
and is its first consumer.

The package ships **no network front end**, on purpose: nothing in it accepts
input from anyone who cannot already exec into the container.

## Adopting it

**1. Depend on the module.**

    go get github.com/blairham/sh@latest

The module's `go` directive is the floor for yours; check `go.mod` in this
repository, and expect to raise your own to match. The module has no runtime
dependencies outside the standard library.

**2. Add a `cmd/sh` — this is the whole of it.**

```go
// Command sh is a gated shell for this image: see github.com/blairham/sh/gated.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/blairham/sh/gated"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(gated.Main(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv("HOSTNAME")))
}
```

The same code is `ExampleMain` in this package, so it is compiled with every
change here.

**3. Build it static and install it at `/bin/sh` in a distroless or scratch
image.** In a multi-stage Dockerfile:

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/sh ./cmd/sh
# ... and your service's own binary, built the same way

FROM gcr.io/distroless/static-debian12:nonroot   # or: FROM scratch
COPY --from=build /out/sh /bin/sh
```

Pin both base images by digest in a real Dockerfile.

**For a GoReleaser `dockers_v2` release**, where the build context already holds
`<os>/<arch>/<binary>` and the image stage compiles nothing, add a build with id
`sh` (Linux only, `CGO_ENABLED=0`, `main: ./cmd/sh`, in no archive — it exists
to be copied into the image), list `sh` in the image's `ids`, and in the release
stage:

```dockerfile
ARG TARGETOS TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/sh /bin/sh
```

**4. Optionally, an HTTP front end of your own.** If you want one, wrap
`Shell.Run` yourself behind your own authentication, return the `Result` (it
carries JSON tags, including the trail and the `denied` count), and put a
deadline on the context. Localiam's `internal/shell` does exactly this, with a
bearer token from a Secret that leaves the endpoint unmounted when empty. The
package reads no environment itself; the session name is yours to supply.

## Operating it: what the deployment must get right

The gate is only half the boundary. The other half is how it is exposed, and
none of this can be enforced from inside the package:

1. **The image must have no other shell or userland.** Distroless or `scratch`,
   and nothing else copied in. Beside busybox, `ls`, `wget` and the rest are
   still there to exec from anything that is not this binary, so a gated
   `/bin/sh` protects nothing.
2. **No `exec` lifecycle hooks or probes.** A `preStop: exec: ["sleep", "5"]`
   needs a `sleep` binary; the image has none and this shell refuses exec
   anyway. Use `preStop: sleep: {seconds: 5}` (Kubernetes >= 1.30), and HTTP or
   gRPC probes rather than `exec` ones.
3. **If you add your own HTTP front end**, it is a remote shell into the pod
   however tightly the gate holds: authenticate it, mount it only when its
   secret is present, and never make it reachable from the internet.

`/bin/sh` needs no credential of its own: it is reachable only by someone who can already
`kubectl exec`, which is its own authorization.

## What it can do

Nothing external. The toolkit is shell builtins plus globbing plus redirection,
pointed at `/proc` and `/sys/fs/cgroup`, which covers most of what anyone execs
in to ask:

```sh
# ps, with no ps
for d in /proc/[0-9]*; do read -r c < "$d/comm"; echo "${d#/proc/} $c"; done

# memory ceiling and current use, from cgroup v2
read -r m < /sys/fs/cgroup/memory.max; read -r c < /sys/fs/cgroup/memory.current; echo "$c / $m"

# OOM and pressure history the pod has seen
while read -r l; do echo "$l"; done < /sys/fs/cgroup/memory.events
```

`sh -c 'script' a b` binds `a` to `$0` and `b` to `$1`, as real `sh` does. A
script on standard input runs with `$0` set to `sh`.

## What it cannot do, and why that is the point

`policy.go` is short and it is the whole security argument; read it rather than
this table.

| Action | Answer |
|---|---|
| `exec`, anything | **Refused, unconditionally**, including a path that exists, so adding a binary to the image later does not widen it. |
| `signal` | Refused. pid 1 is the service, and a shell that can `kill` it is a liveness probe with extra steps. |
| open for **write** | Refused, including inside the roots. `> /proc/sys/...` is a real write on a real kernel knob. |
| read outside `ReadRoots` | Refused. Default roots are `/proc` and `/sys/fs/cgroup`. Paths are cleaned first, so `/proc/1/../../etc/shadow` is tested as `/etc/shadow`. |
| read `*/environ` | Refused, although it sits inside an allowed root. See below. |
| `stat` an ancestor of a root | **Allowed.** Globbing stats each component on the way down, and refusing `stat /proc` made `/proc/1/*` collapse to the literal pattern with exit status 0. Existence of a directory on the way to a root leaks nothing. |
| an action kind added by a later interpreter | Refused, so an upgrade's failure mode is a diagnostic that stops working rather than a boundary that quietly widened. |

Every action, allowed or refused, is recorded. A `Result` carries the trail
and a `denied` count; `/bin/sh` prints a summary of refusals to stderr.
That matters because **an empty answer and a refused answer look identical on
stdout.**

The interpreter is also given an empty environment and `/` as its directory, so
a script cannot print the service's environment variables, and a relative path
the gate is asked about is refused on that basis alone.

### Why `environ` is a deny rather than a comment

`/proc/<pid>/environ` is where a service's credentials arrive, including the
token of any front end you put before this shell, and it is inside `/proc`.
Without the deny, `read -r v < /proc/1/environ` returns every variable. The first probe of it came
back *empty*, which read as safe and was not: it used `while read`, and a file
with no trailing newline never runs a `while read` body. Real dash behaves
identically. `TestTheEnvironmentIsRefusedInsideTheRoots` uses the bare `read`
for that reason and fails if the deny is removed.

## Bounds

8 KiB of script, 256 KiB of output per stream and 4096 trail records. `Run`
stops when its context does (the interpreter checks it at every command), so a
front end of your own should pass one with a deadline. Each run gets a fresh interpreter, so no variable, function or cwd
survives between two scripts, and there is deliberately no interactive mode.

`Main` always runs with `DefaultPolicy`. A front end that needs other roots
builds a `Shell` with its own `Policy` and calls `Run`.

## Tests

`go test -race -count=10 ./gated/` is deterministic: no sleeps, and the
allow-side cases use a `t.TempDir()` root rather than `/proc`, so the verdict
does not depend on the kernel. Every rule is mutation-tested: allowing exec,
dropping `path.Clean`, allowing writes, removing the `environ` deny, dropping
either half of the `$0`/`$@` binding in `Shell.Run`, dropping or joining the
arguments in `Main`, removing the refusal summary, and importing `os` into the
package each turn a test red.

⚠ **Keep struct literals in this package keyed.** A `govet fieldalignment`
fixer reorders struct fields, and in this code's first home it turned a
positional literal into a compile error.
