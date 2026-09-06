// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// unquotingEscapes is the one semantic axis these rows reach: what becomes of
// a backslash before a character no `$'…'` escape claims. It is asked because
// `${(QU)v}` uppercases the `x` of `$'\x61'` before the decoding runs, so
// `\X` arrives at a reader that has to answer for it — and the shell with
// the flag drops the backslash.
func unquotingEscapes(r *Runner) {
	r.Semantics.DollarSingleUnknownEscape = DollarSingleUnknownDropsBackslash
}

// The `Q` flag: one level of quoting off the value, and no expansion at all.
//
// Every row is a measurement on zsh 5.9.2. The rows that matter most are the
// ones where quote *removal* and quote *interpretation* part company — `"$x"`
// with `x` set, and `$(echo hi)` — because a reading that reached for the
// parser would answer those with the expansion and be wrong at status 0.
func TestTheUnquotingFlag(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"single quotes go", `v="'a b'"; printf "[%s]" "${(Q)v}"`, "[a b]"},
		{"double quotes too", `v='a="b c"'; printf "[%s]" "${(Q)v}"`, "[a=b c]"},
		{"and several runs in one word", `v="'a'b'c'"; printf "[%s]" "${(Q)v}"`, "[abc]"},
		{"a backslash escapes the next character", `v='a\ b'; printf "[%s]" "${(Q)v}"`, "[a b]"},
		{"a trailing backslash is dropped", `v='a\'; printf "[%s]" "${(Q)v}"`, "[a]"},
		{"an empty value stays empty", `v=""; printf "[%s]" "${(Q)v}"`, "[]"},
		{"and an empty pair of quotes becomes one", `v="''"; printf "[%s]" "${(Q)v}"`, "[]"},
		// It does not expand, which is the whole flag. `x` is set here, so
		// an implementation that parsed the value would answer `hi`.
		{"a parameter in quotes is text", `x=hi; v='"$x"'; printf "[%s]" "${(Q)v}"`, "[$x]"},
		{"a command substitution is text", `v='$(echo hi)'; printf "[%s]" "${(Q)v}"`, "[$(echo hi)]"},
		{"a backquote is text", "v='\"a`b`c\"'; printf \"[%s]\" \"${(Q)v}\"", "[a`b`c]"},
		{"a tilde and a glob are text", `v='~/x *'; printf "[%s]" "${(Q)v}"`, "[~/x *]"},
		// Nor does it split: `${(@Q)v}` on a value with a space in it is
		// still one word.
		{"it makes no fields", `v="'a b' c"; printf "[%s]" "${(@Q)v}"`, "[a b c]"},
		{"and applies to each element of an array", `a=("'x y'" '"z"'); printf "[%s]" "${(@Q)a}"`, "[x y][z]"},
		// Inside double quotes a backslash reaches four characters and a
		// newline, and is left alone before anything else.
		{"a backslash reaches a quote", `v='"a\"b"'; printf "[%s]" "${(Q)v}"`, `[a"b]`},
		{"a dollar", `v='"a\$b"'; printf "[%s]" "${(Q)v}"`, `[a$b]`},
		{"a backquote", "v='\"a\\`b\"'; printf \"[%s]\" \"${(Q)v}\"", "[a`b]"},
		{"and itself", `v='"a\\"'; printf "[%s]" "${(Q)v}"`, `[a\]`},
		{"but not a letter", `v='"a\qb"'; printf "[%s]" "${(Q)v}"`, `[a\qb]`},
		{"a newline pair leaves nothing", "v='\"a\\\nb\"'; printf \"[%s]\" \"${(Q)v}\"", "[ab]"},
		{"and an unescaped newline stays", "v='\"a\nb\"'; printf \"[%s]\" \"${(Q)v}\"", "[a\nb]"},
		// `$'…'` is decoded, by the same reader the construct itself uses.
		{"a dollar-single is decoded", `v="$'a\tb'"; printf "[%s]" "${(Q)v}"`, "[a\tb]"},
		{"and a dollar before a double quote is not a quote at all", `v='$"x"'; printf "[%s]" "${(Q)v}"`, "[$x]"},
		// An opener with no closer comes back as written. Not an error, not
		// a truncation, and not the contents without the opener — which is
		// the shape the first attempt here got wrong, answering
		// `unterm"unterm` because it had already written what it scanned.
		{"an unterminated single quote is left alone", `v="'unterm"; printf "[%s]" "${(Q)v}"`, "['unterm]"},
		{"an unterminated double quote too", `v='"unterm'; printf "[%s]" "${(Q)v}"`, `["unterm]`},
		{"and an unterminated dollar-single", `v="$'unterm"; printf "[%s]" "${(Q)v}"`, "[$'unterm]"},
		// A substitution is a region rather than ordinary text: its quotes
		// neither close the enclosing one nor are removed themselves, and
		// one that never closes takes the enclosing quote with it.
		{"a quote inside a substitution does not close the outer one", `v='"a$(b"c")d"'; printf "[%s]" "${(Q)v}"`, `[a$(b"c")d]`},
		{"nor inside a backquoted run", "v='\"a`b\"c`d\"'; printf \"[%s]\" \"${(Q)v}\"", "[a`b\"c`d]"},
		{"a substitution survives outside quotes as well", `v='a$("b")c'; printf "[%s]" "${(Q)v}"`, `[a$("b")c]`},
		{"nested parentheses are counted", `v='"a$(b(c))d"'; printf "[%s]" "${(Q)v}"`, "[a$(b(c))d]"},
		// Verbatim means the backslashes inside are not touched either,
		// which is the only row that separates "copied through" from "read
		// as ordinary text that happens to have no escapes in it".
		{"and a backslash inside one is left alone", `v='"a$(b\"c)d"'; printf "[%s]" "${(Q)v}"`, `[a$(b\"c)d]`},
		{"an arithmetic substitution is one too", `v='"$((1+2))x"'; printf "[%s]" "${(Q)v}"`, "[$((1+2))x]"},
		{"braces likewise", `v='"a${b{c}}d"'; printf "[%s]" "${(Q)v}"`, "[a${b{c}}d]"},
		{"an unclosed one leaves the whole value", `v='"a$(x b"'; printf "[%s]" "${(Q)v}"`, `["a$(x b"]`},
		// And outside quotes too, where the text after it is what would
		// otherwise go on being unquoted: the `'x'` here stays written.
		{"outside quotes it takes the rest of the value", `v=$'a$(\'x\' b'; printf "[%s]" "${(Q)v}"`, "[a$('x' b]"},
		{"an unclosed brace the same", `v='"a${x b"'; printf "[%s]" "${(Q)v}"`, `["a${x b"]`},
		{"an unclosed backquote the same", "v='\"a\\`b`c\"'; printf \"[%s]\" \"${(Q)v}\"", "[\"a\\`b`c\"]"},
		{"and outside quotes it is only text", "v='a`b'; printf \"[%s]\" \"${(Q)v}\"", "[a`b]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, ordering, unquotingEscapes)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// Where the unquoting step sits, in the three answers that fix it.
//
// It is the second half of rule 14 and it runs *after* the `q` family, not
// before: `${(Qq)v}` and `${(qQ)v}` on `'a b'` are both the value back again,
// where a `Q` that ran first would have left `a\ b`. It is after the split
// and after the length, and before the ordering step — which is the one
// constraint that moved the ordering step to the end of the group.
func TestWhereTheUnquotingStepSits(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"q runs first, so the pair is a round trip", `v="'a b'"; printf "[%s]" "${(Qq)v}"`, "['a b']"},
		{"either way round", `v="'a b'"; printf "[%s]" "${(qQ)v}"`, "['a b']"},
		{"the split has already run", `v="'a,b'"; printf "[%s]" "${(@Qs.,.)v}"`, "['a][b']"},
		{"and the length", `v="'ab'"; printf "[%s]" "${(Q)#v}"`, "[4]"},
		{"the join too", `a=("'b'" "'a'"); printf "[%s]" "${(Qj.-.)a}"`, "[b-a]"},
		// The case conversion runs first, which is measurable only where a
		// letter is part of an escape: `U` uppercases the `x` of `$'\x61'`,
		// and `\X` is then an escape nothing claims.
		{"the case conversion runs first", `v="$'\x61'"; printf "[%s]" "${(QU)v}"`, "[X61]"},
		{"written either way round", `v="$'\x61'"; printf "[%s]" "${(UQ)v}"`, "[X61]"},
		{"and the ordering step runs last", `a=("'z'" b); printf "[%s]" "${(@Qo)a}"`, "[b][z]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, ordering, unquotingEscapes)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
