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
func TestTheEightRefuseOutsideACompletion(t *testing.T) {
	for _, name := range []string{
		"comparguments", "compdescribe", "compfiles", "compgroups",
		"compquote", "comptags", "comptry", "compvalues",
	} {
		t.Run(name, func(t *testing.T) {
			out, status := runZsh(t, t.TempDir(), name+" -i x")
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
