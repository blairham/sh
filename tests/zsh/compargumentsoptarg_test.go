// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `comparguments -D` describing an **option's own** argument, which it never
// did: a completion that had written `gzip -S` and wanted the suffix, or
// `git checkout --orphan=` and wanted a branch, was answered with nothing
// (#3229).
//
// Every row was measured on zsh 5.9.2, 2026-09-18 through a pseudo-terminal
// from inside a real `zle -C` widget, against one spec set so that a
// difference between two rows can only be the shape being asked about:
//
//	-f+[file]:file:_files   attached, or the next word
//	-d-[dir]:dir:_d         attached, and only attached
//	-o=[out]:out:_o         after an `=`, or the next word
//	-e=-[eq]:eq:_e          after an `=`, and only there
//	-n[none]                no argument at all
//	-T[two]:one:(a):two:(b) two arguments, each a word
//	--orphan=[branch]:…     the long spelling of the `=` form
//	*-I+[inc]:dir:          repeatable
//	1:first:(x y) 2:second:(p q)
//
// The whole set is asked of every row, so a row that answered from the wrong
// spec would name the wrong message.

const optArgSpecs = `'-f+[file]:file:_files' '-d-[dir]:dir:_d' ` +
	`'-o=[out]:out:_o' '-e=-[eq]:eq:_e' '-n[none]' ` +
	`'--orphan=[branch]:branch:(b1 b2)' '-T[two]:one:(a):two:(b)' ` +
	`'*-I+[inc]:dir:_i' '1:first:(x y)' '2:second:(p q)'`

// describedAt asks `-D` about one line and reports the message, the action
// and the tag of what it found — the three arrays together, because the tag
// alone would pass for a shell that found the right *position* and read the
// wrong spec.
func describedAt(t *testing.T, switches, line string) string {
	t.Helper()
	return reported(t, `comparguments -i '' `+switches+` : `+optArgSpecs+`
		local -a d a s
		if comparguments -D d a s; then
			say "${d[*]}|${a[*]}|${s[*]}"
		else
			say "none"
		fi`, line)
}

// Where an option's argument may be written decides what the cursor is
// standing in, and that is the whole rule.
func TestDescribingAnOptionsOwnArgument(t *testing.T) {
	for _, c := range []struct{ name, line, want string }{
		// The word under the cursor holds the argument, attached.
		{"an attached argument being written", "cmd -fval", "file|_files|option-f-1"},
		{"the bare name of an option it attaches to", "cmd -f", "file|_files|option-f-1"},
		{"and of one it must attach to", "cmd -d", "dir|_d|option-d-1"},
		{"after the `=` it needs", "cmd -o=val", "out|_o|option-o-1"},
		{"the `=` alone", "cmd -e=", "eq|_e|option-e-1"},
		{"the long spelling", "cmd --orphan=", "branch|(b1 b2)|option--orphan-1"},
		// The word under the cursor is the option's argument as a word of its
		// own, which only some of the forms allow.
		{"the next word, where the form allows one", "cmd -f ", "file|_files|option-f-1"},
		{"and where the next word is the only place", "cmd -T ", "one|(a)|option-T-1"},
		{"an `=` form still takes the next word", "cmd -o ", "out|_o|option-o-1"},
		{"a second argument is always a word", "cmd -T a ", "two|(b)|option-T-2"},
		{"a repeatable option, again", "cmd -I/u -I", "dir|_i|option-I-1"},
		// And the rows that say it is the *form* deciding: the same cursor
		// position against a form that does not put an argument there is the
		// command's own first argument, or nothing.
		{"attached only, so the next word is not it", "cmd -d ", "first|(x y)|argument-1"},
		{"after an `=` only, so the next word is not it", "cmd -e ", "first|(x y)|argument-1"},
		{"an option with no argument", "cmd -n ", "first|(x y)|argument-1"},
		{"the bare name of an option that needs an `=`", "cmd -e", "none"},
		{"an option already given its argument", "cmd -f a ", "first|(x y)|argument-1"},
		// The command's own arguments are unmoved by all of it.
		{"nothing written yet", "cmd ", "first|(x y)|argument-1"},
		{"one written", "cmd x ", "second|(p q)|argument-2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := describedAt(t, "", c.line); got != c.want {
				t.Errorf("%q described %q, want %q", c.line, got, c.want)
			}
		})
	}
}

// A name whose argument can only be reached by typing more into the same word
// is not read as an option at all: it is a normal argument being written.
//
// Measured — `cmd -o<TAB>` against `-o=[out]:out:` reports the command's first
// argument, `$line` holding `-o` and `$opt_args` empty. The two rows beside it
// are what say the reading is that narrow: `-e=-` needs the same `=` and *is*
// read as an option, and `-f+` puts the cursor in its own argument.
func TestAnOptionNameThatStillNeedsItsSeparator(t *testing.T) {
	line := reported(t, `comparguments -i '' : `+optArgSpecs+`
		local -a l; typeset -A oa
		comparguments -W l oa 0
		say "line=(${l[*]}) opts=(${(k)oa})"`, "cmd -o")
	if want := "line=(-o) opts=()"; line != want {
		t.Errorf("`cmd -o` reported %q, want %q", line, want)
	}
	if got := describedAt(t, "", "cmd -o"); got != "first|(x y)|argument-1" {
		t.Errorf("`cmd -o` described %q, want the command's first argument", got)
	}
	// And with the `=` written it is the option's argument after all.
	if got := describedAt(t, "", "cmd -o="); got != "out|_o|option-o-1" {
		t.Errorf("`cmd -o=` described %q, want the option's own argument", got)
	}
}

// Inside a stack of single-letter options the last letter's argument
// attaches, whatever separator the form asks for when it is written alone.
//
// Measured with `-s` given: a stack is written without separators by
// definition, so `-ae` reaches `-e=-`'s argument where a bare `-e` does not.
// `-an` is the row that says it is still the form deciding — `-n[next]` has
// no argument to attach.
func TestAnOptionArgumentInsideAStack(t *testing.T) {
	const stackSpecs = `'-a[plain]' '-n[next]:nx:_n' '-o=[out]:out:_o' ` +
		`'-e=-[eqd]:ed:_e' '-d-[dir]:dir:_d' '-f+[file]:file:_f' '1:first:(x y)'`
	for _, c := range []struct{ name, line, want string }{
		{"an `=` form in a stack", "cmd -ao", "out|_o|option-o-1"},
		{"an attached form", "cmd -ad", "dir|_d|option-d-1"},
		{"an optionally attached one", "cmd -af", "file|_f|option-f-1"},
		{"an `=`-only form attaches here", "cmd -ae", "ed|_e|option-e-1"},
		{"a form whose argument is a word does not", "cmd -an", "none"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := reported(t, `comparguments -i '' -s : `+stackSpecs+`
				local -a d a s
				if comparguments -D d a s; then
					say "${d[*]}|${a[*]}|${s[*]}"
				else
					say "none"
				fi`, c.line)
			if got != c.want {
				t.Errorf("%q described %q, want %q", c.line, got, c.want)
			}
		})
	}
}

// `-i` and `-D` agree about whether there is anything to complete.
//
// The pair matters because `_arguments` asks `-i` first and gives up on a 1:
// a 0 from `-D` sitting behind a 1 from `-i` is an argument nothing will ever
// ask for. Measured — `cmd -f x<TAB>` is 0 from both.
func TestTheInitStatusAgreesWithTheDescription(t *testing.T) {
	for _, line := range []string{"cmd -f x", "cmd -f ", "cmd -fval", "cmd -o="} {
		got := reported(t, `comparguments -i '' : `+optArgSpecs+`
			local i=$?
			local -a d a s
			comparguments -D d a s
			say "i=$i D=$?"`, line)
		if got != "i=0 D=0" {
			t.Errorf("%q answered %q, want both 0", line, got)
		}
	}
}

// What `$opt_args` holds for an option that has been given more than one
// value, and for a repeatable one given several.
//
// Measured: a two-argument option joins its values with a colon, and the
// count rather than the text decides the separator — one empty argument is
// the empty string and two is a lone colon. A repeatable option keeps what
// earlier occurrences gave it; an occurrence with nothing to add adds
// nothing, not even a separator.
func TestWhatAnOptionCollectsInOptArgs(t *testing.T) {
	for _, c := range []struct{ name, line, want string }{
		{"no value yet", "cmd -T ", "-T ''"},
		{"one, and the second still empty", "cmd -T a ", "-T 'a:'"},
		{"attached, and the second a word", "cmd -f a ", "-f 'a'"},
		{"a repeatable option twice", "cmd -I/u -I/v", "-I '/u:/v'"},
		{"and once with nothing to add", "cmd -I/u -I", "-I '/u'"},
		{"and once with an empty word", "cmd -I/u -I ", "-I '/u:'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := reported(t, `comparguments -i '' : `+optArgSpecs+`
				local -a l; typeset -A oa
				comparguments -W l oa 0
				local k; local -a out
				for k in ${(ko)oa}; do out+=( "$k '${oa[$k]}'" ); done
				say "${out[*]}"`, c.line)
			if got != c.want {
				t.Errorf("%q collected %q, want %q", c.line, got, c.want)
			}
		})
	}
}
