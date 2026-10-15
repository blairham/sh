// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A dialect fills the runner's per-name tables at every start — the builtins
// it adds, the parameters it stores or marks, the math functions it brings —
// and each table was made empty and doubled into its size one insert at a
// time. A Go map that grows rehashes every name it holds at each doubling, and
// a slice that grows copies, so a table that ends at 120 names was paid for
// about twice. Profiled in process on the x86 runner, that growth was nearly
// a third of the largest dialect's setup (#6491).
//
// TableSizes is how a dialect says, before it registers anything, how large
// each table is about to be, so that each is made once at the size it ends at.
// It changes nothing a script can see: a reservation only decides how much
// room a table starts with.

// TableSizes is how many names each of a runner's per-name tables holds, or
// is about to. [Runner.ReserveTables] takes one as a plan and
// [Runner.TableSizes] reports one as a measurement, so a dialect can test that
// its plan still covers what it registers.
type TableSizes struct {
	// Parameters is every name the shell keeps a value, an attribute or a
	// waiting value under.
	Parameters int
	// Builtins is the builtins the dialect adds, replaces or takes away.
	Builtins int
	// KindFixed, ScopeFixed and ShellOwn are the names marked by
	// [Runner.MarkParameterKindFixed], [Runner.MarkParameterScopeFixed] and
	// [Runner.MarkShellOwnParameter].
	KindFixed, ScopeFixed, ShellOwn int
	// Deferred is the names given to [Runner.SetDeferredParameter].
	Deferred int
	// MathFunctions and WithdrawnMathFunctions are the functions arithmetic
	// can call and the ones withdrawn until a module is loaded.
	MathFunctions, WithdrawnMathFunctions int
	// WithdrawnBuiltins is the builtins withdrawn until a module is loaded.
	WithdrawnBuiltins int
	// NotAdopted is the environment entries the shell does not adopt.
	NotAdopted int
	// Declarations is the produced parameters given a declaration.
	Declarations int
}

// ReserveTables makes each table s names a size for, at that size, so that
// registering that many names grows none of them. A table that already
// exists is left as it is: this is for a dialect about to register into a
// runner nothing has written yet, which is where every table is still nil.
func (r *Runner) ReserveTables(s TableSizes) {
	if r.cellIndex == nil && r.cellIndexShare == nil && s.Parameters > 0 {
		r.cellIndex = make(map[string]int32, s.Parameters)
		if r.cellSlab == nil {
			r.cellSlab = make([]varCell, 0, s.Parameters)
		}
	}
	reserve(&r.custom, s.Builtins)
	reserve(&r.kindFixed, s.KindFixed)
	reserve(&r.scopeFixed, s.ScopeFixed)
	reserve(&r.shellOwn, s.ShellOwn)
	reserve(&r.deferredParams, s.Deferred)
	reserve(&r.mathFuncs, s.MathFunctions)
	reserve(&r.withdrawnMathFuncs, s.WithdrawnMathFunctions)
	reserve(&r.withdrawnBuiltins, s.WithdrawnBuiltins)
	reserve(&r.envNotAdopted, s.NotAdopted)
	reserve(&r.dynamicDeclarations, s.Declarations)
}

// reserve makes the table behind m at room for n names, if it is not made
// yet and n asks for any.
func reserve[K comparable, V any](m *map[K]V, n int) {
	if *m == nil && n > 0 {
		*m = make(map[K]V, n)
	}
}

// TableSizes reports how many names each table holds now.
func (r *Runner) TableSizes() TableSizes {
	return TableSizes{
		Parameters:             len(r.cellIndex),
		Builtins:               len(r.custom),
		KindFixed:              len(r.kindFixed),
		ScopeFixed:             len(r.scopeFixed),
		ShellOwn:               len(r.shellOwn),
		Deferred:               len(r.deferredParams),
		MathFunctions:          len(r.mathFuncs),
		WithdrawnMathFunctions: len(r.withdrawnMathFuncs),
		WithdrawnBuiltins:      len(r.withdrawnBuiltins),
		NotAdopted:             len(r.envNotAdopted),
		Declarations:           len(r.dynamicDeclarations),
	}
}
