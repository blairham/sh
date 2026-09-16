// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `--name` is a second spelling for the whole `set -o` namespace, at the
// invocation and at the builtin alike.
//
// ksh93's own manual page states the rule on the machine these were measured
// on: "Options -o name can also be specified with --name and +o name can be
// specifed with --noname except that options names beginning with no are
// turned on by omitting no." Measured against 93u+ 2012-08-01 on 2026-09-16,
// what that comes to is three readings tried in order — an `=value`, then the
// whole word as a name, then the word with a leading `no` taken off. See
// Semantics.LongOptionNamesASetOption.
//
// The spelling is not a shorthand for a handful of long options: every name
// the shell has is reachable by it, so the roster is the `-o` roster and a
// name absent from one is absent from both.

// setLongOption applies one `--name` word at an invocation and reports what
// reached standard error, which is the route `ksh --xtrace script` takes.
func setLongOption(t *testing.T, word string) (string, int) {
	t.Helper()
	var errs strings.Builder
	r := preset.Runner(dialecttest.Base{
		Stdout: &strings.Builder{}, Stderr: &errs, Name: "/bin/ksh",
	})
	// The status first: the two operands of a return are evaluated left to
	// right, so reading the buffer in the same statement reads it empty.
	st := r.SetLongOption(word)
	return errs.String(), st
}

// The whole word is tried as a name before the `no` comes off, which the
// roster forces rather than the manual: `notify` and `noglob` are both option
// names in their own right, so a rule that stripped first would read
// `--notify` as notify *off*.
func TestALongOptionTriesTheWholeWordBeforeItStrips(t *testing.T) {
	for _, c := range []struct{ word, look, want string }{
		{"noglob", "glob", "glob                     off"},
		{"glob", "glob", "glob                     on"},
		{"noclobber", "clobber", "clobber                  off"},
		{"nounset", "unset", "unset                    off"},
		{"noallexport", "allexport", "allexport                off"},
		{"allexport", "allexport", "allexport                on"},
		{"xtrace", "xtrace", "xtrace                   on"},
		{"noxtrace", "xtrace", "xtrace                   off"},
	} {
		out, st := answersRun(t, "set --"+c.word+"\nset -o | grep -w "+c.look+"\n")
		if st != 0 || !strings.Contains(out, c.want) {
			t.Errorf("set --%s said %q at %d, want a line %q", c.word, out, st, c.want)
		}
	}
}

// The `=value` an AST long option may carry is a *number*, and nonzero is the
// option on. `--noglob=on` leaving globbing alone is the row that says so:
// under any boolean reading of the word it would be noglob on.
func TestALongOptionValueIsANumber(t *testing.T) {
	for _, c := range []struct {
		value string
		off   bool
	}{
		{"1", true},
		{"2", true},
		{"0", false},
		{"off", false},
		{"on", false},
		{"true", false},
		{"xyz", false},
		{"", false},
	} {
		out, st := answersRun(t, "set --noglob="+c.value+"\nset -o | grep -w glob\n")
		want := "glob                     on"
		if c.off {
			want = "glob                     off"
		}
		if st != 0 || !strings.Contains(out, want) {
			t.Errorf("set --noglob=%q said %q at %d, want %q", c.value, out, st, want)
		}
	}
}

// A word none of the three readings resolves is named back as it was written.
// `--noprofile` is `noprofile: bad option(s)` and not `profile:` — the reader
// is shown the word they typed rather than the one the shell was left holding
// after a strip that did not help.
func TestARefusedLongOptionNamesTheWordAsWritten(t *testing.T) {
	const usage = "Usage: ksh [ options ] [arg ...]\n"
	for _, word := range []string{"zzznosuch", "noprofile", "no", "tify"} {
		got, st := setLongOption(t, word)
		if want := "ksh: " + word + ": bad option(s)\n" + usage; got != want {
			t.Errorf("--%s said %q, want %q", word, got, want)
		}
		if st != 2 {
			t.Errorf("--%s exited %d, want 2", word, st)
		}
	}
}

// And the block under it is this spelling's own. The letter and the `-o` name
// get the one that lists the letters; a `--name` gets the one that does not,
// because the spelling that was refused has none.
func TestTheLongSpellingPicksItsOwnUsageBlock(t *testing.T) {
	const letters = "Usage: ksh [-cilrsDEabefhkmnprtuvxBCGH] [-R file] [-o[option]] [arg ...]\n"
	var errs strings.Builder
	r := preset.Runner(dialecttest.Base{
		Stdout: &strings.Builder{}, Stderr: &errs, Name: "/bin/ksh",
	})
	r.SetNamedOption("zzznosuch", true)
	if want := "ksh: zzznosuch: bad option(s)\n" + letters; errs.String() != want {
		t.Errorf("-o zzznosuch said %q, want %q", errs.String(), want)
	}
	// The builtin's two blocks, which differ the same way.
	out, _ := answersRun(t, "set -o zzznosuch\n")
	if want := "Usage: set [-sabefhkmnprtuvxBCGH] [-A name] [-o[option]] [arg ...]"; !strings.Contains(out, want) {
		t.Errorf("set -o zzznosuch said %q, want a %q line", out, want)
	}
	out, _ = answersRun(t, "set --zzznosuch\n")
	if want := "Usage: set [--default] [--state] [arg ...]"; !strings.Contains(out, want) {
		t.Errorf("set --zzznosuch said %q, want a %q line", out, want)
	}
}

// `--` is still the terminator and not a name of nothing, and a word behind it
// is a positional parameter however much it looks like an option.
func TestTwoDashesAloneStillEndTheOptions(t *testing.T) {
	out, st := answersRun(t, "set -- --xtrace\nprintf '[%s]\\n' \"$1\"\n")
	if st != 0 || out != "[--xtrace]\n" {
		t.Errorf("set -- --xtrace said %q at %d, want %q", out, st, "[--xtrace]\n")
	}
}
