// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

// Prelude is the part of the dialect written as shell rather than as Go.
func Prelude() string { return identity }

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
