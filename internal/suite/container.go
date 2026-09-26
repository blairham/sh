// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blairham/sh/internal/oracle"
)

// The ash column, and the one thing that makes it possible to have at all.
//
// # Why this is not a second way to reach a shell
//
// There is no BusyBox on a stock macOS machine and no way to get one, so for
// a while `cmd/ash` was measured by hand and graded by nothing. #2263 fixed
// that for the oracle by making the route a value — LocalReach for every
// panel member that is a binary here, ContainerReach for the one that is not
// — and pinning the image by digest.
//
// This reads that same value. [oracle.Container] hands back the *same*
// [oracle.ContainerReach] the corpus runs against: the same repository, the
// same digest, the same runtime probe, the same pull. Nothing about reaching
// BusyBox is written down twice, because two pins for one shell would drift
// apart and both would look authoritative — which is this repository's
// most-repeated failure, and the reason #2263 made the route a value rather
// than a special case bolted beside the local path.
//
// # Why the whole sweep goes inside, rather than only the reference
//
// The obvious shape is to run the reference in the container and our binary
// here, and it is wrong. A suite file calls programs: the run would be
// comparing macOS against Alpine as well as `cmd/ash` against BusyBox, and
// every difference in a libc diagnostic or a coreutil would be scored as a
// disagreement between two shells. That is the confound this project has
// already been bitten by often enough to have a note about it.
//
// So both sides run in the same place, on the same files, and — exactly as
// [oracle.ContainerReach] runs the oracle's own Exec inside the image — what
// runs in there is [Sweep] itself, cross-compiled from this tree. The grader,
// the normalizer, the repeat check and the three numbers are one
// implementation. The host does no scoring at all: it builds, copies, starts,
// and unmarshals a [Report].
//
// # What the host still decides
//
// Whether the column runs. A machine with no container runtime gets an error
// out of [oracle.ContainerReach.Client] written for a person, and the caller
// prints it as a column that did not run. That is not a nicety: `ash: not run
// (docker is not reachable here)` and `ash: agrees` have to be impossible to
// confuse, and a column that vanished quietly reads as one that passed.

// Paths inside the image. The root is the one directory every image has, and
// the names are ours rather than anything the image ships.
//
// oursDir is a **directory** rather than the binary's own path, and that is
// the whole of #4674. The graded binary used to be copied in as `/suiteours`,
// and the name a shell is invoked under is an input to the shell: zsh reads
// the first letter of argv[0] and enters that shell's emulation, so `s` put
// every containerized run of `cmd/zsh` into `emulate sh` while the reference
// beside it ran as zsh. The whole column was a shell in one mode graded
// against a shell in another — `zsh/arith.tests` took C's precedences for its
// first five rows and then refused `$(( 08 + 1 ))`, at 5/38 lines and status
// 1, with nothing in the report able to say why.
//
// The path a binary lives at and the name it is invoked under are different
// things, and a harness that renames is the one place they part. So the
// directory carries the harness's name and the file carries the column's, the
// same trade the Makefile already makes for the `-own-bin` binaries under
// build/own/<dialect>. See [CheckInvocationName], which refuses the state
// rather than leaving the next one to be found by its figures.
const (
	insidePath = "/suiteinside"
	oursDir    = "/suiteours"
	suitePath  = "/suite"
)

// oursPath is where the graded binary lands inside the image: under oursDir,
// named for the column's dialect, so the word the shell finds in argv[0] is
// the one it is being graded as.
func oursPath(s Suite) string { return path.Join(oursDir, s.Dialect) }

// Contained says this column's reference shell is reached inside an image
// rather than as a binary on the machine running the harness.
//
// Two spellings answer it and they are one rule rather than two: a column
// whose shell the oracle panel already reaches that way names the panel
// member, and a column whose shell it does not reaches carries the pin
// itself. See [Suite.Container] and [Suite.Image].
func (s Suite) Contained() bool { return s.Container != "" || s.Image != "" }

// Reach is the container route this column's reference is reached by.
//
// It is the one place the two spellings are resolved, so that everything
// downstream — the pull, the ref printed in the report, the client probe —
// is the same code for both. [oracle.ContainerReach] is reused rather than
// reimplemented for the reason this repository has written down more than
// once: a second helper that does the same job is where a fix lands on one
// copy and not the other.
func (s Suite) Reach() (*oracle.ContainerReach, error) {
	if s.Container != "" {
		reach, ok := oracle.Container(s.Container)
		if !ok {
			return nil, fmt.Errorf("no container route named %q in the oracle's panel", s.Container)
		}
		return reach, nil
	}
	if s.Image == "" || s.Digest == "" {
		return nil, fmt.Errorf("the %s column is not reached inside a container", s.Name)
	}
	return &oracle.ContainerReach{Image: s.Image, Digest: s.Digest}, nil
}

// Reply is what the in-container half writes on its standard output: one
// report, or one reason there is not one.
//
// An envelope rather than an exit status, because "the reference was not
// BusyBox" and "the sweep failed" are different findings from "the container
// would not start", and the host has to print the right one. A bare non-zero
// exit would collapse all three into the same line.
type Reply struct {
	Err    string  `json:"err,omitempty"`
	Report *Report `json:"report,omitempty"`
}

// RunContained grades a column whose reference shell exists only inside a
// container image, and returns the report the sweep in there produced.
//
// ourPkg is the package under cmd/ to cross-compile as the dialect binary. It
// is built here rather than taken as a path because the binary `make suite`
// built for this machine is a Mach-O and the image is Linux — and because a
// binary built from some other commit would grade an older tree under this
// one, which is the reason the oracle builds its runner instead of shipping
// it.
func RunContained(ctx context.Context, s Suite, root, ourPkg string, opts Options) (Report, error) {
	reach, err := s.Reach()
	if err != nil {
		return Report{}, err
	}
	cli, err := reach.Client(ctx)
	if err != nil {
		return Report{}, err
	}
	if err := reach.Have(ctx, cli); err != nil {
		return Report{}, err
	}

	// Counted here so the bound on the whole run is derived from the work
	// rather than guessed. A fixed cap would be too tight the day a tier
	// grows and too loose to be worth having before that.
	files, err := plan(s, root, opts)
	if err != nil {
		return Report{}, err
	}

	stage, err := os.MkdirTemp("", "suite-contained-")
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()

	inside := filepath.Join(stage, "suiteinside")
	if err := oracle.BuildForContainer(ctx, "./internal/cmd/suiteinside", inside); err != nil {
		return Report{}, err
	}
	// Staged under a directory of its own so that the copy below can put the
	// *directory* inside: `docker cp dir ctr:/suiteours` creates the
	// destination and copies the contents into it, where a file copied to a
	// path under a directory that does not exist yet is refused.
	//
	// The file's name is read off [oursPath] rather than written out again,
	// because the two are one name: a staging name and a `-bin` name spelled
	// separately drift, and the drift is silent in the direction that matters
	// — `-bin` pointing at a path nothing was copied to is a column of `ours
	// never started`, which is at least loud, while the reverse is a column
	// graded under a name nobody chose.
	ours := filepath.Join(stage, "ours", path.Base(oursPath(s)))
	if err := os.MkdirAll(filepath.Dir(ours), 0o755); err != nil {
		return Report{}, err
	}
	if err := oracle.BuildForContainer(ctx, ourPkg, ours); err != nil {
		return Report{}, err
	}

	id, err := createContained(ctx, cli, reach.Ref(), s, opts)
	if err != nil {
		return Report{}, err
	}
	defer removeContained(cli, id)

	for _, put := range [][2]string{{inside, insidePath}, {filepath.Dir(ours), oursDir}, {root, suitePath}} {
		if err := copyInto(ctx, cli, id, put[0], put[1]); err != nil {
			return Report{}, err
		}
	}

	// Four runs of every file is the worst case — the reference, ours, and a
	// repeat of each — and the two minutes on top are the container's own
	// setup, which is not a shell's time.
	bound := time.Duration(len(files)+1)*4*opts.timeout() + 2*time.Minute
	run, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	var stdout, stderr bytes.Buffer
	start := exec.CommandContext(run, cli, "start", "-a", id)
	start.Stdout, start.Stderr = &stdout, &stderr
	if err := start.Run(); err != nil && stdout.Len() == 0 {
		return Report{}, fmt.Errorf("the sweep inside %s did not run: %w (%s)",
			reach.Ref(), err, strings.TrimSpace(stderr.String()))
	}

	var reply Reply
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &reply); err != nil {
		return Report{}, fmt.Errorf("the sweep inside %s answered unreadably: %w (%s)",
			reach.Ref(), err, strings.TrimSpace(stderr.String()))
	}
	if reply.Err != "" {
		return Report{}, errors.New(reply.Err)
	}
	if reply.Report == nil {
		return Report{}, errors.New("the sweep inside the container answered without a report")
	}
	rep := *reply.Report
	// Restored on this side: the value that crossed the wire is what the
	// sweep in there was given, and the caller's copy is the one with the
	// column's own Container field and its lookup list.
	rep.Suite = s
	rep.Route = reach.Route()
	return rep, nil
}

func createContained(ctx context.Context, cli, ref string, s Suite, opts Options) (string, error) {
	args := []string{
		"create", "--rm", "-i", "--network", "none", ref,
		insidePath,
		"-dialect", s.Dialect,
		"-root", suitePath,
		"-bin", oursPath(s),
		"-timeout", opts.timeout().String(),
		"-jobs", strconv.Itoa(opts.jobs()),
	}
	if len(opts.Only) > 0 {
		args = append(args, "-only", strings.Join(sortedNames(opts.Only), ","))
	}
	create, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	out, err := exec.CommandContext(create, cli, args...).Output()
	if err != nil {
		return "", fmt.Errorf("creating a container of %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// copyInto puts one file or directory inside, rather than bind-mounting it.
//
// A copy for [oracle.ContainerReach]'s measured reason: which host
// directories a bind mount can reach is a property of the container runtime —
// colima shares $HOME and not /tmp, Docker Desktop shares both — and a copy
// needs nothing shared. The cross-compiled binaries are staged under TMPDIR,
// which is exactly the directory colima does not share, so this is load
// bearing rather than tidy.
func copyInto(ctx context.Context, cli, id, src, dest string) error {
	cp, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(cp, cli, "cp", src, id+":"+dest).CombinedOutput()
	if err != nil {
		return fmt.Errorf("copying %s into the container: %s", filepath.Base(src), strings.TrimSpace(string(out)))
	}
	return nil
}

func removeContained(cli, id string) {
	if id == "" {
		return
	}
	rm, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = exec.CommandContext(rm, cli, "rm", "-f", id).Run()
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
