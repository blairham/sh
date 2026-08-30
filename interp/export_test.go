// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// This file exports internals to the external test package, which is the
// standard way to let tests reach them without widening the public API. The
// tests are external because a dialect package imports this one, so an
// in-package test could not import a dialect without a cycle.

// HasProcessGroups reports whether this platform has them, so a test can skip
// rather than assert something the platform cannot do.
const HasProcessGroups = hasProcessGroups
