// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy

import "strings"

// A deny also covers the other spellings a filesystem answers to (#2044).
//
// The problem is the inverse of the one the walk solves. `internal/opened`
// answers "this name reached a different object" — the object moving under a
// name. This answers "this object has a second name", which no amount of
// resolution reaches, because both names are correct and the filesystem
// holds one file under either.
//
// On macOS's default volumes — and on Windows, and on a case-insensitive
// mount anywhere — `/proj/.env` and `/proj/.ENV` are one file. A rule
// written about the first did not cover the second, so the shape an agent
// sandbox actually writes,
//
//	allow read /proj/**
//	deny  read /proj/.env
//
// handed the secret over to `read -r x < /proj/.ENV`: the respelling missed
// the deny and landed on the allow beside it. Measured, and the file was
// writable through the same door.
//
// # Why only a deny folds
//
// Evaluation is deny-overrides, and that asymmetry is exactly what makes
// this safe in one direction and unsafe in the other. Widening a deny can
// only refuse more, and a refusal is the fail-closed answer; widening an
// allow would *grant* a read of `/p/X` to a policy that named `/p/x`, and on
// a case-sensitive volume those are two different files. So an allow stays
// exact and nothing new is ever permitted by this.
//
// The cost is the other side of the same coin, and it is real: on a
// case-sensitive volume `deny read /p/.env` now also refuses a genuinely
// distinct `/p/.ENV`. That is over-refusal rather than a leak, it is visible
// in the refusal the script is given, and a policy is the place to prefer it.
//
// # Why this is not conditional on the platform
//
// It would be cheap to fold on Darwin and not on Linux, and the file beside
// this one does exactly that for platform aliases. It is not done here,
// because case-insensitivity is a property of a *volume* and not of a
// platform: a case-sensitive APFS volume is a supported macOS arrangement,
// and a case-insensitive mount under Linux is an ordinary one. A rule that
// read one way per operating system would be wrong on both of those, and it
// would make one policy file mean two things with nothing in the file to say
// so — which is the argument `alias_other.go` already makes.
//
// # What this does not reach
//
// Two spellings that differ by Unicode *composition* rather than by case:
// a name stored `café` and reached as `café`. Simple case folding does not
// relate them and nothing in the standard library composes or decomposes a
// character. That is #2045, it is a live escape, and it is recorded there
// rather than half-closed here.
func foldMatch(pattern, name string) bool {
	return match(strings.ToLower(pattern), strings.ToLower(name))
}
