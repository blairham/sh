// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// At a prompt the quoting styles quote the history character, so what they
// write can be typed back without an expansion firing; in a script they do
// not (#5933).
//
// Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh) under `env -u
// FPATH`, scratch `HOME`=`ZDOTDIR`: `zsh -i` with the lines on a pipe against
// `zsh -f` on the same lines as a script. This shell wrote the script's
// answer at the prompt too.
func TestTheQuotingFlagsQuoteTheHistoryCharacterAtAPrompt(t *testing.T) {
	const lines = `c='a!b'; print -r -- ${(q)c} ${(q-)c} ${(q+)c} ${(qq)c} ${(qqq)c} ${(qqqq)c} ${(b)c} ${c:q}
c='it'"'"'s!'; print -r -- ${(q)c} ${(q-)c} ${(qqq)c}
print -r -- $(print -r -- ${(q)c}); printf '%q\n' 'a!b'
histchars='@^#'; c='a@b!'; print -r -- ${(q)c} ${(q-)c} ${(qqq)c} ${(qqqq)c}
setopt nobanghist; c='a!b@'; print -r -- ${(q)c} ${(qqqq)c}
`
	atPrompt := `a\!b 'a!b' 'a!b' 'a!b' "a\!b" $'a\!b' a!b a\!b
it\'s\! it\''s!' "it's\!"
it\'s\!
a\!b
a\@b! 'a@b!' "a\@b!" $'a\@b!'
a!b@ $'a!b@'
`
	inAScript := `a!b a!b a!b 'a!b' "a!b" $'a\!b' a!b a!b
it\'s! it\'s! "it's!"
it\'s!
a!b
a@b! a@b! "a@b!" $'a\@b!'
a!b@ $'a!b@'
`
	scratchHome(t)
	out, errs, _ := prompt(t, lines+"exit\n", "zsh", "-i")
	if out != atPrompt {
		t.Errorf("at a prompt:\n%s\nwant:\n%s\nstderr %q", out, atPrompt, errs)
	}

	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	sh.Stdin = strings.NewReader("")
	driver.MainArgs(sh, []string{"zsh", "-f", "-c", lines})
	if o.String() != inAScript {
		t.Errorf("under -c:\n%s\nwant:\n%s\nstderr %q", o.String(), inAScript, e.String())
	}

	// And `setopt banghist` in a script turns the expander's switch on
	// without there being a prompt, which still quotes nothing: measured,
	// `a!b a!b`.
	o.Reset()
	e.Reset()
	sh = scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	sh.Stdin = strings.NewReader("")
	driver.MainArgs(sh, []string{"zsh", "-f", "-c", `setopt banghist; c='a!b'; print -r -- ${(q)c} ${(q-)c}`})
	if want := "a!b a!b\n"; o.String() != want {
		t.Errorf("setopt banghist under -c: %q, want %q (stderr %q)", o.String(), want, e.String())
	}
}
