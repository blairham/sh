// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `log`, measured 2026-09-12 against zsh 5.9.2 under `-f` (#2325).

// The line that put this builtin in the table: macOS's `/etc/zshrc` writes
// `disable log` to keep the builtin out of the way of `/usr/bin/log`, and a
// shell reading the system-wide files meets it before the first prompt. With
// no `log` in the table that line was `no such hash table element` at 1.
func TestTheNameIsInTheTableForDisableToReach(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "whence -w log\ndisable log\necho st=$?\nwhence -w log\n")
	// Not `log: command` on the second line, which is what the machine this
	// was measured on answers: a scratch PATH need hold no `log` at all, so
	// what this pins is that the name left the builtin table rather than
	// what it fell through to.
	if !strings.HasPrefix(out, "log: builtin\nst=0\n") {
		t.Errorf("output = %q, want the builtin there and `disable log` at 0", out)
	}
	if strings.Count(out, "log: builtin") != 1 {
		t.Errorf("output = %q, want the name gone from the builtin table after", out)
	}
}

// Three of the measured shapes are a builtin that takes no operands and
// reports nothing, which is what an unset `$watch` — the default — means.
func TestLogWithNothingWatchedIsSilentAndZero(t *testing.T) {
	for _, script := range []string{"log\n", "watch=()\nlog\n", "typeset -a watch\nlog\n"} {
		out, st := runZsh(t, t.TempDir(), script+"echo st=$?\n")
		if out != "st=0\n" || st != 0 {
			t.Errorf("%q = %q (status %d), want nothing and 0", script, out, st)
		}
	}
}

// It takes no operands and no options at all: an option-shaped word and an
// empty word are both `too many arguments`, which is the same sentence a
// second name gets.
func TestLogTakesNoOperandsAndNoOptions(t *testing.T) {
	for _, word := range []string{"a b", "-x", `""`, "--"} {
		out, st := runZsh(t, t.TempDir(), "log "+word+"\necho st=$?\n")
		if !strings.Contains(out, ":log:1: too many arguments") || !strings.Contains(out, "st=1\n") {
			t.Errorf("log %s = %q (status %d), want too many arguments at 1", word, out, st)
		}
	}
}

// And the feature is refused by name rather than answered with the silence
// that means "nobody is logged on". A script that set `$watch` and got
// nothing back would read the refusal as an empty login table, which is the
// silent wrong answer dialect/zsh/enable.go already refuses to give for a
// hash table this shell does not keep.
func TestLogRefusesTheWatchedUsersByNameRatherThanAnsweringNothing(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "watch=(all)\nlog\necho st=$?\n")
	if !strings.Contains(out, "reporting the users $watch names is not implemented yet") {
		t.Errorf("output = %q (status %d), want the facility named as missing", out, st)
	}
	if strings.Contains(out, "st=0\n") {
		t.Errorf("output = %q, want a nonzero status from a refusal", out)
	}
}
