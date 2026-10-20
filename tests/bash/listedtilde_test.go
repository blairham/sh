// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A `~` in a listed value or a listed key is quoted only where a tilde
// expansion could begin — as the first byte — and nowhere else. Measured
// 2026-09-17 on bash 5.3.20 and on the 3.2.57 macOS ships, which agree,
// under `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=/orig`, over a bare `set`
// and a `declare -p` of a keyed table:
//
//	value/key   bash              zsh 5.9.2   ksh93u+
//	a~b         a~b     [a~b]     'a~b'       'a~b'
//	b~          b~      [b~]      'b~'        'b~'
//	a:x~b       a:x~b   [a:x~b]   'a:x~b'     'a:x~b'
//	~b          '~b'    ["~b"]    '~b'        '~b'
//	a:~b        'a:~b'  ["a:~b"]  'a:~b'      'a:~b'
//	a=~b        'a=~b'  ["a=~b"]  'a=~b'      'a=~b'
//
// The other three panel shells quote every one, so this is bash's alone —
// Semantics.ListedTildeIsBareWhereItCannotExpand. The quoted rows are the
// three offsets a tilde expansion could start at: the front, and — because
// an assignment's value is a tilde context after each — straight after a `:`
// or an `=`. The `#` rule composes with it: `a#~b` and `a~b#c` are bare and
// `~a#b` is quoted (#2298).

func TestAListedTildeIsQuotedOnlyAtTheFront(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a value with a tilde inside it", `v='a~b'; set | grep '^v='`, "v=a~b\n"},
		{"a value ending in a tilde", `v='b~'; set | grep '^v='`, "v=b~\n"},
		{"a value a tilde opens", `v='~b'; set | grep '^v='`, "v='~b'\n"},
		{"a tilde alone", `v='~'; set | grep '^v='`, "v='~'\n"},
		{"a tilde after a colon", `v='a:~b'; set | grep '^v='`, "v='a:~b'\n"},
		{"a tilde after an equals", `v='a=~b'; set | grep '^v='`, "v='a=~b'\n"},
		{"a tilde one past a colon", `v='a:x~b'; set | grep '^v='`, "v=a:x~b\n"},
		{"a tilde after a comma", `v='a,~b'; set | grep '^v='`, "v=a,~b\n"},
		{
			"a key with a tilde after a colon",
			`k='a:~b'; declare -A m; m[$k]=1; declare -p m`,
			"declare -A m=([\"a:~b\"]=\"1\" )\n",
		},
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

// The leading unquoted `~` of an associative subscript is expanded before the
// text becomes a key, on a store and on a read alike, and through every route
// that reaches an element by its *operand*. Measured 2026-09-17 on bash
// 5.3.20 under `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=/orig`, from a script
// file with stdin closed; ksh93 agrees and zsh takes the characters —
// Semantics.SubscriptKeyExpandsALeadingTilde.
//
// `HOME` here is the one the shell started with, which is deliberate: bash
// 5.3.20 caches it and a `HOME=` in the script does not move `~` (#3484), so
// a row that set HOME itself would be measuring that instead.
func TestASubscriptsLeadingTildeNamesTheHomeDirectory(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a store", `declare -A m; m[~/k]=v; declare -p m`, "declare -A m=([%H/k]=\"v\" )\n"},
		{"a tilde alone", `declare -A m; m[~]=v; declare -p m`, "declare -A m=([%H]=\"v\" )\n"},
		{
			"a read finds what the store made",
			`declare -A m; m[~/k]=v; echo "[${m[$HOME/k]}]"`,
			"[v]\n",
		},
		{
			"a read by the tilde finds what a path stored",
			`declare -A m; m[$HOME/k]=v; echo "[${m[~/k]}]"`,
			"[v]\n",
		},
		{"an array literal", `declare -A m=([~/k]=v); declare -p m`, "declare -A m=([%H/k]=\"v\" )\n"},
		{
			"unset by the tilde",
			`declare -A m; m[$HOME/k]=v; unset 'm[~/k]'; declare -p m`,
			"declare -A m=()\n",
		},
		{
			"test -v by the tilde",
			`declare -A m; m[$HOME/k]=v; test -v 'm[~/k]' && echo yes || echo no`,
			"yes\n",
		},
		{
			"printf -v by the tilde",
			`declare -A m; printf -v 'm[~/k]' '%s' P; declare -p m`,
			"declare -A m=([%H/k]=\"P\" )\n",
		},
		{
			"an indirect read by the tilde",
			`declare -A m; m[$HOME/k]=v; r='m[~/k]'; echo "[${!r}]"`,
			"[v]\n",
		},
		{
			"read by the tilde",
			`declare -A m; read 'm[~/k]' <<< "RD"; declare -p m`,
			"declare -A m=([%H/k]=\"RD\" )\n",
		},
		// The guard, and the one route that must not take it: an indexed
		// name's subscript is an expression, and bash names the tilde in the
		// complaint rather than the path it would have expanded to.
		{
			"an indexed subscript is left as written",
			`q=(1 2 3); unset 'q[~/k]'`,
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := runEmptyArray(t, tc.src)
			if tc.want == "" {
				if !strings.Contains(errs, "~/k") {
					t.Errorf("%s: the complaint should name the subscript as written, got %q", tc.src, errs)
				}
				return
			}
			want := strings.ReplaceAll(tc.want, "%H", homeOfTheRun(t))
			if out != want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.src, out, errs, st, want)
			}
		})
	}
}

// homeOfTheRun is the `$HOME` the rows above expand a tilde to, asked of the
// shell rather than of the test's environment: the harness decides what the
// run's home is and a constant here would only agree with it by luck.
func homeOfTheRun(t *testing.T) string {
	t.Helper()
	out, errs, st := runEmptyArray(t, `printf '%s' "$HOME"`)
	if errs != "" || st != 0 || out == "" {
		t.Fatalf(`$HOME came back %q (stderr %q, status %d)`, out, errs, st)
	}
	return out
}
