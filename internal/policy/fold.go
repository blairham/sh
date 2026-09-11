// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"strings"

	"github.com/blairham/sh/internal/unorm"
)

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
// # Composition, as well as case
//
// The same volumes conflate the two ways of writing one character. A
// directory stored `café` — `é` as U+00E9 — is reached by `café` written
// `e` + U+0301, and for a while a deny naming the first did not cover the
// second (#2045). It is the identical bug: one object, a class of names,
// and a rule that knew one member of it.
//
// Nothing in the standard library composes or decomposes a character —
// `unicode` classifies a combining mark but will not join it to the letter in
// front of it — so the comparison needs Unicode data that has to come from
// somewhere. It comes from internal/unorm, whose tables this repository
// generates, for the reason internal/eastasian generates a width table
// instead of importing one. That package states the argument and carries
// Unicode's own conformance file as the proof.
//
// The alternatives to a fold were worse and are recorded in #2045: reading
// the directory to learn the stored name fails on a `0111`
// traversable-but-unreadable directory, which `internal/opened`'s
// traverseFlags exists to keep working, and would fall back to the written
// name in exactly the place a policy most wants to be right about;
// `F_GETPATH` was already measured and rejected under concurrent rename
// (#1114).
func foldMatch(pattern, name string) bool {
	return match(foldPath(pattern), foldPath(name))
}

// foldPath is the form two names of one file share.
//
// Decomposed first so that the case mapping sees one character per mark,
// lowered, and decomposed again because lowering can take a character apart —
// `İ` (U+0130) lowers to `i` followed by a combining dot — and a form that was
// canonical on the way in has to be canonical on the way out, or two inputs
// that should meet would not.
//
// NFD rather than NFC because canonical equivalence *is* equality of NFD:
// composing afterwards would be work whose result is thrown away, and it is
// the half of a normalizer that needs the composition table and the
// exclusions list. Nothing here shows the folded form to anybody — it is a
// key, compared and discarded.
//
// Separators are ASCII and no canonical mapping moves them, so the path keeps
// its components and match() still sees the shape it expects.
func foldPath(s string) string {
	return unorm.NFD(strings.ToLower(unorm.NFD(s)))
}
