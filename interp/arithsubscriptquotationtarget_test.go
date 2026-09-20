// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The **store** half of the refusal a subscript whose quotation never closes
// earns: a second sentence, about the operand the expression would have
// written through rather than about the subscript it would have read. See
// Diagnostics.ArithSubscriptUnclosedQuoteTarget, and
// arithsubscriptquotation_test.go for the read's own pair.
//
// The two count different things — the read's is written per subscript read
// and this one per store — so neither is the other written again, and a store
// with no read in it was one line short of what the refusing column says.
//
// Tests name axes and wordings, never shells.

// unclosedQuoteTargetDiag is the refusing axis with both sentences worded, so
// a probe can tell them apart in one stream.
func unclosedQuoteTargetDiag(r *Runner) {
	quotedKeyAxis(Yes, false)(r)
	d := CoreDiagnostics()
	d.ArithSubscriptUnclosedQuote = "%[1]s[%[2]s]: bad array subscript"
	d.ArithSubscriptUnclosedQuoteTarget = "`%[1]s[%[2]s]': not a valid identifier"
	r.Diagnostics = &d
}

func TestABadSubscriptsStoreIsRefusedInItsOwnWords(t *testing.T) {
	for _, tc := range []struct {
		name          string
		src           string
		reads, stores int
	}{
		// A store and no read at all, which is the row that says this
		// sentence is not the read's written again: the read's is never
		// written here and this one is.
		{"a store alone", `let "a[$k] = 9"`, 0, 1},
		// A read and a store to a name that is fine, which is the mirror of
		// it: the read's sentence twice and this one never.
		{"a read alone", `let "x = a[$k] + 1"`, 2, 0},
		// Both at once — three lines.
		{"an increment", `let "++a[$k]"`, 2, 1},
		{"an assignment operator", `let "a[$k] += 1"`, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, quotedKeyTable+tc.src+`; printf '[%s]' "${a[$k]}"`,
				quotedKeyGrammar, unclosedQuoteTargetDiag)
			if reads := strings.Count(out, "a[q'r]: bad array subscript"); reads != tc.reads {
				t.Errorf("%d read refusals in %q, want %d", reads, out, tc.reads)
			}
			if stores := strings.Count(out, "`a[q'r]': not a valid identifier"); stores != tc.stores {
				t.Errorf("%d store refusals in %q, want %d", stores, out, tc.stores)
			}
			// The element is left as it was and the expression still
			// finishes, under every one of the four: this is a complaint
			// about an operand and not a failure of the evaluation.
			if !strings.Contains(out, "[4]") || st != 0 {
				t.Errorf("%q (status %d), want the element left at [4] at 0", out, st)
			}
		})
	}
}

// The dialect that reads the key rather than refusing the subscript writes
// neither sentence and stores through the brackets as it always did, which is
// what says both are the one axis's and not a second one nobody answered.
func TestTheReadingAnswerWritesNeitherRefusal(t *testing.T) {
	out, st := runGrammar(t, quotedKeyTable+`let "a[$k] = 9"; printf '[%s]' "${a[$k]}"`,
		quotedKeyGrammar, func(r *Runner) {
			quotedKeyAxis(No, false)(r)
			d := CoreDiagnostics()
			d.ArithSubscriptUnclosedQuoteTarget = "`%[1]s[%[2]s]': not a valid identifier"
			r.Diagnostics = &d
		})
	if out != "[9]" || st != 0 {
		t.Errorf("%q (status %d), want %q at 0", out, st, "[9]")
	}
}

// A dialect that leaves the wording empty writes the sentence anyway, in the
// words Wording falls back to — the refusal is the axis's and the field is
// only how this dialect spells it.
func TestAnUnwordedStoreRefusalStillReportsTheOperand(t *testing.T) {
	out, _ := runGrammar(t, quotedKeyTable+`let "a[$k] = 9"`,
		quotedKeyGrammar, quotedKeyAxis(Yes, false))
	if !strings.Contains(out, "a[q'r]") {
		t.Errorf("%q, want the operand named", out)
	}
}

// **Only an associative name.** The refusing column does not write this
// sentence about an indexed name, an unset one or a scalar — it fails the
// whole expression as unreadable arithmetic instead, which is a different
// answer at a different status and is not what this sentence stands for. See
// Runner.reportArithSubscriptUnclosedQuoteTarget.
func TestAStoreThroughANameThatIsNoTableIsNotRefusedInTheseWords(t *testing.T) {
	for _, tc := range []struct{ name, table string }{
		{"an indexed array", `b=(1 2 3); `},
		{"a name that is not set", ``},
		{"a scalar", `b=hello; `},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runGrammar(t, `k="q'r"; `+tc.table+`let "b[$k] = 9"`,
				quotedKeyGrammar, unclosedQuoteTargetDiag)
			if strings.Contains(out, "not a valid identifier") {
				t.Errorf("%q, want no store refusal", out)
			}
		})
	}
}
