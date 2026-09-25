// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// `$history`, the eleventh view of the `zsh/parameter` module.
//
// Every expectation here was run side by side against zsh 5.9.2
// (aarch64-apple-darwin25.4.0) on 2026-09-24, `zsh -f` over a script file,
// with the list put in by `fc -R` of a six-line file — and every one of them
// is byte-identical between the two shells.
//
// **The file is six lines and the table holds five**, in both. The sixth is
// the event `$HISTCMD` names, which zsh's table leaves out; see
// zshHistoryEvents. The pty half of the same question — what a **widget**
// reads at a prompt, where the entry left out is the line being typed and so
// there is none to leave out — is in cmd/zsh/historyparampty_test.go.

// histSrc is a script that loads a known list and then asks one question of
// it. The file is written per test so that no two share a list.
func histSrc(t *testing.T, dir, ask string) string {
	t.Helper()
	write(t, dir, "hist", "ls alpha\necho two\nls beta\necho three\nls gamma\necho four\n")
	return "fc -R " + filepath.Join(dir, "hist") + "\n" + ask + "\n"
}

func TestTheHistoryTableIsTheListKeyedByEventNumber(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, status := fcScript(t, dir, histSrc(t, dir, `print -r -- "${(k)history}"`+"\n"+`print -rl -- ${(v)history}`))
	if status != 0 {
		t.Fatalf("status %d, output %q", status, out)
	}
	// Newest first, which is zsh's own order and not this engine's sorted
	// one — `${(k)history}` is `5 4 3 2 1` there too, and sorted keys would
	// put `1` in front and, once the list passes ten, `10` in front of `9`.
	want := "5 4 3 2 1\nls gamma\necho three\nls beta\necho two\nls alpha\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestASearchOverTheHistoryTableTakesTheNewestMatch(t *testing.T) {
	t.Parallel()
	// The line zsh-autosuggestions is built on, and the whole reason the
	// order above is not a presentation choice: `(r)` takes the *first*
	// match in scan order, so the order decides whether a suggestion is the
	// command run a minute ago or the one from the top of the list.
	dir := t.TempDir()
	out, status := fcScript(t, dir, histSrc(t, dir, `print -r -- "${history[(r)ls*]}"`+"\n"+`print -r -- "${history[(R)ls*]}"`))
	if status != 0 {
		t.Fatalf("status %d, output %q", status, out)
	}
	want := "ls gamma\nls gamma ls beta ls alpha\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestAHistoryEventNumberIsAKeyAndAnythingElseIsNot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, status := fcScript(t, dir, histSrc(t, dir,
		`print -r -- "five=${history[5]}"`+"\n"+
			`print -r -- "current=${history[6]}"`+"\n"+
			`print -r -- "missing=${history[999]}"`+"\n"+
			`print -r -- "word=${history[nosuch]}"`+"\n"+
			`print -r -- "set=${history[999]+SET}"`))
	if status != 0 {
		t.Fatalf("status %d, output %q", status, out)
	}
	// `six` is the sixth line of the file and the event this shell is on, so
	// it is not a key — the one-key route has to leave out what the whole
	// table leaves out, or `${history[$HISTCMD]}` would answer where
	// `${(k)history}` never names it.
	want := "five=ls gamma\ncurrent=\nmissing=\nword=\nset=\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestTheHistoryTableDescribesAsAReadonlyModuleAssociation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, status := fcScript(t, dir, histSrc(t, dir, `print -r -- "${(t)history}"`))
	if status != 0 {
		t.Fatalf("status %d, output %q", status, out)
	}
	// `association-readonly-hide-hideval-special`, read back from zsh 5.9.2.
	// The pair of hiding letters is hideModuleParameter's, measured there
	// over every module parameter this dialect registers.
	if got := strings.TrimSpace(out); got != "association-readonly-hide-hideval-special" {
		t.Errorf("got %q", got)
	}
}

func TestAssigningToTheHistoryTableIsRefused(t *testing.T) {
	t.Parallel()
	// Readonly rather than given a writer, which is `builtins`' route and
	// for the same reason: an assignment that landed in a stored table would
	// shadow the producer from then on, and the view would silently become a
	// snapshot with nothing about its shape changed.
	dir := t.TempDir()
	out, status := fcScript(t, dir, histSrc(t, dir, `history=(a b); print -r -- after`))
	if status == 0 {
		t.Errorf("the assignment was taken: status 0, output %q", out)
	}
	if !strings.Contains(out, "history") {
		t.Errorf("the refusal does not name the parameter: %q", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("the script carried on past the refusal: %q", out)
	}
}

func TestTheHistoryTableIsReadWithoutLoadingTheModule(t *testing.T) {
	t.Parallel()
	// Measured: the probe against zsh 5.9.2 above carries no `zmodload
	// zsh/parameter` and answers, which is the state the ten views beside
	// this one are already in. A script that had to load the module first
	// would be a script zsh-autosuggestions is not.
	dir := t.TempDir()
	out, status := fcScript(t, dir, histSrc(t, dir, `print -r -- "${#history}"`))
	if status != 0 {
		t.Fatalf("status %d, output %q", status, out)
	}
	if got := strings.TrimSpace(out); got != "5" {
		t.Errorf("got %q, want 5", got)
	}
}

func TestTheHistoryTableTracksTheListRatherThanSnapshottingIt(t *testing.T) {
	t.Parallel()
	// The rule the whole module is held to: a table filled in once would be
	// correct until the next entry and quietly wrong afterwards, because
	// nothing about its shape changes when it stops tracking.
	dir := t.TempDir()
	out, status := fcScript(t, dir, histSrc(t, dir,
		`print -r -- "before=${#history} ${history[(r)ls*]}"`+"\n"+
			`print -s "ls omega"`+"\n"+
			`print -r -- "after=${#history} ${history[(r)ls*]}"`))
	if status != 0 {
		t.Fatalf("status %d, output %q", status, out)
	}
	// `ls omega` is the newest entry once it is pushed, so what moves is the
	// count and the entry that stops being the current event — measured
	// identically in zsh.
	want := "before=5 ls gamma\nafter=6 ls gamma\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestTheHistoryTableNumbersFromWhatTheSizeHasDropped(t *testing.T) {
	t.Parallel()
	// An entry keeps the number it was given, so a trimmed list's oldest
	// entry is numbered by everything that has left it — fcFirst's rule,
	// asked through the parameter. `${history[1]}` on such a list is nothing
	// rather than the oldest command it still holds.
	dir := t.TempDir()
	out, status := fcScript(t, dir, histSrc(t, dir,
		`HISTSIZE=2`+"\n"+
			`print -r -- "keys=${(k)history} one=${history[1]} five=${history[5]}"`))
	if status != 0 {
		t.Fatalf("status %d, output %q", status, out)
	}
	if got := strings.TrimSpace(out); got != "keys=5 one= five=ls gamma" {
		t.Errorf("got %q", got)
	}
}

func TestALineTheReaderRecordsJoinsTheListTheHistoryTableViews(t *testing.T) {
	// The other half of #4408, and a bug on its own: this dialect handed the
	// Runner a history reader and **no adder**, so
	// [interp.Runner.RecordHistoryEntry] — what a session calls for every
	// accepted line (#4177), and what `fc -s` and `fc -e` call for the line
	// they re-run — returned without doing anything. `fc -l` at a prompt
	// listed nothing the session had run, and this table would have viewed an
	// empty list however well it was written.
	//
	// Asked at the seam rather than through a front end, so that the failure
	// this pins is the missing adder and not a session that did not start.
	// The end-to-end half, where a real prompt fills the list and a widget
	// reads the table, is cmd/zsh/historyparampty_test.go.
	r := bindkeyRunner(t, "")
	r.RecordHistoryEntry("echo first")
	r.RecordHistoryEntry("echo second")
	r.RecordHistoryEntry("echo third")
	if got := r.HistoryEntries(); len(got) != 3 {
		t.Fatalf("the list holds %d entries, want 3: %q", len(got), got)
	}
	f, err := syntax.Parse(`print -r -- "${#history} ${history[(r)echo*]}"`+"\n", zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	r.Stdout, r.Stderr = &out, &out
	if _, err := r.Run(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	// Two of the three: nothing here is a widget, so the newest entry is the
	// event this shell is on and the table leaves it out. The list holding
	// three is what says the adder is wired at all — it held none before.
	if got := strings.TrimSpace(out.String()); got != "2 echo second" {
		t.Errorf("got %q, want %q", got, "2 echo second")
	}
}
