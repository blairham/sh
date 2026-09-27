// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// SignalNames is the name this shell's table gives each of the kernel's
// signals, in the kernel's numbering and in the order the bare signal listing
// writes them.
//
// It is the roster `kill -l` prints with no operands, handed back as words
// rather than as a rendered listing, because a dialect can have a *parameter*
// holding the same roster and the two must not be two tables. Where a shell
// has both, they were measured to agree byte for byte before this existed —
// which is precisely the agreement a second copy would be free to lose.
//
// What it does **not** promise is that element i is signal i+1. That holds
// wherever this kernel names every number it takes, and it is the platform's
// answer rather than this function's: a kernel with more numbers than the
// table has names for leaves a position out unless the dialect has a
// rendering for an unnamed one, which is the same third answer
// [Diagnostics.KillListingUnnamedPosition] records for the listing. Asking
// for the numbering is asking the listing, so a caller that needs it reads
// this beside the shell's own `kill -l` rather than counting.
//
// Nothing here names a shell. Which signals exist is the platform's, which of
// them this shell can name is [Semantics.SignalNamesTheShellLacks], and what
// a *parameter* built out of the roster is called is the dialect's to say.
func (r *Runner) SignalNames() []string {
	cells := r.signalListing()
	names := make([]string, 0, len(cells))
	for _, c := range cells {
		names = append(names, c.text)
	}
	return names
}
