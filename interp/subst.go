// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// commandSubst runs the text of a `$( … )` and returns what it wrote.
//
// The inner text was kept raw by the lexer rather than tokenized, because
// what is inside is a program. So it is parsed here, with the same dialect,
// and run on a copy of the state: a substitution is a subshell, and nothing
// it assigns escapes.
//
// Trailing newlines are removed, which is the rule that makes `x=$(pwd)`
// usable at all.
func (r *Runner) commandSubst(ctx context.Context, src string) string {
	p := syntax.NewParser(src, r.dialect())
	f := p.Parse()
	if err := p.Err(); err != nil {
		r.errf("sh: %v\n", err)
		r.status = 2
		return ""
	}

	var out bytes.Buffer
	sub := r.clone()
	sub.Stdout = &out
	if _, err := sub.Run(ctx, f); err != nil {
		r.errf("sh: %v\n", err)
		return ""
	}
	// The status of a substitution is the status of what ran inside it, which
	// `x=$(false)` relies on.
	r.status = sub.status
	return strings.TrimRight(out.String(), "\n")
}

// dialect is the dialect this runner parses nested input with. It is a method
// rather than a field so the default is the core rather than the zero value,
// which would be posix and would refuse constructs the outer parse accepted.
func (r *Runner) dialect() syntax.Dialect {
	if r.Dialect != nil {
		return *r.Dialect
	}
	return syntax.Core()
}
