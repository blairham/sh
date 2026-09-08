// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/interp"
)

// Completion on an empty command word offers everything that could run, and a
// shell can ask it not to.
//
// The capability bash names `no_empty_cmd_completion`, asserted from the
// direction that catches the mistake: the *default* is the option's off state,
// so a test that only checked "the option is on when set" would pass against a
// shell that had never offered anything. Both halves are here — the empty word
// answers with the whole list, and with the capability off it answers with
// nothing.
func TestAnEmptyCommandWordCanBeWithheld(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runnable"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	offered := shellCompleter{names: []string{"echo", "export"}, path: dir}
	withheld := offered
	withheld.emptyWordOffersNothing = true

	empty := Completion{Command: true}
	got := offered.Complete(empty)
	if len(got) != 3 {
		t.Errorf("an empty command word offered %q, want all three names", got)
	}
	if got := withheld.Complete(empty); got != nil {
		t.Errorf("withheld, an empty command word offered %q, want nothing", got)
	}

	// Only the empty word. A word with a letter in it is completed either
	// way, which is what keeps this an option about listing everything rather
	// than an option that turns command completion off.
	word := Completion{Command: true, Word: "e"}
	if got := withheld.Complete(word); len(got) != 2 {
		t.Errorf("withheld, `e` offered %q, want echo and export", got)
	}

	// And a filename is never withheld: the option is about command
	// completion, and an empty word after a command is a filename.
	if got := withheld.Complete(Completion{Word: ""}); len(got) == 0 {
		t.Errorf("withheld, an empty filename word offered nothing")
	}
}

// The shell decides it, per keystroke, rather than the session settling it
// once.
//
// `shopt -s no_empty_cmd_completion` is a line a person types at the prompt,
// so a completer built with the answer baked in at startup would take effect
// on the next shell. This asks the same completer twice with the Runner moved
// in between.
func TestTheEmptyWordAnswerFollowsTheRunner(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runnable"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &interp.Runner{Dir: dir, Vars: map[string]string{"PATH": dir}}
	c := runnerCompleter{r: r}
	empty := Completion{Command: true}
	if got := c.Complete(empty); len(got) == 0 {
		t.Fatal("an empty command word offered nothing with the capability on")
	}
	r.SetCompletesEmptyCommandWord(false)
	if got := c.Complete(empty); got != nil {
		t.Errorf("after the shell said no, an empty command word offered %q", got)
	}
	r.SetCompletesEmptyCommandWord(true)
	if got := c.Complete(empty); len(got) == 0 {
		t.Error("after the shell said yes again, an empty command word offered nothing")
	}
}
