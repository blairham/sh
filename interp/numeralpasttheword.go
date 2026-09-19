// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math"
	"strings"
)

// NumeralPastTheWord is what a well-formed integer numeral larger than the
// machine word comes to — see [Semantics.ArithNumeralPastTheWord].
//
// Four readings, and each is a rule rather than an accident, so this is an
// enumeration and not a pair of booleans. Measured 2026-09-16, each probe a
// script file under `env -i`:
//
//	                       10000000000000000000   0xffffffffffffffff
//	bash 5.3.20            -8446744073709551616   -1
//	bash as `sh`           -8446744073709551616   -1
//	bash 3.2.57            -8446744073709551616   -1
//	BusyBox ash 1.37.0     -8446744073709551616   -1
//	zsh 5.9.2               1000000000000000000    1152921504606846975
//	dash 0.5.12             9223372036854775807    9223372036854775807
//	ksh93u+ 2012-08-01      1e+19                 -1
//
// Every column answers, and every column reports status 0 — refusing the
// numeral is a reading nobody has. ksh93's is not one of these:
// [Semantics.ArithValuesAreCarriedInAFloat] is asked before this axis and
// answers the question there instead.
//
// Not by going straight to the double, which the `-1` in the table above has
// always said and the sentence that used to stand here denied. A numeral with
// an explicit radix is read in the unsigned word there too, and becomes the
// double only when *that* overflows — `01777777777777777777777` is -1 and
// `02000000000000000000000` is `2e+21`. See readArithNum and #3257.
type NumeralPastTheWord uint8

const (
	// NumeralPastTheWordUnspecified is no answer, and is refused like any
	// other axis nothing chose.
	NumeralPastTheWordUnspecified NumeralPastTheWord = iota

	// NumeralPastTheWordWraps reads the digits in an unsigned word and lets
	// it go round, then reads the result as signed — C's `strtoull` with the
	// range error thrown away. bash 5.3, bash as `sh`, bash 3.2 and BusyBox
	// ash.
	//
	// It is arithmetic modulo 2^64 and not a saturation at the top of it:
	// `18446744073709551615` is -1 and `18446744073709551616` is 0, where a
	// reader that stopped at the largest unsigned value would answer -1
	// twice.
	NumeralPastTheWordWraps

	// NumeralPastTheWordKeepsTheDigitsThatFit stops reading part way and says
	// how many digits it got through, leaving the script running. zsh alone,
	// and the only column that says anything at all.
	//
	// The rule is in truncatedNumeral, which is worth reading before writing
	// a case for this: the shell that does it does *not* stop at the last
	// digit that fits.
	NumeralPastTheWordKeepsTheDigitsThatFit

	// NumeralPastTheWordSaturates stops at the largest value the signed word
	// holds, silently. dash, where every numeral past the word — decimal,
	// hexadecimal or octal — is 9223372036854775807.
	NumeralPastTheWordSaturates
)

func (p NumeralPastTheWord) String() string {
	switch p {
	case NumeralPastTheWordWraps:
		return "wraps"
	case NumeralPastTheWordKeepsTheDigitsThatFit:
		return "keeps the digits that fit"
	case NumeralPastTheWordSaturates:
		return "saturates"
	}
	return "unspecified"
}

// wrappedNumeral reads a digit run in an unsigned word and lets it go round.
func wrappedNumeral(digits string, base int) int64 {
	var v uint64
	for i := range len(digits) {
		d, _ := baseDigitValue(digits[i], base)
		v = v*uint64(base) + uint64(d)
	}
	return int64(v)
}

// truncatedNumeral is zsh's reading: the value it keeps and how many digits it
// read to get there, the second being what its diagnostic counts.
//
// The obvious model — take digits while they still fit the signed word — is
// wrong, and wrong in a way a probe near the edge of the word cannot see. Two
// things separate them. The accumulator is *unsigned* and it wraps: reading
// stops only when a digit fails to make the running value larger, so
// `22222222222222222222` is past the unsigned word and is still read whole and
// silently, at 3775478148512670606. And where the digits run out with the
// value merely past the *signed* word, the last digit is dropped by dividing
// the wrapped value rather than by going back to the true prefix — which is
// why `50000000000000000000` is 1310651185258089676 and not 5000000000000000000.
//
// Twenty-five rows were measured against zsh 5.9.2 and this reproduces every
// one, including four that the fit-while-it-fits model gets wrong in the value
// and three more it gets wrong only in the count.
func truncatedNumeral(digits string, base int) (int64, int, bool) {
	var v uint64
	read := 0
	for i := range len(digits) {
		d, _ := baseDigitValue(digits[i], base)
		nv := v*uint64(base) + uint64(d)
		if nv < v {
			// The digit did not make the value larger, so the word went
			// round. Reading stops here and what is already in hand stands,
			// past the signed word or not.
			return int64(v), read, true
		}
		v, read = nv, read+1
	}
	if v > math.MaxInt64 {
		// Every digit was read and the result is past the signed word. The
		// shell drops one, and drops it from the value it has rather than
		// from the numeral, so a run that wrapped on the way here keeps the
		// wrap.
		return int64(v / uint64(base)), read - 1, true
	}
	return int64(v), read, false
}

// numeralPastTheWord answers a well-formed numeral the machine word cannot
// hold, reporting whether the axis was answered at all.
//
// A truncation is written here rather than returned, because it is not a
// failure: the one column that says anything says it and carries on with the
// value beside it. The unanswered refusal *is* one, so it is left to the
// caller to word — writing it here and returning an error as well is what
// makes the base-above-36 refusal print twice.
func (r *Runner) numeralPastTheWord(digits string, base int, tail string) (int64, bool) {
	switch p := r.sem().ArithNumeralPastTheWord; p {
	case NumeralPastTheWordWraps:
		return wrappedNumeral(digits, base), true
	case NumeralPastTheWordKeepsTheDigitsThatFit:
		n, read, cut := truncatedNumeral(digits, base)
		if cut && r.diag().ArithNumberTruncated != "" {
			// What is quoted back is the expression from the numeral to the
			// end of it, not the digits that were read — so `$(( big + 1 ))`
			// quotes `big + 1` and `$(( big ))` quotes the digits and the
			// blank after them. See [syntax.ArithNum].Tail.
			quoted := digits
			if i := strings.Index(tail, digits); i >= 0 {
				quoted = tail[i:]
			}
			r.diagf("%s\n", Wording(r.diag().ArithNumberTruncated,
				"number truncated after %[1]d digits: %[2]s", read, quoted))
		}
		return n, true
	case NumeralPastTheWordSaturates:
		return math.MaxInt64, true
	case NumeralPastTheWordUnspecified:
	}
	r.status = 2
	r.unspecified = true
	return 0, false
}
