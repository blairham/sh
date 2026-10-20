// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// Three things a function body listing spells differently from the source
// (#3196), measured 2026-09-16 on bash 5.3.20 through `declare -f`.
//
// Each is a field on syntax.Layout that this dialect turns on and a formatter
// leaves off, which is the whole reason they are fields: a formatter that
// rewrote `$'\t'` into a literal tab, or a `<<"E"` into `<<'E'`, would be
// changing the program's text rather than laying it out.
func TestAListedBodyIsSpelledThisShellsWay(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The arithmetic `for` is the one loop whose `do` takes a line
			// of its own here, and its header keeps no separator.
			name: "the do of an arithmetic for",
			src:  "f() { for ((i=0;i<2;i++)); do echo $i; done; }\ndeclare -f f",
			want: "f () \n{ \n    for ((i=0; i<2; i++))\n    do\n        echo $i;\n    done\n}\n",
		},
		{
			// And the `1` each missing expression stands for is written out.
			name: "an omitted expression",
			src:  "g() { for ((;;)); do break; done; }\ndeclare -f g",
			want: "g () \n{ \n    for ((1; 1; 1))\n    do\n        break;\n    done\n}\n",
		},
		{
			name: "a partly omitted one",
			src:  "h() { for ((i=0;;i++)); do break; done; }\ndeclare -f h",
			want: "h () \n{ \n    for ((i=0; 1; i++))\n    do\n        break;\n    done\n}\n",
		},
		{
			// A delimiter carrying any quoting is respelled with single
			// quotes; a bare one stays bare, since quoting it would make the
			// body literal.
			name: "a double-quoted here-document delimiter",
			src:  "k() { cat <<\"EOT\"\nraw\nEOT\n}\ndeclare -f k",
			want: "k () \n{ \n    cat <<'EOT'\nraw\nEOT\n\n}\n",
		},
		{
			name: "a backslash-quoted one",
			src:  "k() { cat <<\\EOT\nraw\nEOT\n}\ndeclare -f k",
			want: "k () \n{ \n    cat <<'EOT'\nraw\nEOT\n\n}\n",
		},
		{
			name: "an unquoted one",
			src:  "k() { cat <<EOT\nraw\nEOT\n}\ndeclare -f k",
			want: "k () \n{ \n    cat <<EOT\nraw\nEOT\n\n}\n",
		},
		{
			// And a `$'…'` comes back as the characters it stands for.
			name: "an ANSI-C quoted word",
			src:  "m() { echo $'a\\tb'; }\ndeclare -f m",
			want: "m () \n{ \n    echo 'a\tb'\n}\n",
		},
		{
			name: "one in an assignment",
			src:  "n() { x=$'\\t'; }\ndeclare -f n",
			want: "n () \n{ \n    x='\t'\n}\n",
		},
		{
			name: "one inside a parameter expansion",
			src:  "o() { echo ${x-$'\\t'}; }\ndeclare -f o",
			want: "o () \n{ \n    echo ${x-'\t'}\n}\n",
		},
		{
			name: "one holding a quote",
			src:  "p() { echo $'a\\'b'; }\ndeclare -f p",
			want: "p () \n{ \n    echo 'a'\\''b'\n}\n",
		},
		{
			// The control, and the row that says this is the ANSI-C word
			// rather than the two characters: inside double quotes `$'` is
			// ordinary text here and comes back exactly as it was written.
			name: "a dollar and a quote inside double quotes",
			src:  "s() { echo \"x$'\\t'\"; }\ndeclare -f s",
			want: "s () \n{ \n    echo \"x$'\\t'\"\n}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if st != 0 || out != tc.want {
				t.Errorf("out %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}
