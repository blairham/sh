// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package sandboxcheck grades the boundary from outside the process, by
// running the binary that ships and asking the filesystem what happened.
//
// # Why this exists when the gate already has tests
//
// The unit tests are written by somebody who already knows where the
// boundary is, and they test the routes that were thought of. Every escape
// this repository has had got in the other way: `sysopen` and `zsystem
// flock` opened files with no gate (#1805), `autoload` read and *ran* one
// (#1812), and every mutating `zsh/files` builtin changed the filesystem
// with nothing consulted (#1819) — each one a route nobody had listed, in
// code whose author had no reason to know a gate existed.
//
// So this enumerates the routes instead of the rules: every way a script can
// reach the filesystem, run as a script, against the real binary. A route
// that arrives without a gate shows up as a row, not as a silence.
//
// # Why three runs and not one
//
// The failure mode of a sandbox test is not a false alarm, it is a false
// calm: sixteen routes reporting "refused" from a shell that never started
// looks exactly like a sandbox that works. That has happened here more than
// once, which is why no row is decided by one run. Each route runs three
// times, in a fresh directory each time:
//
//	ungated   no policy at all. The route must WORK, or it is measuring
//	          nothing and is reported INERT rather than passing.
//	denied    a policy granting only the workspace. The route must FAIL,
//	          or it ESCAPED.
//	allowed   a policy granting the whole tree. The route must WORK, or the
//	          gate is refusing what it was told to permit: OVERBLOCKED.
//
// A row is CONTAINED only when all three agree. A shell that does nothing
// scores zero rather than perfect, and a gate that refuses everything scores
// zero too — which is the property that makes the number worth reading.
//
// # What INERT is for
//
// It is not a skip. An INERT row is a route this shell cannot take *yet* —
// `mapfile` is not implemented, `history -w` falls through to an external
// command — and #1808's phrase for that state is "safe by accident". Those
// are the rows that become escapes the day the feature lands, so they are
// printed as a ledger rather than hidden, and the day one of them starts
// working it moves to CONTAINED or to ESCAPED on its own.
package sandboxcheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Verdict is what one route came to on one dialect.
type Verdict int

const (
	// Contained is the only passing answer: the route works, the policy
	// stops it, and a policy that permits it lets it through again.
	Contained Verdict = iota
	// Escaped means the policy did not stop it. The route did its work with
	// a rule in place that forbids exactly that.
	Escaped
	// Overblocked means a policy that permits the route refused it anyway.
	// A boundary that cannot be opened where it was told to is as broken as
	// one that cannot be closed, and it is the failure people actually turn
	// sandboxing off over.
	Overblocked
	// Inert means the route did not work even with no policy, so the other
	// two runs say nothing about the gate. Not a pass and not a failure —
	// a note that this way in is not open yet.
	Inert
)

func (v Verdict) String() string {
	switch v {
	case Contained:
		return "contained"
	case Escaped:
		return "ESCAPED"
	case Overblocked:
		return "OVERBLOCKED"
	case Inert:
		return "inert"
	}
	return "?"
}

// Fixture is one run's own corner of the filesystem.
//
// Rebuilt for every single run rather than shared between the three, because
// the ungated run *succeeds* by design: it deletes the file, or creates the
// target, or takes the lock. A second run against what it left would be
// measuring the leftovers — a `rm` route would report "refused" for a file
// the first run had already removed.
type Fixture struct {
	// Root is the run's directory and the denied half of the world.
	Root string
	// Ws is the workspace inside it that every policy here permits. A
	// script runs from here, which is the shape a caller has: an agent given
	// a directory to work in, inside a tree it may not otherwise touch.
	Ws string
	// Secret is a file in Root holding SecretMark, for the routes that try
	// to read something they should not.
	Secret string
	// Target is a path in Root that does not exist, for the routes that try
	// to create something where they should not.
	Target string
	// Victim is a file in Root that does exist, for the routes that try to
	// remove or change something they should not.
	Victim string
	// VictimDir is a directory in Root, likewise.
	VictimDir string
	// Link is a symbolic link *inside* the workspace whose target is Root.
	//
	// It is the one fixture entry that is a way in rather than a thing to
	// reach, and it is here because a rule matches a name while an open
	// reaches an object. Every other route names somewhere it may not go, so
	// the policy can refuse it by reading the name; a route through this one
	// names somewhere it *may* go and arrives outside anyway, which is the
	// only shape that tests the resolution rather than the glob.
	//
	// It is placed by the fixture rather than made by the script on purpose.
	// A script can make one — `zf_ln -s` is in the workspace and allowed
	// there — but then the row would be graded on whether that builtin
	// exists, and the escape is the core's, reachable from a workspace that
	// simply has a link in it. Which is the ordinary case: an agent is given
	// a directory, and a directory somebody uses has links in it.
	Link string
	// Sock is a path in Root for the route that binds a unix socket, kept
	// deliberately short: sun_path is 104 bytes on Darwin and 108 on Linux,
	// counting the whole absolute path, so a fixture named as plainly as
	// the others pushes a checkout of ordinary depth over the limit. The
	// bind then fails for a reason that has nothing to do with the policy
	// and the row grades inert — safe, because inert is not a pass, but it
	// hides the route rather than testing it.
	Sock string
	// Carved is a file *inside* the workspace that the denied policy names in
	// a deny rule of its own, and the spelling routes are the only ones that
	// use it.
	//
	// Every other route here aims outside the workspace, so the denied policy
	// refuses it with `default deny` and one allow. That shape cannot see
	// #2044: a respelled path outside the workspace is refused because it is
	// outside, whatever the rule did, so a sweep built only from it stays
	// green while the fold is missing. The carve-out — allow the workspace,
	// deny one file in it — is the shape an agent sandbox is actually given,
	// and it is the shape in which a name the deny fails to cover lands on
	// the allow beside it.
	Carved string
	// CarvedUpper and CarvedDirUpper are the same file under the other names
	// the volume answers to: the leaf respelled, and a directory component
	// respelled. Two entries rather than one because a fold that reached only
	// the last component would close the demonstration and not the hole.
	CarvedUpper    string
	CarvedDirUpper string
	// CarvedAccent is a carved-out file under a directory whose name has a
	// character with two spellings, and CarvedAccentNFD is the same path
	// written the other way — `caf\u00e9` against `cafe\u0301`. They render
	// identically and differ in bytes, which is the whole of #2045.
	//
	// A directory rather than the leaf on purpose: the leaf is already
	// covered for case, and putting the interesting character one component
	// up grades the fold over a path rather than over a filename.
	CarvedAccent    string
	CarvedAccentNFD string
}

// SecretMark is what a read route is looking for. Distinctive enough that a
// substring match cannot hit it by accident, which matters because that match
// is the whole verdict for a read.
//
// Words rather than the random-looking string this was first written as: a
// high-entropy literal beside the name "secret" is what a credential scanner
// is built to find, and gitleaks duly refused the commit. Words carry the
// same meaning to a reader and cannot be mistaken for a key.
const SecretMark = "THE-CONTENTS-OF-A-FILE-THE-POLICY-HIDES"

// Mode is which of the three runs this is.
type Mode int

const (
	Ungated Mode = iota
	Denied
	Allowed
)

// Outcome is what one run of one script produced.
type Outcome struct {
	Out, Err string
	Code     int
}

// Says reports whether either stream carried this text. Used by routes whose
// effect is something the script printed rather than something on disk.
func (o Outcome) Says(text string) bool {
	return strings.Contains(o.Out, text) || strings.Contains(o.Err, text)
}

// Result is one route on one dialect.
type Result struct {
	Route   string
	Dialect string
	Verdict Verdict
	// The three runs, kept so a row that did not come out contained can be
	// explained without running it again.
	Runs [3]Outcome
}

// Report is the whole sweep.
type Report struct {
	Shell   string
	Results []Result
}

// Counts totals the verdicts.
func (r Report) Counts() map[Verdict]int {
	n := map[Verdict]int{}
	for _, res := range r.Results {
		n[res.Verdict]++
	}
	return n
}

// Failed reports whether the sweep found something wrong with the boundary.
//
// An inert row is not a failure: it is a route that is not open, which is a
// fact about the shell rather than a fault in the gate. An overblocked one
// is, for the reason given on the constant.
func (r Report) Failed() bool {
	n := r.Counts()
	return n[Escaped] > 0 || n[Overblocked] > 0
}

// AllDialects is every shell the core can be, as -dialect takes them.
//
// The dialect binaries are not driven directly because they have no -policy
// flag of their own: the sandbox is reachable through `sh -dialect X`, and
// that asymmetry is worth knowing about but is not what this grades.
var AllDialects = []string{"posix", "bash", "zsh", "ksh"}

// Run grades every route on every dialect it applies to.
func Run(shell, root, only string) (Report, error) {
	rep := Report{Shell: shell}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return rep, err
	}
	n := 0
	for _, rt := range Routes() {
		if only != "" && !strings.Contains(rt.Name, only) {
			continue
		}
		for _, d := range rt.dialects() {
			res := Result{Route: rt.Name, Dialect: d}
			var did [3]bool
			for _, mode := range []Mode{Ungated, Denied, Allowed} {
				n++
				f, err := newFixture(root, n)
				if err != nil {
					return rep, err
				}
				res.Runs[mode] = rt.run(shell, d, f, mode)
				// Whether the route did its work is asked of the fixture
				// while it is still there, because most of the answers are
				// facts about the filesystem.
				did[mode] = rt.Did(f, res.Runs[mode])
				_ = os.RemoveAll(f.Root)
				// A route that does not work without a policy tells us
				// nothing about the policy, so the other two runs are not
				// worth making.
				if mode == Ungated && !did[Ungated] {
					break
				}
			}
			res.Verdict = verdictOf(did)
			rep.Results = append(rep.Results, res)
		}
	}
	sort.SliceStable(rep.Results, func(i, j int) bool {
		if rep.Results[i].Route != rep.Results[j].Route {
			return rep.Results[i].Route < rep.Results[j].Route
		}
		return rep.Results[i].Dialect < rep.Results[j].Dialect
	})
	return rep, nil
}

// verdictOf reads the three runs.
//
// Separated from the running so that the rule can be stated once and tested
// without a shell: the order of the clauses *is* the definition, and the
// first one is the one that matters. A route that did nothing ungated is
// inert whatever the other two did, because a policy cannot be credited with
// stopping something that was never going to happen — which is the failure
// this whole instrument is shaped around.
func verdictOf(did [3]bool) Verdict {
	switch {
	case !did[Ungated]:
		return Inert
	case did[Denied]:
		return Escaped
	case !did[Allowed]:
		return Overblocked
	}
	return Contained
}

// newFixture builds one run's directory.
//
// Under the root the caller named rather than under os.TempDir(), and that is
// a decision rather than a detail. A workspace inside the temporary directory
// is the classic way to write a sandbox test that asserts nothing: anything
// carving out temporary space for commands to work in exempts the very thing
// under test, and four "a write outside the workspace is refused" tests once
// passed against a policy that was never consulted. Nothing in this shell
// carves out TMPDIR today — but the test that proves it must not be the one
// that would break silently if something did.
func newFixture(root string, n int) (Fixture, error) {
	// Short on purpose — see Fixture.Sock for the budget this is spending.
	f := Fixture{Root: filepath.Join(root, fmt.Sprintf("r%d", n))}
	f.Ws = filepath.Join(f.Root, "ws")
	f.Secret = filepath.Join(f.Root, "secret")
	f.Target = filepath.Join(f.Root, "target")
	f.Victim = filepath.Join(f.Root, "victim")
	f.VictimDir = filepath.Join(f.Root, "victimdir")
	f.Sock = filepath.Join(f.Root, "s")
	if err := os.MkdirAll(f.Ws, 0o755); err != nil {
		return f, err
	}
	if err := os.MkdirAll(filepath.Join(f.VictimDir, "entry"), 0o755); err != nil {
		return f, err
	}
	if err := os.WriteFile(f.Secret, []byte(SecretMark+"\n"), 0o600); err != nil {
		return f, err
	}
	if err := os.WriteFile(f.Victim, []byte("victim\n"), 0o600); err != nil {
		return f, err
	}
	carvedDir := filepath.Join(f.Ws, "carved")
	if err := os.MkdirAll(carvedDir, 0o755); err != nil {
		return f, err
	}
	f.Carved = filepath.Join(carvedDir, "secret.txt")
	f.CarvedUpper = filepath.Join(carvedDir, "SECRET.TXT")
	f.CarvedDirUpper = filepath.Join(f.Ws, "CARVED", "secret.txt")
	if err := os.WriteFile(f.Carved, []byte(SecretMark+"\n"), 0o600); err != nil {
		return f, err
	}
	accentDir := filepath.Join(f.Ws, "caf\u00e9")
	if err := os.MkdirAll(accentDir, 0o755); err != nil {
		return f, err
	}
	f.CarvedAccent = filepath.Join(accentDir, "secret.txt")
	f.CarvedAccentNFD = filepath.Join(f.Ws, "cafe\u0301", "secret.txt")
	if err := os.WriteFile(f.CarvedAccent, []byte(SecretMark+"\n"), 0o600); err != nil {
		return f, err
	}
	f.Link = filepath.Join(f.Ws, "out")
	if err := os.Symlink(f.Root, f.Link); err != nil {
		return f, err
	}
	return f, nil
}

// policy writes the rule set for one mode and returns its path, or "" for the
// ungated run.
//
// The denied set grants the workspace and nothing else, so every route here —
// each of which aims outside it — is refused by a rule rather than by there
// being no rule. The allowed set grants the whole tree through `path`, which
// is the selector covering every kind that names one, plus signals, which
// name a process instead and so have to be said separately.
func policy(f Fixture, mode Mode) (string, error) {
	var lines []string
	switch mode {
	case Ungated:
		return "", nil
	case Denied:
		// The carve-out is what makes the spelling routes mean anything; see
		// Fixture.Carved. It is written with the name the file actually has,
		// so a route reaching it by another spelling is reaching past a rule
		// rather than past the absence of one.
		lines = []string{
			"version 1", "default deny",
			"allow path " + f.Ws + "/**",
			"deny path " + f.Carved,
			"deny path " + f.CarvedAccent,
		}
	case Allowed:
		lines = []string{"version 1", "default deny", "allow path /**", "allow signal"}
	}
	// Beside the run rather than inside the workspace, so that a route which
	// enumerates or writes the workspace cannot see the file deciding its own
	// fate. The policy is apparatus rather than subject and never passes the
	// gate — see cmd/sh/sandbox.go — but it should not be in the frame
	// either.
	at := filepath.Join(f.Root, "rules.policy")
	return at, os.WriteFile(at, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

// run executes one route once.
func (rt Route) run(shell, dialect string, f Fixture, mode Mode) Outcome {
	p, err := policy(f, mode)
	if err != nil {
		return Outcome{Err: err.Error(), Code: -1}
	}
	args := []string{"-dialect", dialect}
	if p != "" {
		args = append(args, "-policy", p)
	}
	args = append(args, "-c", rt.script(f))
	cmd := exec.Command(shell, args...)
	// From inside the workspace, because that is where a caller puts a
	// script it is sandboxing, and because it makes a relative `../` route
	// mean what it means in the wild.
	cmd.Dir = f.Ws
	// A stripped environment: the shell must not pick up the grader's own
	// HOME or PATH and read a startup file or find a program that happens to
	// be installed on the machine running this. PATH is named explicitly so
	// the exec route is asking about a program that exists everywhere.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + f.Root, "SHELL=" + shell}
	var out, errs strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errs
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if ok := asExit(err, &ee); ok {
			code = ee.ExitCode()
		} else {
			code = -1
			errs.WriteString(err.Error())
		}
	}
	return Outcome{Out: out.String(), Err: errs.String(), Code: code}
}
