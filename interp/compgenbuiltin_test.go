// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `compgen` writes the completions a word would have. What is here is the
// part this shell can answer from what it knows — the names of its own
// commands, and of the functions defined — and everything else is refused
// rather than guessed at.
func TestCompgenGeneratesWhatThisShellKnows(t *testing.T) {
	for _, c := range []struct {
		name, src, want, gone string
		status                int
	}{
		{"a builtin by letter", "compgen -b cd\n", "cd\n", "", 0},
		{"and by action name", "compgen -A builtin cd\n", "cd\n", "", 0},
		{"the word is a prefix", "compgen -b unse\n", "unset\n", "unalias", 0},
		{
			// Nothing to offer is a failure, because a completer is asking
			// whether there is anything at all.
			"nothing matching", "compgen -b zzzz\n", "", "zzz", 1,
		},
		{"a function", "f() { :; }\ncompgen -A function f\n", "f\n", "", 0},
		{
			// No action asked for, so nothing was generated — and that is
			// success rather than failure, which is the one place the two
			// come apart.
			"no action at all", "compgen\n", "", "compgen", 0,
		},
		{
			// An action bash has and this shell cannot generate. It was
			// `file` until #2555 gave that one an answer, which is the shape
			// to keep an eye on: a row naming a *specific* gap stops testing
			// the rule the moment the gap is filled.
			"an action we do not generate", "compgen -A alias\n", "not implemented", "invalid action", 2,
		},
		{
			// And one that is not an action anywhere, which is a different
			// thing to tell a script: the first is a shell that is missing
			// something and this is a typo.
			"a name that is no action", "compgen -A nosuch\n", "invalid action name", "not implemented", 2,
		},
		{"an option letter we do not generate", "compgen -v\n", "not implemented", "", 2},
		{
			// bash has no short letter for the function action at all — `-u`
			// is user names there — so a function is not what this answers.
			// It said otherwise once, which is the wrong direction: an answer
			// where the real shell gives a different one.
			"there is no short letter for function",
			"f1() { :; }\ncompgen -u f1\n", "not implemented", "f1\n", 2,
		},
		{
			// A letter bash does not have either, which is a typo rather than
			// a shell that is missing something — the same split the action
			// names get.
			"a letter that is no letter", "compgen -z\n", "invalid option", "not implemented", 2,
		},
		{
			// The first non-option word is the one matched against and the
			// rest are ignored. Written with the narrow prefix first, so a
			// last-wins reading would answer with the wider one and show.
			"the first word is the one matched",
			"compgen -b unse read\n", "unset\n", "readonly", 0,
		},
		{"an action name with nothing after it", "compgen -A\n", "requires an argument", "", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := compgenRun(t, c.src)
			if c.want != "" && !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
			if c.gone != "" && strings.Contains(out, c.gone) {
				t.Errorf("said %q, want %q not in it", out, c.gone)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d (%q)", st, c.status, out)
			}
		})
	}
}

// The listing is the same one `enable` prints, so a name switched off is not
// offered as a completion either — it is not what running the word would
// find.
func TestCompgenDoesNotOfferABuiltinThatIsSwitchedOff(t *testing.T) {
	out, st := compgenRun(t, "enable -n cd\ncompgen -b cd\n")
	if strings.Contains(out, "cd") {
		t.Errorf("said %q, want a switched-off builtin left out", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1 — nothing left to offer", st)
	}
}

func compgenRun(t *testing.T, src string) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh"})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}

// compgenTree is the directory the filename rows below complete in: two
// directories, two files, a link to a file, a link to a directory, a link to
// nothing, and a hidden one of each.
//
// A directory of the test's own and Runner.Dir pointed at it, never the
// process's: the answers are the whole content of a directory, so a shared one
// would make every row a fact about whatever else happened to be there.
func compgenTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"adir", "bdir", ".hid"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"afile", "bfile", ".hidfile"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range [][2]string{{"afile", "alink"}, {"adir", "dlink"}, {"nowhere", "blink"}} {
		if err := os.Symlink(l[0], filepath.Join(dir, l[1])); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func compgenRunIn(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Dir: dir, Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}

// The filesystem half: `-f`, `-d`, the two action names that mean the same
// thing, and the three `-o` names that generate.
//
// Every want here is sorted, which is this shell's order and not bash's — bash
// answers in readdir order and there is nothing stable to copy. See
// compgenfilenames.go.
func TestCompgenCompletesFilenames(t *testing.T) {
	dir := compgenTree(t)
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"files by letter", "compgen -f a\n", "adir\nafile\nalink\n", 0},
		{"directories by letter", "compgen -d a\n", "adir\n", 0},
		{"a link to a directory is one", "compgen -d d\n", "dlink\n", 0},
		{"a link to nothing is not", "compgen -d b\n", "bdir\n", 0},
		{"but is still a file", "compgen -f b\n", "bdir\nbfile\nblink\n", 0},
		{"the action names mean the same", "compgen -A file a\n", "adir\nafile\nalink\n", 0},
		{"and so does the directory one", "compgen -A directory a\n", "adir\n", 0},
		{
			// Not a pattern. A word with a `*` in it matches the names that
			// begin with those characters, which is none of them.
			"the word is a prefix and not a glob", "compgen -f 'a*'\n", "", 1,
		},
		{"nothing matching is a failure", "compgen -f zzz\n", "", 1},
		{
			// Dotfiles are not hidden — there is no glob here for the hidden
			// rule to apply to — while `.` and `..` are put in only when the
			// word asks for them.
			"an empty word is every entry", "compgen -f ''\n",
			".hid\n.hidfile\nadir\nafile\nalink\nbdir\nbfile\nblink\ndlink\n", 0,
		},
		{"a dot asks for the two the read does not return", "compgen -f .\n", ".\n..\n.hid\n.hidfile\n", 0},
		{"and the prefix is written back as typed", "compgen -f ./a\n", "./adir\n./afile\n./alink\n", 0},
		{"a directory that is not there generates nothing", "compgen -f nosuch/a\n", "", 1},
		{
			// The two actions both run, in the shell's order rather than the
			// command line's, and a name both generate is answered twice.
			"two actions both run", "compgen -df a\n", "adir\nafile\nalink\nadir\n", 0,
		},
		{"and the letters may be written the other way", "compgen -fd a\n", "adir\nafile\nalink\nadir\n", 0},
		{"default generates filenames", "compgen -o default a\n", "adir\nafile\nalink\n", 0},
		{"dirnames generates directories", "compgen -o dirnames a\n", "adir\n", 0},
		{"plusdirs does too, with nothing to add them to", "compgen -o plusdirs a\n", "adir\n", 0},
		{
			// The fallback: something else matched, so `default` contributes
			// nothing at all.
			"default is only a fallback", "compgen -o default -A builtin unse\n", "unset\n", 0,
		},
		{"and so is dirnames", "compgen -o dirnames -A builtin unse\n", "unset\n", 0},
		{
			// Where both are given the directories win, which is measured
			// rather than an ordering picked here.
			"dirnames wins over default", "compgen -o default -o dirnames a\n", "adir\n", 0,
		},
		{
			// plusdirs is not a fallback: it appends, so the directory is
			// answered a second time behind what `-f` found.
			"plusdirs appends instead", "compgen -o plusdirs -f a\n", "adir\nafile\nalink\nadir\n", 0,
		},
		{
			// The six that shape a list rather than generate one produce
			// nothing — but they are still an option asked for, so an empty
			// answer is a failure where a bare `compgen a` is a success.
			"an option that does not generate", "compgen -o filenames a\n", "", 1,
		},
		{"and another", "compgen -o nosort a\n", "", 1},
		{"an option name that is not one", "compgen -o nope a\n", "invalid option name", 2},
		{
			// The argument may be attached, and it ends the cluster: the `f`
			// has been read and `default` is not four more letters.
			"the argument may be attached", "compgen -odefault a\n", "adir\nafile\nalink\n", 0,
		},
		{"and the cluster may carry letters first", "compgen -fo dirnames a\n", "adir\nafile\nalink\n", 0},
		{"an action name may be attached too", "compgen -Abuiltin unse\n", "unset\n", 0},
		{
			// A bare `-o` takes the next operand, so there is none, and the
			// usage line follows the complaint.
			"an option with nothing after it", "compgen -o\n", "requires an argument", 2,
		},
		{"and the usage line comes with it", "compgen -o\n", "compgen: usage: compgen", 2},
		{
			// The next operand is taken whatever it looks like, so the `-f`
			// is an option *name* here and not a letter.
			"the next operand is taken as the name", "compgen -o -f\n", "-f: invalid option name", 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := compgenRunIn(t, dir, c.src)
			if c.status == 0 || c.status == 1 {
				if out != c.want {
					t.Errorf("%s = %q, want %q", c.src, out, c.want)
				}
			} else if !strings.Contains(out, c.want) {
				t.Errorf("%s said %q, want %q in it", c.src, out, c.want)
			}
			if st != c.status {
				t.Errorf("%s status = %d, want %d (%q)", c.src, st, c.status, out)
			}
		})
	}
}
