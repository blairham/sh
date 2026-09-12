// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/blairham/sh/interp"
)

// The flip test cannot see an axis whose *type* is too narrow for the panel.
//
// #2029 is the case. CompoundBodyDecidesThePipelineStatusRecord was an Answer
// where the panel splits three ways, so one of its two values stood for a
// reading no shell has — and a flip between them fails something, because
// both values reach different code. The sweep calls that axis healthy and
// moves on, while half of it is a claim about a shell that does not exist.
//
// What does see it is asking the presets instead of the corpus, which costs
// no processes at all:
//
//   - an axis every dialect answers the same way records no disagreement
//     between them, which is the thing an axis is for; and
//   - a legal value no dialect holds is a reading nothing in the panel
//     exhibits.
//
// Neither is a verdict. Both are lists to re-measure, for the reason the
// unpinned list is: the shells have to be asked again before anything is
// concluded about what they do.
//
// And the verdict, once measured, goes on the axis — `unexhibited Value:
// who holds it` in the field's own doc comment, read back by FieldNotes.
// Triaging the first 25 (#2060) found no fiction at all: every entry was a
// reading reached by a run-time mode, a value the type shares with a sibling
// axis, a reading the panel has at an axis that is read rather than asked, or
// the null hypothesis of a one-sided question. So neither list shrinks, and
// the number that means anything is Untriaged.

// ValueUse is what the presets do with one axis.
type ValueUse struct {
	Field string `json:"field"`
	Type  string `json:"type"`
	// Held is the value each dialect preset gives this axis.
	Held map[string]string `json:"held"`
	// Unanimous is true when the four dialects all answer alike.
	Unanimous bool `json:"unanimous,omitempty"`
	// Unexhibited are the type's legal values that no dialect holds here,
	// other than the unspecified one, which means the absence of an answer
	// rather than an answer.
	Unexhibited []string `json:"unexhibited,omitempty"`
	// Why is why the axis is an axis even though the four answer alike,
	// from the field's own comment. Empty means nobody has said.
	Why string `json:"why,omitempty"`
	// Explained is, per unexhibited value, who does hold it — again from
	// the field's comment. A value missing from this map is one nobody has
	// re-measured since the sweep first named it.
	Explained map[string]string `json:"explained,omitempty"`
	// Nowhere are the ones no dialect holds for *any* axis of that type.
	// The distinction matters: a reading another axis of the same type
	// exhibits is a reading some shell has, just not here, and that is a
	// much weaker signal than a constant nothing in the panel has ever been
	// measured doing.
	Nowhere []string `json:"nowhere,omitempty"`
}

// dialectPresets are the four vectors that claim to be a shell somebody runs.
//
// core and posix are deliberately absent. Neither imitates a member of the
// panel — the core refuses where the panel disagrees and posix answers to the
// standard — so a value only they hold is not a shell exhibiting it.
func dialectPresets() map[string]interp.Semantics {
	out := map[string]interp.Semantics{}
	for _, t := range Targets() {
		out[t.Dialect] = t.Semantics
	}
	return out
}

// Values reports what the presets do with every axis.
func PresetUse() ([]ValueUse, error) {
	fields, err := Fields(reflect.TypeOf(interp.Semantics{}))
	if err != nil {
		return nil, err
	}
	presets := dialectPresets()
	names := sortedKeys(presets)
	// What each type's constants are held by *somewhere*, before asking
	// about any one axis.
	anywhere := map[string]map[string]bool{}
	for _, f := range fields {
		for _, name := range names {
			cur, err := At(reflect.ValueOf(presets[name]), f.Path)
			if err != nil {
				return nil, err
			}
			if anywhere[f.Type] == nil {
				anywhere[f.Type] = map[string]bool{}
			}
			anywhere[f.Type][literalOf(cur)] = true
		}
	}
	notes, err := FieldNotes()
	if err != nil {
		return nil, err
	}
	out := make([]ValueUse, 0, len(fields))
	for _, f := range fields {
		use := ValueUse{Field: f.Path, Type: f.Type, Held: map[string]string{}, Why: notes[f.Path].Unanimous}
		held := map[string]bool{}
		for _, name := range names {
			cur, err := At(reflect.ValueOf(presets[name]), f.Path)
			if err != nil {
				return nil, err
			}
			use.Held[name] = heldName(f, cur)
			held[literalOf(cur)] = true
		}
		use.Unanimous = len(held) == 1
		consts, err := typeConstants()
		if err != nil {
			return nil, err
		}
		for _, v := range consts[f.Type] {
			if held[v.Literal] || v.Unspecified {
				continue
			}
			use.Unexhibited = append(use.Unexhibited, v.Name)
			if why, ok := notes[f.Path].Value[v.Name]; ok {
				if use.Explained == nil {
					use.Explained = map[string]string{}
				}
				use.Explained[v.Name] = why
			}
			if !anywhere[f.Type][v.Literal] {
				use.Nowhere = append(use.Nowhere, v.Name)
			}
		}
		out = append(out, use)
	}
	return out, nil
}

// Untriaged are the entries of the two preset lists that no field comment
// has answered yet.
//
// This is the number the instrument is for after #2060. The lists themselves
// do not shrink — a shared enumeration will always have values one axis of it
// does not hold, and a run-time option will always be a reading no preset
// carries — so counting entries measures the shape of the struct rather than
// the state of the work. Counting the ones nobody has re-measured measures
// the work.
func Untriaged(uses []ValueUse) []string {
	var out []string
	for _, u := range uses {
		if u.Unanimous && u.Why == "" {
			out = append(out, u.Field+": answered alike by all four and nothing says why it is an axis")
		}
		for _, v := range u.Unexhibited {
			if _, ok := u.Explained[v]; !ok {
				out = append(out, u.Field+": nothing says who holds "+v)
			}
		}
	}
	return out
}

// PresetReport renders the two lists.
func PresetReport(uses []ValueUse) string {
	var b strings.Builder
	var unanimous, fictional []ValueUse
	for _, u := range uses {
		if u.Unanimous {
			unanimous = append(unanimous, u)
		}
		if len(u.Unexhibited) > 0 {
			fictional = append(fictional, u)
		}
	}
	fmt.Fprintf(&b, "\naxes every dialect answers the same way (%d):\n", len(unanimous))
	b.WriteString("  an axis exists to record a disagreement; these record none between\n" +
		"  the four dialects. Re-measure before concluding — the panel is six\n" +
		"  columns and two of them (bash 3.2, bash as sh) have no dialect here.\n")
	for _, u := range unanimous {
		fmt.Fprintf(&b, "  %-52s all four: %s\n", u.Field, u.Held["bash"])
		b.WriteString(wrapNote(u.Why))
	}
	fmt.Fprintf(&b, "\naxes with a legal value no dialect holds (%d):\n", len(fictional))
	b.WriteString("  a value nothing exhibits is a reading that may belong to no shell —\n" +
		"  the #2029 shape, which the flip test structurally cannot see. A value\n" +
		"  marked * is held by no axis of that type in any dialect, which is the\n" +
		"  strong form; the rest are readings some other axis does exhibit.\n" +
		"  A preset is not the whole of a dialect: an answer reached only by a\n" +
		"  run-time option — zsh's localtraps swaps FunctionLocalTraps — is\n" +
		"  exhibited without any preset holding it. Read the axis before acting.\n")
	for _, u := range fictional {
		nowhere := map[string]bool{}
		for _, n := range u.Nowhere {
			nowhere[n] = true
		}
		marked := make([]string, 0, len(u.Unexhibited))
		for _, v := range u.Unexhibited {
			if nowhere[v] {
				v += "*"
			}
			marked = append(marked, v)
		}
		fmt.Fprintf(&b, "  %-52s %-30s unexhibited: %s\n", u.Field, u.Type, strings.Join(marked, ", "))
		for _, v := range u.Unexhibited {
			if why, ok := u.Explained[v]; ok {
				b.WriteString(wrapNote(v + " — " + why))
			}
		}
	}
	untriaged := Untriaged(uses)
	fmt.Fprintf(&b, "\nuntriaged (%d):\n", len(untriaged))
	b.WriteString("  every entry above is one of four things, and only re-measuring the\n" +
		"  panel tells them apart: a reading reached by a run-time mode rather\n" +
		"  than a preset, a value the type shares with a sibling axis that does\n" +
		"  hold it, a reading the panel has that the axis is *read* rather than\n" +
		"  asked for, or a fiction that should come out. The verdict goes in the\n" +
		"  field's own comment — `unexhibited NAME: who holds it` — so the next\n" +
		"  sweep reports it rather than re-opening it. These have no verdict:\n")
	for _, line := range untriaged {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	return b.String()
}

// wrapNote prints a triage verdict under the entry it answers, folded to
// something a terminal can read.
func wrapNote(s string) string {
	if s == "" {
		return ""
	}
	const width = 68
	var b strings.Builder
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && len(line)+1+len(word) > width {
			fmt.Fprintf(&b, "      %s\n", line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		fmt.Fprintf(&b, "      %s\n", line)
	}
	return b.String()
}
