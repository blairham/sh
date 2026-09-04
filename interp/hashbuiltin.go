// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// hash consults the builtin table through lookupBuiltin, so like `eval` and
// `.` it is registered in an init rather than in the map literal Go would
// call a cycle.
func init() { builtins["hash"] = biHash }

// biHash is `hash`, honest about the cache this shell does not keep.
//
// Every shell measured accepts a bare `hash` and `hash -r` with status 0 —
// scripts call both defensively — and since command lookup here is never
// cached, an empty table and a cleared one are the same true answer. Naming
// a command checks that it could run: found is silent success, and a name
// that resolves to nothing is the dialect's question — three report it at
// status 1, ksh93 says nothing and reports success.
func biHash(r *Runner, _ context.Context, args []string) int {
	args, opts, code := r.builtinOptions("hash", args, "r")
	if code != 0 {
		return code
	}
	if len(args) == 0 {
		if strings.ContainsRune(opts, 'r') {
			// Forgetting a cache that does not exist succeeds quietly.
			return 0
		}
		// One dialect announces the empty table, on standard output.
		if w := r.diag().HashEmptyTable; w != "" {
			r.printf("%s\n", w)
		}
		return 0
	}
	status := 0
	for _, name := range args {
		if !r.ask(r.sem().HashSearchesPathAlone, "`hash` counting only what PATH holds") {
			if r.unspecified {
				return r.status
			}
			// A builtin or a function could run, so it hashes.
			if _, ok := r.lookupBuiltin(name); ok {
				continue
			}
			if _, ok := r.funcs[name]; ok {
				continue
			}
		}
		if r.unspecified {
			return r.status
		}
		if _, err := r.lookPath(name); err == nil {
			continue
		}
		if r.ask(r.sem().HashReportsAMissingName, "`hash` reporting a name that resolves to nothing") {
			r.diagf("%s\n", Wording(r.diag().HashNotFound, "hash: %[1]s: not found", name))
			status = 1
		}
		if r.unspecified {
			return r.status
		}
	}
	return status
}
