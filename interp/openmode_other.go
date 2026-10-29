// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

// descriptorOpenForWriting cannot ask how a descriptor was opened here, and
// answers read-only — what it answers on a unix system for a descriptor the
// question cannot be put to.
func descriptorOpenForWriting(int) bool { return false }
