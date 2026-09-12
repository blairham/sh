// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/axismutate"
	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/interp"
)

// Target is one dialect graded against the shell it claims to be.
//
// The unit is a dialect and not the struct, because an axis is answered per
// dialect: the two that were caught by hand had the same field vacuous in zsh
// *and* in bash, and one report for the pair would have hidden which.
type Target struct {
	// Dialect is the -dialect value, and Against is the panel column its
	// answers are graded against.
	Dialect string
	Against string
	// Semantics is the vector that dialect ships, which is what says what
	// each axis currently holds and therefore which values are a flip.
	Semantics interp.Semantics
}

// Targets are the four dialects that claim to be a panel member. core and
// posix are deliberately absent: neither imitates a shell, so there is no
// column to grade either against and no disagreement for a row to record.
func Targets() []Target {
	return []Target{
		{Dialect: "bash", Against: "bash", Semantics: dialectSemantics("bash")},
		{Dialect: "zsh", Against: "zsh", Semantics: dialectSemantics("zsh")},
		{Dialect: "ksh", Against: "ksh93", Semantics: dialectSemantics("ksh")},
		{Dialect: "dash", Against: "dash", Semantics: dialectSemantics("dash")},
	}
}

// dialectSemantics is the vector a dialect ships.
//
// Named rather than taken as a value so that the four are listed in one
// place, and so that a dialect gained tomorrow is a compile error here rather
// than a column quietly missing from the sweep.
func dialectSemantics(name string) interp.Semantics {
	switch name {
	case "bash":
		return bash.Semantics()
	case "zsh":
		return zsh.Semantics()
	case "ksh":
		return ksh.Semantics()
	case "dash":
		return dash.Semantics()
	}
	panic("axissweep: no dialect named " + name)
}

// Outcome is what the sweep learned about one flip.
type Outcome string

const (
	// Pinned means a corpus row that agreed with the reference stopped
	// agreeing when the axis moved. That row records the disagreement.
	Pinned Outcome = "pinned"
	// Unpinned means the whole graded corpus ran with the axis moved and
	// every row that agreed still agreed. Nothing objected.
	Unpinned Outcome = "unpinned"
)

// Flip is one field moved to one value, and what happened.
type Flip struct {
	Field   string `json:"field"`
	Type    string `json:"type"`
	Dialect string `json:"dialect"`
	From    string `json:"from"`
	To      string `json:"to"`
	// Discriminating is true when both ends of the flip are answers rather
	// than the absence of one. Moving a specified axis to Unspecified makes
	// the shell refuse wherever the axis is consulted, so it measures
	// whether the axis is *reached*; only a flip between two answers asks
	// whether the corpus can tell them apart, which is the question that
	// found the four vacuous axes.
	Discriminating bool    `json:"discriminating"`
	Outcome        Outcome `json:"outcome"`
	// By is the first row that objected, and Scanned is how many rows were
	// run before it did.
	By      string `json:"by,omitempty"`
	Scanned int    `json:"scanned"`
	Elapsed string `json:"elapsed"`
}

// Options configure a run.
type Options struct {
	// Bin is the shell built with -tags shaxissweep.
	Bin string
	// Cases is the corpus, and Golden the recorded panel answers.
	Cases  []oracle.Case
	Golden *oracle.Run
	// Targets defaults to Targets().
	Targets []Target
	// Jobs bounds how many cases run at once. The corpus is 3000 processes
	// per pass and several agents share this machine, so it is a flag with a
	// conservative default rather than GOMAXPROCS.
	Jobs int
	// Only, when set, restricts the sweep to field paths containing it.
	Only string
	// Unspecified includes the flips to and from the "no answer" constant.
	// Off by default: they measure reachability rather than disagreement,
	// they are the slower half, and almost everything is reachable.
	Unspecified bool
	Log         io.Writer
}

// Result is the whole sweep.
type Result struct {
	Flips []Flip `json:"flips"`
	// Flaky are rows whose own output moved between two unmutated runs.
	// They cannot grade anything and are listed rather than dropped.
	Flaky []string `json:"flaky,omitempty"`
	// Presets is what the four dialects hold for each axis, which answers a
	// question the flip test structurally cannot — see values.go.
	Presets []ValueUse `json:"presets,omitempty"`
	// Baseline is how many rows agreed with the reference before anything
	// was moved, per dialect. A field can only be pinned by a row that was
	// passing, so this bounds what the sweep could possibly have found.
	Baseline map[string]int `json:"baseline"`
	// Reached says, for each pair nothing objected to, whether the axis is
	// consulted by that dialect at all — "dialect field" to true or false.
	//
	// This is the discriminator the backlog needed. An unpinned pair is
	// either a missing corpus row or a disagreement that is not there, and
	// the two want opposite fixes; but there is a third thing it can be, and
	// it looks identical from here: an axis the dialect's own code never
	// reaches, where no row could object however it was written. Moving such
	// an axis to the "no answer" constant makes the shell refuse *wherever
	// it is consulted*, so a dialect that does not notice that either is one
	// the axis never reaches. Two flips tell three states apart.
	Reached map[string]bool `json:"reached,omitempty"`
}

// sweepState is what one flip learns and the next one reuses.
//
// All of it is shared across dialects on purpose. The row that pins an axis
// in bash is nearly always the row that pins it in zsh — the row exists
// because the two disagree — so trying it first is the difference between
// three more full sweeps and three cheap ones.
type sweepState struct {
	// pass is the rows that agreed with the reference, per target, in the
	// order Targets gave them.
	pass [][]oracle.Case
	// sensitive counts how many distinct answers our own four dialects gave
	// one row. One means no axis they disagree about reaches it.
	sensitive map[string]int
	// objections counts how often a row has objected to anything.
	objections map[string]int
	// pinnedBy is the row that objected to this axis last, by field path.
	pinnedBy map[string]string
	// flaky are rows whose own answer moved between two unmutated runs.
	flaky map[string]bool
}

func (o *Options) logf(format string, args ...any) {
	if o.Log == nil {
		return
	}
	_, _ = fmt.Fprintf(o.Log, format, args...)
}

// Run sweeps every axis.
func Run(ctx context.Context, o Options) (*Result, error) {
	if o.Jobs <= 0 {
		o.Jobs = 4
	}
	targets := o.Targets
	if len(targets) == 0 {
		targets = Targets()
	}
	fields, err := Fields(reflect.TypeOf(interp.Semantics{}))
	if err != nil {
		return nil, err
	}
	if o.Only != "" {
		var kept []Field
		for _, f := range fields {
			if strings.Contains(strings.ToLower(f.Path), strings.ToLower(o.Only)) {
				kept = append(kept, f)
			}
		}
		fields = kept
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("no axis matches %q", o.Only)
	}

	found, _ := oracle.Resolve(ctx)
	byName := map[string]oracle.Found{}
	for _, f := range found {
		byName[f.Name] = f
	}

	res := &Result{Baseline: map[string]int{}}
	state := &sweepState{
		flaky:      map[string]bool{},
		objections: map[string]int{},
		pinnedBy:   map[string]string{},
	}

	// Every dialect's baseline before any of them is swept, because the four
	// together carry a signal none of them carries alone: a row whose answer
	// is the same in all four dialects is a row no axis those dialects
	// disagree about reaches, and a row whose answer differs is where the
	// vector is doing something. Ranking by that puts the rows that can
	// object at the front of every scan, which is most of what the sweep
	// costs — measured, it is the difference between scanning half the
	// corpus per flip and scanning a handful of rows.
	graded := make([]oracle.Case, 0, len(o.Cases))
	for _, c := range o.Cases {
		if oracle.Graded(c) {
			graded = append(graded, c)
		}
	}
	answers := map[string][]string{}
	for _, t := range targets {
		ref, ok := byName[t.Against]
		if !ok {
			return nil, fmt.Errorf("reference shell %q is not installed, so %s cannot be graded", t.Against, t.Dialect)
		}
		start := time.Now()
		got := o.runAll(ctx, o.column(t, ref, ""), graded)
		var pass []oracle.Case
		for i, c := range graded {
			answers[c.ID] = append(answers[c.ID], got[i].Stdout+"\x00"+got[i].Stderr)
			if ok, _ := oracle.Verdict(c, o.Golden.Results[c.ID][t.Against], got[i]); ok {
				pass = append(pass, c)
			}
		}
		state.pass = append(state.pass, pass)
		res.Baseline[t.Dialect] = len(pass)
		o.logf("%s: %d of %d graded rows agree with %s (%s)\n", t.Dialect, len(pass), len(graded), t.Against, time.Since(start).Round(time.Millisecond))
	}
	state.sensitive = map[string]int{}
	for id, seen := range answers {
		distinct := map[string]bool{}
		for _, a := range seen {
			distinct[a] = true
		}
		state.sensitive[id] = len(distinct)
	}

	for i, t := range targets {
		if err := o.sweepTarget(ctx, t, byName[t.Against], fields, res, state, i); err != nil {
			return nil, err
		}
	}
	if err := o.sweepReach(ctx, targets, byName, fields, res, state); err != nil {
		return nil, err
	}
	flaky := state.flaky
	for id := range flaky {
		res.Flaky = append(res.Flaky, id)
	}
	sort.Strings(res.Flaky)
	return res, nil
}

// column is the implementation under test, dressed as a panel column.
//
// SelfName is taken from the reference for the reason RunConformance takes it
// from there: it is a fact about the shell being imitated, and normalize has
// to erase the same word on both sides.
func (o *Options) column(t Target, ref oracle.Found, spec string) oracle.Found {
	var env []string
	if spec != "" {
		env = []string{axismutate.EnvVar + "=" + spec}
	}
	return oracle.Found{
		Shell: oracle.Shell{
			Name:     "ours",
			Args:     []string{"-dialect", t.Dialect},
			SelfName: ref.SelfName,
			Env:      env,
			Why:      "the implementation under test, with one axis moved",
		},
		Path: o.Bin,
	}
}

func (o *Options) sweepTarget(ctx context.Context, t Target, ref oracle.Found, fields []Field, res *Result, state *sweepState, which int) error {
	plain := o.column(t, ref, "")
	pass := state.pass[which]
	sem := reflect.ValueOf(t.Semantics)
	started := time.Now()
	for n, f := range fields {
		if n%25 == 0 {
			o.logf("%s: axis %d of %d (%s elapsed)\n", t.Dialect, n, len(fields), time.Since(started).Round(time.Second))
		}
		cur, err := At(sem, f.Path)
		if err != nil {
			return err
		}
		values, err := Values(f, cur)
		if err != nil {
			return err
		}
		held := heldName(f, cur)
		for _, v := range values {
			discriminating := !v.Unspecified && !unspecifiedNow(f, cur)
			if !discriminating && !o.Unspecified {
				continue
			}
			spec := axismutate.Spec{Path: f.Path, Value: v.Literal}.String()
			flipStart := time.Now()
			by, scanned := o.firstObjection(ctx, t, ref, spec, order(pass, f, state))
			out := Flip{
				Field: f.Path, Type: f.Type, Dialect: t.Dialect,
				From: held, To: v.Name, Discriminating: discriminating,
				Outcome: Unpinned, Scanned: scanned,
				Elapsed: time.Since(flipStart).Round(time.Millisecond).String(),
			}
			if by != "" {
				// A row whose own answer moves between two unmutated runs
				// would object to everything. Confirm before believing it.
				if o.stable(ctx, plain, by, pass) {
					out.Outcome, out.By = Pinned, by
					state.objections[by]++
					state.pinnedBy[f.Path] = by
				} else {
					state.flaky[by] = true
					o.logf("  %s: row %s is not stable unmutated; not counted\n", f.Path, by)
					by, scanned = o.firstObjection(ctx, t, ref, spec, order(pass, f, state))
					out.Scanned = scanned
					if by != "" {
						out.Outcome, out.By = Pinned, by
						state.objections[by]++
						state.pinnedBy[f.Path] = by
					}
				}
			}
			res.Flips = append(res.Flips, out)
			if out.Outcome == Unpinned {
				o.logf("  UNPINNED %s %s: %s -> %s (%d rows, %s)\n", t.Dialect, f.Path, held, v.Name, out.Scanned, out.Elapsed)
			}
		}
	}
	return nil
}

// sweepReach asks, of every pair nothing objected to, whether the axis is
// consulted by that dialect at all.
//
// It runs only over the backlog, so it costs what the backlog costs and
// shrinks with it — and it is the difference between a list of 111 things to
// look at and a list of the ones somebody could actually do something about.
// A field with no "no answer" constant cannot be asked: a string axis spells
// absence as the empty string, which is an answer, so those are left unknown
// rather than guessed at.
func (o *Options) sweepReach(ctx context.Context, targets []Target, byName map[string]oracle.Found, fields []Field, res *Result, state *sweepState) error {
	unpinned := res.Unpinned()
	if len(unpinned) == 0 {
		return nil
	}
	res.Reached = map[string]bool{}
	byPath := map[string]Field{}
	for _, f := range fields {
		byPath[f.Path] = f
	}
	consts, err := typeConstants()
	if err != nil {
		return err
	}
	for i, t := range targets {
		ref, ok := byName[t.Against]
		if !ok {
			continue
		}
		sem := reflect.ValueOf(t.Semantics)
		for _, flip := range unpinned {
			if flip.Dialect != t.Dialect {
				continue
			}
			f := byPath[flip.Field]
			var none *Value
			for _, v := range consts[f.Type] {
				if v.Unspecified {
					none = &v
					break
				}
			}
			if none == nil {
				continue
			}
			cur, err := At(sem, f.Path)
			if err != nil {
				return err
			}
			if literalOf(cur) == none.Literal {
				continue
			}
			spec := axismutate.Spec{Path: f.Path, Value: none.Literal}.String()
			by, _ := o.firstObjection(ctx, t, ref, spec, order(state.pass[i], f, state))
			res.Reached[t.Dialect+" "+f.Path] = by != ""
			if by == "" {
				o.logf("  UNREACHED %s %s: nothing objects to the axis having no answer at all\n", t.Dialect, f.Path)
			}
		}
	}
	return nil
}

// firstObjection runs rows until one that agreed with the reference stops
// agreeing, and answers which row and how many it took.
func (o *Options) firstObjection(ctx context.Context, t Target, ref oracle.Found, spec string, cases []oracle.Case) (string, int) {
	mutated := o.column(t, ref, spec)
	scanned := 0
	for start := 0; start < len(cases); start += o.Jobs {
		end := min(start+o.Jobs, len(cases))
		batch := cases[start:end]
		got := o.runAll(ctx, mutated, batch)
		for i, c := range batch {
			scanned++
			want := o.Golden.Results[c.ID][t.Against]
			if ok, _ := oracle.Verdict(c, want, got[i]); !ok {
				return c.ID, scanned
			}
		}
	}
	return "", scanned
}

// stable runs one row twice unmutated and reports whether it answered the
// same way both times.
//
// A row whose own answer moves would object to every flip, and would do it
// first, since the order puts what objected before at the front — so one
// flaky row could pin the whole struct. Asking is two processes and it is
// asked only when something objects.
func (o *Options) stable(ctx context.Context, plain oracle.Found, id string, cases []oracle.Case) bool {
	for _, c := range cases {
		if c.ID != id {
			continue
		}
		return sameResult(oracle.Exec(ctx, plain, c), oracle.Exec(ctx, plain, c))
	}
	return false
}

func sameResult(a, b oracle.Result) bool {
	return a.Stdout == b.Stdout && a.Stderr == b.Stderr && a.Status == b.Status &&
		a.Signal == b.Signal && a.TimedOut == b.TimedOut
}

func (o *Options) runAll(ctx context.Context, sh oracle.Found, cases []oracle.Case) []oracle.Result {
	out := make([]oracle.Result, len(cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, o.Jobs)
	for i, c := range cases {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = oracle.Exec(ctx, sh, c)
		}()
	}
	wg.Wait()
	return out
}

func heldName(f Field, cur reflect.Value) string {
	held := literalOf(cur)
	consts, err := typeConstants()
	if err == nil {
		for _, v := range consts[f.Type] {
			if v.Literal == held {
				return v.Name
			}
		}
	}
	if f.Kind == reflect.String {
		return `"` + held + `"`
	}
	return held
}

func unspecifiedNow(f Field, cur reflect.Value) bool {
	held := literalOf(cur)
	consts, err := typeConstants()
	if err != nil {
		return false
	}
	for _, v := range consts[f.Type] {
		if v.Literal == held {
			return v.Unspecified
		}
	}
	return false
}

// order puts the rows most likely to object first.
//
// Four signals, in descending strength: the row that pinned this same axis in
// another dialect, rows that have objected to anything before, how many
// distinct answers our own four dialects gave the row, and whether its name
// or its snippet shares words with the axis.
//
// It is a speed heuristic and nothing else. Every row is still scanned before
// a flip is called unpinned, which is the only claim the instrument makes;
// the order decides how long a *pinned* flip takes to prove itself, which is
// where nearly all of the time goes.
func order(pass []oracle.Case, f Field, state *sweepState) []oracle.Case {
	words := splitCamel(f.Path)
	known := state.pinnedBy[f.Path]
	score := func(c oracle.Case) int {
		s := 0
		if c.ID == known {
			s += 1_000_000
		}
		s += state.objections[c.ID] * 1000
		s += state.sensitive[c.ID] * 100
		id := strings.ToLower(c.ID + " " + c.Snippet)
		for _, w := range words {
			if len(w) > 3 && strings.Contains(id, w) {
				s += 10
			}
		}
		return s
	}
	out := make([]oracle.Case, 0, len(pass))
	for _, c := range pass {
		// A row already caught answering differently on two unmutated runs
		// is dropped for the rest of the sweep rather than only for the flip
		// that caught it: it would object to everything, and the order puts
		// what objected before at the front, so one such row left in would
		// pin the whole struct.
		if !state.flaky[c.ID] {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	return out
}

func splitCamel(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if i > start && unicode.IsUpper(r) {
			out = append(out, strings.ToLower(s[start:i]))
			start = i
		}
	}
	out = append(out, strings.ToLower(s[start:]))
	return out
}

// Unpinned is the deliverable: the axis/dialect pairs no row objected to.
//
// A pair counts as pinned if *any* discriminating flip of it was objected to,
// so an axis with three answers where the corpus tells two of them apart is
// not on the list. Flips to and from the unspecified constant never count —
// they measure whether the axis is reached, which is a different question and
// an easier one to pass.
func (r *Result) Unpinned() []Flip {
	type key struct{ field, dialect string }
	pinned := map[key]bool{}
	for _, f := range r.Flips {
		if f.Discriminating && f.Outcome == Pinned {
			pinned[key{f.Field, f.Dialect}] = true
		}
	}
	seen := map[key]bool{}
	var out []Flip
	for _, f := range r.Flips {
		k := key{f.Field, f.Dialect}
		if !f.Discriminating || f.Outcome != Unpinned || pinned[k] || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
	}
	return out
}

// Report renders the sweep the way the issue asks for it: the fields nothing
// objected to, which is the backlog.
func (r *Result) Report() string {
	var b strings.Builder
	fields := map[string]bool{}
	for _, f := range r.Flips {
		if f.Discriminating {
			fields[f.Field] = true
		}
	}
	fmt.Fprintf(&b, "axes swept: %d\n", len(fields))
	fmt.Fprintf(&b, "flips run:  %d\n", len(r.Flips))
	for _, d := range sortedKeys(r.Baseline) {
		fmt.Fprintf(&b, "baseline %s: %d rows agree\n", d, r.Baseline[d])
	}
	unpinned := r.Unpinned()
	notes, _ := FieldNotes()
	b.WriteString("\nnothing objected — the backlog:\n")
	b.WriteString("  before adding a row to any of these, re-measure: an unpinned axis\n" +
		"  is either a missing corpus row or a disagreement that is not there,\n" +
		"  and an axis nobody exercises is exactly where a mistaken measurement\n" +
		"  survives. A pair marked `never reached` is a third thing: the\n" +
		"  dialect's own code does not consult the axis, so no row could object\n" +
		"  however it was written.\n")
	var untriaged int
	for _, f := range unpinned {
		mark := ""
		if reached, ok := r.Reached[f.Dialect+" "+f.Field]; ok && !reached {
			mark = "   (never reached)"
		}
		fmt.Fprintf(&b, "  %-8s %-52s %s -> %s%s\n", f.Dialect, f.Field, f.From, f.To, mark)
		why := verdict(notes[f.Field], f.Dialect)
		if why == "" {
			untriaged++
			continue
		}
		b.WriteString(wrapNote(why))
	}
	if len(unpinned) == 0 {
		b.WriteString("  (none — every axis has a row that fails when it moves)\n")
	}
	fmt.Fprintf(&b, "\n%d axis/dialect pairs nothing objected to, %d of them with no\nrecorded reason. The second number is the one to drive down: a pair\nthat stays needs a standing verdict on the axis — `unpinned %s: why` in\nthe field's doc comment — and a pair a row now catches leaves on its own.\n",
		len(unpinned), untriaged, "<dialect>")
	if len(r.Flaky) > 0 {
		fmt.Fprintf(&b, "\nrows whose own answer moved between two unmutated runs (%d):\n", len(r.Flaky))
		for _, id := range r.Flaky {
			fmt.Fprintf(&b, "  %s\n", id)
		}
	}
	return b.String()
}

// verdict is the standing reason this pair is still on the list, from the
// field's own comment: the dialect's own line if it has one, otherwise the
// one written for every dialect.
func verdict(n Notes, dialect string) string {
	if why, ok := n.Unpinned[dialect]; ok {
		return why
	}
	return n.Unpinned[""]
}

// Untriaged counts the backlog entries no field comment has answered, which
// is what the exit status is for.
func (r *Result) Untriaged() int {
	notes, _ := FieldNotes()
	n := 0
	for _, f := range r.Unpinned() {
		if verdict(notes[f.Field], f.Dialect) == "" {
			n++
		}
	}
	return n
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
