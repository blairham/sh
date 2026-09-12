// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

// Prelude is the part of the dialect written as shell rather than as Go.
func Prelude() string { return identity + presetAliases }

// presetAliases are the aliases this shell starts with, as `alias` with no
// operands lists them on a bare invocation of ksh93u+ 2012-08-01.
//
// They are shell rather than Go because that is what they are in the shell
// they come from: every one is a word standing for other words, and nothing
// in the substrate has to learn a name to make `autoload f` mean
// `typeset -fu f`. That is also why the list is worth having at all — the
// corpus asked about `autoload` nineteen times and got `autoload: not
// found`, where the shell it grades against reaches `typeset` and refuses
// zsh's letters from there (#2345).
//
// Two carry a trailing blank on purpose. An alias whose value ends in one
// makes the *next* word a candidate for alias expansion too, which is how
// `command` and `nohup` let an alias stand after them.
//
// `2d` names a function this shell does not ship; the alias is listed all
// the same, because what is being reproduced is the table rather than a
// working command.
const presetAliases = `
alias 2d='set -f;_2d'
alias autoload='typeset -fu'
alias command='command '
alias compound='typeset -C'
alias fc=hist
alias float='typeset -lE'
alias functions='typeset -f'
alias hash='alias -t --'
alias history='hist -l'
alias integer='typeset -li'
alias nameref='typeset -n'
alias nohup='nohup '
alias r='hist -s'
alias redirect='command exec'
alias source='command .'
alias stop='kill -s STOP'
alias suspend='kill -s STOP $$'
alias times='{ { time;} 2>&1;}'
alias type='whence -v'
`

// identity is how this shell answers "which shell are you".
//
// ksh93 writes a sentence rather than a number — `Version AJM 93u+
// 2012-08-01` — so there is nowhere to put a tag that a version test would
// step over. The whole string says whose it is instead, which is safe for the
// same reason: a script testing this one tests it as text.
const identity = `
KSH_VERSION='` + kshVersion + `'
`

// The version this dialect implements, as the sentence this shell writes
// rather than as a number. One value, because `$KSH_VERSION` and the line
// `--version` writes are the same claim.
const kshVersion = "Version blairham 93u+ 2026-09-02"

// versionLine is what this shell writes when the invocation asks for its
// version.
//
// Measured 2026-09-11: ksh93 answers `--version` with
// `  version         sh (AT&T Research) 93u+ 2012-08-01` **on standard error**
// and exits 2, which is the one place in the panel where naming a version is
// a failed invocation. The word is read by its generic option reader rather
// than by a case of its own, and the layout — two spaces, the word, then the
// name of the shell and its version — is that reader's. Kept, because a
// script that greps this line greps it as text.
const versionLine = "  version         sh (blairham) 93u+ 2026-09-02"
