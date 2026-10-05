// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// `region_highlight`'s offsets follow the line when the editor edits it, and
// stay put when a widget assigns to it.
//
// Before #5874 nothing moved them, so a highlight set before an edit in front
// of it landed off the text it was set for. Measured 2026-10-04 against zsh
// 5.9.2 under `zsh -i` on a pseudo-terminal with this startup file, pressing
// each key and then ^U:
//
//	^Xa  RHA=[0 2 bold|3 5 underline|5 6 standout memo=m]   delete-char at 0
//	^Xb  RHB=[1 3 bold|3 3 underline|3 3 standout memo=m]   kill-line from 3
//	^Xc  RHC=[1 3 bold|4 6 underline|6 7 standout memo=m]   LBUFFER+=X at 0
//	^Xd  then Z, then ^Xs    RHS=[2 4 bold|5 7 underline]   a typed key
//
// The last row is the editor's own key, with no widget around it, and is
// the half a widget-only fix would miss.
func TestRegionHighlightFollowsTheEditorsEdits(t *testing.T) {
	control, screen := widgetSession(t, `rh() { print -r -- "$1=[${(j:|:)region_highlight}]" }
setup() { BUFFER=abcdefgh; CURSOR=$1; region_highlight=('1 3 bold' '4 6 underline' '6 7 standout memo=m') }
ta() { setup 0; zle .delete-char; rh RHA }
tb() { setup 3; zle .kill-line; rh RHB }
tc() { setup 0; LBUFFER+=X; rh RHC }
td() { BUFFER=abcdefgh; CURSOR=0; region_highlight=('1 3 bold' '4 6 underline') }
ts() { rh RHS }
for f in ta tb tc td ts; do zle -N $f; done
bindkey '^Xa' ta; bindkey '^Xb' tb; bindkey '^Xc' tc; bindkey '^Xd' td; bindkey '^Xs' ts
`)
	for _, c := range []struct{ keys, want string }{
		{"\x18a\x15", "RHA=[0 2 bold|3 5 underline|5 6 standout memo=m]"},
		{"\x18b\x15", "RHB=[1 3 bold|3 3 underline|3 3 standout memo=m]"},
		{"\x18c\x15", "RHC=[1 3 bold|4 6 underline|6 7 standout memo=m]"},
		{"\x18dZ\x18s\x15", "RHS=[2 4 bold|5 7 underline]"},
	} {
		if _, err := control.WriteString(c.keys); err != nil {
			t.Fatalf("typing %q: %v", c.keys, err)
		}
		// Every offset is the shell's, so the typed keys cannot satisfy this.
		if err := screen.Await(c.want, widgetBudget); err != nil {
			t.Fatalf("after %q the offsets are not\n%s\n%v\n%q", c.keys, c.want, err,
				smoke.LastLines(screen.Text(), 6))
		}
	}
}
