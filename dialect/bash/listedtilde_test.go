// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// A `~` in a listed value or a listed key is quoted only where a tilde
// expansion could begin — as the first byte — and nowhere else. Measured
// 2026-09-17 on bash 5.3.20 and on the 3.2.57 macOS ships, which agree,
// under `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=/orig`, over a bare `set`
// and a `declare -p` of a keyed table:
//
//	value/key   bash            zsh 5.9.2      ksh93u+
//	a~b         a~b   [a~b]     'a~b'          'a~b'
//	b~          b~    [b~]      'b~'           'b~'
//	~b          '~b'  ["~b"]    '~b'           '~b'
//
// The other three panel shells quote every one, so this is bash's alone —
// Semantics.ListedTildeIsBareUnlessItOpens. The two position rules compose
// here as well: `a#~b` and `a~b#c` are bare and `~a#b` is quoted (#2298).

func TestAListedTildeIsQuotedOnlyAtTheFront(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a value with a tilde inside it", `v='a~b'; set | grep '^v='`, "v=a~b\n"},
		{"a value ending in a tilde", `v='b~'; set | grep '^v='`, "v=b~\n"},
		{"a value a tilde opens", `v='~b'; set | grep '^v='`, "v='~b'\n"},
		{"a tilde alone", `v='~'; set | grep '^v='`, "v='~'\n"},
		{
			"a key with a tilde inside it",
			`k='a~b'; declare -A m; m[$k]=1; declare -p m`,
			"declare -A m=([a~b]=\"1\" )\n",
		},
		{
			"a key a tilde opens",
			`k='~b'; declare -A m; m[$k]=1; declare -p m`,
			"declare -A m=([\"~b\"]=\"1\" )\n",
		},
		// The `#` rule is the same shape and was already answered; these
		// rows say the two compose rather than excluding each other.
		{"a hash then a tilde", `v='a#~b'; set | grep '^v='`, "v=a#~b\n"},
		{"a tilde then a hash", `v='a~b#c'; set | grep '^v='`, "v=a~b#c\n"},
		{"a value a tilde opens with a hash in it", `v='~a#b'; set | grep '^v='`, "v='~a#b'\n"},
		{
			"a key with both",
			`k='a#~b'; declare -A m; m[$k]=1; declare -p m`,
			"declare -A m=([a#~b]=\"1\" )\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := runEmptyArray(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// `${m[@]@A}` is the statement that would reproduce the name, so it spells a
// key exactly as `declare -p` does. Measured on bash 5.3.20 over the same key
// spellings: both write `["a b"]="v"`, and `@K` writes the key quoted while
// `@k` writes it as itself. Both listings printed the key raw here, so
// `${m[@]@A}` for a key of `"` produced `(["]="v" )` — a statement no shell
// reads back (#2298).
func TestTheKeyTransformsSpellAKeyAsTheListingDoes(t *testing.T) {
	for _, tc := range []struct{ name, key, want string }{
		{"a blank", "a b", `declare -A m=(["a b"]="v" ) ["a b" "v" ] [a b][v]`},
		{"a double quote", `"`, `declare -A m=(["\""]="v" ) ["\"" "v" ] ["][v]`},
		{"the whole-array subscript", "@", `declare -A m=(["@"]="v" ) ["@" "v" ] [@][v]`},
		{"a plain key", "plain", `declare -A m=([plain]="v" ) [plain "v" ] [plain][v]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `k=` + shellQuoted(tc.key) + `; declare -A m; m[$k]=v; ` +
				`echo "${m[@]@A}" "[${m[@]@K}]" "$(printf '[%s]' "${m[@]@k}")"`
			out, errs, st := runEmptyArray(t, src)
			if out != tc.want+"\n" || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", src, out, errs, st, tc.want+"\n")
			}
		})
	}
}

// shellQuoted wraps text in single quotes so a snippet can carry whatever is
// in it.
func shellQuoted(v string) string {
	out := "'"
	for _, c := range v {
		if c == '\'' {
			out += `'\''`
			continue
		}
		out += string(c)
	}
	return out + "'"
}
