// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// nameless is the grammar these expansions need, named by the construct
// rather than by a shell.
func nameless(d *syntax.Dialect) {
	d.NamelessParamExpansion = true
	d.ParamExpansionFlags = true
	d.NestedParamExpansion = true
	d.ParamLengthTakesAnOperator = true
	d.ParamAssignAlways = true
}

// An expansion with no parameter name at all.
//
// The name that is not there is never set and never non-empty, so the two
// conditionals answer from that and not from a value: `:-` fires every time
// and `:+` never does. Measured on zsh 5.9.2, the only shell in the panel
// with the form — the other five call every line here a bad substitution.
//
// The empty rows carry as much as the others. `${}`, `${:-}` and `${%x}` are
// all the empty string, so an implementation that read them as *nothing at
// all* — dropping the word rather than substituting an empty one — looks
// identical until a field count is asked for, which is what the last row
// does.
func TestAnExpansionWithNoNameAtAll(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a default over the name that is not there", `printf "[%s]" "${:-abc}"`, "[abc]"},
		{"and its alternate never fires", `printf "[%s]" "${:+abc}"`, "[]"},
		{"nothing between the braces", `printf "[%s]" "${}"`, "[]"},
		{"an empty operand", `printf "[%s]" "${:-}"`, "[]"},
		{"a trim over nothing", `printf "[%s]" "${%x}" "${%%x}"`, "[][]"},
		// The operand is an ordinary word rather than a literal: an
		// expansion inside it expands, and a nameless expansion nests inside
		// a nameless expansion.
		{"the operand is a word", `v=q; printf "[%s]" "${:-x${v}y}"`, "[xqy]"},
		{"and it nests", `printf "[%s]" "${:-${:-a}}"`, "[a]"},
		// A flag group in front of it renders the result and does not decide
		// the reading — which is the bug this closed, where the group was
		// the only thing letting the empty name through.
		{"a flag group renders the result", `printf "[%s]" "${(U):-abc}" "${:-abc}"`, "[ABC][abc]"},
		{"and it still splits", `printf "[%s]" ${(s.,.):-a,b}`, "[a][b]"},
		// One empty field rather than no field, which the count is the only
		// way to ask.
		{"an empty result is still a word", `set -- "${:-}" "${}"; printf "[%s]" "$#"`, "[2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, nameless, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A character that is a parameter in its own right is read as one, and the
// nameless reading never competes for it.
//
// This is the trap the form sets: `${-x}` is two characters away from
// `${:-x}` and is nothing like it. `-` is taken as the name `$-` before the
// operator scan runs, so `x` is a stray word after a complete name — and
// every shell in the panel refuses, the one with the nameless grammar
// included.
//
// Asserted against `$-`'s own contents rather than by whether anything
// errored, because a probe that only asks *whether* it fails cannot tell the
// two readings apart: they both fail. `${-}` is where they differ, and it is
// silent — the parameter reading answers the option letters and the nameless
// one answers the empty string, both at status 0.
func TestACharacterThatIsAParameterIsNotAMissingName(t *testing.T) {
	out, st := runGrammar(t, `set -u; case "${-}" in *u*) echo the-parameter;; *) echo a-missing-name;; esac`, nameless, nil)
	if out != "the-parameter\n" || st != 0 {
		t.Errorf("got %q (status %d), want the option letters at 0", out, st)
	}
	for _, src := range []string{`echo "[${-x}]"`, `echo "[${?x}]"`} {
		out, st := runGrammar(t, src+`; echo AFTER`, nameless, nil)
		if st == 0 || strings.Contains(out, "AFTER") {
			t.Errorf("%s: got %q (status %d), want a refusal", src, out, st)
		}
	}
}

// The two operators that cannot answer from a word.
//
// `:-` substitutes an operand and needs no parameter; `:=` has to *store* it
// and there is no parameter to store it in, so the shell with the form says
// `not an identifier: ` with the name left blank and ends the script — the
// same refusal, through the same door, that its unconditional `${::=word}`
// gives. `:?` is the other side of the same fact: the name that is not there
// is unset, so the report fires every time, and it names the parameter before
// the word with the parameter left empty.
//
// Both are fatal, and that is the assertion that matters: an implementation
// letting the assignment through answers `abc` at status 0 and carries on,
// which is a plausible value where the shell stopped.
func TestTheNameThatIsNotThereCannotBeAssignedToOrBeSet(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the conditional assignment", `echo "[${:=abc}]"`, "not an identifier: "},
		{"the unconditional one", `echo "[${::=abc}]"`, "not an identifier: "},
		{"the report", `echo "[${:?abc}]"`, ": abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src+`; echo AFTER`, nameless, nil)
			if !strings.Contains(out, tc.want) {
				t.Errorf("output = %q, want %q in it", out, tc.want)
			}
			if strings.Contains(out, "AFTER") {
				t.Errorf("output = %q, want the script ended there", out)
			}
			if st == 0 {
				t.Errorf("status 0, want the refusal to be fatal")
			}
		})
	}
}

// A length whose operand is a nameless expansion, which is the one place the
// two readings of a leading `#` are separated by nothing else.
//
// With the nameless form in the grammar the `#` is the length prefix and
// `${#:-word}` measures `word`; without it the `#` is the parameter `$#`,
// whose default never fires. Measured 2026-09-08 with `set -- p q`: 4 in the
// shell with the form and 2 in the five without it.
//
// The remaining rows are the ones that do *not* move, and they are why the
// exception is written as `:-` rather than as a rule about the colon: an
// implementation that read every `:` operator the new way would answer the
// empty string for `${#:+w}` and 0 for the two after it, all at status 0.
func TestALengthOverANamelessExpansion(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the operand is what is measured", `set -- p q; printf "[%s]" "${#:-word}"`, "[4]"},
		{"and an empty one measures zero", `set -- p q; printf "[%s]" "${#:-}"`, "[0]"},
		{"the alternate stays the parameter's", `set -- p q; printf "[%s]" "${#:+w}"`, "[w]"},
		{"so does the assignment", `set -- p q; printf "[%s]" "${#:=w}"`, "[2]"},
		{"and the report", `set -- p q; printf "[%s]" "${#:?w}"`, "[2]"},
		{"and the bare count", `set -- p q; printf "[%s]" "${#}"`, "[2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, nameless, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// Without the flag the same characters are unreadable, which is what makes it
// a grammar flag rather than a value: there is no parameter at the front of
// `${:-abc}` for a semantics axis to switch between two readings of.
//
// `${#:-w}` is the exception and is here for it: it reads *both* ways and the
// answers differ, so the flag is what decides which — the parameter `$#` with
// a default that never fires, and 2.
func TestWithoutTheFlagANamelessExpansionIsUnreadable(t *testing.T) {
	for _, src := range []string{`echo "${:-abc}"`, `echo "${}"`, `echo "${%x}"`, `echo "${:=abc}"`} {
		out, st := runGrammar(t, src+`; echo AFTER`, nil, nil)
		if st == 0 || strings.Contains(out, "AFTER") {
			t.Errorf("%s: got %q (status %d), want a refusal", src, out, st)
		}
	}
	out, st := runGrammar(t, `set -- p q; printf "[%s]" "${#:-word}"`, nil, nil)
	if out != "[2]" || st != 0 {
		t.Errorf("got %q (status %d), want the parameter at 0", out, st)
	}
}
