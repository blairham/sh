// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// An assignment **prefix** — `name=value cmd` — and what its right-hand side
// is. The axis is [Semantics.ScalarAssignmentValueIsGlobbed], the same one the
// statement form reads, and the rule is keyed on **the assignment**: the
// prefix follows `a=*.txt` and not `typeset a=*.txt`.
//
// Every case runs in a directory this file populates, with a positive control
// beside it: a `printf '%s\n' *.txt` in the same run prints the three names,
// so a row that reports the six characters is reporting a value that was not
// globbed rather than a directory that had nothing in it. A null nobody can
// falsify is silence, and "the pattern matched nothing" and "the value was
// kept" read alike without it.

// prefixGlobTree is the directory the cases below run in:
//
//	a.txt b.txt c.txt  three names one pattern reaches
//	one.only           the single match, which stays a scalar
//	plain              a name with no metacharacter to be about
func prefixGlobTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{"a.txt", "b.txt", "c.txt", "one.only", "plain"} {
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// prefixGlobRun runs src in that directory at one answer for the axis, and
// hands back both streams.
func prefixGlobRun(t *testing.T, src string, answer Answer) (string, string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.ScalarAssignmentValueIsGlobbed = answer
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Dir: prefixGlobTree(t), Name: "testsh",
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v\nstderr: %s", src, rerr, errs.String())
	}
	return out.String(), errs.String()
}

// The instrument first. A pattern in an ordinary word matches in this
// directory at every answer the axis takes, so the rows below that report
// `*.txt` are reporting the value and not an empty directory.
func TestThePrefixGlobDirectoryReallyHoldsTheNames(t *testing.T) {
	for _, answer := range []Answer{Unspecified, No, Yes} {
		out, _ := prefixGlobRun(t, `printf '[%s]' *.txt`, answer)
		if out != "[a.txt][b.txt][c.txt]" {
			t.Errorf("answer %v: control = %q, want the three names", answer, out)
		}
	}
}

// **A prefix's value is an assignment's value, so it is not a pattern.**
// Unanimous across the panel and the core's own answer: measured 2026-09-26,
// `a=*.txt printenv a` is `*.txt` in zsh 5.9.2, bash 5.3.20, ksh93u+
// 2012-08-01, /bin/dash and BusyBox ash v1.37.0 alike. This shell handed the
// command `a.txt b.txt c.txt` in every dialect, because the prefix took
// Runner.expandWord — the *word* road, which globs (#4657).
//
// Both the unspecified core and an explicit No, because they are two different
// claims: the second is what a dialect writes down and the first is what a
// Runner with no dialect chosen does, and this axis is read directly against
// Yes so that a bare core is never asked.
func TestAPrefixAssignmentValueIsNotAPatternAtNo(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"several would match", `f() { printf '[%s]' "$a"; }; a=*.txt f`, "[*.txt]"},
		{"one would match", `f() { printf '[%s]' "$a"; }; a=one.* f`, "[one.*]"},
		{"none would match", `f() { printf '[%s]' "$a"; }; a=*.nomatch f`, "[*.nomatch]"},
		{"no metacharacter", `f() { printf '[%s]' "$a"; }; a=plain f`, "[plain]"},
		{"a builtin", `a=*.txt eval 'printf "[%s]" "$a"'`, "[*.txt]"},
		{"two prefixes", `f() { printf '[%s][%s]' "$a" "$b"; }; a=*.txt b=one.* f`, "[*.txt][one.*]"},
		{"an append", `f() { printf '[%s]' "$a"; }; a=x; a+=*.txt f`, "[x*.txt]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, answer := range []Answer{Unspecified, No} {
				out, _ := prefixGlobRun(t, c.src, answer)
				if out != c.want {
					t.Errorf("answer %v: got %q, want %q", answer, out, c.want)
				}
			}
		})
	}
}

// **The same value through the same road, at Yes.** How many names matched
// decides the kind: several is an array, one is a *scalar* and not a
// one-element array, and a value with no metacharacter in it is stored as
// itself — which is the control that keeps this from being "the axis rewrites
// every prefix as a list".
func TestAPrefixAssignmentValueIsAPatternAtYes(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"several matches", `f() { printf '[%s]' "${a[@]}"; }; a=*.txt f`, "[a.txt][b.txt][c.txt]"},
		{"one match stays a scalar", `f() { printf '[%s]' "${a[@]}"; }; a=one.* f`, "[one.only]"},
		{"no metacharacter", `f() { printf '[%s]' "${a[@]}"; }; a=plain f`, "[plain]"},
		{"a quoted pattern", `f() { printf '[%s]' "${a[@]}"; }; a='*.txt' f`, "[*.txt]"},
		{
			// **The second gate is not this axis.** A written metacharacter
			// counts and a quoted one does not — the row above — while one a
			// *value* carries counts wherever the shell says an unquoted
			// expansion's characters are a pattern, which the core's POSIX
			// answer says and zsh's `GLOB_SUBST` default does not. So this
			// row is the three names here and `*.txt` in zsh 5.9.2 under the
			// option, and both are the same rule read through two different
			// answers to a different question.
			"a value carrying a pattern",
			`v='*.txt'; f() { printf '[%s]' "${a[@]}"; }; a=$v f`,
			"[a.txt][b.txt][c.txt]",
		},
		{"the second of two", `f() { printf '[%s]' "${b[@]}"; }; a=plain b=*.txt f`, "[a.txt][b.txt][c.txt]"},
		{
			// The append is not an append: a match replaces the name
			// whichever operator was written.
			"an append a match replaces",
			`a=x; f() { printf '[%s]' "${a[@]}"; }; a+=one.* f`, "[one.only]",
		},
		{
			// And the joining `+=` is still there for a value that is not a
			// pattern, which keeps the row above from reading as "the axis
			// breaks append".
			"an append with no pattern",
			`a=x; f() { printf '[%s]' "${a[@]}"; }; a+=plain f`, "[xplain]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, _ := prefixGlobRun(t, c.src, Yes); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// **The prefix does not persist**, at either answer, which is the half of the
// pair that says what the rule is keyed on. `a=*.txt cmd` leaves this shell's
// own `a` exactly where it was and globs its value all the same — so "an
// assignment that stores into the shell" is the wrong noun for the axis and
// "the assignment" is the right one. The two readings agree on every row a
// statement can produce and part here.
func TestAGlobbedPrefixIsStillTakenBackAfterward(t *testing.T) {
	for _, answer := range []Answer{No, Yes} {
		for _, c := range []struct{ name, src, want string }{
			{"a name that was holding something", `a=kept; a=*.txt true; printf '[%s]' "$a"`, "[kept]"},
			{"a name that was not", `a=*.txt true; printf '[%s]' "${a-UNSET}"`, "[UNSET]"},
			{"a single match", `a=kept; a=one.* true; printf '[%s]' "$a"`, "[kept]"},
		} {
			t.Run(c.name, func(t *testing.T) {
				if out, _ := prefixGlobRun(t, c.src, answer); out != c.want {
					t.Errorf("answer %v: got %q, want %q", answer, out, c.want)
				}
			})
		}
	}
}

// **A match of more than one name reaches no child's environment.** The
// entry is an array and no environment carries one, so the name is *absent*
// from what the child is handed rather than left showing what the shell was
// exporting under it. Measured on zsh 5.9.2, 2026-09-26: `setopt globassign;
// export a=old; a=*.txt printenv a` prints nothing and exits 1, where the
// same line at one match prints `one.only`.
//
// The controls are the three rows that must still reach the child: one match,
// no metacharacter, and the whole grid at No.
func TestAPrefixThatMatchedSeveralNamesReachesNoChildEnvironment(t *testing.T) {
	if _, err := os.Stat("/usr/bin/printenv"); err != nil {
		t.Skip("no /usr/bin/printenv")
	}
	for _, c := range []struct{ name, src, no, yes string }{
		{"several matches", `a=*.txt /usr/bin/printenv a`, "*.txt\n", ""},
		{"one match", `a=one.* /usr/bin/printenv a`, "one.*\n", "one.only\n"},
		{"no metacharacter", `a=plain /usr/bin/printenv a`, "plain\n", "plain\n"},
		{
			"several matches over an exported name",
			`export a=old; a=*.txt /usr/bin/printenv a`, "*.txt\n", "",
		},
		{
			"the other prefix still reaches it",
			`a=*.txt b=one.* /usr/bin/printenv b`, "one.*\n", "one.only\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct {
				answer Answer
				want   string
			}{{No, c.no}, {Yes, c.yes}} {
				out, _ := prefixGlobRun(t, c.src, state.answer)
				if out != state.want {
					t.Errorf("answer %v: got %q, want %q", state.answer, out, state.want)
				}
			}
		})
	}
}

// The road the prefix takes is the assignment's, and globbing is not the only
// thing that hangs off that. A prefix's value is not split on IFS either, in
// every column of the panel — measured 2026-09-26, `IFS=:; v=a:b; a=$v
// printenv a` is `a:b` in zsh 5.9.2 and bash 5.3.20 alike — and this shell
// answered `a b`, because the word road splits. The colon tildes go the same
// way: `a=~/x:~/y cmd` is two homes in every column and was one here.
//
// Kept beside the glob because the three are one road, and a fix that took
// only the first back would leave the other two saying the road is still the
// word's. The brace row is the control that road carries no brace expansion
// at the core's own answer; the column where braces *are* expanded has it in
// dialect/bash.
func TestAPrefixAssignmentValueTakesTheAssignmentRoad(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"not split on IFS", `IFS=:; v=a:b; f() { printf '[%s]' "$a"; }; a=$v f`, "[a:b]"},
		{"inner spaces kept", `v='p  q'; f() { printf '[%s]' "$a"; }; a=$v f`, "[p  q]"},
		{"not brace-expanded", `f() { printf '[%s]' "$a"; }; a={p,q} f`, "[{p,q}]"},
		{
			// The colon tildes only an assignment has, which is what makes
			// `PATH=~/bin:~/sbin cmd` work.
			"a tilde after each colon",
			`HOME=/H; f() { printf '[%s]' "$a"; }; a=~/x:~/y f`, "[/H/x:/H/y]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, answer := range []Answer{No, Yes} {
				if out, _ := prefixGlobRun(t, c.src, answer); out != c.want {
					t.Errorf("answer %v: got %q, want %q", answer, out, c.want)
				}
			}
		})
	}
}

// The trace says what the command was handed rather than what the script
// typed: a matched prefix whose pattern reached more than one name writes the
// element list a written literal gets, from a line with no parentheses in it.
// One match writes the value bare, and at No the characters stand. Rendered
// with the core's own quoting and bracketing rather than zsh's — the three
// Diagnostics fields that decide those are a different question, and
// dialect/zsh has the row that pins what that shell writes.
func TestAGlobbedPrefixIsTracedAsWhatItHandedOver(t *testing.T) {
	for _, c := range []struct{ name, src, no, yes string }{
		{"several matches", `a=*.txt true`, `+ a=*.txt`, `+ a=(a.txt b.txt c.txt)`},
		{"one match", `a=one.* true`, `+ a=one.*`, `+ a=one.only`},
		{"no metacharacter", `a=plain true`, `+ a=plain`, `+ a=plain`},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct {
				answer Answer
				want   string
			}{{No, c.no}, {Yes, c.yes}} {
				_, errs := prefixGlobRun(t, "set -x\n"+c.src, state.answer)
				if !strings.Contains(errs, state.want) {
					t.Errorf("answer %v: trace = %q, want %q in it", state.answer, errs, state.want)
				}
			}
		})
	}
}
