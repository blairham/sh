// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

func jobSpecGlob(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, `: > '%?b2'; : > '%a'; `+src, nil, func(r *Runner) {
		s := *r.Semantics
		s.JobSpecQuestionMarkIsLiteral = a
		r.Semantics = &s
	})
}

// **Only a field beginning `%` and a live `?` asks**, so a vector that has not
// answered the axis still globs every other word — including a `?` one byte
// further along, and a `%?` whose `?` was quoted.
func TestOnlyALeadingPercentQuestionMarkAsks(t *testing.T) {
	out, st := jobSpecGlob(t, Unspecified, `echo ?a %* b%? '%?'`)
	if strings.Contains(out, "disagree") || st != 0 || out != "%a %?b2 %a b%? %?\n" {
		t.Errorf("got %q (status %d), want every word globbed and nothing refused", out, st)
	}
	out, st = jobSpecGlob(t, Unspecified, `echo %?`)
	if !strings.Contains(out, "a `?` right after a word's leading `%`") || st != 2 {
		t.Errorf("unanswered: %q (status %d), want the refusal at status 2", out, st)
	}
}

// **Each answer does what it says**, the `?` being a character under Yes and
// a pattern under No.
func TestEachJobSpecQuestionMarkAnswer(t *testing.T) {
	for _, c := range []struct {
		a    Answer
		want string
	}{
		{Yes, "%? %?b2\n"},
		{No, "%a %?b2 %a\n"},
	} {
		if out, st := jobSpecGlob(t, c.a, `echo %? %?*`); out != c.want || st != 0 {
			t.Errorf("%v: %q (status %d), want %q", c.a, out, st, c.want)
		}
	}
}
