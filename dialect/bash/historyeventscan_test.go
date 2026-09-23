// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// What starts a history reference, and what a failed modifier is named after.
//
// Every expected string below is a transcript. The file was run through bash
// 5.3.20 on 2026-09-18 with the two streams captured apart, `env -i` and
// `--norc --noprofile`, and what is written here is what came back; the rows
// naming the braced form were run through bash 3.2.57 as well and agree.
//
// Three issues meet here and all three were the same mistake — reading a
// character as punctuation that bash reads as part of an event's name: a
// backslash (#3421), a quote or a backquote (#3421), and a brace (#3220).

// A backslash, a single quote and a backquote after the event character each
// begin a reference here, inside double quotes and outside them alike.
//
// This is the row #3421 is about, and the issue's own premise was wrong about
// half of it: it read the rule as one that held only inside double quotes,
// where re-measuring says `echo T!\xE` is refused unquoted too.
func TestAQuoteOrABackslashAfterTheEventCharacterStartsAReference(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{`echo "!\x"`, `S: line 4: !\x: event not found`},
		{`echo "!'x"`, `S: line 4: !'x: event not found`},
		{`echo T!\xE`, `S: line 4: !\xE: event not found`},
		{`echo T!'xE'`, `S: line 4: !'xE': event not found`},
		{"echo T!`xE`", "S: line 4: !`xE`: event not found"},
		{`echo "T!\x E"`, `S: line 4: !\x: event not found`},
	} {
		t.Run(c.line, func(t *testing.T) {
			out, errs, code := historyRun(t, "set -o history\nset -H\necho a\n"+c.line+"\n")
			if strings.TrimRight(errs, "\n") != c.want || code != 0 {
				t.Errorf("err %q status %d, want %q at 0", errs, code, c.want)
			}
			// The line is dropped, so nothing but the seed reaches
			// standard output.
			if out != "a\n" {
				t.Errorf("out %q, want only the seeded line — the refused one does not run", out)
			}
		})
	}
	// And the characters that really do leave a `!` alone still do.
	out, errs, code := historyRun(t, "set -o history\nset -H\necho a\necho \"! x\"\necho \"!\"\n")
	if out != "a\n! x\n!\n" || errs != "" || code != 0 {
		t.Errorf("out %q err %q status %d, want the two left alone", out, errs, code)
	}
}

// `!{…}` is not a braced reference here: the brace is the first letter of an
// event's name, and the name runs to the end of the word.
//
// This is #3220, where this shell expanded `X!{!!}Y` into the previous command
// and bash refuses every spelling of it. The direction was laxness — a line a
// person expected to stay literal was rewritten — which is the worst shape,
// because the script runs.
func TestABraceAfterTheEventCharacterIsPartOfTheName(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"echo X!{!!}Y", "S: line 4: !{!!}Y: event not found"},
		{"echo X!{!}Y", "S: line 4: !{!}Y: event not found"},
		{"echo X!{echo}Y", "S: line 4: !{echo}Y: event not found"},
		{"echo X!{1}Y", "S: line 4: !{1}Y: event not found"},
		{"echo X!{}Y", "S: line 4: !{}Y: event not found"},
		{`echo "T!{x} E"`, "S: line 4: !{x}: event not found"},
	} {
		t.Run(c.line, func(t *testing.T) {
			out, errs, code := historyRun(t, "set -o history\nset -H\necho abc\n"+c.line+"\n")
			if strings.TrimRight(errs, "\n") != c.want || code != 0 {
				t.Errorf("err %q status %d, want %q at 0", errs, code, c.want)
			}
			if out != "abc\n" {
				t.Errorf("out %q, want only the seeded line", out)
			}
		})
	}
}

// A `G` in front of a substitution substitutes once in each word, and is
// bash's alone.
//
// `foo` is the discriminator: a plain `g` takes its second `o` and a `G` does
// not. The letter also stands alone — a `g` or an `a` beside it is refused,
// and the one named is whichever arrived second.
func TestTheWordwiseSubstitutionModifier(t *testing.T) {
	for _, c := range []struct{ line, wantOut, wantErr string }{
		{"echo !!:Gs/o/0/", "ech0 f0o b0o\n", "echo ech0 f0o b0o\n"},
		{"echo !!:gs/o/0/", "ech0 f00 b00\n", "echo ech0 f00 b00\n"},
		{"echo !!:Gs/o/0/:G&", "ech0 f00 b00\n", "echo ech0 f00 b00\n"},
		{"echo !!:gGs/o/0/", "", "S: line 4: G: unrecognized history modifier\n"},
		{"echo !!:Ggs/o/0/", "", "S: line 4: g: unrecognized history modifier\n"},
	} {
		t.Run(c.line, func(t *testing.T) {
			out, errs, code := historyRun(t, "set -o history\nset -H\necho foo boo\n"+c.line+"\n")
			if out != "foo boo\n"+c.wantOut || errs != c.wantErr || code != 0 {
				t.Errorf("out %q err %q status %d, want %q and %q",
					out, errs, code, "foo boo\n"+c.wantOut, c.wantErr)
			}
		})
	}
	// A `G` the chain ends on leaves nothing to name, which is why the
	// refusal that reaches a person has an empty reference in front of the
	// colon. `!^:G` in the issue is this row.
	_, errs, _ := historyRun(t, "set -o history\nset -H\necho one two\n!^:G\n")
	if want := "S: line 4: : unrecognized history modifier\n"; errs != want {
		t.Errorf("err %q, want %q", errs, want)
	}
	// And an ordinary unknown letter is still named, which is what says the
	// row above is about the `G` being taken rather than about the wording.
	_, errs, _ = historyRun(t, "set -o history\nset -H\necho one two\necho !!:Z\n")
	if want := "S: line 4: Z: unrecognized history modifier\n"; errs != want {
		t.Errorf("err %q, want %q", errs, want)
	}
}

// A failed modifier is named after the whole chain it stands in.
//
// `:s/x/y/` alone was this shell's answer and is nobody's; bash and ksh93 both
// name every modifier from the first colon through the one that failed, and a
// word designator in front of the chain stays out of it.
func TestAFailedModifierNamesTheWholeChain(t *testing.T) {
	for _, c := range []struct{ seed, line, want string }{
		{"echo one/two.one", "!!:t:gs/x/y/", "S: line 4: :t:gs/x/y/: substitution failed"},
		{"echo one two", "!!:q:&", "S: line 4: :q:&: no previous substitution"},
		{"echo one two", "!!:g&", "S: line 4: :g&: no previous substitution"},
		{"echo one two", "echo !!:1:s/x/y/", "S: line 4: :s/x/y/: substitution failed"},
	} {
		t.Run(c.line, func(t *testing.T) {
			_, errs, code := historyRun(t, "set -o history\nset -H\n"+c.seed+"\n"+c.line+"\n")
			if strings.TrimRight(errs, "\n") != c.want || code != 0 {
				t.Errorf("err %q status %d, want %q at 0", errs, code, c.want)
			}
		})
	}
}

// A `$` word designator is the whole designator here: a `-` or a `*` after it
// is text, not a range this shell then refuses.
//
// And the quick-substitution character names word one where a range's **end**
// is written, which ksh93 leaves as text.
func TestTheDesignatorsAtARangeEdge(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"echo !!:$-3", "e-3"},
		{"echo !!:$-", "e-"},
		{"echo !!:$*", "e*"},
		{"echo !!:$x", "ex"},
		{"echo !!:1-^", "a"},
		{"echo !!:^-$", "a b c d e"},
		{"echo !!:1-*", "a b c d*"},
	} {
		t.Run(c.line, func(t *testing.T) {
			// The echoed line rather than what it printed, because two of
			// these rows end in a `*` the shell would then glob against
			// whatever directory the test runs in.
			out, errs, code := historyRun(t, "set -o history\nset -H\necho a b c d e\n"+c.line+"\n")
			if want := "echo " + c.want + "\n"; errs != want || code != 0 {
				t.Errorf("err %q status %d, want %q at 0", errs, code, want)
			}
			if !strings.HasPrefix(out, "a b c d e\n") {
				t.Errorf("out %q, want the seeded line in front of it", out)
			}
		})
	}
	// A range that runs backwards is still refused, and names itself as
	// written — which is what says the `^` was read as word one rather than
	// left as text.
	_, errs, _ := historyRun(t, "set -o history\nset -H\necho a b c d e\necho !!:2-^\n")
	if want := "S: line 4: :2-^: bad word specifier\n"; errs != want {
		t.Errorf("err %q, want %q", errs, want)
	}
}

// Which `"` ends an event's name: the one that **closes** a string open where
// the reference stands, and not a quote reached with nothing open.
//
// #4187, and the transcripts are bash 5.3.20's own, run 2026-09-22 with
// `set -o history; set -H` from a script file. The pair in the first two rows
// is the whole of it — the same two characters, `zz"`, read twice under the
// two quoting states — and the rows after them say it is not a rule about
// `$( )`: there is no substitution in either.
func TestWhichQuoteEndsAnEventName(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{`echo "!zz"`, `S: line 4: !zz: event not found`},
		{`echo "$( echo "!zz" )"`, `S: line 4: !zz": event not found`},
		{`echo $( echo "!zz" )`, `S: line 4: !zz: event not found`},
		{`echo !zz"`, `S: line 4: !zz": event not found`},
		{`echo "a"!zz"b"`, `S: line 4: !zz"b": event not found`},
		{`echo "a" b!zz"`, `S: line 4: !zz": event not found`},
		{`echo "$( echo x"!zz" )"`, `S: line 4: !zz": event not found`},
		// A quote the scan never reaches with the state off still ends the
		// name, which is the control: `!zz` and not `!zz"` here.
		{`echo "a" "!zz"`, `S: line 4: !zz: event not found`},
	} {
		t.Run(c.line, func(t *testing.T) {
			out, errs, code := historyRun(t, "set -o history\nset -H\necho a\n"+c.line+"\n")
			if strings.TrimRight(errs, "\n") != c.want || code != 0 {
				t.Errorf("err %q status %d, want %q at 0", errs, code, c.want)
			}
			if out != "a\n" {
				t.Errorf("out %q, want only the seeded line — the refused one does not run", out)
			}
		})
	}
	// And the name is what was *looked up* rather than only what is blamed.
	// The pair: `!ec` finds the seeded `echo aseed` inside the quotes and
	// finds nothing once the state has been toggled off, because the name
	// searched for was `ec"`. Both transcripts are bash 5.3.20's.
	out, errs, code := historyRun(t, "set -o history\nset -H\necho aseed\necho \"!ec\"\n")
	// The expanded line is echoed on standard *error*, which is where that
	// shell puts it — see docs/spec/history.md — so the pair is read off both
	// streams rather than one.
	if out != "aseed\necho aseed\n" || strings.TrimRight(errs, "\n") != `echo "echo aseed"` || code != 0 {
		t.Errorf("out %q err %q status %d, want the reference to expand", out, errs, code)
	}
	out, errs, code = historyRun(t, "set -o history\nset -H\necho aseed\necho \"$( echo \"!ec\" )\"\n")
	if want := `S: line 4: !ec": event not found`; strings.TrimRight(errs, "\n") != want || out != "aseed\n" || code != 0 {
		t.Errorf("out %q err %q status %d, want %q at 0", out, errs, code, want)
	}
}
