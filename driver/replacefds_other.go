// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package driver

import "os"

// placeFiles has nothing to place where descriptors are not inherited that
// way.
//
// It is the third side of a boundary that already stands down here: interp's
// childFiles hands nothing to an external child, and inheritedFiles finds
// nothing to publish, because os/exec's ExtraFiles is documented as
// unsupported on Windows and close-on-exec is a POSIX property. A table this
// platform cannot give a child is not one it can give a replacement either.
func placeFiles([]*os.File) {}
