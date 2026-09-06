// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// qualifying is the grammar a trailing qualifier list needs, named by the
// constructs rather than by a shell: a bare pattern group, and the flag that
// puts one where an argument stands.
func qualifying(d *syntax.Dialect) {
	d.PatternAlternation = true
	d.GlobQualifiers = true
}

// qualifierDir is a directory with one of each thing a type qualifier can ask
// about, and a hidden name to ask `D` about.
func qualifierDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"f1", "f2", ".dot"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "d1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("f1", filepath.Join(dir, "l1")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runQualified runs src in dir, with a pattern that matches nothing being the
// error it is in the shell that has qualifiers — because half of what a
// qualifier list does is only visible through the miss it causes.
func runQualified(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	return runQualifiedWith(t, dir, src, nil)
}

// runQualifiedWith is runQualified with one more axis moved, for the question
// that is not the dialect flag's.
//
// The dialect is handed to the parser *and* to the runner, which is the whole
// shape of this flag: the parser decides that `( x )` is a word and the
// matcher decides that its group is a list of qualifiers, and they have to
// read the same answer.
func runQualifiedWith(t *testing.T, dir, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	d := syntax.Core()
	qualifying(&d)
	return runGrammar(t, src, qualifying, func(r *Runner) {
		r.Dialect = &d
		r.Dir = dir
		sem := *r.Semantics
		sem.GlobNoMatchIsError = Yes
		if set != nil {
			set(&sem)
		}
		r.Semantics = &sem
	})
}

// The trailing qualifier list, which narrows what a pattern matched.
//
// Every row is a measurement on zsh 5.9.2, 2026-09-06, against exactly the
// directory qualifierDir builds.
func TestATrailingGroupIsAQualifierList(t *testing.T) {
	dir := qualifierDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"everything, for the rows below to be read against", `printf "[%s]" *`, `[d1][f1][f2][l1]`},
		{"a dot is a regular file", `printf "[%s]" *(.)`, `[f1][f2]`},
		{"a slash is a directory", `printf "[%s]" *(/)`, `[d1]`},
		// A link's own type, from an lstat: `l1` points at a regular file
		// and is still `@` rather than `.`, which is the row that says the
		// test does not follow it.
		{"an at-sign is a symbolic link", `printf "[%s]" *(@)`, `[l1]`},
		{"and the link is not a regular file", `printf "[%s]" l1(.)`, ""},
		{"a caret turns the sense of what follows", `printf "[%s]" *(^.)`, `[d1][l1]`},
		// It turns *everything* after it in the section and not only the
		// next one: neither a regular file nor a directory leaves the link.
		{"and everything after it, not only the next", `printf "[%s]" *(^./)`, `[l1]`},
		{"in either order", `printf "[%s]" *(^/.)`, `[l1]`},
		{"and again for a directory", `printf "[%s]" *(^/)`, `[f1][f2][l1]`},
		// Two qualifiers are an *and* and a comma is an *or*, which is the
		// pair that would be got backwards by reading either one alone.
		{"two qualifiers are an and", `printf "[%s]" *(./)`, ""},
		{"a comma is an or", `printf "[%s]" *(.,/)`, `[d1][f1][f2]`},
		{"D takes the hidden names too", `printf "[%s]" *(D)`, `[.dot][d1][f1][f2][l1]`},
		{"beside another qualifier", `printf "[%s]" *(D.)`, `[.dot][f1][f2]`},
		{"in either order", `printf "[%s]" *(.D)`, `[.dot][f1][f2]`},
		// A list makes the field a pattern whatever is in front of it: `f1`
		// on its own is no pattern at all, so a literal name plus a list
		// still reaches the filesystem.
		{"a literal name with a list is still matched", `printf "[%s]" f1(.)`, `[f1]`},
		{"and can be refused by it", `printf "[%s]" f1(/)`, ""},
		{"a directory by name", `printf "[%s]" d1(/)`, `[d1]`},
		// A group holding a `|` is the alternation the dialect already
		// reads, which is the whole disambiguation and exactly one
		// character wide.
		{"a group with a bar is an alternation", `printf "[%s]" *(f1|f2)`, `[f1][f2]`},
		{"even one that would be two valid qualifiers", `printf "[%s]" f(1|2)`, `[f1][f2]`},
		// And a group that is not at the end is an alternation too, so
		// being last is a condition rather than a convenience.
		{"a group before more text is not a list", `printf "[%s]" *(.)x`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runQualified(t, dir, tc.src)
			if tc.want == "" {
				if out == "" || !containsSub(out, "no matches found") {
					t.Errorf("got %q, want the pattern reported as matching nothing", out)
				}
				return
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// `N` is `null_glob` for one pattern: a miss is no error and the word goes.
//
// Asserted beside the same pattern without it, because "no output" is what a
// fatal miss and a deleted word have in common and only the status and the
// diagnostic separate them.
func TestTheNQualifierDeletesAMissRatherThanFailing(t *testing.T) {
	dir := qualifierDir(t)
	// `[]` and not nothing: the *word* is deleted, so `printf` runs with no
	// operands and writes its format once. Which is the answer the shell
	// gives, and the reason this is asserted rather than "no output" — a
	// fatal miss produces no `[]` at all.
	out, st := runQualified(t, dir, `printf "[%s]" zz*(N); printf "done"`)
	if want := "[]done"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
	out, st = runQualified(t, dir, `printf "[%s]" zz*; printf "done"`)
	if !containsSub(out, "no matches found: zz*") || st == 0 {
		t.Errorf("got %q (status %d), want the miss fatal without the flag", out, st)
	}
	// It narrows the *same* match set as any other qualifier beside it.
	out, _ = runQualified(t, dir, `printf "[%s]" *(N.)`)
	if want := `[f1][f2]`; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A character no qualifier claims is refused by name, and the name is the
// point: a list may be long and only one character in it was wrong.
//
// A space is a character like any other here, which is what says
// `echo MY ( x )` was read as a qualifier list rather than as anything of the
// shell's — and it is the answer the shell itself gives.
func TestAnUnknownQualifierIsRefusedByName(t *testing.T) {
	dir := qualifierDir(t)
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" *(qqq)`, "unknown file attribute: q"},
		{`printf "[%s]" *(1)`, "unknown file attribute: 1"},
		{`printf "[%s]" MY ( x )`, "unknown file attribute:  "},
		{`printf "[%s]" ( x )`, "unknown file attribute:  "},
		// The `(#q…)` form needs an option this slice does not carry, so it
		// is refused at the `#` rather than answered.
		{`printf "[%s]" *(#q.)`, "unknown file attribute: #"},
	} {
		out, st := runQualified(t, dir, tc.src)
		if !containsSub(out, tc.want) || st == 0 {
			t.Errorf("%s = %q (status %d), want %q and a failure", tc.src, out, st, tc.want)
		}
	}
	// The whole word is named when the list is good and the pattern missed,
	// which is the other half of the same promise: `*(./)` is not `*`.
	out, _ := runQualified(t, dir, `printf "[%s]" *(./)`)
	if want := "no matches found: *(./)"; !containsSub(out, want) {
		t.Errorf("got %q, want %q", out, want)
	}
}

// Quoting still decides whether text is a pattern.
//
// The parentheses join the set globEscape protects, which is what keeps this
// working: `echo "( x )"` is five characters and not a qualifier list, and
// the escaped spelling is the same. Where the group *came from* is the other
// half of the question and belongs to the dialect that answers it — see
// dialect/zsh, where an unquoted expansion is not a pattern at all.
func TestQuotingKeepsAGroupFromBeingOne(t *testing.T) {
	dir := qualifierDir(t)
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "( x )"`, `[( x )]`},
		{`printf "[%s]" \( x \)`, `[(][x][)]`},
		{`printf "[%s]" "f(1|2)"`, `[f(1|2)]`},
		{`printf "[%s]" *"(.)"`, ""},
	} {
		out, st := runQualified(t, dir, tc.src)
		if tc.want == "" {
			if !containsSub(out, "no matches found") {
				t.Errorf("%s = %q, want the quoted group matched as text", tc.src, out)
			}
			continue
		}
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// With globbing off the word is the same word and only the group's reading
// has changed, which is what says the grammar half is unconditional.
//
// `setopt no_glob; echo MY ( x )` prints `MY ( x )` in the shell with the
// construct — two words, printed with the space between them — where the
// same input with globbing on names a file attribute.
func TestWithGlobbingOffTheGroupIsOnlyText(t *testing.T) {
	dir := qualifierDir(t)
	out, st := runQualifiedWith(t, dir, `set -f; printf "[%s]" MY ( x )`,
		func(s *Semantics) { s.SetFTurnsOffGlobbing = Yes })
	if want := `[MY][( x )]`; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
