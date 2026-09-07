// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/repl"

// HookStyle is what this shell runs between one command and the next.
//
// It is the only dialect in the panel with any: bash has `PROMPT_COMMAND`,
// which fires at the same moment and is a different mechanism — command text
// evaluated, not function names called, and with no `preexec` half at all — so
// it is not spelled here. dash and ksh93 have neither.
//
// The names are this shell's own two, and the list is the hook's name with
// `_functions` after it. That suffix is not decoration: `add-zsh-hook precmd f`
// leaves no function called `precmd` behind, it appends `f` to
// `precmd_functions`, which is why a session that read only the named function
// would find a correctly registered hook and run nothing — #1281 exactly.
//
// CommandLayout is the same arrangement `typeset -f` lists a function in, and
// it is the same for a reason rather than by reuse: the third argument to
// `preexec` is the command about to run written back out over as many lines as
// it takes, and a shell has one way of writing a tree back. Measured on
// 2026-09-07 through a pseudo-terminal, `for i in 1 2; do echo $i; done` typed
// at a zsh 5.9.2 prompt reaches `preexec` as
//
//	for i in 1 2
//	do
//		echo $i
//	done
//
// which is `do` on a line of its own, a tab of indent and no terminators —
// FunctionLayout, field for field.
//
// # What this shell does not fire, and says so
//
// `chpwd`, `periodic`, `zshaddhistory` and `zshexit` take the named function
// and the `_functions` array exactly as `precmd` does — measured, with
// `precmd_functions=(pcA nosuchfn_zz pcB)` and the same shape for each: the
// undefined name was passed over in silence and `pcB` still ran. So the
// *chain* is one mechanism and repl's fireHook serves all six.
//
// The *sites* are four more, and none of them is the prompt loop. `chpwd`
// fires where the directory changed — measured, `cd /tmp` ran `chpwd` and then
// `chpwd_functions` before that line's `precmd`, so it belongs inside `cd` and
// not at the prompt, where it would also miss a `cd` inside a function.
// `periodic` fires on a timer read from `$PERIOD`. `zshaddhistory` fires where
// a line is saved and is handed the raw line, newline included, with the power
// to reject it by returning non-zero. `zshexit` fires on the way out.
//
// Naming them is the point of listing them. A hook that is registered and
// never called is the failure this file exists to fix; one that is registered
// and never called *quietly* is the same failure one level down.
func HookStyle() repl.HookStyle {
	return repl.HookStyle{
		BeforePrompt:  "precmd",
		BeforeCommand: "preexec",
		ListSuffix:    "_functions",
		CommandLayout: FunctionLayout(),
		Unfired:       []string{"chpwd", "periodic", "zshaddhistory", "zshexit"},
	}
}
