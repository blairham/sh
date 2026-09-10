// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The nested split a meta-plugin annex reads its `skip”` ice with, end to
// end as this dialect.
//
// `~/.zi/plugins/z-shell---z-a-meta-plugins/functions/_z_a_meta_plugins_before_load_handler`
// takes the names to leave out of a meta-plugin from one ice value, which an
// rc writes as `skip'forgit tig'`, and reads it with three splits stacked on
// each other:
//
//	on_demand_cand=( ${(@)${(@ps:\t:)${(@s: :)${(@s.;.)ICE[skip]}}}} )
//
// so a semicolon, a blank and a tab all separate names. Every one of the
// outer three is a split flag over a list the one inside it made, and a list
// is joined on IFS before a separator split *unless* the group carries `@` —
// which all three do. Without that exemption the two inner splits are undone
// by the joins above them, the whole value comes back as the single
// candidate `forgit tig`, no plugin matches it, and the annex loads the two
// members the rc asked it to leave out (#1683). Measured 2026-09-10 on
// zsh 5.9.2.
//
// The `banner` line is the same fault's other half, closed as a duplicate
// (#1681): what the annex has not skipped it then looks for on disk, the two
// it should have skipped are the two with no clone there, and one missing
// name is what turns its loading banner on. So the banner is not a second
// bug to fix but a reading of this one, and it goes out with it — the row
// where a name really is missing keeps saying the banner still comes on when
// it should.
//
// Written out rather than reduced to the flags because the reduction is what
// hid this: `${(@s: :)v}` on a scalar is right in both readings, and only a
// split standing on another split's result tells them apart.
func TestTheSkipIceIsReadThroughStackedSplits(t *testing.T) {
	const src = `
handler() {
  local -a plugin_array on_demand_cand on_demand_skip keep not_existing
  plugin_array=( paulirish/git-open tj/git-extras wfxr/forgit
                 voronkovich/gitignore.plugin.zsh jonas/tig )
  # The two the rc leaves out are the two with no clone under the plugin
  # directory, which is the state the annex met when this was reported.
  local -a on_disk
  on_disk=( paulirish/git-open tj/git-extras voronkovich/gitignore.plugin.zsh )
  on_demand_cand=( ${(@)${(@ps:\t:)${(@s: :)${(@s.;.)ICE[skip]}}}} )
  local p
  for p ( $on_demand_cand ) {
    integer idx1 count=1
    local pattern="(*/$p*|$p*|*$p|*-$p*|*$p-*)"
    while (( (idx1 = $plugin_array[(in:count:)$pattern]) != ${#plugin_array} + 1 )) {
      count+=1
      on_demand_skip+=( $plugin_array[idx1] )
    }
  }
  keep=( ${plugin_array:|on_demand_skip} )
  integer ibshow_messages=0
  for p ( $keep ) {
    if (( ! $on_disk[(I)$p] )) { ibshow_messages=1; not_existing+=( $p ) }
  }
  print -r -- "cand=${#on_demand_cand} skip=${(j:,:)on_demand_skip}"
  print -r -- "keep=${(j:,:)keep}"
  print -r -- "banner=$ibshow_messages missing=${(j:,:)not_existing}"
}
typeset -A ICE
`
	for _, tc := range []struct{ name, ice, want string }{
		{
			"the ice an rc writes",
			`ICE=( skip 'forgit tig' )`,
			"cand=2 skip=wfxr/forgit,jonas/tig\n" +
				"keep=paulirish/git-open,tj/git-extras,voronkovich/gitignore.plugin.zsh\n" +
				"banner=0 missing=\n",
		},
		{
			// The other two separators the same expression carries, so a
			// fix that reached the blank alone would still be caught.
			"a semicolon separates them too",
			`ICE=( skip 'forgit;tig' )`,
			"cand=2 skip=wfxr/forgit,jonas/tig\n" +
				"keep=paulirish/git-open,tj/git-extras,voronkovich/gitignore.plugin.zsh\n" +
				"banner=0 missing=\n",
		},
		{
			"and a tab",
			`ICE=( skip $'forgit\ttig' )`,
			"cand=2 skip=wfxr/forgit,jonas/tig\n" +
				"keep=paulirish/git-open,tj/git-extras,voronkovich/gitignore.plugin.zsh\n" +
				"banner=0 missing=\n",
		},
		{
			// One name is the shape that cannot tell the two readings
			// apart, kept so the row that can is not the only evidence the
			// expression works at all.
			"one name",
			`ICE=( skip 'tig' )`,
			"cand=1 skip=jonas/tig\n" +
				"keep=paulirish/git-open,tj/git-extras,wfxr/forgit,voronkovich/gitignore.plugin.zsh\n" +
				"banner=1 missing=wfxr/forgit\n",
		},
		{
			// The ice is registered empty — `skip''` — so "absent" and
			// "present and empty" are both live, and neither may skip
			// anything.
			"the ice present and empty",
			`ICE=( skip '' )`,
			"cand=0 skip=\n" +
				"keep=paulirish/git-open,tj/git-extras,wfxr/forgit,voronkovich/gitignore.plugin.zsh,jonas/tig\n" +
				"banner=1 missing=wfxr/forgit,jonas/tig\n",
		},
		{
			"the ice absent",
			`ICE=( )`,
			"cand=0 skip=\n" +
				"keep=paulirish/git-open,tj/git-extras,wfxr/forgit,voronkovich/gitignore.plugin.zsh,jonas/tig\n" +
				"banner=1 missing=wfxr/forgit,jonas/tig\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, src+tc.ice+"\nhandler")
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.ice, out, st, tc.want)
			}
		})
	}
}
