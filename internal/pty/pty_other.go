// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package pty

import "os"

// Neither family's sequence is known here. A test asks and skips rather than
// failing: a terminal that cannot be made is not evidence about the shell.
func open() (*os.File, *os.File, error) { return nil, nil, ErrUnsupported }
