// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package emulatesweep measures how far `emulate MODE` reaches into the
// grammar, by asking every corpus snippet whether the three modes disagree
// about it.
//
// It implements no shell behavior. It writes a script, runs a shell binary
// over it and records the exit status, which is what keeps it clean under
// CLEANROOM.md: the snippets are ours and the status is a fact about the
// binary.
//
// # What it asks
//
// Each snippet is run as a file of three lines — `emulate MODE`, `set -n`,
// then the snippet — so the mode is in force and nothing executes. A snippet
// **moves with the mode** when the three modes do not agree about whether it
// parses. The reference moves on 155 of the 4485 corpus snippets and this
// shell moves on none of them (#4734, measured 2026-09-26), because the mode
// reaches the option table and the semantics vector and never reaches
// syntax.Dialect at all.
//
// The number the epic closes on is not either of those two counts but the
// **disagreement** between them: the snippets where this shell and the
// reference differ about whether the modes differ. That is 155 today and the
// bar is zero.
//
// # Why the file rather than `-c`
//
// The route is not what decides it. A file is the discriminator: both shells
// read one a line at a time, so the `emulate` has already run, on its own
// line, before the line that refuses is read. A `-c` string read whole cannot
// explain that, and neither can an `argv[0]` of `sh`, which moves the same
// rows. So the noun is the mode, not the route.
//
// # Why the controls are not optional
//
// "Nothing moved" is also what a harness that cannot see a parse error says,
// and this shell's answer today *is* a null — zero of 4485. So the sweep
// fires two controls on **every** binary it points at before it reports
// anything, and refuses to report at all if either fails:
//
//	{ fi; }   must be refused under all three modes
//	echo hi   must be refused under none
//
// The first proves a refusal is visible from this binary and the second that
// one is not manufactured. A sweep broken into reporting 0 fails rather than
// reads as success. TestTheControlsFireOnOurOwnBinary pins the same pair as a
// test, against the binary that is always in the tree.
//
// # Why per-prefix counts
//
// A total that falls is not a per-row check. The 155 spread over seventeen
// corpus prefixes — `pat` 64, `core` 21, `cond` 16, `parameter` 15 and a long
// tail — and each epic row owns one family, so a row that made its own family
// worse while another improved would be invisible in the total. The report
// prints the split by prefix on every run.
package emulatesweep

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blairham/sh/internal/oracle"
)

// Modes are the emulations, in the order every verdict is written in.
//
// zsh is included although it is the shell's own mode: without it a snippet
// the *native* grammar refuses and a mode accepts would read as agreement,
// and there are two of those (`cmd/close-brace-ends-the-word-it-ends` and
// `cmd/close-brace-as-an-ordinary-word`). They are why this is a grammar
// vector per mode rather than a narrowing of zsh's.
var Modes = [3]string{"zsh", "sh", "ksh"}

// perCase bounds one shell run by default. A snippet under `set -n` is parsed
// and not run, so anything that takes longer than this is hung rather than
// slow.
const perCase = 10 * time.Second

// Binary is one shell the sweep points at.
type Binary struct {
	// Name is how the report calls it: "reference" or "ours".
	Name string

	// Path is the binary. It is run with -f and a script path, under a
	// scrubbed environment, so nothing on the machine reaches it.
	Path string

	// Version is what the binary says it is, printed beside every number it
	// produced. A reference that is really our own build agrees with
	// everything, and the version string is the cheapest thing that tells
	// them apart.
	Version string

	// Timeout is how long one run of this binary may take. Zero means
	// perCase. It is per binary rather than global because the two sides of
	// a comparison are two programs, and a budget is a fact about the one it
	// is applied to.
	Timeout time.Duration
}

func (b Binary) timeout() time.Duration {
	if b.Timeout > 0 {
		return b.Timeout
	}
	return perCase
}

// Verdict is one snippet's answer from one binary: whether each mode refused
// it at the parse.
type Verdict struct {
	ID      string
	Refused [3]bool

	// Hung records a mode whose run did not finish, which is neither a
	// refusal nor an acceptance. It is a column of its own rather than a
	// third value on Refused because a hang that was folded into "not
	// refused" would read as the mode reaching nothing — and this shell's
	// whole answer today is a null, so the one shape that must not be
	// manufactured is another zero.
	//
	// It is not hypothetical: the reference hangs on
	// `cmd/function-keyword-with-a-name-holding-a-dollar` under `emulate
	// ksh`, measured 2026-09-27 against zsh 5.9.2, and the first full run of
	// this sweep died on it.
	Hung [3]bool
}

// Unmeasured reports whether any mode failed to answer, in which case the
// snippet is named in the report and kept out of every count.
func (v Verdict) Unmeasured() bool { return v.Hung[0] || v.Hung[1] || v.Hung[2] }

// Moves reports whether the modes disagree about this snippet.
func (v Verdict) Moves() bool {
	rel := v.Relative()
	return rel[1] || rel[2]
}

// Relative is the verdict read against the shell's own native mode: what
// each emulation *changed*, rather than what it answered.
//
// This is the noun the bar is counted on, and the distinction is not
// cosmetic. A snippet this shell refuses in all three modes while the
// reference accepts it in all three is an ordinary grammar gap — the corpus
// already grades those, and counting them here would swamp the emulation
// signal with several hundred rows that have nothing to do with it. Reading
// each mode against its own binary's native answer removes exactly that, and
// leaves the thing `emulate` is being asked about.
func (v Verdict) Relative() [3]bool {
	return [3]bool{false, v.Refused[1] != v.Refused[0], v.Refused[2] != v.Refused[0]}
}

// String renders the verdict as the report writes it: [T F T].
func (v Verdict) String() string {
	var b strings.Builder
	b.WriteByte('[')
	for i, r := range v.Refused {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch {
		case v.Hung[i]:
			b.WriteByte('?')
		case r:
			b.WriteByte('T')
		default:
			b.WriteByte('F')
		}
	}
	b.WriteByte(']')
	return b.String()
}

// Prefix is the family a corpus ID belongs to — everything before the first
// slash. It is what the epic's rows are cut along.
func Prefix(id string) string {
	if i := strings.IndexByte(id, '/'); i >= 0 {
		return id[:i]
	}
	return id
}

// Control is a snippet whose answer is known before the sweep runs, and whose
// disagreeing would mean the instrument rather than the shell is what moved.
type Control struct {
	Snippet string

	// Want is the verdict every binary must give. It is deliberately the
	// same for both: a control whose expected answer differs per binary is
	// measuring the shells, which is the population's job.
	Want [3]bool

	// Why says what a failure of this control would have hidden.
	Why string
}

// Controls are fired on every binary before the population, and a failure
// stops the sweep rather than being reported beside the count.
var Controls = []Control{
	{
		Snippet: "{ fi; }",
		Want:    [3]bool{true, true, true},
		Why:     "the positive: a parse refusal is visible from this binary at all",
	},
	{
		Snippet: "echo hi",
		Want:    [3]bool{false, false, false},
		Why:     "the negative: a refusal is not manufactured where there is none",
	},
}

// ControlFailure is a control that did not answer what it must.
type ControlFailure struct {
	Binary  string
	Control Control
	Got     Verdict
}

func (f ControlFailure) Error() string {
	want := Verdict{Refused: f.Control.Want}
	return fmt.Sprintf("control %q on %s: got %s, want %s — %s",
		f.Control.Snippet, f.Binary, f.Got, want, f.Control.Why)
}

// Options is one sweep.
type Options struct {
	// Reference is the shell whose movement is the target, and Ours the one
	// being graded against it.
	Reference Binary
	Ours      Binary

	// Cases is the population. oracle.Corpus is the whole of it.
	Cases []oracle.Case

	// Only, when set, keeps the cases whose ID contains it. For triaging one
	// family without paying for the other sixteen.
	Only string

	// Jobs is how many shells run at once.
	Jobs int
}

// Result is what one sweep found.
type Result struct {
	Reference Binary
	Ours      Binary

	// Total is how many snippets were asked, and Scored how many of them
	// produced an answer from both binaries.
	Total  int
	Scored int

	// Unmeasured are the snippets a binary did not finish. They are named
	// rather than dropped: a snippet silently removed from the denominator
	// is a row nobody is working on that nothing reports.
	Unmeasured []Unmeasured

	// RefMoves and OurMoves are how many snippets each binary moves on.
	RefMoves int
	OurMoves int

	// Disagreements are the snippets where the two binaries differ about
	// whether the modes differ. Its length is the number the epic closes on.
	Disagreements []Disagreement
}

// Unmeasured is one snippet a binary would not finish, and which binary.
type Unmeasured struct {
	ID     string
	Binary string
	Got    Verdict
}

// Disagreement is one snippet the two binaries answer differently.
type Disagreement struct {
	ID  string
	Ref Verdict
	Our Verdict
}

// Split is how the reference's movers divide between the modes. It is here
// because the division is the evidence that the modes are not a subset
// lattice, which is the assumption a table would otherwise be built on.
type Split struct {
	Both     int // refused under sh and ksh, accepted under zsh's own mode
	ShOnly   int
	KshOnly  int
	Reversed int // refused under zsh's own mode and accepted by a mode
	Other    int
}

// Sweep asks every case of both binaries, after firing the controls on each.
//
// An error is returned for a failed control and for a harness fault, and in
// neither case is a count produced: a number from an instrument that has not
// shown it can fire is not a measurement.
func Sweep(ctx context.Context, o Options) (*Result, error) {
	dir, err := os.MkdirTemp("", "emulatesweep")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	for _, b := range []Binary{o.Reference, o.Ours} {
		if err := fireControls(ctx, dir, b); err != nil {
			return nil, err
		}
	}

	cases := o.Cases
	if o.Only != "" {
		kept := make([]oracle.Case, 0, len(cases))
		for _, c := range cases {
			if strings.Contains(c.ID, o.Only) {
				kept = append(kept, c)
			}
		}
		cases = kept
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no cases selected (-only %q): a sweep of nothing reports the same zero as a sweep that found nothing", o.Only)
	}

	jobs := o.Jobs
	if jobs < 1 {
		jobs = 1
	}

	refs := make([]Verdict, len(cases))
	ours := make([]Verdict, len(cases))
	errs := make([]error, len(cases))

	var wg sync.WaitGroup
	sem := make(chan struct{}, jobs)
	for i, c := range cases {
		wg.Add(1)
		go func(i int, c oracle.Case) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var err error
			if refs[i], err = Ask(ctx, dir, o.Reference, c.ID, c.Snippet); err != nil {
				errs[i] = err
				return
			}
			if ours[i], err = Ask(ctx, dir, o.Ours, c.ID, c.Snippet); err != nil {
				errs[i] = err
			}
		}(i, c)
	}
	wg.Wait()

	res := &Result{Reference: o.Reference, Ours: o.Ours, Total: len(cases)}
	for i := range cases {
		if errs[i] != nil {
			return nil, errs[i]
		}
		if refs[i].Unmeasured() || ours[i].Unmeasured() {
			if refs[i].Unmeasured() {
				res.Unmeasured = append(res.Unmeasured, Unmeasured{ID: cases[i].ID, Binary: o.Reference.Name, Got: refs[i]})
			}
			if ours[i].Unmeasured() {
				res.Unmeasured = append(res.Unmeasured, Unmeasured{ID: cases[i].ID, Binary: o.Ours.Name, Got: ours[i]})
			}
			continue
		}
		res.Scored++
		if refs[i].Moves() {
			res.RefMoves++
		}
		if ours[i].Moves() {
			res.OurMoves++
		}
		// The bar is the *relative* verdict, so an ordinary grammar gap —
		// one that shifts all three modes together — is not counted here.
		if refs[i].Relative() != ours[i].Relative() {
			res.Disagreements = append(res.Disagreements, Disagreement{
				ID: cases[i].ID, Ref: refs[i], Our: ours[i],
			})
		}
	}
	return res, nil
}

func fireControls(ctx context.Context, dir string, b Binary) error {
	for _, c := range Controls {
		got, err := Ask(ctx, dir, b, "control", c.Snippet)
		if err != nil {
			return err
		}
		if got.Unmeasured() || got.Refused != c.Want {
			return ControlFailure{Binary: b.Name, Control: c, Got: got}
		}
	}
	return nil
}

// Ask runs one snippet under all three modes on one binary.
//
// Refusal is a nonzero exit status. Under `set -n` nothing the snippet
// contains can run, so the only thing left to fail is the parse — which is
// also why the sweep does not match on the wording of a diagnostic, a thing
// two shells have no reason to spell alike.
func Ask(ctx context.Context, dir string, b Binary, id, snippet string) (Verdict, error) {
	v := Verdict{ID: id}
	// Resolved once, here, where the caller's directory still means what the
	// person who typed the path meant. Every run happens inside a scratch
	// directory of its own, so a relative -bin would resolve against that and
	// start nothing at all — and a shell that never started answers every
	// mode alike, which reads as a shell the mode does not reach.
	abs, err := filepath.Abs(b.Path)
	if err != nil {
		return v, err
	}
	b.Path = abs
	for i, mode := range Modes {
		refused, hung, err := askOne(ctx, dir, b, mode, snippet)
		if err != nil {
			return v, fmt.Errorf("%s: %s under emulate %s: %w", b.Name, id, mode, err)
		}
		v.Refused[i], v.Hung[i] = refused, hung
	}
	return v, nil
}

func askOne(ctx context.Context, dir string, b Binary, mode, snippet string) (refused, hung bool, err error) {
	run, err := os.MkdirTemp(dir, "run")
	if err != nil {
		return false, false, err
	}
	defer os.RemoveAll(run)

	script := filepath.Join(run, "case.zsh")
	body := fmt.Sprintf("emulate %s\nset -n\n%s\n", mode, snippet)
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		return false, false, err
	}

	ctx, cancel := context.WithTimeout(ctx, b.timeout())
	defer cancel()

	// -f so no startup file is read, and an environment holding only what a
	// process needs to start: an rc file or an FPATH on the machine running
	// this would be an input to the measurement, and a deficient one reads
	// exactly like a difference between the shells.
	cmd := exec.CommandContext(ctx, b.Path, "-f", script)
	cmd.Dir = run
	cmd.Env = []string{"HOME=" + run, "ZDOTDIR=" + run, "PATH=/usr/bin:/bin", "TERM=dumb"}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	err = cmd.Run()
	if ctx.Err() != nil {
		// A hung shell is neither a refusal nor an acceptance, and calling it
		// either would move a row for a reason that has nothing to do with
		// the mode. The snippet is named in the report instead.
		return false, true, nil
	}
	if err == nil {
		return false, false, nil
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return false, false, err
	}
	return true, false, nil
}

// ByPrefix counts the disagreements per corpus family, which is the number
// each epic row owns. The total is the board's number and the wrong one for
// "did my row do what it said".
func (r *Result) ByPrefix() map[string]int {
	out := make(map[string]int)
	for _, d := range r.Disagreements {
		out[Prefix(d.ID)]++
	}
	return out
}

// Split divides the reference's movers by which modes refused them.
func (r *Result) Split() Split {
	var s Split
	for _, d := range r.Disagreements {
		if !d.Ref.Moves() {
			continue
		}
		zshMode, shMode, kshMode := d.Ref.Refused[0], d.Ref.Refused[1], d.Ref.Refused[2]
		switch {
		case zshMode:
			s.Reversed++
		case shMode && kshMode:
			s.Both++
		case shMode:
			s.ShOnly++
		case kshMode:
			s.KshOnly++
		default:
			s.Other++
		}
	}
	return s
}

// Report is the whole reading, controls first.
func (r *Result) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "emulate-sweep: %d snippets, modes %s/%s/%s, refusal = nonzero status under `set -n`\n\n",
		r.Total, Modes[0], Modes[1], Modes[2])

	b.WriteString("controls, fired on both binaries before anything was counted\n")
	for _, c := range Controls {
		fmt.Fprintf(&b, "  %-10s want %s  %s\n", c.Snippet, Verdict{Refused: c.Want}, c.Why)
	}

	fmt.Fprintf(&b, "\nmoving with the mode\n")
	fmt.Fprintf(&b, "  %-10s %4d of %d   %s (%s)\n", r.Reference.Name, r.RefMoves, r.Scored, r.Reference.Path, r.Reference.Version)
	fmt.Fprintf(&b, "  %-10s %4d of %d   %s (%s)\n", r.Ours.Name, r.OurMoves, r.Scored, r.Ours.Path, r.Ours.Version)

	// Printed on every run, empty or not: a snippet quietly dropped from the
	// denominator is exactly what a closed row looks like.
	fmt.Fprintf(&b, "\nunmeasured — a binary did not finish, so neither counted   %d of %d\n", r.Total-r.Scored, r.Total)
	for _, u := range r.Unmeasured {
		fmt.Fprintf(&b, "  %-56s %s answered %s\n", u.ID, u.Binary, u.Got)
	}

	fmt.Fprintf(&b, "\nthe bar: snippets the two answer differently   %d\n", len(r.Disagreements))

	if len(r.Disagreements) > 0 {
		fmt.Fprintf(&b, "\nby corpus prefix — the share each epic row owns\n")
		counts := r.ByPrefix()
		names := make([]string, 0, len(counts))
		for n := range counts {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool {
			if counts[names[i]] != counts[names[j]] {
				return counts[names[i]] > counts[names[j]]
			}
			return names[i] < names[j]
		})
		for _, n := range names {
			fmt.Fprintf(&b, "  %-14s %4d\n", n, counts[n])
		}

		s := r.Split()
		fmt.Fprintf(&b, "\nhow the reference's movers split — not a lattice, which is why the table has to be measured\n")
		fmt.Fprintf(&b, "  refused under sh and ksh                       %4d\n", s.Both)
		fmt.Fprintf(&b, "  refused under sh alone                         %4d\n", s.ShOnly)
		fmt.Fprintf(&b, "  refused under ksh alone                        %4d\n", s.KshOnly)
		fmt.Fprintf(&b, "  refused under zsh's own mode, accepted by a mode %3d\n", s.Reversed)
		if s.Other > 0 {
			fmt.Fprintf(&b, "  neither mode refused (should not happen)       %4d\n", s.Other)
		}
	}
	return b.String()
}

// Rows renders every disagreement, for attributing a family to a row.
func (r *Result) Rows() string {
	var b strings.Builder
	ids := make([]Disagreement, len(r.Disagreements))
	copy(ids, r.Disagreements)
	sort.Slice(ids, func(i, j int) bool { return ids[i].ID < ids[j].ID })
	for _, d := range ids {
		fmt.Fprintf(&b, "%-56s ref %s  ours %s\n", d.ID, d.Ref, d.Our)
	}
	return b.String()
}

// ResolveReference finds the panel's zsh on this machine.
//
// It goes through oracle.Panel rather than naming a path, so the sweep and
// the golden record are pointed at the same binary by the same rule.
func ResolveReference() (Binary, error) {
	for _, s := range oracle.Panel {
		if s.Name != "zsh" {
			continue
		}
		path, ok := oracle.Locate(s.Lookup)
		if !ok {
			return Binary{}, fmt.Errorf("no zsh among %s", strings.Join(s.Lookup, ", "))
		}
		return Binary{
			Name:    "reference",
			Path:    path,
			Version: oracle.Version(context.Background(), path),
		}, nil
	}
	return Binary{}, fmt.Errorf("no zsh in the oracle panel")
}
