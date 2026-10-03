// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

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
	if r.Interactive && r.Terminal && editorOptionOn(r) {
		bootLineEditor(r)
	}
}

// StartLine is what this dialect does as a new line begins: the editor, where
// it runs, has loaded its module, and the line's highlighting starts empty.
func StartLine(r *interp.Runner) {
	if r.Interactive && r.Terminal && editorOptionOn(r) {
		bootLineEditor(r)
	}
	ResetRegionHighlight(r)
}

// editorOptionOn is the `zle` option.
func editorOptionOn(r *interp.Runner) bool {
	on, _ := r.DialectOption("zle")
	return on
}
