// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `shopt -s assoc_expand_once` moves where a declaration operand's subscript
// is taken to **end**, and it moves it in both directions.
//
// Two spellings that are one string by the time the builtin sees them are not
// one declaration: `declare m['foo[bar']=v` and `declare m[foo[bar]=v` both
// arrive as `m[foo[bar]=v`, and with the option set the reference takes the
// first and refuses the second. The quoting is the only thing that tells them
// apart and it is gone from the string, so the fact is carried from the word
// — see interp.Runner.sourceClosedSubscriptOperands.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over a script file with standard input on the null device, against bash
// 5.3.20 and bash 5.3.15 in the digest-pinned image the suite is graded in,
// which agree. With a table declared in front of each line:
//
//	                     option unset            option set
//	m['foo[bar']=v       not a valid identifier  the key `foo[bar`
//	m['a[b][c]']=v       the key `a[b][c]`       not a valid identifier
//
// The two columns **swap** on those rows, which is what rules out a rule that
// only loosens or only tightens. Unset, the subscript is found by counting
// brackets in the text the quoting came off of; set, the source's own closer
// is taken and the key is whatever stands before the **first** `]`.
//
// This is the line `assoc9.sub` reaches, located 2026-09-23 with a `DEBUG`
// trap carried into the file's children by `BASH_ENV` and then reproduced
// from the panel rather than from the file (#4179).
func TestTheOptionMovesWhereADeclarationOperandsSubscriptEnds(t *testing.T) {
	for _, tc := range []struct{ name, operand, set, unset string }{
		// The row the suite reaches, and its three spellings: the quoting
		// construct does not matter, only that the `[` is not the source's.
		{"a quoted bracket", `m['foo[bar']=v`, `["foo[bar"]="v"`, refused},
		{"a double-quoted bracket", `m["foo[bar"]=v`, `["foo[bar"]="v"`, refused},
		{"an escaped bracket", `m[foo\[bar]=v`, `["foo[bar"]="v"`, refused},
		// And the same text with the bracket written bare, which is refused
		// under both — the control that says the quoting is what moved.
		{"an unquoted bracket", `m[foo[bar]=v`, refused, refused},
		// The rows that swap the other way: balanced in the quote-removed
		// text and so a key with the option unset, past the first `]` and so
		// refused with it set.
		{"a balanced pair inside", `m['a[b][c]']=v`, refused, `["a[b][c]"]="v"`},
		{"a closer inside", `m['a[b]c']=v`, refused, `["a[b]c"]="v"`},
		{"escaped brackets inside", `m[a\[b\]c]=v`, refused, `["a[b]c"]="v"`},
		// An opener alone is a key with the option set and not without it.
		{"an opener alone", `m['[']=v`, `["["]="v"`, refused},
		{"two openers", `m['[[']=v`, `["[["]="v"`, refused},
		// A `]` inside runs the key past its closer under both readings.
		{"a closer and a tail", `m['a]b']=v`, refused, refused},
		// Unless the operator stands right behind that first `]`, where both
		// readings find the same short key and leave the rest as the value.
		{"a closer and the operator", `m['a]=b']=v`, `[a]="b]=v"`, `[a]="b]=v"`},
		// And the ordinary spellings, unmoved by the option.
		{"a plain key", `m[foo]=v`, `[foo]="v"`, `[foo]="v"`},
		{"a quoted plain key", `m['foo']=v`, `[foo]="v"`, `[foo]="v"`},
		{"a key with a space", `m['x y']=v`, `["x y"]="v"`, `["x y"]="v"`},
		// The appending spelling takes the same reading, which is a second
		// route to the same list and was missed by the first draft: the `+`
		// is part of the operand and not of the name.
		{"appending", `m['foo[bar']+=v`, `["foo[bar"]="v"`, refused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, state := range []struct {
				word, want string
			}{{"-s", tc.set}, {"-u", tc.unset}} {
				src := "shopt " + state.word + " assoc_expand_once\ndeclare -A m\ndeclare " +
					tc.operand + "\ndeclare -p m\n"
				out, _ := answersRun(t, src)
				if state.want == refused {
					if !strings.Contains(out, "not a valid identifier") {
						t.Errorf("shopt %s: = %q, want the operand refused", state.word, out)
					}
					continue
				}
				want := "declare -A m=(" + state.want + " )"
				if strings.TrimSpace(out) != want {
					t.Errorf("shopt %s: = %q, want %q", state.word, out, want)
				}
			}
		})
	}
}

// refused is a `want` no listing can spell, so a row saying it cannot be
// confused with one naming a key.
const refused = "\x00refused"

// `readonly` and `export` do not take the looser closer, and their refusal
// quotes the **whole** operand.
//
// Measured in the same run: `readonly r1['a[b']=2` is
// “r1[a[b]=2': not a valid identifier“ with the option set and unset alike,
// where the same operand under `declare` is a key with it set. So the option
// does not reach these two, and letting it reach would shorten the word they
// quote back — which is how the first draft of the fix was caught.
func TestTheOptionDoesNotReachReadonlyOrExport(t *testing.T) {
	for _, builtin := range []string{"readonly", "export"} {
		for _, state := range []string{"-s", "-u"} {
			t.Run(builtin+" "+state, func(t *testing.T) {
				src := "shopt " + state + " assoc_expand_once\ndeclare -A r1\n" +
					builtin + " r1['a[b']=2\n"
				out, _ := answersRun(t, src)
				if !strings.Contains(out, "`r1[a[b]=2'") {
					t.Errorf("= %q, want the whole operand quoted back", out)
				}
			})
		}
	}
}
