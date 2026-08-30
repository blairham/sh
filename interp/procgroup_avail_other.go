// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

// hasProcessGroups says whether this platform has them, so a test can skip
// rather than fail where the concept does not exist.
const hasProcessGroups = false
