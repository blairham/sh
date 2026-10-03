// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// Every element on the left assigned to `region_highlight` inside a widget,
// and what zsh 5.9.2 read back on the right — measured 2026-10-02 through a
// pseudo-terminal with `${(V)e}`. See regionelement.go for the five rules
// the rows come to.
func TestARegionElementReadsBackAsZshSpellsIt(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"0 4 fg=green memo=someplugin,futureattribute=futurevalue", "0 4 fg=green memo=someplugin"},
		{"0 4 fg=green,bold", "0 4 fg=green,bold"},
		{"0 4 bold,fg=green", "0 4 fg=green,bold"},
		{"P0 4 bold", "P0 4 bold"},
		{"P 0 4 bold", "P0 4 bold"},
		{"4 0 bold", "4 0 bold"},
		{"0 99 bold", "0 99 bold"},
		{"x y bold", "-1 -1 none"},
		{"0 4 none", "0 4 none"},
		{"0 4 fg=#ff0000", "0 4 fg=#ff0000"},
		{"0 4 fg=196", "0 4 fg=196"},
		{"0 4 fg=default", "0 4 none"},
		{"0 4 memo=a", "0 4 none memo=a"},
		{"0 4 fg=red  memo=a b", "0 4 fg=red memo=a"},
		{"0 4 fg=red,memo=x", "0 4 fg=red memo=x"},
		{"0 4 standout,underline", "0 4 standout,underline"},
		{"1 2", "1 2 none"},
		{"0 4 fg=red extra", "0 4 fg=red"},
		{"0 4 bg=blue,fg=1", "0 4 fg=red,bg=blue"},
		{"0 4 fg=nosuch", "0 4 none"},
		{"-1 4 bold", "-1 4 bold"},
		{"0 4 underline,bold,standout,fg=red,bg=green", "0 4 fg=red,bg=green,bold,standout,underline"},
		{"0 4 fg=#F00", "0 4 fg=#ff0000"},
		{"0 4 fg=#ABCDEF", "0 4 fg=#abcdef"},
		{"0 4 bg=red", "0 4 bg=red"},
		{"0 4 none,bold", "0 4 bold"},
		{"0 4 bold,none", "0 4 none"},
		{"0 4 bold memo=", "0 4 bold memo="},
		{"0 4 bold memo=a=b", "0 4 bold memo=a=b"},
		{" 0 4 bold", "0 4 bold"},
		{"1x 2 bold", "1 -1 none"},
		{"0x 4 bold", "0 -1 none"},
		{"0 4x bold", "0 4 none"},
		{"0 4 bold memo=a\x00b", "0 4 bold memo=a"},
		{"0 4 fg=8", "0 4 fg=8"},
		{"0 4 fg=15", "0 4 fg=15"},
		{"0 4 fg=Red", "0 4 none"},
		{"0 4 fg=bl", "0 4 fg=black"},
		{"0 4 bold,bold", "0 4 bold"},
		{"0 4 fg=red,fg=blue", "0 4 fg=magenta"},
		{"0 4 reverse", "0 4 none"},
		{"0 4 fg=red memo=x memo=y", "0 4 fg=red memo=x"},
		{"0 4 memo=x fg=red", "0 4 none memo=x"},
		{"P1 2 bold memo=z", "P1 2 bold memo=z"},
		{"0 4 fg=red,fg=196", "0 4 fg=197"},
		{"0 4 fg=#ff0000,fg=#00ff00", "0 4 fg=#ffff00"},
		{"0 4 fg=196,fg=#00ff00", "0 4 fg=#00ffc4"},
		{"0 4 fg=#00ff00,fg=1", "0 4 fg=#00ff01"},
		{"0 4 fg=red,fg=default", "0 4 fg=red"},
		{"0 4 bg=1,bg=2", "0 4 bg=yellow"},
		{"0 4 fg=1,none,fg=2", "0 4 fg=yellow"},
		{"0 4 bold,fg=none", "0 4 bold"},
		{"0 4 fg=300", "0 4 none"},
		{"0 4 fg=-1", "0 4 none"},
		{"0 4 fg=#12", "0 4 none"},
		{"0 4 +2 bold", "0 4 none"},
		{"0 4 fg=re", "0 4 fg=red"},
		{"0 4 fg=", "0 4 fg=black"},
		{"0 4 fg=red,none", "0 4 none"},
		{"0 4 fg=red,bold,none", "0 4 none"},
		{"0 4 bold,none,fg=red", "0 4 fg=red"},
		{"0 4 underline,none,standout", "0 4 standout"},
		{"0 4 fg=#ff0000,none", "0 4 none"},
	} {
		if got := parseRegionText(c.in, false).String(); got != c.want {
			t.Errorf("%q reads back %q, want %q", c.in, got, c.want)
		}
	}
}
