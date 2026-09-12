// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A feature `zmodload -F` leaves off is taken out of the builtin table, and
// one it puts back is put back.
//
// Measured on zsh 5.9.2, 2026-09-10, and every line below is byte-identical
// in the two shells:
//
//	zmodload zsh/zutil
//	zmodload -F zsh/zutil -b:zparseopts
//	zparseopts -D a:     zsh:1: command not found: zparseopts   127
//	zstyle -s x y z                                             1
//
// Recording the selection was not enough: `-lF` reported it truthfully and
// every builtin stayed callable, so a script that narrowed a module went on
// using what it had just switched off (#1635).
//
// Each case asserts the **status** as well as the sentence, because the two
// refusals here are one digit apart in meaning and identical in shape: 127 is
// the name resolving to nothing, and 1 is the builtin running and complaining
// about its operands. A test matching only "there was a diagnostic" would
// pass for a shell that never withdrew anything.
func TestAFeatureLeftOffIsNotCallable(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the one left off",
			"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\nzparseopts -D a:\nprint \"st=$?\"",
			"st=127",
		},
		{
			"the ones left on still answer",
			"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\nzstyle -s x y z\nprint \"st=$?\"",
			"st=1",
		},
		{
			"and the sign puts it back",
			"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\n" +
				"zmodload -F zsh/zutil +b:zparseopts\nzparseopts -D a:\nprint \"st=$?\"",
			"st=1",
		},
		{
			"so does a plain load of the module",
			"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\n" +
				"zmodload zsh/zutil\nzparseopts -D a:\nprint \"st=$?\"",
			"st=1",
		},
		{
			"a selection in a subshell does not reach out of it",
			"zmodload zsh/zutil\n( zmodload -F zsh/zutil -b:zparseopts; zparseopts -D a: )\n" +
				"print \"in=$?\"\nzparseopts -D a:\nprint \"out=$?\"",
			"in=127",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, _ := runZshSplit(t, t.TempDir(), tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s\n = %q, want %q in it", tc.src, out, tc.want)
			}
		})
	}
	// The other half of the last one, kept apart so a failure names which
	// direction leaked.
	out, _, _ := runZshSplit(t, t.TempDir(),
		"zmodload zsh/zutil\n( zmodload -F zsh/zutil -b:zparseopts )\nzparseopts -D a:\nprint \"out=$?\"")
	if !strings.Contains(out, "out=1") {
		t.Errorf("got %q, want the parent to still have the builtin", out)
	}
}

// Withdrawing is not `disable`: that shell lists nothing afterwards and
// answers `enable zparseopts` with a hash-table complaint rather than putting
// the builtin back. Measured, and the reason `SetBuiltinEnabled` was the
// wrong seam for this.
func TestAWithdrawnFeatureIsNotADisabledBuiltin(t *testing.T) {
	out, _, _ := runZshSplit(t, t.TempDir(),
		"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\ndisable\nprint \"dis=$?\"")
	if strings.Contains(out, "zparseopts") || !strings.Contains(out, "dis=0") {
		t.Errorf("got %q, want an empty listing at 0", out)
	}
}

// `-lF` still reports the selection it always did: the listing and the table
// are two halves of one answer, and the half that was already right must not
// move.
func TestTheFeatureListingStillReportsTheSelection(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\nzmodload -lF zsh/zutil")
	const want = "+b:zformat\n-b:zparseopts\n+b:zregexparse\n+b:zstyle\n"
	if out != want || st != 0 {
		t.Errorf("listing = %q (status %d), want %q at 0", out, st, want)
	}
}

// The other listing has to agree with it. `disable` was right and `enable`
// was not: a withdrawn name went on appearing in `enable`'s output and in
// `$builtins`, and `enable`/`disable` given that name went on succeeding as
// though the module had never taken it away.
//
// Measured on zsh 5.9.2, 2026-09-12, after
// `zmodload zsh/zutil; zmodload -F zsh/zutil -b:zparseopts`:
//
//	enable | grep -c zparseopts       0
//	${+builtins[zparseopts]}          0
//	enable zparseopts    zsh:enable:1: no such hash table element: zparseopts   1
//	disable zparseopts   zsh:disable:1: no such hash table element: zparseopts  1
//
// Each row here is one this shell answered differently before the fix — `1`,
// `1`, silence at 0 and silence at 0 — so any one of them failing is a
// regression rather than a rewording.
func TestAWithdrawnBuiltinIsOutOfTheEnableListingToo(t *testing.T) {
	const setUp = "zmodload zsh/parameter\nzmodload zsh/zutil\n" +
		"zmodload -F zsh/zutil -b:zparseopts\n"
	for _, tc := range []struct{ name, src, want string }{
		{
			"the listing does not name it",
			setUp + listingHolds,
			"n=0",
		},
		{
			"nor does the parameter that reads the same table",
			setUp + "print \"n=${+builtins[zparseopts]}\"",
			"n=0",
		},
		{
			"and enabling it is a hash-table complaint, not a restore",
			setUp + "enable zparseopts\nprint \"st=$?\"",
			"st=1",
		},
		{
			"as is disabling it",
			setUp + "disable zparseopts\nprint \"st=$?\"",
			"st=1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, _ := runZshSplit(t, t.TempDir(), tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s\n = %q, want %q in it", tc.src, out, tc.want)
			}
		})
	}
	// The sentence, once — the two builtins share it and differ only in the
	// name in front, which is the part a caller greps for.
	_, _, errOut := runZshSplit(t, t.TempDir(), setUp+"enable zparseopts")
	if !strings.Contains(errOut, "no such hash table element: zparseopts") ||
		!strings.Contains(errOut, "enable:") {
		t.Errorf("diagnostic = %q, want zsh's hash-table sentence", errOut)
	}
	// And the module can still put it back: the name is out of the *lookup*,
	// not forgotten, so `+b:` restores the builtin this shell already had.
	out, _, _ := runZshSplit(t, t.TempDir(),
		setUp+"zmodload -F zsh/zutil +b:zparseopts\n"+listingHolds)
	if !strings.Contains(out, "n=1") {
		t.Errorf("got %q, want the name back in the listing", out)
	}
}

// listingHolds prints `n=1` when `enable`'s listing names the withdrawn
// builtin and `n=0` when it does not. Counted in the shell rather than
// through `grep`, so the test measures this shell's listing and not whether
// the machine running it has a `grep`.
const listingHolds = "n=0\nfor b in ${(f)\"$(enable)\"}; do [[ $b == zparseopts ]] && n=1; done\nprint \"n=$n\""
