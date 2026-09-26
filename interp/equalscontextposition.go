// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// Two positions a word can stand in where the tildes an assignment's value
// gets do not reach it, however the word is shaped.
//
// interp/assignmentshapedword.go is the rule and this is its scope. Both
// roads were reachable and neither was answered — they were written down on
// Semantics.TheFirstUnquotedEqualsInAWordOpensATildeContext rather than
// decided — and #4564 is the number on them.
//
//	HOME=/Users/testhome, `-f -c`, zsh 5.9.2 (aarch64-apple-darwin25.4.0):
//
//	setopt magicequalsubst; x=(a=~); print -r -- "[$x]"   [a=~]     was [a=/Users/testhome]
//	setopt magicequalsubst; --opt=~                       command not found: --opt=~
//	                                                                was: no such file or directory: --opt=/Users/testhome
//	setopt magicequalsubst; command --opt=~               command not found: --opt=~
//	setopt magicequalsubst; noglob --opt=~                command not found: --opt=~
//	setopt magicequalsubst; builtin --opt=~               no such builtin: --opt=~
//
// **The array literal is not this option's question and not zsh's**, which
// the narrow rule's own column says: `a=(FOO=~/x); echo "${a[0]}"` is
// `FOO=~/x` in bash 5.3.20 while `echo FOO=~/x` on the same line is a path.
// So the exemption is the position rather than the axis, and it is asked of
// both readings. Measured 2026-09-26 on /opt/homebrew/bin/bash and
// /opt/homebrew/bin/zsh; `go version -m` on each says *not a Go executable*.
//
// **The command word is decided by where the word is written and not by what
// the words in front of it came to.** The pair that says so, measured on the
// reference: `e=; $e --opt=~` *expands* — the word is the command's name at
// run time and the shell had already decided it was not — and `command
// --opt=~` does not, where the word is the second one written. A rule read
// off the expanded argument list answers both of those the other way round.
//
// The scan in front of the command word is the same shape Runner.precommand
// reads and a different set: `command` stops that scan and is an ordinary
// builtin here, and it still leaves the word behind it naming a command. So
// the names are a list of their own, supplied by the dialect.
//
// The flag is armed by the caller and **spent by the word pipeline**, which
// is what keeps it off the words inside the one it names: `$(print -r -- a=~)`
// written in a command word does expand in the reference, and a substitution's
// body is expanded by a pipeline of its own that finds the flag already down.
// See Runner.expandOneWordFields.

// SetCommandWordModifier makes a word one behind which the next written word
// still names the command. A dialect names the words it has; a runner with
// none scans nothing.
func (r *Runner) SetCommandWordModifier(name string) {
	if r.commandWordModifiers == nil {
		r.commandWordModifiers = map[string]bool{}
	}
	r.commandWordModifiers[name] = true
}

// namesTheCommand reports whether the word at i is the one naming the
// command: the first written word, or one behind a run of modifier words.
//
// Written words, read as literals, because that is what the reference
// decides on — see the pair in this file's comment. A word that is not a
// plain literal stops the scan, since nothing before it can be read.
func (r *Runner) namesTheCommand(args []*syntax.Word, i int) bool {
	for j := range i {
		name := literalName(args[j])
		if name == "" {
			return false
		}
		if _, ok := r.precommands[name]; ok {
			continue
		}
		if !r.commandWordModifiers[name] {
			return false
		}
	}
	return true
}

// outsideTheEqualsContextWord arms the flag for one word and hands back the
// restore, in the shape every other one-word arming here has.
func (r *Runner) outsideTheEqualsContextWord() func() {
	prev := r.outsideTheEqualsContext
	r.outsideTheEqualsContext = true
	return func() { r.outsideTheEqualsContext = prev }
}

// spendTheEqualsContextPosition reads the flag and puts it down for the
// duration of the word, so that nothing expanded *inside* the word inherits
// it. The value is restored afterwards because a word may be expanded once
// per brace alternative and each of them stands in the same position.
func (r *Runner) spendTheEqualsContextPosition() (outside bool, restore func()) {
	outside = r.outsideTheEqualsContext
	if !outside {
		return false, func() {}
	}
	r.outsideTheEqualsContext = false
	return true, func() { r.outsideTheEqualsContext = true }
}
