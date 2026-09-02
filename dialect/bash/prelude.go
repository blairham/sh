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

const functions = `
pushd() { cd "$1"; }
popd()  { cd "$OLDPWD"; }
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
// Fixed rather than read from whatever bash is installed. A binary whose
// behavior depends on the machine it runs on is what the conformance harness
// exists to keep separate — and the one part that *is* about this machine,
// the architecture, comes from the build rather than from another shell.
func identity() string {
	return fmt.Sprintf(`
BASH_VERSION='%[1]s(%[2]d)-%[3]s'
BASH_VERSINFO=(%[4]d %[5]d %[6]d %[2]d %[3]s %[7]s)
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

// machine is the triple bash puts last in BASH_VERSINFO. Ours is the build's,
// which is the honest answer to "what was this compiled for".
func machine() string { return runtime.GOARCH + "-" + runtime.GOOS }
