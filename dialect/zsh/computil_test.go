// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// `zsh/computil`'s eight builtins, asked of this shell what they answered on
// zsh 5.9.2.
//
// Every expectation here was measured through a pseudo-terminal on
// 2026-09-15, either by calling the builtin from inside a real `zle -C`
// widget's function or by shadowing it with a logging shell function and
// letting the *shipped* `_arguments`, `_tags` and `_describe` drive it. The
// measurements are written out in computil.go, comparguments.go,
// compargumentsline.go, comptags.go and compdescribe.go; what is here is
// them, asked of this shell.

// The seven `uname` option specs, exactly as the shipped `_uname` hands them
// to `comparguments` — read off the trace rather than retyped, because the
// whole point of the end-to-end test below is that this is not a shape
// somebody here invented.
const unameSpecs = `-i '' -s : ` +
	`'(-m -n -r -s -v)-a[print all basic information]' ` +
	`'-m[print hardware class]' ` +
	`'-n[print network node hostname]' ` +
	`'-p[print processor type]' ` +
	`'-r[print operating system release level]' ` +
	`'-s[print name of the operating system]' ` +
	`'-v[print detailed operating system version]'`

// TestTheShippedArgumentsProtocolOffersTheOptions is the whole of what this
// change buys, done the way the shipped `_arguments` does it: parse the
// specs, ask for the option lists, filter them through `compadd -D`, build
// the displays with `compdescribe`, and add what comes back.
//
// Measured end to end against `/bin/zsh` on the same line and the same rc
// (`compinit` against this machine's own functions) on 2026-09-15: `uname -`
// and Tab offers `-a -m -n -p -r -s -v`, in that order.
func TestTheShippedArgumentsProtocolOffersTheOptions(t *testing.T) {
	got := completionFor(t, widgetOf(`
		comparguments `+unameSpecs+` || return
		local -a next direct odirect equal
		comparguments -O next direct odirect equal || return
		local -a names=( ${next%%:*} )
		compadd -o nosort -J -default- -D next - "${names[@]}"
		compdescribe -I '' 40 '-- ' _expl -g next
		local csl; local -a _args _tmpm _tmpd
		while compdescribe -g csl _args _tmpm _tmpd; do
			compadd "${_args[@]}" -d _tmpd -a _tmpm
		done
	`), "uname -")
	want := []string{"-a", "-m", "-n", "-p", "-r", "-s", "-v"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("uname - offered %q, want %q", got, want)
	}
}

// TestTheShippedArgumentsProtocolOffersTheRestOfAStack is the one of #3039's
// four recorded gaps that turned out not to be one, asked end to end.
//
// **The offering is built by `_arguments` and not by the builtin.** When
// `comparguments -s` answers 0 the shipped function takes the option names out
// of `-O`'s own four arrays, strips the leading `-` from each single-letter
// one and writes `$PREFIX` back in front of it — so everything a stacked
// offering needs is `-s`'s status and `-O`'s arrays, and both were already
// here. What is written below is that, with the name-building spelled as a
// loop rather than as the nested expansion the shipped function writes.
//
// Measured on zsh 5.9.2, 2026-09-16 through a pseudo-terminal with `compinit`
// over this machine's own functions, Tab left where `compinit` put it:
// `uname -a<TAB>` completes to `uname -ap ` on `/bin/zsh` and on this shell
// alike — `-p` is all that `-a`'s exclusion list leaves — and `gzip -c<TAB>`
// twice lists the same twenty-three stacked words from both. The array the
// shipped `_arguments` builds was read out of a shadowing function on the
// same line and is identical in both shells:
//
//	_a_12=(-cd -cf -ch -ck -cl -cL -cn -cN -cq -cr -ct -cv -cV -c1 … -cS)
//
// Three specs with no exclusions between them, so that the answer is a list
// and not the single name `uname` happens to leave.
func TestTheShippedArgumentsProtocolOffersTheRestOfAStack(t *testing.T) {
	got := completionFor(t, widgetOf(`
		comparguments -i '' -s : '-a[all]' '-m[machine]' '-p[processor]' || return
		local -a next direct odirect equal
		comparguments -O next direct odirect equal || return
		local single
		comparguments -s single || return
		local -a stacked; local o
		for o in ${next%%:*}; do stacked+=( "$PREFIX${o#-}" ); done
		compadd -a stacked
	`), "uname -a")
	want := []string{"-am", "-ap"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("uname -a offered %q, want %q", got, want)
	}
}

// TestAStackIsOnlyOfferedWhereOneIsBeingWritten is the other side of it: the
// status `_arguments` reads before it builds any of that. Measured on zsh
// 5.9.2 from both directions — `uname -` is 1, because a lone `-` is not yet a
// stack, and `uname -a` is 0.
func TestAStackIsOnlyOfferedWhereOneIsBeingWritten(t *testing.T) {
	for _, c := range []struct{ name, line, want string }{
		{"a lone dash is not a stack", "uname -", "1"},
		{"a letter after it is", "uname -a", "0"},
		{"and a long option is not", "uname --a", "1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := reported(t, `comparguments -i '' -s : '-a[all]' '-m[machine]'
				local one; comparguments -s one; say $?`, c.line)
			if got != c.want {
				t.Errorf("%s answered %q, want %q", c.line, got, c.want)
			}
		})
	}
}

// TestTheOptionUnderTheCursorIsOfferedBack is the one spent option that is
// offered all the same, and it is the whole twelve-cell table rather than the
// rows that happen to differ — a shell that withheld every spent option passes
// six of them and a shell that offered every one passes the other six.
//
// Measured on zsh 5.9.2, 2026-09-16 through a pseudo-terminal from inside a
// `zle -C` widget, the word under the cursor being exactly the option named.
// Visible as well as recorded: `git checkout --force<TAB>` closes the word and
// adds a space on `/bin/zsh` against this machine's own functions, and offered
// nothing here.
func TestTheOptionUnderTheCursorIsOfferedBack(t *testing.T) {
	// One spec set holding all six argument forms, so that every row is asked
	// of the same parse and a difference can only be the form.
	const forms = `'-a[plain]' '-n[next]:nx:' '-o=[out]:out:' ` +
		`'-e=-[eqd]:ed:' '-d-[dir]:dir:' '-f+[file]:file:' ` +
		`'--long[long]' '1:first:(x y)'`
	for _, c := range []struct {
		name, line, switches string
		want                 bool
	}{
		{"plain, no -s", "cmd -a", "", true},
		{"a separate argument, no -s", "cmd -n", "", true},
		{"an = argument, no -s", "cmd -o", "", true},
		{"an =-only argument, no -s", "cmd -e", "", true},
		{"an attached argument, no -s", "cmd -d", "", false},
		{"an optionally attached one, no -s", "cmd -f", "", false},
		{"plain, with -s", "cmd -a", "-s", false},
		{"a separate argument, with -s", "cmd -n", "-s", true},
		{"an = argument, with -s", "cmd -o", "-s", false},
		{"an =-only argument, with -s", "cmd -e", "-s", false},
		{"an attached argument, with -s", "cmd -d", "-s", false},
		{"an optionally attached one, with -s", "cmd -f", "-s", false},
		// And it is the stack and not the switch: a long option under `-s` is
		// not one, so it is offered back like any other. Measured beside the
		// short `-a` in one spec set, because the two rows together are what
		// say the predicate is `comparguments -s`' own.
		{"a long option under -s is not a stack", "cmd --long", "-s", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := reported(t, `comparguments -i '' `+c.switches+` : `+forms+`
				local -a n d od e; comparguments -O n d od e
				local -a all=( ${n%%:*} ${d%%:*} ${od%%:*} ${e%%:*} )
				say "${all[*]}"`, c.line)
			name := c.line[strings.LastIndex(c.line, " ")+1:]
			if back := strings.Contains(" "+got+" ", " "+name+" "); back != c.want {
				t.Errorf("%s %s offered %q, want %s back: %v",
					c.line, c.switches, got, name, c.want)
			}
		})
	}
}

// TestTheOptionUnderTheCursorIsTheOnlyOneOfferedBack is the other half of it,
// because "offered back" would pass just as well if every spent option were.
// Measured with `--all` and `--almost` declared: `cmd --all --almost<TAB>`
// offers `--almost` and not `--all`.
func TestTheOptionUnderTheCursorIsTheOnlyOneOfferedBack(t *testing.T) {
	got := reported(t, `comparguments -i '' : '--all[all]' '--almost[almost]' '-p[proc]'
		local -a n d od e; comparguments -O n d od e
		local -a names=( ${n%%:*} ); say "${names[*]}"`,
		"cmd --all --almost")
	if want := "--almost -p"; got != want {
		t.Errorf("cmd --all --almost offered %q, want %q", got, want)
	}
}

// TestComparguments is the six read-back verbs, each asked in the position
// its measurement was taken in.
func TestComparguments(t *testing.T) {
	for _, c := range []struct{ name, line, body, want string }{
		// `-i` is 0 where an option could be written here.
		{"options here", "uname -", `comparguments ` + unameSpecs + `; say $?`, "0"},
		// And 1 where the word is in argument position and nothing describes
		// an argument. Measured: `comparguments -i '' '-v[verbose]'` against
		// `git che` is 1.
		{
			"nothing here", "git che",
			`comparguments -i '' : '-v[verbose]'; say $?`, "1",
		},
		// Adding one argument spec is the whole of the difference.
		{
			"an argument here", "git che",
			`comparguments -i '' : '-v[verbose]' ':cmd:(alpha beta)'; say $?`, "0",
		},
		// `-O` sorts by where the option's argument may be written. Measured
		// against `make -`, where all four are non-empty.
		{
			"four lists", "cmd -",
			`comparguments -i '' : '-B[always]' '-O-[sync]' '-E+[eval]' '--file=[read]'
			 local -a n d od e
			 comparguments -O n d od e
			 say "${n[*]}/${d[*]}/${od[*]}/${e[*]}"`,
			"-B:always/-O:sync/-E:eval/--file:read",
		},
		// A spec with no `[…]` comes back as the bare name — measured,
		// `gzip`'s `--fast` has no colon in it.
		{
			"no description", "cmd -",
			`comparguments -i '' : '-B[always]' '--fast'
			 local -a n d od e; comparguments -O n d od e; say "${n[*]}"`,
			"-B:always --fast",
		},
		// The word under the cursor counts as already on the line, so an
		// option spelled out in full is spent and what it excludes is gone.
		// Measured: `uname -a` leaves `-p` and nothing else, because `-a` is
		// spent and it excludes the other five.
		{
			"the cursor's word is spent", "uname -a",
			`comparguments ` + unameSpecs + `
			 local -a n d od e; comparguments -O n d od e; say "${n[*]%%:*}"`,
			"-p",
		},
		// `-D` answers for the argument position even when the word is an
		// option, which is what lets `_arguments` offer both.
		{
			"the argument here", "uname -",
			`comparguments -i '' : '-v[verbose]' ':cmd:(alpha beta)'
			 local -a ds as ss; comparguments -D ds as ss
			 say "$ds/$as/$ss"`,
			"cmd/(alpha beta)/argument-1",
		},
		// **A numbered spec wins over the rest specification at its own
		// position**, and the rest specification takes over after it. Both
		// sides, because one alone does not separate the rule from a shell
		// that always answers with the first spec it has, or always with the
		// last. Measured on zsh 5.9.2, 2026-09-15, with `1:first:(a b)` and
		// `*:rest:(c d)` in force: `cmd foo` reports `first` and
		// `argument-1`, and `cmd x foo` reports `rest` and `argument-rest`.
		{
			"a numbered spec wins at its own position", "cmd foo",
			`comparguments -i '' : '1:first:(a b)' '*:rest:(c d)'
			 local -a ds as ss; comparguments -D ds as ss
			 say "$ds/$as/$ss"`,
			"first/(a b)/argument-1",
		},
		{
			"and the rest takes over after it", "cmd x foo",
			`comparguments -i '' : '1:first:(a b)' '*:rest:(c d)'
			 local -a ds as ss; comparguments -D ds as ss
			 say "$ds/$as/$ss"`,
			"rest/(c d)/argument-rest",
		},
		// A `*:…` spec covers whatever number the argument turns out to be.
		{
			"the rest", "make foo ",
			`comparguments -i '' : '-B[always]' '*:make target:->target'
			 local -a ds as ss; comparguments -D ds as ss
			 say "$ds/$as/$ss"`,
			"make target/->target/argument-rest",
		},
		// `-M` is `_arguments`' own match specification, or the documented
		// default where it had none.
		{
			"the default matcher", "uname -",
			`comparguments -i '' : '-v[verbose]'
			 local m; comparguments -M m; say "$m"`,
			`r:|[_-]=* r:|=*`,
		},
		{
			"a given matcher", "uname -",
			`comparguments -i '' -M 'l:|=*' : '-v[verbose]'
			 local m; comparguments -M m; say "$m"`,
			`l:|=*`,
		},
		// `-W` is the normal arguments and the options already written.
		// Measured with `uname -a`: `$line` is empty and `$opt_args` holds
		// `-a`.
		{
			"the line so far", "uname -a",
			`comparguments ` + unameSpecs + `
			 local -a l; local -A oa; comparguments -W l oa 0
			 say "${#l}/${(k)oa}"`,
			"0/-a",
		},
		// And the word under the cursor is on it too, unless it is an
		// option: measured, `uname -a foo bar` reports `line=(foo bar)`.
		{
			"an argument reaches the line", "cmd -B foo bar",
			`comparguments -i '' : '-B[always]' '*:thing:'
			 local -a l; local -A oa; comparguments -W l oa 0
			 say "${l[*]}/${(k)oa}"`,
			"foo bar/-B",
		},
		{
			"an unknown word is an argument too", "uname -a -",
			`comparguments ` + unameSpecs + `
			 local -a l; local -A oa; comparguments -W l oa 0; say "${l[*]}"`,
			"-",
		},
		// **An option is in `$opt_args` whether or not its argument is there
		// yet.** Measured on zsh 5.9.2, 2026-09-16 over the five argument
		// forms: every one of them maps the option to an empty string when
		// the word under the cursor is the option itself, and to the value
		// when one is attached. This recorded nothing at all for the four
		// forms that declare an argument and had not been given one.
		{
			"an option with no argument yet", "cmd -n",
			`comparguments -i '' : '-n[next]:nx:' '-a[plain]'
			 local -a l; local -A oa; comparguments -W l oa 0
			 say "${(ko)oa}/[${oa[-n]}]"`,
			"-n/[]",
		},
		{
			"and one with its argument attached", "cmd -fval",
			`comparguments -i '' : '-f+[file]:file:' '-a[plain]'
			 local -a l; local -A oa; comparguments -W l oa 0
			 say "${(ko)oa}/[${oa[-f]}]"`,
			"-f/[val]",
		},
		// And the option under the cursor is not offered back where the word
		// is the option *plus* the start of its argument. Measured: `-o=val`
		// with `-o=[out]:out:` and `-f+[file]:file:` declared answers with
		// `-f` in `odirect` and an empty `equal` — the option being written
		// is past the point where its own name would help.
		{
			"an attached argument is not the option again", "cmd -o=val",
			`comparguments -i '' : '-o=[out]:out:' '-f+[file]:file:' '-p[proc]'
			 local -a n d od e; comparguments -O n d od e
			 local -a names=( ${n%%:*} ${d%%:*} ${od%%:*} ${e%%:*} )
			 say "${names[*]}"`,
			"-p -f",
		},
		// `-a` is whether any normal argument is described at all.
		{"no argument described", "uname -", `comparguments ` + unameSpecs + `
			 comparguments -a; say $?`, "1"},
		{"an argument described", "uname -", `comparguments -i '' : ':cmd:(a b)'
			 comparguments -a; say $?`, "0"},
		// `-s` is whether a stack of single-letter options is being
		// continued. Measured from both sides on `uname`.
		{"not a stack", "uname -", `comparguments ` + unameSpecs + `
			 local one; comparguments -s one; say $?`, "1"},
		{"a stack", "uname -a", `comparguments ` + unameSpecs + `
			 local one; comparguments -s one; say $?`, "0"},
		// **`-O` is not asked about the word under the cursor, and `-i`
		// is.** Measured on zsh 5.9.2, 2026-09-16 from inside a `zle -C`
		// widget with `-v[verbose]` and `-o[opt]:val:` the only specs: `cmd
		// f` answers 1 from `-i` — nothing can be completed there, the word
		// has begun as something no option can be — and 0 from `-O` with
		// both options in `next`, because the *position* still takes them.
		// `_arguments` does that filtering itself and reads `-O`'s status to
		// decide whether to ask `_tags` for the `options` tag at all, so a 1
		// here is a shipped completion told this position takes no options.
		//
		// Both halves, because a shell that read the word in neither place
		// would pass the second row and a shell that read it in both — which
		// is what this was — passes the first.
		{
			"a word no option can be is nothing to complete", "cmd f",
			`comparguments -i '' : '-v[verbose]' '-o[opt]:val:'; say $?`, "1",
		},
		{
			"but the position still takes them", "cmd f",
			`comparguments -i '' : '-v[verbose]' '-o[opt]:val:'
			 local -a n d od e; comparguments -O n d od e
			 say "$?/${n[*]}"`,
			"0/-v:verbose -o:opt",
		},
		// And what does stop `-O` is the position, from both directions.
		// Measured on the same day: an argument whose `(-)` spent the
		// options, and a `*::` rest specification the sub-command's words
		// have begun under, are each 1 with four empty arrays on zsh.
		{
			"an argument that spent the options", "cmd a foo",
			`comparguments -i '' : '-v[verbose]' '(-)1:first:(a b)' '*:rest:(x y)'
			 local -a n d od e; comparguments -O n d od e
			 say "$?/${n[*]}"`,
			"1/",
		},
		{
			"a rest specification that took over", "cmd sub foo",
			`comparguments -i '' : '-v[verbose]' '*:: :->rest'
			 local -a n d od e; comparguments -O n d od e
			 say "$?/${n[*]}"`,
			"1/",
		},
		// **A `*::` rest specification does not take its arguments out of
		// `$line`.** #3039 recorded the opposite — that zsh moves them out of
		// `$line` and into `$words`, leaving `$#line` 0 where this reports 2 —
		// and it is not so. Measured on zsh 5.9.2, 2026-09-16 through a
		// pseudo-terminal, both by asking the builtin from inside a widget and
		// by letting the shipped `_arguments` run and printing its own `$line`,
		// with `cmd sub arg <TAB>` under `-v[verbose] -o[opt]:val: *:: :->rest`:
		// `$line` is `(sub arg '')` and `$words` is `(sub arg '')` as well.
		// `*:`, `*::`, `*:::`, `(-)*::` and a numbered spec in front of one all
		// answer the same, each asked on the same line so that a shell reading
		// the colons differently would separate.
		{
			"a rest specification leaves the line alone", "cmd sub arg ",
			`comparguments -i '' : '-v[verbose]' '-o[opt]:val:' '*:: :->rest'
			 local -a ds as ss; comparguments -D ds as ss
			 local -a l; local -A oa; comparguments -W l oa 0
			 say "${#l}/${l[1]}/${l[2]}/[${l[3]}]"`,
			"3/sub/arg/[]",
		},
		// **`-s` answers 1 where `_arguments` was never given `-s`**, whatever
		// the word looks like. Measured: the same `uname -a` that is 0 with
		// the switch is 1 without it, because there is no stack to continue.
		{
			"no stacking, no stack", "uname -a",
			`comparguments -i '' : '-a[all]' '-m[machine]'
			 local one; comparguments -s one; say $?`, "1",
		},
		// And the parameter it names is `next` for one shape only: a stack of
		// exactly one letter whose option takes a separate word. Measured on
		// zsh 5.9.2, 2026-09-16 over five lines of one spec set, because a
		// shell that filled it from the last letter of any stack passes the
		// first row and fails the third.
		{
			"a single option with an argument of its own", "cmd -n",
			`comparguments -i '' -s : '-n[next]:nx:' '-a[plain]' '-p[proc]'
			 local one; comparguments -s one; say "$?/$one"`, "0/next",
		},
		{
			"a single option with no argument", "cmd -a",
			`comparguments -i '' -s : '-n[next]:nx:' '-a[plain]' '-p[proc]'
			 local one; comparguments -s one; say "$?/$one"`, "0/",
		},
		{
			"a stack of two", "cmd -an",
			`comparguments -i '' -s : '-n[next]:nx:' '-a[plain]' '-p[proc]'
			 local one; comparguments -s one; say "$?/$one"`, "0/",
		},
		// And without `-s` the same word fills nothing, because there is no
		// stack for `_arguments` to be told to stop building. Measured: the
		// `cmd -n` that answers `0/next` with the switch answers `1/`
		// without it.
		{
			"no stacking, nothing to say about one", "cmd -n",
			`comparguments -i '' : '-n[next]:nx:' '-a[plain]' '-p[proc]'
			 local one; comparguments -s one; say "$?/$one"`, "1/",
		},
		// **A word is a stack only where every letter after the dash is a
		// single-letter option**, and a word that reaches one the specs do
		// not know is not a stack at all — the letters before it are not
		// spent either. Measured on zsh 5.9.2, 2026-09-16 with `-s` and
		// `-n[next]:nx:`, `-a[plain]`, `-p[proc]` and `1:first:(x y)`:
		//
		//	typed     -s   $opt_args     $line   -O next
		//	cmd -z    1    (empty)       -z      -n -a -p
		//	cmd -az   1    (empty)       -az     -n -a -p
		//	cmd -na   0    -a '' -n ''   (empty) -p
		//
		// so `-az` spends nothing and is the first argument being written,
		// and `-na` spends both. All three rows, because a shell that spent
		// as it walked and gave up part-way — which is what this did —
		// passes the first and the third.
		{
			"an undeclared letter is not a stack", "cmd -az",
			`comparguments -i '' -s : '-n[next]:nx:' '-a[plain]' '-p[proc]' '1:first:(x y)'
			 local -a l; local -A oa; comparguments -W l oa 0
			 local one; comparguments -s one
			 say "$?/${l[*]}/${(ko)oa}"`,
			"1/-az/",
		},
		{
			"nor is a longer option spelled out", "cmd -ab",
			`comparguments -i '' -s : '-ab[two]:x:' '-a[plain]' '-p[proc]'
			 local -a l; local -A oa; comparguments -W l oa 0
			 local one; comparguments -s one
			 say "$?/${l[*]}/${(ko)oa}"`,
			"1//-ab",
		},
		// And the two halves of that rule pulled apart: with `-b` declared as
		// well, the letter walk accepts `-ab` and only "the word is itself a
		// longer option" refuses it. Measured: zsh answers 1 and records
		// `-ab` in `$opt_args`, with `-a` and `-b` untouched.
		{
			"a longer option beats the letters that spell it", "cmd -ab",
			`comparguments -i '' -s : '-ab[two]:x:' '-a[plain]' '-b[bee]' '-p[proc]'
			 local -a l; local -A oa; comparguments -W l oa 0
			 local one; comparguments -s one
			 say "$?/${(ko)oa}"`,
			"1/-ab",
		},
		{
			"and every letter known is", "cmd -na",
			`comparguments -i '' -s : '-n[next]:nx:' '-a[plain]' '-p[proc]' '1:first:(x y)'
			 local -a l; local -A oa; comparguments -W l oa 0
			 local one; comparguments -s one
			 say "$?/${l[*]}/${(ko)oa}"`,
			"0//-a -n",
		},
		// **A `+` leads a stack as a `-` does**, and a `-+x` spec is two
		// names with the `+` spelling first. Measured on zsh 5.9.2,
		// 2026-09-16 with `-s` and `-+a[plus]`, `-+b[bee]`, `-o[opt]:val:`
		// and `-p[proc]` declared:
		//
		//	cmd +ab<TAB>          $opt_args +a '' +b '', $line empty
		//	cmd -o val foo<TAB>   next=(+a:plus -a:plus +b:bee -b:bee -p:proc)
		//	cmd +a<TAB>           next=(-a:plus +b:bee -b:bee -o:opt -p:proc)
		//
		// The `+` stack used to land on `$line` as an ordinary argument, and
		// the pair used to come back `-` first. The third row is what says
		// the pair really is two names: `+a` is spent and `-a` is not.
		{
			"a plus leads a stack too", "cmd +ab",
			`comparguments -i '' -s : '-+a[plus]' '-+b[bee]' '-p[proc]' '1:first:(x y)'
			 local -a l; local -A oa; comparguments -W l oa 0
			 local one; comparguments -s one
			 say "$?/${l[*]}/${(ko)oa}"`,
			"0//+a +b",
		},
		{
			"and the plus spelling is offered first", "cmd -o val foo",
			`comparguments -i '' -s : '-+a[plus]' '-+b[bee]' '-o[opt]:val:' '-p[proc]' '1:first:(x y)'
			 local -a n d od e; comparguments -O n d od e
			 local -a names=( ${n%%:*} ); say "${names[*]}"`,
			"+a -a +b -b -p",
		},
		{
			"and each spelling is spent on its own", "cmd +a",
			`comparguments -i '' -s : '-+a[plus]' '-+b[bee]' '-o[opt]:val:' '-p[proc]' '1:first:(x y)'
			 local -a n d od e; comparguments -O n d od e
			 local -a names=( ${n%%:*} ); say "${names[*]}"`,
			"-a +b -b -o -p",
		},
		// An option's argument taken from the **following word** is the value
		// `$opt_args` carries. Measured: `cmd -o val foo` reports `-o val`
		// and `$line` as `foo` alone.
		{
			"an argument from the next word", "cmd -o val foo",
			`comparguments -i '' : '-o[opt]:val:' '-p[proc]' '*:rest:'
			 local -a l; local -A oa; comparguments -W l oa 0
			 say "${l[*]}/${(ko)oa}/[${oa[-o]}]"`,
			"foo/-o/[val]",
		},
		// **An option that takes more than one word joins them with a
		// colon.** Measured on zsh 5.9.2, 2026-09-16 with
		// `-C+[copy]:from:(f1 f2):to:(t1 t2)` declared and `cmd -C a b foo`:
		// `-C` is mapped to `a:b` and `$line` is `foo` alone. This kept only
		// the last word, so a two-argument option lost its first.
		{
			"two words joined by a colon", "cmd -C a b foo",
			`comparguments -i '' : '-C+[copy]:from:(f1 f2):to:(t1 t2)' '-p[proc]' '*:rest:'
			 local -a l; local -A oa; comparguments -W l oa 0
			 say "${l[*]}/${(ko)oa}/[${oa[-C]}]"`,
			"foo/-C/[a:b]",
		},
		// And `-W` takes **three** arguments: the shipped `_arguments` always
		// writes its own `$opt_args_use_NUL_separators` as the third.
		// Measured: two is `comparguments:9: not enough arguments` at 1.
		{
			"the line wants three names", "uname -a",
			`comparguments ` + unameSpecs + `
			 local -a l; local -A oa
			 comparguments -W l oa 2>/dev/null; say $?`,
			"1",
		},
		// **A stack spends every letter in it.** Measured with `-s` and
		// `-a -m -p`: `cmd -am <TAB>` leaves `-p` and nothing else, and
		// `$line` has neither `-am` nor its letters on it.
		{
			"a stack spends its letters", "cmd -am ",
			`comparguments -i '' -s : '-a[all]' '-m[machine]' '-p[proc]' '*:rest:'
			 local -a n d od e; comparguments -O n d od e
			 local -a names=( ${n%%:*} ); say "${names[*]}"`,
			"-p",
		},
		{
			"a stack is not an argument", "cmd -am foo",
			`comparguments -i '' -s : '-a[all]' '-m[machine]' '-p[proc]' '*:rest:'
			 local -a l; local -A oa; comparguments -W l oa 0
			 say "${l[*]}/${(ko)oa}"`,
			"foo/-a -m",
		},
		// And with no `-s` the same word is not a stack at all: it is an
		// option nobody declared, so it lands on `$line` whole. Measured.
		{
			"without -s a stack is an argument", "cmd -am foo",
			`comparguments -i '' : '-a[all]' '-m[machine]' '-p[proc]' '*:rest:'
			 local -a l; local -A oa; comparguments -W l oa 0
			 say "${l[*]}/${(ko)oa}"`,
			"-am foo/",
		},
		// An option whose exclusion list names another takes it off the
		// offering once it is on the line.
		{
			"an exclusion", "uname -a -",
			`comparguments ` + unameSpecs + `
			 local -a n d od e; comparguments -O n d od e; say "${n[*]%%:*}"`,
			"-p",
		},
		// A spec that is not one is an error naming itself, which is the
		// measured diagnostic.
		{"not a spec", "uname -", `comparguments -i '' : x 2>/dev/null; say $?`, "1"},
		// And a query before `-i` has parsed anything is refused.
		{"no state", "uname -", `comparguments -a 2>/dev/null; say $?`, "1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := reported(t, c.body, c.line); got != c.want {
				t.Errorf("%s answered %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestComptagsRunsTheTagLoop is `_tags`, `_requested` and `_next_label`
// arriving at the two sets the shipped `_arguments` gets for `make `.
//
// Measured: `-i` with `argument-rest options`, then the three `comptry` calls
// the shipped `_tags` always writes, gives two sets — `argument-rest` then
// `options` — and the third `comptry` adds nothing, because a tag is used
// once.
func TestComptagsRunsTheTagLoop(t *testing.T) {
	const loop = `
		comptags -i :complete:make: argument-rest options
		comptry -m '(|*-)argument-* (|*-)option[-+]* values'
		comptry -m options
		comptry argument-rest options
		comptags -T || return
		local out=
		while comptags -N; do
			comptags -R argument-rest && out="$out a"
			comptags -R options && out="$out o"
			out="$out;"
		done
		say "$out"`
	if got := reported(t, loop, "make "); got != " a; o;" {
		t.Errorf("the tag loop ran %q, want %q", got, " a; o;")
	}
}

// TestComptags is the rest of the verbs, each from both sides.
func TestComptags(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		// `-T` is whether any set was built, and it is 1 until one is.
		{"nothing tried", `comptags -i :x: aa; comptags -T; say $?`, "1"},
		{"something tried", `comptags -i :x: aa; comptry aa; comptags -T; say $?`, "0"},
		// A `comptry` naming a tag that was not offered builds no set.
		{"an unoffered tag", `comptags -i :x: aa; comptry zz; comptags -T; say $?`, "1"},
		// `-R` is membership of the set `-N` stepped to, from both sides.
		{
			"requested",
			`comptags -i :x: aa bb; comptry aa; comptags -N
			 comptags -R aa; local one=$?; comptags -R bb; say "$one$?"`,
			"01",
		},
		// `-N` is 1 when the sets run out.
		{
			"the sets run out",
			`comptags -i :x: aa; comptry aa; comptags -N; comptags -N; say $?`, "1",
		},
		// `-A` is the label loop: once per tag in a set, and then done.
		{
			"one label per tag",
			`comptags -i :x: aa; comptry aa; comptags -N
			 local t s n=0
			 while comptags -A aa t s; do n=$(( n + 1 )); done
			 say "$n/$t"`,
			"1/aa",
		},
		// `-i` needs a tag to loop over, which is the measured diagnostic.
		{"no tags", `comptags -i :x: 2>/dev/null; say $?`, "1"},
		// And every other verb is refused before one.
		{"no state", `comptags -N 2>/dev/null; say $?`, "1"},
		{"comptry without state", `comptry aa 2>/dev/null; say $?`, "1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := reported(t, c.body, "make "); got != c.want {
				t.Errorf("%s answered %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestCompdescribe is the two-pass builder, and the padding is the row worth
// having: it is what makes a listing line up, and it is measured.
func TestCompdescribe(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{
			"names and descriptions",
			`local -a g=(alpha:one beta:two) expl=()
			 compdescribe -I '' 40 '-- ' expl g || return
			 local csl; local -a a m d
			 compdescribe -g csl a m d || return
			 say "${m[*]}|${d[1]}|${d[2]}"`,
			"alpha beta|alpha  -- one|beta   -- two",
		},
		// A second array says what to insert, and the first then only says
		// what to show. Measured.
		{
			"a separate match array",
			`local -a g=(alpha:one beta:two) gm=(A1 B1) expl=()
			 compdescribe -I '' 40 '-- ' expl g gm || return
			 local csl; local -a a m d
			 compdescribe -g csl a m d || return
			 say "${m[*]}|${d[1]}"`,
			"A1 B1|alpha  -- one",
		},
		// `-i` is the same without the descriptions, and takes one argument
		// fewer — measured, `compdescribe -i '' 40 '-- ' expl …` is
		// `invalid argument: -- `.
		{
			"no descriptions",
			`local -a g=(alpha:one beta:two) expl=()
			 compdescribe -i '' 40 expl g || return
			 local csl; local -a a m d
			 compdescribe -g csl a m d || return
			 say "${m[*]}|${d[*]}"`,
			"alpha beta|alpha beta",
		},
		// An empty group is stepped over rather than handed back, which is
		// what `_arguments` relies on: it defines three and `compadd -D` has
		// emptied two of them by the time `-g` runs.
		{
			"an empty group is skipped",
			`local -a empty=() g=(alpha:one) expl=()
			 compdescribe -I '' 40 '-- ' expl empty -- g || return
			 local csl; local -a a m d; local n=0
			 while compdescribe -g csl a m d; do n=$(( n + 1 )); done
			 say "$n/${m[*]}"`,
			"1/alpha",
		},
		// The group's own `compadd` options come back for the caller to pass
		// on, with the listing flag in front of them.
		{
			"the group's options",
			`local -a g=(alpha:one) expl=(-J -default-)
			 compdescribe -I '' 40 '-- ' expl -g g -M 'r:|=*' || return
			 local csl; local -a a m d
			 compdescribe -g csl a m d || return
			 say "${a[*]}"`,
			"-l -M r:|=*",
		},
		// One definition whose rows are not all described is **two** groups,
		// the described half first and carrying the `-l`. Measured through a
		// pseudo-terminal on zsh 5.9.2 over `gzip -c`, where the described
		// options are drawn a row each and `-1` … `-9` are packed under
		// them; the trace of that line is in compdescribe.go (#3232).
		{
			"a definition with both kinds is two groups",
			`local -a g=(alpha:one bare: beta:two) expl=()
			 compdescribe -I '' 40 '-- ' expl g || return
			 local csl; local -a a m d; local out=
			 while compdescribe -g csl a m d; do out="${out}[${a[*]}|${m[*]}]"; done
			 say "$out"`,
			"[-l|alpha beta][|bare]",
		},
		// And a definition where every row is described is still one group,
		// which is what says the split is the arrangement rather than a
		// second pass over everything.
		{
			"a definition of one kind stays one group",
			`local -a g=(alpha:one beta:two) expl=()
			 compdescribe -I '' 40 '-- ' expl g || return
			 local csl; local -a a m d; local out=
			 while compdescribe -g csl a m d; do out="${out}[${a[*]}|${m[*]}]"; done
			 say "$out"`,
			"[-l|alpha beta]",
		},
		// `-i` describes nothing, so nothing carries `-l` and there is
		// nothing to split.
		{
			"nothing described is one plain group",
			`local -a g=(alpha:one beta:two) expl=()
			 compdescribe -i '' 40 expl g || return
			 local csl; local -a a m d; local out=
			 while compdescribe -g csl a m d; do out="${out}[${a[*]}|${m[*]}]"; done
			 say "$out"`,
			"[|alpha beta]",
		},
		{"no state", `compdescribe -g a b c d 2>/dev/null; say $?`, "1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := reported(t, c.body, "uname -"); got != c.want {
				t.Errorf("%s answered %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestCompvalues is `_values`' half of the same machinery.
func TestCompvalues(t *testing.T) {
	for _, c := range []struct{ name, line, body, want string }{
		// The value under the cursor counts as already given, exactly as an
		// option does. Measured with `tar c`: `c` is missing from the answer
		// and nothing else is.
		{
			"the cursor's value is spent", "tar c",
			`compvalues -i -s '' 'tar function' '(c t)A[append]' 'c[create]' 'v[verbose]'
			 local -a na ar op; compvalues -V na ar op; say "${na[*]}"`,
			"A:append v:verbose",
		},
		// A value taking an argument is in the second array.
		{
			"with and without an argument", "cmd x",
			`compvalues -i -S '=' opt 'aa[first]' 'bb[second]:val:(x y)'
			 local -a na ar op; compvalues -V na ar op; say "${na[*]}/${ar[*]}"`,
			"aa:first/bb:second",
		},
		// The argument separator defaults to `=` rather than to nothing,
		// which is measured: a `-i` naming only `-s` still answers `=`.
		{
			"the default argument separator", "cmd x",
			`compvalues -i -s '' opt 'aa[first]'
			 local s; compvalues -S s; say "$s"`,
			"=",
		},
		{
			"the description", "cmd x",
			`compvalues -i -s '' 'tar function' 'c[create]'
			 local d; compvalues -d d; say "$d"`,
			"tar function",
		},
		// `-D` is the value whose argument the cursor is inside, and 1 where
		// the cursor is still on a name.
		// Measured with `cmd bb=`: `-D` answers for the argument, and `-V`
		// goes on listing the values all the same, so the two are not two
		// halves of one question.
		{
			"an argument's description", "cmd bb=",
			`compvalues -i -S '=' opt 'aa[first]' 'bb[second]:val:(x y)'
			 local d a; compvalues -D d a; local one=$?
			 local -a na ar op; compvalues -V na ar op
			 say "$one/$d/$a/${na[*]}/${ar[*]}"`,
			"0/val/(x y)/aa:first/bb:second",
		},
		{
			"not inside an argument", "cmd bb",
			`compvalues -i -S '=' opt 'bb[second]:val:(x y)'
			 local d a; compvalues -D d a; say $?`,
			"1",
		},
		{"no state", "cmd x", `compvalues -V a b c 2>/dev/null; say $?`, "1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := reported(t, c.body, c.line); got != c.want {
				t.Errorf("%s answered %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestCompquoteQuotesInPlace is the one of the eight the manual calls useful
// outside the shipped tree.
func TestCompquoteQuotesInPlace(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"a scalar", `local one='a b'; compquote one; say "$one"`, `a\ b`},
		{
			"an array",
			`local -a many=('a b' 'c$d'); compquote many; say "${many[*]}"`,
			`a\ b c\$d`,
		},
		// `-p` is accepted and changes nothing here, because this shell does
		// not escape a leading `=` in the first place.
		{"-p", `local one='=x'; compquote -p one; say "$one"`, `=x`},
		{"nothing named", `compquote 2>/dev/null; say $?`, "1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := reported(t, c.body, "uname "); got != c.want {
				t.Errorf("%s answered %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestCompfilesAndCompgroupsAnswerWithoutClaiming is the pair that is
// registered rather than written, and the assertion is that each one's answer
// is the *conservative* one — see compfiles.go and compgroups.go.
func TestCompfilesAndCompgroupsAnswerWithoutClaiming(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"compfiles builds no pattern", `compfiles -p t a '' ' ' '' f '*'; say $?`, "0"},
		{"compfiles removes nothing", `compfiles -r t ''; say $?`, "1"},
		{"compfiles ignores nothing", `compfiles -i t ''; say $?`, "1"},
		{"compgroups", `compgroups alpha beta; say $?`, "0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := reported(t, c.body, "ls f"); got != c.want {
				t.Errorf("%s answered %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestTheEightRefuseOutsideACompletion is the same refusal `compadd` gives,
// and the reason the state lives on the context: a `comparguments` reached in
// a script, a hook or at a prompt finds nothing and says so.
//
// Each call here carries enough words to pass the dispatcher's count, which
// is what has to happen before the refusal about the place is reached at all
// — `compdescribe -i x` is two words and answers the count instead. See
// TestTheCompletionBuiltinsCountTheirWordsFirst.
func TestTheEightRefuseOutsideACompletion(t *testing.T) {
	for _, name := range []string{
		"comparguments", "compdescribe", "compfiles", "compgroups",
		"compquote", "comptags", "comptry", "compvalues",
	} {
		t.Run(name, func(t *testing.T) {
			out, status := runZsh(t, t.TempDir(), name+" -i x y")
			if status == 0 {
				t.Errorf("%s outside a completion was 0", name)
			}
			if !strings.Contains(out, "completion function") {
				t.Errorf("%s said %q, want a refusal naming a completion function",
					name, out)
			}
		})
	}
}

// TestTheCompletionBuiltinsCountTheirWordsFirst is the gate zsh applies
// before any of the ten runs, and the order this shell had backwards.
//
// zsh's dispatcher checks each builtin's declared minimum and maximum against
// the words it was given and refuses there, so the commonest call a script
// can make — a bare one — is answered about the *count* and never reaches the
// sentence about completion functions. Eight of the ten declare a minimum;
// `compadd` and `comptry` declare none and go straight to the place.
//
// Measured on zsh 5.9.2, 2026-09-16, every builtin called outside a
// completion with 0 to 20 words, with `zmodload zsh/complete` and `zmodload
// zsh/computil` first. The count is of words rather than operands: an option
// letter is a word, so `compdescribe -i a` is two and refuses.
//
// Found by `make coverage` (#2293, under #2291), which reported all ten as
// surface no case in the tree ever asked about. Eight of the ten were wrong.
func TestTheCompletionBuiltinsCountTheirWordsFirst(t *testing.T) {
	const few, many, place = "not enough arguments", "too many arguments",
		"can only be called from completion function"
	for _, c := range []struct{ name, src, want string }{
		{"compadd declares no minimum", "compadd", place},
		{"comptry declares no minimum", "comptry", place},
		{"comparguments", "comparguments", few},
		{"comparguments with one word", "comparguments x", place},
		{"compfiles", "compfiles", few},
		{"compgroups", "compgroups", few},
		{"compquote", "compquote", few},
		{"compquote parses its option off first", "compquote -p", few},
		{"compquote with two options and nothing else", "compquote -p -p", few},
		{"compquote with an operand behind them", "compquote -p -p x", place},
		{"compdescribe with three operands", "compdescribe a b c", place},
		{"comptags", "comptags", few},
		{"compvalues", "compvalues", few},
		{"compset", "compset", few},
		{"compset takes at most three", "compset -p 1 x", place},
		{"compset refuses a fourth", "compset -p 1 x x", many},
		{"compdescribe declares three", "compdescribe", few},
		{"compdescribe with two words", "compdescribe -i a", few},
		{"compdescribe with three words", "compdescribe -i a b", place},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, status := runZsh(t, t.TempDir(), c.src)
			if status != 1 {
				t.Errorf("%s left status %d, want 1", c.src, status)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("%s said %q, want %q", c.src, out, c.want)
			}
		})
	}
}

// TestCompaddEndsItsOptionsOnALoneDash is the old spelling the manual's own
// example uses and the one `_arguments` writes to this day.
//
// **Asked on a word that begins with a hyphen**, which is the only place the
// question discriminates: on `git che` a stray `-` candidate is filtered out
// by the prefix anyway, so the test passes whether the dash was read as an
// end-of-options marker or as a candidate. On `uname -` it is not, and a
// shell that read it as a candidate offers an eighth match spelled `-`.
// Measured on zsh 5.9.2, 2026-09-15: `compadd - -a -m` there offers two.
func TestCompaddEndsItsOptionsOnALoneDash(t *testing.T) {
	got := completionFor(t, widgetOf("compadd -o nosort - -a -m"), "uname -")
	want := []string{"-a", "-m"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("compadd - offered %q, want %q", got, want)
	}
	// And `--` still ends them, on the same word, so the row above is not
	// accidentally asserting that every leading dash is swallowed.
	if got := completionFor(t, widgetOf("compadd -o nosort -- -a -m"), "uname -"); //
	strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("compadd -- offered %q, want %q", got, want)
	}
}

// TestCompaddStrikesThroughTheArraysItIsGiven is `-D`, by position, more than
// once, and with an array longer than the candidate list.
func TestCompaddStrikesThroughTheArraysItIsGiven(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{
			"by position",
			`local -a d=(A1 A2 A3); compadd -D d -- checkout commit cherry
			 say "${d[*]}"`,
			"A1 A3",
		},
		{
			"more than once",
			`local -a d=(A1 A2) e=(B1 B2); compadd -D d -D e -- checkout commit
			 say "${d[*]}/${e[*]}"`,
			"A1/B1",
		},
		{
			"past the candidates",
			`local -a d=(A1 A2 A3); compadd -D d -- checkout commit
			 say "${d[*]}"`,
			"A1 A3",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := reported(t, c.body, "git che"); got != c.want {
				t.Errorf("%s left %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// reported runs a completion function and hands back the one thing it said
// with `say`.
//
// Through `compadd -U -Q`, which is how a completion function gets a string
// out past this editor's own filtering: `-U` turns the matching off and `-Q`
// stops the quoting, so what comes back is exactly what went in.
func reported(t *testing.T, body, line string) string {
	t.Helper()
	got := completionFor(t,
		"say() { compadd -U -Q -- \"$*\" }\n"+widgetOf(body), line)
	if len(got) == 0 {
		return ""
	}
	return got[len(got)-1]
}

// TestACompletionWidgetsFunctionIsCalledWithNoArguments is the second thing
// the shipped system found, and it could not be seen from a hand-written
// completion at all.
//
// Measured on zsh 5.9.2, 2026-09-15: a `zle -C wtest .complete-word _f` whose
// `_f` prints `$#` reports 0. This shell used to pass the widget's name, and
// `_main_complete` takes the *completers to try* as its arguments — so the
// widget's own name arrived as one and came back as `command not found:
// expand-or-complete`, which is what the shipped completion system's first
// call looked like from the outside.
func TestACompletionWidgetsFunctionIsCalledWithNoArguments(t *testing.T) {
	got := completionFor(t, widgetOf(`compadd -U -Q -- "n=$# [$*]"`), "git che")
	if len(got) != 1 || got[0] != "n=0 []" {
		t.Errorf("the function saw %q, want [n=0 []]", got)
	}
}

// TestARedefinedStandardWidgetOnItsOwnKeyReachesTheEditor is the third, and
// it is the one that made everything above invisible.
//
// `compinit` does not rebind Tab. It redefines the widget Tab is already
// bound to — `zle -C expand-or-complete .expand-or-complete _main_complete` —
// so the sequence and the widget name both still read as this editor's
// default, and the binding that carries the whole completion system was
// dropped as "nothing to say". Measured: with `compinit` run, `uname -` and
// Tab completed nothing here and offered seven options in zsh.
func TestARedefinedStandardWidgetOnItsOwnKeyReachesTheEditor(t *testing.T) {
	r := bindkeyRunner(t, "_f() { compadd -U -Q -- x }\n"+
		"zle -C expand-or-complete .expand-or-complete _f\n")
	got, bound := zsh.KeyBindings(r, repl.KeymapMain)["\t"]
	if !bound {
		t.Fatalf("Tab is absent from %v", zsh.KeyBindings(r, repl.KeymapMain))
	}
	if got.Widget != repl.WidgetComplete || got.Candidates != "expand-or-complete" {
		t.Errorf("Tab is %+v, want the editor's completion asking expand-or-complete", got)
	}
	// And a standard widget nobody redefined is still left out, which is what
	// keeps this table the override layer repl/bindings.go describes.
	plain := bindkeyRunner(t, "bindkey '^I' expand-or-complete\n")
	if _, present := zsh.KeyBindings(plain, repl.KeymapMain)["\t"]; present {
		t.Error("Tab left at its own default is in the table")
	}
}
