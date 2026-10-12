// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// An arithmetic expression with nothing to expand is read once, when the
// text around it is, and a function that writes the options on its way in
// does not change that.
//
// `emulate -L zsh` writes every option, `c_precedences` among them, and
// writing that one used to count as an option taking charge of the order:
// every `(( ))` after the first such call was expanded and parsed again on
// every evaluation, for the rest of the session. F-Sy-H's highlighter opens
// with `emulate -L zsh`, and 140 of the 182 expressions it evaluates per
// keystroke were read again for that alone (#6423).
//
// Allocations rather than time, so the row is deterministic: a fresh parser
// and a fresh tree are dozens of them, and an evaluation of a tree already
// read is a handful.
func TestEmulateDoesNotMakeArithmeticReadAgain(t *testing.T) {
	const call = "(( x & 1 ))"
	plain := localCostRunner(t, "x=3\n", call)
	emulated := localCostRunner(t, "x=3\nf() { emulate -L zsh; }\nf\n", call)
	plain()
	emulated()
	base, after := testing.AllocsPerRun(200, plain), testing.AllocsPerRun(200, emulated)
	if after > base+2 {
		t.Errorf("(( x & 1 )) allocates %v times after a function ran emulate -L zsh, and %v before; it is being read again", after, base)
	}
}
