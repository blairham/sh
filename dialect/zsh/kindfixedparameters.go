// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The names this shell holds in a slot of a fixed kind, and the kinds each
// slot will take.
//
// `f() { typeset -i path }` is “f:typeset: path: can't change type of a
// special parameter“, the shell **ends** at 1, and until this every one of
// them was taken here at 0 (#4834). The mechanism, the sweep and the rule the
// four groups below are an instance of are in interp/parameterkindfixed.go;
// what this file holds is the lists, because the names are this dialect's and
// nothing in the core may know one.
//
// Swept 2026-09-27 against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, one run per cell, `-f` from a script file
// under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`: 155 candidate
// names — every name `typeset +` lists in a fresh shell, plus every parameter
// the manual's *Parameters Used By The Shell* section names that such a shell
// leaves unset — over the five kind letters under both signs.
//
// **The taken column is the control and it is the larger one.** 77 of the 155
// take every kind letter under either sign: `PWD`, `OLDPWD`, `HOST`,
// `OSTYPE`, `MACHTYPE`, `CPUTYPE`, `VENDOR`, `LOGNAME`, `TTY`, `ZSH_NAME`,
// `ZSH_VERSION`, `ZSH_PATCHLEVEL`, `ZSH_ARGZERO`, `EPOCHSECONDS`,
// `EPOCHREALTIME`, `epochtime`, `HISTFILE`, `MAILCHECK`, `KEYTIMEOUT`,
// `LISTMAX`, `TIMEFMT`, `TMPPREFIX`, `dirstack`, `watch`, `WATCH`, and every
// parameter the `zsh/parameter` module provides — `options`, `functions`,
// `aliases`, `commands`, `jobstates` and the rest. So this is not "every
// parameter the shell provides" and not "every parameter with a producer
// behind it": `EPOCHSECONDS` is produced and takes `typeset -A`.
//
// **And it is not interp/parameterscopefixed.go's set**, which is the reason
// this file exists rather than a second walk of that one: `RANDOM`,
// `SECONDS`, `LINENO`, `UID`, `status` and `COLUMNS` all refuse `private` and
// all take `typeset -i`. Reusing that table here would refuse six names this
// shell takes. The overlap is large and the difference is the point.
func markTheKindFixedParameters(r *interp.Runner) {
	for _, name := range arraySlotParameters {
		r.MarkParameterKindFixed(name, interp.ArrayParameter)
	}
	for _, name := range integerSlotParameters {
		r.MarkParameterKindFixed(name, interp.IntegerParameter)
	}
	// The four names that say who the process is, walked rather than
	// repeated — all four are integer slots, measured in the same sweep.
	for _, name := range identityParameters {
		r.MarkParameterKindFixed(name, interp.IntegerParameter)
	}
	for _, name := range scalarSlotParameters {
		r.MarkParameterKindFixed(name, interp.ScalarParameter)
	}
	// Both halves of all eight built-in ties, which is the fourth walk of
	// that table and for the same reason as the others: a pair added to it
	// is a pair this shell hardwires. The scalar half is a scalar slot and
	// the array half an array slot, and every one of the sixteen refuses
	// every other kind letter. A tie a *script* makes does not — `typeset -T
	// TT tt` in front, and `typeset -i tt` inside a function is 0 — which is
	// why this reads the table and not r.Tie.
	for _, pair := range builtInTies {
		r.MarkParameterKindFixed(pair[0], interp.ScalarParameter)
		r.MarkParameterKindFixed(pair[1], interp.ArrayParameter)
	}
	// And the one name whose slot takes two kinds. A float `SECONDS` is a
	// real thing in this shell, so `typeset -F SECONDS` and `typeset -E
	// SECONDS` are both 0 and `${(t)SECONDS}` afterwards is `float-special`
	// — where `typeset -F RANDOM` is refused. That is the row that says the
	// rule is about the *slot* and not about the kind the name is holding,
	// since `SECONDS` is an integer until something says otherwise.
	r.MarkParameterKindFixed("SECONDS", interp.IntegerParameter, interp.FloatParameter)
}

// arraySlotParameters take `-a` and refuse every other kind letter, and
// refuse `+a` besides: taking the array attribute off would leave the name a
// kind its slot has not got.
//
// The eight array halves of the built-in ties are walked separately above.
var arraySlotParameters = [...]string{
	"argv",
	"pipestatus",
	"zsh_eval_context",
}

// integerSlotParameters take `-i` and refuse every other kind letter, and
// refuse `+i`.
//
// `RANDOM` and `LINENO` are here and are the pair that rules out reading this
// off a producer: both are produced on being read, and so is `EPOCHSECONDS`,
// which takes everything.
var integerSlotParameters = [...]string{
	"ARGC",
	"COLUMNS",
	"FUNCNEST",
	"HISTCMD",
	"HISTSIZE",
	"LINENO",
	"LINES",
	"OPTIND",
	"PPID",
	"RANDOM",
	"SAVEHIST",
	"SHLVL",
	"TRY_BLOCK_ERROR",
	"TRY_BLOCK_INTERRUPT",
	"TTYIDLE",
	"ZLE_RPROMPT_INDENT",
	"ZSH_SUBSHELL",
	"status",
}

// scalarSlotParameters refuse every kind letter under a minus and take every
// one of them under a plus, which is the whole of what "a scalar slot" means
// here: no letter spells a scalar, and taking a kind the name has not got
// away changes nothing.
//
// The eight scalar halves of the built-in ties are walked separately above.
var scalarSlotParameters = [...]string{
	"HISTCHARS",
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
	"NULLCMD",
	"OPTARG",
	"POSTEDIT",
	"PROMPT",
	"PROMPT2",
	"PROMPT3",
	"PROMPT4",
	"PS1",
	"PS2",
	"PS3",
	"PS4",
	"READNULLCMD",
	"RPROMPT",
	"RPROMPT2",
	"RPS1",
	"RPS2",
	"SPROMPT",
	"TERM",
	"TERMINFO",
	"TERMINFO_DIRS",
	"USERNAME",
	"WORDCHARS",
	"ZSH_EVAL_CONTEXT",
	"_",
	"histchars",
	"prompt",
}
