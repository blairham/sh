// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The names this shell will not let `private` move into a function's scope.
//
// `f() { private path }` is “f:private: can't change scope of existing
// param: path“ at status 1 with the script carrying on, where `f() { private
// v }` is 0 — and until this every one of them was taken here (#4802). The
// mechanism and the measurement that says the set is a list rather than a
// rule are in interp/parameterscopefixed.go; what this file holds is the
// list, because the names are this dialect's and nothing in the core may know
// one.
//
// Swept 2026-09-27 against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, one name per run, `-f` from a script file
// under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`, behind `zmodload
// zsh/param/private`. 177 candidates — every name `typeset +` lists in a
// fresh shell, plus every parameter the manual's *Parameters Used By The
// Shell* section names that such a shell leaves unset — of which **80 refuse
// and 97 are taken**.
//
// The taken column is the control and it is not short: `PWD`, `OLDPWD`,
// `HOST`, `OSTYPE`, `MACHTYPE`, `CPUTYPE`, `VENDOR`, `LOGNAME`, `TTY`,
// `ZSH_NAME`, `ZSH_VERSION`, `ZSH_PATCHLEVEL`, `ZSH_ARGZERO`, `ZSH_SCRIPT`,
// `MAILCHECK`, `KEYTIMEOUT`, `LISTMAX`, `TIMEFMT`, `TMPPREFIX`, `HISTFILE`,
// `TMOUT`, `ZDOTDIR`, `BAUD`, `DIRSTACKSIZE`, `FCEDIT`, `ENV`, `STTY`,
// `REPORTTIME`, `WATCHFMT`, `LOGCHECK`, `MAIL`, `dirstack`, every parameter
// the `zsh/parameter` module provides — `options`, `functions`, `aliases`,
// `commands` and the rest — and, tellingly, `WATCH` and `watch`, which
// `${(t)}` calls `scalar-special` and `array-special` and which are taken all
// the same. A script's own tie is taken too: `typeset -T TT tt` in front, and
// `private tt` and `private TT` inside a function are both 0.
func markTheScopeFixedParameters(r *interp.Runner) {
	for _, name := range scopeFixedParameters {
		r.MarkParameterScopeFixed(name)
	}
	// The four names that say who the process is, walked rather than repeated
	// — all four refuse, measured in the same sweep.
	for _, name := range identityParameters {
		r.MarkParameterScopeFixed(name)
	}
	// And both halves of all eight built-in ties, which is the third walk of
	// that table and for the same reason as the second: a pair added to it is
	// a pair this shell hardwires, and every one of the sixteen refuses. A
	// tie a *script* makes does not, which is why this reads the table and
	// not r.Tie.
	for _, pair := range builtInTies {
		r.MarkParameterScopeFixed(pair[0])
		r.MarkParameterScopeFixed(pair[1])
	}
}

// scopeFixedParameters is the rest of the measured set, in the order a sweep
// over sorted names produced it.
//
// Written out rather than derived. Three readings the sweep rules out, each
// with its counterexample in the list above or in this one: not "every
// parameter the shell provides" (`PWD` is provided and taken), not "every
// parameter with a producer" (`HOME` and `IFS` hold stored values and
// refuse), and not "every parameter that is set" — `TERM`, `LANG`, the six
// `LC_*`, `RPROMPT`, `RPS1`, `RPROMPT2`, `RPS2`, `POSTEDIT`, `TERMINFO`,
// `TERMINFO_DIRS` and `ZLE_RPROMPT_INDENT` have no value at all in a `-f`
// shell under `env -i` and every one of them refuses. The slot is there
// whether or not a value is, which is also why `unset path; private path`
// still refuses.
var scopeFixedParameters = [...]string{
	"ARGC",
	"COLUMNS",
	"ERRNO",
	"FUNCNEST",
	"HISTCHARS",
	"HISTCMD",
	"HISTSIZE",
	"HOME",
	"IFS",
	"KEYBOARD_HACK",
	"LANG",
	"LC_ALL",
	"LC_COLLATE",
	"LC_CTYPE",
	"LC_MESSAGES",
	"LC_NUMERIC",
	"LC_TIME",
	"LINENO",
	"LINES",
	"NULLCMD",
	"OPTARG",
	"OPTIND",
	"POSTEDIT",
	"PPID",
	"PROMPT",
	"PROMPT2",
	"PROMPT3",
	"PROMPT4",
	"PS1",
	"PS2",
	"PS3",
	"PS4",
	"RANDOM",
	"READNULLCMD",
	"RPROMPT",
	"RPROMPT2",
	"RPS1",
	"RPS2",
	"SAVEHIST",
	"SECONDS",
	"SHLVL",
	"SPROMPT",
	"TERM",
	"TERMINFO",
	"TERMINFO_DIRS",
	"TRY_BLOCK_ERROR",
	"TRY_BLOCK_INTERRUPT",
	"TTYIDLE",
	"USERNAME",
	"WORDCHARS",
	"ZLE_RPROMPT_INDENT",
	"ZSH_EVAL_CONTEXT",
	"ZSH_SUBSHELL",
	"_",
	"argv",
	"histchars",
	"pipestatus",
	"prompt",
	"status",
	"zsh_eval_context",
}
