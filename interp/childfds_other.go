// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

import "os"

// childFiles hands nothing over where descriptors are not inherited that way.
//
// os/exec's ExtraFiles is documented as unsupported on Windows, and a Start
// with one set fails there — so returning nothing is not a lesser answer, it
// is the only one that keeps external commands running at all. A script that
// passes a descriptor to a child is unsupported on such a platform, in the
// same way process groups are.
func (r *Runner) childFiles() []*os.File { return nil }
