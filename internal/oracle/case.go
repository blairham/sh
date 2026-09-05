// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

// Case is one snippet, run across the panel.
//
// Cases are ours. They are written from the POSIX text and from what a spec
// entry needs to assert, never lifted from another project's suite — see
// CLEANROOM.md.
type Case struct {
	// ID is stable and is the key in the golden file. Renaming one reads as
	// a deletion plus an addition, which is correct: the old evidence is gone.
	ID string

	// Category groups rows in the rendered table.
	Category string

	// Snippet is the shell code. It should print something that makes the
	// behavior visible — field boundaries as [a][b], counts as n=2 — rather
	// than relying on exit status alone.
	Snippet string

	// Why records what this case exists to pin down. A case without a reason
	// cannot be evaluated when it later changes, so this is required in
	// practice even though nothing enforces it.
	Why string

	// Script runs the snippet from a file instead of -c. Set it only when the
	// behavior depends on how input is read, and say why.
	Script bool

	// Args is the argv the shell is invoked with, in place of the harness's
	// own `-c` and the snippet. It is what lets the corpus grade the
	// *invocation* surface — `-e` with a script, a bundle of option letters,
	// `-o name`, which operand becomes `$0` — rather than only the language.
	//
	// Both sides of a comparison are handed the same words, after the flags
	// that say which shell each of them is: `-dialect bash` for the binary
	// under test, nothing for a panel member. So an option this harness has
	// never heard of is still the same option in both runs, which is the
	// whole point — the drivers once scored 178/198 while the core scored
	// 198/198, because the corpus graded the core and nothing graded the
	// invocation.
	//
	// The snippet still has to reach the shell, and Args says where it goes.
	// At most one placeholder may appear, and it is replaced before the shell
	// sees it:
	//
	//	ArgSnippet  the Snippet — where `-c` wants its operand, or
	//	            `-o name -c` does, or a bundle ending in c.
	//	ArgScript   the path of a file holding the Snippet: the same file
	//	            Script writes, in the same scratch directory, and it
	//	            normalizes to <script> in the recorded output.
	//
	// A placeholder is replaced **wherever it appears**, rather than only as
	// a whole word, which is the same rule Stdin has always followed. That
	// matters because `-c` takes its command string as its own word only when
	// it is written that way: `sh -c'echo hi'` attaches the string to the
	// letter, and an invocation needing the snippet inside a larger word is
	// spelled `"-c" + ArgSnippet`.
	//
	// Requiring a whole word meant such a case had to write the text twice —
	// once in Snippet and once literally in Args — and nothing checked that
	// the two stayed in step, so an edit to either one silently made the case
	// test something other than what it recorded. Interpolation removes the
	// second copy rather than guarding it (#535).
	//
	// A case with no placeholder never hands the shell its Snippet at all,
	// and that is deliberate rather than an oversight: an invocation that
	// must fail before it reads anything — a script path that does not exist,
	// `-c` with nothing after it — is one of the shapes worth pinning, and it
	// still wants a Snippet written down as the thing that would have run.
	// It is also how the program arrives on standard input; see Stdin.
	//
	// Args and Script are exclusive. Script is the shorthand for
	// []string{ArgScript}, and setting both is a harness error rather than a
	// silent precedence rule nobody would think to look for.
	Args []string

	// Stdin is what the shell finds on its standard input, handed to both
	// sides of a comparison byte for byte. Empty means the input is closed,
	// which is what every case without one gets and why the record does not
	// depend on what the harness itself was started with.
	//
	// It is a separate question from where the *program* comes from, and the
	// two combine in three useful ways:
	//
	//	Snippet alone, or with Args naming ArgSnippet or ArgScript: the
	//	program arrives the usual way and Stdin is data — the line `read`
	//	consumes, the choice `select` is answered with. Those are otherwise
	//	only reachable through a here-string or a pipe inside the snippet,
	//	neither of which is the shell's own input.
	//
	//	Args naming no placeholder, plus Stdin: the shell reads its program
	//	from standard input. That is the route `sh < script` and `echo … |
	//	sh` take, and it is its own invocation — `$0` stays the shell rather
	//	than becoming a path, and with `-s` every operand is a positional
	//	parameter and none of them a script. There is no way to spell it with
	//	Args alone, because Args with no placeholder hands the shell nothing
	//	to run.
	//
	//	Script or ArgScript, plus Stdin: a file is the program and standard
	//	input is still data, which is the ordinary shape of a script that
	//	reads.
	//
	// ArgSnippet and ArgScript are replaced here as well, wherever they
	// appear rather than only as the whole string, so the stdin-program route
	// spells its program once: Stdin is ArgSnippet + "\n" and Snippet stays
	// the single copy of the text. That keeps the rendered table showing what
	// actually ran, which a second hand-kept copy would not.
	Stdin string

	// Argv0 is the name every shell in the run is invoked under for this
	// case, in place of the one its panel entry gives it — and the binary
	// under test is given it too, which is what makes it a graded surface
	// rather than only a recorded one.
	//
	// It exists because a shell's own name is a startup input like any other,
	// and Args cannot reach it: Args is what comes *after* argv[0], and the
	// one word it never contains is the one being asked about. A shell called
	// `sh` starts in POSIX mode, and until this there was no way to write
	// that down as a case — the panel's own bash-as-sh column pins it for
	// bash and for no other shell, and it pins it for the whole corpus rather
	// than for the case that means to ask.
	//
	// Every column moves together, which is the point. A case that sets it is
	// asking what each shell does under that name, so the answer wanted from
	// the `bash` column is bash-under-that-name; a case that leaves it empty
	// is asking about each shell under its own, which is every other case.
	// The recorded output normalizes the given name exactly as it normalizes
	// a panel entry's, so a diagnostic still reads `<shell>:` on both sides.
	Argv0 string

	// GradedOnRefusal grades a case on the fact that the shell said no,
	// rather than on the words it said no in.
	//
	// The shapes most worth pinning about an invocation are the ones a shell
	// *refuses* — a command string attached to its letter, an option that is
	// not one, `-c` with nothing after it — and each shell refuses in its own
	// words. Those wordings are deliberately different: they are what the
	// Diagnostics vector exists for, and a dialect is supposed to sound like
	// the shell it names. So an exact comparison scores four shells that all
	// agree about the behavior as four disagreements, and the case that would
	// catch the real bug cannot be written at all.
	//
	// What is forgiven is exactly the text of the diagnostic. What is not:
	//
	//	the outcome    the same status, signal and timeout as always
	//	standard out   byte for byte as always, so a shell that *ran* what
	//	               the reference declined to run still fails
	//	the refusal    both sides must have ended nonzero *and* written
	//	               something to standard error — two requirements the
	//	               exact comparison never makes
	//
	// The last of those is the safeguard. This mode is not "compare less",
	// it is "compare a different thing that can still be false", and it is
	// stricter than exact grading in the one direction that matters: put the
	// flag on a case the reference does not refuse and the case *fails*,
	// because a misused flag has to be louder than a correct one. A case that
	// refuses silently fails too — saying nothing is not a wording
	// difference.
	//
	// It changes grading only. The record stays exact: measurements.md prints
	// what each shell actually said, and the drift check still compares every
	// byte, so a shell that changes its wording is still caught. The rendered
	// table marks these rows, because a relaxation a reader cannot enumerate
	// is indistinguishable from a score that is wrong.
	GradedOnRefusal bool

	// LayoutSensitive marks a case whose output depends on how the source is
	// laid out rather than only on what it means — a diagnostic naming the
	// line it happened on, where that line is a fact about the text.
	//
	// The printer is excused from such a case. Its promise is that printed
	// source *means* the same thing, and its zero layout deliberately
	// belongs to no shell: it puts a loop's `do` on the line of its `while`,
	// which moves everything after. Both are right and they cannot both be
	// checked by running the printed form and comparing what it said.
	LayoutSensitive bool

	// SyntaxError marks a case that does not parse **under the bash
	// dialect**, which is the one the parser's conformance test uses. The
	// corpus records rejections as well as successes, because a rule is only
	// pinned by showing both sides of it.
	//
	// Bash rather than "the shells" because rejection is not always
	// unanimous: `@(abc|xyz)` is a syntax error in dash and bash, accepted by
	// ksh93, and parsed-but-unmatched by zsh. Naming the dialect makes the
	// flag answerable; naming the panel would not.
	SyntaxError bool

	// ReferenceRaces marks a case whose *reference* output is not stable,
	// because the shell being measured races with itself.
	//
	// Conformance grades a binary against a live panel shell rather than
	// against the golden record, so a reference that answers differently on
	// different runs makes the score move for reasons that have nothing to do
	// with the implementation under test. Measured on the one case that does
	// this: ksh93 prints a pipeline's trace lines in the order its two
	// processes happen to reach them, 374 times one way and 26 the other in
	// 400 runs — which showed up as a conformance number that wandered by one
	// about once in fourteen full runs, in *both* directions, because the
	// same coin flip fails a dialect that expects one order and passes the one
	// that expects the other.
	//
	// Such a case is still worth recording: the divergence is real and the
	// measurement documents it. It is simply not evidence about anybody's
	// conformance, so it is not graded and not checked for drift.
	//
	// And not resampled. Neither of those two exemptions stopped `make
	// oracle` from writing down whichever answer the coin gave, which was
	// the *only* way such a row could move and made every move noise — a
	// regeneration in a pull request about something else carrying a diff a
	// reviewer then has to recognize and dismiss. So a regeneration keeps
	// what is recorded for a marked row, per shell, for as long as the shell
	// is the same build; see Run.KeepRacingRows, which also has the measured
	// flip rates. To resample deliberately, delete the row from the golden
	// record and regenerate.
	ReferenceRaces bool

	// Unfinished marks a case whose input is legitimately *unfinished* even
	// though it runs to completion — a here-document whose delimiter never
	// arrives is the only shape so far.
	//
	// The parser reports both facts, and they are not the same one: the body
	// is everything to the end and the command runs, which is what every
	// shell does, and the input is still open in the sense a prompt cares
	// about, which is why a terminal asks for another line rather than
	// running with what it has. The corpus otherwise checks that a snippet
	// which ran is not reported unfinished, and that check is right for
	// every case but this shape.
	Unfinished bool
}

// Corpus is the checked-in set. Every table in docs/spec should be derivable
// from a case here; a spec claim with no case behind it is a claim nobody can
// re-check.
var Corpus = []Case{
	// --- field splitting: what is subject to it ------------------------
	{
		ID: "split/unquoted-param", Category: "field splitting",
		Snippet: `x="a b"; set -- $x; printf "[%s]" "$@"`,
		Why:     "the base case: an unquoted parameter expansion is split",
	},
	{
		ID: "split/quoted-param", Category: "field splitting",
		Snippet: `x="a b"; set -- "$x"; printf "[%s]" "$@"`,
		Why:     "quoting suppresses splitting; without this the base case proves nothing",
	},
	{
		ID: "split/cmdsub-unquoted", Category: "field splitting",
		Snippet: `set -- $(printf "a b"); echo "n=$#"`,
		Why:     "zsh does not split parameter expansions but does split this — the axis is narrower than 'word splitting'",
	},
	{
		ID: "split/param-braced", Category: "field splitting",
		Snippet: `x="a b"; set -- ${x}; echo "n=$#"`,
		Why:     "braces are not quoting; ${x} splits wherever $x does",
	},
	{
		ID: "split/empty-value", Category: "field splitting",
		Snippet: `x=""; set -- $x; echo "n=$#"`,
		Why:     "an unquoted empty expansion produces no field at all",
	},
	{
		ID: "split/empty-value-quoted", Category: "field splitting",
		Snippet: `x=""; set -- "$x"; echo "n=$#"`,
		Why:     "quoted, the same value produces one empty field — the pair is the point",
	},
	{
		ID: "split/unset-value", Category: "field splitting",
		Snippet: `unset u; set -- $u; echo "n=$#"`,
		Why:     "unset behaves as empty here, rather than as an error",
	},

	// --- field splitting: IFS mechanics --------------------------------
	{
		ID: "ifs/default-runs-collapse", Category: "IFS",
		Snippet: `x="a b   c"; set -- $x; printf "[%s]" "$@"`,
		Why:     "a run of IFS whitespace is one delimiter",
	},
	{
		ID: "ifs/default-edges-stripped", Category: "IFS",
		Snippet: `x="  a  b  "; set -- $x; printf "[%s]" "$@"`,
		Why:     "leading and trailing IFS whitespace is discarded entirely",
	},
	{
		ID: "ifs/nonws-separates", Category: "IFS",
		Snippet: `IFS=:; x="a:b:c"; set -- $x; printf "[%s]" "$@"`,
		Why:     "each non-whitespace IFS character delimits",
	},
	{
		ID: "ifs/nonws-adjacent-empty-field", Category: "IFS",
		Snippet: `IFS=:; x="a::b"; set -- $x; printf "[%s]" "$@"`,
		Why:     "adjacent non-whitespace delimiters produce an empty field, unlike whitespace",
	},
	{
		ID: "ifs/nonws-leading", Category: "IFS",
		Snippet: `IFS=:; x=":a"; set -- $x; printf "[%s]" "$@"`,
		Why:     "half of the leading/trailing asymmetry: a leading delimiter makes an empty field",
	},
	{
		ID: "ifs/nonws-trailing", Category: "IFS",
		Snippet: `IFS=:; x="a:"; set -- $x; printf "[%s]" "$@"`,
		Why:     "the other half: a trailing delimiter does not. A symmetric implementation fails here",
	},
	{
		ID: "ifs/nonws-only-delimiters", Category: "IFS",
		Snippet: `IFS=:; x="::"; set -- $x; echo "n=$#"`,
		Why:     "pins the asymmetry as a count rather than as a rendering",
	},
	{
		ID: "ifs/mixed-ws-around-nonws", Category: "IFS",
		Snippet: `IFS=" :"; x="a : b"; set -- $x; printf "[%s]" "$@"`,
		Why:     "whitespace around a non-whitespace delimiter joins it into one delimiter",
	},
	{
		ID: "ifs/mixed-adjacent-nonws", Category: "IFS",
		Snippet: `IFS=" :"; x="a::b"; set -- $x; printf "[%s]" "$@"`,
		Why:     "mixing does not disable the empty-field rule",
	},
	{
		ID: "ifs/empty-disables-splitting", Category: "IFS",
		Snippet: `IFS=; x="a b"; set -- $x; printf "[%s]" "$@"`,
		Why:     "an empty IFS disables the stage; it does not mean 'split on nothing'",
	},
	{
		ID: "ifs/unset-is-default", Category: "IFS",
		Snippet: `unset IFS; x="a b"; set -- $x; printf "[%s]" "$@"`,
		Why:     "unset is not the same state as empty, and restores the default",
	},

	// --- special parameters ---------------------------------------------
	{
		ID: "params/at-quoted-keeps-fields", Category: "special parameters",
		Snippet: `set -- "a b" c; printf "[%s]" "$@"`,
		Why:     `"$@" is one field per parameter, each keeping its spaces`,
	},
	{
		ID: "params/star-quoted-joins", Category: "special parameters",
		Snippet: `set -- a b; printf "[%s]" "$*"`,
		Why:     `"$*" joins into a single field`,
	},
	{
		ID: "params/star-joins-with-ifs", Category: "special parameters",
		Snippet: `set -- a b; IFS=:; printf "[%s]" "$*"`,
		Why:     `the joiner is IFS's first character, not a space`,
	},
	{
		ID: "params/at-empty-is-zero-fields", Category: "special parameters",
		Snippet: `set --; set -- "$@"; echo "n=$#"`,
		Why:     `"$@" with no parameters yields zero fields — why set -- "$@" is safe`,
	},
	{
		ID: "params/star-empty-is-one-field", Category: "special parameters",
		Snippet: `set --; set -- "$*"; echo "n=$#"`,
		Why:     `"$*" yields one empty field instead — why set -- "$*" is not safe`,
	},

	// --- expansion pipeline ---------------------------------------------
	{
		ID: "expand/results-not-rescanned-quote", Category: "expansion",
		Snippet: `x="a\"b"; set -- $x; printf "[%s]" "$@"`,
		Why:     "a quote in expanded text is a literal quote",
	},
	{
		ID: "expand/results-not-rescanned-dollar", Category: "expansion",
		Snippet: `x="\$HOME"; echo "$x"`,
		Why:     "expansion does not recurse; this is why eval exists",
	},
	{
		ID: "expand/results-not-rescanned-semicolon", Category: "expansion",
		Snippet: `x="a;b"; set -- $x; printf "[%s]" "$@"`,
		Why:     "an operator in expanded text is data, not syntax",
	},
	{
		ID: "expand/an-assignment-value-is-not-split", Category: "expansion",
		Snippet: `two="a b"; x=$two; IFS=:; y="p:q"; z=$y; printf "<%s><%s>" "$x" "$z"`,
		Why:     "an assignment's value expands but is never field-split, whatever IFS holds — the same exemption the case subject and `[[ ]]` operands have, so the splitting question does not arise in this position",
	},
	{
		ID: "expand/an-assignment-value-from-a-command-is-not-split", Category: "expansion",
		Snippet: `x=$(echo a b); printf "<%s>" "$x"`,
		Why:     "the exemption covers a command substitution's result too, which the splitting question otherwise governs — the value is stored with its space, unanimous",
	},
	{
		ID: "expand/a-here-string-is-not-split", Category: "expansion",
		Snippet: "x=\"a:b:c\"; IFS=:; cat <<< $x; cat <<< \"$x\"",
		Why:     "the no-split exemption on a here-string word, and the one place in the set where the panel is not unanimous: bash 3.2 splits the unquoted word and rejoins the fields on a space, where bash 5.3, ksh93 and zsh hand the value through whole. A dated answer rather than a disputed one, and the quoted line beside it shows the split is the only difference",
	},
	{
		ID: "expand/a-heredoc-body-is-not-split", Category: "expansion",
		Snippet: "x=\"a:b:c\"; IFS=:; cat <<EOF\n[$x]\nEOF",
		Why:     "a here-document body is input rather than a word list, so an expansion in it is never split however IFS is set — unanimous, including the bash 3.2 that splits a here-string",
	},
	{
		ID: "expand/arithmetic-text-is-not-split", Category: "expansion",
		Snippet: `IFS=+; n="1+2"; echo "[$(( $n ))]"`,
		Why:     "the inside of $(( )) is expanded like a double-quoted string and only then read as an expression, so a value holding an operator that is also in IFS is still one expression — splitting it would leave `1 2`, which every shell calls an arithmetic syntax error",
	},
	{
		ID: "expand/glob-applies-to-expansion", Category: "expansion",
		Snippet: `cd /; x="et*"; set -- $x; printf "[%s]" "$@"`,
		Why:     "globbing runs after splitting, so an expansion result is matched — except in zsh",
	},
	{
		ID: "expand/glob-not-applied-when-quoted", Category: "expansion",
		Snippet: `cd /; x="et*"; set -- "$x"; printf "[%s]" "$@"`,
		Why:     "quoting the expansion suppresses the match; the pair isolates the cause",
	},
	{
		ID: "expand/glob-literal-pattern", Category: "expansion",
		Snippet: `cd /; set -- et*; printf "[%s]" "$@"`,
		Why:     "a literal pattern is matched even in zsh — separating that from expansion results",
	},
	{
		ID: "expand/glob-no-match", Category: "expansion",
		Snippet: `echo /zzz_no_such*`,
		Why:     "an unmatched pattern passes through, except in zsh where it is an error",
	},
	{
		ID: "expand/brace", Category: "expansion",
		Snippet: `echo {1..3}`,
		Why:     "brace expansion is absent from dash",
	},
	{
		ID: "expand/brace-range-alphabetic", Category: "expansion",
		Snippet: `echo {a..e}; echo {e..a}`,
		Why:     "a letter range counts bytes either way — unanimous among the shells that expand braces at all",
	},
	{
		ID: "expand/brace-range-stepped", Category: "expansion",
		Snippet: `echo {1..10..3}`,
		Why:     "a third number strides the range — unanimous among the shells that have ranges",
	},
	{
		ID: "expand/brace-range-zero-padded", Category: "expansion",
		Snippet: `echo {01..03}; echo {1..03}; echo {-03..3..3}`,
		Why:     "a leading zero on either endpoint pads the whole range to the widest, zeros after the sign; ksh93 alone strips the padding",
	},
	{
		ID: "expand/brace-range-step-sign-and-direction", Category: "expansion",
		Snippet: `echo {10..1..3}; echo {1..10..-3}`,
		Why:     "the endpoints decide the direction and the step contributes magnitude alone in bash; ksh93 honors the sign and stops after one element when it points the wrong way; zsh ignores a positive sign like bash but a negative one reverses its result",
	},
	{
		ID: "expand/brace-range-negative-step-reversal", Category: "expansion",
		Snippet: `echo {3..1..-1}; echo {1..10..-4}`,
		Why:     "the corners that separate the three step-sign models: a negative sign agreeing with the endpoints changes nothing in bash and ksh93 but still reverses zsh; and zsh reverses the walk rather than swapping the endpoints — 9 5 1, bash's 1 5 9 backwards, not 10 6 2",
	},
	{
		ID: "expand/brace-range-alpha-stepped", Category: "expansion",
		Snippet: `echo {a..e..2}`,
		Why:     "a stride over a letter range: bash and ksh93 expand it, zsh leaves the word alone",
	},
	{
		ID: "expand/brace-nested", Category: "expansion",
		Snippet: `echo {a,{b,c}}; echo x{1,{2,3}}y`,
		Why:     "an alternative may itself be a brace expansion, and a prefix and suffix distribute over the flattened result — {a,{b,c}} is three words, not a word containing braces",
	},
	{
		ID: "expand/brace-before-param", Category: "expansion",
		Snippet: `a=1; echo {$a,2}`,
		Why:     "braces resolve before parameter expansion, so variable ranges cannot work",
	},
	{
		ID: "expand/tilde-unquoted", Category: "expansion",
		Snippet: `case $(echo ~) in /*) echo abs;; *) echo literal;; esac`,
		Why:     "an unquoted leading tilde expands",
	},
	{
		ID: "expand/tilde-quoted", Category: "expansion",
		Snippet: `case "$(echo "~")" in /*) echo abs;; *) echo literal;; esac`,
		Why:     "quoting suppresses it",
	},
	{
		ID: "expand/tilde-in-assignment", Category: "expansion",
		Snippet: `x=~; case $x in /*) echo abs;; *) echo literal;; esac`,
		Why:     "assignment values are a tilde context, which is why PATH=~/bin works",
	},
	{
		ID: "expand/tilde-plus-and-minus", Category: "expansion",
		Snippet: `cd /tmp; cd /; echo ~+ ~- | sed "s|/private||g"; unset OLDPWD; echo ~-`,
		Why:     "~+ is $PWD and ~- is $OLDPWD in three of the four — dash keeps both as written — and only while the variable is set, except zsh, which still answers from directory state of its own",
	},
	{
		ID: "expand/tilde-after-a-colon-in-an-assignment", Category: "expansion",
		Snippet: `v=a:~/b:~; echo "$v" | sed "s|$HOME|H|g"; w=":~/q"; echo "$w"`,
		Why:     "the context adds a tilde after each unquoted colon — PATH=~/bin:~/sbin — and quoting turns it back off; unanimous",
	},
	{
		ID: "expand/tilde-into-an-expansion-diverges", Category: "expansion",
		Snippet: `u=/x; v=a:~$u; echo "$v" | sed "s|$HOME|H|g"`,
		Why:     "a tilde whose segment runs into an expansion stays literal in three of the four; zsh alone expands it and then appends the value",
	},

	// --- semantics axes --------------------------------------------------
	{
		ID: "axis/array-base", Category: "semantics axes",
		Snippet: `a=(x y); echo "${a[1]}"`,
		Why:     "zsh indexes arrays from 1; dash has no arrays at all",
	},
	{
		ID: "axis/echo-backslash", Category: "semantics axes",
		Snippet: `if [ "$(echo 'a\tb')" = 'a\tb' ]; then echo literal; else echo expanded; fi`,
		Why:     "dash and zsh expand escapes in echo; bash and ksh do not — a grouping no ladder predicts",
	},
	{
		ID: "echo/dash-e-and-capital-e", Category: "builtins",
		Snippet: `echo -e 'a\tb'; echo -E 'c\td'`,
		Why:     "-e turns escapes on where the shell has the letter and -E off where it has that one: dash has neither and prints them, ksh93 has only -e",
	},
	{
		ID: "echo/backslash-c-stops-the-output", Category: "builtins",
		Snippet: `echo 'p\cq'; echo done`,
		Why:     "\\c discards the rest of the output and the newline with it, wherever escapes are live — by default in dash and zsh, not at all without -e in bash and ksh93",
	},
	{
		ID: "echo/hex-escape-diverges", Category: "builtins",
		Snippet: `echo -e 'A\x41B'`,
		Why:     "\\xHH is bash and zsh on top of the XSI set: ksh93 leaves it as written even under -e, and dash has no -e at all",
	},
	{
		ID: "echo/the-order-of-e-and-capital-e", Category: "builtins",
		Snippet: `echo -e -E 'm\tn'`,
		Why:     "-e then -E: bash lets the last flag win and prints the backslash, zsh lets -e win and expands — the other order agrees everywhere and asks nothing",
	},
	{
		ID: "axis/pipeline-last-element", Category: "semantics axes",
		Snippet: `echo x | read v; echo "[$v]"`,
		Why:     "ksh and zsh run the last pipeline element in the current shell; dash and bash use a subshell",
	},
	{
		ID: "axis/dollar-zero-in-function", Category: "semantics axes",
		Snippet: `f() { echo "$0"; }; f`,
		Why:     "zsh reports the function name where the others report the shell",
	},
	{
		ID: "axis/local-builtin", Category: "semantics axes",
		Snippet: `f() { local v=1; echo "$v"; }; f`,
		Why:     "ksh93 is the only panel member without local, which is why it is not a compatibility target",
	},
	{
		ID: "axis/shift-past-end", Category: "semantics axes",
		Snippet: `shift 5; echo survived`,
		Why:     "fatal in dash and ksh, survivable in bash and zsh",
	},
	{
		ID: "axis/local-outside-a-function", Category: "semantics axes",
		Script:  true,
		Snippet: "local x=2\necho x=$x\necho end",
		Why:     "bash says so and carries on without setting it, dash says so and stops, ksh93 has no `local` at all, zsh sets a global",
	},
	{
		ID: "axis/local-inside-a-function", Category: "semantics axes",
		Script:  true,
		Snippet: "f() { local x=2; echo in=$x; }\nf\necho out=$x",
		Why:     "the control: where there is a function to be local to, three of the four agree and ksh93 still has no `local`",
	},
	{
		ID: "axis/trap-body-line-exit", Category: "diagnostics",
		Script:  true,
		Snippet: "echo one\ntrap 'echo a\nnosuchcmd-xyz' EXIT\necho two",
		Why:     "a two-line EXIT body: bash, dash and ksh93 name its second line, zsh names the line after the script's last",
	},
	{
		ID: "axis/trap-body-parse-failure-location", Category: "diagnostics",
		Script:  true,
		Snippet: "echo one\ntrap 'echo a\nif' USR1\necho two\nkill -USR1 $$\necho three",
		Why:     "the two lines are different numbers in ksh93: the location names where the trap fired and the wording names where in the body the parse gave out. bash and dash name the parse position in both, and zsh refused the trap when it was set",
	},
	{
		ID: "axis/trap-body-line-signal", Category: "diagnostics",
		Script:  true,
		Snippet: "trap 'echo a\nnosuchcmd-xyz' INT\necho two\nkill -INT $$\necho three",
		Why:     "the same body on a signal: ksh93 counts it from where it fired and zsh names only where it fired",
	},
	{
		ID: "axis/trap-body-will-not-parse", Category: "semantics axes",
		Script:  true,
		Snippet: "trap 'if' EXIT\necho after",
		Why:     "each dialect words it its own way, dash ends the script over it, and zsh refused the trap when it was set",
	},
	{
		ID: "axis/trap-body-runs-what-parsed", Category: "semantics axes",
		Script:  true,
		Snippet: "echo one\ntrap 'echo a\nif' EXIT\necho end",
		Why:     "bash and dash run the line that parsed before complaining; ksh93 reads the body whole and runs none of it",
	},
	{
		ID: "axis/trap-action-read-when-set", Category: "semantics axes",
		Script:  true,
		Snippet: "trap 'if' INT\necho after",
		Why:     "zsh reads the action now and refuses the trap; the other three store the text, and on INT never parse it at all",
	},
	{
		ID: "axis/trap-bad-option", Category: "semantics axes",
		Script:  true,
		Snippet: "trap -Q INT\necho end",
		Why:     "three read a leading dash as an option and refuse this one; zsh takes it as the action. INT rather than EXIT so the trap zsh sets never fires",
	},
	{
		ID: "axis/trap-print-p", Category: "semantics axes",
		Script:  true,
		Snippet: "trap 'echo hi' INT\ntrap -p\necho end",
		Why:     "bash and ksh93 print the traps, dash refuses the letter, zsh sets a trap on a condition it does not know",
	},
	{
		ID: "axis/trap-one-argument", Category: "semantics axes",
		Script:  true,
		Snippet: "trap 'echo hi' INT\ntrap INT\ntrap\necho end",
		Why:     "`trap INT` puts INT back in three of the four; ksh93 refuses the form and the refusal ends the script",
	},
	{
		ID: "axis/trap-one-argument-unknown", Category: "semantics axes",
		Script:  true,
		Snippet: "trap notacondition\necho end",
		Why:     "bash prints its usage, dash names the word, ksh93 refuses the form, zsh says nothing",
	},
	{
		ID: "axis/trap-prints-a-bare-action", Category: "diagnostics",
		Script:  true,
		Snippet: "trap : INT\ntrap\necho end",
		Why:     "a one-word action: bash and dash quote it anyway, ksh93 and zsh leave it bare",
	},
	{
		ID: "axis/trap-prints-a-quoted-action", Category: "diagnostics",
		Script:  true,
		Snippet: "trap 'echo hi' INT\ntrap\necho end",
		Why:     "a multi-word action, which all four quote the same way — a bare one does not, and that is its own question",
	},
	{
		ID: "axis/redirect-opened-for-a-builtin", Category: "diagnostics",
		Script:  true,
		Snippet: "true\necho hi > /nonexistent-dir-xyz/x\necho end",
		Why:     "ksh93 counts a redirection opened for a builtin as the builtin's own and brackets the line; zsh does not name the builtin for it",
	},
	{
		ID: "axis/builtin-names-the-place", Category: "diagnostics",
		Script:  true,
		Snippet: "true\ncd /no/such/dir-xyz\necho done",
		Why:     "ksh93 brackets the line for a builtin's own complaint: script[2]: cd: ...",
	},
	{
		ID: "axis/shell-names-the-place", Category: "diagnostics",
		Script:  true,
		Snippet: "true\nnosuchcmd-xyz\necho done",
		Why:     "the other half of the same script: not a builtin's complaint, so the word form",
	},
	{
		ID: "axis/builtin-names-the-place-command-string", Category: "diagnostics",
		Snippet: "true\nshift 99",
		Why:     "and names it from line 2 on, which is where the bracketed style shows under -c",
	},
	{
		ID: "axis/builtin-names-the-place-first-line", Category: "diagnostics",
		Snippet: "shift 99",
		Why:     "ksh93 leaves the line out on line 1 of a command string, in the bracketed style too",
	},
	{
		ID: "axis/readonly-refusal-names-the-builtin", Category: "diagnostics",
		Script:  true,
		Snippet: "readonly r=1\ndeclare r=2\necho end",
		Why:     "bash names the builtin for its own two spellings of a declaration and not for the two POSIX has, which is the case below",
	},
	{
		ID: "axis/readonly-refusal-does-not-name-export", Category: "diagnostics",
		Script:  true,
		Snippet: "readonly r=1\nexport r=2\necho end",
		Why:     "the other half: the same refusal from `export` carries no builtin name in bash, where dash names both of the two it has",
	},
	{
		ID: "axis/readonly-reassign-abandons-the-line", Category: "semantics axes",
		Script:  true,
		Snippet: "readonly r=1\nr=2; echo one\necho two",
		Why:     "the dialect that is not stopped by this still gives up the rest of the line: `one` never prints and `two` does",
	},
	{
		ID: "axis/readonly-reassign-abandons-a-loop", Category: "semantics axes",
		Script:  true,
		Snippet: "readonly r=1\nfor i in 1 2; do r=2; echo one; done\necho two",
		Why:     "and gives up whatever encloses it — the loop stops on its first round, where a plain failure would have carried on to the second",
	},
	{
		ID: "axis/readonly-reassign-status", Category: "semantics axes",
		Script:  true,
		Snippet: "readonly r=1\nr=2\necho st=$?",
		Why:     "the refusal leaves 1 behind in the dialect that carries on from it — the three that end the script never reach the line that would show it",
	},
	{
		ID: "axis/readonly-reassign-declaration-status", Category: "semantics axes",
		Script:  true,
		Snippet: "readonly r=1\nexport r=2\necho st=$?",
		Why:     "the same through a declaration, where the builtin's own status would otherwise report success for a name it refused to assign",
	},
	{
		ID: "axis/readonly-reassign", Category: "semantics axes",
		Script:  true,
		Snippet: "readonly r=1\nr=2\necho survived",
		Why:     "must be a plain assignment in a script: adding a redirect makes it a command and reverses the answer",
	},
	{
		ID: "axis/arith-error-status", Category: "semantics axes",
		Snippet: `echo $((1/0)); echo "st=$?"`,
		Why:     "dash exits 2 where bash, ksh93 and zsh exit 1; found by a test disagreeing with the conformance run, not by the sweep",
	},
	// --- tokenization -----------------------------------------------------
	{
		ID: "token/spans-within-a-word", Category: "tokenization",
		Snippet: `set -- a"b c"d; printf "[%s]" "$@"; echo " n=$#"`,
		Why:     "one word carrying quoted and unquoted spans; the case expansion.md's per-span requirement rests on",
	},
	{
		ID: "token/dquote-backslash-escapes-quote", Category: "tokenization",
		Snippet: `printf "[%s]" "a\"b"`,
		Why:     `inside double quotes backslash escapes " — one of only four characters it acts on`,
	},
	{
		ID: "token/dquote-backslash-literal-before-n", Category: "tokenization",
		Snippet: `printf "[%s]" "a\nb"`,
		Why:     "the rule C intuition gets wrong: \\n inside double quotes is backslash-then-n, not a newline",
	},
	{
		ID: "token/dquote-backslash-literal-before-other", Category: "tokenization",
		Snippet: `printf "[%s]" "a\qb"`,
		Why:     "confirms the previous case is a general rule rather than something special about n",
	},
	{
		ID: "token/squote-protects-backslash", Category: "tokenization",
		Snippet: `printf '[%s]' 'a$HOME'`,
		Why:     "single quotes protect everything; no escape exists inside them",
	},
	{
		ID: "token/backslash-escapes-dollar", Category: "tokenization",
		Snippet: `printf "[%s]" a\$HOME`,
		Why:     "an unquoted backslash protects the single following character",
	},
	{
		ID: "token/operator-delimits-without-space", Category: "tokenization",
		Snippet: `echo a>b; printf "[%s]" "$(cat b)"`,
		Why:     "a>b is three tokens; a lexer that splits on whitespace is wrong before it starts",
	},
	{
		ID: "token/io-number-is-not-a-word", Category: "tokenization",
		Snippet: `echo 1>b; printf "[%s]" "$(cat b)"`,
		Why:     "a digit immediately before a redirect is a file descriptor, so echo gets no argument",
	},
	{
		ID: "token/io-number-needs-adjacency", Category: "tokenization",
		Snippet: `echo 1 >b; printf "[%s]" "$(cat b)"`,
		Why:     "one space and the same digit is an argument instead; the pair is the whole rule",
	},
	{
		ID: "token/longest-match-append", Category: "tokenization",
		Snippet: `echo x>b; echo y>>b; printf "[%s]" "$(tr '\n' ',' < b)"`,
		Why:     ">> is one operator, not two; longest match decides",
	},
	{
		ID: "token/comment-needs-word-boundary", Category: "tokenization",
		Snippet: `echo a#b`,
		Why:     "# mid-word is an ordinary character",
	},
	{
		ID: "token/comment-at-word-boundary", Category: "tokenization",
		Snippet: `echo a #b`,
		Why:     "and starts a comment where a word could begin",
	},
	{
		ID: "token/reserved-word-is-positional", Category: "tokenization",
		Snippet: `echo if then done`,
		Why:     "keywords are keywords only where a command name is expected; the lexer cannot classify them alone",
	},
	{
		ID: "token/line-continuation-joins-a-word", Category: "tokenization",
		Snippet: "printf \"[%s]\" ab\\\ncd",
		Why:     "backslash-newline is removed before tokens form, so it can split a word anywhere",
	},
	{
		ID: "token/heredoc-unquoted-delimiter-expands", Category: "tokenization",
		Snippet: "x=VAL; cat <<EOF\n[$x]\nEOF",
		Why:     "an unquoted delimiter means the body is expanded",
	},
	{
		ID: "token/heredoc-quoted-delimiter-literal", Category: "tokenization",
		Snippet: "x=VAL; cat <<\"EOF\"\n[$x]\nEOF",
		Why:     "quoting anywhere in the delimiter makes the whole body literal; the quoting must survive onto the token",
	},
	{
		ID: "token/heredoc-backslash-delimiter-literal", Category: "tokenization",
		Snippet: "x=VAL; cat <<\\EOF\n[$x]\nEOF",
		Why:     "and a backslash counts as quoting the delimiter, same as quotes",
	},
	{
		ID: "token/ampersand-redirect-means-two-things", Category: "tokenization",
		Snippet: `echo hi &>b; wait; printf "[%s]" "$(cat b)"`,
		// `wait` rather than a sleep: where &> is not an operator this really
		// does background a command, and timing its output would be a race
		// recorded into the golden file.
		Why: "the dangerous case: &> redirects both streams in bash and zsh, and is `&` then `>` in dash and ksh93 — no error, different meaning",
	},
	{
		ID: "token/dollar-double-is-a-translatable-string", Category: "tokenization",
		Snippet: `printf "[%s]" $"hello"`,
		Why: "with no message catalog bash and ksh93 strip the `$` and read a plain double-quoted string; " +
			"dash and zsh keep the `$` as a literal — no error, an extra byte, the &> failure mode again",
	},
	{
		ID: "token/dollar-double-expands-inside", Category: "tokenization",
		Snippet: `x=world; printf "[%s]" $"hi $x"`,
		Why: "the translatable string is double quotes in every detail: expansions inside happen, " +
			"and the quoting holds the result together as one field",
	},
	// --- the parameters a shell provides ----------------------------------
	{
		ID: "special/ifs-has-a-default", Category: "parameters",
		Snippet: `printf '%s' "$IFS" | od -An -c | tr -s " "`,
		Why:     "space, tab and newline in three of them and a NUL as well in zsh — and read as bytes because whitespace is what it is made of. Splitting worked here while `$IFS` was empty, so a script could neither read it nor tell it had been changed",
	},
	{
		ID: "special/underscore-follows-the-last-argument", Category: "parameters",
		Snippet: `echo one two >/dev/null; echo "[$_]"; x=5; echo "[$_]"`,
		Why:     "bash and zsh move $_ to the previous command's last argument and to empty after a bare assignment; dash and ksh93 leave it at the shell's own path forever",
	},
	{
		ID: "special/lineno-is-where-you-are", Category: "parameters",
		Snippet: `echo "$LINENO"; echo "$LINENO"`,
		Why:     "produced when it is read rather than stored, which is the whole of the distinction: a stored copy would be the line the shell started on",
	},
	{
		ID: "special/random-is-absent-from-dash", Category: "parameters",
		Snippet: `[ -n "${RANDOM-}" ] && echo have || echo none`,
		Why:     "which parameters a shell provides is the same kind of question as which builtins it has — dash has neither RANDOM nor SECONDS, and the value cannot be recorded because it is a different number every time",
	},
	{
		ID: "special/uid-is-bash-and-zsh", Category: "parameters",
		Snippet: `[ -n "${UID-}" ] && echo have || echo none`,
		Why:     "the parameter a real system script began with — `if [ $UID -ne 0 ]` is `[ -ne 0 ]` where it is unset, which is not the same test and does not fail the same way",
	},
	{
		ID: "special/assigning-random-seeds-it", Category: "parameters",
		Snippet: `[ -n "${RANDOM-}" ] || { echo none; exit; }; RANDOM=5; a=$RANDOM; b=$RANDOM; [ "$a" = 5 ] && echo stored || echo produced`,
		Why:     "assigning a produced parameter is a message to whatever produces it rather than a replacement for it: the next read is a new number and not the 5",
	},

	// --- the core language, as documented ---------------------------------
	{
		ID: "core/dollar-single-expands-escapes", Category: "quoting",
		Snippet: `printf '[%s]' $'a\tb' | od -An -c | tr -s " "`,
		Why:     "read as bytes, because the failure mode was a literal backslash-t that looks almost right in a terminal — the quoting was recorded and nothing decoded it",
	},
	{
		ID: "core/dollar-single-control-character", Category: "quoting",
		Snippet: `printf '[%s]' $'\cA\cz' | od -An -c | tr -s " "`,
		Why:     "the escape that reached the output as a backslash, a c and a letter: bash and ksh93 decode it, zsh is the one shell with $'…' and no \\c in it, and dash has no $'…' at all — four columns and three different answers to one snippet",
	},
	{
		ID: "core/dollar-single-control-arithmetic", Category: "quoting",
		Snippet: `printf '[%s]' $'\c1\c?\c[' | od -An -c | tr -s " "`,
		Why:     "where the two decoding rules part: both uppercase and then agree over @ through _, so a case built only from letters cannot tell masking the low five bits from toggling bit 6 — `\\c1` is 0x11 in bash and `q` in ksh93, and `\\c?` is where bash 3.2 differs from bash 5.3",
	},
	{
		ID: "core/dollar-single-nul-truncates", Category: "quoting",
		Snippet: `x=$'a\0b'; printf '[%s][%s]' "$x" "${#x}" | od -An -c | tr -s " "`,
		Why:     "a decoded NUL ends the text where a shell holds words as C strings and is an ordinary byte where it counts them — the length is recorded beside the bytes because a truncated word and a word with a NUL in it print the same way anywhere the NUL is invisible",
	},
	{
		ID: "core/dollar-single-nul-ends-the-span-only", Category: "quoting",
		Snippet: `printf '[%s]' $'a\0b'ccc | od -An -c | tr -s " "`,
		Why:     "what the NUL truncates is the quoted span and not the word: the characters after the closing quote were never inside it, so `accc` rather than `a` — the distinction a case built from a bare $'…' cannot make",
	},
	{
		ID: "core/dollar-single-esc-escape", Category: "quoting",
		Snippet: `printf '[%s]' $'\e[m\E' | od -An -c | tr -s " "`,
		Why:     "both spellings of the escape character, which POSIX has for neither and every shell with $'…' decodes — the escape that makes a color sequence writable without a literal control character in the source",
	},
	{
		ID: "core/dollar-single-hex-escape", Category: "quoting",
		Snippet: `printf '[%s]' $'\x41\x4a\x9' | od -An -c | tr -s " "`,
		Why:     "one and two hex digits both, because the length is not fixed and a reader that demands two would silently take the `\\x9` of `\\x9Z` as 0x9Z",
	},
	{
		ID: "core/dollar-single-octal-escape", Category: "quoting",
		Snippet: `printf '[%s]' $'\101\0101\1' | od -An -c | tr -s " "`,
		Why:     "the octal forms, and the reason they are one rule rather than two: `\\0101` is not four digits after a zero but three from the zero onward, so it is a backspace and then a `1`",
	},
	{
		ID: "core/dollar-single-unicode-escape", Category: "quoting",
		Snippet: `printf '[%s]' $'\u41\u0041\U00000058' | od -An -c | tr -s " "`,
		Why:     "the code-point escapes, both widths and fewer digits than either allows — and the one place in this table where bash 3.2 keeps the text as written instead, which is what makes the pair a dated answer rather than a disputed one; the code points stay inside ASCII because what a shell does with a wider one depends on the locale and this record is made under LC_ALL=C",
	},
	{
		ID: "core/dollar-single-unknown-escape", Category: "quoting",
		Snippet: `printf '[%s]' $'\q\8' | od -An -c | tr -s " "`,
		Why:     "a backslash before a character no escape claims: bash keeps both, ksh93 and zsh drop the backslash — the axis `\\c` falls to in the one shell that has no `\\c`",
	},
	{
		ID: "core/append-assignment", Category: "parameters",
		Snippet: `x=a; x+=b; echo "[$x]"`,
		Why:     "dash has no += and reads the whole word as a command name, which is the divergence — the other three append",
	},
	{
		ID: "core/append-to-an-array", Category: "parameters",
		Snippet: `a=(one two); a+=(three); echo "[${a[*]}] ${#a[@]}"`,
		Why:     "appending to an array adds to its end rather than to its first element, which is the same spelling doing a different thing",
	},
	{
		ID: "core/array-star-joins", Category: "parameters",
		Snippet: `a=(one two); echo "[${a[*]}]"; IFS=-; echo "[${a[*]}]"`,
		Why:     "`[*]` is one field with the elements joined by the first character of IFS where `[@]` is one field each — the same difference `$*` has from `$@`",
	},
	{
		ID: "core/c-style-for", Category: "command language",
		Snippet: `for ((i=0;i<3;i++)); do printf "%s" "$i"; done; echo`,
		Why:     "a loop on a condition rather than over a list; dash does not have it and says so about the loop variable rather than about the parenthesis",
	},
	{
		ID: "core/c-style-for-with-a-brace-body", Category: "command language",
		Snippet: `for ((i=0;i<3;i++)) { printf "%s" "$i"; }; echo`,
		Why:     "a brace group may stand where `do … done` stands, and every shell that has the C-style form at all accepts it — so it is a production of this construct rather than a dialect's addition. Unanimous, which is why it is core; dash has no C-style loop to give a body to and says so about the loop variable, exactly as it does for `core/c-style-for`",
	},
	{
		ID: "core/c-style-for-brace-body-after-a-separator", Category: "command language",
		Snippet: `for ((i=0;i<2;i++)); { printf "%s" "$i"; }; echo`,
		Why:     "the terminator between the header and the body is optional before the brace exactly as it is before `do`, which is what says the brace stands where `do` stands rather than being glued to the header",
	},
	{
		ID: "core/c-style-for-takes-a-redirection", Category: "command language",
		Snippet: `for ((i=0;i<2;i++)); do echo "$i"; done > f; echo end; cat f`,
		Why:     "a redirection after a compound command covers the whole of it, and the C-style loop is no exception — the quiet failure is that a node with nowhere to keep one leaves the operator standing as a statement of its own, which truncates the file, redirects nothing, and says not a word. dash has no C-style loop and refuses the header",
	},
	{
		ID: "core/c-style-for-with-a-brace-body-takes-a-redirection", Category: "command language",
		Snippet: `for ((i=0;i<2;i++)) { echo "$i"; } > f; echo end; cat f`,
		Why:     "the same on the brace-bodied spelling, which is where the suffix is easiest to lose: the body ends in a `}` rather than a keyword and the redirection reads as the brace group's",
	},
	{
		ID: "core/c-style-for-takes-an-input-redirection", Category: "command language",
		Snippet: `printf 'L1\nL2\n' > d; for ((i=0;i<2;i++)); do read x; echo "got=$x"; done < d`,
		Why:     "the reading half, and the one that shows the redirection outlives an iteration: the second `read` continues where the first left off, which it could not do if the file were opened per pass",
	},
	{
		ID: "core/c-style-for-with-an-empty-header", Category: "command language",
		Snippet: `i=0; for ((;;)); do i=$((i+1)); if [ $i -ge 3 ]; then break; fi; done; echo "i=$i"`,
		Why:     "every section omitted, which is how the endless loop is spelled: an absent condition is *true* rather than the expression 0, so the body runs until something breaks out of it. The distinction is one an AST can lose — a parser that fills a missing section in with a zero writes a loop that never runs a single pass. dash has no C-style loop and blames the loop variable",
	},
	{
		ID: "core/c-style-for-without-a-condition", Category: "command language",
		Snippet: `for ((i=0;;i++)); do if [ $i -ge 3 ]; then break; fi; done; echo "i=$i"`,
		Why:     "the same absent condition with the other two sections present, which is what says the sections are independent rather than all-or-nothing: the initialization and the step still run, and `i` is 3 at the break",
	},
	{
		ID: "core/c-style-for-with-only-a-condition", Category: "command language",
		Snippet: `i=0; for ((;i<3;)); do i=$((i+1)); done; echo "i=$i"`,
		Why:     "a `while` loop written as a `for`: the initialization and the step are the ones omitted here, and an omitted step must not be run as an expression whose value could end the loop. The trio with the two cases above covers every section this header can leave out",
	},
	{
		ID: "core/a-list-for-with-a-brace-body", Category: "command language",
		Snippet: `for i in a b; { printf "%s" "$i"; }; echo`,
		Why:     "the same production on the ordinary `for`, which is the half easiest to miss: the brace body is not the C-style loop's alone. It needs the separator, and the next case says why — this is the one three of the four accept and dash refuses, dash being the only panel shell without the form",
	},
	{
		ID: "core/a-list-for-brace-body-needs-a-separator", Category: "command language", SyntaxError: true,
		Snippet: `for i in a b { echo "$i"; }`,
		Why:     "the same line without the `;`, refused by all four — and not because the brace body is refused there. With nothing between, `{` is another *item* of the list, so the loop reads on and meets `}` where `do` belongs, which is what every one of the four then names. The C-style header takes the brace with nothing between because `))` has already ended it",
	},
	{
		ID: "core/a-brace-body-is-not-a-while-body", Category: "command language", SyntaxError: true,
		Snippet: `while true; { echo hi; break; }`,
		Why:     "the brace-body production belongs to the loops built on a `for` header and to nothing else. Written with the separator, so that it is the same shape the list `for` accepts and the difference is the construct rather than the punctuation. Three refuse it; the fourth prints hi, and *not* because it read a body — the `;` keeps the condition list going, so the brace group is the last thing tested and the `break` inside it is what ends the loop. `core/a-tested-brace-group-is-not-a-body` is the same shape with a counter, which counts up there rather than stopping",
	},
	{
		ID: "core/a-tested-brace-group-is-not-a-body", Category: "command language", SyntaxError: true,
		Snippet: `i=0; while [ $i -lt 2 ]; { echo $i; i=$((i+1)); [ $i -lt 4 ]; }`,
		Why:     "which half of `while cond; { … }` the brace group is, in the one shell that takes the line at all. As a *body* the loop would print 0 and 1 and stop on the header's test; as the tail of the condition list it prints 0..3 and stops on the test written inside the braces, and that is what happens — so the shell has no `while` brace body, it has an *omitted* one with the whole line as the condition. The counter and the inner test are what tell the readings apart; the shape alone cannot, which is why `core/a-brace-body-is-not-a-while-body` was read the other way for a while. Nothing may follow the group, either: a `; echo end` after it joins the condition list too, and its status is then what the loop tests, so the loop never ends. Three shells refuse the line outright",
	},
	{
		ID: "core/a-while-condition-that-ends-itself-takes-a-body", Category: "command language", SyntaxError: true,
		Snippet: `i=0; while (( i < 2 )) { echo $i; i=$((i+1)); }; echo end`,
		Why:     "the short loop proper, and the same text as the row above with the separator taken *out*. Without one the condition list cannot continue, so the brace group is the body and the loop stops at 2 — which is the opposite of what the separator gives. `(( … ))` is what ends the header; `while true { … }` is a syntax error in the same shell, because a word cannot end one",
	},
	{
		ID: "core/a-short-loop-body-is-one-command", Category: "command language", SyntaxError: true,
		Snippet: `i=0; while (( i < 2 )) echo $((i++)); echo end`,
		Why:     "the body of a short loop need not be a brace group, and it is exactly one command: the `; echo end` after it is outside the loop, so `end` prints once rather than per iteration. A second command inside would need a separator, and a separator there is the enclosing list's",
	},
	{
		ID: "core/a-for-over-a-parenthesized-list", Category: "command language", SyntaxError: true,
		Snippet: `for i (a b) { echo "$i"; }; for j (p q) echo "$j"`,
		Why:     "the short `for`, in both its body spellings. The parentheses say what `in` says and end the header as `in` does not, which is why this one takes a brace body with nothing between where `for i in a b { … }` cannot. One shell parses it and four call the `(` a syntax error",
	},
	{
		ID: "core/a-for-header-that-ends-itself-needs-no-body", Category: "command language", SyntaxError: true,
		Snippet: `if true; then for i (a b); fi; echo no-body; for j (p q) echo body`,
		Why:     "the body left out of a `for`, which is legal exactly where the header closed itself. Reaching it needs something the body cannot be, because anything that could be one *is* one — hence the `fi`, a word no command may start with. The second loop is the contrast in the same line: the same header with a command after it runs that command per item, so `no-body` prints once and `body` twice. `for i in a b` with nothing after it is a syntax error in the same shell, so this is a property of the header rather than of the loop",
	},
	{
		ID: "core/a-short-loop-redirection-is-the-bodys", Category: "command language", SyntaxError: true,
		Snippet: `for i (a b) > f$i; ls`,
		Why:     "where a short loop's redirection lands, and the loop variable in the target is what makes the answer visible: two files named for the two items mean the redirection ran once per iteration and is the *body* — a command that only redirects. A redirection on the loop is expanded once before it starts and would leave a single `f`, which is what `for i (a b) { echo hi } > f$i` does. The distinction is unreachable from the tree alone, so it is pinned here",
	},
	{
		ID: "core/for-wants-a-name", Category: "command language", SyntaxError: true,
		Snippet: `for 1x in a; do echo; done`,
		Why:     "four wordings for one refusal, and only one of them blames the word rather than saying something about names",
	},

	// --- getopts: the builtin a borrowed program could not have been ------
	{
		ID: "getopts/loop-reads-each-option", Category: "getopts",
		Snippet: `set -- -a -b x; while getopts "ab:" o; do echo "[$o:${OPTARG-}]"; done; echo "ind=$OPTIND"`,
		Why:     "the shape every script uses it in, and the one that reported success while running its body zero times when getopts was a separate program that could not reach the shell's variables",
	},
	{
		ID: "getopts/clustered-options", Category: "getopts",
		Snippet: `set -- -ab; while getopts "ab" o; do printf "[%s]" "$o"; done; echo " ind=$OPTIND"`,
		Why:     "two options in one word, which is why the position inside a word cannot be OPTIND — that counts words",
	},
	{
		ID: "getopts/argument-attached-or-apart", Category: "getopts",
		Snippet: `set -- -bval; getopts "b:" o; echo "[$o][$OPTARG]"; set -- -b val; OPTIND=1; getopts "b:" o; echo "[$o][$OPTARG]"`,
		Why:     "`-bval` and `-b val` are the same option, and resetting OPTIND is how a script starts a second scan",
	},
	{
		ID: "getopts/a-dash-word-where-the-optstring-belongs", Category: "getopts",
		Snippet: `set -- -q b; getopts -q o; echo "st=$? o=[$o]"`,
		Why:     "the GetoptsRejectsUnknownOption axis: bash and ksh93 refuse the word as an option getopts does not have, dash and zsh read it as the optstring and then parse -q by it. Probed with a letter no panel shell owns — ksh93 has -a for real, and a probe written with -a read ksh93's own option as a refusal policy it does not have",
	},
	{
		ID: "getopts/unknown-option-diverges", Category: "getopts",
		Snippet: `set -- -z; getopts "ab" o; echo "st=$? o=[$o]"`,
		Why:     "four wordings, and four different amounts of prefix: bash names itself with no line where it gives a line to everything else, and dash prints neither a name nor a line — the only diagnostic in the panel with nothing in front of it",
	},
	{
		ID: "getopts/silent-mode-reports-through-optarg", Category: "getopts",
		Snippet: `set -- -z; getopts ":ab" o; echo "st=$? o=[$o] arg=[$OPTARG]"`,
		Why:     "a leading colon turns the complaint off and puts the letter in OPTARG instead, which is how a script takes the reporting over — unanimous, unlike the message it replaces",
	},
	{
		ID: "getopts/missing-argument-diverges", Category: "getopts",
		Snippet: `set -- -b; getopts "b:" o; echo "st=$? o=[$o]"`,
		Why:     "the second of the two complaints, worded four ways again",
	},
	{
		ID: "getopts/silent-missing-argument-is-a-colon", Category: "getopts",
		Snippet: `set -- -b; getopts ":b:" o; echo "st=$? o=[$o] arg=[$OPTARG]"`,
		Why:     "`:` rather than `?` in silent mode, which is what lets a script tell a missing argument from an unknown option without reading a sentence",
	},
	{
		ID: "getopts/double-dash-ends-the-options", Category: "getopts",
		Snippet: `set -- -a -- -b; while getopts "ab" o; do printf "[%s]" "$o"; done; echo " ind=$OPTIND"`,
		Why:     "`--` ends them and OPTIND points past it, so what follows is an operand however much it looks like an option",
	},

	// --- cd, and what unset takes away -----------------------------------
	{
		ID: "cd/missing-directory-diverges", Category: "cd",
		Snippet: `cd /nope-xyz-abc; echo "st=$?"`,
		Why:     "four shapes and two statuses for one failure, and dash gives no reason at all — the one shell whose message cannot tell you why",
	},
	{
		ID: "cd/onto-a-file-is-a-different-reason", Category: "cd",
		Snippet: `: > f; cd ./f; echo "st=$?"`,
		Why:     "the case dash cannot express: three of the four say `not a directory` where they said `no such file`, and dash says the same sentence for both",
	},
	{
		ID: "cd/no-home-diverges", Category: "cd",
		Snippet: `unset HOME; cd; echo "st=$?"`,
		Why:     "bash and ksh93 call this an error and dash and zsh stay where they are and report success, which is the quieter answer and the surprising one",
	},
	{
		ID: "cd/dash-announces-where-it-went", Category: "cd",
		// Whether anything was printed rather than what it was: the path
		// itself would measure symlink resolution instead, which is a
		// different divergence and not this one.
		Snippet: `cd /; out=$(cd -); [ -n "$out" ] && echo printed || echo silent`,
		Why:     "`cd -` prints where it went in three of the four; zsh alone moves silently, so a script that pipes it gets an extra line everywhere but there",
	},
	{
		ID: "unset/takes-away-an-environment-name", Category: "parameters",
		Snippet: `unset HOME; echo "[${HOME-gone}]"`,
		Why:     "a name that arrived in the environment rather than from an assignment is still a name `unset` removes — deleting it from the shell's own table is not enough, because a lookup reads both",
	},

	// --- printf: the last builtin that was not one ------------------------
	{
		ID: "printf/assigns-with-v", Category: "printf",
		Snippet: `printf -v out "%05d" 42; echo "[$out]"`,
		Why:     "`printf -v name` puts the formatted text in a variable and prints nothing, which is how a script formats a value without a command substitution and a subshell. bash and zsh have it; dash and ksh93 reject it as an unknown option, and each words that differently",
	},
	{
		ID: "printf/double-dash-ends-the-options", Category: "printf",
		Snippet: `printf -- "x\n"`,
		Why:     "unanimous, and the reason a format that begins with a dash can be written at all — without it `printf -- \"x\\n\"` printed the `--`",
	},
	{
		ID: "printf/a-dash-word-is-an-option", Category: "printf",
		Snippet: `printf -q x; echo "st=$?"`,
		Why:     "three of the four read a leading `-` word as options and refuse one they do not know, each wording it differently; zsh takes it as the format and prints `-q`. Not written with a dash-initial *format* — `printf \"-%s\\n\" x` is refused the same way and by its first letter, `-%`, but ksh93 then goes on through the rest of the bundle and complains about `-s` as well, which is a divergence of its own",
	},
	{
		ID: "printf/a-lone-dash-is-an-operand", Category: "printf",
		Snippet: `printf "%s\n" -`,
		Why:     "a bare `-` is not an option in any of the four, which is what keeps the rule above from swallowing it",
	},
	{
		ID: "printf/format-is-reused", Category: "printf",
		Snippet: `printf "[%s]" a b c; echo`,
		Why:     "the format runs again until the arguments are gone, which is the property that makes printf a loop rather than a formatter",
	},
	{
		ID: "printf/missing-argument-is-empty", Category: "printf",
		Snippet: `printf "[%s][%s]\n" a`,
		Why:     "an argument that is not there is the empty string rather than an error, unanimously — and different from one that is there and empty",
	},
	{
		ID: "printf/b-escapes-and-s-does-not", Category: "printf",
		Snippet: `printf "[%b][%s]\n" "a\tb" "a\tb"`,
		Why:     "the whole reason %b exists: the same argument, escaped by one verb and left alone by the other",
	},
	{
		ID: "printf/bad-number-diverges", Category: "printf",
		Snippet: `printf "[%d]\n" abc; echo "st=$?"`,
		Why:     "bash and dash complain and report failure where ksh93 and zsh say nothing, and all four print the zero — so the complaint sits beside the output rather than instead of it",
	},
	{
		ID: "printf/empty-operand-is-bash-only", Category: "printf",
		Snippet: `printf "[%d]\n" ""; echo "st=$?"`,
		Why:     "an operand that is present and empty is an error in bash alone, where a missing one is an error in none of them",
	},
	{
		ID: "printf/quote-diverges", Category: "printf",
		Snippet: `printf "[%q]\n" "a b"; echo "st=$?"`,
		Why:     "three answers and an absence: bash and zsh backslash-escape, ksh93 single-quotes, and dash has no %q at all",
	},
	{
		ID: "printf/unknown-verb-diverges", Category: "printf",
		Snippet: `printf "[%z]\n" x; echo "st=$?"`,
		Why:     "half the panel names the character *after* the one it could not read and half names the conversion, with four wordings and three statuses between them",
	},
	{
		ID: "printf/no-format-at-all", Category: "printf",
		Snippet: `printf; echo "st=$?"`,
		Why:     "four usages, two of them printed with no shell name in front, and zsh alone not treating it as worth a different status from any other failure",
	},
	{
		ID: "printf/a-date-conversion-and-the-shells-without-one", Category: "printf",
		Snippet: `printf "%(%%)T\n" 1000000000; echo "st=$?"`,
		Why:     "`%(fmt)T` writes an epoch through a date format, and one shell in the panel has it as bash does: the other four meet `%(` with three wordings and two statuses. The format here is a literal `%` on purpose, so that the row is about the conversion being *read* and every column is the same on every run — ksh93's own `%T` takes a date string and answers a number with a warning and the time it is now",
	},
	{
		ID: "printf/a-date-through-a-fixed-epoch", Category: "printf",
		Snippet: `x=$(printf "%(%Y)T" 1000000000 2>/dev/null); echo "st=$?"; case $x in 2001) echo "the year the epoch falls in";; "") echo "no such conversion";; *) echo "some other year";; esac`,
		Why:     "the epoch is fixed and the answer compared rather than printed, for two reasons: the conversion's other operands are `now` and `when this shell started`, neither of which a recorded case could be asked twice — and the one shell with a `%T` of its own writes the current year here, which is a fact that would go stale in January",
	},
	{
		ID: "printf/a-date-with-a-zone-and-a-whole-timestamp", Category: "printf",
		Snippet: `export TZ=UTC; x=$(printf "%(%Y-%m-%dT%H:%M:%S %Z)T" 1000000000 2>/dev/null); case $x in "2001-09-09T01:46:40 UTC") echo "the epoch, in UTC";; "") echo "no such conversion";; *) echo "some other date";; esac`,
		Why:     "a format worth writing, and the zone it is read in: `$TZ` decides, so a case that did not set it would record the zone of whichever machine ran it",
	},
	{
		ID: "printf/a-date-with-an-empty-format-and-a-width", Category: "printf",
		Snippet: `export TZ=UTC; x=$(printf "[%()T][%12(%Y)T]" 1000000000 1100000000 2>/dev/null); case $x in "[01:46:40][        2004]") echo "the time of day, then a padded year";; "") echo "no such conversion";; *) echo "something else";; esac`,
		Why:     "an empty format is the C locale's time of day, and a width belongs to the *result* rather than to the date. Two operands as well, so the format runs twice and the second year is the second epoch's",
	},
	{
		ID: "printf/a-date-from-something-that-is-not-a-number", Category: "printf",
		Snippet: `export TZ=UTC; x=$(printf "%(%Y)T" abc); echo "st=$?"; case $x in 1970) echo "the epoch zero";; "") echo "no such conversion";; *) echo "some other year";; esac`,
		Why:     "an operand that is not an epoch: the shell with the conversion complains and writes the epoch zero anyway, which is what its numeric conversions do with a word that is not a number. The complaint is on standard error and stays in the record",
	},
	{
		ID: "printf/backslash-c-means-three-things", Category: "printf",
		Snippet: `printf "a\cbZ" | od -An -c | tr -s " "`,
		Why:     "read as bytes rather than as text, because that is the only way to tell ksh93's control character from zsh's stopping: bash and dash write two literal characters, ksh93 reads \\cX as control-X, and zsh ends the output there",
	},
	{
		ID: "printf/backslash-c-with-a-digit", Category: "printf",
		Snippet: `printf "a\c1Z" | od -An -c | tr -s " "`,
		Why:     "a letter cannot tell the two control arithmetics apart, because clearing the top bits and toggling bit 6 agree over @ through _. A digit is outside that span and separates them: masking gives 0x11 and toggling gives q, and the shell that reads the escape at all does the second",
	},
	{
		ID: "printf/backslash-c-with-a-symbol-outside-the-letters", Category: "printf",
		Snippet: `printf "a\c~Z:a\c?Z" | od -An -c | tr -s " "`,
		Why:     "the other side of the span, and the DEL that a masking shell would have to special-case: toggling bit 6 makes ~ into > and ? into 0x7f without one",
	},
	{
		ID: "printf/backslash-c-controls-a-decoded-escape", Category: "printf",
		Snippet: `printf "a\c\tZ" | od -An -c | tr -s " "`,
		Why:     "what \\c applies to is read before it is controlled, so the argument here is a tab and not a backslash followed by a t. The same reading the quoted-escape form does, which is the point: one rule, and a format is not a second dialect of it",
	},
	{
		ID: "printf/backslash-c-at-the-end-of-a-format", Category: "printf",
		Snippet: `printf 'a\c\' | od -An -c | tr -s " "`,
		Why:     "a backslash with nothing after it escapes the end of the format, so what \\c controls is a NUL rather than the backslash itself — an @ and not a control-backslash for the shell that reads the escape",
	},

	// --- kill: a builtin, because a shell has to know what it sent ---------
	{
		ID: "kill/probe-with-signal-zero", Category: "kill",
		Snippet: `kill -0 $$; echo "st=$?"`,
		Why:     "signal 0 is not a signal: it asks whether the process is there, and every shell in the panel answers 0 for one that is",
	},
	{
		ID: "kill/no-such-process", Category: "kill",
		Snippet: `kill 999999; echo "st=$?"`,
		Why:     "status 1 in all four and four different sentences, one of which does not name the process it could not find",
	},
	{
		ID: "kill/not-a-pid", Category: "kill",
		Snippet: `kill abc; echo "st=$?"`,
		Why:     "an operand that is not a number is a complaint about the argument rather than a target that failed, which is why dash reports 2 here and 1 for a process that is not there",
	},
	{
		ID: "kill/no-operands-is-usage", Category: "kill",
		Snippet: `kill; echo "st=$?"`,
		Why:     "the only diagnostic in the panel that two shells print with no location in front of it, and zsh is alone in not treating it as worth a different status from any other failure",
	},
	{
		ID: "kill/unknown-signal-as-a-flag", Category: "kill",
		Snippet: `kill -Q 1; echo "st=$?"`,
		Why:     "half the panel answers `what did you just give me` by where it appeared: dash and ksh93 call this an unknown option where `-s Q` is an unknown signal, and ksh93 prints its usage after one and not the other",
	},
	{
		ID: "kill/unknown-signal-after-s", Category: "kill",
		Snippet: `kill -s Q 1; echo "st=$?"`,
		Why:     "the same signal spelled the other way, which is where the two wordings and the two statuses come apart",
	},
	{
		ID: "kill/list-a-number", Category: "kill",
		Snippet: `kill -l 9`,
		Why:     "unanimous, and the only part of `kill -l` that is: the bare listing is four formats over a table that is not the same on two operating systems",
	},
	{
		ID: "kill/list-a-name", Category: "kill",
		Snippet: `kill -l INT; echo "st=$?"`,
		Why:     "dash's `-l` takes an exit status rather than a signal, so a name is an illegal number there and the number 2 everywhere else",
	},
	{
		ID: "kill/several-targets-disagreeing", Category: "kill",
		Snippet: `kill -0 $$ 999999; echo "st=$?"`,
		Why:     "three answers to one question: bash reports success because it signaled something, dash and ksh93 failure because something failed, and zsh how many failed",
	},
	{
		ID: "kill/every-target-failing", Category: "kill",
		Snippet: `kill 999998 999999; echo "st=$?"`,
		Why:     "the same axis read again, and the case that shows zsh's status is a count rather than a verdict: two dead targets is 2",
	},
	{
		ID: "kill/signaling-a-process-that-is-not-ours", Category: "kill",
		Snippet: `kill -TERM 1; echo "st=$?"`,
		Why:     "pid 1 exists and will not take a signal from us, which is the other half of the target failure and worded differently again",
	},
	{
		ID: "kill/exit-trap-after-a-fatal-signal", Category: "traps and exit",
		Script:  true,
		Snippet: "trap 'echo bye' EXIT\nkill -INT $$\necho after\n",
		Why:     "whether being killed counts as exiting: bash and ksh93 run the EXIT trap and dash and zsh do not, and all four report 130 without reaching the next command",
	},
	{
		ID: "trap/bad-signal-name", Category: "traps and exit",
		Snippet: `trap 'echo x' NOPE; echo "st=$?"`,
		Why:     "status 1 in all four and four different sentences, one of which arrives with no shell name in front of it where the same shell prefixes every `kill` diagnostic it has",
	},
	{
		ID: "trap/sig-prefix-diverges", Category: "traps and exit",
		Snippet: `trap 'echo caught' SIGUSR1; echo "st=$?"`,
		Why:     "dash reads no SIG-prefixed name: the prefix is simply not part of a signal's name there, so a script that traps SIGUSR1 traps nothing and says so, where the other three take it",
	},
	{
		ID: "kill/sig-prefix-as-a-flag", Category: "kill",
		Snippet: `kill -SIGCONT $$; echo "st=$?"`,
		Why:     "the same refusal reached the other way, and dash names only the first character of what it could not read — it stopped there",
	},
	{
		ID: "kill/sig-prefix-after-s", Category: "kill",
		Snippet: `kill -s SIGCONT $$; echo "st=$?"`,
		Why:     "and once more with the POSIX spelling, where dash calls the same word an invalid signal rather than an illegal option: one refusal, three wordings from the shell that refuses",
	},
	{
		ID: "trap/signal-handler-runs-and-continues", Category: "traps and exit",
		Script:  true,
		Snippet: "trap 'echo caught' INT\nkill -INT $$\necho after\n",
		Why:     "a caught signal runs its handler and the script carries on, which is the whole reason to catch one",
	},
	{
		ID: "trap/empty-handler-ignores", Category: "traps and exit",
		Script:  true,
		Snippet: "trap '' INT\nkill -INT $$\necho after\n",
		Why:     "an empty handler ignores the signal, which is different from having no trap at all",
	},
	{
		ID: "trap/default-signal-terminates", Category: "traps and exit",
		Script:  true,
		Snippet: "kill -INT $$\necho after\n",
		Why:     "untrapped, INT kills the shell and the status is 128 plus the number",
	},
	{
		ID: "trap/reset-restores-the-default", Category: "traps and exit",
		Script:  true,
		Snippet: "trap 'echo caught' INT\ntrap - INT\nkill -INT $$\necho after\n",
		Why:     "`trap -` puts the default back rather than leaving an empty handler",
	},
	{
		ID: "signal-death/status-encodes-the-signal", Category: "traps and exit",
		Snippet: `sh -c 'kill -PIPE $$'; echo "st=$?"; echo after`,
		Why:     "a command killed by a signal has no exit status of its own, so the signal goes in the number: 128 + 13 in bash, dash and zsh, and 256 + 13 in ksh93. PIPE is the signal to ask with — it is one of only two the panel does not announce (INT is the other), so the case is about the number and not about three wordings, and unlike INT it does not end the script in ksh93",
	},
	{
		ID: "signal-death/the-shell-dies-by-the-signal-rather-than-exiting", Category: "traps and exit",
		Snippet: `kill -TERM $$; echo after`,
		Why:     "the discipline the record could not see until it kept the signal: a shell with no trap for a fatal signal does not exit with 128 plus the number, it *re-raises the signal at itself*, so its own caller is told the shell was killed and by which one. `sh -c 'kill -TERM $$'; echo $?` says 143 either way, which is why every earlier case had to nest a child and read the parent shell's arithmetic instead of the death",
	},
	{
		ID: "signal-death/an-uncatchable-signal-ends-it-outright", Category: "traps and exit",
		Snippet: `kill -KILL $$; echo after`,
		Why:     "the same death by a signal no shell can trap, handle or re-raise deliberately — the kernel ends it — which is what says the row above is the shell's own discipline and not simply what happens to a process that is signaled",
	},
	{
		ID: "signal-death/a-signal-with-no-meaning-of-its-own-is-fatal-too", Category: "traps and exit",
		Snippet: `kill -USR1 $$; echo after`,
		Why:     "the same discipline for a signal the shell is not expected to have an opinion about: USR1 has no default meaning beyond ending the process, and every shell in the panel is killed by it. It is here because ours was not — Go's runtime forwards a signal it classifies as killing and silently discards the rest, so USR1, USR2, ALRM, PIPE, XCPU, XFSZ, VTALRM and PROF raised at ourselves did nothing and the shell hung waiting for a death that was never coming, at ten seconds of harness timeout each",
	},
	{
		ID: "signal-death/dying-by-a-signal-says-nothing", Category: "traps and exit",
		Snippet: `kill -ABRT $$; echo after`,
		Why:     "a shell killed by a signal writes no diagnostic of its own — both streams are empty in all four — and ABRT is the sharp way to ask, because it is one of the signals Go's runtime treats as a crash: it printed a full goroutine dump to standard error and exited 2 where a shell must print nothing at all and be killed by the signal. The same leak internal/panicguard exists to stop, arriving by another road",
	},
	{
		ID: "signal-death/quit-is-not-fatal-in-every-shell", Category: "traps and exit",
		Snippet: `kill -QUIT $$; echo after`,
		Why:     "the one fatal signal the panel disagrees about: bash 5.3 and zsh take QUIT's default action away and print after with status 0, where dash, ksh93 — and bash 3.2, so the two bash columns differ — are killed by it. Measured with a signal from another process too, so it is a disposition rather than a deferral, and it disappears with `-i`, where all five ignore it",
	},
	{
		ID: "signal-death/hangup-is-an-exit-in-one-shell", Category: "traps and exit",
		Snippet: `kill -HUP $$; echo after`,
		Why:     "the second fatal signal the panel disagrees about, and the only other one: zsh reports 1 where bash, dash and ksh93 are killed by SIGHUP and report 129. Nothing prints after it anywhere, so the disagreement is about how the shell ended rather than about whether it did — and 1 is not 128 plus anything, which is the first sign that zsh is exiting rather than dying. Measured across all nineteen signals whose default action ends a process: this and QUIT are the whole of the split",
	},
	{
		ID: "signal-death/a-hangup-that-exits-runs-the-exit-trap", Category: "traps and exit",
		Snippet: `trap 'echo bye' EXIT; kill -HUP $$; echo after`,
		Why:     "what says the row above is an exit and not merely a different number. zsh does not run the EXIT trap when a signal kills it — `trap 'echo bye' EXIT; kill -TERM $$` prints nothing there — and it prints bye here, so SIGHUP produced no death for that question to be asked about. bash and ksh93 print bye because dying counts as exiting for them, and dash prints nothing for either signal, which is why the trap alone cannot tell the two apart and the status beside it can",
	},
	{
		ID: "signal-death/a-handled-signal-is-not-a-death", Category: "traps and exit",
		Snippet: `trap "echo caught" TERM; kill -TERM $$; echo after`,
		Why:     "the control: the same signal with a trap for it runs the handler and the shell carries on to exit normally, so the two rows above are about the *absence* of a handler rather than about the signal arriving",
	},
	{
		ID: "signal-death/an-ordinary-failure-is-untouched", Category: "traps and exit",
		Snippet: `sh -c 'exit 3'; echo "st=$?"`,
		Why:     "a command that exits by itself reports what it exited with, unanimously — which is what says the encoding above is about being killed rather than about failing",
	},
	{
		ID: "umask/reads-the-mask", Category: "traps and exit",
		Snippet: `umask`,
		Why:     "three of the four write four octal digits and zsh writes three — `0022` against `022`. The value itself is the machine's, so this pins the shape rather than the number, and the harness runs every case with the same mask",
	},
	{
		ID: "umask/symbolic-is-unanimous", Category: "traps and exit",
		Snippet: `umask -S`,
		Why:     "`-S` writes the permissions the mask *allows* rather than the bits it takes away, and all four spell it identically — the one part of this builtin needing no dialect",
	},
	{
		ID: "umask/setting-then-reading", Category: "traps and exit",
		Snippet: `umask 077; umask; umask -S`,
		Why:     "the mask a script sets is the mask it reads back, which is the whole point of the builtin and exactly what a `/usr/bin/umask` in a child process cannot do",
	},
	{
		ID: "umask/setting-is-silent", Category: "traps and exit",
		Snippet: `umask 077; echo "st=$?"`,
		Why:     "setting writes nothing in any of the four — so a script can set a mask without its output changing, and the `-S` form below is the exception rather than the rule",
	},
	{
		ID: "umask/dash-s-with-a-mask-echoes-in-bash", Category: "traps and exit",
		Snippet: `umask -S 077; umask 022`,
		Why:     "bash alone echoes the new mask symbolically when asked to set *and* shown `-S`; dash, ksh93 and zsh set it and say nothing. The trailing `umask 022` puts the machine back so the case leaves nothing behind",
	},
	{
		ID: "umask/a-mask-it-cannot-read", Category: "traps and exit",
		Snippet: `umask 9999; echo "st=$?"`,
		Why:     "four wordings and two statuses — 1 in bash, ksh93 and zsh, 2 in dash — and zsh names no operand at all, saying only `bad umask`",
	},
	{
		ID: "let/evaluates-and-assigns", Category: "arithmetic",
		Snippet: `let "x = 2 + 3"; echo "$x"`,
		Why:     "`let` is `(( ))` with the expression as a word rather than inside parentheses. bash, ksh93 and zsh have it; dash has only `$(( ))` and reports it as a command it never heard of",
	},
	{
		ID: "let/zero-is-a-failure", Category: "arithmetic",
		Snippet: `let "x=5"; echo "a=$?"; let "x=0"; echo "b=$?"`,
		Why:     "the surprising part, and unanimous among the three that have it: an expression coming out *zero* reports 1, because a shell reports false for it — so `let` cannot be used to assign 0 without the caller expecting a failure",
	},
	{
		ID: "let/the-last-expression-decides", Category: "arithmetic",
		Snippet: `let a=0 b=1; echo "st=$? a=$a b=$b"`,
		Why:     "every argument is evaluated — they have side effects — and only the last one decides the status, so a leading zero does not make the whole thing fail",
	},
	{
		ID: "let/with-nothing-to-evaluate", Category: "arithmetic",
		Snippet: `let; echo "st=$?"`,
		Why:     "three wordings and two statuses: bash and zsh report 1, ksh93 reports 2 and prints a bare usage line with no shell name in front of it",
	},
	{
		ID: "ulimit/reads-the-file-size-limit", Category: "traps and exit",
		Snippet: `ulimit; ulimit -f`,
		Why:     "bare `ulimit` is `-f`, which is why it reports the file-size limit rather than a summary — unanimous, and the reason a script that means something else has to say which",
	},
	{
		ID: "ulimit/hard-and-soft", Category: "traps and exit",
		Snippet: `ulimit -Ht; ulimit -St; ulimit -t`,
		Why:     "`-H` and `-S` choose which of the two limits is read, and neither means the soft one — so the third line repeats the second. CPU time rather than open files: the file-descriptor limit is the one resource whose value differs between our process and bash's, for reasons outside either shell",
	},
	{
		ID: "ulimit/unlimited-is-a-word", Category: "traps and exit",
		Snippet: `ulimit -Hf`,
		Why:     "no limit is printed as `unlimited` rather than as a very large number, in all four — and is read back from that word too, which is what lets a script save and restore one",
	},
	{
		ID: "ulimit/setting-then-reading", Category: "traps and exit",
		Snippet: `ulimit -t 3600; ulimit -t; ulimit -Ht`,
		Why:     "setting without -H or -S lowers both, which is what makes it irreversible — the hard limit follows the soft one down and cannot be raised again",
	},
	{
		ID: "ulimit/a-limit-it-cannot-read", Category: "traps and exit",
		Snippet: `ulimit -t abc; echo "st=$?"`,
		Why:     "four wordings, and ksh93's is the odd one — `parameter not set` where the others call it a bad or invalid number",
	},
	{
		ID: "ulimit/a-letter-zsh-does-not-have", Category: "traps and exit",
		Snippet: `ulimit -m >/dev/null; echo "st=$?"`,
		Why:     "the resident-set letter, read by bash, dash and ksh93 and a bad option in zsh — the UlimitHasResidentSet axis. The value goes to /dev/null because it is the machine's, not the shell's; the letter's existence is the question",
	},
	{
		ID: "ulimit/a-letter-dash-does-not-have", Category: "traps and exit",
		Snippet: `ulimit -u >/dev/null; echo "st=$?"`,
		Why:     "the process-count letter, read by bash, ksh93 and zsh where dash refuses it at 2 and carries on — the UlimitHasProcessCount axis, and the other half of the pair above: neither absence is a subset of the other, which is what makes them two axes",
	},
	{
		ID: "builtin/an-option-it-does-not-have", Category: "traps and exit",
		Snippet: `export -Q x; echo "st=$?"; echo after`,
		Why:     "four wordings, two statuses and a divergence about whether the script survives: bash and zsh report it and carry on — with 2 and 1 — while dash and ksh93 stop there, which is the POSIX rule that a special builtin's failure is fatal. bash and ksh93 print a usage line after it and word that per builtin",
	},
	{
		ID: "builtin/the-same-refusal-for-another-builtin", Category: "traps and exit",
		Snippet: `unset -Q x; echo "st=$?"; echo after`,
		Why:     "the same shape from a different builtin, which is what says the wording is one rule rather than one per name — only the usage line changes, and only in the two that print one",
	},
	{
		ID: "builtin/an-option-it-does-have", Category: "traps and exit",
		Snippet: `x=1; export x; unset -v x; echo "[${x-unset}]"`,
		Why:     "the options each of them really has still work, which is the half a refusal could break — and `--` and a bare name have to keep meaning what they did",
	},
	{
		ID: "trap/numeric-signal-name", Category: "traps and exit",
		Script:  true,
		Snippet: "trap 'echo caught' 2\nkill -INT $$\necho after\n",
		Why:     "a signal can be named by number, and 2 is INT everywhere the panel runs",
	},
	{
		ID: "trap/numeric-signal-beyond-the-common-few", Category: "traps and exit",
		Script:  true,
		Snippet: "trap 'echo caught' 5\nkill -TRAP $$\necho after\n",
		Why:     "every signal has a number, not just the handful a script usually names. 5 is TRAP on every platform the panel runs on, and /usr/bin/bzless on macOS traps 0 2 3 5 10 13 15 — a script written against a shell that takes them all",
	},
	{
		ID: "trap/number-and-name-are-one-trap", Category: "traps and exit",
		Script:  true,
		Snippet: "trap 'echo one' 5\ntrap 'echo two' TRAP\nkill -TRAP $$\necho after\n",
		Why:     "the two spellings are the same signal rather than two entries, so the second setting replaces the first and only `two` runs",
	},
	{
		ID: "trap/signal-handler-status-diverges", Category: "traps and exit",
		Script:  true,
		Snippet: "trap 'echo st=$?' INT\nfalse\nkill -INT $$\necho after\n",
		Why:     "zsh shows the handler the status from before the command that triggered it; the other three show that command's own",
	},
	// The three cases below share one shape, and the trailing `sleep 0.2`
	// inside the background subshell is load-bearing in all three.
	//
	// Without it, `kill -USR1 $$` is the subshell's last command, so the
	// signal and the subshell's own death reach the parent at effectively the
	// same instant — and a `wait` woken by SIGCHLD first reaps the job and
	// reports 0, where a `wait` woken by SIGUSR1 reports the signal. Both
	// outcomes were reachable and the case graded whichever the kernel picked:
	// measured over 200 runs each, zsh answered `st=0` twice on the bare form
	// and twice on the `$!` form, and dash twice in 60. That is a wandering
	// conformance score in *both* directions, since the same coin flip fails
	// the dialect expecting one answer and passes the one expecting the other.
	//
	// Lingering after the kill makes the separation a fact rather than a
	// scheduling accident: the job is still alive when the signal lands, so
	// `wait` can only be leaving early because it was interrupted. 300 runs
	// per form under zsh, and 60 under each of dash, bash 5, bash 3.2 and
	// ksh93, agree unanimously — and every recorded answer is the one the
	// racy form gave when it won the race, so this pins the measurement down
	// rather than changing what is measured.
	//
	// It is not `ReferenceRaces`: that flag is for a shell that genuinely
	// races with itself, and here it was the *probe* that left the race open.
	// A marked row would have stopped grading three cases about the one
	// delivery path no other case covers.
	{
		ID: "trap/wait-cut-short-by-a-signal", Category: "traps and exit",
		Snippet: `trap 'echo T' USR1; (sleep 0.3; kill -USR1 $$; sleep 0.2) & wait; echo "st=$?"`,
		Why:     "the async delivery path, which every other trap case misses: the signal comes from a background job rather than from the shell's own line, and it has to reach a `wait` that is already blocked. The handler runs and then `wait` reports the signal — 128 + USR1 in bash, dash and zsh, 256 + USR1 in ksh93 — where a wait nobody interrupted reports 0. `wait; check $?` is the supervisor loop every job-runner script is built on, so a 0 here is silence in place of the whole point. The job outlives the signal it sends, so `wait` is still blocked when the signal lands rather than racing the job's own death",
	},
	{
		ID: "trap/wait-for-a-job-cut-short-by-a-signal", Category: "traps and exit",
		Snippet: `trap 'echo T' USR1; (sleep 0.3; kill -USR1 $$; sleep 0.2) & wait $!; echo "st=$?"`,
		Why:     "naming the job splits the panel where the bare form did not: bash, dash and zsh answer exactly as above and ksh93 drops its own 256 encoding for a plain 1 (WaitForAJobFailsWhenInterrupted). `wait %1` is the same answer in all four, so the axis is about having an operand and not about how it is spelled",
	},
	{
		ID: "trap/wait-is-not-cut-short-by-an-ignored-signal", Category: "traps and exit",
		Snippet: `trap '' USR1; (sleep 0.3; kill -USR1 $$; sleep 0.2) & wait; echo "st=$?"`,
		Why:     "the control, and the line between the two: an ignored signal has no handler to run, so it does not interrupt anything and `wait` still reports 0 in all four. Without it the case above would be evidence about a signal arriving rather than about a *trapped* one arriving. Same shape to the character, so the trap is the only variable",
	},
	{
		ID: "trap/exit-runs-at-the-end", Category: "traps and exit",
		Snippet: `trap 'echo bye' EXIT; echo hi`,
		Why:     "the EXIT trap runs after the script, not where it was set",
	},
	{
		ID: "trap/exit-sees-the-last-status", Category: "traps and exit",
		Script:  true,
		Snippet: "trap 'echo st=$?' EXIT\nfalse\n",
		Why:     "the body reads `$?` at the moment it fires, which is what makes an EXIT trap useful for reporting",
	},
	{
		ID: "trap/exit-trap-can-override-the-status", Category: "traps and exit",
		Snippet: `trap 'echo bye; exit 7' EXIT; exit 2`,
		Why:     "the trap's own exit wins over the one that triggered it",
	},
	{
		ID: "trap/second-trap-replaces", Category: "traps and exit",
		Snippet: `trap 'echo one' EXIT; trap 'echo two' EXIT; echo body`,
		Why:     "traps are set rather than accumulated, and `trap -` removes",
	},
	{
		ID: "trap/err-fires-on-failure", Category: "traps and exit",
		Snippet: `trap 'echo "E=$?"' ERR; false; echo "after=$?"`,
		Why:     "ERR fires on a failing command with `set -e` nowhere in sight, sees the failing status, and leaves it for the script — dash alone refuses the name, with its ordinary bad-trap words",
	},
	{
		ID: "trap/err-under-errexit", Category: "traps and exit",
		Snippet: `set -e; trap 'echo ERR' ERR; false`,
		Why:     "the reason the condition exists: the trap runs first and errexit then stops the script with the failure's own status, so a script can say where it died",
	},
	{
		ID: "trap/debug-fires-before-each-command", Category: "traps and exit",
		Snippet: `trap 'echo D' DEBUG; echo a; echo b`,
		Why:     "DEBUG runs before each simple command rather than after — the D precedes what it announces — and dash refuses the name like any other word that is no signal",
	},
	{
		ID: "trap/return-fires-when-a-sourced-file-ends", Category: "traps and exit",
		Snippet: `trap 'echo R' RETURN; echo 'echo insource' > lib.sh; . ./lib.sh; echo after`,
		Why:     "RETURN is one shell's alone — three of the four refuse it as a bad signal — and where it exists a sourced file fires it on the way out, wherever the trap was set",
	},
	{
		ID: "trap/subshell-does-not-refire", Category: "traps and exit",
		Snippet: `trap 'echo T' EXIT; (echo sub); x=$(echo cs); echo after`,
		Why:     "the trap fires once for the script: neither a subshell nor a command substitution repeats it",
	},
	{
		ID: "trap/subshell-resets-a-handled-trap", Category: "traps and exit",
		Snippet: `trap 'echo x' USR1; (trap); echo done`,
		Why:     "a subshell starts with a handled trap back at its default, unanimously — what differs is the listing: bash and ksh93 still show the trap they will not fire, dash and zsh show nothing",
	},
	{
		ID: "trap/subshell-keeps-an-ignored-one", Category: "traps and exit",
		Snippet: `trap '' USR2; (trap); echo done`,
		Why:     "an ignored signal crosses the fork still ignored, and three of the four list it in the subshell; zsh keeps the ignore working and hides it from the listing",
	},
	{
		ID: "trap/listing-in-a-pipeline-element", Category: "traps and exit",
		Snippet: `trap 'echo x' USR1; trap | cat; echo done`,
		Why:     "a pipeline element is a subshell environment with an inheritance rule of its own: bash and zsh keep the parent's listing there, dash and ksh93 print nothing — the shape issue #339 measured, orthogonal to which end of the pipeline forks",
	},
	{
		ID: "trap/set-in-a-function-diverges", Category: "traps and exit",
		Script:  true,
		Snippet: "f() { trap 'echo TRAP' EXIT; echo enter; }\nf\necho between\n",
		Why:     "zsh runs a trap set inside a function when the function returns; dash, bash and ksh93 keep it for the end of the script",
	},
	{
		ID: "exit/status-and-wrapping", Category: "traps and exit",
		Snippet: `(exit 300); echo "[$?]"; (false; exit); echo "[$?]"`,
		Why:     "a status is taken modulo 256, and a bare `exit` reports what the last command did",
	},
	{
		ID: "exit/bad-argument-diverges", Category: "traps and exit",
		Snippet: `(exit -1); echo "[$?]"; (exit abc); echo "[$?]"`,
		Why:     "an ordering rather than a side: dash refuses both, bash refuses only the one that is not a number, ksh93 and zsh take either",
	},
	{
		ID: "xtrace/traces-each-command", Category: "shell options",
		Snippet: `set -x; echo a b`,
		Why:     "the structure is unanimous: every simple command goes to stderr, expanded, before it runs",
	},
	{
		ID: "xtrace/quoting-diverges", Category: "shell options",
		Snippet: `set -x; x="hello wor"; echo "$x"`,
		Why:     "dash prints an expanded field with a space in it unquoted, so two arguments and one are indistinguishable; the others quote",
	},
	{
		ID: "xtrace/embedded-quote-diverges", Category: "shell options",
		Snippet: `set -x; x="it's"; echo "$x"`,
		Why:     "ksh93 reaches for $'…' where bash and zsh close, escape and reopen",
	},
	{
		ID: "xtrace/prefix-diverges", Category: "shell options",
		Snippet: `set -x; f() { echo in; }; f`,
		Why:     "zsh names the script and line, and the function and 0 inside one, where the others print a bare plus",
	},
	{
		ID: "xtrace/assignments-per-line-diverges", Category: "shell options",
		Snippet: `set -x; a=1 b=2`,
		Why:     "bash and ksh93 give each assignment its own line; dash and zsh put them on one",
	},
	{
		ID: "xtrace/disabling-set-diverges", Category: "shell options",
		Snippet: `set -x; set +x; echo done`,
		Why:     "ksh93 applies the change before printing the command that makes it, so the command that stops tracing leaves no trace of itself",
	},
	{
		ID: "xtrace/compound-header-diverges", Category: "shell options",
		Snippet: `set -x; for i in 1 2; do echo $i; done`,
		Why:     "three answers, not two: dash and ksh93 print only the commands inside, bash reprints the header as written once per iteration, and zsh prints neither but shows the assignment the iteration made",
	},
	{
		ID: "xtrace/pipeline-order-diverges", Category: "shell options",
		Snippet:        `set -x; echo a | cat`,
		ReferenceRaces: true,
		Why:            "ksh93 usually prints the last element first, which follows from its running that one in the current shell — but only usually: its two processes race to their trace points, 41 runs in 400 come out the other way. Nor is that ksh93's alone, which is what the row looked like until every column was counted rather than the loudest one: bash 5 reorders 5 times in 200, bash 3.2 once in 400 and zsh once in 200, so four of the six columns were seen to answer both ways and only dash held still. A pipeline's elements are separate processes and nothing sequences their trace points, so the order is the scheduler's and not the shell's — a fact about how the trace is emitted rather than about anything measured against it",
	},
	{
		ID: "nounset/unset-variable-is-an-error", Category: "shell options",
		Script:  true,
		Snippet: "set -u\necho \"[$NOPE]\"\necho after\n",
		Why:     "the whole point of -u, and the baseline the exemptions are measured against",
	},
	{
		ID: "shopt/nullglob-empties-a-miss", Category: "shell options",
		Snippet: `shopt -s nullglob 2>/dev/null; echo zz*zz; echo done`,
		Why: "the builtin is bash's alone, so the miss becomes an empty line there " +
			"and the pattern stands literal in the three shells where `shopt` is " +
			"not a command — the error is discarded and the divergence is the point",
	},
	{
		ID: "shopt/globstar-crosses-directories", Category: "shell options",
		Snippet: `shopt -s globstar 2>/dev/null; mkdir -p d/e; touch d/e/f; echo **/f`,
		Why: "with the option, `**` alone as a component crosses directory levels " +
			"in bash; zsh crosses natively without any option, and dash and ksh93 " +
			"read `**` as `*` and leave the unmatched pattern standing",
	},
	{
		ID: "shopt/nocasematch-folds-case", Category: "shell options",
		Snippet: `shopt -s nocasematch 2>/dev/null; case A in a) echo hit;; *) echo exact;; esac`,
		Why: "the option folds `case` and `[[ ]]` matching in bash and nothing " +
			"else has it: the other three keep matching exact because the command " +
			"that would have changed it was never theirs",
	},
	{
		ID: "shopt/query-answers-by-status", Category: "shell options",
		Snippet: `shopt -q nullglob 2>/dev/null; echo q=$?; ` +
			`shopt -s nullglob 2>/dev/null; shopt -q nullglob 2>/dev/null; echo q=$?`,
		Why: "-q answers by status alone — 1 while the option is off and 0 once " +
			"-s has set it; the shells without the builtin answer 127 twice, " +
			"which records what a probing script would see there",
	},
	{
		ID: "readonly/reassignment-by-a-declaration", Category: "builtins",
		Snippet: "readonly x=1; export x=2; echo after",
		Why:     "the same refusal reached through a declaration utility rather than by an assignment standing alone, and a different set of shells stops for it — three here, where a plain assignment stops all four. So which of the two ways the name was set decides, and one shell answers the two oppositely: it stops for the plain form given as an argument and never stops for this one",
	},
	{
		ID: "readonly/reassignment-from-a-command-string", Category: "builtins",
		Snippet: "readonly x=1; x=2; echo after",
		Why:     "an assignment to a name that cannot take one, given as an argument rather than read from a file. All four stop here — and one of them does not when the same three lines come from a file, which the case recorded elsewhere shows. So a readonly reassignment is fatal in three shells always and in the fourth by invocation, which is the second thing found to work that way after an expansion that failed",
	},
	{
		ID: "nounset/unset-variable-from-a-command-string", Category: "expansion",
		Snippet: "set -u\necho \"$NOPE\"\n",
		Why:     "the same two lines as the case above, given as an argument instead of read from a file. Three of the panel answer the same either way; one answers 127 here and 1 there, which is a fact about how the shell was started rather than about the expansion — and only the pair can show it",
	},
	{
		ID: "nounset/defaults-are-exempt", Category: "shell options",
		Script:  true,
		Snippet: "set -u\necho \"[${NOPE:-d}][${NOPE-d}][${NOPE+a}]\"\necho after\n",
		Why:     "a form that supplies a value, or asks whether one is set, is not a use of an unset one",
	},
	{
		ID: "nounset/empty-is-not-unset", Category: "shell options",
		Script:  true,
		Snippet: "set -u\nE=\necho \"[$E]\"\necho after\n",
		Why:     "set-but-empty is the distinction -u rests on, and it is unanimous",
	},
	{
		ID: "nounset/no-parameters-is-not-unset", Category: "shell options",
		Script:  true,
		Snippet: "set -u\necho \"[$@][$*]\"\necho after\n",
		Why:     "`$@` and `$*` with nothing to expand are quiet in all four, which is not obvious and is often got wrong",
	},
	{
		ID: "nounset/unset-positional-diverges", Category: "shell options",
		Script:  true,
		Snippet: "set -u\necho \"[$1]\"\necho after\n",
		Why:     "ksh93 lets an argument it was not given expand to nothing where the other three stop, and neither says anything about it",
	},
	{
		ID: "param/unset-positional-takes-a-default", Category: "parameter expansion",
		Snippet: `echo "[${1-default}]"`,
		Why:     "an out-of-range positional is unset rather than empty, so the plain default form fires for it",
	},
	{
		ID: "errexit/failure-ends-the-script", Category: "shell options",
		Snippet: `set -e; false; echo reached`,
		Why:     "the whole point of -e, and the baseline the exemptions are measured against",
	},
	{
		ID: "errexit/condition-is-exempt", Category: "shell options",
		Snippet: `set -e; if false; then :; fi; while false; do :; done; echo reached`,
		Why:     "a command whose status is being tested is not a failure; -e would otherwise make `if` useless",
	},
	{
		ID: "errexit/exemption-reaches-into-functions", Category: "shell options",
		Snippet: `set -e; f() { false; echo inner; }; if f; then :; fi; echo reached`,
		Why:     "the subtle one: the exemption is inherited, so the function keeps going past its own failure — unanimous, and the part most implementations get wrong",
	},
	{
		ID: "errexit/only-the-last-of-a-chain", Category: "shell options",
		Snippet: `set -e; false && :; echo one; : && false; echo two`,
		Why:     "-e judges the final operand of an && chain and nothing before it, so the first line survives and the second does not",
	},
	{
		ID: "errexit/negation-is-exempt", Category: "shell options",
		Snippet: `set -e; ! true; echo reached`,
		Why:     "`!` tests a status rather than requiring success, so a failing negation is not a failure",
	},
	{
		ID: "status/an-assignment-can-read-the-previous-status", Category: "shell options",
		Snippet: `false; E=$?; echo "E=$E ?=$?"`,
		Why:     "the most common idiom there is, and it asks two things at once: `$?` on the right names the command before the assignment, and the assignment then reports its own success. Getting the order wrong makes `E=$?` read 0 and nothing looks broken",
	},
	{
		ID: "status/an-assignment-reports-its-substitution", Category: "shell options",
		Snippet: `true; x=$(false); echo "st=$?"; false; y=1; echo "st=$?"`,
		Why:     "the other half of the same rule, and why the status cannot simply be set before the right-hand sides run: an assignment reports what a substitution in it reported, and reports success when there is none — even where a failing command came first",
	},
	{
		ID: "jobs/a-running-background-job", Category: "builtins",
		Snippet: `sleep 0.4 & jobs`,
		Why:     "one line of a `jobs` listing, and four shells write it four ways — the marker spacing, the width of the state column, the case of the word, and whether the command is there at all. dash shows an empty column and ksh93 `<command unknown>`, because neither kept the text; bash puts the `&` back on",
	},
	{
		ID: "jobs/a-finished-background-job", Category: "builtins",
		Snippet: `sleep 0.05 & sleep 0.5; jobs; echo "---"; jobs`,
		Why:     "reported once and then forgotten, in every shell that reports it at all — the second listing is empty. zsh never mentions it and ksh93 still calls it Running, which is not a reaping race: it says so after `wait` too",
	},
	{
		ID: "jobs/a-background-job-that-failed", Category: "builtins",
		Snippet: `false & sleep 0.3; jobs`,
		Why:     "the status reaches the listing, and the two shells that say so disagree about how: `Exit 1` against `Done(1)`",
	},
	{
		ID: "jobs/two-jobs-and-which-end-it-starts-from", Category: "builtins",
		Snippet: `sleep 0.4 & sleep 0.4 & jobs`,
		Why:     "dash and ksh93 print the most recent first and bash and zsh the oldest, and the number stays with the job either way — `%2` has to mean the same thing at both ends. Also where the `+` and `-` markers become visible",
	},
	{
		ID: "jobs/dash-p-is-the-process-ids-alone", Category: "builtins",
		Snippet: `sleep 0.4 & jobs -p >p.txt; read x <p.txt; case $x in "$!") echo "the job's process id alone";; *"$!"*) echo "a listing with the id in it";; *) echo "neither: [$x]";; esac; wait`,
		Why:     "the `kill $(jobs -p)` idiom, and the one place the letter splits: dash, bash and ksh93 print process ids and nothing else, zsh reads the same letter as the job's process *group* and prints its ordinary rows. Compared against `$!` rather than printed, because a process id is not the same twice. Through a file rather than a pipe: a subshell has no job table in dash or zsh",
	},
	{
		ID: "jobs/dash-l-puts-the-process-id-in-the-listing", Category: "builtins",
		Snippet: `sleep 0.4 & jobs -l >l.txt; sed -e "s/ [0-9][0-9]*/ PID/" l.txt | tr -s " "; wait`,
		Why:     "the one `jobs` letter all five have, and each puts the id somewhere different — after the marker, or eating one of the two spaces behind it, or with a tab after it. Runs of spaces are squeezed because dash narrows its state column by the width of the id, so the untouched line would depend on how many digits this machine's process ids have",
	},
	{
		ID: "jobs/dash-l-and-dash-p-the-last-one-wins", Category: "builtins",
		Snippet: `sleep 0.4 & jobs -pl >a.txt; read x <a.txt; case $x in "$!") echo "-pl: ids";; *) echo "-pl: a listing";; esac; jobs -lp >b.txt; read y <b.txt; case $y in "$!") echo "-lp: ids";; *) echo "-lp: a listing";; esac; wait`,
		Why:     "the two format letters are exclusive, and the last one given decides rather than either winning outright — unanimous in the three shells whose `-p` is the ids alone, and moot in the one whose is not",
	},
	{
		ID: "jobs/dash-r-lists-the-running-ones", Category: "builtins",
		Snippet: `sleep 0.4 & jobs -r; echo "st=$?"; wait`,
		Why:     "a state filter bash and zsh have and dash and ksh93 have never heard of — an illegal option in the two without it, which is why the letter set has to be the dialect's rather than the engine's",
	},
	{
		ID: "jobs/dash-s-is-a-letter-two-shells-do-not-have", Category: "builtins",
		Snippet: `jobs -s; echo "st=$?"`,
		Why:     "the other half of the same split, with nothing stopped to list: silence and 0 where the letter exists, a refusal and 2 where it does not. Absence is the answer here rather than a divergence in behavior",
	},
	{
		ID: "jobs/an-option-no-shell-has", Category: "builtins",
		Snippet: `jobs -Q; echo "st=$?"`,
		Why:     "the refusal itself: four wordings, two of them with a usage line naming the letters that shell really does have, and 2 everywhere but zsh. An option silently ignored is the failure this pins against",
	},
	{
		ID: "jobs/a-subshell-and-the-parents-jobs", Category: "builtins",
		Snippet: `sleep 0.4 & first=$!; (jobs -p) >s.txt; x=; read x <s.txt; case $x in "$first") echo "the parent's job";; "") echo "no jobs";; *) echo "something else";; esac; wait`,
		Why:     "ksh93 alone hands a subshell the jobs the shell around it started; bash, dash and zsh hand it an empty table. Through a file rather than by printing the id, because a process id is not the same twice — and with commands after the `( … )`, because a subshell that is the last thing a script does need not be a subshell at all: without them dash answers the parent's job instead",
	},
	{
		ID: "jobs/a-pipeline-element-and-the-parents-jobs", Category: "builtins",
		Snippet: `sleep 0.4 & first=$!; jobs -p | cat >s.txt; x=; read x <s.txt; case $x in "$first") echo "the parent's job";; "") echo "no jobs";; *) echo "something else";; esac; wait`,
		Why:     "`jobs -p | cat` is the idiom the question is really about, and it splits the panel differently from the row above: bash and ksh93 list the parent's job here, dash and zsh list nothing. Two rows, two different pairs, which is why one yes-or-no cannot hold both",
	},
	{
		ID: "jobs/a-group-in-a-pipeline-and-the-parents-jobs", Category: "builtins",
		Snippet: `sleep 0.4 & first=$!; { jobs -p; } | cat >s.txt; x=; read x <s.txt; case $x in "$first") echo "the parent's job";; "") echo "no jobs";; *) echo "something else";; esac; wait`,
		Why:     "the same pipeline with braces around the same builtin, and bash changes its answer: a simple command as an element keeps the parent's jobs there and a compound one does not. ksh93 still lists and dash and zsh still do not, so this is the row that says bash's answer is neither of the other two",
	},
	{
		ID: "jobs/a-substitution-and-the-parents-jobs", Category: "builtins",
		Snippet: `sleep 0.4 & first=$!; case "$(jobs -p)" in "$first") echo "the parent's job";; "") echo "no jobs";; *) echo "something else";; esac; wait`,
		Why:     "command substitution goes with the simple pipeline element rather than with the parentheses it is written like — bash and ksh93 list, dash and zsh do not. `cat <(jobs -p)` is measured the same way in the three shells that have it",
	},
	{
		ID: "jobs/a-subshell-lists-a-job-it-started-itself", Category: "builtins",
		Snippet: `sleep 0.4 & first=$!; (sleep 0.4 & jobs -p >s.txt; wait); x=; read x <s.txt; case $x in "$first") echo "the parent's job";; "") echo "no jobs";; *) echo "its own";; esac; wait`,
		Why:     "the control for all four rows above: whatever a shell hands a subshell of the parent's table, a job the subshell starts for itself is listed there — unanimously. Without it an empty listing would be evidence that `jobs` does not work in a subshell rather than that the table is emptied on the way in",
	},
	{
		ID: "jobs/two-job-specs-in-the-order-written", Category: "builtins",
		Snippet: `sleep 0.4 & sleep 0.5 & jobs %2 %1; wait`,
		Why:     "operands settle the order themselves — `%2 %1` lists 2 then 1 in all five, including the two whose bare listing starts from the newest — and each row keeps the job's own number rather than counting from the start of the listing",
	},
	{
		ID: "errexit/assignment-takes-the-substitution", Category: "shell options",
		Snippet: `set -e; x=$(false); echo reached`,
		Why:     "an assignment reports what the substitution reported, so this ends the script where `echo \"$(false)\"` does not",
	},
	{
		ID: "procsub/reads-a-command-as-a-file", Category: "redirection",
		Snippet: `cat <(echo hi)`,
		Why:     "`<(cmd)` runs cmd and expands to a path its output can be read from — the last of the core language, and the clearest case of a dialect being a runtime switch: bash 3.2 has it as `bash` and loses it as `sh`. dash has it in neither guise and reports the `(` as unexpected",
	},
	{
		ID: "procsub/two-of-them-in-one-command", Category: "redirection",
		Snippet: `diff <(echo a) <(echo a) && echo same`,
		Why:     "the reason the construct exists: two commands compared as though they were files, with no temporary file named anywhere. Two substitutions in one command also have to keep their own pipes, which is exactly what the first attempt got wrong",
	},
	{
		ID: "procsub/a-redirection-where-a-target-belongs", Category: "redirection",
		SyntaxError: true,
		Snippet:     `cat < < x; echo "st=$?"`,
		Why:         "what the three without process substitution make of the second `<`, and the one wording dash does not share: it says `redirection unexpected` where the other three name the token. Reached here because `< <(cmd)` is this text in a dialect that has no such construct",
	},
	{
		ID: "procsub/feeds-a-loop", Category: "redirection",
		Snippet: `while read -r l; do echo "[$l]"; done < <(printf "a\nb\n")`,
		Why:     "the idiom people actually reach for it with, and the reason a pipeline will not do: the loop runs in *this* shell, so what it reads is still there afterwards",
	},
	{
		ID: "procsub/quoted-is-not-a-substitution", Category: "redirection",
		Snippet: `printf "[%s]\n" "<(echo hi)"`,
		Why:     "the construct is unquoted-only: inside double quotes the same ten characters are text, unanimously and dash included. It is the completeness half of `procsub/reads-a-command-as-a-file` — that case says the lexer reads the form, this one says where it stops looking, and a lexer that also read it inside quotes would pass the first and fail here",
	},
	{
		ID: "read/a-failing-read-still-assigns", Category: "builtins",
		Snippet: `l=keep; read -r l </dev/null; echo "st=$? l=[$l]"`,
		Why:     "end of input clears the variables rather than leaving what was there, which is what stops `while read -r l` from leaving the last line behind for the code after the loop. Unanimous across the panel, so it is the core's answer and not an axis",
	},
	{
		ID: "read/a-final-line-without-a-newline", Category: "builtins",
		Snippet: `printf 'x' | { read -r l; echo "st=$? l=[$l]"; }`,
		Why:     "both answers at once: there is a line, and there will not be another. Every shell assigns it *and* reports failure, which reads as a contradiction until the loop below explains it",
	},
	{
		ID: "read/an-unterminated-last-line-is-dropped", Category: "builtins",
		Snippet: `printf 'a\nb' | while read -r l; do printf "<%s>" "$l"; done; echo`,
		Why:     "the consequence of the status above, and the reason it is worth pinning rather than fixing: a file whose last line has no newline loses that line in every shell there is. Returning 0 instead would run it twice — once as the line, once as the empty read after it",
	},
	{
		ID: "read/options-cluster-in-one-word", Category: "builtins",
		Snippet: `printf 'x\\\ny\n' | { read -rr v; echo "[$v]"; }`,
		Why:     "`-rr` is two options in one word — the POSIX guideline every shell follows, and the reason `read -ra arr` means `-r -a arr`. Two of the same letter so the case needs only the one option everybody implements: the bundle is read letter by letter, the raw flag holds, and the backslash survives its line. Reading whole words refused the bundle outright (#347)",
	},
	{
		ID: "read/a-bad-letter-in-a-bundle-is-named-alone", Category: "builtins",
		Snippet: `read -rx v </dev/null; echo "st=$?"; echo after`,
		Why:     "the complaint about a bundle names the letter the walk stopped on, never the word it rode in on: `-x` in all four, including the dialect recorded as whole-word naming from `export -Q`, where the letter and the word are the same thing. Four wordings, zsh reporting 1 to everyone else's 2, and all four carry on — `read` is not a special builtin, so nobody's fatality rule reaches it",
	},
	{
		ID: "read/fields-into-an-array", Category: "builtins",
		Snippet: `echo "a b c" | { read -a arr; echo "[${arr[1]}]"; }`,
		Why:     "the letter is the dialect's before the behavior is: bash's -a puts the fields in the named array, ksh93 spells the option -A and refuses -a with its usage, zsh refuses it in one line, and dash refuses the option and then the subscript too",
	},
	{
		ID: "read/until-a-delimiter", Category: "builtins",
		Snippet: `printf 'a:b c\n' | { read -d : v; echo "[$v]"; }`,
		Why:     "-d renames the delimiter: the three shells with the letter stop at the colon and leave the rest unread — the newline they would have stopped at now ordinary input — and dash refuses the option",
	},
	{
		ID: "read/a-count-of-characters", Category: "builtins",
		Snippet: `printf 'abcdef' | { read -n 3 v; echo "[$v]"; }`,
		Why:     "-n takes a count in bash and ksh93 and three characters arrive; zsh reads the same -n as a bare flag for its completion widgets, so the count becomes the name read into and v stays empty — one spelling, two shapes",
	},
	{
		ID: "read/a-backslash-escapes-and-is-removed", Category: "builtins",
		Snippet: `printf 'a\\tb\n' | { read v; echo "[$v]"; }`,
		Why:     "without -r a backslash removes the special meaning of the character after it and is itself removed — a literal backslash-t reads as `atb`, unanimously. We kept the backslash (#320), which -r mode and the line continuation both being right had hidden",
	},
	{
		ID: "read/an-escaped-separator-does-not-split", Category: "builtins",
		Snippet: `printf 'a\\ b c\n' | { read x y; echo "[$x][$y]"; }`,
		Why:     "the subtle half of the escape: an escaped IFS character is data, so `a\\ b` is one field in all four shells. The escape has to reach the splitter — after a pre-pass that removes the backslashes, an escaped space and a separating one are the same byte",
	},
	{
		ID: "read/silent-still-reads", Category: "builtins",
		Snippet: `printf 'secret\n' | { read -s v; echo "v=$v"; }`,
		Why:     "-s is about a terminal's echo and there is no terminal here, so it must parse, read and stay quiet: skipping the word unread is how a password gets echoed, and refusing it fails a script that works everywhere else (#321). dash alone has no -s",
	},
	{
		ID: "read/a-prompt-or-a-coprocess", Category: "builtins",
		Snippet: `printf 'data\n' | { v=keep; read -p PR0MPT v; echo "st=$? v=[$v]"; }`,
		Why:     "one letter, two shapes (#422): bash and dash take a prompt as -p's argument and show it only to a terminal, so the word is consumed, nothing is printed, and the pipe is read as though the flag were absent; ksh93 and zsh read -p as the coprocess and, with none running, refuse — 1, their own words, and v still holding `keep`, because the read failed before reaching any input. The optstring carries the split the way it does for -n",
	},
	{
		ID: "read/a-prompt-that-never-arrives", Category: "builtins",
		Snippet: `read -p </dev/null; echo "st=$?"`,
		Why:     "the same word missing means three different things: bash wants -p's argument and says so with its usage, status 2; dash wants it too and says `No arg for -p option`; ksh93 and zsh never wanted one — their -p is the coprocess flag, so this is the no-coprocess refusal again at 1. A letter's arity is part of the dialect's answer, not just its spelling",
	},
	{
		ID: "read/from-the-shells-own-standard-input", Category: "builtins",
		Snippet: `read a; echo "[$a]"; read b; echo "[$b]"; read c; echo "eof=$?|[$c]"`,
		Stdin:   "one\ntwo\n",
		Why:     "every other read case feeds a pipe or a here-string built inside the snippet, so what the *shell* was started with was never read at all. Here it is the shell's own input: two lines arrive in order and the third read finds the end, reporting 1 with the variable cleared rather than left holding the line before",
	},
	{
		ID: "read/a-zero-timeout-and-what-it-does-to-the-stream", Category: "builtins",
		Snippet: `printf "a\nb\n" > f; exec < f; read -t 0 v; echo "st=$? v=[$v]"; read w; echo "w=[$w]"`,
		Why:     "a timeout of zero is not a deadline that has already passed: one shell answers whether input is waiting and reads nothing, so the next read still finds the first line, while two read the line and leave the second for it. The second read is the whole point — the status alone cannot tell a poll from a read, and a poll that consumed what it reported would have a loop eating its own input. The input is a file rather than a pipe because a file is always ready, which makes the case a fact rather than a race",
	},
	{
		ID: "read/a-zero-timeout-at-the-end-of-the-input", Category: "builtins",
		Snippet: `: > f; exec < f; read -t 0 v; echo "st=$? v=[$v]"`,
		Why:     "the end of a stream is *ready* to the shell that polls — a read there would return at once, with nothing — so it reports success where the shells that read report the end of input. Two answers to the same question from one empty file",
	},
	{
		ID: "redir/a-target-that-is-not-one-word", Category: "redirection",
		Snippet: `e="a b"; echo hi > $e; echo "st=$?"`,
		Why:     "a redirection target is expanded and then, in three of the four, neither split nor matched — so `> $e` writes to a file called `a b`. bash expands it as an ordinary word and refuses anything that is not exactly one, naming the target *as written*. Doing bash's expansion and taking the first field is the answer nobody gives, and it wrote to `a`",
	},
	{
		ID: "redir/a-target-that-expands-to-nothing", Category: "redirection",
		Snippet: `e=; echo hi > $e; echo "st=$?"`,
		Why:     "the same question with no words rather than two, and all four complain in four different ways — one of them shorter than its own wording for any other failed open, and with no reason attached",
	},
	{
		ID: "redir/a-target-holding-a-pattern", Category: "redirection",
		Snippet: `e="nomatch-*"; echo hi > $e; ls nomatch-*; rm -f nomatch-*`,
		Why:     "a target is not matched as a pattern, so this creates a file whose name holds an asterisk rather than writing to whatever matched. The dangerous half is invisible here and was real: matching it truncated a file the script never named",
	},
	{
		ID: "redir/a-quoted-target-with-a-space", Category: "redirection",
		Snippet: `e="a b"; echo hi > "$e"; cat "a b"; rm -f "a b"`,
		Why:     "quoting settles it in every dialect, including the one that refuses the unquoted form: splitting is what bash objects to, not the space",
	},
	{
		ID: "redir/a-here-string-is-one-line", Category: "redirection",
		Snippet: `two="a b"; cat <<< $two`,
		Why:     "a here-string's word expands but is never split — one line of input with its space kept, in every shell that has the construct. dash's row is its parser refusing `<<<`, not an opinion about splitting; the word is a body rather than a filename, so the ordinary-word question a target gets never arises here",
	},
	{
		ID: "exec/a-command-is-named-as-it-was-written", Category: "commands",
		Snippet: `basename --bad 2>&1 | head -1`,
		Why:     "a command names itself from `argv[0]`, and what belongs there is the word that was typed rather than the path PATH resolved to. Unanimous, invisible until something fails, and then it is in the output of a program the shell did not write — which is why a whole-machine run sweep had eighteen lines differing by nothing else",
	},
	{
		ID: "name/unset-f-on-a-name-no-function-could-have", Category: "builtins",
		Snippet: `unset -f 1x; echo "st=$?"`,
		Why:     "two of the panel are quiet here and two are not, and the two that speak are not answering the same question — one is judging the name, which `1x` could never be, and the other is reporting that its table holds nothing under it. The case next to this one is what tells them apart",
	},
	{
		ID: "name/unset-f-on-a-name-that-is-merely-undefined", Category: "builtins",
		Snippet: `unset -f nosuch; echo "st=$?"`,
		Why:     "a name a function could perfectly well have, and none does. Only one of the panel says anything, and it is not the one that complained about `1x` — so the two questions are independent and each needs its own answer. Three quiet and one not, where the case above is two and two",
	},
	{
		ID: "location/a-message-from-inside-a-function", Category: "diagnostics",
		// The answer is the line *within the function*, so reprinting the
		// snippet onto different lines changes it.
		LayoutSensitive: true,
		Snippet:         "f() {\n  nosuchcmd\n}\ntrue\nf\n",
		Why:             "three of the panel name the file and count from the top of it wherever the message came from. zsh names the *function* instead and counts within it, so the same failure is reported at a line that is not the line it is on — which is the sort of thing that looks like an off-by-one until it is measured",
	},
	{
		ID: "location/a-message-from-inside-a-sourced-file", Category: "diagnostics",
		// A script rather than -c so the outer name is a file that could
		// plausibly be named instead: the question is which file the location
		// names, and under -c there is only one. The sourced file is written
		// by the snippet itself, so its name — which is the point — is the
		// same on every machine.
		Script:  true,
		Snippet: "printf 'nosuchcmd-xyz\\n' > inc.sh\n. ./inc.sh\necho st=$?",
		Why:     "the line is the sourced file's in all four, and the name splits the panel: bash and zsh name the sourced file as written, where dash and ksh93 keep the script's own name — dash writing the file's path after the location and ksh93 naming `.` and pinning its outer line where the dot was",
	},
	{
		ID: "location/a-message-from-a-function-a-sourced-file-defined", Category: "diagnostics",
		// Called after the sourcing has finished, so nothing about the `.` is
		// still on the stack — what is named is what the function remembered.
		Script:  true,
		Snippet: "printf 'f() {\\n  nosuchcmd-xyz\\n}\\n' > inc.sh\n. ./inc.sh\nf\necho st=$?",
		Why:     "bash names the file the function was *defined* in, zsh names the function, and dash and ksh93 name the script — while all four count the defining file's lines, so three of the panel report a line the named file does not have",
	},
	{
		ID: "name/a-lone-dash-given-to-a-builtin", Category: "builtins",
		Snippet: `unalias -; echo "st=$?"`,
		Why:     "a `-` on its own is an operand in three of the panel and an option in zsh, which eats it. `unalias` is where that shows: the three complain about an alias called `-`, each in its own words, and the fourth complains that it was given nothing to unalias at all. `unset -` looks the same in bash for a different reason — its bare form validates no operand — which is why the case is not written with that one",
	},
	{
		ID: "name/unset-v-validates-the-lone-dash", Category: "builtins",
		Snippet: `unset -v -; echo "st=$?"`,
		Why:     "the other visible site of the LoneDashIsAnOption axis: `unset -v` validates its operand where bare `unset` does not, so bash, dash and ksh93 name the dash as a bad variable name — fatally in dash, whose unset failure ends the script — while zsh eats the dash as an option and complains it has nothing left to unset",
	},
	{
		ID: "param/error-operator-on-an-unset-name", Category: "expansion",
		// A script rather than -c: bash exits 127 for this when it was given
		// its program as an argument and 1 when it read a file, which is a
		// question about how the shell was started rather than about this
		// operator.
		Script:  true,
		Snippet: `unset V; echo "[${V?}]"; echo after`,
		Why:     "the operator a script uses to say a variable is required. Unanimous in shape — the name, then the word — and unanimous in stopping the script, which is what the missing `after` records. The status is where they part: one answers 127 here and 1 or 2 everywhere else it stops",
	},
	{
		ID: "param/error-operator-default-word", Category: "expansion",
		// A script rather than -c: bash exits 127 for this when it was given
		// its program as an argument and 1 when it read a file, which is a
		// question about how the shell was started rather than about this
		// operator.
		Script:  true,
		Snippet: `V=; echo "[${V:?}]"`,
		Why:     "with no word given there is a default, and the colon form covers two cases at once — absent, and there but empty — so each shell has to decide how to say both. Two have a phrase for the pair and word it differently, one says only that it is not set, and one keeps a separate word for a parameter that is there and empty. Four answers to one question",
	},
	{
		ID: "param/error-operator-on-a-name-that-is-set", Category: "expansion",
		// A script rather than -c: bash exits 127 for this when it was given
		// its program as an argument and 1 when it read a file, which is a
		// question about how the shell was started rather than about this
		// operator.
		Script:  true,
		Snippet: `V=x; echo "[${V?}][${V:?}][${V?why}]"; echo after`,
		Why:     "the other side, and unanimous: with the parameter set the operator is not an error at all and expands to the value, word or no word. Recorded because the bug this pair was written for expanded it to nothing here while reporting nothing either — a case that only tested the failing side would have passed",
	},
	{
		ID: "return/with-nothing-to-return-from", Category: "builtins",
		Snippet: `echo before; return 7; echo "after st=$?"`,
		Why:     "a `return` outside both a function and a sourced file has nothing to return from, and the panel splits over what that means — not over the wording but over *where the script stops*. Three obey it and end there with the status given; one reports it, leaves 2 behind and runs the next command. A script whose last statement is such a `return` therefore ends two different ways with the same output, which is why the status is half the case",
	},
	{
		ID: "return/inside-a-sourced-file", Category: "builtins",
		Snippet: "printf 'return 7\\n' > s.sh\n. ./s.sh\necho \"st=$?\"\n",
		Why:     "the other side of the same question, and unanimous: a sourced file is something to return *from*, so all four obey it and it becomes the source's status. Recorded next to the case above because together they say the disagreement is about having nothing to return from rather than about `return` itself",
	},
	{
		ID: "set/a-name-only-one-shell-has", Category: "builtins",
		Snippet: `set +o posix; echo "st=$?"`,
		Why:     "which long option names a shell has is not one list: fourteen are unanimous and the rest belong to one, two or three of the panel. `posix` belongs to one, and the other three refuse it — each in its own words and with its own status. Turning it *off* is the direction that matters, because it is what the thirteenth line of Homebrew's own script does and what a shell without a posix mode can honestly grant",
	},
	{
		ID: "set/allexport-marks-what-follows", Category: "builtins",
		Snippet: "set -a\nFOO=bar\n/bin/sh -c 'echo [$FOO]'\nset +a\nBAR=two\n/bin/sh -c 'echo [$BAR]'\n",
		Why:     "an assignment is not an export until something says so, and `set -a` is the something. Unanimous both ways, and the second half is what makes it evidence: turning it off again has to stop it, or a shell that exported everything always would pass the first half",
	},
	{
		ID: "cd/keeps-or-resolves-the-name-it-was-given", Category: "builtins",
		Snippet: "mkdir -p real/sub && ln -s real link\ncase $(cd link/sub && pwd) in *link*) echo 'plain kept';; *) echo 'plain resolved';; esac\ncase $(cd -L link/sub && pwd) in *link*) echo 'L kept';; *) echo 'L resolved';; esac\ncase $(cd -P link/sub && pwd) in *link*) echo 'P kept';; *) echo 'P resolved';; esac\n",
		Why:     "the two names a directory has — the one it was reached by and the one it is at — and `cd` is where a shell chooses between them. Unanimous. The paths themselves are never printed because they are this machine's; what is compared is which of the two came back",
	},
	{
		ID: "cd/which-path-option-decides", Category: "builtins",
		Snippet: "mkdir -p real/sub && ln -s real link\ncase $(cd -P -L link/sub && pwd) in *link*) echo 'PL kept';; *) echo 'PL resolved';; esac\ncase $(cd -L -P link/sub && pwd) in *link*) echo 'LP kept';; *) echo 'LP resolved';; esac\n",
		Why:     "given both, three of them let the last one win and zsh gives `-P` the answer wherever it stands, so the two orders agree in one shell and disagree in the other three. Written out rather than looped over a variable, because a loop would have measured word splitting instead — the variable stays one word in zsh and the case would have said nothing about `cd` at all",
	},
	{
		ID: "cd/pwd-p-resolves-symlinks", Category: "builtins",
		Snippet: `mkdir -p a/b; ln -s a/b l; cd l; pwd -P | grep -c "/a/b$"; pwd | grep -c "/l$"`,
		Why:     "`pwd -P` reports where the directory is with symlinks resolved, and a plain `pwd` keeps the name it was reached by — both unanimous, and `pwd -L -P` (not shown) lets the last option win in all four as well",
	},
	{
		ID: "subst/a-body-that-runs-in-the-current-shell", Category: "commands",
		Snippet: `x=0; y=${ x=1; echo hi;}; echo "[$x][$y]"`,
		Why:     "a third spelling of command substitution, and the only one that does not run in a subshell — so what it assigns survives, which is the whole reason it exists. Two of the panel have it, one of them only since 5.3, and the other two call it a bad substitution. The `x=0` before it and the `[$x]` after are what tell it from `$( … )`, which would leave the nought",
	},
	{
		ID: "subst/the-subshell-form-loses-what-it-assigns", Category: "commands",
		Snippet: `x=0; y=$(x=1; echo hi); echo "[$x][$y]"`,
		Why:     "the same script with the older spelling, and unanimous: the assignment is lost. Recorded beside the case above because the pair is the difference — either alone says nothing about which shell the body ran in",
	},
	{
		ID: "subst/a-body-is-placed-in-the-script", Category: "commands",
		Snippet: "true\ntrue\ntrue\nx=$(nosuchcmd)\n",
		Why:     "the body of a substitution is a program of its own and is read as one, so its lines count from the body rather than from the file — and every shell in the panel reports what happens inside it at the line it was *written* on. Unanimous for this spelling, which is what makes it the core's answer; the backquoted spelling is not, and one shell numbers that one from the top",
	},
	{
		ID: "subst/a-backquoted-body-is-placed-differently", Category: "commands",
		Snippet: "true\ntrue\ntrue\nx=`nosuchcmd`\n",
		Why:     "the same substitution written the older way, and three of the four number it exactly as they number the other spelling. dash numbers it from one instead, so it keeps two answers for two spellings of one construct — the only place in the panel where how a substitution is written changes where its contents are reported",
	},
	{
		ID: "signal/an-interrupt-that-ended-a-child", Category: "commands",
		Snippet: "/bin/sh -c 'kill -INT $$' 2>/dev/null; echo after",
		Why:     "^C is one of the two deaths nothing remarks on, and in one shell it is also the one that ends the script — silently, and with 128 plus the signal rather than the 256 plus it that the same shell reports for a command killed by one. The others run the next command. Only SIGINT does this: QUIT, TERM, HUP, USR1 and PIPE are all carried on from by all four",
	},
	{
		ID: "signal/a-command-ended-by-a-terminate", Category: "commands",
		Snippet: "{ /bin/sh -c 'kill -TERM $$'; } 2>e.txt\nsed -E \"s/ [0-9]+/ N/g; s/  +/ /g\" e.txt\n",
		Why:     "the same notice as the case above with a different signal, and one shell writes this one with neither its own name, nor the line, nor the process id — the words and the command alone. Its own older version writes the full prefix here, which is the second column that makes this worth recording: the two are the same shell and disagree, so it is a change rather than a convention",
	},
	{
		ID: "signal/a-pipeline-element-that-is-not-the-last", Category: "commands",
		Snippet: "{ /bin/sh -c 'kill -USR1 $$' | cat; } 2>e.txt\nsed -E \"s/ [0-9]+/ N/g; s/  +/ /g\" e.txt\necho after\n",
		Why:     "a signal ends an element whose status the pipeline does not take. Three of the panel pass over it entirely — the same command as the *last* element is remarked on by two of them — and dash says the same thing wherever the element stands. One against three, and the case next to this one is where the same signal is worth a sentence",
	},
	{
		ID: "signal/a-command-a-signal-ended-in-a-group", Category: "commands",
		Snippet: "{ { /bin/sh -c 'kill -USR1 $$'; }; } 2>e.txt\nsed -E \"s/ [0-9]+/ N/g; s/  +/ /g\" e.txt\necho after\n",
		Why:     "a group is not a job, so the shell that writes the command back out names the command and not the group. The subshell that *is* one cannot be recorded here: its notice is written by the parent shell after the redirect inside the script has ended, so no snippet can capture it and the process id in it is not the same twice. That half is held by a test instead",
	},
	{
		ID: "signal/a-command-a-signal-ended-in-a-substitution", Category: "commands",
		Snippet: "{ x=$(/bin/sh -c 'kill -USR1 $$'); } 2>e.txt\nsed -E \"s/ [0-9]+/ N/g; s/  +/ /g\" e.txt\necho after\n",
		Why:     "one shell remarks on a command a signal ended inside `( … )` and says nothing about the same command inside `$(…)`, so the two are separate questions rather than one about copies. The other two that remark on it at all do so wherever it happened",
	},
	{
		ID: "signal/a-command-a-signal-ended", Category: "commands",
		Snippet: "{ /bin/sh -c 'kill -USR1 $$'; } 2>e.txt\nsed -E \"s/ [0-9]+/ N/g; s/  +/ /g\" e.txt\n",
		Why:     "the status carries the signal and nothing in the output says one was involved, so three of the four say it out loud — in three different shapes. One names the process and pads the words to a fixed column before writing the command back out, one names the process and stops, and one prints the words with no process, no command and no location at all, which is the only message it writes that way. The fourth says nothing, with a terminal or without. The digits are masked in the snippet because a process id is not the same twice, and the spaces with them because the padding is one column wide and a shorter process id would move it. The signal is one that ends a process without dumping core, so that running the corpus does not leave a `core` file behind on a machine where dumping is turned on",
	},
	{
		ID: "type/a-function-and-its-body", Category: "builtins",
		Snippet: `f(){ echo hi; }; type f`,
		Why:     "one shell follows the sentence with the function itself, laid out its own way, and the other three stop at the sentence. What is printed is not what was typed — the shell has a tree by then — so this is the one place a shell has to say a command back",
	},
	{
		ID: "type/a-body-with-a-construct-in-it", Category: "builtins",
		Snippet: `f(){ if true; then echo y; fi; }; type f`,
		Why:     "the layout is per construct and not one rule: a `then` stays on the line of its `if` where a `do` moves to a line of its own, and a body closed by a keyword ends with a `;` where one closed by a brace does not",
	},
	{
		ID: "type/a-body-with-redirections-in-it", Category: "builtins",
		Snippet: `f(){ echo hi 2>&1 >&2 3>/dev/null <&-; }; type f`,
		Why:     "the one shell that says a body back writes a redirection two ways, and the tree it prints from has forgotten which was typed. A file target takes a space after the operator and keeps only the descriptor that was written; a dup is written tight and has the descriptor it acts on filled in, so `>&2` comes back as `1>&2` and a close comes back as `>&-` whichever operator asked for it",
	},
	{
		ID: "type/what-a-name-would-run", Category: "builtins",
		Snippet: `type cd; type if; type ls`,
		Why:     "the same lookup `command -v` does, said in a sentence for a person to read — and every part of the sentence is worded differently: a keyword is `a shell keyword`, `a keyword` or `a reserved word`, and one shell reports an external as a tracked alias for the path",
	},
	{
		ID: "type/a-name-that-is-nothing", Category: "builtins",
		Snippet: `type nope; echo "st=$?"`,
		Why:     "four wordings and two statuses, and two of the four write this line with no shell name in front of it where every other message they print carries one. The status is a plain failure in three and a missing command's 127 in the fourth",
	},
	{
		ID: "type/several-names-and-a-double-dash", Category: "builtins",
		Snippet: `type -- cd ls; echo "st=$?"`,
		Why:     "`--` ends the options in three of them and is a name in the fourth, which has no options for `type` at all — so it answers about `--` first and then about the names, and reports the failure",
	},
	{
		ID: "type/dash-t-names-the-kind", Category: "builtins",
		Snippet: `f(){ :; }; type -t f; type -t cd; type -t if; type -t ls; echo "st=$?"`,
		Why:     "the scripted form of the question: one shell answers `-t` with one bare word per name — function, builtin, keyword, file, never the path — where its plain `type` writes sentences. The other three have no `-t` at all, and each declines its own way: two refuse the letter — one of them naming the builtin it aliases and printing that usage line — and one has no options here, so `-t` is a name and gets answered as one",
	},
	{
		ID: "type/dash-t-on-nothing-is-silent-failure", Category: "builtins",
		Snippet: `type -t a-name-that-is-nothing; echo "st=$?"`,
		Why:     "the kind of nothing is silence: where plain `type` writes a not-found line, `-t` writes no line and no diagnostic, and the failing status is the whole of the answer — which is the half a script branches on, and exactly what prose-parsing was adopted to avoid",
	},
	{
		ID: "param/a-default-holding-a-space", Category: "expansion",
		Snippet: `x=${U:-grep -E}; echo "[$x]"; y=${U:-a  b}; echo "[$y]"`,
		Why:     "everything up to the closing brace belongs to the operand, blanks included — `${GREP:-grep -E}` is the usual spelling and losing the space turns it into a command nobody has. A run of them is one run and not one space, which is what says the text was kept rather than rebuilt",
	},
	{
		ID: "param/a-default-holding-a-hash", Category: "expansion",
		Snippet: `echo "[${U:-a #b}]"`,
		Why:     "a `#` inside an operand starts no comment, so the text after it is text. The same rule as the space and the same way of getting it wrong: an operand read as though it were a command line",
	},
	{
		ID: "shell/naming-itself-in-a-variable", Category: "expansion",
		Snippet: `[ -n "$BASH_VERSION$ZSH_VERSION$KSH_VERSION" ] && echo named || echo anonymous`,
		Why:     "three of the four say which shell they are in a variable and the fourth says nothing, which is how a script tells them apart before it uses anything. The value differs between builds and this asks only whether there is one, which is the part every shell agrees on",
	},
	{
		ID: "shell/a-version-gate", Category: "expansion",
		Snippet: `case ${BASH_VERSION:-0} in 5.*) echo five;; 0) echo none;; *) echo other;; esac`,
		Why:     "what bats and brew actually do: read the number and decide whether to run at all. A shell claiming to be bash has to answer this the way bash does or it never reaches the part of the script that would tell it apart",
	},
	{
		ID: "callstack/a-function-knows-it-is-one", Category: "expansion",
		Snippet: `f(){ echo "[${FUNCNAME[*]}] n=${#FUNCNAME[@]}"; }; f; echo "outside=[${FUNCNAME[*]}]"`,
		Why:     "the stack of functions a shell is inside, which one dialect names and the rest do not have at all — and which is absent outside a function rather than empty, a different thing to a script testing it",
	},
	{
		ID: "callstack/nesting-is-innermost-first", Category: "expansion",
		Snippet: `g(){ echo "${FUNCNAME[*]}"; }; f(){ g; }; f`,
		Why:     "the order is the one a stack is printed in, and it is what a script reads to find out who called it. The bottom entry is the shell's own frame, which is there only when the shell was given a file to run",
	},
	{
		ID: "export/unset-f-removes-a-function", Category: "builtins",
		Snippet: `f(){ echo F; }; unset -f f; f 2>/dev/null; echo "st=$?"`,
		Why:     "unanimous, and it was read and then ignored here: the option was accepted, the function survived being unset, and it went on answering to its name. Plain `unset f` is a different question and the shells split on it",
	},
	{
		ID: "export/a-function-through-the-environment", Category: "builtins",
		Snippet: `f(){ echo carried; }; export -f f 2>/dev/null; env | grep -c '^BASH_FUNC'`,
		Why:     "one shell carries a function to its children and the other three have no way to: there is nothing but a string in an environment, so the source goes in and is parsed again at the other end. The count rather than the text, because what the entry holds is a shell's own spelling of a body",
	},
	{
		ID: "export/the-n-option-takes-the-attribute-off", Category: "builtins",
		Snippet: `V=1; export V; export -n V; echo "st=$?"; env | grep -c "^V="; echo alive`,
		Why:     "the letter itself is the question, not what it does: bash has `-n` and the other three refuse it as an option, in three different ways and two of them fatally. Left out of #459 deliberately, because that change was about the export attribute and this row would have been recording an option-surface gap instead — which it now is, on purpose. Read through a real child, since what `-n` buys is that the name stays set in the shell and stops reaching one",
	},
	{
		ID: "export/a-name-that-is-not-a-function", Category: "builtins",
		Snippet: `export -f nope; echo "st=$?"`,
		Why:     "a name that is not a function now will not become one by being exported. The shells that have the option refuse it and the ones that do not read `-f` as something else entirely, which is the more interesting half",
	},
	{
		ID: "export/an-imported-name-keeps-the-attribute", Category: "builtins",
		Snippet: `TERM=changed; env | grep '^TERM='`,
		Why:     "a variable that arrived in the environment is exported by having done so, and POSIX has it keep the attribute for the life of the shell — so a plain reassignment changes what every later child is told, unanimously. Read through a real child rather than through `export -p`, because the damage is what the child gets: the new value landed in the shell's own table alone here, and `PATH=/new:$PATH; make` handed every command the PATH the shell started with while the shell's own lookup was already right",
	},
	{
		ID: "export/an-imported-name-reassigned-in-a-function", Category: "builtins",
		Snippet: `f() { TERM=changed; }; f; env | grep '^TERM='`,
		Why:     "an assignment with no `local` in front of it is an ordinary global one wherever it is written, and the attribute travels with the value: all four hand the child the function's value after the function has returned. The neighbor that would have made the fix half a fix, since a shell that only noticed assignments at the top level would pass the case above and fail this one",
	},
	{
		ID: "export/an-imported-name-reassigned-in-a-subshell", Category: "builtins",
		Snippet: `TERM=changed; (env | grep '^TERM=')`,
		Why:     "the same through a subshell, which is worth its own row here rather than in a real shell: ours is a cloned runner in one process where every panel member forks, so the attribute has to be carried across the clone by hand where the others get it from the kernel",
	},
	{
		ID: "export/an-imported-name-reassigned-is-listed", Category: "builtins",
		Snippet: `TERM=changed; export -p | grep -c -E "^(declare -x|export) TERM="`,
		Why:     "the listing has to agree with what the child gets, in whichever of the two spellings the shell uses. The two answers came apart: the environment kept the imported entry and the listing dropped the name entirely, because one was reading the attribute and the other the record of having set it",
	},
	{
		ID: "export/a-prefix-over-an-imported-name", Category: "builtins",
		Snippet: `TERM=prefixed env | grep '^TERM='; env | grep '^TERM='`,
		Why:     "a prefix over a name the shell inherited: the command sees the prefix and the next command sees what came in, unanimously. `cmd/assignment-prefix-is-transient` asks this of a shell variable and reads it back in the shell; this one asks it of an inherited name and reads it back through a real environment, which is where a superseding entry would be one of two rather than instead of the other",
	},
	{
		ID: "array/a-subscript-past-the-end", Category: "expansion",
		Snippet: `a=(x); a[5]=y; echo "n=${#a[@]} all=[${a[@]}]"`,
		Why:     "whether an unassigned subscript is an element. Two shells say an array is a map from subscript to value and this is an array of two; one walks the whole extent and finds the gap empty, giving five. Counting the gap as elements is what a list representation does, and it is nobody's answer",
	},
	{
		ID: "array/removing-one-element", Category: "expansion",
		Snippet: `a=(p q r); unset "a[2]"; echo "n=${#a[@]} all=[${a[@]}]"`,
		Why:     "the same question reached from the other side, and `unset a[i]` had been doing nothing at all: the subscript was read as part of the name, so a name that was never in the table was deleted from it. What the hole then looks like is the same axis",
	},
	{
		ID: "array/appending-to-an-element", Category: "expansion",
		Snippet: `a=(x y); a[-1]+=Q; printf "[%s]" "${a[@]}"; echo; a+=(z); printf "[%s]" "${a[@]}"; echo`,
		Why:     "`+=` is two operations sharing a spelling and the subscript is what tells them apart: with one it joins the element it names, without one it adds an element after the last. Both halves in one case because the bug was that the first did what neither does — it replaced the element — and a case showing only the array form would have passed throughout. The subscript is `-1` so the case says nothing about the array base: the last element is the last element in all three shells with arrays. bash 3.2 refuses it, which is about having no negative subscripts rather than about appending — `array/appending-to-an-element-inherits-the-base` is the row it answers",
	},
	{
		ID: "array/appending-to-an-element-inherits-the-base", Category: "expansion",
		Snippet: `a=(x y); a[1]+=Q; printf "[%s]" "${a[@]}"; echo`,
		Why:     "the base axis reaches the append: the same numeral names the second element in two shells and the first in the third, exactly as a plain `a[1]=Q` does. Appending is therefore not a form with a subscript rule of its own, which is the claim worth pinning before anything special-cases it",
	},
	{
		ID: "array/appending-to-an-unset-element", Category: "expansion",
		Snippet: `a[3]+=Q; echo "[${a[3]}]"`,
		Why:     "an element that was never assigned has nothing to append to, and all three place the value as it stands rather than refusing. Unanimous, and it is the half a fix is most likely to get wrong by reading an absent element as an error instead of as an empty one",
	},
	{
		ID: "array/appending-to-an-associative-element", Category: "expansion",
		Snippet: `typeset -A m; m[k]+=x; m[k]+=Q; echo "[${m[k]}]"`,
		Why:     "the declared form appends by key just as the indexed form appends by subscript — unanimous in the three that have the attribute. Both assignments are written with `+=` so that the first one also stands as the unset-key case, and so that dash, which has neither, reports the two identically",
	},
	{
		ID: "array/a-literal-with-a-space-before-it", Category: "expansion",
		SyntaxError: true,
		Snippet:     `a= (echo x); echo "n=${#a[@]} 0=${a[0]} 1=${a[1]}"`,
		Why:         "the `(` has to be adjacent to the `=` everywhere but ksh93, and ksh93 is not merely lenient about the space — it reads the whole thing as the *array literal*, leaving `a` holding `echo` and `x` rather than assigning an empty value and running a subshell. So the panel splits on what the text means and not only on whether it is accepted, and a parser cannot settle the construct on adjacency alone: adjacency decides which diagnostic, and the non-adjacent form falls through to the ordinary rule for a `(` after a word",
	},
	{
		ID: "array/a-literal-places-its-subscripts", Category: "expansion",
		Snippet: `a=([2]=c [1]=b); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "the ordinary way to build an array out of order, and unanimous in the three that have arrays: the value goes where the subscript says and the brackets are not part of it. It used to be kept as text — two elements reading `[2]=c` and `[1]=b` — which is the silent kind of wrong, because the array is the right length and only its contents are nonsense. Both subscripts are at or above every shell's first, so the case asks nothing about the base",
	},
	{
		ID: "array/a-literal-leaves-a-gap", Category: "expansion",
		Snippet: `a=([2]=c); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "placing at a subscript nothing has filled up to leaves the positions below it unassigned, which is the sparse-array axis reached through the literal rather than through `a[5]=y`: two shells count one element and the one that walks the extent counts the gap as well",
	},
	{
		ID: "array/a-literal-subscript-is-an-expression", Category: "expansion",
		Snippet: `i=2; a=([1+1]=c [i]=d); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "a subscript written inside a literal is evaluated in two of the three and kept as the text between the brackets in the other, which is the same characters meaning two different things — the `ArrayLiteralSubscriptIsAKey` axis. Both spellings evaluate to 2, so where they are expressions the second overwrites the first and one element comes back; where they are keys they are two different keys and two elements do. The count is what tells the readings apart, which is why it is printed",
	},
	{
		ID: "array/a-literal-repeats-a-subscript", Category: "expansion",
		Snippet: `a=([2]=c [2]=d); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "the same subscript twice is one element holding the later value, unanimously — the elements are placed in the order written rather than gathered and reconciled, which is the only reading under which the second wins",
	},
	{
		ID: "array/a-literal-mixes-subscripts-and-positions", Category: "expansion",
		Snippet: `a=(x [3]=y z); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "a bare element after a subscripted one continues from that subscript rather than from where the count had reached, so `z` lands one past `y`. Two of the three do this; ksh93 does not take the mixture at all and keeps the subscripted element as text, which is the divergence this case exists to record rather than a wording difference",
	},
	{
		ID: "array/appending-a-literal-with-a-subscript", Category: "expansion",
		Snippet: `a=([2]=c); a+=([5]=f); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "`+=` keeps what is there and the subscripted element still places rather than landing after the end, so the two elements are at 2 and 5 with nothing between. Unanimous in the three, and it is the combination a fix is likeliest to miss because each half works alone",
	},
	{
		ID: "array/a-literal-value-is-an-assignment-value", Category: "expansion",
		Snippet: `x="p q"; a=([2]=$x); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "the value of a subscripted element is not field-split, exactly as the right side of `a[2]=$x` is not — unanimous, and the opposite of a bare element in the same literal, which is a word and does split. So one set of parentheses holds two expansion rules and the subscript is what chooses between them",
	},
	{
		ID: "array/a-subscript-holding-an-expansion", Category: "expansion",
		Snippet: `a=(x y); i=1; a[$i]=Q; printf "[%s]" "${a[@]}"; echo`,
		Why:     "the ordinary way a loop writes an element, and it used to be a command name: the assignment scan looked only at the first span of the word, which ends at the bracket as soon as anything expands inside it. The result was a `command not found` and an untouched array, so the loop kept going and everything read back afterwards was stale. zsh writes the first element rather than the second, which is the array-base axis and not a disagreement about the form",
	},
	{
		ID: "array/a-subscript-expansion-spelled-three-ways", Category: "expansion",
		Snippet: `a=(p q r); i=1; a[$i]=A; a[${i}]=B; a[$((i))]=C; printf "[%s]" "${a[@]}"; echo`,
		Why:     "the three spellings of one subscript all name the same element, so the last written is what comes back and the array alone cannot tell them apart. What separates them is standard error: a shell that reads one of the three as a command name says so there, which is exactly how this failed — all three at once, because they are one gap and a case with only `$i` would pass a fix that special-cased the dollar",
	},
	{
		ID: "array/a-subscript-holding-a-command-substitution", Category: "expansion",
		Snippet: `a=(x y); a[$(echo 1)]=Q; printf "[%s]" "${a[@]}"; echo`,
		Why:     "the subscript is a word and not a numeral, so a command substitution stands in one — the shape that shows the parser has to keep the spans rather than flatten the brackets to text. Unanimous in the three with arrays, at the base each of them counts from",
	},
	{
		ID: "array/appending-through-a-subscript-holding-an-expansion", Category: "expansion",
		Snippet: `a=(x y); i=1; a[$i]+=Q; printf "[%s]" "${a[@]}"; echo`,
		Why:     "`+=` after an expanded subscript, which is the combination that reaches both halves of the assignment scan at once: the `]` and the `+=` after it are in a span the old scan never looked at. Joining rather than replacing is `array/appending-to-an-element`'s question and the base is `array/appending-to-an-element-inherits-the-base`'s; this case is only about the form parsing at all",
	},
	{
		ID: "array/a-subscript-where-there-are-no-arrays", Category: "expansion",
		Snippet: `a[1]=Q; echo done`,
		Why:     "the other side of the same gate. Where the dialect has no subscript the word is not an assignment at all and the shell looks for a command by that name, which is the answer the shell without arrays gives — so accepting the shape everywhere would have made this one silently assign instead of reporting. Three shells assign and say nothing; the fourth reports on standard error and carries on",
	},
	{
		ID: "array/appending-to-an-element-where-there-are-no-arrays", Category: "expansion",
		Snippet: `a[1]+=Q; echo done`,
		Why:     "the append half of the same gate, and the one that was accidentally right. `a[1]=Q` leaked into the no-array dialect while this form did not, because `AppendAssign` is off there and refused it by another road — so the two forms agreed with the shell for different reasons and only one of them was gated on having arrays. Pinned separately so a dialect that ever takes `+=` without arrays cannot make this the leak the plain form was",
	},
	{
		ID: "array/a-subscript-below-the-first-element", Category: "expansion",
		Snippet: `a=(p q); a[0]=x; printf "[%s]" "${a[@]}"; echo " ok"`,
		Why:     "an assignment before the array's first element is refused, and the refusal ends the script at 1. Which subscript reaches it is the array base and nothing else: where the first element is 1 this writes nothing and stops, and where it is 0 the same numeral names the first element and nothing is wrong at all. It used to be a wording of our own at status 0 with the script running on, so the element was not written and everything after read the array as though it had been",
	},
	{
		ID: "array/appending-below-the-first-element", Category: "expansion",
		Snippet: `a=(p q); a[0]+=Q; printf "[%s]" "${a[@]}"; echo " ok"`,
		Why:     "`+=` is the same assignment reaching the same refusal rather than a form with a rule of its own, so the shell that refuses `a[0]=x` refuses this identically and the two that do not join the first element. It is the half a fix is likeliest to miss, because the append reads the element before it writes one",
	},
	{
		ID: "array/a-negative-subscript-past-the-start", Category: "expansion",
		Snippet: `a=(p q); a[-3]=x; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "the same boundary reached from the other side, and the one spelling the panel disagrees about: bash and ksh93 refuse a negative subscript that counts back past the first element and end the script, where zsh places one in front of every other and the array grows by exactly one. That is the axis; the refusal itself is not",
	},
	{
		ID: "array/a-negative-subscript-on-an-array-with-nothing-in-it", Category: "expansion",
		Snippet: `a[-1]=x; printf "[%s]" "${a[@]}"; echo " ok"`,
		Why:     "with no elements to count back from, `-1` is past the start already — so this is the shortest reachable form of the refusal in the two shells whose first element is 0, and the shortest form of the placement in the one that places. No array is built first, which is what makes it a statement about the boundary rather than about any particular length",
	},
	{
		ID: "array/a-refused-subscript-is-named-as-written", Category: "expansion",
		Snippet: `x=1; a[x-2]=v; echo ok`,
		Why:     "what the refusal names, which is three answers: bash quotes the subscript back as it was written — `a[x-2]`, not the -1 it came to — ksh93 names the array alone, and zsh has nothing to refuse here because a negative subscript counts back from the last element there. The expression is what tells the written text from the evaluated number; a bare `-2` could not",
	},
	{
		ID: "array/a-literal-subscript-below-the-first-element", Category: "expansion",
		Snippet: `a=([0]=p); printf "[%s]" "${a[@]}"; echo " ok"`,
		Why:     "the same refusal reached through a literal, which the shell that refuses it words differently from the plain form — naming the subscript and the kind of assignment rather than the array. Two spellings of one rule needing two sentences is why the wording is a field of its own rather than the plain one used twice",
	},
	{
		ID: "array/a-subscript-that-will-not-evaluate", Category: "expansion",
		Snippet: `a=(x y z); echo "[${a[b c]}]"; echo after`,
		Why:     "a subscript is an expression, so one that does not read is the failure `$((b c))` is — the identical sentence in all four, the command abandoned, and a non-zero status. It expanded to nothing at status 0 and the script carried on, which is the worst shape available: an empty string is a plausible value for a real element, so nothing downstream could tell. `after` is printed so the case records that the input unit is given up on rather than only that a line went to standard error",
	},
	{
		ID: "array/a-subscript-that-will-not-parse", Category: "expansion",
		Snippet: `a=(x y z); echo "[${a[1+]}]"; echo after`,
		Why:     "the other half of the same reading: an expression can fail before it is evaluated as well as while it is, and the panel words the two differently — `operand expected` against `operator expected`, in each shell's own sentence. Both spellings had one silent answer here, so a fix that only caught the evaluator would leave this one empty at 0",
	},
	{
		ID: "array/a-length-through-a-subscript-that-will-not-evaluate", Category: "expansion",
		Snippet: `a=(x y z); echo "[${#a[b c]}]"; echo after`,
		Why:     "the length operator reaches the element through the same reading, and reported `0` for it — a plausible length for a real element, where the shells all refuse the word. The operator forms are worth one row between them because they share the subscript path rather than each having one",
	},
	{
		ID: "array/assigning-through-a-subscript-that-will-not-evaluate", Category: "expansion",
		Snippet: `a=(x y z); a[1+]=v; printf "[%s]" "${a[@]}"; echo " after"`,
		Why:     "writing an element names it by expression too, and the failure ends the script in all four — unanimously, where the same failure inside `unset` splits them. It had a wording of its own that named the array rather than the expression, and it carried on to the next command, so the array a script thought it had written was untouched and nothing stopped",
	},
	{
		ID: "array/unsetting-through-a-subscript-that-will-not-evaluate", Category: "expansion",
		Snippet: `a=(x y z); unset "a[1+]"; echo "st=$?"; echo " n=${#a[@]}"`,
		Why:     "the same expression in `unset`, which is where the panel divides: bash gives up on the script as it does for any bad expression, and ksh93 and zsh leave a failed builtin behind and run the next command — the shape a script can test. ksh93 also names the builtin in front of the sentence, having worded the identical failure in an expansion without one. It was silent at 0 in all three",
	},
	{
		ID: "param/a-substring-offset-that-will-not-evaluate", Category: "parameter expansion",
		Snippet: `x=abcdef; echo "[${x:1+:2}]"; echo after`,
		Why:     "a substring's offset is the same reading reached by another spelling, and it took the failure silently: an offset of 0 is a real substring of the right length. What is *blamed* is three shapes rather than one — bash puts the parameter in front of the sentence, ksh93 names the offset together with everything after it in the range, and zsh gives the sentence bare",
	},
	{
		ID: "param/a-substring-length-that-will-not-evaluate", Category: "parameter expansion",
		Snippet: `x=abcdef; echo "[${x:2:1+}]"; echo after`,
		Why:     "the length rather than the offset, which is what separates the two namings: the shell that blames an offset along with the rest of the range has nothing after a length and names it alone. Extending the text before *evaluating* it rather than only before reporting it invented a second failure, so the pair is what pins that the extension is a wording and not a reading",
	},
	{
		ID: "param/a-substring-length-with-an-operand-it-found", Category: "parameter expansion",
		Snippet: `x=abcdef; echo "[${x:2:%}]"; echo after`,
		Why:     "the found-an-operand wording reached through a range rather than through `$(( ))`, which is where the two are held together: the shell that blames an offset along with the rest of the range also *reads* the rest of the range, so `${x:1+:2}` is its found case where `${x:1+}` is its ran-out one. A length has nothing after it and is the plain case in every column",
	},
	{
		ID: "param/a-substring-offset-that-names-a-variable", Category: "parameter expansion",
		Snippet: `x=abcdef; i=2; echo "[${x:i:2}]"; echo after`,
		Why:     "the ordinary way to walk a string, and one shell does not read it as a range at all: a range there is also its history-modifier syntax, so a segment beginning with a letter is a modifier and `i` names none of them — refused at 1, with the command abandoned. The other three evaluate the expression and answer `cd`, and we answered `cd` in every dialect, which is the silent half: a script that shell would have stopped ran on with a plausible substring",
	},
	{
		ID: "param/a-substring-offset-that-is-a-modifier", Category: "parameter expansion",
		Snippet: `x=/tmp/Dir/File.Txt; h=9; echo "[${x:h}]"`,
		Why:     "the same spelling where the letter *is* a modifier, which is what makes this a reading rather than a refusal: one shell answers the head of the path and the other three the substring from offset 9, because `h` is a variable to them. The variable is set on purpose — with `h` unset the three would answer from offset 0 and the case would look like a disagreement about the whole string rather than about what `h` means",
	},
	{
		ID: "param/a-substring-offset-that-does-not-begin-with-a-letter", Category: "parameter expansion",
		Snippet: `x=abcdef; _q=1; echo "[${x:_q:2}] [${x: _q:2}] [${x:(_q):2}]"`,
		Why:     "the boundary of that reading, measured three ways: an underscore is not a letter, a leading space puts the letter second, and a parenthesis does the same — so all three are ranges rather than modifiers, and the shell that has modifiers answers them exactly as bash does. It says the rule is about the first byte and not about the segment containing a name. ksh93 is the odd column and for an unrelated reason: it refuses a parenthesized offset outright, naming it with the parentheses backslashed",
	},
	{
		ID: "param/a-substring-length-that-names-a-variable", Category: "parameter expansion",
		Snippet: `x=abcdef; i=2; echo "[${x:2:i}]"; echo after`,
		Why:     "the length is the same question as the offset, and the shell with modifiers reads it the same way — the segment begins with a letter, so it is a modifier and not a count. Pinned separately because the offset and the length are evaluated by different call sites and a fix that caught one silently left the other",
	},
	{
		ID: "param/a-substring-modifier-after-an-offset", Category: "parameter expansion",
		Snippet: `x=/tmp/Dir/File.Txt; t=3; echo "[${x:2:t}]"`,
		Why:     "an offset and then a modifier, which is what shows the two readings are segments of one range rather than alternatives: the shell with modifiers takes the substring and then the tail of it, and the other three take two characters. It is also what a chain has to agree with — `${x:h:t}` is two modifiers by the same rule",
	},
	{
		ID: "param/a-substring-modifier-chain", Category: "parameter expansion",
		Snippet: `x=/tmp/Dir/File.Txt; h=9; t=9; echo "[${x:h:t}]"`,
		Why:     "two modifiers in one range, applied left to right — the head and then the tail of it. The parser splits a range once, so a chain arrives as one segment and a word holding the rest, and this is the case that says the rest is split again rather than evaluated whole",
	},
	{
		ID: "param/a-substring-modifier-with-something-after-it", Category: "parameter expansion",
		Snippet: `x=/tmp/Dir/File.Txt; echo "[${x:ha}]"; echo after`,
		Why:     "a segment is one modifier and the letter is the whole of it, so a good modifier with a letter after it is refused — and refused *without naming anything*, where an unknown first letter is named. The two shapes of one sentence are why this is a case: `${x:i:2}` names `i` and this names nothing, and a single wording would have to pick one",
	},
	{
		ID: "param/a-substring-offset-on-a-subscripted-parameter", Category: "parameter expansion",
		Snippet: `a=(p q r); echo "[${a[@]:1+}]"; echo after`,
		Why:     "the parameter a diagnostic names is the name and its subscript, not the name alone — `a[@]` — in the one shell that names it at all. The list form of the substring reaches the same evaluation as the string form, so this also says the two spellings share it",
	},
	{
		ID: "array/reading-a-subscript-is-arithmetic", Category: "expansion",
		Snippet: `a=(x y z); echo "[${a[1+1]}]"`,
		Why:     "a subscript being read is an expression, exactly as one being written through is. It took a numeral and nothing else, so this expanded to the empty string with status 0 — and `a[1+1]=v` had already learned to store where `${a[1+1]}` could not look, which is two spellings of one subscript naming two different elements. zsh answers the element before, which is the base rather than a different reading",
	},
	{
		ID: "array/a-subscript-reads-a-variable", Category: "expansion",
		Snippet: `a=(x y z); i=1; echo "[${a[i+1]}]"`,
		Why:     "the same question with a name in it, which is how a loop indexes relative to where it is. A bare name inside a subscript is an arithmetic operand and needs no `$`, so this is the spelling that separates evaluating the text from expanding it",
	},
	{
		ID: "array/an-unset-name-in-a-subscript", Category: "expansion",
		Snippet: `a=(x y z); echo "[${a[k]}]"`,
		Why:     "an unset name in an expression is zero, so the subscript is the first element rather than an error — unanimous in the three with arrays once the base is allowed for, which is why zsh finds nothing where the other two find the first. It is the row that says the subscript goes through the arithmetic reading rather than through a numeral test that happens to fail softly",
	},
	{
		ID: "array/an-element-length-by-expression", Category: "expansion",
		Snippet: `a=(xx yy zzz); echo "[${#a[1+1]}]"`,
		Why:     "the length operator reaches the element the expression names, so a subscript that did not evaluate reported the length of nothing — `0`, which is a plausible answer for a real element and so hides the failure completely",
	},
	{
		ID: "array/assigning-through-an-expression-subscript", Category: "expansion",
		Snippet: `a=(x y); echo "[${a[1+1]:=Q}]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "`${a[i]:=v}` assigns to the element the subscript names, so the expression has to be evaluated before the store as well as before the read. The subscript is past the end so the assignment happens, which is what tells this apart from a case that only reads. zsh finds an element there and assigns nothing, which is the base again",
	},
	{
		ID: "array/unsetting-an-element-by-expression", Category: "expansion",
		Snippet: `a=(x y z); unset "a[1+1]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "removing an element names it by expression too, and this did nothing at all: the operand was rejected for not being a numeral and then deleted as a whole variable that was never there, so the array came back unchanged with status 0. What the hole then looks like is the sparse-array axis, the same one `array/removing-one-element` records",
	},
	{
		ID: "array/an-unset-operand-subscript-expands", Category: "expansion",
		Snippet: `a=(x y z); i=1; unset 'a[$i]'; printf "[%s]" "${a[@]}"; echo`,
		Why:     "the operand reached `unset` unexpanded — it was in single quotes — and two of the three still substitute into it, because an arithmetic expression is expanded before it is read wherever one is written. ksh93 does not and says so, which is the divergence worth having recorded rather than discovered",
	},
	{
		ID: "array/unsetting-every-element", Category: "expansion",
		Snippet: `a=(p q r); unset "a[@]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "the spelling that starts a list over, and it did nothing at all: `@` is not an arithmetic expression, so the subscript failed to evaluate and the element nobody named was quietly not removed — the array came back with every element in place at status 0. Three answers among the shells that have arrays, which is why it is a policy and not a switch: bash empties it, zsh replaces the elements with a single empty one, and ksh93 has no such reading at all and reports the operand as a bad subscript with the array untouched",
	},
	{
		ID: "array/unsetting-every-element-with-a-star", Category: "expansion",
		Snippet: `a=(p q r); unset "a[*]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "the star is the at here, which is worth pinning because the two part company elsewhere — a quoted `${a[*]}` joins where `${a[@]}` splits. Every column answers this exactly as it answers the `[@]` spelling, so the case exists to say the two are one question rather than to record a difference",
	},
	{
		ID: "array/unsetting-every-element-then-appending", Category: "expansion",
		Snippet: `a=(p q); unset "a[@]"; a+=(z); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "where the next append lands, which is what shows the difference between the two shells that clear rather than only counting it: bash has nothing left and `z` is the whole array, zsh has one empty element left and `z` goes after it. A count alone cannot tell an emptied array from one holding a single empty string, and a script that resets a list and pushes onto it sees the difference on the first read",
	},
	{
		ID: "array/removing-the-last-element", Category: "expansion",
		Snippet: `a=(x y z); unset "a[3]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "the same axis as `array/unsetting-every-element` at a span of one, and the only place its two readings can be told apart. `array/removing-one-element` cannot see it: a removed subscript in the middle reads back empty under a dense reading, so removing and blanking leave the same array there. At the *end* they do not — removing shrinks the extent and blanking does not — and the length was coming back one short in the shell that blanks, which puts every later count and every append one place out. The subscript is `3` so the base decides who is being asked: it is the last element where the first is 1 and one past the end where the first is 0, which is why the two columns hold the same length for opposite reasons",
	},
	{
		ID: "array/removing-the-last-element-then-appending", Category: "expansion",
		Snippet: `a=(p q r); unset "a[3]"; a+=(z); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "where the next append lands, which is the half a script feels rather than counts: the blanked element still holds a place, so `z` goes after it and the list a script built by pushing is one longer than it thinks. The same probe `array/unsetting-every-element-then-appending` uses on the whole array, at one element",
	},
	{
		ID: "array/removing-the-only-element", Category: "expansion",
		Snippet: `a=(only); unset "a[1]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "an array of one, where blanking and removing part company most sharply — one comes back holding an empty element and the other is untouched, because `1` is the first element under one base and the second under the other. It says that a blanked array is not an empty one, which is the same distinction `array/unsetting-every-element-of-a-scalar` draws from the other side",
	},
	{
		ID: "array/removing-an-element-from-the-end", Category: "expansion",
		Snippet: `a=(x y z); unset "a[-2]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "an end-relative subscript, which the shell that blanks does not act on at all beyond `-1` — the array comes back whole where the shells that remove take the middle element away. Pinned because the blanking rule would otherwise be applied to every negative subscript by symmetry, and it is not. bash 3.2 has no negative subscripts and reports a bad one, which is the same absence `array/appending-to-an-element` records",
	},
	{
		ID: "array/unsetting-below-the-first-element", Category: "expansion",
		Snippet: `a=(x y z); unset "a[0]"; echo "st=$?"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "`array/a-subscript-below-the-first-element`'s boundary reached from `unset` rather than from an assignment, and the base decides who is being asked exactly as it does there: where the first element is 1 this names nothing and is refused, and where it is 0 the same numeral is the first element and it is removed. What the two routes do *not* share is the ending — the assignment stops the script and this leaves a failed builtin behind for the next command to test — so a fix that reused the fatal path would have been a new bug. It was silent at 0, which told a script it had removed something out of reach",
	},
	{
		ID: "array/unsetting-past-the-start", Category: "expansion",
		Snippet: `a=(x y z); unset "a[-4]"; echo "st=$?"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "the same boundary from the other side, which is the only side the shells whose first element is 0 can reach it from. The shell that blanks is silent here rather than refusing, and that is not a second rule: blanking replaces a span that is there, and a subscript counting back past the start names none — the same reading `array/removing-an-element-from-the-end` records at `-2`. bash 3.2 refuses it for having no negative subscripts at all, which is a different reason for the same line",
	},
	{
		ID: "array/an-unset-subscript-refused-is-named-as-written", Category: "expansion",
		Snippet: `a=(x y z); unset "a[x-9]"; echo "st=$?"; echo "n=${#a[@]}"`,
		Why:     "what the refusal names on this route, which is not what it names on the assignment's: bash keeps the subscript as written but drops the array in front of it, ksh93 names the array alone, and both put the builtin's name before the sentence where neither does for an assignment. The expression is what tells the written text from the -9 it came to. zsh has nothing to refuse, a negative subscript reaching nothing being silent there",
	},
	{
		ID: "array/unsetting-every-element-of-a-scalar", Category: "expansion",
		Snippet: `a=hello; unset "a[@]"; echo "st=$? [$a]"`,
		Why:     "the same spelling on a name that is no array, which is where the two readings show what they mean: bash means take every element away, a scalar has none, and it refuses and says so at 1; zsh means the span becomes one empty string, a scalar is one such span, and it comes back empty at 0. ksh93 reports its bad subscript and leaves the value alone. Nobody turns the scalar into an array",
	},
	{
		ID: "param/a-substring-offset-is-an-expression", Category: "parameter expansion",
		Snippet: `x=abcdef; echo "[${x:1+1:2}]"`,
		Why:     "the same numeral-only reading, reached through the substring rather than through a subscript: unanimous in all four with substrings, and taking the numeral alone gave an offset of 0 — `ab`, which is a real substring of the right length and so looks like an answer rather than a failure",
	},
	{
		ID: "array/a-quoted-gap-is-one-field", Category: "expansion",
		Snippet: `a=(x); a[5]=y; set -- "${a[0]}" "${a[1]}" "${a[5]}"; echo "n=$#"`,
		Why:     "quoting guarantees exactly one field, and it does not stop guaranteeing it because the element is not there — an unassigned subscript in quotes is one empty field, the same as `\"$unset\"`, unanimously in the three with arrays. It produced no field at all, so the count came back 2 and every argument after the gap moved up one: the script keeps running with everything off by one, which is the worst shape a wrong answer takes",
	},
	{
		ID: "array/a-quoted-gap-keeps-its-place", Category: "expansion",
		Snippet: `a=(x); a[5]=y; printf "[%s]" "${a[0]}" "${a[1]}" "${a[5]}"; echo`,
		Why:     "the same three expansions read as arguments rather than counted, which is what shows *where* the missing field was: the empty one is between the two that are there. A count alone cannot say that, and the shifting is the whole harm. zsh puts the gap first because its first element is 1",
	},
	{
		ID: "array/an-unquoted-gap-is-no-field", Category: "expansion",
		Snippet: `a=(x); a[5]=y; set -- ${a[0]} ${a[1]} ${a[5]}; echo "n=$#"`,
		Why:     "the other side, and the reason the first is about quoting rather than about arrays: unquoted, an empty expansion is no field at all and the count is 2 in all three. The pair is what says the rule is `\"$x\"` versus `$x` applied to an element",
	},
	{
		ID: "array/a-subscript-on-a-name-that-is-no-array", Category: "expansion",
		Snippet: `set -- "${b[3]}"; echo "n=$#"`,
		Why:     "a subscript may be written on a name that was never an array, and quoted it is still one empty field rather than none — the same guarantee reached without any array at all, so a fix that only counted gaps inside one would miss it",
	},
	{
		ID: "array/a-quoted-empty-array-is-not-the-same-question", Category: "expansion",
		Snippet: `a=(); set -- "${a[@]}"; echo "n=$#"`,
		Why:     "how many fields an empty *list* makes is a dialect's answer — two shells say none and ksh93 says one, which is why careful scripts write `\"${a[@]+\"${a[@]}\"}\"`. It is recorded next to the gap cases because the two were being answered by one test: asking this axis about a single subscript is what made a gap disappear, and only this spelling is entitled to ask it",
	},
	{
		ID: "array/a-quoted-empty-array-with-a-star", Category: "expansion",
		Snippet: `a=(); set -- "${a[*]}"; echo "n=$#"`,
		Why:     "`[*]` joins, so a quoted one is a single field whether or not there is anything to join — one, unanimously, where `[@]` splits the panel. The two spellings differing on an empty array is the sharpest statement that the star is not the at",
	},
	{
		ID: "assoc/unsetting-at-is-a-key-and-not-every-element", Category: "expansion",
		Snippet: `typeset -A m; m[k]=v; m[j]=w; unset "m[@]"; echo "n=${#m[@]}"`,
		Why:     "the whole-array reading belongs to the indexed array alone. With the attribute on, `@` is a key like any other and nothing was stored under it, so all three that have the attribute leave both elements where they are — including the two that clear an indexed array through the same spelling. It is the boundary a fix is likeliest to cross by accident, because the two kinds share a builtin and an operand shape",
	},
	{
		ID: "assoc/a-missing-key-quoted-is-one-field", Category: "expansion",
		Snippet: `typeset -A m; m[k]=v; set -- "${m[nokey]}"; echo "n=$#"`,
		Why:     "the same guarantee where the subscript is a key rather than an index: a key nothing was stored under is one empty field in quotes, in all three that have the attribute. The declared path had its own reading of an absent element and gave no field either",
	},
	{
		ID: "cmd/a-name-broken-by-an-expansion", Category: "commands",
		Snippet: `b=X; a$b=c; echo "rc=$?"`,
		Why:     "a name interrupted by an expansion is not a name, unanimously in all five: the word is a command name and the expansion happens first, so the diagnostic reports `aX=c`. It is the boundary the subscript form has to stop at — a scan that follows the `=` across spans wherever it finds one would turn this into an assignment to `a`",
	},
	{
		ID: "assoc/a-string-subscript", Category: "expansion",
		Snippet: `typeset -A m; m[k]=v; echo "${m[k]}" "${!m[@]}"`,
		Why:     "the declaration that turns a subscript from an expression into a key. bash and ksh93 store under the letter and answer it back; zsh has the arrays but rejects `${!m[@]}` outright; dash has none of it — the issue's own snippet, spelled with the name all three declarers share",
	},
	{
		ID: "assoc/the-subscript-is-not-arithmetic", Category: "expansion",
		Snippet: `typeset -A m; m[1+1]=x; echo "[${m[1+1]}]"`,
		Why:     "the sharpest edge of the attribute: the same characters that name element 2 of an indexed array name the three-character key of a declared one, unanimously in the shells that have it — so the switch has to be thrown before the subscript is read, not after",
	},
	{
		ID: "assoc/values-in-some-order", Category: "expansion",
		Snippet: `typeset -A m; m[b]=2; m[a]=1; printf "%s\n" "${m[@]}" | sort | tr "\n" " "; echo "n=${#m[@]}"`,
		Why:     "`${m[@]}` is one field per element and `${#m[@]}` counts them, exactly as for an indexed array. Sorted before comparing because no shell promises an order — bash's own moves between versions — and a case that depended on one would pin an accident",
	},
	{
		ID: "assoc/a-compound-literal-and-unset", Category: "expansion",
		Snippet: `typeset -A m; m=([a]=1 [b]=2); unset "m[a]"; echo "n=${#m[@]} b=${m[b]} a=${m[a]:-gone}"`,
		Why:     "the `([k]=v …)` literal keys its elements rather than counting them, and `unset m[k]` takes one key rather than doing nothing — unanimous in the three that have the attribute, and the removed element is gone rather than empty",
	},
	{
		ID: "subst/a-case-inside-a-substitution", Category: "expansion",
		Snippet: `x=$(case a in a) echo yes;; esac); echo "[$x]"`,
		Why:     "where a substitution ends is a question about the grammar and not about how many parentheses have been counted: an arm's `)` closes nothing, so counting stops early and takes half the arm with it. Unanimous, and the shape that made two installed scripts parse into a tree nobody wrote",
	},
	{
		ID: "subst/a-substitution-inside-an-arm", Category: "expansion",
		Snippet: `case a in a) echo "[$(echo inner)]";; esac`,
		Why:     "the other nesting, which counting got right and which has to keep working: the parentheses here really do pair",
	},
	{
		ID: "heredoc/no-delimiter-and-a-warning", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "cat <<X\nbody\n",
		Why:        "one shell remarks on a here-document whose delimiter never arrived and three say nothing. The remark is located where the input ran out and names the line the here-document began on, which is two different lines and the reason a parse-time remark carries two positions",
	},
	{
		ID: "heredoc/no-delimiter-and-no-body", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "cat <<X\n",
		Why:        "the same with nothing between: the remark then names the here-document's own line in both places, which is what says the location is the last line that had something on it rather than one past the end",
	},
	{
		ID: "heredoc/a-body-that-runs-to-the-end", Category: "redirection",
		Snippet: `x=$(cat <<EOF
body
EOF
); echo "[$x]"`,
		Why: "a delimiter that does arrive, as the control for the one below it — and with standard error no longer discarded, that nobody warns when it does",
	},
	{
		ID: "heredoc/a-delimiter-that-never-matches", Category: "redirection",
		Snippet: `{ x=` + "`" + `cat <<EOF
a)
 EOF` + "`" + `; } 2>/dev/null; echo "[$x]"`,
		Why: "the terminator has a leading space, so it is not the delimiter and the body runs to the end of the input. Every shell takes it and runs the command — this made it a syntax error. Standard error is still discarded here, and the reason changed: the warning about it is now produced, but this body is inside a backquoted substitution, which the one shell that warns re-parses while carrying its line counter on from the outer input — so a three-line script is remarked on at line 5. The two cases above pin the warning where the lines are the file's own",
	},
	{
		ID: "redir/open-failure-wording", Category: "redirection",
		Snippet: `cat < nosuchfile; echo "st=$?"`,
		Why:     "all four word a failed open differently and only two of them use a verb: bash prints the name then the OS string, dash puts `cannot open` in front, ksh93 puts the name first and brackets the reason after it, and zsh prints the reason first, lowercased. dash also writes its own text for this errno — `No such file`, where the OS says `No such file or directory`",
	},
	{
		ID: "redir/failure-line-when-the-redirect-is-elsewhere", Category: "redirection",
		LayoutSensitive: true,
		Snippet: `echo one
while read -r x
do
  echo $x
done < nosuchfile
echo "st=$?"`,
		Why: "three answers about which line a failed open is reported at, and they only differ when the redirect is not on the line its command began on. bash names the redirect's own line, dash and zsh name the line the command opened on, and ksh93 names the line before the redirect's — the same one-off in a loop, a brace group and a backslash continuation alike. Found on an installed script that opens a `while read` on line 5 and redirects it on line 11",
	},
	{
		ID: "redir/failure-line-across-a-continuation", Category: "redirection",
		LayoutSensitive: true,
		Snippet:         "echo one\ncat \\\n< nosuchfile\necho \"st=$?\"",
		Why:             "a simple command split by a backslash, with its redirect two physical lines below the command word. All four name the command's line, which is what shows that this axis only ever answers about a *compound* command — and reading the redirect's own position here gave bash the wrong line",
	},
	{
		ID: "redir/failure-line-for-a-compound-on-one-line", Category: "redirection",
		LayoutSensitive: true,
		Snippet: `echo one
{ echo hi; } < nosuchfile
echo "st=$?"`,
		Why: "a compound command and its redirect on the same line, which is what separates ksh93's rule from the others': it steps back a line for a compound command whatever the layout, so this says line 1 there — printing no line at all — and line 2 in the other three. The loop shape alone could not show it, because with the redirect already on a later line, stepping back and naming the command's line agree",
	},
	{
		ID: "diag/a-line-worth-naming", Category: "diagnostics",
		LayoutSensitive: true,
		Snippet: `echo one
nosuchcmd
echo "st=$?"`,
		Why: "ksh93 names the line under `-c` only after the first: `ksh: nosuchcmd: not found` on line 1 and `ksh: line 2: nosuchcmd: not found` here. Every earlier measurement used a one-line `-c`, where naming no line and naming line 1 are the same output",
	},
	{
		ID: "redir/create-failure-says-create", Category: "redirection",
		Snippet: `mkdir d; echo x > d; echo "st=$?"`,
		Why:     "two of the four say `create` rather than `open` when the redirect was making the file — dash and ksh93 — where bash and zsh word it identically either way. A directory is the way to fail a create without needing a permission the test machine may or may not have",
	},
	{
		ID: "redir/create-failure-reason-diverges", Category: "redirection",
		Snippet: `echo x > nodir/out; echo "st=$?"`,
		Why:     "the same errno as a failed open, and dash alone gives it a different name depending on the direction: `Directory nonexistent` for a write against its own `No such file` for a read, where the OS says `No such file or directory` for both",
	},
	{
		ID: "redir/failure-does-not-run-the-command", Category: "redirection",
		Snippet: `echo reached > nodir/out; echo "st=$?"`,
		Why:     "the redirect is set up before the command runs, so a redirect that cannot be opened means the command does not run at all — nothing prints but the status, unanimously, and 1 rather than the 2 a syntax error would give",
	},
	{
		ID: "redir/dup-to-stderr", Category: "redirection",
		Snippet: `{ echo hi >&2; } 2>/dev/null; echo done`,
		Why:     "`>&2` sends to stderr, so discarding stderr discards it — the check that the duplication happened rather than the word being an argument",
	},
	{
		ID: "redir/exec-saves-a-stream", Category: "redirection",
		Snippet: `exec 6>&1; echo through >&6; exec 6>&-; echo "st=$?"`,
		Why:     "the saved-stdout idiom every configure script uses: `exec 6>&1` keeps the stream, `>&6` finds it again, `6>&-` lets it go",
	},
	{
		ID: "redir/read-write-opens-without-truncating", Category: "redirection",
		Snippet: `printf 'keep\n' > f; exec 3<> f; exec 3>&-; cat f; exec 4<> made; echo "st=$?"; ls made`,
		Why:     "`<>` opens for reading and writing, creates a file that is not there, and never truncates one that is — unanimous, and the reason lock and state files use it",
	},
	{
		ID: "redir/a-write-to-a-closed-descriptor-fails", Category: "redirection",
		Snippet: `echo hi >&-; echo "st=$?"`,
		Why:     "`>&-` closes stdout before the builtin writes, and a write that went nowhere is not a command that worked: three shells report 1 — two of them with a message naming the builtin, ksh93 silently — and zsh alone keeps 0 and quietly loses the text",
	},
	{
		ID: "redir/a-group-writing-to-a-closed-descriptor", Category: "redirection",
		Snippet: `{ echo a; echo b; } >&-; echo "st=$?"`,
		Why:     "the failure is per write, never fatal: each echo inside the group fails on its own — bash and dash complain twice — and the group reports the last one. zsh says `write error` here where it said nothing for a simple command's own `>&-`, and still answers 0",
	},
	{
		ID: "redir/exec-opens-a-high-descriptor", Category: "redirection",
		Snippet: `exec 3>f; echo hi; echo aside >&3; exec 3>&-; cat f`,
		Why:     "`exec 3>file` holds the file on a descriptor of its own — stdout stays where it was, and only what is aimed at 3 reaches the file",
	},
	{
		ID: "redir/a-two-digit-descriptor-number", Category: "redirection",
		Snippet: `exec 10>f; echo hi >&10; exec 10>&-; cat f`,
		Why:     "how many digits a descriptor number may have is not unanimous: bash reads ten as a number, and to dash, ksh93 and zsh the digits are an ordinary word, so the line runs a command called `10` with its output in the file. Those three still reach descriptors above nine through `exec {v}>f`, which is what makes this a question about the token rather than about the table",
	},
	{
		ID: "redir/digits-before-a-redirection-that-are-not-a-number", Category: "redirection",
		Snippet: `echo x 10>f; echo "st=$?"; cat f`,
		Why:     "the same split read from the quiet side, and the reason it is a grammar flag rather than a refusal: where the digits are a word nothing is reported and a different command runs — `x 10` into the file, against `x` on the terminal and an empty file",
	},
	{
		ID: "redir/a-descriptor-number-over-the-open-file-limit", Category: "redirection",
		Snippet: `ulimit -n 64; exec 70>fresh; echo "st=$?"; ls fresh`,
		Why:     "no shell in the panel has a ceiling of its own — the bound is the kernel's limit on open files, and only some shells hand its refusal back. The limit is moved by the case rather than assumed, so the row is about the rule and not about the machine's default. bash refuses and still creates the file; the other three cannot write a two-digit number at all and run a command called `70`",
	},
	{
		ID: "redir/exec-descriptor-reaches-an-external-child", Category: "redirection",
		Snippet: `exec 3>f; /bin/sh -c "echo child >&3" 2>/dev/null; exec 3>&-; cat f`,
		Why:     "a descriptor parked with `exec 3>file` is inherited by an external command, which is what the flock and shared-log idioms are built on — the child writes in every shell but ksh93, which alone keeps it to itself. The child's complaint is discarded because its wording is a fact about whatever /bin/sh is on the machine",
	},
	{
		ID: "redir/a-commands-own-redirection-crosses", Category: "redirection",
		Snippet: `/bin/sh -c "echo own >&3" 3>f 2>/dev/null; echo "st=$?"; cat f`,
		Why:     "the boundary of the shell that keeps `exec`'s descriptors to itself: a redirection the *command* carries crosses in every shell, ksh93 included, so what that shell withholds is what `exec` opened rather than what the table holds",
	},
	{
		ID: "redir/restating-the-number-hands-an-exec-descriptor-over", Category: "redirection",
		Snippet: `exec 3>f; /bin/sh -c "echo restated >&3" 3>&3 2>/dev/null; echo "st=$?"; cat f`,
		Why:     "the same boundary read from the other side, and unanimous: `exec 3>f` alone leaves the child nothing in ksh93, and naming 3 again on the command hands it over there — so the rule is about which redirection list opened the descriptor and not about the number",
	},
	{
		ID: "redir/exec-descriptor-reaches-a-replacement", Category: "redirection",
		Snippet: `exec 3>f; exec /bin/sh -c "{ echo repl >&3; } 2>/dev/null; cat f"`,
		Why:     "the same descriptor and the harder seam: `exec cmd` replaces the shell rather than forking one, so nothing renumbers the table on the way across and the command inherits the *process's* descriptors. The replacement writes in every shell but ksh93, exactly as a child does, and the reading is done by the replacement because there is no shell left to do it. The complaint is discarded for the reason the child's is",
	},
	{
		ID: "redir/a-replacements-descriptor-numbers-keep-their-gaps", Category: "redirection",
		Snippet: `exec 5>f; exec /bin/sh -c "{ echo five >&5; echo three >&3; } 2>/dev/null; cat f"`,
		Why:     "the replacement's table is the shell's table by number rather than a packing of it: with 3 and 4 never opened, the file parked on 5 is on 5 there and 3 is closed rather than shifted down to fill the hole. Unanimous but for ksh93, which passes neither",
	},
	{
		ID: "redir/a-replacement-keeps-a-redirected-stdout", Category: "redirection",
		Snippet: `exec >f; exec /bin/sh -c "echo repl; cat f >&2"`,
		Why:     "the named streams cross a replacement as the rest of the table does, and unanimously — ksh93 included, which is the boundary of what that shell keeps to itself. The replacement reads the file back on the one stream still going to the terminal, because there is no shell left to read it",
	},
	{
		ID: "redir/a-replacement-keeps-a-redirected-stderr", Category: "redirection",
		Snippet: `exec 2>f; exec /bin/sh -c "echo err >&2; cat f"`,
		Why:     "the same claim for the stream a shell reports its own failures on, read back on the standard output the script left alone",
	},
	{
		ID: "redir/a-replacement-keeps-a-redirected-stdin", Category: "redirection",
		Snippet: `printf 'line\n' >f; exec <f; exec /bin/cat`,
		Why:     "the reading half: the replacement reads the file the script opened rather than the shell's own input, which with a terminal there is the difference between printing a line and hanging",
	},
	{
		ID: "redir/a-replacements-own-redirection-crosses", Category: "redirection",
		Snippet: `exec /bin/sh -c "echo own; cat f >&2" >f`,
		Why:     "the form a script is likelier to write — the redirection on the `exec` itself rather than on an `exec` before it — and the same answer, so the rule is about the stream and not about which command opened it",
	},
	{
		ID: "redir/a-replacement-keeps-a-merged-stream", Category: "redirection",
		Snippet: `exec >f 2>&1; exec /bin/sh -c 'echo out; echo err >&2; exit $(grep -c . f)'`,
		Why:     "`>f 2>&1` is one file under two numbers rather than two targets, so a replacement that places files by number gets both — the count is the only channel left once every stream is in the file, and it is 2 in every shell. The command substitution is the replacement's, quoted so that the shell being replaced does not run it against a file it has only just truncated",
	},
	{
		ID: "redir/a-multi-target-stream-crosses-a-replacement", Category: "redirection",
		Snippet: `exec >a >b; exec /bin/echo hi`,
		Why:     "a repeated redirection of the same stream, and then a replacement. Nothing is said and the status is 0 in all six, which is the fact this pins: four of them put `hi` in the last file and zsh puts it in both, and neither hands the command a closed descriptor. The files themselves are not recorded here — the shell is gone before anything could read them — so the case grades the command having run at all, which is what a stream that is no single number once cost it",
	},
	{
		ID: "redir/a-closed-stdout-is-closed-for-a-replacement", Category: "redirection",
		Snippet: `exec >&-; exec /bin/echo hi 2>/dev/null`,
		Why:     "a stream the script closed stays closed across the replacement rather than falling back to the process's own: the command fails where it would otherwise have written, unanimously. The complaint is discarded because its wording is a fact about whatever /bin/echo is on the machine",
	},
	{
		ID: "redir/a-closed-stdin-is-closed-for-a-replacement", Category: "redirection",
		Snippet: `exec <&-; exec /bin/cat 2>/dev/null`,
		Why:     "the same rule on the reading side, and the case that tells a closed stream from an empty one: a replacement handed the shell's own input would read to end of file and report success",
	},
	{
		ID: "redir/an-inherited-descriptor-keeps-its-number", Category: "redirection",
		Snippet: `exec 5>g; /bin/sh -c "echo three >&3" 2>/dev/null; /bin/sh -c "echo five >&5" 2>/dev/null; exec 5>&-; cat g`,
		Why:     "the discriminating case for how the table crosses: with 3 and 4 never opened, the file parked on 5 is still on 5 in the child and 3 is a hole, so the file holds `five`. A table packed from the bottom would put it on 3 and the file would hold `three`",
	},
	{
		ID: "redir/a-dup-prefix-is-that-commands-alone", Category: "redirection",
		Snippet: `true 6>&1; echo hi >&6; echo "st=$?"`,
		Why:     "a duplication prefixed to one command does not outlive it — only `exec`'s do",
	},
	{
		ID: "redir/merge-then-file", Category: "redirection",
		Snippet: `{ echo out; echo err >&2; } >f 2>&1; printf "[%s]" "$(cat f)"`,
		Why:     "both streams reach the file: stdout is redirected first, then stderr is pointed at where stdout now goes",
	},
	{
		ID: "redir/file-then-merge", Category: "redirection",
		Snippet: `{ echo out; echo err >&2; } 2>&1 >f; printf "[%s]" "$(cat f)"`,
		Why:     "the same two operators in the other order put only stdout in the file, because 2>&1 copied stdout before it was redirected — the classic one, and unanimous",
	},
	{
		ID: "redir/multios-is-zsh-only", Category: "redirection",
		Snippet: `echo x >a >b; printf "[%s][%s]" "$(cat a 2>/dev/null)" "$(cat b 2>/dev/null)"`,
		Why:     "zsh writes to every target and the others only to the last, with no error either way — the &> failure mode in a redirection: one spelling, two meanings, and no diagnostic to tell them apart",
	},
	{
		ID: "token/clobber-override", Category: "tokenization",
		Snippet: `set -C; echo one>b; echo two>|b; printf "[%s]" "$(cat b)"`,
		Why:     ">| overrides noclobber with the same meaning everywhere, unlike &>",
	},
	// --- the command language ---------------------------------------------
	{
		ID: "cmd/andor-equal-precedence", Category: "command language",
		Snippet: `true || echo A && echo B`,
		Why:     "the discriminating case: C's precedence would short-circuit and print nothing; the shell prints B",
	},
	{
		ID: "cmd/andor-left-to-right-success", Category: "command language",
		Snippet: `true && echo A || echo B`,
		Why:     "left to right: A runs and succeeds, so || is skipped",
	},
	{
		ID: "cmd/andor-left-to-right-failure", Category: "command language",
		Snippet: `false && echo A || echo B`,
		Why:     "and the failing && hands off to ||",
	},
	{
		ID: "cmd/bang-negates-the-pipeline", Category: "command language",
		Snippet: `! true | false; echo "st=$?"`,
		Why:     "the pipeline's status is false's, so ! yields 0; binding ! to true alone would give 1",
	},
	{
		ID: "cmd/pipeline-status-is-last", Category: "command language",
		Snippet: `false | true; echo "st=$?"`,
		Why:     "a pipeline reports its last command, not its first failure",
	},
	{
		ID: "cmd/pipeline-status-is-last-failing", Category: "command language",
		Snippet: `true | false; echo "st=$?"`,
		Why:     "the other direction, so the previous case cannot pass by accident",
	},
	{
		ID: "cmd/redirect-before-command-name", Category: "command language",
		Snippet: `>b echo hi; printf "[%s]" "$(cat b)"`,
		Why:     "a redirection may precede the command name",
	},
	{
		ID: "cmd/redirect-between-arguments", Category: "command language",
		Snippet: `echo one >b two; printf "[%s]" "$(cat b)"`,
		Why:     "and may sit between arguments; a parser treating redirections as a suffix is wrong",
	},
	{
		ID: "cmd/assignment-prefix-is-transient", Category: "command language",
		Snippet: `x=1; x=2 true; echo "[$x]"`,
		Why:     "an assignment prefix applies to that command's environment only",
	},
	{
		ID: "cmd/assignment-prefix-special-builtin", Category: "command language",
		Snippet: `x=1; x=2 export y=3; echo "[$x]"`,
		Why:     "except before a special builtin, where POSIX says it persists — dash and ksh93 comply, bash and zsh do not",
	},
	{
		ID: "cmd/assignment-prefix-reaches-a-builtin", Category: "command language",
		Snippet: `echo "a:b" | { IFS=: read x y; echo "[$x][$y]"; v="p q"; set -- $v; echo "n=$#"; }`,
		Why:     "transient is not invisible: the prefix is in effect while the builtin runs — `IFS=: read` splits on the colon — and is taken back after, so the later unquoted expansion splits on whitespace again",
	},
	{
		ID: "cmd/subshell-isolates-state", Category: "command language",
		Snippet: `x=1; (x=2); echo "[$x]"`,
		Why:     "( ) runs in a subshell, so assignments do not escape",
	},
	{
		ID: "cmd/brace-group-shares-state", Category: "command language",
		Snippet: `x=1; { x=2; }; echo "[$x]"`,
		Why:     "{ } runs in the current shell, which is the whole difference between them",
	},
	{
		ID: "cmd/brace-group-needs-terminator", SyntaxError: true, Category: "command language",
		Snippet: `{ echo a }`,
		Why:     "{ } is made of reserved words and needs a terminator before the brace — except in zsh",
	},
	{
		ID: "cmd/subshell-needs-no-terminator", Category: "command language",
		Snippet: `(echo a)`,
		Why:     "( ) is made of operators, so it needs neither blanks nor a terminator",
	},
	{
		ID: "cmd/compound-takes-redirection", Category: "command language",
		Snippet: `{ echo a; echo b; } >f; printf "[%s]" "$(tr '\n' ',' <f)"`,
		Why:     "a redirection on a compound command applies to everything inside it",
	},
	{
		ID: "cmd/loop-takes-redirection", Category: "command language",
		Snippet: `for i in 1 2; do echo $i; done >f; printf "[%s]" "$(tr '\n' ',' <f)"`,
		Why:     "so every compound AST node needs a redirection list, not just simple commands",
	},
	{
		ID: "cmd/loop-status-when-body-never-runs", Category: "command language",
		Snippet: `while false; do :; done; echo "st=$?"`,
		Why:     "zero iterations exits 0; \"status of the last command\" is the obvious wrong answer when there was none",
	},
	{
		ID: "cmd/for-status-empty-list", Category: "command language",
		Snippet: `for i in; do echo x; done; echo "st=$?"`,
		Why:     "same rule for an empty for list",
	},
	{
		ID: "cmd/case-fallthrough", Category: "command language",
		Snippet: `case a in a) echo one;& b) echo two;; esac`,
		Why:     ";& falls through to the next body; core, but absent from dash",
	},
	{
		ID: "cmd/case-continue-matching", Category: "command language",
		Snippet: `case a in a) echo one;;& a) echo two;; esac`,
		Why:     ";;& keeps testing later patterns and is bash-only — lumping it with ;& would put a bash construct in the core",
	},
	{
		ID: "cmd/case-fallthrough-chain", Category: "command language",
		Snippet: `case a in a) echo one;& b) echo two;& c) echo three;; esac`,
		Why:     "each arm reached by ;& has a terminator of its own to honor — honoring only the matched arm's ran one extra body and stopped",
	},
	{
		ID: "cmd/case-fallthrough-then-continue-matching", Category: "command language",
		Snippet: `case a in a) echo one;& b) echo two;;& c) echo three;; a) echo four;; esac`,
		Why:     "a ;& into an arm ending in ;;& goes back to pattern testing, not to falling — the two operators compose rather than alias",
	},
	{
		ID: "cmd/case-fallthrough-last-arm", Category: "command language",
		Snippet: `case a in x) echo no;; a) echo last;& esac`,
		Why:     ";& on the final arm has nothing to fall into and must end the case cleanly, not read past the item list",
	},
	{
		ID: "cmd/function-body-simple-command", Category: "command language",
		SyntaxError: true,
		Snippet:     `f() echo hi; f`,
		Why:         "bash alone wants a compound body after the parens; dash, ksh93 and zsh take the simple command as a one-command body and run it — being more permissive than bash here is the dangerous direction only for scripts aimed at bash",
	},
	{
		ID: "cmd/function-body-an-assignment", Category: "command language",
		SyntaxError: true,
		Snippet:     `f() x=1; f`,
		Why:         "the same refusal reached by a body that is not even a command name: the shell that wants a compound body names the assignment as the offending token, which the token has to be *saved* to do — the rule is only decided once the body has been read",
	},
	{
		ID: "cmd/function-body-a-redirection", Category: "command language",
		SyntaxError: true,
		Snippet:     `f() >out; f`,
		Why:         "a body of nothing but a redirection: the token blamed is the operator rather than the word after it, and two shells refuse it where two run it",
	},
	{
		ID: "cmd/function-body-a-command-with-a-redirection", Category: "command language",
		SyntaxError: true,
		Snippet:     `f() echo hi >out; f`,
		Why:         "the same refusal with a command in front of the operator, which says what the rule is: the shell that refuses `f() >out` is not asking for a command, it refuses a redirection in an uncompounded body at all — and it names the operator, with the `echo` already accepted. The two that run it write the line to the file and print nothing",
	},
	{
		ID: "cmd/function-body-compound-with-a-redirection", Category: "command language",
		Snippet: `f() { echo hi; } >out; f`,
		Why:     "the control for the row above: braces make the same redirection acceptable everywhere, including in the shell that refuses it on a bare command, so that refusal is about the *body* and not about redirecting a function",
	},
	{
		ID: "cmd/function-with-no-body-at-all", Category: "command language",
		SyntaxError: true,
		Snippet:     `f() ;`,
		Why:         "no grammar has a body here, refusing or permissive, and all four name the token rather than describing the function",
	},
	{
		ID: "cmd/function-parens-then-end-of-input", Category: "command language",
		SyntaxError: true,
		Snippet:     `f()`,
		Why:         "the input runs out with the parens already closed, so there is no construct left open to name — the shell that names one in its end-of-input sentence has to say something shorter here",
	},
	{
		ID: "cmd/function-name-with-a-dash", Category: "command language",
		Snippet: `f-g(){ echo ok; }; f-g; echo after`,
		Why:     "four answers: bash and zsh define and run it, dash refuses the name at parse time, ksh93 parses and stops the script at the definition",
	},
	{
		ID: "cmd/function-name-with-a-dot", Category: "command language",
		Snippet: `a.b(){ echo ok; }; a.b; echo after`,
		Why:     "the dot is its own sentence in ksh93 — an invalid discipline function — and the same split everywhere else",
	},
	{
		ID: "cmd/function-posix-form", Category: "command language",
		Snippet: `f() { echo posix; }; f`,
		Why:     "the universal definition form",
	},
	{
		ID: "cmd/function-keyword-form", Category: "command language",
		Snippet: `function f { echo kw; }; f`,
		Why:     "the ksh keyword form: core, absent from dash",
	},
	{
		ID: "cmd/function-keyword-and-parens", Category: "command language",
		Snippet: `function f() { echo both; }; f`,
		Why:     "the hybrid is rejected by ksh93, where the keyword originated, so it is not core",
	},
	// --- substitutions ------------------------------------------------------
	{
		ID: "subst/does-not-end-the-word", Category: "substitutions",
		Snippet: `set -- $(printf a)b; printf "[%s]" "$@"; echo " n=$#"`,
		Why:     "a substitution is part of a word, not a word of its own",
	},
	{
		ID: "subst/result-is-split-and-text-attaches", Category: "substitutions",
		Snippet: `set -- x$(printf "a b")y; printf "[%s]" "$@"; echo " n=$#"`,
		Why:     "the result is field-split and the literal text either side attaches to the first and last fields",
	},
	{
		ID: "subst/paren-in-quotes-does-not-close", Category: "substitutions",
		Snippet: `echo "[$(echo ")" )]"`,
		Why:     "the rule that decides the implementation: counting parens truncates the substitution and silently changes the program",
	},
	{
		ID: "subst/brace-in-quotes-does-not-close", Category: "substitutions",
		Snippet: `printf "[%s]" "${x:-"a}b"}"`,
		Why:     "the same rule one construct over: counting braces stops at the quoted } and leaves the rest of the expansion behind as text",
	},
	{
		ID: "subst/nesting", Category: "substitutions",
		Snippet: `echo "[$(echo "$(echo deep)")]"`,
		Why:     "nesting works because the scan tracks quoting, not because of a separate rule",
	},
	{
		ID: "subst/arith-vs-subshell", Category: "substitutions",
		Snippet: `echo "[$((1+2))] [$( (echo sub) )]"`,
		Why:     "$(( starts arithmetic, so a substitution beginning with a subshell needs the space — the only disambiguation available",
	},
	{
		ID: "subst/backticks-nest-with-escaping", Category: "substitutions",
		Snippet: "echo \"[`echo \\`echo deep\\``]\"",
		Why:     "the older form nests only with backslash escaping, which is why $( ) exists",
	},
	// --- compound command shapes ---------------------------------------------
	{
		ID: "shape/terminator-required-before-then", SyntaxError: true, Category: "compound shapes",
		Snippet: `if true then echo x; fi`,
		Why:     "the keyword does not delimit the condition; a ; or newline does, so the production needs a separator",
	},
	{
		ID: "shape/terminator-required-before-do", SyntaxError: true, Category: "compound shapes",
		Snippet: `while false do echo x; done`,
		Why:     "the same rule for loops, so it is a property of the grammar rather than of `if`",
	},
	{
		ID: "shape/condition-is-a-list-last-wins", Category: "compound shapes",
		Snippet: `if false; true; then echo yes; else echo no; fi`,
		Why:     "the condition is a list judged by its last command, not a single command",
	},
	{
		ID: "shape/condition-is-a-list-last-fails", Category: "compound shapes",
		Snippet: `if true; false; then echo yes; else echo no; fi`,
		Why:     "the other direction, so the previous case cannot pass by accident",
	},
	{
		ID: "shape/elif-chain", Category: "compound shapes",
		Snippet: `if false; then echo a; elif true; then echo b; else echo c; fi`,
		Why:     "elif is a chain rather than a nested if in the surface syntax",
	},
	{
		ID: "shape/for-omitted-list-is-positional", Category: "compound shapes",
		Snippet: `set -- x y; for i; do printf "[%s]" "$i"; done`,
		Why:     "omitting the word list iterates the positional parameters",
	},
	{
		ID: "shape/for-empty-list-is-no-iterations", Category: "compound shapes",
		Snippet: `set -- x y; for i in; do printf "[%s]" "$i"; done; echo "(none)"`,
		Why:     "which is not the same as an empty list — so the AST must distinguish absent from empty",
	},
	{
		ID: "shape/for-newline-separator", Category: "compound shapes",
		Snippet: "for i in a b\ndo printf \"[%s]\" \"$i\"; done",
		Why:     "a newline is a separator wherever ; is",
	},
	{
		ID: "shape/case-leading-paren", Category: "compound shapes",
		Snippet: `case x in (x) echo paren;; esac`,
		Why:     "a case pattern may carry a leading open paren",
	},
	{
		ID: "shape/case-pattern-alternatives", Category: "compound shapes",
		Snippet: `case x in a|x) echo alt;; esac`,
		Why:     "patterns alternate with |",
	},
	{
		ID: "shape/case-empty-body-and-no-match", Category: "compound shapes",
		Snippet: `case x in x) ;; esac; echo "empty=$?"; case x in y) echo no;; esac; echo "nomatch=$?"`,
		Why:     "an empty body is legal and a case matching nothing exits 0",
	},

	// --- [[ ]] and (( )) ------------------------------------------------------
	{
		ID: "cond/double-bracket-less-is-comparison", Category: "[[ ]] and (( ))",
		Snippet: `if [[ a < b ]]; then echo less; else echo notless; fi`,
		Why:     "inside [[ ]] the < is a comparison; in dash, which has no [[, the same text is a command with a redirection that opens a file",
	},
	{
		ID: "cond/double-bracket-needs-blanks", Category: "[[ ]] and (( ))",
		Snippet: `[[a == a]]`,
		Why:     "[[ is a reserved word rather than an operator, so it must be delimited by blanks",
	},
	{
		ID: "cond/double-bracket-andand", Category: "[[ ]] and (( ))",
		Snippet: `[[ a == a && b == b ]] && echo andand`,
		Why:     "&& inside [[ ]] joins conditions rather than commands",
	},
	{
		ID: "cond/arith-command-status-inverted", Category: "[[ ]] and (( ))",
		Snippet: `(( 1+1 )); echo "nonzero=$?"; (( 0 )); echo "zero=$?"`,
		Why:     "(( expr )) exits 0 when the expression is non-zero, the reverse of the usual convention",
	},
	{
		ID: "cond/arith-command-comparison", Category: "[[ ]] and (( ))",
		Snippet: `(( 2 > 1 )) && echo gt`,
		Why:     "> inside (( )) is a comparison; in dash the whole thing is nested subshells running a command",
	},
	// --- parameter expansion --------------------------------------------------
	{
		ID: "param/colon-extends-the-test-unset", Category: "parameter expansion",
		Snippet: `unset u; printf "[%s]" "${u:-D}" "${u-D}"`,
		Why:     "with the variable unset both forms fire, so this row alone proves nothing — it is the pair with the next case that does",
	},
	{
		ID: "param/colon-extends-the-test-empty", Category: "parameter expansion",
		Snippet: `e=; printf "[%s]" "${e:-D}" "${e-D}"`,
		Why:     "the colon is the whole difference: it extends the test from unset to unset-or-empty",
	},
	{
		ID: "param/plus-is-the-mirror", Category: "parameter expansion",
		Snippet: `unset u; e=; s=S; printf "[%s]" "${u:+A}" "${e:+A}" "${s:+A}" "${u+A}" "${e+A}" "${s+A}"`,
		Why:     "+ fires when the test does not, and the colon shifts it the same way",
	},
	{
		ID: "param/assign-has-a-side-effect", Category: "parameter expansion",
		Snippet: `unset u; printf "[%s]" "${u:=V}"; printf "[%s]" "$u"`,
		Why:     ":= leaves the variable set afterwards — the only expansion here with a side effect",
	},
	{
		ID: "param/word-is-itself-expanded", Category: "parameter expansion",
		Snippet: `unset u; d=DEF; printf "[%s]" "${u:-$d}" "${u:-$(echo sub)}"`,
		Why:     "the word is a word, not a literal, so the AST cannot store it as a string",
	},
	{
		ID: "param/prefix-shortest-and-longest", Category: "parameter expansion",
		Snippet: `p=a.b.c; printf "[%s]" "${p#*.}" "${p##*.}"`,
		Why:     "doubling the operator selects the longer match; there is no greediness syntax in the pattern",
	},
	{
		ID: "param/suffix-shortest-and-longest", Category: "parameter expansion",
		Snippet: `p=a.b.c; printf "[%s]" "${p%.*}" "${p%%.*}"`,
		Why:     "the same rule from the other end",
	},
	{
		ID: "param/pattern-is-a-glob", Category: "parameter expansion",
		Snippet: `p=abc; printf "[%s]" "${p#[ab]}" "${p#?}" "${p#x}"`,
		Why:     "patterns are globs rather than regular expressions, and one that does not match removes nothing",
	},
	{
		ID: "param/length-of-a-value", Category: "parameter expansion",
		Snippet: `x=abcd; printf "[%s]" "${#x}"`,
		Why:     "the length of the value",
	},
	{
		ID: "param/length-of-special-diverges", Category: "parameter expansion",
		Snippet: `set -- p q r; printf "[%s]" "${#@}" "${#*}"`,
		Why:     "dash gives the length of the joined string where the others give the count — the first axis where dash stands alone, and silent because both answers are plausible numbers",
	},
	{
		ID: "param/substitution", Category: "parameter expansion",
		Snippet: `x=a-b-c; printf "[%s]" "${x/-/+}" "${x//-/+}"`,
		Why:     "replace first versus replace every; absent from dash",
	},
	{
		ID: "param/bang-with-an-operator-is-the-parameter", Category: "parameter expansion",
		Snippet: `true & echo "${!:+set}"`,
		Why:     "a `!` with an operator right after it is $! — not the start of an indirection that then has no name. All four shells print set",
	},
	{
		ID: "param/a-bad-operator-in-a-branch-never-taken", Category: "parameter expansion",
		Snippet: `if false; then echo "${foo ~}"; fi; echo ok`,
		Why:     "a bad substitution is a runtime error in bash, dash and zsh — an expansion never reached is never diagnosed. ksh93 alone refuses it while reading, which makes this an axis; Terraform templates rely on the runtime answer",
	},
	{
		ID: "param/a-bad-operator-reached", Category: "parameter expansion",
		Snippet: `echo "${foo ~}"; echo "st=$?"`,
		Why:     "and when it is reached: bash names the construct and abandons the line, dash and zsh say only that the substitution was bad, ksh93 never got this far",
	},
	{
		ID: "param/substitution-anchored", Category: "parameter expansion",
		Snippet: `x=a-b; printf "[%s]" "${x/#a/X}" "${x/%b/Y}"`,
		Why:     "anchored to the start and the end of the value",
	},
	{
		ID: "param/transform-quotes-for-reuse", Category: "parameter expansion",
		Snippet: `x="a b'c"; printf "[%s]" "${x@Q}"`,
		Why:     "bash alone has the @ transformations: single quotes with the quote spelled '\\''; the other three call the construct a bad substitution at run time",
	},
	{
		ID: "param/transform-distributes-over-an-array", Category: "parameter expansion",
		Snippet: `a=(one "t w"); printf "[%s]" "${a[@]@Q}"`,
		Why:     "a transformation distributes: one quoted word per element, not one word holding the joined array",
	},
	{
		ID: "param/transform-deferred-in-a-branch-never-taken", Category: "parameter expansion",
		Snippet: `if false; then echo "${x@Q}"; fi; echo ok`,
		Why:     "the @ family is deferred to run time by every shell in the panel — including ksh93, which refuses every *other* unrecognized operator while reading. The second defect of the sweep that found the family",
	},
	{
		ID: "param/transform-expands-escapes", Category: "parameter expansion",
		Snippet: `x='a\tb'; y='\101'; printf "[%s][%s]\n" "${x@E}" "${y@E}"`,
		Why:     "@E reads the value under the $'…' rules — a named escape and an octal one, applied to the value rather than to the source text, which is the only way to get that decoding from a variable",
	},
	{
		ID: "param/transform-prompt-escapes", Category: "parameter expansion",
		Snippet: `x='a\nb\\c'; printf "[%s]\n" "${x@P}"`,
		Why:     "@P runs the prompt language over the value: \\n is a newline and \\\\ is one backslash. Probed with only the two escapes whose answer is the same on every machine — \\t is the time of day and \\w the directory, so a case using either would record the clock",
	},
	{
		ID: "param/transform-writes-an-assignment", Category: "parameter expansion",
		Snippet: `x='a b'; echo "${x@A}"; declare -irx v=1; echo "${v@A}"; printf "[%s][%s]\n" "${v@a}" "${x@a}"`,
		Why:     "@A writes the statement that would recreate the variable — the value @Q-quoted, and attributes turning it into a declare with its letters in front — while @a is those letters alone, empty for a name with none",
	},
	{
		ID: "param/transform-keys-and-values", Category: "parameter expansion",
		Snippet: `a=(one "t w"); printf "[%s]" "${a[@]@K}"; printf "<%s>" "${a[@]@k}"; s=q; printf "{%s}\n" "${s@K}"`,
		Why:     "@K is one field of subscripts and double-quoted values; @k is the same pairs as separate fields with the values bare; on a scalar, which has no keys, both fall back to the @Q quoting. bash 3.2 is the surprise: it does not have the family and yet answers ${a[@]@K} with the plain elements instead of a bad substitution, so a script guarding on the error never sees one",
	},
	{
		ID: "param/transform-case-letters", Category: "parameter expansion",
		Snippet: `x='abC dEf'; printf "[%s][%s][%s]\n" "${x@L}" "${x@U}" "${x@u}"`,
		Why:     "the three case letters, kept in one case because the difference between them is the point: L and U reach every letter of the value and u only its first character — which is not the first character of each word",
	},
	{
		ID: "param/transform-takes-exactly-one-letter", Category: "parameter expansion",
		Snippet: `echo "[${u@QQ}]"; echo "u=$?"; x=a; echo "[${x@QQ}]"; echo unreached`,
		Why:     "the operator is @ followed by one letter: doubling a letter that is valid on its own is a bad substitution, so the family cannot be extended by repetition the way zsh's (q) can. And bash checks the letter only once it has a value — an *unset* name yields empty and status 0 for the very same spelling, so a probe that forgets to set the variable measures nothing. The three shells without the family reject both",
	},
	{
		ID: "param/bad-substitution-names-the-word", Category: "parameter expansion",
		Snippet: `x=a; echo "pre${x@QQ}post"`,
		Why:     "what the sentence names is the *word*, not the ${…} inside it: bash writes `pre${x@QQ}post: bad substitution` and ksh93 the same word in its own quotes, while dash and zsh name nothing at all — three answers across the four. One command, because the first failure ends the line",
	},
	{
		ID: "param/bad-substitution-names-only-one-quoting", Category: "parameter expansion",
		Snippet: `x=a; echo 'lit'"${x@QQ}"`,
		Why:     "the same question where the word changes quoting in the middle, which is what separates the two shells that name a word: bash stops at the change and blames ${x@QQ} alone, ksh93 blames 'lit'\"${x@QQ}\" whole. A separate row because the first bad word ends the command, so the two spellings cannot share one",
	},
	{
		ID: "param/bad-substitution-stops-at-the-first", Category: "parameter expansion",
		Snippet: `x=a; printf "[%s]" "${x@QQ}" "${x@ZZ}" "${x@YY}"; echo " after"`,
		Why:     "one diagnostic per command and not one per bad word — every column abandons the command at the first expansion it cannot answer. This implementation expanded the remaining words and reported each of them, which is four lines where a script's log expects one",
	},
	{
		ID: "param/an-unset-name-stops-at-the-first-too", Category: "parameter expansion",
		Snippet: `set -u; printf "[%s]" "$a" "$b"; echo " after"`,
		Why:     "the same rule reached through a different failure: `set -u` on two unset names names only the first. Kept beside the bad-substitution row because the two shared one cause — the command expanded every word before deciding not to run",
	},
	{
		ID: "param/an-array-length-without-a-subscript", Category: "parameter expansion",
		Snippet: `a=(hello by z); echo "${#a[@]} ${#a}"`,
		Why:     "zsh counts the elements for a bare ${#a}; bash and ksh93 measure element zero — probed with distinct lengths, because (one two three) hides the difference behind a three",
	},
	{
		ID: "param/an-empty-array-quoted-at", Category: "parameter expansion",
		Snippet: `a=(); set -- "${a[@]}"; echo "n=$#"`,
		Why:     "ksh93 hands the quotes one empty field where bash and zsh hand none — the reason careful scripts write \"${a[@]+\"${a[@]}\"}\"",
	},
	{
		ID: "param/a-negative-substring-length", Category: "parameter expansion",
		Snippet: `x=abcdef; echo "[${x:1:-2}]"`,
		Why:     "bash and zsh count a negative length from the end; ksh93 answers with nothing at all",
	},
	{
		ID: "special/lineno-in-a-function-diverges", Category: "parameters",
		Script:          true,
		LayoutSensitive: true,
		Snippet:         "f(){\necho $LINENO\n}\nf\n",
		Why:             "zsh numbers a function's lines from the line the function was written on; the other three count from the file",
	},
	{
		ID: "param/substring", Category: "parameter expansion",
		Snippet: `x=abcdef; printf "[%s]" "${x:1:3}" "${x:2}"`,
		Why:     "offset with and without a length; absent from dash",
	},
	{
		ID: "param/case-change-is-bash-only", Category: "parameter expansion",
		Snippet: `x=aBc; printf "[%s]" "${x^^}" "${x,,}"`,
		Why:     "bash alone: ksh93 reports a syntax error and zsh a bad substitution, so it belongs to the bash dialect rather than the core",
	},
	{
		ID: "param/case-toggle-is-newer-bash-still", Category: "parameter expansion",
		Snippet: `x=aBc; printf "[%s][%s][%s]" "${x~}" "${x~~}" "${x~~[ab]}"`,
		Why:     "the third spelling of case change, swapping instead of forcing a direction, single and doubled like the other two and carrying a pattern the same way. bash 5.3 alone — bash 3.2 refuses it too — so it rides the same dialect flag as `^^` and `,,`",
	},
	{
		ID: "param/indirection-diverges-four-ways", Category: "parameter expansion",
		Snippet: `x=y; y=V; printf "[%s]" "${!x}"`,
		Why:     "bash indirects, dash and zsh reject, and ksh93 yields x — not an error there, a different meaning, which is the &> failure mode inside an expansion",
	},
	{
		ID: "param/expansion-flags-are-one-dialects", Category: "parameter expansion",
		Snippet: `x=abc; echo ${(U)x}`,
		Why:     "the parenthesized expansion flags are zsh's alone: it uppercases where bash and dash call the expansion a bad substitution at run time and ksh93 refuses it while reading — the same three-way split every unreadable expansion follows",
	},
	{
		ID: "param/expansion-flags-split-and-join", Category: "parameter expansion",
		Snippet: `x=a:b:c; printf "[%s]" ${(s.:.)x}; a=(1 2); printf "<%s>" "${(j.,.)a}"`,
		Why:     "the s flag splits a scalar at its separator into real fields and j joins an array with its own — string-to-list and list-to-string, which no operator the other shells have can say",
	},
	{
		ID: "param/prompt-percent-names-the-script", Category: "parameter expansion",
		Snippet: `echo ${(%):-%x}`,
		Script:  true,
		Why:     "the wild idiom for a file's own path — /opt/homebrew's ruby-lsp-activate.sh opens with it: the %-flag prompt escape %x names the file being read, and the empty parameter with a :- is how a bare string reaches the flags at all",
	},
	{
		ID: "param/expansion-flags-quote-four-ways", Category: "parameter expansion",
		Snippet: `x="a b'c"; printf "[%s]" "${(q)x}" "${(qq)x}" "${(qqq)x}" "${(qqqq)x}"; echo`,
		Why:     "the q family is one flag repeated, and each repetition is a different quoting: backslashes, single quotes, double quotes, then $'…'. Repetition is what selects it, which is exactly what bash's @ family refuses",
	},
	{
		ID: "param/expansion-flags-split-at-newlines", Category: "parameter expansion",
		Snippet: `x=$'a\nb'; printf "[%s]" ${(f)x}; echo`,
		Why:     "(f) is the line-splitting flag — the read-a-command's-output-into-an-array idiom, and the one split that does not consult IFS",
	},
	{
		ID: "param/expansion-flags-at-keeps-array-fields", Category: "parameter expansion",
		Snippet: `a=(x "y z" ""); printf "<%s>" "${(@)a}"; printf "{%s}" "${a}"; echo`,
		Why:     "in double quotes an array is joined on the first character of IFS unless (@) is present, which keeps one field per element including the empty one — the flag spelling of what \"${a[@]}\" says with a subscript",
	},
	{
		ID: "param/expansion-flags-name-indirection", Category: "parameter expansion",
		Snippet: `y=hello; x=y; printf "[%s]" ${(P)x}; echo`,
		Why:     "(P) reads the value as a further name — zsh's spelling of the indirection bash writes ${!x}, and the reason the two shells need no shared syntax for it",
	},
	{
		ID: "param/expansion-flags-keys-and-values", Category: "parameter expansion",
		Snippet: `typeset -A m=(k1 v1); printf "<%s>" ${(k)m} ${(kv)m}; echo`,
		Why:     "(k) yields an associative array's keys and (v) beside it interleaves key and value. Probed with a single pair on purpose: zsh yields hash order for more, which it does not promise and a golden record must not pin",
	},
	{
		ID: "param/expansion-flags-run-after-the-operator", Category: "parameter expansion",
		Snippet: `x=hello; printf "[%s]" "${(U)x#h}" "${(U)nope:-def}"; y=val; z=y; printf "<%s>" "${(P)z:-def}"; echo`,
		Why:     "the operator runs first and the flags transform what it produced — ${(U)x#h} is ELLO, not a trim of HELLO — with (P) the exception, resolving the name before the default can fire",
	},
	{
		ID: "param/array-element-inherits-the-base", Category: "parameter expansion",
		Snippet: `a=(p q r); printf "[%s]" "${a[1]}" "${#a[@]}"`,
		Why:     "array subscripting inherits the 0-versus-1 base axis",
	},
	{
		ID: "param/an-operator-reaches-an-array-element", Category: "parameter expansion",
		Snippet: `a=(hello); printf "[%s]" "${a[0]#h}" "${a[0]/l/L}" "${a[0]%%o}" "${a[0]:1}"`,
		Why:     "an operator applies to a subscripted value exactly as it does to a variable — four spellings at once, because they shared one bug: the subscript answered and every operator was skipped, so each of these came back as the untouched element",
	},
	{
		ID: "param/a-missing-element-fires-the-default", Category: "parameter expansion",
		Snippet: `a=(hello); printf "[%s]" "${a[0]:-d}" "${a[9]:-d}"`,
		Why:     "the element being absent is what the test is about, so an out-of-range subscript has to be distinguishable from one holding a value — and an element holding the empty string is set, which is the case `:-` and `-` disagree about",
	},
	{
		ID: "param/array-slice", Category: "parameter expansion",
		Snippet: `a=(p q r s); printf "[%s]" "${a[@]:1}" "${a[@]:1:2}" "${a[@]: -2}"`,
		Why:     "a slice of the list rather than a substring of its elements joined together. Unanimous in the three that have arrays, including that the offset counts from 0 in zsh, whose *subscripts* count from 1 — so the slice does not inherit the base axis that `${a[1]}` does",
	},
	{
		ID: "param/array-slice-keeps-its-fields", Category: "parameter expansion",
		Snippet: `a=("a b" c d); printf "[%s]" "${a[@]:0:2}"`,
		Why:     "the slice is a list, so an element holding a space stays one field — which is the whole reason this is not a substring of the joined text",
	},
	{
		ID: "param/a-negative-subscript-counts-from-the-end", Category: "parameter expansion",
		Snippet: `a=(10 20 30); printf "[%s]" "${a[-1]}" "${a[-2]}" "$((a[-3]))"`,
		Why:     "a negative subscript is end-relative in bash, ksh93 and zsh — zsh included, whose positive subscripts count from 1 — so it does not inherit the base axis, in an expansion or in arithmetic. bash 3.2 predates the form and refuses it. We expanded it to nothing",
	},
	{
		ID: "param/assigning-through-a-negative-subscript", Category: "parameter expansion",
		Snippet: `a=(one two three); a[-1]=X; echo "${a[@]}"`,
		Why:     "the write is end-relative exactly as the read is: `a[-1]=X` replaces the last element in bash, ksh93 and zsh; bash 3.2 predates the form and refuses it fatally",
	},
	{
		ID: "param/an-operator-distributes-over-the-elements", Category: "parameter expansion",
		Snippet: `a=(aa ab); printf "[%s]" "${a[@]#a}" "${a[@]%%a*}" "${a[@]//a/X}"`,
		Why:     "an operator on `${a[@]}` applies to every element and each stays a field — unanimous in the three with arrays. We joined the elements first, so the pattern reached the first alone and came back `a ab` with status 0",
	},
	{
		ID: "param/an-operator-on-the-star-subscript-diverges", Category: "parameter expansion",
		Snippet: `a=(aa ab); echo "${a[*]#a}"`,
		Why:     "the star form is the axis the at form is not: bash and ksh93 trim each element and join what is left (`a b`), zsh joins first and trims the joined string once (`a ab`)",
	},
	{
		ID: "cond/a-newline-continues-a-condition", Category: "pattern matching",
		Snippet: `[[ 1 == 1 &&
2 == 2 ]] && echo yes`,
		Why: "a multi-line condition, which is ordinary formatting — found by the wild sweep in two unrelated files. A newline inside `[[ ]]` continues the condition rather than ending a command, and nothing multi-line parsed at all before: not after `&&`, not after `[[` itself, not before `]]`",
	},
	{
		ID: "cond/a-newline-at-every-point", Category: "pattern matching",
		// Deliberately without a parenthesised group, which the parser
		// handles the same way and the unit tests cover: dash reads `(` as
		// opening a subshell and names what it expected to close it, which
		// is #216 and nothing to do with this.
		//
		// And with `&&` rather than `||`, for a reason of the same kind:
		// dash has no `[[`, so the clause fails and `&&` stops there, where
		// `||` would run the next line and expose a separate line-numbering
		// bug — a command after an operator at end of line is reported at
		// the operator's line, not its own.
		Snippet: `[[
1 == 1 &&
2 == 2
]] && echo yes`,
		Why: "the same rule at three structural points at once — after `[[` itself, after `||`, and before `]]`. Unanimous in all three shells that have `[[ ]]`, and none of the three worked before",
	},
	{
		ID: "decl/an-array-assignment-as-an-operand", Category: "parameter expansion",
		// `typeset` rather than `local`, and at the top level rather than in
		// a function, only to keep the case about this: dash adds an
		// "expecting }" to the syntax error when the `(` is inside a brace
		// group and we do not, which is a separate gap that would show up
		// here as a wording mismatch about something else.
		Snippet: `typeset a=(x y); echo "[${a[1]}]"`,
		Why:     "an array assignment written as an *operand* of a declaration utility, which is ordinary bash and did not parse at all — found by the wild sweep in three installed bats-core files. dash has no array literal so it is a syntax error there, and the subscript base makes the answer differ between the three that do",
	},
	{
		ID: "decl/a-local-array-stays-local", Category: "parameter expansion",
		Snippet: `a=(g); f() { local a=(x y); }; f; echo "[${a[0]}]"`,
		Why:     "the second half, and the one that is silent: making the form parse showed the array outliving the function, because `local` had saved a scalar of that name and nothing had saved the *array*. Arrays are a second table and shadowing has to cover both",
	},
	{
		ID: "decl/readonly-takes-its-array-first", Category: "parameter expansion",
		Snippet: `readonly a=(p q); echo "[${a[1]}]"`,
		Why:     "`readonly` has to set the array before it locks the name, because locking first refuses the very assignment the command was given — the opposite order from `local`, which must make the name local before the array lands",
	},
	{
		ID: "diag/a-command-after-an-operator", Category: "diagnostics",
		LayoutSensitive: true,
		Snippet: `echo one
false ||
nosuchcmd
echo "st=$?"`,
		Why: "a command written after `&&`, `||` or `|` at the end of a line is reported at its own line, not at the one the chain began on. Unanimous, so it is the core's behavior — and it was off by however many lines the chain had run for, which in a long `&&` chain is every line of it",
	},
	{
		ID: "trap/a-bare-exit-in-an-exit-trap", Category: "traps",
		Snippet: `trap "false; exit" 0; true
echo unreachable`,
		Why: "a bare `exit` in an EXIT trap reports the status the shell had when the trap began, not the trap's own last command — 0 in bash, dash and ksh93 and 1 in zsh. Found by `make wild-run`: /usr/bin/bzless traps `stty …; exit` and with no terminal the `stty` fails, so the script exited 1 where every shell exits 0, with identical output. Only a *run* comparison can see that",
	},
	{
		ID: "param/case-change-applies-its-pattern", Category: "parameter expansion",
		Snippet: `x=abc; printf "[%s]" "${x^^[ab]}"`,
		Why:     "the operator carries a pattern saying *which* characters to convert, matched one at a time — `ABc`, not `ABC`. Discarding it was a silent wrong answer in a feature already claimed: the script asked for a subset and got the whole string, with status 0. bash alone has the operator, so the other three refuse the word",
	},
	{
		ID: "param/case-change-of-the-first-character", Category: "parameter expansion",
		Snippet: `x=hello; printf "[%s][%s]" "${x^}" "${x^l}"`,
		Why:     "the single form converts the first character and only when the pattern matches it, which is what tells it from the doubled one — `Hello` and then `hello`, where `${x^^l}` would be `heLLo`",
	},
	{
		ID: "param/an-expansion-inside-an-operand", Category: "parameter expansion",
		Snippet: `a=(x y); printf "[%s]" "${a[@]+${a[@]}}"`,
		Why:     "the standard way to expand a possibly-empty array under `set -u`, and where the wild sweep found that a subscript was ending at the *last* `]` in the word rather than its own — so the inner expansion's bracket closed the outer's subscript and the operator after it was unreadable. The simple `${a[@]+x}` always worked, which is why a real script had to find it",
	},
	{
		ID: "param/array-indices", Category: "parameter expansion",
		Snippet: `a=(p q r); for i in "${!a[@]}"; do printf "%s=%s " "$i" "${a[$i]}"; done`,
		Why:     "`${!a[@]}` is the array's subscripts, not its elements — written as the loop that uses it, since iterating an array by index is the only reason the form exists and answering with the elements made that loop silently iterate the wrong thing",
	},
	// --- pattern matching -----------------------------------------------------
	{
		ID: "pat/star-matches-dot-in-case", Category: "pattern matching",
		Snippet: `case .hidden in *) echo star-matches-dot;; esac`,
		Why:     "in case there is no filesystem, so nothing restricts the star",
	},
	{
		ID: "pat/star-skips-leading-dot-in-glob", Category: "pattern matching",
		Snippet: `touch .hidden vis; printf "[%s]" *`,
		Why:     "the same pattern in pathname expansion skips a leading period — the restriction belongs to the context, not to the pattern",
	},
	{
		ID: "pat/star-matches-slash-in-case", Category: "pattern matching",
		Snippet: `case a/b in a*b) echo star-matches-slash;; esac`,
		Why:     "and nothing stops it crossing a slash either, because there are no components",
	},
	{
		ID: "pat/unterminated-bracket", Category: "pattern matching",
		Snippet: `case "[" in [) echo hit;; *) echo miss;; esac`,
		Why:     "a bare `[` is the test builtin's name, so this is load-bearing: bash and ksh93 take it literally and hit, dash matches nothing, zsh calls it a bad pattern",
	},
	{
		ID: "pat/unterminated-bracket-globs", Category: "pattern matching",
		Snippet: `echo [ ; echo [a`,
		Why:     "the same question against the filesystem, where all four agree a lone `[` is literal and only zsh rejects `[a`",
	},
	{
		ID: "pat/star-stops-at-slash-in-glob", Category: "pattern matching",
		Snippet: `mkdir s; touch s/f; printf "[%s]" *f`,
		Why:     "in pathname expansion it cannot cross a directory boundary; an unmatched pattern is passed through, except in zsh",
	},
	{
		ID: "pat/only-a-leading-period-is-special", Category: "pattern matching",
		Snippet: `touch a.b; printf "[%s]" *.b`,
		Why:     "only a leading period, so a star matches one in the middle of a name",
	},
	{
		ID: "pat/bracket-set-and-range", Category: "pattern matching",
		Snippet: `case b in [abc]) printf set;; esac; case c in [a-z]) printf " range";; esac`,
		Why:     "the two universal bracket forms",
	},
	{
		ID: "pat/bracket-dash-at-an-edge-is-literal", Category: "pattern matching",
		Snippet: `case - in [a-]) echo trailing;; esac; case - in [-a]) echo leading;; esac; case a in [a-]) echo a;; esac; case b in [a-]) echo range;; *) echo not-b;; esac`,
		Why:     "a dash first or last in a bracket expression is a character rather than a range, so [a-] matches a and a literal dash and nothing between",
	},
	{
		ID: "pat/bracket-bang-negates-everywhere", Category: "pattern matching",
		Snippet: `case d in [!abc]) echo negated;; esac`,
		Why:     "! is the portable negation",
	},
	{
		ID: "pat/bracket-caret-is-an-extension", Category: "pattern matching",
		Snippet: `case d in [^abc]) echo caret;; *) echo no-caret;; esac`,
		Why:     "^ negates everywhere but dash, where it is an ordinary character — so the pattern still matches, just not what was meant",
	},
	{
		ID: "pat/character-class", Category: "pattern matching",
		Snippet: `case 5 in [[:digit:]]) echo class;; esac`,
		Why:     "POSIX character classes are universal",
	},
	{
		ID: "pat/the-remaining-character-classes", Category: "pattern matching",
		Snippet: `case "$(printf '\1')" in [[:cntrl:]]) printf cntrl;; esac; case " " in [[:blank:]]) printf " blank";; esac; case x in [[:graph:]]) printf " graph";; esac; case " " in [[:print:]]) printf " print";; esac`,
		Why:     "the four classes the matcher grew last, unanimous like the other eight — pinned because they were silently matching nothing while every panel shell answered",
	},
	{
		ID: "pat/classes-that-overlap-and-differ", Category: "pattern matching",
		Snippet: `t=$(printf "\t"); case "$t" in [[:blank:]]) printf blank;; esac; case "$t" in [[:print:]]) printf " P";; *) printf " noprint";; esac; case " " in [[:graph:]]) printf " G";; *) printf " nograph";; esac`,
		Why:     "the edges that tell the four apart in the C locale the panel runs under: a tab is blank but not printable, and a space is printable but not graphic — unanimous, and the pair an implementation that aliases print to graph gets wrong",
	},
	{
		ID: "pat/an-unknown-character-class", Category: "pattern matching",
		Snippet: `case b in [[:bogus:]]) printf yes;; *) printf no;; esac; case "b]" in [[:bogus:]]) printf " lit";; *) printf " no";; esac`,
		Why:     "a class name nothing defines matches nothing, silently — no error, no output, status 0 — in every panel shell but bash 3.2, which alone falls back to reading the brackets as literal characters, so `b]` matches there and nowhere else",
	},
	{
		ID: "pat/escaped-metacharacter-is-literal", Category: "pattern matching",
		Snippet: `case "a*b" in a\*b) printf escaped;; esac; case axb in a\*b) printf " matched";; *) printf " no";; esac`,
		Why:     "an escaped star matches a literal star and nothing else",
	},
	{
		ID: "pat/quoting-decides-pattern-or-literal", Category: "pattern matching",
		Snippet: `p="a*b"; case "a*b" in $p) printf pattern;; esac; case axb in "$p") printf " literal-matched";; *) printf " literal-no";; esac`,
		Why:     "per-span quoting reaches all the way into matching, so a pattern is a word rather than a string",
	},
	{
		ID: "pat/case-subject-is-not-split", Category: "pattern matching",
		Snippet: `t=$(printf "\t"); case $t in [[:blank:]]) printf blank;; *) printf other;; esac; m="a b"; case $m in "a b") printf " one-word";; a) printf " first-field";; *) printf " neither";; esac`,
		Why:     "an unquoted case subject expands but is never field-split, so a whitespace-only value still reaches its arm and a value with a space in it stays one subject — splitting sent the tab to the star arm silently, status 0",
	},
	{
		ID: "pat/case-subject-is-not-globbed", Category: "pattern matching",
		Snippet: `touch afile; g="*"; case $g in afile) printf globbed;; \*) printf literal;; esac; unset u; case $u in "") printf " empty";; *) printf " nonempty";; esac`,
		Why:     "pathname expansion never touches the subject either — a value of * stays the character even in a directory it would match — and an unset subject is the empty string rather than no subject",
	},
	{
		ID: "pat/extended-patterns-are-not-core", SyntaxError: true, Category: "pattern matching",
		Snippet: `case abc in @(abc|xyz)) echo at;; esac`,
		Why:     "ksh93 alone accepts them as written; dash and bash report a syntax error and zsh parses but does not match — three behaviors, so not core",
	},
	// --- arithmetic -----------------------------------------------------------
	{
		ID: "arith/bare-name-is-a-variable", Category: "arithmetic",
		Snippet: `x=5; printf "[%s]" "$((x+1))" "$(($x+1))"`,
		Why:     "a bare name inside arithmetic is a variable reference, which is why the contents cannot be lexed as ordinary words",
	},
	{
		ID: "arith/unset-is-zero", Category: "arithmetic",
		Snippet: `unset u; printf "[%s]" "$((u+1))"`,
		Why:     "an unset variable is 0 rather than an error",
	},
	{
		ID: "arith/precedence-follows-c", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((1+2*3))" "$(((1+2)*3))" "$((2*3%4))"`,
		Why:     "POSIX defers the operator set and precedence to ISO C",
	},
	{
		ID: "arith/division-truncates", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((3/2))"`,
		Why:     "integer division, which is the baseline the float divergence departs from",
	},
	{
		ID: "arith/float-is-a-dialect-axis", SyntaxError: true, Category: "arithmetic",
		Snippet: `printf "[%s]" "$((1.5))"`,
		Why:     "ksh93 and zsh evaluate floating point where POSIX says integers only, so neither promising integers nor accepting floats is right everywhere",
	},
	{
		ID: "arith/leading-zero-octal", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((010))" "$((0100))"`,
		Why:     "zsh does not read a leading zero as octal — a plausible number, silently different, in code that looks portable, and file modes are written this way",
	},
	{
		ID: "arith/invalid-octal-digit", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((08))"`,
		Why:     "the same split from the other side: an error where octal is read, a decimal digit where it is not",
	},
	{
		ID: "arith/a-base-above-sixteen", Category: "arithmetic",
		Snippet: `echo $((36#z)); echo $((64#z))`,
		Why:     "base#digits runs 2 through 64 in bash and ksh93, with letters splitting into cases above 36; zsh stops at 36 and says so; dash has no bases at all",
	},
	{
		ID: "arith/a-digit-the-base-does-not-have", Category: "arithmetic",
		Snippet: `echo $((2#12)); echo "st=$?"`,
		Why:     "the failure is unanimous and the sentence is not: bash calls it value too great for base, ksh93 an arithmetic syntax error at 1, zsh stops the number at the bad digit, dash never parsed a base",
	},
	{
		ID: "arith/overflow-saturates-in-one-shell", Category: "arithmetic",
		Snippet: `echo $((9223372036854775807 + 1))`,
		Why:     "ksh93 clamps at the maximum where the other three wrap to the minimum",
	},
	{
		ID: "arith/an-empty-expression-diverges", Category: "arithmetic",
		Snippet: `echo $(( )); echo "st=$?"`,
		Why:     "zero in three of the four; dash wants a primary and stops the script at 2",
	},
	{
		ID: "arith/a-name-shaped-value-is-chased", Category: "arithmetic",
		Snippet: `a=b; b=3; echo $((a)); echo "st=$?"`,
		Why:     "bash, ksh93 and zsh resolve a value that names another variable until it is a number; dash calls b an illegal number and stops",
	},
	{
		ID: "arith/an-operand-the-expression-ran-out-of", SyntaxError: true, Category: "arithmetic",
		Snippet: `echo "[$((1+))]"; echo "st=$?"`,
		Why:     "an operand was wanted and the text ended, which two of the panel word apart from an operand that was wanted and found: ksh93 says more tokens expected and zsh names the end of the string. The pair with `arith/an-operand-the-expression-found` is the whole of it — either row alone passes under one wording for both, which is what let the end-of-input sentence stand for every operand failure in two dialects",
	},
	{
		ID: "arith/an-operand-the-expression-found", SyntaxError: true, Category: "arithmetic",
		Snippet: `echo "[$((%))]"; echo "st=$?"`,
		Why:     "the other half: a token is there and it cannot begin a value. ksh93 drops to its bare arithmetic syntax error and zsh names the text — ``operand expected at `%'`` — where both said the expression had run out. bash words the two identically, which is why the bash column cannot see this at all and why the failure survived every conformance read",
	},
	{
		ID: "arith/an-operand-found-after-an-operator", SyntaxError: true, Category: "arithmetic",
		Snippet: `echo "[$((1+&2))]"; echo "st=$?"`,
		Why:     "the found case reached mid-expression rather than at its start, and the text named runs to the end of the expression rather than being the one refused byte — `&2`, which is what the two shells that name anything name. It also says the distinction is not about where in the expression the failure is: the same operator wanting the same operand is worded one way here and the other way in `arith/an-operand-the-expression-ran-out-of`",
	},
	{
		ID: "arith/an-operand-a-lexer-refuses-outright", SyntaxError: true, Category: "arithmetic",
		Snippet: `echo "[$((@))]"; echo "st=$?"`,
		Why:     "a byte that is not part of any arithmetic token, which zsh alone words a third way — `illegal character: @` rather than the operand sentence it gives `%`. The row is recorded rather than reproduced: the third wording turns on where in the expression the byte stands as well as on which byte it is, since `$((1+@))` gets the operand sentence and `$((1 @))` and `$((@))` do not, and that is a lexer's table rather than a grammar rule. bash, ksh93 and dash word it exactly as they word `%`, so three of the four columns pass",
	},
	{
		ID: "arith/the-error-names-what-was-consumed", Category: "arithmetic",
		Snippet: `echo $((1+08)); echo "st=$?"`,
		Why:     "bash's leading position is what the evaluator had consumed when the token failed — 1+08 blamed as 1+08 but 08+1 as 08 — where dash names the whole expression and the octal-tolerant shells answer 9",
	},
	{
		ID: "arith/explicit-base", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((2#101))" "$((0x10))"`,
		Why:     "hex is universal; the base#number form is absent from dash",
	},
	{
		ID: "arith/comparison-yields-one-or-zero", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((1<2))" "$((2<1))" "$((1==1))"`,
		Why:     "comparisons yield 1 or 0",
	},
	{
		ID: "arith/logical-yields-one-not-an-operand", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((2 && 3))" "$((0 || 5))"`,
		Why:     "a logical operator yields 1 or 0 rather than one of its operands, unlike some languages",
	},
	{
		ID: "arith/short-circuit-is-observable", Category: "arithmetic",
		Snippet: `x=0; printf "[%s]" "$((0 && (x=9)))" "$x"`,
		Why:     "assignment is an operator here, so evaluation order is part of the specification rather than an implementation detail",
	},
	{
		ID: "arith/assignment-escapes", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((x=5))"; printf "[%s]" "$x"`,
		Why:     "an assignment inside an expression is a side effect that outlives it, like ${x:=5}",
	},
	{
		ID: "arith/ternary", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((1?2:3))" "$((0?2:3))"`,
		Why:     "the conditional operator",
	},
	{
		ID: "arith/compound-arithmetic-assignment", Category: "arithmetic",
		Snippet: `x=44; a=$((x-=2)); b=$((x/=6)); c=$((x%=4)); echo "$a $b $c x=$x"`,
		Why:     "-=, /= and %= each apply their operator to the target and leave the result behind, so the chain reads 42, 7, 3 off one variable",
	},
	{
		ID: "arith/compound-bitwise-assignment", Category: "arithmetic",
		Snippet: `x=12; a=$((x&=10)); b=$((x^=3)); c=$((x|=16)); echo "$a $b $c x=$x"`,
		Why:     "the bitwise half of the assignment family: &=, ^= and |= are POSIX and unanimous, dash included",
	},
	{
		ID: "arith/compound-shift-assignment", Category: "arithmetic",
		Snippet: `x=3; a=$((x<<=4)); b=$((x>>=2)); echo "$a $b x=$x"`,
		Why:     "<<= and >>= complete the family, and the second reads the first's result rather than the original — the target is written before the next expression runs",
	},
	{
		ID: "arith/logical-not", Category: "arithmetic",
		Snippet: `echo "$((!0)) $((!1)) $((!7))"`,
		Why:     "! yields 1 on zero and 0 on anything else — a truth rather than a bit flip, so !7 is 0 and not -8",
	},
	{
		ID: "arith/increment-absent-from-dash", Category: "arithmetic",
		Snippet: `x=1; printf "[%s]" "$((x++))" "$x"`,
		Why:     "++ is not POSIX and dash rejects it",
	},
	{
		ID: "arith/comma-absent-from-dash", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((1,2))"`,
		Why:     "the sequence operator, likewise",
	},
	{
		ID: "arith/exponent-absent-from-dash", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((2**10))"`,
		Why:     "exponentiation is not POSIX — nor ISO C — and dash rejects it",
	},
	{
		ID: "arith/exponent-binds-right-and-below-unary", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((2**3**2))" "$((-2**2))"`,
		Why:     "** associates to the right and a prefix sign belongs to the base, so 512 and 4 — neither follows from C, which has no such operator",
	},
	{
		ID: "arith/negative-exponent-diverges", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((2**-1))"`,
		Why:     "no integer answer exists: bash refuses where ksh93 and zsh go float and answer 0.5",
	},
	{
		ID: "arith/division-by-zero-is-a-runtime-error", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((1/0))"`,
		Why:     "unanimous, and a runtime error rather than a syntax one — the expression parses",
	},
	{
		ID: "arith/non-numeric-variable-diverges", Category: "arithmetic",
		Snippet: `x=abc; printf "[%s]" "$((x+1))"`,
		Why:     "three answers: dash and ksh93 error differently, while bash and zsh re-evaluate the value as an expression and reach 0",
	},
	// --- conditions -----------------------------------------------------------
	{
		ID: "cond/no-field-splitting-inside", Category: "conditions",
		Snippet: `x="a b"; [[ $x == "a b" ]] && echo no-splitting`,
		Why:     "[[ ]] is parsed rather than executed, so the words never become arguments and are never split",
	},
	{
		ID: "cond/unset-needs-no-quoting", Category: "conditions",
		Snippet: `unset u; [[ -z $u ]] && echo fine`,
		Why:     "the case where the [ builtin needs its argument quoted and this does not",
	},
	{
		ID: "cond/no-pathname-expansion-inside", Category: "conditions",
		Snippet: `touch f1 f2; [[ * == "*" ]] && echo no-globbing || echo globbed`,
		Why:     "and no pathname expansion either, for the same reason",
	},
	{
		ID: "cond/rhs-is-a-pattern", Category: "conditions",
		Snippet: `[[ abc == a* ]] && printf pattern; [[ abc == "a*" ]] && printf " quoted-matched" || printf " quoted-literal"`,
		Why:     "unquoted the right operand is a pattern, quoted it is a literal — the same rule as case",
	},
	{
		ID: "cond/pattern-through-a-variable-diverges", Category: "conditions",
		Snippet: `p="a*"; [[ abc == $p ]] && echo var-is-pattern || echo var-is-literal`,
		Why:     "zsh does not treat the result of an expansion as a pattern, which is the glob-expansion-results axis reaching into conditions rather than a second rule",
	},
	{
		ID: "cond/numeric-versus-string-comparison", Category: "conditions",
		Snippet: `[[ 10 -gt 9 ]] && printf numeric; [[ 10 > 9 ]] && printf " string-gt" || printf " string-lt"`,
		Why:     "the sharpest trap in the construct: -gt compares numbers and > compares strings, so 10 sorts before 9",
	},
	{
		ID: "cond/regex-match", Category: "conditions",
		Snippet: `[[ abc =~ ^a.c$ ]] && echo regex || echo no-regex`,
		Why:     "the one place in the shell where the pattern language is regular expressions rather than globs",
	},
	{
		ID: "cond/quoted-regex-diverges", Category: "conditions",
		Snippet: `[[ abc =~ "^a.c$" ]] && echo still-regex || echo literal`,
		Why:     "bash treats a quoted right operand as a literal string where ksh93 and zsh keep it a regex, so quoting a regex is not portable in either direction",
	},
	{
		ID: "cond/regex-captures-are-recorded", Category: "conditions",
		Snippet: `[[ abcd =~ (b)(c) ]]; echo "[${BASH_REMATCH[0]}|${BASH_REMATCH[1]}|${BASH_REMATCH[2]}]"`,
		Why:     "a successful =~ records the whole match at 0 and the groups after it, under bash's name for the record; ksh93 and zsh keep their captures under names of their own and leave this one unset",
	},
	{
		ID: "cond/regex-failure-empties-the-record", Category: "conditions",
		Snippet: `[[ ab =~ a ]]; [[ ab =~ q ]]; echo "n=${#BASH_REMATCH[@]}"`,
		Why:     "a failed match empties the record rather than leaving the capture before last, so a script that forgets to check the status reads nothing instead of stale groups",
	},
	{
		ID: "cond/logical-and-grouping", Category: "conditions",
		Snippet: `[[ ( -n x || -n y ) && ! -z z ]] && echo grouped`,
		Why:     "&& and || join conditions and ( ) groups them rather than starting a subshell, so the parser needs its own production for the inside",
	},
	{
		ID: "cond/andand-binds-tighter-than-oror", Category: "conditions",
		Snippet: `[[ -n a || -n b && -z x ]] && echo true || echo false`,
		Why:     "inside [[ ]] && binds tighter, as in C; outside it the two share one level, so the same operators have different precedence on either side of the bracket",
	},
	{
		ID: "cond/command-language-is-the-other-way", Category: "conditions",
		Snippet: `true || true && false; echo "st=$?"`,
		Why:     "the contrast that makes the previous case a finding rather than a curiosity",
	},
	{
		ID: "cond/file-kind-operators", Category: "conditions",
		Snippet: `mkfifo p; touch f; [[ -p p ]] && echo fifo; [[ -p f ]] || echo plain; [[ -c /dev/null ]] && echo chardev; [[ -b /dev/null ]] || echo notblock; [[ -S f ]] || echo notsocket`,
		Why:     "-p, -S, -b and -c classify a file by what it is rather than by its bits, unanimously in the three shells that have the construct — these parsed and then died at run time with `unsupported test`",
	},
	{
		ID: "cond/permission-bit-operators", Category: "conditions",
		Snippet: `touch f; chmod 2600 f; [[ -g f ]] && echo setgid; chmod 4600 f; [[ -u f ]] && echo setuid; chmod 1600 f; [[ -k f ]] && echo sticky; chmod 600 f; [[ -g f || -u f || -k f ]] || echo plain`,
		Why:     "-g, -u and -k read the setgid, setuid and sticky bits — unanimous, and octal modes rather than symbolic ones so the umask has no say",
	},
	{
		ID: "cond/existence-and-kind-operators", Category: "conditions",
		Snippet: `touch f; mkdir d; [[ -e f ]] && echo e-file; [[ -e d ]] && echo e-dir; [[ -e missing ]] || echo e-missing; [[ -d d ]] && echo dir; [[ -d f ]] || echo not-dir; [[ -f f ]] && echo file; [[ -f d ]] || echo not-file`,
		Why:     "-e answers for anything that exists where -f and -d split it by kind: a directory passes -e and fails -f, a plain file the reverse — unanimous in the three shells that have the construct",
	},
	{
		ID: "cond/permission-operators", Category: "conditions",
		Snippet: `touch f; chmod 400 f; [[ -r f ]] && echo readable; [[ -w f ]] || echo not-writable; [[ -x f ]] || echo not-executable; chmod 300 f; [[ -w f ]] && echo writable; [[ -x f ]] && echo executable; [[ -r f ]] || echo not-readable`,
		Why:     "-r, -w and -x ask about access rather than existence, shown by flipping the owner bits under the same name — octal modes so the umask has no say, and each operator is pinned both true and false",
	},
	{
		ID: "cond/size-operator", Category: "conditions",
		Snippet: `touch empty; printf x > full; [[ -s full ]] && echo full; [[ -s empty ]] || echo is-empty; [[ -s missing ]] || echo missing`,
		Why:     "-s wants a size greater than zero, so a file that exists but is empty answers the same as one that does not exist at all",
	},
	{
		ID: "cond/newer-older-and-equal-times", Category: "conditions",
		Snippet: `touch -t 202001010000 old; touch -t 202101010000 new; [[ new -nt old ]] && echo newer; [[ old -ot new ]] && echo older; touch -t 202001010000 twin; [[ old -nt twin ]] || [[ old -ot twin ]] || echo neither`,
		Why:     "-nt and -ot compare modification times, pinned with touch -t rather than a sleep — and equal times are neither newer nor older, which a <= in either direction would get wrong",
	},
	{
		ID: "cond/a-missing-file-is-older-diverges", Category: "conditions",
		Snippet: `touch f; [[ f -nt missing ]]; echo "nt=$?"; [[ missing -ot f ]]; echo "ot=$?"; [[ missing -nt f ]]; echo "mirror=$?"`,
		Why:     "the one disagreement in the comparisons: bash and ksh93 count a file that does not exist as older than any file that does, zsh wants both to exist — the MissingFileIsOlder axis. The mirror is unanimous: a missing file is never *newer* anywhere",
	},
	{
		ID: "cond/same-file-is-identity", Category: "conditions",
		Snippet: `touch a c; ln a b; [[ a -ef b ]] && echo linked; [[ a -ef c ]] || echo distinct`,
		Why:     "-ef is identity rather than equality: a hard link to the file compares equal and a file with the same content does not",
	},
	{
		ID: "cond/terminal-test-closed-descriptors", Category: "conditions",
		Snippet: `[[ -t 0 ]]; echo "t0=$?"; [[ -t 1 ]]; echo "t1=$?"; [[ -t 99 ]]; echo "t99=$?"`,
		Why:     "-t asks whether a descriptor is a terminal, and under the harness none is: stdin is /dev/null and stdout a pipe, so every answer is a quiet false — which is also the honest permanent answer for a runner whose streams are io.Writers",
	},
	{
		ID: "cond/terminal-test-non-number-diverges", Category: "conditions",
		Snippet: `[[ -t x ]]; echo "st=$?"`,
		Why:     "a -t operand that is not a number: bash names an integer at status 2 where ksh93 and zsh answer a plain false at 1 — and bash 3.2 answers 1 too, so the complaint is younger than the operator. The TerminalTestRequiresANumber axis",
	},

	// --- eval and . : the special builtins that run text in this shell ---
	{
		ID: "eval/runs-in-the-calling-shell", Category: "eval and dot",
		Snippet: `x=1; eval "x=2"; echo $x`,
		Why:     "the reason eval is a builtin and not a command: a child process could not change this shell's variable",
	},
	{
		ID: "eval/expands-twice", Category: "eval and dot",
		Snippet: `a=b; b=hi; eval "echo \$$a"`,
		Why:     "the point of eval — the text is expanded once as a word and again as a script, so $$a reaches the second pass as $b",
	},
	{
		ID: "eval/joins-arguments-with-a-space", Category: "eval and dot",
		Snippet: `eval echo a b c`,
		Why:     "eval rejoins its arguments before parsing, so the word boundaries the shell made are not preserved",
	},
	{
		ID: "eval/nothing-to-run-reports-success", Category: "eval and dot",
		Snippet: `false; eval ""; echo st=$?`,
		Why:     "reads like it should leave the status alone and does not: an eval with no commands clears a failure rather than preserving it",
	},
	{
		ID: "eval/is-transparent-to-return", Category: "eval and dot",
		Snippet: `f() { eval return 3; echo NOT-REACHED; }; f; echo st=$?`,
		Why:     "eval is not a scope: `return` inside it returns from the function around it, which is the opposite of what a sourced file does",
	},
	{
		ID: "eval/is-transparent-to-break", Category: "eval and dot",
		Snippet: `for i in 1 2 3; do eval break; echo NOT-REACHED; done; echo done`,
		Why:     "the same transparency for loop control, which a naive implementation running the text on a child runner would lose",
	},
	{
		ID: "eval/unparseable-text-diverges", Category: "eval and dot",
		Snippet: `eval "if"; echo REACHED st=$?`,
		Why:     "POSIX makes a special builtin's failure fatal to a non-interactive shell and only dash still does it; bash, ksh93 and zsh report it and carry on, each with its own status",
	},
	{
		ID: "dot/unterminated-file-names-the-source", Category: "eval and dot",
		// A file with nothing in it but the unterminated construct, so this
		// records the naming and not the axis next to it: three of the four
		// run what they parsed *before* the failure, and a file whose first
		// line printed something would measure both at once.
		Snippet: `printf 'if\n' > p.sh; . ./p.sh; echo "st=$?"`,
		Why:     "one question with three answers and a fourth wrinkle: dash names the file after the location, ksh93 names the *builtin* before it, bash and zsh put the path where the shell's own name goes — and bash does that for a file while labeling `eval` after its name instead",
	},
	{
		ID: "dot/runs-in-the-calling-shell", Category: "eval and dot",
		Snippet: `echo "x=7" > p.sh; . ./p.sh; echo $x`,
		Why:     "the same property as eval, from a file: a sourced assignment survives because no child process was involved",
	},
	{
		ID: "dot/status-is-the-last-command", Category: "eval and dot",
		Snippet: `echo false > p.sh; . ./p.sh; echo st=$?`,
		Why:     "`.` reports what the file's last command reported, which is what makes it usable in a conditional",
	},
	{
		ID: "dot/empty-file-clears-the-status", Category: "eval and dot",
		Snippet: `: > p.sh; false; . ./p.sh; echo st=$?`,
		Why:     "\"the status of the last command\" with no last command is 0 rather than whatever came before, the same surprise as an empty eval",
	},
	{
		ID: "dot/return-ends-the-source", Category: "eval and dot",
		Snippet: `printf 'echo one\nreturn 5\necho NOT-REACHED\n' > p.sh; . ./p.sh; echo st=$?`,
		Why:     "a sourced file *is* a scope for `return`, unlike eval — the one way the two builtins differ in how they treat control flow",
	},
	{
		ID: "dot/searches-path-and-path-wins", Category: "eval and dot",
		Snippet: `mkdir -p d; echo "echo from-path" > d/amb.sh; echo "echo from-cwd" > amb.sh; PATH=$PWD/d:$PATH; . amb.sh`,
		Why:     "an operand with no slash is a PATH lookup and PATH beats an identically named file next to you, which surprises everyone and is unanimous",
	},
	{
		ID: "dot/arguments-diverge", Category: "eval and dot",
		Snippet: `echo 'echo got=$1' > p.sh; set -- OUTER; . ./p.sh INNER; echo after=$1`,
		Why:     "bash, ksh93 and zsh give a sourced file its own positional parameters and restore the caller's afterwards; dash ignores the words entirely, so the file still sees OUTER",
	},
	{
		ID: "dot/missing-file-diverges", Category: "eval and dot",
		Snippet: `. ./nonexistent-xyz.sh; echo REACHED st=$?`,
		Why:     "the other half of the POSIX fatal rule, and the panel splits differently than for eval: dash and ksh93 end the script, bash reports 1 and zsh 127",
	},
	{
		ID: "dot/no-operand-diverges", Category: "eval and dot",
		Snippet: `. ; echo REACHED st=$?`,
		Why:     "four answers to one degenerate input: dash calls it success and does nothing, bash reports 2 and survives, ksh93 reports 2 and exits, zsh reports 1 and survives",
	},
	{
		ID: "dot/cwd-fallback-is-bash-only", Category: "eval and dot",
		Snippet: `echo "echo cwd-hit" > fb.sh; PATH=/usr/bin:/bin; . fb.sh; echo st=$?`,
		Why:     "only bash looks in the current directory once PATH has missed; the other three call it not found, so a script relying on it is bash-only",
	},
	// --- test and [ : a command, not a construct ---------------------
	{
		ID: "test/argument-count-decides", Category: "test",
		Snippet: `test -f; echo "one=$?"; test -n x; echo "two=$?"`,
		Why:     "POSIX defines `test` by argument count before grammar, which is why `test -f` alone is *true*: one argument is a string, and `-f` is a non-empty one. Two arguments make the same word an operator",
	},
	{
		ID: "test/the-quoting-trap", Category: "test",
		Snippet: `u=; test -n $u; echo "unquoted=$?"; test -n "$u"; echo "quoted=$?"`,
		Why:     "the reason `[ ]` needs quotes where `[[ ]]` does not: an unquoted empty expansion is not an empty argument, it is *no* argument, so the count changes and with it the meaning",
	},
	{
		ID: "test/equals-is-not-a-pattern", Category: "test",
		Snippet: `test abc = "a*"; echo "st=$?"`,
		Why:     "the sharpest difference from `[[ ]]`, where the same right operand is a pattern and the answer is the opposite. `test` sees words, so `=` can only compare strings",
	},
	{
		ID: "test/numeric-and-string-compare-differ", Category: "test",
		Snippet: `test 10 -gt 9; echo "numeric=$?"; test 10 = 9; echo "string=$?"`,
		Why:     "the word-spelled operators compare numbers and `=` compares text, which is the same trap `[[ ]]` has and worth pinning on both surfaces",
	},
	{
		ID: "test/and-binds-tighter-than-or", Category: "test",
		Snippet: `test a = b -a b = b -o c = c; echo "st=$?"`,
		Why:     "`-a` binds tighter than `-o`, so this is (false and true) or true rather than false and (true or true)",
	},
	{
		ID: "test/parentheses-group", Category: "test",
		Snippet: `test \( a = b -o c = c \) -a d = d; echo "st=$?"`,
		Why:     "the parenthesised form is words rather than syntax — they are ordinary arguments the builtin matches, which is why they need quoting from the shell",
	},
	{
		ID: "test/bracket-wants-its-bracket", Category: "test",
		Snippet: `[ x ; echo "st=$?"`,
		Why:     "`[` is a command and `]` is an ordinary argument, so nothing but the builtin checks for it — every shell reports it and words it differently",
	},
	{
		ID: "test/a-malformed-expression-is-2", Category: "test",
		Snippet: `test -Q x; echo "st=$?"`,
		Why:     "2 for \"this is not an expression\", deliberately distinct from 1, \"the expression is false\" — a script that branches on $? can tell them apart, and every shell in the panel agrees on the number while wording it four ways",
	},
	{
		ID: "test/classification-diverges", Category: "test",
		Snippet: `test a b c; echo "st=$?"`,
		Why:     "all four report a malformed expression and none agrees on what it is: bash blames the middle word and says a binary operator was expected, dash blames the *first* word, ksh93 calls it an unknown operator and zsh a condition. Status 2 everywhere, so this is a wording and classification divergence rather than a behavioral one",
	},
	{
		ID: "test/double-equal-is-equal", Category: "test",
		Snippet: `test a "==" a; echo "st=$?"`,
		Why:     "bash, ksh93 and zsh take `==` as a second spelling of `=`; dash has only the one, and refusing it is not \"unequal\" but \"that word is not an operator\", so the three words are reported as a malformed expression instead of compared. The quoting is zsh's doing: an unquoted `==` starts with `=` and would be expanded to a path",
	},
	{
		ID: "test/bracket-double-equal", Category: "test",
		Snippet: `[ a "==" a ]; echo "st=$?"`,
		Why:     "`[` is the same builtin under another name, so it answers the same question the same way",
	},
	{
		ID: "test/bracket-names-the-bracket", Category: "test",
		Snippet: `[ a b c ]; echo "st=$?"`,
		Why:     "the same malformed expression as test/classification-diverges, invoked by its other name. Every shell in the panel blames the word that was typed — `[`, not `test` — so the name in a test diagnostic is the builtin's argv[0] and not a fixed part of the wording. zsh carries it in the location rather than the message, and drops it here for the same reason it drops it from `test a b c`: an expression that never parsed is not the builtin's complaint",
	},
	{
		ID: "test/double-equal-unequal", Category: "test",
		Snippet: `test a "==" b; echo "st=$?"`,
		Why:     "the same operator answering false, which is 1 — distinct from the 2 that dash reports for the same words, so the status alone tells the two readings apart",
	},
	{
		ID: "test/file-comparisons", Category: "test",
		Snippet: `touch -t 202001010000 old; touch -t 202101010000 new; test new -nt old; echo "nt=$?"; test old -ot new; echo "ot=$?"; touch a; ln a b; test a -ef b; echo "ef=$?"`,
		Why:     "-nt, -ot and -ef are in every shell in the panel — dash included, whose lack of `[[ ]]` does not extend to the builtin — and with both files present all five agree",
	},
	{
		ID: "test/a-missing-file-is-older-diverges", Category: "test",
		Snippet: `touch f; test f -nt missing; echo "nt=$?"; test missing -ot f; echo "ot=$?"`,
		Why:     "the MissingFileIsOlder axis on the builtin's surface, where dash joins in: bash and ksh93 answer true, dash and zsh want both files to exist — the same split each shell shows in its `[[ ]]`, so it is one axis and not two",
	},
	{
		ID: "test/terminal-test-non-number-diverges", Category: "test",
		Snippet: `test -t x; echo "st=$?"; test -t 99; echo "closed=$?"`,
		Why:     "the TerminalTestRequiresANumber axis: bash and dash refuse the operand with their integer wordings at 2, ksh93 and zsh answer a plain false at 1 — while a numeric descriptor that is simply not a terminal is a quiet 1 everywhere",
	},

	// --- times: the last special builtin, and the most divergent for its size
	//
	// Every snippet masks the digits. The figures are real timings and cannot
	// be compared between two runs of the same shell, let alone between
	// shells; the *shape* is what is being pinned, and it is what diverges.
	{
		ID: "times/shape-diverges", Category: "times",
		Snippet: `times | sed -E "s/[0-9]+/N/g"`,
		Why:     "four shells, four renderings of the same two facts: dash, bash and zsh print the shell's times then its children's, and ksh93 prints labeled `user` and `sys` lines with no children's times at all — genuinely less information rather than the same information rearranged",
	},
	{
		ID: "times/decimals-diverge", Category: "times",
		Snippet: `times | head -1 | sed -E "s/.*\.([0-9]+)s.*/\1/" | tr -d "\n" | wc -c | tr -d " "`,
		Why:     "the number of decimal places is a four-way split nobody would think to ask about: dash 6, bash 3, ksh93 and zsh 2",
	},
	{
		ID: "times/two-lines-of-two", Category: "times",
		Snippet: `times | awk "{print NR\": \"NF}"`,
		Why:     "two lines of two fields in every shell, including ksh93 — its label counts as a field, so the shapes agree here and disagree in what the fields mean",
	},
	{
		ID: "times/goes-to-stdout", Category: "times",
		Snippet: `times >o.txt 2>e.txt; wc -c <e.txt | tr -d " "`,
		Why:     "unanimous, and worth pinning because a pipeline makes it look otherwise: `times 2>&1 >/dev/null | wc -l` suggests zsh uses stderr, and writing each stream to its own file shows all four use stdout",
	},
	{
		ID: "times/status-is-zero", Category: "times",
		Snippet: `times >/dev/null; echo "st=$?"`,
		Why:     "unanimous",
	},
	{
		ID: "case/an-operator-where-a-pattern-belongs", SyntaxError: true, Category: "compound shapes",
		Snippet: `case a in & ) echo hit;; *) echo miss;; esac`,
		Why:     "dash parses this and prints miss: one operator is accepted where the pattern list would start, and the arm it opens matches nothing at all — not `&`, not the empty string. The other three refuse at the `&`. Measured rather than inferred from the diagnostic, because a shell that only worded the error differently would still not reach the esac",
	},
	{
		ID: "times/argument-diverges", Category: "times",
		Snippet: `times foo 2>&1 | sed -E "s/[0-9]+/N/g"; echo "st=$?"`,
		Why:     "dash and bash ignore the argument, zsh refuses it with status 1, and ksh93 makes it a *syntax* error with status 3 — `times` is a reserved word there, so no builtin ever runs. The ksh93 answer is a grammar question rather than a builtin one and is recorded here rather than implemented; zsh's wording needs the builtin name inside the location prefix, which is a structural gap this repository has now measured three times",
	},

	// --- time: the reserved word ------------------------------------------
	// Both cases pin the *shape* rather than any figure: the timings are
	// nondeterministic, so every report is either silenced or reduced to a
	// count before it reaches the record.
	{
		ID: "time/report-lands-outside-the-pipeline", Category: "time",
		Snippet: `(time true 2>&1 | wc -l) 2>/dev/null; echo "st=$?"`,
		Why: "`time` is a reserved word timing the whole pipeline, and its report goes to the *shell's* stderr — the `2>&1` belongs to an element inside the pipeline, so `wc` counts 0 in every shell that has the keyword. " +
			"dash has no keyword at all: `time` resolves to /usr/bin/time there, whose report the element's own redirect does catch, and the count is 1. The outer redirect silences the reports themselves, which are numbers no record could hold",
	},
	{
		ID: "time/report-format-is-per-dialect", Category: "time",
		Snippet: `{ time true; } 2>&1 | grep -c real`,
		Why: "the report redirected from *outside* the construct, reduced to whether it says `real`: bash and ksh93 print a real/user/sys block — apart only in decimals — so 1; zsh reports per pipeline element and only for one that forked, and a lone builtin forks nothing, so 0 lines and 0; " +
			"dash's /usr/bin/time prints one line whose words include `real`, so 1 — three shapes under one count",
	},

	// --- finding a command: the script's PATH, and why it will not run ---
	{
		ID: "path/script-path-governs-lookup", Category: "command lookup",
		Snippet: `mkdir -p d; printf '#!/bin/sh\necho on-path\n' > d/c; chmod +x d/c; PATH=$PWD/d; c`,
		Why:     "setting PATH in a script decides what it can reach — an implementation that asks os/exec instead answers with the *process's* PATH and ignores the script entirely",
	},
	{
		ID: "path/clearing-path-finds-nothing", Category: "command lookup",
		Snippet: `mkdir -p d; printf '#!/bin/sh\necho ran\n' > d/c; chmod +x d/c; cp d/c ./c; PATH=$PWD/d; c; PATH=; c; echo "st=$?"`,
		Why:     "the other half, and the worse one: a script that clears PATH to control what it can reach must not still reach everything on the machine. The command is one this case creates, because a builtin would prove nothing — printf is a builtin in bash and an external in dash, so it answers a different question in each",
	},
	{
		ID: "path/builtins-ignore-path", Category: "command lookup",
		Snippet: `PATH=; echo builtin-ok`,
		Why:     "the contrast that makes the previous case a finding rather than a broken shell: a builtin is not looked up at all",
	},
	{
		ID: "path/first-match-wins", Category: "command lookup",
		Snippet: `mkdir -p a b; printf '#!/bin/sh\necho from-a\n' > a/dup; printf '#!/bin/sh\necho from-b\n' > b/dup; chmod +x a/dup b/dup; PATH=$PWD/a:$PWD/b; dup`,
		Why:     "PATH is searched in order and the first executable wins",
	},
	{
		ID: "path/empty-element-is-the-cwd", Category: "command lookup",
		Snippet: `mkdir -p a; printf '#!/bin/sh\necho from-a\n' > a/dup; printf '#!/bin/sh\necho from-cwd\n' > dup; chmod +x a/dup dup; PATH=:$PWD/a; dup`,
		Why:     "an empty PATH element means the current directory, which is POSIX and is not the same as searching it by default — none of them do that",
	},
	{
		ID: "path/non-executable-is-skipped", Category: "command lookup",
		Snippet: `mkdir -p a b; printf '#!/bin/sh\necho from-a\n' > a/dup; printf '#!/bin/sh\necho from-b\n' > b/dup; chmod -x a/dup; chmod +x b/dup; PATH=$PWD/a:$PWD/b; dup`,
		Why:     "a file without the execute bit does not end the search — the walk carries on and the next entry wins, which an implementation stopping at the first name match gets wrong",
	},
	{
		ID: "path/unrunnable-is-126-not-127", Category: "command lookup",
		Snippet: `printf '#!/bin/sh\necho hi\n' > ne; chmod -x ne; ./ne; echo "st=$?"`,
		Why:     "126 for a file that is there and will not start, against 127 for a name that resolved to nothing — two different failures that a single \"command not found\" collapses into one",
	},
	{
		ID: "path/directory-as-a-command", Category: "command lookup",
		Snippet: `mkdir -p adir; ./adir; echo "st=$?"`,
		Why:     "126 as well, and the reason diverges: bash and ksh93 check for a directory and say so, dash and zsh report the permission error execve returns",
	},
	{
		ID: "export/p-names-what-is-exported", Category: "builtins",
		Snippet: `export V=1; export -p | grep -c -E "^(declare -x|export) V="`,
		Why:     "the listing must name the exported variable in one of the two spellings the shells use — the grep finds either, and found neither before",
	},
	{
		ID: "readonly/p-names-what-is-readonly", Category: "builtins",
		Snippet: `readonly R=2; readonly -p | grep -c -E "R="`,
		Why:     "the same question through readonly -p, whose spelling zsh alone moves to typeset -r",
	},
	{
		ID: "hash/bare-and-r-succeed-everywhere", Category: "builtins",
		Snippet: `hash; echo "st=$?"; hash -r; echo "r=$?"`,
		Why:     "scripts call both defensively; every shell answers 0, and bash alone announces its empty table — on standard output",
	},
	{
		ID: "hash/a-missing-name-diverges", Category: "builtins",
		Snippet: `hash nosuchcmd-xyz; echo "st=$?"`,
		Why:     "three report the name and answer 1; ksh93's hash is alias -t and succeeds in silence",
	},
	{
		ID: "hash/a-builtin-counts-except-in-zsh", Category: "builtins",
		Snippet: `hash shift; echo "st=$?"`,
		Why:     "zsh hashes only what PATH holds, so a builtin is 'no such command' there; measured with shift because macOS ships /usr/bin/cd",
	},
	{
		ID: "complete/registers-in-a-script", Category: "builtins",
		Snippet: `complete -W "a b" foo; echo "st=$?"; complete -p foo`,
		Why:     "every bash_completion.d file runs in a non-interactive shell and must register at status 0 and read itself back; the other three have no such command and die at 127, which is what broke carapace here",
	},
	{
		ID: "complete/removes-and-misses", Category: "builtins",
		Snippet: `complete -W x foo; complete -r foo; echo "r=$?"; complete -p foo; echo "p=$?"`,
		Why:     "-r takes a spec away and naming an unregistered command is an error at 1, in the shell that has the builtin",
	},
	{
		ID: "mapfile/reads-lines-into-an-array", Category: "builtins",
		Snippet: `printf 'a\nb\n' | { mapfile -t arr; echo "${arr[1]}"; }`,
		Why:     "mapfile -t is the idiomatic subshell-free file-to-array read; bash alone has the command, and the array lands in the shell that called it",
	},
	{
		ID: "mapfile/defaults-to-MAPFILE-and-keeps-the-newline", Category: "builtins",
		Snippet: `printf 'a\nb' | { mapfile; printf '[%s]%s' "${MAPFILE[1]}" "${#MAPFILE[@]}"; }`,
		Why:     "with no operand the elements land in MAPFILE, each keeping its delimiter, and a final line the stream never terminated is still an element",
	},
	{
		ID: "readarray/is-mapfile-under-another-name", Category: "builtins",
		Snippet: `arr=(0); printf '1\n2\n3\n4\n' | { readarray -t -s 1 -n 2 -O 1 arr; echo "${arr[0]} ${arr[1]} ${arr[2]} ${#arr[@]}"; }`,
		Why:     "the synonym takes the same letters — skip, cap, and an origin that writes into the array it finds rather than replacing it",
	},
	{
		ID: "mapfile/an-empty-delimiter-means-NUL", Category: "builtins",
		Snippet: `printf 'a\0b\0' | { mapfile -d "" -t arr; printf "[%s]" "${arr[@]}"; echo " n=${#arr[@]}"; }`,
		Why:     "-d renames the delimiter and an *empty* argument is not 'no delimiter' but NUL — the half of find -print0 | mapfile that makes the idiom work, and the reason the letter cannot simply take the argument's first byte",
	},
	{
		ID: "mapfile/a-NUL-delimiter-is-stripped-without-t", Category: "builtins",
		Snippet: `printf 'a\0' | { mapfile -d "" x; echo "len=${#x[0]}"; }; printf 'a:' | { mapfile -d : y; echo "len=${#y[0]}"; }`,
		Why:     "a delimiter is normally kept on the element unless -t asks for it to go — and NUL is the exception, dropped either way, because the value could not carry it",
	},
	{
		ID: "mapfile/reads-a-descriptor", Category: "builtins",
		Snippet: `printf 'x\ny\n' > f; exec 3<f; mapfile -u 3 -t arr; printf "[%s]" "${arr[@]}"; echo " n=${#arr[@]}"`,
		Why:     "-u reads the shell's own descriptor table rather than standard input, which is how the command is used without a pipe putting it in a subshell — the whole reason to prefer it to a while-read loop",
	},
	{
		ID: "path/directory-on-path-is-walked-past", Category: "command lookup",
		Snippet: `mkdir -p first/target real; printf '#!/bin/sh\necho ran\n' > real/target; chmod +x real/target; PATH=$PWD/first:$PWD/real; target; echo "st=$?"`,
		Why:     "a directory whose name matches the command does not stop the PATH search — the shim-directory-early-on-PATH arrangement every version manager relies on",
	},
	{
		ID: "path/directory-on-path-alone-diverges", Category: "command lookup",
		Snippet: `mkdir -p only/target; PATH=$PWD/only; target; echo "st=$?"`,
		Why:     "when the directory was the only match the panel splits three ways: bash says never found (127), dash names it and still says 127, ksh93 and zsh name it at 126",
	},
	{
		ID: "path/missing-path-is-not-a-missing-name", Category: "command lookup",
		Snippet: `./nope; echo "st=$?"`,
		Why:     "a path that is not there and a bare name PATH never had are both 127 and are worded differently in three of the four",
	},
	{
		ID: "exec/replaces-the-shell", Category: "eval and dot",
		Snippet: `exec echo replaced; echo NOT-REACHED`,
		Why:     "the defining property: nothing after a successful exec runs, because in a real shell there is no shell left to run it",
	},
	{
		ID: "exec/no-args-clears-the-status", Category: "eval and dot",
		Snippet: `false; exec; echo st=$?`,
		Why:     "exec with neither a command nor a redirection reports success rather than preserving a failure, the same surprise as an empty eval",
	},
	{
		ID: "exec/redirection-is-permanent", Category: "eval and dot",
		Snippet: `exec > out.txt; echo one; echo two; exec 1>&2; cat out.txt`,
		Why:     "the other exec: no command, so the redirections outlive the command that made them and the shell carries on",
	},
	{
		ID: "exec/successful-exec-runs-no-exit-trap", Category: "eval and dot",
		Snippet: `trap "echo TRAP" EXIT; exec echo hi`,
		Why:     "unanimous, and not a special case in a real shell: the trap died with the process the exec replaced. An implementation standing in a child has to say so explicitly or it prints a TRAP nothing else prints",
	},
	{
		ID: "exec/failed-exec-trap-diverges", Category: "eval and dot",
		Snippet: `trap "echo TRAP" EXIT; exec nosuchcmd-xyz 2>/dev/null`,
		Why:     "the failure is the only case with a shell left to decide anything, and the panel splits: dash and bash run the EXIT trap, ksh93 and zsh drop it",
	},
	{
		ID: "exec/missing-command-is-127", Category: "eval and dot",
		Snippet: `exec nosuchcmd-xyz; echo NOT-REACHED`,
		Why:     "127 and fatal in every shell, so the status is not an axis even though every shell words it differently",
	},
	{
		ID: "exec/unrunnable-file-is-126", Category: "eval and dot",
		Snippet: `echo x > ne.sh; chmod -x ne.sh; exec ./ne.sh; echo NOT-REACHED`,
		Why:     "126 rather than 127: the file is there and will not run, which is a different failure from not finding it and the one an implementation using a single error type gets wrong",
	},
	{
		ID: "exec/directory-reason-diverges", Category: "eval and dot",
		Snippet: `exec /tmp; echo NOT-REACHED`,
		Why:     "same failure and same status of 126, two explanations: bash and ksh93 check for a directory and say so, dash and zsh hand it to execve and report the permission error it returns",
	},
	{
		ID: "exec/in-a-subshell-spares-the-parent", Category: "eval and dot",
		Snippet: `( exec echo in-sub ); echo after`,
		Why:     "a subshell is a separate process in a real shell, so exec replaces only that one — an implementation whose subshells share a process must not call execve in one, or the parent goes too",
	},
	{
		ID: "exec/in-a-pipeline-spares-the-parent", Category: "eval and dot",
		Snippet: `exec echo piped | cat; echo after`,
		Why:     "a pipeline element is a subshell by another name, and the same hazard applies to it",
	},
	{
		ID: "exec/in-a-function-replaces-the-shell", Category: "eval and dot",
		Snippet: `f() { exec echo in-f; }; f; echo NOT-REACHED`,
		Why:     "the contrast that makes the subshell cases a finding: a function is not a subshell, so exec inside one does replace the shell",
	},
	{
		ID: "exec/options-diverge", Category: "eval and dot",
		Snippet: `exec -a myname /bin/sh -c 'echo $0'`,
		Why:     "bash, ksh93 and zsh read options here and dash reads none, so in dash the leading -a is the name of a command and is reported as not found",
	},
	{
		ID: "dot/source-is-not-in-dash", Category: "eval and dot",
		Snippet: `echo "echo via-source" > p.sh; source ./p.sh; echo st=$?`,
		Why:     "`source` is a synonym for `.` in bash, ksh93 and zsh and absent from dash, which is why the substrate keeps `.` and leaves the second name to each dialect",
	},
	{
		ID: "declare/typeset-assigns", Category: "declarations",
		Snippet: `typeset x=1; echo "[$x]"`,
		Why:     "`typeset` is the older of the two names and the one three of the four have; dash has neither and reports a command it cannot find",
	},
	{
		ID: "declare/declare-is-the-second-name", Category: "declarations",
		Snippet: `declare x=1; echo "[$x]"`,
		Why:     "bash and zsh spell it `declare` as well, ksh93 only `typeset`, which makes the name a dialect's answer rather than an axis",
	},
	{
		ID: "declare/local-in-a-posix-function", Category: "declarations",
		Script:  true,
		Snippet: "x=outer\nf() { typeset x=inner; }\nf\necho \"[$x]\"\n",
		Why:     "ksh93 gives a local scope only to a function defined with the `function` word, so here its assignment reaches the caller and bash's and zsh's do not",
	},
	{
		ID: "declare/local-in-a-keyword-function", Category: "declarations",
		Script:  true,
		Snippet: "x=outer\nfunction f { typeset x=inner; }\nf\necho \"[$x]\"\n",
		Why:     "the same declaration in the other definition form, which is where ksh93 agrees with the rest — the pair is the whole of the axis",
	},
	{
		ID: "declare/valueless-local", Category: "declarations",
		Script:  true,
		Snippet: "f() { local u; echo \"[${u-UNSET}]\"; }\nf\n",
		Why:     "zsh alone considers a name declared without a value to be set, so `${u-UNSET}` is empty there and UNSET elsewhere; ksh93 has no `local` at all",
	},
	{
		ID: "declare/valueless-local-with-an-outer-value", Category: "declarations",
		Script:  true,
		Snippet: "u=out\nf() { local u; echo \"[${u-UNSET}]\"; }\nf\necho \"after=[$u]\"\n",
		Why:     "whether the local hides the caller's value: bash says UNSET, dash lets `out` show through until the first assignment, zsh declares it empty — the declare-then-assign-conditionally shape makes the difference silent",
	},
	{
		ID: "declare/valueless-typeset-with-an-outer-value", Category: "declarations",
		Script:  true,
		Snippet: "u=out\nfunction f { typeset u; echo \"[${u-UNSET}]\"; }\nf\necho \"after=[$u]\"\n",
		Why:     "the same question through `typeset` in a keyword function, which is the form ksh93 gives a scope: ksh93 hides the value as bash's local does",
	},
	{
		ID: "declare/local-shadowing-an-exported-name", Category: "declarations",
		Snippet: `export FOO=bar; f() { local FOO=baz; env | grep '^FOO=' || echo "(none)"; }; f; env | grep '^FOO='`,
		Why:     "whether the local inherits the export attribute of the name it shadows: bash and dash hand the child the local's value, zsh hands it nothing at all under that name, and ksh93 has no `local` to ask with. Read through a real child rather than through a listing, because what the attribute decides is what a command is told",
	},
	{
		ID: "declare/local-shadowing-an-imported-name", Category: "declarations",
		Snippet: `f() { local TERM=changed; env | grep '^TERM=' || echo "(none)"; }; f; env | grep '^TERM='`,
		Why:     "the same question where the name arrived in the environment rather than being exported by hand, which is the route that made the two answers one: an imported name is exported by having been imported, so the local either inherits that or does not, and the split is the same either way",
	},
	{
		ID: "declare/typeset-local-shadowing-an-exported-name", Category: "declarations",
		Snippet: `export FOO=bar; function f { typeset FOO=baz; env | grep '^FOO=' || echo "(none)"; }; f; env | grep '^FOO='`,
		Why:     "the same question through `typeset` in a keyword function, which is the only form that asks it of ksh93 — and it answers as zsh does, by a road of its own: this shell's `typeset` takes the attribute off any name it assigns, at the top level as well as in a function",
	},
	{
		ID: "declare/integer-attribute-evaluates-a-later-assignment", Category: "declarations",
		Snippet: `typeset -i n; n=5+2; echo "[$n]"`,
		Why:     "the attribute belongs to the name, so an ordinary assignment made afterwards is an expression — which is the whole point of it",
	},
	{
		ID: "declare/integer-attribute-on-the-declaration", Category: "declarations",
		Snippet: `typeset -i n=3*3; echo "[$n]"`,
		Why:     "the value on the declaring line is evaluated too, so the attribute has to be in place before its own assignment runs",
	},
	{
		ID: "declare/integer-attribute-removed", Category: "declarations",
		Snippet: `typeset -i n=1; typeset +i n; n=5+2; echo "[$n]"`,
		Why:     "`+i` takes the attribute away, which is the one place a shell spells an option with a plus",
	},
	{
		ID: "declare/integer-attribute-with-text", Category: "declarations",
		Snippet: `typeset -i n; n=abc; echo "[$n]"`,
		Why:     "text that is not a number is not an error: `abc` is an expression whose value is an unset name, so the result is zero and nothing is said",
	},
	{
		ID: "declare/readonly-attribute-allows-its-own-value", Category: "declarations",
		Script:  true,
		Snippet: "typeset -r c=1\necho \"[$c]\"\nc=2\necho \"[$c]\"\n",
		Why:     "the declaration assigns and then freezes, so its own value survives and the next assignment does not — applying both at once would refuse the value it was given",
	},
	{
		ID: "declare/print-a-scalar-back", Category: "declarations",
		Snippet: `v='a b'; export e=E; typeset -p v e; echo "st=$?"`,
		Why:     "`-p` writes a declaration back and the three shells with it produce three texts for identical state: one word `declare` with a `--` placeholder and double quotes, `typeset`/`export` with quoting only when needed, and a bare `v='a b'` with no command word at all",
	},
	{
		ID: "declare/print-arrays-back", Category: "declarations",
		Snippet: `arr=(x y); typeset -A m; m[k]='a b'; typeset -p arr m`,
		Why:     "the array shapes move with the form: subscripts always, never, or only where the array has gaps — and the associative element's trailing space in one engine is real. One key only, because key order is promised by nobody",
	},
	{
		ID: "declare/print-a-missing-name", Category: "declarations",
		Snippet: `typeset -p nosuch; echo "st=$?"`,
		Why:     "how scripts test whether a name is set: two shells report it in their own words and answer 1, one prints nothing at all and answers 0 — an axis, not a wording",
	},
	{
		ID: "declare/f-says-a-named-function-back", Category: "declarations",
		Snippet: `f() { if true; then echo one; fi; }; typeset -f f; echo "st=$?"`,
		Why:     "three engines, three renderings of identical state: one gives the brace a line of its own and terminates with `;`, one keeps the brace on the header and terminates with nothing, and one prints the source text verbatim — which this engine does not keep, so the third is refused as unimplemented rather than approximated",
	},
	{
		ID: "declare/capital-f-names-a-function", Category: "declarations",
		Snippet: `f() { echo hi; }; declare -F f; echo "st=$?"; declare -F nosuch; echo "st2=$?"`,
		Why:     "only one shell has `-F` as a function listing — a named operand answers with the bare name, a missing one with silence and 1 — while another spells a *float's precision* with the same letter and answers 0 to both, and two have no `declare` at all",
	},
	{
		ID: "declare/local-reads-the-integer-letter", Category: "declarations",
		Snippet: `f() { local -i n; n=2+3; echo "v=$n"; }; f; echo "st=$?"`,
		Why:     "`local` takes declare's letters in two shells, none at all in a third — where `-i` is a variable name, and a bad one, refused fatally — and does not exist in the fourth",
	},
	{
		ID: "declare/local-readonly-letter", Category: "declarations",
		Snippet: `f() { local -r ro=5; ro=6; echo "unreached"; }; f; echo "after"`,
		Why:     "`local -r` assigns and then freezes, so the reassignment is the readonly refusal — fatal in both shells that read the letter — while the shell with no letters dies earlier, on `-r` as a bad name",
	},
	{
		ID: "declare/local-bad-name", Category: "declarations",
		Snippet: `f() { local 1x=5; echo "in=ok"; }; f; echo "st=$?"`,
		Why:     "four answers to one bad operand: quoted back with its value and carried past, refused fatally with the digit-led wording, refused fatally with the builtin's name left off the line, and `local: not found` from the shell that never had the builtin",
	},
	{
		ID: "declare/bare-local-lists-the-locals", Category: "declarations",
		Snippet: `f() { local x=1 y; local; }; f | grep -c '^declare'; echo "st=$?"`,
		Why:     "a bare `local` writes the running function's locals as clustered declarations in exactly one shell — counted through grep because another lists its whole parameter table there, which is a fact about that engine rather than about the script",
	},
	{
		ID: "declare/global-letter-declares-a-global", Category: "declarations",
		Snippet: `f() { typeset -g gv=7; }; f; echo "gv=$gv st=$?"`,
		Why:     "`-g` reaches the global table from inside a function in the two shells that spell it; the third refuses it with its usage lines and stops — typeset is one of its own special builtins — and the fourth has no typeset at all",
	},
	{
		ID: "declare/global-letter-against-a-local", Category: "declarations",
		Snippet: `x=out; f() { local x=in; typeset -g x=new; echo "in=$x"; }; f; echo "out=$x"`,
		Why:     "the axis inside the letter: with a local standing in front of the name, one engine writes the global cell past it and the other assigns the local it can see — `in=in out=new` against `in=new out=out`",
	},
	{
		ID: "declare/lower-case-attribute", Category: "declarations",
		Snippet: `typeset -l v=ABC; echo "v=$v"; v=DEF; echo "v2=$v"`,
		Why:     "a property of the name, not of the assignment: the declaring value folds and so does every later one, in all three shells with the letter — one folds on expansion rather than assignment, which only its listing can tell apart",
	},
	{
		ID: "declare/upper-case-attribute", Category: "declarations",
		Snippet: `typeset -u w=abc; echo "w=$w"; echo "st=$?"`,
		Why:     "the other direction of the same attribute, unanimous among the shells that have typeset",
	},
	{
		ID: "declare/an-option-typeset-does-not-have", Category: "declarations",
		Snippet: `typeset -q v=1; echo "st=$?"`,
		Why:     "the refusal splits three ways: reported with a usage line and status 2, reported bare with 1, and fatal with the usage lines in the shell whose typeset failures end the script",
	},
	{
		ID: "declare/an-option-local-does-not-have", Category: "declarations",
		Snippet: `f() { local -q x; echo "in"; }; f; echo "st=$?"`,
		Why:     "the same question of `local`, where the shell with no letters reads `-q` as a name and dies on it, and the one with no `local` at all never reaches the question",
	},
	{
		ID: "set/bare-set-lists-the-variables", Category: "builtins",
		Snippet: `v1=plain; v2='has space'; v3="quo'te"; set | grep "^v[123]"; echo "st=$?"`,
		Why:     "one listing, three spellings of the same three values: bare-until-needed with `'\\''` for the embedded quote, always-single-quoted with the quote doubled out, and `$'...'` — filtered to the script's own names because the rest of the listing is the machine's",
	},
	{
		ID: "set/bare-set-and-the-functions", Category: "builtins",
		Snippet: `myfn() { echo hi; }; set | grep -c '^myfn'; echo "st=$?"`,
		Why:     "exactly one shell follows the variables with every defined function; counted rather than shown, so the answer is 1 against three 0s whatever the body's layout",
	},
	{
		ID: "command/capital-v-says-a-sentence", Category: "builtins",
		Snippet: `command -V echo; command -V if; echo "st=$?"`,
		Why:     "POSIX's other letter: every shell answers with its `type` sentence, keyword wording and all, so the option costs no vocabulary of its own until something is missing",
	},
	{
		ID: "command/capital-v-a-name-that-is-nothing", Category: "builtins",
		Snippet: `command -V nosuchcmd_zz; echo "st=$?"`,
		Why:     "the one line `-V` words for itself: two shells blame `command` where their `type` blames `type` or `whence`, the other two keep the shell's name off the line here as there — and the status is `type`'s, 127 in the shell that answers a missing command's number",
	},
	{
		ID: "select/menu-and-choice", Category: "select",
		Snippet: `select x in a b; do echo "got=$x rep=$REPLY"; break; done <<< "1"`,
		Why:     "the base case, and the layouts diverge immediately: two lines in bash and ksh93, one in zsh — and the prompt is `#? ` in two, `?# ` in the third and absent in ksh93, which prints none unless the input is a terminal",
	},
	{
		ID: "select/menu-goes-to-standard-error", Category: "select",
		Snippet: `select x in a b; do echo "$x"; break; done 2>/dev/null <<< "1"`,
		Why:     "the menu and the prompt are both on standard error, so a script's own output survives without them — unanimous, and the reason the base case above shows them at all",
	},
	{
		ID: "select/reply-out-of-range", Category: "select",
		Snippet: `select x in a b; do echo "[$x] rep=$REPLY"; break; done <<< "9"`,
		Why:     "a reply naming no item leaves the variable empty and REPLY holding what was typed, and the body still runs — which is how a script detects it",
	},
	{
		ID: "select/reply-is-not-a-number", Category: "select",
		Snippet: `select x in a b; do echo "[$x] rep=$REPLY"; break; done <<< "zz"`,
		Why:     "the same answer by a different route: nothing is an error, so the two kinds of bad reply are one case in the implementation",
	},
	{
		ID: "select/blank-reply-reprints-the-menu", Category: "select",
		Snippet: `printf '\n2\n' | select x in a b; do echo "got=$x"; break; done`,
		Why:     "a blank line reprints the menu and does not run the body, which is the only way to see the menu again — every other iteration reprints the prompt alone",
	},
	{
		ID: "select/input-ends", Category: "select",
		Snippet: `select x in a; do echo hi; done; echo "st=$?"`,
		Why:     "three answers to what happens when the input runs out: bash reports 1 and writes a newline to standard output, ksh93 reports 1 and writes nothing, zsh reports 0 and closes the prompt line on standard error",
	},
	{
		ID: "select/empty-list", Category: "select",
		Snippet: `select x in; do echo hi; done; echo "st=$?"`,
		Why:     "an empty menu does not prompt at all — the loop never runs and the status is 0, which is the difference between a menu with nothing in it and a menu nobody answered",
	},
	{
		ID: "select/no-list-uses-the-positionals", Category: "select",
		Snippet: `set -- p q; select x; do echo "got=$x"; break; done <<< "2"`,
		Why:     "`in` omitted builds the menu from the positional parameters, the same distinction ForClause draws between an absent list and an empty one",
	},
	{
		ID: "select/ps3-is-read-each-time", Category: "select",
		Snippet: "PS3=A; select x in a b; do PS3=B; echo \"$x\"; done <<< '1\n2'",
		Why:     "the prompt is read fresh each iteration, so a body that changes PS3 changes the next prompt — and the menu itself is printed once",
	},
	{
		ID: "select/break-leaves-the-loop", Category: "select",
		Snippet: `select x in a b; do echo "$x"; break; done <<< "2"; echo "st=$?"`,
		Why:     "`break` ends a menu loop like any other, and the status is the body's rather than the input-ended one",
	},
	{
		ID: "select/the-choice-comes-from-the-shells-own-input", Category: "select",
		Snippet: `select x in a b; do echo "got=$x rep=$REPLY"; break; done`,
		Stdin:   "2\n",
		Why:     "the same menu as the base case with nothing redirected onto it: a select loop reads the shell's own standard input, which is what makes it usable at a prompt at all. The here-string in every other select case hides whether the loop can reach the shell's input or only a redirection",
	},
	{
		ID: "select/is-not-in-dash", Category: "select", SyntaxError: false,
		Snippet: `select x in a; do :; done`,
		Why:     "dash has no `select`, so the word is ordinary and the `do` after it has nothing to open — the grammar flag is what the other three turn on",
	},
	{
		ID: "set/o-at-the-end-of-a-bundle", Category: "pipeline status",
		Snippet: `set -euo noglob; echo "ok=$?"; echo *`,
		Why:     "`-o` is nearly always the last letter of a bundle rather than a word of its own — `set -euo pipefail` is the line at the top of a great many scripts — and the letters before it are ordinary letters that still apply. noglob rather than pipefail because every shell in the panel has it, so the case is about where the `o` sits and not about which options exist",
	},
	{
		ID: "set/o-in-a-bundle-turns-off-too", Category: "pipeline status",
		Snippet: `set -o noglob; set +uo noglob; echo *`,
		Why:     "the same shape with `+`, which turns the named option off — so the bundle is read the same way whichever sign it carries",
	},
	{
		ID: "pipefail/errexit-does-not-always-see-it", Category: "pipeline status",
		Snippet: "if ( set -o pipefail ) 2>/dev/null; then set -eo pipefail; else set -e; fi\nfalse | true\necho reached\n",
		Why:     "`set -e` stops for a failure that only pipefail produced in bash and zsh, and does not in ksh93 — which runs on and reaches the echo. dash reaches it too, for the other reason: it has no pipefail, so the pipeline reports its last element and never failed at all. An ordinary failing pipeline stops all four, so this is about the failure the option adds rather than about pipelines",
	},
	{
		ID: "pipefail/last-failure-not-last-element", Category: "pipeline status",
		Snippet: "if ( set -o pipefail ) 2>/dev/null; then set -o pipefail; fi\n(exit 3) | (exit 4) | true; echo \"st=$?\"\n(exit 4) | (exit 3) | true; echo \"st=$?\"\n",
		Why:     "`set -o pipefail` makes a pipeline report its last *failing* element rather than its last element — 4 then 3, so it is the rightmost failure and not the first. dash has no such option and reports 0 both times, which is the answer the option exists to avoid. The probe is how a portable script asks: run it in a subshell, throw the complaint away, and go without if it did not take",
	},
	{
		ID: "pipefail/all-succeeding-is-zero", Category: "pipeline status",
		Snippet: "if ( set -o pipefail ) 2>/dev/null; then set -o pipefail; fi\ntrue | true | true; echo \"st=$?\"\n(exit 7) | true | true; echo \"st=$?\"\n",
		Why:     "nothing failing is still 0, and a failure in the *first* element is the case a plain pipeline cannot see at all — 7 where the option is available and 0 where it is not",
	},
	{
		ID: "pipefail/turned-off-again", Category: "pipeline status",
		Snippet: "if ( set -o pipefail ) 2>/dev/null; then set -o pipefail; set +o pipefail; fi\n(exit 3) | true; echo \"st=$?\"\n",
		Why:     "the option is an option: `set +o` puts the pipeline back to reporting its last element, so every shell answers 0 here — the one case where the panel agrees for two different reasons",
	},
	{
		ID: "pipestatus/every-element", Category: "pipeline status",
		Snippet: `false | true | false; echo "[${PIPESTATUS[@]}]"`,
		Why:     "`$?` reports the last element only, so without this a script cannot tell the first half of a pipeline failed — bash keeps them under this name, ksh93 has no name for it, and dash rejects the subscript outright because it has no arrays",
	},
	{
		ID: "pipestatus/lowercase-is-zsh", Category: "pipeline status",
		Snippet: `false | true | false; echo "[${pipestatus[@]}]"`,
		Why:     "the same record under the name zsh gives it; the pair of cases is what makes the name a dialect's answer rather than an axis",
	},
	{
		ID: "pipestatus/a-single-command-records-one", Category: "pipeline status",
		Snippet: `false; echo "[${PIPESTATUS[@]}]"`,
		Why:     "it is not only for pipelines: a command on its own records one element, which is why the record is kept by the pipeline runner rather than by the pipe",
	},
	{
		ID: "pipestatus/a-compound-command-records-its-own", Category: "pipeline status",
		Snippet: `if false | true; then :; fi; echo "[${PIPESTATUS[@]}]"`,
		Why:     "the `if` is the command that just ran, so the record holds its status and not the pipeline inside it — the inner one is gone by the time the clause finishes",
	},
	{
		ID: "pipestatus/negation-does-not-reach-it", Category: "pipeline status",
		Snippet: `! false | true; echo "st=$? [${PIPESTATUS[@]}]"`,
		Why:     "`!` inverts what the pipeline reports and not what its elements did, so the record is taken before the inversion",
	},
	{
		ID: "pipestatus/a-bare-assignment", Category: "pipeline status",
		Snippet: `false | true; x=1; echo "[${PIPESTATUS[@]}]"`,
		Why:     "the axis: bash counts an assignment with no command name as a command and replaces the record with one element, and zsh does not count it and leaves the pipeline's two",
	},
	{
		ID: "pipestatus/a-bare-assignment-in-zsh", Category: "pipeline status",
		Snippet: `false | true; x=1; echo "[${pipestatus[@]}]"`,
		Why:     "the other half of that axis, under the name that makes it observable in the shell that answers the other way",
	},
	{
		ID: "pipestatus/unset-then-another-pipeline", Category: "pipeline status",
		Snippet: `unset PIPESTATUS; false | true; echo "[${PIPESTATUS[@]}]"`,
		Why:     "in bash the producer outlives `unset` and the next pipeline fills the name again, which is the opposite of what a produced *scalar* does — `unset RANDOM` leaves an ordinary empty name in every shell",
	},
	{
		ID: "pipestatus/unset-then-another-pipeline-in-zsh", Category: "pipeline status",
		Snippet: `unset pipestatus; false | true; echo "[${pipestatus[@]}]"`,
		Why:     "and in zsh the name is gone for good, which is why `unset` is an axis here and not the rule Dynamic already follows",
	},
	{
		ID: "pipestatus/read-as-a-plain-parameter", Category: "pipeline status",
		Snippet: `false | true | false; echo "[$PIPESTATUS]"`,
		Why:     "a plain `$a` on an array is the first element in bash and ksh93 and every element joined in zsh, which is a rule about arrays and not about this record — it is measured here because this is the array every shell that has one builds without being asked",
	},
	{
		ID: "pipestatus/plain-parameter-on-an-array", Category: "pipeline status",
		Snippet: `a=(x y z); echo "[$a]"`,
		Why:     "the same rule on an ordinary array, which is where it belongs: zsh joins and the other two take the first element",
	},
	{
		// The doubling loop is how a builtin is made to write more than a
		// pipe will hold using nothing but the shell: 2^17 bytes against a
		// buffer of 64K, so the write must block and the reader has already
		// gone. A fixed literal that large would be a corpus file nobody can
		// read, and an external `yes` would be measuring the *command's*
		// death rather than a builtin's.
		ID: "pipeline/a-builtin-writing-into-a-pipe-nobody-reads", Category: "pipeline status",
		Snippet: `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; { echo "$v"; echo reached >&2; } | true; echo after`,
		Why:     "the quiet death, and the one no other case reaches: a builtin whose output goes into a pipe nobody is reading is killed by SIGPIPE where it stands, so `reached` never runs and nothing is said about it — unanimous in all four, and the point of the `>&2` is that a shell which merely swallowed the write would still print it. This is `yes | head` seen from the writing end, and it is the case the corpus was missing while `printf x | { read -d : v; }` measured the same thing by accident: there the write is small enough to fit, so whether it beats the reader's exit is the machine's to decide and the score wandered by one",
	},
	{
		// The same doubling loop and the same dead reader, with one thing
		// changed: SIGPIPE is ignored. An ignore is the one disposition that
		// crosses into a pipeline element intact, so the writer meets the
		// broken pipe with the signal already disarmed and the kernel hands
		// back EPIPE instead. `2>/dev/null` on the failing write alone is
		// what keeps this about the outcome rather than about the wording —
		// four of the panel say something here and each says it differently,
		// which is a separate question from whether the shell is still
		// running to say anything at all.
		ID: "pipeline/an-ignored-broken-pipe-does-not-kill-the-writer", Category: "pipeline status",
		Snippet: `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; ( trap '' PIPE; { echo "$v" 2>/dev/null; echo reached >&2; } | true ); echo after`,
		Why:     "the other half of the quiet death, and the half that shows it was never about the errno: EPIPE is not what kills the writer, SIGPIPE is, and a shell that has ignored SIGPIPE gets the failed write back as an ordinary failure. `reached` runs and `after` follows in every shell measured, against the neighboring case where the identical write ends the writer where it stands — so the difference between dying and carrying on is one `trap ''` and nothing else. The ignore is set inside a subshell rather than at the top: it is inherited from there either way, and a shell that is *the* shell ignores a signal for its whole process, which one harness runs the corpus inside of. bash 3.2 is recorded with a stray newline ahead of `reached`: its failed write leaves the line's terminator queued on the shared output stream and the next write to that stream flushes it out first",
	},
	{
		ID: "cmd/close-brace-as-an-ordinary-word", Category: "command language",
		Snippet: `echo }`,
		Why:     "`}` is reserved only where a command may begin in three of the four, so as an argument it is an ordinary brace — and in zsh it is reserved wherever a word may stand, which is a parse error here and is the same rule that lets `{ echo a }` close without a terminator",
	},
	{
		ID: "cmd/reserved-word-inside-a-brace-group", SyntaxError: true, Category: "command language",
		Snippet: `{ echo a; do :; done; }`,
		Why:     "every shell names the word it stopped on; dash also says what it expected, and only here — `(expecting \"}\")` appears after a group with something in it",
	},
	{
		ID: "cmd/reserved-word-in-an-empty-brace-group", SyntaxError: true, Category: "command language",
		Snippet: `{ fi; }`,
		Why:     "the same failure with nothing before it, which is where dash drops the expectation: an empty group has nothing to be in the middle of",
	},
	{
		ID: "cmd/unterminated-brace-group-line", SyntaxError: true, Category: "command language",
		Snippet: `{ echo a; echo b`,
		Why:     "where an unterminated construct is reported: bash puts it on the line *after* the input's last when the text does not end in one, and the other three on the last line itself",
	},
	{
		ID: "token/an-unterminated-quote-at-end-of-input", SyntaxError: true, Category: "command language",
		Snippet: "echo \"abc",
		Why:     "ksh93 alone closes a quote the input never closed and runs the command, printing abc, where the other three refuse to parse. The grammar flag CloseQuotesAtEOF, and it matters beyond wording: a truncated file runs a command under one shell and not another",
	},
	{
		ID: "token/an-unterminated-substitution-still-refuses", SyntaxError: true, Category: "command language",
		Snippet: "echo $(echo hi",
		Why:     "the other half of the flag, and the reason it is about quotes rather than about leniency at end of input: `$(` is not a quote, so ksh93 refuses this one too — `(' unmatched",
	},
	{
		ID: "cmd/a-malformed-function-header-blames-two-tokens", SyntaxError: true, Category: "command language",
		Snippet: `f ( x ) { echo hi; }`,
		Why:     "which token a bad definition is blamed on says when the shell committed to reading one: bash and dash are already inside a definition looking for `)` and name the word x, ksh93 never entered one and names the `(`, and zsh gets as far as the `}`. The grammar flag FuncDefAtParen, reached most often through a construct a dialect lacks — `[[ ( -n x ) ]]` is a definition of a function called `[[` to a shell without `[[`",
	},
	{
		ID: "arith/integer-division-stays-integer", Category: "arithmetic",
		Snippet: `echo $((3/2))`,
		Why:     "whole numbers mean the same thing everywhere, floats or not — an expression is integer until a float enters it, which is why the shells with floats still answer 1 here",
	},
	{
		ID: "arith/one-float-makes-the-expression-float", Category: "arithmetic", SyntaxError: true,
		Snippet: `echo $((3.0/2))`,
		Why:     "the same division with one operand written as a float, which is the whole difference: 1.5 where the dialect has floats and not a number at all where it does not",
	},
	{
		ID: "arith/a-whole-float-keeps-its-point", Category: "arithmetic", SyntaxError: true,
		Snippet: `echo $((1.5+2.5))`,
		Why:     "zsh writes a trailing point so a float still reads as one, and ksh93 writes the integer — same arithmetic, two spellings of four",
	},
	{
		ID: "arith/float-precision-differs", Category: "arithmetic", SyntaxError: true,
		Snippet: `echo $((0.1+0.2))`,
		Why:     "the classic float, and the two shells show it differently: 15 significant digits rounds it to 0.3 and 17 does not, so the precision is a value the dialect supplies",
	},
	{
		ID: "arith/a-float-may-begin-with-its-point", Category: "arithmetic", SyntaxError: true,
		Snippet: `echo $((.5))`,
		Why:     "`.5` is a literal where the dialect has floats and, where it does not, an operator that cannot be one — which is why the shells without floats blame the point rather than the number it was part of",
	},
	{
		ID: "arith/a-float-may-carry-an-exponent", Category: "arithmetic", SyntaxError: true,
		Snippet: `echo $((1.5e2))`,
		Why:     "the exponent needs its own scan, because the digit reader accepts `e` as a hex digit and stops at a sign — written with its point so that every dialect divides it into the same tokens, which `1e-3` does not: bash reads `1e` as a number with a bad digit and dash reads `1` and stops",
	},
	{
		ID: "arith/hex-is-not-a-float", Category: "arithmetic",
		Snippet: `echo $((0x1e)) $((0x1e-3))`,
		Why:     "the counter-case that keeps the exponent scan honest: `0x1e` is an integer whose digits include an `e`, and the second half is the one that bites — read as an exponent, `0x1e-3` is a literal that will not parse rather than a subtraction",
	},
	{
		ID: "arith/a-remainder-of-floats", Category: "arithmetic", SyntaxError: true,
		Snippet: `echo $((7%2.5))`,
		Why:     "zsh takes a remainder in floating point and ksh93 refuses a float here at all, so `%` is not simply an integer operator in both",
	},
	{
		ID: "arith/a-bitwise-operator-on-a-float", Category: "arithmetic", SyntaxError: true,
		Snippet: `echo $((1.5 & 1))`,
		Why:     "and here they part the other way: zsh truncates to an integer and ksh93 refuses, which is the axis — a remainder is not the same question as a bitwise and",
	},
	{
		ID: "arith/comparing-floats-yields-a-truth", Category: "arithmetic", SyntaxError: true,
		Snippet: `echo $((1.5 < 2))`,
		Why:     "a comparison answers 0 or 1 whatever it compared, so the shell that writes a point after a whole float does not write one here",
	},
	{
		ID: "arith/a-float-assignment-outlives-the-expression", Category: "arithmetic", SyntaxError: true,
		Snippet: `i=0; echo $((i+=1.5)); echo "[$i]"`,
		Why:     "the value left behind is the float and not its truncation, which is what makes the stored form the dialect's business as much as the printed one",
	},
	{
		ID: "arith/dividing-a-float-by-zero", Category: "arithmetic", SyntaxError: true,
		Snippet: `echo $((1.0/0))`,
		Why:     "an infinity rather than the error the integer division gives, and each shell spells it its own way — there is no integer to hand back, so there is nothing to refuse",
	},
	{
		ID: "arith/a-negative-result", Category: "arithmetic",
		Snippet: `echo $((2-7)) $((0-5)) $((-5)) $((~1))`,
		Why:     "every shell prints the minus sign, and nothing in the corpus had a negative result in it — the hand-written integer writer looped while `n > 0`, so all four dialects expanded a negative to nothing at all and `echo $((2-7))` printed an empty line",
	},
	{
		ID: "arith/a-negative-result-in-a-variable", Category: "arithmetic",
		Snippet: `x=$((3-9)); echo "[$x]"`,
		Why:     "the same value stored rather than printed, since an assignment writes the number by the same route an expansion does",
	},
	{
		ID: "pat/extended-pattern-quantifiers", Category: "pattern matching", SyntaxError: true,
		Snippet: `case abc in ?(abc)) echo q;; esac; case aaa in +(a)) echo plus;; esac; case b in !(a)) echo bang;; esac`,
		Why:     "the rest of the family, which only ksh93 has in a `case` pattern — zsh reads each `?`, `+` and `!` as an ordinary character in front of a group of its own, so none of them match",
	},
	{
		ID: "pat/extended-patterns-in-a-condition", Category: "pattern matching",
		Snippet: `[[ abc == @(abc|xyz) ]] && echo yes || echo no`,
		Why:     "bash has extended patterns here and nowhere else — the same text is a syntax error in a `case` pattern there — so where they are available is a separate question from whether the shell has them",
	},
	{
		ID: "pat/a-bare-group-is-alternation", Category: "pattern matching", SyntaxError: true,
		Snippet: `case ab in a(b|c)) echo yes;; *) echo no;; esac`,
		Why:     "zsh takes a group with no quantifier in front of it, which the other three refuse; it is the feature that makes `@(abc|xyz)` a literal `@` and a group there rather than an extended pattern",
	},
	{
		ID: "pat/a-literal-at-before-a-bare-group", Category: "pattern matching", SyntaxError: true,
		Snippet: `case @abc in @(abc|xyz)) echo yes;; *) echo no;; esac`,
		Why:     "the other half of that reading: the subject that zsh matches and the extended-pattern shells do not, which is what proves the two are reading the same text by different rules",
	},
	{
		ID: "pat/a-nested-group-needs-no-quantifier", Category: "pattern matching", SyntaxError: true,
		Snippet: `case b in @(a|(b))) echo y;; *) echo n;; esac`,
		Why:     "ksh93 needs a quantifier at the top level and not inside a group, so `@(a|(b))` matches b there — the lexer is what refuses the bare one, and by the time the matcher sees text it came from somewhere the dialect allows",
	},
	{
		ID: "pat/a-group-from-an-expansion", Category: "pattern matching",
		Snippet: `p="(b)"; case b in $p) echo y;; *) echo n;; esac`,
		Why:     "whether an expansion's text is re-read for groups: ksh93 says yes and matches b, and the other three leave the parentheses literal — the same axis that decides whether `p=\"a*\"` globs, reaching a construct it was not written for",
	},
	{
		ID: "pat/a-subshell-is-not-a-group", Category: "pattern matching",
		Snippet: `(echo hi)`,
		Why:     "the third thing a group must not swallow: a `(` that begins a word opens a subshell, so a group is only ever read mid-word",
	},
	{
		ID: "pat/an-empty-group-is-a-function", Category: "pattern matching",
		Snippet: `f() { echo hi; }; f`,
		Why:     "the case that keeps a bare group from eating a function definition: `()` is empty, and the shell with bare groups rejects an empty one as a pattern, so the definition always wins",
	},
	{
		ID: "pat/an-array-literal-is-not-a-group", Category: "pattern matching",
		Snippet: `a=(x y); echo "[${a[@]}]"`,
		Why:     "and the case that keeps it from eating an array literal: a `(` straight after `=` opens one, never a group — `a=(b|c)` is a parse error in that shell rather than a pattern",
	},
	{
		ID: "exec/lines-run-as-they-are-read", Category: "command language", SyntaxError: true,
		Script:  true,
		Snippet: "echo one\n{ fi; }\necho three\n",
		Why:     "a shell runs what it has read rather than reading everything first, so the first line runs before the second fails to parse — unanimous, and the reason a script that ends badly still does what its good lines said",
	},
	{
		ID: "exec/a-line-is-the-unit-not-a-statement", Category: "command language", SyntaxError: true,
		Script:  true,
		Snippet: "echo one\necho two; { fi; }\n",
		Why:     "the whole line is parsed before any of it runs, so `echo two` never happens even though it precedes the failure and would have been fine on its own",
	},
	{
		ID: "exec/the-exit-trap-fires-after-a-syntax-error", Category: "traps and exit", SyntaxError: true,
		Script:  true,
		Snippet: "trap 'echo bye' EXIT\n{ fi; }\n",
		Why:     "the trap set by a line that ran still fires when a later line will not parse, which is what makes the failure an ending rather than an abort",
	},
	{
		ID: "exec/a-construct-spans-its-lines", Category: "command language", SyntaxError: true,
		Script:  true,
		Snippet: "echo one\nfor i in 1 2\ndo\n  echo $i\ndone\n{ fi; }\n",
		Why:     "the unit stretches past a newline while a construct is open, so the whole loop runs before the line after it fails — a line-at-a-time reader that stopped at the first newline could not run it at all",
	},
	{
		ID: "heredoc/quotes-in-the-body-are-literal", Category: "redirection",
		Script:  true,
		Snippet: "cat <<EOF\ndon't say \"hi\"\nEOF\n",
		Why:     "a here-document body is not a word: a quote in it is an ordinary character with nothing to quote, so running the word lexer over it removed them and turned don't into dont — silently, with status 0",
	},
	{
		ID: "heredoc/a-backslash-escapes-three-things", Category: "redirection",
		Script:  true,
		Snippet: "x=VAL\ncat <<EOF\n\\$x \\\\ \\n \\' \\\"\nEOF\n",
		Why:     "only `$`, a backtick and a backslash; before anything else the backslash stays and so does what follows it, which is the half that makes `\\n` two characters here and one inside double quotes",
	},
	{
		ID: "heredoc/an-unquoted-body-expands", Category: "redirection",
		Script:  true,
		Snippet: "x=VAL\ncat <<EOF\n$x ${x} $(echo sub) $((1+2))\nEOF\n",
		Why:     "the reason an unquoted body is treated differently at all: every substitution happens, which is what makes the quoting of the *delimiter* worth recording",
	},
	{
		ID: "heredoc/a-quoted-delimiter-takes-the-body-whole", Category: "redirection",
		Script:  true,
		Snippet: "x=VAL\ncat <<'EOF'\ndon't $x \\$x \\\\ \"hi\"\nEOF\n",
		Why:     "the other side of the same switch: nothing expands and nothing is escaped, so the body is exactly what was written",
	},
	{
		ID: "heredoc/a-continued-line-is-joined", Category: "redirection",
		Script:  true,
		Snippet: "cat <<EOF\nabc\\\ndef\nEOF\n",
		Why:     "a backslash before the newline joins the lines with nothing between them, which is the one escape that removes rather than reveals a character",
	},
	{
		ID: "heredoc/dash-strips-tabs-and-only-tabs", Category: "redirection",
		Script:  true,
		Snippet: "cat <<-EOF\n\tone\n\t\ttwo\n    spaced\nzero\n\tEOF\necho after\n",
		Why:     "<<- strips every leading tab from a body line and from the delimiter line, and nothing else: a spaces-indented line keeps its spaces, an unindented one is untouched, and the tabbed delimiter still ends the body",
	},
	{
		ID: "heredoc/dash-does-not-strip-spaces-from-the-delimiter", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "cat <<-EOF\nbody\n    EOF\n",
		Why:        "the other half of tabs-and-only-tabs: a spaces-indented delimiter never matches even under <<-, so the body runs to the end of the input — the shell that remarks on that does so here too",
	},
	{
		ID: "heredoc/the-body-follows-the-next-newline", Category: "redirection",
		Script:  true,
		Snippet: "cat <<EOF; echo after\none\nEOF\necho done\n",
		Why:     "the body is not after the operator but after the newline: the rest of the line is ordinary input, so the command after the semicolon runs with the body already consumed",
	},
	{
		ID: "heredoc/two-on-one-line-collect-in-operator-order", Category: "redirection",
		Script:  true,
		Snippet: "cat <<A; cat <<B\nfirst\nA\nsecond\nB\n",
		Why:     "two here-documents behind one newline: the first operator takes the first body and the second the second, unanimously — reversed collection would print them swapped",
	},
	{
		ID: "arith/expansion-happens-before-reading", Category: "arithmetic",
		Snippet: `x='1+'; y=2; echo $(( $x$y ))`,
		Why:     "the substitution is textual and comes first, so the *result* is the expression — 3, which no tree built from `$x$y` as written could give, and the reason an expression containing a `$` has no tree until it runs",
	},
	{
		ID: "arith/a-positional-parameter-in-an-expression", Category: "arithmetic",
		Snippet: `set -- 5 7; echo $(( $2-2 ))`,
		Why:     "`$2` is not a name and the arithmetic grammar has no room for it; it is text that is substituted before the grammar sees anything — the form that /usr/bin/man uses and that this could not read",
	},
	{
		ID: "arith/the-parameter-count-in-an-expression", Category: "arithmetic",
		Snippet: `set -- a b c; echo $(( $# + 1 ))`,
		Why:     "the same for the special parameters, which are the ones a script most often does arithmetic on",
	},
	{
		ID: "arith/the-last-status-in-an-expression", Category: "arithmetic",
		Snippet: `false; echo $(( $? + 1 ))`,
		Why:     "and for `$?`, where the value only exists at the moment the expression runs",
	},
	{
		ID: "arith/a-braced-parameter-in-an-expression", Category: "arithmetic",
		Snippet: `x=4; echo $(( ${x} + 1 ))`,
		Why:     "the braced form goes the same way, which is what makes this about expansion rather than about a longer list of things the grammar accepts",
	},
	{
		ID: "arith/a-command-substitution-in-an-expression", Category: "arithmetic",
		Snippet: `x=5; echo $(( $(echo 2) + x ))`,
		Why:     "a command runs to produce part of the expression, which settles that the substitution is the ordinary one and not a special case for parameters",
	},
	{
		ID: "arith/a-name-is-not-substituted", Category: "arithmetic",
		Snippet: `x=7; echo $(( x + 1 ))`,
		Why:     "the counter-case: without a `$` nothing is substituted and the name is resolved by the evaluator, which is a different rule with a different answer where a value is not a number",
	},
	{
		ID: "opt/set-v-echoes-lines-as-read", Category: "shell options",
		Script:  true,
		Snippet: "set -v\necho a\necho b\n",
		Why:     "the verbose option writes each line back as it is read — not the line that turned it on, which was spent before it took effect. Unanimous, and reachable only from a file: a -c string is read whole before it runs",
	},
	{
		ID: "opt/set-e-carries-the-err-trap", Category: "shell options",
		Snippet: `set -E 2>/dev/null || exit 7; trap "echo ERR" ERR; f(){ false; }; f; echo done`,
		Why:     "set -E makes the ERR trap fire inside functions too — two firings where plain bash has one; dash and ksh93 refuse the letter and zsh means a different option by it",
	},
	{
		ID: "opt/set-n-reads-and-never-runs", Category: "shell options",
		Snippet: `set -n; echo nope; set +n; echo plusn; echo "st=$?"`,
		Why:     "the syntax-check option: nothing after it runs — not even the set +n that would turn it off — and the shell still exits 0. It printed a refusal and ran everything anyway, which is the worst of the three possible behaviors",
	},
	{
		ID: "opt/set-f-turns-off-pathname-expansion", Category: "shell options",
		Snippet: `touch a.txt b.txt; set -f; echo *.txt`,
		Why:     "`-f` is the short spelling of noglob in three of the four; zsh spells that option the long way only and uses `-f` for something else entirely, so the pattern still expands there",
	},
	{
		ID: "opt/set-o-lists-the-table", Category: "shell options",
		Snippet: `set -o | grep errexit | head -1`,
		Why:     "with no name, -o lists every option and its state — four column layouts, two with a header; the grep keeps the row every shell has",
	},
	{
		ID: "opt/set-plus-o-writes-input-back", Category: "shell options",
		Snippet: `set +o | head -1`,
		Why:     "+o writes re-inputtable set commands in three shells; ksh93's one line names only what is on, --default first",
	},
	{
		ID: "opt/set-o-noexec-reads-and-never-runs", Category: "shell options",
		Snippet: "echo before\nset -o noexec\necho after\n",
		Why:     "the long spelling of `set -n`, and it behaves identically in all four: everything after it is read and never run, and the script still ends at 0 — the same option under its other name, which was refused as unimplemented here while the letter worked",
	},
	{
		// The body goes to standard error on purpose, so that the command's
		// own output and the echo of its lines land on one descriptor and
		// their order is visible. With the two apart the same four lines come
		// out in the same order whether the terminator is echoed with the
		// command or after it, which is why this went unnoticed.
		ID: "opt/set-v-echoes-a-here-document-with-its-command", Category: "shell options",
		Script:  true,
		Snippet: "set -v\ncat <<END >&2\nbody\nEND\necho after\n",
		Why:     "all three physical lines of a command carrying a here-document are written back before any of it runs, terminator included — unanimous. The delimiter belongs to the command that opened it rather than to the input after it, and a shell that counts only the lines the parser turned into a command echoes it on the back of the next one, after the body has already been written out",
	},
	{
		ID: "opt/set-v-echoes-the-tail-after-the-last-command", Category: "shell options",
		Script:  true,
		Snippet: "set -v\necho one\n\n\n# the end\n",
		Why:     "the option's contract is to write back what it reads, and the lines after the last command are read like any others: two blank lines and a comment, echoed by all four. Blank lines and comments *between* commands are dragged out by the line that follows them, so only the tail — where there is no line after — shows a shell that echoes per command rather than per line. The comment is last because a record trims trailing newlines, and blank lines at the very end would leave nothing to compare",
	},
	{
		ID: "opt/set-o-verbose-echoes-what-is-read", Category: "shell options",
		Snippet: "echo before\nset -o verbose\necho after\n",
		Why:     "the long spelling of `set -v`: input is written back to stderr as it is read, and never the line that turned it on. From a file all four agree; a -c string is read differently — bash echoes it where dash and zsh do not — so the case pins the route every script uses",
		Script:  true,
	},
	{
		ID: "opt/pipefail-appears-in-the-plus-o-listing", Category: "shell options",
		Snippet: "set +o | grep -o pipefail | head -1\n",
		Why:     "the option is in the listings of the shells that have it, off or on: bash and zsh write the re-inputtable `set +o pipefail` line even while it is off. ksh93's one `+o` line names only what is on, and dash has no such option, so both are silent here — the grep -o keeps the question about the name rather than about four line layouts",
	},
	{
		ID: "opt/pipefail-turned-on-is-listed-on", Category: "shell options",
		Snippet: "if ( set -o pipefail ) 2>/dev/null; then set -o pipefail; fi\nset +o | grep -o pipefail | head -1\n",
		Why:     "the other state: once on, ksh93's active-options line names it too, so all three shells with the option now answer and only dash stays silent",
	},
	{
		ID: "opt/set-o-lists-a-pipefail-row", Category: "shell options",
		Snippet: `set -o | grep pipefail`,
		Why:     "the `set -o` table has a pipefail row wherever the option exists — three column widths, all saying off — and no row at all in dash, whose grep comes up empty at 1",
	},
	{
		ID: "opt/set-m-in-a-script", Category: "shell options",
		Snippet: "set -m\necho \"st=$?\"\necho done\n",
		Why:     "job control in a shell with no terminal splits the panel three ways: bash and ksh93 grant it silently at 0, dash declines it in a remark — `can't access tty; job control turned off` — and still reports 0, and zsh refuses it outright at 1, fatally, so nothing after runs",
	},
	{
		ID: "opt/set-o-monitor-is-the-same-request", Category: "shell options",
		Snippet: "set -o monitor\necho \"st=$?\"\n",
		Why:     "the long spelling gets each shell's same answer, and zsh's refusal echoes the spelling that asked — `monitor` here where the case above says `-m`",
	},
	{
		ID: "kill/the-listing-has-four-shapes", Category: "builtins",
		Snippet: `kill -l | head -1`,
		Why:     "bash numbers five to a row, zsh space-joins one line, ksh93 goes one per line, dash opens with a 0",
	},
	{
		ID: "test/a-non-number-where-one-belongs", Category: "builtins",
		Snippet: `[ a -eq 1 ]; echo "st=$?"`,
		Why:     "three complain at 2; ksh93 is a plain false at 1 with no sentence",
	},
	{
		ID: "read/with-no-variable-diverges", Category: "builtins",
		Snippet: `echo x | { read; echo "r=$? REPLY=[$REPLY]"; }`,
		Why:     "three fill REPLY at 0; dash wants a name — read: arg count, status 2, REPLY untouched",
	},
	{
		ID: "builtin/ksh93s-registers-names", Category: "builtins",
		Snippet: `builtin echo hi; echo "st=$?"`,
		Why:     "bash and zsh run the builtin with its arguments; ksh93's command of the same name registers builtins, so hi is a name it cannot find, at 1; dash has no such command",
	},
	{
		ID: "fc/with-no-history", Category: "builtins",
		Snippet: `fc -l; echo "st=$?"`,
		Why:     "the name must exist — builtin fc was reporting something untrue — and with no history bash and dash answer silence at 0, zsh no-such-event at 1, and ksh93 reads a history file this shell keeps no equivalent of",
	},
	{
		ID: "opt/set-o-noglob-is-unanimous", Category: "shell options",
		Snippet: `touch a.txt b.txt; set -o noglob; echo *.txt`,
		Why:     "the long name means the same thing in all four, which is what makes it the spelling that needs no dialect — and the pair with the case above is the whole of the axis",
	},
	{
		ID: "opt/command-tracking-has-two-long-names", Category: "shell options",
		Snippet: `set -o hashall 2>/dev/null; echo "hashall=$?"; set -o trackall 2>/dev/null; echo "trackall=$?"`,
		Why:     "the same idea under two names, and the membership is not the clean split it looks like: bash has hashall alone, ksh93 trackall alone, and zsh has *both* — so a dialect table that gives trackall to ksh93 only is wrong, which is how this row came to exist",
	},
	{
		ID: "opt/an-unknown-long-name-is-refused", Category: "shell options",
		Snippet: `set -o zzznosuch; echo "on=$?"; set +o zzznosuch; echo "off=$?"`,
		Why:     "a name outside the shell's table is refused in both directions, with four different wordings and two different statuses — the boundary the accept-off policy stops at, since a name that does not exist is not a state anything is already in",
	},
	{
		ID: "opt/an-unknown-letter-is-refused", Category: "shell options",
		Snippet: `set -q; echo "st=$?"; echo alive`,
		Why:     "the letter half of the question the long name asks, and the panel answers the two identically — `-q`, `-j`, `-z` and `-A` are the letters all six refuse, and each shell reports for `set -q` exactly what it reports for `set -o zzznosuch` and ends the script or does not in the same way. bash alone carries on, at 2; dash and ksh93 stop at 2 and zsh at 1. The letter had no dialect answer at all until #483: it reported 2 everywhere and never stopped a script, so the same shell answered its own two spellings differently",
	},
	{
		ID: "opt/a-refused-letter-echoes-the-sign", Category: "shell options",
		Snippet: `set +q; echo "st=$?"`,
		Why:     "the other half of the letter's spelling: bash and ksh93 echo the `+` back where dash and zsh write `-q` whichever way they were asked, so the sign is a verb in two of the wordings and a literal in the other two",
	},
	{
		ID: "opt/an-unknown-letter-at-an-invocation", Category: "shell options",
		Snippet: `echo hi`, Args: []string{"-q", "-c", ArgSnippet},
		Why: "the same refusal by the other route, and it is not the same sentence: nobody names `set` here, bash and ksh93 print the whole *shell* usage rather than the builtin's, and zsh drops the location its run-time form writes. The letter is first in the word deliberately — bash answers 1 with no usage block when a letter it has comes before the one it does not, which is a position axis nobody has asked for yet",
	},
	{
		ID: "opt/an-unknown-long-name-at-an-invocation", Category: "shell options",
		Snippet: `echo hi`, Args: []string{"-o", "zzznosuch", "-c", ArgSnippet},
		Why: "the long spelling by the same route, and bash alone shapes it differently from its own letter: dash, ksh93 and zsh word both the same way here, while bash hands this one to the builtin and prints `<shell>: line 0: <shell>: …` — its own name standing where `set` would",
	},
	{
		ID: "opt/turning-off-a-name-a-shell-does-not-implement", Category: "shell options",
		Snippet: `set +o posix; echo "st=$?"; set +o history; echo "st=$?"`,
		Why:     "the thirteenth line of Homebrew's own brew script is `set +o posix`, and it is the shape this implementation's accept-off/refuse-on policy exists for: turning off what a shell was never doing is a request that has been granted, where turning it *on* would be a promise. Recorded across the panel because the two names split it — bash has both, and the others have neither",
	},
	{
		ID: "opt/noglob-does-not-stop-matching", Category: "shell options",
		Snippet: `set -o noglob; case a.txt in *.txt) echo match;; *) echo no;; esac`,
		Why:     "only the filesystem half is switched off: a pattern in a `case` arm still matches, because that is matching rather than expansion",
	},
	{
		ID: "opt/an-option-can-be-turned-back-off", Category: "shell options",
		Snippet: `touch a.txt; set -o noglob; set +o noglob; echo *.txt`,
		Why:     "`+o` is the other half of the spelling, and without it an option once set could not be unset",
	},
	{
		ID: "opt/an-expansions-result-is-not-globbed-under-noglob", Category: "shell options",
		Snippet: `touch a.txt b.txt; set -o noglob; x=*.txt; echo $x`,
		Why:     "the option reaches the result of an expansion too, which is the same stage by a different route",
	},
	{
		ID: "opt/dollar-dash-shows-a-letter-the-script-set", Category: "shell options",
		Snippet: `set -e; case $- in *e*) echo has-e;; *) echo no-e;; esac`,
		Why:     "`case $- in *e*)` is the standard errexit check, and asking for a letter the script itself set is the deterministic question: the whole string differs per shell — a different baseline in each, and no two order the letters alike — but every one of them shows `e` here",
	},
	{
		ID: "opt/dollar-dash-drops-a-letter-turned-back-off", Category: "shell options",
		Snippet: `set -e; set +e; case $- in *e*) echo has-e;; *) echo no-e;; esac`,
		Why:     "the parameter is produced when it is read rather than stored at startup, and this is the half that proves it: a letter present a moment ago is gone once `set +` takes the option off",
	},
	{
		ID: "opt/dollar-dash-noglob-letter-diverges", Category: "shell options",
		Snippet: `set -o noglob; case $- in *f*) echo lower;; *F*) echo upper;; *) echo neither;; esac`,
		Why:     "the letter itself is an axis: POSIX names `f` and three of the four report it, while zsh reports the capital — `-F` being its own short spelling of noglob, the same split `set -f` measures from the writing side",
	},
	{
		ID: "arith/an-array-element-in-an-expression", Category: "arithmetic",
		Snippet: `a=(3 4 5); echo $(( a[1] ))`,
		Why:     "written without a `$`, so it is read as part of the expression rather than substituted into it — and it counts from the dialect's own base, which is why the same text is 4 in two shells and 3 in the third",
	},
	{
		ID: "arith/a-subscript-is-an-expression", Category: "arithmetic",
		Snippet: `a=(3 4 5); i=1; echo $(( a[i+1] ))`,
		Why:     "the subscript is evaluated rather than taken as text, and the name inside it resolves without a `$` for the same reason the array's does",
	},
	{
		ID: "arith/an-element-past-the-end-is-zero", Category: "arithmetic",
		Snippet: `a=(3 4); echo $(( a[9] + 1 ))`,
		Why:     "reading past the end is zero rather than an error, which is what lets a script test an element it may not have",
	},
	{
		ID: "arith/a-subscript-on-something-that-is-not-an-array", Category: "arithmetic",
		Snippet: `echo $(( nosucharray[0] + 1 ))`,
		Why:     "and the same for a name that was never an array at all, which is the form a version check uses before it knows whether the shell set one",
	},
	{
		ID: "arith/assigning-to-an-element", Category: "arithmetic",
		Snippet: `a=(3 4); (( a[1] = 9 )); echo "${a[1]}"`,
		Why:     "the subscript belongs to the assignment's target, so `a[1] = 9` writes an element rather than evaluating one and throwing it away",
	},
	{
		ID: "cond/a-group-in-a-regex", Category: "conditions",
		Snippet: `[[ abc =~ ^(a|x)bc$ ]] && echo y || echo n`,
		Why:     "the parentheses belong to the regular expression rather than to the shell, so the word does not end at one — which the lexer has to be told, since where a word ends is settled before any parser sees a token",
	},
	{
		ID: "cond/a-group-starting-a-regex", Category: "conditions",
		Snippet: `[[ abc =~ (b) ]] && echo y || echo n`,
		Why:     "and at the very start of the operand, where the `(` would otherwise be taken as an operator before the word scanner ran at all",
	},
	{
		ID: "cond/nested-groups-in-a-regex", Category: "conditions",
		Snippet: `[[ abc =~ ^((a)(b))c$ ]] && echo y || echo n`,
		Why:     "nesting, which is what makes the scan need to balance rather than stop at the first `)`",
	},
	{
		ID: "cond/escaped-parens-in-a-regex", Category: "conditions",
		Snippet: `[[ "(b)" =~ \(b\) ]] && echo y || echo n`,
		Why:     "escaped, they are ordinary characters to the regex and match literal parentheses — the counter-case that keeps the rule about the regex's syntax rather than about the character",
	},
	{
		ID: "cond/a-subshell-is-still-a-subshell", Category: "conditions",
		Snippet: `( echo subshell )`,
		Why:     "the counter-case for the lexer change: a `(` outside a regex operand still opens a subshell, which is what it would stop doing if the rule were not scoped to the operand",
	},
	{
		ID: "param/the-names-with-a-prefix", Category: "parameter expansion",
		Snippet: `ZQ_a=1; ZQ_b=2; echo ${!ZQ_@}`,
		Why:     "yields the *names* rather than any value, sorted — bash and ksh93 have it and the other two call it a bad substitution, which is the same flag that governs `${!x}`",
	},
	{
		ID: "param/the-names-with-a-prefix-one-field-each", Category: "parameter expansion",
		Snippet: `ZQ_a=1; ZQ_b=2; printf "[%s]" "${!ZQ_@}"; echo`,
		Why:     "quoted, `@` is one field per name exactly as `\"$@\"` is one per parameter — joining them would lose a name that contained a space, which a name cannot, but the two spellings still differ",
	},
	{
		ID: "param/the-names-with-a-prefix-joined", Category: "parameter expansion",
		Snippet: `ZQ_a=1; ZQ_b=2; printf "[%s]" "${!ZQ_*}"; echo`,
		Why:     "and `*` is one field with the names joined, the same difference `\"$*\"` has from `\"$@\"` — which is why both spellings exist here too",
	},
	{
		ID: "param/no-names-with-that-prefix", Category: "parameter expansion",
		Snippet: `echo "[${!ZQNOSUCH_@}]"`,
		Why:     "nothing matching is empty rather than an error, which is what lets a script ask for a family of variables it may not have been given",
	},
	{
		ID: "param/the-names-with-a-prefix-are-sorted", Category: "parameter expansion",
		Snippet: `ZQ_b=2; ZQ_a=1; echo ${!ZQ_@}`,
		Why:     "set in the other order and returned in the same one, so the order is the names' rather than the order they happened to be created in — a map has none to inherit",
	},
	{
		ID: "exit/from-inside-a-while-loop", Category: "traps and exit",
		Snippet: `g() { exit 3; }; while :; do g; done; echo after`,
		Why:     "`exit` ends the shell from inside a loop as surely as from anywhere else — the loop must stop and must not touch the status on the way out, which is what turned `exit 3` into an exit of 0",
	},
	{
		ID: "exit/from-inside-an-until-loop", Category: "traps and exit",
		Snippet: `g() { exit 3; }; until false; do g; done; echo after`,
		Why:     "the same for `until`, which read the shell's refusal to run anything as its condition still holding and span forever rather than stopping",
	},
	{
		ID: "exit/from-inside-a-for-loop", Category: "traps and exit",
		Snippet: `g() { exit 3; }; for i in 1 2 3; do g; done; echo after`,
		Why:     "and for `for`, which came out right by accident: a finite list ends on its own, so the loop stopped even without being told to — the shape most scripts use, and the reason this went unnoticed",
	},
	{
		ID: "exit/from-inside-a-nested-loop", Category: "traps and exit",
		Snippet: `g() { exit 3; }; while :; do while :; do g; done; done; echo after`,
		Why:     "an exit passes out through every loop it is inside, unlike `break`, which counts them",
	},
	{
		ID: "exit/break-still-counts-its-loops", Category: "traps and exit",
		Snippet: `while :; do while :; do break 2; done; echo inner; done; echo after`,
		Why:     "the counter-case: `break` still stops only as many loops as it was asked to, which is what an exit must not be confused with",
	},
	{
		ID: "cmd/command-v-names-a-builtin", Category: "command lookup",
		Snippet: `command -v echo`,
		Why:     "`command -v` asks what would run rather than running it, and answers a builtin with the name as written — the portable way a script tests whether it has a tool",
	},
	{
		ID: "cmd/command-v-names-an-external-by-path", Category: "command lookup",
		Snippet: `command -v ls`,
		Why:     "an external is answered by the path, because that is the part a script cannot work out for itself",
	},
	{
		ID: "cmd/command-v-on-nothing", Category: "command lookup",
		Snippet: `command -v nosuchthing; echo "st=$?"`,
		Why:     "nothing found prints nothing at all, which is what makes `command -v x >/dev/null` the usual spelling — and dash answers 127 where the others answer a plain failure",
	},
	{
		ID: "cmd/command-v-names-a-function", Category: "command lookup",
		Snippet: `f() { :; }; command -v f`,
		Why:     "a function is answered like a builtin, by name, even though `command` without -v would refuse to run it",
	},
	{
		ID: "cmd/command-bypasses-a-function", Category: "command lookup",
		Snippet: `echo() { echo overridden; }; command echo hi`,
		Why:     "the whole reason `command` exists: a function may wrap the thing it is named after without calling itself",
	},
	{
		ID: "cmd/command-with-an-option-nobody-has", Category: "command lookup",
		Snippet: `command -q true; echo "st=$?"`,
		Why:     "the CommandRejectsUnknownOption axis: bash, dash and ksh93 refuse an option command does not have, at 2; zsh stops reading options and looks up -q as the command, at 127. Probed with a letter no panel shell owns — -x is a real ksh93 option, and a probe written with -x read ksh93 as tolerant off ksh93's own feature",
	},
	// --- names: what may stand where a builtin wants one ---------------
	{
		ID: "name/export-refuses-an-operand-that-is-not-a-name", Category: "builtin names",
		Snippet: `export 1x; echo "st=$?"`,
		Why:     "the headline: an operand that is not a name was taken silently, which hides a typo. Three answers in four shells — bash reports it and carries on with 1, dash reports it and ends the script with 2, ksh93 and zsh end it with 1 — and three wordings, `not a valid identifier`, `bad variable name` and `is not an identifier`",
	},
	{
		ID: "name/unset-is-not-export", Category: "builtin names",
		Snippet: `unset 1x
echo after`,
		Why: "ksh93 splits the two: `export 1x` ends the script there and `unset 1x` prints the same kind of complaint, returns 1 and carries on. Not `unset` being less special — a bad *option* to it is fatal in ksh93 — so the split is about the kind of failure",
	},
	{
		ID: "name/readonly-may-be-worded-apart-from-export", Category: "builtin names",
		Snippet: `readonly 1x; echo "st=$?"`,
		Why:     "ksh93 says `is not an identifier` for `export` and `invalid variable name` for `readonly`, which is why the wording is per builtin rather than per dialect",
	},
	{
		ID: "name/every-bad-operand-is-reported", Category: "builtin names",
		Snippet: `export a=1 1x b=2 2y; echo "[$a][$b]"`,
		Why:     "bash prints a line for each operand that is not a name and exports the ones that are; the other three print one line because the first is fatal there and the loop never reaches the second",
	},
	{
		ID: "name/a-lone-dash-is-an-operand", Category: "builtin names",
		Snippet: `export -- -; echo "st=$?"`,
		Why:     "`export -` is the form the bug was found on, written with `--` because a bare one lists the environment in zsh and would put this machine's into the record. A special parameter is a name to zsh and not to the other three",
	},
	{
		ID: "name/a-special-parameter-is-not-a-positional-one", Category: "builtin names",
		Snippet: `export -- 0; echo "a=$?"; export -- 12; echo "b=$?"`,
		Why:     "zsh takes `0` and refuses `12`, which is the line between a special parameter and a positional one; the other three refuse both",
	},
	{
		ID: "name/unset-takes-what-export-refuses", Category: "builtin names",
		Snippet: `unset -- 12; echo "a=$?"`,
		Why:     "the two sets zsh adds overlap only at `0`: its `unset` takes a positional where its `export` will not, and refuses the special parameters its `export` takes. One answer could not say that",
	},
	{
		ID: "name/the-operand-may-be-quoted-back-whole", Category: "builtin names",
		Snippet: `export 1x=v; echo "st=$?"`,
		Why:     "bash and ksh93 quote back `1x=v` as written; dash and zsh name `1x`, the part they judged",
	},
	{
		ID: "name/space-around-a-name-is-not-a-name", Category: "builtin names",
		Snippet: `export -- " a "; echo "st=$?"`,
		Why:     "unanimous, and worth a case because the obvious implementation is not: the name check this reached for trims its input first, which is right for judging an assignment and wrong here, and made ` a ` a name",
	},
	{
		ID: "name/bare-unset-is-not-unset-v", Category: "builtin names",
		Snippet: `unset 1x; echo "a=$?"; unset -v 1x; echo "b=$?"`,
		Why:     "bash 5.3's bare `unset` checks nothing and its `unset -v` checks a name, which is the sharpest line in this whole area — and bash 3.2 refuses both, so the panel's two bash columns disagree here on purpose. The other three check either way",
	},
	// --- umask: the symbolic spelling ----------------------------------
	{
		ID: "shift/a-leading-dash-that-is-not-a-number", Category: "builtins",
		Snippet: `shift -x; echo "st=$?"`,
		Why:     "two of the four read it as an *option* and refuse it as one; the other two read it as the count and complain about the number. Same input, two kinds of complaint — and both end the script where a special builtin's failure is fatal",
	},
	{
		ID: "shift/a-count-that-is-an-expression", Category: "builtins",
		Snippet: `set -- a b c; shift 1+1; echo "[$*] st=$?"`,
		Why:     "two of the four evaluate the count as an expression and move two; the other two want a plain number and say so",
	},
	{
		ID: "shift/a-count-that-is-a-name", Category: "builtins",
		Snippet: `set -- a b c; shift nosuchname; echo "[$*] st=$?"`,
		Why:     "the same reading with an unset name, which is zero in an expression — so the two that evaluate shift nothing and succeed where the other two refuse it",
	},
	{
		ID: "wait/a-leading-dash", Category: "builtins",
		Snippet: `wait -x; echo "st=$?"`,
		Why:     "three of the four read it as an option and refuse it in the words their bad options already use; zsh has none and answers with the job it could not find. And none of them ends the script over it, which is the tell that `wait` is not a special builtin however much its neighbors are",
	},
	{
		ID: "wait/dash-dash-ends-the-options", Category: "builtins",
		Snippet: `wait --; echo "st=$?"`,
		Why:     "unanimous, and the control for the case above: the same leading dashes that are refused as an option are taken as the end of them",
	},
	{
		ID: "wait/an-operand-that-is-neither", Category: "builtins",
		Snippet: `wait nosuchjob; echo "st=$?"`,
		Why:     "an operand naming neither a process nor a job: four wordings and no two alike, and three statuses — one quotes it and names both things it could have been, one calls it an illegal number, one lists what it would have taken, and one calls it a job that was not found and reports the 127 of a command that is not there",
	},
	{
		ID: "wait/a-pid-that-is-not-ours", Category: "builtins",
		Snippet: `wait 999999; echo "st=$?"`,
		Why:     "a number that could be a process and is not one of this shell's children. Unanimous on 127, and two of the four say so out loud — so silence here is a wording rather than a behavior",
	},
	{
		ID: "jobspec/wait-by-number", Category: "builtins",
		Snippet: `sleep 0.1 & wait %1; echo "st=$?"`,
		Why:     "the base case: `%1` resolves in all four, so the split below is about specs and not about wait",
	},
	{
		ID: "jobspec/wait-a-number-that-names-nothing", Category: "builtins",
		Snippet: `wait %5; echo "st=$?"`,
		Why:     "a spec that resolves to no job: two shells refuse at a missing command's 127, one at 2 with its own wording — and one says nothing at all and reports 0, which is a behavior and not a wording",
	},
	{
		ID: "jobspec/wait-a-name-that-names-nothing", Category: "builtins",
		Snippet: `wait %nosuchname; echo "st=$?"`,
		Why:     "the `%string` spelling of the same miss. POSIX gives the spelling; the shell that resolves only numbers gives the same refusal it gives `%5`, which is how its answer to the whole feature shows in one line",
	},
	{
		ID: "jobspec/wait-for-the-next-job", Category: "builtins",
		Snippet: `(exit 3) & wait -n; echo "st=$?"`,
		Why:     "one shell's letter: the first finisher's status. The other three are the whole taxonomy of not having it — an illegal option, an unknown option with the usage line, and a job named -n that was not found",
	},
	{
		ID: "jobspec/kill-a-job-that-is-not-there", Category: "builtins",
		Snippet: `kill %9; echo "st=$?"`,
		Why:     "a missing job is its own complaint, not a malformed pid — two wordings and two statuses among the three that answer. The fourth dies of it: a segmentation fault, recorded here as the fact it is and deliberately not reproduced",
	},
	{
		ID: "disown/lets-go-or-only-shields", Category: "builtins",
		Snippet: `sleep 0.3 & disown; jobs; echo "st=$?"; wait`,
		Why:     "what letting go means: two shells take the job out of the table and their `jobs` shows nothing, one only shields it from a HUP and goes on listing it, and one has no disown at all and resolves the name like any missing command",
	},
	{
		ID: "disown/with-nothing-held", Category: "builtins",
		Snippet: `disown; echo "st=$?"`,
		Why:     "a bare disown with no job: worded at 1 twice — differently — and a silent 1 in the third, so the silence is the dialect's answer rather than a gap",
	},
	{
		ID: "ulimit/one-row-of-the-table", Category: "builtins",
		Snippet: `(ulimit -f 12345; ulimit -a) | grep '12345$'; echo "st=$?"`,
		Why:     "`ulimit -a` is four tables that share nothing — labels, order, units — pinned through one row whose value this snippet sets, because the rest of the table is the machine's",
	},
	{
		ID: "dirstack/push-list-pop", Category: "builtins",
		Snippet: `cd; pushd /tmp; dirs; popd; echo "p=$?"`,
		Why:     "the directory stack's success path: one shell prints the stack at every push and pop, one moves in silence and answers only `dirs`, and two have none of the three names. The home directory lists as `~` in both that list at all",
	},
	{
		ID: "dirstack/rotate-to-an-entry-counted-from-the-front", Category: "builtins",
		Snippet: `cd /; pushd /tmp >/dev/null; pushd /usr >/dev/null; pushd +1; dirs; echo "pwd=$PWD"`,
		Why:     "the half of the directory stack that was never exercised: `+N` counts the current directory as entry 0 and turns the stack until entry N is the one the shell stands in. The two shells that have it agree exactly, down to which entry the shell ends up in",
	},
	{
		ID: "dirstack/rotate-to-an-entry-counted-from-the-back", Category: "builtins",
		Snippet: `cd /; pushd /tmp >/dev/null; pushd /usr >/dev/null; pushd -0; dirs; echo "pwd=$PWD"`,
		Why:     "`-N` counts from the other end, so `-0` is the oldest entry — the one a `+N` would have to name by a number that depends on how deep the stack is",
	},
	{
		ID: "dirstack/rotate-past-the-end-of-the-stack", Category: "builtins",
		Snippet: `cd /; pushd /tmp >/dev/null; pushd +9 2>/dev/null; echo "st=$?"; dirs`,
		Why:     "an index there is no entry for: refused, at 1, with the stack left alone. The complaint itself is discarded here and pinned in the dialect tests instead — this engine's stack is a prelude function, and a shell function cannot reach the location prefix (`<shell>: line 1:`) the real builtin writes in front of it",
	},
	{
		ID: "dirstack/rotate-with-nothing-pushed", Category: "builtins",
		Snippet: `cd /; pushd +1 2>/dev/null; echo "st=$?"`,
		Why:     "the same refusal against a stack with nothing in it — a different sentence in both shells, and the same status. The wording is the dialect tests' to pin, for the location reason above",
	},
	{
		ID: "dirstack/popd-an-entry-that-is-not-the-top", Category: "builtins",
		Snippet: `cd /; pushd /tmp >/dev/null; pushd /usr >/dev/null; popd +1; dirs; echo "pwd=$PWD"`,
		Why:     "`popd +N` takes an entry out where it stands and leaves the shell where it is — the current directory only moves when N picks the entry the shell is in, which is what a bare `popd` does",
	},
	{
		ID: "dirstack/popd-with-nothing-pushed", Category: "builtins",
		Snippet: `cd /; popd 2>/dev/null; echo "st=$?"`,
		Why:     "the underflow, which is 1 in both shells that have `popd` and 127 in the two that do not have the name at all — absence rather than divergence, and the case says which by the status",
	},
	{
		ID: "dirstack/clear-the-stack", Category: "builtins",
		Snippet: `cd /; pushd /tmp >/dev/null; pushd /usr >/dev/null; dirs -c; echo "st=$?"; dirs; popd 2>/dev/null; echo "p=$?"`,
		Why:     "`dirs -c` empties the stack in silence, and what is left is the current directory alone — which `dirs` still prints, because it reads $PWD rather than an entry. A `popd` after it is the underflow",
	},
	{
		ID: "dirstack/dirs-one-to-a-line-and-numbered", Category: "builtins",
		Snippet: `cd /; pushd /tmp >/dev/null; pushd /usr >/dev/null; dirs -p; echo "--"; dirs -v`,
		Why:     "`-p` writes one entry to a line and `-v` numbers them, and the numbering is the whole difference between the two shells: a right-aligned two-column number and two spaces against a bare number and a tab",
	},
	{
		ID: "dirstack/dirs-unabbreviated", Category: "builtins",
		Snippet: `cd; pushd / >/dev/null; dirs; echo "--"; dirs -l`,
		Why:     "`-l` writes the home directory out in full where the plain listing abbreviates it to `~`. Both shells agree, and the abbreviation is the only thing the letter turns off",
	},
	{
		ID: "dirstack/dirs-a-letter-it-does-not-have", Category: "builtins",
		Snippet: `dirs -q 2>/dev/null; echo "st=$?"`,
		Why:     "the shape of the option parser, by its status alone: 2 where a bad letter is an invalid *number* with a usage line after it, 1 where it is a bad option, 127 where there is no `dirs`. The same split decides whether `dirs -lv` is two letters or one malformed index",
	},
	{
		ID: "type/f-skips-or-prints-the-function", Category: "builtins",
		Snippet: `f() { :; }; type -f f; echo "st=$?"`,
		Why:     "one letter, two opposite meanings: two shells use -f to leave functions out of the search — so a name that is only a function is not found — and one turns it around and prints the definition. The fourth has no options and answers -f as a name",
	},
	{
		ID: "type/a-lists-a-keyword", Category: "builtins",
		Snippet: `type -a if; echo "st=$?"`,
		Why:     "`-a` on a name with one resolution is the plain sentence, which keeps the case machine-independent while still proving the letter parses — the multi-hit listing is pinned by unit tests against a made path",
	},
	{
		ID: "type/p-on-a-name-that-is-nothing", Category: "builtins",
		Snippet: `type -p nosuchzz_qq; echo "st=$?"`,
		Why:     "the shape of -p's miss: silence at 1 twice, the dialect's own not-found sentence in the shell whose -p answers in sentences, and an operand in the one with no options",
	},
	{
		ID: "type/capital-p-on-a-name-that-is-nothing", Category: "builtins",
		Snippet: `type -P nosuchzz_qq; echo "st=$?"`,
		Why:     "-P is one shell's letter: silence and 1 there, and three refusals — unknown option with usage, bad option, and an operand — everywhere else",
	},
	{
		ID: "enable/a-letter-one-shell-does-not-have", Category: "builtins",
		Snippet: `enable -n cd; echo "st=$?"`,
		Why:     "`enable` is two different builtins: one takes -n to switch a name off, one has no -n at all and reads its options as the *table* to act on. The other two have no `enable`, so the same line is four answers",
	},
	{
		ID: "enable/a-name-that-is-not-a-builtin", Category: "builtins",
		Snippet: `disable nosuchthing; echo "st=$?"`,
		Why:     "`disable` exists in one shell only, and there it complains about a hash table element rather than about a command — where the other three have no such builtin and say so",
	},
	{
		ID: "enable/switching-one-off-and-back-on", Category: "builtins",
		Snippet: `disable cd 2>/dev/null; enable cd 2>/dev/null; echo "st=$?"`,
		Why:     "switching a builtin off is not forgetting it: the name comes back with the same builtin behind it. Standard error is discarded because three of the four have neither word and their complaint is about a missing command, which the case above pins",
	},
	// --- compgen: the completion generator, and the one shell that has it -
	{
		ID: "compgen/names-of-the-shells-own-builtins", Category: "builtins",
		Snippet: `compgen -A builtin retu; echo "st=$?"`,
		Why:     "the shape Homebrew's `brew` uses to check that none of the shell's own commands has been shadowed, and the reason `compgen` is registered at all. One shell answers; the other three have no such command and say so at 127. A prefix narrow enough to name one builtin, because the whole list differs between bash builds",
	},
	{
		ID: "compgen/the-short-letter-for-builtin", Category: "builtins",
		Snippet: `compgen -b retu; echo "st=$?"`,
		Why:     "`-b` is the short spelling of `-A builtin` and must answer identically to the case above — the letter table and the action table are two ways to the same generator, and only a case that runs both notices when one of them drifts",
	},
	{
		ID: "compgen/names-of-the-defined-functions", Category: "builtins",
		Snippet: `f1() { :; }; f2() { :; }; compgen -A function f; echo "st=$?"`,
		Why:     "the second thing this shell can generate from what it knows, and the one whose contents a case can fix: the functions are defined in the snippet, so the answer does not depend on the build the way the builtin list does",
	},
	{
		ID: "compgen/there-is-no-short-letter-for-function", Category: "builtins",
		Snippet: `f1() { :; }; compgen -u f1; echo "st=$?"`,
		Why:     "`-u` is *user* names, not functions, and bash's letters have no short spelling for the function action at all. This shell's letter table said otherwise and answered `f1` where bash answers nothing at 1 — a wrong claim in the one direction nothing catches, since it produces an answer rather than an error",
	},
	{
		ID: "compgen/nothing-matched-is-a-failure", Category: "builtins",
		Snippet: `compgen -A builtin zzzznosuch; echo "st=$?"`,
		Why:     "an empty completion is 1 rather than 0, because the question a completer asks is whether there is anything to offer — the opposite of the usual reading of a command that printed nothing without complaining",
	},
	{
		ID: "compgen/no-action-is-a-quiet-success", Category: "builtins",
		Snippet: `compgen foo; echo "st=$?"`,
		Why:     "a word with nothing to generate it from is 0 and silent, which is the other half of the case above: 1 means asked-and-empty and 0 means never asked",
	},
	{
		ID: "compgen/the-first-word-is-the-one-matched", Category: "builtins",
		Snippet: `compgen -A builtin retu read; echo "st=$?"`,
		Why:     "the extra words are ignored rather than replacing the first, which this shell had the wrong way round; measured both orders, since one order alone cannot tell first-wins from last-wins",
	},
	{
		ID: "compgen/an-action-name-that-is-not-one", Category: "builtins",
		Snippet: `compgen -A nosuchaction x; echo "st=$?"`,
		Why:     "`invalid action name` at 2 — the wording that separates a typo from a shell that is missing something, which is the distinction the action table exists to keep",
	},
	{
		ID: "compgen/an-action-this-shell-does-not-generate", Category: "builtins",
		Snippet: `compgen -A alias zzzznosuch; echo "st=$?"`,
		Why:     "bash generates it and answers 1 for no match; this shell refuses it as not implemented at 2, and the divergence is recorded here deliberately — an action generated from a guess would be a promise the shell cannot keep, and the honest refusal is the answer docs/spec/semantics.md scopes",
	},
	{
		ID: "glob/matches-are-in-order", Category: "expansion",
		Script:  true,
		Snippet: "mkdir -p g && cd g && : > Apple && : > banana && : > Cherry && : > _under && : > 1digit && echo *",
		Why:     "byte order, which every shell in the panel gives under the LC_ALL=C both sweeps run in. Outside that locale three of the four collate and dash does not, and the two platforms disagree about where punctuation goes — none of which this can record, which is exactly why the ordering it does record is worth pinning",
	},
	{
		ID: "axis/export-a-subscripted-operand", Category: "semantics axes",
		Script:  true,
		Snippet: "export 'a[0]'\necho \"st=$?\"\necho after",
		Why:     "ksh93 takes it, bash and dash refuse it in the words they give any bad name, and zsh has a complaint of its own about the subscript — naming the base rather than the operand, and without naming the builtin in the location where its other messages do",
	},
	{
		ID: "axis/readonly-a-subscripted-operand", Category: "semantics axes",
		Script:  true,
		Snippet: "readonly 'a[0]'\necho \"st=$?\"\necho after",
		Why:     "the same operand through the other declaration, where the one shell with its own complaint has a *second* one — about array elements, naming the whole operand, and this time naming the builtin in the location. Two messages in one shell is why which of them names the builtin is a set",
	},
	{
		ID: "axis/unset-a-subscripted-operand", Category: "semantics axes",
		Snippet: "unset 'a[0]'; echo \"st=$?\"; echo after",
		Why:     "three of the four take a subscript as naming an element; dash has no arrays and refuses it in the words it gives any bad name, which is fatal there",
	},
	{
		ID: "umask/symbolic-two-operators-in-one-clause", Category: "umask",
		Snippet: "umask 022; umask u+rw-x; umask",
		Why:     "three of the four apply each operator in turn; zsh takes one per clause and names the second",
	},
	{
		ID: "umask/symbolic-set-with-no-who", Category: "umask",
		Snippet: "umask 022; umask -- =w; umask",
		Why:     "an omitted who before `=` means all three in three of the four. zsh wants one, and names a character that is not in the input at all",
	},
	{
		ID: "umask/symbolic-a-who-with-no-operator", Category: "umask",
		Snippet: "umask 022; umask g; umask",
		Why:     "four answers: ksh93 reads it as `g=`, bash and dash refuse it in their own words, and zsh answers with the complaint it gives a number it could not read",
	},
	{
		ID: "umask/symbolic-the-setuid-letter", Category: "umask",
		Snippet: "umask 022; umask u=rs; umask",
		Why:     "`s` changes no bit a umask has, and three of the four take it anyway. zsh refuses it",
	},
	{
		ID: "umask/symbolic-the-sticky-letter", Category: "umask",
		Snippet: "umask 022; umask u=rt; umask",
		Why:     "the same for `t`, and a different set of shells: dash takes `s` and refuses this one, which is why the two letters are not one question",
	},
	{
		ID: "umask/symbolic-sets-the-mask", Category: "umask",
		Snippet: `umask 022; umask u=rwx,g=,o=; umask`,
		Why:     "the form the issue was filed on and the one scripts write. Unanimous — all four accept it and all four give 0077, which is what made accepting only octal a plain gap rather than a dialect question",
	},
	{
		ID: "umask/symbolic-omitted-who-is-all-three", Category: "umask",
		Snippet: `umask 022; umask -- -w; umask`,
		Why:     "an omitted who is `a`, not the owner: 022 becomes 222 in all four. Written with `--` because a leading `-` is otherwise read as an option, which is how the first measurement of this got a wrong answer",
	},
	{
		ID: "umask/symbolic-clauses-run-left-to-right", Category: "umask",
		Snippet: `umask 022; umask a=r,+w; umask`,
		Why:     "the second clause sees what the first did rather than both applying to the mask in force, which is the difference between 0111 and 0333",
	},
	{
		ID: "umask/symbolic-plus-and-minus-are-not-equals", Category: "umask",
		Snippet: `umask 077; umask g+r; umask -S`,
		Why:     "`+` allows and `=` replaces, so this leaves the group's other bits alone where `g=r` would clear them. The mask records what is taken away and the symbolic form names what is allowed, and getting that backwards is silent",
	},
	{
		ID: "umask/a-bad-number-is-not-a-bad-mode", Category: "umask",
		Snippet: `umask 022; umask -- 1x; echo "st=$?"`,
		Why:     "which spelling was meant is decided by the first character, and the complaint proves it: `1x` is a bad *number* in all four, so it was never a candidate for the symbolic reading despite containing a letter",
	},
	{
		ID: "umask/a-bad-mode-is-not-a-bad-number", Category: "umask",
		Snippet: `umask 022; umask -- u=q; echo "st=$?"; umask`,
		Why:     "the other half: `u=q` is a bad *mode*, and bash names the character where dash and ksh93 quote the whole argument back. The mask is unchanged either way",
	},
	{
		ID: "umask/a-refused-mode-leaves-the-mask-alone", Category: "umask",
		Snippet: `umask 022; umask -- zz 2>/dev/null; umask`,
		Why:     "a mask that could not be read must not half-apply — every clause is parsed before any of it lands",
	},
	// --- alias: the table and the two builtins -------------------------
	{
		ID: "alias/expands-a-command-word", Category: "alias",
		Snippet: `alias a='echo hit'
a`,
		Why: "the headline of the expansion half. dash and ksh93 expand in a script; bash needs `shopt -s expand_aliases` and zsh will not under -c at all, so this is `hit` in two of the four and a command not found in the other two",
	},
	{
		ID: "alias/an-alias-may-hold-a-keyword", Category: "alias",
		// Does not parse under the bash dialect, and that is the point: bash
		// does not expand in a script, so `iff` stays a word and the `fi`
		// after it has nothing to close. Real bash says so too, which is what
		// the case records.
		SyntaxError: true,
		Snippet: `alias iff='if true; then'
iff echo yes; fi`,
		Why: "the reason expansion belongs in the parser rather than in command lookup: the body supplies the `if` and the `then` that the grammar then reads. Nothing substituting at execution time can do this, because the shape of the command is settled by then",
	},
	{
		ID: "alias/a-trailing-space-carries-on", Category: "alias",
		Snippet: `alias a='echo ' b=BEE
a b`,
		Why: "a value ending in a space makes the *next* word eligible too, which is the rule behind `alias sudo='sudo '`. The space makes the word after the value eligible and not the value's own second word",
	},
	{
		ID: "alias/a-self-reference-does-not-loop", Category: "alias",
		Snippet: `alias echo='echo x'
echo hi`,
		Why: "an alias is not expanded twice in one command, which is what stops `alias echo='echo x'` from recurring forever — the second `echo` is an ordinary word and runs the builtin",
	},
	{
		ID: "alias/not-on-the-line-that-defines-it", Category: "alias",
		Snippet: `alias a='echo hit'; a
echo "st=$?"`,
		Why: "expansion happens when a line is *read*, and the whole line was read before the `alias` ran — so this is a command not found in every shell, including the two that expand",
	},
	{
		ID: "alias/a-diagnostic-names-the-use-site", Category: "alias",
		LayoutSensitive: true,
		Snippet: `alias bad='nosuchcmd'
bad`,
		Why: "a command that came from an alias is reported at the line the *alias word* was written on, never a line inside the body. That is what makes a token-level splice honest: every position still points into the real input",
	},
	{
		ID: "alias/a-script-file-is-a-different-route", Category: "alias",
		Script: true,
		Snippet: `alias a='echo hit'
a`,
		Why: "the same two lines as `alias/expands-a-command-word`, from a file instead of -c, and zsh changes its answer: it declines to expand under -c and expands from a script file. So whether aliases expand is a property of *how the program arrived* rather than of the shell, which one boolean on the dialect cannot say",
	},
	{
		ID: "alias/standard-input-is-a-route-too", Category: "alias",
		Args:  []string{"--"},
		Stdin: ArgSnippet + "\n",
		Snippet: `alias a='echo hit'
a`,
		Why: "the third of the three routes, and the one that completes the table: zsh expands here as it does from a file and unlike under -c, so the two rows beside this one are a pair rather than a curiosity. `--` is what says the program is not on the argv",
	},
	{
		ID: "alias/a-body-newline-shifts-later-lines", Category: "alias",
		Script:          true,
		LayoutSensitive: true,
		Snippet: `shopt -s expand_aliases 2>/dev/null
alias two='echo one
echo two'
two
echo "LINENO=$LINENO"`,
		Why: "the one place a token-level splice is distinguishable from a textual one, and the shopt line is there so bash expands too and can be compared. The last line is physically line 5: bash reports 5, and dash, ksh93 and zsh all report 6 because they counted the newline inside the alias body",
	},
	{
		ID: "alias/a-body-newline-shifts-a-diagnostic-too", Category: "alias",
		Script:          true,
		LayoutSensitive: true,
		Snippet: `alias two='echo one
nosuchcmd'
two
echo "LINENO=$LINENO"`,
		Why: "the same shift seen from the other side: the failing command is on the body's *second* line, so the three that count the newline report it a line further down than the alias word — 4 against bash's 3 — and $LINENO after it moves with it. A diagnostic naming a line inside a body is the half `alias/a-diagnostic-names-the-use-site` cannot show, because a one-line body has no second line to name",
	},
	{
		ID: "alias/two-newlines-shift-by-two", Category: "alias",
		Script:          true,
		LayoutSensitive: true,
		Snippet: `alias three='echo one
echo two
echo three'
three
echo "LINENO=$LINENO"`,
		Why: "the shift is per newline rather than per expansion that had one: a three-line body moves the line after it by two. Pinned because one newline cannot tell a count from a flag",
	},
	{
		ID: "alias/an-alias-never-used-shifts-nothing", Category: "alias",
		Script:          true,
		LayoutSensitive: true,
		Snippet: `alias two='echo one
echo two'
echo "LINENO=$LINENO"`,
		Why: "and the shift belongs to the *expansion*, not to the definition: the same two-line body, never used, leaves the line after it where it was written. Unanimous, which is what makes it the control for the two rows above",
	},
	{
		ID: "alias/defines-and-lists-one", Category: "alias",
		Snippet: `alias a='echo x'; alias a`,
		Why:     "the shape of a listing, and it is not unanimous: bash writes `alias ` in front so the line reads back as a command, and the other three write only the assignment",
	},
	{
		ID: "alias/a-value-that-needs-no-quotes", Category: "alias",
		Snippet: `alias b=ls; alias b`,
		Why:     "bash and dash quote every value; ksh93 and zsh quote only one that needs it, so this is `b=ls` in half the panel and `b='ls'` in the other half",
	},
	{
		ID: "alias/a-value-holding-a-quote", Category: "alias",
		Snippet: `alias q="it's"; alias q`,
		Why:     "four engines and no two alike — a backslashed quote, a double-quoted one, and `$'...'` — because each listing has to be text its own shell could read back",
	},
	{
		ID: "alias/a-name-the-table-does-not-hold", Category: "alias",
		Snippet: `alias nope; echo "st=$?"`,
		Why:     "three wordings and a silence, all reporting 1 — and two of the three write it without the shell and line in front, which they do almost nowhere else",
	},
	{
		ID: "alias/unalias-is-not-alias", Category: "alias",
		Snippet: `unalias nope; echo "st=$?"`,
		Why:     "the panel does not pair the two: ksh93 complains about a missing name to `alias` and says nothing to `unalias`, and zsh does exactly the reverse. One answer could not say that",
	},
	{
		ID: "alias/the-status-may-count-what-was-missing", Category: "alias",
		Snippet: `alias n1 n2 n3; echo "st=$?"`,
		Why:     "ksh93 answers with how many it could not find — 3 here — where the other three answer 1 however many were missing. Its own `unalias` does not count",
	},
	{
		ID: "alias/unalias-removes-and-a-removes-all", Category: "alias",
		Snippet: `alias a=1 b=2; unalias a; alias b; unalias -a; alias b; echo "st=$?"`,
		Why:     "the table shrinks by one and then empties, and the second lookup fails — which is what proves -a did anything",
	},
	{
		ID: "select/an-unterminated-final-reply", Category: "select",
		Snippet: `printf 2 | { select x in a b; do echo "picked=$x"; break; done; }; echo "st=$?"`,
		Why:     "a reply with no trailing newline is a reply in zsh and is not one in bash and ksh93, which end the loop with 1 instead. Written through a pipe because a terminal ends every line, so this is only reachable from a pipe or a file — and `read` answers the same question unanimously, which is why that one is the core's behavior and this one is an axis",
	},
	{
		ID: "cmd/command-v-names-a-reserved-word", Category: "command lookup",
		Snippet: `command -v if`,
		Why:     "a word of the grammar is answered too, which is not obvious — it is not a command at all, and every shell in the panel still names it",
	},
	{
		ID: "redir/the-shell-picks-the-descriptor", Category: "redirection",
		Snippet: `exec {fd}> f; echo hi >&$fd; exec {fd}>&-; cat f`,
		Why:     "three of the four allocate a descriptor for `{fd}` and assign its number to the variable; to dash the braces are a command word and exec goes looking for it",
	},
	{
		ID: "redir/a-picked-descriptor-may-outlive-its-command", Category: "redirection",
		Snippet: `echo one {fd}>pf
echo two >&$fd 2>/dev/null || echo dead
cat pf`,
		Why: "bash and zsh keep the picked descriptor open past the simple command that carried it, so the second write lands in the file; ksh93 takes it back with the command's other redirections, and the number the variable still holds is already dead",
	},
	{
		ID: "redir/closing-through-a-name-that-holds-nothing", Category: "redirection",
		Snippet: `exec {nofd}>&-; echo "st=$?"`,
		Why:     "bash calls it an ambiguous redirect and zsh says the parameter holds no descriptor, both with 1; ksh93 says nothing at all and reports success",
	},
	{
		ID: "redir/the-picked-descriptors-name-may-be-an-element", Category: "redirection",
		Snippet: `exec {a[1]}>f; echo "a1=${a[1]}"; echo written >&${a[1]}; exec {a[1]}>&-; cat f`,
		Why:     "the name inside the braces may be a subscripted one, and the panel splits three ways rather than two: bash 5.3 and ksh93 open the file and leave the number in the element, bash 3.2 and dash have no `{name}` token at all, and zsh has one and still will not take a subscript in it — the braces stay a word there, and a word with brackets is a pattern, so zsh reports no matches. Subscript 1 rather than 0 deliberately: it names the first element in every shell with arrays, so zsh's column is about the token and not about the array base",
	},
	{
		ID: "redir/closing-a-descriptor-through-an-element", Category: "redirection",
		Snippet: `exec 3>f; a[1]=3; exec {a[1]}>&-; echo "st=$?"; echo x >&3; echo "after=$?"; cat f`,
		Why:     "the same three-way split on the closing form, which is the one scripts reach for: `exec {COPROC[1]}>&-` is how a coprocess is told its input has ended, and the workaround for a shell without it is a scalar copied out of the element first. The write afterwards is what proves the close happened rather than being reported",
	},
	{
		ID: "jobs/bg-with-no-job-control", Category: "commands",
		Snippet: `bg --version; echo "st=$?"`,
		Why:     "bash and zsh refuse before reading the operand — there is no job control under -c and they say so first; dash and ksh93 read the operand and complain about that instead",
	},
	{
		ID: "cd/cdpath-may-announce-the-move", Category: "cd",
		Snippet: `mkdir -p pool/sub
out=$(CDPATH=./pool cd sub)
[ -n "$out" ] && echo announced || echo silent`,
		Why: "a winning CDPATH entry that is not `.` makes three of the four print where they went; zsh moves in silence. Captured rather than shown, because the announced path is absolute and no two runs share one",
	},
	{
		ID: "commands/coproc-is-one-dialect-s-keyword", Category: "commands",
		Snippet: `coproc cat; echo hi >&"${COPROC[1]}"; read -r l <&"${COPROC[0]}"; echo "$l"`,
		Why:     "bash runs cat in the background with the pipe's near ends in COPROC and reads its own line back; the other three have no such keyword — even zsh, whose coprocess speaks `print -p` rather than an array",
	},
	{
		ID: "commands/coproc-names-a-compound", Category: "commands",
		Snippet: `coproc MY { cat; }; echo hi >&"${MY[1]}"; read -r l <&"${MY[0]}"; echo "$l"`,
		Why:     "a name may be written before a *compound* command, and then it is the name: the near ends arrive in MY rather than in COPROC. Nothing decides this at run time — the shape of what follows settles it while parsing — and bash 3.2 dates the feature by refusing the `}` outright, as do dash and ksh93. zsh has the word and no name to give it",
	},
	{
		ID: "commands/coproc-names-a-subshell", Category: "commands",
		Snippet: `coproc MY ( cat </dev/null ); echo "n=${#MY[@]}"`,
		Why:     "a subshell is a compound command too, so the same rule binds the name — which is the half a parser that only looks for `{` gets wrong. Read back as a count rather than through the descriptors, because a coprocess whose feed is /dev/null has nothing to answer with and the count is the whole claim",
	},
	{
		ID: "commands/coproc-does-not-name-a-simple-command", Category: "commands",
		Snippet: `MY() { echo ran-MY; }; coproc MY cat; read -r l <&"${COPROC[0]}"; echo "l=$l COPROC=${#COPROC[@]} MY=${#MY[@]}"`,
		Why:     "the other side of the same rule, and the one worth pinning: before a *simple* command the first word is the command, so `coproc MY cat` runs MY and the array is the default COPROC. A function named MY is what makes that visible without a race — bash reports `MY: command not found` from the background job otherwise, whenever the job gets there — and the line read back is the function's own output, which no shell that had taken MY as a name could produce",
	},
	{
		ID: "syntax/an-unmatched-double-quote", Category: "diagnostics",
		SyntaxError: true,
		Snippet:     `echo "abc`,
		Why:         "one end of file, three sentences and a silence: bash wants the matching mark, dash calls the string unterminated, zsh calls the opener unmatched — and ksh93 closes the quote, runs the command, and prints abc",
	},
	{
		ID: "syntax/standard-input-reads-on-past-a-parse-failure", Category: "diagnostics",
		SyntaxError: true,
		Args:        []string{"--"},
		Stdin:       ArgSnippet + "\n",
		Snippet: `echo one
{ fi; }
echo three`,
		Why: "the program arriving on standard input, where zsh alone reports the bad line and then reads the next one: it prints one, the complaint, and three, and exits 0 where the other three stop at the complaint. `--` is what says the program is not on the argv",
	},
	{
		ID: "syntax/a-file-stops-at-the-same-parse-failure", Category: "diagnostics",
		SyntaxError: true,
		Script:      true,
		Snippet: `echo one
{ fi; }
echo three`,
		Why: "the same three lines from a file, and zsh stops: nothing after the complaint and status 1. Which says the row above is about the *route* rather than about the text, and is the reason the answer cannot live with the parse status",
	},
	{
		ID: "syntax/reading-on-does-not-invent-a-status", Category: "diagnostics",
		SyntaxError: true,
		Args:        []string{"--"},
		Stdin:       ArgSnippet + "\n",
		Snippet: `{ fi; }
exit 7`,
		Why: "the shell that reads on does not force a status either: what runs after the bad line reports as it always would, so this is 7 there. In the other three nothing after the complaint runs at all and the status is the parse failure's",
	},
	{
		ID: "syntax/reading-on-with-nothing-left-to-read", Category: "diagnostics",
		SyntaxError: true,
		Args:        []string{"--"},
		Stdin:       ArgSnippet + "\n",
		Snippet: `echo one
{ fi; }`,
		Why: "the other half of the status: where the bad line is the last one there is nothing to report but the failure, so the shell that reads on still exits with the parse status. Together with the row above this says the status is left behind rather than chosen",
	},
	{
		ID: "syntax/an-unmatched-command-substitution", Category: "diagnostics",
		SyntaxError: true,
		Snippet:     `echo $(echo`,
		Why:         "a substitution is not a quote even to the shell that closes quotes at end of input: all four refuse, naming the closer, the end of the file, the opener, and the nearby text respectively — and bash alone counts the line as the one after the input's last",
	},
	{
		ID: "syntax/a-brace-opened-inside-a-quote", Category: "diagnostics",
		SyntaxError: true,
		Snippet:     `echo "${x"`,
		Why:         "three of the panel blame the quote the `${` began inside; ksh93 blames the quote character itself, `\"' unexpected — the one shape of unterminated input it refuses with the quote in it",
	},
	{
		ID: "expansion/an-operator-where-the-name-belongs", Category: "diagnostics",
		Snippet: `echo "${%x}"; echo "st=$?"`,
		Why:     "an operator where the parameter name belongs: ksh93 refuses while reading and must name the `%` — the token used to come through blank there, ``' unexpected — while the other three defer and call it a bad substitution when the expansion is reached",
	},
	{
		ID: "redir/noclobber-names-its-refusal", Category: "redirection",
		Snippet: `set -C; echo a > f; echo b > f; echo "st=$?"`,
		Why:     "the refusal is unanimous and the sentence is not: bash cannot overwrite an existing file, ksh93 says it already exists with the errno in brackets, dash and zsh word it as any other failed create",
	},
	{
		ID: "redir/exec-with-a-redirection-that-cannot-be-made", Category: "redirection",
		Snippet: `exec 3>/nope/x; echo after`,
		Why:     "POSIX makes a redirection error on a special builtin fatal to a non-interactive shell, and the panel splits three to two over it: dash stops at 2, ksh93 and bash-as-`sh` stop at 1, and bash and zsh complain and print `after`. The bash and bash-as-`sh` rows are the same binary, which is what says the answer belongs to posix mode rather than to a shell",
	},
	{
		ID: "redir/a-failed-redirection-on-a-colon", Category: "redirection",
		Snippet: `: 3>/nope/x; echo after`,
		Why:     "the same rule reached without `exec`: `:` is a special builtin too, and every column answers exactly as it does above — so the rule is about which builtin carries the redirection and not about replacing the shell",
	},
	{
		ID: "redir/a-failed-redirection-on-an-ordinary-command", Category: "redirection",
		Snippet: `true 3>/nope/x; echo after`,
		Why:     "the boundary, and it is unanimous: on a builtin POSIX does not mark special nothing stops anywhere, so a rule written as `a failed redirection is fatal` would be wrong in five columns at once",
	},
	{
		ID: "redir/a-failed-redirection-on-a-compound-command", Category: "redirection",
		Snippet: `{ echo x; } 3>/nope/x; echo after`,
		Why:     "the other half of the boundary: a redirection written on a group belongs to the group and not to any builtin, so every column complains and carries on — including the three that stop for the identical redirection on `exec`",
	},
	{
		ID: "redir/a-duplication-target-of-more-than-one-digit", Category: "redirection",
		Snippet: `echo hi >&10; echo "st=$?"`,
		Why:     "dash alone will not take a duplication target wider than one digit and refuses before it looks at what is open — `Syntax error: Bad fd number`, status 2, and nothing after it runs. The other four read the number and report `Bad file descriptor` at 1. bash 3.2's `hi` is not a fifth answer: that build parks its own saved streams at descriptor 10, so the write really does land somewhere, without crossing any boundary — the next case is the tell",
	},
	{
		ID: "redir/reading-through-a-wide-duplication-target", Category: "redirection",
		Snippet: `cat <&10; echo "st=$?"`,
		Why:     "the same target read from instead of written to, and the row that settles bash 3.2: it complains here where it printed `hi` for `>&10`, which is what a saved stream open for writing looks like from both sides. dash refuses this one identically, so its rule is about the width of the word and not about the direction",
	},
	{
		ID: "redir/a-wide-duplication-target-with-a-leading-zero", Category: "redirection",
		Snippet: `echo hi >&08; echo "st=$?"`,
		Why:     "the refusal is about the width and not the value: `08` names descriptor 8, which is a number every shell would otherwise accept, and dash refuses it exactly as it refuses `10`. That is what keeps this a question of its own rather than a second reading of the one about numbers the open-file limit will not give out",
	},
	{
		ID: "redir/a-duplication-target-that-expands-to-two-digits", Category: "redirection",
		Snippet: `exec 2>/dev/null; n=10; echo hi >&$n; echo "st=$?"`,
		Why:     "the check is on the *expanded* word rather than on what was typed, which is what says it cannot be the lexer's: dash refuses `>&$n` once `n` holds two digits and takes it when `n` holds one. Standard error is put aside first because the shells name the target differently here — as written or as expanded — and the question is which of them stops",
	},
	{
		ID: "redir/a-failed-redirection-inside-a-subshell", Category: "redirection",
		Snippet: `( exec 3>/nope/x; echo inner ); echo after`,
		Why:     "what `ends the shell` means where there is a process boundary: the three that stop lose `inner` and still print `after` at status 0, so the subshell ends and the parent does not — which our cloned-runner subshells have to reconstruct by hand",
	},
	{
		ID: "set/posix-mode-makes-a-failed-redirection-fatal", Category: "invocation",
		Snippet: `set -o posix; exec 3>/nope/x; echo after`,
		Why:     "the same binary, both answers: bash 5.3 and bash 3.2 print `after` without this line and stop at 1 with it, which is the bash-as-`sh` column reached at run time. The other three have no such name and refuse the `set` instead, each in its own words",
	},
	{
		ID: "set/leaving-posix-mode-restores-the-shells-own-answer", Category: "invocation",
		Snippet: `set -o posix; set +o posix; exec 3>/nope/x; echo after`,
		Why:     "the round trip, which is what makes it a mode rather than a one-way door: bash goes back to printing `after` at 0. Turning it off is also the direction a shell without a posix mode can honestly grant, and the thirteenth line of Homebrew's own script",
	},
	{
		ID: "shift/an-operand-that-was-never-given", Category: "builtins",
		Snippet: `shift; echo "st=$?"`,
		Why:     "past the end with no count written down, ksh93 reports `(null)` — the operand it did not get — where its complaint about `shift 99` names the 99; dash keeps one sentence for both and bash and zsh keep their usual answers",
	},

	// --- dialect-scoped builtins (#431): zsh's setopt/unsetopt and emulate,
	// ksh93's whence and print, bash's caller. Each is one shell's own word;
	// the others' command-not-found answers are the evidence of that, and are
	// recorded rather than avoided.
	{
		ID: "setopt/normalizes-zsh-spellings", Category: "builtins",
		Snippet: `setopt No_Glob; echo "st=$?"; echo x*`,
		Why:     "zsh's option namespace ignores case and underscores, so No_Glob is noglob and the glob comes back literal; everyone else has no setopt at all and says so",
	},
	{
		ID: "setopt/acts-past-a-bad-name", Category: "builtins",
		Snippet: `setopt no_glob zzqq; echo "st=$?"; echo x*`,
		Why:     "zsh refuses the name it does not have — `no such option`, as typed, status 1 — and still acts on the one it does: the glob is off despite the complaint",
	},
	{
		ID: "setopt/the-no-prefix-strips-once", Category: "builtins",
		Snippet: `setopt no_no_glob; echo "st=$?"`,
		Why:     "a single `no` negates and a second is nobody's option: zsh says `no such option: no_no_glob` rather than restoring the glob",
	},
	{
		ID: "setopt/bare-lists-the-deviations", Category: "builtins",
		Snippet: `setopt err_exit no_clobber; setopt`,
		Why:     "a bare setopt lists what differs from zsh's defaults, canonically spelled and ordered by the base name — noclobber prints between allexport's place and errexit — with nohashdirs as the -c baseline's one line",
	},
	{
		ID: "setopt/shwordsplit-is-an-option", Category: "builtins",
		Snippet: `x="a b"; setopt shwordsplit; set -- $x; echo "n=$#"`,
		Why:     "zsh's no-splitting is an option, not a law: shwordsplit turns the sh behavior on. The name is zsh's own for a semantics axis, which is what makes it a one-line dialect answer",
	},
	{
		ID: "setopt/nonomatch-passes-the-glob-through", Category: "builtins",
		Snippet: `unsetopt nomatch; echo x*; echo "st=$?"`,
		Why:     "unsetopt is setopt inverted, and nomatch is the option behind zsh's `no matches found`: off, the pattern passes through as itself the way the other shells default to",
	},
	{
		ID: "setopt/errexit-is-the-same-switch-as-set-e", Category: "builtins",
		Snippet: `setopt err_exit; false; echo reached`,
		Why:     "setopt and set -o drive one table: err_exit ends the run exactly as set -e would, rather than being a second errexit that drifts",
	},
	{
		ID: "emulate/names-the-current-mode", Category: "builtins",
		Snippet: `emulate; emulate sh; emulate`,
		Why:     "a bare emulate answers which shell zsh is currently being — zsh until something changes it, and the word that changed it after",
	},
	{
		ID: "emulate/sh-moves-the-measured-axes", Category: "builtins",
		Snippet: `x="a b"; emulate sh; set -- $x; echo "n=$#"; echo x*; a=(p q); echo "${a[1]}"`,
		Why:     "what `emulate sh` means, measured probe by probe: unquoted expansions split, a failed glob passes through as itself, and arrays base at zero — three axes, not a new shell",
	},
	{
		ID: "emulate/resets-the-options", Category: "builtins",
		Snippet: `setopt err_exit; emulate zsh; false; echo reached`,
		Why:     "a plain emulation resets options to its defaults with no -R asked for: the errexit set before it is gone and the script reaches its end",
	},
	{
		ID: "emulate/dash-c-restores-after", Category: "builtins",
		Snippet: `setopt no_glob; emulate sh -c 'echo inner'; echo x*; echo "st=$?"`,
		Why:     "-c runs the string under the emulation and puts everything back, options included: the no_glob set before it still holds after, so the glob prints literal at 0",
	},
	{
		ID: "emulate/an-unknown-mode-is-passed-over", Category: "builtins",
		Snippet: `emulate fish; echo "st=$?"; emulate`,
		Why:     "a word naming no emulation is silence and 0 in zsh, the mode unchanged — measured, and strange enough to be worth pinning against the guess that it would complain",
	},
	{
		ID: "whence/bare-is-the-resolution", Category: "builtins",
		Snippet: `whence echo; echo "st=$?"; whence if; f() { :; }; whence f`,
		Why:     "ksh93's own question about a name, answered bare: a builtin, keyword or function is its own name and nothing more. zsh has a whence too — the same ancestry — and bash and dash have none",
	},
	{
		ID: "whence/v-is-the-sentence", Category: "builtins",
		Snippet: `whence -v echo; echo "st=$?"`,
		Why:     "whence -v is what ksh93 spells type as, so the sentence is type's; zsh words its own",
	},
	{
		ID: "whence/a-name-that-resolves-to-nothing", Category: "builtins",
		Snippet: `whence nosuchcmd431; echo "st=$?"; whence -v nosuchcmd431; echo "st=$?"`,
		Why:     "bare whence misses in silence at 1 where -v says so out loud — ksh93's complaint naming whence itself — and the two shells that have the builtin disagree only about the wording",
	},
	{
		ID: "whence/p-searches-path-alone", Category: "builtins",
		Snippet: `mkdir -p d; printf '#!/bin/sh\n' > d/tool431; chmod +x d/tool431; PATH=$PWD/d:$PATH; f() { :; }; w=$(whence -p tool431); case $w in */d/tool431) echo path-ok;; *) echo "got=$w";; esac; whence -p f; echo "st=$?"`,
		Why:     "-p is the PATH search with functions and builtins invisible: the file answers with its path — matched rather than printed, because zsh spells the temp directory through /private and ksh93 does not — and the function is nobody, status 1",
	},
	{
		ID: "whence/an-alias-answers-as-its-value", Category: "builtins",
		Snippet: `alias ll="ls -l"; whence ll; whence -v ll`,
		Why:     "the one resolution that is the parser's fact rather than the runner's: ksh93 prints the value quoted and the -v sentence calls it an alias; zsh words the same answer its own way",
	},
	{
		ID: "whence/an-unknown-letter", Category: "builtins",
		Snippet: `whence -z echo; echo "st=$?"`,
		Why:     "ksh93 refuses with `unknown option` and its whence usage line at 2, the same shape its other builtins use; zsh's whence reads -z as its own flag set differs",
	},
	{
		ID: "whence/c-is-the-csh-listing", Category: "builtins",
		Snippet: `alias ll="ls -l"; whence -c ll; whence -c echo; whence -c if; whence -c ls; whence -c nosuchcmd431; echo "st=$?"`,
		Why:     "zsh's -c is a fourth shape, and it is not the bare one with words added: an alias is `ll: aliased to ls -l`, a builtin `echo: shell built-in command`, a reserved word `if: shell reserved word`, and a file its bare path. ksh93 has no -c at all, so the two builtins under one spelling diverge on the letter as well as on the wording",
	},
	{
		ID: "whence/a-lists-every-resolution", Category: "builtins",
		Snippet: `whence -a echo; echo "st=$?"; whence -a nosuchcmd431; echo "st=$?"`,
		Why:     "-a is every resolution rather than the first: the builtin and then the PATH hit, in that order. ksh93 has the letter and this build does not implement it, which the row records as the refusal it is",
	},
	{
		ID: "whence/w-is-the-bare-kind", Category: "builtins",
		Snippet: `alias ll="ls -l"; f() { :; }; whence -w ll; whence -w f; whence -w echo; whence -w if; whence -w ls; whence -w nosuchcmd431; echo "st=$?"`,
		Why:     "-w is zsh's own vocabulary for the kinds, and it is not `type -t`'s: a file is `command` here where the shell with -t says `file`, and a name that is nothing answers `none` rather than saying nothing. All six kinds in one row, because what is being pinned is the vocabulary rather than any one lookup",
	},
	{
		ID: "whence/where-is-whence-with-c-and-a", Category: "builtins",
		Snippet: `alias ll="ls -l"; where echo; where ll; where nosuchcmd431; echo "st=$?"`,
		Why:     "zsh's second name for the same question, and it is exactly `whence -ca` — the csh listing over every resolution. Nothing else in the panel has a `where` at all",
	},
	{
		ID: "whence/where-takes-no-options", Category: "builtins",
		Snippet: `where -v echo; echo "st=$?"`,
		Why:     "and it is a name rather than a synonym with flags: `where -v` is a bad option, not the sentence `whence -v` writes. A shell that implemented it by handing its arguments to whence would pass every other row here and fail this one",
	},
	{
		ID: "whence/nothing-to-ask-about", Category: "builtins",
		Snippet: `whence; echo "st=$?"`,
		Why:     "the two shells with the builtin part company over an empty operand list: zsh says nothing and reports 1, ksh93 prints its usage line and reports 2. The quiet one is the trap — a script testing the status sees a plain miss",
	},
	{
		ID: "print/joins-expands-and-ends-the-line", Category: "builtins",
		Snippet: `print hello world; print -n ab; print cd`,
		Why:     "ksh93's echo: operands joined with single spaces, a newline after, and -n withholding it. zsh has print too; bash and dash do not, which is the dialect boundary this case records",
	},
	{
		ID: "print/r-withholds-the-escapes", Category: "builtins",
		Snippet: `print 'a\tb'; print -r 'a\tb'`,
		Why:     "print expands echo's escapes by default and -r prints the operands raw — the pair a script chooses between where bash would choose echo -e against echo -E",
	},
	{
		ID: "print/backslash-c-stops-everything", Category: "builtins",
		Snippet: `print 'ab\c def'; print after`,
		Why:     "\\c ends the command's output where it stands: the rest of the text and the newline are both unwritten, so `after` lands directly against `ab`",
	},
	{
		ID: "print/u-aims-at-a-descriptor", Category: "builtins",
		Snippet: `print -u2 to-err; echo "st=$?"; print -u9 x; echo "st=$?"`,
		Why:     "-u writes to the named descriptor — 2 lands on the diagnostic stream — and a number nothing is open at is ksh93's `bad file unit number` with the errno in brackets, 1",
	},
	{
		ID: "print/a-lone-dash-ends-the-options", Category: "builtins",
		Snippet: `print -- -n; print - -n`,
		Why:     "both enders work in ksh93, so `-n` prints as a word; zsh's print reads the lone dash its own way",
	},
	{
		ID: "print/f-is-printf", Category: "builtins",
		Snippet: `print -f '%s|' a b; echo .`,
		Why:     "-f hands the whole command to printf: the format is reused over the operands and no newline is added, so the dot lands against the second bar",
	},
	{
		ID: "print/the-coprocess-letter-with-nothing-there", Category: "builtins",
		Snippet: `print -p x; echo "st=$?"`,
		Why:     "-p writes to the coprocess, and with no |& in this grammar there is never one: ksh93's `no query process` at 1, the same shape read -p measured",
	},
	{
		ID: "caller/from-a-function-under-dash-c", Category: "builtins",
		Snippet: `f() { caller; echo "st=$?"; caller 0; echo "st0=$?"; }; f`,
		Why:     "bash's question about who called: under -c there is no script frame, so bare caller prints the call line and NULL at 0, and caller 0 — which needs a frame above — is silence at 1. The other three have no caller at all",
	},
	{
		ID: "caller/walks-a-script-s-stack", Category: "builtins",
		Script: true, LayoutSensitive: true,
		Snippet: "f() { caller; caller 0; caller 1; caller 2; echo \"st=$?\"; }\ng() { f; }\ng",
		Why:     "from a file the frames are real: bare caller is the line and the file, a depth adds the function — the script's own frame answering as main — and past the stack is silence at 1. Line numbers are the output, which is why this runs as a script",
	},
	{
		ID: "caller/refuses-what-is-not-a-depth", Category: "builtins",
		Snippet: `f() { caller x; echo "st=$?"; }; f`,
		Why:     "the expression is a plain number despite the manual's word for it — 1+1 is refused the same way — and bash answers `invalid number` with its caller usage at 2",
	},

	// --- invocation: what the argv itself decides -----------------------
	//
	// These use Case.Args, so the shell is started the way a script or a
	// harness starts it rather than the way the corpus starts everything
	// else. Without them the invocation surface is graded only by the
	// driver's own unit tests, which is how the drivers once scored 178/198
	// against a core scoring 198/198.
	{
		ID: "invoke/errexit-with-a-script", Category: "invocation",
		Args:    []string{"-e", ArgScript},
		Snippet: "echo one\nfalse\necho two",
		Why:     "the first line of most scripts, spelled on the command line instead: a set option given at invocation has to reach the runner, and abandon the script at the failure rather than run to the end",
	},
	{
		ID: "invoke/a-bundle-of-set-letters", Category: "invocation",
		Args:            []string{"-eu", ArgScript},
		Snippet:         "echo \"[${nope}]\"\necho two",
		LayoutSensitive: true,
		Why:             "letters bundle into one word, and both have to arrive: -u makes the unset expansion an error and -e stops there. A bundle that kept only the last letter would still print the diagnostic and then run on",
	},
	{
		ID: "invoke/the-long-option-name", Category: "invocation",
		Args:    []string{"-o", "errexit", "-c", ArgSnippet},
		Snippet: `echo one; false; echo two`,
		Why:     "-o names the option instead of lettering it, and the name is a separate word read at the end of the bundle — the `set -euo pipefail` shape, minus the parts the panel disagrees about",
	},
	{
		ID: "invoke/errexit-reaches-the-option-letters", Category: "invocation",
		Args:    []string{"-e", ArgScript},
		Snippet: `case $- in *e*) echo has-e ;; *) echo no-e ;; esac`,
		Why:     "$- answers for how the shell was *started*, not only for what `set` did later. It asks whether the letter is there rather than printing $-, because the spelling is a live disagreement: the panel differs over whether c and s appear at all, and over the order",
	},
	{
		ID: "invoke/end-of-options-before-a-script", Category: "invocation",
		Args:    []string{"--", ArgScript, "a", "b"},
		Snippet: `echo "$0|$#|$1"`,
		Why:     "-- ends the options, so the next word is the script rather than a flag — and the script's own path becomes $0 while the words after it become the parameters",
	},
	{
		ID: "invoke/the-command-string-names-zero-itself", Category: "invocation",
		Args:    []string{"-c", ArgSnippet, "name", "a", "b"},
		Snippet: `echo "$0|$#|$*"`,
		Why:     "-c names its operands differently from every other route: the first is $0 and only the rest are parameters, so a shell that handed all three to $1 onward would report n=3 and keep its own name",
	},
	{
		ID: "invoke/c-in-the-middle-of-a-bundle", Category: "invocation",
		Args:    []string{"-ce", ArgSnippet, "name", "a"},
		Snippet: `echo "$0|$#|$*"; false; echo two`,
		Why:     "a bundle does not end at c: the letters after it are options and the command string is still the first operand. Reading the rest of the word as the string ran a command called `e` with the string as $0",
	},
	{
		ID: "invoke/options-between-c-and-its-string", Category: "invocation",
		Args:    []string{"-c", "-e", ArgSnippet, "name", "a"},
		Snippet: `echo "$0|$#|$*"; false; echo two`,
		Why:     "the option loop stops at the first operand rather than at c, so a whole option word may sit between -c and the command string — which is how an agent harness writes it. Taking the next word regardless would run `-e` as the program",
	},
	{
		ID: "invoke/end-of-options-between-c-and-its-string", Category: "invocation",
		Args:    []string{"-c", "--", ArgSnippet, "name", "a"},
		Snippet: `echo "$0|$#|$*"`,
		Why:     "the same rule from the other side: -- ends the options and the first operand after it is the command string, not a path",
	},
	{
		ID: "invoke/plus-c-still-runs-the-command", Category: "invocation",
		Args:    []string{"+c", ArgSnippet},
		Snippet: `echo hi`,
		Why:     "both signs select the command string — unanimous. Gating c on the minus sign made +c a set letter to turn off and then opened the command string as a script file",
	},
	{
		ID: "invoke/c-outranks-standard-input", Category: "invocation",
		Args:    []string{"-sc", ArgSnippet},
		Snippet: `echo "hi|$#"`,
		Why:     "-s and -c in one bundle: all four run the command rather than reading standard input, so a shell that let -s win would print nothing and still exit 0",
	},
	{
		ID: "invoke/plus-c-names-itself", Category: "invocation",
		Args:    []string{"+c", ArgSnippet, "name", "a"},
		Snippet: `echo "$0|$#|$*"; true`,
		Why:     "the sign of the `c` is not decoration in one shell: ksh93 leaves the command string in $0 and makes both operands parameters, where bash, dash and zsh read `+c` as `-c` and name $0 from the first operand. The trailing `true` is a shield rather than part of the question — the same shell also hands the operands to the program as literal words appended to its last command, which is a second divergence recorded in docs/spec/invocation.md and not modeled, and a command that ignores its arguments keeps it out of this case's output",
	},
	{
		ID: "invoke/plus-c-with-no-operands-names-itself", Category: "invocation",
		Args:    []string{"+c", ArgSnippet},
		Snippet: `echo "$0|$#|$*"`,
		Why:     "the same question with nothing for the operand rules to disagree about, which is where the naming still splits: three of the four keep the shell's own name in $0 and ksh93 puts the command string there. No shield is needed, because there is no operand to be appended",
	},
	{
		ID: "invoke/plus-c-in-a-bundle-names-itself", Category: "invocation",
		Args:    []string{"+ce", ArgSnippet, "name", "a"},
		Snippet: `echo "$0|$#|$*"; true`,
		Why:     "the sign belongs to the word rather than to the letter, so a bundle answers the same way — and the `e` riding with it is errexit being turned *off*, which is the plus sign doing its ordinary job to the letter beside the one that is not ordinary",
	},
	{
		ID: "invoke/c-with-s-names-the-operands", Category: "invocation",
		Args:    []string{"-sc", ArgSnippet, "name", "a"},
		Snippet: `echo "$0|$#|$*"`,
		Why:     "the same bundle with operands after it, which is where the panel splits 2-2: bash and dash apply the command string's rule and make `name` $0, ksh93 and zsh apply the standard-input rule and let no operand be $0, so the shell keeps its own name and both operands are parameters. Only the naming splits — all four run the command string, which the case above pins",
	},
	{
		ID: "invoke/c-with-s-unbundled", Category: "invocation",
		Args:    []string{"-s", "-c", ArgSnippet, "name", "a"},
		Snippet: `echo "$0|$#|$*"`,
		Why:     "the same question spelled as two words, which changes nothing anywhere: whichever rule a shell applies, it applies it to the bundle and to the pair alike",
	},
	{
		ID: "invoke/c-before-s-names-the-operands", Category: "invocation",
		Args:    []string{"-c", "-s", ArgSnippet, "name", "a"},
		Snippet: `echo "$0|$#|$*"`,
		Why:     "and in the other order, where the letter that comes first might have been expected to win and does not — the answer is the shell's rather than the invocation's",
	},
	{
		ID: "invoke/c-with-s-and-no-operands", Category: "invocation",
		Args:    []string{"-sc", ArgSnippet},
		Snippet: `echo "$0|$#|$*"`,
		Why:     "the same invocation with nothing for the two rules to disagree about: with no operand past the command string all four keep the shell's own name and no parameters, which is why the question is only asked where an operand follows",
	},
	{
		ID: "invoke/standard-input-that-is-not-a-terminal", Category: "invocation",
		Args:    []string{"-s"},
		Snippet: `echo unreachable`,
		Why:     "the harness gives every child the null device for standard input, and the null device is a character device — which is exactly what made the prompt decision say terminal, ask it for raw mode, and exit 2 with `operation not supported by device` (#509). No shell in the panel prompts here: -s says read standard input, standard input ends at once, and the shell exits 0 having said nothing. Deliberately no placeholder — what is pinned is what a shell does before it reads anything, and the snippet is written down as the thing that would have run",
	},
	{
		ID: "invoke/the-program-arrives-on-standard-input", Category: "invocation",
		Args:    []string{"--"},
		Stdin:   ArgSnippet + "\n",
		Snippet: `echo "0=[$0]|n=$#"`,
		Why:     "the third invocation route, and the only one with nothing on the command line to name: $0 stays the shell rather than becoming a path, and there are no operands to become parameters. `--` is there because Args with no placeholder is what says the program is not on the argv, and an empty Args would mean the harness's own -c",
	},
	{
		ID: "invoke/dash-s-makes-every-operand-a-parameter", Category: "invocation",
		Args:    []string{"-s", "a", "b"},
		Stdin:   ArgSnippet + "\n",
		Snippet: `echo "0=[$0]|n=$#|[$*]"`,
		Why:     "-s says the program is on standard input, so the words after it are parameters rather than a script path — the one route where $1 is set and $0 is still the shell. Without it the same two words would make `a` the script and `b` its first parameter",
	},
	{
		ID: "invoke/a-read-takes-the-next-line-of-a-program-on-standard-input", Category: "invocation",
		// The line a diagnostic names is the whole point: the shells that
		// hand the line to `read` never parse it, so the line after it is
		// numbered as though it were not there.
		LayoutSensitive: true,
		Args:            []string{"--"},
		Stdin:           ArgSnippet + "\nDATA-LINE\necho end\n",
		Snippet:         "read x\necho \"[$x]\"",
		Why:             "the split this route turns on, and the reason it matters: bash, ksh93 and zsh read the program a line at a time off the descriptor, so `read x` takes the *next line of the program*, `echo` is never parsed, and DATA-LINE is the first thing run — at line 2, because a line handed to `read` is never counted. dash takes the program in blocks and keeps what it took, so `read` finds end of input, prints [] and DATA-LINE runs at line 3. Taking the whole input for everyone was the bug (#470): a payload piped after a `curl | sh` script was executed as commands",
	},
	{
		ID: "invoke/a-read-at-the-end-of-a-program-on-standard-input", Category: "invocation",
		Args:    []string{"--"},
		Stdin:   ArgSnippet + "\n",
		Snippet: `read x; echo "$? [$x]"`,
		Why:     "the half of the split that is unanimous: with nothing after it there is nothing to take, so `read` reports end of input at 1 and leaves the variable empty in all four. It is the control for the case above — the difference there is what the *rest of the program* is, not how `read` behaves",
	},
	{
		ID: "invoke/a-while-read-loop-eats-the-rest-of-a-program-on-standard-input", Category: "invocation",
		LayoutSensitive: true,
		Args:            []string{"--"},
		Stdin:           ArgSnippet + "\nA\nB\nC\n",
		Snippet:         `while read -r l; do echo "got:$l"; done`,
		Why:             "the shape a script piped to a shell is actually written in, and the sharpest reading of the same split: three shells feed the loop the three lines that follow it and dash runs all three as commands. One loop, two entirely different programs, decided by nothing in the text",
	},
	{
		ID: "invoke/a-command-reads-the-rest-of-a-program-on-standard-input", Category: "invocation",
		LayoutSensitive: true,
		Args:            []string{"--"},
		Stdin:           ArgSnippet + "\nNOT-A-COMMAND\necho end\n",
		Snippet:         "echo one\ncat",
		Why:             "not a builtin's doing: an external command inherits the descriptor the program is on, so `cat` prints the rest of the program instead of running it — in bash, ksh93 and zsh alike. dash's block has already taken those bytes, so cat sees end of input and the lines run. The descriptor is shared with everything the script starts, which is why this is a property of the route and not of `read`",
	},
	{
		ID: "invoke/exec-repoints-the-rest-of-a-program-on-standard-input", Category: "invocation",
		LayoutSensitive: true,
		Args:            []string{"--"},
		Stdin:           ArgSnippet + "\necho never reached\n",
		Snippet:         "printf 'FIRST\\nSECOND\\n' > d.txt\nexec 0< d.txt",
		Why:             "what the sharing really is: the program *is* the descriptor, so pointing descriptor 0 at a file half way through replaces the rest of the program with that file's contents — bash, ksh93 and zsh all run FIRST and SECOND as commands and never see the line after the `exec`. dash runs its remaining block first and only then reads the file, which is the same rule with a bigger unit rather than a different rule",
	},
	{
		ID: "invoke/a-heredoc-in-a-program-on-standard-input", Category: "invocation",
		Args:    []string{"--"},
		Stdin:   ArgSnippet + "\n",
		Snippet: "cat <<EOF\nbody\nEOF\necho after",
		Why:     "unanimous, and the case that stops a line-at-a-time reader from being wrong: a here-document's body arrives on the same descriptor as the program and belongs to the command that opened it, so a reader that treated every line as a command would run `body` and `EOF`. All four print the body once and carry on",
	},
	{
		ID: "invoke/a-continued-line-in-a-program-on-standard-input", Category: "invocation",
		Args:    []string{"--"},
		Stdin:   ArgSnippet + "\n",
		Snippet: "echo one \\\ntwo\necho three",
		Why:     "unanimous, and the other way a line-at-a-time reader goes wrong: a trailing backslash joins the line to the one after it, and a reader that ran each line as it arrived would print `one` and then fail on `two`. A parser cannot answer this — the same text at the end of the input is a finished command — so only the reader can",
	},
	{
		ID: "invoke/a-construct-over-several-lines-in-a-program-on-standard-input", Category: "invocation",
		Args:    []string{"--"},
		Stdin:   ArgSnippet + "\n",
		Snippet: "if true\nthen\n\techo yes\nfi\necho done",
		Why:     "unanimous: a construct is read until it finishes however many lines that takes, on this route as on any other. The unit a shell runs is the whole construct and not the line, which a reader that takes one line at a time has to be told rather than discover",
	},
	{
		ID: "invoke/a-script-that-is-not-there", Category: "invocation",
		Args:    []string{"nosuch.sh"},
		Snippet: `echo this file was never written`,
		Why:     "an operand naming nothing, so the snippet never reaches the shell — the shape Case.Args exists to allow. The status is the split: bash, ksh93 and zsh answer with a missing command's 127 and dash with its usage 2, where a shell that folded this into the generic input error would answer 2 for all four",
	},
	{
		ID: "invoke/a-script-under-a-directory-that-is-not-there", Category: "invocation",
		Args:    []string{"nodir/nosuch.sh"},
		Snippet: `echo this file was never written`,
		Why:     "the same diagnosis reached a different way: it is the *parent* that is missing, and every shell in the panel words and numbers it exactly as it does a missing leaf rather than complaining about the directory",
	},
	{
		ID: "invoke/a-script-that-is-there-runs", Category: "invocation",
		Args:    []string{ArgScript},
		Snippet: `echo ran; echo "st=$?"`,
		Why:     "the control the two above need: the same route, with a path that opens, runs the file and reports 0. Without it a front end that called every script path unreadable would pass both failure cases. The truly empty script the pair also wants is not expressible here — the corpus requires a snippet that parses to at least one statement — so it is measured in docs/spec/semantics.md instead",
	},
	{
		ID: "invoke/a-script-is-not-interactive", Category: "invocation",
		Args:    []string{ArgScript},
		Snippet: "case $- in *i*) echo interactive ;; *) echo not ;; esac",
		Why:     "the negative half of #472: `i` belongs in $- only where the shell is interactive, and a script operand is not — unanimous. Membership rather than the spelling, for the same reason invoke/errexit-reaches-the-option-letters uses it. The positive half cannot live here: bash and dash announce that job control is off when -i has no terminal, and bash's line carries a pid, so `-i` under the harness is not a recordable fact",
	},

	// --- invocations a shell refuses ------------------------------------
	//
	// Graded on the refusal rather than on its wording. Every shell here
	// declines, and every one of them declines in words of its own, so an
	// exact comparison would score four shells that agree about the behavior
	// as four disagreements — and these shapes would be unwritable, which is
	// exactly what kept the sharpest of them out of the corpus (#534).
	{
		ID: "invoke/a-command-string-attached-to-the-letter", Category: "invocation",
		Args:            []string{"-c" + ArgSnippet},
		Snippet:         `echo hi`,
		GradedOnRefusal: true,
		Why: "`sh -c'echo hi'` — the command string written against the letter rather than as its own word, which is how a hand and a generated command line both get it wrong. All six refuse it: `-c` takes its operand as a separate word, so the rest of this one is read as more option letters and `echo hi` is not a run of them. We ran it. That is the bug this case exists for, and it is caught here against every reference, because a shell that ran the string writes `hi` to standard output and standard output is still compared exactly. " +
			"bash is the reason the row is marked: it answers by writing its entire `set -o` table to standard *output* as part of the usage, so against a bash reference this reports a real gap until we write the table too — the relaxation forgives a diagnostic's wording and declines to forgive a stream of output nobody produced",
	},
	{
		ID: "invoke/c-with-nothing-after-it", Category: "invocation",
		Args:            []string{"-c"},
		Snippet:         `echo this never arrives`,
		GradedOnRefusal: true,
		Why:             "the option that requires an argument, given none — deliberately no placeholder, because what is pinned is the shell refusing before it has a program at all. Unanimous in behavior and unanimous in nothing else: five of the six exit 2 and zsh exits 1, and all six word it differently, which is the pair of facts that makes this gradable only on the refusal. A front end that treated a missing operand as an empty command string would exit 0 having done nothing",
	},
	{
		ID: "invoke/an-option-letter-that-is-not-one", Category: "invocation",
		Args:            []string{"-Z", ArgSnippet},
		Snippet:         `echo hi`,
		GradedOnRefusal: true,
		Why:             "a letter no shell has. Four of them say so and stop; zsh is the divergence worth recording — it accepts `-Z` as one of its own and then fails on the operand as a script it cannot open, at 127 rather than 2. So the panel is unanimous that this fails and split on why, which is a fact the row keeps in full even though the grading only asks whether the shell declined",
	},
	{
		ID: "invoke/a-long-option-name-that-is-not-one", Category: "invocation",
		Args:            []string{"-o", "nosuchoption", "-c", ArgSnippet},
		Snippet:         `echo hi`,
		GradedOnRefusal: true,
		Why:             "the same question one level in: `-o` is a valid letter and its operand is not a valid name, so the refusal comes from the option table rather than from the letter table. All six decline before running the command string, which is the part that matters — a shell that warned and carried on would print `hi` and standard output would catch it",
	},
	{
		ID: "invoke/a-script-has-neither-route-letter", Category: "invocation",
		Args:    []string{ArgScript},
		Snippet: `case $- in *c*) echo has-c ;; *) echo no-c ;; esac; case $- in *s*) echo has-s ;; *) echo no-s ;; esac`,
		Why:     "the baseline the other rows are read against: a script named on the command line is neither a command string nor standard input, and no shell in the panel puts either letter there. Membership rather than the spelling, because the spelling is what splits — no two shells order $- alike",
	},
	{
		ID: "invoke/dollar-dash-shows-c-for-a-command-string", Category: "invocation",
		Args:    []string{"-c", ArgSnippet},
		Snippet: `case $- in *c*) echo has-c ;; *) echo no-c ;; esac`,
		Why:     "two against two, which is what makes it an axis rather than a rule: bash and ksh93 put `c` in $- for a program given as a command string and dash and zsh do not. There is no majority to follow, so the preset takes the POSIX text — $- is the option flags specified on invocation, and -c is one",
	},
	{
		ID: "invoke/dollar-dash-and-the-s-letter-for-a-command-string", Category: "invocation",
		Args:    []string{"-c", ArgSnippet},
		Snippet: `case $- in *s*) echo has-s ;; *) echo no-s ;; esac`,
		Why:     "the corner that keeps `s` from being modeled as `the program came from standard input` outright: ksh93 alone also shows it under -c. Read down its rows and ksh93's rule is `no script file was named` where the other three's is the standard-input route, and this is the one invocation where the two rules differ",
	},
	{
		ID: "invoke/dollar-dash-shows-s-for-a-program-on-standard-input", Category: "invocation",
		Args:    []string{"--"},
		Stdin:   ArgSnippet + "\n",
		Snippet: `case $- in *s*) echo has-s ;; *) echo no-s ;; esac`,
		Why:     "the unanimous half, and the reason it could be implemented without waiting for the -c corner to be settled: a program arriving on standard input puts `s` in $- whether -s was written or not. bash 3.2 is the one dissent and it is a shell disagreeing with its own later build rather than a panel split — it shows the letter only where -s was written",
	},
	{
		ID: "invoke/dollar-dash-shows-s-for-the-s-option", Category: "invocation",
		Args:    []string{"-s"},
		Stdin:   ArgSnippet + "\n",
		Snippet: `case $- in *s*) echo has-s ;; *) echo no-s ;; esac`,
		Why:     "the same letter with the option written out, which is where all six agree including bash 3.2 — the pair with the row above is what tells the route from the spelling",
	},
	{
		ID: "invoke/dollar-dash-keeps-s-when-a-command-string-overrides-it", Category: "invocation",
		Args:    []string{"-s", "-c", ArgSnippet},
		Snippet: `case $- in *s*) echo has-s ;; *) echo no-s ;; esac`,
		Why:     "-c wins about where the program comes from and does not take the letter away: all six run the command string and all six still show `s`. So the letter follows either the route or the spelling, and a shell that read only the route would lose it here",
	},

	// --- the name a shell was called by (#733). Every one of these sets
	// Argv0, so the whole panel is measured under one name rather than each
	// under its own — which is the only way to ask the question at all, since
	// Args is what comes after argv[0] and never argv[0] itself.
	{
		ID: "invoke/called-sh-starts-in-posix-mode", Category: "invocation",
		Argv0:   "sh",
		Snippet: `exec 3>/nope/x; echo after`,
		Why:     "the name is a startup input: the same bash prints `after` at 0 called `bash` and stops at 1 called `sh`, which is POSIX's rule that a failed redirection on a special builtin is fatal. zsh moves with it under the same name; dash and ksh93 keep that rule under every name and so answer alike in both rows",
	},
	{
		ID: "invoke/called-sh-and-a-script-operand-starts-in-posix-mode", Category: "invocation",
		Argv0:   "sh",
		Args:    []string{ArgScript},
		Snippet: `exec 3>/nope/x; echo after`,
		Why:     "the name and not the route: a script named on the command line answers exactly as the command string above does, so the front end reads argv[0] once rather than per route",
	},
	{
		ID: "invoke/called-sh-by-a-path-starts-in-posix-mode", Category: "invocation",
		Argv0:   "./sh",
		Snippet: `exec 3>/nope/x; echo after`,
		Why:     "the last element of the path is the name — `/bin/sh` is how anything really reaches it — so a shell reading argv[0] whole would miss every real invocation of this",
	},
	{
		ID: "invoke/the-login-spelling-of-the-name-starts-in-posix-mode", Category: "invocation",
		Argv0:   "-sh",
		Snippet: `exec 3>/nope/x; echo after`,
		Why:     "one leading dash is the login convention and not part of the name, so `-sh` is both a login shell and the standard's name at once; bash answers `--sh` the other way, which is what says one dash and not any number",
	},
	{
		ID: "invoke/a-name-that-is-not-sh-does-not-start-in-posix-mode", Category: "invocation",
		Argv0:   "myshell",
		Snippet: `exec 3>/nope/x; echo after`,
		Why:     "the control, and it has to be a name no shell in the panel reads as its own: zsh takes the *first letter* of the name, so `bash`, `shx` and even `s` all put it in sh emulation, while `m` leaves it alone. bash and zsh both print `after` here and both stop in the row above",
	},
	{
		ID: "invoke/called-sh-and-then-leaving-posix-mode", Category: "invocation",
		Argv0:   "sh",
		Snippet: `set +o posix; exec 3>/nope/x; echo after`,
		Why:     "leaving the mode reaches the shell's *own* answer rather than the standard's opposite, which is what makes the startup override a mode and not a written-down axis: bash-as-`sh` prints `after` at 0 again. The other three have no such name and refuse the `set` instead, each in its own words",
	},
	{
		ID: "invoke/called-sh-outranks-the-invocations-own-posix-option", Category: "invocation",
		Argv0:   "sh",
		Args:    []string{"+o", "posix", "-c", ArgSnippet},
		Snippet: `exec 3>/nope/x; echo after`,
		Why:     "the name is read after the invocation's options and wins over the one they can name: `sh +o posix -c` still stops where `bash +o posix -c` carries on. Not the loop being ignored — `+o errexit` on the same invocation is honored — so this pins the order rather than the reading. The three shells without the name refuse the invocation instead",
	},
}
