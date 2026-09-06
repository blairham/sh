// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// This is the only shell in the panel that quotes the offending text back for
// a construct the input ran out inside, so it is the only one that can say
// what the text is. It is the **word** the construct was written in — not the
// construct, and not the command.
//
// The distinction matters because the two readings agree on every shape #1022
// measured. `v=$(echo hi` is one word *and* the whole of one command, so it
// cannot tell them apart; the rows below can, and they say word:
//
//	echo a$(echo hi          a$(echo hi          not `echo a$(echo hi`
//	echo one two $(echo hi   $(echo hi           not the two words before it
//	cat <(echo hi            <(echo hi           the redirection ended a word
//	echo x >f$(echo hi       f$(echo hi          and so did the operator
//
// And the word is not something the source can be searched backwards for,
// which is the second reason it comes from the scanner: a blank inside quotes
// or behind a backslash does not end a word, so `echo "a b"$(echo hi` names
// all of `"a b"$(echo hi` and a scan back to the previous space would cut it
// in half.
//
// Whole rendered lines with their locations, because the line was already
// right and a substring assertion could not tell a widened text from a
// prefixed one.
func TestAnUnmatchedConstructIsQuotedBackWithItsWholeWord(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		line      int
	}{
		{"v=$(echo hi\necho after\n", "parse error near `v=$(echo hi'", 3},
		{"echo a$(echo hi\necho after\n", "parse error near `a$(echo hi'", 3},
		{"echo one two $(echo hi\necho after\n", "parse error near `$(echo hi'", 3},
		{"echo one; v=x$(echo hi\necho after\n", "parse error near `v=x$(echo hi'", 3},
		{"echo \"a b\"$(echo hi\necho after\n", "parse error near `\"a b\"$(echo hi'", 3},
		{"echo a\\ b$(echo hi\necho after\n", "parse error near `a\\ b$(echo hi'", 3},
		{"echo x >f$(echo hi\necho after\n", "parse error near `f$(echo hi'", 3},
		// A construct inside a construct: the outer one is what ran out, and
		// its word is what is quoted.
		{"echo $(echo $(echo hi\necho after\n", "parse error near `$(echo $(echo hi'", 3},
		// The word ends at the line, which it always did.
		{"v=$(cat <<EOF\na\nEOF)\necho after\n", "parse error near `v=$(cat <<EOF'", 5},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		d := zsh.Diagnostics()
		if got := d.ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
		if got := d.ParseFailureLine(err); got != tc.line {
			t.Errorf("%q: blamed line %d, want %d", tc.src, got, tc.line)
		}
	}
}
