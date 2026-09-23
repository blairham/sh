// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The quoting on a builtin operand's subscript decides what the second round
// expands, and comes off with it rather than before it.
//
// Measured 2026-09-22 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C bash f.sh`, standard input on the null device, bash 5.3.20, with
// `declare -A a; key='$(echo foo)'` and the five characters `$key` stored as
// the one key:
//
//	unset "a['$key']"   the element is gone — the apostrophes protected the
//	                    `$` from the round and were gone from the key anyway
//	unset "a[$key]"     still there: the round expanded `$key` and named an
//	                    element nothing has
//
// Taking the quoting off first turned the first row into the second, and
// `unset` says nothing when it removes nothing — so a script clearing an entry
// out of a keyed table kept it, at status 0 (#4254).
func TestAnOperandSubscriptsQuotingSurvivesUntilTheRound(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an apostrophe protects the round's dollar",
			`declare -A a; key='$(echo foo)'; a['$key']=2; unset "a['\$key']"; printf '[%s]' "${#a[@]}"`,
			"[0]",
		},
		{
			"an unquoted dollar is expanded by the round",
			`declare -A b; key='$(echo foo)'; b['$key']=2; unset "b[\$key]"; printf '[%s]' "${b['$key']}"`,
			"[2]",
		},
		{
			"the protected text is the key and not what it expands to",
			`declare -A g; k=x; g[x]=1; unset "g['\$k']"; printf '[%s]' "${g[x]}"`,
			"[1]",
		},
		{
			"and it does name the key spelled that way",
			`declare -A h; h['$k']=1; k=x; unset "h['\$k']"; printf '[%s]' "${#h[@]}"`,
			"[0]",
		},
		{
			"a store through an operand protects it too",
			`declare -A e; k=x; e[x]=1; read "e['\$k']" <<< z; printf '[%s][%s]' "${e['$k']}" "${e[x]}"`,
			"[z][1]",
		},
		// The controls: quoting with nothing to protect still comes off, and
		// an apostrophe that never closes is no quoting to take off at all.
		{
			"quoting with nothing in it still comes off",
			`declare -A d; d[x]=1; unset "d['x']"; printf '[%s]' "${#d[@]}"`,
			"[0]",
		},
		{
			"an unbalanced apostrophe is a character of the key",
			`declare -A n; c="x'y"; n[$c]=1; unset "n[$c]"; printf '[%s]' "${#n[@]}"`,
			"[1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A leading tilde is on the same side of that line: it is taken where the
// subscript *as written* opens with one.
//
// Measured in the same run, with both `~/k` and the expanded path stored as
// keys of one table:
//
//	unset "m[~/k]"       removes the home-directory key
//	unset "m[\"~/k\"]"   removes the literal `~/k`
//
// Runner.operandSubscriptTilde said exactly this and was handed text whose
// quotes had already gone, so the tilde was taken on both spellings (#4254).
func TestAnOperandSubscriptsTildeIsReadFromTheTextAsWritten(t *testing.T) {
	// The home directory comes from the environment rather than from an
	// assignment in the snippet, because a tilde is expanded against the
	// value the shell was started with.
	homed := func(t *testing.T, src string) (string, int) {
		t.Helper()
		out, st, err := preset.Combined(t, dialecttest.Base{
			Name: "sh", Env: []string{"PATH=/usr/bin:/bin", "HOME=/hm"},
		}, src)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		return out, st
	}
	for _, tc := range []struct{ name, src, want string }{
		{
			"unquoted, so the tilde is taken",
			`declare -A m; m['~/k']=1; m[/hm/k]=2; unset "m[~/k]"; printf '[%s][%s]' "${m['~/k']}" "${m[/hm/k]}"`,
			"[1][]",
		},
		{
			"quoted, so it is a character of the key",
			`declare -A q; q['~/k']=1; q[/hm/k]=2; unset "q[\"~/k\"]"; printf '[%s][%s]' "${q['~/k']}" "${q[/hm/k]}"`,
			"[][2]",
		},
		{
			"quoted with an expansion behind it, which the round still performs",
			`declare -A p; v=z; p['~/z']=1; p[/hm/z]=2; unset "p[\"~/\$v\"]"; printf '[%s][%s]' "${p['~/z']}" "${p[/hm/z]}"`,
			"[][2]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := homed(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
