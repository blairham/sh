// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `compdescribe -g`, the grid of names and descriptions the completion system
// asks for so that options sharing a description share a row (#6178), and
// the colon rule for an empty description (#6161).
//
// Every row was measured on zsh 5.9.2, 2026-10-05, from inside a `zle -C`
// widget with `$COLUMNS` at 120 and `E=(-J ej -X ex)`, reading `-g` back
// until it answered 1; compdescribepack.go has the rules. Each answer is
// written `listing(args)words/displays`, a description padded to its column
// written with its width after a `#`.

// cells runs one definition call and writes out every answer.
func cells(t *testing.T, setup, call string) string {
	t.Helper()
	return reported(t, `COLUMNS=120
		setopt localoptions extendedglob
		local -a E=(-J ej -X ex) G H G2 args tm td; local csl out= x
		`+setup+`
		compdescribe `+call+` || { say "def failed"; return }
		while compdescribe -g csl args tm td; do
			local -a shown=()
			for x in "${td[@]}"; do
				if (( ${#x} > 20 )); then shown+=("${x%% #}#${#x}"); else shown+=("$x"); fi
			done
			out+="$csl(${(j: :)args})${(j:,:)tm}/${(j:,:)shown} "
		done
		say "$out"`, "x ")
}

func TestCompdescribeGroupsNamesThatShareADescription(t *testing.T) {
	for _, c := range []struct{ name, setup, call, want string }{
		{
			"three names on one row, one on another",
			"G=(-a:same -b:same -c:same -d:other)", "-I '' 60 '-- ' E -g G",
			"packed(-2V ej -X ex)-c/-c packed(-2V ej -X ex)-d/-d packed(-2V ej -X ex)-b/-b " +
				"packed(-E1 -J ej -X ex)/ packed(-2V ej -X ex)-a/-a packed(-E1 -J ej -X ex)/ " +
				"packed(-E2 -J ej -X ex)/-- same#106,-- other#106 ",
		},
		{
			// Shortest first and then reversed, ties in definition order.
			"the order within a row", "G=(-a:s --bb:s -c:s --dd:s)", "-I '' 60 '-- ' E -g G",
			"packed(-2V ej -X ex)--dd/--dd packed(-2V ej -X ex)--bb/--bb " +
				"packed(-2V ej -X ex)-c/-c packed(-2V ej -X ex)-a/-a packed(-E1 -J ej -X ex)/-- s#98 ",
		},
		{
			// Each name carries its own definition's options; the fillers the
			// options of the definition owning the last row's first name; the
			// undescribed names come last, unpacked, with no heading.
			"two definitions",
			"G=(-a:s -b:t -z); H=(--aa:s -c:u -y)", "-I '' 60 '-- ' E -g G -Q -- H -S=",
			"packed(-S= -2V ej -X ex)--aa/--aa packed(-Q -2V ej -X ex)-b/-b packed(-S= -2V ej -X ex)-c/-c " +
				"packed(-Q -2V ej -X ex)-a/-a packed(-E2 -S= -J ej -X ex)/ " +
				"packed(-E3 -S= -J ej -X ex)/-- s#108,-- t#108,-- u#108 (-Q -J ej)-z/-z (-S= -J ej)-y/-y ",
		},
		{
			// A row wider than the width wraps, its description on its last
			// line: two nine-character names a line at 30.
			"a row that wraps",
			"G=(--aaaaaaa:s --bbbbbbb:s --ccccccc:s --ddddddd:s --eeeeeee:s --fffffff:s --ggggggg:s -x:t)",
			"-I '' 30 '-- ' E -g G",
			"packed(-2V ej -X ex)--bbbbbbb/--bbbbbbb packed(-2V ej -X ex)--ddddddd/--ddddddd " +
				"packed(-2V ej -X ex)--fffffff/--fffffff packed(-2V ej -X ex)--ggggggg/--ggggggg " +
				"packed(-2V ej -X ex)-x/-x packed(-2V ej -X ex)--aaaaaaa/--aaaaaaa " +
				"packed(-2V ej -X ex)--ccccccc/--ccccccc packed(-2V ej -X ex)--eeeeeee/--eeeeeee " +
				"packed(-E2 -J ej -X ex)/ packed(-E5 -J ej -X ex)/,,,-- s#96,-- t#96 ",
		},
		{
			// One name a line saves no row, so the answer is the ordinary one.
			"packing that saves nothing",
			"G=(--aaaaaaa:s --bbbbbbb:s -x:t)", "-I '' 12 '-- ' E -g G",
			"(-l -J ej -X ex)--aaaaaaa,--bbbbbbb,-x/--aaaaaaa  -- s,--bbbbbbb  -- s,-x         -- t ",
		},
		{
			"every description its own", "G=(-a:x -b:y)", "-I '' 60 '-- ' E -g G",
			"(-l -J ej -X ex)-a,-b/-a  -- x,-b  -- y ",
		},
		{
			"a matches array", "G=(-a:s -b:s -c:t); G2=(AA BB CC)", "-I '' 60 '-- ' E -g G G2 -Q",
			"packed(-Q -2V ej -X ex)BB/-b packed(-Q -2V ej -X ex)CC/-c packed(-Q -2V ej -X ex)AA/-a " +
				"packed(-E1 -Q -J ej -X ex)/ packed(-E2 -Q -J ej -X ex)/-- s#110,-- t#110 ",
		},
		// #6161: the colon decides whether a name is described, not what
		// follows it.
		{
			"an empty description is a description", "G=(alpha:one beta: gam)", "-I '' 40 '-- ' E G -Q",
			"(-l -Q -J ej -X ex)alpha,beta/alpha  -- one,beta   --  (-Q -J ej -X ex)gam/gam ",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := cells(t, c.setup, c.call); got != c.want {
				t.Errorf("compdescribe %s with %s:\n got %q\nwant %q", c.call, c.setup, got, c.want)
			}
		})
	}
}

// TestCompdescribeCutsADescriptionToItsColumn — a description longer than
// the screen leaves it is cut there, not wrapped.
func TestCompdescribeCutsADescriptionToItsColumn(t *testing.T) {
	long := strings.Repeat("x", 150)
	got := cells(t, "G=(-a:s -b:s -c:"+long+")", "-I '' 60 '-- ' E -g G")
	if want := "-- " + strings.Repeat("x", 107) + "#110 "; !strings.HasSuffix(got, ","+want) {
		t.Errorf("got %q, want it to end %q", got, ","+want)
	}
}
