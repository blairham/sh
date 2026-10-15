// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// A refused IO number is a redirection, and is worded as one rather than
// named `IO number` (#6552). Measured 2026-10-15: `case 2>x in x) esac` is
// `redirection unexpected (expecting word)` on dash 0.5.12 and
// `unexpected redirection (expecting word)` on BusyBox ash 1.37.0.
func TestARefusedIONumberIsARedirection(t *testing.T) {
	_, err := syntax.Parse("case 2>x in x) esac\n", dash.Dialect())
	if err == nil {
		t.Fatal("parsed, want a refusal")
	}
	got := dash.Diagnostics().ParseFailure(err)
	if want := "Syntax error: redirection unexpected (expecting word)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
