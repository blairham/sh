// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `shopt -s shift_verbose` makes `shift` say something when its count is
// above `$#`. The status was already 1 and `$#` was already left alone; what
// the option buys is the sentence, and refusing the name left a script that
// asked for it silent (#3465).
//
// Measured 2026-09-17 on bash 5.3.20 and bash 3.2.57 alike, from a script
// file under `env -i PATH=/usr/bin:/bin LC_ALL=C`. The two builds agree on
// every row here; they part only on `shift -- 5`, where 3.2 names the marker
// and 5.3 names the count, which is that build's own age rather than this
// option's and is the `bash32` column's to record.
func TestShiftVerboseNamesACountAboveTheEnd(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Silent by default, which is this shell's own answer and the reason
		// the wording alone could not be installed.
		{`set -- a; shift 3; echo "st=$?"`, "st=1\n"},
		{
			`shopt -s shift_verbose; set -- a; shift 3; echo "st=$?"`,
			"bash: line 1: shift: 3: shift count out of range\nst=1\n",
		},
		// A count word that was never written has no slot to fill, so the
		// sentence drops it rather than naming a placeholder.
		{
			`shopt -s shift_verbose; shift; echo "st=$?"`,
			"bash: line 1: shift: shift count out of range\nst=1\n",
		},
		// Past a marker the operand is the count, and that is what is named.
		{
			`shopt -s shift_verbose; set -- a; shift -- 2; echo "st=$?"`,
			"bash: line 1: shift: 2: shift count out of range\nst=1\n",
		},
		// The parameters are untouched either way, which is what makes this
		// a complaint rather than a failure with a consequence.
		{`shopt -s shift_verbose; set -- a b; shift 5 2>/dev/null; echo "[$*]"`, "[a b]\n"},
		// In range is silent with the option on: a count landing exactly on
		// `$#`, and a nought over no parameters at all.
		{`shopt -s shift_verbose; set -- a b; shift 2; echo "st=$? [$*]"`, "st=0 []\n"},
		{`shopt -s shift_verbose; shift 0; echo "st=$?"`, "st=0\n"},
		// A function is not a different shell for this.
		{
			`shopt -s shift_verbose; f() { shift 9; echo "st=$?"; }; f`,
			"bash: line 1: shift: 9: shift count out of range\nst=1\n",
		},
		// And the way back.
		{
			`shopt -s shift_verbose; shopt -u shift_verbose; set -- a; shift 3; echo "st=$?"`,
			"st=1\n",
		},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The other end of the range is not the option's: a count below zero is named
// whether it is set or not. Measured on both builds — the option moves one end
// and one end only, which is why the gate is at the call site rather than
// around the whole complaint.
func TestShiftVerboseDoesNotGovernANegativeCount(t *testing.T) {
	for _, src := range []string{
		`set -- a; shift -1; echo "st=$?"`,
		`shopt -s shift_verbose; set -- a; shift -1; echo "st=$?"`,
	} {
		out, st := runBash(t, t.TempDir(), src)
		want := "bash: line 1: shift: -1: shift count out of range\nst=1\n"
		if out != want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", src, out, st, want)
		}
	}
}

// And the name is answered by the builtin the way every wired name is, which
// is what it was not while it sat in the refusing table.
func TestShiftVerboseIsAnOptionRatherThanARecordedName(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`shopt -s shift_verbose; echo "st=$?"`, "st=0\n", 0},
		{`shopt -s shift_verbose; shopt -p shift_verbose`, "shopt -s shift_verbose\n", 0},
		{`shopt -p shift_verbose`, "shopt -u shift_verbose\n", 1},
		{`shopt -s shift_verbose; shopt shift_verbose`, "shift_verbose       \ton\n", 0},
		{`shopt -u shift_verbose; echo "st=$?"`, "st=0\n", 0},
		{
			`shopt -s shift_verbose; case ":$BASHOPTS:" in *:shift_verbose:*) echo in ;; *) echo out ;; esac`,
			"in\n", 0,
		},
		{`case ":$BASHOPTS:" in *:shift_verbose:*) echo in ;; *) echo out ;; esac`, "out\n", 0},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q status %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}
