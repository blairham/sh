// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild_test

import (
	"context"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/wild"
)

// Two scripts that fail on the same construct are one finding. The grouping
// is the whole point of the report: a reader looking at a list of paths cannot
// tell forty problems from one, and the sweep is the only thing that can.
func TestCausesGroupsAndRanks(t *testing.T) {
	dir := t.TempDir()
	// Three scripts sharing one gap, and one with a different gap.
	for _, name := range []string{"a", "b", "c"} {
		// A construct this parser does not have: that shell parses **any**
		// `-word` with an operand as a unary condition and refuses an
		// unknown one when it runs, which is the rule
		// docs/spec/grammar/conditions.md declines to adopt — it is #965,
		// and it is open.
		//
		// It was `[[ $k == (x|y) ]]` until #826 implemented that, and
		// `[[ -prefix - ]]` until #1879 added that operator by name — the
		// same hazard the second fixture's comment is about, reached from
		// the other side. Whatever replaces this one has to be something
		// real zsh reads and this parser still refuses **at an ordinary
		// word**, so that its reason does not collapse onto the reserved
		// word the second fixture is refused at.
		write(t, dir, name, "#!/bin/zsh\n[[ -nosuch - ]]\n")
	}
	// A different cause, and the seventh fixture to stand here. The six
	// before it — `repeat 3 { echo x; }` until #827 implemented that,
	// `echo (aa|bb)` until #995 made a bare group where a *word* stands part
	// of the word, `if true; then; fi` until #1142 let a `;` stand where a
	// command belongs, `case x in a) : ;| …` until #1188 read that
	// terminator, `cat =(echo hi)` until #1878 implemented the temp-file
	// process substitution, `case x { x) echo hit;; }` until #1928 read the
	// brace-spelled header, and `if (( 1 )) { echo A } fi` until #2242 read
	// the redundant terminator — were each chosen to be a **live gap**, and
	// every one of them was overtaken by the thing it stood in for. That is
	// not bad luck. A live gap is by definition something somebody is going
	// to close, so choosing one guarantees the replacement.
	//
	// So this one is chosen the other way round: it is a **syntax error in
	// every shell in the panel**, measured on zsh 5.9.2 — `if (( 1 )) { echo
	// A } fi fi` is ``parse error near `fi'`` there and here alike — and
	// nothing will ever implement it. What this test needs from a fixture is
	// a *second distinct reason*, not a second open issue, and the sweep's
	// one filter that would have excluded it from a real report — a file the
	// reference shell also refuses is not counted — is stubbed out below with
	// `/usr/bin/true`, which refuses nothing.
	//
	// It is the control row of #2242 rather than an arbitrary error, so it
	// earns a second keep: exactly one redundant `fi` closes a short `if`
	// with no `else`, and a widening that took a run of them would fail here.
	//
	// The two causes also have to be refused at *different* token classes,
	// which is the constraint that decides the fixture and is easy to miss.
	// Reason keeps an operator or a reserved word and replaces an ordinary
	// word with "a word", so `{ … } always { … }` is no use here: it is
	// refused at `always`, which is an ordinary word to this grammar and
	// collapses onto the same reason as the `-` of the first three. The
	// second `fi` is a reserved word, which is kept, so it has a reason of
	// its own and the report has the two causes this test is about.
	write(t, dir, "d", "#!/bin/zsh\nif (( 1 )) { echo A } fi fi\n")

	rep := wild.Sweep(context.Background(), wild.Scope{Dirs: []string{dir}, Shells: wild.ZshScope},
		zsh.Dialect(), "/usr/bin/true")
	if len(rep.Failures) != 4 {
		t.Fatalf("failures = %d, want 4: %+v", len(rep.Failures), rep.Failures)
	}

	causes := wild.Causes(rep.Failures)
	if len(causes) != 2 {
		t.Fatalf("causes = %d, want 2: %+v", len(causes), causes)
	}
	if causes[0].Count() != 3 {
		t.Errorf("the largest cause accounts for %d scripts, want 3", causes[0].Count())
	}
	if causes[1].Count() != 1 {
		t.Errorf("the second cause accounts for %d scripts, want 1", causes[1].Count())
	}
	// A cause carries a line a person can look at, not only a category.
	if causes[0].Example.Text == "" {
		t.Error("the example carries no source line")
	}
}

// A reason keeps what came from the grammar's vocabulary and drops what came
// from the script's, so that two scripts refused at different identifiers are
// one cause rather than two.
func TestReasonDropsWhatDiffersPerScript(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "one", "#!/bin/zsh\nif true { install_deps; }\n")
	write(t, dir, "two", "#!/bin/zsh\nif true { build_all; }\n")

	rep := wild.Sweep(context.Background(), wild.Scope{Dirs: []string{dir}, Shells: wild.ZshScope},
		zsh.Dialect(), "/usr/bin/true")
	if len(rep.Failures) != 2 {
		t.Fatalf("failures = %d, want 2: %+v", len(rep.Failures), rep.Failures)
	}
	if got := wild.Causes(rep.Failures); len(got) != 1 {
		t.Errorf("causes = %d, want 1 — the two differ only in a name: %+v", len(got), got)
	}
	// And the position is gone, so the same construct on two different lines
	// is still one cause.
	if a, b := wild.Reason(rep.Failures[0].Err), wild.Reason(rep.Failures[1].Err); a != b {
		t.Errorf("reasons %q and %q differ", a, b)
	}
}

// The line a failure sits on is the closest thing to a minimal reproduction
// the sweep can honestly offer, so it has to survive being asked for out of
// range rather than taking the report down with it.
func TestLineAt(t *testing.T) {
	const src = "first\n  second  \nthird"
	for _, tc := range []struct {
		line int
		want string
	}{
		{1, "first"},
		{2, "second"},
		{3, "third"},
		{0, ""},
		{-1, ""},
		{4, ""},
	} {
		if got := wild.LineAt(src, tc.line); got != tc.want {
			t.Errorf("LineAt(%d) = %q, want %q", tc.line, got, tc.want)
		}
	}
}
