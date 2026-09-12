// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

import (
	"errors"
	"os"
)

// dupFile has nothing to duplicate with where descriptors are not what the
// platform hands a child — the same absence childFiles answers, so a shell
// there shares the table entry as it did before ownDescriptors existed.
func dupFile(*os.File) (*os.File, error) { return nil, errors.ErrUnsupported }
