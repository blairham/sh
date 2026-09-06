// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import "testing"

// What SH_BLOCKS_OUTPUT says, and it says three things rather than two.
//
// The third is the default and it is the point of #720: keeping output used to
// cost a child its terminal, so it was off unless asked; a front end that can
// put a pseudo-terminal behind the capture pays nothing, and one that cannot
// still pays the whole price. The variable says which of those a session will
// accept and the front end says which it is.
func TestWhatTheOutputVariableSays(t *testing.T) {
	for _, tc := range []struct {
		name string
		vars map[string]string
		want CaptureMode
	}{
		{
			name: "unset is the default, which is where a terminal allows it",
			vars: map[string]string{},
			want: CaptureIfTerminal,
		},
		{
			// The one way to say "not this session", and it has to exist now
			// that the default is on. The same gesture an empty HISTFILE is.
			name: "empty is off outright",
			vars: map[string]string{OutputVar: ""},
			want: CaptureOff,
		},
		{
			name: "a value is on whatever it costs",
			vars: map[string]string{OutputVar: "1"},
			want: CaptureAlways,
		},
		{
			// Measured against nothing, because it is this shell's own
			// variable: the rule is "empty is off", so a word is a value. It
			// is a trap worth pinning rather than leaving to be discovered —
			// somebody will write `=0` and mean off.
			name: "a word that looks false is still a value",
			vars: map[string]string{OutputVar: "0"},
			want: CaptureAlways,
		},
	} {
		got, _ := CaptureFrom(lookup(tc.vars))
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The bound is read alongside, and a value that is not a number leaves the
// default rather than zero — which would turn the output half back off through
// a typo, the same rule HISTFILESIZE follows.
func TestWhatTheOutputBoundSays(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
	}{
		{value: "4096", want: 4096},
		{value: " 4096 ", want: 4096},
		{value: "", want: DefaultMaxOutput},
		{value: "lots", want: DefaultMaxOutput},
		{value: "0", want: DefaultMaxOutput},
		{value: "-1", want: DefaultMaxOutput},
	} {
		_, got := CaptureFrom(lookup(map[string]string{
			OutputVar: "1", MaxOutputVar: tc.value,
		}))
		if got != tc.want {
			t.Errorf("%s=%q gave %d, want %d", MaxOutputVar, tc.value, got, tc.want)
		}
	}
	// And a session that is keeping nothing is told nothing about a bound,
	// so a caller cannot build a capture out of a mode it was refused.
	if mode, max := CaptureFrom(lookup(map[string]string{OutputVar: ""})); mode != CaptureOff || max != 0 {
		t.Errorf("off gave mode %v max %d, want off and 0", mode, max)
	}
}

// lookup is the get function these take, over a plain map.
func lookup(vars map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := vars[name]
		return v, ok
	}
}
