// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/internal/dialecttest"
)

// The prompt style travels with the dialect, including this one, whose answer
// is that there is no escape language. #1455.
//
// "Nothing but expansion" is an answer rather than an absence, and the
// difference is visible: a runner told this table draws `\u` as `\u` because
// dash does, and a runner told nothing draws it that way because nobody said
// otherwise. The two look alike until a field that is not the escape table —
// Expand, or the defaults a script can observe — is read off the value.
func TestApplyInstallsThePromptStyle(t *testing.T) {
	preset.PromptTableInstalled(t, dialecttest.Base{Dir: t.TempDir()}, dash.PromptStyle())
}

// And this dialect's answer is deliberately the empty escape table, recorded
// here so that a row appearing in it later is a change somebody made rather
// than a change nobody noticed.
func TestThisDialectHasNoPromptEscapeLanguage(t *testing.T) {
	st := dash.PromptStyle()
	if st.Escape != 0 {
		t.Errorf("Escape = %q, want none: measured, dash draws `\\u` as `\\u`", st.Escape)
	}
	if len(st.Codes) != 0 {
		t.Errorf("Codes = %v, want none", st.Codes)
	}
	if st.Expand == nil {
		t.Error("Expand is nil: measured, an inherited PS1='<$LOGNAME>@ ' draws the name")
	}
}
