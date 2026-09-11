// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A letter a dialect's optstring claims must never also appear in its
// Diagnostics.UnimplementedOptionLetters, because the second entry can never
// be reached: the shared reader consults that map only from refuseOption,
// which runs for a letter the optstring did *not* claim. An entry on both
// lists is dead data that says the opposite of what the shell does.
//
// It is an invariant rather than a per-letter assertion because that is what
// makes it worth having: it caught nothing when it was written and it fails
// the moment a letter is implemented and its old "not implemented yet" line
// is left behind — which is exactly what happened to `read -i` (#761), and
// which no behavioral test can see, the unreachable branch being unreachable.
func TestNoLetterIsBothImplementedAndNot(t *testing.T) {
	for _, d := range []struct {
		name string
		sem  interp.Semantics
		diag interp.Diagnostics
	}{
		{"bash", bash.Semantics(), bash.Diagnostics()},
		{"dash", dash.Semantics(), dash.Diagnostics()},
		{"ksh", ksh.Semantics(), ksh.Diagnostics()},
		{"zsh", zsh.Semantics(), zsh.Diagnostics()},
	} {
		t.Run(d.name, func(t *testing.T) {
			// The builtins whose letters are a Semantics axis rather than a
			// constant; those are the only ones with two lists to disagree.
			for _, b := range []struct{ builtin, opts string }{
				{"read", d.sem.ReadOptions},
				{"echo", d.sem.EchoOptions},
				// The declaration builtins reach the same map by the same
				// route, and one of them now has a letter taken in *silence*
				// rather than refused — Semantics.DeclareOptionsWithoutEffect
				// — which is a second way for a letter to be claimed and its
				// "not implemented yet" line to become dead data (#1037).
				//
				// DeclareOptions alone is the list to compare, and
				// deliberately not it plus the silent letters: a silent
				// letter is one *out of* DeclareOptions, so adding them
				// changes nothing, and a letter named silent that the dialect
				// does not claim is refused first — which makes its line
				// reachable rather than dead. A second mechanism here could
				// only disagree with the first.
				//
				// Both spellings, because a dialect with `declare` and
				// `typeset` under one implementation keys the map twice.
				{"typeset", d.sem.DeclareOptions},
				{"declare", d.sem.DeclareOptions},
				{"local", d.sem.LocalOptions},
				// `integer` is the declaration under a third spelling, with
				// its own letter set: it is *narrower* than typeset's in both
				// shells that have the word, so it has its own optstring and
				// its own row here rather than riding on DeclareOptions.
				{"integer", d.sem.IntegerOptions},
			} {
				claimed := strings.ReplaceAll(b.opts, ":", "")
				missing := d.diag.UnimplementedOptionLetters[b.builtin]
				for i := 0; i < len(missing); i++ {
					if strings.IndexByte(claimed, missing[i]) >= 0 {
						t.Errorf("%s: -%c is among the letters this dialect claims, %q, and in "+
							"UnimplementedOptionLetters %q; the second can never be reached",
							b.builtin, missing[i], b.opts, missing)
					}
				}
			}
		})
	}
}

// A letter whose *argument* is a number has to be a letter the dialect
// claims: Semantics.DeclareOptionsTakingANumber decides how a word is parsed,
// and DeclareOptions decides whether the letter exists at all, so a letter in
// the first and not the second describes a parse nothing can reach.
//
// The reachable half matters more than it looks. `-E`, `-L`, `-R` and `-Z`
// take a number in the real shells and are refused by name here, and the
// refusal fires in the letter loop before any number is read — so listing one
// of them would be a claim that the argument is handled when the letter never
// gets that far. #1461 is where they are implemented; this is what makes the
// table and the refusals disagree out loud when one moves without the other.
func TestEveryNumberTakingLetterIsALetterTheDialectHas(t *testing.T) {
	for _, d := range []struct {
		name string
		sem  interp.Semantics
		diag interp.Diagnostics
	}{
		{"bash", bash.Semantics(), bash.Diagnostics()},
		{"dash", dash.Semantics(), dash.Diagnostics()},
		{"ksh", ksh.Semantics(), ksh.Diagnostics()},
		{"zsh", zsh.Semantics(), zsh.Diagnostics()},
	} {
		t.Run(d.name, func(t *testing.T) {
			taking := d.sem.DeclareOptionsTakingANumber
			for i := 0; i < len(taking); i++ {
				if strings.IndexByte(d.sem.DeclareOptions, taking[i]) < 0 {
					t.Errorf("-%c takes a number in DeclareOptionsTakingANumber %q but is not "+
						"in DeclareOptions %q, so no word can reach the parse that reads it",
						taking[i], taking, d.sem.DeclareOptions)
				}
				for _, b := range []string{"typeset", "declare"} {
					if strings.IndexByte(d.diag.UnimplementedOptionLetters[b], taking[i]) >= 0 {
						t.Errorf("%s: -%c takes a number and is also refused as unimplemented; "+
							"the refusal runs first, so the number is never read", b, taking[i])
					}
				}
			}
			// And the integer letter is deliberately absent: it reads its
			// base through Semantics.IntegerAttributeTakesABase, which
			// decides a second thing besides the parse. Two fields both
			// claiming it takes a number could disagree.
			if strings.IndexByte(taking, 'i') >= 0 {
				t.Errorf("DeclareOptionsTakingANumber = %q names the integer letter, which "+
					"IntegerAttributeTakesABase already answers for", taking)
			}
		})
	}
}

// The same invariant for `set`, whose letters are a switch in the shared
// reader rather than an optstring a dialect hands over.
//
// A `set` letter the reader acts on is claimed by a Semantics answer instead
// of by a string, so the pairing is between that answer and
// UnimplementedOptionLetters: a dialect that says yes has the letter
// implemented, and its "not implemented yet" line can never be reached. That
// is the shape #1856 was filed on — `-B` lived in bash's refusal table while
// nothing accepted it, and moving one half without the other is how half a
// fix ships.
//
// Only the axis-gated letters are here. The unconditional ones — `-e`, `-x`
// and the rest — are implemented in every dialect and would be a constant row
// saying nothing; the per-dialect question is exactly the one an axis asks.
func TestNoSetLetterIsBothAnsweredYesAndRefused(t *testing.T) {
	for _, d := range []struct {
		name string
		sem  interp.Semantics
		diag interp.Diagnostics
	}{
		{"bash", bash.Semantics(), bash.Diagnostics()},
		{"dash", dash.Semantics(), dash.Diagnostics()},
		{"ksh", ksh.Semantics(), ksh.Diagnostics()},
		{"zsh", zsh.Semantics(), zsh.Diagnostics()},
	} {
		t.Run(d.name, func(t *testing.T) {
			refused := d.diag.UnimplementedOptionLetters["set"]
			for _, l := range []struct {
				letter byte
				answer interp.Answer
				axis   string
			}{
				{'f', d.sem.SetFTurnsOffGlobbing, "SetFTurnsOffGlobbing"},
				{'B', d.sem.SetBTurnsOffBraceExpansion, "SetBTurnsOffBraceExpansion"},
				{'t', d.sem.SetHasTheTLetter, "SetHasTheTLetter"},
				{'h', d.sem.SetHasTheHLetter, "SetHasTheHLetter"},
			} {
				if l.answer == interp.Yes && strings.IndexByte(refused, l.letter) >= 0 {
					t.Errorf("set: %s says this shell has -%c, and UnimplementedOptionLetters %q "+
						"refuses it; the refusal can never be reached",
						l.axis, l.letter, refused)
				}
			}
		})
	}
}
