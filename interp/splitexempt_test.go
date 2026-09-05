// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The contexts that never field-split — an assignment's value, `[[ ]]`
// operands, a case subject, a here-string, a here-document body, the text
// inside `$(( ))` — are exempt unanimously across the panel, so the bare core
// answers them without a dialect. An unanswered axis refuses only where the
// shells genuinely disagree, and these positions are agreement: each of these
// held a "splitting an unquoted parameter expansion" refusal to stderr before
// the exemption stopped asking.
func TestUnanimousNoSplitContextsAskNoAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an assignment's value",
			`two="a b"; x=$two; printf "<%s>" "$x"`, "<a b>",
		},
		{
			"an assignment's value under a live IFS",
			`IFS=:; y="p:q"; x=$y; printf "<%s>" "$x"`, "<p:q>",
		},
		{
			"a command substitution as an assignment's value",
			`x=$(echo a b); printf "<%s>" "$x"`, "<a b>",
		},
		{
			"a `[[ ]]` operand",
			`two="a b"; [[ $two = "a b" ]] && printf one || printf split`, "one",
		},
		{
			"a case subject",
			`two="a b"; case $two in "a b") printf one;; *) printf other;; esac`, "one",
		},
		{
			"a here-string",
			`two="a b"; cat <<< $two`, "a b\n",
		},
		{
			"a here-document body",
			"two=\"a b\"; cat <<X\n[$two]\nX\n", "[a b]\n",
		},
		{
			"the text inside an arithmetic expansion",
			`x="1 + 2"; printf "<%s>" "$(($x))"`, "<3>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) {
				sem := CoreSemantics()
				r.Semantics = &sem
			})
			if out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q status 0 and no refusal", out, st, tc.want)
			}
		})
	}
}

// A redirection target is the one no-split position where the shells
// genuinely part company, so the bare core keeps refusing a target that is
// not one word either way — and the refusal names the redirection's own axis,
// not the splitting one, whose exemption here is as unanimous as everywhere
// else. Having refused, it must not act on either reading: the command does
// not run and no file appears.
func TestARefusedRedirectionTargetNamesItsOwnAxis(t *testing.T) {
	dir := t.TempDir()
	out, st := run(t, `two="a b"; echo hi > $two`, func(r *Runner) {
		sem := CoreSemantics()
		r.Semantics, r.Dir = &sem, dir
	})
	if !strings.Contains(out, "a redirection target expanded as an ordinary word") {
		t.Errorf("out = %q, want the redirection's own axis named", out)
	}
	if strings.Contains(out, "splitting an unquoted") {
		t.Errorf("out = %q, want no splitting axis asked in a no-split position", out)
	}
	if strings.Contains(out, "hi") {
		t.Errorf("out = %q, want the command not run after the refusal", out)
	}
	if st == 0 {
		t.Error("status 0, want the refusal reported")
	}
	if got := readFile(t, dir, "a b"); got != "" {
		t.Errorf("file %q written, want neither reading acted on", got)
	}
}
