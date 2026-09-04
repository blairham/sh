// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// export -p and readonly -p list what carries the attribute, in the form the
// dialect chose — and nothing else: an unexported name stays out of the
// first, an unmarked one out of the second.

func TestExportListingNamesTheExported(t *testing.T) {
	for _, tc := range []struct {
		name string
		form DeclarationListingForm
		want string
	}{
		{"clustered", DeclareListingClustered, `declare -x V="1"`},
		{"command word", DeclareListingCommandWord, `export V='1'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, `export V=1; plain=2; export -p`, func(r *Runner) {
				sem := CoreSemantics()
				sem.ExportListing = tc.form
				if tc.form == DeclareListingClustered {
					sem.DeclareValueQuoting = ListingQuoteAlwaysDouble
				} else {
					sem.DeclareValueQuoting = ListingQuoteAlwaysEscaped
				}
				r.Semantics = &sem
			})
			if st != 0 || !strings.Contains(out, tc.want) {
				t.Errorf("out = %q st=%d, want %q listed", out, st, tc.want)
			}
			if strings.Contains(out, "plain") {
				t.Errorf("out = %q, want the unexported name kept out", out)
			}
		})
	}
}

func TestReadonlyListingNamesTheReadonly(t *testing.T) {
	out, st := run(t, `readonly R=2; free=3; readonly -p`, func(r *Runner) {
		sem := CoreSemantics()
		sem.ReadonlyListing = DeclareListingCommandWord
		sem.DeclareValueQuoting = ListingQuoteWhenNeededDollar
		r.Semantics = &sem
	})
	if st != 0 || !strings.Contains(out, "readonly R=2") {
		t.Errorf("out = %q st=%d, want the readonly named", out, st)
	}
	if strings.Contains(out, "free") {
		t.Errorf("out = %q, want the unmarked name kept out", out)
	}
}
