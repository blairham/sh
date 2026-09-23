// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/interp"

// promptVarsIsOn answers interp.PromptStyle.Expand for this dialect: whether
// a prompt's value goes through parameter expansion, command substitution and
// arithmetic each time it is drawn.
//
// It is `shopt promptvars`, on with nothing said, and the option gates the
// **second** pass only — the backslash language runs either way. Measured
// 2026-09-23 on bash 5.3.15 through a pty, `bash --norc --noprofile -i` under
// `script`, one prompt per run:
//
//	PS1='[$x]> ' with x=XVAL     on: `[XVAL]> `        off: `[$x]> `
//	PS1='[$(echo SUB)]> '        on: `[SUB]> `         off: `[$(echo SUB)]> `
//	PS1='[\u]> '                 on: `[root]> `        off: `[root]> `
//
// The third row is the one that says which pass the option gates, and it is
// why this is PromptStyle.Expand rather than a switch over the whole
// rendering: `\u` is answered with the option off, so the escapes are not the
// option's business. ExpandBeforeEscapes stays false here — bash draws the
// escapes first and expands afterwards, which is the order this dialect
// already measured.
//
// A pty was needed to *measure* it and is not needed to read it: PS4 is
// unaffected — `shopt -u promptvars; PS4='+$x+'; set -x` still expands,
// measured the same day — so the option is PS1's and PS2's alone and there is
// no non-interactive route to the difference in bash either.
//
// The same shape zsh's promptSubstIsOn has, one dialect over, and asked at
// every draw for the reason PromptStyle.Expand gives: the option is one a
// person can type, so an answer read once at startup would be a setting they
// watch do nothing (#4149).
func promptVarsIsOn(r *interp.Runner) bool {
	if r == nil {
		// No shell, so nothing has moved the option, and this one is **on**
		// with nothing said — which is the other way round from zsh's, where
		// a fresh shell has `promptsubst` off.
		return true
	}
	// The stored state directly rather than through shoptState, which would
	// be the tidier read and cannot be had: shoptState consults
	// shoptSwitches, this function *is* a shoptSwitches entry's getter, and Go
	// will not build the cycle. The same shape interp/restricted.go registers
	// around, and the one fact both readers share — the default — is the
	// constant below rather than a repeated literal.
	return shoptStoredState(r, promptVarsName, promptVarsDefault)
}

// promptVarsName is the option's spelling and promptVarsDefault is bash's own
// state for it, named once each because the switch entry and the reader above
// must not drift apart — a default written twice is a listing that disagrees
// with the drawing.
const (
	promptVarsName    = "promptvars"
	promptVarsDefault = true
)
