// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// matchOperatorByTable is the operator scan as it was written before the
// first-byte check (#5873): every entry of the table, in order, asking the
// dialect first and the text second. It is the reference the fast scan has to
// agree with.
func matchOperatorByTable(l *Lexer) (Kind, bool) {
	rest := l.src[l.off:]
	for _, k := range operators {
		if !l.enabled(k) {
			continue
		}
		if strings.HasPrefix(rest, text[k]) {
			return k, true
		}
	}
	return 0, false
}

// TestOperatorScanAgreesWithTheTable puts every operator spelling, every
// spelling run into the bytes that could extend it, and a sample of words
// through both scans, under dialects with each optional operator family on
// and off, inside and outside a case arm's parentheses.
func TestOperatorScanAgreesWithTheTable(t *testing.T) {
	all := Core()
	all.AmpersandRedirect = true
	all.AmpersandAppendRedirect = true
	all.CaseFallthrough = true
	all.CaseContinue = true
	all.CaseContinuePipe = true
	all.Herestring = true
	all.BackgroundAndDisown = true
	all.PipeBothStreams = true
	all.CoprocPipeOperator = true
	all.CasePatternListPipeIsOnlyASeparator = true
	all.ClobberOverrideMarker = true
	all.RenameOnSuccessRedirect = true
	all.SeekRedirect = true
	all.ReversedAmpersandRedirect = true
	dialects := map[string]Dialect{"core": Core(), "posix": POSIX(), "every family": all}

	var inputs []string
	for _, k := range operators {
		s := text[k]
		inputs = append(inputs, s)
		for _, c := range []string{"", "&", "|", ";", "<", ">", "!", "#", "-", "(", ")", "a", " "} {
			inputs = append(inputs, s+c, s+c+"x")
		}
	}
	inputs = append(inputs, "", "a", "echo", "$x", "'q'", `"q"`, "#c", "!", "{", "}", "[[", "]]", "=", "~", "1>&2")

	for name, d := range dialects {
		for _, inCase := range []bool{false, true} {
			for _, in := range inputs {
				l := NewLexer(in, d)
				l.inCaseParenList = inCase
				gotK, gotOK := l.matchOperator()
				wantK, wantOK := matchOperatorByTable(l)
				if gotK != wantK || gotOK != wantOK {
					t.Errorf("%s, case parens %v, %q: scan says %v %v, table says %v %v",
						name, inCase, in, gotK, gotOK, wantK, wantOK)
				}
			}
		}
	}
}

// TestAWordIsNotAnOperatorCandidate pins the first-byte check: no operator
// spelling begins with a byte a word can begin with, so the table is never
// walked for one.
func TestAWordIsNotAnOperatorCandidate(t *testing.T) {
	for _, k := range operators {
		c := text[k][0]
		if !strings.ContainsRune("&|;()<>", rune(c)) {
			t.Errorf("operator %q begins with %q, outside the set a word cannot begin with", text[k], c)
		}
	}
	for _, c := range []byte("abcXYZ019_$'\"{}[]=~-+.,/:@%^*?") {
		if operatorStarts[c] {
			t.Errorf("%q is marked as beginning an operator", c)
		}
	}
}
