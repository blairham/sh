// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// named runs with the pipeline-status record exposed under a name, which is
// all a dialect supplies.
func named(name string, tune func(*Semantics)) func(*Runner) {
	return func(r *Runner) {
		r.SetPipelineStatus(name)
		if tune != nil {
			s := *r.Semantics
			tune(&s)
			r.Semantics = &s
		}
	}
}

// What the record holds. `$?` reports the last element only, which is the
// whole reason the others are worth keeping.
func TestThePipelineStatusRecordsEveryElement(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"three elements", `false | true | false; echo "${P[@]}"`, "1 0 1"},
		{"a command on its own records one", `false; echo "${P[@]}"`, "1"},
		// The `if` is the command that just ran; the pipeline inside it is
		// gone by the time the clause finishes.
		{"a compound command records its own", `if false | true; then :; fi; echo "${P[@]}"`, "0"},
		// `!` inverts what the pipeline reports, not what its elements did.
		{"negation does not reach it", `! false | true; echo "${P[@]}"`, "1 0"},
		{"count", `true | true | true; echo "${#P[@]}"`, "3"},
		{"one element by index", `false | true; echo "${P[0]}${P[1]}"`, "10"},
		{"a later pipeline replaces it", `false | true; true | false; echo "${P[@]}"`, "0 1"},
		{"inside a function", `f() { false | true; echo "${P[@]}"; }; f`, "1 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The compound row needs the third axis answered: what a
			// compound does to the record is
			// CompoundPipelineStatusRecord's question, and both of its
			// answers have a test of their own below. The body-reading one
			// is the reading these rows are about, and the `then` holds a
			// command that counts, so the clause writes its own status.
			out, _ := run(t, tc.src, named("P", func(s *Semantics) {
				s.CompoundPipelineStatusRecord = CompoundPipelineStatusFromTheBody
			}))
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// Without a dialect to name it, there is no record at all — nothing could read
// one, so nothing is kept.
func TestNoNameMeansNoRecord(t *testing.T) {
	out, _ := run(t, `false | true; echo "[${PIPESTATUS[@]}]"`, nil)
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("got %q, want an ordinary absent variable", out)
	}
}

// Whether an assignment with no command name counts as a command.
func TestAssignmentUpdatesPipelineStatusIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		a    Answer
		want string
	}{
		{Yes, "0"},
		{No, "1 0"},
	} {
		out, _ := run(t, `false | true; x=1; echo "${P[@]}"`, named("P", func(s *Semantics) {
			s.AssignmentUpdatesPipelineStatus = tc.a
		}))
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%v: got %q, want %q", tc.a, strings.TrimSpace(out), tc.want)
		}
	}
	// Every other shape of command updates it whatever the answer, so nothing
	// is asked for those.
	// `x=1 true` is the shape that makes the Args check load-bearing: an
	// assignment in front of a command name is that command's, and the
	// command records whatever the answer.
	for _, src := range []string{`false | true; :; echo "${P[@]}"`, `false | true; x=1 true; echo "${P[@]}"`} {
		for _, a := range []Answer{Yes, No, Unspecified} {
			out, _ := run(t, src, named("P", func(s *Semantics) {
				s.AssignmentUpdatesPipelineStatus = a
			}))
			if strings.TrimSpace(out) != "0" {
				t.Errorf("%v: %s gave %q, want 0 with no dialect needed", a, src, strings.TrimSpace(out))
			}
		}
	}
}

// enableTestAndArith turns on the two constructs whose effect on the record is
// the axis below: `[[ … ]]` and `(( … ))`, neither of which the core grammar
// has.
func enableTestAndArith(d *syntax.Dialect) {
	d.DoubleBracket = true
	d.ArithCommand = true
}

// Whether `[[ … ]]` and `(( … ))` count as commands for the record.
//
// Every case here puts the construct *between* the pipeline and the read,
// because that is the only place the answer is visible: a script that reads
// the record straight after a pipeline gets the same two elements whatever the
// answer, so a test written that way passes against both.
//
// The wants are elements rather than a count. `[[ a = b ]]` with Yes leaves
// one element holding 1, and asserting only the length would also accept a
// record holding 0 — the value the bug this fixes actually produced by its
// third write.
func TestTestAndArithmeticUpdatePipelineStatusIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		yes  string
		no   string
	}{
		// `(( 1 ))` succeeds and `(( 0 ))` fails, so the Yes column below is
		// the inverse of the expression's truth rather than a copy of it.
		{"a true test", `false | true; [[ a = a ]]; echo "${P[@]}"`, "0", "1 0"},
		{"a false test", `false | true; [[ a = b ]]; echo "${P[@]}"`, "1", "1 0"},
		{"true arithmetic", `false | true; (( 1 )); echo "${P[@]}"`, "0", "1 0"},
		{"false arithmetic", `false | true; (( 0 )); echo "${P[@]}"`, "1", "1 0"},
		// `$?` is a separate record and tracks the construct either way,
		// which is what makes the two distinguishable at all.
		{"the status still moves", `false | true; (( 0 )); echo "$? ${P[@]}"`, "1 1", "1 1 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, w := range []struct {
				a    Answer
				want string
			}{{Yes, tc.yes}, {No, tc.no}} {
				out, _ := runGrammar(t, tc.src, enableTestAndArith, named("P", func(s *Semantics) {
					s.TestAndArithmeticUpdatePipelineStatus = w.a
				}))
				if got := strings.TrimSpace(out); got != w.want {
					t.Errorf("%v: got %q, want %q", w.a, got, w.want)
				}
			}
		})
	}
}

// The shape real code uses, and the one the bug was found in: test an element,
// then branch on it. Three reads of the record in one if/elif chain.
//
// With Yes the first `(( … ))` writes its own status over the array it just
// read, so the `elif` reads that instead of the pipeline's — and the chain
// reports a status the pipeline never had. It is the mechanism behind a failed
// clone being reported as `code: 0`.
func TestReadingTheRecordTwiceInOneChain(t *testing.T) {
	const src = `false | true; if (( P[0] == 141 )); then echo signal; ` +
		`elif (( P[0] )); then echo "code=${P[0]}"; else echo clean; fi`
	for _, tc := range []struct {
		a    Answer
		want string
	}{
		// The first test read 1, was false, and left it alone: the second
		// read still sees 1 and the message reports it.
		{No, "code=1"},
		// With Yes the chain reports a status the pipeline never had, and
		// gets there by two separate writes. The first test is false, so it
		// writes *its own* 1 over the pipeline's 1 — the `elif` is then true
		// for a reason that has nothing to do with the pipeline. Its own
		// `(( … ))` writes 0 before the message is expanded, so the number
		// printed is 0. This is `Clone failed (code: 0)` in full.
		{Yes, "code=0"},
	} {
		out, _ := runGrammar(t, src, enableTestAndArith, named("P", func(s *Semantics) {
			s.TestAndArithmeticUpdatePipelineStatus = tc.a
			// The `if` around the chain is a compound, so the third axis is
			// reached; it is not this test's subject and neither answer moves
			// either row, since the record is read inside the clause rather
			// than after it.
			s.CompoundPipelineStatusRecord = CompoundPipelineStatusFromTheBody
		}))
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.a, got, tc.want)
		}
	}
}

// A redirection or a leading `!` makes a pipeline of the construct, and a
// pipeline writes the record whatever the axis says. Shared by all three of
// the constructs the axes cover, which is why they share one predicate: a
// second helper written for `[[ … ]]` alone would not have carried this.
func TestNegationAndRedirectionAlwaysWriteTheRecord(t *testing.T) {
	for _, src := range []string{
		`false | true; ! [[ a = a ]]; echo "${P[@]}"`,
		`false | true; ! (( 1 )); echo "${P[@]}"`,
		`false | true; ! x=1; echo "${P[@]}"`,
		`false | true; [[ a = a ]] >/dev/null; echo "${P[@]}"`,
		`false | true; (( 1 )) >/dev/null; echo "${P[@]}"`,
		`false | true; x=1 >/dev/null; echo "${P[@]}"`,
	} {
		for _, a := range []Answer{Yes, No} {
			out, _ := runGrammar(t, src, enableTestAndArith, named("P", func(s *Semantics) {
				s.TestAndArithmeticUpdatePipelineStatus = a
				s.AssignmentUpdatesPipelineStatus = a
				// *Whether* a negated construct writes the record is this
				// test's subject; *which* status it writes is the axis
				// NegatedTestRecordsThePostNegationStatus answers, and both
				// of its answers are covered in
				// TestANegatedTestRecordsOneStatusOrTheOther. Answered here
				// so this asks its own question rather than that one.
				s.NegatedTestRecordsThePostNegationStatus = No
			}))
			// The status recorded under `!` is the one from before the
			// inversion, so every line here leaves a single 0.
			if got := strings.TrimSpace(out); got != "0" {
				t.Errorf("%v: %s gave %q, want 0 whatever the answer", a, src, got)
			}
		}
	}
}

// One snippet under both mechanisms, and neither of the two axes about a
// command's *shape* is asked about the compound itself.
//
// The `(( … ))` in the body is what makes this discriminating. Write `:`
// there instead and the body's own write leaves the same single 0 the clause
// would, so the snippet cannot tell a clause that writes from one that is
// transparent — which is exactly what the corpus row of that shape could not
// tell, and why its reason now says so.
//
// The body-reading mechanism answers `0` whatever the arithmetic says,
// because the **condition** is body too and a pipeline of two counts there
// whatever it holds. The other one asks nothing of the parse: with the
// arithmetic answered Yes the `(( … ))` ran and wrote the clause's 0, and
// with it answered No nothing inside wrote at all, so the condition's own two
// elements are what stands.
func TestACompoundCommandWritesTheRecord(t *testing.T) {
	const src = `if false | true; then (( 1 )); fi; echo "${P[@]}"`
	for _, tc := range []struct {
		policy CompoundPipelineStatusPolicy
		a      Answer
		want   string
	}{
		{CompoundPipelineStatusFromTheBody, Yes, "0"},
		{CompoundPipelineStatusFromTheBody, No, "0"},
		{CompoundPipelineStatusFromWhatRan, Yes, "0"},
		{CompoundPipelineStatusFromWhatRan, No, "1 0"},
	} {
		out, _ := runGrammar(t, src, enableTestAndArith, named("P", func(s *Semantics) {
			s.TestAndArithmeticUpdatePipelineStatus = tc.a
			s.CompoundPipelineStatusRecord = tc.policy
		}))
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%v/%v: got %q, want %q", tc.policy, tc.a, got, tc.want)
		}
	}
}

// Whether `unset` is permanent. This is the opposite of what a produced
// *scalar* does, where unset ends it everywhere, which is why it is asked
// rather than inherited from the rule Dynamic follows.
func TestUnsetEndsTheProducedPipelineStatusIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		a    Answer
		want string
	}{
		{Yes, "[]"},
		{No, "[1 0]"},
	} {
		out, _ := run(t, `unset P; false | true; echo "[${P[@]}]"`, named("P", func(s *Semantics) {
			s.UnsetEndsTheProducedPipelineStatus = tc.a
		}))
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%v: got %q, want %q", tc.a, strings.TrimSpace(out), tc.want)
		}
	}
}

// What a plain `$a` gives when `a` is an array. A rule about arrays rather
// than about this record, measured here because this is the array every shell
// that has one builds without being asked.
func TestArrayScalarIsTheWholeArrayIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    Answer
		src  string
		want string
	}{
		{"joined", Yes, `a=(x y z); echo "[$a]"`, "[x y z]"},
		{"first element", No, `a=(x y z); echo "[$a]"`, "[x]"},
		{"joined, on the record", Yes, `false | true; echo "[$P]"`, "[1 0]"},
		{"first element, on the record", No, `false | true; echo "[$P]"`, "[1]"},
		// With none or one element the two agree, so no dialect is needed —
		// and building an array is not a question about how it flattens.
		{"one element needs no dialect", Unspecified, `a=(x); echo "[$a]"`, "[x]"},
		{"building a long one needs no dialect", Unspecified, `a=(x y z); echo "[${a[@]}]"`, "[x y z]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, named("P", func(s *Semantics) {
				s.ArrayScalarIsTheWholeArray = tc.a
			}))
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// A shell with no name for the record must not be asked how to maintain one.
// Without the guard, the bare core refused every `x=1` over a difference
// nothing in that shell could observe.
func TestNoNameAsksNoAxis(t *testing.T) {
	d := syntax.Core()
	enableTestAndArith(&d)
	f, err := syntax.Parse(`false | true; x=1; [[ a = a ]]; (( 1 )); echo done`, d)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// The strict core: every axis unanswered, so anything asked is reported.
	s := PosixSemantics()
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Semantics: &s})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "done\n" {
		t.Errorf("got %q, want no dialect to be needed", got)
	}
}

// NegatedTestRecordsThePostNegationStatus, both answers — #1513.
//
// The record a negated `[[ … ]]` leaves is the construct's own status under
// one answer and the negation's result under the other, and the pair of
// subjects is what makes the row discriminating: a probe using only a
// *matching* test records 0 under one answer and 1 under the other, and a
// probe using only a failing one records the two the other way round, so
// either alone passes for a shell that always writes the same number.
//
// `! false` is the third row and the reason the axis names these two
// constructs rather than negation: an ordinary command records what it
// reported whatever the answer.
func TestANegatedTestRecordsOneStatusOrTheOther(t *testing.T) {
	for _, c := range []struct{ src, post, pre string }{
		{`false | true; ! [[ a = a ]]; echo "${P[@]}"`, "1", "0"},
		{`false | true; ! [[ a = b ]]; echo "${P[@]}"`, "0", "1"},
		{`false | true; ! (( 1 )); echo "${P[@]}"`, "1", "0"},
		{`false | true; ! (( 0 )); echo "${P[@]}"`, "0", "1"},
		// A redirection makes no difference to *this* question, which is
		// measured and is why the axis is asked before the job question:
		// `! [[ a = a ]] >/dev/null` records the same either way.
		{`false | true; ! [[ a = a ]] >/dev/null; echo "${P[@]}"`, "1", "0"},
		// An ordinary command, an assignment and a compound record what they
		// reported under both answers.
		{`false | true; ! false; echo "${P[@]}"`, "1", "1"},
		{`false | true; ! true; echo "${P[@]}"`, "0", "0"},
		{`false | true; ! x=1; echo "${P[@]}"`, "0", "0"},
		{`false | true; ! { [[ a = a ]]; }; echo "${P[@]}"`, "0", "0"},
		{`false | true; ! ( [[ a = a ]] ); echo "${P[@]}"`, "0", "0"},
		// And with no `!` at all the axis is not reached.
		{`false | true; [[ a = a ]]; echo "${P[@]}"`, "0", "0"},
	} {
		for _, tc := range []struct {
			a    Answer
			want string
		}{{Yes, c.post}, {No, c.pre}} {
			out, _ := runGrammar(t, c.src, enableTestAndArith, named("P", func(s *Semantics) {
				s.TestAndArithmeticUpdatePipelineStatus = Yes
				s.AssignmentUpdatesPipelineStatus = Yes
				s.NegatedTestRecordsThePostNegationStatus = tc.a
			}))
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%v: %s gave %q, want %q", tc.a, c.src, got, tc.want)
			}
		}
	}
}

// `$?` is not the record, and the axis must not move it: the negation's result
// is the status in every shell either way. A mutant that inverted the status
// instead of the copy written down survives every row above.
func TestTheAxisMovesTheRecordAndNotTheStatus(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		out, _ := runGrammar(t, `false | true; ! [[ a = a ]]; echo "st=$? ps=${P[@]}"`,
			enableTestAndArith, named("P", func(s *Semantics) {
				s.TestAndArithmeticUpdatePipelineStatus = Yes
				s.NegatedTestRecordsThePostNegationStatus = a
			}))
		want := "st=1 ps=0"
		if a == Yes {
			want = "st=1 ps=1"
		}
		if got := strings.TrimSpace(out); got != want {
			t.Errorf("%v: got %q, want %q", a, got, want)
		}
	}
}

// CompoundPipelineStatusFromTheBody, the mechanism that reads the parse —
// #1931.
//
// The rule is about the **parse** rather than about what ran, and the first
// two rows are what say so: neither body runs, the condition being false both
// times, and the only difference between them is the text inside `then`. An
// unexecuted `:` is enough.
//
// Every `want` is the answer measured from zsh 5.9.2 on 2026-09-11. The other
// mechanism is TestACompoundLeavesTheRecordToWhatRan below, whose rows are
// chosen for where the two differ.
func TestACompoundFollowsItsBody(t *testing.T) {
	for _, c := range []struct{ src, follows string }{
		// The pair the issue is written around.
		{`if [[ a = b ]]; then :; fi`, "0"},
		{`if [[ a = b ]]; then [[ b = b ]]; fi`, "1 0"},
		// A condition is body too, and an `elif` and an `else` are as well.
		{`if :; then [[ b = b ]]; fi`, "0"},
		{`if [[ a = b ]]; then [[ b = b ]]; else :; fi`, "0"},
		{`if [[ a = b ]]; then [[ b = b ]]; elif :; then [[ c = c ]]; fi`, "0"},
		// A brace group, where the record is left alone for a body holding
		// only the three constructs that make no job.
		{`{ :; }`, "0"},
		{`{ [[ a = a ]]; }`, "1 0"},
		{`{ x=1; }`, "1 0"},
		{`{ (( 1 )); }`, "1 0"},
		{`{ [[ a = a ]]; :; }`, "0"},
		// An empty body counts as nothing rather than as something: the
		// empty `case` arm below is that row, `{ }` needing a grammar flag
		// this test does not turn on.
		// Nesting, in both directions.
		{`{ { :; } }`, "0"},
		{`{ { [[ a = a ]]; } }`, "1 0"},
		{`{ if false; then :; fi; }`, "0"},
		{`{ if false; then [[ a = a ]]; fi; }`, "0"},
		{`{ for i in 1; do [[ a = a ]]; done; }`, "1 0"},
		// Every loop, and the `while` pair is the one that says a condition
		// counts: the same body answers differently for the command in front
		// of `do`.
		{`while false; do [[ a = a ]]; done`, "0"},
		{`while [[ a = b ]]; do [[ a = a ]]; done`, "1 0"},
		{`until [[ a = a ]]; do [[ a = a ]]; done`, "1 0"},
		{`for i in 1; do :; done`, "0"},
		{`for i in 1; do [[ a = a ]]; done`, "1 0"},
		{`for i in ; do :; done`, "0"},
		{`for ((i=0;i<1;i++)); do [[ a = a ]]; done`, "1 0"},
		// A `case`, including the arm that holds nothing at all.
		{`case a in b) :;; esac`, "0"},
		{`case a in b) [[ a = a ]];; esac`, "1 0"},
		{`case a in b) ;; esac`, "1 0"},
		// A subshell answers for itself whatever it holds: it is a job.
		{`( [[ a = a ]] )`, "0"},
		{`( : )`, "0"},
		// And so do the other three shapes that make a job, reached from
		// inside a body that would otherwise not count.
		{`{ ( [[ a = a ]] ); }`, "0"},
		{`{ [[ a = a ]] & }`, "0"},
		{`{ [[ a = a ]] | [[ b = b ]]; }`, "0"},
		// A failing test under a `!` rather than a matching one, so the
		// group's own status is 0 under both answers and the row is about
		// *whether* it wrote rather than what it wrote.
		{`{ ! [[ a = b ]]; }`, "0"},
		// A redirection makes the job wherever it is written, which is the
		// rule the other two axes follow.
		{`{ [[ a = a ]]; } >/dev/null`, "0"},
		{`{ [[ a = a ]] >/dev/null; }`, "0"},
		{`{ { [[ a = a ]]; } >/dev/null; }`, "0"},
		// An and-or list is read through, and a definition is not read into.
		{`{ [[ a = a ]] && [[ b = b ]]; }`, "1 0"},
		{`{ [[ a = a ]] && :; }`, "0"},
		{`{ f() { :; }; }`, "1 0"},
		{`{ f() { :; }; [[ a = a ]]; }`, "1 0"},
	} {
		out, _ := runGrammar(t, "false | true; "+c.src+`; echo "${P[@]}"`, enableTestAndArith,
			named("P", func(s *Semantics) {
				s.TestAndArithmeticUpdatePipelineStatus = No
				s.AssignmentUpdatesPipelineStatus = No
				s.NegatedTestRecordsThePostNegationStatus = No
				s.CompoundPipelineStatusRecord = CompoundPipelineStatusFromTheBody
			}))
		if got := strings.TrimSpace(out); got != c.follows {
			t.Errorf("`false | true; %s` gave %q, want %q", c.src, got, c.follows)
		}
	}
}

// A function *definition* leaves the record alone, and no dialect is asked:
// both shells that keep a record answer the same way. Measured 2026-09-11,
// `false | true; f() { :; }` is `1 0` in bash 5.3.15 and zsh 5.9.2 alike.
func TestAFunctionDefinitionLeavesTheRecordAlone(t *testing.T) {
	for _, policy := range []CompoundPipelineStatusPolicy{
		CompoundPipelineStatusFromTheBody,
		CompoundPipelineStatusFromWhatRan,
		CompoundPipelineStatusUnspecified,
	} {
		out, _ := run(t, `false | true; f() { :; }; echo "${P[@]}"`,
			named("P", func(s *Semantics) {
				s.CompoundPipelineStatusRecord = policy
			}))
		if got := strings.TrimSpace(out); got != "1 0" {
			t.Errorf("%v: got %q, want the pipeline's elements with no dialect needed", policy, got)
		}
	}
}

// CompoundPipelineStatusFromWhatRan, the other mechanism — #2016.
//
// Nothing is written *for* the compound, so what stands after one is whatever
// the last pipeline that actually ran inside it wrote, and a compound that ran
// nothing leaves the record from before it. Every row is measured on bash
// 5.3.15 and 3.2.57 alike, each snippet after `false | true` so a replaced
// record shows as one element, and the two axes about a command's shape are
// answered as that shell answers them so the rows are comparable with it.
//
// The rows are chosen for where the two mechanisms differ: under the
// body-reading one every `want` here would be the compound's own status
// instead — a single 0 for all but the last two.
func TestACompoundLeavesTheRecordToWhatRan(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The condition ran and wrote the record; the clause wrote nothing
		// over it. This is the row the issue is written around.
		{`if false; then :; fi`, "1"},
		{`if [[ a = b ]]; then :; fi`, "1"},
		{`while false; do :; done`, "1"},
		// Nothing inside ran, so the record from *before* the compound is
		// what stands — which is the half a mechanism reading the parse
		// cannot produce at all.
		{`case a in b) :;; esac`, "1 0"},
		{`case a in b) ;; esac`, "1 0"},
		{`for i in ; do :; done`, "1 0"},
		// A backgrounded statement writes no record in the shell that
		// started it.
		{`{ [[ a = a ]] & }`, "1 0"},
		// Two elements survive the braces, which is what says it is the
		// inner *pipeline* rather than the inner last command.
		{`{ true | false; }`, "0 1"},
		{`{ [[ a = a ]] | [[ b = b ]]; }`, "0 0"},
		// And what did run writes it, however deep.
		{`{ :; }`, "0"},
		{`{ { :; } }`, "0"},
		{`{ if false; then :; fi; }`, "1"},
		{`for i in a; do false; done`, "1"},
		// A redirection on the compound does not change it, which is the
		// opposite of the rule the neighbouring axes follow — and the row
		// that needs the axis asked before the redirection is looked at.
		{`if false; then :; fi >/dev/null`, "1"},
		{`{ :; } >/dev/null`, "0"},
		// A subshell is not a compound for this purpose in either mechanism:
		// it is a job and reports its own status, one element.
		{`( : )`, "0"},
		{`( false | true | false )`, "1"},
		// A leading `!` makes a pipeline of whatever follows it, and a
		// pipeline writes the record wherever this axis stands.
		{`! { false; }`, "1"},
	} {
		out, _ := runGrammar(t, "false | true; "+c.src+`; echo "${P[@]}"`, enableTestAndArith,
			named("P", func(s *Semantics) {
				s.TestAndArithmeticUpdatePipelineStatus = Yes
				s.AssignmentUpdatesPipelineStatus = Yes
				s.NegatedTestRecordsThePostNegationStatus = Yes
				s.CompoundPipelineStatusRecord = CompoundPipelineStatusFromWhatRan
			}))
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("`false | true; %s` gave %q, want %q", c.src, got, c.want)
		}
	}
}

// And with no answer the compound writes, which is the safe direction, with
// the axis named — rather than a default standing in for a dialect.
func TestACompoundWithNoAnswerReports(t *testing.T) {
	out, _ := run(t, `false | true; { :; }; echo "${P[@]}"`,
		named("P", func(s *Semantics) {
			s.CompoundPipelineStatusRecord = CompoundPipelineStatusUnspecified
		}))
	if !strings.Contains(out, "what a compound does to the pipeline status") {
		t.Errorf("got %q, want the axis named", out)
	}
}

// A shell with no name for the record asks nothing, whatever the axis says:
// the question is unobservable there, and the core must not refuse `{ :; }`
// over a difference nothing could see.
func TestACompoundAsksNothingWithoutANameForTheRecord(t *testing.T) {
	out, st := run(t, `false | true; { :; }; echo done`, func(r *Runner) {
		sem := PosixSemantics()
		sem.CompoundPipelineStatusRecord = CompoundPipelineStatusUnspecified
		r.Semantics = &sem
	})
	if strings.TrimSpace(out) != "done" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0 with nothing asked", out, st, "done")
	}
}
