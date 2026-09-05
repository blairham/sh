// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// unansweredAxisTail is the sentence a refusal of an unanswered axis ends
// with: the panel disagrees, and nothing chose between them.
//
// It is the substrate's central discipline said out loud, so it is worth
// saying identically every time. It was spelled out at each of the twenty-odd
// places that refuse, which is twenty-odd chances for one of them to drift.
const unansweredAxisTail = "the shells disagree here and no dialect was chosen"

// unanswered is the whole diagnostic for an axis no dialect answered: what was
// being asked, that the shells disagree about it, and — where the caller has
// supplied one — what to do about it.
//
// One function rather than a phrase repeated at every refusal, because the
// remedy is the half that was missing and a remedy appended by hand at twenty
// sites is a remedy appended at nineteen. A diagnostic that states the problem
// and not the fix is half a diagnostic; the fix is AxisRemedy's, since only the
// caller knows what it would be.
func (r *Runner) unanswered(what string) string {
	msg := what + ": " + unansweredAxisTail
	if r.AxisRemedy != "" {
		msg += "; " + r.AxisRemedy
	}
	return msg
}
