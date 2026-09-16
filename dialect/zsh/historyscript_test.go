// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// The axis itself, asserted beside the behaviour: zsh has an expander and
// does not use it where nobody is typing.
//
// Worth asserting rather than leaving to the transcript below, because the
// transcript cannot reach it — zsh has no `set -o history`, so a zsh script
// can never turn the list on and the axis is never read there at all. A
// preset flipped to Yes would change nothing this shell can be made to do,
// and would change bash's neighbours the moment one of them grew the option.
func TestZshAnswersNoToHistoryExpansionInAScript(t *testing.T) {
	if got := zsh.Semantics().HistoryExpansionInAScript; got != interp.No {
		t.Errorf("HistoryExpansionInAScript is %v, want No", got)
	}
	if got := zsh.Semantics().HistoryExpansion; got != interp.Yes {
		t.Errorf("HistoryExpansion is %v, want Yes — the expander exists, it is the route that does not", got)
	}
}

// zsh has an expander and does not use it in a script, which is the third
// history axis and the reason it is an axis rather than a constant.
//
// Measured 2026-09-16 on zsh 5.9.2: `setopt banghist` is taken, `zsh -o
// banghist` is taken, and `echo !!` after `echo one two three` prints the two
// characters either way. The same shell expands at a prompt, so this is about
// the route and not about the option.
//
// It refuses `set -o history` outright — `no such option: history` — so a
// script written for bash cannot even reach the question here.
func TestZshDoesNotExpandHistoryInAScript(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"setopt banghist", "setopt banghist\necho one two three\necho !!\n"},
		{"the option already on", "echo one two three\necho !!\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			sh := zshShell()
			sh.Stdout, sh.Stderr = &out, &errs
			if code := driver.MainArgs(sh, []string{"zsh", "-c", c.src}); code != 0 {
				t.Fatalf("status %d (stderr %q)", code, errs.String())
			}
			if out.String() != "one two three\n!!\n" {
				t.Errorf("ran %q, want the two characters left alone", out.String())
			}
			if errs.String() != "" {
				t.Errorf("said %q, want nothing — there is no expansion to echo", errs.String())
			}
		})
	}
}
