// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A function definition's name here is decided by how the word was
// **written**: bare and it is a name whatever is in it, quoted or expanded
// and the definition is read and binds nothing, silently, at status 0.
//
// The seventh column's own answer and no other's, which is what #2590 is: the
// three bash columns and ksh93 refuse the *name* where the definition runs,
// dash refuses to parse it, zsh defines what the word comes to, and this
// shell does none of those. Measured 2026-09-13 against BusyBox v1.37.0
// through the container route the oracle reaches this shell by; the flag is
// syntax.Dialect.FunctionNameIsAnyBareWord and the run-time answer is
// interp.FuncNameDefinesNothing.
//
// Asserted here and not in `interp` because it is what *this shell* answers:
// the substrate's tests name the flag and the axis and run them at every
// value.

// A word written bare is a name, and the characters do not come into it. The
// three a pattern is made of are the row that matters: zsh matches such a
// word against the filesystem and defines nothing, and this shell defines it
// and calls it.
//
// `PATH` is emptied here as well as below, and it strengthens the row rather
// than only tidying it: with nothing on the path, `hi` can only have come
// from a definition, so a run where the definition silently failed cannot be
// rescued by a program of that name.
func TestABareNameIsANameWhateverIsInIt(t *testing.T) {
	for _, tc := range []struct{ src, why string }{
		{`a.b() { echo hi; }; a.b`, "the punctuated name the dialect already recorded"},
		{`a*b() { echo hi; }; a*b`, "no filesystem match here, where zsh has one"},
		{`a?b() { echo hi; }; a?b`, "the same for the single-character pattern"},
		{`a[b() { echo hi; }; a[b`, "and for the bracket, which zsh calls a bad pattern"},
		{`a{b() { echo hi; }; a{b`, "a brace is an ordinary character in a bare word here"},
		{`a~b() { echo hi; }; a~b`, "and so is a tilde"},
		{`function a.b { echo hi; }; a.b`, "the keyword spelling answers alike"},
		{`function a*b { echo hi; }; a*b`, "which is why one flag covers both forms"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := run(t, "PATH=; "+tc.src)
			if strings.TrimSpace(out) != "hi" || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0 — %s", tc.src, out, st, "hi", tc.why)
			}
		})
	}
}

// And a word written any other way defines nothing at all. `f` is the control
// — a name nobody could object to — so what stops the definition is the
// quoting and not the characters.
//
// Every row empties `PATH` first, and that is not tidiness: what is being
// asked is whether the *function table* holds the name, and a call that finds
// no function falls through to the filesystem. `a"b"()` names `ab`, and `ab`
// is Apache Bench on the Linux runner — so this suite passed on a Mac and
// came back `wrong number of arguments` at 22 in CI, from a shell that had
// behaved exactly as intended. With no `PATH` the only thing that can answer
// the call is a definition.
func TestANameNotWrittenBareDefinesNothing(t *testing.T) {
	for _, tc := range []struct{ src, why string }{
		{`'f'() { echo p; }; f; echo st=$?`, "the sharpest control: the name is `f`"},
		{`"f"() { echo p; }; f; echo st=$?`, "either spelling of the quotes"},
		{`\f() { echo p; }; f; echo st=$?`, "one backslash over an ordinary letter is enough"},
		{`a"b"() { echo p; }; ab; echo st=$?`, "quoting part of the word is quoting the word"},
		{`a\ b() { echo b; }; 'a b'; echo st=$?`, "the row #2590 measured for the POSIX form"},
		{`w=foo; _p_${w}() { echo HI; }; _p_foo; echo st=$?`, "an expansion is not written bare either"},
		{`function 'f' { echo p; }; f; echo st=$?`, "the row #2590 measured for the keyword form"},
		{`function '@#%' { echo p; }; '@#%'; echo st=$?`, "punctuation that is bare-legal, quoted"},
		{`function a\*b { echo p; }; a*b; echo st=$?`, "and a pattern character escaped rather than quoted"},
		{`function $(echo n) { echo p; }; n; echo st=$?`, "a substitution in the name"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := run(t, "PATH=; "+tc.src)
			if !strings.Contains(out, "st=127") || st != 0 {
				t.Errorf("%s gave %q at %d, want the call not found at 127 and the script at 0 — %s",
					tc.src, out, st, tc.why)
			}
			if strings.Contains(out, "\np\n") || strings.HasPrefix(out, "p\n") {
				t.Errorf("%s gave %q; the body must never run", tc.src, out)
			}
			if strings.Contains(out, "invalid") || strings.Contains(out, "not a valid") {
				t.Errorf("%s gave %q; this shell says nothing about the name", tc.src, out)
			}
		})
	}
}

// The empty name is the same rule and not a special case, and it is the one
// whose *definition* can be watched rather than the call: the script simply
// carries on.
func TestAnEmptyNameIsADefinitionOfNothing(t *testing.T) {
	for _, src := range []string{`function '' { echo b; }; echo after=$?`, `''() { echo b; }; echo after=$?`} {
		out, st := run(t, src)
		if strings.TrimSpace(out) != "after=0" || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0", src, out, st, "after=0")
		}
	}
}

// Nothing is bound, rather than something bound under another name — which no
// call site could show on its own, since a call under the wrong name is `not
// found` either way.
//
// Three probes, because each one alone leaves a hiding place: `command -v`
// asks after the word's text, the same question asked of its *source* text
// covers a reading that kept the quotes in the name, and a definition already
// standing under that name covers a table entry being replaced by nothing.
//
// `PATH` is emptied here for the reason the row above records: `command -v`
// answers 0 for a program as readily as for a function, so a machine with a
// `g` on it would pass the first probe while saying nothing about the table.
func TestASilentlyRefusedDefinitionLeavesTheTableAlone(t *testing.T) {
	for _, tc := range []struct{ src, want, why string }{
		{`'g'() { echo x; }; command -v g; echo cv=$?`, "cv=127", "not under the word's text"},
		{`'g'() { echo x; }; command -v "'g'"; echo cv=$?`, "cv=127", "nor under the source text"},
		{`g() { echo old; }; 'g'() { echo new; }; g`, "old", "and an earlier definition still stands"},
	} {
		t.Run(tc.why, func(t *testing.T) {
			out, st := run(t, "PATH=; "+tc.src)
			if !strings.Contains(out, tc.want) || st != 0 {
				t.Errorf("%s gave %q at %d, want %q — %s", tc.src, out, st, tc.want, tc.why)
			}
		})
	}
}

// The body is read whole, which is what makes this a definition the grammar
// takes rather than a line it passes over. A shell that skipped the line
// would say nothing here.
//
// Through `eval`, because a snippet the *parser* refuses never reaches the
// runner at all and the helper above has nowhere to report one. Measured that
// way too: `eval "\'h\'() { if; }"` is `eval: line 1: syntax error:
// unexpected ";"` in the pinned image.
func TestABodyUnderARefusedNameIsStillRead(t *testing.T) {
	out, _ := run(t, `eval "'h'() { if; }"; echo after=$?`)
	if !strings.Contains(out, "syntax error") {
		t.Errorf("gave %q, want the broken body refused", out)
	}
}

// An assignment is still an assignment, and lexically: `a=()` is a syntax
// error here — this shell has no array literal — and a quoted `=` is an
// ordinary character of a name that binds nothing.
//
// The append spelling is the row that says the reading is this shell's own
// and not a borrowed one: `a+=(2)` has no `+=` to be an assignment, so the
// word before the parenthesis is a name and the shell reads a definition,
// then wants the `)` the `2` is standing where. That is the corpus's
// `core/appending-an-array-literal-to-a-scalar` and the four rows beside it,
// and it is what this change moved them onto.
//
// The two refusals go through `eval` for the reason the row above does.
func TestAnAssignmentIsStillAnAssignment(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`eval "a=() { echo x; }"; echo after=$?`, "syntax error"},
		{`eval "a=1; a+=(2)"; echo after=$?`, "syntax error"},
		{`'a=b'() { echo x; }; echo after=$?`, "after=0"},
	} {
		out, _ := run(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s gave %q, want it to contain %q", tc.src, out, tc.want)
		}
	}
}
