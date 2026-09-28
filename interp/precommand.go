// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// PrecommandModifier says what a word standing in front of a command does to
// the command behind it.
//
// The class exists because one shell in the panel has words that are neither
// commands nor grammar: they are *builtins*, so an expansion can produce one
// and quoting does not take the reservation away, and they are read before the
// words behind them are matched against the filesystem, so what they change is
// an earlier stage than a builtin normally reaches.
//
// Measured 2026-09-08 against zsh 5.9.2, which is where every fact below comes
// from. `whence -w` sorts the family in one line:
//
//	noglob     builtin      switches pathname expansion off for this command
//	nocorrect  reserved     grammar — see syntax.Dialect.ReservedPrecommands
//	command    builtin      stops the scan: what follows is an external name
//	builtin    builtin      transparent — the scan carries on past it
//	exec       builtin      transparent
//	-          builtin      the dash — see PrecommandDash
//
// The three shared names are ordinary builtins here and stay that way. What
// this table adds is only what a *scan* of the leading words has to know, and
// the two answers it needs are the two constants below. Nothing in it is a
// code path: a dialect names words and says which of the two each one is.
//
// The scan is the reason `command` and `builtin` part company, which is
// measured rather than reasoned: `builtin noglob echo a[b]c` and `exec noglob
// echo a[b]c` both print `a[b]c`, and `command noglob echo a[b]c` is `no
// matches found: a[b]c` — the pattern was matched, so the scan had already
// stopped at `command`.
type PrecommandModifier uint8

const (
	// PrecommandNoGlob switches pathname expansion off for the rest of this
	// command, and is taken away. Only pathname expansion, and only for the
	// words of this command: `noglob f` where `f` globs in its body still
	// reports `f: no matches found`, and `noglob eval 'echo a[b]c'` still
	// reports the pattern from inside the eval.
	//
	// Not the redirections. Measured, and it is the one part of a command
	// the modifier does not reach: `noglob echo x >out[1].txt` is `no
	// matches found: out[1].txt` with the modifier exactly as without it.
	PrecommandNoGlob PrecommandModifier = iota + 1
	// PrecommandTransparent is a word the scan reads past without the word
	// itself doing anything to the scan. It stays in the command — it is a
	// builtin with work of its own to do — and a modifier may stand behind
	// it.
	PrecommandTransparent
	// PrecommandDash runs the command with a `-` on the front of its argv[0],
	// and is taken away. The word is exactly `-`.
	//
	// **It is a modifier and not a word thrown away**, which is the whole of
	// #5018 and is one line to see: `- /bin/sh -c 'printf "[%s]" "$0"'`
	// writes `[-/bin/sh]` in zsh 5.9.2 where a discard would write
	// `[/bin/sh]`. Every probe that reaches the command through `echo`
	// agrees with the discard, because `echo hi` prints `hi` either way — a
	// grid keyed on *how the dash arrives* can be seven rows wide and never
	// vary whether argv[0] is observable at all.
	//
	// Measured 2026-09-28 from script files under `env -i PATH=/usr/bin:/bin`
	// with a scratch HOME, against /opt/homebrew/bin/zsh — zsh 5.9.2
	// (aarch64-apple-darwin25.4.0), `go version -m`: *not a Go executable*.
	// Every row runs `/bin/sh -c 'printf "[%s]" "$0"'`:
	//
	//	/bin/sh …           [/bin/sh]    the control
	//	- /bin/sh …         [-/bin/sh]
	//	- sh …              [-sh]        the word as written, not the path
	//	- - /bin/sh …       [-/bin/sh]   one dash however many were written
	//	- - - /bin/sh …     [-/bin/sh]
	//	- command /bin/sh … [-/bin/sh]
	//	- exec /bin/sh …    [-/bin/sh]
	//	noglob - /bin/sh …  [-/bin/sh]
	//	- noglob /bin/sh …  [-/bin/sh]   a modifier behind it is still read
	//	exec - /bin/sh …    [-/bin/sh]   and it is read behind one
	//
	// The doubled row is what says the dash is a property of the invocation
	// rather than a character prepended per modifier, and the `- sh` row is
	// what says it goes on the word and not on what PATH resolved it to.
	//
	// It reaches an external command alone. `- f` where `f` is a function
	// reports `$0` as `f`, and `- :` runs the builtin with nothing to dash.
	//
	// **And it switches off the option scan of every modifier behind it**,
	// which is #5028 and is a second fact rather than a consequence of the
	// first. The same rows:
	//
	//	exec -l /bin/sh …            [-/bin/sh]               the control
	//	exec -a zz /bin/sh …         [zz]                     …
	//	command -p /bin/sh …         [/bin/sh]                …
	//	noglob exec -l /bin/sh …     [-/bin/sh]               …
	//	- exec -l /bin/sh …          command not found: -l, 127
	//	- exec -a zz /bin/sh …       command not found: -a, 127
	//	- command -p /bin/sh …       command not found: -p, 127
	//	- exec -- /bin/sh …          command not found: --, 127
	//	noglob - exec -l /bin/sh …   command not found: -l, 127
	//	exec - exec -l /bin/sh …     command not found: -l, 127
	//	exec -l exec -a zz /bin/sh … [zz]
	//
	// `command not found: -l` rather than a usage error is what says the
	// word became the *command name*: the option scan did not run at all,
	// rather than running and refusing. The four controls are what make it
	// one rule and not four — each letter reads perfectly well on its own,
	// and `noglob exec -l` still reads it, so it is this word and not "a
	// modifier in front of a modifier".
	//
	// **The noun is the word `-` and not the dash it asks for**, which the
	// last row is the discriminating case for: `exec -l` asks for the same
	// dash on argv[0] and leaves the scan behind it reading options
	// normally. Written the other way round, `exec - exec -l` refuses —
	// so what decides is which of the two spellings arrived, not what
	// either of them did.
	//
	// It reaches only the modifiers. An ordinary builtin behind the dash
	// reads its own options as it always did: `- echo -n hi` writes `hi`
	// with no newline, and `- print -n hi` the same.
	PrecommandDash
)

// SetPrecommand makes a word a precommand modifier for this runner. A dialect
// names the words it has; a runner with none scans nothing.
func (r *Runner) SetPrecommand(name string, m PrecommandModifier) {
	if r.precommands == nil {
		r.precommands = map[string]PrecommandModifier{}
	}
	r.precommands[name] = m
}

// precommand reads a modifier off one expanded field.
//
// A field, still carrying its glob marks, and not a word: the scan happens
// after expansion and before the match, which is measured from both sides.
// `x=noglob; $x echo a[b]c` prints `a[b]c`, so the word is read after it is
// expanded; `noglob echo a[b]c` prints it too, so the pattern behind it was
// never matched.
//
// Fields rather than words because one word can produce several and the front
// of *that* list is what is scanned: `c=(noglob echo); $c a[b]c` prints the
// three characters, and so does `c="noglob echo"; ${=c} a[b]c`. Reading a word
// that produced two fields as "not a modifier" is the plausible narrower rule
// and it is wrong in both of those.
func (r *Runner) precommand(field string) (PrecommandModifier, bool) {
	if len(r.precommands) == 0 {
		return 0, false
	}
	m, ok := r.precommands[globUnescape(field)]
	return m, ok
}

// globFieldsUnlessSuppressed is globFields with the match switched off when
// the command carries a modifier that switched it off.
//
// It suspends the same flag every other "read this as text" context suspends
// rather than matching by a second route, because a second route is how the
// two would come to disagree about what a pattern is. Suspended around the
// *match* alone and not around the expansion: what a command substitution
// inside the word does is its own business, and measured — `noglob echo
// $(echo a[b]c)` still reports the pattern from inside the substitution.
func (r *Runner) globFieldsUnlessSuppressed(fields []string, suppressed bool) []string {
	if !suppressed {
		return r.globFields(fields)
	}
	defer r.withoutGlobbing()()
	return r.globFields(fields)
}

// afterPrecommands drops the leading modifier words a command carries, for the
// questions asked about it *before* it is expanded.
//
// The scan in Runner.simple is the one that matters and this is the same rule
// asked one stage earlier, where only the written word is in hand. It is here
// rather than written out again at the caller because a second copy of "which
// word names the command" is how the two would come to disagree: without it
// `echo a | noglob read x` read into a subshell and left `x` empty, where a
// bare `echo a | read x` in the same shell sets it.
//
// Only the ones the scan takes away. A transparent modifier stays in the
// command, so the word that names it is still the command's name.
func (r *Runner) afterPrecommands(args []*syntax.Word) []*syntax.Word {
	for len(args) > 0 {
		if m, ok := r.precommands[literalName(args[0])]; !ok || m == PrecommandTransparent {
			return args
		}
		args = args[1:]
	}
	return args
}
