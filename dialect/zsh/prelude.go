// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

// Prelude is the part of the dialect written as shell rather than as Go.
//
// With the dialect rather than in the binary, so that `sh -dialect zsh` and
// `./zsh` are the same shell reached by two roads — see the same file under
// dialect/bash for what went wrong when they were not.
func Prelude() string {
	return identity + nullCommands + "WORDCHARS='" + wordCharacters + "'\n" + functions
}

// The directory stack, as shell. The same machinery the bash dialect's
// prelude carries, with the measured differences kept: this engine's `pushd`
// and `popd` move in silence — only `dirs` prints, one line, current
// directory first, $HOME abbreviated to `~`. An empty stack refuses `popd`
// with status 1, and says so through `diagnose`, which is what puts this
// shell's own location — the builtin's name between the file and the line —
// in front of a sentence written here (#603, interp/prelude.go).
//
// Rotation is the half that was missing (#468), and it is measured as
// *identical* to bash's: `pushd +N` counts the current directory as entry 0
// and turns the stack so that entry N is the one you are standing in, `-N`
// counts from the other end, and `popd +N` takes an entry out and leaves the
// current directory alone unless N picks it. Every one of the twelve
// rotations compared came out the same in both shells. What differs is the
// wording of a refusal — one sentence here where bash has two — the silence,
// and the shape of `dirs`.
//
// Three things about `dirs` are this shell's own: its option letters bundle,
// where bash reads `dirs -lv` as a malformed number; `-v` numbers with a tab
// and no padding; and an operand *replaces* the stack rather than selecting
// an entry from it, so `dirs +1` leaves a stack holding the two characters
// `+1`. Measured, and implemented as measured.
//
// Rotation goes through the positional parameters rather than through array
// subscripts. This dialect's arrays start at 1 and bash's at 0, and
// `set -- "$@" "$1"; shift` means the same thing in both — which is what
// keeps the two texts one dialect apart rather than one index apart.
//
// Every read of the stack is written `${DIRSTACK[@]+"${DIRSTACK[@]}"}` and
// not `"${DIRSTACK[@]}"`, because nothing declares the name until the first
// push and this shell reads an *undeclared* name through a subscript as a
// scalar — Semantics.UnsetNameAtIsOneEmptyField, which real zsh answers yes.
// Unguarded, the first `pushd` stored an empty entry beside the old
// directory and `dirs` printed a trailing space for it, in the one shape a
// count cannot see. Real zsh has no such state to reach: its stack parameter
// is an array from startup. The guard is the idiom that holds either way,
// and it is the same text in the bash prelude so the two stay one dialect
// apart.
//
// Out of scope, and recorded in docs/spec/semantics.md: `pushd old new`, the
// substitution form, and `-c` in company with a printing letter, which this
// engine measures as doing nothing at all.
const functions = `
dirs() {
	local __d __c __clear= __long= __lines= __numbers= __set= __i=0
	while [ $# -gt 0 ]; do
		case $1 in
		-)  break ;;
		-*)
			__d=${1#-}
			while [ -n "$__d" ]; do
				__c=${__d%${__d#?}}
				case $__c in
				c) __clear=1 ;;
				l) __long=1 ;;
				p) __lines=1 ;;
				v) __lines=1; __numbers=1 ;;
				*)
					diagnose "bad option: -$__c"
					return 1
					;;
				esac
				__d=${__d#?}
			done
			shift
			;;
		*) break ;;
		esac
	done
	if [ $# -gt 0 ]; then
		# An operand is a new stack here rather than an index into the old
		# one, which is why this shell has no dirs +N.
		DIRSTACK=("$@")
		return 0
	fi
	if [ -n "$__clear" ] && [ -z "$__lines" ]; then
		DIRSTACK=()
		return 0
	fi
	set -- "$PWD" ${DIRSTACK[@]+"${DIRSTACK[@]}"}
	__d=
	for __c in "$@"; do
		if [ -z "$__long" ]; then
			__c=${__c/#$HOME/\~}
		fi
		if [ -n "$__numbers" ]; then
			printf '%d\t%s\n' "$__i" "$__c"
		elif [ -n "$__lines" ]; then
			echo "$__c"
		elif [ "$__i" -eq 0 ]; then
			__d=$__c
		else
			__d="$__d $__c"
		fi
		__i=$(( __i + 1 ))
	done
	if [ -z "$__lines" ]; then
		echo "$__d"
	fi
}
__dirs_rotate() {
	local __spec=$1 __i __new __opts=$2
	set -- "$PWD" ${DIRSTACK[@]+"${DIRSTACK[@]}"}
	case $__spec in
	+*) __i=${__spec#+} ;;
	*)  __i=$(( $# - 1 - ${__spec#-} )) ;;
	esac
	if [ "$__i" -lt 0 ] || [ "$__i" -ge $# ]; then
		# One sentence for an index that is out of range and for a stack
		# with nothing in it, where bash has two.
		diagnose "no such entry in dir stack"
		return 1
	fi
	while [ "$__i" -gt 0 ]; do
		set -- "$@" "$1"
		shift
		__i=$(( __i - 1 ))
	done
	__new=$1
	shift
	cd ${__opts:+"$__opts"} "$__new" || return 1
	DIRSTACK=("$@")
}
__dirs_cdopts() {
	# The letters of a leading option word that cd reads, or nothing when
	# the word is not one of those. A bundle is kept whole and an unknown
	# letter refuses the whole word, so a single letter this engine does
	# not carry cannot turn -qs into a half-read bundle.
	local __d=$1 __c __o=
	case $__d in
	-?*) __d=${__d#-} ;;
	*) return 1 ;;
	esac
	while [ -n "$__d" ]; do
		__c=${__d%${__d#?}}
		case $__c in
		[LPq]) __o=$__o$__c ;;
		*) return 1 ;;
		esac
		__d=${__d#?}
	done
	__DIRS_OPTS=$__DIRS_OPTS$__o
	return 0
}
pushd() {
	local __old=$PWD __spec= __DIRS_OPTS=
	while [ $# -gt 0 ]; do
		case $1 in
		+[0-9]*|-[0-9]*) __spec=$1; shift ;;
		--) shift; break ;;
		-?*) __dirs_cdopts "$1" || break; shift ;;
		*) break ;;
		esac
	done
	if [ -n "$__spec" ]; then
		__dirs_rotate "$__spec" ${__DIRS_OPTS:+-$__DIRS_OPTS}
		return $?
	fi
	if [ $# -eq 0 ]; then
		if [ ${#DIRSTACK[@]} -eq 0 ]; then
			# Nothing to exchange with, so this shell goes home and pushes
			# where it was — where bash refuses and stays put.
			cd ${__DIRS_OPTS:+-$__DIRS_OPTS} "$HOME" || return 1
			DIRSTACK=("$__old")
			return 0
		fi
		cd ${__DIRS_OPTS:+-$__DIRS_OPTS} "${DIRSTACK[1]}" || return 1
		DIRSTACK[1]=$__old
		return 0
	fi
	cd ${__DIRS_OPTS:+-$__DIRS_OPTS} "$1" || return 1
	DIRSTACK=("$__old" ${DIRSTACK[@]+"${DIRSTACK[@]}"})
}
popd() {
	local __spec= __i __k __len __DIRS_OPTS=
	while [ $# -gt 0 ]; do
		case $1 in
		+[0-9]*|-[0-9]*) __spec=$1; shift ;;
		-?*) __dirs_cdopts "$1" || break; shift ;;
		*) break ;;
		esac
	done
	if [ ${#DIRSTACK[@]} -eq 0 ]; then
		diagnose "directory stack empty"
		return 1
	fi
	set -- "$PWD" ${DIRSTACK[@]+"${DIRSTACK[@]}"}
	__i=0
	if [ -n "$__spec" ]; then
		case $__spec in
		+*) __i=${__spec#+} ;;
		*)  __i=$(( $# - 1 - ${__spec#-} )) ;;
		esac
		if [ "$__i" -lt 0 ] || [ "$__i" -ge $# ]; then
			diagnose "no such entry in dir stack"
			return 1
		fi
	fi
	if [ "$__i" -eq 0 ]; then
		# The entry you are standing in: the shell moves to the next one
		# down, which is what a bare popd does.
		cd ${__DIRS_OPTS:+-$__DIRS_OPTS} "${DIRSTACK[1]}" || return 1
		DIRSTACK=("${DIRSTACK[@]:1}")
		return 0
	fi
	# Any other entry is taken out where it stands and the shell does not
	# move. Built by walking the whole stack rather than by two slices,
	# because a subscript is the one thing the two dialects spell alike and
	# mean differently.
	__len=$#
	__k=0
	while [ "$__k" -lt "$__len" ]; do
		if [ "$__k" -ne "$__i" ]; then
			set -- "$@" "$1"
		fi
		shift
		__k=$(( __k + 1 ))
	done
	shift
	DIRSTACK=("$@")
}
`

// identity is how this shell answers "which shell are you".
//
// zsh names itself twice: a version, and a name that says which of the shells
// in its family is running. Both are plain values rather than a version
// string with a tag in it, so the tag goes where a reader will see it — a
// script comparing the numbers gets what it asked for either way.
//
// ZSH_ARGZERO joins them because it is the same kind of fact — what this
// shell was called — and because it is the one value `$0` cannot answer for
// once Semantics.DollarZeroNamesTheInnermostCall is on: `$0` follows the
// innermost function or sourced file, and this is what it was before
// anything moved it. Captured by reading `$0` here, which works because the
// prelude is the first thing the runner reads and nothing has been called
// yet: measured against real zsh, that is the path of the binary for `-c`,
// for standard input and for an interactive shell, and the script's path as
// written for a script.
//
// A plain assignment and not a special parameter, which is what it is: real
// zsh reports `typeset ZSH_ARGZERO=…`, does not export it, and lets a script
// assign to it or unset it like any other scalar.
const identity = `
ZSH_VERSION='5.9.2-blairham'
ZSH_NAME=zsh
ZSH_ARGZERO=$0
`

// nullCommands is what a command that is only redirections runs, as the two
// ordinary parameters this shell keeps them in.
//
// Here rather than in Go because a script owns them: it reads them, reassigns
// them and unsets them, and `typeset -p` reports both as plain unexported
// scalars in real zsh. The core is told the two *names* — see
// Semantics.NullCommandVariable — and finds whatever the script has left in
// them, which is the whole reason the hook is observable at all: pointing
// `READNULLCMD` at a function that prints a marker is the only probe that
// separates this route from the `$(<file)` form.
//
// `more` on the reading side is why `<f` at a prompt pages the file and `cat`
// on the writing side is why `>g` truncates it with a `cat` that reads
// nothing. Measured 2026-09-10 on zsh 5.9.2.
const nullCommands = `
NULLCMD=cat
READNULLCMD=more
`
