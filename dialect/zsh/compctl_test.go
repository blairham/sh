// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `compctl` records definitions and lists them back; it completes nothing,
// because this shell has no completer. Every row below was run against zsh
// 5.9.2 on 2026-09-11 before it was written down, and the whole set was then
// diffed against that binary — see dialect/zsh/compctl.go for why recording
// is the right bargain and for the one thing that is not reproduced (#2105).

// A fresh shell already has three entries, and the two spellings of the
// listing name them differently: the bare form by entry, `-L` by the letter
// that sets it.
func TestCompctlListsThreeEntriesInAFreshShell(t *testing.T) {
	dir := t.TempDir()
	if out, st := runZsh(t, dir, "compctl"); st != 0 || out != "COMMAND -c -tn\nDEFAULT -f -tn\nFIRST\n" {
		t.Errorf("bare listing = %q, status %d", out, st)
	}
	if out, st := runZsh(t, dir, "compctl -L"); st != 0 ||
		out != "compctl -C -c -tn\ncompctl -D -f -tn\ncompctl -T\n" {
		t.Errorf("-L listing = %q, status %d", out, st)
	}
}

// A definition is silent and is listed back afterwards, in both spellings.
// This is the shape the real configuration uses — `OMZP::golang` opens with
// ten `compctl -g '…'` lines and reads none of them back.
func TestCompctlRecordsADefinition(t *testing.T) {
	dir := t.TempDir()
	if out, st := runZsh(t, dir, `compctl -g "*.go" gofmt`); st != 0 || out != "" {
		t.Errorf("defining said %q at %d, want silence at 0", out, st)
	}
	if out, _ := runZsh(t, dir, `compctl -g "*.go" gofmt; compctl`); out != "gofmt -g '*.go'\nCOMMAND -c -tn\nDEFAULT -f -tn\nFIRST\n" {
		t.Errorf("bare listing = %q", out)
	}
	if out, _ := runZsh(t, dir, `compctl -g "*.go" gofmt; compctl -L`); out != "compctl -g '*.go' gofmt\ncompctl -C -c -tn\ncompctl -D -f -tn\ncompctl -T\n" {
		t.Errorf("-L listing = %q", out)
	}
}

// The listing is a normalization and not a replay, which is the half that has
// to be right or it is a plausible-but-wrong answer.
func TestCompctlNormalizesWhatItListsBack(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// Simple flags cluster and sort, whatever order they arrived in.
		{"flags cluster and sort", `compctl -q -f ls`, "compctl -fq ls\n"},
		{"the other order is the same", `compctl -f -q ls`, "compctl -fq ls\n"},
		// The clustered ones come first, then the argument-taking ones in
		// the order they were written.
		{"clustered first, then arguments", `compctl -f -k "(a)" -S x -q ls`, "compctl -fq -k '(a)' -S x ls\n"},
		// Arguments are requoted: quoted where a character needs it, bare
		// where none does, and an embedded quote escaped the shell's way.
		{"a space is quoted", `compctl -g "a b" ls`, "compctl -g 'a b' ls\n"},
		{"a slash is bare", `compctl -S "/" ls`, "compctl -S / ls\n"},
		{"an empty argument is quoted", `compctl -S "" ls`, "compctl -S '' ls\n"},
		{"a quote is escaped", `compctl -S "a'b" ls`, `compctl -S 'a'\''b' ls` + "\n"},
		// Entries are listed by name, sorted, whatever order they arrived.
		{
			"names sort", `compctl -g "*.go" zz; compctl -f aa; compctl -k "(1)" mm`,
			"compctl -f aa\ncompctl -k '(1)' mm\ncompctl -g '*.go' zz\n",
		},
		// A second definition replaces rather than merges.
		{"a redefinition replaces", `compctl -g "*.go" g; compctl -g "*.c" g`, "compctl -g '*.c' g\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src+"; compctl -L")
			want := tc.want + "compctl -C -c -tn\ncompctl -D -f -tn\ncompctl -T\n"
			if out != want || st != 0 {
				t.Errorf("out = %q (status %d), want %q", out, st, want)
			}
		})
	}
}

// The three special entries are replaced whole rather than merged — setting
// DEFAULT to `-f` loses the `-tn` it had — and a set one is listed once, by
// its letter, rather than twice.
func TestCompctlSpecialEntriesAreReplacedWhole(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runZsh(t, dir, "compctl -f -D; compctl -L"); out != "compctl -C -c -tn\ncompctl -D -f\ncompctl -T\n" {
		t.Errorf("-D listing = %q, want the default replaced and listed once", out)
	}
	if out, _ := runZsh(t, dir, "compctl -f -T; compctl"); out != "COMMAND -c -tn\nDEFAULT -f -tn\nFIRST -f\n" {
		t.Errorf("-T listing = %q", out)
	}
	if out, _ := runZsh(t, dir, `compctl -g "*.x" -C; compctl`); out != "COMMAND -g '*.x'\nDEFAULT -f -tn\nFIRST\n" {
		t.Errorf("-C listing = %q", out)
	}
}

// Two forms are taken at 0 and store nothing, which is measured and is not
// what most builtins do with either.
func TestCompctlTakesSomeFormsAndStoresNothing(t *testing.T) {
	dir := t.TempDir()
	fresh := "compctl -C -c -tn\ncompctl -D -f -tn\ncompctl -T\n"
	for _, src := range []string{"compctl + ls", "compctl -nosuchthing"} {
		t.Run(src, func(t *testing.T) {
			out, st := runZsh(t, dir, src+"; compctl -L")
			if out != fresh || st != 0 {
				t.Errorf("%q left %q at %d, want the table untouched at 0", src, out, st)
			}
		})
	}
}

// And the refusals, each with zsh's own sentence and status.
func TestCompctlRefusals(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"an argument letter with nothing after it", "compctl -g", "glob pattern expected after -g"},
		{"a matching spec with no colon", "compctl -M x", "missing `:'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if st != 1 {
				t.Errorf("status %d, want 1", st)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("said %q, want it to carry %q", out, tc.want)
			}
		})
	}
	// A well-formed matching spec is refused *silently* at 1, which is the
	// row that says the sentence above belongs to the malformed one.
	if out, st := runZsh(t, dir, `compctl -M "m:{a-z}={A-Z}"`); st != 1 || out != "" {
		t.Errorf("a well-formed -M said %q at %d, want silence at 1", out, st)
	}
}
