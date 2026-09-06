// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin

package policy

// No platform aliases, which is a statement about these systems rather than a
// gap left for later.
//
// Linux has nothing of the shape macOS's `/tmp -> private/tmp` has: `/tmp` is a
// directory, and it is the same directory by the only name it has. The nearest
// thing is a distribution merging `/bin` into `/usr/bin`, and that is
// deliberately not here — it is a *distribution's* arrangement rather than the
// platform's, it is absent on machines that predate it or decline it, and a
// table that assumed it would make one policy file mean two things with nothing
// in the file to say so. An ordinary symbolic link, wherever it came from, is
// not this file's business at all: it is caught at the open rather than at the
// rule — see alias.go, and interp/verifyopen.go.
var platformAliases []alias
