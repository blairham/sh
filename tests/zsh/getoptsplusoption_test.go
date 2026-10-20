// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A word beginning with `+` is an option word here, and this shell is alone
// in it (#2946).
//
// interp proves what the axis does; this file pins that this preset answers
// it yes and that the sign reaches the wording. Measured 2026-09-17 on 5.9.2,
// as a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin on
// /dev/null.
func zshGetoptsRun(t *testing.T, src string) string {
	t.Helper()
	out, _, err := preset.Combined(t, dialecttest.Base{
		Name: "zsh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out
}

// The scan, over the shapes the sign reaches.
func TestGetoptsTakesAPlusPrefixedOption(t *testing.T) {
	for _, tc := range []struct{ name, params, spec, want string }{
		{"a lone plus option", "set -- +a rest", "a", "[+a] ind=2\n"},
		{"a cluster", "set -- +ab", "ab", "[+a][+b] ind=2\n"},
		{"both signs in one line", "set -- +a -b", "ab", "[+a][b] ind=3\n"},
		{"an argument in the next word", "set -- +a v", "a:", "[+a=v] ind=3\n"},
		{"an argument in the same word", "set -- +av", "a:", "[+a=v] ind=2\n"},
		// Not an end-of-options word: `++` is the option `+`, which the
		// string does not have.
		{"a double plus", "set -- ++ a", "a", "[?] ind=2\n"},
		// A lone `+` is an operand, as a lone `-` is everywhere.
		{"a lone plus", "set -- + a", "a", " ind=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.params + "\nOPTIND=1\n" +
				"while getopts '" + tc.spec + "' o 2>/dev/null; do\n" +
				"  if [ -z \"${OPTARG-}\" ]; then printf '[%s]' \"$o\"; else printf '[%s=%s]' \"$o\" \"$OPTARG\"; fi\n" +
				"done\nprintf ' ind=%s\\n' \"$OPTIND\"\n"
			if out := zshGetoptsRun(t, src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The sign is in the complaint and in OPTARG as well as in the name, and the
// sign and the letter are two verbs rather than one string — which only a
// word whose letter *is* a sign can show.
func TestGetoptsPlusPrefixedOptionWording(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an unknown letter", "OPTIND=1\ngetopts 'a' o +z\n", "zsh:2: bad option: +z\n"},
		{"a missing argument", "OPTIND=1\ngetopts 'a:' o +a\n", "zsh:2: argument expected after +a option\n"},
		{"the other sign", "OPTIND=1\ngetopts 'a' o -z\n", "zsh:2: bad option: -z\n"},
		{"a dash word naming a plus", "OPTIND=1\ngetopts 'a' o -+\n", "zsh:2: bad option: -+\n"},
		{"a plus word naming a plus", "OPTIND=1\ngetopts 'a' o ++\n", "zsh:2: bad option: ++\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out := zshGetoptsRun(t, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
	// The silent form reports the same signed letter through OPTARG.
	for _, tc := range []struct{ name, src, want string }{
		{"an unknown letter", "OPTIND=1\ngetopts ':a' o +z\nprintf '[%s][%s]\\n' \"$o\" \"$OPTARG\"\n", "[?][+z]\n"},
		{"a missing argument", "OPTIND=1\ngetopts ':a:' o +a\nprintf '[%s][%s]\\n' \"$o\" \"$OPTARG\"\n", "[:][+a]\n"},
	} {
		t.Run("silently, "+tc.name, func(t *testing.T) {
			if out := zshGetoptsRun(t, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
