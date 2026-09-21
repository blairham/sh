// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `hash` handed a word that is a *path* rather than a name — as an operand,
// and as the argument of `-p`. Three questions that were three bugs (#4064).

// TestHashAndAnOperandWithASlash is the operand form, over the two axes that
// decide it between them.
//
// Measured 2026-09-21, `env -i PATH=/usr/bin:/bin LC_ALL=C`, `hash /bin/ls`
// and `hash /nosuchfile` one probe at a time — the existence of the file
// moves no column, which is what makes this a question about the operand as
// written:
//
//	bash 5.3.20   silent, 0, table empty
//	dash          silent, 0, table empty
//	BusyBox ash   silent, 0, table empty
//	zsh 5.9.2     `no such command: /bin/ls` at 1, table empty
//	ksh93u+       silent, 0, table holds `/bin/ls=/bin/ls`
//
// So the three readings are: passed over, searched for the only way PATH can
// be searched and therefore missed, and taken as the path it names.
func TestHashAndAnOperandWithASlash(t *testing.T) {
	for _, tc := range []struct {
		name       string
		ignores    Answer
		pathAlone  Answer
		wantStatus string
		wantSaid   string
		wantEntry  bool
	}{
		{"passed over", Yes, No, "st=0", "", false},
		{"searched and missed", No, Yes, "st=1", "hash: ./d1/zzc: not found", false},
		{"taken as the path it names", No, No, "st=0", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A file that really is there, so "not remembered" cannot be
			// mistaken for "could not be found": the two answers that keep
			// the table empty do it about an operand the shell could have
			// resolved perfectly well.
			out, _ := withDirs(t, `hash ./d1/zzc; echo st=$?; hash`,
				func(d1, _ string) { hashable(t, d1, "zzc", "echo V1") },
				func(r *Runner) {
					s := testSemantics()
					s.HashIgnoresAnOperandWithASlash = tc.ignores
					s.HashSearchesPathAlone = tc.pathAlone
					r.Semantics = &s
				})
			if !strings.Contains(out, tc.wantStatus+"\n") {
				t.Errorf("out=%q, want %s", out, tc.wantStatus)
			}
			if tc.wantSaid != "" && !strings.Contains(out, tc.wantSaid) {
				t.Errorf("out=%q, want it to say %q", out, tc.wantSaid)
			}
			if tc.wantSaid == "" && strings.Contains(out, "not found") {
				t.Errorf("out=%q, want nothing said about the operand", out)
			}
			// The listing is the whole of the second half: an entry made
			// where the shell makes none is the defect this pins, and it is
			// invisible in the status.
			listed := strings.Contains(out, filepath.Join("d1", "zzc")+"\n")
			if listed != tc.wantEntry {
				t.Errorf("out=%q, want an entry in the table: %v", out, tc.wantEntry)
			}
		})
	}
}

// TestATrustedHashedPathIsReportedWithoutLookingForItAgain is `type` and
// `command -v` over an entry whose path is not there.
//
// Measured 2026-09-21 on bash 5.3.20: `hash -p /nosuchfile cat; type cat` is
// `cat is hashed (/nosuchfile)` at 0 and `command -v cat` is `/nosuchfile`,
// while running `cat` is `/nosuchfile: No such file or directory` at 127. So
// the report answers from the table and the run does not — and before this
// the report said `not found` about a name that had a perfectly good
// /bin/cat behind it, which left `hash -p` making the name *worse off* than
// it was.
//
// `shopt -s checkhash` does not move the report, measured with the option on,
// which is why this reads the trust axis and not that switch. The far side is
// zsh, measured the same day: a hashed `zzc` whose file has been removed is
// `zzc not found` at 1 there.
func TestATrustedHashedPathIsReportedWithoutLookingForItAgain(t *testing.T) {
	const gone = "/nosuchfile-zz"
	for _, tc := range []struct {
		name    string
		trusted Answer
		want    string
	}{
		{"trusted", Yes, "zzprog is " + gone},
		{"looked for again", No, "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `hash -p `+gone+` zzprog; echo st=$?; type zzprog; command -v zzprog`,
				func(r *Runner) {
					// A PATH with nothing on it, so the only thing that
					// could answer is the table.
					r.Env = []string{"PATH=" + t.TempDir()}
					s := testSemantics()
					s.HashTakesAPathToRemember = Yes
					s.CommandHashIsTrusted = tc.trusted
					r.Semantics = &s
				})
			if !strings.Contains(out, "st=0\n") {
				t.Errorf("out=%q, want `hash -p` itself to succeed", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("out=%q, want it to say %q", out, tc.want)
			}
		})
	}
}

// TestHashWillNotRememberADirectory is the one check `hash -p` makes on the
// path it is handed.
//
// Measured 2026-09-21 on bash 5.3.20 and bash 3.2.57, the only column with
// the letter: `hash -p /nosuchfile cat` and `hash -p /dev/null cat` are both
// remembered in silence at 0 — so neither existing nor being executable is
// asked about — while `hash -p /tmp cat` and `hash -p . cat` are
// `hash: /tmp: Is a directory` at 1 with the table left empty. The complaint
// is per name: `hash -p /tmp a b` prints it twice.
func TestHashWillNotRememberADirectory(t *testing.T) {
	dir := t.TempDir()
	setup := func(r *Runner) {
		s := testSemantics()
		s.HashTakesAPathToRemember = Yes
		r.Semantics = &s
	}
	out, st := run(t, `hash -p `+dir+` zza zzb; echo st=$?; hash`, setup)
	if st != 0 {
		t.Fatalf("st=%d, want the script itself to finish", st)
	}
	if n := strings.Count(out, ": Is a directory"); n != 2 {
		t.Errorf("out=%q, want the complaint once per name, got %d", out, n)
	}
	if !strings.Contains(out, "st=1\n") {
		t.Errorf("out=%q, want the builtin to answer 1", out)
	}
	// And nothing is remembered, which is the half a status cannot show: the
	// name must be no worse off than it was.
	if strings.Contains(out, dir) != true || strings.Count(out, dir) != 2 {
		t.Errorf("out=%q, want the directory named in the two complaints and nowhere else", out)
	}
	// A path that is merely absent is remembered without a word, which is
	// what keeps this a check for a *directory* rather than a check that the
	// path is good.
	out, _ = run(t, `hash -p `+filepath.Join(dir, "nosuchfile")+` zza; echo st=$?; hash`, setup)
	if strings.Contains(out, "Is a directory") || !strings.Contains(out, "st=0\n") {
		t.Errorf("out=%q, want an absent path remembered in silence", out)
	}
	if !strings.Contains(out, filepath.Join(dir, "nosuchfile")+"\n") {
		t.Errorf("out=%q, want the absent path in the table", out)
	}
}
