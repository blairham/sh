// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// zsh's option namespace ignores case and underscores, and a single `no`
// prefix negates: `No_Glob`, `NOGLOB` and `noglob` are one request. Measured
// 2026-09-04, zsh 5.9: each turns globbing off, and `no_no_glob` is `no such
// option` rather than glob restored — the prefix strips once.
func TestSetoptNormalizesZshSpellings(t *testing.T) {
	for _, spelling := range []string{"No_Glob", "NOGLOB", "n_o_g_l_o_b"} {
		out, st := runZsh(t, t.TempDir(), `setopt `+spelling+`; echo x*`)
		if st != 0 || !strings.Contains(out, "x*") {
			t.Errorf("setopt %s: out %q status %d, want the glob off", spelling, out, st)
		}
	}
	out, st := runZsh(t, t.TempDir(), `setopt no_no_glob; echo st=$?`)
	if !strings.Contains(out, "no such option: no_no_glob") || !strings.Contains(out, "st=1") {
		t.Errorf("out %q status %d, want a single-strip refusal at 1", out, st)
	}
}

// A name outside the namespace is refused with the operand as it was typed,
// and the other operands are still acted on — measured, in both orders.
func TestSetoptActsPastABadName(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `setopt zzqq no_glob; echo st=$?; echo x*`)
	if !strings.Contains(out, "no such option: zzqq") ||
		!strings.Contains(out, "st=1") || !strings.Contains(out, "x*") {
		t.Errorf("out %q, want the complaint, status 1, and globbing still off", out)
	}
}

// `unsetopt` is the same request inverted, from either spelling of the name.
// The failed glob then ends the run, which is zsh's own answer too.
func TestUnsetoptInverts(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt no_glob; unsetopt noglob; echo x*`)
	if st != 1 || !strings.Contains(out, "no matches found") {
		t.Errorf("out %q status %d, want globbing back on and the zsh refusal at 1", out, st)
	}
}

// Options that set -o also holds are one switch: `setopt err_exit` ends the
// script the way `set -e` does.
func TestSetoptDrivesTheSharedOptionTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt err_exit; false; echo reached`)
	if st != 1 || strings.Contains(out, "reached") {
		t.Errorf("out %q status %d, want errexit to have ended the run at 1", out, st)
	}
}

// The three axis-backed names: shwordsplit splits unquoted expansions,
// nonomatch passes a failed glob through, ksharrays bases arrays at zero.
// Each is zsh's own name for a semantics axis the vector already carries.
func TestSetoptAxisBackedOptions(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"shwordsplit", `x="a b"; setopt shwordsplit; set -- $x; echo n=$#`, "n=2"},
		{"nonomatch", `unsetopt nomatch; echo x*`, "x*"},
		{"ksharrays", `setopt ksh_arrays; a=(p q); echo ${a[0]}`, "p"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || !strings.Contains(out, tc.want) {
			t.Errorf("%s: out %q status %d, want %q", tc.name, out, st, tc.want)
		}
	}
}

// A subshell's setopt stays in the subshell: the vector is swapped
// copy-on-write, never mutated in place.
func TestSetoptStaysInItsSubshell(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `(setopt shwordsplit); x="a b"; set -- $x; echo n=$#`)
	if !strings.Contains(out, "n=1") {
		t.Errorf("out %q, want the parent still unsplit", out)
	}
}

// Bare `setopt` lists what differs from zsh's defaults, canonically spelled
// and ordered by the base name — measured, `noclobber` prints between
// `allexport` and `errexit`, and `nohashdirs` is the baseline's one line:
// zsh's default is to hash directories and this shell never does.
func TestBareSetoptListsTheDeviations(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt err_exit no_clobber; setopt`)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if got, want := out, "noclobber\nerrexit\nnohashdirs\n"; got != want {
		t.Errorf("listing = %q, want %q", got, want)
	}
}

// An option that exists and cannot be moved answers with zsh's own wording
// for exactly that — measured on `setopt monitor` in a non-interactive zsh.
func TestSetoptRefusesWhatItCannotChange(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `setopt monitor; echo st=$?`)
	if !strings.Contains(out, "can't change option: monitor") || !strings.Contains(out, "st=1") {
		t.Errorf("out %q, want the measured refusal at 1", out)
	}
	// Asking for the state it is already in is granted, the same bargain the
	// substrate's own table strikes for `set +o posix`.
	out, _ = runZsh(t, t.TempDir(), `unsetopt monitor; echo st=$?`)
	if !strings.Contains(out, "st=0") {
		t.Errorf("out %q, want the already-off state granted", out)
	}
}
