// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A grammar that admits a punctuated function name may still refuse to
// define one — the axis one dialect answers yes, fatally and with a second
// sentence for a dot.
func TestAPunctuatedFunctionNameIsAnAxisAtDefinition(t *testing.T) {
	refuse := func(r *Runner) {
		sem := CoreSemantics()
		sem.PunctuatedFunctionNameIsRefused = Yes
		sem.FatalErrorStatusIsOne = Yes
		r.Semantics = &sem
	}
	out, st := run(t, `f-g(){ echo ok; }; f-g; echo after`, refuse)
	if st != 1 || strings.Contains(out, "after") || !strings.Contains(out, "invalid function name") {
		t.Errorf("out=%q st=%d, want a fatal refusal at the definition", out, st)
	}
	out, _ = run(t, `a.b(){ :; }; echo after`, func(r *Runner) {
		refuse(r)
		dg := Diagnostics{FunctionNameDiscipline: "%[1]s: invalid discipline function"}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "invalid discipline function") {
		t.Errorf("out=%q, want the dot's own sentence", out)
	}

	accept := func(r *Runner) {
		sem := CoreSemantics()
		sem.PunctuatedFunctionNameIsRefused = No
		r.Semantics = &sem
	}
	out, st = run(t, `f-g(){ echo ok; }; f-g`, accept)
	if st != 0 || !strings.Contains(out, "ok") {
		t.Errorf("out=%q st=%d, want the function defined and run", out, st)
	}
}
