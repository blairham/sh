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
KSH_VERSION='Version blairham 93u+ 2026-09-02'
`
