// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The two opt-in lints about where an assignment inside a function landed —
// see Runner.WarnsAboutAGlobalCreatedInAFunction and
// Runner.WarnsAboutAnEnclosingScopeSet. Session switches and not axes, so
// this names the switches and no shell.
//
// The wordings are the test's own rather than a dialect's, and they are
// deliberately unlike anything a shell writes: what is being pinned is which
// verbs reach the format and when it is written at all, and borrowing a
// dialect's sentence would have made the rows read as a claim about that
// shell. `testsh: ` in front of each expectation is this shell's own
// location, which the lint goes through like any other diagnostic — and
// which is not the *builtin's*: a `read` or a `printf -v` that makes the
// write is not what the remark is about.
func scopeLintDiagnostics() Diagnostics {
	return Diagnostics{
		GlobalCreatedInAFunction:     "CREATE<%[1]s|%[2]s|%[3]s>",
		EnclosingScopeSetInAFunction: "OUTER<%[1]s|%[2]s|%[3]s>",
	}
}

func scopeLintRun(t *testing.T, src string, create, outer bool) string {
	t.Helper()
	out, _ := sourceRunWith(t, t.TempDir(), src, permissive(), scopeLintDiagnostics(),
		func(r *Runner) {
			r.SetWarnsAboutAGlobalCreatedInAFunction(create)
			r.SetWarnsAboutAnEnclosingScopeSet(outer)
		})
	return out
}

func TestTheScopeLintsSayWhatAnAssignmentDid(t *testing.T) {
	for _, tc := range []struct {
		name         string
		src          string
		create, both string
	}{
		{
			"a name nothing had is created",
			"f() { gv=1; }\nf\necho gv=$gv\n",
			"testsh: CREATE<scalar|gv|f>\ngv=1\n", "testsh: CREATE<scalar|gv|f>\ngv=1\n",
		},
		{
			"a name the caller made local is set in an enclosing scope",
			"outer() { local lv=1; inner; echo lv=$lv; }\ninner() { lv=2; }\nouter\n",
			"lv=2\n", "testsh: OUTER<scalar|lv|inner>\nlv=2\n",
		},
		{
			"and a global the script already had is the same question",
			"gt=0\ns() { gt=5; }\ns\n",
			"", "testsh: OUTER<scalar|gt|s>\n",
		},
		{
			"one body can draw one of each",
			"q() { gq=1; gq=2; }\nq\n",
			"testsh: CREATE<scalar|gq|q>\n", "testsh: CREATE<scalar|gq|q>\ntestsh: OUTER<scalar|gq|q>\n",
		},
		{
			"an array literal is an array to the sentence",
			"i() { arr=(a b); }\ni\necho n=${#arr[@]}\n",
			"testsh: CREATE<array|arr|i>\nn=2\n", "testsh: CREATE<array|arr|i>\nn=2\n",
		},
		{
			"a local in this call silences the bare assignment after it",
			"r() { local rv=0; rv=1; echo rv=$rv; }\nr\n",
			"rv=1\n", "rv=1\n",
		},
		{
			"and a declaration is silent even where it creates the global",
			"d() { local dv=1; export ev=2; }\nd\necho ev=$ev\n",
			"ev=2\n", "ev=2\n",
		},
		{
			"the top level has no function to name",
			"tv=1\n",
			"", "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := scopeLintRun(t, tc.src, true, false); got != tc.create {
				t.Errorf("create only: got %q, want %q", got, tc.create)
			}
			if got := scopeLintRun(t, tc.src, true, true); got != tc.both {
				t.Errorf("both on: got %q, want %q", got, tc.both)
			}
			// The control every row shares: with neither switch on the same
			// source writes only what it computes. A row that passed above
			// and here cannot be a shell that says nothing at all.
			quiet := strings.ReplaceAll(tc.both, "testsh: CREATE<", "\x00")
			quiet = strings.ReplaceAll(quiet, "testsh: OUTER<", "\x00")
			var want strings.Builder
			for _, line := range strings.SplitAfter(quiet, "\n") {
				if !strings.HasPrefix(line, "\x00") {
					want.WriteString(line)
				}
			}
			if got := scopeLintRun(t, tc.src, false, false); got != want.String() {
				t.Errorf("neither on: got %q, want %q", got, want.String())
			}
		})
	}
}

// A shell that holds no wording says nothing however the switches stand,
// which is what keeps the capability in the core and the sentence in the
// dialect.
func TestAShellWithNoWordingSaysNothing(t *testing.T) {
	out, _ := sourceRunWith(t, t.TempDir(), "f() { gv=1; }\nf\necho gv=$gv\n",
		permissive(), Diagnostics{}, func(r *Runner) {
			r.SetWarnsAboutAGlobalCreatedInAFunction(true)
			r.SetWarnsAboutAnEnclosingScopeSet(true)
		})
	if want := "gv=1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A subshell is a shell of its own and is not running inside the call that
// made it, so an assignment written straight into one draws nothing — while a
// function *called* inside it draws the sentence and names itself. The pair
// is what says the test is the frame this shell made rather than a name still
// standing from the caller.
func TestTheScopeLintsAskAboutTheCallThisShellMade(t *testing.T) {
	out := scopeLintRun(t, "t() { (tv=1); }\nt\n", true, true)
	if out != "" {
		t.Errorf("an assignment in a subshell: got %q, want nothing", out)
	}
	out = scopeLintRun(t, "t() { (f() { tv=1; }; f); }\nt\n", true, true)
	if want := "testsh: CREATE<scalar|tv|f>\n"; out != want {
		t.Errorf("a call inside a subshell: got %q, want %q", out, want)
	}
}
