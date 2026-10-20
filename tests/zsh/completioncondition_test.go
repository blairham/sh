// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// `[[ -prefix … ]]` and `[[ -suffix … ]]` are in this shell's condition
// grammar unconditionally, and the restriction is on where they may *run*.
// Measured 2026-09-12 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin` with a
// scratch HOME; the other four have no such operator at all.
//
//	$ zsh -n s.zsh      # [[ -prefix : ]]
//	(nothing)
//	$ zsh s.zsh
//	s.zsh:1: condition can only be used in completion function
//	$ echo $?
//	1
func TestTheCompletionConditionsParseHere(t *testing.T) {
	if !zsh.Dialect().CompletionConditions {
		t.Error("this shell has `[[ -prefix ]]` and `[[ -suffix ]]` in the grammar")
	}
	for _, src := range []string{
		"[[ -prefix : ]]\n",
		"[[ -prefix 'ab' ]]\n",
		// The operand is a pattern rather than an option name, which is the
		// shape both real occurrences have: a completion for `_chromium`
		// writes `[[ -prefix //(127.0.0.1|localhost)/ ]]`.
		"[[ -prefix //(a|b)/ ]]\n",
		"[[ -suffix : ]]\n",
	} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

// Reaching one anywhere but a completion function is a refusal, and a fatal
// one: the line after it does not run and the status is 1. The sentence names
// neither the operator nor the operand, and it is the same for all three
// operand shapes.
func TestACompletionConditionOutsideACompletionIsFatalHere(t *testing.T) {
	for _, src := range []string{
		`[[ -prefix : ]]; echo after`,
		`[[ -prefix //(a|b)/ ]]; echo after`,
		`[[ -suffix : ]]; echo after`,
		`if [[ -prefix : ]]; then echo T; else echo F; fi; echo after`,
	} {
		out, st := answersRun(t, src)
		if strings.Contains(out, "after") {
			t.Errorf("%s: ran on after the refusal: %q", src, out)
		}
		if !strings.Contains(out, "condition can only be used in completion function") {
			t.Errorf("%s: said %q", src, out)
		}
		if st != 1 {
			t.Errorf("%s: status = %d, want 1", src, st)
		}
	}
}

// With no operand after it the word is an ordinary one, so the condition is
// the bare-word test and answers 0 — which bash 5.3, that binary as `sh` and
// ksh93 also answer, reading the word as ordinary because they have no such
// operator. The pair is the only one in this shell's table that falls back
// this way: `[[ -n ]]` is `unknown condition: -n` here.
//
// dash and bash 3.2 do not answer 0 and are not counter-examples: dash has no
// `[[ ]]` at all, and bash 3.2 reads `-prefix` as *a* conditional unary
// operator and calls the `]]` an unexpected argument to it, which bash 5.3
// does not.
func TestACompletionConditionWithNoOperandIsAWordHere(t *testing.T) {
	for _, src := range []string{
		`[[ -prefix ]]; echo st=$?`,
		`[[ -suffix ]]; echo st=$?`,
		`[[ -prefix && -n x ]]; echo st=$?`,
		`[[ -prefix || -n x ]]; echo st=$?`,
		`[[ ( -prefix ) ]]; echo st=$?`,
		`[[ -n -prefix ]]; echo st=$?`,
	} {
		out, _ := answersRun(t, src)
		if got := strings.TrimSpace(out); got != "st=0" {
			t.Errorf("%s: said %q, want %q", src, got, "st=0")
		}
	}
}
