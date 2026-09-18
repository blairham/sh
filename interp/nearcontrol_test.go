// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Diagnostics.NearTextEscapesControlCharacters reaches **both** messages that
// quote a script's own text back, and this is the half a dialect suite cannot
// reach: a construct the input ran out inside is refused while the file is
// read, so nothing runs and no runner is involved.
//
// The other message — a substitution body that would not parse — is exercised
// in dialect/nearcontrol_test.go against the preset that answers yes. Between
// them the claim that the two move together is checked at both ends rather
// than asserted once.
func TestTheUnmatchedConstructsQuoteEscapesControlCharactersToo(t *testing.T) {
	d := syntax.Core()
	_, err := syntax.Parse("v=$(echo\ta\n", d)
	if err == nil {
		t.Fatal("the text parsed, so there is no refusal to render")
	}
	dg := Diagnostics{
		UnmatchedCmdSubst:     "parse error near `%[3]s'",
		UnmatchedNearMaxBytes: 20,
	}
	if got := dg.ParseFailure(err); !strings.Contains(got, "`v=$(echo\ta'") {
		t.Fatalf("with the field off the message was %q, want the tab written through", got)
	}
	dg.NearTextEscapesControlCharacters = true
	if got := dg.ParseFailure(err); !strings.Contains(got, "`v=$(echo\\ta'") {
		t.Errorf("with the field on the message was %q, want the tab written as an escape", got)
	}
}
