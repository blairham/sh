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
// # Why two denied policies and not one
//
// The three runs above are made twice, under two *shapes* of denied policy,
// because the shape decides which half of Policy.Allow does the refusing. A
// route aiming outside an allowed workspace is refused by there being no rule
// — a fallthrough past defaultFor and past the stat exemption an allowed exec
// earns. A route aiming at a region carved out of that workspace by a `deny`
// is refused by an early return. Nothing graded the second path for most
// selectors, and that is why #2044 stayed live: the sweep read 91 contained
// and 0 escaped on the same binary, the same afternoon, that handed a
// credential to a respelled path. Measured by breaking deny-overrides on
// purpose, the outside shape catches 24 escapes and the carved-out one 125.
//
// # What INERT is for
//
// It is not a skip. An INERT row is a route this shell cannot take *yet* —
// `mapfile` is not implemented, `history -w` falls through to an external
// command — and #1808's phrase for that state is "safe by accident". Those
// are the rows that become escapes the day the feature lands, so they are
// printed as a ledger rather than hidden, and the day one of them starts
// working it moves to CONTAINED or to ESCAPED on its own.
//
// # Why a documented escape is a verdict and not a paragraph
//
// One route here escapes on purpose. An allowed `exec` starts a process that
// makes its own system calls, so a child of the shell reads a path the
// policy denies — and that is not a defect to be fixed inside this
// repository, because containing a running child needs the operating system
// and the substrate does not ship an OS backend. The design doc states it
// outright, and since #4411 the grammar makes a policy state it too: the
// rule is spelled `allow exec-unconfined`.
//
// The obvious thing to do with a hole like that is leave it out of the sweep
// and describe it in prose. That was the state this package was in, and it
// is worse than it looks. #4409 found the route by measuring it by hand;
// what the table said was `exec/external`, which asks the *opposite*
// question — that a **denied** exec is refused — and whose one-line reason
// states the premise of the missing row and then grades the other half of
// it. A hole nothing measures is indistinguishable from a hole nobody has
// found, which is this package's own founding argument turned on the
// package.
//
// Grading it ESCAPED does not work either: the row would be permanently red,
// `make sandbox` would exit non-zero forever, and a red light nobody can
// ever turn green is trained away inside a week.
//
// So it has a verdict of its own, and the verdict earns two things a
// paragraph cannot:
//
//   - the day a Gate implementation contains the process tree — a seccomp
//     backend, Landlock, a sandbox profile — the denied run stops leaking and
//     the row moves to CONTAINED **by itself**, rather than waiting for
//     somebody to remember that a paragraph has gone stale;
//   - if the route ever stops working at all, the ungated run fails and it
//     reports INERT, out loud, instead of a documented hole quietly becoming
//     an unmeasured one.
//
// Both halves of the discipline above still apply to it, and neither is a
// formality. The route must work ungated or it is measuring nothing, and it
// is graded under both denied shapes, since #2044 was live for as long as it
// was because of the shape rather than the route.
//
// A DOCUMENTED row is not a pass. It does not fail the sweep, and it is
// printed as a ledger of its own, each row naming the line of the design doc
// that accounts for it.
package sandboxcheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// Documented means the route escaped, and the design doc says it does.
	//
	// It is the same three runs as Escaped and the same answer from the
	// filesystem; what differs is that a line of docs/design/sandboxing.md
	// names this hole and gives the reason it is one. See "Why a documented
	// escape is a verdict and not a paragraph" on the package.
	Documented
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
	case Documented:
		return "documented"
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
	// Root is the run's directory, and in the Outside shape it is also the
	// denied half of the world.
	Root string
	// Ws is the workspace inside it that every policy here permits. A
	// script runs from here, which is the shape a caller has: an agent given
	// a directory to work in, inside a tree it may not otherwise touch.
	Ws string
	// Shape is which denied policy this run is graded under, and Denied is
	// the region the routes aim at under it: Root in the Outside shape, and
	// a directory *inside* the workspace in the Carved one.
	//
	// Every path below hangs off Denied rather than off Root, which is what
	// lets one set of route scripts ask both questions. A route names
	// somewhere it may not go; what moves between the shapes is the reason
	// the policy says no — no allow covers it, or a deny does.
	Shape  Shape
	Denied string
	// Secret is a file in Denied holding SecretMark, for the routes that try
	// to read something they should not.
	Secret string
	// Target is a path in Denied that does not exist, for the routes that try
	// to create something where they should not.
	Target string
	// Victim is a file in Denied that does exist, for the routes that try to
	// remove or change something they should not.
	Victim string
	// VictimDir is a directory in Denied, likewise.
	VictimDir string
	// Link is a symbolic link *inside* the workspace whose target is Denied.
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
	// Sock is a path in Denied for the route that binds a unix socket, kept
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

// relative is the denied region as a script running in the workspace would
// name it without a path: `..` in the Outside shape, and the region's own
// name in the Carved one.
//
// It exists so that one route can ask the relative question under both
// shapes. A relative name is a route of its own — the shell resolves it and
// the gate sees whatever comes out — and hardcoding `..` would have left the
// carved-out shape with no relative row at all.
func (f Fixture) relative() string {
	if f.Shape == Carved {
		return filepath.Base(f.Denied)
	}
	return ".."
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

// Shape is the *shape* of the denied policy a row is graded under, and there
// are two of them because they ask different questions of the same gate.
//
// #2044 was live for as long as it was because of the shape every route was
// graded under rather than because of any route that was missing. The denied
// policy was `default deny` plus one allow, and every route but the spelling
// family aimed outside that allow — so a respelled path was refused for being
// outside, whatever the rule did, and the sweep reported 91 contained and 0
// escaped on the same binary, the same afternoon, that `read -r x <
// /proj/.ENV` handed over a credential.
//
// The two shapes are not two spellings of one question. A deny is an **early
// return** in Policy.Allow; the absence of an allow is a **fallthrough** past
// defaultFor and past the stat exemption that lets an allowed exec find its
// program. Grading every route under the first shape alone leaves the deny
// path untried for all but a handful of selectors.
type Shape int

const (
	// Outside grants the workspace and nothing else, and the route aims
	// beyond it. It answers: does the boundary of an allowed region hold?
	Outside Shape = iota
	// Carved grants the workspace and denies one region inside it, with the
	// route aiming at that region. It answers: does a deny hold *inside* a
	// region the policy otherwise allows?
	//
	// This is the shape a real caller writes. An agent is given a directory
	// and told which parts of it are off limits — `.env`, `.git/config`, a
	// secrets directory, a credentials file mounted in — and `default deny`
	// with one allow is the easy half.
	Carved
)

func (s Shape) String() string {
	if s == Carved {
		return "carved-out"
	}
	return "outside"
}

// Shapes is every denied policy shape a route is graded under.
var Shapes = []Shape{Outside, Carved}

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

// Result is one route on one dialect, under one denied policy shape.
type Result struct {
	Route   string
	Dialect string
	Shape   Shape
	Verdict Verdict
	// Cites is where the design doc accounts for this route escaping, copied
	// off the route so the report can print the ledger without looking the
	// row back up. Empty for every route that is not meant to escape.
	Cites string
	// The three runs, kept so a row that did not come out contained can be
	// explained without running it again.
	Runs [3]Outcome
}

// Report is the whole sweep.
type Report struct {
	Shell   string
	Results []Result
}

// Counts totals the verdicts over every shape.
func (r Report) Counts() map[Verdict]int {
	n := map[Verdict]int{}
	for _, res := range r.Results {
		n[res.Verdict]++
	}
	return n
}

// CountsFor totals the verdicts under one denied policy shape.
//
// Reported per shape rather than only in the total, because the two shapes
// refuse through different halves of Policy.Allow and a sum would let a
// deny-side hole be read as a route the shell does not have.
func (r Report) CountsFor(shape Shape) map[Verdict]int {
	n := map[Verdict]int{}
	for _, res := range r.Results {
		if res.Shape == shape {
			n[res.Verdict]++
		}
	}
	return n
}

// Failed reports whether the sweep found something wrong with the boundary.
//
// An inert row is not a failure: it is a route that is not open, which is a
// fact about the shell rather than a fault in the gate. An overblocked one
// is, for the reason given on the constant.
//
// Nor is a documented one. It is the one row here that escapes on purpose,
// and a permanently red light is a light people learn to read past — which
// would cost the sweep the rows that are red for a reason. It is counted and
// laid out in a ledger instead; see the package comment.
func (r Report) Failed() bool {
	n := r.Counts()
	return n[Escaped] > 0 || n[Overblocked] > 0
}

// AllDialects is every shell the core can be, as -dialect takes them.
var AllDialects = []string{"posix", "bash", "zsh", "ksh"}

// column is one shell the sweep grades: a binary, the dialect it is being,
// the words that select that dialect, and how a policy is named on it.
//
// Two routes reach the same shell and both are swept, which is the point.
// `sh -dialect bash -policy p` is the substrate driver being bash; `bash
// --policy p` is the binary `make install` puts on disk under the name a
// shebang, `chsh` and `login` use. They were not the same before #1826 —
// the second had no flag at all — and a sweep that only ever took the first
// is how that stayed invisible, in the same shape as the `-c` drift the
// shared front end exists to prevent.
type column struct {
	dialect    string
	bin        string
	args       []string
	policyFlag string
}

// Run grades every route on every dialect it applies to, through the
// substrate driver's `-dialect` and `-policy`.
func Run(shell, root, only string) (Report, error) {
	cols := make([]column, 0, len(AllDialects))
	for _, d := range AllDialects {
		cols = append(cols, column{
			dialect: d, bin: shell,
			args: []string{"-dialect", d}, policyFlag: "-policy",
		})
	}
	return run(cols, shell, root, only)
}

// RunBinaries grades the same routes against the dialect binaries themselves,
// through the `--policy` each one carries since #1826. bins maps a name in
// AllDialects to the binary that claims to be that shell; a dialect with no
// binary named is left out of the sweep rather than silently graded some
// other way.
func RunBinaries(bins map[string]string, root, only string) (Report, error) {
	var cols []column
	var named []string
	for _, d := range AllDialects {
		bin, ok := bins[d]
		if !ok {
			continue
		}
		named = append(named, bin)
		cols = append(cols, column{dialect: d, bin: bin, policyFlag: "--policy"})
	}
	return run(cols, strings.Join(named, " "), root, only)
}

func run(cols []column, label, root, only string) (Report, error) {
	rep := Report{Shell: label}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return rep, err
	}
	n := 0
	for _, rt := range Routes() {
		if only != "" && !strings.Contains(rt.Name, only) {
			continue
		}
		for _, col := range cols {
			if !slices.Contains(rt.dialects(), col.dialect) {
				continue
			}
			// Each shape gets its own three runs rather than borrowing the
			// first shape's ungated and allowed ones. The two shapes aim at
			// different paths, and a route that fails at the second set for
			// a reason of its own — a socket path over the limit, a
			// directory that is not there — would otherwise be credited to
			// the gate. That is exactly the false calm the three-run rule
			// exists to prevent, and it is not worth saving: the extra runs
			// cost seconds.
			for _, shape := range Shapes {
				res := Result{Route: rt.Name, Dialect: col.dialect, Shape: shape, Cites: rt.Cites}
				var did [3]bool
				for _, mode := range []Mode{Ungated, Denied, Allowed} {
					n++
					f, err := newFixture(root, n, shape)
					if err != nil {
						return rep, err
					}
					res.Runs[mode] = rt.run(col, f, mode)
					// Whether the route did its work is asked of the fixture
					// while it is still there, because most of the answers
					// are facts about the filesystem.
					did[mode] = rt.Did(f, res.Runs[mode])
					_ = os.RemoveAll(f.Root)
					// A route that does not work without a policy tells us
					// nothing about the policy, so the other two runs are
					// not worth making.
					if mode == Ungated && !did[Ungated] {
						break
					}
				}
				res.Verdict = verdictOf(did, rt.Cites != "")
				rep.Results = append(rep.Results, res)
			}
		}
	}
	sort.SliceStable(rep.Results, func(i, j int) bool {
		if rep.Results[i].Route != rep.Results[j].Route {
			return rep.Results[i].Route < rep.Results[j].Route
		}
		if rep.Results[i].Dialect != rep.Results[j].Dialect {
			return rep.Results[i].Dialect < rep.Results[j].Dialect
		}
		return rep.Results[i].Shape < rep.Results[j].Shape
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
//
// documented says the design doc accounts for this route escaping, and it is
// read in exactly one clause: it softens ESCAPED and touches nothing else.
// That placement is the whole of what makes the verdict worth having rather
// than a way of hiding a row. The inert clause still comes first, so a
// documented route that stops working says so instead of staying quiet; and
// a documented route that the gate starts containing falls through to
// CONTAINED on its own, with nobody editing this file.
func verdictOf(did [3]bool, documented bool) Verdict {
	switch {
	case !did[Ungated]:
		return Inert
	case did[Denied]:
		if documented {
			return Documented
		}
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
func newFixture(root string, n int, shape Shape) (Fixture, error) {
	// Short on purpose — see Fixture.Sock for the budget this is spending.
	f := Fixture{Root: filepath.Join(root, fmt.Sprintf("r%d", n)), Shape: shape}
	f.Ws = filepath.Join(f.Root, "ws")
	// Where the routes aim, and the whole of what the shape changes. In the
	// Outside shape it is the run's own root, one level above the workspace;
	// in the Carved one it is a directory inside the workspace, which the
	// denied policy names in a deny rule of its own.
	//
	// `off` rather than a plainer word because of the socket budget: sun_path
	// is 104 bytes counting the whole absolute path, and this shape spends
	// seven more of them than the other one.
	f.Denied = f.Root
	if shape == Carved {
		f.Denied = filepath.Join(f.Ws, "off")
	}
	f.Secret = filepath.Join(f.Denied, "secret")
	f.Target = filepath.Join(f.Denied, "target")
	f.Victim = filepath.Join(f.Denied, "victim")
	f.VictimDir = filepath.Join(f.Denied, "victimdir")
	f.Sock = filepath.Join(f.Denied, "s")
	if err := os.MkdirAll(f.Ws, 0o755); err != nil {
		return f, err
	}
	if err := os.MkdirAll(f.Denied, 0o755); err != nil {
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
	if err := os.Symlink(f.Denied, f.Link); err != nil {
		return f, err
	}
	return f, nil
}

// policy writes the rule set for one mode and returns its path, or "" for the
// ungated run.
//
// The denied set always grants the workspace and nothing else. What the
// fixture's shape changes is where the routes are aiming, and so which half
// of Policy.Allow refuses them:
//
//	Outside   the route aims above the workspace, and there is no rule
//	          covering it. The refusal is the fallthrough past defaultFor.
//	Carved    the route aims at a region inside the workspace, and a deny
//	          names it. The refusal is the early return, which nothing else
//	          here grades for most selectors.
//
// The allowed set grants the whole tree through `path`, which is the selector
// covering every kind that names one, plus signals, which name a process
// instead and so have to be said separately.
//
// grant is the denied set's one opening, and it belongs to a single route —
// see Route.Grant, which is the only caller that passes a non-empty one. It
// exists because "an allowed exec escapes" is not a question this shape can
// otherwise ask: with no exec rule at all, the child never starts, the row
// grades contained, and the contained verdict would be about the exec having
// been refused rather than about the route. Appended after the denies, but
// the order is not what makes it safe: what makes it safe is that it grants
// the exec slot only, so the path rules the route aims past are untouched.
// TestOnlyADocumentedRouteMayOpenTheDeniedPolicy holds that line.
func policy(f Fixture, mode Mode, grant []string) (string, error) {
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
		if f.Shape == Carved {
			// Two lines and not one. `<region>/**` covers what is *under*
			// the region and not the region itself, so a glob that reads the
			// directory would fall through to the allow beside it and
			// enumerate what the policy meant to hide — an escape written
			// into the instrument rather than found by it.
			lines = append(lines,
				"deny path "+f.Denied,
				"deny path "+f.Denied+"/**")
		}
		lines = append(lines, grant...)
	case Allowed:
		// `exec` is named on its own because `path` does not reach it (#4409),
		// and it is named here because this shape has to permit *everything*
		// or a route it forbids reports Overblocked and reads as a bug in the
		// shell rather than as a gap in the fixture.
		lines = []string{
			"version 1", "default deny",
			"allow path /**", "allow exec-unconfined /**", "allow signal",
		}
	}
	// Beside the run rather than inside the workspace, so that a route which
	// enumerates or writes the workspace cannot see the file deciding its own
	// fate. The policy is apparatus rather than subject and never passes the
	// gate — see driver/sandbox.go — but it should not be in the frame
	// either.
	at := filepath.Join(f.Root, "rules.policy")
	return at, os.WriteFile(at, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

// run executes one route once.
func (rt Route) run(col column, f Fixture, mode Mode) Outcome {
	p, err := policy(f, mode, rt.Grant)
	if err != nil {
		return Outcome{Err: err.Error(), Code: -1}
	}
	args := slices.Clone(col.args)
	if p != "" {
		args = append(args, col.policyFlag, p)
	}
	// A route's own arguments go in front of `-c`, because what they say is
	// what kind of shell this is rather than what it should run — see
	// Route.Args, and `builtin/fc-write`, which is the one row that needs a
	// shell that thinks somebody is watching.
	args = append(args, rt.Args...)
	args = append(args, "-c", rt.script(f))
	cmd := exec.Command(col.bin, args...)
	// From inside the workspace, because that is where a caller puts a
	// script it is sandboxing, and because it makes a relative `../` route
	// mean what it means in the wild.
	cmd.Dir = f.Ws
	// A stripped environment: the shell must not pick up the grader's own
	// HOME or PATH and read a startup file or find a program that happens to
	// be installed on the machine running this. PATH is named explicitly so
	// the exec route is asking about a program that exists everywhere.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + f.Root, "SHELL=" + col.bin}
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
