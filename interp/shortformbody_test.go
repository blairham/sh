// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The reading is written and read back, and the dialect is replaced rather
// than written through: a subshell holds the same pointer, and the front end
// has nothing but the pointer to notice a grammar by.
func TestTheShortFormBodyReadingIsWrittenAndReadBack(t *testing.T) {
	for _, want := range []bool{true, false} {
		d := syntax.Core()
		d.ShortForm = true
		d.ShortFormBody = !want
		// testrunner:bare — nothing here runs a line; the subject is the
		// dialect pointer, and a directory of its own would only hide that.
		r := &interp.Runner{Dialect: &d}
		shared := r.Dialect

		r.SetShortFormBodyIsOneCommandOrNone(want)
		if got := r.ShortFormBodyIsOneCommandOrNone(); got != want {
			t.Errorf("the reading is %v after being set to %v", got, want)
		}
		if r.Dialect == shared {
			t.Error("the dialect was written through rather than replaced")
		}
	}
}

// And a reading already in place is left alone, so a runner whose grammar has
// not moved does not hand the front end a fresh pointer on every option word.
func TestSettingTheShortFormBodyReadingItAlreadyHasSwapsNothing(t *testing.T) {
	d := syntax.Core()
	d.ShortForm = true
	d.ShortFormBody = true
	// testrunner:bare — the subject is the dialect pointer and nothing runs.
	r := &interp.Runner{Dialect: &d}
	shared := r.Dialect

	r.SetShortFormBodyIsOneCommandOrNone(true)
	if r.Dialect != shared {
		t.Error("the dialect was replaced although nothing about it moved")
	}
}
