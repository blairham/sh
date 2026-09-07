// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The dialect supplies the escape set, so the measured alphabet is asserted
// here — the substrate names the extension point and holds nobody's escapes.
//
// Every want below is zsh 5.9.2's own answer, measured 2026-09-07 with
// `a=(x y)`.
func TestThePrintFlagReadsThisDialectsEscapes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a newline", `a=(x y); printf "[%s]" "${(pj:\n:)a}"`, "[x\ny]"},
		{"a tab", `a=(x y); printf "[%s]" "${(pj:\t:)a}"`, "[x\ty]"},
		{"an escape", `a=(x y); printf "[%s]" "${(pj:\e:)a}"`, "[x\x1by]"},
		{"and its capital", `a=(x y); printf "[%s]" "${(pj:\E:)a}"`, "[x\x1by]"},
		{
			"a bell, a backspace, a form feed, a return and a vertical tab",
			`a=(x y); printf "[%s]" "${(pj:\a\b\f\r\v:)a}"`, "[x\a\b\f\r\vy]",
		},
		// Octal with no leading zero, which is where this set parts company
		// with the same shell's `echo`.
		{"three octal digits", `a=(x y); printf "[%s]" "${(pj:\101:)a}"`, "[xAy]"},
		{"one octal digit", `a=(x y); printf "[%s]" "${(pj:\7:)a}"`, "[x\ay]"},
		{"and three is the limit", `a=(x y); printf "[%s]" "${(pj:\0101:)a}"`, "[x\b1y]"},
		{"hex", `a=(x y); printf "[%s]" "${(pj:\x41:)a}"`, "[xAy]"},
		{"a short unicode escape", `a=(x y); printf "[%s]" "${(pj:\u0041:)a}"`, "[xAy]"},
		{"and a long one", `a=(x y); printf "[%s]" "${(pj:\U00000041:)a}"`, "[xAy]"},
		{"a control character", `a=(x y); printf "[%s]" "${(pj:\C-a:)a}"`, "[x\x01y]"},
		{"and the pair applied in turn", `a=(x y); printf "[%s]" "${(pj:\M-\C-a:)a}"`, "[x\x81y]"},
		{"a doubled backslash is one", `a=(x y); printf "[%s]" "${(pj:\\:)a}"`, `[x\y]`},
		{"a trailing backslash is a backslash", `a=(x y); printf "[%s]" "${(pj:x\:)a}"`, `[xx\y]`},
		// An escape the set does not know loses its backslash, which is the
		// other half of the parting from `echo`.
		{"an unknown escape loses its backslash", `a=(x y); printf "[%s]" "${(pj:\q:)a}"`, "[xqy]"},
		{"a dollar included", `a=(x y); printf "[%s]" "${(pj:\$:)a}"`, "[x$y]"},
		// And the one place a flag argument and a `print` operand differ.
		{"a c is an unknown escape here", `a=(x y); printf "[%s]" "${(pj:A\cB:)a}"`, "[xAcBy]"},
		{"even alone", `a=(x y); printf "[%s]" "${(pj:\c:)a}"`, "[xcy]"},
		{"and a doubled backslash in front of one stays", `a=(x y); printf "[%s]" "${(pj:\\c:)a}"`, `[x\cy]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// `print` still truncates at `\c` — the escape set is shared and the one
// difference is a parameter, so this is the row that would fail if the
// parameter were wired the wrong way round.
func TestPrintStillTruncatesAtTheCEscape(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -- "A\cB"; print -- END`)
	// `\c` drops the rest of *that* command's output, its trailing newline
	// included, and the next command still runs — so `AEND` on one line is
	// the whole answer and not a truncation that leaked.
	if out != "AEND\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "AEND\n")
	}
}

// The real lines that need the flag, from the plugin manager that reported it.
func TestThePrintFlagIsThisDialects(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// `+zi-prehelp-usage-message` line 28: a separator built out of
			// this shell's own color table and then joined with. This is
			// the expansion that refused by name on every startup (#1382).
			"the plugin manager's subcommand list",
			`cmds=( load snippet update delete ); ` +
				`typeset -gA ZI; ZI=( col-rst R col-cmd C ); ` +
				"sep=\"${ZI[col-rst]}${ZI[col-cmd]}\\`, \\`${ZI[col-cmd]}\"; " +
				`printf "[%s]" "${(pj:$sep:)cmds}"`,
			"[load" + "RC`, `C" + "snippet" + "RC`, `C" + "update" + "RC`, `C" + "delete]",
		},
		{
			// The same message's other spelling, where a `M` rides in front
			// of the pair: keep the operands that look like options and join
			// them with the same separator.
			"and the same separator behind a second flag",
			`set -- -a b -c; sep=", "; printf "[%s]" "${(Mpj:$sep:)@:#-*}"`,
			"[-a, -c]",
		},
		{
			// An array whose scalar view is the whole array in this dialect,
			// which is the reading the substrate cannot assert.
			"an array separator arrives joined",
			`arr=(A B); a=(x y); printf "[%s]" "${(pj:$arr:)a}"`,
			"[xA By]",
		},
		{
			"splitting on a NUL, which is the idiom the flag is reached for",
			`v=$(printf 'a\0b'); printf "[%s]" ${(ps:\0:)v}`,
			"[a][b]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The padding pair is still absent, and the refusal names the padding rather
// than the flag that would have read its argument.
func TestThePaddingPairIsStillRefusedInThisDialect(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `v=ab; printf "[%s]" "${(pl:5::-:)v}"`)
	if !strings.Contains(out, "the (l) expansion flag is not implemented") {
		t.Errorf("output = %q, want the padding refused by name", out)
	}
	if st == 0 {
		t.Errorf("status 0, want the refusal to be fatal")
	}
}
