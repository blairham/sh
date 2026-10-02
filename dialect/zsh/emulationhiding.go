// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// hideTheParametersAnEmulationDoesNotHave gives up, at startup, the parameters
// this shell creates only when it starts as itself (#5157).
//
// Measured 2026-10-01 on zsh 5.9.2 under `-f`: started as `sh` or `ksh` — by
// `--emulate` or by the name it was invoked as — the shell has no `ARGC`,
// `argv`, `status`, `pipestatus`, `prompt` or `PROMPT`…`PROMPT4`, no
// `HISTCHARS`, `WATCH` or `MANPATH`, none of the lower-case arrays tied to
// `PATH` and its siblings, and none of the zsh/parameter tables: `${+argv}`
// is 0 for every one of them and no listing names one. The upper-case halves
// of the ties stay, as plain scalars — `${(t)PATH}` is `scalar-export-special`
// where it is `scalar-tied-export-special` — and `KEYTIMEOUT`, `LISTMAX` and
// `MAILCHECK` lose their integer letter. Started as `csh` only the module
// tables, the watch pair and the scheduled events go — its ties and letters
// stay — and started as `zsh` nothing moves. A later `emulate zsh` brings none
// of it back: the parameters are a fact about how the shell started and not
// about the mode it is in.
func hideTheParametersAnEmulationDoesNotHave(r *interp.Runner) {
	if r.InvocationEmulation == "" {
		return
	}
	mode := emulationForWord(r, r.InvocationEmulation)
	if mode == "zsh" {
		return
	}
	for _, name := range parametersOnlyZshStartsWith {
		r.ForgetParameter(name)
	}
	if mode == "csh" {
		// csh keeps the rest, ties and letters and all: `${(t)PATH}` is
		// `scalar-tied-export-special` there and `${(t)KEYTIMEOUT}` is
		// `integer`.
		return
	}
	for _, pair := range builtInTies {
		r.Untie(pair[0])
	}
	// And the eval-context pair, whose upper half stays as a plain readonly
	// scalar: `${(t)ZSH_EVAL_CONTEXT}` is `scalar-readonly-special`.
	r.Untie("ZSH_EVAL_CONTEXT")
	for _, name := range parametersOnlyZshAndCshStartWith {
		r.ForgetParameter(name)
	}
	for _, name := range [...]string{"KEYTIMEOUT", "LISTMAX", "MAILCHECK"} {
		r.UnmarkInteger(name)
	}
}

// parametersOnlyZshStartsWith is what a shell started as anything but itself
// lacks — as measured, a bare `typeset +` started as itself against the same
// started as `csh`, every name the second lacks: the module tables, the
// watch pair and the scheduled events.
var parametersOnlyZshStartsWith = [...]string{
	"WATCH", "watch", "zsh_scheduled_events",
	"aliases", "builtins", "commands", "dirstack",
	"dis_aliases", "dis_builtins", "dis_functions", "dis_functions_source", "dis_galiases",
	"dis_patchars", "dis_reswords", "dis_saliases",
	"funcfiletrace", "funcsourcetrace", "funcstack", "functions",
	"functions_source", "functrace", "galiases", "history", "historywords",
	"jobdirs", "jobstates", "jobtexts", "keymaps", "modules", "nameddirs",
	"options", "parameters", "patchars", "reswords", "saliases", "termcap", "terminfo",
	"userdirs", "usergroups", "widgets",
}

// parametersOnlyZshAndCshStartWith is what started as `sh` or `ksh` lacks
// besides: the same comparison against `sh`, less the list above.
var parametersOnlyZshAndCshStartWith = [...]string{
	"ARGC", "HISTCHARS", "MANPATH", "PROMPT", "PROMPT2", "PROMPT3", "PROMPT4",
	"argv", "cdpath", "fignore", "fpath", "mailpath", "manpath", "module_path",
	"path", "pipestatus", "prompt", "psvar", "status", "zsh_eval_context",
}
