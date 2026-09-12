// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Reach is how the harness reaches a panel member.
//
// There used to be one way and it was never named: Resolve called
// exec.LookPath over Shell.Lookup and command() started a process from the
// path it returned. That was a fact about the panel rather than about
// shells — every member happened to be a binary on the machine — and it held
// until ash. There is no BusyBox on a macOS machine and no way to get one
// (#2263 lists what was tried), so the fifth dialect shipped measured and
// ungraded, and within the hour a semantics axis was added with no ash answer
// and nothing went red (#2272).
//
// So the route is a value now, the same way "which shell am I" is a value:
// one concept with two implementations rather than a container special case
// bolted beside the local path. Everything above this line — Resolve,
// Execute, Exec, the conformance run — asks a Reach for a Found and then
// stops caring where the shell is.
//
// The two implementations differ in exactly one thing, and it is worth
// stating because it is what keeps the columns comparable: LocalReach runs
// Exec in this process, and ContainerReach runs *the same Exec*, compiled
// from this tree, inside the image. Not a docker-run command line built to
// look like what Exec does — Exec itself, so the environment scrub, the
// signal-disposition scrub, the timeout, the wait-status reading and the
// normalization are one implementation and cannot drift apart.
type Reach interface {
	// open makes the shell runnable here, or says why it cannot be. The
	// error is shown to a person, so it says what would have to be true.
	open(ctx context.Context, s Shell) (Found, error)

	// route names the way this member is reached, for a report that has to
	// say what did not happen. See Absence.
	route() string
}

// Absence is a panel member this machine could not reach, and why.
//
// The reason travels with the name because a missing column is only safe
// while nobody can mistake it for an agreeing one. `ash: not run (docker is
// not reachable here)` and `ash: agrees` have to be impossible to confuse,
// and a bare list of names is exactly the shape that gets skimmed as the
// second. This repository's recorded scar is the same one from the other
// side: a gate inert on one platform reads as a pass.
type Absence struct {
	Name   string
	Reason string
}

func (a Absence) String() string { return a.Name + ": " + a.Reason }

// LocalReach is a binary on the machine running the harness: the original
// route, and still the one every panel member but ash takes.
type LocalReach struct{}

func (LocalReach) route() string { return "a binary on this machine" }

func (LocalReach) open(ctx context.Context, s Shell) (Found, error) {
	path, ok := locate(s.Lookup)
	if !ok {
		return Found{}, fmt.Errorf("not installed (looked for %s)", strings.Join(s.Lookup, ", "))
	}
	v := version(ctx, path)
	if err := s.believable(v); err != nil {
		return Found{}, err
	}
	return Found{Shell: s, Path: path, Version: v}, nil
}

// believable applies Shell.MustReport: the path exists, but is it this shell?
func (s Shell) believable(v string) error {
	if s.MustReport == "" || strings.Contains(strings.ToLower(v), s.MustReport) {
		return nil
	}
	return fmt.Errorf("found, but it reports %q rather than %q, so it is not the shell this column names", v, s.MustReport)
}

// ContainerReach runs a panel member inside a container image, because the
// shell exists nowhere else on the machine.
//
// # The image is pinned by digest, and that is the whole provenance argument
//
// A tag moves. `alpine:3` is a different BusyBox every few weeks, so a record
// generated against the tag would change underneath its own drift check and
// the change would be indistinguishable from a shell that behaved
// differently — which is precisely the failure the golden record exists to
// detect. The digest is the bytes, so `make oracle` a year from now measures
// the BusyBox this record was made from, and moving to a newer one is an edit
// to this file that shows up in a diff.
//
// The cost is stated rather than hidden: this column pins the behavior of an
// image, not of a binary on this machine, and regenerating it needs Docker.
// That was accepted (#2263) because it is the only shape where `make oracle`
// on a developer's own machine can produce the column at all, and the
// alternative was a dialect nothing re-checks.
type ContainerReach struct {
	// Image is the repository and Digest is the manifest the tag pointed at
	// when the column was recorded. Both are written down: the digest is what
	// is pulled, and the repository is what a person needs in order to look
	// it up.
	Image  string
	Digest string

	// Runtime is the container command, tried in order. Docker is the only
	// one measured; the list exists so a machine with podman is a
	// configuration rather than a code change.
	Runtime []string

	// once makes opening happen at most once per process. Starting a
	// container, copying a cross-compiled runner into it and probing the
	// version is seconds, and Resolve is called by every test in this
	// package; without this, `go test ./internal/oracle` would start a
	// container per test and wait out a timeout per test on a machine with
	// no Docker.
	once  sync.Once
	found Found
	err   error
}

func (c *ContainerReach) ref() string { return c.Image + "@" + c.Digest }

func (c *ContainerReach) route() string { return "a container of " + c.ref() }

func (c *ContainerReach) open(ctx context.Context, s Shell) (Found, error) {
	c.once.Do(func() { c.found, c.err = c.start(ctx, s) })
	return c.found, c.err
}

func (c *ContainerReach) start(ctx context.Context, s Shell) (Found, error) {
	cli, err := c.client(ctx)
	if err != nil {
		return Found{}, err
	}
	if err := c.have(ctx, cli); err != nil {
		return Found{}, err
	}
	runner, err := buildRunner(ctx)
	if err != nil {
		return Found{}, err
	}
	conn := &containerConn{cli: cli, ref: c.ref(), runner: runner, lookup: s.Lookup}
	if err := conn.dial(ctx); err != nil {
		return Found{}, err
	}
	if err := s.believable(conn.version); err != nil {
		conn.stop()
		return Found{}, err
	}
	f := Found{Shell: s, Path: conn.path, Version: conn.version}
	f.sess = &session{exec: conn.exec, close: conn.stop}
	registerSession(f.sess)
	return f, nil
}

// client picks the container command, and is the first thing that can fail on
// a machine that simply has none. `docker version` is asked rather than
// exec.LookPath alone, because a docker client with no daemon behind it is
// the common case and looks identical to a working one until something runs.
func (c *ContainerReach) client(ctx context.Context) (string, error) {
	names := c.Runtime
	if len(names) == 0 {
		names = []string{"docker"}
	}
	var why []string
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			why = append(why, name+" is not on PATH")
			continue
		}
		probe, cancel := context.WithTimeout(ctx, 20*time.Second)
		out, err := exec.CommandContext(probe, name, "version", "--format", "{{.Server.Version}}").CombinedOutput()
		cancel()
		if err != nil {
			why = append(why, name+" is installed but its daemon did not answer: "+firstLine(string(out)))
			continue
		}
		return name, nil
	}
	return "", fmt.Errorf("no container runtime here (%s)", strings.Join(why, "; "))
}

// have makes sure the pinned image is on this machine, pulling it if not.
func (c *ContainerReach) have(ctx context.Context, cli string) error {
	if err := exec.CommandContext(ctx, cli, "image", "inspect", c.ref()).Run(); err == nil {
		return nil
	}
	pull, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(pull, cli, "pull", "--quiet", c.ref()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pulling %s: %s", c.ref(), firstLine(string(out)))
	}
	return nil
}

// session is an opened route that is not this process: something has to be
// asked to run a case, and something has to be shut down afterwards.
//
// A Found carries a pointer to it, so a copy of a Found — which every caller
// makes, since Exec takes one by value — shares the one container rather than
// starting its own.
type session struct {
	exec  func(ctx context.Context, sh Found, c Case) Result
	close func()
}

var (
	sessionsMu sync.Mutex
	sessions   []*session
)

func registerSession(s *session) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	sessions = append(sessions, s)
}

// Shutdown releases every route this process opened.
//
// It is the counterpart of the once in ContainerReach: a container is started
// at most once and has to be stopped exactly once, and neither belongs to any
// single caller of Resolve. A command that runs the panel defers this; a
// process that forgets leaves a container that reaps itself when the runner
// inside it has waited long enough for work that is not coming.
func Shutdown() {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	for _, s := range sessions {
		s.close()
	}
	sessions = nil
}

// Request is one case, sent to the runner inside a container.
//
// The whole Found goes with it rather than a name to look up, so the two
// sides cannot hold different ideas of what the column is: Argv0, SelfName
// and Args are read by Exec and by normalize, and a runner that reconstructed
// them from its own copy of the panel would be a second statement of the same
// fact.
type Request struct {
	Shell Found `json:"shell"`
	Case  Case  `json:"case"`
}

// Reply is one measurement, coming back. Greeting is set only on the first
// line the runner writes, which reports what it found inside the image.
type Reply struct {
	Path    string  `json:"path,omitempty"`
	Version string  `json:"version,omitempty"`
	Err     string  `json:"err,omitempty"`
	Result  *Result `json:"result,omitempty"`
}

// containerConn is one runner process inside one container, spoken to over
// its standard input and output.
//
// One long-lived container rather than one invocation per case, and that is
// the difference between a column that costs seconds and one that costs half
// an hour: `docker run` is a quarter of a second of image setup before any
// shell starts, and the corpus is over three thousand cases. Inside, each
// case is an ordinary fork and exec, which is what the other columns cost.
type containerConn struct {
	cli    string
	ref    string
	runner string
	lookup []string

	path    string
	version string

	mu   sync.Mutex
	id   string
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Reader
	logs bytes.Buffer
}

// dial creates the container, copies the cross-compiled runner into it and
// starts it, then reads the greeting.
//
// The runner is copied rather than bind-mounted deliberately. A bind mount
// needs the host directory to be one the container runtime shares, and which
// directories those are is a property of the *runtime*: measured here, colima
// shares $HOME and not /tmp, and Docker Desktop shares both. A copy needs
// nothing shared, so the column does not depend on a virtual machine's mount
// list.
func (c *containerConn) dial(ctx context.Context) error {
	create, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	out, err := exec.CommandContext(create, c.cli, "create", "--rm", "-i",
		"--network", "none", c.ref, runnerPath, "-lookup", strings.Join(c.lookup, ",")).Output()
	if err != nil {
		return fmt.Errorf("creating a container of %s: %w", c.ref, err)
	}
	c.id = strings.TrimSpace(string(out))

	cp, cancelCp := context.WithTimeout(ctx, time.Minute)
	defer cancelCp()
	if msg, err := exec.CommandContext(cp, c.cli, "cp", c.runner, c.id+":"+runnerPath).CombinedOutput(); err != nil {
		c.remove()
		return fmt.Errorf("copying the runner into the container: %s", firstLine(string(msg)))
	}

	// Not the caller's ctx: this process outlives any one case, and a
	// cancelled measurement must not take the container with it.
	c.cmd = exec.Command(c.cli, "start", "-ai", c.id)
	in, err := c.cmd.StdinPipe()
	if err != nil {
		c.remove()
		return err
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		c.remove()
		return err
	}
	c.logs.Reset()
	c.cmd.Stderr = &c.logs
	if err := c.cmd.Start(); err != nil {
		c.remove()
		return err
	}
	c.in, c.out = in, bufio.NewReaderSize(stdout, 1<<20)

	hello, err := c.read(ctx)
	if err != nil {
		c.stop()
		return fmt.Errorf("the runner in %s said nothing: %w (%s)", c.ref, err, firstLine(c.logs.String()))
	}
	if hello.Err != "" {
		c.stop()
		return errors.New(hello.Err)
	}
	c.path, c.version = hello.Path, hello.Version
	return nil
}

// exec sends one case and waits for its measurement.
//
// A case that kills its own process group takes the runner with it — exactly
// one case in the corpus does — so a dead connection is redialed and the case
// retried once. Retrying more than once would turn a case that always kills
// the runner into an endless loop, and reporting a harness error for the
// second attempt is the honest answer: the cell says the harness could not
// measure it rather than inventing a measurement.
func (c *containerConn) exec(ctx context.Context, sh Found, cs Case) Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	for attempt := range 2 {
		res, err := c.roundTrip(ctx, sh, cs)
		if err == nil {
			return res
		}
		if attempt == 1 {
			return harnessError(fmt.Errorf("the container runner did not survive this case: %w", err))
		}
		c.shutdown()
		if err := c.dial(ctx); err != nil {
			return harnessError(fmt.Errorf("restarting the container runner: %w", err))
		}
	}
	return harnessError(errors.New("unreachable"))
}

func (c *containerConn) roundTrip(ctx context.Context, sh Found, cs Case) (Result, error) {
	if c.in == nil {
		return Result{}, errors.New("no runner")
	}
	// The session pointer must not cross the wire: on the far side this same
	// Found has to take the local route, which is what makes both columns one
	// implementation. It is unexported and so already invisible to
	// encoding/json; cleared here as well because the claim is load-bearing
	// and a reader should not have to know that rule to believe it.
	sh.sess = nil
	line, err := json.Marshal(Request{Shell: sh, Case: cs})
	if err != nil {
		return Result{}, err
	}
	if _, err := c.in.Write(append(line, '\n')); err != nil {
		return Result{}, err
	}
	reply, err := c.read(ctx)
	if err != nil {
		return Result{}, err
	}
	if reply.Err != "" {
		return harnessError(errors.New(reply.Err)), nil
	}
	if reply.Result == nil {
		return Result{}, errors.New("the runner answered without a result")
	}
	return *reply.Result, nil
}

// read takes one reply line, bounded by rather more than the runner's own
// per-case timeout. The runner enforces RunTimeout inside the container, so
// anything past this is the container or the daemon and not the shell.
func (c *containerConn) read(ctx context.Context) (Reply, error) {
	type outcome struct {
		line []byte
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		line, err := c.out.ReadBytes('\n')
		done <- outcome{line, err}
	}()
	select {
	case got := <-done:
		if got.err != nil && len(bytes.TrimSpace(got.line)) == 0 {
			return Reply{}, got.err
		}
		var r Reply
		if err := json.Unmarshal(bytes.TrimSpace(got.line), &r); err != nil {
			return Reply{}, fmt.Errorf("unreadable reply %q: %w", truncate(string(got.line)), err)
		}
		return r, nil
	case <-time.After(RunTimeout + 30*time.Second):
		return Reply{}, errors.New("the container runner stopped answering")
	case <-ctx.Done():
		return Reply{}, ctx.Err()
	}
}

func (c *containerConn) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown()
}

// shutdown ends the runner and the container. Closing standard input is the
// polite half — the runner returns on end of file — and the removal is what
// makes it certain.
func (c *containerConn) shutdown() {
	if c.in != nil {
		_ = c.in.Close()
		c.in = nil
	}
	if c.cmd != nil {
		_ = c.cmd.Wait()
		c.cmd = nil
	}
	c.out = nil
	c.remove()
}

func (c *containerConn) remove() {
	if c.id == "" {
		return
	}
	rm, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = exec.CommandContext(rm, c.cli, "rm", "-f", c.id).Run()
	c.id = ""
}

// runnerPath is where the cross-compiled runner is put inside the image. The
// root is the one directory every image has.
const runnerPath = "/oraclerunner"

var (
	runnerOnce sync.Once
	runnerBin  string
	runnerErr  error
)

// buildRunner cross-compiles the in-container half of the harness for the
// container's platform, once per process.
//
// It is built rather than committed for the reason every generated artifact
// here is: a binary checked in is a binary nobody re-derives, and this one has
// to be the *same tree* as the harness that sent the case. A runner built from
// an older commit would measure an older corpus and record it under this one.
func buildRunner(ctx context.Context) (string, error) {
	runnerOnce.Do(func() { runnerBin, runnerErr = compileRunner(ctx) })
	return runnerBin, runnerErr
}

func compileRunner(ctx context.Context) (string, error) {
	root, err := moduleRoot()
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "oracle-runner-")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "oraclerunner")
	build, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(build, "go", "build", "-o", bin, "./internal/cmd/oraclerunner")
	cmd.Dir = root
	// The container is Linux on whatever this machine is, and static: an
	// image with no libc at all still has to be able to run it.
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("cross-compiling the container runner: %s", strings.TrimSpace(string(out)))
	}
	return bin, nil
}

// moduleRoot walks up for the go.mod this package belongs to.
//
// The harness is run from the repository root by make and from a package
// directory by go test, and it has to build the same runner either way.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if b, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(b), "module "+modulePath) {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s module above the working directory, so the container runner cannot be built", modulePath)
		}
		dir = parent
	}
}

const modulePath = "github.com/blairham/sh"
