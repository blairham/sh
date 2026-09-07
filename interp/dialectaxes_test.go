// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Single-dialect divergences, each an axis asked at the disagreement.

func axisRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		// The scalar reading an array's bare name yields is its own axis,
		// answered so these tests reach the question they are about.
		sem.ArrayScalarIsTheWholeArray = No
		set(&sem)
		r.Semantics = &sem
	})
}

func TestArrayLengthWithoutASubscriptIsAnAxis(t *testing.T) {
	out, _ := axisRun(t, `a=(hello by z); echo "${#a}"`, func(s *Semantics) {
		s.ArrayLengthWithoutSubscriptIsCount = Yes
	})
	if strings.TrimSpace(out) != "3" {
		t.Errorf("got %q, want the element count", out)
	}
	out, _ = axisRun(t, `a=(hello by z); echo "${#a}"`, func(s *Semantics) {
		s.ArrayLengthWithoutSubscriptIsCount = No
	})
	if strings.TrimSpace(out) != "5" {
		t.Errorf("got %q, want the scalar's length", out)
	}
}

// And the axis is asked at **one** element as much as at several, which is a
// correction rather than a new question. The reading was skipped there on the
// grounds that a one-element array is its own element either way — true of the
// value and false of its length, which is the whole of what `${#a}` asks. An
// array of one three-character word is 1 under one answer and 3 under the
// other, and answering 5 for both was a plausible number at status 0 (#1060). The
// associative shape lands in the same place and is graded in the corpus,
// where the other axes a keyed array asks are answered by a dialect.
func TestArrayLengthAsksTheAxisAtOneElementToo(t *testing.T) {
	for _, tc := range []struct {
		src, count, width string
	}{
		{`a=(hello); echo "${#a}"`, "1", "5"},
		// The empty and the several-element cases, which agreed before and
		// still do — they are here so that a change moving them would show.
		{`a=(); echo "${#a}"`, "0", "0"},
		{`a=(hello there); echo "${#a}"`, "2", "5"},
	} {
		out, _ := axisRun(t, tc.src, func(s *Semantics) {
			s.ArrayLengthWithoutSubscriptIsCount = Yes
		})
		if strings.TrimSpace(out) != tc.count {
			t.Errorf("%s counting: got %q, want %q", tc.src, out, tc.count)
		}
		out, _ = axisRun(t, tc.src, func(s *Semantics) {
			s.ArrayLengthWithoutSubscriptIsCount = No
		})
		if strings.TrimSpace(out) != tc.width {
			t.Errorf("%s measuring: got %q, want %q", tc.src, out, tc.width)
		}
	}
}

// TestABareArrayNameBeingTheListIsAnAxis — an unquoted `$a` is the elements
// in one shell and the first element in the others (#929).
//
// Every assertion names the exact fields, never the absence of a failure. The
// bug this axis fixes returns a plausible string at status 0, so a test that
// only checked for no diagnostic passed against it.
func TestABareArrayNameBeingTheListIsAnAxis(t *testing.T) {
	const src = `a=(one two); set -- $a; echo "n=$# [$1][$2]"`
	out, _ := axisRun(t, src, func(s *Semantics) {
		s.ArrayNameWithoutSubscriptIsTheList = Yes
	})
	if strings.TrimSpace(out) != "n=2 [one][two]" {
		t.Errorf("got %q, want one field per element", out)
	}
	out, _ = axisRun(t, src, func(s *Semantics) {
		s.ArrayNameWithoutSubscriptIsTheList = No
	})
	if strings.TrimSpace(out) != "n=1 [one][]" {
		t.Errorf("got %q, want the one field the scalar reading gives", out)
	}
}

// The list reading is not a join followed by a split. With IFS empty nothing
// splits, so a join-then-split would give one field `xyz` where the elements
// were never joined at all gives three.
func TestABareArrayNameAsAListDoesNotJoinFirst(t *testing.T) {
	out, _ := axisRun(t, `IFS=; a=(x y z); set -- $a; echo "n=$# [$1][$2][$3]"`,
		func(s *Semantics) { s.ArrayNameWithoutSubscriptIsTheList = Yes })
	if strings.TrimSpace(out) != "n=3 [x][y][z]" {
		t.Errorf("got %q, want three fields with IFS empty", out)
	}
}

// Quoted, the bare name is one field holding the joined value under either
// answer — that is ArrayScalarIsTheWholeArray's question — and the join is on
// the first character of IFS, not a hard space (#854). Both answers are
// asserted so the field cannot quietly take over the quoted spelling.
func TestABareArrayNameQuotedStillJoinsOnIfs(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		out, _ := axisRun(t, `IFS=-; a=(x y z); set -- "$a"; echo "n=$# [$1]"`,
			func(s *Semantics) {
				s.ArrayScalarIsTheWholeArray = Yes
				s.ArrayNameWithoutSubscriptIsTheList = a
			})
		if strings.TrimSpace(out) != "n=1 [x-y-z]" {
			t.Errorf("%v: got %q, want one field joined on IFS", a, out)
		}
	}
}

// And an unquoted bare name in a context that does not split is the joined
// value too, measured — an assignment's value and a `case` subject among
// them. Routing those through the list path would join them on a hard space.
func TestABareArrayNameJoinsWhereNothingSplits(t *testing.T) {
	out, _ := axisRun(t, `IFS=-; a=(x y z); v=$a; case $a in "x-y-z") echo joined;; *) echo split;; esac; echo "[$v]"`,
		func(s *Semantics) {
			s.ArrayScalarIsTheWholeArray = Yes
			s.ArrayNameWithoutSubscriptIsTheList = Yes
		})
	if strings.TrimSpace(out) != "joined\n[x-y-z]" {
		t.Errorf("got %q, want the joined value in both positions", out)
	}
}

// Asked only where the readings differ. A one-element array is that element
// either way, so an unanswered axis must still run rather than refuse — and
// an empty one is no field at all under both.
func TestABareArrayNameOfOneElementAsksNothing(t *testing.T) {
	out, status := axisRun(t, `a=(only); set -- $a; echo "n=$# [$1]"`, func(*Semantics) {})
	if strings.TrimSpace(out) != "n=1 [only]" || status != 0 {
		t.Errorf("got %q at %d, want the element without a question", out, status)
	}
	out, status = axisRun(t, `a=(); set -- $a; echo "n=$#"`, func(*Semantics) {})
	if strings.TrimSpace(out) != "n=0" || status != 0 {
		t.Errorf("got %q at %d, want no field without a question", out, status)
	}
}

// The operators inherit the subject. `${a:1}` is a slice of the list under one
// answer and a substring of the joined scalar under the other, and a trim
// applies to each element rather than to one joined string.
//
// `${a:#p}` — the element filter whose silent no-op the axis was opened for —
// is not here: it is one shell's grammar, which the core has not got, so it is
// measured in the corpus against that shell instead of asserted on a vector
// that cannot parse it.
func TestOperatorsOnABareArrayNameFollowTheAxis(t *testing.T) {
	out, _ := axisRun(t, `a=(one two three); set -- ${a:1}; echo "n=$# [$1]"`,
		func(s *Semantics) { s.ArrayNameWithoutSubscriptIsTheList = Yes })
	if strings.TrimSpace(out) != "n=2 [two]" {
		t.Errorf("got %q, want a slice of the list", out)
	}
	out, _ = axisRun(t, `a=(one two three); set -- ${a:1}; echo "n=$# [$1]"`,
		func(s *Semantics) { s.ArrayNameWithoutSubscriptIsTheList = No })
	if strings.TrimSpace(out) != "n=1 [ne]" {
		t.Errorf("got %q, want a substring of the scalar", out)
	}
	out, _ = axisRun(t, `a=(one two); set -- ${a#o}; echo "n=$# [$1][$2]"`,
		func(s *Semantics) { s.ArrayNameWithoutSubscriptIsTheList = Yes })
	if strings.TrimSpace(out) != "n=2 [ne][two]" {
		t.Errorf("got %q, want the trim applied to each element", out)
	}
	// The slice at *one* element, which is where the question has to be
	// asked even though the name is that element either way: one element
	// with the first dropped is no element at all, where the same offset
	// against the characters leaves `bcdef`.
	out, _ = axisRun(t, `a=(abcdef); set -- ${a:1}; echo "n=$# [$1]"`,
		func(s *Semantics) { s.ArrayNameWithoutSubscriptIsTheList = Yes })
	if strings.TrimSpace(out) != "n=0 []" {
		t.Errorf("got %q, want the only element sliced away", out)
	}
	out, _ = axisRun(t, `a=(abcdef); set -- ${a:1}; echo "n=$# [$1]"`,
		func(s *Semantics) { s.ArrayNameWithoutSubscriptIsTheList = No })
	if strings.TrimSpace(out) != "n=1 [bcdef]" {
		t.Errorf("got %q, want the characters after the first", out)
	}
}

// The axis must not be asked for a spelling the list path never goes on to
// answer. Nothing about the *value* would change if it were — the rewrite is
// local — but an unanswered axis is a diagnostic, so a question asked too
// widely turns ordinary shell into a refusal on a core that has chosen no
// shell. Each row below is one such spelling, and each guard that keeps it out
// is what mutation kills through this test.
func TestABareArrayNameAsksNothingWhereTheListPathDoesNotAnswer(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// `${#a}` is ArrayLengthWithoutSubscriptIsCount's question.
		{`a=(one two); set -- ${#a}; echo "[$1]"`, "[2]"},
		// `${a:=d}`, `${a:?e}` and `${a=d}` were here, as operators the
		// array path did not answer with elements. It answers all three
		// since #984 — they come to the *parameter* when their test does
		// not fire, and the parameter is the whole array — so they are no
		// longer spellings the axis is asked too widely for. They ask it
		// because they use the answer, which is what
		// TestABareArrayNameUnderAConditionalAsksTheAxis pins.
		// And a name that is not an array at all.
		{`v=xy; set -- $v; echo "[$1]"`, "[xy]"},
	} {
		out, status := axisRun(t, tc.src, func(s *Semantics) {
			// Answered so that only the axis under test can refuse.
			s.ArrayScalarIsTheWholeArray = Yes
			s.ArrayLengthWithoutSubscriptIsCount = Yes
			s.SplitParamExpansion = No
			s.GlobExpansionResults = No
		})
		if strings.Contains(out, "no dialect was chosen") {
			t.Errorf("%s: refused over an axis it never uses: %q", tc.src, out)
		}
		if got := strings.TrimSpace(out); got != tc.want || status != 0 {
			t.Errorf("%s: got %q at %d, want %q", tc.src, got, status, tc.want)
		}
	}
}

// The four conditionals reach the array path since #984, so a bare name under
// one of them asks the axis and *uses* the answer — which is the opposite of
// the neighbouring test's subject and belongs beside it for that reason.
//
// Measured 2026-09-06: `a=(one two); printf "[%s]" ${a:=d}` is `[one][two]` in
// zsh 5.9.2 and `[one]` in bash 5.3, bash 3.2 and ksh93, and `${a:?e}`,
// `${a=d}` and `${a?e}` answer alike. Before #984 it was one field holding
// `one two` in every dialect — nobody's answer, at status 0.
func TestABareArrayNameUnderAConditionalAsksTheAxis(t *testing.T) {
	for _, op := range []string{":=d", ":?e", "=d", "?e"} {
		src := `a=(one two); set -- ${a` + op + `}; echo "n=$# [$1]"`
		set := func(s *Semantics) {
			s.ArrayScalarIsTheWholeArray = No
			s.SplitParamExpansion = No
			s.GlobExpansionResults = No
		}
		out, status := axisRun(t, src, func(s *Semantics) {
			set(s)
			s.ArrayNameWithoutSubscriptIsTheList = Yes
		})
		if got := strings.TrimSpace(out); got != "n=2 [one]" || status != 0 {
			t.Errorf("%s where the name is the list: got %q at %d, want %q", op, got, status, "n=2 [one]")
		}
		out, status = axisRun(t, src, func(s *Semantics) {
			set(s)
			s.ArrayNameWithoutSubscriptIsTheList = No
		})
		if got := strings.TrimSpace(out); got != "n=1 [one]" || status != 0 {
			t.Errorf("%s where the name is one element: got %q at %d, want %q", op, got, status, "n=1 [one]")
		}
	}
}

// A plain scalar is not an array and asks nothing, however the axis is
// answered: the rewrite must key on the *store*, not on the spelling.
func TestABareScalarNameIsNotAList(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		out, status := axisRun(t, `v="x y"; set -- $v; echo "n=$# [$1]"`,
			func(s *Semantics) {
				s.SplitParamExpansion = Yes
				s.ArrayNameWithoutSubscriptIsTheList = a
			})
		if strings.TrimSpace(out) != "n=2 [x]" || status != 0 {
			t.Errorf("%v: got %q at %d, want the scalar split as always", a, out, status)
		}
	}
}

// A quoted `"${a[@]}"` on a name that holds nothing is the axis; the same
// spelling on an array that exists and has no elements is not.
//
// Both halves are asserted here because the axis was written without the
// second: one answer to "how many fields does an empty list make" was given
// to the unset name and to the declared empty array alike, and the dialect
// that answered yes handed a spurious empty argument to every
// `f "${arr[@]}"` written before anything filled `arr`.
func TestAQuotedAtOnAnUnsetNameIsAnAxis(t *testing.T) {
	out, _ := axisRun(t, `set -- "${a[@]}"; echo "n=$#"`, func(s *Semantics) {
		s.UnsetNameAtIsOneEmptyField = Yes
	})
	if !strings.Contains(out, "n=1") {
		t.Errorf("got %q, want one empty field", out)
	}
	out, _ = axisRun(t, `set -- "${a[@]}"; echo "n=$#"`, func(s *Semantics) {
		s.UnsetNameAtIsOneEmptyField = No
	})
	if !strings.Contains(out, "n=0") {
		t.Errorf("got %q, want none", out)
	}
	// An array that exists and has no elements asks nothing, under either
	// answer, and is no field in every column measured.
	for _, a := range []Answer{Yes, No} {
		out, _ = axisRun(t, `a=(); set -- "${a[@]}"; echo "n=$#"`, func(s *Semantics) {
			s.UnsetNameAtIsOneEmptyField = a
		})
		if !strings.Contains(out, "n=0") {
			t.Errorf("%v: declared empty array gave %q, want n=0 without a question", a, out)
		}
		// And an association declared with no keys is the same shape,
		// reached down a different branch: it never sees subscriptTarget, so
		// a guard that only asked that would take it for absent.
		out, _ = axisRun(t, `typeset -A m; set -- "${m[@]}"; echo "n=$#"`, func(s *Semantics) {
			s.UnsetNameAtIsOneEmptyField = a
		})
		if !strings.Contains(out, "n=0") {
			t.Errorf("%v: declared empty association gave %q, want n=0", a, out)
		}
	}
	// A non-empty array asks nothing and keeps its fields either way.
	out, _ = axisRun(t, `a=(x y); set -- "${a[@]}"; echo "n=$#"`, func(s *Semantics) {})
	if !strings.Contains(out, "n=2") {
		t.Errorf("got %q, want the fields kept without a question", out)
	}
}

func TestANegativeSubstringLengthIsAnAxis(t *testing.T) {
	out, _ := axisRun(t, `x=abcdef; echo "[${x:1:-2}]"`, func(s *Semantics) {
		s.SubstringNegativeLengthIsEmpty = Yes
	})
	if !strings.Contains(out, "[]") {
		t.Errorf("got %q, want nothing at all", out)
	}
	out, _ = axisRun(t, `x=abcdef; echo "[${x:1:-2}]"`, func(s *Semantics) {
		s.SubstringNegativeLengthIsEmpty = No
	})
	if !strings.Contains(out, "[bcd]") {
		t.Errorf("got %q, want the count from the end", out)
	}
}

func TestLinenoInAFunctionIsAnAxis(t *testing.T) {
	src := "f(){\necho $LINENO\n}\nf"
	out, _ := axisRun(t, src, func(s *Semantics) { s.LinenoCountsFromTheFunction = Yes })
	if strings.TrimSpace(out) != "1" {
		t.Errorf("got %q, want the function-relative line", out)
	}
	out, _ = axisRun(t, src, func(s *Semantics) { s.LinenoCountsFromTheFunction = No })
	if strings.TrimSpace(out) != "2" {
		t.Errorf("got %q, want the file line", out)
	}
}

// The three brace-range axes, each asked only when a range reaches its
// disagreement: what an endpoint's leading zeros mean, and what a written
// step's sign means.

func TestBraceRangePaddingIsAnAxis(t *testing.T) {
	src := `echo {01..3}`
	out, _ := axisRun(t, src, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangePadsToEndpointWidth = Yes
	})
	if strings.TrimSpace(out) != "01 02 03" {
		t.Errorf("got %q, want the range padded to the widest endpoint", out)
	}
	out, _ = axisRun(t, src, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangePadsToEndpointWidth = No
	})
	if strings.TrimSpace(out) != "1 2 3" {
		t.Errorf("got %q, want the padding stripped", out)
	}
	// Unanswered, the padded endpoint is a refusal — but only in a dialect
	// whose braces expand at all: with them off the word is a literal and
	// the zeros are never a question.
	if _, st := axisRun(t, src, func(s *Semantics) { s.BraceExpansion = Yes }); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
	out, st := axisRun(t, src, func(s *Semantics) { s.BraceExpansion = No })
	if st != 0 || strings.TrimSpace(out) != "{01..3}" {
		t.Errorf("got %q status %d, want the literal word without a question", out, st)
	}
	// An unpadded range asks nothing.
	out, st = axisRun(t, `echo {1..3}`, func(s *Semantics) { s.BraceExpansion = Yes })
	if st != 0 || strings.TrimSpace(out) != "1 2 3" {
		t.Errorf("got %q status %d, want the range expanded without a question", out, st)
	}
}

func TestBraceRangeStepSignIsAnAxis(t *testing.T) {
	// A sign pointing away from the far endpoint: honored, the range holds
	// its first element alone; ignored, the endpoints set the direction and
	// the step contributes magnitude.
	for _, tc := range []struct{ src, honored, ignored string }{
		{`echo {10..1..3}`, "10", "10 7 4 1"},
		{`echo {a..e..-1}`, "a", "a b c d e"},
	} {
		out, _ := axisRun(t, tc.src, func(s *Semantics) {
			s.BraceExpansion = Yes
			s.BraceRangeStepSignHonored = Yes
		})
		if strings.TrimSpace(out) != tc.honored {
			t.Errorf("%s honored: got %q, want %q", tc.src, out, tc.honored)
		}
		out, _ = axisRun(t, tc.src, func(s *Semantics) {
			s.BraceExpansion = Yes
			s.BraceRangeStepSignHonored = No
			s.BraceRangeNegativeStepReverses = No
		})
		if strings.TrimSpace(out) != tc.ignored {
			t.Errorf("%s ignored: got %q, want %q", tc.src, out, tc.ignored)
		}
	}
	if _, st := axisRun(t, `echo {10..1..3}`, func(s *Semantics) { s.BraceExpansion = Yes }); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
	// A step whose sign agrees with the endpoints asks nothing of this axis.
	out, st := axisRun(t, `echo {1..10..3}`, func(s *Semantics) { s.BraceExpansion = Yes })
	if st != 0 || strings.TrimSpace(out) != "1 4 7 10" {
		t.Errorf("got %q status %d, want the stride without a question", out, st)
	}
}

func TestBraceRangeNegativeStepReversalIsAnAxis(t *testing.T) {
	// `{3..1..-1}` is the corner where the sign agrees with the endpoints,
	// so the sign axis is not asked and only the reversal question remains.
	out, _ := axisRun(t, `echo {3..1..-1}`, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangeNegativeStepReverses = Yes
	})
	if strings.TrimSpace(out) != "1 2 3" {
		t.Errorf("got %q, want the walk reversed", out)
	}
	out, _ = axisRun(t, `echo {3..1..-1}`, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangeNegativeStepReverses = No
	})
	if strings.TrimSpace(out) != "3 2 1" {
		t.Errorf("got %q, want the endpoints' order kept", out)
	}
	// The walk is reversed rather than the endpoints swapped: `{1..10..-4}`
	// is `1 5 9` backwards, not the `10 6 2` of `{10..1..4}`.
	out, _ = axisRun(t, `echo {1..10..-4}`, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangeStepSignHonored = No
		s.BraceRangeNegativeStepReverses = Yes
	})
	if strings.TrimSpace(out) != "9 5 1" {
		t.Errorf("got %q, want the endpoint walk reversed", out)
	}
	// Honored, the sign never reaches the reversal question.
	out, _ = axisRun(t, `echo {1..10..-4}`, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangeStepSignHonored = Yes
	})
	if strings.TrimSpace(out) != "1" {
		t.Errorf("got %q, want the honored sign to answer first", out)
	}
	if _, st := axisRun(t, `echo {3..1..-1}`, func(s *Semantics) { s.BraceExpansion = Yes }); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
}

func TestALocalInheritingTheExportAttributeIsAnAxis(t *testing.T) {
	// Read through a real child rather than through a listing: what the axis
	// decides is what a command is told, and a shell can hold the attribute
	// and still hand the entry over, or the other way about.
	const shadow = `export FOO=bar; f() { local FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`
	out, _ := axisRun(t, shadow, func(s *Semantics) { s.LocalInheritsTheExportAttribute = Yes })
	if !strings.Contains(out, "FOO=baz") {
		t.Errorf("got %q, want the child told the local's value", out)
	}
	out, _ = axisRun(t, shadow, func(s *Semantics) { s.LocalInheritsTheExportAttribute = No })
	if !strings.Contains(out, "(none)") {
		t.Errorf("got %q, want the child told nothing under the name", out)
	}
	// Taking the attribute off is the local's, and it goes back with the
	// value when the function returns.
	out, _ = axisRun(t, `export FOO=bar; f() { local FOO=baz; }; f; /usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) { s.LocalInheritsTheExportAttribute = No })
	if !strings.Contains(out, "FOO=bar") {
		t.Errorf("got %q, want the outer name exported again", out)
	}
	// A local naming the attribute itself answers the question outright.
	out, _ = axisRun(t, `export FOO=bar; f() { local -x FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`,
		func(s *Semantics) {
			s.LocalInheritsTheExportAttribute = No
			s.LocalOptions = "x"
		})
	if !strings.Contains(out, "FOO=baz") {
		t.Errorf("got %q, want the letter to say so outright", out)
	}
	// A name nothing exported asks nothing, so an unanswered axis is not
	// reached and the child is told nothing either way.
	out, st := axisRun(t, `FOO=bar; f() { local FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`, func(*Semantics) {})
	if st != 0 || !strings.Contains(out, "(none)") {
		t.Errorf("got %q status %d, want no question and no entry", out, st)
	}
	// And where it is exported, an unanswered axis is refused rather than
	// guessed at: the declaration is not made at all.
	if _, st := axisRun(t, `export FOO=bar; f() { local FOO=baz; }; f`, func(*Semantics) {}); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
}
