// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"fmt"
	"runtime"
)

// Prelude is the part of the dialect written as shell rather than as Go.
//
// A function shadows a builtin and an external command alike, so anything here
// replaces the core's answer without the core knowing — and most of a real
// dialect belongs in a file like this rather than in the interpreter.
//
// It lives with the dialect rather than in the binary because it *is* part of
// the dialect: `sh -dialect bash` had no `pushd` while `./bash` did, and the
// two are meant to be the same shell reached by two roads. The conformance
// harness grades both, and a difference between them is a front-end bug by
// construction.
func Prelude() string { return identity() + functions }

// The directory stack, as shell — the prelude seam's whole point.
// `__dirstack` holds the pushed entries, newest first, and `$DIRSTACK` is the
// *view* onto it that a script reads: the current directory, then those
// entries. See shellparameters.go, which registers the view and its writer.
//
// The storage used to be `DIRSTACK` itself, and the deviation that arrangement
// recorded here is what #3098 was about: the real engine's `$DIRSTACK[0]` is
// wherever the shell is standing **now**, so a plain `cd` moves it — measured
// 2026-09-18, `pushd /usr; cd /etc` leaves `${DIRSTACK[0]}` as `/etc`. A
// stored array cannot say that, and the name a script reads cannot be the name
// this text writes, so the two are separated and the view does the joining.
// `dirs` still reads `$PWD` at print time, which is the same fact from the
// other side and is why this text does not consult the view at all.
//
// Every read of the stack is written `${__dirstack[@]+"${__dirstack[@]}"}` and
// not `"${__dirstack[@]}"`. This shell answers no to
// Semantics.UnsetNameAtIsOneEmptyField, so the plain spelling is safe here —
// but it is not in the zsh prelude, whose shell reads an undeclared name
// through a subscript as a scalar and stored an empty entry on the first
// push. The guard holds under both answers, and keeping the two texts
// identical is what makes them one dialect apart rather than two programs.
//
// Measured (2026-09-04, extended 2026-09-05): `pushd dir` prints the stack
// after pushing, a bare `pushd` exchanges the top entry with the current
// directory, `popd` prints what remains, and `dirs` writes everything on one
// line, current directory first, each entry with $HOME abbreviated to `~`.
// An empty stack refuses `popd` with status 1, located and named the way the
// real engine locates and names it: a function here *is* the shell, so
// `diagnose` hands the sentence over and the engine puts the rest in front of
// it (#603, interp/prelude.go). A second line — the usage line — stays a
// plain `echo`, because bash locates only the first.
//
// The rotating forms are the half that was missing (#468). `pushd +N` and
// `pushd -N` turn the stack — counting the *current* directory as entry 0,
// from the left with `+` and from the right with `-` — so that entry N
// becomes the one you are standing in; `popd +N` takes an entry out and
// leaves the current directory alone unless N picks it. `dirs -c` empties
// the stack, `-l` writes paths unabbreviated, `-p` one to a line, `-v` the
// same numbered, and `+N` / `-N` print one entry.
//
// Rotation goes through the positional parameters rather than through array
// subscripts, and that is not a flourish: this text is one line different
// from the zsh dialect's, whose arrays start at 1, and `set -- "$@" "$1";
// shift` means the same thing in both.
//
// `-n` — do the stack work and stay where you are — is the half of the
// option parser that used to be a refusal (#3036). Measured on bash 5.3.20,
// 2026-09-15: `pushd -n dir` puts the directory in the slot below the current
// one, unresolved and unchecked, and lists the stack; `pushd -n` alone and
// `pushd -n +N` change the stack in silence; `popd -n` takes out the entry
// below the current one, so it and `popd -n +0` are the same operation. A
// letter that is neither `-n` nor an index is an index that will not parse,
// which is why `pushd -L` now answers in `pushd`'s name rather than handing
// the word to `cd`. docs/spec/semantics.md records the rest.
const functions = `
dirs() {
	local __d __n= __clear= __long= __lines= __numbers= __i= __out=
	while [ $# -gt 0 ]; do
		case $1 in
		-c) __clear=1 ;;
		-l) __long=1 ;;
		-p) __lines=1 ;;
		-v) __lines=1; __numbers=1 ;;
		--) ;;
		+[0-9]*|-[0-9]*) __n=$1 ;;
		-*|+*)
			# A word that looks like an index and is not one, against a
			# word that is neither an index nor an option: two complaints,
			# and this shell says which of the two it met.
			diagnose "$1: invalid number"
			echo "dirs: usage: dirs [-clpv] [+N] [-N]" >&2
			return 2
			;;
		*)
			diagnose "$1: invalid option"
			echo "dirs: usage: dirs [-clpv] [+N] [-N]" >&2
			return 2
			;;
		esac
		shift
	done
	if [ -n "$__clear" ]; then
		__dirstack=()
		return 0
	fi
	set -- "$PWD" ${__dirstack[@]+"${__dirstack[@]}"}
	if [ -n "$__n" ]; then
		case $__n in
		+*) __i=${__n#+} ;;
		*)  __i=$(( $# - 1 - ${__n#-} )) ;;
		esac
		if [ "$__i" -lt 0 ] || [ "$__i" -ge $# ]; then
			diagnose "${__n#[-+]}: directory stack index out of range"
			return 1
		fi
		while [ "$__i" -gt 0 ]; do
			shift
			__i=$(( __i - 1 ))
		done
		if [ -n "$__long" ]; then echo "$1"; else echo "${1/#$HOME/\~}"; fi
		return 0
	fi
	__i=0
	for __d in "$@"; do
		if [ -z "$__long" ]; then
			__d=${__d/#$HOME/\~}
		fi
		if [ -n "$__numbers" ]; then
			printf '%2d  %s\n' "$__i" "$__d"
		elif [ -n "$__lines" ]; then
			echo "$__d"
		elif [ "$__i" -eq 0 ]; then
			__out=$__d
		else
			__out="$__out $__d"
		fi
		__i=$(( __i + 1 ))
	done
	if [ -z "$__lines" ]; then
		echo "$__out"
	fi
}
__dirs_rotate() {
	local __spec=$1 __nocd=$2 __i __new
	set -- "$PWD" ${__dirstack[@]+"${__dirstack[@]}"}
	case $__spec in
	+*) __i=${__spec#+} ;;
	*)  __i=$(( $# - 1 - ${__spec#-} )) ;;
	esac
	if [ "$__i" -lt 0 ] || [ "$__i" -ge $# ]; then
		# An empty stack is its own complaint, and not the same one: with
		# nothing pushed there is no index that could have been in range.
		if [ $# -eq 1 ]; then
			diagnose "directory stack empty"
		else
			diagnose "$__spec: directory stack index out of range"
		fi
		return 1
	fi
	while [ "$__i" -gt 0 ]; do
		set -- "$@" "$1"
		shift
		__i=$(( __i - 1 ))
	done
	if [ -n "$__nocd" ]; then
		# The entry the rotation brought to the front is dropped rather
		# than moved to. Slot zero is wherever the shell is standing and
		# -n says not to leave it, so what the rotation put there has
		# nowhere to go — which is why pushd -n +2 on a four-deep stack
		# lists the current directory twice rather than once.
		shift
		__dirstack=("$@")
		return 0
	fi
	__new=$1
	shift
	cd "$__new" || return 1
	__dirstack=("$@")
}
pushd() {
	local __old=$PWD __spec= __nocd=
	while [ $# -gt 0 ]; do
		case $1 in
		-n) __nocd=1; shift ;;
		+[0-9]*|-[0-9]*) __spec=$1; shift ;;
		--) shift; break ;;
		-*|+*)
			# Every letter but -n is an index that will not parse, and
			# that is the complaint bash makes: pushd -L is not an
			# unknown option here and it is not a directory called -L
			# either. Reading it as one used to hand the word to cd,
			# which answered in its own name and with its own usage line.
			diagnose "$1: invalid number"
			echo "pushd: usage: pushd [-n] [+N | -N | dir]" >&2
			return 2
			;;
		*) break ;;
		esac
	done
	if [ -n "$__spec" ]; then
		__dirs_rotate "$__spec" "$__nocd" || return 1
		# A suppressed rotation prints nothing, where every other form of
		# pushd lists the stack it just changed. Measured, and the one
		# place -n changes more than where the shell ends up.
		if [ -n "$__nocd" ]; then
			return 0
		fi
	elif [ $# -eq 0 ]; then
		if [ -n "$__nocd" ]; then
			# No directory, no index, and no move: there is nothing left
			# for this to do, and bash says nothing rather than refusing.
			return 0
		fi
		if [ ${#__dirstack[@]} -eq 0 ]; then
			diagnose "no other directory"
			return 1
		fi
		cd "${__dirstack[0]}" || return 1
		__dirstack[0]=$__old
	elif [ -n "$__nocd" ]; then
		# Stored as written. Nothing goes there, so nothing resolves it:
		# a relative word stays relative and a directory that does not
		# exist is pushed without complaint.
		__dirstack=("$1" ${__dirstack[@]+"${__dirstack[@]}"})
	else
		cd "$1" || return 1
		__dirstack=("$__old" ${__dirstack[@]+"${__dirstack[@]}"})
	fi
	dirs
}
popd() {
	local __spec= __i __k __len __nocd=
	while [ $# -gt 0 ]; do
		case $1 in
		-n) __nocd=1; shift ;;
		+[0-9]*|-[0-9]*) __spec=$1; shift ;;
		--) shift; break ;;
		-*|+*)
			diagnose "$1: invalid number"
			echo "popd: usage: popd [-n] [+N | -N]" >&2
			return 2
			;;
		*)
			# Not an index and not an option: popd takes no directory, and
			# says so with a third wording of its own.
			diagnose "$1: invalid argument"
			echo "popd: usage: popd [-n] [+N | -N]" >&2
			return 2
			;;
		esac
	done
	if [ ${#__dirstack[@]} -eq 0 ]; then
		diagnose "directory stack empty"
		return 1
	fi
	set -- "$PWD" ${__dirstack[@]+"${__dirstack[@]}"}
	__i=0
	if [ -n "$__spec" ]; then
		case $__spec in
		+*) __i=${__spec#+} ;;
		*)  __i=$(( $# - 1 - ${__spec#-} )) ;;
		esac
		if [ "$__i" -lt 0 ] || [ "$__i" -ge $# ]; then
			diagnose "$__spec: directory stack index out of range"
			return 1
		fi
	fi
	if [ -n "$__nocd" ] && [ "$__i" -eq 0 ]; then
		# Slot zero is the directory the shell is standing in and -n
		# says not to leave it, so the entry below is the one that goes.
		# That makes popd -n and popd -n +0 one operation rather than
		# a refusal, which is what bash does with the pair.
		__i=1
	fi
	if [ "$__i" -eq 0 ]; then
		# The entry you are standing in: the shell moves to the next one
		# down, which is what a bare popd does.
		cd "${__dirstack[0]}" || return 1
		__dirstack=("${__dirstack[@]:1}")
		dirs
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
	__dirstack=("$@")
	dirs
}
`

// identity is how this shell answers "which shell are you, and which one".
//
// Scripts ask, and a shell that does not answer is not the shell it claims to
// be for the part of the world that checks: bats and brew both refuse to run
// on the strength of `BASH_VERSION` alone, without ever finding out whether
// the shell could have run them.
//
// The number is the compatibility level this dialect targets, and the tag —
// where upstream writes `release` and Apple writes `apple` — says whose bash
// this is. A version test parses the digits and passes; anything that prints
// the string sees at once that it is not upstream. Both are true at the same
// time, which is what bash's version string has a tag for.
//
// `BASH_VERSINFO` is frozen on the line after it is written, which is where
// the freeze has to be: the array is this text's to fill in, so a mark made
// from Go before the prelude ran would refuse the very assignment that fills
// it. Measured 2026-09-18 — `declare -p BASH_VERSINFO` in bash 5.3.20 is
// `declare -ar …`, and this shell wrote `declare -a …` (#3099). The `-i` half
// of that listing is the *elements* being numbers rather than an attribute on
// the array, so there is no integer mark to make.
//
// Fixed rather than read from whatever bash is installed. A binary whose
// behavior depends on the machine it runs on is what the conformance harness
// exists to keep separate — and the one part that *is* about this machine,
// the architecture, comes from the build rather than from another shell.
func identity() string {
	return fmt.Sprintf(`
BASH_VERSION='%[1]s(%[2]d)-%[3]s'
BASH_VERSINFO=(%[4]d %[5]d %[6]d %[2]d %[3]s %[7]s)
readonly BASH_VERSINFO
BASH=$0
`, version, patch, tag, major, minor, build, machine())
}

// The compatibility level this dialect targets, spelled the way bash spells
// it: three numbers, a patch level in parentheses, and a tag.
const (
	major, minor, build = 5, 3, 15
	patch               = 1
	version             = "5.3.15"
	tag                 = "blairham"
)

// versionLine is what this shell writes when the invocation asks for its
// version, and it is the same identity BASH_VERSION carries one line up —
// spelled the way the real shell's first line spells it, because that line is
// what a version test reads: everything after `version ` up to the first space
// is what `--version | head -1` is cut down to.
//
// The first line and nothing else. The real shell follows it with a copyright
// notice and a statement of the GPL, and reproducing that here would be a
// false statement about this code's license as well as its authorship — this
// is Apache-2.0 and is not that program. The tag in the version says the same
// thing to anything that reads the string rather than the digits.
func versionLine() string {
	return fmt.Sprintf("GNU bash, version %s(%d)-%s (%s)", version, patch, tag, machine())
}

// helpVersionLine is the same line under `--help`, and the hyphen before the
// parenthesis is not a typo: bash writes `5.3.20(1)-release (aarch64-…)` for
// `--version` and `5.3.20(1)-release-(aarch64-…)` for `--help`, in one binary
// in one run. Measured 2026-09-18 on bash 5.3.20 and again on 3.2.57, which
// writes the same shape. So the two lines are two strings rather than one
// said twice (#3268).
func helpVersionLine() string {
	return fmt.Sprintf("GNU bash, version %s(%d)-%s-(%s)", version, patch, tag, machine())
}

// machine is the triple bash puts last in BASH_VERSINFO. Ours is the build's,
// which is the honest answer to "what was this compiled for".
func machine() string { return runtime.GOARCH + "-" + runtime.GOOS }
