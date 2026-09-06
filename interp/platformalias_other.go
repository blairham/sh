// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin

package interp

// No platform links, which is a statement about these systems rather than a
// gap left for later. See the darwin file beside this one for the bar an
// entry has to clear, and why `/bin -> usr/bin` on a merged-/usr Linux does
// not clear it.
var platformLinks [][2]string
