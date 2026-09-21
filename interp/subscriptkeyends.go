// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// sourceSubscript is where a subscript the *source* wrote landed in the
// expanded text, and where inside it an apostrophe-quoted run performed an
// expansion.
//
// Both halves are collected while the text is being assembled, because
// neither can be recovered from the finished string: a bracket a value
// carried is the same byte as one a script wrote, and an expansion whose
// result holds no syntax character leaves no mark behind. See
// [Runner.expandArithText], which records them on the way past.
type sourceSubscript struct {
	// start and stop bound the text between the brackets.
	start, stop int
	// runs holds the offset of the opening apostrophe of each quoted run
	// that performed an expansion, in order.
	runs []int
}

// truncateSubscriptsAtAQuotedExpansion ends each written subscript at the
// apostrophe-quoted run that performed an expansion in it, where the dialect
// says a key stops there. See Semantics.SubscriptQuotationEndsTheKey.
//
// Applied last to first, so an earlier subscript's offsets are still the ones
// it was recorded with.
func (r *Runner) truncateSubscriptsAtAQuotedExpansion(text string, subs []sourceSubscript) string {
	asked, ends := false, false
	for i := len(subs) - 1; i >= 0; i-- {
		s := subs[i]
		if len(s.runs) == 0 || s.start < 0 || s.stop > len(text) || s.start > s.stop {
			continue
		}
		if !asked {
			asked = true
			// Asked only where a subscript really holds an apostrophe run
			// that performed an expansion, so the ordinary `m[$k]` and the
			// ordinary `m['k']` never put the question.
			ends = r.ask(r.sem().SubscriptQuotationEndsTheKey,
				"a key ending at the apostrophe-quoted run that performed an expansion in it")
		}
		if !ends {
			return text
		}
		runs := make([]int, len(s.runs))
		for j, at := range s.runs {
			runs[j] = at - s.start
		}
		text = text[:s.start] + endKeyAtAQuotedExpansion(text[s.start:s.stop], runs) + text[s.stop:]
	}
	return text
}

// endKeyAtAQuotedExpansion is that truncation over one subscript's text,
// given where its quoted runs with an expansion in them open.
//
// The **last** such run decides, and what happens to it depends on whether
// anything follows it: where the run ends the subscript its own value is part
// of the key, and where text follows, the run and everything after it are
// dropped. Measured 2026-09-20, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell>
// f.sh` over a script file with standard input on the null device, with
// `typeset -A m; kq=q` and one store, listing the key ksh93u+ 2012-08-01
// ended up holding:
//
//	written        key      written             key
//	'$kq'          q        '$kq'z              (empty)
//	q'$kq'         qq       q'$kq'z             q
//	$kq'$kq'       qq       '$kq'$kq            (empty)
//	'$kq''$kq'     qq       a'$kq'b'$kq'c       a
//	'$kq'z'$kq'    q        a'$kq'b             a
//
// bash 5.3.20 keeps every character of all ten — `$kqz`, `a$kqb$kqc` —
// because it does not perform the expansion at all, and zsh 5.9.2 keeps them
// too, quotation included.
//
// The text in front of the run is read by the same rule again, which the
// two-run rows need: `'$kq'z'$kq'` is `q` because the run that ends it
// contributes `q` and the prefix `'$kq'z` truncates to nothing.
//
// The apostrophes of the run that is kept come off here rather than at quote
// removal, because what they held is read again: `m['"$kq"']` is the key `q`
// in that column, so the double quotation inside the run is removed too, and
// apostrophes still standing would have protected it. See
// subscriptQuoteRemoval, which reads what is left.
func endKeyAtAQuotedExpansion(sub string, runs []int) string {
	if len(runs) == 0 {
		return sub
	}
	open := runs[len(runs)-1]
	if open < 0 || open >= len(sub) {
		return sub
	}
	before := runs[:len(runs)-1]
	end := indexUnmarked(sub[open+1:], '\'')
	if end < 0 {
		return sub
	}
	end += open + 1
	if end == len(sub)-1 {
		// Its **content** and not its text: the apostrophes come off here
		// rather than at quote removal, because what they held is read
		// again — measured, `m['"$kq"']` is the key `q` in that column and
		// not `"q"`, so the double quotation inside is removed too, which
		// leaving the apostrophes standing would have protected.
		return endKeyAtAQuotedExpansion(sub[:open], before) + sub[open+1:end]
	}
	return endKeyAtAQuotedExpansion(sub[:open], before)
}
