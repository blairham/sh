// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "github.com/blairham/sh/interp"

// diagTable is the dialect's table, addressable, for a test that calls one of
// its methods on the constructor's result: the methods take the table by
// pointer (#6610), and a function's result is not addressable.
func diagTable(d interp.Diagnostics) *interp.Diagnostics { return &d }
