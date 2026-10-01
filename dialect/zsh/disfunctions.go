// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `$dis_functions` and `$dis_functions_source` are the switched-off half of
// `$functions` and `$functions_source`: every function `disable -f` took out
// of the live table, to its body and to the file it was read from.
//
// Measured 2026-10-01 on zsh 5.9.2 (`-f`, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`), with `zq() { print zq; }` and `disable -f zq`:
//
//	${dis_functions[zq]}          the body, a tab and `print zq`, as
//	                              ${functions[zq]} writes a live one
//	${dis_functions_source[zq]}   the script's path, as functions_source
//	dis_functions[zz]='print zz'  0, and zz is a *switched-off* function:
//	                              ${+functions[zz]} 0, ${+dis_functions[zz]} 1
//	unset 'dis_functions[zq]'     0, and zq is gone from both tables
//
// They were empty views until `disable -f` existed (#5267); see
// zshEmptyParams.
func registerDisabledFunctions(r *interp.Runner) {
	r.SetDynamicAssoc("dis_functions", func(rr *interp.Runner) interp.AssocArray {
		out := interp.AssocArray{}
		for _, name := range rr.WithdrawnFunctionNames() {
			if body, ok := rr.WithdrawnFunctionBodyText(name); ok {
				out[name] = interp.Scalar(body)
			}
		}
		return out
	})
	r.SetDynamicAssocElement("dis_functions", func(rr *interp.Runner, name string) (string, bool) {
		return rr.WithdrawnFunctionBodyText(name)
	})
	r.SetDynamicAssocWriter("dis_functions", func(rr *interp.Runner, name, body string, set bool) {
		if !set {
			rr.RemoveWithdrawnFunction(name)
			return
		}
		// Defined live and then switched off, so the body is read by the
		// same route `functions[name]=` takes and nothing here parses it a
		// second way.
		writeZshFunction(rr, name, body, true)
		rr.SetFunctionWithdrawn(name, true)
	})
	hideModuleParameter(r, "dis_functions")

	r.SetDynamicAssoc("dis_functions_source", func(rr *interp.Runner) interp.AssocArray {
		out := interp.AssocArray{}
		for _, name := range rr.WithdrawnFunctionNames() {
			out[name] = interp.Scalar(rr.WithdrawnFunctionSourceFile(name))
		}
		return out
	})
	r.SetDynamicAssocElement("dis_functions_source", func(rr *interp.Runner, name string) (string, bool) {
		if !rr.FunctionWithdrawn(name) {
			return "", false
		}
		return rr.WithdrawnFunctionSourceFile(name), true
	})
	r.MarkReadonly("dis_functions_source")
	hideModuleParameter(r, "dis_functions_source")
}
