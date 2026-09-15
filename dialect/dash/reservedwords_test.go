// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// `command -v` answers for the grammar this shell has, and this shell has
// four constructs fewer than the other three.
//
// The lookup used to hold a written-out list of reserved words, which was the
// union of every dialect's — so this shell answered `yes, a word of the
// grammar` for `[[`, `]]`, `select` and `function`, and then refused to run
// any of them. Measured on dash 0.5.12, 2026-09-15: each is 127 with nothing
// printed, which is what makes `command -v '[['` the portable test for the
// construct (#2918).
func TestCommandVDoesNotFindWordsThisShellHasNot(t *testing.T) {
	for _, word := range []string{"[[", "]]", "select", "function", "coproc"} {
		out, _ := answersRun(t, `command -v `+quoteOperand(word)+`; echo "st=$?"`)
		if !strings.Contains(out, "st=127") {
			t.Errorf("command -v %q gave %q, want 127: this shell has no such construct", word, out)
		}
		if strings.Contains(out, word+"\n") {
			t.Errorf("command -v %q printed the word back in %q, want silence", word, out)
		}
	}
}

// And the words it *does* reserve are still words, which is what says the
// four above are about the constructs rather than about the builtin having
// stopped answering for the grammar at all.
func TestCommandVStillFindsTheWordsThisShellHas(t *testing.T) {
	for _, word := range []string{
		"if", "then", "else", "elif", "fi", "for",
		"while", "until", "do", "done", "case", "in", "esac", "{", "}", "!",
	} {
		out, st := answersRun(t, `command -v `+quoteOperand(word))
		if st != 0 || strings.TrimSpace(out) != word {
			t.Errorf("command -v %q gave %q at %d, want the word back at 0", word, out, st)
		}
	}
}

// `time` is the one with teeth. It is a keyword in the other three, so a
// report naming the word is a report a script can use; here it is not a
// keyword at all, and `timer=$(command -v time); "$timer" …` is the reason
// the answer has to be a path. Measured: dash answers `/usr/bin/time`.
//
// The assertion is "never the bare word" rather than a path, because whether
// the machine has the program is the machine's business — it is there on this
// one and absent on some Linux images, and the bug is the same either way.
func TestCommandVAnswersTimeWithAFileOrWithNothing(t *testing.T) {
	out, _ := answersRun(t, `command -v time`)
	got := strings.TrimSpace(out)
	if got == "time" {
		t.Fatalf("command -v time = %q, want a path or nothing: this shell has no `time` keyword", got)
	}
	if got != "" && !strings.HasPrefix(got, "/") {
		t.Errorf("command -v time = %q, want a path", got)
	}
	// And the same question through `type`, which shares the lookup.
	sentence, _ := answersRun(t, `type time`)
	if strings.Contains(sentence, "keyword") {
		t.Errorf("type time said %q, want it named as a file or not found", sentence)
	}
}

// quoteOperand wraps a word in single quotes so that the operand reaches
// `command` as itself rather than as grammar — `{`, `}` and `!` are words the
// grammar claims, and an unquoted one would be read rather than passed on.
func quoteOperand(word string) string { return "'" + word + "'" }
