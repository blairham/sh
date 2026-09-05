// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

// Prelude is the part of the dialect written as shell rather than as Go.
//
// With the dialect rather than in the binary, so that `sh -dialect zsh` and
// `./zsh` are the same shell reached by two roads — see the same file under
// dialect/bash for what went wrong when they were not.
func Prelude() string { return identity + functions }

// The directory stack, as shell. The same machinery the bash dialect's
// prelude carries, with the one measured difference kept: this engine's
// `pushd` and `popd` move in silence — only `dirs` prints, one line, current
// directory first, $HOME abbreviated to `~`. The subscripts are spelled for
// this dialect's one-based arrays, which is the one line the two preludes
// cannot share. An empty stack refuses `popd`
// with status 1; the real engine locates that complaint the way it locates
// any message, which a shell function cannot, so ours is the bare sentence.
const functions = `
dirs() {
	local __d __out=${PWD/#$HOME/\~}
	for __d in "${DIRSTACK[@]}"; do
		__out="$__out ${__d/#$HOME/\~}"
	done
	echo "$__out"
}
pushd() {
	local __old=$PWD
	if [ $# -eq 0 ]; then
		if [ ${#DIRSTACK[@]} -eq 0 ]; then
			echo "pushd: no other directory" >&2
			return 1
		fi
		cd "${DIRSTACK[1]}" || return 1
		DIRSTACK[1]=$__old
	else
		cd "$1" || return 1
		DIRSTACK=("$__old" "${DIRSTACK[@]}")
	fi
}
popd() {
	if [ ${#DIRSTACK[@]} -eq 0 ]; then
		echo "popd: directory stack empty" >&2
		return 1
	fi
	cd "${DIRSTACK[1]}" || return 1
	DIRSTACK=("${DIRSTACK[@]:1}")
}
`

// identity is how this shell answers "which shell are you".
//
// zsh names itself twice: a version, and a name that says which of the shells
// in its family is running. Both are plain values rather than a version
// string with a tag in it, so the tag goes where a reader will see it — a
// script comparing the numbers gets what it asked for either way.
const identity = `
ZSH_VERSION='5.9.2-blairham'
ZSH_NAME=zsh
`
