// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"

	"github.com/blairham/sh/interp"
)

// When the line editor's module loads, which is when its parameter
// `zle_bracketed_paste` comes into being (#5497).
//
// Measured 2026-10-02 on zsh 5.9.2 through a pseudo-terminal:
//
//	-fi                                  there at the first prompt
//	-i +Z, a startup file that binds     there (the file's bindkey loads it)
//	-fiV +Z                              not there; `zmodload -e zsh/zle` is 1
//	-fiV +Z, then `bindkey -l` or `zle -l`  there after it
//	-fiV +Z, then `setopt zle`           not there on that line; the editor
//	                                     writes `ESC[?2004h` at the next prompt
//	-fc 'zmodload zsh/zle'               there
//
// So the module loads once, the first time any of these happens: an
// interactive shell on a terminal starting with the editor's option on, a
// call to one of its builtins, an explicit `zmodload`, or the editor itself
// starting a line. Once is the point — a script that unset the parameter
// does not get it back from the next keystroke.

// zleBooted records that the module has loaded, under a name no script can
// reach, so a subshell has its own copy.
const zleBooted = zshEngineStorePrefix + "zle.booted"

// zleEditorStarted records that the editor itself has run, or is about to at
// startup, which is what brings `zsh/complete` with it: under `-fiV +Z` a
// `bindkey` loads `zsh/zle` and not `zsh/complete`, and `setopt zle` then the
// next line loads both (#5524).
const zleEditorStarted = zshEngineStorePrefix + "zle.started"

// lineEditorStarted is zleEditorStarted read.
func lineEditorStarted(r *interp.Runner) bool {
	v, _ := r.GetVar(zleEditorStarted)
	return v != ""
}

// bootLineEditor loads the line editor's module, if it has not been loaded.
func bootLineEditor(r *interp.Runner) {
	if v, _ := r.GetVar(zleBooted); v != "" {
		return
	}
	r.SetVar(zleBooted, "1")
	r.SetArray(bracketedPasteParameter, []string{"\x1b[?2004h", "\x1b[?2004l"})
}

// BeforeStartupFiles is the shell with its options settled and no startup
// file read yet: where an interactive shell on a terminal with the editor's
// option on has loaded the editor's module. See driver.Shell.BeforeStartupFiles.
func BeforeStartupFiles(r *interp.Runner) {
	// A terminal called `emacs` has no line editor, decided before the
	// startup files as an assignment to `$TERM` decides it after them. See
	// editorOffInsideEmacs.
	if term, _ := r.GetVar("TERM"); r.Interactive {
		editorOffInsideEmacs(r, term)
	}
	if r.Interactive && r.Terminal && editorOptionOn(r) {
		bootLineEditor(r)
		r.SetVar(zleEditorStarted, "1")
	}
}

// StartLine is what this dialect does as a new line begins: the editor, where
// it runs, has loaded its module, and the line's highlighting starts empty.
func StartLine(r *interp.Runner) {
	if r.Interactive && r.Terminal && editorOptionOn(r) {
		bootLineEditor(r)
		r.SetVar(zleEditorStarted, "1")
	}
	ResetRegionHighlight(r)
}

// editorOptionOn is the `zle` option.
func editorOptionOn(r *interp.Runner) bool {
	on, _ := r.DialectOption("zle")
	return on
}

// lineEditorBooted says the line editor's module has loaded.
func lineEditorBooted(r *interp.Runner) bool {
	v, _ := r.GetVar(zleBooted)
	return v != ""
}

// unbootLineEditor is `zmodload -u zsh/zle`: the next use loads it again, and
// makes its parameter again with it.
func unbootLineEditor(r *interp.Runner) { r.SetVar(zleBooted, "") }

// editorOffInsideEmacs turns the `zle` option off in an interactive shell
// whose terminal is called `emacs`, which is what zsh does whenever `$TERM`
// takes that value: at startup, and at every assignment after it. Turning
// `$TERM` back does not turn the option back on, and `setopt zle` does.
//
// Measured 2026-10-07 on zsh 5.9.2 through a pseudo-terminal, `[[ -o zle ]]`
// after each:
//
//	TERM=emacs at startup                    off
//	TERM=emacs-foo, or EMACS=t / INSIDE_EMACS
//	  on TERM=dumb or with TERM unset        on — the name alone
//	TERM=emacs typed at an xterm prompt      off
//	TERM=emacs in .zshrc                     off
//	TERM=emacs true, f() { local TERM=emacs; } off — any assignment
//	TERM=emacs at startup, then TERM=xterm   off
//	TERM=emacs at startup, then setopt zle   on, and the editor draws
//
// So the session reads its lines with the terminal's own echo, and writes
// none of the editor's sequences (#6315).
func editorOffInsideEmacs(r *interp.Runner, term string) {
	if term != "emacs" {
		return
	}
	if unset, ok := r.Builtin("unsetopt"); ok {
		unset(r, context.Background(), []string{"zle"})
	}
}
