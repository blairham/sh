// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

// Prelude is the part of the dialect written as shell rather than as Go.
//
// With the dialect rather than in the binary, so that `sh -dialect zsh` and
// `./zsh` are the same shell reached by two roads — see the same file under
// dialect/bash for what went wrong when they were not.
func Prelude() string { return identity + functions }

const functions = `
pushd() { cd "$1"; }
popd()  { cd "$OLDPWD"; }
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
