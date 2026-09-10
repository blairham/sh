// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// What a declaration that takes a fresh scope finds in the cell it declares
// into: nothing, including the array or the table the outer name held.
//
// shadow says so already — "the cell the declaration is about to write is new
// and holds nothing, whatever the outer name held" — and it was true of the
// scalar table alone. Arrays and keyed tables live in tables of their own, and
// nothing took the outer name's elements out of view, so a declaration into a
// fresh scope declared into a cell that was still holding the caller's
// compound (#1660).
//
// Measured 2026-09-09 with `-f` / `--norc --noprofile`, over a caller's
// `arr=(a b c)`. Four shapes go wrong and three controls say why:
//
//	f(){ local -a arr; … }          bash5.3 ${#arr[@]} 0   zsh $#arr 0
//	f(){ local    arr; … }          bash5.3 ${#arr[@]} 0   zsh $#arr 0
//	f(){ local    arr=x;  … }       bash5.3 declare -- arr="x"
//	f(){ local    arr[1]=z; … }     bash5.3 declare -a arr=([1]="z")
//	  --- controls, no scope taken ---
//	declare arr=x                   bash5.3 declare -a arr=([0]="x" [1]="b" [2]="c")
//	declare arr[1]=z                bash5.3 declare -a arr=([0]="a" [1]="z" [2]="c")
//	f(){ declare -g arr=x; … }      bash5.3 declare -a arr=([0]="x" [1]="b" [2]="c")
//
// The controls are the whole of the rule. The same three lines that replace
// the elements inside a function *merge* with them at the top level and under
// `-g`, and the only difference is whether a scope was taken — so this is the
// fresh cell and not an answer about what a scalar store does to a compound,
// which is Semantics.ScalarAssignedOverACompoundReplacesTheName and disagrees
// per dialect. Here the panel is unanimous, so it is the core's.
//
// Unanimous through two different readings of a valueless declaration, which
// is why it had been found in only one of them. Where a declared name is
// *hidden* — Semantics.DeclaredNameWithoutValueIsEmpty is no — hideVar takes
// the array away with the scalar and `local -a arr` was already right; where
// the name is set *empty* the emptying writes the scalar table, and the
// elements stayed. The dialect with the second reading is the one #1660 was
// reported against, and the dialect with the first was wrong on the other two
// shapes for the same reason.
//
// Not a rule about the caller's array: the scope has already saved a copy of
// both compound tables — see shadow — so the elements come back when the
// function returns, which every measurement above confirms from the other
// side and which every test here keeps as its control.
//
// One place, called from markDeclaredCompound, because that is the single
// step every declaration loop already takes after the shadow. Written into
// each of the three loops instead, it would be the shape a declaration bug
// here has taken twice: `local` carried the table's half of the compound mark
// and not the array's (#1535), and only the loop that was edited got the fix.

// dropTheOuterCompound takes the outer name's array and table out of view when
// this declaration is what made the cell.
//
// fresh is the shadow's own answer and not a question asked again here. A
// second declaration of a name its own scope already shadowed is writing over
// a cell that is its own, and the compound in front of it is the local the
// first declaration made: measured, `f(){ local -a a=(q w); local -a a; }`
// leaves `q w` in bash 5.3, bash 3.2 and zsh 5.9.2 alike.
func (r *Runner) dropTheOuterCompound(name string, fresh bool) {
	if !fresh {
		return
	}
	// Both kinds, because the declaration cannot know which one the caller
	// left and a name is one kind at a time: `local m` over a caller's table
	// reads back as an ordinary empty scalar in the shell that sets a
	// declared name empty, exactly as it does over a caller's array.
	delete(r.Arrays, name)
	delete(r.AssocArrays, name)
}
