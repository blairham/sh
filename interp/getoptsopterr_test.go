// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A parameter that turns the `getopts` diagnostic off (#2945).
//
// One dialect has it and the rest read OPTERR as an ordinary variable, and it
// is not the silent form by another spelling: a leading colon in the option
// string also changes what arrives in the name and in OPTARG, where this
// changes only whether the sentence is written. A script that wants to keep
// `?` and lose the noise has exactly one way to say so.

func opterrSem(a Answer) Semantics {
	s := getoptsSem()
	s.GetoptsOptErrSilencesTheComplaint = a
	return s
}

func opterrDiag() *Diagnostics {
	return &Diagnostics{
		GetoptsBadOption:       "illegal option -- %[1]s",
		GetoptsMissingArgument: "option requires an argument -- %[1]s",
	}
}

// The axis itself, over both complaints — a letter the string does not have
// and a letter whose argument is missing.
func TestGetoptsOptErrSilencesTheComplaintIsAnAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, msg string }{
		{"a bad option", `set -- -z; getopts "a" o`, "illegal option -- z"},
		{"a missing argument", `set -- -a; getopts "a:" o`, "option requires an argument -- a"},
	} {
		for _, ans := range []struct {
			name   string
			answer Answer
			quiet  bool
		}{
			// bash 5.3.20, bash 3.2.57 and bash as `sh`.
			{"silenced", Yes, true},
			// ksh93u+, dash 0.5.12, BusyBox ash 1.37.0 and zsh 5.9.2, for
			// which OPTERR is a name like any other.
			{"an ordinary variable", No, false},
		} {
			t.Run(tc.name+", "+ans.name, func(t *testing.T) {
				sem := opterrSem(ans.answer)
				out, _ := run(t, "OPTERR=0\n"+tc.src, func(r *Runner) {
					r.Semantics, r.Diagnostics = &sem, opterrDiag()
				})
				if got := strings.Contains(out, tc.msg); got == ans.quiet {
					t.Errorf("got %q, want the complaint written = %v", out, !ans.quiet)
				}
			})
		}
	}
}

// What it leaves alone, which is everything else: the name, the status and
// OPTARG are the same either way, so the two answers differ in the sentence
// and in nothing a script can branch on.
func TestGetoptsOptErrLeavesTheAnswerAlone(t *testing.T) {
	const src = `OPTERR=0
set -- -z rest
getopts "a" o
echo "st=$? [$o] [${OPTARG-unset}] ind=$OPTIND"`
	for _, answer := range []Answer{No, Yes} {
		t.Run(answer.String(), func(t *testing.T) {
			sem := opterrSem(answer)
			out, _ := run(t, src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, opterrDiag() })
			if want := "st=0 [?] [unset] ind=2\n"; !strings.HasSuffix(out, want) {
				t.Errorf("got %q, want it to end with %q", out, want)
			}
		})
	}
}

// The usage complaint is not this question: `getopts` given fewer than two
// operands still writes its line with the parameter set to zero, which is the
// one place the two could have been folded together and must not be.
func TestGetoptsOptErrDoesNotReachTheUsageLine(t *testing.T) {
	sem := opterrSem(Yes)
	dg := opterrDiag()
	dg.BuiltinUsage = map[string]string{"getopts": "usage: getopts optstring name [arg]"}
	out, _ := run(t, `OPTERR=0; getopts`, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, dg })
	if !strings.Contains(out, "usage: getopts") {
		t.Errorf("got %q, want the usage line written", out)
	}
}

// The silent form was already silent and stays so, which is the half that
// says the two mechanisms are separate: the leading colon reports through
// OPTARG and the name, and the parameter only ever removed a sentence.
func TestGetoptsOptErrLeavesTheSilentFormAlone(t *testing.T) {
	for _, answer := range []Answer{No, Yes} {
		t.Run(answer.String(), func(t *testing.T) {
			sem := opterrSem(answer)
			out, _ := run(t, "OPTERR=0\nset -- -z\ngetopts \":a\" o\necho \"[$o][$OPTARG]\"",
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, opterrDiag() })
			if want := "[?][z]\n"; out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}

// How the value is read, which is not how arithmetic reads one: blanks, an
// optional sign, then decimal digits, stopping at the first byte that is not
// one. Each row is a spelling measured on the shell that has the parameter.
func TestGetoptsOptErrValueReading(t *testing.T) {
	for _, tc := range []struct {
		value string
		quiet bool
	}{
		{"0", true},
		{"1", false},
		{"2", false},
		{"-1", false},
		{"-0", true},
		{"+0", true},
		{"+", true},
		{"-", true},
		{"00", true},
		{"08", false},
		{"0000000000000000001", false},
		{"9999999999999999999999", false},
		{"x", true},
		{"0x0", true},
		{"0abc", true},
		{"1abc", false},
		{"0e0", true},
		{"0.0", true},
		{"1+1", false},
		{"a[0]", true},
		{" 1", false},
		{" 0 ", true},
		{"\t0", true},
		{"\n0", true},
		{"\r0", true},
		{"\v0", true},
		{"\f0", true},
		{"\n1", false},
		{" ", true},
		{"0 1", true},
		{"1 0", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			sem := opterrSem(Yes)
			// Handed over as a variable rather than written into the snippet:
			// several of these hold a newline or a tab, and quoting them back
			// into shell source would be testing the lexer rather than the
			// read.
			out, _ := run(t, "set -- -z\ngetopts \"a\" o", func(r *Runner) {
				r.Semantics, r.Diagnostics = &sem, opterrDiag()
				r.Vars = map[string]string{"OPTERR": tc.value}
			})
			if got := out == ""; got != tc.quiet {
				t.Errorf("OPTERR=%q: got %q, want silenced = %v", tc.value, out, tc.quiet)
			}
		})
	}
}

// The empty string is the one value that is not a number: `OPTERR=` writes
// the sentence exactly as an unset OPTERR does, and an implementation reading
// it through the same number read would silence it — zero being what an empty
// string comes to.
func TestGetoptsOptErrEmptyAndUnsetAreNotZero(t *testing.T) {
	for _, tc := range []struct{ name, prefix string }{
		{"unset", "unset OPTERR\n"},
		{"empty", "OPTERR=\n"},
		{"never mentioned", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := opterrSem(Yes)
			out, _ := run(t, tc.prefix+"set -- -z\ngetopts \"a\" o",
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, opterrDiag() })
			if !strings.Contains(out, "illegal option -- z") {
				t.Errorf("got %q, want the complaint written", out)
			}
		})
	}
}

// An unanswered axis is reported rather than guessed — but only where a
// script has set the parameter to a value that reads as zero. Every other
// `getopts`, in every dialect, reaches no question at all.
func TestGetoptsOptErrIsAskedOnlyWhereItReadsZero(t *testing.T) {
	const axis = "OPTERR reading zero silencing the `getopts` complaint"
	t.Run("set to zero", func(t *testing.T) {
		sem := opterrSem(Unspecified)
		out, _ := run(t, "OPTERR=0\nset -- -z\ngetopts \"a\" o",
			func(r *Runner) { r.Semantics, r.Diagnostics = &sem, opterrDiag() })
		if !strings.Contains(out, axis) {
			t.Fatalf("got %q, want it to carry %q", out, axis)
		}
	})
	for _, tc := range []struct{ name, src string }{
		{"never set", "set -- -z\ngetopts \"a\" o"},
		{"set to one", "OPTERR=1\nset -- -z\ngetopts \"a\" o"},
		{"no bad option at all", "OPTERR=0\nset -- -a\ngetopts \"a\" o\necho \"[$o]\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := opterrSem(Unspecified)
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, opterrDiag() })
			if strings.Contains(out, axis) {
				t.Errorf("got %q, want no question asked", out)
			}
		})
	}
}
