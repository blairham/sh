// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import "github.com/blairham/sh/interp"

// promptDefaults puts this dialect's default prompts into PS1 and PS2, so a
// person's run-commands file can read them.
//
// The values themselves are the dialect's and are already written down —
// interp.PromptStyle.Default and DefaultContinued, which the prompt drawer has
// always fallen back to. What was missing is that they are *parameters* and
// not only a fallback: every shell in the panel assigns them, and the first
// thing a real `~/.bashrc` does is read one.
//
//	[ -z "$PS1" ] && return
//
// That line is how an rc bails out when there is nobody to prompt, and it is
// the most common first line there is. With PS1 empty in an interactive shell
// the guard fires at a prompt and the whole file is skipped — no aliases, no
// functions, no prompt, at status 0 with nothing said (#1421).
//
// Three rules, each measured rather than reasoned about, and each one is what
// keeps the guard working in the direction it was written for:
//
//   - Only when the shell is interactive. bash 5.3.15, bash 3.2.57, bash under
//     argv[0] `sh` and ksh93 all leave PS1 *unset* in a shell with nobody to
//     prompt — on `-c` and on a script file alike — and that is precisely what
//     the guard detects. Assigning unconditionally would get the value right
//     and break the guard from the other side.
//
//   - Only when the name is not already set. An inherited `PS1='INH> '`
//     reaches the rc as `INH> `, and an inherited *empty* `PS1=` reaches it
//     empty: the rule is unset, not empty, because `PS1=` is a prompt of
//     nothing that somebody asked for.
//
//   - Not exported. With no inherited PS1, bash's `export -p` names none and
//     a child's environment has none. An inherited exported one keeps its
//     export attribute, which falls out of not touching it.
//
// `--norc`, `--noprofile` and `-f` do not suppress this. Measured: with no
// startup file read at all, every column still has its default in hand. So
// this runs before the escape hatch in startup rather than after it.
func (sh Shell) promptDefaults(r *interp.Runner, in source, afterTheFiles bool) {
	if sh.PromptStyle.DefaultsFollowTheStartupFiles != afterTheFiles {
		return
	}
	st := sh.PromptStyle
	ps1, ps2, say := st.Default, st.DefaultContinued, true
	if !in.interactive {
		// Two of the panel assign a prompt to a shell with nobody to prompt
		// as well, and one of the two assigns the empty string — which is
		// why "assigns" and "what" are separate answers here. See
		// PromptStyle.AssignsWithNobodyToPrompt.
		ps1, ps2, say = st.DefaultWithNobodyToPrompt, st.DefaultContinuedWithNobodyToPrompt,
			st.AssignsWithNobodyToPrompt
	}
	if !say {
		return
	}
	for _, p := range [...]struct{ name, value string }{{"PS1", ps1}, {"PS2", ps2}} {
		if st.Default == "" {
			// The dialect has not said anything about prompts at all, so
			// nothing is invented. A core without a dialect assigns none:
			// the substrate's own `$ ` and `> ` stay what they have always
			// been, a thing the drawer falls back to rather than a value it
			// wrote down.
			//
			// Asked of Default rather than of the value about to be written,
			// because an empty value is a real answer in the dialect that
			// gives it — zsh's non-interactive PS1 is set and empty, and a
			// guard reading `${PS1+set}` there sees a difference.
			continue
		}
		if _, ok := r.GetVar(p.name); ok {
			continue
		}
		r.SetVar(p.name, p.value)
	}
}
