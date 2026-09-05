// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package pty

import "os"

// SetSize has no pseudo-terminal to size here, for the same reason Open has
// none to open.
func SetSize(*os.File, int, int) error { return ErrUnsupported }
