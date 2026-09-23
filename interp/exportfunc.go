// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A function carried to a child through the environment.
//
// A shell cannot hand a child a function the way it hands it a variable —
// there is nothing but a string to put it in — so the one dialect that does
// this writes the *source* into the environment under a name of its own, and
// a shell starting up reads it back and parses it. Which is why this needs a
// printer: the shell has the tree and has to say it again.
//
// The naming is the dialect's, because only one shell in the panel does it at
// all. The core keeps the set and renders the value; what the entry is called
// comes from above.

// SetFunctionLayout says how this shell lays a function out: shown to a
// person, and written into the environment.
//
// Two arrangements because they differ, and both the dialect's because an
// arrangement is one shell's taste. The zero value of either is the neutral
// one, which keeps whatever line structure the source had.
func (r *Runner) SetFunctionLayout(shown, exported syntax.Layout) {
	r.functionLayout, r.exportedFunctionLayout = shown, exported
}

// SetFunctionExport says how a function is written into the environment: the
// text before the name and the text after it.
//
// Empty means this shell does not carry functions that way, which is three of
// the four — and then `export -f` has nothing to do rather than doing it
// invisibly.
func (r *Runner) SetFunctionExport(prefix, suffix string) {
	r.funcExportPrefix, r.funcExportSuffix = prefix, suffix
}

// exportFuncs answers `export -f`, and `export -nf`, which takes the attribute
// back off.
//
// The `-n` direction was not read here at all until #4143: `export -f ab;
// export -nf ab` left the function in the environment, so a script that had
// stopped exporting a function went on handing it to every child. Measured on
// bash 5.3.20, the entry is gone and the status is 0 — and a name that is not a
// function is still refused either way round, which is why that check stands in
// front of the split rather than inside one arm.
func (r *Runner) exportFuncs(names []string, remove bool) int {
	status := 0
	for _, name := range names {
		if _, ok := r.funcs[name]; !ok {
			// Refused rather than remembered: a name that is not a function
			// now will not become one by being exported, and every shell
			// that has this option says so.
			r.diagf("%s\n", Wording(r.diag().ExportNotAFunction, "export: %[1]s: not a function", name))
			status = 1
			continue
		}
		if remove {
			// Nothing is being put anywhere, so a name that could not be
			// carried is not a refusal: measured, `export -nf 'a=b'` is silent
			// at 0 where `export -f 'a=b'` is `cannot export` at 1.
			delete(r.exportedFuncs, name)
			continue
		}
		if r.funcNameCannotBeExported(name) {
			r.diagf("%s\n", Wording(r.diag().ExportCannotExportFunction,
				"export: %[1]s: cannot export", name))
			status = 1
			continue
		}
		if r.exportedFuncs == nil {
			r.exportedFuncs = map[string]bool{}
		}
		r.exportedFuncs[name] = true
	}
	return status
}

// funcNameCannotBeExported reports whether a function's name is one this shell
// will not carry in the environment.
//
// A function name is nearly anything, and an environment entry's name is not:
// the entry is the name with this dialect's prefix and suffix around it, and a
// name holding an `=` would make an entry no reader could take apart.
//
// **Measured rather than derived from that argument**, because the argument
// alone would stop at the equals sign. On bash 5.3.20, and identically on the
// 5.3.15 the suite is graded against in its image: `=` and `/` are refused —
// `export -f 'foo=bar'` and `export -f '/bin/echo'` are each `cannot export` at
// 1 — and `a-b`, `a.b`, `a+b`, `a:b`, `a!b` and `@x` all export at 0. So `/` is
// this shell's own line and not POSIX's, and a rule written from the entry's
// grammar would have let it through.
//
// Three things about the refusal are measured too. It comes **after** the
// not-a-function check, so `export -f 'nosuch=name'` is `not a function`. It
// does **not** stop the operand list: `export -f a=b ok` refuses the first,
// exports `ok`, and reports 1 at the end. And it is only the exporting
// direction — `export -nf 'a=b'` is silent at 0, since nothing is being put
// anywhere.
//
// Behind the dialect having a form for it at all, which is the same test
// functionEnviron makes: three of the four carry no functions in the
// environment, and a name that cannot be carried is not a refusal in a shell
// that carries none of them.
func (r *Runner) funcNameCannotBeExported(name string) bool {
	if r.funcExportPrefix == "" && r.funcExportSuffix == "" {
		return false
	}
	return strings.ContainsAny(name, "=/")
}

// functionEnviron is the environment entries for the exported functions.
func (r *Runner) functionEnviron() []string {
	if r.funcExportPrefix == "" && r.funcExportSuffix == "" {
		return nil
	}
	out := make([]string, 0, len(r.exportedFuncs))
	for name := range r.exportedFuncs {
		fn, ok := r.funcs[name]
		if !ok {
			// Defined, exported, then unset. The child is told nothing
			// rather than told about a function this shell no longer has.
			continue
		}
		body := syntax.PrintWith(fn.Body, r.exportedFunctionLayout)
		out = append(out, r.funcExportPrefix+name+r.funcExportSuffix+"=() "+body)
	}
	return out
}

// ensureImportedFunctions reads the environment's functions, once.
func (r *Runner) ensureImportedFunctions() {
	if r.importedFuncs {
		return
	}
	r.importedFuncs = true
	r.importFunctions()
}

// importFunctions defines the functions the environment carries.
//
// Read once, when the shell starts, and parsed with this shell's own grammar
// — the text came from a shell that may not have been this one, and what it
// means here is what this grammar says it means.
func (r *Runner) importFunctions() {
	if r.funcExportPrefix == "" && r.funcExportSuffix == "" {
		return
	}
	// r.Env and never os.Environ: what functions arrived is decided by the
	// environment the embedder handed this Runner, not by the process's.
	for _, kv := range r.Env {
		key, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, r.funcExportPrefix) || !strings.HasSuffix(key, r.funcExportSuffix) {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, r.funcExportPrefix), r.funcExportSuffix)
		if name == "" {
			continue
		}
		// The value is a function *body* with its parentheses, so it becomes
		// a declaration by having the name put back in front of it.
		f, err := syntax.Parse(name+value, r.dialect())
		if err != nil || len(f.Stmts) != 1 {
			// Not something this shell can read. Ignored rather than
			// reported: it came from the environment, which is not this
			// script's doing and not something it can fix.
			continue
		}
		pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
		if !ok || len(pipe.Cmds) != 1 {
			continue
		}
		if decl, ok := pipe.Cmds[0].(*syntax.FuncDecl); ok {
			if r.funcs == nil {
				r.funcs = map[string]*syntax.FuncDecl{}
			}
			r.funcs[name] = decl
		}
	}
}
