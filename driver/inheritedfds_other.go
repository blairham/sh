// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package driver

import "os"

// inheritedFiles finds nothing where descriptors are not inherited that way.
//
// The other half of this boundary already stands down here — os/exec's
// ExtraFiles is documented as unsupported on Windows and a Start with one set
// fails there, so interp's childFiles hands nothing over either. Publishing an
// inbound descriptor a child could never be given back would be half a
// feature, and the discriminator itself does not exist: close-on-exec is a
// POSIX property, and the process's open descriptors are not listed under a
// path.
func inheritedFiles() []*os.File { return nil }
