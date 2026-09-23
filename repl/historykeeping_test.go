// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runKept drives an editor-less session over text and answers the lines the
// history file holds afterwards.
//
// The loop with no editor rather than the one with a terminal, because the
// two share the recorder, the rules and the file — see
// TestAnEditorLessSessionKeepsItsHistory, which is the row that holds them
// together — and this one can be driven from a test without a pty.
func runKept(t *testing.T, seed []string, vars map[string]string, text string,
	setup func(s *Shell),
) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hist")
	if len(seed) > 0 {
		if err := os.WriteFile(path, []byte(strings.Join(seed, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if vars == nil {
		vars = map[string]string{}
	}
	vars["PS1"], vars["PS2"], vars["HISTFILE"] = "", "", path
	var ran, said strings.Builder
	r := newTestRunner(vars)
	r.Stdout = &ran
	s := Shell{Runner: r, In: strings.NewReader(text), Out: &ran, Err: &said}
	if setup != nil {
		setup(&s)
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// A command typed over several lines is one joined entry where the dialect
// says so, and the lines it was typed on where it does not.
//
// The two states bash spells `shopt -u lithist` and `shopt -s lithist`, and
// the joined side is bash's default — measured 2026-09-22 against bash 5.3.20
// by reading its own HISTFILE back after `for q in ZZ` / `do` / `echo MARK$q`
// / `done`: the default records `for q in ZZ; do echo MARK$q; done` and
// `shopt -s lithist` records the four lines.
//
// The separator is not decided here. internal/histjoin holds that rule and
// two other readers already reach it; what this asserts is that a *prompt* is
// the third, and that the switch chooses between its answer and the text as
// typed.
func TestAJoinedEntryFollowsTheDialect(t *testing.T) {
	const typed = "echo one\nfor q in ZZ\ndo\necho MARK$q\ndone\necho two\n"
	for _, c := range []struct {
		name string
		join bool
		want []string
	}{
		{
			name: "joined, which is what bash records with nothing said",
			join: true,
			want: []string{"echo one", "for q in ZZ; do echo MARK$q; done", "echo two"},
		},
		{
			// The state `shopt -s lithist` asks for, and the state every
			// shell with no such option is in — which is why it is the
			// core's zero value and the bash preset turns the other one on.
			name: "or the lines it was typed on",
			join: false,
			want: []string{"echo one", "for q in ZZ", "do", "echo MARK$q", "done", "echo two"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := runKept(t, nil, nil, typed, func(s *Shell) {
				s.Runner.SetHistoryJoinsATypedCommand(c.join)
			})
			if strings.Join(got, "\u0000") != strings.Join(c.want, "\u0000") {
				t.Errorf("history file = %q, want %q", got, c.want)
			}
		})
	}
}

// A joined entry is what the *list* holds too, not only the file.
//
// The half a file test cannot see, and the half that matters at a prompt: the
// up arrow walks the list, and an entry recorded one way and recalled another
// would be two records of one command. Asked through `!!`, which reads the
// same list.
func TestAJoinedEntryIsWhatIsRecalled(t *testing.T) {
	got := runKept(t, nil,
		map[string]string{"HISTCONTROL": ""},
		"for q in ZZ\ndo\necho MARK$q\ndone\n:\n",
		func(s *Shell) { s.Runner.SetHistoryJoinsATypedCommand(true) })
	want := []string{"for q in ZZ; do echo MARK$q; done", ":"}
	if strings.Join(got, "\u0000") != strings.Join(want, "\u0000") {
		t.Errorf("history file = %q, want %q", got, want)
	}
}

// The history file is appended to, unless the option asks for the rewrite and
// the session's list no longer holds every line it added.
//
// bash's `shopt histappend` read the other way round, and the condition is
// narrower than the name suggests: measured 2026-09-22 against bash 5.3.20
// through a pseudo-terminal, with eight lines already in HISTFILE,
// `HISTFILESIZE=100` and a session typing four —
//
//	HISTSIZE=20, histappend off   the eight, then the session's four
//	HISTSIZE=2,  histappend off   two lines; the eight are gone
//	HISTSIZE=2,  histappend on    the eight, then the last two
//
// So the rewrite is what the option turns *off*, and it is reached only where
// HISTSIZE trimmed the list below the number of lines the session added. The
// middle row is the one that separates the two states, which is why the case
// below is written at a HISTSIZE of two.
func TestTheHistoryFileIsRewrittenOnlyWhereTheListRanShort(t *testing.T) {
	seed := []string{"seed1", "seed2", "seed3"}
	const typed = "echo one\necho two\necho three\necho four\n"
	for _, c := range []struct {
		name    string
		size    string
		rewrite bool
		want    []string
	}{
		{
			// The list holds everything the session added, so there is
			// nothing to rewrite and the option changes nothing.
			name:    "the list holds it all, so the option changes nothing",
			size:    "20",
			rewrite: true,
			want: []string{
				"seed1", "seed2", "seed3",
				"echo one", "echo two", "echo three", "echo four",
			},
		},
		{
			name:    "the list ran short and the file becomes the list",
			size:    "2",
			rewrite: true,
			want:    []string{"echo three", "echo four"},
		},
		{
			// The same session with the option on, which is `shopt -s
			// histappend`: the file keeps what it had and the tail of the
			// list goes after it. The tail, because HISTSIZE bounds what is
			// written either way — measured on the same binary.
			name:    "or is appended to where the option asks for it",
			size:    "2",
			rewrite: false,
			want:    []string{"seed1", "seed2", "seed3", "echo three", "echo four"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := runKept(t, seed,
				map[string]string{"HISTSIZE": c.size, "HISTFILESIZE": "100"},
				typed,
				func(s *Shell) { s.Runner.SetRewritesTheHistoryFile(c.rewrite) })
			if strings.Join(got, "\u0000") != strings.Join(c.want, "\u0000") {
				t.Errorf("history file = %q, want %q", got, c.want)
			}
		})
	}
}

// How many entries a command typed over several lines makes is a **different**
// question from what goes between its lines, and the two bits compose in one
// direction only.
//
// The states bash spells `shopt -s cmdhist` (its default) and `shopt -u
// cmdhist`. Measured 2026-09-23 on bash 5.3.15 through a pty, typing
// `for i in 1 2` / `do` / `  echo $i` / `done` and reading `history`:
//
//	cmdhist on,  lithist off   one entry, `for i in 1 2; do   echo $i; done`
//	cmdhist on,  lithist on    one entry, holding the newlines
//	cmdhist off, either        four entries
//
// The third row is why `lithist` is moot once this is off, and the rows are
// read off `history`'s numbering rather than the file: one entry holding
// newlines and four separate entries are the same bytes in `$HISTFILE`, and an
// instrument that read the file reported the option doing nothing.
func TestHowManyEntriesATypedCommandMakesFollowsTheDialect(t *testing.T) {
	const typed = "echo one\nfor q in ZZ\ndo\necho MARK$q\ndone\necho two\n"
	for _, c := range []struct {
		name        string
		whole, join bool
		want        []string
	}{
		{
			name:  "whole, which is what bash records with nothing said",
			whole: true, join: true,
			want: []string{"echo one", "for q in ZZ; do echo MARK$q; done", "echo two"},
		},
		{
			// The state `shopt -u cmdhist` asks for: one entry per typed line,
			// with nothing added between them — the separators exist to join and
			// this is the road that does not.
			name:  "a line at a time",
			whole: false, join: true,
			want: []string{"echo one", "for q in ZZ", "do", "echo MARK$q", "done", "echo two"},
		},
		{
			// And the joining bit is moot once the count bit is off, which is the
			// row that says these are two questions and not three states of one.
			name:  "a line at a time, with joining off as well",
			whole: false, join: false,
			want: []string{"echo one", "for q in ZZ", "do", "echo MARK$q", "done", "echo two"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := runKept(t, nil, nil, typed, func(s *Shell) {
				s.Runner.SetHistoryJoinsATypedCommand(c.join)
				s.Runner.SetHistoryKeepsATypedCommandWhole(c.whole)
			})
			if strings.Join(got, "\u0000") != strings.Join(c.want, "\u0000") {
				t.Errorf("history file = %q, want %q", got, c.want)
			}
		})
	}
}

// TestASingleLineCommandIsOneEntryEitherWay: the split is for a command that
// took more than one line, and a one-line command must not be touched by it.
// Without this the option would look right on the `for` loop above and record
// every ordinary command twice the moment the collector held one line.
func TestASingleLineCommandIsOneEntryEitherWay(t *testing.T) {
	got := runKept(t, nil, nil, "echo one\necho two\n", func(s *Shell) {
		s.Runner.SetHistoryKeepsATypedCommandWhole(false)
	})
	want := []string{"echo one", "echo two"}
	if strings.Join(got, "\u0000") != strings.Join(want, "\u0000") {
		t.Errorf("history file = %q, want %q", got, want)
	}
}
