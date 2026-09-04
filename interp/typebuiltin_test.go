// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `type` says what a name would run, and every part of the sentence is a
// dialect's own words — down to whether a missing name gets the shell's name
// in front of it.
func TestTypeSaysWhatANameWouldRun(t *testing.T) {
	dg := Diagnostics{
		TypeKeyword:  "%[1]s is a reserved word",
		TypeFunction: "%[1]s is a shell function",
		TypeExternal: "%[1]s is a tracked alias for %[2]s",
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a builtin", `type cd`, "cd is a shell builtin"},
		{"a keyword", `type if`, "if is a reserved word"},
		{"a function", `f(){ :; }; type f`, "f is a shell function"},
		{"an external", `type /bin/ls`, "/bin/ls is a tracked alias for /bin/ls"},
		// A function wins over a builtin of the same name, which is the
		// order a command is actually resolved in.
		{"a function shadowing a builtin", `cd(){ :; }; type cd`, "cd is a shell function"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) {
				sem := CoreSemantics()
				sem.TypePrintsFunctionBody = No
				r.Semantics, r.Diagnostics = &sem, &dg
			})
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("out = %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// A name it cannot account for: four wordings, two statuses, and two of the
// four print the line with nothing in front of it.
func TestTypeReportsANameThatIsNothing(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dg         Diagnostics
		wantStatus int
	}{
		{"a plain failure", Diagnostics{TypeNotFound: "type: %[1]s: not found"}, 1},
		{"or a missing command's", Diagnostics{TypeNotFound: "%[1]s: not found", TypeNotFoundStatus: 127}, 127},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, `type nope`, func(r *Runner) {
				sem := CoreSemantics()
				r.Semantics, r.Diagnostics = &sem, &tc.dg
			})
			if st != tc.wantStatus {
				t.Errorf("status %d, want %d", st, tc.wantStatus)
			}
			if !strings.Contains(out, "nope") {
				t.Errorf("out = %q, want the name in it", out)
			}
		})
	}
}

// Two of the four write the not-found line with nothing in front of it, where
// every other message they print carries the shell's name. Asserted as the
// whole line, because a prefix is exactly what a substring check cannot see.
func TestTypeCanReportAMissingNameWithNoPrefix(t *testing.T) {
	for _, tc := range []struct {
		name       string
		unprefixed bool
		want       string
	}{
		{"with the shell's name", false, "sh: type: nope: not found"},
		{"and without it", true, "type: nope: not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `type nope`, func(r *Runner) {
				sem := CoreSemantics()
				dg := Diagnostics{
					TypeNotFound:           "type: %[1]s: not found",
					TypeNotFoundUnprefixed: tc.unprefixed,
				}
				r.Semantics, r.Diagnostics, r.Name = &sem, &dg, "sh"
			})
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("out = %q, want exactly %q", got, tc.want)
			}
		})
	}
}

// `--` ends the options in three of the four, and is a name in the fourth,
// which has no options for `type` at all.
func TestTypeAndTheDoubleDash(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ends  Answer
		want  string
		notIn string
	}{
		{"where it ends the options", Yes, "cd is a shell builtin", "--"},
		{"and where it is a name", No, "--", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `type -- cd`, func(r *Runner) {
				sem := CoreSemantics()
				sem.TypeEndsOptionsWithDashDash = tc.ends
				dg := Diagnostics{TypeNotFound: "%[1]s: not found"}
				r.Semantics, r.Diagnostics = &sem, &dg
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want it to contain %q", out, tc.want)
			}
			if tc.notIn != "" && strings.Contains(out, tc.notIn) {
				t.Errorf("out = %q, want no answer about %q", out, tc.notIn)
			}
			// Either way the real name is still answered.
			if !strings.Contains(out, "cd is a shell builtin") {
				t.Errorf("out = %q, want the name answered", out)
			}
		})
	}
}

// One name failing does not stop the rest, and the failure is what the whole
// command reports.
func TestTypeAnswersEveryNameAndReportsTheFailure(t *testing.T) {
	out, st := run(t, `type cd nope ls`, func(r *Runner) {
		sem := CoreSemantics()
		dg := Diagnostics{TypeNotFound: "type: %[1]s: not found"}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	for _, want := range []string{"cd is a shell builtin", "nope", "ls is /"} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q, want it to contain %q", out, want)
		}
	}
	if st == 0 {
		t.Error("status 0, want the one failure reported")
	}
}

// The same guard `command -v` has: there is an executable called
// /usr/bin/umask and this shell will not run it, so `type` must not say where
// it is. A script that asks before using it would be told yes and then fail.
func TestTypeWillNotNameAReservedBuiltinOnPath(t *testing.T) {
	dir := t.TempDir()
	// `hash` rather than `alias`, which is a builtin now and would be named
	// as one — correctly. The guard needs a name that is still reserved and
	// still unimplemented.
	if err := os.WriteFile(filepath.Join(dir, "hash"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	out, st := run(t, `type hash`, func(r *Runner) {
		sem := CoreSemantics()
		dg := Diagnostics{TypeNotFound: "type: %[1]s: not found"}
		r.Semantics, r.Diagnostics = &sem, &dg
		r.Vars = map[string]string{"PATH": dir}
	})
	if strings.Contains(out, dir) {
		t.Errorf("out = %q, want the shell not to name a PATH hit it would refuse to run", out)
	}
	if st == 0 {
		t.Error("status 0, want it reported as not found")
	}
}

// One dialect follows the sentence with the function itself, laid out — which
// is what the printer exists for: the tree is what the shell has by then, and
// the spelling it was written with is gone.
func TestTypeCanShowAFunctionsBody(t *testing.T) {
	out, st := run(t, "f(){ echo hi; }; type f", func(r *Runner) {
		sem := CoreSemantics()
		sem.TypePrintsFunctionBody = Yes
		r.Semantics = &sem
		// The arrangement is the caller's: the core has none, so a runner
		// told nothing prints the body on one line. Supplying one here is
		// what a dialect does.
		r.SetFunctionLayout(syntax.Layout{
			Indent: "    ", Nested: true, Lines: true,
			Separator: ";", KeywordTerminator: ";",
			BraceOpenSuffix: " ", OutermostBraceOpensALine: true,
		}, syntax.Layout{})
	})
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	// The sentence first, then the function — reformatted rather than
	// echoed, which is the whole difference between a printer and keeping
	// the text.
	want := "f is a function\nf () \n{ \n    echo hi\n}\n"
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// And the three that stop at the sentence print nothing after it, however
// long the function is.
func TestTypeCanStopAtTheSentence(t *testing.T) {
	out, _ := run(t, "f(){ echo hi; echo there; }; type f", func(r *Runner) {
		sem := CoreSemantics()
		sem.TypePrintsFunctionBody = No
		r.Semantics = &sem
	})
	if out != "f is a function\n" {
		t.Errorf("out = %q, want the sentence alone", out)
	}
}

// TestTypeRefusesAnOptionItDoesNotHave. Skipping a leading `-` word silently
// made `type -t ls` answer about a name called `-t` and then about ls, which
// is two wrong answers where one complaint was wanted.
func TestTypeRefusesAnOptionItDoesNotHave(t *testing.T) {
	out, _ := run(t, `type -x ls`, func(r *Runner) {
		s := *r.Semantics
		s.TypeEndsOptionsWithDashDash = Yes
		r.Semantics = &s
		dg := Diagnostics{BuiltinBadOption: "type: %[2]s: bad"}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "type: -x: bad") {
		t.Errorf("got %q, want the option refused", out)
	}
	if strings.Contains(out, "not found") {
		t.Errorf("got %q, want it not looked up as a name", out)
	}

	// The dialect with no options at all still reads it as a name, which is
	// the answer the existing axis already carried.
	out, _ = run(t, `type -x`, func(r *Runner) {
		s := *r.Semantics
		s.TypeEndsOptionsWithDashDash = No
		r.Semantics = &s
	})
	if strings.Contains(out, "bad") {
		t.Errorf("got %q, want no option complaint where there are no options", out)
	}
}
