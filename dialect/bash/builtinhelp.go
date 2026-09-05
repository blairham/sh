// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "strings"

// What a builtin says about its own usage, in the two places bash says it.
//
// The two are one string. Measured 2026-09-05 on bash 5.3.15 over every
// builtin this shell has: the first line of `help NAME` is the line a bad
// option earns from NAME, with the `usage: ` taken out — `alias: usage:
// alias [-p] [name[=value] ... ]` against `alias: alias [-p] [name[=value]
// ... ]`. So the synopsis is written once here and both spellings are
// derived from it, rather than written twice and left to drift.
//
// What follows the synopsis when bash answers `--help` is a paragraph of
// description, a list of the options and an "Exit Status" note. That is
// documentation prose rather than shell behavior, and this tree carries no
// other project's text (CLEANROOM.md), so it is deliberately not reproduced:
// what is implemented is the part a script can act on — that the option is
// recognized at all, that the answer goes to standard output rather than to
// standard error, and that the builtin exits 2.

// builtinUsage is the usage line each builtin's refusal is followed by.
//
// A fresh map per call, as every other map in Diagnostics is: a dialect's
// answers are a value handed to a Runner, and two Runners must not share one
// a third could edit.
func builtinUsage() map[string]string {
	return map[string]string{
		"set":    "set: usage: set [-abefhkmnptuvxBCEHPT] [-o option-name] [--] [-] [arg ...]",
		"export": "export: usage: export [-fn] [name[=value] ...] or export -p [-f]",
		// The six the real-script sweep found missing, and the reason a
		// `/usr/bin/alias` stub looked nothing like bash: every other
		// builtin printed a usage line after a bad option and these did
		// not (#825).
		"alias":   "alias: usage: alias [-p] [name[=value] ... ]",
		"unalias": "unalias: usage: unalias [-a] name [name ...]",
		"cd":      "cd: usage: cd [-L|[-P [-e]]] [-@] [dir]",
		"fc":      "fc: usage: fc [-e ename] [-lnr] [first] [last] or fc -s [pat=rep] [command]",
		"hash":    "hash: usage: hash [-lr] [-p pathname] [-dt] [name ...]",
		"ulimit":  "ulimit: usage: ulimit [-SHabcdefiklmnpqrstuvxPRT] [limit]",
		// And the rest of the builtins this shell has, measured the same
		// way, so the line is not missing from the next one somebody
		// gives a bad option to.
		".":        ".: usage: . [-p path] filename [arguments]",
		"source":   "source: usage: source [-p path] filename [arguments]",
		"builtin":  "builtin: usage: builtin [shell-builtin [arg ...]]",
		"disown":   "disown: usage: disown [-h] [-ar] [jobspec ... | pid ...]",
		"enable":   "enable: usage: enable [-a] [-dnps] [-f filename] [name ...]",
		"eval":     "eval: usage: eval [arg ...]",
		"exec":     "exec: usage: exec [-cl] [-a name] [command [argument ...]] [redirection ...]",
		"pwd":      "pwd: usage: pwd [-LP]",
		"shopt":    "shopt: usage: shopt [-pqsu] [-o] [optname ...]",
		"times":    "times: usage: times",
		"compgen":  "compgen: usage: compgen [-V varname] [-abcdefgjksuv] [-o option] [-A action] [-G globpat] [-W wordlist] [-F function] [-C command] [-X filterpat] [-P prefix] [-S suffix] [word]",
		"complete": "complete: usage: complete [-abcdefgjksuv] [-pr] [-DEI] [-o option] [-A action] [-G globpat] [-W wordlist] [-F function] [-C command] [-X filterpat] [-P prefix] [-S suffix] [name ...]",
		// The refusal lines a bad option earns from these two, measured
		// with a letter nobody has (-q).
		"command": "command: usage: command [-pVv] command [arg ...]",
		"getopts": "getopts: usage: getopts optstring name [arg ...]",
		// Not one line under two names: `typeset` loses the brackets
		// around its name operand where `declare` keeps them. As
		// written, both.
		"declare": "declare: usage: declare [-aAfFgiIlnrtux] [name[=value] ...] or declare -p [-aAfFilnrtux] [name ...]",
		"typeset": "typeset: usage: typeset [-aAfFgiIlnrtux] name[=value] ... or typeset -p [-aAfFilnrtux] [name ...]",
		"local":   "local: usage: local [option] name[=value] ...",
		"read": "read: usage: read [-Eers] [-a array] [-d delim] [-i text] " +
			"[-n nchars] [-N nchars] [-p prompt] [-t timeout] [-u fd] [name ...]",
		"mapfile": "mapfile: usage: mapfile [-d delim] [-n count] [-O origin] [-s count] " +
			"[-t] [-u fd] [-C callback] [-c quantum] [array]",
		"readarray": "readarray: usage: readarray [-d delim] [-n count] [-O origin] [-s count] " +
			"[-t] [-u fd] [-C callback] [-c quantum] [array]",
		"readonly": "readonly: usage: readonly [-aAf] [name[=value] ...] or readonly -p",
		"trap":     "trap: usage: trap [-Plp] [[action] signal_spec ...]",
		"type":     "type: usage: type [-afptP] name [name ...]",
		"jobs":     "jobs: usage: jobs [-lnprs] [jobspec ...] or jobs -x command [args]",
		"wait":     "wait: usage: wait [-fn] [-p var] [id ...]",
		"unset":    "unset: usage: unset [-f] [-v] [-n] [name ...]",
	}
}

// usagePrefix is what a usage line has that the same builtin's synopsis does
// not, in the one dialect that prints both.
const usagePrefix = "usage: "

// builtinHelp is what each builtin answers `--help` with: its synopsis.
//
// Derived from the usage lines above, plus the builtins that have a synopsis
// and no usage line at all — the ones with no options to give a bad one to,
// which is why measuring them takes `--help` itself. `:`, `true`, `false`,
// `test`, `[` and `echo` are absent on purpose: measured, they read `--help`
// as an ordinary word and are the six builtins in this shell that have no
// answer to give.
func builtinHelp() map[string]string {
	help := map[string]string{
		// No usage line to derive one from; measured with `--help`.
		"bg":       "bg: bg [job_spec ...]",
		"fg":       "fg: fg [job_spec]",
		"break":    "break: break [n]",
		"continue": "continue: continue [n]",
		"exit":     "exit: exit [n]",
		"return":   "return: return [n]",
		"shift":    "shift: shift [n]",
		"caller":   "caller: caller [expr]",
		"let":      "let: let arg [arg ...]",
		// Three that keep their usage line in a field of its own, so it is
		// not in the map below either.
		"kill":   "kill: kill [-s sigspec | -n signum | -sigspec] pid | jobspec ... or kill -l [sigspec]",
		"printf": "printf: printf [-v var] format [arguments]",
		"umask":  "umask: umask [-p] [-S] [mode]",
	}
	for name, usage := range builtinUsage() {
		help[name] = strings.Replace(usage, usagePrefix, "", 1)
	}
	return help
}
