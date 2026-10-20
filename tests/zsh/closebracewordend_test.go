// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// `ignoreclosebraces` takes the **reserved** reading of `}` away and leaves
// the **lexical** one: the brace still ends the word it ends, and is then an
// ordinary word rather than a group's closer.
//
// Refusal alone cannot see that half — `echo A}` takes under either name —
// which is why #4988's nine rows, all of them graded on whether a line
// parses, were right and still left this standing. What separates the two
// names here is how many words reach the command.
//
// Every `want` is the reference's own answer, measured 2026-09-28 on
// `/opt/homebrew/bin/zsh` — `zsh 5.9.2 (aarch64-apple-darwin25.4.0)`,
// `go version -m` says *not a Go executable* for it — run `-f` over a script
// file under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, the
// names moved on the line above the one being read (#5011).
func TestTheClosingNameLeavesTheBraceEndingTheWord(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts string
		src  string
		want string
	}{
		// **The discriminator.** One word under the name that makes the
		// brace an ordinary character, two under the name that only takes
		// the reserved reading, and refused under neither.
		{"neither", "", "print a}\n", ""},
		{"ignorebraces", "setopt ignorebraces\n", "print a}\n", "a}\n"},
		{"ignoreclosebraces", "setopt ignoreclosebraces\n", "print a}\n", "a }\n"},
		{"both", "setopt ignorebraces ignoreclosebraces\n", "print a}\n", "a}\n"},
		// The controls. A `}` that does not end the word is never the
		// reserved word and never splits, under any of the four states, so a
		// wiring that had made the brace a terminator everywhere would show
		// here and in none of the rows above.
		{"neither, mid-word", "", "print a}b\n", "a}b\n"},
		{"ignoreclosebraces, mid-word", "setopt ignoreclosebraces\n", "print a}b\n", "a}b\n"},
		{"neither, word-initial", "", "print }a\n", "}a\n"},
		{"ignoreclosebraces, word-initial", "setopt ignoreclosebraces\n", "print }a\n", "}a\n"},
		// An assignment's value is the carve-out the reserved reading
		// already had, and the lexical one has it too.
		{"ignoreclosebraces, an assignment's value", "setopt ignoreclosebraces\n", "x=a}\nprint $x\n", "a}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := zshScriptFileAt(t, "setopt noaliases\n"+tc.opts+tc.src)
			var out, errs bytes.Buffer
			code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-f", script})
			if tc.want == "" {
				if code == 0 || errs.Len() == 0 {
					t.Fatalf("ran %q at %d, want the refusal the reference gives", out.String(), code)
				}
				return
			}
			if out.String() != tc.want || code != 0 {
				t.Errorf("wrote %q at %d (said %q), want %q",
					out.String(), code, errs.String(), tc.want)
			}
		})
	}
}

// TestAnAliasEndingAtACloseBraceStillExpands is the reference's own
// D08cmdsubst asking the same question, and the reason the row was found: an
// alias is looked up by the word, so a `}` that stops ending the word takes
// the alias with it.
//
// `alias CLOSE='};'` inside `$({ OPEN print bye; CLOSE})` under
// `ignoreclosebraces` is `bye` in the reference and was `parse error near
// `)'` here — the substitution running out of text because `CLOSE}` had been
// read as one word that names no alias. Measured the same way, 2026-09-28.
func TestAnAliasEndingAtACloseBraceStillExpands(t *testing.T) {
	const src = "(\n" +
		"  setopt ignoreclosebraces\n" +
		"  alias OPEN='{' CLOSE='};'\n" +
		"  eval '{ OPEN print hi; CLOSE }\n" +
		"  var=$({ OPEN print bye; CLOSE}) && print $var'\n" +
		")\n"
	script := zshScriptFileAt(t, src)
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-f", script})
	if out.String() != "hi\nbye\n" || code != 0 {
		t.Errorf("wrote %q at %d (said %q), want hi then bye",
			out.String(), code, errs.String())
	}
}
