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

	// Env are environment entries added to the fixed four every case gets,
	// in `NAME=value` form, and handed to both sides of a comparison.
	//
	// The environment is a **startup input** and not only a place variables
	// live: a shell reads names out of it before the first line runs, and
	// what it finds there changes what every command afterwards does. That is
	// the class of behavior this exists for — an inherited list of options, a
	// file to source before the script — and until now the corpus could not
	// ask about it at all, because the harness hands every case the same four
	// entries and a snippet cannot put anything in the environment of the
	// shell that is already running it.
	//
	// The base four stay: a case adds to them rather than replacing them, so
	// no case can quietly drop the PATH and HOME that keep the record from
	// depending on whose machine produced it. An entry naming one of the four
	// overrides it, which is what execve does with a duplicate and is the
	// only reading that lets a case ask about HOME.
	//
	// ArgScript is honored in a value, wherever it appears, exactly as it is
	// in Args and Stdin — which is what lets a case name a *file* to be read
	// at startup without spelling its path, since the path is a scratch
	// directory this run invented. The file holds the Snippet, and the case's
	// Args then say what the shell is to run instead.
	Env []string

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
		ID: "ifs/nonws-trailing-split-on", Category: "IFS",
		Snippet: `setopt shwordsplit; IFS=:; x="a:"; set -- $x; printf "%d" "$#"; printf "[%s]" "$@"`,
		Why:     "the row that turns the asymmetry above from a fact into an axis: with the splitting turned on in the shell that has it off, a trailing separator *delimits* there and is absorbed in the other five — two fields against one. The `setopt` is not found in the other five and costs them nothing, which is what lets one snippet ask the same question of all six. The count is printed with the fields because `[a]` and `[a][]` are the same characters once the boundaries are gone, and a wrong answer here is a plausible count at status 0",
	},
	{
		ID: "ifs/nonws-trailing-doubled-split-on", Category: "IFS",
		Snippet: `setopt shwordsplit; IFS=:; x="a::"; set -- $x; printf "%d" "$#"; printf "[%s]" "$@"`,
		Why:     "one separator at the tail against two: the fifth shell answers two fields here and the sixth three, so the extra field is the tail's and not a miscount of the pair. An implementation that special-cased a value *ending* in a separator by keeping the whole run would answer four",
	},
	{
		ID: "ifs/nonws-trailing-pair-split-on", Category: "IFS",
		Snippet: `setopt shwordsplit; IFS=:; x="a:b:"; set -- $x; printf "%d" "$#"; printf "[%s]" "$@"`,
		Why:     "the shape a real script writes — a list padded with a trailing separator on purpose, which is the ordinary way to write one. Three fields in the shell that delimits and two in the rest, so a `for` loop over it runs one iteration fewer with nothing to say so",
	},
	{
		ID: "ifs/nonws-both-ends-split-on", Category: "IFS",
		Snippet: `setopt shwordsplit; IFS=:; x=":a:"; set -- $x; printf "%d" "$#"; printf "[%s]" "$@"`,
		Why:     "the leading separator delimits in all six and the trailing one in one, so this row holds both halves at once. It is the row a fix that made the rule symmetric would pass while breaking the five shells it was not about",
	},
	{
		ID: "ifs/ws-trailing-absorbed-split-on", Category: "IFS",
		Snippet: `setopt shwordsplit; x=" a "; set -- $x; printf "%d" "$#"; printf "[%s]" "$@"`,
		Why:     "the guard, and the reason the axis is the non-whitespace half alone: under the default IFS whitespace is absorbed at both ends in all six shells, the one that delimits on a trailing separator included. A fix that stopped absorbing anything would break the shell it was written to match",
	},
	{
		ID: "ifs/mixed-trailing-run-split-on", Category: "IFS",
		Snippet: `setopt shwordsplit; IFS=" :"; x="a: "; set -- $x; printf "%d" "$#"; printf "[%s]" "$@"; y="a  "; set -- $y; printf " %d" "$#"; printf "[%s]" "$@"`,
		Why:     "it is the closing *run* of separators that decides and not the last byte: the trailing space does not hide the colon in front of it, so this is two fields where `'a  '` is one. A reading that looked at the last character alone would answer one here",
	},
	{
		ID: "ifs/nonws-trailing-cmdsub", Category: "IFS",
		Snippet: `IFS=:; set -- $(printf "a:"); printf "%d" "$#"; printf "[%s]" "$@"`,
		Why:     "the same divergence with no option set and no flag written, which is what says it is reachable by ordinary means in every shell: an unquoted command substitution is split in all six, including the one that leaves parameter expansions alone, so the tail question is asked here whatever the splitting option says",
	},
	{
		ID: "ifs/nonws-trailing-read-remainder", Category: "IFS",
		Snippet: `IFS=:; printf 'a:b:\n' | { read -r x y; printf "[%s][%s]" "$x" "$y"; }`,
		Why:     "`read` feeds the same splitter, and the last name takes the remainder of the line — which is text rather than a field, so it keeps the separators the input had. Five shells give `b` and the sixth `b:`, which is the trailing separator surviving in the one shell where it delimits. Two facts in one row: an implementation that rejoins the remainder from its fields answers `b c` for `a:b:c` in every shell, which no shell in the panel does",
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
		ID: "echo/the-two-spellings-of-the-escape-character", Category: "builtins",
		Snippet: `echo -e 'a\eZ:a\EZ' | od -An -tx1 | tr -s " "`,
		Why:     "read as bytes, because an ESC is invisible in a rendered table and this row is entirely about which of the two letters produces one. The two shells that split `\\e` from `\\E` split them in *opposite* directions — ksh93 has `\\E` and not `\\e`, zsh has `\\e` and not `\\E` — so one answer for both letters is wrong for half the panel. bash 5.3 has both and bash 3.2 neither, which is also the version line here; dash has no -e and prints it. The same asymmetry the `%b` site has, and the row that says `echo` cannot borrow one answer for the pair",
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
		ID: "axis/dollar-zero-in-a-sourced-file", Category: "semantics axes",
		// A script rather than -c, so the outer name is a file that could
		// plausibly still be the answer: under -c there is only one name in
		// play and the question cannot be asked. The sourced file is written
		// by the snippet, so the name it might report is the same on every
		// machine.
		Script:  true,
		Snippet: "printf 'echo \"in=[$0]\"\\n' > inc.sh\necho \"before=[$0]\"\n. ./inc.sh\necho \"after=[$0]\"",
		Why:     "`$0` inside a sourced file is the *sourced file* in zsh and the outer script in the other five, and it goes back afterwards in all six — which is what makes it a property of where the shell is rather than of what it has read. zsh names the operand as written, `./inc.sh` and not the resolved path. This is the gate a plugin manager on this machine computes its own install directory from (#978)",
	},
	{
		ID: "axis/dollar-zero-in-a-file-read-by-source", Category: "semantics axes",
		Script:  true,
		Snippet: "printf 'echo \"in=[$0]\"\\n' > inc.sh\nsource ./inc.sh\necho st=$?",
		Why:     "the other spelling of the same route, and the same answer wherever the name exists: `source` moves `$0` in zsh exactly as `.` does. dash is the column that answers something else entirely, because it has no `source` at all",
	},
	{
		ID: "axis/dollar-zero-in-a-function-a-sourced-file-defined", Category: "semantics axes",
		// Called after the sourcing has finished, so nothing about the `.` is
		// still on the stack: what is reported is what the innermost call is
		// now, not what the function was read from.
		Script:  true,
		Snippet: "printf 'f() { echo \"in=[$0]\"; }\\n' > inc.sh\n. ./inc.sh\nf",
		Why:     "the function wins over the file it came from: zsh reports `f` and not `./inc.sh`, which is the evidence that `$0` follows the innermost call rather than remembering where anything was defined. The other five report the script, as they do everywhere",
	},
	{
		ID: "axis/dollar-zero-in-a-nested-sourced-file", Category: "semantics axes",
		Script:  true,
		Snippet: "printf 'echo \"deep=[$0]\"\\n' > deep.sh\nprintf 'echo \"mid=[$0]\"\\n. ./deep.sh\\necho \"mid-again=[$0]\"\\n' > mid.sh\n. ./mid.sh",
		Why:     "a file sourced by a sourced file: zsh reports the innermost of the three and restores the middle one when it returns, so the answer is a stack and not a flag. The other five report the outermost script for all three lines",
	},
	{
		ID: "axis/dollar-zero-in-a-file-sourced-from-a-function", Category: "semantics axes",
		Script:  true,
		Snippet: "printf 'echo \"in=[$0]\"\\n' > inc.sh\nf() { echo \"fn=[$0]\"; . ./inc.sh; echo \"fn-again=[$0]\"; }\nf",
		Why:     "the case that settles which of the two halves is really being asked. A function that sources a file reports the *file* while it runs and its own name again afterwards, so it is the innermost call and not the innermost function — a rule written as `is a function on the stack` gets this one wrong",
	},
	{
		ID: "axis/dollar-zero-in-a-file-found-on-path", Category: "semantics axes",
		// The file goes in a *subdirectory* named on PATH, so the path the
		// search joins is a different string from the operand. A `.` on PATH
		// would not show it: the joined name is then the bare name again.
		// The entry is relative rather than absolute on purpose, so the row
		// carries no machine's temporary directory in it.
		Script:  true,
		Snippet: "mkdir sub\nprintf 'echo \"in=[$0]\"\\n' > sub/inc.sh\nPATH=sub:$PATH\n. inc.sh",
		Why:     "the one route where a sourced file's `$0` is not the name a diagnostic would use: zsh answers with the bare operand `inc.sh` where a failure inside the same file is located at `sub/inc.sh` — recorded by `location/a-message-from-inside-a-sourced-file`. So `$0` is the word the script wrote rather than the file that was opened, which is what a shell computing its own install directory from `${0:h}` depends on",
	},
	{
		ID: "name/zsh-argzero-under-a-moved-dollar-zero", Category: "parameters",
		Script:  true,
		Snippet: "printf 'echo \"in=[$0] az=[${ZSH_ARGZERO-unset}]\"\\n' > inc.sh\n. ./inc.sh",
		Why:     "zsh alone carries the value `$0` had before anything moved it, which is the only way a sourced file can tell what the shell itself was called. It is what the `${${0:#$ZSH_ARGZERO}:-…}` idiom in a plugin manager on this machine tests against; the other five have no such name and no moved `$0` for it to record",
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
		ID: "axis/readonly-reassign-from-c", Category: "semantics axes",
		Snippet: "readonly r=1\nr=2\necho survived\n",
		Why:     "the same three lines through `-c`, and the row whose absence cost a wrong axis. `Semantics.ReadonlyReassignmentFatalFromCommandString` existed because `-c 'readonly r=1; r=2; echo survived'` stops and the three lines in a *file* print `survived` — two cells that differ in both the route and the separator, which is the diagonal of the square and confirmatory for either reading. The full square says the separator decides and the route decides nothing: bash is 1 for one line and 0 for three by **both** routes, and dash, ksh93 and zsh are fatal in all four cells. This row is the other end of the diagonal, so the pair can only be read one way (#1182)",
	},
	{
		ID: "axis/readonly-reassign-in-one-list", Category: "semantics axes",
		Script:  true,
		Snippet: "readonly r=1; r=2; echo survived\n",
		Why:     "the third corner: the same program as `axis/readonly-reassign` with `;` where it has newlines, over the same route. What ends is the command *list*, so the `echo` goes with the refusal here and runs there — which is the difference the removed field was reading as a route. Cheap to record and the only row that makes the separator visible on a fixed route",
	},
	{
		ID: "axis/arith-error-status", Category: "semantics axes",
		Snippet: `echo $((1/0)); echo "st=$?"`,
		Why:     "dash exits 2 where bash, ksh93 and zsh exit 1; found by a test disagreeing with the conformance run, not by the sweep",
	},
	{
		ID: "axis/failed-expansion-abandons-the-line", Category: "semantics axes",
		Script:  true,
		Snippet: "echo pre\necho $((1/0))\necho after\n",
		Why:     "how far a failed expansion reaches, which every earlier row was the wrong shape to ask: `echo $((1/0)); echo \"st=$?\"` above puts both commands in one list, and giving up the list and giving up the shell print exactly the same thing there. On three lines they part — bash reports the failure and runs `after`, and dash, ksh93 and zsh stop — so this is the row that separates `Semantics.FailedExpansionAbandonsTheLine` from a fatal error, and the one whose absence let a single unreadable expansion end a whole file in the dialect that survives one (#1171)",
	},
	{
		ID: "axis/a-failed-for-word-list", Category: "semantics axes",
		Script:  true,
		Snippet: "echo pre\nfor i in a $((1/0)) b; do echo \"A$i\"; done\necho after\n",
		Why:     "a failed expansion in a compound command's *heading* rather than in one of its commands, which is the same axis reaching a place that never asked it: the word list is what the loop iterates, so a failure in it costs the loop and not the one pass it would have been. Every column stops before the body — bash then runs `after` and the other three do not — and ours ran the body three times and exited 0. Good words on either side of the bad one, because a rule that abandoned only when the whole list failed would pass a single-word row (#1215)",
	},
	{
		ID: "axis/a-failed-case-subject", Category: "semantics axes",
		Script:  true,
		Snippet: "echo pre\ncase $((1/0)) in \"\") echo E;; *) echo A;; esac\necho after\n",
		Why:     "the same question on the subject, and the empty arm is what makes this the worst face of it rather than another row: a subject whose expansion failed is empty, and empty *matches*, so the shell chose a branch from a value it had just reported it could not compute — printing `E` at status 0, which is the one outcome nothing downstream can detect. The arm is written `\"\"` deliberately; a row with only `*)` cannot tell \"did not choose\" from \"chose the catch-all\" (#1215)",
	},
	{
		ID: "axis/failed-expansion-abandons-the-line-from-c", Category: "semantics axes",
		Snippet: "echo pre\necho $((1/0))\necho after\n",
		Why:     "the same three lines through `-c` rather than a script file, and it is a row about the *absence* of a route rule: the answer is byte-identical to the script one in all six columns. That is worth pinning because the neighboring readonly axis has a route field derived from comparing `-c` with a `;` against a file with newlines, which varied two things at once, and this pair is the shape that keeps the same mistake from being made here (#1171, #1182)",
	},
	{
		ID: "axis/a-failed-for-word-list-from-c", Category: "semantics axes",
		Snippet: "echo pre\nfor i in a $((1/0)) b; do echo \"A$i\"; done\necho after\n",
		Why:     "the same three lines through `-c` rather than a script file, and its answer is byte-identical to the script row in all six columns — which is the point. A fatality axis measured on the *diagonal* of route-by-separator is confirmatory for both \"the route decides\" and \"the separator decides\", and that is how a route field came to exist for a rule that has none (#1182). The same-program pair is what makes the absence of a route rule a measurement rather than an omission (#1215)",
	},
	{
		ID: "axis/a-failed-case-subject-from-c", Category: "semantics axes",
		Snippet: "echo pre\ncase $((1/0)) in \"\") echo E;; *) echo A;; esac\necho after\n",
		Why:     "the subject's route twin, for the reason the word list's has one. Both rows answer alike everywhere, so nothing here is route-dependent, and the pair says so rather than leaving it assumed",
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
		Why:     "bash and zsh move $_ to the previous command's last argument and to empty after a bare assignment; dash and ksh93 answer with nothing at all, because they keep no such parameter and `$_` is an ordinary unset name there. This reason said they leave it at the shell's own path, which the row beside it has never shown — the harness writes a shell's path as `<shell>` and both cells are empty. A wrong sentence over a right measurement is the failure `oracle-check` cannot see, since it compares behavior and not prose (#706)",
	},
	{
		ID: "special/underscore-after-a-declaration-command", Category: "parameters",
		Snippet: `x=1; echo "a=[$_]"; export y=2; echo "b=[$_]"`,
		Why:     "what a *declaration* command leaves in $_, and the panel gives four answers to it: bash 5.3 binds the assignment word as written, `y=2`; bash 3.2 binds the name alone, `y`; zsh binds the command word, `export`; dash and ksh93 keep no $_ at all. A bare assignment leaves it empty everywhere that has one, which is the first line and the control. The bash-to-bash disagreement is the point — a claim about this recorded against one build would have been wrong for the other",
	},
	{
		ID: "special/underscore-at-startup", Category: "parameters",
		Script:  true,
		Snippet: `echo "[$_]"`,
		Why:     "before any command has run, $_ holds how the shell was *invoked*: bash writes the path it was started as, and the same binary called `sh` writes `sh`, which is argv[0] rather than the path — so the parameter carries the invocation and not the executable. dash, ksh93 and zsh leave it empty. Run from a script rather than -c because that is the route where the answer is a path at all",
	},
	{
		ID: "special/underscore-inherited-from-the-environment", Category: "parameters", Env: []string{"_=inherited"},
		Snippet: `echo "[$_]"`,
		Why:     "the other half of the startup value, and the half no snippet can ask about, since nothing running inside a shell can put a name in the environment that shell was started with. An exported `_` beats the invocation in five of the six — bash writes argv[0] only where the environment said nothing — and zsh alone discards it and starts empty however it was called. It is also the row that shows the neighboring case is not the whole rule: three columns are empty there and only one of them is empty here",
	},
	{
		ID: "special/dollar-dash-in-full", Category: "parameters",
		Snippet: `echo "[$-]"`,
		Why:     "the whole of $- rather than a test for one letter in it. The `invoke/dollar-dash-*` cases ask whether `c` or `s` is present, which is the axis; this records what each shell actually carries, and the answers share almost nothing: dash writes *nothing at all* for a command string, bash `hBc`, ksh93 `chsB`, and zsh a set of digits. So there is no common alphabet to write a default over, and a claim about `$-` that does not name a shell is not a claim",
	},
	{
		ID: "special/dollar-dash-in-full-from-a-script", Category: "parameters",
		Script:  true,
		Snippet: `echo "[$-]"`,
		Why:     "the same string by the other route, and it moves in three of the six: the `c` goes, and ksh93 loses its `s` as well and lands on exactly bash's `hB`. Two shells that agree on one route and not on another is why a `$-` answer has to be recorded per route, and it is the pair with the case above that says so",
	},
	{
		ID: "special/dollar-dash-orders-the-letters-its-own-way", Category: "parameters",
		Snippet: `set -f; set -u; set -e; echo "[$-]"`,
		Why:     "three options turned on in a written order, and no shell reports them in it. bash sorts the lowercase letters and keeps its own suffix (`efhuBc`); ksh93 sorts including the letters it already had (`cefhsuB`); zsh puts its digits first (`569Xefu`); dash answers `ufe`, which is neither the order they were set in nor alphabetical but its own option table's. The order is therefore a property of the shell and never a fact about `$-`, which is worth pinning because a reader of any one row would assume otherwise",
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
		ID: "core/a-substitution-inside-an-expansion-brings-its-own-quoting", Category: "quoting",
		Snippet: `printf '[%s]\n' "${x:-"$( echo 'a"b' )"}"`,
		Why:     "the quote that ends a double-quoted run is not the next `\"` in the text: a `$( )` written inside one holds a *program*, so the single quotes in it quote that `\"` and the run continues past it. Every shell in the panel prints the word; a scanner that reads the run as text takes the inner `\"` as the closer, is left on the second `'`, and swallows the rest of the file — which is what all four dialects did, each in its own wording, until #1140. This is the shape `~/.zi/bin/lib/zsh/install.zsh` stops at on line 1389, written with `[^\"]` inside a `grep` pattern",
	},
	{
		ID: "core/a-swallowed-quote-changes-the-program-rather-than-refusing-it", Category: "quoting",
		Snippet: `printf '[%s]' "${x:-"$( echo 'a"b' )"}" "${y:-"$( echo "c'd" )"}"; echo`,
		Why:     "the same defect with a second expansion after it, and the reason this row is not a duplicate of the one above: two of them put the stray quotes back in balance, so nothing is ever unterminated and the misreading is *silent*. The output was `[a\"b} c'd]` — one field where there are two, a literal `}` that belongs to the grammar, and no diagnostic anywhere. A shell that only refused this shape would pass a corpus written from the refusal. The second expansion is the mirror of the first, a `'` inside double quotes, so neither quote character is the special one",
	},
	{
		ID: "core/what-a-nested-substitution-holds-is-not-a-delimiter", Category: "quoting",
		Snippet: `printf '[%s]' "${x:-"$(( 1 + $( printf %s '"' | wc -c ) ))"}" "${y:-"$( echo 'a")b' )"}"; echo`,
		Why:     "the two neighbors the fix has to reach as well: the arithmetic spelling, which nests a `$( )` of its own, and a single-quoted `)` that closes nothing. Both are unanimous. They are here because the counting scanner got the `)` and the `}` right already when they stood at the body's own level — `${x:-\"$( echo ')' )\"}` parsed on either side of the change — so a case built only from those two characters measures nothing, and only the `\"` inside the substitution tells the two readings apart",
	},
	{
		ID: "core/the-older-substitution-spelling-brings-its-own-quoting-too", Category: "quoting",
		Snippet: "printf '[%s]\\n' \"${x:-\"`echo 'e\"f'`\"}\"",
		Why:     "the backquoted spelling of the same rule, and it is here as a measurement rather than as a symmetry: ksh93 refuses a single quote inside backquotes inside a double quote when the word stands on its own — `a=\"`echo 'e\"f'`\"` is a syntax error there — and accepts it inside a `${ }` body, so all six agree on this row and only on this row. A fix written for `$( )` alone leaves the older spelling refused in all four dialects while the un-nested one is accepted, which is one construct answered two ways",
	},
	{
		ID: "core/single-quotes-in-a-quoted-expansion-body", Category: "quoting",
		Snippet: `v=VAL; printf '[%s]' "${u:-'$v'}" "${w:-''}"; echo`,
		Why:     "a `${ }` body written inside double quotes is double-quoted *content*, so a single quote in it is an ordinary character rather than a quote: all six shells keep the two quote characters and substitute the `$v` between them. The empty pair is the guard on the same fact — `''` is two characters here where a quoting reading makes it nothing",
	},
	{
		ID: "core/a-substitution-inside-those-quotes-is-performed", Category: "quoting",
		Snippet: `printf '[%s]' "${u:-'$(echo hi)'}" "${w:-'` + "`echo bq`" + `'}"; echo`,
		Why:     "the row that says the substitution inside those quotes is *recognized and run* rather than merely scanned past for the delimiter, which is the question the two readings could not otherwise be told apart on. Both spellings of a command substitution, and all six shells run both",
	},
	{
		ID: "core/an-unbalanced-substitution-inside-those-quotes", Category: "quoting",
		SyntaxError: true,
		Snippet:     `printf '[%s]' "${x:-'a$(b'}"; echo`,
		Why:         "the same fact reaching the parse, and unanimous as a refusal: with the `'` an ordinary character there is nothing to close the `$(`, so every shell in the panel fails the line. They part company only on the wording and the status, which makes this a diagnostics row as well as a grammar one",
	},
	{
		ID: "core/single-quotes-in-an-unquoted-expansion-body", Category: "quoting",
		Snippet: `v=VAL; printf '[%s]' ${u:-'$v'}; echo`,
		Why:     "the contrast that says the rule belongs to the enclosing context and not to the body: unquoted, the same characters are an ordinary single-quoted run in all six — the quotes removed and the `$v` never substituted. Without this row a fix could take the quotes literally everywhere and still pass the quoted one",
	},
	{
		ID: "core/double-quotes-in-a-quoted-expansion-body", Category: "quoting",
		Snippet: `v=VAL; printf '[%s]' "${u:-"$v"}" "${w:-x"y"z}"; echo`,
		Why:     "the other quote character in the same position, and it does *not* follow the same rule: an unescaped one there opens a run of its own and is removed, so `\"$v\"` comes to `VAL` where `'$v'` comes to `'VAL'`. The two quote characters part company inside a quoted body, and a reading that made both literal answers this row wrongly",
	},
	{
		ID: "core/single-quotes-in-a-quoted-pattern-operand", Category: "quoting",
		Snippet: `s=xay; printf '[%s]' "${s#'x'}" "${s#'a$(b'}"; echo`,
		Why:     "the guard that keeps the rule off the operand it does not govern: a *pattern* operand's quotes quote and are removed, so the first trims the `x` and the unbalanced substitution in the second is no substitution at all. Five shells answer both; zsh refuses the second, which is its own divergence and not this one",
	},
	{
		ID: "core/quotes-in-a-quoted-replacement-operand", Category: "quoting",
		Snippet: `s=xay; v=VAL; printf '[%s]' "${s/'a'/Z}" "${s/a/'$v'}" ${s/a/'$v'}; echo`,
		Why:     "the third reading of a quote in a `${ }` operand, and the one the panel divides on: a *pattern* operand's quotes quote and are removed everywhere, and a **replacement** operand's do in bash 5.3, that build as `sh` and ksh93 while bash 3.2 and zsh keep them as characters. Unquoted all five agree again, which is what says the disagreement is about the quoted context and not about the operator. dash has no operator and refuses the line. Recorded rather than answered — it wants a semantics axis, and this row is what a fix routed through the word operand's rule would break",
	},
	{
		ID: "core/single-quotes-in-a-heredoc-expansion-body", Category: "quoting",
		Snippet: "v=VAL; cat <<EOF\n[${u:-'$v'}]\nEOF",
		Why:     "a here-document body is the same context reached by the other road: it expands like a double-quoted string, so the quotes are characters there too and the `$v` between them is substituted. Unanimous, and it is the row that says the answer is the context's rather than the double quote character's",
	},
	{
		ID: "core/a-case-arm-inside-backquotes-inside-an-expansion", Category: "quoting",
		Snippet: "printf '[%s]\\n' \"${x:-\"$( echo `case a in a) echo y;; esac` )\"}\"",
		Why:     "the `)` of a case arm closes nothing, which `$( )` learned on its own — and one level further in, inside backquotes, the counting scan is the only thing that can be reached. Unanimous across the panel. It is the row that says the older spelling holds a program too rather than a run of text with a delimiter somewhere in it: a scan that reads across the backquotes takes the arm's `)` as the substitution's, ends it in the middle of the `case`, and leaves the expansion with no `}`",
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
		ID: "core/a-condition-that-ended-itself-takes-its-body", Category: "command language",
		Script: true, SyntaxError: true,
		Snippet: `if [[ -n x ]] { echo A; }; if (( 0 )) { echo B; } else { echo C; }`,
		Why:     "the rule the short-loop flag stands for, reaching `if`: a header that has *ended* may be followed straight by its body, and `(( … ))` and `[[ … ]]` end where a word does not. One shell takes it and four call the `{` a syntax error. Run from a file rather than through -c, because a command string whose last character is the `}` of a short body is a parse error in the shell that has the construct — the same text with a trailing newline parses, so -c would measure a grammar no script on disk has (#827)",
	},
	{
		ID: "core/a-short-if-takes-elif-and-else-the-same-way", Category: "command language",
		Script: true, SyntaxError: true,
		Snippet: `if [[ -z x ]] { echo A } elif [[ -n y ]] { echo B } else { echo C }`,
		Why:     "the arms take the body by the rule the `if` did, and the whole thing ends where the last one does: there is no `fi`. A `;` between them would end the command instead, which is the same separator that puts a brace group into a `while`'s condition. From a file, for the reason above",
	},
	{
		ID: "core/a-short-if-body-is-one-command", Category: "command language",
		Script: true, SyntaxError: true,
		Snippet: `if [[ -n x ]] echo A; echo after`,
		Why:     "and the body need not be a brace group at all — one command, exactly as a short loop's is, with the `;` after it belonging to whatever encloses the `if`. It is the row that says this is the loop rule widened rather than a brace-body production of its own",
	},
	{
		ID: "core/a-word-condition-does-not-end-itself", Category: "command language",
		Script: true, SyntaxError: true, GradedOnRefusal: true,
		Snippet: `if true { echo A; }`,
		Why:     "the boundary, refused by all five including the one that takes every row above: `true` is a simple command and `{` is another of its words, so the header never ended. Graded on the refusal because five shells decline in five wordings; what is pinned is that all five decline",
	},
	{
		ID: "core/a-separator-after-a-condition-ends-the-if", Category: "command language",
		Script: true, SyntaxError: true, GradedOnRefusal: true,
		Snippet: `if true; { echo A; }`,
		Why:     "the row the spec already recorded, and the other half of the rule: a `;` continues the condition list, so the brace group is the last thing *tested* and the `if` is left with no `then`. Refused everywhere, which is why the separator is the opposite of help here",
	},
	{
		ID: "core/a-count-loop", Category: "command language",
		Script: true, SyntaxError: true,
		Snippet: `repeat 3 { echo R; }; repeat 2 echo S; echo after`,
		Why:     "`repeat N` is a loop over a count rather than over a list or a condition, and its header is one word, so every body spelling the dialect has is reachable — the brace group and the one-command body here. Four shells have no such keyword and report `repeat: not found`",
	},
	{
		ID: "core/a-count-that-is-not-one-runs-nothing", Category: "command language",
		Script:  true,
		Snippet: `repeat x { echo R }; echo st=$?`,
		Why:     "the count is read once as an arithmetic expression before the first iteration, and a word that is not a number is not an error: the body runs no times and the loop reports success, which is the same answer `repeat 0` gives. A shell that refused it would stop a script the reference does not",
	},
	{
		ID: "core/a-loop-that-ends-with-end", Category: "command language",
		Script: true, SyntaxError: true,
		Snippet: `foreach f (a b); echo $f; end`,
		Why:     "the same loop a `for` is, under two other words. `end` is what it adds — reserved wherever a command may begin in that shell, so `end` alone is a parse error there — and `for f (a b); …; end` is refused, which is what says the terminator belongs to the opening word rather than to the parenthesized list",
	},
	{
		ID: "core/a-function-with-no-name-runs-where-it-stands", Category: "command language",
		Script: true, SyntaxError: true,
		Snippet: `() { echo "[$0][$1][$#]"; } p q`,
		Why:     "`()` where a command begins is an empty parameter list rather than a subshell, and the words after the body are the call's positional parameters — which is what makes it a call and not only a definition, since there is no name to invoke it by later. The shell that has it names the frame `(anon)`",
	},
	{
		ID: "core/a-nameless-function-is-a-function", Category: "command language",
		Script: true, SyntaxError: true,
		Snippet: `x=1; () { local x=2; echo in=$x }; echo out=$x; function { echo fanon; }`,
		Why:     "and it is a function rather than a group, which `local` is the test for: the name is restored on the way out where a brace group would have left it changed. The `function` word with no name after it is the other spelling of the same thing",
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
		ID: "core/two-commands-need-a-separator-between-them", Category: "command language", SyntaxError: true,
		Snippet: `(echo a) echo b`,
		Why:     "the list production's separator is required, and this is the shortest text that shows it. Every shell in the panel names the second command's word and refuses the line; the reading that takes it is two statements, which would print a and b — a *different program*, so the wrong answer here is silent rather than noisy. Nothing shows the rule until a compound is written, because a simple command's words absorb whatever follows and `true echo x` is one command with an argument",
	},
	{
		ID: "core/a-compound-does-not-absorb-the-word-after-it", Category: "command language", SyntaxError: true,
		Snippet: `echo one; { :; } echo x`,
		Why:     "the same rule with a statement in front of it, so that the refusal cannot be a property of the line's first command. `echo one` never runs either: the panel parses the whole `-c` string before running any of it, so a failure anywhere in it discards everything. The construct is a brace group here rather than a subshell to show that the rule is about the *list* and not about parentheses",
	},
	{
		ID: "core/a-separator-is-needed-after-a-redirected-compound", Category: "command language", SyntaxError: true,
		Snippet: `{ :; } 2>/dev/null echo b`,
		Why:     "a redirection after a compound command belongs to the compound and does not reopen it, so a word after the redirection is still a second command with nothing between. Worth pinning apart from the bare form because the suffix is the one place a parser might keep reading words — and if it did, `echo b` would become an argument of nothing",
	},
	{
		ID: "core/the-missing-separator-is-named-inside-a-group", Category: "command language", SyntaxError: true,
		Snippet: `{ (echo a) echo b; }`,
		Why:     "where the failure is reported when the list is a construct's rather than the program's. All five name the token the list stopped on, and the one shell that prints an expectation adds the closer that was waiting — `(expecting \"}\")` here, `\")\"` in a subshell and `\"done\"` in a loop, so the token comes from the list and the expectation from whatever enclosed it",
	},
	{
		ID: "core/two-subshells-with-nothing-between-them", Category: "command language", SyntaxError: true,
		Snippet: `(echo a) (echo b)`,
		Why:     "the same missing separator where the token that follows is an operator rather than a word. It changes what the diagnostics say — the shell that classifies a token calls this one `\"(\"` where the rows above are `word` — so it grades the class as well as the refusal",
	},
	{
		ID: "core/a-missing-separator-inside-a-loop-body", Category: "command language", SyntaxError: true,
		Snippet: `while true; do (echo a) echo b; done`,
		Why:     "a `do … done` body is an ordinary list and needs the separator an ordinary list needs. Paired with the short-loop rows below, which are the one place the panel splits: the shell with short loops reads a command after an *ended header* as the loop's body, and even there a `do … done` body is this",
	},
	{
		ID: "core/a-for-over-a-parenthesized-list", Category: "command language", SyntaxError: true,
		Snippet: `for i (a b) { echo "$i"; }; for j (p q) echo "$j"`,
		Why:     "the short `for`, in both its body spellings. The parentheses say what `in` says and end the header as `in` does not, which is why this one takes a brace body with nothing between where `for i in a b { … }` cannot. One shell parses it and four call the `(` a syntax error",
	},
	{
		ID: "core/a-for-name-that-is-an-expansion", Category: "command language", SyntaxError: true,
		Snippet: `n=x; for $n in a b; do echo "[$x][$n]"; done`,
		Why:     "a loop whose name comes out of an expansion, which all six columns refuse and every dialect of ours took: the loop bound a variable literally called `n`, so the script's own `$n` read the list's words and the name it meant to reach through the expansion stayed empty — at status 0 and with nothing said. The token's literal is what hid it, since a `$n` word reports `n` and satisfies the name test. The refusal has four wordings across the six columns and three statuses, and the two `echo`s are in the body so that a shell which *ran* the loop would be caught by the output rather than only by the number. Nothing follows the loop, because bash reports the complaint and goes on where the other five stop, which is #1110 and not this",
	},
	{
		ID: "core/a-for-name-that-is-a-quoted-word", Category: "command language", SyntaxError: true,
		Snippet: `for "i" in a b; do echo "[$i]"; done`,
		Why:     "the axis beside it: ksh93 removes the quoting and binds `i`, and bash, bash as `sh`, bash 3.2, dash and zsh all refuse the same word. So the *quoting* of a loop's name is a dialect's answer where an expansion in it is core, and the two had to be separated in one predicate rather than being one plainness test — `for \"$n\"` is refused by the quoting shell too. `for 'i'`, `for i\"\"`, `for \"i\"x` and `for \\i` were measured beside this and answer alike in every column, so the escape travels with the quotes and the flag is one bit",
	},
	{
		ID: "core/a-select-name-that-is-an-expansion", Category: "command language", SyntaxError: true,
		Snippet: `n=x; select $n in a b; do break; done`,
		Why:     "the menu loop's header is a for-loop's here too, and the same refusal reaches it — which is worth a row of its own because dash gets there differently: it has no `select` at all, so the word is a command, `$n in a b` are its arguments, and the complaint is about the `do`. Four wordings again and a fifth failure that is not this rule",
	},
	{
		ID: "core/a-for-with-more-than-one-name", Category: "command language", SyntaxError: true,
		Snippet: `for key value ( a 1 b 2 ) { echo "$key=$value" }`,
		Why:     "a loop that names two variables takes two words from its list on every pass, so a flat list is walked as pairs — one shell has it and four call the second name or the `(` a syntax error. It is how a script reads a serialized key/value table, and it is what stopped a real zsh library's own parse. Both of the halves it could be confused with are already pinned: the parenthesized list is `core/a-for-over-a-parenthesized-list` and the brace body is `core/a-list-for-with-a-brace-body`, and this row is the *name count* and nothing else",
	},
	{
		ID: "core/a-for-with-more-than-one-name-and-a-do-body", Category: "command language", SyntaxError: true,
		Snippet: `for key value in a 1 b 2; do echo "$key=$value"; done`,
		Why:     "the same name count with the body and the list spelled the way every shell spells them, which is what says the three are independent rather than one feature arriving together. Measured over all four combinations before it was built: the shell that has the name count accepts it with `in` and with parentheses, with `do … done` and with braces, and with no list at all. Ours read the first name and refused at `do`",
	},
	{
		ID: "core/a-for-with-more-than-one-name-and-a-short-last-pass", Category: "command language", SyntaxError: true,
		Snippet: `for a b ( 1 2 3 ) { echo "[${a-U}][${b-U}]" }; echo "after=[${a-U}][${b-U}]"`,
		Why:     "where the binding rule is actually decided: a list whose length is not a multiple of the name count still runs the short pass, and the names it did not reach are **empty rather than unset** — `[3][]`, not `[3][U]` and not `[3][2]`. The three plausible wrong answers all agree with the right one on a list that divides, which is why the row is written with three items and not four. The names keep those values after the loop",
	},
	{
		ID: "core/a-for-with-more-than-one-name-and-no-list", Category: "command language", SyntaxError: true,
		Snippet: `set -- p q r s; for a b; do echo "[$a][$b]"; done`,
		Why:     "with no list the loop walks the positional parameters, and it walks them in groups of the same size — so the absent list and the empty one stay the different questions ForClause.HasItems draws them as. The separator is what ends the header here, and it has to: without one the next word would be another name",
	},
	{
		ID: "core/a-short-for-body-may-not-follow-the-names", Category: "command language", SyntaxError: true,
		Snippet: `set -- p q; for a print -r -- "[$a]"`,
		Why:     "the subtractive half of the name count, and the one a shell without it gets wrong in the permissive direction: every word after the first is another name until the header ends, so `print` is a name and `-r` is not one — a parse error in the only shell that has any of this. We accepted it as a short body and ran the loop. `core/a-for-header-that-ends-itself-needs-no-body` and the parenthesized rows are the shapes that still take a short body, and they are what this must not break",
	},
	{
		ID: "core/a-select-takes-one-name", Category: "command language", SyntaxError: true,
		Snippet: `select a b ( x y ) { echo "[$a]"; break }`,
		Why:     "the menu loop's header is a for-loop's in every other respect and parts company over exactly this: the one shell that gives `for` more than one name refuses to give `select` more than one. Recorded because sharing the header is the obvious way to implement it and would have been wrong in the one place a test written from the `for` side would not look",
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
		ID: "getopts/optind-starts-at-one", Category: "getopts",
		Snippet: `echo "OPTIND=[$OPTIND]"`,
		Why:     "OPTIND is 1 before anything has called getopts, in all six — it is initialized when the shell starts rather than when the builtin first runs. A script that reads it to decide how many operands to `shift` past does so *after* the loop, but one that tests it before entering the loop, or that runs no options at all, reads whatever startup left. Found by the function case beside it, which is why a one-line case sits in front of a nine-line one",
	},
	{
		ID: "getopts/optind-ignores-an-inherited-value", Category: "getopts", Env: []string{"OPTIND=7"},
		Snippet: `echo "OPTIND=[$OPTIND]"`,
		Why:     "the startup value is written rather than merely defaulted: an OPTIND in the environment is overwritten with 1 by all six, so a shell that stops at `unset means 1` still answers 7 here and a script that inherited one from its caller would start its scan in the middle. It is the half of the startup value that reading the variable alone cannot see, since both a fresh shell and a leaking one print a number",
	},
	{
		ID: "getopts/a-function-with-its-own-optind", Category: "getopts",
		Snippet: `f() { local OPTIND=1 o; while getopts ab o; do printf "[%s]" "$o"; done; echo " rest=$((OPTIND))"; }; f -a -b; f -b -a; echo "outer OPTIND=$OPTIND"`,
		Why:     "the way a function is written so it can be called twice: a local OPTIND starts each scan at 1 and leaves the caller's alone. Five of the six do exactly that; ksh93 has no `local`, so its OPTIND is the one global and the second call finds the scan already finished — which is not a getopts difference but the `local` axis reaching a builtin's state, and the reason a portable function resets OPTIND by assigning to it rather than by declaring it",
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
		// The refusal itself is the core answer: every shell in the panel
		// says something, keeps the value, and reports 1 where the report can
		// be read. What follows splits — dash, zsh and bash-as-`sh` end the
		// script and print nothing more — and the split is not the one a
		// failed special builtin usually makes, which is why the case records
		// the whole line rather than only the status.
		ID: "unset/a-readonly-name-is-refused", Category: "semantics axes",
		Snippet: `readonly x=1; unset x; echo "st=$? [${x-gone}]"; echo after`,
		Why:     "`unset` of a readonly name is refused by every shell in the panel: all six print a complaint and leave the value standing, which makes the refusal and the survival core. Three carry on and report 1 — bash, bash 3.2 and ksh93, and ksh93 calls it a *warning* in so many words — while dash, zsh and bash invoked as `sh` end the script there, at dash's 2 and the others' 1. No two of the four wordings agree",
	},
	{
		ID: "unset/a-readonly-name-refused-alongside-another", Category: "semantics axes",
		Snippet: `readonly x=1; y=2; unset x y; echo "st=$? [${x-gone}][${y-gone}]"; echo after`,
		Why:     "the refusal is one name's and not the builtin's: the shells that carry on go on to remove `y`, so a single readonly operand does not save the rest. The status is still 1, which says the builtin reports the failure rather than the last name it managed",
	},
	{
		ID: "unset/a-readonly-name-with-no-value-is-refused-too", Category: "semantics axes",
		Snippet: `readonly y; unset y; echo "st=$? [${y-gone}]"; echo after`,
		Why:     "the attribute is what is refused and not the value: `readonly y` with nothing assigned leaves a name that is unset and unremovable, and every shell in the panel complains about removing it. The `gone` here is the name never having had a value, not the unset succeeding",
	},
	{
		ID: "unset/a-readonly-name-under-v-is-refused", Category: "semantics axes",
		Snippet: `readonly x=1; unset -v x; echo "st=$? [${x-gone}]"; echo after`,
		Why:     "the letter that says the operand is a variable changes nothing here, which is worth pinning because it changes the *name* rules in bash — `unset 1x` is quiet there and `unset -v 1x` is refused, so the two routes into the builtin are not the same code in every shell",
	},
	{
		ID: "unset/a-readonly-name-behind-a-subscript-is-refused", Category: "semantics axes",
		Snippet: `readonly a=1; unset a[0]; echo "st=$? [${a-gone}]"; echo after`,
		Why:     "a subscripted operand is refused by the variable the subscript indexes, and the shells that reach the check name the base — `a`, not `a[0]` — so the refusal stands ahead of the element path and the subscript is never evaluated. Only bash and ksh93 get that far: dash refuses `a[0]` as a bad variable name first and zsh reads the brackets as a pattern that matches nothing, so this row records three different complaints for one line and is the reason the wording is worth reading rather than the status alone",
	},
	{
		ID: "unset/takes-away-an-environment-name", Category: "parameters",
		Snippet: `unset HOME; echo "[${HOME-gone}]"`,
		Why:     "a name that arrived in the environment rather than from an assignment is still a name `unset` removes — deleting it from the shell's own table is not enough, because a lookup reads both",
	},
	{
		ID: "unset/the-m-option-unsets-by-pattern", Category: "parameters",
		Snippet: `x=1 xy=2 z=3; unset -m "x*"; echo "st=$? [${x-gone}][${xy-gone}][${z-gone}]"`,
		Why:     "`unset -m` reads its operands as patterns rather than as names, and is one shell's alone — a prompt theme clears its whole namespace with it. The three values are what says the pattern is anchored at both ends of the *name*: `x*` takes `x` and `xy` and leaves `z`. The other four refuse the letter, in four wordings and at three statuses, and two of them print a usage line after it",
	},
	{
		ID: "unset/the-n-option-splits-the-panel", Category: "parameters",
		Snippet: `x=1; unset -n x; echo "st=$?"`,
		Why:     "the letter beside it, and the row that says `unset`'s options are the dialect's rather than one set: bash 5.3 and ksh93 take `-n` where bash 3.2, bash-as-`sh`, dash and zsh refuse it, at three statuses and in four wordings. The *value* is deliberately not printed: what `-n` then does to a name that is not a reference splits the two shells that have the letter — bash removes nothing at all and ksh93 removes the variable — which is a second question, and one this shell does not yet answer",
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
		Why:     "half the panel names the conversion character alone and half names the whole directive as written, with four wordings and three statuses between them. `z` is a length modifier in three of them, so the character they cannot read here is the `]`",
	},
	{
		ID: "printf/a-length-modifier-on-a-conversion", Category: "printf",
		Snippet: `printf "[%ld][%5ld][%Lf]\n" 42 42 1.5; echo "st=$?"`,
		Why:     "the C length modifiers every shell with any of them takes, in the place C puts them — after the precision and before the verb. The one shell with none reads the `l` as the conversion and says so",
	},
	{
		ID: "printf/a-length-modifier-c99-added", Category: "printf",
		Snippet: `printf "[%zX][%jd][%lld][%hhd]\n" 255 42 42 42; echo "st=$?"`,
		Why:     "the C99 additions, which is where the panel splits three ways rather than two: two shells take them, one takes only C89's `h`, `l` and `L` and calls these invalid directives, and one takes none at all",
	},
	{
		ID: "printf/a-length-modifier-is-read-and-thrown-away", Category: "printf",
		Snippet: `printf "[%hhd][%lld]\n" 300 9223372036854775807; echo "st=$?"`,
		Why:     "an accepted modifier never narrows or widens anything: 300 through `%hhd` is 300 and not 44, and the largest signed 64-bit value survives `%lld` — so this is about what a format may say and never about what it means",
	},
	{
		ID: "printf/a-run-of-length-modifiers", Category: "printf",
		Snippet: `printf "[%llld][%hld]\n" 42 42; echo "st=$?"`,
		Why:     "the shells with the C99 set skip a run of the letters rather than a list of spellings, so nonsense like `lll` and `hl` is accepted; the shell with C89's set takes exactly one letter and refuses both",
	},
	{
		ID: "printf/a-format-that-ends-at-the-percent", Category: "printf",
		Snippet: `printf 'a%'; echo " st=$?"`,
		Why:     "four answers to a format that ran out before its conversion character, and not one of them is the ordinary bad-conversion complaint: bash has a second wording for it, zsh spells its usual one with the directive, dash names nothing and reports 2, and ksh93 writes a literal % and succeeds",
	},
	{
		ID: "printf/a-format-that-ends-after-a-width", Category: "printf",
		Snippet: `printf 'a%5'; echo " st=$?"`,
		Why:     "the same with a prefix to name, which is what shows bash and zsh naming the whole directive rather than a conversion character there is none of — and shows ksh93 dropping the prefix, since `a%5` is `a%` and not `a%5`",
	},
	{
		ID: "printf/a-format-that-ends-after-a-length-modifier", Category: "printf",
		Snippet: `printf 'a%ll'; echo " st=$?"`,
		Why:     "the modifier belongs to the directive, so the shells that name it say `%ll`. dash has no modifiers at all, so for it the format did not run out — the `l` is the conversion it could not read, and it says so instead",
	},
	{
		ID: "printf/a-trailing-percent-after-a-doubled-one", Category: "printf",
		Snippet: `printf 'a%%b%'; echo " st=$?"`,
		Why:     "a doubled percent earlier in the format is unrelated to one that ends it: everything before the last % is written by every shell, and only the trailing one is the unfinished conversion",
	},
	{
		ID: "printf/a-flag-after-the-width", Category: "printf",
		Snippet: `printf '[%5-d][%5 d]' 42 42; echo " st=$?"`,
		Why:     "ksh93 reads a flag after the width and acts on it — left-justified in five, then the space flag — where the rest of the panel wants flags first and reports the flag character as a conversion it could not read. No length modifier is in either directive, so this is about the shape of the prefix and nothing to do with the modifier set",
	},
	{
		ID: "printf/a-second-dot-is-an-output-base", Category: "printf",
		Snippet: `printf '[%..36d][%..2d]' 1295 5; echo " st=$?"`,
		Why:     "the field after a second dot is ksh93's output base and not a second precision: 1295 in base 36 is zz and 5 in base 2 is 101. The rest of the panel stops at the second dot and calls it a conversion it could not read",
	},
	{
		ID: "printf/an-output-base-beside-a-precision", Category: "printf",
		Snippet: `printf '[%.3.16d]' 255; echo " st=$?"`,
		Why:     "both fields at once, which is how the base is told from a precision that happens to look like one: 255 in base 16 padded to three digits is 0ff, where a second precision could only have produced a decimal",
	},
	{
		ID: "printf/a-flag-after-a-precision-drops-it", Category: "printf",
		Snippet: `printf '[%.3-d][%-.3d]' 42 42; echo " st=$?"`,
		Why:     "the two spellings are not the same directive in ksh93 even though both hold a precision of 3 and a minus: written after the precision the minus loses it and 42 stays 42, written before it the precision survives and 42 is 042 — so the prefix is not simply read in any order",
	},
	{
		ID: "printf/an-unknown-conversion-names-the-character", Category: "printf",
		Snippet: `printf "%v]xY" 1; echo "st=$?"`,
		Why:     "a conversion no shell in the panel has, with a tail after it, which is what separates naming the conversion character from naming what follows it: two shells say `v` and two say `%v`, and none of them names the `]`",
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
	{
		ID: "printf/hex-escape-in-a-format", Category: "printf",
		Snippet: `printf 'a\x41Z' | od -An -tx1 | tr -s " "`,
		Why:     "\\xHH is an escape in a format for every shell in the panel but dash, which has none and writes the six characters as they stand",
	},
	{
		ID: "printf/hex-escape-writes-one-raw-byte", Category: "printf",
		Snippet: `printf 'a\x80Z' | od -An -tx1 | tr -s " "`,
		Why:     "the escape produces a byte and not a character, so a value no encoding claims is one byte wide — read as hexadecimal because the display cannot tell 80 from the two bytes UTF-8 spells U+0080 with, and unanimous among the five that have the escape at all",
	},
	{
		ID: "printf/hex-escape-digit-run-diverges", Category: "printf",
		Snippet: `printf '[\x0ff]' | od -An -tx1 | tr -s " "`,
		Why:     "how wide the digit run is: bash and zsh stop at two and the value is a byte, so this is 0x0f followed by an f, and ksh93 takes every digit and reads more than two of them as a code point, so the same text is U+00FF in UTF-8",
	},
	{
		ID: "printf/hex-escape-four-digits-is-a-code-point", Category: "printf",
		Snippet: `printf '[\x0041]' | od -An -tx1 | tr -s " "`,
		Why:     "the same split with a value that is ASCII, which shows the switch is on the number of digits rather than on the size of the value: two digits would be a NUL and a 4 and a 1, and every digit read as a code point is an A",
	},
	{
		ID: "printf/hex-escape-with-no-digits", Category: "printf",
		Snippet: `printf 'a\xZ' | od -An -tx1 | tr -s " "`,
		Why:     "an empty digit run: bash leaves the escape standing and warns without failing, ksh93 and zsh read the run as a zero and write a NUL, and dash has no escape here to have an empty run",
	},
	{
		ID: "printf/hex-escape-is-not-a-b-escape", Category: "printf",
		Snippet: `printf '%b' 'a\x41Z' | od -An -tx1 | tr -s " "`,
		Why:     "the site matters and not only the shell: ksh93 reads \\x41 in a format and leaves it as written in a %b argument, which expands the set echo expands. bash and zsh have it in both and dash in neither, so ksh93 alone separates the two tables",
	},
	{
		ID: "printf/an-octal-in-a-b-escape-is-not-the-formats", Category: "printf",
		Snippet: `printf '%b' 'a\0101Z' | od -An -tx1 | tr -s " "; printf 'a\0101Z' | od -An -tx1 | tr -s " "`,
		Why:     "the two escape tables in one line, and unanimous in both halves: a %b reads \\0 and up to three octal digits after it, so \\0101 is an A, where a format reads up to three digits with the zero optional, so the same text is a backspace and a 1. Reading a %b the format's way gives neither answer",
	},
	{
		ID: "printf/a-b-escape-octal-is-a-byte", Category: "printf",
		Snippet: `printf '%b' 'a\0300Z' | od -An -tx1 | tr -s " "`,
		Why:     "the same byte-and-not-a-code-point rule a format's octal follows, asked at the other site: 0300 is the one byte 0xc0 in all six and never the two UTF-8 spells U+00C0 with",
	},
	{
		ID: "printf/a-b-escape-octal-without-the-zero-diverges", Category: "printf",
		Snippet: `printf '%b' 'a\101Z' | od -An -tx1 | tr -s " "`,
		Why:     "whether the \\0 is required: bash and dash read a bare \\101 as an A, ksh93 and zsh write the four characters. Not the same grouping as the same text in an echo argument, where dash alone reads it — which is what makes this an axis of its own rather than echo's answer reused",
	},
	{
		ID: "printf/a-b-escape-splits-esc-from-capital-esc", Category: "printf",
		Snippet: `printf '%b' 'a\eZ:a\EZ' | od -An -tx1 | tr -s " "`,
		Why:     "the two spellings of the escape character are two questions, because the two shells that split them split them in opposite directions: ksh93 has \\E and not \\e, zsh has \\e and not \\E, bash has both and dash neither. One answer for both letters is wrong for half the panel",
	},
	{
		ID: "printf/a-b-escape-hex-with-no-digits", Category: "printf",
		Snippet: `printf '%b' 'a\xZ' | od -An -tx1 | tr -s " "`,
		Why:     "the empty digit run at the %b site rather than the format's: bash leaves the escape standing and warns without failing, zsh reads the run as a zero and writes a NUL, and ksh93 and dash have no \\x here at all — so ksh93's answer is the one that differs from its own format's",
	},
	{
		ID: "printf/backslash-c-in-a-b-escape-always-stops", Category: "printf",
		Snippet: `printf '%b' 'a\cbZ' | od -An -tx1 | tr -s " "`,
		Why:     "the one place the two tables converge where the format's diverges: \\c ends the output in all six here, where the same two characters in a format are literal in bash and dash, control-X in ksh93 and a full stop in zsh",
	},
	{
		ID: "printf/a-b-escape-a-stop-cut-short-still-fills-its-field", Category: "printf",
		Snippet: `printf '[%5b]' 'a\cb'; printf '[%.1b]' 'ab\cc'`,
		Why:     "what a `\\c` leaves goes through the conversion's field like any other text in five of the six — padded to the width, cut to the precision — where ksh93 alone writes it as it stands. A property of the *stop*: with nothing stopping it that shell pads and truncates like the rest, which `printf/a-b-escape-a-field-with-nothing-stopping-it` is here to show",
	},
	{
		ID: "printf/a-b-escape-a-field-with-nothing-stopping-it", Category: "printf",
		Snippet: `printf '[%5b][%.1b]' 'ab' 'abc'`,
		Why:     "the control for the row above, and unanimous: a `%b` with no `\\c` in it is padded and truncated by every shell in the panel. Without this row the other one reads as `ksh93 has no field for %b`, which is not what it does",
	},
	{
		ID: "printf/backslash-c-in-a-b-escape-ends-the-whole-printf", Category: "printf",
		Snippet: `printf '[%b][%s]' 'a\cb' x; echo END`,
		Why:     "the stop is not confined to the conversion that read it: the format is abandoned where it stands, the operands after it go unused, and the reused format does not run again — unanimous, and the half of \\c that a case reading only one conversion cannot see",
	},
	{
		ID: "printf/an-octal-escape-is-a-byte-and-not-a-code-point", Category: "printf",
		Snippet: `printf 'a\300Z' | od -An -tx1 | tr -s " "`,
		Why:     "unanimous, and worth pinning as bytes: \\300 is the single byte 0xc0 in all six, never the two bytes UTF-8 gives the code point of the same number",
	},
	{
		ID: "printf/a-quoted-escape-used-as-a-format", Category: "printf",
		Snippet: `printf $'a\xc0Z' | od -An -tx1 | tr -s " "`,
		Why:     "the byte arrives already decoded and the format only has to carry it, which is a different route to the output from an escape the format decodes itself. dash has no quoted-escape form, so the dollar sign is part of the word there",
	},
	{
		ID: "printf/a-c-conversion-writes-one-byte", Category: "printf",
		Snippet: `printf '%c' $'\xc0' | od -An -tx1 | tr -s " "`,
		Why:     "%c takes the first byte of its operand rather than the first character, so a byte above the ASCII range is written alone and not as the pair an encoding would spell it with",
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
		// The shape that was wrong, and the reason it is this one. A handler
		// runs between commands, so an `exit` raised by a signal delivered
		// during the body is read at the top of the *next* command — and in
		// a `while` the next command is the condition. A loop that reads
		// that refusal as a condition's answer finds it non-zero, decides
		// the loop is over, and writes its own bookkeeping over the 7: the
		// handler's line is printed, `after` is never reached, and the shell
		// exits 0 all the same.
		//
		// Bounded rather than `while :`, so a shell that loses the exit
		// fails this case instead of hanging on it.
		ID: "trap/an-exit-from-a-handler-ends-a-while-loop", Category: "traps and exit",
		Snippet: `trap 'echo caught; exit 7' USR1; i=0; while [ $i -lt 3 ]; do i=$((i+1)); kill -USR1 $$; done; echo after`,
		Why:     "an `exit` raised inside a signal handler ends the shell from wherever it was raised, and a loop that was running when the signal arrived is not an exception: all six print `caught` once, never reach `after`, and exit 7. It is the status a caller acts on, and the one shape most likely to be got wrong, because a `while` asks its condition a question immediately after the handler has answered a different one",
	},
	{
		ID: "trap/an-exit-from-a-handler-ends-an-until-loop", Category: "traps and exit",
		Snippet: `trap 'echo caught; exit 7' USR1; i=0; until [ $i -ge 3 ]; do i=$((i+1)); kill -USR1 $$; done; echo after`,
		Why:     "the same for the loop that reads its condition the other way round, unanimously 7. It is worth having beside the `while` row rather than assumed from it: an `until` inverts the sense of the status it reads, so a shell that mistakes a refusal for a condition gets the *opposite* wrong answer here — it runs the body again rather than deciding the loop is over",
	},
	{
		ID: "trap/an-exit-from-a-handler-ends-a-for-loop", Category: "traps and exit",
		Snippet: `trap 'echo caught; exit 7' USR1; for i in 1 2 3; do kill -USR1 $$; done; echo after`,
		Why:     "the control row. A `for` reads no condition, so it has nothing to mistake a refusal for, and it was right while the `while` was wrong — which is what says the fault was in how a loop reads its control state rather than in how a trap sets one. All six exit 7 here too",
	},
	{
		ID: "trap/an-exit-from-a-handler-ends-nested-loops", Category: "traps and exit",
		Snippet: `trap 'echo caught; exit 7' USR1; i=0; while [ $i -lt 2 ]; do j=0; while [ $j -lt 2 ]; do j=$((j+1)); kill -USR1 $$; done; i=$((i+1)); done; echo after`,
		Why:     "an exit raised two loops deep leaves both of them, rather than the inner one only: the outer loop's own condition is the next command after the inner loop returns, so a shell that recovers at one level goes round the outer loop and reports its bookkeeping instead. Still `caught` once and 7 in all six",
	},
	{
		ID: "trap/an-exit-from-a-handler-ends-a-loop-inside-a-function", Category: "traps and exit",
		Snippet: `trap 'echo caught; exit 7' USR1; f() { i=0; while [ $i -lt 3 ]; do i=$((i+1)); kill -USR1 $$; done; }; f; echo after`,
		Why:     "and the same through a function boundary, which is the shape a real script has: an `exit` is not a `return`, so the function call it was raised inside does not absorb it. `caught` and 7 in all six, with `after` unreached",
	},
	{
		ID: "trap/lineno-inside-an-action", Category: "traps and exit",
		Script:          true,
		LayoutSensitive: true,
		Snippet:         "trap 'echo \"in trap LINENO=$LINENO\"' USR1\necho one\nkill -USR1 $$\necho two\n",
		Why:             "which line a trap action thinks it is on, and the panel gives two answers: bash 5.3 numbers the action's own text from 1, while bash 3.2, ksh93, dash and zsh report the line the signal was delivered on. So a trap body is a little program of its own in one shell and part of the script in four, and the split runs *through* bash rather than between bash and the rest — which is why the case is worth having over an assertion that names `bash`",
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
		// The doubling loop of the pipeline cases, so the write cannot fit in
		// the pipe and the reader has already gone. `2>/dev/null` on the
		// failing write alone is what keeps this about the handler rather
		// than about four different wordings for the write.
		ID: "trap/a-pipeline-element-runs-the-pipe-handler-it-set-for-itself", Category: "traps and exit",
		Snippet: `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; { trap 'echo child >&2' PIPE; echo "$v" 2>/dev/null; echo reached >&2; } | true; echo after`,
		Why:     "a handler is not only the shell's to set: an element that traps PIPE for itself runs that handler when its own write meets the broken pipe, and every shell in the panel does it — `child` then `reached` on standard error, in that order, in dash, both bash builds, ksh93 and zsh. The trap has to be set *inside* the element, because a handled signal is back at its default across the boundary and only an ignore crosses intact, so this cannot be spelled from the outside. ksh93 prints `after` before either of them, which is why the two streams are recorded apart",
	},
	{
		ID: "trap/an-elements-broken-pipe-is-not-the-outer-shells-to-handle", Category: "traps and exit",
		Snippet: `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; trap 'echo outer >&2' PIPE; { trap 'echo child >&2' PIPE; echo "$v" 2>/dev/null; echo reached >&2; } | true; echo after`,
		Why:     "the same shape with a handler on both sides, which is the half that says where the signal went: the element's handler runs and the shell's never does, in all five, because the broken pipe was the element's and the process never had it. It is the case a naive fix breaks — recording the arrival where the shell can see it makes the shell run `outer` for a signal it was never sent",
	},
	{
		ID: "trap/an-elements-pipe-handler-runs-inside-the-elements-redirections", Category: "traps and exit",
		Snippet: `v=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done; { trap 'echo child >&2' PIPE; echo "$v"; } 2>e | true; echo "e=[$(cat e)]"`,
		Why:     "two facts one shape can hold, and both are about *when* the handler runs. The failed write is the element's last command, so there is no command after it to run a handler between — and every shell in the panel runs it anyway, which makes the end of the element's body a boundary of its own. And `child` lands in the file rather than on the shell's standard error, so it runs while the element's own redirection is still in force: the boundary is at the end of the element's *list*, inside the redirection, and not after the body has been taken down. The wording of the failed write lands in the file too, ahead of the handler, which is the ordering all five agree on — ksh93 has no wording, and zsh says its own twice and runs the handler three times",
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
		ID: "signal-death/a-subshell-that-signaled-the-shell", Category: "traps and exit",
		Snippet: `(kill -TERM $$; echo inner); echo outer`,
		Why:     "the whole of what a subshell changes about a fatal self-signal, and the one place in the grammar that changes anything. The shell ends by the signal in all six and `outer` prints in none of them, so what splits is `inner`: bash 5.3.15, bash 3.2.57, bash 3.2 as `sh`, dash and zsh print it and ksh93 does not. Not a delivery race — the same answer on twenty-five runs of each under load, and unchanged by a `sleep 0.3` between the kill and the echo. The reason is the opposite of the obvious one: measured with a child started inside the subshell and its parent process id read back, the five that keep going are the five that gave the subshell a **process of its own**, so the signal aimed at `$$` never reached it; ksh93 runs the subshell in the shell's own process and the signal lands on the thing that was about to run the echo. Semantics.SubshellRunsOnAfterSignalingTheShell",
	},
	{
		ID: "signal-death/a-brace-group-does-not-outlive-the-signal", Category: "traps and exit",
		Snippet: `{ kill -TERM $$; echo inner; }; echo outer`,
		Why:     "the control for the row above, and the reason that one names a subshell rather than a compound command: a brace group is the same shell, so all six stop at once and print nothing. Grouping is not what defers the death — being a separate process is",
	},
	{
		ID: "signal-death/a-function-body-does-not-outlive-the-signal", Category: "traps and exit",
		Snippet: `f() { kill -TERM $$; echo inner; }; f; echo outer`,
		Why:     "the same control one level further in, because a function body is the place a reader would next expect the boundary to be. Unanimous: nothing prints, in all six. Together with the brace group and the loop below this is what makes the subshell row an axis about processes and not an axis about scopes",
	},
	{
		ID: "signal-death/a-loop-body-does-not-outlive-the-signal", Category: "traps and exit",
		Snippet: `i=0; while [ $i -lt 1 ]; do kill -TERM $$; echo inner; i=1; done; echo outer`,
		Why:     "the third control, and the one that asks whether a shell checks for its own death only between *top-level* commands. It does not: all six stop inside the loop body, printing neither the echo after the kill nor anything after the loop",
	},
	{
		ID: "signal-death/a-command-substitution-that-signaled-the-shell", Category: "traps and exit",
		Snippet: `x=$(kill -TERM $$; echo inner); echo "outer x=$x"`,
		Why:     "the other subshell environment, and it does not divide the panel the way `( )` does: all six print nothing and die, because whatever the child wrote went into the assignment rather than to the output and the shell never reached the echo that would have shown it. Here so that the `( )` row is not read as a claim about every subshell environment",
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
		ID: "umask/a-created-file-takes-the-mask", Category: "traps and exit",
		Snippet: `umask 077; : > f; ls -l f | cut -c1-10; umask 022; : > g; ls -l g | cut -c1-10`,
		Why:     "every other umask case interrogates the builtin, and a mask nothing is created under is a number the shell is keeping rather than a mask. This is the one that opens a file on each side of a change and reads the mode back off the file system, unanimously — and it is the case the no-process-state rule makes worth having, since a core that may not call umask(2) has to carry the mask itself and apply it at every open. Two masks rather than one, because a shell that ignored the mask entirely would still pass with whatever the process started with",
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
		ID: "ulimit/setting-with-neither-letter-moves-both", Category: "traps and exit",
		Snippet: `ulimit -n 100; h=$(ulimit -H -n); s=$(ulimit -S -n); echo "s=$s hard_moved=$([ "$h" = 100 ] && echo yes || echo no)"`,
		Why:     "`ulimit -n 100` with neither -H nor -S sets *both* limits in five of the six and only the soft one in zsh — which matters because lowering both is a door that cannot be reopened, while lowering the soft limit alone can be undone. The hard limit is compared rather than printed: its starting value is a property of the machine, and a row that recorded it would record where it was generated",
	},
	{
		ID: "ulimit/the-file-size-block", Category: "traps and exit",
		Snippet: `{ ( ulimit -f 1; printf "%0600d" 0 > f ); } 2>/dev/null; ls -l f | awk "{print \"size=\" \$5}"`,
		Why:     "how many bytes a block is, asked of the file system rather than of the builtin: one block, six hundred bytes written, and the file is 600 where a block is 1024 and 512 where it is 512. The prior reading of this, taken from bash alone, was that a block is 1024 bytes — true for bash 5.3 and bash 3.2 as `bash`, and false for dash, ksh93, zsh *and the same bash 5.3 called `sh`*, all of which use POSIX's 512. So the unit is argv[0]'s to decide, which is not a shape a one-shell measurement could have found. Written in a subshell whose group carries the redirection, because the shell that reaps a child killed by SIGXFSZ announces it with a process id in the text",
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
		ID: "shopt/expand-aliases-is-a-live-switch", Category: "shell options",
		Script: true,
		Snippet: `shopt -s expand_aliases 2>/dev/null
alias a='echo hit'
a
shopt -u expand_aliases 2>/dev/null
a
echo "st=$?"`,
		Why: "the option a bash script has to set before an alias means anything, and it is a switch rather than a door: the word expands after -s and is a command not found again after -u. The three shells that always expand ignore the missing builtin and expand both times, which is what makes the pair a divergence rather than a bash detail",
	},
	{
		ID: "shopt/expand-aliases-is-off-until-it-is-asked-for", Category: "shell options",
		Script: true,
		Snippet: `alias a='echo hit'
a
echo "st=$?"`,
		Why: "the other half, and the reason the option exists: the identical script without the `shopt` line is a command not found in bash from a file, where dash, ksh93 and zsh all expand. Without both halves recorded, a shell that expanded unconditionally would pass the first",
	},
	{
		ID: "shopt/posix-mode-turns-alias-expansion-on", Category: "shell options",
		Script: true,
		Snippet: `set -o posix 2>/dev/null
alias a='echo hit'
a
echo "st=$?"`,
		Why: "the standard has aliases expand in a script, so the mode carries the option with it: bash expands here with no `shopt` written anywhere, which is also why the shell invoked as `sh` expands where the same binary called `bash` does not",
	},
	{
		ID: "shopt/leaving-posix-mode-drops-alias-expansion-again", Category: "shell options",
		Script: true,
		Snippet: `shopt -s expand_aliases 2>/dev/null
set -o posix 2>/dev/null
set +o posix 2>/dev/null
alias a='echo hit'
a
echo "st=$?"`,
		Why: "leaving the mode restores what the *route* said rather than what was set before entering it — measured, and the surprising half: the `shopt -s` on the first line does not survive the round trip, so the alias is a command not found",
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
		ID: "jobs/the-last-background-pid-before-any-job", Category: "builtins",
		Snippet: `echo "[$!]"`,
		Why:     "one line, no pid in it, and the whole `$!` grid's only machine-independent row (#937): what the parameter is before a background command has been started. Five columns write nothing and zsh writes `0`, a number nothing ever had — and zero is not the same answer as nothing here, because a background builtin has no process and a shell really can hold a recorded zero. Everything else about `$!` carries a pid and cannot be recorded",
	},
	{
		ID: "jobs/an-unstarted-last-background-pid-under-set-u", Category: "builtins",
		Snippet: `set -u; echo "[$!]"; echo "st=$?"`,
		Why:     "the bigger split on the same parameter, and the one scripts rely on: `set -u` exists to stop exactly this read. bash stops with `$!: unbound variable` at 127 — the `$` written back, as it is for a positional — dash stops with `!: parameter not set` at 2, and ksh93 and zsh carry on, ksh93 with nothing and zsh with its zero. Two against two, and neither answer predicts the row above: zsh's zero is a value and ksh93's empty is a set parameter, so the two quiet columns are quiet for different reasons",
	},
	{
		ID: "jobs/the-last-background-pid-under-set-u-once-a-job-has-run", Category: "builtins",
		Snippet: `set -u; sleep 0 & wait; x=$!; echo "st=$? set=[${x:+yes}]"`,
		Why:     "the control for the row above, and it is what says the refusal is about *nothing having been started* rather than about `$!`: with one job behind it the parameter is set in all six columns and `set -u` has nothing to say. Read into a variable and reported as a yes rather than printed, because the value is a pid and a pid is not the same twice",
	},
	{
		ID: "jobs/a-disowned-job-is-not-in-the-listing", Category: "builtins", SyntaxError: true,
		Snippet: `sleep 0.4 &| jobs >j.txt; echo "n=$(grep -c . j.txt)"`,
		Why:     "`&|` starts a job in the background and lets go of it, so nothing lists it — one shell's, and the only shape of it a record can hold. The sibling spelling `&!` cannot be recorded at all: bash 5.3 and ksh93 read those two characters as `&` and the `!` that negates a pipeline, and what they then do with a bare `!` is a divergence of its own (#948), so a row about the disowning would carry four unrelated ones beside it. `&|` has no such second reading in bash or dash, which both refuse it outright. ksh93's cell is the exception and is not this change's: it parses `&|` and means something else again — `echo hi &| echo done` prints only `done` there. Counted rather than printed, because what a listing looks like is four other rows' question",
	},
	{
		ID: "jobs/a-background-job-outlives-the-shell", Category: "builtins",
		Snippet: `(sleep 0.2; echo LATE-42) & echo NOW-42`,
		Why:     "the question nothing in this corpus asked before (#519): what happens to a `&` job *after* the shell ends. Every other job row waits for the job or inspects `jobs` first. All five keep it — a real shell forks, so the job is a process that outlives its parent by construction — and the order says the shell did not wait for it either. This shell prints only NOW-42: a background job here is a goroutine in this process, so the process ending takes the rest of the job with it. Recorded as a knowing difference rather than avoided",
	},
	{
		ID: "jobs/a-background-external-command-outlives-the-shell", Category: "builtins",
		Snippet: `sh -c 'sleep 0.2; echo LATE-42' & echo NOW-42`,
		Why:     "the same question where the whole job is one external command, and this shell answers it correctly — the child is a real process and outliving the shell is what a real process does. The pair with the row above is what says the difference is about the *shell logic* in a job and not about `&`: a subshell, an and-list, a brace group and a function all lose their remainder, and a single command loses nothing",
	},
	{
		ID: "jobs/an-ampersand-returns-before-the-job-opens-a-fifo", Category: "builtins",
		Snippet: `mkfifo p; (read x <p; echo "GOT-$x") & echo NOW-42; echo hi >p; wait`,
		Why:     "the question the three rows above it do not reach (#1003): what `&` does when the job's *first* act is to wait rather than to run. A named pipe with nobody at the other end is what makes the job block where it opens the redirection, before the shell knows whether the command is a builtin or a program — and every shell in the panel prints the marker at once, because a real shell forks before it opens anything. This shell printed nothing at all and timed out: starting a job waited for a process id that a job blocked before its first command was never going to have. The late write is what makes the case terminate instead of recording six timeouts, and the order is fixed by the pipe rather than by the scheduler — the job cannot get past its open until the write arrives, and the write does not happen until the marker is printed",
	},
	{
		ID: "jobs/an-ampersand-returns-when-an-external-commands-redirection-blocks", Category: "builtins",
		Snippet: `mkfifo p; /bin/cat <p >o.txt & echo NOW-42; echo hi >p; wait; cat o.txt`,
		Why:     "the same question where the job is one external command, and it is the row that says the fault was not about compound commands: `sh -c ... &` returns here because the child is a real process, but a program whose redirection blocks never becomes one, so the pid the shell was waiting for did not exist yet. The pair with the row above is what separates opening a redirection from dispatching a command — the open comes first in every shape",
	},
	{
		ID: "jobs/wait-brings-a-background-job-back-before-the-shell-ends", Category: "builtins",
		Snippet: `(sleep 0.2; echo LATE-42) & wait; echo NOW-42`,
		Why:     "the same job with a `wait` in front of the ending, which every shell including this one gets right and which is the workaround the row above leaves a script needing. It is the pair that says the loss is about the shell *ending*, not about the job: nothing is wrong with the job while there is still a shell to run it",
	},
	{
		ID: "jobs/a-running-background-job", Category: "builtins",
		Snippet: `sleep 0.4 & jobs`,
		Why:     "one line of a `jobs` listing, and four shells write it four ways — the marker spacing, the width of the state column, the case of the word, and whether the command is there at all. dash shows an empty column and ksh93 `<command unknown>`, because neither kept the text; bash puts the `&` back on",
	},
	{
		ID: "jobs/a-finished-background-job", Category: "builtins",
		Snippet: `m='s/^\(\[[0-9][0-9]*\]\) *[-+]* */\1 /'; sleep 0.05 & sleep 0.5; jobs >a.txt; sed -e "$m" a.txt; echo "---"; jobs >b.txt; sed -e "$m" b.txt`,
		Why:     "reported once and then forgotten, in every shell that reports it at all — the second listing is empty. zsh never mentions it and ksh93 still calls it Running, which is not a reaping race: it says so after `wait` too. The marker is normalized away by the same expression as the failed-job row below, and for the same reason: this job has finished too, so what the listing says about which job is current is not a fact about the job that finished",
	},
	{
		ID: "jobs/a-background-job-that-failed", Category: "builtins",
		Snippet: `false & sleep 0.3; jobs >j.txt; sed -e "s/^\(\[[0-9][0-9]*\]\) *[-+]* */\1 /" j.txt`,
		Why:     "the status reaches the listing, and the two shells that say so disagree about how: `Exit 1` against `Done(1)`. The current-job marker is normalized to one space rather than recorded, because it answers a different question and does not answer it the same way twice: bash spelled it `[1]+` in every one of 2,900 measured runs of this snippet and `[1] ` once in three runs of the record check, which is rare enough that a regeneration bakes in whichever it saw and every later check then fails for a reason unrelated to what changed. What the marker does mean is pinned by the row below, where the blank is reachable on purpose. Through a file rather than a pipe, because a subshell has no job table in dash or zsh and `jobs | sed` would empty two of the columns instead of normalizing them",
	},
	{
		ID: "jobs/a-job-that-is-neither-current-nor-previous", Category: "builtins",
		Snippet: `false & sleep 0.05 & wait %2; jobs >j.txt; sed -e "s/^\(\[[0-9][0-9]*\]\) *\([-+]\) */\1[\2] /;s/^\(\[[0-9][0-9]*\]\)  */\1[ ] /" j.txt`,
		Why:     "the marker belongs to the job table and not to the job. With a second job started and waited for, bash has nothing left to call current and writes a *blank* where the `+` would be — the other spelling the failed-job row above was seeing at random, here on purpose and the same 200 times out of 200. dash marks both of its rows `+` rather than keeping a previous job at all, and the three shells that print nothing have forgotten both. The marker is bracketed because a blank one is invisible beside a `+`, and a reader cannot check what they cannot see",
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
		ID: "jobs/slot-a-running-job-occupies-its-slot", Category: "builtins",
		Snippet: "sleep 5 & sleep 5 &\njobs %1 >/dev/null 2>&1; echo \"one=$?\"\njobs %2 >/dev/null 2>&1; echo \"two=$?\"\njobs %3 >/dev/null 2>&1; echo \"three=$?\"\nkill %1 2>/dev/null; kill %2 2>/dev/null; :",
		Why:     "the jobs table asked one slot at a time, through a status rather than a listing — which is the only way this is gradeable at all. A bare `jobs` prints a `Done` row for a reaped job on some runs of the same binary and not others, so a family of listing cases would be a family of rows nobody can use (#783). A status has no text to race: two slots are occupied and the third is not, and the number for *not there* is four different answers — bash 1, dash 2, ksh93 1, zsh 127 — which is the whole reason the probe needed the number to be right before it could grade anything. The jobs are five seconds long and killed at the end, so nothing here waits on a scheduler; the trailing `:` keeps the case about slots rather than about what `kill %n` reports, which dash alone answers 1",
	},
	{
		ID: "jobs/slot-a-status-query-does-not-consume-it", Category: "builtins",
		Snippet: "sleep 5 &\njobs %1 >/dev/null 2>&1; echo \"a=$?\"\njobs %1 >/dev/null 2>&1; echo \"b=$?\"\nkill %1 2>/dev/null; :",
		Why:     "the control that makes the row above a probe rather than a measurement of itself: asking about a slot twice gives the same answer twice in all six. It matters because a *listing* does consume what it reports — a finished job is reported once and then forgotten — so a reader could reasonably expect the query to be destructive too. It is not, for a job that is still running",
	},
	{
		ID: "jobs/slot-an-empty-table-has-no-first-slot", Category: "builtins",
		Snippet: `jobs %1 >/dev/null 2>&1; echo "one=$?"`,
		Why:     "the floor of the probe, and the cheapest reading of the four not-there statuses: no job has ever been started, so slot one is not there. Same four answers as the row above, with nothing else in the script that could have produced them",
	},
	{
		ID: "jobs/slot-the-complaint-about-one-that-is-not-there", Category: "builtins",
		Snippet: `jobs %9 2>&1 >/dev/null`,
		Why:     "the wording beside the status, and it is three different sentences: bash and zsh name the spec after the builtin, dash puts the sentence first and the spec after it, and ksh93 names no spec at all — `jobs: no such job`, which is the shell and not a truncation, because `%nope` produces the same line",
	},
	{
		ID: "jobs/slot-the-last-background-pid-outlives-a-bare-wait", Category: "builtins",
		Snippet: `sleep 0 & wait; case ${!:-} in "") echo "bang=empty";; *) echo "bang=set";; esac`,
		Why:     "`$!` is a value the shell keeps and not a job it is still holding: after a bare `wait` the job has ended, and every shell in the panel still reports its pid. Asked as a yes/no because the pid itself is different every run — the same reason the rest of this family asks for a status rather than for text. It is here because it was not true: `$!` read the current job, and the notice that reports a finished job drops that, so on the interactive route `$!` went empty the moment the `Done` row was printed",
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
		ID: "read/interrupted-by-a-trapped-signal", Category: "builtins",
		Snippet: `mkfifo p; exec 3<>p; trap "echo T" INT; (sleep 0.3; kill -INT $$; sleep 0.5; echo late >p) & read -r l <&3; echo "st=$? l=[$l]"; wait`,
		Why:     "a `read` waiting on a pipe when a trapped signal arrives, which is where the prior reading of `$?` after an interrupt — 130, measured against bash — turns out to be one of four answers. bash 5.3, bash 3.2 and zsh run the handler and *resume* the read, so the line that arrives afterwards is read and the status is 0; the same bash 5.3 called `sh` abandons it at 130; dash abandons it at 1; ksh93 answers 258. The late write is what makes the case terminate at all rather than recording three timeouts, and it is what makes the resuming shells observably different from a shell that merely returned 0",
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
		ID: "help/a-builtin-answers-the-help-option", Category: "builtins",
		Snippet: `h=$(alias --help 2>/dev/null); echo "st=$?"; printf '%s\n' "$h" | head -n 1`,
		Why:     "one shell answers `--help` and the rest refuse it as an option nobody has, and the answer goes to standard *output* while every refusal goes to standard error — which is the whole reason a script can tell the two apart. Written as a first line and a status rather than as the block itself: bash's answer carries a paragraph of its own documentation after the synopsis, and this tree reproduces the behavior rather than another project's prose (CLEANROOM.md). It matters far past its size — one platform ships fifteen /usr/bin commands as a stub around `builtin`, so `/usr/bin/alias --help` is this exact call, and every disagreement the real-script run sweep found was this (#825, #815)",
	},
	{
		ID: "help/the-help-option-comes-before-what-the-builtin-cannot-do", Category: "builtins",
		Snippet: `h=$(bg --help 2>/dev/null); echo "st=$?"; printf '%s\n' "$h" | head -n 1`,
		Why:     "asked with a builtin that cannot run at all here — there is no job control in a non-interactive shell — so the two orders give different answers: the shell that has the option answers it and exits 2 without ever looking for a job, and the ones that do not reach the complaint about job control and exit 1. A shell that checked its preconditions first would pass every other row of this family and fail this one",
	},
	{
		ID: "help/a-builtin-with-nothing-to-say-takes-it-as-a-word", Category: "builtins",
		Snippet: `true --help; echo "st=$?"; echo --help`,
		Why:     "unanimous, and the boundary of the rule above: the builtins with no options to speak of read `--help` as an ordinary operand, so `true` succeeds and `echo` prints it. The option belongs to the builtins that parse options, not to the shell",
	},
	{
		ID: "help/the-help-option-is-the-whole-word-and-stands-where-an-option-stands", Category: "builtins",
		Snippet: `alias --help=x; echo "st=$?"; alias -- --help; echo "st=$?"; h=$(hash -r --help 2>/dev/null); printf '%s\n' "$h" | head -n 1`,
		Why:     "two ways of writing something near it that are not it, and one that is: a word with anything after `--help` is a bad option, a `--help` past the `--` that ends the options is an operand — a *name*, which `alias` then cannot find — and a `--help` after another option letter is still the option, which is what puts the answer in the option reader rather than in a scan of the first word. One shell is the exception and matches an abbreviated option, which is why it answers the first of these with its whole option list",
	},
	{
		ID: "help/a-bad-option-is-followed-by-the-builtins-usage", Category: "builtins",
		Snippet: `alias -Q; echo "alias=$?"; ulimit -Q; echo "ulimit=$?"`,
		Why:     "the two shells that print a usage line after a bad option print one for *every* builtin, and these two reach it by different routes — `alias` through the shared option reader, `ulimit` through a refusal of its own, because its letters are resources rather than a fixed set. Both had the complaint and neither had the line under it (#825)",
	},
	{
		ID: "help/a-bad-option-to-umask-is-named-the-way-the-dialect-names-one", Category: "builtins",
		Snippet: `umask --version; echo "st=$?"; umask -Q; echo "st=$?"`,
		Why:     "which part of a `--` word a complaint names is the dialect's rule and not the builtin's: `--` in bash and dash, the whole word in ksh93, and the first letter past the dashes — `-v` — in zsh, the same three answers `export --version` gives. A builtin that spelled the word out itself was the one place that rule did not hold. `-Q` beside it is the single-dash half, where all four name the letter",
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
		ID: "read/a-timeout-that-expires", Category: "builtins",
		Snippet: `mkfifo p; exec 3<>p; l=keep; read -t 1 -r l <&3; echo "st=$? l=[$l]"`,
		Why:     "the deadline the two zero-timeout cases cannot reach: a pipe held open with nothing in it, so the read has to wait and then give up. The panel splits three ways rather than agreeing — bash 5.3 answers 142, which is 128 plus the alarm, and clears the variable; bash 3.2, ksh93 and zsh answer 1 and leave what was there; dash has no -t at all. A fifo opened read-write is what makes it a deadline instead of an end of input, since the harness closes standard input and any file is already at its end",
	},
	{
		ID: "read/a-descriptor-to-read-from", Category: "builtins",
		Snippet: `printf "hello\nworld\n" > f; exec 8< f; l=keep; read -u 8 -r l; echo "st=$? l=[$l]"; read -u 8 -r l; echo "st=$? l=[$l]"`,
		Why:     "-u reads from a descriptor the script opened rather than from standard input, and the second read is what says the descriptor keeps its position between calls rather than being reopened. Five accept it and dash calls the letter illegal, which is the same shape its -t and -n answers have",
	},
	{
		ID: "read/a-count-that-stops-at-the-delimiter", Category: "builtins",
		Snippet: `printf "ab\ncd" | { read -n 4 v; echo "st=$? [$v]"; }`,
		Why:     "-n is an *upper bound*, not a length: four characters were asked for and the line ended after two, so two arrive and the read still succeeds. `read/a-count-of-characters` asks for fewer characters than the line has; this is the other side, and it is what makes the -N case below a different question rather than a spelling of the same one",
	},
	{
		ID: "read/a-count-that-crosses-the-delimiter", Category: "builtins",
		Snippet: `printf "ab\ncd" | { read -N 4 v; echo "st=$? [$v]"; }`,
		Why:     "-N is the count that means it: the delimiter stops counting for -n and is just another character for -N, so bash 5.3 and ksh93 read `ab`, the newline, and the `c` after it into one variable. bash 3.2 has no such letter and answers with its usage, and zsh has neither -N nor a usage to print",
	},
	{
		ID: "read/an-initial-value-for-the-line", Category: "builtins",
		Snippet: `printf "x\n" | { l=keep; read -i pre -r l; echo "st=$? l=[$l]"; }`,
		Why:     "-i seeds the line editor and is therefore about a terminal, and this is what it does when there is not one: bash takes the option, ignores the seed, and reads the line — three others refuse the letter in three wordings, and only one of them refuses at 1. Worth pinning because the tempting reading of the manual is that the seed is a *default* for an empty line, and no shell here does that",
	},
	{
		ID: "read/an-initial-value-is-not-a-default", Category: "builtins",
		Snippet: `printf "\n" | { l=keep; read -i pre -r l; echo "st=$? l=[$l]"; }`,
		Why:     "the reading of the manual that is wrong: an empty line leaves the variable empty in the one shell that takes the letter, not holding the seed. The seed is what a line editor opens with and nothing else, so with no terminal it has no effect at all — this is the row that says so, where `read/an-initial-value-for-the-line` only says the option is taken",
	},
	{
		ID: "read/an-initial-value-with-nothing-after-it", Category: "builtins",
		Snippet: `printf "x\n" | { l=keep; read -i; echo "st=$? l=[$l]"; }`,
		Why:     "the letter takes an argument, which is the half that has to be right in a shell that means to ignore the value: bash asks for the argument it is missing where the four that do not have the letter refuse it as one they never heard of, in the same words they use with a value after it",
	},
	{
		ID: "read/a-prompt-is-for-a-terminal-and-not-a-character-device", Category: "builtins",
		Snippet: `read -p 'PROMPT-42 ' v < /dev/urandom; echo "st=$?"`,
		Why:     "a character device is not a terminal, and this is the device that says so rather than /dev/null: the read *succeeds* here, so there is every reason to have prompted and no shell prompts. Testing a character device instead of asking the kernel printed `PROMPT-42 ` into something nobody was reading (#525). ksh93 and zsh spell `-p` as the coprocess flag and refuse it, which is the same row saying that too",
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
		ID: "type/a-body-with-a-here-document-in-it", Category: "builtins",
		Script:  true,
		Snippet: "f(){\ncat <<E\nbody\nE\necho after\n}\ntype f\n",
		Why:     "the one shell that says a body back has to say a here-document back too, and the body's lines are not the shell's to arrange: they go out as they were read, between the operator's line and the delimiter's. What the arrangement does own is the seam — the `;` that would separate the next statement is dropped, because the delimiter's line has already ended, and a blank line is left where it was. A `;` written there is alone on a line of its own, which bash, dash and ksh93 all refuse to read back (#962). bash 3.2 writes a second space between the command and the operator where 5.3 writes one, which is the same tree said twice",
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
		ID: "export/an-exported-name-reaches-a-real-child", Category: "builtins",
		Snippet: `export E=2; X=1 env | grep -E "^(E|X)=" | sort`,
		Why:     "the export cases either side of it ask about a name the shell *inherited*; this asks about one the shell exported itself, and about a prefix over a name that was never in the environment at all. Both reach a real child's environment, unanimously. It is worth its own row because the two paths are separate in an implementation — an imported name arrives already in the table the child is built from, and an exported one has to be put there",
	},
	{
		ID: "export/a-prefix-over-an-imported-name", Category: "builtins",
		Snippet: `TERM=prefixed env | grep '^TERM='; env | grep '^TERM='`,
		Why:     "a prefix over a name the shell inherited: the command sees the prefix and the next command sees what came in, unanimously. `cmd/assignment-prefix-is-transient` asks this of a shell variable and reads it back in the shell; this one asks it of an inherited name and reads it back through a real environment, which is where a superseding entry would be one of two rather than instead of the other",
	},
	{
		ID: "setarray/the-array-assignment-through-a-held-name", Category: "builtins",
		Snippet: `h=hh; set -A $h m n o; echo "st=$? n=${#hh[@]} [${hh[@]}] first=[${hh[0]}]"`,
		Why:     "`set -A name value …` is the thing `name=(…)` cannot do — the name is a literal there, so a script holding it in a variable has no other spelling. ksh93 and zsh have the letter; bash calls it an invalid option and prints a usage block, dash calls it illegal and stops. So which shells have it is a dialect's answer rather than an axis. The `first=` field reads the base back and the two shells that have the letter disagree about it, which is `ArrayBaseIsZero` seen through a new builtin rather than a new question: it goes through the same whole-array store `name=(…)` reaches, so the base is answered once (#1156)",
	},
	{
		ID: "setarray/the-plus-form-replaces-from-the-front", Category: "builtins",
		Script:  true,
		Snippet: "set -A b 1 2 3 4 5\nset +A b Q R\necho \"1 n=${#b[@]} [${b[@]}]\"\nset -A c 1 2 3\nset +A c\necho \"2 n=${#c[@]} [${c[@]}]\"\n",
		Why:     "the plus form replaces from the *front* and leaves the rest of the array standing, which is a different operation from the minus form rather than the same one — and it is the half a single implementation gets wrong, since `set -A` replaces the whole array. Both shells with the letter agree on both lines, including that a plus form with nothing to put at the front leaves the array exactly as it was — which is *not* the minus form's answer to the same emptiness, and is why the two emptinesses are not one question",
	},
	{
		ID: "setarray/no-values-and-whether-the-name-survives", Category: "builtins",
		Script:  true,
		Snippet: "set -A a 1 2 3\nset -A a\necho \"n=${#a[@]} set=[${a+x}]\"\ntypeset -p a\n",
		Why:     "`set -A name` with no values at all: ksh93 **unsets** the name and zsh leaves an array with no elements. Both count 0, so the difference reaches a script only through `${a+x}` and a listing — which is exactly what makes it a field and not a rule, because a script asking whether the name is set gets opposite answers. The `typeset -p` line is the same finding from the other side: nothing at all against `typeset -a a=(  )`",
	},
	{
		ID: "setoption/every-bad-option-word-is-reported", Category: "builtins",
		Script:  true,
		Snippet: "set -q -z\necho \"after st=$?\"\n",
		Why:     "how many bad option words `set` reports before it gives up, which one column answers differently from the other four — and answers differently for a reason that is not the fatality. Three of the five print one line because the refusal is fatal there and the loop never reaches the second word, and the fourth prints one because it stops too; ksh93's refusal is fatal as well and it still prints a line for each, then a single usage block after both. That makes it a rule of its own rather than a consequence, and it is the same rule `Runner.builtinNames` already follows for bad *operands* — where bash is the shell that reports each one, which is what keeps the two questions separate fields (#1170)",
	},
	{
		ID: "setoption/two-bad-letters-in-one-bundle", Category: "builtins",
		Script:  true,
		Snippet: "set -qz\necho \"after st=$?\"\n",
		Why:     "the same two letters written as one word, which was the open question beside the row above: a bundle might have been one refusal in every column. It is not — the shell that reports each word reports each *letter*, so the unit is the letter and the word boundary does not enter into it. The four that stop at the first are unmoved, which is what says the pair measures the axis and not the spelling",
	},
	{
		ID: "setoption/a-bad-name-between-two-bad-letters", Category: "builtins",
		Script:  true,
		Snippet: "set -q -o nosuchoption -z\necho \"after st=$?\"\n",
		Why:     "the long spelling and the short one interleaved, which says one loop reports them in the order they were written rather than two passes over the words. It also carries the usage block's own question: the shell that reports every word prints exactly one block, after all three sentences, where a fix that only kept the loop going would print one per word",
	},
	{
		ID: "setarray/whether-the-options-carry-on-past-the-name", Category: "builtins",
		Script:  true,
		Snippet: "set -A dd -- 1 2\necho \"1 n=${#dd[@]} [${dd[@]}]\"\nset -A ff -q -z\necho \"2 st=$? n=${#ff[@]} [${ff[@]}]\"\n",
		Why:     "whether the words behind `set -A name` are more options or the array's values, and the two shells that have the letter answer opposite ways. ksh93 keeps parsing, so a `--` still ends the options and the values are exactly the words that would have become the positional parameters; zsh stops at the name and every word behind it is a value, `--` and dash words included. **One question and not two** — whether `--` is an operand *is* whether options are still being read, so both lines move together. The second line uses letters neither shell implements, so the shell that carries on refuses one and the shell that does not stores both",
	},
	{
		ID: "setarray/a-name-that-is-not-a-name", Category: "builtins",
		Snippet: `set -A 1bad v; echo "st=$? after"`,
		Why:     "the name operand goes through the same check every other builtin's operands go through, fatality included: both shells that have the letter end the script there, and word it their own way — `set: 1bad: invalid variable name` against `not an identifier: 1bad`. The location is the finding: zsh does *not* name the builtin here where `unset 1x` and `typeset 1w` from the same shell are `<script>:unset:1:` and `<script>:typeset:1:`, so the sentence and the location are decided separately. This row is the `-c` route, and its *status* is the second finding — zsh leaves 0 behind here where the script-file row below leaves 1 (#1172)",
	},
	{
		ID: "setarray/a-name-that-is-not-a-name-in-a-script", Category: "builtins",
		Script:  true,
		Snippet: "set -A 1bad v\necho \"st=$? after\"\n",
		Why:     "the same refusal from a script file, and the pair is what makes the status a measurement rather than an assumption: the stderr is byte-identical on both routes and `after` runs on neither, so the refusal is fatal either way and the only thing that moves is the number the shell leaves behind — 1 here against 0 from `-c` in zsh, and 1 on both in ksh93. A single row could not have seen it, and the neighboring refusals were all measured to check the rule is this one's and not the letter's or the route's (#1172)",
	},
	{
		ID: "setarray/over-a-frozen-name", Category: "builtins",
		Script:  true,
		Snippet: "typeset -r ro=1\nset -A ro q\necho after\n",
		Why:     "the readonly refusal reaches this store too, and it has to stand in *front* of it: the whole-array store keeps the scalar view in step and that call is guarded, so a check made afterwards prints its complaint with the array already written. ksh93 names the builtin and keeps its bracketed builtin location — `<script>[2]: set: ro: is read only` — where a plain `ro=(x y)` in the same shell is `<script>: line 2: ro: is read only`, so one table decides the sentence and the location together",
	},
	{
		ID: "setarray/the-line-a-hook-installer-assigns-with", Category: "builtins",
		Script:  true,
		Snippet: "hook=precmd_functions\ntypeset -ga $hook\nfn=my_hook\nset -A $hook ${(P)hook} $fn\necho \"n=${#precmd_functions[@]} [${precmd_functions[@]}]\"\n",
		Why:     "the line that put the letter on the release bar: zsh's own `add-zsh-hook` installs a hook exactly this way, and it is the whole reason a name held in a variable has to be assignable. The `${(P)hook}` is zsh's own indirection and reaches only that shell, so this row records what each of the others makes of the line rather than claiming they all run it",
	},
	{
		ID: "array/a-subscript-past-the-end", Category: "expansion",
		Snippet: `a=(x); a[5]=y; echo "n=${#a[@]} all=[${a[@]}]"`,
		Why:     "whether an unassigned subscript is an element. Two shells say an array is a map from subscript to value and this is an array of two; one walks the whole extent and finds the gap empty, giving five. Counting the gap as elements is what a list representation does, and it is nobody's answer",
	},
	{
		ID: "array/a-bare-name-is-the-element-at-the-base", Category: "expansion",
		Snippet: `a[5]=q; echo "bare=[${a-U}] set=[${a+S}] whole=[${a[@]+S}]"`,
		Why:     "what a bare array name reads is the element at the *base* position and not the lowest subscript that happens to be assigned, so a name with nothing written where a bare name looks is unset while the array itself is set. Subscript 5 is the base in no shell with arrays, which is what keeps this a statement about the position rather than about the array base — bash and ksh93 answer `U` with `${a[@]+S}` still set, and zsh reads the list and is set either way. We answered `q`, the element nobody asked for, at every one of the three reads",
	},
	{
		ID: "array/a-bare-name-after-the-base-element-is-removed", Category: "expansion",
		Snippet: `a=(x y z); unset "a[0]"; echo "bare=[${a-U}] len=${#a} quoted=[$a] at=[${a[@]}]"`,
		Why:     "the four reads that go through one view, in one line: the two shells whose first element is 0 take the base element away and every read of the bare name then finds nothing — `U`, length 0, an empty quoted field — while `${a[@]}` still yields the two elements left. The removal itself was already right and the view behind it was not, so a case counting elements would have passed. zsh's cell is the refusal `array/a-subscript-below-the-first-element` records, since 0 is below its first element",
	},
	{
		ID: "array/a-bare-name-in-arithmetic-after-the-base-element-is-removed", Category: "expansion",
		Snippet: `a=(1 2 3); unset "a[0]"; echo "arith=$((a+5))"`,
		Why:     "an arithmetic reference reads the same view, where an unset name is 0 — so the wrong reading is a wrong *number* rather than a wrong string. `5` in bash and ksh93 against the `7` reading the surviving element gave: a value that is plausible, silent and off by the element that was removed",
	},
	{
		ID: "nounset/a-bare-array-name-whose-base-element-is-gone", Category: "shell options",
		Snippet: `a=(x y z); unset "a[0]"; set -u; echo "[$a]"`,
		Why:     "the loudest form of the same reading, and the one that fails in the opposite direction: with the base element gone the bare name is not set, so `set -u` refuses it and the shell gives up — 127 in bash through `-c` and 1 in ksh93, the statuses `nounset/unset-variable-from-a-command-string` already pins. ksh93's sentence names `a[0]` rather than `a`, which says out loud which element a bare name is. We printed a value and reported 0",
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
		ID: "param/a-substring-modifier-that-needs-the-working-directory", Category: "parameter expansion",
		Snippet: `x=/x/y/../z; a=1; echo "[${x:a}]"`,
		Why:     "`:a` makes a path absolute lexically — `..` and `.` canceled by name, no link followed — where the other three read `a` as an offset of 1 and take the substring. An absolute value on purpose, so the answer needs no working directory and no machine agrees or disagrees by accident",
	},
	{
		ID: "param/a-substring-modifier-that-needs-the-command-search", Category: "parameter expansion",
		Snippet: `x=/tmp/Dir/File; c=5; echo "[${x:c}]"`,
		Why:     "`:c` is the path the command search would find, and a name holding a slash is left exactly as written — so the value comes back unchanged, where the other three take the substring from offset 5. The unchanged answer is the point: it is what a modifier does with a name that is not a bare command, and it is why the column that looks like a no-op is not one",
	},
	{
		ID: "param/a-substring-modifier-that-needs-the-quoting-table", Category: "parameter expansion",
		Snippet: `x="a b*c"; q=2; echo "[${x:q}]"`,
		Why:     "`:q` quotes the value in that shell's own quoting — `a\\ b\\*c` — where the other three take the substring from offset 2. The same table `${(q)x}` uses, with one measured difference the flag does not share: an empty value is empty here and `''` there",
	},
	{
		ID: "param/a-substring-modifier-that-substitutes", Category: "parameter expansion",
		Snippet: `x=aXbXc; echo "[${x:s/X/-/}]"; echo "st=$?"`,
		Why:     "`:s` replaces a **literal** substring, not a pattern — measured: `${x:s/?/Z/}` on `abc` answers `abc`, and the `?` is replaced only where the value really holds one. The other three read `s/X/-/` as arithmetic and refuse it, which is why the status is the second line",
	},
	{
		ID: "param/a-substring-modifier-after-an-offset-and-a-length", Category: "parameter expansion",
		Snippet: `x=/tmp/Dir/File.Txt.gz; t=0; echo "[${x:1:5:t}]"; echo "st=$?"`,
		Why:     "a modifier after **both** an offset and a length, which is a third shape rather than a variation of either: the tail of the five characters from offset one. The parser splits a range once, so `5:t` arrives whole — it reached the evaluator as an expression here and was an arithmetic failure over a range the shell with modifiers reads without complaint",
	},
	{
		ID: "param/a-substring-modifier-with-a-count", Category: "parameter expansion",
		Snippet: `x=/a/b/c/d/e; h=0; echo "[${x:h1}][${x:h3}]"`,
		Why:     "a digit after `h` counts **separators from the left**, not repetitions of the modifier: `:h1` is `/` where the head once is `/a/b/c/d` and three times over is `/a/b`. Both are here because `:h3` alone is `/a/b` under either reading, which is the coincidence that let the repetition reading survive being written down. The other three read `h1` as a name and take the substring from offset 0",
	},
	{
		ID: "param/a-substring-offset-on-a-subscripted-parameter", Category: "parameter expansion",
		Snippet: `a=(p q r); echo "[${a[@]:1+}]"; echo after`,
		Why:     "the parameter a diagnostic names is the name and its subscript, not the name alone — `a[@]` — in the one shell that names it at all. The list form of the substring reaches the same evaluation as the string form, so this also says the two spellings share it",
	},
	{
		ID: "array/a-subscript-without-braces", Category: "expansion",
		Snippet: `a=(x y z); echo "[$a[1]]"`,
		Why:     "the same element by the spelling that has no braces, and it is a grammar split rather than a value: `$a[1]` is one subscripted expansion in zsh and the parameter `$a` followed by the three characters `[1]` in bash 3.2, bash 5.3 and ksh93 alike, so the panel cuts the word in two different places. Written inside quotes so the difference is the parse and not a glob — the unquoted form is `array/a-subscript-without-braces-is-a-pattern-elsewhere`",
	},
	{
		ID: "array/a-subscript-without-braces-is-a-pattern-elsewhere", Category: "expansion",
		Snippet: `a=(x y z); echo [$a[1]]`,
		Why:     "the same word unquoted, which is where the split stops being quiet: to zsh the whole word is `[x]` and a pattern that matches no file, so it refuses the command outright, while the other three leave `[x[1]]` standing as an unmatched pattern and print it. One shell reports an error and the rest print a wrong answer, from the same nine characters",
	},
	{
		ID: "array/a-subscript-without-braces-is-not-a-positional", Category: "expansion",
		Snippet: `set -- abcd; echo "[$1[2]]"`,
		Why:     "the parameters that take a bare subscript are not simply all of them: zsh subscripts a name and a scalar alike but reads `$1[2]` as the positional and then two literal characters, so this is unanimous across the panel — dash included, which has no arrays at all. It is the row that keeps a dialect from granting the form to every `$`",
	},
	{
		ID: "array/a-subscript-without-braces-is-read-once", Category: "expansion",
		Snippet: `a=(x y z); echo "[$a[1][1]]"`,
		Why:     "one subscript and no more: zsh reads `[1]` and leaves the second bracket group as text, so the answer is `x[1]` rather than a character of `x`. The braced form has the same rule and no way to show it, since `${a[1][1]}` is a bad substitution",
	},
	{
		ID: "array/a-length-without-braces", Category: "expansion",
		Snippet: `a=(x y z); echo "[$#a]"`,
		Why:     "`$#a` is the array's count in zsh and `$#` followed by the letter `a` everywhere else, which is the same grammar split read through the other operator — and the more dangerous half, because both readings produce a number and neither shell says anything. A script testing `$#a` against zero is testing the count in one shell and the string `0a` in the other",
	},
	{
		ID: "array/a-length-without-braces-stops-at-two-specials", Category: "expansion",
		Snippet: `set -- p q; a=(x y z); echo "[$##][$#@][$#a]"`,
		Why:     "how far the no-brace length form reaches, measured at the two edges at once: zsh takes a name and takes `@`, and does *not* take `#` — `$##` is the positional count and then a literal `#` there exactly as it is in bash. So the parameter after the `#` is drawn from a set with holes in it rather than from every parameter, and a dialect that read one more character than the shell does would differ only on the shapes nothing tests",
	},
	{
		ID: "array/a-subscript-takes-its-own-flag-group", Category: "expansion",
		Snippet: `a=(alpha beta gamma); printf "[%s]" "${a[(r)beta]}"; echo`,
		Why:     "a parenthesized flag group inside the brackets makes the subscript a search: zsh answers with the first element the operand matches, and the other four read the same characters as arithmetic and fail there — bash 5.3 and ksh93 naming the token, bash 3.2 with its older wording, and dash at the array literal it has no grammar for. `~/.zi/bin/zi.zsh` has sixty-four of them and six stand between the plugin manager and its first definition",
	},
	{
		ID: "array/a-subscript-flag-group-searches-both-ways", Category: "expansion",
		Snippet: `a=(alpha beta gamma beta delta); printf "[%s]" "${a[(r)*a]}" "${a[(R)*a]}" "${a[(i)be*]}" "${a[(I)be*]}" "${a[(i)zz]}" "${a[(I)zz]}"; echo`,
		Why:     "the four selecting flags in one row, with their two no-match answers: `r` and `R` are the first and last matching element, `i` and `I` their indices, and a search that matches nothing is one *past* the last element for `i` and one *before* the first for `I` — which is what makes `a[(i)new]=v` an append. A row that pinned only the hits would pass with the misses answering an empty string",
	},
	{
		ID: "array/a-subscript-flag-group-exact-matching", Category: "expansion",
		Snippet: `a=(alpha beta gamma); printf "[%s]" "${a[(r)be*]}" "${a[(re)be*]}" "${a[(re)beta]}"; echo`,
		Why:     "`(e)` is what turns the operand from a pattern into a string, and `(re)` — the combination every real script writes — is \"the first element *equal* to this\". The three together are the whole difference: without the `e` the pattern matches, with it the same text matches nothing, and the literal spelling still finds its element",
	},
	{
		ID: "array/a-subscript-flag-group-unknown-flag-is-arithmetic", Category: "expansion",
		Snippet: `a=(alpha beta gamma); printf "[%s]" "${a[(z)2]}"; echo`,
		Why:     "the row that makes this an *additive* grammar flag rather than a semantics axis: a group the shell cannot read is no group at all, so the subscript stands as written and is read as arithmetic — and the shell that has the construct fails here exactly as the four without it do. There is no text the flag gives a second meaning to",
	},
	// The two rows below pin what a *failed* subscript costs the file it is
	// in, which is the other half of the abandonment the `dot/` rows measure:
	// there the question is how far an error reaches, here it is whether
	// there is an error at all. They are on separate lines from the marker
	// after them on purpose — a one-line snippet cannot tell a shell that
	// gave up the rest of the file from one that printed nothing.
	{
		ID: "array/a-subscript-flag-group-unreadable-ends-the-file", Category: "expansion",
		Snippet: "a=(alpha beta gamma)\nprintf \"[%s]\" \"${a[(z)2]}\"\necho NEXT-RAN",
		Why:     "a group the shell cannot read leaves arithmetic that will not parse, and that is fatal to the file in every shell that got as far as reading it: the marker on the next line runs nowhere, so being more permissive here would show up as an extra line rather than only as a missing diagnostic",
	},
	{
		ID: "array/a-subscript-range-with-four-parts-ends-the-file", Category: "expansion",
		Snippet: "a=(alpha beta gamma)\nprintf \"[%s]\" \"${a[1,2,3,4]}\"\necho NEXT-RAN",
		Why:     "the shell with array *slicing* is the one that refuses a subscript with more commas than a slice has parts, and refusing it costs the file; the shells that read the same text as arithmetic with comma operators take the last value, subscript with it, print nothing and carry on — so the row records a refusal and a silent success side by side",
	},
	{
		ID: "array/a-subscript-flag-group-operand-is-text-as-written", Category: "expansion",
		Snippet: `b=('"beta"' beta); printf "[%s]" "${b[(r)be*]}" "${b[(r)"beta"]}"; echo`,
		Why:     "a subscript is not a quoting context: the first element's value is the six characters `\"beta\"`, and the quoted operand finds *it* rather than the four-character one — the quotes were matched, not removed. The unquoted half is on the same row because it is the same rule read the other way: nothing escaped the `*`, so it is a live pattern, which is why a substituted value's metacharacters are live here while the same shell's `${(@)a:#$g}` leaves them alone",
	},
	{
		ID: "array/a-subscript-flag-group-that-selects-nothing", Category: "expansion",
		Snippet: `a=(alpha beta gamma); printf "[%s]" "${a[()2]}" "${a[(e)2]}" "${a[(e)beta]}"; echo`,
		Why:     "a group with none of the four selecting flags in it leaves the subscript read exactly as it would have been without a group: an empty group and a bare `(e)` are both the second element, and `(e)beta` is *empty* rather than the element called beta, because the subscript behind the group is still arithmetic and an unset name is zero there",
	},
	{
		ID: "array/a-subscript-flag-group-counts-matches-and-moves-the-start", Category: "expansion",
		Snippet: `a=(alpha beta gamma beta delta); printf "[%s]" "${a[(rn:2:)*a]}" "${a[(rb:3:)*a]}" "${a[(Rb:3:)*a]}" "${a[(ib:6:)*a]}"; echo`,
		Why:     "`n` asks for the nth match rather than the first and `b` moves where the search starts, forwards for `r` and backwards for `R`. The last of the four is the one worth a row of its own: a start past the end is *not* clamped to the end, so a reverse search from 6 over five elements finds nothing rather than finding the fifth",
	},
	{
		ID: "array/a-subscript-pair-is-a-range", Category: "expansion",
		Snippet: `a=(w x y z); echo "[${a[1,3]}]"`,
		Why:     "the comma in a subscript, which is two readings of one spelling: zsh separates a range and gives `w x y`, while bash 5.3, bash 3.2, bash as sh and ksh93 read the whole text as arithmetic, take the comma operator's right operand and name element 3 alone. Both answer and neither reports, so a script cannot tell which shell it is on except by the value — the definition of a semantics axis rather than a construct one grammar has",
	},
	{
		ID: "array/a-subscript-pair-counts-back-from-the-end", Category: "expansion",
		Snippet: `a=(w x y z); echo "[${a[2,-1]}][${a[-3,-2]}]"`,
		Why:     "both endpoints take the negative reading a single subscript takes, so a range to the end needs no length. The shells that read the comma as an operator give the last operand's element for the first — nothing, since -1 is the last element — which is what makes this row separate from the plain range: it pins the endpoint rule rather than the comma",
	},
	{
		ID: "array/a-subscript-pair-past-either-end", Category: "expansion",
		Snippet: `a=(w x y z); echo "[${a[0,99]}][${a[3,2]}][${a[-5,2]}]"`,
		Why:     "the three edges of the range, measured together because a fix is most likely to impose a symmetry on them that the shell does not have: an end past the last is the last, a range that runs backwards is empty, and a start counted back past the *first* leaves an array empty rather than clamping. The last of those is the asymmetry — the same reach past the start of a string is clamped, which `array/a-string-subscript-pair` records",
	},
	{
		ID: "array/a-subscript-pair-is-counted-as-elements", Category: "expansion",
		Snippet: `a=(aa bb ccc); echo "[${#a[1,2]}]"`,
		Why:     "whether `${#…}` over a range is a count or a width. The element the arithmetic reading lands on is three characters long and the range holds two, so the two answers are 3 and 2 rather than the same number twice — which is what an array of equal-length elements would have hidden",
	},
	{
		ID: "array/a-subscript-on-a-string", Category: "expansion",
		Snippet: `s=hello; echo "[${s[0]}][${s[1]}][${s[2]}][${s[-1]}]"`,
		Why:     "a subscript on a plain string: zsh reaches into its characters, and bash and ksh93 read the scalar as an array of one — so the same four subscripts give `h` and `e` and `o` on one side and the whole string at 0 and nothing elsewhere on the other. The empty-or-error question this settles is the core one: neither bash nor ksh93 says anything about `${s[2]}`, so a shell without the character reading answers empty at status 0 rather than reporting",
	},
	{
		ID: "array/a-string-subscript-pair", Category: "expansion",
		Snippet: `s=hello; echo "[${s[2,4]}][${s[-6,2]}]"`,
		Why:     "the two readings of a subscript meeting on one string: a range of characters is a substring, and a start counted back past the first character is *clamped* here where the same reach on an array is empty. The asymmetry is measured rather than derived, and it is the half a shared range helper would flatten",
	},
	{
		ID: "array/a-string-subscript-in-a-single-byte-locale", Category: "expansion",
		Snippet: `s=héllo; echo "[${s[2,3]}][${s[4]}]"`,
		Why:     "the character reading of a subscript is the *locale's* character: under the fixed locale zsh's `${s[2,3]}` is two bytes that happen to spell one character and `${s[4]}` is the byte after them. Reading runes unconditionally, which is what #854 shipped, answered `él` and `o` here — right for a UTF-8 locale and wrong for this one",
	},
	{
		ID:       "array/a-string-subscript-in-a-utf8-locale",
		Category: "expansion",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `s=héllo; echo "[${s[2,3]}][${s[4]}]"`,
		Why:      "the same subscripts with the codeset changed: `él` and `l`, two characters and the one after them. Paired with the case above so that `${s[N]}` and `${#s}` are held to the same reading of what a character is — they were two answers in one shell before #899",
	},
	{
		ID:       "array/a-string-subscript-of-an-invalid-byte",
		Category: "expansion",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `s=$(printf 'a\200b'); b=$(printf '\200'); [ "${s[2]}" = "$b" ] && echo same || echo differs`,
		Why:      "the undecodable byte reached by a subscript rather than by a length, compared against itself rather than printed, so the record stays text. zsh answers with the byte; the shells with no character reading answer with nothing and so say `differs`, which is the same statement about them the plain string case makes",
	},
	{
		ID: "array/a-subscript-on-the-positional-list", Category: "expansion",
		Snippet: `set -- a b c; echo "[${@[1]}][${*[2]}]"`,
		Why:     "whether the positional parameters are a list a subscript reaches into. zsh says yes; every other member of the panel refuses the expansion outright — bash calls it a bad substitution when it is reached, ksh93 refuses the bracket while reading, dash says `Bad substitution`. Five refusals and one reading, with nobody meaning something else by it, which is what makes it a grammar flag rather than an axis. Expanding it to nothing at status 0 was the silent middle answer nobody gives",
	},
	{
		ID: "array/a-subscript-on-a-positional-parameter", Category: "expansion",
		Snippet: `set -- abcd; echo "[${1[2]}]"`,
		Why:     "the same grammar flag reaching a parameter that holds a *value* rather than a list, which is the half that shows the flag is about the name and not about `@`: the character reading answers it, so `${1[2]}` is `b` where the flag is on and a bad substitution everywhere else. `array/a-subscript-without-braces-is-not-a-positional` is the neighboring row and says the opposite about the *brace-less* spelling, which is a difference the two rows exist to hold apart",
	},
	{
		ID: "array/a-quoted-subscript-pair-joins", Category: "expansion",
		Snippet: `a=(x y z); set -- "${a[1,2]}"; echo "n=$# [$1]"; set -- "${a[@]}"; echo "n=$# [$1]"`,
		Why:     "how many fields a quoted range makes, and what is in the first of them. One field with the elements joined, exactly as a quoted plain `$a` is in the shell that reads a bare array name as the whole array — and the count alone does not say so, because the arithmetic reading names one element and that is one field too. The `[@]` half beside it is one field each, which is the difference the case is really about",
	},
	{
		ID: "array/a-quoted-pair-of-parameters-keeps-its-fields", Category: "expansion",
		Snippet: `set -- a b c; set -- "${@[1,3]}"; echo "n=$#"; set -- a b c; set -- "${*[1,3]}"; echo "n=$#"`,
		Why:     "and the name is what decides it: `@` keeps its fields however it is subscripted, `*` joins. The same difference `\"$@\"` has from `\"$*\"`, reached through a subscript rather than through the bare spelling — which is the rule a range has to follow rather than a rule of its own",
	},
	{
		ID: "array/a-subscript-pair-with-three-parts", Category: "expansion",
		Snippet: `a=(w x y z); echo "[${a[1,2,3]}]"; echo after`,
		Why:     "a range has two ends and a third is not a wider one. The shell with ranges calls it a bad substitution and gives up on the command; the shells reading arithmetic take the comma operator's last operand and name an element. Answering the second in a dialect that has ranges would be the other reading wearing this one's name, and it is exactly the shape a fix reaches for when it splits on the first comma and ignores the rest",
	},
	{
		ID: "param/the-ordering-flags-sort-a-list", Category: "parameter expansion",
		Snippet: `a=(10 9 1); printf "[%s]" "${(@o)a}"; printf "[%s]" "${(@O)a}"; printf "[%s]" "${(@n)a}"; printf "[%s]" "${(@nO)a}"; echo`,
		Why:     "`o` and `O` sort a list up and down and `n` reads the words as numbers, and the data is `10 9 1` rather than three letters because that is the only kind that tells the two sorts apart: lexically 10 comes before 9. All four on one row, since the flags compose and a reading that got `O` as `reverse the input` rather than `reverse the order` passes the first two and fails the fourth",
	},
	{
		ID: "param/the-ordering-flags-and-case", Category: "parameter expansion",
		Snippet: `a=(B a C b); printf "[%s]" "${(@o)a}"; printf "[%s]" "${(@oi)a}"; printf "[%s]" "${(@Oi)a}"; printf "[%s]" "${(@i)a}"; echo`,
		Why:     "the sort is byte order under this corpus's locale, so the cases separate — and `i` folds them, which makes ties reachable for the first time. On these four the tie keeps the order the elements were written in; that is not a rule either shell states, and the same shape at sixteen elements comes back with some of the pairs reversed, so the row pins the size rather than the principle. `(i)` alone sorts, which is what says it is not merely a modifier of `o`. The locale is the reason this row is worth pinning rather than reasoning about: outside `LC_ALL=C` the same shell orders by the collation instead and answers `a b B C` to the first",
	},
	{
		ID: "param/the-unique-flag-is-not-a-sort", Category: "parameter expansion",
		Snippet: `a=(b a b c a); printf "[%s]" "${(@u)a}"; printf "[%s]" "${(@ou)a}"; printf "[%s]" "${(@uO)a}"; echo`,
		Why:     "`u` keeps the first of each repeat and leaves the order alone, which is the row that fixes where it runs: with a sort beside it either order of the two gives the same answer, and only `u` on its own says which. It does not fold case either, so `(a A a)` keeps two",
	},
	{
		ID: "param/where-the-ordering-step-sits", Category: "parameter expansion",
		Snippet: `a=(zb ya); printf "[%s]" "${(@o)a#z}"; a=(B a); printf "[%s]" "${(@oU)a}"; a=(c a b); printf "[%s]" "${(oj.-.)a}"; printf "[%s]" "${(o)a}"; echo`,
		Why:     "where the step sits, in four answers that would each be different if it sat anywhere else: the operator has already run, so trimming `z` off `zb` puts it first; the case conversion has already run, so `(B a)` uppercased sorts as `A B` and not `B A`; a forced join has already made one word, which is in order however it was written; and the quoted join without `(@)` does the same. None of it is what the rule numbers suggest by name",
	},
	{
		ID: "param/the-index-is-an-ordering-key", Category: "parameter expansion",
		Snippet: `a=(c a b); printf "[%s]" "${(@a)a}"; printf "[%s]" "${(@aO)a}"; printf "[%s]" "${(@oa)a}"; echo`,
		Why:     "`a` is the sort *key* and not a sort: the element's own position, so ascending is the order it was written in and `O` reverses it. The third answer is the row that earns the case — `(oa)` on this data is `c a b`, so the index wins over the lexical comparison rather than joining it, and a reading where the two composed would give `a b c` there and pass the first two",
	},
	{
		ID: "param/the-length-flags-count-characters-and-words", Category: "parameter expansion",
		Snippet: `a=(abc de f); printf "[%s]" "${#a}" "${(c)#a}" "${(w)#a}"; v="a b  c"; printf "[%s]" "${(w)#v}" "${(W)#v}"; echo`,
		Why:     "three flags that change what a length counts, against the plain answer beside them: the elements are 3, their characters *joined* are 8 rather than 6 because the separators count, and the words in each element added up are 3 again. The scalar separates the last two — `w` is 3 words where `W` is 4 fields, the extra one being the empty field between the two spaces — which is the only shape that tells them apart",
	},
	{
		ID: "param/a-length-is-taken-before-the-joining-and-the-split", Category: "parameter expansion",
		Snippet: `a=(abc de f); printf "[%s]" "${(U)#a}" "${(Uj.-.)#a}"; v=":a:"; printf "[%s]" "${(s.:.)#v}" "${(ws.:.)#v}" "${(Ws.:.)#v}"; echo`,
		Why:     "where the length step sits, which is earlier than the rule numbers put it: a quoted list still counts its elements, so this is 3 and not the 8 characters of the joined text, and a `j` separator changes the join without reaching the count. The scalar says the same about splitting — `${(s.:.)#v}` is the three characters of the value and not the fields the separator would have made — and the two rows after it are the same value asked as words, where the leading empty field is dropped and the trailing one is not",
	},
	{
		ID: "param/the-unquoting-flag-removes-one-level", Category: "parameter expansion",
		Snippet: `x=hi; v='"$x" c'; printf "[%s]" "${(@Q)v}"; v="'unterm"; printf "[%s]" "${(Q)v}"; v='$(echo hi)'; printf "[%s]" "${(Q)v}"; echo`,
		Why:     "quoting removed and nothing expanded, which is the whole flag and is what the first answer pins: `x` is set, so a reading that handed the value to the parser would substitute `hi` where this leaves the two characters `$x` — and it stays one field, the space inside the quotes not making two. An opener with no closer comes back as written rather than as an error or a truncation, and a command substitution stays ten characters of text",
	},
	{
		ID: "param/where-the-unquoting-and-ordering-steps-sit", Category: "parameter expansion",
		Snippet: `v="'a b'"; printf "[%s]" "${(Qq)v}"; a=("a b" "a!"); printf "[%s]" "${(@qo)a}"; a=("'z'" b); printf "[%s]" "${(@Qo)a}"; echo`,
		Why:     "the two halves of rule 14 in the order they run, and the ordering step after both of them. `q` runs before `Q`, so the pair written together is a round trip and not `a\\ b`. Then the sort sees what the quoting left: `a!` comes before `a\\ b` only because the space has become a backslash, and `b` before `z` only because the quotes have gone — both orders reverse if the sort runs first, which is where this implementation had it",
	},
	{
		ID: "param/the-matching-flag-keeps-what-a-trim-took", Category: "parameter expansion",
		Snippet: `v=hello; echo "[${(M)v#h*l}][${(M)v##h*l}][${(M)v%l*o}][${(M)v%%l*o}][${(M)v#zzz}][${(M)v#}]"`,
		Why:     "one flag turns each of the four trims inside out: the same operator, the same match, and the *other* side of the split substituted. The operator still chooses how much — the doubled forms take the longest match here exactly as they drop the longest without the flag — so this is not a fifth and sixth operator but a second reading of the four. The last two are the rows that separate it from a no-op: a pattern that matches nothing leaves nothing, where the trim without the flag leaves the whole value, and an empty pattern takes the empty string",
	},
	{
		ID: "param/the-matching-flag-inverts-the-exclusion", Category: "parameter expansion",
		Snippet: `a=(f1 f22 f333); printf "[%s]" "${(M@)a:#f2*}"; echo; printf "[%s]" "${(@)a:#f2*}"; echo`,
		Why:     "the same flag on the operator that chooses *elements* rather than characters, and the same inversion: keep what the pattern matched instead of dropping it. Both spellings on one row, because the pair is what says it is an inversion and not an unrelated second operator — and the `(@)` is load-bearing, since quoted without it the array joins to one string first and the whole-match rule then takes all of it or none",
	},
	{
		ID: "param/the-matching-flag-elsewhere-does-nothing", Category: "parameter expansion",
		Snippet: `v=hello; echo "[${(M)v/l/L}][${(M)v:1}][${(M)v:-alt}][${(M)v}]"`,
		Why:     "and the boundary, which is worth a row because a flag named for matching looks as though it should reach the operator that matches and replaces: it does not. A replacement, a substring, a default and an expansion with no operator at all are what they would have been without it, so the flag reaches exactly the trims and the exclusion — measured one operator at a time rather than reasoned from the name",
	},
	{
		ID: "param/an-expansion-where-a-name-belongs", Category: "parameter expansion",
		Snippet: `v=abc; echo "[${${v}}][${${v}#a}][${${v}%c}][${${v}/b/X}]"`,
		Why:     "one expansion applied to the result of another, which is one shell's grammar and is idiomatic there rather than a corner — two of the third-party files a real startup sources use it. Every operator may follow the inner brace, so the row carries four of them: with the name position taken by an expansion there is nothing for `#` to be a length of and nothing for `%` to be a job specification of, and an implementation that reached for the name first has no name to reach for. bash and dash call the whole thing a bad substitution and ksh93 a syntax error",
	},
	{
		ID: "param/nested-expansions-nest", Category: "parameter expansion",
		Snippet: `v=abc; echo "[${${${v}}}][${${${v}#a}%c}][${${v#a}}]"`,
		Why:     "the depth is not one: an expansion standing in the name position may itself have one standing in *its* name position, and the operators stack outward — the inner `#a` runs before the outer `%c` sees anything. The third form is the same characters with the operator on the inside instead, which has to keep working and is the reading a parser gets by accident if it splits on the first brace",
	},
	{
		ID: "param/a-nested-expansion-takes-flags-and-a-substitution", Category: "parameter expansion",
		Snippet: `v=abc; echo "[${(U)${v}}][${${(U)v}}][${$(echo xy)#x}]"`,
		Why:     "what may sit either side of the nesting: a flag group belongs to the expansion it opens, so the outer and the inner may each carry one and the two spellings of `(U)` come to the same word. And the inner need not be a parameter expansion at all — a command substitution stands in the same position, which is what says the shape is `an expansion` rather than `another ${`",
	},
	{
		ID: "param/a-nested-expansion-is-the-whole-name", Category: "parameter expansion",
		GradedOnRefusal: true,
		Snippet:         `v=abc; echo "${x${v}}"`,
		Why:             "and the boundary, refused by every shell in the panel including the one that has the construct: the inner expansion is the whole of the name position, so text in front of it is not a longer name and `${${v}x}` is not a name with a suffix. Graded on the refusal because five shells decline the same characters in five wordings, which is what the Diagnostics vector is for",
	},
	{
		ID: "param/a-quoted-substitution-in-the-name-position", Category: "parameter expansion",
		Snippet: `printf "[%s]" ${(@f)"$(printf "a b\nc")"}; echo`,
		Why:     "`${(@f)\"$(cmd)\"}` is *the* way to split a command's output into an array by line in the shell with the grammar, and it appears throughout real configuration. The quotes are not decoration: quoted, the inner comes to one field and `(f)` splits that on newlines; the same characters *without* them are a different program — `${(@f)$(printf \"a b\\nc\")}` is three fields there, because an unquoted inner is split on IFS before the flag sees it. That spelling has no row of its own: this shell does not split an unquoted inner yet, which is #883's remaining half and is filed, and a row for it would record that gap rather than this one",
	},
	{
		ID: "param/a-quoted-non-substitution-is-not-a-name", Category: "parameter expansion",
		GradedOnRefusal: true,
		Snippet:         `v=abc; echo "${\"abc\"}"`,
		Why:             "the boundary the quotes do not move: what may stand in the name position is a *substitution*, and quoting a string does not make it one. Refused by every shell in the panel, including the one that reads a quoted `${\"${v}\"}` two characters away — so the quotes are a wrapper on the shape rather than a shape of their own",
	},
	{
		ID: "param/a-nested-expansions-value-is-what-the-outer-operator-tests", Category: "parameter expansion",
		Snippet: `v=; echo "[${${v}:-d}]"; v=abc; echo "[${${v}:-d}]"; echo "[${${v}:+y}]"`,
		Why:     "the conditional operators test what the inner expansion came to rather than whether some name is set, which is the whole reason the idiom exists: `${${0:#$ZSH_ARGZERO}:-${(%):-%N}}`, out of a plugin manager on this machine, is a default over the *result* of a pattern exclusion. An empty inner fires the colon test and a set one does not",
	},
	{
		ID: "expansion/element-exclusion-by-pattern", Category: "expansion",
		Snippet: `a=(one two three); printf "[%s]" "${(@)a:#t*}"; echo`,
		Why:     "`:#` drops the elements a pattern matches, which is one shell's alone: to bash the characters after the colon are an offset and `#t*` is arithmetic it refuses, and ksh93 refuses the flag group before it gets that far. The shape a startup file on this machine uses to take a hook out of a list, and the one that produced `operand expected at ``#fig_precmd''` here",
	},
	{
		ID: "expansion/element-exclusion-is-a-whole-match-not-a-prefix", Category: "expansion",
		Snippet: `v=hello; echo "[${v:#hel*}][${v#hel*}][${v:#xyz}]"`,
		Why:     "the sharpest row in the family, because two shells accept the same six characters and mean different things by them: zsh matches the pattern against the *whole* value and substitutes nothing when it hits, while ksh93 ignores the colon entirely and gives exactly what `${v#hel*}` gives — `lo`. bash refuses it as arithmetic. So `:#` is not `#` with a colon in front, and a dialect that treated it as one would silently answer ksh93's question in zsh's grammar",
	},
	{
		ID: "expansion/a-bare-array-name-unquoted", Category: "expansion",
		Snippet: `a=(one two); printf "[%s]" $a; echo`,
		Why:     "the headline of the family: an array named without a subscript is the *elements* in zsh — one field each, exactly what `${a[@]}` gives — where bash and ksh93 read the bare name as `${a[0]}` and hand over one field. One shell against two, and it is silent in the direction that hurts: the joined reading answers at status 0 with a plausible string, so nothing says the loop that follows will run once instead of twice",
	},
	{
		ID: "expansion/a-bare-array-name-in-a-for-loop", Category: "expansion",
		Snippet: `a=(one two); for f in $a; do printf "<%s>" "$f"; done; echo`,
		Why:     "the same rule where a script actually meets it. `for f in $files` is the idiom, and the two readings differ in how many times the body runs — twice in zsh, once in bash and ksh93 with the whole list in `$f`. Written as a loop and not only as a `printf` because the field count is the observable, and a body that receives one argument where it expected several is the damage the field count causes",
	},
	{
		ID: "expansion/a-bare-array-name-quoted-joins-on-ifs", Category: "expansion",
		Snippet: `IFS=-; a=(x y z); printf "[%s]" "$a"; echo`,
		Why:     "the guard on the other half: quoted, the bare name is one field in every shell that has arrays, and the shell that reads it as the whole array joins with the first character of IFS rather than a hard space. So `IFS=-` gives `x-y-z` and not `x y z`, the same rule `\"${a[*]}\"` follows. A reading that made the unquoted name a list by joining and then splitting would keep this row green while breaking the next one",
	},
	{
		ID: "expansion/a-bare-array-name-unquoted-is-not-a-join-then-split", Category: "expansion",
		Snippet: `IFS=; a=(x y z); printf "[%s]" $a; echo`,
		Why:     "the row that tells the two implementations of the same answer apart, and the reason it is here rather than a duplicate of the first. With IFS empty nothing splits, so joining the elements first and splitting the result gives one field `xyz` — while zsh gives three, because the elements were never joined at all. bash and ksh93 give the first element, unaffected either way. An implementation that reached the right answer under the default IFS by the wrong route fails exactly here",
	},
	{
		ID: "expansion/a-bare-array-name-in-a-context-that-does-not-split", Category: "expansion",
		Snippet: `IFS=-; a=(x y z); v=$a; case $a in "x-y-z") echo joined;; *) echo split;; esac; echo "[$v]"`,
		Why:     "where the unquoted spelling stops being a list: an assignment's value and a `case` subject split in no shell, and measured, zsh joins the bare name there exactly as quotes do — on IFS, so `x-y-z`. This is what keeps the rule about *fields* from leaking into the contexts that have none, and it is the row a fix routed through the list path unconditionally would break, silently turning `v=$a` into a hard-space join",
	},
	{
		ID: "expansion/a-bare-array-name-of-one-element", Category: "expansion",
		Snippet: `a=(only); printf "[%s]" $a; echo`,
		Why:     "the case the question is not asked in: with one element the array *is* that element under both readings, so all three shells agree and a dialect that had chosen neither still has an answer. It is recorded because a core with no shell behind it must run this rather than refuse it — an axis asked wider than the disagreement turns the ordinary way to read a one-element array into a diagnostic",
	},
	{
		ID: "expansion/a-bare-array-name-trimmed", Category: "expansion",
		Snippet: `a=(one two); printf "[%s]" ${a#o}; echo`,
		Why:     "an operator on the bare name, which inherits whatever the name turned out to be: zsh trims each element and keeps the fields, bash and ksh93 trim the one element the name gave them. The operator is the same everywhere — this row is about its *subject*, and it is the neighbor the scope note asked for by name",
	},
	{
		ID: "expansion/a-bare-array-name-sliced", Category: "expansion",
		Snippet: `a=(one two three); printf "[%s]" ${a:1}; echo`,
		Why:     "the sharpest of the operators, because the two readings do not merely differ in field count — they read the same offset against different things. Where the name is the list, `:1` drops the first *element* and leaves two; where it is a scalar, it drops the first *character* and leaves `ne`. Nothing in the spelling says which, and both answer at status 0",
	},
	{
		ID: "expansion/element-exclusion-on-a-bare-array-name", Category: "expansion",
		Snippet: `a=(1 2 3); printf "[%s]" ${a:#2}; echo`,
		Why:     "the second failure the join causes, and the less obvious one: `:#` asks about *elements*, so a bare name that joined to one string first matches the pattern against `1 2 3` as a whole, fails, and hands the array back looking like a filter that found nothing. zsh drops the element and leaves two fields. ksh93 reads the same six characters as `${a#2}` on its own bare name and answers `1`, which is neither shell's other reading — so the operator and its subject diverge together",
	},
	{
		ID: "expansion/element-exclusion-on-a-bare-array-name-by-pattern", Category: "expansion",
		Snippet: `a=(foo bar baz); printf "[%s]" ${a:#ba*}; echo`,
		Why:     "the same operator with a pattern that matches more than one element, which is what makes the no-op visible: two of three go in zsh. The joined reading cannot drop a *part* of its one string, so it drops nothing at all and the answer is indistinguishable from a pattern that simply did not match — the quiet shape this whole family is recorded to catch",
	},
	{
		ID: "expansion/a-bare-array-name-with-a-default", Category: "expansion",
		Snippet: `a=(one two); printf "[%s]" ${a:-d}; echo`,
		Why:     "the `-` test came to the parameter rather than to the word, so what it yields is the parameter under this shell's reading of it: the elements in zsh, the first alone in bash and ksh93. The row exists because the word side is already pinned elsewhere and the two must not be confused — a substitution that fires takes its fields from the *word*, and this one does not fire",
	},
	{
		ID: "expansion/unquoted-array-elements-keep-their-separators", Category: "expansion",
		Snippet: `a=("b 2" c); printf "[%s]" ${a[@]}; echo`,
		Why:     "what an unquoted whole-array expansion does to an element holding a separator, which is the same question an unquoted scalar asks and had never been asked here: bash and ksh93 split it into two fields, zsh leaves it whole. Silent in the direction that hurts — a filename with a space in it becomes two arguments and the status is 0 either way",
	},
	{
		ID: "expansion/unquoted-positional-parameters-keep-their-separators", Category: "expansion",
		Snippet: `set -- "p q" r; printf "[%s]" $@; echo`,
		Why:     "the same question on the construct scripts write most: `cmd $@` and `for f in $@` are everywhere, and in the shell that does not split an unquoted expansion every argument holding a separator survives whole. The `[*]` spelling is a different row because joining is a different question from splitting",
	},
	{
		ID: "expansion/unquoted-array-elements-under-a-changed-ifs", Category: "expansion",
		Snippet: `IFS=-; a=("x-y" z); printf "[%s]" ${a[@]}; echo`,
		Why:     "the row that says the split is asked against the *actual* separators rather than a guess at whitespace: with IFS a dash, an element holding a dash is two fields where the shells split and one where they do not. A reading hardcoded to spaces answers the previous row correctly and this one wrongly",
	},
	{
		ID: "expansion/an-unquoted-array-element-that-looks-like-a-pattern", Category: "expansion",
		Snippet: `: > zz1; a=("zz*" other); printf "[%s]" ${a[@]}; echo`,
		Why:     "the second of the two stages an expansion's result goes through, on the same path: bash and ksh93 read what came out as a pattern and hand back the file that matched, zsh hands back the text. The element decides what the command receives from the state of the directory, which is the quietest failure in the family — nothing is written and nothing fails",
	},
	{
		ID: "expansion/a-bare-array-name-with-a-separator-in-an-element", Category: "expansion",
		Snippet: `a=("b 2" c); printf "[%s]" $a; echo`,
		Why:     "the bare name inherits both stages, because it *is* the whole-array expansion in the shell that reads it as a list. The rows that made the bare name a list avoid separators inside elements and so pass under either reading; this one does not, and it is where `for f in $files` stops being right for a filename with a space in it",
	},
	{
		ID: "expansion/an-empty-element-under-a-non-whitespace-separator", Category: "semantics axes",
		Snippet: `IFS=:; set -- x "" y; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the row #1013 is about, and the one that turns the empty-element rule from a unanimous drop into an axis. bash, bash 3.2 and bash as `sh` join the parameters on the first character of IFS before splitting, so `x::y` has two separators meeting and the field between them survives — three fields from three parameters. ksh93 and dash take the elements one at a time and the empty one is no field at all, so two come out; zsh drops it too, and keeps it under `setopt shwordsplit`. The count is printed with the fields because `[x][y]` and `[x][][y]` are the same characters once the boundaries are gone",
	},
	{
		ID: "expansion/an-empty-element-under-a-whitespace-separator", Category: "expansion",
		Snippet: `set -- x "" y; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the contrast that says the disagreement is the separator's and not the empty element's: with a whitespace IFS every shell in the panel drops it, because a run of separators is one delimiter there however the fields were arrived at. This is the row that made the drop look unanimous, and it is why the axis must not be asked here",
	},
	{
		ID: "expansion/a-trailing-empty-element-under-a-non-whitespace-separator", Category: "semantics axes",
		Snippet: `IFS=:; set -- x y ""; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the other face of the same answer, and the reason the axis cannot be spelled *an empty element survives*: the join that keeps a middle empty is what loses a trailing one, because `x:y:` ends in a separator and a trailing separator makes no field. bash answers two. zsh under `shwordsplit` does not join and keeps three, and ksh93 keeps three here while dropping the middle one in the row above — the one shape in the panel neither reading explains",
	},
	{
		ID: "expansion/an-element-ending-in-a-separator-meets-the-join", Category: "semantics axes",
		Snippet: `IFS=:; set -- "x:" y; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the face no rule about *empty elements* reaches at all: nothing here is empty, and bash still answers three fields because the separator ending the first element meets the one the join puts after it. Taken one at a time the trailing separator makes no field and two come out, which is ksh93's and dash's answer. It is the row that says the question is the join rather than the element",
	},
	{
		ID: "expansion/the-array-spelling-joins-with-the-parameters", Category: "expansion",
		Snippet: `IFS=:; a=(x "" y); set -- ${a[@]}; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "`$@` and `${a[@]}` are the same fields for the same elements in every shell measured, which is what keeps the two branches from parting company — they reach the answer by different code and must not disagree",
	},
	{
		ID: "expansion/the-scalar-path-splits-the-same-string-the-join-makes", Category: "expansion",
		Snippet: `IFS=:; v="x::y"; set -- $v; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the string bash's join produces, split on its own: every shell that splits answers three fields, so the scalar path is unanimous and is not the axis. It is the anchor for the list rows above — bash's list answer is exactly this one, which is what says bash joins rather than that it has a second rule about empty elements",
	},
	{
		ID: "expansion/an-empty-ifs-joins-nothing", Category: "expansion",
		Snippet: `IFS=""; set -- x y; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "an IFS that is set and empty has no first character to join on, and no shell in the panel joins there — two fields, where a join would leave one. The guard on the axis: it must not be asked where there is nothing to join with",
	},
	{
		ID: "expansion/an-unquoted-star-does-not-always-join-the-parameters", Category: "semantics axes",
		Snippet: `IFS=:; set -- x y; set -- $*; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the same question on the bare spelling, which had a branch of its own: `$*` joined on the scalar path and the split that would have undone it never ran in zsh, so one field `x:y` came out where every shell in the panel gives two. One answer settles both faces",
	},
	{
		ID: "expansion/an-unquoted-star-subscript-under-a-non-whitespace-separator", Category: "semantics axes",
		Snippet: `IFS=:; a=("x:" y); set -- ${a[*]}; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the join itself, on the star spelling, with nothing empty anywhere: bash makes `x::y` and answers three fields, and ksh93 splits each element on its own, loses the trailing separator and answers two. It is the same answer `${a[@]}` asks — measured, an unquoted `[*]` and an unquoted `[@]` are the same fields in every shell in the panel",
	},
	{
		ID: "expansion/an-unquoted-star-subscript-and-at-agree", Category: "expansion",
		Snippet: `IFS=:; a=("x:" y); printf "@"; printf "[%s]" ${a[@]}; printf " *"; printf "[%s]" ${a[*]}; echo`,
		Why:     "the two spellings side by side on the same elements, which is what says one answer settles both rather than the star being a question the `[@]` path had already answered differently. They agree in every shell in the panel — with one shape excepted, recorded rather than folded in here: ksh93 alone parts them on a *trailing* empty element, where `${a[@]}` keeps a field and `${a[*]}` does not. That is the same ksh93 oddity `expansion/a-trailing-empty-element-under-a-non-whitespace-separator` records, seen from the second spelling, and neither reading of the join explains it",
	},
	{
		ID: "expansion/an-unquoted-star-subscript-is-not-always-a-pattern", Category: "expansion",
		Snippet: `: > zz1; a=("zz*" other); set -- ${a[*]}; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the stage this path never performed at all: the result of an expansion is matched against the filesystem only where the dialect says it is, and the star join returned its fields raw. zsh leaves the star alone and bash and ksh93 expand it, and ours matched the directory in every dialect — the same silent wrong answer #981 fixed on the `[@]` path",
	},
	{
		ID: "expansion/a-quoted-star-subscript-always-joins", Category: "expansion",
		Snippet: `IFS=-; a=("x y" z); set -- "${a[*]}"; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the guard on the half that must not move: inside quotes every shell measured joins on the first character of IFS and yields one field, which is what the two spellings exist to differ about. The axis is the *unquoted* spelling and this row is what watches the boundary",
	},
	{
		ID: "expansion/a-quoted-star-always-joins-the-parameters", Category: "expansion",
		Snippet: `IFS=-; set -- "x y" z; set -- "$*"; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the same guard on the bare spelling, beside `\"$@\"` keeping one field per parameter — the pair that says quoting is what decides the join and the dialect is what decides it without quotes",
	},
	{
		ID: "expansion/an-unquoted-star-subscript-does-not-always-join", Category: "expansion",
		Snippet: `a=("x y" z); printf "[%s]" ${a[*]}; echo`,
		Why:     "the neighbor of the `[@]` rows and a question of its own: bash and ksh93 join the elements on IFS and split the result, which is why they answer three fields, while zsh does not join an *unquoted* `[*]` at all and answers two — the same two `${a[@]}` gives it. Quoted, all three join. So whether an unquoted `[*]` joins is not decided by whether the shell splits, and no arrangement of the splitting answer produces zsh's reading here",
	},
	{
		ID: "expansion/an-unquoted-star-subscript-under-an-operator-distributes", Category: "expansion",
		Snippet: `a=(oxo yo); printf "[%s]" ${a[*]%o}; echo`,
		Why:     "quoting is what decides whether an operator sees the list or the joined string, and the *unquoted* spelling has one answer nobody has to be asked for: every shell in the panel with arrays trims each element and hands back two fields. The axis was asked here regardless, so the dialect that answers no gave the unquoted form the quoted reading and returned a single field holding `oxo y` — the trim applied to a field boundary instead of to an element, at status 0",
	},
	{
		ID: "expansion/a-quoted-star-subscript-under-an-operator-is-the-axis", Category: "semantics axes",
		Snippet: `a=(oxo yo); printf "[%s]" "${a[*]%o}"; echo`,
		Why:     "the half that really is a disagreement, and the pair with the row above is the whole rule: quotes join first, `[*]` joins last. bash and ksh93 trim each element and join what is left, zsh joins first and trims the joined string once — one field either way, differing only in whether the `o` inside the join survived. Both answers are plausible strings, which is why the two rows have to stand together",
	},
	{
		ID: "expansion/a-quoted-star-subscript-does-not-filter-elements", Category: "expansion",
		Snippet: `a=(foo bar baz); printf "[%s]" "${a[*]:#ba*}"; echo`,
		Why:     "the silent direction of the same rule: quoted, the array joins to one string first and the pattern is tested against that whole value, so `ba*` — which matches two of the three elements — drops nothing and the array comes back entire. Filtering leaves `foo`, and a filter that ran where the shell would have left the array alone is indistinguishable from a pattern that did not match. ksh93 reads the same characters as `${a[*]#ba*}` and answers `foo r z`, which is neither",
	},
	{
		ID: "expansion/a-quoted-star-subscript-set-intersection-empties-to-one-field", Category: "expansion",
		Snippet: `a=(x y z); b=(nope); set -- "${a[*]:*b}"; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the direction that empties it, and what quoting still guarantees when it does: the joined value is held by no other array, so nothing survives — and the result is one *empty* field rather than no field at all. The unquoted spelling of the same snippet is zero fields, so this is the row that says quoting decides the count independently of what the operator left",
	},
	{
		ID: "expansion/a-quoted-star-subscripts-operator-sees-the-ifs-join", Category: "expansion",
		Snippet: `IFS=-; a=(oxo yo); set -- "${a[*]:#ox?-yo}"; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "which string the operator is handed, once it is handed one: the elements joined on the first character of IFS, not on a space. The pattern is written with the dash and matches; written with a space it would not. It is the row that keeps the quoted reading from being implemented with a hardcoded separator, which would pass every row above and fail this one",
	},
	{
		ID: "expansion/a-slice-under-a-star-subscript-reads-the-list-quoted-too", Category: "expansion",
		Snippet: `a=(one two three); printf "[%s]" "${a[*]:1}" "${a[*]:1:1}"; echo`,
		Why:     "the operator that is the exception, and the reason the rule is about the operator rather than about quoting alone: a slice's offset counts *elements* under `[*]` however the expansion is quoted, and only the join afterwards differs. Unanimous in all three shells with arrays, so it is core and must reach no axis — a reading that gave every quoted `[*]` the joined string would answer `wo three` here",
	},
	{
		ID: "expansion/a-quoted-bare-array-name-sliced", Category: "expansion",
		Snippet: `a=(one two three); printf "[%s]" "${a:1}"; echo`,
		Why:     "the same rule reached through a bare name, and the spelling that stayed wrong while `[*]` was: where the name is the whole array this is the list sliced and then joined — `two three` — and where it is `${a[0]}` it is that element with its first character dropped, `ne`. Both are plausible strings at status 0. The unquoted spelling is `${a[@]}`'s question and was already recorded; this is the quoted half, and only the slice needs it, because every other operator applied to the joined value is already the reading the shell follows",
	},
	{
		ID: "expansion/a-quoted-bare-array-name-sliced-past-one-element", Category: "expansion",
		Snippet: `a=(solo); printf "[%s]" "${a:1}"; echo`,
		Why:     "one element is enough for the two readings to part, which is why the slice cannot be waved through on a short array the way the value-to-value operators can: dropping the only element leaves nothing, where dropping the first character leaves `olo`. The empty answer is the quiet one — a script slicing a one-element array gets a field that is there and empty rather than a complaint",
	},
	{
		ID: "expansion/an-at-list-joined-where-nothing-splits", Category: "semantics axes",
		Snippet: `IFS=-; a=(x y z); v=${a[@]}; echo "[$v]"`,
		Why:     "the character a list is joined with when it reaches a context that keeps no fields, which had never been asked: zsh joins on the first character of IFS and bash and ksh93 rejoin on a hard space. Silent in the shape that is hardest to see — the default IFS begins with a space, so every script that leaves IFS alone gets the right answer and the one that sets it gets a wrong one at status 0. The `*` spelling is a different question and is core; this is the `@` one",
	},
	{
		ID: "expansion/an-at-list-joined-in-a-here-document", Category: "semantics axes",
		Snippet: "IFS=-; a=(x y z); cat <<E\n[${a[@]}]\nE",
		Why:     "the same answer in a second context, which is what says it is one axis rather than one per context: a here-document body is lexed as double-quoted text and expanded span by span, so a whole-array subscript in one reaches the scalar view rather than the list path and was joined by a third copy of the same hard space. All four never-splitting contexts — an assignment's value, a `case` subject, a `[[ ]]` operand and this — answer alike within each shell",
	},
	{
		ID: "expansion/an-at-list-joined-into-a-pattern-operand", Category: "semantics axes",
		Snippet: `IFS=-; set -- x y; v="Zx-y"; echo "[${v%$@}]"`,
		Why:     "the quietest face of the join, because the joined string is a *pattern* and what it joins with decides whether it matches at all: on IFS the operand is the suffix `x-y` and the trim leaves `Z`, on a space it is `x y` and the trim silently does nothing. A trim that ran and changed nothing is indistinguishable from a pattern that did not match, and the status is 0 either way",
	},
	{
		ID: "expansion/a-star-list-joined-where-nothing-splits", Category: "expansion",
		Snippet: `IFS=-; a=(x y z); v=${a[*]}; echo "[$v]"`,
		Why:     "the other spelling, and core rather than the axis: bash 5.3, ksh93 and zsh all join on the first character of IFS here, and ours joined on a space in every dialect. bash 3.2 is the panel's one deviation and is recorded rather than modeled — it joins this on a space while joining `$*` on IFS in the same build, so it disagrees with itself, and no dialect here is graded against 3.2. The pair with the `[@]` row is what says the spelling decides which question is asked",
	},
	{
		ID: "expansion/a-star-list-of-parameters-joined-where-nothing-splits", Category: "expansion",
		Snippet: `IFS=-; set -- x y z; v=$*; echo "[$v]"`,
		Why:     "the bare spelling of the same core answer, unanimous across the whole panel including dash and bash 3.2 — the row that says the star half needs no dialect. It was a hard space here in every dialect, so this was the one shape wrong in all four at once against a panel that agrees completely",
	},
	{
		ID: "expansion/an-empty-ifs-joins-an-unsplit-list-with-nothing", Category: "expansion",
		Snippet: `IFS=""; a=(x y); v=${a[*]}; echo "[$v]"`,
		Why:     "an IFS that is set and empty joins with *nothing* rather than falling back to a space: `xy`, in bash 5.3, ksh93 and zsh alike. It is the guard that separates this question from whether an unquoted list is joined before it is split — that one must not be asked when there is nothing to join with, and this one must, because the join happens either way and joining with nothing is a real answer",
	},
	{
		ID: "expansion/a-star-subscript-as-a-redirection-target", Category: "expansion",
		Snippet: `IFS=-; a=(x y); : > ${a[*]}; ls`,
		Why:     "the fourth place the join was a hard space, and the one where the answer becomes a *filename*: ksh93 writes `x-y` where we wrote `x y`, so a script's redirection created a different file from the one the shell would have. bash refuses the target as ambiguous and zsh writes to both names, so ksh93 is the only member that reaches the joined reading — which is exactly why nothing here had ever measured it",
	},
	{
		ID: "expansion/a-conditional-assignment-keeps-the-arrays-fields", Category: "expansion",
		Snippet: `a=("x y" z); set -- "${a[@]:=d}"; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "`:=` comes to the *parameter* when its test does not fire, and the parameter is the whole array — so it keeps its fields exactly as `${a[@]:-d}` does. Every panel member with arrays answers two fields; joining them gives one holding `x y z`, which is a plausible string at status 0 and the reason the count is printed. The element holding a separator is what makes the bug visible at all: on `(one two)` the unquoted spelling joins on IFS and the split takes the fields straight back out, so it was right by coincidence wherever splitting is on",
	},
	{
		ID: "expansion/a-conditional-error-keeps-the-arrays-fields", Category: "expansion",
		Snippet: `a=("x y" z); set -- "${a[@]:?e}"; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the other operator with the same two outcomes, and the same answer when the test does not fire. It is a separate row because the firing side is a *fatal* one: an implementation that answered `the parameter` for a fired `?` would keep these fields and never raise the error, which is the one direction where being wrong loses a diagnostic the script asked for rather than a field boundary",
	},
	{
		ID: "expansion/a-conditional-assignment-under-a-star-subscript-still-joins", Category: "expansion",
		Snippet: `IFS=-; a=("x y" z); set -- "${a[*]:=d}"; printf "%d" "$#"; printf "[%s]" "$@"; echo`,
		Why:     "the guard on the half that must not move: `[*]` is one field with the elements joined on the first character of IFS, and it stays so under an operator. The pair with the `[@]` row above is what says the fix belongs to the subscript rather than to the operator — a reading that simply kept the fields under every conditional would answer this one wrongly, and it is quiet either way because both answers are the same characters differently divided",
	},
	{
		ID: "expansion/a-bare-array-name-with-a-conditional-assignment", Category: "expansion",
		Snippet: `a=(one two); printf "[%s]" ${a:=d}; echo`,
		Why:     "the same operator reached through a bare name, which follows from the rewrite rather than from a second rule: where the name is the list the elements come through, and where it is `${a[0]}` the first alone does. zsh answers two fields and bash and ksh93 one. It was one joined field in all three, so the two spellings disagreed with the shell in opposite directions at once",
	},
	{
		ID: "expansion/a-bare-array-name-with-a-conditional-error", Category: "expansion",
		Snippet: `a=(one two); printf "[%s]" ${a:?e}; echo`,
		Why:     "the fourth conditional through the bare name, recorded beside the third because the two are answered by one test and a drifting copy of it would part them. The test does not fire on a set array, so nothing is fatal here and the row is about the fields — which is exactly the shape that returns a plausible answer with no complaint",
	},
	{
		ID: "expansion/element-exclusion-without-a-flag-group", Category: "expansion",
		Snippet: `a=(one two three); echo "[${a:#two}]"`,
		Why:     "the same operator with no `(@)` in front and inside quotes, where the array joins to one string first: the pattern is then matched against `one two three` as a whole, matches nothing, and the value is left standing. Not a no-op by accident — `${a:#*}` on the same array is empty — and it is what says the operator asks about *elements*, of which a joined scalar has one. ksh93 answers `one` from the same characters, which is `${a#two}` against a scalar that is only the first element, so the two shells agree on neither the operator nor what `$a` names",
	},
	{
		ID: "expansion/a-length-with-an-operator", Category: "expansion",
		Snippet: `v=abc; echo ${#v#a}`,
		Why:     "the length and an operator in one expansion, which two shells read as different things and four refuse outright. zsh measures what the operator *leaves* and answers 2; bash, bash 3.2, bash as `sh`, dash and ksh93 all call it a bad substitution. The number we answered was 3 — the length of the untouched value, the operator dropped on the floor — which is not merely the wrong one of two readings but nobody's at all, and it came back at status 0. A script computing a trimmed length got the untrimmed one",
	},
	{
		ID: "expansion/a-length-with-a-substring-operator", Category: "expansion",
		Snippet: `v=abc; echo ${#v:1}`,
		Why:     "the same pairing with the operator that looks least like one, and the row that says the refusal is about the operator rather than about the character `#`: there is no second `#` here and four of the five still refuse. zsh answers 2 for the same reason as the trim — apply, then measure",
	},
	{
		ID: "expansion/a-length-with-a-replacement-operator", Category: "expansion",
		Snippet: `v=abc; echo ${#v/b/XX}`,
		Why:     "the operator that makes the value *longer*, which is what separates measuring the result from measuring the subject: zsh answers 4 where the untouched value is 3, so a reading that quietly ignored the operator cannot hide behind a coincidence here the way it can on a pattern that trims nothing",
	},
	{
		ID: "expansion/a-length-with-an-operator-is-refused-late", Category: "expansion",
		Snippet: `v=abc; if false; then echo ${#v#a}; fi; echo reached`,
		Why:     "when each shell decides. Every one of the five that refuses the pairing defers it to the expansion — the branch is never taken, so nothing is wrong and all six print `reached`. That includes ksh93, whose grammar otherwise refuses an unknown operator while reading, so the deferral is measured rather than assumed from where the other refusals happen",
	},
	{
		ID: "expansion/a-length-with-no-operator-is-unanimous", Category: "expansion",
		Snippet: `v=abc; a=(xx yy); set -- p qq; echo "${#v} ${#a[@]} ${#@}"`,
		Why:     "the boundary the previous rows need: the length on its own is the value's, a whole-array subscript counts elements and the positional spelling counts parameters, and all three are unanimous. A grammar that refused the length whenever anything followed the name would break every one of these while fixing the pairing above",
	},
	{
		ID: "expansion/a-colon-before-a-trim-is-a-third-reading", Category: "expansion",
		Snippet: `v=hello; printf "[%s]" "${v:#hel*}" "${v:##hel*}" "${v:%lo}" "${v:%%l*}"; echo`,
		Why:     "the three readings of the same six characters, in the one shell whose reading had no implementation behind it: ksh93 ignores the colon entirely, so all four of these are the plain trims and answer `lo`, ``, `hel` and `he`. zsh reads `:#` as an element exclusion and matches the pattern against the whole value, and bash refuses it as arithmetic with `#hel*` named as the offending token. Four operators in one row because the shortest-against-longest distinction has to survive the colon — a reading that dropped it and then re-scanned gets all four, and one that named them over somewhere else can lose the doubling",
	},
	{
		ID: "expansion/a-colon-before-a-trim-reaches-the-array-forms", Category: "expansion",
		Snippet: `a=(foo bar baz); printf "[%s]" "${a[@]:#ba*}"; echo`,
		Why:     "the same node, so the per-element mapping under `[@]` comes for free: ksh93 trims each element and answers `foo r z`, which is exactly what `${a[@]#ba*}` does there. zsh drops the elements the pattern matches and answers `foo`, and the two are one field apart with no diagnostic either way. It is the row that says the colon is *ignored* rather than recorded — a node carrying a flag downstream would have had to be taught this separately",
	},
	{
		ID: "expansion/a-colon-before-anything-but-a-trim", Category: "expansion",
		Snippet: `v=hello; printf "[%s]" "${v:2}" "${v:2:2}" "${v:-d}"; echo`,
		Why:     "the boundary of the previous two rows, and what stops the colon from meaning nothing at all: an offset is still an offset in ksh93, a second colon is still a length, and `:-` is still a default. Unanimous across the panel, so a grammar that swallowed every colon rather than the four trims would break the shapes every shell shares while fixing the one only ksh93 has",
	},
	{
		ID: "expansion/element-exclusion-empty-pattern", Category: "expansion",
		Snippet: `a=("" one); printf "[%s]" "${(@)a:#}"; echo`,
		Why:     "an operator with nothing after it, which the whole-match rule makes meaningful rather than degenerate: the empty pattern matches only the empty string, so exactly the empty element goes. A reading that treated an absent pattern as `*` would empty the array, and one that treated it as no operator at all would leave it whole",
	},
	{
		ID: "expansion/element-exclusion-pattern-out-of-a-parameter", Category: "expansion",
		Snippet: `p='t*'; a=(one two); printf "[%s]" "${(@)a:#$p}"; echo`,
		Why:     "whether the pattern may come out of a variable, and it may not: the shell that has the operator is the one that does not glob the result of an expansion, so `$p` is the two literal characters and matches no element. The same axis that keeps `p='et*'; echo $p` from expanding, reaching a third construct — and the reason the pattern is built from the operand's *word* rather than from its text",
	},
	{
		ID: "expansion/element-set-difference-and-intersection", Category: "expansion",
		Snippet: `a=(x y z); b=(y w); printf "[%s]" "${(@)a:|b}"; echo; printf "[%s]" "${(@)a:*b}"; echo`,
		Why:     "the two operators that live beside `:#` and take the *name* of another array rather than a pattern, comparing elements for equality: `:|` keeps what the other does not hold and `:*` keeps only what it does. Both are the same one shell's, and both are refused as arithmetic elsewhere — bash names the token `|b` and `*b`, which is the tell that it read an offset",
	},
	{
		ID: "expansion/element-set-operators-against-an-unset-name", Category: "expansion",
		Snippet: `a=(x y z); printf "[%s]" "${(@)a:|nope}"; echo; printf "[%s]" "${(@)a:*nope}"; echo`,
		Why:     "a name nothing is stored under is an empty set and not a complaint, in both directions: the difference keeps everything and the intersection keeps nothing. Quietly — no diagnostic and status 0 — which is the half a reading that refused an unknown name would get wrong on the safe-looking side",
	},
	{
		ID: "expansion/a-colon-before-anything-else-is-still-an-offset", Category: "expansion",
		Snippet: `v=abcdef; echo "[${v:2}][${v:2:2}][${v: -2}][${v:-alt}][${v:+set}]"`,
		Why:     "the guard on the whole family: exactly three characters after a colon make it an operator, and every other spelling is what it always was. All five are unanimous across the panel, so a grammar that widened the disambiguation by one character would break the shapes every shell shares rather than the ones only zsh has",
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
		ID: "array/unsetting-a-range-of-elements", Category: "expansion",
		Snippet: `a=(x y z); unset "a[1,2]"; echo "st=$?"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "`unset` reaching a *span* written as a range, which one shell reads and the others read as the arithmetic comma operator naming its right operand alone. The two answers are two different arrays and neither says anything, so the value is the only evidence: the span becomes one empty element and the array shrinks, against one element blanked or removed where the operator's value points",
	},
	{
		ID: "array/unsetting-a-range-below-the-first-element", Category: "expansion",
		Snippet: `a=(x y z); unset "a[0,1]"; echo "st=$?"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"; unset "a[0,0]"; echo "st=$?"; echo "n=${#a[@]}"`,
		Why:     "the pair that shows the below-the-first-element refusal is a rule about the *span* and not about the subscript that starts it. A range that begins out of reach and ends inside is not refused — the start is the first element — and one that lies wholly out of reach is. `array/unsetting-below-the-first-element` is the same rule at a span of one, which is why `a[0]` alone is refused",
	},
	{
		ID: "array/unsetting-a-reversed-range", Category: "expansion",
		Snippet: `a=(x y z); unset "a[2,1]"; echo "st=$?"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "an end before the start names a span with nothing in it, and the shell that replaces a span with one empty element still puts one there — so the array *gains* an element where it would have begun. Measured rather than guessed, and it is the strongest evidence that the reading is a replacement and not a removal",
	},
	{
		ID: "array/unsetting-a-range-that-starts-past-the-end", Category: "expansion",
		Snippet: `a=(x y z); unset "a[4,5]"; echo "st=$?"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"; unset "a[3,4]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`,
		Why:     "the other end of the same rule, and the pair is the point: an end past the last is the last and a span of one is left, but a *start* past the last has nothing to replace and nothing to stand in front of, so nothing at all happens — where a reversed range inside the array inserts",
	},
	{
		ID: "array/unsetting-a-range-from-the-end", Category: "expansion",
		Snippet: `a=(x y z); unset "a[-1,-1]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"; b=(x y z); unset "b[-2,-1]"; printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"`,
		Why:     "a range's start takes the same negative rule its single subscript does: in the shell that blanks, only `-1` acts, so `[-1,-1]` blanks the last element and `[-2,-1]` leaves all three. Pinned because a span would otherwise be expected to act on both, and it does not",
	},
	{
		ID: "array/unsetting-a-range-of-characters", Category: "expansion",
		Snippet: `a=hello; unset "a[2,3]"; echo "st=$? [${a-UNSET}]"; b=hello; unset "b[-2,-1]"; echo "st=$? [${b-UNSET}]"`,
		Why:     "the same span over a string, where a subscript names a character: the characters go and the rest closes up. Every negative within reach acts here where on an array only `-1` does, so the two halves of the range reading part exactly where the single subscript's do",
	},
	{
		ID: "array/unsetting-a-reversed-range-of-characters", Category: "expansion",
		Snippet: `a=hello; unset "a[3,2]"; echo "st=$? [${a-UNSET}]"`,
		Why:     "the reversed range on a string, which is where the insertion the array case shows becomes invisible: an empty character put where the span would have begun leaves the string as it was. The two are one rule and only the array can see it",
	},
	{
		ID: "array/an-unset-subscript-refused-is-named-as-written", Category: "expansion",
		Snippet: `a=(x y z); unset "a[x-9]"; echo "st=$?"; echo "n=${#a[@]}"`,
		Why:     "what the refusal names on this route, which is not what it names on the assignment's: bash keeps the subscript as written but drops the array in front of it, ksh93 names the array alone, and both put the builtin's name before the sentence where neither does for an assignment. The expression is what tells the written text from the -9 it came to. zsh has nothing to refuse, a negative subscript reaching nothing being silent there",
	},
	{
		ID: "array/unsetting-a-subscript-of-a-scalar", Category: "expansion",
		Snippet: `a=hello; unset "a[2]"; echo "st=$? [${a-UNSET}]"`,
		Why:     "a single subscript on a name that is no array, where the three readings show what they mean and give three different answers: the shell that reads a subscript as a character takes that character out, the shell that reads it as an element finds none — a scalar is the one element at the base — and refuses, and the other element-reading shell says nothing at all. Nobody turns the scalar into an array, and the value is what tells them apart rather than the status",
	},
	{
		ID: "array/unsetting-the-subscript-that-names-a-scalar", Category: "expansion",
		Snippet: `a=hello; unset "a[0]"; echo "st=$? [${a-UNSET}]"`,
		Why:     "the one subscript the element reading answers to: at the base, a scalar *is* the element, and unsetting through it takes the whole name away — not the value, the name. Below the first character where the base is 1, so the shell that reads characters refuses the same line. bash 3.2 refuses it too, having read `${a[0]}` as the whole string moments earlier, so the two bash columns disagree here on purpose and the graded one is 5.3's",
	},
	{
		ID: "array/unsetting-a-subscript-of-a-scalar-takes-the-attribute-too", Category: "expansion",
		Snippet: `export a=hello; unset "a[0]"; echo "st=$?"; env | grep '^a=' || echo "(none)"`,
		Why:     "it is the name that goes and not the value: after it, no child is told about `a` either. Read through a real child because a name that is unset and a name that is empty look alike to the shell's own `$a`",
	},
	{
		ID: "array/unsetting-a-subscript-of-a-name-holding-nothing", Category: "expansion",
		Snippet: `unset b; unset "b[0]"; echo "st=$?"; unset "b[1]"; echo "st=$?"`,
		Why:     "the boundary of all of it: a name holding nothing has neither an element nor a character for a subscript to name, and every shell is quiet about both spellings. Without this the refusals above read as being about the subscript rather than about the name having a value",
	},
	{
		ID: "array/unsetting-a-character-from-the-end-of-a-scalar", Category: "expansion",
		Snippet: `a=hello; unset "a[-1]"; echo "st=$? [$a]"; b=hello; unset "b[-5]"; echo "st=$? [$b]"; c=hello; unset "c[-6]"; echo "st=$? [$c]"`,
		Why:     "every negative within reach acts on a character, where on an *array* under the same shell only `-1` does — so a string is a character position and not a one-element array wearing one. Reaching back past the first character is quiet rather than refused, which is the other half of `array/unsetting-past-the-start` and the reason the boundary is not one rule for both signs",
	},
	{
		ID: "array/unsetting-a-subscript-past-the-end-of-a-scalar", Category: "expansion",
		Snippet: `a=hello; unset "a[9]"; echo "st=$? [${a-UNSET}]"`,
		Why:     "past the end names nothing under the character reading and the string is untouched — where the element readings are still looking at a name that is no array and answer as they do for any other subscript. One line, and it separates 'the subscript found nothing' from 'the name is not an array'",
	},
	{
		ID: "array/unsetting-a-subscript-of-an-empty-scalar", Category: "expansion",
		Snippet: `a=; unset "a[1]"; echo "st=$? [${a-UNSET}]"`,
		Why:     "an empty string still has the one element a scalar is, and has no character at all — so where the base is 1 the character reading leaves the name empty and the element readings reach for a name that is not an array. Reading 'no characters' as 'no question' took the empty name away entirely, which is the bug this pins",
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
		ID: "assoc/a-quoted-key-keeps-its-quotes-or-does-not", Category: "expansion",
		Snippet: `typeset -A m; m["k"]=W; kk='"k"'; printf '[%s]' "${m[$kk]}" "${m[k]}"; echo`,
		Why:     "the two readings of a quoted subscript, told apart by storing under one spelling and reading with the other — a key that is one string under both would hide it. bash and ksh93 run the subscript through quote removal, so the key is `k` and the second read finds `W`; zsh takes it as written, so the key is the three characters and the *first* read finds it. dash has no attribute and refuses the line",
	},
	{
		ID: "assoc/a-quoted-substitution-in-a-key", Category: "expansion",
		Snippet: `typeset -A q; q[k]=K; v=k; printf '[%s]' "${q[$v]}" "${q["$v"]}"; echo`,
		Why:     "the crisp form of the same question: the substitution is performed under both readings and only the quote characters around it differ. bash and ksh93 find the element both ways; zsh finds it bare and not quoted, because there the key is `\"k\"`. So it is a rule about quoting rather than about expansion",
	},
	{
		ID: "assoc/a-key-is-not-space-trimmed", Category: "expansion",
		Snippet: `typeset -A p; p[s]=T; printf '[%s]' "${p[ s ]}" "${p[s]}"; echo`,
		Why:     "unanimous in the three shells with the attribute, and the row this implementation answered wrongly: a subscript's blanks belong to the key, so `${p[ s ]}` looks up three characters and finds nothing. The trimming an *arithmetic* subscript can afford — the evaluator ignores blanks — is wrong here",
	},
	{
		ID: "assoc/a-quoted-at-is-a-key", Category: "expansion",
		Snippet: `typeset -A n; n[a]=1; n[b]=2; printf 'bare=%d' "${#n[@]}"; printf ' quoted[%s]' "${n["@"]}"; echo`,
		Why:     "the guard on the reading above: a *bare* `@` is still the whole array in all three, and a quoted one is a key — so the quoted spelling finds nothing, since nothing is stored under it. A fix that took the subscript as written everywhere would turn `${n[@]}` into a lookup and lose the whole-array spelling. The bare spelling is counted rather than printed, because the order an associative array yields its values in is not a fact this corpus can record",
	},
	{
		ID: "subscript/a-search-operand-keeps-a-backslash", Category: "expansion",
		Snippet: `a=('bet\a' beta); printf '[%s]\n' "${a[(r)bet\a]}"`,
		Why:     "the pattern half of the same rule, on the one construct that reaches it: a backslash before a character that needed no escaping is two characters of the pattern in zsh, so the operand finds the element whose value holds a backslash and does not find `beta`. The other five have no flag group in a subscript and read the whole thing as arithmetic, which is a diagnostic rather than an answer",
	},
	{
		ID: "pattern/an-escape-before-an-ordinary-character", Category: "patterns",
		Snippet: `setopt globsubst 2>/dev/null; p='bet\a'; case beta in $p) echo strips;; *) echo keeps;; esac; case 'bet\a' in $p) echo literal;; *) echo no;; esac`,
		Why:     "the pattern language's own answer, asked the only way it can be: quote removal spends an escape written in the source before the matcher sees it, so a `case` pattern spelled `bet\\a` is `beta` in all six and says nothing. A *substituted* pattern asks it — five shells match the result of an expansion as a pattern, and the sixth does under the option this line sets. A backslash before a character that needed no escaping is spent in five and kept in zsh, where the pattern is five characters",
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
		ID: "heredoc/the-delimiter-is-the-whole-line", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "cat <<EOF\nline\nEOF x\necho \"st=$?\"\n",
		Why:        "the delimiter is compared against the *physical line as written*, so `EOF x` is body and not a terminator — unanimously, in a shape that would read as a terminator to anything matching a prefix. The body then runs to the end of the input, which is why the last line is printed rather than run, and bash 5.3 alone remarks that the document ended at end of file where bash 3.2 says nothing. Prior work of our own had this as a rule about prefixes, and the prefix reading is exactly what is false",
	},
	{
		ID: "unterminated/a-substitution-is-quoted-back-with-its-word", Category: "diagnostics", SyntaxError: true,
		Script:  true,
		Snippet: "echo a$(echo hi\necho after\n",
		Why:     "the shell that quotes the offending text back quotes the **word** the construct was written in, and this is the shape that says so: `a$(echo hi` and not `$(echo hi`. The assignment-prefix rows say the same thing and are read equally well as naming the *command*, which they are not — here the command is `echo` and no shell names it",
	},
	{
		ID: "unterminated/a-substitution-in-a-word-of-its-own-quotes-only-itself", Category: "diagnostics", SyntaxError: true,
		Script:  true,
		Snippet: "echo one two $(echo hi\necho after\n",
		Why:     "the control the row above needs. Two words in front of the construct and neither is quoted, so what is quoted is the word and nothing before it — a rule about the command would have taken `echo one two $(echo hi` and a rule about the line would have taken the whole line",
	},
	{
		ID: "unterminated/a-word-is-not-cut-at-a-quoted-blank", Category: "diagnostics", SyntaxError: true,
		Script:  true,
		Snippet: "echo \"a b\"$(echo hi\necho after\n",
		Why:     "the blank inside the quotes does not end the word, so the quoted text reaches back past it: `\"a b\"$(echo hi`. It is why the word has to come from the scanner rather than from a search backwards through the source for the previous space, which is the plausible wrong implementation",
	},
	{
		ID: "heredoc/a-delimiter-that-closes-a-command-substitution", Category: "redirection",
		Script:  true,
		Snippet: "v=$(cat <<EOF\na\nEOF)\necho \"v=[$v] st=$?\"\n",
		Why:     "the one place a line that merely *begins* with the delimiter ends the body: `EOF)` inside `$( )`, where the parenthesis that closes the substitution is what follows it. bash and ksh93 take it and the body is `a`; dash and zsh refuse the whole construct, dash wanting the `)` and zsh naming the assignment. So the prefix rule the case above disproves is real for this one shape and in only two of the six — and `EOF junk` in the same position is body in every one of them, which is how the two shapes tell each other apart",
	},
	{
		ID: "heredoc/a-substitutions-delimiter-is-remarked-on-before-it-runs", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "false && v=$(cat <<EOF\na\nEOF)\necho done\n",
		Why:        "the remark about the same shape is a fact about *reading* rather than about running: the substitution is on the right of a `&&` that never reaches it, and the one shell that says anything still says it. Which is what makes it the parser's to produce and not the interpreter's",
	},
	{
		ID: "heredoc/a-second-delimiter-closes-the-substitution", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "v=$(cat <<A\nx\nA\ncat <<B\ny\nB)\necho \"v=[$v]\"\n",
		Why:        "the same shape reached through the *second* here-document: the first is delimited on a line of its own and the one whose delimiter carries the `)` is the one after it. The four that read a body from between the parentheses take it and `v` is both bodies; the two that read it from the whole input refuse the construct exactly as they refuse the single-document row. It is what tells a rule about the first document in a construct apart from a rule about each of them",
	},
	{
		ID: "heredoc/a-delimiter-closes-two-substitutions", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "v=$(echo $(cat <<E\nz\nE))\necho \"v=[$v]\"\n",
		Why:        "`E))` closes both, so the body would have to take two parentheses with it rather than one. Same split, which is what says the question is asked of every construct the body reaches rather than of the innermost one",
	},
	{
		ID: "heredoc/backquotes-cannot-be-taken-into-a-body", Category: "redirection",
		Script:  true,
		Snippet: "v=`cat <<E\nq\nE`\necho \"v=[$v]\"\n",
		Why:     "the control that says the question is about parentheses. A backquoted substitution ends at a mark a here-document body cannot contain, so the body never takes the closing delimiter and there is nothing to decide — all six run it and `v` is `q`, where the same three lines written with `$( )` and `E)` split the panel four to two",
	},
	{
		ID: "heredoc/a-nested-delimiter-names-the-programs-lines", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "echo pad\necho pad\nset -- $(echo $(cat <<E\nz\nE))\necho \"v=[$1]\"\n",
		Why:        "the nested shape moved down the file, which is the only way to see whether the remark's two lines survive being read by a parser of the *substitution's* text rather than of the program. Every un-nested row begins on line 1, where a relative line and an absolute one are the same number and a sub-parse that started counting at 1 looks right. No assignment prefix, so the one shell that quotes the offending word quotes the same word we do and this row measures the nesting and nothing else",
	},
	{
		ID: "heredoc/a-delimiter-closes-three-substitutions", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "set -- $(echo $(echo $(cat <<E\nz\nE)))\necho \"n=$# after\"\n",
		Why:        "a third level, because the remark has to travel one parse further to be seen and a carry that goes exactly one level would pass the two-level row. Each enclosing read succeeds where the innermost one failed, so the depth of the nesting is the depth of the carry. `echo` rather than `:`, which is what it was written with while the shell that cuts a quoted word at twenty bytes was still printing it whole here (#1024): the word is now thirty bytes and cut, so this row carries the elision as well as the carry, which is the shape #1095 wanted for it",
	},
	{
		ID: "heredoc/a-substitution-that-is-not-a-dollar-sign", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "cat <(cat <<EOF\na\nEOF)\necho done\n",
		Why:        "the same shape spelled the other way. What decides the remark is that the parentheses hold a *program*, not which sigil opened them — bash 5.3 says the same thing here as for `$( )`, bash 3.2 and ksh93 stay silent as they do there, and the two that have no process substitution refuse the line outright. The counter-case is arithmetic: `$(( a << b ))` is a shift and reading it as a program would invent a here-document",
	},
	{
		ID: "unterminated/a-quoted-word-longer-than-the-shell-prints", Category: "syntax errors", SyntaxError: true,
		Script:  true,
		Snippet: "v=$(echo one two three four five\necho after\n",
		Why:     "the word a construct ran out inside, made long enough to reach the limit the one shell that quotes it back cuts at. Thirty-one bytes against a limit of twenty, so that shell prints twenty and marks them and the other four are unmoved — two of them print the whole word in their own sentence and two name no text at all, which is what says the limit belongs to the rendering of this one dialect's `near` and not to every quoted text. Measured 2026-09-07: the mark is appended at exactly twenty as well, where nothing has been cut, so a row at the boundary is worth more than a row far past it and the unit test carries that one",
	},
	{
		ID: "unterminated/an-arithmetic-substitution-that-never-closes", Category: "syntax errors", SyntaxError: true,
		Script:  true,
		Snippet: "echo $((1+2\necho after\n",
		Why:     "the construct whose parentheses hold an *expression* rather than a program, which #1023 left worded by the lexer. All four dialects said the lexer's own sentence and none said what its shell says, and the panel gives four: the closer echoed back at the opener's line, `Missing '))'` at the line the input ran out on, the parenthesis blamed at the opener's line, and the word quoted at the end. The line is the reason this row exists rather than following from the `$(` one — the shell that reports `$(` at the line after the input's last reports this at line 1, so the two constructs disagree inside one column and a row that measured only the sentence would not have seen it",
	},
	{
		ID: "unterminated/an-arithmetic-substitution-in-the-older-spelling", Category: "syntax errors", SyntaxError: true,
		Script:  true,
		Snippet: "echo $[1+2\necho after\n",
		Why:     "the same failure from the same helper in the spelling only two of the panel have, and it is the row that says the closer belongs to the construct rather than to the wording: the shell that echoes the closer says `]` here and `)` for `$((`, so one sentence with the closer as a verb serves both and a sentence with `)` written into it would be wrong on this line. The other columns are the interesting half — the two shells without the construct do not refuse the line at all, one running it silently and the other accepting it with a warning about the `$`, which is a status of 0 where the two that have it stop",
	},
	{
		ID: "unterminated/a-quote-inside-a-substitution-that-never-closes", Category: "syntax errors", SyntaxError: true,
		Script:  true,
		Snippet: "echo $( echo \"hi\necho after\n",
		Why:     "which construct is blamed when they nest, with the quote *inside* the substitution — the arrangement that tells the two readings apart, since the row below has them the other way round and a rule of \"always the quote\" or \"always the substitution\" satisfies one and not the pair. bash, dash and ksh93 name the innermost here and the innermost there; zsh names the enclosing one in both, and prints a second line about the outer construct that we still do not. Ours blamed the substitution in every dialect because the scan that steps over a quote returned quietly when the input ran out (#1151)",
	},
	{
		ID: "unterminated/a-substitution-inside-a-quote-that-never-closes", Category: "syntax errors", SyntaxError: true,
		Script:  true,
		Snippet: "echo \"${x:-\"$( echo hi\necho after\n",
		Why:     "the same question with the nesting reversed and one construct deeper: a `$( )` inside a quote inside a `${ }`. Three of the four blame the substitution — exactly what they say for the un-nested `echo \"$( echo hi` — and the fourth blames the quote, which is what it says for that one too. So every column answers both shapes alike and ours answered them differently, which is the whole of the issue: the skip through the expansion's body reached end of input and said nothing, so the `${` scan blamed its own opener (#1151)",
	},
	{
		ID: "unterminated/a-substitution-in-an-expansions-body", Category: "syntax errors", SyntaxError: true,
		Script:  true,
		Snippet: "echo ${x:-$( echo hi\necho after\n",
		Why:     "the same nesting without the quote around it, which is what says the body of a `${ }` holds a substitution rather than text: the three that name the innermost name the `$(` here as well, and a scan that counted only braces reached the end of the file with the `${` still open and blamed that. It is also the row that guards the other half of the same change — a `}` written inside the substitution must not close the expansion",
	},
	{
		ID: "unterminated/a-process-substitution-that-never-closes", Category: "syntax errors", SyntaxError: true,
		Script:  true,
		Snippet: "cat <(echo hi\necho after\n",
		Why:     "the plainly unterminated shape of the construct the here-document rows reach the long way round. Every panel member words it, and no two alike: the two that read a program in parentheses either way say exactly what they say for `$(`, the third names the end of the file where it names an unmatched parenthesis for `$(`, the fourth quotes the word the construct began, and the one without the construct at all refuses the `(` on line 1. A refusal the lexer worded could be none of those",
	},
	{
		ID: "unterminated/an-output-process-substitution-that-never-closes", Category: "syntax errors", SyntaxError: true,
		Script:  true,
		Snippet: "cat >(echo hi\necho after\n",
		Why:     "the other direction, which no shell in the panel distinguishes — the same sentence and the same line from all six. It is the row that says the opener is carried into the diagnostic for what it is rather than as the one spelling somebody tested",
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
		// The subshell and the `>/dev/null` on it are what keep bash 3.2
		// out of the answer twice over. Its failed write leaves the line
		// queued on the shared output stream, so the next write to standard
		// output flushes the builtin's own text ahead of `st=` — here that
		// next write goes to /dev/null and the case measures the status
		// rather than the leftovers. Everything the builtin says still
		// reaches `e`.
		ID: "redir/pwd-writing-to-a-closed-descriptor", Category: "redirection",
		Snippet: `( pwd >&- ) 2>e >/dev/null; echo "st=$?"; cat e`,
		Why:     "the same question as `redir/a-write-to-a-closed-descriptor-fails` asked of a builtin that is not `echo`, and the panel does not answer it the same way: dash and both bash builds report the failed write and fail the command, and ksh93 and zsh both answer 0 in silence — where ksh93 *does* report `echo`'s. So whether a builtin's failed write fails the command is not one answer per shell; ksh93 gives one answer for `echo` and the opposite for `pwd`",
	},
	{
		ID: "redir/times-writing-to-a-closed-descriptor", Category: "redirection",
		Snippet: `( times >&- ) 2>e >/dev/null; echo "st=$?"; cat e`,
		Why:     "the third of the panel's answers on the same question: dash, bash 5.3 and ksh93 all fail the command, bash 3.2 and zsh do not, and ksh93 words it as a failure to *open* rather than to write — `cannot open [Bad file descriptor]`, with no builtin named. bash 3.2's 0 is its queued write rather than a decision: the text is still in its buffer when the builtin returns",
	},
	{
		ID: "redir/export-writing-to-a-closed-descriptor", Category: "redirection",
		Snippet: `( export >&- ) 2>e >/dev/null; echo "st=$?"; cat e`,
		Why:     "`export` with no operands writes the exported names, so it is a builtin whose output exists without being asked for — and the failed write is reported by dash and bash 5.3 and by nobody else. It is here because the listing itself was missing: writing nothing, this had no write to fail and answered 0 everywhere",
	},
	{
		ID: "redir/readonly-writing-to-a-closed-descriptor", Category: "redirection",
		Snippet: `readonly RO=1; ( readonly >&- ) 2>e >/dev/null; echo "st=$?"; cat e`,
		Why:     "the same for `readonly`, which lists the same way and had the same silence. The name is set first because a shell with nothing readonly writes nothing, and a case where the write never happens cannot say whether a failed one is reported",
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
		ID: "redir/great-amp-names-a-file", Category: "redirection",
		Snippet: `echo hi >&qq; echo "st=$? [$(cat qq 2>/dev/null)]"`,
		Why:     "the csh spelling four of the six kept: an unnumbered `>&` whose word is not a descriptor opens the word as a file. bash 5.3, bash 3.2 and bash-as-`sh` write `hi` into `qq` and report 0; zsh does the same; ksh93 refuses the word as a bad file unit number and makes nothing; dash refuses it while parsing, so not even the first command runs",
	},
	{
		ID: "redir/great-amp-names-a-file-takes-both-streams", Category: "redirection",
		Snippet: `sh -c 'echo O; echo E >&2' >&qq; printf "[%s]" "$(cat qq 2>/dev/null)"`,
		Why:     "and it is `&>` exactly, not `>`: the shells that open the file put *both* output streams in it, which is the half a reading of `>&` as a plain redirect would get wrong and nothing else in the corpus would notice",
	},
	{
		ID: "redir/great-amp-with-a-number-in-front-is-not-a-file", Category: "redirection",
		Snippet: `echo hi 2>&qq; echo "st=$? [$(cat qq 2>/dev/null)]"`,
		Why:     "the leading number is the whole of the question. bash writes the file for the bare spelling and calls this one an ambiguous redirect; zsh takes it as a redirect of that one stream; ksh93 refuses it the same way it refuses the bare one",
	},
	{
		ID: "redir/less-amp-is-never-a-file", Category: "redirection",
		Snippet: `echo x > qq; cat <&qq; echo "st=$?"`,
		Why:     "the reading side of the same operator, which no shell opens as a file — and three sentences for one refusal: bash calls the redirect ambiguous, ksh93 calls the unit number bad, zsh says a file number was expected and names nobody",
	},
	{
		ID: "redir/an-amp-target-that-came-to-nothing", Category: "redirection",
		Snippet: `echo hi >&""; cat <&""; echo "st=$?"`,
		Why:     "what a script reaches when it writes `>&\"${COPROC[1]}\"` in a shell with no such array, and where the two shells that open a file part company: zsh takes the empty word as a name and fails on the empty path, bash takes it as a descriptor and calls it bad — naming the target as it was *written*, quotation marks and all, where bash 3.2 names the descriptor number instead",
	},
	{
		ID: "redir/an-amp-target-that-came-to-nothing-on-a-builtin", Category: "redirection",
		Snippet: `read -r l <&""; echo "after st=$?"`,
		Why:     "the same refusal on a command that runs *in* the shell, which is where zsh alone stops: nothing after it runs. The external command in the row above survives it there, so the boundary is the command and not the redirection",
	},
	{
		ID: "redir/noclobber-refuses-both-streams-to-one-file", Category: "redirection",
		Snippet: `set -C; : > qq; true &>qq; echo "st=$?"`,
		Why:     "`set -C` refuses this truncation exactly as it refuses a plain `>`, unanimously and each in its own words — and unlike `>` there is no override spelling to exempt, because `>|&` is a syntax error in all six. A command with no output on purpose: the two shells that have no `&>` read the line as a background `true` and a bare `>qq`, and anything the job printed would arrive against the clock",
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
		ID: "cmd/subshell-isolates-a-function-definition", Category: "command language",
		Snippet: `(g(){ echo in; }); if command -v g >/dev/null 2>&1; then echo leaked; else echo gone; fi`,
		Why:     "the row above asks it of a variable and this one of a *function*, which is the same question and was a different answer here: the function table was shared with a subshell where every variable table was copied, so a definition made in one was the parent's afterwards — at status 0, with a `command -v` that found it and no way for the caller to tell (#1125). Unanimous across the panel. `command -v` rather than `type` because the four word a `type` differently and the subject is whether the name is there at all",
	},
	{
		ID: "cmd/subshell-isolates-a-function-redefinition", Category: "command language",
		Snippet: `g(){ echo old; }; (g(){ echo new; }); g`,
		Why:     "the shape a script writes on purpose — run a helper in `( … )` so the version it needs stays out of the way — and the one where sharing the table is worst, because there is no diagnostic and no status to notice: the parent simply calls the subshell's function. Separate from the definition row because a name the parent already holds takes a different road into the table",
	},
	{
		ID: "cmd/subshell-isolates-a-function-removal", Category: "command language",
		Snippet: `f(){ echo yes; }; (unset -f f); f`,
		Why:     "the other direction, and the half a measurement of definitions alone would miss: with the table shared, a removal in a subshell took the *parent's* function away, so this printed a command-not-found where every shell in the panel prints `yes`. One shared map is two bugs, and they point opposite ways",
	},
	{
		ID: "cmd/subshell-isolates-an-alias-definition", Category: "command language",
		Snippet: `alias z=echo; (alias q=ls); if alias q >/dev/null 2>&1; then echo leaked; else echo gone; fi`,
		Why:     "the same for the alias table, where the consequence is a *parse* difference rather than a value one — an alias defined in a subshell changes how the parent's later lines are read. The `alias z=echo` first is load-bearing and not scene-setting: the table is allocated on first use, so with no alias anywhere the subshell built a map of its own and the leak did not appear at all, which is why it hid for so long",
	},
	{
		ID: "cmd/subshell-isolates-an-alias-removal", Category: "command language",
		Snippet: `alias z=echo; (unalias z); if alias z >/dev/null 2>&1; then echo there; else echo gone; fi`,
		Why:     "and the removal direction for aliases, for the reason the function removal row exists: a shared table loses the parent's alias to an `unalias` that was meant to be thrown away with the subshell",
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
		ID: "core/a-while-loop-answers-with-its-body", Category: "command language",
		Snippet: `i=0; while [ $i -lt 1 ]; do i=1; false; done; echo "st=$?"`,
		Why:     "the other half of the zero-iterations rule, and the half a shell written only for that one loses: once the body has run, the loop's status is the body's last command. Unanimous, and POSIX says the same",
	},
	{
		ID: "core/a-loop-answers-with-its-body-and-not-its-condition", Category: "command language",
		Snippet: `i=0; while [ $i -lt 1 ]; do i=1; true; done; echo "st=$?"`,
		Why:     "the row that tells the two readings apart, which the one above cannot: here the condition that ended the loop is *false* and the body's last command succeeded, so a shell reporting the condition would say 1 and every shell in the panel says 0. Both rows are needed — with `false` in the body the condition and the body agree, and the wrong answer looks right",
	},
	{
		ID: "core/an-until-loop-answers-with-its-body", Category: "command language",
		Snippet: `i=0; until [ $i -ge 1 ]; do i=1; false; done; echo "st=$?"`,
		Why:     "the same rule for `until`, whose condition is inverted and whose status is not. Written out rather than assumed from the `while` row, because inverting the test is exactly where an implementation might invert the answer too",
	},
	{
		ID: "core/a-loop-that-never-ran-does-not-inherit", Category: "command language",
		Snippet: `false; while false; do :; done; echo "st=$?"`,
		Why:     "zero iterations is 0 rather than whatever the shell's status happened to be, which `cmd/loop-status-when-body-never-runs` cannot show because nothing preceded the loop there. It is the guard on the row above: a shell that stopped resetting the status would pass that one and fail this",
	},
	{
		ID: "core/a-c-style-loop-answers-with-its-body", Category: "command language",
		Snippet: `for ((i=0;i<1;i++)); do false; done; echo "st=$?"`,
		Why:     "the same rule for the arithmetic header, in the four shells that have one. Written out rather than assumed from the list `for`, because it is a separate clause with a condition of its own and the condition is evaluated once more after the last iteration — the same shape that lost the answer in the conditional loops",
	},
	{
		ID: "core/the-status-a-loop-body-starts-from", Category: "command language",
		Snippet: `false; for i in a b; do echo "it=$?"; done`,
		Why:     "what `$?` is *inside* a loop before the body has set one, which is a different question from what the loop reports and is answered by the same line of an implementation. The first iteration sees the command before the loop and the second sees the first iteration's `echo`, so this prints 1 then 0 — a shell that zeroes the status on the way in prints 0 twice and still passes every row about what the loop reports",
	},
	{
		ID: "core/a-loop-condition-sees-the-status-before-it", Category: "command language",
		Snippet: `false; while [ $? -eq 0 ]; do echo ran; break; done; echo end`,
		Why:     "the same question asked of the condition rather than the body, and it is the sharper one because the answer changes what runs: the condition tests the status of the command before the loop, so it is false and the body never runs. A shell that zeroes the status before the first test prints `ran` — a loop that runs where no shell in the panel runs one",
	},
	{
		ID: "core/a-break-is-a-command-of-the-body", Category: "command language",
		Snippet: `i=0; while [ $i -lt 3 ]; do i=$((i+1)); false; break; done; echo "st=$?"`,
		Why:     "a loop left by `break` is not an exception to the rule but an instance of it: `break` is the last command the body ran and it succeeded, so the answer is 0 even though the command before it failed. Unanimous",
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
		ID: "cmd/function-name-with-a-colon", Category: "command language",
		Snippet: `:f(){ echo ok; }; :f; echo after`,
		Why:     "the leading colon a plugin manager's whole namespace is written with — `:zi-reload-and-run` — and the same four answers as the dash: bash and zsh define and run it, dash refuses the name at parse time, ksh93 parses and stops the script at the definition",
	},
	{
		ID: "cmd/function-name-with-a-plus", Category: "command language",
		Snippet: `f+g(){ echo ok; }; f+g; echo after`,
		Why:     "the second mark of the measured class, and the one that says this is not about `-` and `.`: the punctuation a function name may carry is a character class, and a dialect either has it or refuses every member",
	},
	{
		ID: "cmd/function-name-with-a-percent", Category: "command language",
		Snippet: `a%b(){ echo ok; }; a%b; echo after`,
		Why:     "the third, and the one nearest a job specification — `%` is a job only where a job may stand, and in a name it is a name",
	},
	{
		ID: "cmd/function-keyword-name-with-punctuation", Category: "command language",
		Snippet: `function :f { echo ok; }; :f; echo after`,
		Why:     "the same name in the keyword form, which is a separate production and had to be measured separately; dash has no keyword at all, so its refusal here is a different sentence from the one it gives the parens",
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
	{
		ID: "shape/case-pattern-with-an-empty-alternative", Category: "compound shapes", SyntaxError: true,
		Snippet: "case \"\" in (|https|git) echo empty-matched;; *) echo no;; esac\ncase git in (|https|git) echo named-matched;; *) echo no;; esac\ncase ftp in (|https|git) echo m;; *) echo unmatched;; esac",
		Why:     "an alternative of the pattern list written as **nothing**, so the arm matches the empty subject as well as the named ones — the idiom for \"one of these schemes, or none\", and the line `~/.zi/bin/lib/zsh/install.zsh` stops at. One shell accepts it and the other four refuse in three wordings at two statuses, each blaming the `|`. All three subjects are in one row because an implementation that accepted the line and matched only the *named* alternatives would pass a row that asked about parsing alone (#1083)",
	},
	{
		ID: "shape/case-pattern-list-ending-in-a-separator", Category: "compound shapes", SyntaxError: true,
		Snippet: `case a in (a|b|) echo trailing;; *) echo no;; esac`,
		Why:     "the emptiness at the other end, which is not a separate feature in the shell that has it — a `|` may have no pattern after it as well as none before it. It is also where the refusals stop agreeing about the token: bash and ksh93 blame the `)`, and dash blames a *word* and names the paren as what it expected, because one operator may stand where any pattern belongs there and the slot swallows the `)` before the complaint is raised",
	},
	{
		ID: "shape/case-pattern-list-with-no-pattern-at-all", Category: "compound shapes", SyntaxError: true,
		Snippet: `case a in ) echo m;; *) echo no;; esac`,
		Why:     "the boundary the empty alternative must not be widened past: with no `|` there is nothing to read the emptiness off, and **every** shell refuses it — including the one that accepts an empty alternative in every position a separator can put one. A rule written as \"the pattern list may be empty\" passes the two rows above and fails this one",
	},
	{
		ID: "shape/an-operator-where-a-later-case-pattern-belongs", Category: "compound shapes", SyntaxError: true,
		Snippet: "case a in (a| ; |b) echo m-a;; *) echo no-a;; esac\ncase \"\" in (a| ; |b) echo m-empty;; *) echo no-empty;; esac",
		Why:     "one shell lets an operator stand where a pattern belongs, and it lets it stand where **any** of them belongs rather than only the first — measured at half its reach until this row. The arm keeps the patterns beside it, so it still matches `a`, and the operator contributes no pattern at all: the empty subject does not match, which is what separates this from the empty alternative another shell reads out of a bare `|`. The spaces around the `;` keep it a token of its own; written against the `|` it is one token in the shell that has `&|` and `;|`",
	},

	// --- [[ ]] and (( )) ------------------------------------------------------
	{
		ID: "cond/a-pattern-operand-may-start-with-a-group", Category: "[[ ]] and (( ))", SyntaxError: true,
		Snippet: `k=a; [[ $k == (a|b) ]] && echo hit || echo miss; k=c; [[ $k == (a|b) ]] && echo hit || echo miss; k=a; [[ $k = (a|b) ]] && echo hit || echo miss; k=a; [[ $k != (a|b) ]] && echo hit || echo miss`,
		Why:     "the commonest idiom in one shell's completion files, and a syntax error in the other four. A bare group already belonged to a word *mid*-pattern there; what was missing is the first character, since `(` is in the operator table and a token beginning with one never reaches the word scanner. All three pattern operators take it, which is what says the rule is about the operand being a pattern rather than about `==` (#826)",
	},
	{
		ID: "cond/a-group-may-be-followed-by-more-pattern", Category: "[[ ]] and (( ))", SyntaxError: true,
		Snippet: `k=abc; [[ $k == (a|b)* ]] && echo hit || echo miss; k=xbc; [[ $k == (a|b)* ]] && echo hit || echo miss; k=ab; [[ $k == ((a|b)|x)(b|c) ]] && echo hit || echo miss`,
		Why:     "the group is a piece of the pattern rather than the whole of it, and it nests. The nested one is the sharp half: a group inside a group starts `((`, which is the one place in the grammar where two parentheses are a single token, so a pattern operand has to be recognized before an arithmetic command is",
	},
	{
		ID: "cond/a-group-is-one-word-whitespace-and-all", Category: "[[ ]] and (( ))", SyntaxError: true,
		Snippet: `k="a b"; [[ $k == (a b) ]] && echo hit || echo miss; k=a; [[ $k == (a b) ]] && echo hit || echo miss`,
		Why:     "the scanner takes everything to the matching `)` rather than stopping at a blank, so a space inside a group is part of the pattern and matches a space in the subject. It is the rule a group already had, reached from a position it could not be reached from before",
	},
	{
		ID: "cond/a-pattern-operands-group-ends-at-an-operator", Category: "[[ ]] and (( ))", SyntaxError: true,
		Snippet: `k='a<b'; [[ $k == (a<b) ]] && echo hit || echo miss; echo after`,
		Why:     "the exception to `a-group-is-one-word-whitespace-and-all`: the scanner takes a blank and stops at `;`, `<`, `>` or `&`, so the word ended at the `<` and the rest of the group was left with nowhere to go. Every column refuses it and the row is about *where* — the shell with bare groups blames the `<`, the three bashes blame the `(` they could not read as an operand, ksh93 blames the `(` as an operator and dash has no `[[ ]]` at all. It parsed here and matched until #1175, which is the guard this pins",
	},
	{
		ID: "cond/a-numeric-range-may-start-a-pattern-group", Category: "[[ ]] and (( ))", SyntaxError: true,
		Snippet: `k=5.9; [[ $k == (5.<1->*|<6->.*) ]] && echo hit || echo miss; k=6.1; [[ $k == (5.<1->*|<6->.*) ]] && echo hit || echo miss; k=4.3; [[ $k == (5.<1->*|<6->.*) ]] && echo hit || echo miss`,
		Why:     "the version gate every powerlevel10k config opens with, and the whole of its configuration never loaded because of it. A range parses bare and a group parses without one; the range *inside* a group that starts the operand did not, because the scanner for a word-leading group stops at `;`, `<`, `>` and `&` and the range's `<` is none of those — it is pattern text. Eight of the twenty-two remaining parse failures in a real plugin tree were this one line (#1217)",
	},
	{
		ID: "cond/a-leading-groups-numeric-range-has-four-shapes", Category: "[[ ]] and (( ))", SyntaxError: true,
		Snippet: `k=6; [[ $k == (<->) ]] && echo bare || echo no; [[ $k == (<1-9>) ]] && echo both || echo no; [[ $k == (<6->) ]] && echo lower || echo no; [[ $k == (<-9>) ]] && echo upper || echo no; k=x6; [[ $k == (x(<6->)) ]] && echo nested || echo no`,
		Why:     "all four spellings reach the range lexer from inside a leading group, and so does a group nested in one — the nested row is the sharp half, because the outer group is what the word-leading scanner reads and it has to carry the inner one's `<` through. Only the shell with bare pattern groups has ranges at all, so the other five refuse the `(` as they always did",
	},
	{
		ID: "cond/a-leading-groups-range-shape-is-exact", Category: "[[ ]] and (( ))", SyntaxError: true,
		Snippet: `k=x; [[ $k == (<a-b>) ]] && echo hit || echo miss; echo after`,
		Why:     "the discriminating half of `a-numeric-range-may-start-a-pattern-group`: `<` digits `-` digits `>` is the entire disambiguation, so a `<` that is not one still ends the word exactly as `a-pattern-operands-group-ends-at-an-operator` records. Measured on zsh 5.9.2 — `(<a-b>)`, `(<1-2-3>)`, `(<-->)` and `(<1)` are all still parse errors there, so admitting the range must not admit them. Written with `<a-b>` because it is the shape a reader is most likely to assume works",
	},
	{
		ID: "cond/a-regex-operands-group-keeps-its-operators", Category: "[[ ]] and (( ))",
		Snippet: `k='a<b'; [[ $k =~ (a<b) ]]; echo "lt=$?"; k2='a;b'; [[ $k2 =~ (a;b) ]]; echo "semi=$?"; [[ ab =~ (a<b) ]]; echo "no=$?"`,
		Why:     "and the operand that does *not* end there, which is the pair that makes the rule a rule about two constructs rather than about parentheses. A regular expression owns its `<` and its `;`: all four shells with `=~` match `a<b` and `a;b` against those groups and fail `ab` against the first, so the characters are regex text and not a redirection or a terminator. zsh is the dissenter and refuses the `<` while parsing, which is where its wording layer answers; dash has no `[[ ]]`. Written beside the pattern operand's row deliberately — the two differ in nothing a reader can see, and the shells still separate them (#1175)",
	},
	{
		ID: "cond/a-quoted-group-is-a-literal", Category: "[[ ]] and (( ))",
		Snippet: `k=a; [[ $k == "(a|b)" ]] && echo hit || echo miss; k="(a|b)"; [[ $k == "(a|b)" ]] && echo hit || echo miss`,
		Why:     "the same per-span quoting that decides whether `a*` is a pattern decides whether a group is one, so this parses everywhere and matches nowhere but against the literal text. It is the case that keeps the fix from being `a group is always a group`",
	},
	{
		ID: "cond/a-group-is-the-patterns-and-not-the-conditions", Category: "[[ ]] and (( ))", SyntaxError: true, GradedOnRefusal: true,
		Snippet: `[[ -n (a|b) ]]; echo "st=$?"`,
		Why:     "the boundary, and it is refused in the shell that takes the rows above as well as in the four that take none of them: a group is read where a *pattern* is read and nowhere else in the construct, so `-n`'s operand is not one. Graded on the refusal because five shells decline in five wordings; what is being pinned is that all five decline",
	},
	{
		ID: "cond/a-group-may-not-start-the-left-operand", Category: "[[ ]] and (( ))", SyntaxError: true, GradedOnRefusal: true,
		Snippet: `k=a; [[ (a|b) == $k ]]; echo "st=$?"`,
		Why:     "the other side of the same boundary: the left operand of `==` is not a pattern, so a `(` there is the condition-grouping parenthesis it has always been and the `|` inside it has nothing to belong to. Together with the row above this is what makes the flag a fact about the *operand* rather than about `[[ ]]`",
	},
	{
		ID: "cond/an-empty-group-is-not-a-pattern", Category: "[[ ]] and (( ))", SyntaxError: true, GradedOnRefusal: true,
		Snippet: `k=a; [[ $k == () ]]; echo "st=$?"`,
		Why:     "`()` is a function definition's parentheses everywhere it can be, which is why a bare group never takes an empty one — the rule that keeps `f() { …; }` parsing in the dialect with bare groups, asked here in the one position where a group is otherwise read",
	},
	{
		ID: "cond/a-paren-after-a-condition-is-still-a-subshell", Category: "[[ ]] and (( ))",
		Snippet: `[[ 1 == 1 ]] && (echo x); [[ 1 == 1 ]]; (echo y)`,
		Why:     "the boundary of how a pattern operand is read, from outside the construct: whatever the lexer is told about the operand of `==` has to stop being true at the `]]`, or every `(` after a condition is scanned as part of a word and `&& (echo x)` becomes a command named `(echo x)` — which still parses, which is what makes it invisible. Found by mutation rather than by a shell (#826)",
	},
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
	{
		ID: "cond/touching-grouping-parens", Category: "[[ ]] and (( ))",
		Snippet: `[[ ((1 -eq 1)) ]]; echo "st=$?"`,
		Why:     "inside [[ ]] no command may begin, so `((` is two grouping parentheses and not the arithmetic command — unanimous in bash 3.2 and 5.3, bash-as-sh, ksh93 and zsh, and the one line a real ~/.bashrc in the wild tripped over (#859)",
	},
	{
		ID: "cond/spacing-the-grouping-parens-changes-nothing", Category: "[[ ]] and (( ))",
		Snippet: `[[ ( (1 -eq 1) ) ]]; echo "st=$?"`,
		Why:     "the spaced form of the case above. It is the pair that makes the finding: a lexer that reads `((` as one token answers these two differently, and no shell does",
	},
	{
		ID: "cond/touching-grouping-parens-after-oror", Category: "[[ ]] and (( ))",
		Snippet: `[[ (1 -eq 1) || ((2 -eq 2) && (3 -eq 3)) ]]; echo "st=$?"`,
		Why:     "the shape that appears in the wild: a group after || whose first element is itself a group. Each structural position has to be checked separately because each is a different production",
	},
	{
		ID: "cond/touching-grouping-parens-after-andand", Category: "[[ ]] and (( ))",
		Snippet: `[[ 1 -eq 1 && ((2 -eq 2)) ]]; echo "st=$?"`,
		Why:     "the same after &&",
	},
	{
		ID: "cond/touching-grouping-parens-after-not", Category: "[[ ]] and (( ))",
		Snippet: `[[ ! ((1 -eq 2)) ]]; echo "st=$?"`,
		Why:     "the same after !, which is the third place a condition may begin",
	},
	{
		ID: "cond/touching-grouping-parens-before-a-unary", Category: "[[ ]] and (( ))",
		Snippet: `[[ (( -z "" )) ]]; echo "st=$?"`,
		Why:     "`(( -z` is the shape that most looks like an arithmetic command and least is one: an arithmetic expression cannot start with `-z` at all, and every shell still reads two groups",
	},
	{
		ID: "cond/three-touching-grouping-parens", Category: "[[ ]] and (( ))",
		Snippet: `[[ (((1 -eq 1))) ]]; echo "st=$?"`,
		Why:     "nesting is unbounded rather than a one-deep special case, so a fix that only looks one character ahead is caught here",
	},
	{
		ID: "cond/a-regex-group-that-opens-with-a-group", Category: "[[ ]] and (( ))",
		Snippet: `[[ xa =~ ((a)) ]]; echo "st=$?"`,
		Why:     "the =~ operand owns its parentheses, so `((` there is the regular expression's and not the shell's either — the same confusion reached by a different route",
	},
	{
		ID: "cond/the-arithmetic-command-survives-the-condition", Category: "[[ ]] and (( ))",
		Snippet: `x=0; [[ 1 -eq 1 ]] && (( x++ )); echo "x=$x"`,
		Why:     "`((` is only two parentheses *inside* the brackets. One character past `]]` it is the arithmetic command again, which is what a fix written as a lexer mode has to give back",
	},
	{
		ID: "arith/a-command-expression-may-open-with-a-paren", Category: "arithmetic",
		Snippet: `(( (1+2)*3 == 9 )); echo "st=$?"`,
		Why:     "the other direction of the same ambiguity: at command position `(( (` is an arithmetic command whose expression is parenthesized, not three nested subshells. dash, which has no arithmetic command, is the contrast",
	},
	{
		ID: "arith/two-parens-at-command-position-are-not-a-subshell", Category: "arithmetic",
		Snippet: `((echo hi)); echo "st=$?"`,
		Why:     "outside a condition the distinction really is textual: with no space the three shells that have (( )) read an arithmetic command and fail on `echo`, while dash runs the nested subshell and prints hi. It is not a syntax error in any of them — the expression is read when the command runs, so the failing one is a failed command and the script goes on to print its status (#865)",
	},
	{
		ID: "arith/a-command-in-a-branch-that-never-runs", Category: "arithmetic",
		Snippet: `false && ((echo hi)); echo "reached st=$?"`,
		Why:     "the consequence of *when* the expression is read, rather than of when a message arrives: on the right of a `&&` that never reaches it, so nothing ever reads it. All six print `reached st=1` — including dash, whose two subshells are equally unreached — which is what makes reading it while reading the file a program taken down for a command it was never going to run (#865)",
	},
	{
		ID: "arith/a-substitution-in-a-branch-that-never-runs", Category: "arithmetic",
		Snippet: `false && echo "$((echo hi))"; echo "reached st=$?"`,
		Why:     "the same, through the spelling dash has as well — so the row is unanimous for one reason rather than for two. `$(( ))` is in every shell in the panel and none of them reads the expression before the command that carries it runs",
	},
	{
		ID: "arith/a-for-header-in-a-branch-that-never-runs", Category: "arithmetic",
		Snippet: `false && for ((echo hi;;)); do :; done; echo "reached st=$?"`,
		Why:     "the third construct that carries an expression, and it defers too: bash, bash 3.2, ksh93 and zsh all print `reached st=1`. dash has no C-style `for` and refuses the line where the `(` is, which is the same answer it gives for the construct anywhere",
	},
	{
		ID: "arith/a-substitution-that-will-not-read-is-fatal", Category: "arithmetic",
		Snippet: `echo "$((echo hi))"; echo "reached st=$?"`,
		Why:     "and when it is reached, the two spellings part on how far the damage goes: a failed *expansion* ends the script in bash, ksh93 and zsh alike, so the line after it never runs, where the row above's failed arithmetic *command* is one failed command and the script carries on. dash reports its own wording and stops at 2",
	},
	{
		ID: "cmd/a-subshell-that-opens-with-a-subshell", Category: "command language",
		Snippet: `( (echo hi) ); echo "st=$?"`,
		Why:     "one space is the whole difference from the case above, and it is unanimous — including in the shells that would otherwise have taken the two parentheses as one token",
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
		ID: "param/a-length-in-a-single-byte-locale", Category: "parameter expansion",
		Snippet: `s=héllo; t=日本語; echo "[${#s}][${#t}]"`,
		Why:     "the length of a string holding characters wider than a byte, under the locale every case here runs in. Unanimous at the byte count — 6 and 9 — dash and the four multibyte shells alike, which is the half of #899 that makes the other half readable: the panel does not divide over whether a character is a byte, it divides over whether the locale is consulted",
	},
	{
		ID:       "param/a-length-in-a-utf8-locale",
		Category: "parameter expansion",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `s=héllo; t=日本語; echo "[${#s}][${#t}]"`,
		Why:      "the same snippet with the codeset changed and nothing else, which is where the panel divides: bash, ksh93 and zsh count characters — 5 and 3 — and dash counts bytes in every locale there is, having no multibyte decoder. Silent on both sides, since either answer is a plausible number. The `MultibyteEncodingIsHonored` axis; the locale itself is runtime state and not a second axis, which the pair of these two cases is what shows",
	},
	{
		ID:       "param/a-length-counts-an-invalid-byte-as-one",
		Category: "parameter expansion",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `s=$(printf 'a\200b'); echo "[${#s}]"`,
		Why:      "a byte that begins no valid sequence, under the locale that decodes: 3 unanimously, so an undecodable byte is one character of its own rather than an error, a skipped byte or a replacement character. The trap is real in both directions — a decoder that folded it into U+FFFD would answer 3 with a value nothing put there, and one that rejected the replacement rune answered 0 for `$((##\\x80))` in #855",
	},
	{
		ID: "param/a-substring-in-a-single-byte-locale", Category: "parameter expansion",
		Snippet: `s=héllo; echo "[${s:1:2}][${s:3}]"`,
		Why:     "a substring's offset and length count the same units its shell's `${#s}` does, so under the fixed locale they are bytes and `${s:1:2}` is the two bytes of one character. Written to keep both halves valid text on either side of the axis, which `${s:2}` would not be",
	},
	{
		ID:       "param/a-substring-in-a-utf8-locale",
		Category: "parameter expansion",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `s=héllo; echo "[${s:1:2}][${s:3}]"`,
		Why:      "the same offsets over characters: `él` and `lo` where the byte reading gives `é` and `llo`. The pair is what pins that the offset moves with the length rather than being a separate decision — a fix that counted `${#s}` in characters and indexed the string in bytes would pass the length case and cut this one in half",
	},
	{
		ID:       "param/a-substring-from-the-end-in-a-utf8-locale",
		Category: "parameter expansion",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `s=héllo; echo "[${s: -4}][${s: -4:2}][${s:1:-1}]"`,
		Why:      "the negative offset and the negative length counted in characters, which is where an implementation that converts one end and not the other comes apart: a negative offset added to a byte length lands in the middle of a character. ksh93 keeps its own answer for a negative *length* — nothing — under any locale, so this also holds `SubstringNegativeLengthIsEmpty` still where it was",
	},
	{
		ID:       "param/a-length-of-an-array-element-in-a-utf8-locale",
		Category: "parameter expansion",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `a=(héllo 日本語); echo "[${#a[@]}][${#a[0]}][${#a[1]}]"`,
		Why:      "the count and the width in one line: `${#a[@]}` is 2 in every shell with arrays whatever the locale, and `${#a[N]}` is the length of one element and moves with the codeset. The two questions share a spelling and one of them is not a length at all, so a change to the length machinery has to leave the count alone",
	},
	{
		ID: "param/a-pattern-trims-the-same-either-way", Category: "parameter expansion",
		Snippet: `s=héllo; echo "[${s%??}][${s%%?}]"`,
		Why:     "the suffix probe #899 cites as evidence that pattern matching is already rune-aware, pinned so that it stays where it is. It is unanimous, and it is also *not* discriminating: `héllo` ends in two ASCII characters, so two bytes and two characters off the end leave the same four bytes. `${s#???}` from the other end is what separates the readings, and is the pair of cases below (#905)",
	},
	{
		ID:       "param/a-pattern-trims-characters-in-a-utf8-locale",
		Category: "parameter expansion",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `s=héllo; echo "[${s%??}][${s#???}]"`,
		Why:      "the suffix probe beside the prefix one that separates the readings. Two characters off the end is the same four bytes either way, so `${s%??}` agrees with everyone; three off the front is `lo` here and `llo` under a single-byte locale, and dash gives `llo` in both, having no decoder to consult the locale with. Paired with the single-byte row below so that a matcher which simply always counted characters would fail one of them (#905)",
	},
	{
		ID:       "param/a-pattern-trims-bytes-in-a-single-byte-locale",
		Category: "parameter expansion",
		Snippet:  `s=héllo; echo "[${s%??}][${s#???}]"`,
		Why:      "the other half of the pair above, under the locale every case here runs in: three off the front is three *bytes*, `llo`, in all six including the four that answer `lo` when the locale names a multibyte encoding. Without it a matcher that counted characters unconditionally would look right — which is what the scalar subscript did before #899",
	},
	{
		ID:       "param/a-replacement-runs-over-characters",
		Category: "parameter expansion",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `s=日本語; t=héllo; echo "[${s//?/X}][${t/??/X}]"`,
		Why:      "replacement walks the subject looking for a match at each position, so it has the same unit question the trim has and one more: where it *restarts*. Three characters become three X here and nine under a single-byte locale, and the anchored form has to leave `llo` rather than a continuation byte in front of it — a scan that found matches by character and advanced by byte would answer neither",
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
		ID: "param/the-tilde-flag-is-one-dialects", Category: "parameter expansion",
		Snippet: `touch inn1 inn2; g='inn*'; printf "[%s]" ${~g}; echo`,
		Why:     "a `~` between the `${` and the parameter makes the substituted value eligible for filename generation: zsh matches the pattern where bash and dash call the whole expansion a bad substitution when it is reached and ksh93 refuses it while reading with the `~` named — the same three-way split every unreadable expansion follows. ~/.zi/bin/zi.zsh uses it eighteen times in nineteen lines and a refusal leaves eighteen variables empty",
	},
	{
		ID: "param/the-tilde-flag-quoting-suppresses-it", Category: "parameter expansion",
		Snippet: `touch inn1 inn2; g='inn*'; printf "[%s]" "${~g}" ${~g}; echo`,
		Why:     "quoting suppresses the flag exactly as it suppresses ordinary filename generation, so the quoted form is the value unchanged and the unquoted one splits into as many words as the pattern matched — the two halves in one row, because a fix that produced the matches in both would look right in half the record",
	},
	{
		ID: "param/the-tilde-flag-doubled-turns-it-off", Category: "parameter expansion",
		Snippet: `touch inn1 inn2; g='inn*'; printf "[%s]" ${~g} ${~~g} ${~~~g}; echo`,
		Why:     "the count is parity and not a toggle of the option: one tilde is on, two are off, three on again, and measured under GLOB_SUBST both ways the answer is the same — `${~~name}` is how a nested use says \"not here\"",
	},
	{
		ID: "param/the-tilde-flag-expands-a-tilde", Category: "parameter expansion",
		Snippet: `t='~/zz'; echo ${~t} | sed "s|$HOME|H|g"; echo ${t} | sed "s|$HOME|H|g"`,
		Why:     "the other half of the same flag: the value's leading tilde becomes a home directory, which no shell in the panel does to an expansion's result on its own — the second line is the same value without the flag, so the row pins the difference rather than the machine's HOME",
	},
	{
		ID: "param/the-tilde-flag-marks-a-pattern-operand", Category: "parameter expansion",
		Snippet: "p='a*'\ncase abc in ${~p}) printf hit;; *) printf miss;; esac\ncase abc in ${p}) printf hit;; *) printf miss;; esac\nv=abc\nprintf \"[%s]\" \"${v#${~p}}\" \"${v#${p}}\"\necho",
		Why:     "the pattern half is not about the filesystem: it is the same question that decides whether a `case` subject and a `${x#$p}` operand read a value's metacharacters as live, so the flag reaches all three or the implementation has three copies of one rule. One statement per *line* on purpose — a bad substitution abandons the rest of the line, so writing them with semicolons made the row about that abandonment instead, which two other rows already pin",
	},
	{
		ID: "param/the-split-flag-is-one-dialects", Category: "parameter expansion",
		Snippet: `f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; v="a b c"; f ${v}; f ${=v}; echo`,
		Why:     "an `=` between the `${` and the parameter splits the substituted value into words on IFS: zsh answers one field then three where bash and dash call the whole expansion a bad substitution when it is reached and ksh93 refuses it while reading with the `=` named — the same three-way split every unreadable expansion follows. Both readings in one row, because the field *count* is the whole of the behavior and a row printing only the text would read the same either way. `is-at-least`, the version predicate in zsh's own function library, is `${=1}` and `${=2:-$ZSH_VERSION}`, and a refusal there answers \"at least\" for every version at status 0",
	},
	{
		ID: "param/the-split-flag-reaches-through-quotes", Category: "parameter expansion",
		Snippet: `f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; v=" a "; f "${v}"; f "${=v}"; f ${=v}; echo`,
		Why:     "quoting does *not* suppress this flag, which is where it parts company with its sibling `${~spec}`: the quoted form splits, and it keeps the empty fields at the edges of the value that the unquoted form discards — one, three and one field. A fix that made the quoted form the value unchanged would look right beside the tilde flag and be wrong here",
	},
	{
		ID: "param/the-split-flag-doubled-turns-it-off", Category: "parameter expansion",
		Snippet: `f(){ printf "%d:" "$#"; }; v="a b c"; f ${=v}; f ${==v}; f ${===v}; echo`,
		Why:     "the count is parity and not a toggle of the option: one `=` is on, two are off, three on again, and measured under SH_WORD_SPLIT both ways the answer is the same — `${==name}` is how a nested use says \"not here\"",
	},
	{
		ID: "param/the-split-flag-does-not-reach-an-assignment", Category: "parameter expansion",
		Snippet: `v=" a  b "; x=${=v}; printf "[%s]" "$x"; case ${=v} in " a  b ") printf joined;; a) printf first;; esac; echo`,
		Why:     "the flag decides the *option's* question and not the context's, so a context that never splits is not overridden: an assignment's value and a `case` subject are the value unchanged, spaces and all. The two halves in one row because an implementation that forced the split everywhere would still print the right thing for one of them",
	},
	{
		ID: "param/the-set-test-flag-is-one-dialects", Category: "parameter expansion",
		Snippet: `v=1; e=; printf "[%s]" ${+v} ${+e} ${+NOPE}; echo`,
		Why:     "a `+` between the `${` and the parameter asks whether it is set and answers 1 or 0 without ever failing: zsh counts where bash and dash call the whole expansion a bad substitution when it is reached and ksh93 refuses it while reading with the `+` named. The empty-but-set name is in the row because set-ness is not emptiness — an implementation that answered like `${v:+1}` would get two of the three right",
	},
	{
		ID: "param/the-set-test-flag-does-not-trip-nounset", Category: "parameter expansion",
		Snippet: `set -u; printf "[%s]" ${+NOPE}; printf "[alive]"; echo`,
		Why:     "asking without tripping nounset is the whole of what the construct is for: it is the guard a script writes in front of everything else, so a shell that made it fatal would turn every guard into a stop. The `alive` is the half that matters — a `0` printed by a shell that then gave up looks the same without it",
	},
	{
		ID: "param/the-set-test-flag-has-no-effect-beside-an-operator", Category: "parameter expansion",
		Snippet: `v=abc; printf "[%s]" ${+v#a} ${+v:-x} ${+nope:-D}; echo`,
		Why:     "measured, and not what a flag suggests: with an operator written the `+` is neither the answer nor a refusal — the expansion is exactly what it would have been without it. Three rows because the first is the value trimmed, the second the value where the count would have been 1, and the third the default where the count would have been 0",
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
		ID: "param/prompt-percent-names-the-user", Category: "parameter expansion",
		Snippet: `[[ "${(%):-%n}" == "$(id -un)" ]] && echo matches-the-login-name || echo differs`,
		Why:     "`%n` is the user the shell runs as, and the row is written as a comparison rather than a name so the record is a fact about the escape and not about the machine that made it. The first two lines of a real ~/.zshrc build the prompt theme's cache path out of it, twice",
	},
	{
		ID: "param/prompt-percent-user-is-not-a-variable", Category: "parameter expansion",
		Snippet: `USER=someone-else; LOGNAME=someone-else; case "${(%):-%n}" in someone-else) echo follows-the-variable;; *) echo ignores-it;; esac`,
		Why:     "the half that decides how `%n` may be implemented: it is a fact about the *process*, not about the environment, so assigning `USER` or `LOGNAME` does not move it — and neither does injecting them before the shell starts, measured separately. Reading either name would make `env USER=someone-else zsh` draw the wrong person, and bash's `\\u` was measured to ignore them too",
	},
	{
		ID: "param/prompt-percent-refuses-an-escape-by-name", Category: "parameter expansion",
		Snippet: `echo "[${(%):-%zz}]"; echo "st=$?"`,
		Why:     "an escape outside the prompt language: zsh expands `%z` to nothing at all and carries on at 0, so the shell that has the construct is *quieter* here than this one, which refuses it by name. The refusal is the deliberate difference — a wrong answer wearing a success is worse than a complaint — and this row is where it is written down rather than left to be discovered",
	},
	{
		ID: "param/a-split-flag-keeps-the-field-at-each-edge", Category: "parameter expansion",
		Snippet: `x=":"; set -- "${(s.:.)x}"; echo "colon=$#"; x="a::b"; set -- "${(s.:.)x}"; echo "interior=$#"; x=":a:"; set -- "${(s.:.)x}"; echo "both=$#"`,
		Why:     "which empty field a quoted `(s)` keeps, put where one snippet separates the two answers: the field at each *edge* survives and the interior ones do not, so `\":\"` is two fields and `\"a::b\"` is two as well — the second of `a::b` being `b` and not the empty one between them. An implementation that drops every empty field passes the middle test and answers 0 for the other two, which is a count rather than a complaint. zsh alone has the flag; the other four read the parenthesis as text or as a bad substitution",
	},
	{
		ID: "param/a-split-flag-on-an-empty-value", Category: "parameter expansion",
		Snippet: `x=""; set -- "${(s::)x}"; echo "chars=$#"; set -- "${(f)x}"; echo "lines=$#"; set -- ${(s::)x}; echo "unquoted=$#"`,
		Why:     "an empty value split is one empty field in quotes and no field at all without them, and the two separators have to agree: splitting into characters answers the same 1 as splitting at newlines, where a character loop over an empty string naturally produces nothing. The quoted-versus-unquoted pair is on the same line because 1 and 0 are both defensible on their own and only the pair says which is which",
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
		ID: "cond/a-newline-before-the-operator", Category: "pattern matching",
		// `&&` rather than `||` for the reason the case above gives: dash
		// has no `[[`, so the clause fails and `&&` stops there.
		Snippet: `[[ -n x
&& -z "" ]] && echo yes`,
		Why: "the other side of the operator, and the one the list of structural points was missing: the condition is complete at the end of the line and the operator leads the continuation. It is how a real prompt theme is written, and at the newline it is not yet known whether an operator or the `]]` follows",
	},
	{
		ID: "cond/a-newline-before-the-operator-indented", Category: "pattern matching",
		Snippet: `[[ -n x
      && -z "" ]] && echo yes`,
		Why: "the same with the continuation indented, which is the form in the wild — the leading whitespace is not what makes it work, and the case exists so that a fix keyed on a line beginning with the operator would fail here",
	},
	{
		ID: "cond/a-newline-before-a-second-operand-is-refused", Category: "pattern matching",
		SyntaxError: true,
		Snippet: `[[ -n x
-z "" ]] && echo yes`,
		Why: "the boundary: skipping a newline before an operator must not become skipping one before anything at all. Every shell that has `[[ ]]` refuses two conditions with only a line between them, and each blames a different token on a different line",
	},
	{
		ID: "cond/a-semicolon-inside-a-condition", Category: "pattern matching",
		SyntaxError: true,
		Snippet:     `[[ -n x ; ]]; echo "st=$?"`,
		Why:         "a `;` where the condition wanted an operator or its `]]`, and the row that says a refusal names the *offending* token rather than what was wanted. Each of the three shells with `[[ ]]` words it in its own way and all three name the `;`. The `|` spelling of the same question is deliberately not a row beside it: dash has no `[[ ]]`, so there the words are a *pipeline* of two commands that do not exist, and which of the two `not found` lines arrives first is a race. bash writes two lines and the second is not the sentence it gives a stray token anywhere else — `syntax error near` without the words `unexpected token` — which is measurable only because it keeps them everywhere else",
	},
	{
		ID: "cond/an-unterminated-condition", Category: "pattern matching",
		SyntaxError: true,
		Snippet:     `[[ -n x`,
		Why:         "the `[[` the input ran out inside of, which is a different failure from a token the grammar did not want and is named differently by every shell that has the construct: ksh93 calls the `[[` unmatched, bash names it as the command the end of file came from *and* writes a line in front saying what it was looking for, and zsh blames the last word it read. `[[` is not a word the list parser stacks, so nothing filled either of the two verbs those wordings use and one of them came out as a hole",
	},
	{
		ID: "cond/a-condition-opened-on-one-line-and-refused-on-another", Category: "pattern matching",
		SyntaxError: true,
		Snippet: `[[ -n x
 ; ]]; echo "st=$?"`,
		Why: "and the reason bash's extra line carries a location of its own: the construct is named at the `[[`'s line and the token at the token's, so a condition opened on line 1 and refused on line 2 names both. One line would have looked right in every single-line case above",
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
		ID: "readonly/an-associative-element-is-refused", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -A m\nm[a]=1\ntypeset -r m\nm[k]=v\necho \"st=$? k=[${m[k]-unset}] a=[${m[a]}]\"\necho after",
		Why:     "the refusal on an *element* of a frozen name, which is the row #1012 is about. All three shells with the attribute refuse it and leave the table alone — bash 3.2 has no `-A` and dash has no arrays, so the panel's intersection here is the three that can be asked. Ours stored the element, said nothing and reported 0, which is a silent write to a table a script deliberately froze. Run from a script rather than `-c` because the two shells that end the script here would stop before the line that reads the table back",
	},
	{
		ID: "readonly/an-indexed-element-is-not-written", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -a a=(x y)\ntypeset -r a\na[0]=z\necho \"st=$? all=[${a[*]}]\"\necho after",
		Why:     "the same refusal on the indexed spelling, and the reason the array is read back rather than the status alone: ours printed the complaint and wrote the element anyway, because the only thing consulting the readonly set on this path was the scalar view storeArray keeps in step — after the store. The message alone matched every shell in the panel while the table did not",
	},
	{
		ID: "readonly/an-element-append-is-refused", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -A m\nm[a]=1\ntypeset -r m\nm[a]+=Q\necho \"st=$? a=[${m[a]}]\"\necho after",
		Why:     "`m[k]+=v` reads the element, joins the value and stores the result, so it is a write by a second route and had to be asked separately — a guard on the plain element assignment that missed this one would leave the append as the way through",
	},
	{
		ID: "readonly/a-compound-literal-is-refused", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -A m\nm[a]=1\ntypeset -r m\nm=([z]=9)\necho \"st=$? a=[${m[a]-unset}] z=[${m[z]-unset}]\"\necho after",
		Why:     "the whole-name assignment on an associative array, which never went through the scalar path at all and so replaced the table outright: `a` came back unset where every shell in the panel still has it. The name path being refused is what a fix to the element path must not loosen, and this row is what watches it",
	},
	{
		ID: "readonly/a-compound-append-is-refused", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -A m\nm[a]=1\ntypeset -r m\nm+=([z]=9)\necho \"st=$? a=[${m[a]-unset}] z=[${m[z]-unset}]\"\necho after",
		Why:     "`m+=(…)` keeps what is there and adds to it, which is a third store with its own path into the table",
	},
	{
		ID: "readonly/a-scalar-over-a-frozen-array-is-refused", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -a a=(x y)\ntypeset -r a\na=p\necho \"st=$? all=[${a[*]}]\"\necho after",
		Why:     "a scalar assignment replaces any array of the same name, and the removal that does it ran after the refusal had already reported — so a refused assignment destroyed the array it was refused for. The array is read back because the status and the message were both already right",
	},
	{
		ID: "readonly/a-declarations-own-array-value-survives-its-flag", Category: "parameter expansion",
		Script:  true,
		Snippet: "typeset -ar a=(x y)\necho \"st=$? all=[${a[*]}]\"\ntypeset -Ar m=([k]=v)\necho \"st=$? k=[${m[k]}]\"",
		Why:     "the other half of the rule `decl/readonly-takes-its-array-first` records, reached by the spelling that cannot solve it the same way: `readonly` assigns its operands before it locks, and `typeset` cannot, because the array has to land after the shadow a declaration takes inside a function. So the freeze waits for the operand instead. bash and zsh both keep the value; ksh93 does not cluster `-ar` and refuses the flag rather than the value, which is why the row is worth having in all four",
	},
	{
		ID: "readonly/a-refused-array-operand-gives-up-the-line", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -a a=(x y)\ntypeset -r a\ntypeset -a a=(p q); echo one\necho two",
		Why:     "an array operand's refusal is a bare assignment's and not a declaration's, which the spelling does not suggest. bash tells the two apart twice over: `declare r=2` against a frozen name reports, names the builtin and runs the next command on the same line, and `typeset -a a=(p q)` names only the variable and gives the rest of the line up — `one` never prints and `two` does. The array operand goes through the assignment machinery and the builtin's name never reaches it. See axis/readonly-refusal-names-the-builtin for the scalar half",
	},
	{
		ID: "readonly/a-refused-declaration-operand-reports-one", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -a a=(x y)\ntypeset -r a\ntypeset -a a=(p q)\necho \"st=$? all=[${a[*]}]\"\necho after",
		Why:     "the status of a declaration whose *operand* was refused. The operand assignments land after the builtin has returned, so the 0 the builtin reported for the attributes it did apply stood over the value it did not — the message printed and `$?` said success",
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
		ID:       "pat/a-question-mark-is-one-character",
		Category: "pattern matching",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `s=héllo; case $s in ?????) echo five;; ??????) echo six;; esac`,
		Why:      "the shortest statement of the whole question: the pattern is five ASCII bytes and the answer still moves with the locale, because what a `?` consumes is one character of the *subject*. bash, ksh93 and zsh say five; dash says six, having no decoder",
	},
	{
		ID:       "pat/a-question-mark-is-one-byte-in-a-single-byte-locale",
		Category: "pattern matching",
		Snippet:  `s=héllo; case $s in ?????) echo five;; ??????) echo six;; esac`,
		Why:      "and the same pattern under the fixed locale, where every member of the panel says six. The pair is the axis; either row alone reads as a fact about `?`",
	},
	{
		ID:       "pat/a-bracket-holds-a-whole-character",
		Category: "pattern matching",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `case é in [é]) echo lit;; *) echo no;; esac; case é in [ae]) echo half;; *) echo whole;; esac`,
		Why:      "a bracket matches one unit of the subject as `?` does, so a two-byte character is in a bracket that lists it and is *not* matched by a bracket holding either of its bytes' worth of ASCII. The second half is the one a byte matcher gets wrong in the direction nobody notices: it would take `é` for the `e` it starts near",
	},
	{
		ID:       "pat/a-bracket-range-is-ranked-by-code-point",
		Category: "pattern matching",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `case ç in [a-é]) echo in;; *) echo out;; esac; case é in [a-ÿ]) echo in;; *) echo out;; esac`,
		Why:      "the endpoints of a range are characters too, and the comparison is by code point rather than by text: ç is between a and é as a number and is not between them byte for byte, so a matcher that compared the encoded forms would answer the first of these wrongly and the second by luck",
	},
	{
		ID:       "pat/a-character-class-outside-ascii",
		Category: "pattern matching",
		Env:      []string{"LC_ALL=C.UTF-8"},
		Snippet:  `for c in alpha alnum upper lower digit space punct print graph; do case é in [[:$c:]]) printf "%s " "$c";; esac; done; echo`,
		Why:      "where the panel is least uniform. bash 5.3, bash 3.2 and zsh agree exactly — a letter outside ASCII is alpha, alnum, lower, print and graph — and dash answers none of them, having no decoder. ksh93 answers **alpha and nothing else**, which is a partial implementation rather than a different reading of the classes: it agrees with the other three on every ASCII character and on the name of the class it does implement. This follows the three that agree, so the ksh93 cell is a recorded difference rather than a modeled one",
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
		ID: "arith/float-is-a-dialect-axis", Category: "arithmetic",
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
		ID: "arith/an-operand-the-expression-ran-out-of", Category: "arithmetic",
		Snippet: `echo "[$((1+))]"; echo "st=$?"`,
		Why:     "an operand was wanted and the text ended, which two of the panel word apart from an operand that was wanted and found: ksh93 says more tokens expected and zsh names the end of the string. The pair with `arith/an-operand-the-expression-found` is the whole of it — either row alone passes under one wording for both, which is what let the end-of-input sentence stand for every operand failure in two dialects",
	},
	{
		ID: "arith/an-operand-the-expression-found", Category: "arithmetic",
		Snippet: `echo "[$((%))]"; echo "st=$?"`,
		Why:     "the other half: a token is there and it cannot begin a value. ksh93 drops to its bare arithmetic syntax error and zsh names the text — ``operand expected at `%'`` — where both said the expression had run out. bash words the two identically, which is why the bash column cannot see this at all and why the failure survived every conformance read",
	},
	{
		ID: "arith/an-operand-found-after-an-operator", Category: "arithmetic",
		Snippet: `echo "[$((1+&2))]"; echo "st=$?"`,
		Why:     "the found case reached mid-expression rather than at its start, and the text named runs to the end of the expression rather than being the one refused byte — `&2`, which is what the two shells that name anything name. It also says the distinction is not about where in the expression the failure is: the same operator wanting the same operand is worded one way here and the other way in `arith/an-operand-the-expression-ran-out-of`",
	},
	{
		ID: "arith/an-operand-a-lexer-refuses-outright", Category: "arithmetic",
		Snippet: `echo "[$((@))]"; echo "st=$?"`,
		Why:     "a byte that is part of no arithmetic token, which zsh alone words a third way — `illegal character: @` rather than the operand sentence it gives `%`. bash, ksh93 and dash word it exactly as they word `%`. The byte alone does not decide, which is why the three rows below it exist: the same `@` gets the operand sentence where an operator has just been consumed. Two tables and no code path — the bytes are Dialect.ArithBytesRefusedOutright and the sentence Diagnostics.ArithIllegalByte, and both are empty for the three shells that have neither",
	},
	{
		ID: "arith/a-refused-byte-where-an-operator-belonged", Category: "arithmetic",
		Snippet: `echo "[$((1 @))]"; echo "st=$?"`,
		Why:     "the second position the byte answers for itself at: a complete value has been read and the expression could have stopped, so zsh reports the byte rather than a missing operand. bash splits here too and already did — `invalid arithmetic operator` where an operand position gets `operand expected` — but on a byte set of its own, so the two shells' tables overlap without matching",
	},
	{
		ID: "arith/a-refused-byte-where-an-operand-belonged", Category: "arithmetic",
		Snippet: `echo "[$((1+@))]"; echo "st=$?"`,
		Why:     "and the position where it does not: an operator has just been consumed, so the failure is the operator's and zsh names the text from the byte to the end of the expression instead. Same byte, same shell, different sentence — which is the whole reason the byte table cannot be read on its own. This row and the two around it are one measurement in three parts and are worth nothing separately",
	},
	{
		ID: "arith/a-refused-byte-with-nothing-read-but-space", Category: "arithmetic",
		Snippet: `echo "[$((  @  ))]"; echo "st=$?"`,
		Why:     "whitespace is not reading. The cursor is not at the start of the text and zsh still gives the byte its own verdict, which rules out the offset as the test — what decides is whether anything has been *consumed*, and here nothing has",
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
		ID: "arith/a-chained-assignment", Category: "arithmetic",
		Snippet: `echo "$((a=b=5)) a=$a b=$b"`,
		Why:     "assignment associates to the right and is itself an expression, so one statement sets both names and the whole thing is worth 5. Unanimous, dash included. A parser that read assignment as a statement rather than an operator would take the left name and lose the right, which is a shape that produces a plausible number and a variable nobody set",
	},
	{
		ID: "arith/a-negative-modulo", Category: "arithmetic",
		Snippet: `printf "[%s]" "$((-7 % 3))" "$((7 % -3))"; echo`,
		Why:     "the sign of a remainder follows the *dividend*, unanimously — C's truncating rule rather than a mathematical modulus, which is the other plausible answer and the one that would make the first of these 2. `arith/division-truncates` pins the quotient's half of the same rule",
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
	{
		ID: "arith/increment-through-a-subscript", Category: "arithmetic",
		Snippet: `a=(3 4 5); (( a[1]++ )); printf "[%s]" "${a[1]}"`,
		Why:     "`++` took a bare name and nothing else, so this was `++ needs a variable` in every dialect while `(( a[1] += 1 ))` on the same element was fine. A name that can be assigned to can be incremented; the three shells with arrays all say so, and they differ here only by where the first element is",
	},
	{
		ID: "arith/increment-through-a-key", Category: "arithmetic",
		Snippet: `typeset -A m; m[k]=1; (( m[k]++ )); printf "[%s]" "${m[k]}"`,
		Why:     "the spelling a real script reached — a counter kept in an association, incremented in place. The declared attribute makes the subscript a key on the way in *and* on the way out, so the operator has to know which kind of array it is writing to; dash and bash 3.2 have no `-A` and split the panel",
	},
	{
		ID: "arith/a-key-is-not-an-index-in-an-expression", Category: "arithmetic",
		Snippet: `typeset -A m; m[k]=7; m[0]=99; k=0; printf "[%s]" "$(( m[k] ))"`,
		Why:     "the case that tells the two readings apart, and the reason it was worth a row of its own: evaluating the subscript answered 99 with no diagnostic at all, which is a wrong value a script cannot see. The three shells with the attribute all read the key",
	},
	{
		ID: "arith/an-element-holding-a-name-is-chased", Category: "arithmetic",
		Snippet: `y=5; a=(y); printf "[%s]" "$(( a[0] ))"`,
		Why:     "the chase a bare name already got, asked of an element: the storage an operand came out of is not what decides how it reads. bash and ksh93 reach 5 here and zsh counts from 1 so its element 0 is nothing, which is the same split `${a[0]}` shows and not a second rule",
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
		ID: "cond/empty-operand-is-not-a-path", Category: "conditions",
		Snippet: `[[ -d $NOPE ]]; echo "d=$?"; [[ -e $NOPE ]]; echo "e=$?"; [[ -r $NOPE ]]; echo "r=$?"; [[ -s $NOPE ]]; echo "s=$?"; [[ -f $NOPE ]]; echo "f=$?"`,
		Why:     "an empty operand is not a path, so every file question about it is false — the whole panel agrees, and `[[ -d $x ]]` guarding a computed directory is the reason it matters. Joining an empty operand onto the working directory instead makes the five answers 0 0 0 0 1: only `-f` stays right, because a directory is not a regular file, and that is why the wrong answer is easy to miss",
	},
	{
		ID: "cond/empty-operand-is-not-the-working-directory", Category: "conditions",
		Snippet: `[[ -d "" ]]; echo "empty=$?"; [[ -d . ]]; echo "dot=$?"; [[ -w "" ]]; echo "wempty=$?"; [[ -w . ]]; echo "wdot=$?"`,
		Why:     "the pair that separates the two readings. `.` is the spelling that means the working directory and answers true; an empty operand names nothing and answers false. A shell that resolves the empty one against its own directory gives the same answer to both",
	},
	{
		ID: "cond/empty-operand-in-the-file-comparisons", Category: "conditions",
		Snippet: `touch f; [[ "" -ef "" ]]; echo "ee=$?"; [[ f -ef "" ]]; echo "fe=$?"; [[ "" -nt f ]]; echo "nt=$?"; [[ f -ot "" ]]; echo "ot=$?"`,
		Why:     "the binary comparisons take their operands the same way, so they need the same probe: two empty operands are not the same file anywhere in the panel. Resolving both against the working directory makes `[[ \"\" -ef \"\" ]]` compare that directory with itself and answer *true*",
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
	{
		ID: "cond/option-test-reads-a-set-option", Category: "conditions",
		Snippet: `set -e; [[ -o errexit ]]; echo "on=$?"; set +e; [[ -o errexit ]]; echo "off=$?"`,
		Why:     "`-o` asks whether a shell option is set, and it is the core's rather than a dialect's: every shell in the panel that has `[[ ]]` at all has it, and dash is absent only because it has no `[[ ]]` to put it in. The name every one of them agrees about, read in both states from the same run",
	},
	{
		ID: "cond/option-test-operand-is-an-ordinary-word", Category: "conditions",
		Snippet: `set -u; v=nounset; [[ -o $v ]]; echo "var=$?"; [[ -o 'nounset' ]]; echo "quoted=$?"`,
		Why:     "how the name reaches the operator: it is an ordinary word, expanded out of a variable and stripped of its quotes, unanimously. The quoted spelling is the one the prompt theme on this machine writes — `[[ ! -o 'aliases' ]]` — and a parser that took the operand as a literal token would read the apostrophes as part of the name",
	},
	{
		ID: "cond/option-test-unknown-name-diverges", Category: "conditions",
		Snippet: `[[ -o nosuchoption ]]; echo "st=$?"; echo alive`,
		Why:     "a name no shell in the panel has: bash and ksh93 answer a quiet false at 1 where zsh complains and answers 3, and all three carry on to the next command — so this is not the fatality `set -o nosuchoption` produces in the same shell. The UnknownConditionOptionIsAStatus axis",
	},
	{
		ID: "cond/option-test-unknown-name-is-not-a-false", Category: "conditions",
		Snippet: `[[ ! -o nosuchoption ]]; echo "not=$?"; [[ -o nosuchoption || 1 == 1 ]]; echo "or=$?"; [[ 1 == 1 && -o nosuchoption ]]; echo "and=$?"`,
		Why:     "what the divergent answer *is*, which the bare status hides: in zsh it is a third value rather than a false, so `!` leaves it at 3 instead of turning it into 0, `||` walks on past it to a right-hand side that answers 0, and `&&` stops on it. bash and ksh93 have a plain false in the same three places and answer 0, 0 and 1. A dialect that returned false with a status painted on would get the negation wrong",
	},
	{
		ID: "cond/option-test-missing-operand", Category: "conditions",
		SyntaxError: true,
		Snippet:     `[[ -o ]]; echo "st=$?"`,
		Why:         "`-o` with nothing after it is refused in all three, which is what says the operator takes an operand rather than defaulting one — and refused in three different ways: bash and ksh93 make it a *parse* error, where zsh takes the `-o` for a condition name it does not know and answers 2 at run time. It is also the shape that separates this from the single-bracket `-o`, where the same two words are a non-empty string test that succeeds",
	},
	{
		ID: "cond/single-bracket-o-is-not-the-option-test", Category: "conditions",
		Snippet: `[ -o nosuchoption ]; echo "two=$?"; [ -o ]; echo "one=$?"; [ '' -o x ]; echo "or=$?"`,
		Why:     "the operator the standard gives `[` is `or`, and conflating it with the option test is the mistake this row exists to catch: bash and ksh93 do have a unary `-o` in the builtin, dash refuses it as an unexpected operator and zsh calls it too many arguments, so the single-bracket spelling is not core the way the double-bracket one is. `[ -o ]` is one argument and a true everywhere, and `[ '' -o x ]` is the binary or",
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
	// --- what an error inside a sourced file costs ---------------------
	//
	// The operand is an unset parameter under `set -u` throughout, chosen
	// because every shell in the panel calls it a failure and words it almost
	// identically: the rows are about how far the abandonment reaches, and an
	// operand only some of them refuse would measure the operator instead.
	{
		ID: "dot/fatal-error-ends-the-sourced-file-only", Category: "eval and dot",
		Snippet: `printf 'echo IN-BEFORE\nset -u\necho X${NOPE}\necho IN-AFTER\n' > p.sh; . ./p.sh; echo "OUT-AFTER st=$?"`,
		Why:     "the whole axis on one row: all six stop at the failing line inside the file, and only ksh93 and zsh come back — reporting 1 and 126 — where dash and the three bashes end the shell and never reach the `echo` on the same line as the `.`",
	},
	{
		ID: "dot/fatal-error-ends-one-file-not-the-stack", Category: "eval and dot",
		Snippet: `printf 'echo IN-BEFORE\nset -u\necho X${NOPE}\necho IN-AFTER\n' > p.sh; printf 'echo MID-BEFORE\n. ./p.sh\necho "MID-AFTER st=$?"\n' > m.sh; . ./m.sh; echo "OUT-AFTER st=$?"`,
		Why:     "the file given up is the innermost one and not every file above it: the middle file runs the line after its own `.` in the two shells that catch anything, which is what makes this one boundary rather than an unwind that stops at the outermost",
	},
	{
		ID: "dot/fatal-error-in-a-function-is-not-at-a-boundary", Category: "eval and dot",
		Snippet: `printf 'f() { echo F-BEFORE; set -u; echo X${NOPE}; echo F-AFTER; }\n' > p.sh; . ./p.sh; f; echo "OUT-AFTER st=$?"`,
		Why:     "the boundary is the `.` that is *running* and not the file the text was read from: a function defined in a sourced file and called afterwards ends the shell in all six, ksh93 and zsh included, so nothing about where a function came from survives the call",
	},
	{
		ID: "dot/fatal-error-resumes-the-function-that-sourced", Category: "eval and dot",
		Snippet: `printf 'echo IN-BEFORE\nset -u\necho X${NOPE}\necho IN-AFTER\n' > p.sh; f() { echo F-BEFORE; . ./p.sh; echo "F-AFTER st=$?"; }; f; echo "OUT-AFTER st=$?"`,
		Why:     "the other side of the same rule: a `.` inside a function *is* the boundary, so the two shells that catch resume the function body at the command after it rather than unwinding the call",
	},
	{
		ID: "dot/exit-in-a-sourced-file-is-never-caught", Category: "eval and dot",
		Snippet: `printf 'echo IN-BEFORE\nexit 7\necho IN-AFTER\n' > p.sh; . ./p.sh; echo NOT-REACHED`,
		Why:     "unanimous, and it is what the row above is *not*: a request to stop ends the shell from inside a sourced file in every member of the panel, so an implementation that caught controlExit at the `.` without asking which kind it was holding would break this",
	},
	{
		ID: "dot/error-operator-ends-the-shell-in-zsh", Category: "eval and dot",
		Snippet: `printf 'echo IN-BEFORE\necho X${NOPE?msg}\necho IN-AFTER\n' > p.sh; . ./p.sh; echo "OUT-AFTER st=$?"`,
		Why:     "`${x?word}` parts company with the unset-parameter row two above it in exactly one shell: ksh93 catches it like any other error and reports 1, and zsh ends the shell — which is what its own manual says the operator does, and is why the two are separate axes rather than one",
	},
	{
		ID: "eval/fatal-error-ends-the-evaluated-text-only", Category: "eval and dot",
		Snippet: `eval 'echo IN-BEFORE
set -u
echo X${NOPE}
echo IN-AFTER'; echo "OUT-AFTER st=$?"`,
		Why: "`eval` is the same boundary as `.` and splits the same way — the four that end the shell for a sourced file end it here too, and ksh93 and zsh come back — but the status is *not* the same: zsh reports 1 for text it evaluated where it reports 126 for a file it sourced, which is why the number is a second field",
	},
	{
		ID: "eval/exit-in-evaluated-text-is-never-caught", Category: "eval and dot",
		Snippet: `eval 'echo IN-BEFORE
exit 7
echo IN-AFTER'; echo NOT-REACHED`,
		Why: "unanimous, and the guard on the row above: a request to stop is not an error, so the boundary that catches one must not catch the other",
	},
	{
		ID: "eval/an-abandoned-statement-does-not-end-the-text", Category: "eval and dot",
		Script: true,
		Snippet: `eval 'echo IN-BEFORE
readonly rr=1
rr=2
echo IN-AFTER'; echo "OUT-AFTER st=$?"`,
		Why: "a statement a shell *gives up* rather than dies over is given up as far as the end of its line and no further, evaluated text included: bash reports the refusal and runs the `echo` on the next line of the same eval, where a boundary that mistook the give-up for a fatal error would lose the rest of the text",
	},
	{
		ID: "dot/a-given-up-statement-takes-the-rest-of-its-line", Category: "eval and dot",
		// From a file: `-c` would answer with bash's own rule that a bare
		// readonly reassignment is fatal when the program *is* the command
		// string, which is a different question.
		Script:  true,
		Snippet: `printf 'echo IN-BEFORE\nreadonly rr=1\nrr=2; echo SAME-LINE\necho NEXT-LINE\n' > p.sh; . ./p.sh; echo "OUT-AFTER st=$?"`,
		Why:     "the give-up reaches to the end of the line and no further, inside a sourced file exactly as at the top of a script: SAME-LINE never prints in any shell that survives the refusal, and NEXT-LINE does — a row asserting only that the file kept running would pass with the same-line half missing",
	},
	{
		ID: "dot/readonly-refusal-ends-the-sourced-file-only", Category: "eval and dot",
		Snippet: `printf 'echo IN-BEFORE\nreadonly rr=1\nrr=2\necho IN-AFTER\n' > p.sh; . ./p.sh; echo "OUT-AFTER st=$?"`,
		// From a file rather than from `-c`, which is not decoration: bash
		// alone makes a bare readonly reassignment fatal when the program
		// *is* the command string, and that rule would answer this row
		// instead of the one it is asking about. Measured — the same three
		// lines are fatal as `bash -c` and survivable in a file, and text
		// `eval` or `.` runs is survivable either way.
		Script: true,
		Why:    "one snippet, two rules. Where a readonly reassignment is *fatal* it ends only the sourced file — ksh93 1, zsh 126 — which is the reach being a fact about any error a shell calls fatal rather than about expansion. Where it is not fatal, bash gives up the statement and its line and runs the `echo` after it, inside the file, which is the give-up costing a line rather than the text it is in",
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
		ID: "redirect/an-empty-target-is-not-the-working-directory", Category: "redirection",
		Snippet: `printf secret > in-there; cat < ""; echo "r=$?"; echo hi > ""; echo "w=$?"`,
		Why:     "an empty target names no file, and the same rule the file tests need: joining it onto the working directory opens the *directory*, so the read succeeds and the complaint arrives from the command as `Is a directory` rather than from the shell. Every shell in the panel refuses to open the name — with four wordings and two statuses, so the case pins the wording too",
	},
	{
		ID: "test/empty-operand-is-not-a-path", Category: "test",
		Snippet: `[ -d "$NOPE" ]; echo "d=$?"; test -e "$NOPE"; echo "e=$?"; [ -x "$NOPE" ]; echo "x=$?"; [ -f "$NOPE" ]; echo "f=$?"`,
		Why:     "the same rule on the builtin's surface, and measured through both of its spellings: the two constructs obtain an operand differently and answer the filesystem question identically, so an empty one is false in `[ ]` and `test` exactly as in `[[ ]]`",
	},
	{
		ID: "test/empty-operand-however-it-became-empty", Category: "test",
		Snippet: `unset u; e=; z=$(false); [ -d "$u" ]; echo "unset=$?"; [ -d "$e" ]; echo "empty=$?"; [ -d "$z" ]; echo "sub=$?"; [ -d "" ]; echo "lit=$?"`,
		Why:     "unset, set-and-empty, a substitution that printed nothing, and a literal empty word are four states of the variable table and one operand: expansion has made them the same word before the builtin sees it, so a caller needs all four rejected and the panel rejects all four alike",
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
		ID: "export/no-operands-lists-what-is-exported", Category: "builtins",
		Snippet: `export MYNAME=1; export | grep MYNAME`,
		Why:     "`export` with nothing after it lists, in all five: POSIX says so and the panel agrees that something is written. This wrote nothing at all, which is also why `export >&-` had no failed write to report. The *shape* is where they part, and it is not the shape `-p` uses: dash and both bash builds write the same line either way, while ksh93 and zsh drop the leading `export` and write a plain assignment for the bare form alone. The grep keeps the case to one line, so the split is the whole of what is recorded",
	},
	{
		ID: "readonly/no-operands-lists-what-is-readonly", Category: "builtins",
		Snippet: `readonly RONAME=2; readonly | grep RONAME`,
		Why:     "the same rule for `readonly`, and the same split in the same two shells — with zsh's `-p` form being `typeset -r` rather than `readonly`, so its bare form differs from its own `-p` by more than a missing word",
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
		ID: "declare/an-operand-that-is-not-a-name", Category: "declarations",
		Snippet: `typeset ':'; echo "st=$?"; echo A`,
		Why:     "the operand check every other declaration builtin already had. Four sentences at three statuses across six columns: dash has no such builtin and reports a command it cannot find, bash quotes the operand back as `not a valid identifier` and carries on at 1, ksh93 says `invalid variable name` and stops the script, and zsh says `not valid in this context` and stops it too — so `echo A` is the half of the row that measures the fatality rather than the wording. Taking the operand silently creates a parameter called `:` and reports success, which is a shell answering yes to a line no shell in the panel accepts (#1096)",
	},
	{
		ID: "declare/an-operand-that-starts-with-a-digit", Category: "declarations",
		Snippet: `typeset 1x; echo "st=$?"; echo A`,
		Why:     "the same refusal for the other shape of bad name, and the row that separates the two shells with a second sentence for it: zsh says `not an identifier: 1x` here where it says `not valid in this context` for a punctuation operand, and bash and ksh93 say to both what they say to either. Written as its own case because a dialect carrying one wording for both would pass the punctuation row and fail nothing else",
	},
	{
		ID: "declare/a-bad-operand-with-a-value-attached", Category: "declarations",
		Snippet: `typeset '1x=v'; echo "st=$?"; typeset ':=v'; echo "st=$?"`,
		Why:     "*which* part of the operand is quoted back, which is not the same question in every column: bash and ksh93 name the whole word — `1x=v` and `:=v` — and zsh names only the part before the `=`, so its second line is `not valid in this context: :` rather than `:=v`. An implementation that judged the name and then reported the name would be right in one column and one character short in two others",
	},
	{
		ID: "declare/a-subscripted-operand-to-a-declaration", Category: "declarations",
		Snippet: `typeset a[1]=v; echo "st=$?"; typeset -p a 2>&1`,
		Why:     "the operand a declaration takes that `export` does not, and the reason the name check needed a third answer rather than reusing `export`'s: bash refuses `export a[1]=v` as a bad name and *takes* this, creating the element, and ksh93 and zsh take both. Measured before the check was routed through, because a shared answer would have made bash start refusing a line it has always accepted",
	},
	{
		ID: "declare/an-integer-declaration-with-an-operand-that-is-not-a-name", Category: "declarations",
		Snippet: `integer 1x; echo "st=$?"; echo A`,
		Why:     "the same check under the third name, and the row that says whose name the complaint uses: ksh93's `integer` calls itself `typeset` in its own diagnostic where zsh's calls itself `integer`, so one shell renames the builtin in the sentence and the other does not. bash and dash have no such word at all",
	},
	{
		ID: "declare/declare-is-the-second-name", Category: "declarations",
		Snippet: `declare x=1; echo "[$x]"`,
		Why:     "bash and zsh spell it `declare` as well, ksh93 only `typeset`, which makes the name a dialect's answer rather than an axis",
	},
	{
		ID: "declare/integer-is-the-third-name", Category: "declarations",
		Snippet: `integer n=3; echo "[$n]"; n=5+2; echo "[$n]"`,
		Why:     "the other pairing of the family: `integer` is the declaration with the integer attribute already decided, and it is ksh93's and zsh's where `declare` is bash's and zsh's — so which names exist is a dialect's answer three times over rather than an axis. The second line is what the word is *for*: the attribute arrived without the letter, so a later assignment is an expression. bash and dash report a command they cannot find, which is what they really do with the word (#1155)",
	},
	{
		ID: "declare/integer-and-a-plus-form", Category: "declarations",
		Script:  true,
		Snippet: "integer n=5\ninteger +i n\nn=3+4\necho \"i[$n]\"\ninteger -x e=1\ninteger +x e\necho \"x[$(env | grep -c '^e=1$')]\"\n",
		Why:     "whether a plus word on `integer` removes anything at all, and the two shells that have the word answer opposite ways: zsh prepends the letter to an ordinary declaration, so `+i` cancels it and `+x` unexports, where ksh93 has a declaration command whose type the word itself fixes and a plus form reaches neither. **One question about the word and not one per letter** — both rows move together, which is what makes it a single field. ksh93's own `typeset +x` does unexport, so this is not the letter's answer being asked twice",
	},
	{
		ID: "declare/integer-with-an-output-base", Category: "declarations",
		Snippet: `integer -i 16 b=255; echo "1[$b]"; typeset -i2 c=5; echo "2[$c]"`,
		Why:     "`-i` takes an output base in both shells that have `integer` and the name prints in it afterwards — `16#ff` in ksh93 against `16#FF` in zsh, differing in nothing but the case of the digits, and `2#101` in both for the attached spelling. bash has no base at all: `-i2` is an invalid option there and a bare `16` is not a valid identifier, which is the row that makes it a dialect's answer. This engine records that a name is an integer and has nowhere to keep a base, so it names the base as missing rather than reading it and dropping it — dropping it left `255` standing at status 0 and turned the `16` into a variable of its own",
	},
	{
		ID: "declare/integer-refuses-a-letter-its-own-declaration-takes", Category: "declarations",
		Snippet: `integer -A m; echo "st=$?"`,
		Why:     "the second name's letter set is not the declaration's, and the two shells with the word narrow it differently: `-A` is a bad option to zsh's `integer` where its own `typeset -A` makes an association, and ksh93 hands `integer` the whole typeset grammar and takes it. So the letters are a field of their own — a shell that reused the declaration's would accept `integer -A m`, which is an associative array in neither shell",
	},
	{
		ID: "declare/integer-the-lines-a-hook-installer-opens-with", Category: "declarations",
		Script:  true,
		Snippet: "f() {\n  integer del list help\n  echo \"[${del-UNSET}][${list-UNSET}][${help-UNSET}]\"\n}\nf\n",
		Why:     "the line that put the word on the release bar: zsh's own `add-zsh-hook` function file declares three integers this way before it does anything else, so a shell without `integer` cannot install a precmd hook — which is the whole of what a zsh startup file does with the function. It also reads the valueless-declaration answer through the new word: zsh gives the fresh cell 0 and ksh93 leaves it unset, which is `DeclaredNameWithoutValueIsEmpty` seen from a third spelling rather than a new question",
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
		ID: "declare/valueless-local-shadowing-an-exported-name", Category: "declarations",
		Snippet: `export FOO=bar; f() { echo "read=[${FOO-UNSET}]"; local FOO; echo "read=[${FOO-UNSET}]"; env | grep '^FOO=' || echo "(none)"; }; f`,
		Why:     "what a child is told for an exported name a valueless declaration has taken out of view, which is where the two bash builds part company: 5.3 reads the name as unset and still hands a child the value it hid, and 3.2 hands it nothing. Both columns are right about their own build and the `bash` column is the one graded, so this case splits them on purpose",
	},
	{
		ID: "declare/valueless-local-shadowing-an-imported-name", Category: "declarations",
		Snippet: `f() { local TERM; echo "read=[${TERM-UNSET}]"; env | grep '^TERM=' || echo "(none)"; }; f`,
		Why:     "the same question by the other route, and the one both bash builds answer the same way: a name is exported by having arrived in the environment, so a valueless local over it hands a child what arrived while the shell itself reads the name as unset",
	},
	{
		ID: "declare/valueless-local-shadowing-a-callers-local", Category: "declarations",
		Snippet: `export FOO=bar; g() { local FOO; env | grep '^FOO=' || echo "(none)"; }; f() { local FOO=mid; g; }; f`,
		Why:     "which value it is: the caller's local rather than the global standing behind it, so the answer belongs to the scope that took the name and not to the name. The two readings differ only two functions deep, which is why the pair is here",
	},
	{
		ID: "declare/valueless-local-over-an-unexported-name", Category: "declarations",
		Snippet: `FOO=bar; f() { local -x FOO; env | grep '^FOO=' || echo "(none)"; }; f`,
		Why:     "the other side of it: `-x` is the local's own attribute and says nothing about the name it shadows, so a shadowed name nothing exported reaches no child — an exported name with no value is told to nobody in any shell measured",
	},
	{
		ID: "declare/a-local-that-drops-the-export-attribute", Category: "declarations",
		Snippet: `export FOO=bar; f() { local +x FOO=z; env | grep '^FOO=' || echo "(none)"; echo "read=[$FOO]"; }; f; echo "after=[$FOO]"`,
		Why:     "`+x` takes the attribute off the *local* and says nothing about the binding behind it, which is still exported and still holds `bar` — so bash hands the child `bar` while the shell itself reads `z`. The same shape as `declare/valueless-local-shadowing-an-exported-name` with a value on the line, and the value is what made it fail: the local had one of its own, so the list of shadowed exports passed the name over, and the unexported local made the ordinary loop pass it over too. The child was told nothing at all by either. zsh and ksh93 tell it nothing for the reason `declare/local-shadowing-an-exported-name` records — the local never inherited the attribute — and `after` shows the outer binding untouched",
	},
	{
		ID: "declare/a-local-that-drops-the-export-attribute-over-an-unexported-name", Category: "declarations",
		Snippet: `FOO=bar; f() { local +x FOO=z; env | grep '^FOO=' || echo "(none)"; }; f`,
		Why:     "the control the row above needs, and what says it is the shadowed binding speaking rather than a value the local kept: with nothing exported behind it, the same line tells the child nothing in every shell. A fix that handed over the outer value unconditionally would pass the case above and fail here",
	},
	{
		ID: "declare/a-local-that-drops-the-export-attribute-then-puts-it-back", Category: "declarations",
		Snippet: `export FOO=bar; f() { local +x FOO=z; export FOO; env | grep '^FOO=' || echo "(none)"; }; f`,
		Why:     "the third leg: `export` inside the function puts the attribute on the local, and the child is then told `z` rather than the `bar` it was told a line earlier. Unanimous in bash and zsh, which is what makes the difference between the two rows above a fact about the *attribute* and not about the spelling",
	},
	{
		ID: "declare/a-typeset-that-drops-the-export-attribute", Category: "declarations",
		Snippet: `export FOO=bar; function f { typeset +x FOO=z; env | grep '^FOO=' || echo "(none)"; echo "read=[$FOO]"; }; f`,
		Why:     "the same rule through the spelling ksh93 has, in the keyword function ksh93 gives a scope. bash hands the child the outer `bar`; ksh93 and zsh tell it nothing, the same way they answer the `local` spelling — so the split is `LocalInheritsTheExportAttribute` and not a rule of its own. It is the row that says ksh93 is on that axis at all, since it has no `local` to be asked with",
	},
	{
		ID: "declare/a-local-declared-twice-in-one-function", Category: "declarations",
		Snippet: `export FOO=bar; f() { local FOO=x; local FOO; echo "read=[${FOO-UNSET}]"; env | grep '^FOO=' || echo "(none)"; }; f`,
		Why:     "a valueless declaration of a name its own scope already declared: bash 5.3, bash 3.2 and zsh all leave `x` standing, because the value it would hide is the local the first line made and there is no outer value in front of it any more. We read UNSET — a declaration that only meant to name the variable again threw away what the function had just put in it, silently and at status 0. `declare/valueless-local-shadowing-a-callers-local` is the case this must not become: a *different* function's declaration does hide",
	},
	{
		ID: "declare/a-typeset-declared-twice-in-one-function", Category: "declarations",
		Snippet: `export FOO=bar; function f { typeset FOO=x; typeset FOO; echo "read=[${FOO-UNSET}]"; }; f`,
		Why:     "the same shape through `typeset` in a keyword function, and the one spelling all four shells with the builtin answer — ksh93 included, which the `local` row cannot ask. Unanimous `x`, which is what makes this core rather than either shell's reading",
	},
	{
		ID: "declare/a-global-declaration-over-this-functions-local", Category: "declarations",
		Snippet: `export FOO=bar; f() { local FOO=x; typeset -g FOO; echo "read=[${FOO-UNSET}]"; }; f; echo "after=[${FOO-UNSET}]"`,
		Why:     "the third spelling of the same bug: `-g` takes no shadow, so it is not the declaration that put anything in front of the name — but the scope had one already and the value went away anyway. bash 5.3 and zsh read `x` and leave the global `bar` alone. bash 3.2 and ksh93 have no `-g` and their cells are the refusal, which is why the row is worth having rather than folding into the one above: it reaches the same code by the path where no shadow is taken at all",
	},
	{
		ID: "declare/a-declaration-without-a-value-exports-nothing", Category: "declarations",
		Snippet: `typeset -x FOO; env | grep '^FOO=' || echo "(none)"; typeset -x BAR=; env | grep '^BAR=' || echo "(none)"`,
		Why:     "declared and assigned-empty are the same to every listing and not to a child: the shell that considers a name declared without a value to be *set* still tells no command about it, where `=` on the same line hands over an empty entry. Found by the case above, which had this shell exporting an empty value under a name it should say nothing about",
	},
	{
		ID: "declare/a-declared-name-a-local-has-shadowed", Category: "declarations",
		Snippet: `typeset -x FOO; env | grep '^FOO=' || echo "(none)"; f() { local FOO=v; }; f; env | grep '^FOO=' || echo "(none)"`,
		Why:     "the shell that considers a name declared without a value to be set forgets that on the way out of a function: after any local of that name, the caller's is an *empty* export where before it was told to no child at all. The pair is the point — the same `env` twice, with only a function call between them",
	},
	{
		ID: "declare/local-shadowing-an-imported-name", Category: "declarations",
		Snippet: `f() { local TERM=changed; env | grep '^TERM=' || echo "(none)"; }; f; env | grep '^TERM='`,
		Why:     "the same question where the name arrived in the environment rather than being exported by hand, which is the route that made the two answers one: an imported name is exported by having been imported, so the local either inherits that or does not, and the split is the same either way",
	},
	{
		ID: "declare/typeset-assignment-and-the-export-attribute", Category: "declarations",
		Snippet: `export FOO=bar; typeset FOO=baz; env | grep '^FOO=' || echo "(none)"; echo "read=[$FOO]"`,
		Why:     "whether a declaration that assigns takes the export attribute off the name it assigned to: one shell tells the child nothing and goes on holding the value, the other two hand the child the new value. Not about scope — this is the top level — and read through a real child, because the name keeps its value either way and only a command can see the difference",
	},
	{
		ID: "declare/typeset-assignment-and-the-export-attribute-put-back", Category: "declarations",
		Snippet: `export FOO=bar; typeset FOO=baz; export FOO; env | grep '^FOO=' || echo "(none)"`,
		Why:     "a reset and not a refusal: naming the attribute again afterwards puts it back, so the shell that clears it has not decided the name may never be exported",
	},
	{
		ID: "declare/readonly-assignment-and-the-export-attribute", Category: "declarations",
		Snippet: `export FOO=bar; readonly FOO=baz; env | grep '^FOO=' || echo "(none)"; export BAR=b; readonly BAR; env | grep '^BAR=' || echo "(none)"`,
		Why:     "the same question through `readonly`, which is that shell's `typeset -r` and answers it the same way — and the valueless half is the other end of the axis: with no value on the line the attribute is left alone everywhere, which is what makes the pair above a statement about assignment rather than about the builtin. Spelled with `readonly` rather than `typeset` because one shell *lists* a valueless `typeset` of a name it already has, which is a divergence of its own and not this one. dash reaches this case where it has no `typeset` to reach the others with",
	},
	{
		ID: "declare/export-assignment-keeps-the-export-attribute", Category: "declarations",
		Snippet: `export FOO=bar; export FOO=baz; env | grep '^FOO=' || echo "(none)"; FOO=qux; env | grep '^FOO=' || echo "(none)"`,
		Why:     "the two spellings that never ask it: `export` names the attribute outright, and a plain assignment is not a declaration utility's doing at all. Both hand the child the new value in every shell, which is the boundary the axis is drawn at",
	},
	{
		ID: "declare/typeset-assignment-in-a-posix-function", Category: "declarations",
		Snippet: `export FOO=bar; f() { typeset FOO=baz; }; f; env | grep '^FOO=' || echo "(none)"; echo "read=[$FOO]"`,
		Why:     "where the declaration reaches the caller rather than taking a scope, the attribute goes off for good — the pair with the keyword-function case below, where the same shell only takes it off for the function's duration",
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
		ID: "declare/unset-takes-the-integer-attribute-away", Category: "declarations",
		Snippet: "typeset -i n=5\nunset n\nn=3+4\necho \"[$n]\"",
		Why:     "`unset` removes the name *and* what it was declared to be, so a name assigned afterwards is a plain new name: `3+4` and not 7. Unanimous across every column that has the letter, which is what makes it a rule rather than an axis — and it is the ground the other attribute rows stand on, because a shell whose attribute maps are keyed by name and are never deleted from gives the *old* attribute to the *new* value, silently and at status 0. `unset PATH; PATH=a:a:b` is the shape that made it visible (#1047)",
	},
	{
		ID: "declare/unset-takes-the-case-attributes-away", Category: "declarations",
		Snippet: "typeset -u u=abc\nunset u\nu=def\necho \"u[$u]\"\ntypeset -l l=ABC\nunset l\nl=GHI\necho \"l[$l]\"",
		Why:     "the same rule over the two case letters, which is where a per-letter fix would have stopped: `def` and `GHI` rather than `DEF` and `ghi`. Both letters on one row because they share the answer the way they share the re-read one. bash 3.2 has neither letter and answers with two usage lines while still printing the unfolded values, so the row says the same thing there for a different reason",
	},
	{
		ID: "declare/unset-takes-the-export-attribute-away", Category: "declarations",
		Snippet: "typeset -x e=1\nunset e\ne=2\nenv | grep '^e=' || echo '(gone)'",
		Why:     "the half only a child can see, and the only one of these attributes whose loss is invisible to the shell's own reads: after the `unset` the name is not exported, so a value assigned afterwards reaches no child. `(gone)` everywhere. A slice the case sets itself and greps for, because a listing of the whole environment would grade the machine",
	},
	{
		ID: "declare/unset-takes-the-table-attribute-away", Category: "declarations",
		Snippet: "typeset -A m; m[k]=v\nunset m\nm=plain\necho \"[$m] st=$?\"",
		Why:     "the attribute that is not a flag but a *kind*: once the table is gone the name takes a scalar, so `m=plain` is an assignment and not a non-numeric subscript. `plain` at status 0 in the two columns that have `-A`; bash 3.2 has no `-A` and reaches the same answer having refused the letter, and ksh93's `typeset` is special so the row records what a fatal refusal does to the rest of the snippet",
	},
	{
		ID: "declare/unset-of-an-element-is-not-unset-of-the-name", Category: "declarations",
		Snippet: "typeset -U u=(1 2 3)\nunset 'u[1]'\nu+=(3)\necho \"[${u[@]}] n=${#u[@]}\"",
		Why:     "the bound on the rule above, and the reason it cannot be written as `unset` clearing whatever it is handed: an element is not the name, so the attribute stands and the appended `3` is dropped as the duplicate it is — `[ 2 3]`, the hole the element unset left and three elements still. zsh is the only column with the letter; the three bash columns refuse it and answer `[1 3 3]` with the duplicate kept, and ksh93's refusal of it is fatal. Recorded through `-U` because it is the one attribute whose presence an *append* can prove — a fix that cleared the maps wherever `unset` was called would pass every row above and keep the duplicate here",
	},
	{
		ID: "declare/unset-of-a-readonly-name-clears-nothing", Category: "declarations",
		Snippet: "typeset -r r=1\nunset r\necho \"st=$?\"\necho \"[$r]\"",
		Why:     "the other bound: the refusal stands in front of the removal, so a frozen name keeps its value *and* everything it was declared to be. `st=1` and `[1]` in bash, bash 3.2 and ksh93 — which calls it a warning and carries on — where bash as `sh` and zsh make the refusal fatal and never print either line. The row that says the clearing belongs behind the refusal rather than beside it, and no column reaches a cleared name to disagree about",
	},
	{
		ID: "declare/a-name-assigned-after-an-unset-lists-again", Category: "declarations",
		Snippet: "typeset -i q=5\nunset q\nq=7\ntypeset -p q\necho \"st=$?\"\nunset PATHX\nPATHX=zz\ntypeset -p PATHX\necho \"st=$?\"",
		Why:     "the other record `unset` leaves behind, and the one only a listing can see: the value is gone *and* the name is marked gone, so whatever brings it back has to lift both. Every column that can be asked says the name back with the value it now holds and no attribute — bash's `declare -- q=\"7\"`, ksh93's bare `q=7`, zsh's `typeset q=7` — and answers 0. The second half has no attribute anywhere in it, which is what says the mark and the attribute are two records rather than one: a shell that lifts only the attribute reports a name it is holding a value for as not found (#1047)",
	},
	{
		ID: "declare/an-array-assigned-after-an-unset-lists-again", Category: "declarations",
		Snippet: "a=(1 2)\nunset a\na=(3 4)\ntypeset -p a\necho \"1 st=$?\"\ntypeset -A m; m[k]=v\nunset m\ntypeset -A m; m[j]=w\ntypeset -p m\necho \"2 st=$?\"",
		Why:     "the same mark lifted by the two stores that are not the scalar table — an indexed array and a keyed one, each of which reaches its own store and neither of which goes through the scalar one. Both list back with their new elements and only their new elements: the `1 2` and the `[k]=v` are gone. Worth its own row because a fix to the scalar store passes the row above and leaves these two saying the name is not there, which is the shape a single-choke-point claim has to be tested against",
	},
	{
		ID: "declare/integer-attribute-with-text", Category: "declarations",
		Snippet: `typeset -i n; n=abc; echo "[$n]"`,
		Why:     "text that is not a number is not an error: `abc` is an expression whose value is an unset name, so the result is zero and nothing is said",
	},
	{
		ID: "declare/an-attribute-re-reads-the-value-the-name-already-holds", Category: "declarations",
		Snippet: "FOO=bar; typeset -i FOO; echo \"i[$FOO]\"\nd=MiXeD; typeset -u d; echo \"u[$d]\"\ne=MiXeD; typeset -l e; echo \"l[$e]\"",
		Why:     "whether an attribute added to a name that is already holding something reaches *backwards* to the value it found, or waits for the next assignment. ksh93 and zsh re-read at once — `bar` is an expression made of an unset name, so 0 is what is left, and `MiXeD` folds on the spot — where the three bash columns leave all three values standing. **Both answers lose data**, which is what makes it a field and not a rule: one destroys the text and the other leaves a name declared integer holding text that is not a number. The three letters are on one line because they share one answer per shell rather than each having one, which is the whole finding: the issue this closes recorded `-u` as unanimous and bash 5.3.15 does not fold it, and recorded zsh on bash's side of `-i` when it is on ksh93's. `declare/integer-attribute-added-to-a-name-with-a-value` and `declare/case-attribute-added-to-a-name-with-a-value` ask two of these of a value that is already a number or already mixed-case; this one asks all three of text, which is where the two answers part company hardest — one of them destroys it (#1000)",
	},
	{
		ID: "declare/an-integer-attribute-over-a-number-written-oddly", Category: "declarations",
		Snippet: "a=08; typeset -i a; echo \"1[$a]\"\nb=\" 7 \"; typeset -i b; echo \"2[$b]\"\nc=+7; typeset -i c; echo \"3[$c]\"\ng=5+2; typeset -i g; echo \"4[$g]\"\nh=; typeset -i h; echo \"5[$h]\"",
		Why:     "the same question over text that *is* a number written some other way, which is where a narrower reading of it goes wrong. The shells that re-read canonicalize all five — `08` is 8 and not an octal error, a padded `7` loses its spaces, `+7` its sign, `5+2` is 7 and an empty value is 0 — and the shells that do not leave every one of them as written. `08` is the row that decides how the *question* may be asked: a predicate that folded first to find out whether the readings differ would report a bad octal digit and fail, where the shells that keep the text say nothing at all, so the answer of the question would depend on the answer being asked for",
	},
	{
		ID: "declare/an-attribute-that-would-change-nothing-needs-no-dialect", Category: "declarations",
		Snippet: "a=7; typeset -i a; echo \"1[$a]\"\nb=abc; typeset -x b; echo \"2[$b]\"\nc=MiXeD; typeset -r c; echo \"3[$c]\"",
		Why:     "the control, and it is unanimous across all six columns: a value the attribute would read back unchanged, and two attributes that have nothing to say about a value at all. So the divergence above belongs to the *re-reading* and not to declaring — a reading under which a declaration empties or rewrites whatever it touches passes the rows above and fails this one, and an earlier one of ours emptied the name here",
	},
	{
		ID: "declare/an-attribute-over-a-cell-a-function-just-shadowed", Category: "declarations",
		Snippet: "v=5\nfunction f { typeset -i v; echo \"in[${v-UNSET}]\"; }\nf\necho \"out[$v]\"",
		Why:     "the boundary: inside a function the cell the declaration just made is **new** whatever the caller was holding, so there is nothing standing in it to re-read. bash and ksh93 leave it unset and zsh gives it the empty-declaration value 0 — which is `DeclaredNameWithoutValueIsEmpty` and not this axis, and is why the two had to be separated: the re-read used to live inside that branch, so zsh got it by accident of answering that question yes and ksh93, which answers it no, never reached it. The caller's 5 is untouched in every column. The keyword spelling of the definition is load-bearing for one shell, where `typeset` makes a local only in that form",
	},
	{
		ID: "declare/an-attribute-added-to-an-exported-name-reaches-the-child", Category: "declarations",
		Snippet: `export FOO=bar; typeset -i FOO; env | grep '^FOO='`,
		Why:     "the same divergence seen from the only place it cannot be argued about: what the *child* is told. `FOO=0` in ksh93 and zsh and `FOO=bar` in the three bash columns, so the re-read is a change to the value and not a way of reading it back — a shell that merely rendered the name differently to its own expansions would answer this row `bar` everywhere",
	},
	{
		ID: "declare/a-type-over-a-name-the-shell-was-started-with", Category: "declarations",
		Env:     []string{"INHERITED=bar"},
		Snippet: "echo \"before=[$INHERITED]\"\ntypeset -i INHERITED\necho \"after=[${INHERITED+SET}][$INHERITED]\"\nenv | grep '^INHERITED=' || echo '(gone)'",
		Why:     "the third answer to the re-read, on the one input where the two shells that agree about a scalar part company. bash leaves `bar` and hands it down; zsh reads it back to 0 and hands *that* down; **ksh93 does neither** — the name comes back with no value at all and the export goes with it, so the child is told nothing. Modeling ksh93 as the re-read is wrong in two places at once and silently: a script that declares an inherited name and then runs a child hands it `0` where this shell hands it nothing. The `${INHERITED+SET}` is what tells the two failures apart — an empty value would still be SET, and it is not (#1121)",
	},
	{
		ID: "declare/a-type-over-an-inherited-name-a-fold-would-not-touch", Category: "declarations",
		Env:     []string{"A=UPPER", "B=7", "C=lower"},
		Snippet: "typeset -u A; echo \"u[${A+SET}][$A]\"\ntypeset -i B; echo \"i[${B+SET}][$B]\"\ntypeset -l C; echo \"l[${C+SET}][$C]\"\nenv | grep -c '^[ABC]=' || true",
		Why:     "the row that says the question is not the re-read narrowed, and the reason it has to be asked *ahead* of the canonical-spelling predicate rather than behind it: none of these three values is one any fold would alter — `UPPER` is already upper, `7` is already the canonical spelling of itself, `lower` is already lower — and ksh93 discards all three anyway, taking the count of environment entries from 3 to 0. Asked behind that predicate, the shape's commonest spellings would have gone unanswered. All three letters on one row because they share this answer the way they share the re-read one",
	},
	{
		ID: "declare/an-inherited-name-started-over-is-a-fresh-one", Category: "declarations",
		Env:     []string{"E=jj"},
		Snippet: "typeset -u E\ntypeset -p E\necho \"st=$?\"\nE=mix\necho \"[$E]\"\nenv | grep '^E=' || echo '(no child)'",
		Why:     "what the shell that discards leaves behind, which is not an emptied name but a **fresh** one. The attribute is intact — `mix` folds to `MIX` — the listing says the name with its attribute and no value, and the export does not come back by being assigned to. That is exactly the state that shell's own `DeclaredNameWithoutValueIsEmpty` = no leaves a name it has never held, which is what makes this one answer rather than a special case: the declaration starts the name over. bash and zsh keep the name in the child's environment throughout",
	},
	{
		ID: "declare/an-inherited-name-the-same-declaration-also-exports", Category: "declarations",
		Env:     []string{"G=3+4", "K=6+6", "P=4+4"},
		Snippet: "typeset -ix G; echo \"ix[$G] env=$(env | grep -c '^G=')\"\ntypeset -ir K; echo \"ir[$K] env=$(env | grep -c '^K=')\"\ntypeset -x P; typeset -i P; echo \"two[${P+SET}][$P] env=$(env | grep -c '^P=')\"",
		Why:     "the bound, and it is a fact about **one command's letters** rather than about the name's standing attributes. `-x` or `-r` beside the type letter keeps the inherited value and re-reads it — 7 and 12, still exported — where the same two attributes split across two commands discard it. So a reading that consulted whether the name is exported when the type arrives gets the third column wrong: `P` is exported by the line before, and is discarded all the same",
	},
	{
		ID: "declare/an-inherited-name-the-script-assigned-first", Category: "declarations",
		Env:     []string{"D=bar"},
		Snippet: "D=$D\ntypeset -i D\necho \"[${D+SET}][$D] env=$(env | grep -c '^D=')\"",
		Why:     "the control that says the question is about where the value *lives* and not about the export attribute: the script assigns the name its own value first — the same value it already had — and every column then answers the plain re-read question. `0` in ksh93 and zsh and `bar` in the three bash columns, exported in all six. A shell that keyed the discard off the export bit would answer this row the same as the one above it, and no shell does",
	},
	{
		ID: "declare/a-local-that-shadows-a-readonly", Category: "declarations",
		Script:  true,
		Snippet: "typeset -r x=1\nf() { local x=2; echo \"in=[$x]\"; echo running; }\nf\necho \"st=$? out=[$x]\"\necho after\n",
		Why:     "whether a declaration inside a function may make a local of a name the shell has frozen, and the two shells that can be asked answer opposite ways. zsh takes the shadow — `in=[2]`, the function runs on, and the outer 1 is back afterwards — where bash reports `local: x: readonly variable`, leaves the *outer* value in view and **still runs the rest of the function**. Three things ride on the refusal and each was wrong here: it names the builtin the script wrote, the builtin reports 1 rather than abandoning anything, and the outer value is what the body sees. ksh93 has no `local` and dash no `typeset -r`, so those two columns record the absence rather than an answer (#1159)",
	},
	{
		ID: "declare/a-valueless-local-that-shadows-a-readonly", Category: "declarations",
		Script:  true,
		Snippet: "typeset -r x=1\nf() { local x; echo \"in=[${x-UNSET}]\"; }\nf\necho \"st=$? out=[$x]\"\n",
		Why:     "the same question with no value on the declaration, and it is the spelling that used to slip past: with nothing to assign there was no assignment to meet the refusal, so the local was made in **silence** and the body saw an empty name where bash shows it the frozen 1. Worth its own row because a fix reached only through the assignment passes the row above and fails this one. `in=[]` against `in=[UNSET]` in the shell that takes the shadow is `DeclaredNameWithoutValueIsEmpty`, a different question showing through",
	},
	{
		ID: "declare/a-refused-shadow-keeps-the-other-operands", Category: "declarations",
		Script:  true,
		Snippet: "typeset -r x=1\ny=outer; z=outer\nf() { local y=1 x=5 z=2; echo \"in y=[$y] x=[$x] z=[$z]\"; }\nf\necho \"out y=[$y] z=[$z]\"\n",
		Why:     "one frozen name among three, which pins that the refusal costs *that operand* and not the command: bash leaves `y` and `z` local and only `x` refused, so the caller's `y` and `z` are untouched when the function returns. A refusal that gave up the whole declaration would leave both of them assigned globally — a change to the caller's variables that nothing said out loud",
	},
	{
		ID: "declare/a-local-that-shadows-a-readonly-and-the-name-afterwards", Category: "declarations",
		Script:  true,
		Snippet: "typeset -r x=1\nf() { local x=2; }\nf\nx=9\necho \"after=[$x]\"\n",
		Why:     "the other end of the shadow: the outer name is frozen **again** when the function returns, so the assignment after it is refused in every column. A shadow that merely took the attribute off would pass the rows above and leave a readonly quietly thawed by a function call, which is a hole in the whole point of the attribute — and it is only visible from outside the function, which is why this is a row of its own",
	},
	{
		ID: "declare/a-freeze-a-declaration-adds-ends-with-the-call", Category: "declarations",
		Script:  true,
		Snippet: "f() { local -r y=1; echo \"in=[$y]\"; }\nf\ny=2\necho \"st=$? y=[$y]\"\necho end\n",
		Why:     "the third end of it, and the one that goes the other way: the attribute a declaration *adds* inside a function is the call's and goes away with it. bash, ksh93 and zsh all leave y writable after the return, so this is core and not an axis — and the shadow that puts a *displaced* freeze back does nothing for a freeze the declaration itself made, which left a `local -r` freezing the caller's name for the rest of the script",
	},
	{
		ID: "declare/a-freeze-a-typeset-adds-ends-with-the-call", Category: "declarations",
		Script:  true,
		Snippet: "function f { typeset -r y=1; echo \"in=[$y]\"; }\nf\ny=2\necho \"st=$? y=[$y]\"\necho end\n",
		Why:     "the same through the word ksh93 has, in the function form that gives it a scope — the row that says this is unanimous rather than a two-shell agreement",
	},
	{
		ID: "axis/declaration-shadows-a-readonly-keyword-function", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -r x=1\nfunction f { typeset x=2; echo \"in=[$x]\"; }\nf\necho \"st=$? out=[$x]\"\necho end\n",
		Why:     "ksh93 has no `local` and answers the shadow question all the same, through `typeset` in a keyword function — where it agrees with zsh and not with bash. Written `f() { … }` the same shell has no scope to shadow into and the declaration is the ordinary fatal refusal, which is TypesetLocalNeedsKeywordFunction rather than this; reading that fatality as the answer had ksh93 down as a refusal it does not make",
	},
	{
		ID: "axis/declaration-shadows-a-readonly-posix-spelling", Category: "semantics axes",
		Script:  true,
		Snippet: "readonly x=1\nf() { local x=2; echo \"in=[$x]\"; }\nf\necho \"st=$? out=[$x]\"\necho end\n",
		Why:     "and dash answers too, asked with the freeze POSIX spells rather than with `typeset -r`: `local: x: is read only`, and the script ends. Both shells the axis was written down as unable to ask do answer it, and they answer on opposite sides",
	},
	{
		ID: "axis/readonly-attribute-removed", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -r s1=1\ntypeset +r s1\necho \"d=$?\"\ns1=9\necho \"st=$? s1=[$s1]\"\necho end\n",
		Why:     "zsh alone lets `+r` take the readonly attribute off; bash refuses and carries on, and ksh93 refuses and ends the script. A different split from the shadow rows above — ksh93 crosses to bash's side — which is what makes the two separate questions rather than one",
	},
	{
		ID: "axis/readonly-attribute-removed-declare-spelling", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -r b=1\ndeclare +r b\nb=9\necho \"b=[$b] st=$?\"\necho end\n",
		Why:     "the same word under its other spelling, which is also the one bash names in the refusal",
	},
	{
		ID: "axis/readonly-attribute-removed-with-a-value", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -r x=1\ntypeset +r x=5\necho \"d=$? x=[$x]\"\necho end\n",
		Why:     "the freeze comes off before the same command's own assignment in zsh, so 5 lands. And ksh93 refuses this one in its plain *line* form where the valueless `typeset +r x` names the builtin — one word, two shapes, and the value is what tells them apart",
	},
	{
		ID: "axis/readonly-removal-refusal-names-the-builtin", Category: "diagnostics",
		Script:  true,
		Snippet: "typeset -r s1=1\ntypeset +r s1\necho end\n",
		Why:     "the valueless half of that pair: ksh93 answers it in its builtin location with `typeset` named — the shape `set -A` gets — where `typeset s1=2` on the same name is `<script>: line 2: s1: is read only`. bash names the builtin for both and needs no second answer",
	},
	{
		ID: "axis/readonly-removal-on-a-free-name", Category: "semantics axes",
		Script:  true,
		Snippet: "a=1\ntypeset +r a\necho \"d=$?\"\na=2\necho \"st=$? a=[$a]\"\necho end\n",
		Why:     "a plus form on a name nothing froze reports 0 and says nothing in every shell that spells it — the shape a script actually writes, to make sure a name is writable, and the one an engine must not reach an axis for",
	},
	{
		ID: "axis/readonly-removal-on-a-startup-frozen-name", Category: "semantics axes",
		Script: true,
		// The value is compared rather than printed: this shell's own UID is
		// whatever machine recorded the row, and a case that echoed it could
		// only ever match the one that wrote it.
		Snippet: "was=$UID\ntypeset +r UID\necho \"d=$?\"\nUID=99\necho \"st=$?\"\n[ \"$UID\" = \"$was\" ] && echo unchanged || echo changed\necho end\n",
		Why:     "a name the shell was *started* frozen with rather than one the script froze: bash refuses `+r` on its own UID exactly as it refuses it on a script's name, so the answer is about the attribute and not about who set it. The row is the panel's rather than a target — this engine has no startup-frozen parameters at all, so it reads the plus form on an unset name and reports 0",
	},
	{
		ID: "core/readonly-letter-carries-its-own-sign", Category: "semantics axes",
		Script:  true,
		Snippet: "n=1\ntypeset -r +x n\nn=2\necho \"st=$? n=[$n]\"\necho end\n",
		Why:     "the sign that decides the readonly attribute is the letter's and not the last option word's: every shell with both letters freezes n here, so reading the word's sign would have made this a request to unfreeze — a refusal where the panel is unanimously silent",
	},
	{
		ID: "axis/readonly-removed-by-local-in-the-same-call", Category: "semantics axes",
		Script:  true,
		Snippet: "f() { local -r y=1; local +r y; y=2; echo \"in=[$y]\"; }\nf\necho \"st=$?\"\necho end\n",
		Why:     "the plus form spelled `local`, over a freeze the same call made — the one shape where shadowing cannot answer, since a name this scope has already shadowed has nothing left to displace. zsh takes the attribute off and reads 2; bash refuses the declaration before the question arises, and the two shells without `local` never reach it",
	},
	{
		ID: "core/a-plus-word-beside-the-readonly-letter-removes-nothing", Category: "semantics axes",
		Script:  true,
		Snippet: "typeset -r x=1\ntypeset -r +x x\necho \"d=$?\"\nx=2\necho \"st=$? x=[$x]\"\necho end\n",
		Why:     "the other side of `core/readonly-letter-carries-its-own-sign`, over a name that is already frozen: silent, status 0, and the freeze still standing in bash, ksh93 and zsh alike. Reading the last option word's sign rather than the letter's would make this a removal — a refusal in two of those shells and a thawed name in the third, and it is the shape that tells the two readings apart",
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
		ID: "declare/bare-typeset-is-a-listing-of-its-own", Category: "declarations",
		Snippet: `f() { local x=1; typeset; }; f | grep -c "^local x=1$"; echo "st=$?"`,
		Why:     "a bare `typeset` is a listing too, and not the one a bare `local` is in every shell that has both: one shell writes the same parameter table for either word — the line counted here is the attribute words and the assignment — while bash writes every variable it holds and ksh93 its own attribute listing, neither of which is the other's answer",
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
		ID: "declare/global-letter-keeps-the-associative-attribute", Category: "declarations",
		Snippet: `typeset -gA m; m[k]=v; echo "k=${m[k]} st=$?"`,
		Why:     "`-g` says where a declaration lands and must not cost it the `-A` attribute: the two shells that spell both letters store a string key, bash 3.2 refuses the letter it does not have and carries on with an ordinary array, ksh93 refuses it and stops — typeset is one of its own special builtins — and dash has no typeset at all",
	},
	{
		ID: "declare/global-associative-declared-in-a-function", Category: "declarations",
		Snippet: `f() { typeset -gA m; }; f; m[k]=v; echo "k=${m[k]} st=$?"`,
		Why:     "the shape a startup script actually writes — the table declared inside a function and filled in by the caller — which needs the attribute *and* the global scope to survive the return; the same four answers as the row before it",
	},
	{
		ID: "declare/global-letter-without-a-value", Category: "declarations",
		Snippet: `typeset -g gq; echo "set=${gq-unset}"; gq=1; echo "gq=$gq"`,
		Why:     "a valueless `-g` declaration brings the name into being at the global scope in the shell whose valueless declarations set the name, and leaves it unset in the shells whose do not — the whole of a `typeset -gA a b c` setup line, which carries no values at all",
	},
	{
		ID: "declare/global-letter-over-a-standing-value", Category: "declarations",
		Snippet: `gr=1; typeset -g gr; echo "gr=$gr"`,
		Why:     "`-g` takes no shadow, so there is no fresh cell to fill in and the value already there stays — unanimous among the shells that read the letter",
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
		ID: "declare/hide-attribute-keeps-the-value", Category: "declarations",
		Snippet: `typeset -H h=hid; echo "[$h] st=$?"`,
		Why:     "the letter two shells have and split over. In zsh `-H` hides a name's value from listings and changes nothing about a read; in ksh93 the same letter is a wholly different attribute — file name mapping — and hides nothing at all. This row is the half they agree on: the name holds `hid` and `$h` says so in both, so nothing downstream of the declaration can tell them apart. bash refuses the letter under `typeset` and under `declare`, with the usage line and 2, and dash has no such builtin",
	},
	{
		ID: "declare/hide-attribute-keeps-the-value-out-of-a-listing", Category: "declarations",
		Snippet: `typeset -H h=hid; typeset -p h; echo "st=$?"`,
		Why:     "and the half they disagree on, which is the whole of what the letter does in zsh: `typeset h` there, with no `=hid` after it, against ksh93's `typeset -H h=hid`, which writes the value *and* the letter back. So the attribute cannot be read as one thing with two spellings — one shell expresses it by omitting the value and the other by naming the flag, and a listing is the only place either of them says anything",
	},
	{
		ID: "declare/hide-attribute-removed", Category: "declarations",
		Snippet: `typeset -H h=hid; typeset +H h; typeset -p h; echo "st=$?"`,
		Why:     "`+H` puts the value back in the listing, which is the tell that the value was there all along — and it is also the tell that a second declaration of a name that already holds one must not empty it, because a shell that re-declared `h` here would answer `typeset h=''` and look like it had merely un-hidden an empty name",
	},
	{
		ID: "declare/hide-attribute-on-a-table", Category: "declarations",
		Snippet: `typeset -AH m; m[k]=v; echo "[${m[k]}]"; typeset -p m; echo "st=$?"`,
		Why:     "the spelling scripts actually use: the hiding letter bundled with the table letter, where the element still reads back and only the listing is short of it — `typeset -A m` in zsh against ksh93's `typeset -A -H m=([k]=v)`. bash 3.2 has no `-A` either, so the panel splits three ways over one word",
	},
	{
		ID: "declare/global-letter-with-the-table-attribute", Category: "declarations",
		Snippet: `typeset -gA m; m[k]=v; echo "[${m[k]}] st=$?"`,
		Why:     "the two letters together, which is not the sum of the rows that have each alone: the global letter says where the declaration lands and the table letter says what it is, and a shell that reads the first and drops the second leaves `m` an *indexed* array, so the very next `m[k]=v` is a non-numeric subscript and is refused. bash and zsh have both letters; ksh93 has no `-g` at all and, because its `typeset` is special, the refusal ends the script",
	},
	{
		ID: "declare/integer-attribute-added-to-a-name-with-a-value", Category: "declarations",
		Snippet: `v=5+2; typeset -i v; echo "[$v] st=$?"`,
		Why:     "the attribute arriving *after* the value, which is a different question from the two rows above it: bash leaves the four characters alone, while ksh93 and zsh read what the name already holds back through the attribute that just arrived and store 7. Either way the value survives — the answer no shell gives is the empty string, which is what a declaration that treated `typeset -i v` as `typeset -i v=` would produce",
	},
	{
		ID: "declare/case-attribute-added-to-a-name-with-a-value", Category: "declarations",
		Snippet: `d=MiXeD; typeset -u d; echo "[$d] st=$?"`,
		Why:     "the same question of a case letter, and the same split — `MIXED` in ksh93 and zsh, `MiXeD` in bash — which is what makes it a rule about attributes rather than about arithmetic. bash 3.2 has no `-u` at all and answers 2 while leaving the value where it is",
	},
	// The ties this shell arrives with, rather than the ones `typeset -T`
	// makes. Every row is a slice the case sets itself, because the values
	// these names really hold are the machine's.
	{
		ID: "tie/a-built-in-scalar-fills-its-array", Category: "declarations",
		Snippet: `CDPATH=a:b; echo "n=${#cdpath[@]} [${cdpath[*]}] st=$?"`,
		Why:     "`CDPATH` and `cdpath` are one value in zsh, so assigning the scalar fills the array: two elements. `cdpath` does not exist anywhere else and answers 0, which is what makes this the whole of the difference rather than a wording. Chosen over `PATH` because a case must not depend on what the machine's search path holds",
	},
	{
		ID: "tie/a-built-in-array-fills-its-scalar", Category: "declarations",
		Snippet: `cdpath=(x y); echo "CDPATH=[$CDPATH] st=$?"`,
		Why:     "the same tie from the array end — `x:y` — which is the half a one-directional mirror gets wrong while passing the row above. Everywhere else `cdpath` is an ordinary array nothing reads, so `CDPATH` stays empty",
	},
	{
		ID: "tie/writing-path-reaches-PATH", Category: "declarations",
		Snippet: `PATH=/aa:/bb; path=(/zz "${path[@]}"); echo "[$PATH] st=$?"`,
		Why:     "the line every rc file in the world writes, and the reason the ties matter at all: prepending to `path` puts the directory on the search path. `PATH` is set first so the row says the same thing on every machine — `/zz:/aa:/bb` in zsh against `/aa:/bb` everywhere else, where the array write reaches nothing and does so **silently**",
	},
	{
		ID: "tie/a-built-in-tie-lists-as-one", Category: "declarations",
		Snippet: `CDPATH=a; typeset -p CDPATH; echo "st=$?"`,
		Why:     "how a built-in tie says itself back: `typeset -T CDPATH cdpath=( a )` — both names and the array's elements — against bash's `declare -- CDPATH=\"a\"` and ksh93's bare `CDPATH=a`. The row that says the tie is a property of the name rather than something only assignments can see",
	},
	{
		ID: "tie/all-eight-built-in-pairs", Category: "declarations",
		Snippet: `MANPATH=a:b; PSVAR=c:d; FIGNORE=e:f; MODULE_PATH=g; MAILPATH=h:i; echo "${#manpath[@]} ${#psvar[@]} ${#fignore[@]} ${#module_path[@]} ${#mailpath[@]}"`,
		Why:     "the membership, in one row: five of the eight pairs at once, each counted rather than printed so the row says nothing about a machine. `2 2 2 1 2` in zsh and zeroes everywhere else. `PATH`, `FPATH` and `CDPATH` have rows of their own above; `ZSH_EVAL_CONTEXT` is deliberately absent, being a produced parameter rather than a tie",
	},
	{
		ID: "tie/FPATH-fills-fpath", Category: "declarations",
		Snippet: `FPATH=/x:/y; echo "n=${#fpath[@]} [${fpath[*]}] st=$?"`,
		Why:     "the tie `autoload` depends on: a function is looked for in `$fpath`, and `$fpath` is whatever `FPATH` says. Worth its own row rather than folding into the membership one, because this is the pair whose absence made every autoload a `function definition file not found` for a reason that had nothing to do with autoloading",
	},
	{
		ID: "declare/tie-mirrors-a-scalar-and-an-array", Category: "declarations",
		Snippet: `typeset -T TS ts; TS=a:b:c; echo "n=${#ts[@]} [${ts[*]}] st=$?"`,
		Why:     "the letter one shell in the panel has with this meaning: zsh ties a scalar to an array so each reflects the other, splitting the scalar on `:`. bash refuses `-T` with its usage line and 2. **ksh93 is the interesting column**: `typeset -T tname` declares a *type* there, so it takes this line without a word and answers `n=0 []` — the same letter, silently doing something else, which is why it must be refused by name in that dialect rather than shared as one attribute with two readings",
	},
	{
		ID: "declare/tie-mirrors-the-other-way-too", Category: "declarations",
		Snippet: `typeset -T TS ts; ts=(x y z); echo "TS=[$TS] st=$?"`,
		Why:     "the same tie from the array end — `x:y:z` — which is the half a one-directional mirror gets wrong while passing the row above. Both directions are needed or neither is measured",
	},
	{
		ID: "declare/tie-joins-a-later-append", Category: "declarations",
		Snippet: `typeset -T TS ts; ts=(x y); ts+=(w); echo "TS=[$TS] st=$?"`,
		Why:     "the tie is a property of the *pair* and not of the one assignment that made it, so an append rejoins: `x:y:w`. This is the row that separates a tie from a one-off split, and it is what makes `path=( new \"${path[@]}\" )` reach `PATH` at all",
	},
	{
		ID: "declare/tie-uses-the-separator-it-was-given", Category: "declarations",
		Snippet: `typeset -T TS ts '#'; TS=a#b#c; echo "n=${#ts[@]} [${ts[*]}] st=$?"`,
		Why:     "the third operand is the separator, not another name — three fields on `#` where the default would have found one. Worth a row because an implementation reading the third operand as a name would look right on every line that omits it",
	},
	{
		ID: "declare/unsetting-half-a-tie-unsets-all-of-it", Category: "declarations",
		Snippet: `typeset -T TS ts; TS=a:b; unset TS; echo "n=${#ts[@]} ts=[${ts[@]-UNSET}] st=$?"`,
		Why:     "half a tie is not a state this shell has: unsetting the scalar leaves the array unset too, not merely empty. The `-UNSET` default is what tells those apart, and an implementation that deleted one name and left the other would answer `n=0 ts=[]`",
	},
	{
		ID: "declare/a-tie-over-a-standing-value", Category: "declarations",
		Snippet: `TV=one:two:three; typeset -T TV tv; echo "n=${#tv[@]} [${tv[*]}] st=$?"`,
		Why:     "the tie arriving *after* the value, which is the question the integer and case rows ask of their attributes: zsh reads what the scalar already holds back through the tie and fills the array. The answer no shell gives is an empty pair, which is what a tie that treated the declaration as an assignment would leave — and it is exactly what a tie made over `PATH` would then have done to `PATH`",
	},
	{
		ID: "declare/tying-a-name-to-itself", Category: "declarations",
		Snippet: `typeset -T TZ TZ; echo "st=$?"`,
		Why:     "`can't tie a variable to itself` in zsh, and it ends the script — where `can't tie already tied scalar` and `second argument of tie must be array` are both reported and run on. That split is this shell's own and the row is what records it rather than letting one fatality rule be assumed for all three",
	},
	{
		ID: "declare/tie-listed-back", Category: "declarations",
		Snippet: `typeset -T TL tl; tl=(1 2); typeset -p TL; echo "st=$?"`,
		Why:     "where a tie is said back: `typeset -T TL tl=( 1 2 )` in zsh — both names, the *array's* elements as the value whichever half was asked for, and the `T` letter last of all of them. Narrowed to one name the case invents, because a whole listing is the machine's",
	},
	{
		ID: "declare/unique-attribute-dedupes-an-array", Category: "declarations",
		Snippet: `typeset -U a=(1 1 2 2 3 1); echo "a=[${a[*]}] st=$?"`,
		Why:     "the letter one shell in the panel has: zsh keeps the first occurrence of each element and answers `1 2 3`. bash refuses `-U` with its usage line and 2, ksh93 answers `unknown option` with its own usage line, and because its `typeset` is special the refusal ends the script — so the row also records that this is a letter to refuse by name rather than an axis to answer",
	},
	{
		ID: "declare/unique-attribute-dedupes-a-later-append", Category: "declarations",
		Snippet: `typeset -U a=(1 2 3); a+=(2 4 4); echo "a=[${a[*]}] st=$?"`,
		Why:     "the attribute is a property of the *name*, so the append is deduped too and against what is already there — `1 2 3 4`, not `1 2 3 2 4`. This is the row that separates the attribute from a one-off dedupe at the declaration, and it is the whole reason `path=( new \"${path[@]}\" )` does not grow a duplicate every time an rc file runs",
	},
	{
		ID: "declare/unique-attribute-added-to-an-array-with-duplicates", Category: "declarations",
		Snippet: `b=(1 1 2); typeset -U b; echo "b=[${b[*]}] st=$?"`,
		Why:     "the attribute arriving after the value, the same question the integer and case rows ask: zsh dedupes what the name already holds on the spot and answers `1 2`. The answer no shell gives is the empty string, which is what a declaration reading `typeset -U b` as `typeset -U b=` would produce",
	},
	{
		ID: "declare/unique-attribute-keeps-the-earlier-place", Category: "declarations",
		Snippet: `typeset -U b=(1 2 3); b=(3 "${b[@]}"); echo "b=[${b[*]}] st=$?"`,
		Why:     "which of two equal elements survives, put where a first-wins and a last-wins reading answer differently: zsh answers `3 1 2`, so the occurrence written *first* is kept where it stands and the later copy goes. A last-wins dedupe answers `1 2 3` to the same line and would pass every other row here",
	},
	{
		ID: "declare/unique-attribute-removed", Category: "declarations",
		Snippet: `typeset -U c=(1 2 3); typeset +U c; c+=(1); echo "c=[${c[*]}] st=$?"`,
		Why:     "`+U` takes the attribute away and leaves the elements alone — `1 2 3 1` in zsh, the duplicate the append brought standing. It is also the tell that the dedupe happens when a value is written rather than when one is read: a read-time dedupe would still answer `1 2 3` here",
	},
	{
		ID: "declare/unique-attribute-on-a-scalar", Category: "declarations",
		Snippet: `typeset -U s=a:b:a; echo "s=[$s] st=$?"`,
		Why:     "the letter is about an array's elements, and a colon-joined string is not elements: zsh leaves `a:b:a` exactly as written. Worth a row because the letter is used almost entirely on `path` and `fpath`, whose scalar halves *are* colon lists, and a shell that deduped the scalar too would look right on those names and wrong on every other",
	},
	{
		ID: "declare/unique-attribute-listed-back", Category: "declarations",
		Snippet: `typeset -aU q=(1 1 2); typeset -p q; echo "st=$?"`,
		Why:     "where the attribute is said back: `typeset -aU q=( 1 2 )` in zsh — the letter written last of all of them, after the kind letter rather than before it. Narrowed to one name the case invents, because a whole listing is the machine's",
	},
	{
		ID: "set/bare-set-and-a-hidden-value", Category: "builtins",
		Snippet: `typeset -H zzh=hid; zzv=plain; set | grep '^zz'; echo "st=$?"`,
		Why:     "the hiding letter reaches the other listing too: zsh writes the bare name `zzh` where every other shell writes `zzh=hid` or nothing, and the ordinary `zzv=plain` on the next line is what proves the listing ran rather than failing. Filtered to two names the script invents, because the rest of a bare `set` is the machine's",
	},
	{
		ID: "declare/an-option-typeset-does-not-have", Category: "declarations",
		Snippet: `typeset -q v=1; echo "st=$?"`,
		Why:     "the refusal splits three ways: reported with a usage line and status 2, reported bare with 1, and fatal with the usage lines in the shell whose typeset failures end the script",
	},
	{
		ID: "declare/an-option-written-with-a-plus", Category: "declarations",
		Snippet: `typeset +q v; echo "st=$?"`,
		Why:     "the sign a refusal names is the one the word was written with — `+q` in all four shells that have the builtin, each in its own words and its own status. It is the only place a shell spells an option with a plus, so it is the only place the question can be asked, and a refusal that normalized the word to `-q` would be pointing at a word the script never wrote",
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
		ID: "pipefail/a-builtin-killed-by-a-signal", Category: "pipeline status",
		Snippet: "if ( set -o pipefail ) 2>/dev/null; then set -o pipefail; fi\nv=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done\n{ echo \"$v\"; } | true; echo \"st=$?\"\n",
		Why:     "the status pipefail hands back for an element a signal killed, which is not the status that death reports everywhere else in the same shell: 141 in bash and zsh, and in ksh93 **13** rather than the 269 its own convention would give. The string is grown past a pipe buffer on purpose, so the write is certain to outlive the reader rather than fitting in the pipe and racing it",
	},
	{
		ID: "pipefail/an-external-command-killed-by-a-signal", Category: "pipeline status",
		Snippet: "if ( set -o pipefail ) 2>/dev/null; then set -o pipefail; fi\nyes 2>/dev/null | head -1 >/dev/null; echo \"st=$?\"\n",
		Why:     "the same question with a real process on the writing end rather than a builtin, and the same three answers — so the split is about the substitution and not about which side of the process boundary the death happened on",
	},
	{
		ID: "pipefail/an-ordinary-failure-is-substituted-unchanged", Category: "pipeline status",
		Snippet: "if ( set -o pipefail ) 2>/dev/null; then set -o pipefail; fi\n(exit 42) | true; echo \"st=$?\"\n(exit 42) | { echo x >/dev/null; } | true; echo \"st=$?\"\n",
		Why:     "the control: an element that merely *failed* is handed back as it stands in every shell with the option, first or middle. Without this the signal rows read as a difference about pipefail in general rather than about a signal death in particular",
	},
	{
		ID: "pipefail/a-substituted-death-against-an-ordinary-one", Category: "pipeline status",
		Snippet: "if ( set -o pipefail ) 2>/dev/null; then set -o pipefail; fi\nv=x; i=0; while [ $i -lt 17 ]; do v=$v$v; i=$((i+1)); done\n{ echo \"$v\"; } | true; a=$?\nb=$( { sleep 5 & p=$!; kill -TERM $p; wait $p; echo $?; } 2>/dev/null )\necho \"substituted=$a foreground=$b\"\n",
		Why:     "both encodings in one line, which is the whole point: ksh93 says 13 for the substituted death and 271 for the waited-for one, so its 256-plus-the-signal convention is intact and stops in exactly one place. bash and zsh say 141 and 143 and never distinguish the two. dash has neither the option nor a second answer. The job's own death notice is thrown away because it names a pid, and a pid is different on every run",
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
		ID: "cmd/an-empty-brace-group", Category: "command language",
		Snippet:     `{ }; echo ok`,
		SyntaxError: true,
		Why:         "a compound command's body may not be empty: dash, both bash builds and ksh93 refuse the brace group and name the `}` they met where a command belonged, and zsh alone takes it. The core is the language every shell accepts, so an empty body is outside it and a dialect adds it back rather than the core allowing what four shells reject",
	},
	{
		ID: "cmd/an-empty-subshell", Category: "command language",
		Snippet:     `( ); echo ok`,
		SyntaxError: true,
		Why:         "the same rule with parentheses, which is what shows it is a rule about bodies rather than about braces — and it is not a `}` this time, so a shell that special-cased the brace would take it. The same four refuse and the same one takes it",
	},
	{
		ID: "cmd/an-empty-loop-body", Category: "command language",
		Snippet:     `while false; do done; echo ok`,
		SyntaxError: true,
		Why:         "the keyword form of the same rule: `done` where a command belongs. Worth its own case because a loop's body has a terminator of its own, so a parser could reach the end of it without ever asking whether anything was in it",
	},
	{
		ID: "cmd/an-empty-command-substitution", Category: "command language",
		Snippet: `x=$( ); echo "[$x] ok"`,
		Why:     "the counter-case, and the line the rule stops at: a command substitution's body is a whole program rather than a compound command's body, and every shell in the panel takes an empty one. Without it a shell could refuse every empty thing and look right on four cases out of four",
	},
	{
		ID: "cmd/a-case-with-no-arms", Category: "command language",
		Snippet: `case x in esac; echo ok`,
		Why:     "the other line the rule stops at, and the panel splits the other way here: a `case` with no arms is a list of *arms* rather than a command list, and dash, bash and zsh take it where ksh93 alone refuses. Recorded so the empty-body rule is not quietly widened to cover it",
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
		ID: "arith/one-float-makes-the-expression-float", Category: "arithmetic",
		Snippet: `echo $((3.0/2))`,
		Why:     "the same division with one operand written as a float, which is the whole difference: 1.5 where the dialect has floats and not a number at all where it does not",
	},
	{
		ID: "arith/a-whole-float-keeps-its-point", Category: "arithmetic",
		Snippet: `echo $((1.5+2.5))`,
		Why:     "zsh writes a trailing point so a float still reads as one, and ksh93 writes the integer — same arithmetic, two spellings of four",
	},
	{
		ID: "arith/float-precision-differs", Category: "arithmetic",
		Snippet: `echo $((0.1+0.2))`,
		Why:     "the classic float, and the two shells show it differently: 15 significant digits rounds it to 0.3 and 17 does not, so the precision is a value the dialect supplies",
	},
	{
		ID: "arith/a-float-may-begin-with-its-point", Category: "arithmetic",
		Snippet: `echo $((.5))`,
		Why:     "`.5` is a literal where the dialect has floats and, where it does not, an operator that cannot be one — which is why the shells without floats blame the point rather than the number it was part of",
	},
	{
		ID: "arith/a-float-may-carry-an-exponent", Category: "arithmetic",
		Snippet: `echo $((1.5e2))`,
		Why:     "the exponent needs its own scan, because the digit reader accepts `e` as a hex digit and stops at a sign — written with its point so that every dialect divides it into the same tokens, which `1e-3` does not: bash reads `1e` as a number with a bad digit and dash reads `1` and stops",
	},
	{
		ID: "arith/hex-is-not-a-float", Category: "arithmetic",
		Snippet: `echo $((0x1e)) $((0x1e-3))`,
		Why:     "the counter-case that keeps the exponent scan honest: `0x1e` is an integer whose digits include an `e`, and the second half is the one that bites — read as an exponent, `0x1e-3` is a literal that will not parse rather than a subtraction",
	},
	{
		ID: "arith/a-remainder-of-floats", Category: "arithmetic",
		Snippet: `echo $((7%2.5))`,
		Why:     "zsh takes a remainder in floating point and ksh93 refuses a float here at all, so `%` is not simply an integer operator in both",
	},
	{
		ID: "arith/a-bitwise-operator-on-a-float", Category: "arithmetic",
		Snippet: `echo $((1.5 & 1))`,
		Why:     "and here they part the other way: zsh truncates to an integer and ksh93 refuses, which is the axis — a remainder is not the same question as a bitwise and",
	},
	{
		ID: "arith/comparing-floats-yields-a-truth", Category: "arithmetic",
		Snippet: `echo $((1.5 < 2))`,
		Why:     "a comparison answers 0 or 1 whatever it compared, so the shell that writes a point after a whole float does not write one here",
	},
	{
		ID: "arith/a-float-assignment-outlives-the-expression", Category: "arithmetic",
		Snippet: `i=0; echo $((i+=1.5)); echo "[$i]"`,
		Why:     "the value left behind is the float and not its truncation, which is what makes the stored form the dialect's business as much as the printed one",
	},
	{
		ID: "arith/dividing-a-float-by-zero", Category: "arithmetic",
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
		ID: "pat/a-group-arm-that-matches-nothing", Category: "pattern matching",
		Snippet: "[[ b == @(|a)b ]] && echo one || echo no-one\n[[ b == @(*)b ]] && echo two || echo no-two\n[[ \"\" == @(a|) ]] && echo three || echo no-three\n[[ \"\" == +(|a) ]] && echo four || echo no-four",
		Why:     "one repetition of an arm that matches no text is no text, so a group with such an arm stands for nothing — however it is quantified and including not at all. Unanimous in the shells that read the construct here, and this answered the opposite way in every dialect: the matcher tried the group against one character of the subject and upward, so a zero-length arm could never be reached. Asked as \"can an arm match nothing\" and not \"is an arm empty\", which is what the `@(*)b` line is for — a rule written the second way answers it wrong. Written in a condition rather than in a `case` so that bash reaches it without `extglob`, which is not in force until the line after the one that sets it",
	},
	{
		ID: "pat/the-shortest-match-of-a-group-may-be-nothing", Category: "pattern matching", SyntaxError: true,
		Snippet: "shopt -s extglob 2>/dev/null\nv=abc; echo \"[${v#@(|a)}][${v##@(|a)}]\"\ncase b in @(|a)b) echo m;; *) echo no;; esac",
		Why:     "the same rule reaching the trim operators, where it is the whole difference between the two of them: `#` takes the shortest match an alternative offers and the empty one is the shortest there is, so it trims nothing, while `##` takes the longest and trims the `a`. This trimmed the `a` for both — a wrong *value* rather than a match not made, which is the failure mode a status can never show. `shopt` is on a line of its own because the option is not in force until the next line is parsed, and silenced because two panel shells have no such builtin and one of those reads the group natively anyway",
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
		ID: "procsub/an-operand-of-an-expansion", Category: "pattern matching",
		Snippet: `unset u; p=${u:-<(:)}; case "$p" in /*) echo pathy;; *) echo "[$p]";; esac`,
		Why:     "whether `<(cmd)` opens a process substitution *inside* a `${…}` operand, which is a question about the position rather than about the shell: bash answers with a path and ksh93, zsh and dash with the five characters — and the last two have process substitution everywhere else, so this is not the construct being absent. The operand is a word here rather than a pattern, because a path is only visible in a value: as a pattern it merely fails to match, which the literal reading does too",
	},
	{
		ID: "procsub/an-operand-that-is-a-pattern", Category: "pattern matching",
		Snippet: `v='<:'; echo "[${v#<(:)}]"; v=':'; echo "[${v#<(:)}]"`,
		Why:     "and the same question where the wrong answer is silent. Where the characters are not a substitution they are pattern text — a `<` and, in the two shells with bare groups, the group `(:)` — so the first line strips in ksh93 and zsh and leaves the value standing in bash and dash. The second line is the one that caught the bug: a reading that took the substitution's *inner* text as the pattern trimmed a bare `:` from `:`, which no column here does",
	},
	{
		ID: "pat/an-operand-out-of-a-command-substitution", Category: "pattern matching",
		Snippet: `v=abcd; echo "[${v#$(echo ab)}]"`,
		Why:     "a pattern operand is a word and is expanded like one, unanimously across the panel — the row that was missing while `${v#$(echo ab)}` answered `abcd` here, stripping the five characters `echo ab` from a string that does not begin with them. No diagnostic, no status, a plausible string: exactly the shape the corpus exists to catch, and a variable holding the same pattern worked, which is what hid it",
	},
	{
		ID: "pat/an-operand-out-of-a-backquoted-substitution", Category: "pattern matching",
		Snippet: "v=abcd; echo \"[${v#`echo ab`}]\"",
		Why:     "the other spelling of the same substitution, which the parser records as a different span and which an implementation reaching for one kind by name would leave behind",
	},
	{
		ID: "pat/an-expanded-operands-metacharacter", Category: "pattern matching",
		Snippet: `v=abcdabcd; echo "[${v##$(echo 'a*a')}]"`,
		Why:     "and what the expanded text is then worth: a `*` that arrived from a substitution is a metacharacter in dash, bash and ksh93 and an ordinary character in zsh, which is the same axis that decides `p='a*a'; ${v##$p}` and `x='et*'; echo $x`. One answer observed in a third place rather than a quirk of this operator, and the reason the operand's *word* is what becomes the pattern",
	},
	{
		ID: "pat/an-operand-out-of-an-arithmetic-expansion", Category: "pattern matching",
		Snippet: `v=2bcd; echo "[${v#$((1+1))}]"`,
		Why:     "the same omission asked about the other substitution: unanimous, and the digit is what makes it visible — an operand of `$((1+1))` left unexpanded is the four characters `1+1`, which strips nothing from a value beginning with a 2",
	},
	{
		ID: "pat/a-replacements-pattern-out-of-a-substitution", Category: "pattern matching",
		Snippet: `v=abcd; echo "[${v/$(echo bc)/X}]"`,
		Why:     "the replacement operator's pattern side, which is a separate call site from the trims and was wrong in the same way. dash has no `/` at all and says so, so this is the three-shell agreement rather than the panel's",
	},
	{
		ID: "pat/a-case-arm-out-of-a-command-substitution", Category: "pattern matching",
		Snippet: `case ab in $(echo ab)) echo hit;; *) echo miss;; esac`,
		Why:     "the same word in the other construct that takes a pattern, and unanimous: an arm is expanded before it is matched. Worth its own row because the two constructs are the same function here — a report that this position was *unaffected* was measured against the globbing question and did not ask the expansion one",
	},
	{
		ID: "pat/a-case-arms-metacharacter-out-of-a-substitution", Category: "pattern matching",
		Snippet: `case abc in $(echo 'a*')) echo hit;; *) echo miss;; esac`,
		Why:     "and the arm's half of the globbing axis, which is where the two questions come apart: every shell expands the arm and only zsh then declines to read a `*` in what came back. So `case` is not exempt from the expansion and is exempt from the glob in exactly one shell",
	},
	{
		ID: "pat/a-condition-operand-out-of-a-command-substitution", Category: "pattern matching",
		Snippet: `[[ ab == $(echo ab) ]] && echo hit || echo miss`,
		Why:     "the third construct, agreeing with the other two in every shell that has `[[ ]]` at all. dash has none and reports the words as a command, which is the row's other half: a condition is a keyword rather than a builtin",
	},
	{
		ID: "pat/an-operand-holding-a-dollar-single-escape", Category: "pattern matching",
		Snippet: "v=$'\\tx'; echo \"[${v#$'\\t'}]\"",
		Why:     "quoting that carries an escape is decoded in a pattern operand as it is anywhere else — the same tab on both sides, so the prefix goes. Unanimous, including dash by a different route: it has no such quoting, so both sides are a literal dollar and a backslash-t and they still match each other",
	},
	{
		ID: "pat/a-process-substitution-in-an-operand", Category: "pattern matching",
		Snippet: `v=abcd; echo "[${v#<(:)}]"`,
		Why:     "the substitution the panel does *not* agree about, recorded here so that the agreement above is not read as covering it: only bash runs the command in this position, and no shell's pattern then matches, so the value comes back whole in all six. The visible half is unanimous and the invisible half is not, which is why this is a pinned row rather than an implemented behavior",
	},
	{
		ID: "pat/an-operand-expands-once-for-a-whole-array", Category: "pattern matching",
		Snippet: `a=(abc bcd); printf "[%s]" "${a[@]#$(printf x >>marks; echo a)}"; printf "n=%s\n" "$(cat marks)"`,
		Why:     "one operand, one expansion, however many elements it is applied to: the mark file holds a single x in every shell with arrays. The alternative reading is not visible in the fields — both spellings print the same two — and shows only in how many times the word's side effects fired, which is what makes it worth a file rather than an assertion about output",
	},
	{
		ID: "pat/a-numeric-range-is-any-number", Category: "pattern matching", SyntaxError: true,
		Snippet: `[[ 1 = <-> ]]; echo "st=$?"; [[ x = <-> ]]; echo "st=$?"`,
		Why:     "the gate a plugin manager's whole file sits behind — `[[ $1 = <-> && … ]]` — and a `<` where four of the panel read only a redirection. It matches a run of digits and nothing else, so the second half is what says it is a pattern rather than a truth. bash's row is three lines because a conditional operator's refusal is followed by the offending token and the source line, which is the older gap `cond/a-pattern-operand-may-start-with-a-group` records; dash has no `[[ ]]` at all and tries to open a file called `-`",
	},
	{
		ID: "pat/a-numeric-range-has-four-shapes", Category: "pattern matching", SyntaxError: true,
		Snippet: `case 42 in <1-100>) echo in;; esac; case 200 in <1-100>) echo in;; *) echo out;; esac; case 5 in <6->) echo up;; *) echo notup;; esac; case 5 in <-4>) echo down;; *) echo notdown;; esac`,
		Why:     "`<n-m>`, `<n->`, `<-m>` and the bare `<->` are one operator with either bound left out, and the comparison is on the *value*: the range is the whole of what distinguishes it from four ordinary characters. Written as `case` arms rather than conditions so that what the other four say is their ordinary syntax error rather than the three-line conditional one",
	},
	{
		ID: "pat/a-numeric-range-compares-values-not-text", Category: "pattern matching", SyntaxError: true,
		Snippet: `case 007 in <1-10>) echo seven;; *) echo no;; esac; case 100 in <1-10>0) echo backtrack;; *) echo no;; esac; case 12 in <-><->) echo split;; *) echo no;; esac`,
		Why:     "leading zeros belong to the run of digits and not to the number, so `007` is 7; and the run's length is decided by what follows it, which is why `<1-10>0` matches `100` and two ranges in a row split `12` between them. A matcher that took the longest run it could would fail both",
	},
	{
		ID: "pat/a-substituted-default-in-an-assignment", Category: "pattern matching",
		Snippet: `: > gf.a; : > gf.b; unset u; p=${u:-gf.*}; printf "[%s]" "$p"; unset v; printf "[%s]" ${v:-gf.*}; echo`,
		Why:     "where a substituted word is matched against the filesystem and where it is not, unanimous in all five: the value an assignment stores is the pattern as written, and the same word standing as an argument is the two files. One row rather than two because either half alone reads as a rule about `:-` — it is the *position* that decides, and only the pair says so. This answered the listing on both sides until #995, the assignment's value having been built through the entry point that promises no matching while the substituted word inside it went through the one that does",
	},
	{
		ID: "pat/a-trailing-group-is-a-list-of-qualifiers", Category: "pattern matching", SyntaxError: true,
		Snippet: `mkdir -p qd/d1; : > qd/f1; : > qd/f2; ln -s f1 qd/l1; cd qd; printf "[%s]" *(.); printf "[%s]" *(/); printf "[%s]" *(@); printf "[%s]" *(^.); echo`,
		Why:     "one shell reads the parentheses at the end of a pattern as a list of qualifiers narrowing what it matched, and the other four read `*(` as a syntax error. Four probes rather than one because a single type test cannot show that the list is a *filter*: the regular files, the directories, the symbolic links — `l1` points at a regular file and is still a link, so the test does not follow it — and then `^`, which turns the sense of what follows and answers with everything the first probe left out",
	},
	{
		ID: "pat/a-qualifier-list-is-not-an-alternation", Category: "pattern matching", SyntaxError: true,
		Snippet: `mkdir -p qd; : > qd/f1; : > qd/f2; cd qd; printf "[%s]" f1(.); printf "[%s]" f(1|2); printf "[%s]" zz*(N); echo; printf "[%s]" *(qq); echo after`,
		Why:     "the disambiguation is exactly one character, and the row is four readings of the same three-character shape. A group with no `|` is a list — so `f1(.)` sends a literal name to the filesystem, a name being no pattern on its own. A group *with* one is the alternation that shell already had, and `f(1|2)` matches two files where `1` alone would be an unknown attribute. `N` makes a miss no error and deletes the word, which is why `printf` still writes its format once. And a character no qualifier claims is named and fatal, so `after` is not reached",
	},
	{
		ID: "pat/a-qualifier-list-reads-the-permission-bits", Category: "pattern matching", SyntaxError: true,
		Snippet: `mkdir -p pq; : > pq/plain; : > pq/prog; chmod 644 pq/plain; chmod 755 pq/prog; cd pq; printf "[%s]" *(x); echo; printf "[%s]" *(^x); echo; printf "[%s]" *(w); echo; printf "[%s]" *(W); echo after`,
		Why:     "the nine permission letters, and the row is written to separate the three triples rather than to show one of them working. `x` is owner-execute, so 0755 passes and 0644 does not, and `^x` answers with the other file. `w` is owner-write and holds of both; `W` is *world*-write and holds of neither, which is the only difference between the two triples the fixture can show — and it makes the miss fatal, so `after` is not reached in the shell that has qualifiers either. The other four refuse `*(` while parsing (#1053)",
	},
	{
		ID: "pat/a-bare-qualifier-list-names-the-word", Category: "pattern matching", SyntaxError: true,
		Snippet: `mkdir -p bq; : > bq/prog; chmod 755 bq/prog; cd bq; printf "[%s]" prog(x); echo; printf "[%s]" (x); echo after`,
		Why:     "a group standing for the whole word is a list over an *empty* pattern, and an empty pattern matches nothing — so `(x)` names the word and not the letter, where `prog(x)` sends a literal name to the filesystem and finds it. This was filed as the qualifier production winning where that shell reads an alternation, and it is neither: `x` is the owner-execute test, and the only thing wrong was refusing a letter the language has (#1053)",
	},
	{
		ID: "pat/a-glob-flag-where-an-array-element-begins", Category: "pattern matching", SyntaxError: true,
		Snippet: `files=( (#i)zz ); echo "n=${#files[@]}"; echo after`,
		Why:     "the same rule as `a-paren-where-an-argument-stands`, one production over: an array literal's element stands where an argument does, so the `(` belongs to the element and not to the assignment. The assignment used to find its closing `)` by counting, and a flag opens one that is part of a word — `expected ) to close an array assignment` where the shell reaches the pattern and reports the miss. The pattern matches nothing on purpose: the row is about the element being *read*, and an unmatched pattern is fatal there, so `after` is never printed in any of the five",
	},
	{
		ID: "pat/a-glob-flag-on-a-later-array-element", Category: "pattern matching", SyntaxError: true,
		Snippet: `files=( x (#i)zz ); echo "n=${#files[@]}"; echo after`,
		Why:     "and the second element, which is a different parser state from the first — a word has been read, so a `(` after one had already to be told apart from the assignment's own. Worth its own row because a fix that only handled the leading position would leave this one counting",
	},
	{
		ID: "pat/a-glob-flag-where-a-loop-item-begins", Category: "pattern matching", SyntaxError: true,
		Snippet: `for x in (#i)zz; do echo "got=$x"; done; echo after`,
		Why:     "the same rule two productions over: a loop's item stands where an argument does, so the `(` belongs to the item. Refused by bash 5.3, bash 3.2, dash and ksh93 at the `(` and read by zsh, which then reports the miss — the pattern matches nothing on purpose, so the row is about the item being *read* rather than about what it matched (#1161)",
	},
	{
		ID: "pat/a-glob-flag-on-a-later-loop-item", Category: "pattern matching", SyntaxError: true,
		Snippet: `for x in a (#i)zz; do echo "got=$x"; done; echo after`,
		Why:     "and behind a word, which is the other parser state — the same pairing the array literal's two rows have, and for the same reason: a fix that only reached the leading position would leave this one refusing",
	},
	{
		ID: "pat/a-glob-flag-where-a-select-item-begins", Category: "pattern matching", SyntaxError: true,
		Snippet: `select x in (#i)zz; do :; done </dev/null; echo "st=$? after"`,
		Why:     "the menu loop's list is the same list, and it had its own copy of the reader — which is what makes it worth a row of its own rather than an assumption. dash has no `select` at all and refuses the word before it can reach the paren, which is the row that shows the two refusals are different refusals",
	},
	{
		ID: "pat/a-glob-flag-where-a-case-arm-begins", Category: "pattern matching", SyntaxError: true,
		Snippet: `case F.ZIP in (#i)*.zip) echo hit;; *) echo miss;; esac`,
		Why:     "a `case` arm's pattern is the third position, and the hardest: the arm has an optional leading `(` of its own, so a group at the front of the pattern and the arm's own paren are the same character. zsh reads `(#i)*.zip` as the pattern and the `)` behind it as the arm's, where the four others refuse at the paren. The flag needs `extended_glob` to *match*, which no column has on here, so every one of them prints `miss` or refuses — the row is about the parse",
	},
	{
		ID: "pat/a-case-arms-own-paren-in-front-of-a-group", Category: "pattern matching", SyntaxError: true,
		Snippet: `case b in ((a|b)) echo hit;; *) echo miss;; esac`,
		Why:     "the same arm with its optional paren written as well, and **no `#` anywhere in it** — which is what says this half is not about the flag's spelling at all. Two parens where no command may begin were read as an arithmetic command here, and `parse error near 'arithmetic command'` is a complaint about something the script never wrote. zsh takes it as the arm's paren in front of an alternation group and matches `b`; bash, dash and ksh93 all refuse it, so this is the one row of the family that separates zsh from the whole rest of the panel (#1161)",
	},
	{
		ID: "pat/a-case-arms-paren-in-front-of-an-expression", Category: "pattern matching", SyntaxError: true,
		Snippet: `case 1 in ((1)) echo hit;; *) echo miss;; esac`,
		Why:     "the same shape with no pattern language in it whatsoever, which is the row that pins *why* the arithmetic reading was wrong rather than merely that it was: `((1))` is a perfectly good expression anywhere a command may begin, and an arm is not one of those places. The control for it is `case x in a) ((1));; esac`, where an arm's **body** is ordinary commands and the expression really is one — a suspension left on for the body would have broken that instead",
	},
	{
		ID: "loop/no-word-in-an-item-list-is-reserved", Category: "commands",
		Script:  true,
		Snippet: "for x in do; do echo \"1=$x\"; done\nfor x in done; do echo \"2=$x\"; done\nfor x in a b\ndo echo \"3=$x\"; done\necho after\n",
		Why:     "the item list ends at a `;` or a newline and at nothing else — no word in it is a reserved word — and this is **unanimous across all six columns**, so it is core and not a dialect's. A loop over a list holding the word `done` is not exotic: `for f in $(ls)` reaches it the moment a file is called that. This engine read `do` and `done` as stop words here and refused all three of these lines (#1161)",
	},
	{
		ID: "loop/an-item-list-with-no-separator-before-do", Category: "commands", SyntaxError: true,
		Snippet: `for x in a b do echo "got=$x"; done; echo after`,
		Why:     "the other direction of the same rule, and the worse half: with no separator the `do` is an *item*, so `done` stands where `do` belongs and every one of the six refuses the line. This engine accepted it — the list stopped at the stop word — and then ran the body with `do` bound as a value and said nothing. A rule that only added the reserved words to the list would pass the row above and fail this one, which is why the pair is the measurement",
	},
	{
		ID: "pat/a-paren-where-an-argument-stands", Category: "pattern matching", SyntaxError: true,
		Snippet: `echo MY ( x ); echo after`,
		Why:     "the same three characters are a syntax error after a word in four shells and one argument in the fifth, where they are a pattern carrying a qualifier list — the complaint names the *space* inside the group, which is what says it was read as a list and not as anything of the shell's. Command position is the whole of the difference there: `( x )` written where a command begins is still a subshell, which `core/two-subshells-with-nothing-between-them` still pins from the other side",
	},
	{
		ID: "pat/a-quoted-numeric-range-is-four-characters", Category: "pattern matching",
		Snippet: `[[ 1 = "<->" ]] && echo hit || echo miss; [[ "<->" = "<->" ]] && echo hit || echo miss; p="<->"; [[ 1 = $p ]] && echo hit || echo miss`,
		Why:     "the same per-span quoting that decides whether `a*` is a pattern decides whether a range is one, and the shell that has ranges is also the one that does not re-read an expansion as a pattern — so the third answer is `miss` there for a reason unrelated to the first two. This is the case that keeps the fix from being `a `<` is always a range`, and it parses everywhere because the quotes take the `<` out of the grammar's hands",
	},
	{
		ID: "pat/a-numeric-range-in-a-case-arm", Category: "pattern matching", SyntaxError: true,
		Snippet: `case 42 in <->) echo num;; *) echo other;; esac; case abc in <->) echo num;; *) echo other;; esac`,
		Why:     "the same pattern where a `case` arm stands, which is a different production from the condition's operand and had to be measured on its own — dash reports a word where it wanted `)` and the rest report the `<`",
	},
	{
		ID: "pat/a-numeric-range-in-a-group-starting-a-case-arm", Category: "pattern matching", SyntaxError: true,
		Snippet: `case 6 in ((<6->)) echo up;; *) echo notup;; esac; case 4 in ((<6->)) echo up;; *) echo notup;; esac; case 6 in ((6|7)) echo alt;; *) echo noalt;; esac`,
		Why:     "the same word-leading group scanner reached from a `case` arm rather than from a condition, which is what says the gap was the scanner's and not `[[ ]]`'s. The arm has a paren of its own, so a group that starts the pattern is written `((` — the shape #1161 already had to keep away from the arithmetic command — and the third arm is the control that had always worked",
	},
	{
		ID: "pat/a-numeric-range-in-a-group-starting-an-argument", Category: "pattern matching", SyntaxError: true,
		Snippet: `touch v5 v6 va; echo v<5-6>; echo v(<5-6>); echo after`,
		Why:     "the third route into the same scanner, and the one whose answer is not a match: where a group *starts* a word in argument position the shell with bare groups reads its parentheses as glob qualifiers instead, so `v(<5-6>)` is `unknown file attribute` rather than a pattern — while the bare range beside it expands. Recorded because the fix for #1217 has to reach that reading rather than turn it into a parse error, and a silent expansion there would be the wrong answer at status 0",
	},
	{
		ID: "pat/a-numeric-range-against-the-filesystem", Category: "pattern matching", SyntaxError: true,
		Snippet: `touch 1 2 10 007 21 22 abc; echo <->; echo <2-9>; echo 2<->; echo <->zzz; echo after`,
		Why:     "the whole of the expansion half in one row: a range is matched per component and sorted with everything else, the digits in front of one are part of the word rather than a file descriptor — `2<->` names `21` and `22` and is not a redirection of descriptor 2 — and a miss is the ordinary unmatched-pattern answer, which in this shell stops the command, so `after` never runs. ksh93's cell is the second finding here and is not this change's: it *parses* `echo <->` as a redirection from a file called `-` where we refuse the `;` after it, which is a gap in the ksh grammar rather than in the range",
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
		ID: "heredoc/two-on-one-command-and-the-last-is-read", Category: "redirection",
		Script:  true,
		Snippet: "cat <<A <<B\na\nA\nb\nB\n",
		Why:     "two here-documents on *one* command rather than on two, which is the shape that says what a second input redirection does to the first. Five of the six keep only the last, so `cat` reads B and prints `b`; zsh concatenates them and prints both, which is the input half of the same rule that makes two output redirections write to both files. The row is also the printer's: the second body follows the first delimiter's line, so a newline written in front of it opens B with a blank line and the command prints one before `b` (#962)",
	},
	{
		ID: "heredoc/a-body-that-never-ended-its-last-line", Category: "redirection",
		Script:     true,
		Unfinished: true,
		Snippet:    "cat <<X\nbody",
		Why:        "`heredoc/no-delimiter-and-a-warning` with the final newline taken away, which is the only way to write a here-document body that does not end in one. What moves is where the remark is located: the line the input ran out on is 2 here and 3 there, so it is the line the last character sat on rather than the count of lines the file has. The body and the status are the same in all six. It is the shape that found #962 — the printer wrote the delimiter onto that unfinished last line and the body became `bodyEND` — and it is the round trip's only case of a here-document with no delimiter of its own",
	},
	{
		ID: "arith/expansion-happens-before-reading", Category: "arithmetic",
		Snippet: `x='1+'; y=2; echo $(( $x$y ))`,
		Why:     "the substitution is textual and comes first, so the *result* is the expression — 3, which no tree built from `$x$y` as written could give, and the reason an expression containing a `$` has no tree until it runs",
	},
	{
		ID: "arith/an-expansion-inside-is-not-split", Category: "arithmetic",
		Snippet: `IFS=1; x="1 + 2"; echo "[$(( $x ))]"; set -- $x; echo "n=$#"`,
		Why:     "the whitespace-bearing half of `expand/arithmetic-text-is-not-split`, which asks the same question of a value with no blanks in it. Splitting on a *space* is what a shell does by default and without being told to, so a value holding one is the shape an exemption is most likely to be missing for, and IFS is set to a digit here so a split would visibly eat the operands rather than merely rearrange them. The second line is the control the older case has no room for: the same value in an ordinary command position *is* split, which is what makes this an exemption of the arithmetic context rather than a property of the value",
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
		ID: "opt/set-plus-o-writes-the-shells-own-vocabulary", Category: "shell options",
		Snippet: `set +o | grep -cE '^set [-+]o (autocd|autopushd|extendedglob)$'`,
		Why:     "whether the `+o` listing is written in the vocabulary of the shell that produced it. Three names one shell in the panel has and no other does: it counts 3 and everybody else counts 0, at 0 either way, which is what a re-inputtable capture depends on. Ours wrote the substrate's shared two dozen under that shell — 23 names against its own 185 — so this counted 0, silently and at status 0, and a caller sourcing the file back got a shell it had never described (#1080). A named slice rather than the table's length, because the length is a fact about the build",
	},
	{
		ID: "opt/set-plus-o-does-not-borrow-another-shells-vocabulary", Category: "shell options",
		Snippet: `set +o | grep -cE '^set [-+]o (braceexpand|onecmd|histexpand)$'`,
		Why:     "the same question from the other side, and the half that a shell answering with somebody else's table passes without it. Three names the bash columns have and write: they count 3, and the other three count 0 — including the shell that *accepts* two of them as input and lists its own spellings instead, which is why a listing has to be graded on what it writes rather than on what it takes",
	},
	{
		ID: "opt/set-o-takes-a-name-only-this-shell-has", Category: "shell options",
		Snippet: `set -o autocd; echo "st=$?"`,
		Why:     "the setting half of the same namespace, and four refusals against one acceptance. `autocd` is one shell's own: it takes it at 0, and the rest split every way a refused `set -o` name can — bash 5.3 says `invalid option name` at 2 and carries on, bash 3.2 says the same at 1 and carries on, the same 5.3 binary under an `argv[0]` of `sh` says it and **stops**, and dash and ksh93 each have wording of their own and stop too. A shell listing 185 names it would then refuse would be writing a capture it could not read back",
	},
	{
		ID: "opt/set-o-and-the-listing-are-one-namespace", Category: "shell options",
		Snippet: `set -o extendedglob 2>/dev/null; set +o | grep -cE '^set -o extendedglob$'`,
		Why:     "written through `set -o` and read back through `set +o`, which is what makes the two rows above facts about one namespace rather than about two tables that happen to differ. One shell counts 1; bash 5.3 and bash 3.2 refuse the name, carry on and count 0; and the three that treat a refused `set -o` as fatal — bash-as-`sh`, dash and ksh93 — never reach the second command, so the case records their exit rather than a count",
	},
	{
		ID: "opt/set-o-a-name-this-shell-has-and-will-not-move", Category: "shell options",
		Snippet: "set -o onecmd\necho \"st=$?\"\necho after\n",
		Why:     "a name a shell *has* and refuses to change, which is a third answer beside taking it and never having heard of it. The bash columns have `onecmd` and set it silently at 0; dash and ksh93 have no such name and stop; zsh has it — as a borrowed spelling for its own `singlecommand` — and refuses it with `can't change option`, fatally, which is a different sentence from the `no such option` it gives a name it does not have. Ours said `not implemented` and carried on, which was neither",
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
		ID: "opt/set-v-writes-to-the-descriptor-the-script-points", Category: "shell options",
		Script:  true,
		Snippet: "exec 2>&1\nset -v\ncat <<END >&2\nbody\nEND\necho after",
		Why:     "the echo goes to descriptor 2 as the *script* has pointed it, not to the stream the shell started with: after `exec 2>&1` every echoed line joins the output and standard error is empty. Unanimous. Written this way rather than with the body on descriptor 2, which is the workaround the same question had to use while a front end held its own stream (#771)",
	},
	{
		ID: "opt/set-v-follows-a-descriptor-that-moves", Category: "shell options",
		Script:  true,
		Snippet: "set -v\nexec 2>/dev/null\necho gone\nexec 2>&1\necho back",
		Why:     "the descriptor is read at the moment of the echo, which decides three things at once: the line holding the `exec` goes to the descriptor it is about to replace, the lines after it disappear into what it was pointed at, and only the line after the restore comes back. Unanimous. The snippet ends without a newline because the harness adds one, and a trailing blank line is not the same question: one shell reads ahead in blocks and echoes that line *before* running the command above it",
	},
	{
		ID: "opt/set-v-echoes-the-line-that-would-not-parse", Category: "shell options",
		Args:        []string{"-v", ArgScript},
		SyntaxError: true,
		Snippet:     "echo one\nfi",
		Why:         "the shell writes back what it read before it complains about it, so the offending line comes out and *then* the syntax error. Unanimous, and it is the line rather than the whole input: the walk stops where the reader did. The wordings and statuses differ — 2, 2, 3 and 1 — and the order does not. The letter is on the invocation rather than in the script, because one shell prefixes a syntax error with the line it had reached and only when the option was turned on from inside",
	},
	{
		ID: "opt/set-v-echoes-a-first-token-that-will-not-lex", Category: "shell options",
		Args:        []string{"-v", ArgScript},
		SyntaxError: true,
		Snippet:     "\"abc\necho two",
		Why:         "the same rule where the parser has nothing at all to hand back — the *first* token of the input will not lex, so there is no logical line to drive an echo from. Both physical lines are still written back before the complaint, in all four; the line each of them blames is 1, 3, 1 and 3",
	},
	{
		ID: "opt/set-v-echoes-input-that-ran-out", Category: "shell options",
		Args:        []string{"-v", ArgScript},
		SyntaxError: true,
		Snippet:     "echo one\nif true; then",
		Why:         "the second face of the same rule, where there is no offending line to name: what was read runs to the end of the text and is written back before the complaint. Unanimous",
	},
	{
		ID: "opt/set-v-echoes-before-a-remark", Category: "shell options",
		Args:       []string{"-v", ArgScript},
		Unfinished: true,
		Snippet:    "echo one\ncat <<END\nbody",
		Why:        "and for input the shell *accepted*: the here-document's lines are written back, then the one shell that remarks on a delimiter that never arrived says so, then the command's own output. The order is the same rule as the two rows above, which is why they are three rows and one fix. The last line ends without a newline, which is the shape that found #962 — the printer wrote the delimiter onto it and the body became `bodyEND`",
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
		ID: "arith/a-character-code-on-a-name", Category: "arithmetic",
		Snippet: `b=zebra; echo "[$((#b))]"`,
		Why:     "the leading `#` in an expression, which is a character code in zsh and an arithmetic syntax error in bash 5.3, bash 3.2, bash as sh, ksh93 and dash alike — every one of them naming the operand it could not read. One shell has an operator and five refuse it, so the split is additive and belongs in the grammar rather than on the semantics vector",
		// Refused by the bash dialect the parser test grades against, as it is
		// by bash itself — at a different moment, since this parser reads an
		// expression while parsing where bash reads it while expanding. That
		// difference is older than this operator and is not about it.
	},
	{
		ID: "arith/a-character-code-is-not-a-length", Category: "arithmetic",
		Snippet: `a=(1 2); echo "[$((#a))]"`,
		Why:     "the reason the operator is worth a case at all: `$((#a))` looks like a length and is not one. It is 49 in zsh — the code of the `1` that begins the array's first element — where the count is `$(( $#a ))`. An implementation that read it as a length would answer 2 here and pass every test written on a single-character value",
	},
	{
		ID: "arith/a-character-code-with-nothing-to-take", Category: "arithmetic",
		Snippet: `b=; echo "[$((#nothing))][$((#b))]"`,
		Why:     "the quiet edges, both zero in the shell that has the operator: a name never set, and a name holding the empty string. Neither reports, which is what makes an implementation that refused either one louder than the shell a script was written for. A third edge — a `#` with no operand at all — is left out of the corpus on purpose: `$(( # ))` makes bash lose the closing parenthesis of the whole word, so the row would be about its scanner rather than about the operator",
	},
	{
		ID: "arith/a-character-code-on-a-character", Category: "arithmetic",
		Snippet: `echo "[$((##a))][$((##A))][$((##\n))][$((##\x41))]"`,
		Why:     "the doubled spelling, which takes the character written out rather than a parameter — and decodes the escapes `$'…'` decodes, so the third is a newline and not the letter n. Four probes in one case because the escape table is where an implementation is most likely to stop early",
	},
	{
		ID: "arith/a-character-code-missing-its-character", Category: "arithmetic",
		Snippet: `echo "[$((##))]"; echo after`,
		Why:     "the operator with nothing after it, worded by the shell that has it as neither of the two failures the rest of arithmetic has — `character missing after ##` — and fatal to the command. The shells without the operator reach a refusal too, by the ordinary route, which is what makes this a row about wording rather than about behavior",
	},
	{
		ID: "arith/a-radix-literal-is-not-a-character-code", Category: "arithmetic",
		Snippet: `echo "[$((16#ff))][$((2#101))]"`,
		Why:     "the one place the two spellings of `#` could collide: `base#digits` is a literal in every shell with arithmetic and its `#` follows digits, where the character code stands where an operand belongs. Unanimous except in dash, which has no based literal — and the row exists so that adding the operator cannot quietly cost the literal",
	},
	{
		ID: "arith/a-dollar-bracket-is-arithmetic", Category: "arithmetic",
		Snippet: `x=5; echo "[$[1+1]][$[x*2]][$[2**10]][${p:-$[3*3]}]"`,
		Why:     "the older spelling of `$((…))`, and the panel splits four to two: bash 5.3, bash 3.2, bash as `sh` and zsh read it as arithmetic, and ksh93 and dash do not read it at all — there the `$` is literal and the brackets are a pattern, so the word stands as its own text. bash has documented it as deprecated for years and both of its builds still take it, which is why they are separate columns here. The additive kind of split, so a grammar flag rather than an axis: nobody means something *else* by it",
	},
	{
		ID: "arith/a-dollar-bracket-nests-and-quotes-like-the-other-spelling", Category: "arithmetic",
		Snippet: `a=(7 8 9); echo "[$[a[1]+1]][$[$[2+2]*2]][${p:-$[ (1+2)*3 ]}]"`,
		Why:     "three shapes that a scan taking the first `]` gets wrong: a subscript, which is arithmetic too and carries a bracket of its own; the spelling nested in itself; and parentheses inside it. The first also inherits the array base, so the two shells that read the construct give different numbers for the same text — which is `ArrayBaseIsZero` showing through and not a second question",
	},
	{
		ID: "arith/a-dollar-bracket-in-a-here-document", Category: "arithmetic",
		Snippet: "cat <<EOF\nv=$[6*7]\nEOF\ncat <<'EOF'\nw=$[6*7]\nEOF\n",
		Why:     "an unquoted here-document body expands it exactly as it expands `$((…))`, and a quoted one leaves it alone — so the construct belongs to every place a substitution is read and not only to a word. The pair is the point: a lexer that added it to the word scanner alone would pass the first line and fail nothing",
	},
	{
		ID: "arith/a-dollar-bracket-is-text-in-single-quotes", Category: "arithmetic",
		Snippet: `echo '$[1+1]'; echo "$[1+1]"`,
		Why:     "the quoting rule the construct shares with every other substitution — single quotes make it text and double quotes do not — pinned so that adding the spelling cannot reach inside a quote nobody expands. Unanimous on the first half, since the shells without the construct have nothing to expand anywhere",
	},
	{
		ID: "arith/a-dollar-bracket-refuses-like-the-other", Category: "arithmetic",
		Snippet: `echo "[$[1+]]"; echo after`,
		Why:     "a bad expression in the older spelling, which is the claim that the two are one construct stated where it can be checked: the two shells that read it word the refusal exactly as they word it for `$((1+))`, down to the error token, and neither invents a diagnostic of its own for the brackets. The two that do not read it reach no expression at all and print the text",
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
		ID: "shift/past-the-end-with-a-count", Category: "builtins",
		Snippet: `set -- a b; shift 5; echo "st=$? n=$# rest=[$*]"`,
		Why:     "a count larger than `$#`, which the prior reading of this — measured against bash alone — had as 'moves nothing and answers 1'. That is bash and zsh; dash and ksh93 *end the script*, which is `ShiftPastEndFatal` arriving with a count rather than without one, and the two say so in different words at different statuses. And the same bash 5.3 binary is silent as `bash` and prints `shift count out of range` as `sh`, so the diagnostic is argv[0]'s and not the build's. `shift/an-operand-that-was-never-given` is the no-count half",
	},
	{
		ID: "shift/a-negative-count", Category: "builtins",
		Snippet: `set -- a b c; shift -1; echo "st=$? n=$#"`,
		Why:     "a count that cannot be one, and it divides the panel on *what kind of thing* the word is before it divides them on the answer: ksh93 reads `-1` as an option and refuses it as one, dash reads it as a number it calls illegal, and bash and zsh read it as a count that is out of range — three complaints, and fatal in the two where a special builtin's failure is. In the four that carry on, `$#` is untouched; the two that end the script end it before anything could be read back, which is the one thing this shape cannot say about them",
	},
	{
		ID: "shift/a-double-dash-before-the-count", Category: "builtins",
		Snippet: `set -- a b c; shift -- 2; echo "st=$? n=$# rest=[$*]"`,
		Why:     "the end-of-options marker in front of the count: five take it and shift two, and dash calls `--` an illegal number — it has no option parsing here for `--` to end. It is the counterpart of `shift/a-leading-dash-that-is-not-a-number`, which asks what a dash word that is *not* the marker does, and together they say which shells read options at all",
	},
	{
		ID: "shift/a-double-dash-alone-still-shifts-one", Category: "builtins",
		Snippet: `set -- a b c; shift --; echo "st=$? n=$# rest=[$*]"`,
		Why:     "the marker with nothing after it: the count falls back to its default of one in the five that have a marker, so `--` is consumed rather than read as the operand. dash keeps calling it an illegal number, which is the same answer it gives the marker anywhere",
	},
	{
		ID: "shift/a-negative-count-after-the-marker", Category: "builtins",
		Snippet: `set -- a b c; shift -- -1; echo "st=$? n=$#"`,
		Why:     "what the marker is *for*, and the row that separates the two dash-word questions: past it ksh93 reads `-1` as a count and says `bad number` where a bare `-1` is an option it does not have. bash and zsh answer the same either way and dash still refuses the `--`",
	},
	{
		ID: "shift/a-count-with-a-plus-sign", Category: "builtins",
		Snippet: `set -- a b c; shift +1; echo "st=$? n=$# rest=[$*]"`,
		Why:     "unanimous, and the other half of the sign: a `+` in front of the digits is read and the count is one, in all six. Worth a row beside the negative count because the same reader decides both, and one that has no sign at all calls this a word that is not a number",
	},
	{
		ID: "shift/a-dash-zero-count", Category: "builtins",
		Snippet: `set -- a b c; shift -0; echo "st=$? n=$#"`,
		Why:     "a negative zero, which is the sharpest line between the two shells that read dash words as options: zsh takes it as a count of nothing and succeeds, ksh93 refuses `-0` as an option it does not have. bash and dash shift nothing too, so this row is ksh93 alone against the rest",
	},
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
		ID: "wait/the-status-of-the-last-background-job", Category: "builtins",
		Snippet: `(exit 3) & wait $!; echo "st=$?"`,
		Why:     "`$!` names the job just started and `wait` on it reports *that job's* status rather than its own success — 3, unanimously. The pair is the idiom every script uses to run something in the background and still find out how it went, and the failure mode is quiet: a `wait` that answered 0 because the wait itself worked would pass every case that only checks it returned",
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
		Snippet: `cd /; pushd /tmp >/dev/null; pushd +9; echo "st=$?"; dirs`,
		Why:     "an index there is no entry for: refused, at 1, with the stack left alone, and the complaint compared rather than discarded (#603) — a different sentence in each of the two shells that have the builtin, both carrying the location the shell puts in front of its own refusals",
	},
	{
		ID: "dirstack/rotate-with-nothing-pushed", Category: "builtins",
		Snippet: `cd /; pushd +1; echo "st=$?"`,
		Why:     "the same refusal against a stack with nothing in it: one shell has a second sentence for it and the other reuses the one above, and both are compared whole",
	},
	{
		ID: "dirstack/popd-an-entry-that-is-not-the-top", Category: "builtins",
		Snippet: `cd /; pushd /tmp >/dev/null; pushd /usr >/dev/null; popd +1; dirs; echo "pwd=$PWD"`,
		Why:     "`popd +N` takes an entry out where it stands and leaves the shell where it is — the current directory only moves when N picks the entry the shell is in, which is what a bare `popd` does",
	},
	{
		ID: "dirstack/popd-with-nothing-pushed", Category: "builtins",
		Snippet: `cd /; popd; echo "st=$?"`,
		Why:     "the underflow, which is 1 in both shells that have `popd` and 127 in the two that do not have the name at all — absence rather than divergence, and the case says which by the status",
	},
	{
		ID: "dirstack/clear-the-stack", Category: "builtins",
		Snippet: `cd /; pushd /tmp >/dev/null; pushd /usr >/dev/null; dirs -c; echo "st=$?"; dirs; popd; echo "p=$?"`,
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
		ID: "dirstack/pushd-into-a-directory-that-is-not-there", Category: "builtins",
		Snippet: `cd /; pushd /no/such-xyz; echo "st=$?"; dirs`,
		Why:     "whose complaint it is. The directory change is what fails, and neither shell blames `cd`: the word the script wrote is the word the refusal carries, and the stack is left alone. The case that showed a prelude function could not say either (#603)",
	},
	{
		ID: "dirstack/dirs-a-letter-it-does-not-have", Category: "builtins",
		Snippet: `dirs -q; echo "st=$?"`,
		Why:     "the shape of the option parser: 2 where a bad letter is an invalid *number* with an unlocated usage line after it, 1 where it is a bad option, 127 where there is no `dirs`. The same split decides whether `dirs -lv` is two letters or one malformed index",
	},
	{
		ID: "dirstack/unset-the-function-name-of-a-builtin", Category: "builtins",
		Snippet: `cd /; unset -f pushd; echo "u=$?"; pushd /tmp; echo "st=$?"`,
		Why:     "whether `unset -f` can take a name the shell itself provides. It cannot, in any of the six: `pushd` is a builtin in the two that have it, so there is no function of that name to remove and the name still pushes afterwards. What splits is only what the *unset* says — silence at 0 in five, and in zsh the same `no such hash table element` it writes for any name it does not hold, at 1. Ours deleted the declaration and lost the directory stack for the rest of the session at a silent 0, and the failure surfaced later as `command not found` from an unrelated line (#1082)",
	},
	{
		ID: "dirstack/unset-a-function-that-shadowed-a-builtin", Category: "builtins",
		Snippet: `cd /; pushd() { echo mine; }; unset -f pushd; echo "u=$?"; pushd /tmp; echo "st=$?"`,
		Why:     "the other half, and the reason the row above is not a refusal. A function of the script's own is removable by the same rule that makes it the script's — and removing it *uncovers* the builtin, so both shells that have one push. Silent at 0 in all six, zsh included, because the name really was a function. The two names have no relationship in a shell where one is a builtin, and every relationship in a shell where the dialect is written as shell, which is what this pins",
	},
	{
		ID: "dirstack/unset-a-shadowing-function-twice", Category: "builtins",
		Snippet: `cd /; pushd() { echo mine; }; unset -f pushd; unset -f pushd; echo "u=$?"; pushd /tmp; echo "st=$?"`,
		Why:     "the sharpest of the three, because it separates three outcomes a status alone cannot. The second `unset -f` is answered as a name that is *not* a function — silence at 0 in five and the hash-table complaint at 1 in zsh, the same answer as the first row — which is only true if the first removal gave the name back to the shell rather than keeping the script's function or deleting the name for good. `pushd /tmp` after it still pushes",
	},
	{
		ID: "dirstack/unset-the-whole-directory-stack-by-name", Category: "builtins",
		Snippet: `cd /; unset -f dirs popd pushd; echo "u=$?"; pushd /tmp; dirs; echo "st=$?"`,
		Why:     "the shape a real rc file writes: `unset -f` over a list of names, defensively, before defining any. A widely used zsh plugin manager runs exactly this line. All three names survive it in the two shells that have them and the stack works, and the operands are answered one at a time — zsh writes three complaints and hands 1 on, the rest are silent at 0",
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
		ID: "compgen/a-function-listing-is-the-persons-own", Category: "builtins",
		Snippet: `f() { :; }; compgen -A function | grep -cE '^dirs$|^popd$|^pushd$'; compgen -A function | grep -c "^f$"`,
		Why:     "who owns a name a completion generator is offered, graded as two counts of a slice the case sets itself rather than as a listing — a listing grades everything a build happens to have defined. Both counts are unanimous where the command exists at all: 0 for the shell's own directory-stack commands, which are builtins in bash and therefore not functions, and 1 for the function the snippet wrote. Ours counted 3 on the first line, because a dialect written as shell has them as prelude functions and this generator read every function callable (#1081). The other three shells have no `compgen`, so both counts are grep's 0 and the pipeline's 1. `grep -cE` and not a BRE alternation: `^a$` + BRE alternation anchors only its last branch on this platform's grep, so an anchored BRE here counts 1 for three matching lines and could not have graded the first line at all",
	},
	{
		ID: "compgen/a-prefix-only-the-shells-own-names-have", Category: "builtins",
		Snippet: `compgen -A function pu; echo "st=$?"`,
		Why:     "the silent half of the row above, and the one a status cannot hide. `pu` matches nothing in bash — no function begins with it — so the answer is nothing at **1**, and ours wrote `pushd` at **0**: a completer asking whether there is anything to offer was told yes and handed a name the person never defined. Nothing on stderr in any column, which is what makes it the silent kind",
	},
	{
		ID: "compgen/a-prefix-the-person-defined-over-the-shells-name", Category: "builtins",
		Snippet: `pushd() { :; }; compgen -A function pu; echo "st=$?"`,
		Why:     "the same prefix once the person has written their own `pushd`, which is the control the row above is read against: bash offers it at 0, because there the function shadows the builtin and really is a function. So the answer turns on whose declaration is standing rather than on the name, which is the rule #603 and #1035 already set — and a fix that suppressed the name outright would pass the row above and fail this one",
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
		ID: "commands/coproc-speaks-by-a-letter-where-it-has-no-array", Category: "commands",
		Snippet: `coproc cat; print -p hi; read -p l; echo "l=$l COPROC=${#COPROC[@]}"`,
		Why:     "the other coprocess model, and the reason the word alone is not the whole feature: zsh starts one with the same keyword, gives it no name and publishes no array, and a script reaches its two ends with `print -p` and `read -p` — `l=hi` with COPROC still empty. bash has the array and neither letter, ksh93 has both letters and starts no coprocess for them, and bash 3.2 and dash have none of it",
	},
	{
		ID: "commands/coproc-with-no-name-takes-a-compound", Category: "commands",
		Snippet: `coproc { cat; }; print -p hi; read -p l; echo "l=$l"`,
		Why:     "a compound command needs no name in front of it, which is what separates the two grammars rather than the keyword: the shell with no name for a coprocess still takes `{ … }` here, where the one that reads a name would have read `{` as the name's absence. bash 3.2, dash and ksh93 date the construct by refusing the `}`",
	},
	{
		ID: "commands/the-coprocess-letters-with-nothing-started", Category: "commands",
		Snippet: `print -p x; echo "p=$?"; read -p y; echo "r=$?"`,
		Why:     "the same two letters before any `coproc`, which is the refusal each shell keeps for the case: two sentences and 1 in the shell that has both letters and a coprocess, two others and 1 in the shell that has the letters and no way here to start one, and in the two without `print` a command that was not found at 127 with `-p` reading as a prompt",
	},
	{
		ID: "commands/an-and-or-ending-with-its-operator", Category: "commands", SyntaxError: true,
		Snippet: `{ : ||
}
echo after`,
		Why: "an and-or list whose right-hand side is simply not there. zsh runs the group and prints `after`; the other five each name the closer they met and run nothing. `~/.zi/bin/lib/zsh/install.zsh` line 2048 ends with `|| \\` and the line after it closes the block, so this is the shape that decides whether that file can be read at all (#1174). The brace group is the minimal form and needs no `if`: a first attempt with `if true { : || }` measured the *opposite* answer, because with no `[[ ]]` to close the condition the `{` is read as an argument to `true`",
	},
	{
		ID: "commands/an-and-or-ending-with-its-operator-reaches-every-closer", Category: "commands", SyntaxError: true,
		Snippet: `if true; then echo one ||
fi
echo after`,
		Why: "the same absence before a keyword rather than a brace, which is what says it is a property of the and-or list and not of brace groups: zsh prints `one` and `after` and the rest name the `fi`. Measured the same way for `( : || )`, `while …; do : || done`, `until`, `for`, `case x in x) : || ;; esac`, a function body's `}` and a command substitution's `)`",
	},
	{
		ID: "commands/an-absent-and-or-operand-is-dropped-not-stood-in-for", Category: "commands", SyntaxError: true,
		Snippet: `{ false ||
}
echo "st=$?"`,
		Why: "what the absence *means*, which is the half a grammar flag alone would leave open. The status is the left-hand side's — `st=1` — so the missing operand is not an implicit `true`, which would answer 0 here. The next case is the other side of the pair, and the two together are why nothing in the interpreter needs a value for this: the operator is dropped and the left-hand side is the whole list",
	},
	{
		ID: "commands/an-absent-and-or-operand-keeps-a-success-too", Category: "commands", SyntaxError: true,
		Snippet: `{ true &&
}
echo "st=$?"`,
		Why: "the other side: `st=0`, so the missing operand is not an implicit `false` either — that would answer non-zero here. One stand-in gets the case above wrong and the other gets this one wrong, which is what rules both of them out and leaves dropping the operator as the only reading that fits",
	},
	{
		ID: "commands/an-open-ended-pipeline-is-refused-everywhere", Category: "commands", SyntaxError: true,
		Snippet: `{ : |
}
echo after`,
		Why: "the discriminating half of the same measurement: the leniency belongs to the and-or list alone and **no** shell in the panel extends it to a bar. zsh refuses this while taking `{ : || ⏎ }`, so a flag written for control operators in general would accept a line zsh rejects. Its own refusal names the `}`",
	},
	{
		ID: "commands/an-and-or-with-a-terminator-after-it", Category: "commands", SyntaxError: true,
		Snippet: `: || & echo two`,
		Why:     "a `&` where the right-hand side belongs, which the shell that takes an absent operand still refuses, naming the `&` it found — so a terminator does not close the list for this purpose. ksh93 is the odd column: it parses the line and runs neither side of it, and `: || & echo two > f` then refuses the `>`, so whatever it read `echo two` as is not a command that could take a redirection. Recorded rather than implemented; it is not the same rule as the `;` two cases down",
	},
	{
		ID: "commands/an-and-or-with-a-reserved-word-after-it", Category: "commands", SyntaxError: true,
		Snippet: `echo a && fi`,
		Why:     "a command may begin after `&&`, so a reserved word standing there is reserved — the shell that names a word's *class* instead of quoting it quotes this one rather than calling it a word. The same distinction #1115 found for a bar, and the row that catches the operand-form helper being used here. All six refuse it, including zsh: `fi` with no `if` open closes nothing, which is what separates this from the case above where the `fi` is the one the list was inside",
	},
	{
		ID: "commands/an-and-or-with-a-separator-after-it", Category: "commands", SyntaxError: true,
		Snippet: `false || ; echo two`,
		Why:     "a `;` between a control operator and its right-hand side. zsh and ksh93 print `two` and the other four refuse the line — and the `;` is *skipped* rather than standing in for an absent operand, which is what `false` makes visible: `echo two` is the `||`'s right-hand side and runs because the left one failed. Writing `true` there prints nothing in both. So this is a separator rule and not the absence rule the cases above are about (#1142)",
	},
	{
		ID: "commands/an-absent-and-or-operand-after-a-separator", Category: "commands", SyntaxError: true,
		Snippet: `false || ;
echo "st=$?"`,
		Why: "the same `;` with nothing after it at all, and the row where the two lenient columns disagree about *meaning*: ksh93 answers `st=0` and zsh answers `st=1`. So an absent operand is an implicit success in one and a dropped operator in the other, which is a semantics question rather than a grammar one and cannot be a single additive flag. ksh93 also takes this shape only with the `;` present — `false ||` alone and `{ false || ⏎ }` are both syntax errors there — where zsh takes it either way",
	},
	{
		ID: "commands/a-pipeline-with-a-separator-after-it", Category: "commands", SyntaxError: true,
		Snippet: `echo one | ; cat -n`,
		Why:     "the same separator after a bar, and this is where the two lenient columns part: zsh prints a numbered `one`, so `cat -n` really is the pipeline's right-hand side with the `;` skipped, and ksh93 refuses the `;` outright. It is the discriminating probe of the pair — a rule stated for control operators generally would have to accept it in ksh93 too. zsh skips any number of them, `echo one | ; ; cat -n` included, and still refuses `echo one | ;` with nothing after",
	},
	{
		ID: "commands/a-list-beginning-with-a-separator", Category: "commands", SyntaxError: true,
		Snippet: `; echo two`,
		Why:     "the same separator rule at the *start* of a list, where there is no operator in front of it at all. ksh93 and zsh print `two` and the other four refuse the line, which is what says this is one rule about a `;` written where a command belongs rather than a rule about `&&` — the position varies and the answer does not (#1142)",
	},
	{
		ID: "commands/a-separator-at-the-head-of-a-compound-body", Category: "commands", SyntaxError: true,
		Snippet: `{ ; echo two; }`,
		Why:     "the same rule at the *first* thing in a body rather than between two statements, which is its own position in the grammar: ksh93 and zsh print `two` and the other four refuse it. Measured the same way for `( ; … )`, `if …; then ; … fi`, `while …; do ; … done` and a `case` arm, all five alike. It is here because a mutant that removed the skip at the head of a list survived every other case — every one of them wrote the `;` after something",
	},
	{
		ID: "commands/two-separators-with-nothing-between-them", Category: "commands", SyntaxError: true,
		Snippet: `false ; ; echo "st=$?"`,
		Why:     "the same rule between two statements, and the row that says **nothing runs** for the skipped separator: `st=1` in both lenient columns, so it does not even set a status. A reading that put an empty successful command here would answer `st=0` and be wrong about every `$?` written after one",
	},
	{
		ID: "commands/a-separator-after-a-background-terminator", Category: "commands", SyntaxError: true,
		Snippet: `true & ; echo two`,
		Why:     "the same rule after a `&` rather than a `;`, which is worth its own row because `&` terminates the statement and could have been read as the thing being tolerated. It is not: what is stepped over is the `;` after it, exactly as in the row above",
	},
	{
		ID: "commands/a-newline-after-a-skipped-separator", Category: "commands", SyntaxError: true,
		Snippet: `true || ;
echo two`,
		Why: "the third place the two lenient columns part, and the sharpest: ksh93 prints `two` and zsh prints nothing over the same two lines. zsh steps over the newline as well, so `echo two` is the `||`'s right-hand side and the successful `true` short-circuits past it; ksh93 stops at the newline, the and-or ends with nothing on its right, and the `echo` is the next statement. `true` rather than `false` because a failing left-hand side runs the right-hand side and prints either way, which is exactly the reading this has to tell apart",
	},
	{
		ID: "commands/a-second-separator-where-a-command-belongs", Category: "commands", SyntaxError: true,
		Snippet: `false || ; ; echo two`,
		Why:     "how many may be stepped over, which is one of the two places the lenient columns part: zsh takes as many as are written and prints `two`, ksh93 takes one and calls the second `` `;' unexpected ``. So a single flag could not be given a value for both, and the count is part of the dialect's answer",
	},
	{
		ID: "commands/an-and-or-operator-at-the-end-of-input", Category: "commands",
		SyntaxError: true, Unfinished: true,
		Snippet: `echo one &&`,
		Why:     "the operator with nothing whatever after it, which is a question about the *route* and not only the grammar: zsh takes it here and prints `one`, and the same text typed at a terminal draws a continuation prompt in zsh, bash and ksh93 alike — measured through a pty. So the end of a `-c` string ends the list where the end of a line does not, and the four refusing columns each say the input ran out rather than naming a token. ksh93 refuses it while accepting the `;` form two cases up, which is what says its leniency is the separator's and not the list's",
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
		ID: "pipe/both-streams-is-a-pipe-in-two-shells-and-a-coprocess-in-one", Category: "redirection",
		Script: true,
		Snippet: `f() { echo O; echo E >&2; }
f |& while read l; do echo "<$l>"; done
wait`,
		Why: "the two characters with two readings. bash 5.3 and zsh pipe both streams, so the reader brackets `O` and `E` alike; bash 3.2 has no `|&` at all and dash never had one, and both lex a bar and an ampersand and blame the ampersand; ksh93 reads a *coprocess* — the left side is backgrounded onto a pipe the shell would read with `read -p`, so the reader gets nothing and the coprocess's standard error reaches the terminal on its own. Three answers, one spelling, and the reason the ksh form cannot be this flag with another value. The reader brackets what reaches it so the row does not depend on which stream a column joins, and the `wait` is what makes the ksh column deterministic: without it the coprocess is racing the shell's exit and drops its line about once in ten",
	},
	{
		ID: "pipe/both-streams-redirection-comes-last", Category: "redirection",
		Script: true,
		Snippet: `e() { echo O; echo E >&2; }
e 2>/dev/null |& while read l; do echo "<$l>"; done
wait`,
		Why: "where in the list the `2>&1` goes, which is the whole of the operator and the only thing an implementation can get silently wrong. Written *after* the command's own redirections it overrides the `2>/dev/null` and `<E>` arrives; written before, the `2>/dev/null` would win and only `<O>` would. Both shells that have the operator bracket both lines, so the answer is unanimous where it exists — and the same pair of shells answer the mirror shape, `e >/dev/null |&`, with nothing at all, which is the other direction of the same rule",
	},
	{
		ID: "pipe/both-streams-takes-no-blank-between-its-two-bytes", Category: "redirection",
		Script: true, SyntaxError: true,
		Snippet: `echo one | & echo two`,
		Why:     "the adjacency, which is what keeps the operator from taking a construct away from the shells that spell a background command with a bar before it: five of the six refuse this where four of them accept `|&` written closed up, and they refuse it in four wordings. ksh93 is the exception in both directions, because its `|&` is a coprocess rather than a pipe",
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
		ID: "setopt/a-name-with-no-behavior-behind-it-is-still-recorded", Category: "builtins",
		Snippet: `setopt auto_cd share_history no_list_types; echo "st=$?"; setopt`,
		Why:     "three of the sixteen names a real rc file writes at startup, none of them a feature this shell has: zsh takes them at 0 and then reports them back canonically spelled, which is what recognizing a name buys when implementing it is a separate job",
	},
	{
		ID: "setopt/a-compat-spelling-lists-under-the-canonical-name", Category: "builtins",
		Snippet: `setopt dotglob; setopt`,
		Why:     "zsh carries twelve sh and ksh spellings as second names for options it already has, and they are never what a listing prints: `dotglob` sets `globdots` and `globdots` is the line that comes back",
	},
	{
		ID: "setopt/the-no-prefix-reaches-a-compat-spelling", Category: "builtins",
		Snippet: `setopt nolog; setopt`,
		Why:     "the prefix and the alias table compose, and the alias is the inverted kind: `nolog` is zsh's `histnofunctions` turned on, which is the name the listing then prints",
	},
	{
		ID: "setopt/nullglob-wins-over-nomatch", Category: "builtins",
		Snippet: `setopt nullglob; echo "[" zz* "]"; echo "st=$?"`,
		Why:     "the only ordering the two settings can have: deleting a word that matched nothing leaves nothing for `no matches found` to complain about, so zsh prints the brackets at 0 with nomatch still on",
	},
	{
		ID: "setopt/caseglob-is-the-globs-alone", Category: "builtins",
		Snippet: `mkdir d; : > d/B.txt; unsetopt caseglob; echo d/b*; case AB in ab) echo yes;; *) echo no;; esac`,
		Why:     "zsh's caseglob governs pathname expansion and nothing else — the glob folds case and the case statement still does not — which is what says it is not the same switch as bash's nocasematch",
	},
	{
		ID: "setopt/globdots-brings-back-the-hidden-names", Category: "builtins",
		Snippet: `mkdir d; : > d/.h; : > d/a; setopt globdots; echo d/*`,
		Why:     "the leading period stops being special, and only that: `.h` joins the expansion where `.` and `..` still do not",
	},
	{
		ID: "setopt/an-interactive-only-option-will-not-move", Category: "builtins",
		Snippet: `setopt zle; echo "st=$?"; unsetopt zle; echo "st=$?"`,
		Why:     "five of zsh's 185 options refuse to be turned on in a shell that is not interactive and every other one is granted, measured name by name; turning one of the five off is asking for where it already is, which is granted",
	},
	{
		ID: "setopt/a-recorded-name-answers-the-condition-too", Category: "builtins",
		Snippet: `[[ -o auto_cd ]]; echo "a=$?"; setopt auto_cd; [[ -o auto_cd ]]; echo "b=$?"; [[ -o no_auto_cd ]]; echo "c=$?"`,
		Why:     "`[[ -o name ]]` reads the same namespace `setopt` writes, so a name the table only records still answers the condition — and the `no` prefix inverts the answer rather than being an unknown name. bash has the condition and not the namespace, which is what makes the pair worth one row",
	},
	{
		ID: "setopt/nounset-is-the-same-switch-as-set-u", Category: "builtins",
		Snippet: `setopt no_unset; echo "${zz}"; echo reached`,
		Why:     "zsh spells `set -u` as `unsetopt unset`, and it is one switch rather than two: an unset parameter ends the run exactly as `set -u` makes it, which is the pairing that says the dialect's own builtin has to reach the substrate's table",
	},
	{
		ID: "setopt/hist-ignore-space-moves-and-is-read-back", Category: "builtins",
		Snippet: `setopt hist_ignore_space; echo "st=$?"; [[ -o histignorespace ]] && echo on; unsetopt hist_ignore_space; [[ -o hist_ignore_space ]] || echo off`,
		Why:     "the option an interactive session reads before it records a line, asked of the shell a script can see: the state moves in both directions and reads back under either spelling. A script has no history for it to govern, which is exactly why the state has to be pinned here rather than inferred from a session",
	},
	{
		ID: "setopt/hist-ignore-dups-is-the-set-h-switch", Category: "builtins",
		Snippet: `set -h; [[ -o histignoredups ]] && echo on; setopt no_hist_ignore_dups; [[ -o histignoredups ]] || echo off`,
		Why:     "zsh's `set -h` is histignoredups and not command hashing, which is what bash and ksh93 spell with the same letter — one switch reached by two words, and the row that says the dialect builtin and the substrate's option table are not two states that drift",
	},
	{
		ID: "setopt/an-option-and-a-variable-are-two-namespaces", Category: "builtins",
		Snippet: `setopt hist_ignore_space HISTORY_IGNORE; echo "st=$?"; HISTORY_IGNORE=x; echo "v=$HISTORY_IGNORE"`,
		Why:     "zsh keeps two of its history knobs as options and one as a variable, and they are not spellings of one namespace: `setopt HISTORY_IGNORE` is `no such option` at 1 even though the variable by that name is real and settable in the same breath",
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
		ID: "emulate/dash-o-sets-an-option", Category: "builtins",
		Snippet: `emulate zsh -o extendedglob; echo "st=$?"; [[ -o extendedglob ]]; echo "on=$?"; emulate zsh +o extendedglob; [[ -o extendedglob ]]; echo "off=$?"`,
		Why:     "`{+|-}o name` names an option for the emulation to apply after it has placed its own defaults, which is the form a prompt theme opens every one of its functions with. The name is `setopt`'s and takes `setopt`'s spellings, underscores and all; `+o` is the same request the other way",
	},
	{
		ID: "emulate/dash-l-lasts-as-long-as-the-function", Category: "builtins",
		Snippet: `f() { emulate -L zsh -o extendedglob; [[ -o extendedglob ]]; echo "in=$?"; }; f; [[ -o extendedglob ]]; echo "out=$?"`,
		Why:     "`-L` is LOCAL_OPTIONS: the emulation and everything moved after it last as long as the call. Written as a function on purpose — outside one the letter changes nothing, measured, so a case at the top level would pass with the letter ignored",
	},
	{
		ID: "emulate/dash-l-covers-a-later-setopt", Category: "builtins",
		Snippet: `f() { emulate -L zsh; setopt err_exit; [[ -o err_exit ]]; echo "in=$?"; }; f; [[ -o err_exit ]]; echo "out=$?"`,
		Why:     "and it is the *call* that is restored rather than the emulation's own changes: an option set later in the same function goes back too, which is the whole of what the option is for and the half a snapshot taken after the emulation would miss",
	},
	{
		ID: "setopt/no-aliases-stops-the-expansion", Category: "builtins",
		Script:  true,
		Snippet: "alias hi='echo expanded'\nhi\nsetopt no_aliases\nhi\nsetopt aliases\nhi\n",
		Why:     "`no_aliases` is an option a script throws to protect its own code from a user's aliases, and it reaches the *parser*: the second `hi` is a command that does not exist. Run from a file because that is the route this shell expands on at all — under `-c` it never does, while `[[ -o aliases ]]` still reads on there, which is what makes the option and the route two questions",
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
		ID: "whence/which-is-whence-with-c", Category: "builtins",
		Snippet: `alias ll="ls -l"; which echo; which ll; which nosuchcmd431; echo "st=$?"`,
		Why:     "zsh has `which` as a builtin and it is `whence -c`, so it answers for an alias and for a builtin and says `not found` for neither. The other five have no such builtin and reach /usr/bin/which, which knows only PATH — the same word, two entirely different questions, and the reason a dialect that leaves it out is not this shell",
	},
	{
		ID: "whence/which-refuses-the-letters-its-preset-decided", Category: "builtins",
		Snippet: `which -c echo; echo "st=$?"; which -v echo; echo "st=$?"`,
		Why:     "and it is not `whence` with a flag set: the letters `-c` already decided are no longer on offer, so `which -c` and `which -v` are bad options where `which -a`, `-p` and `-w` are taken. A shell that implemented it by handing its arguments to whence would pass the row above and fail this one",
	},
	{
		ID: "whence/where-takes-the-letters-its-preset-left", Category: "builtins",
		Snippet: `where -p echo; where -w echo; echo "st=$?"`,
		Why:     "the same rule from the other end: `where` is `whence -ca`, so `-c` and `-a` are gone and `-p` and `-w` remain — two letters it does answer, beside the `-v` that `whence/where-takes-no-options` shows it refuses. Refusing every dash word passes that row and fails this one",
	},
	{
		ID: "whence/a-on-a-name-that-is-only-on-path", Category: "builtins",
		Snippet: `whence -a ls; echo "st=$?"`,
		Why:     "the PATH hit standing alone keeps the sentence `-v` gives it — ksh93 says `tracked alias for` here and plain `N is <path>` when a builtin or function line came first, which is the difference `whence -a echo` cannot show. zsh, whose `-a` has no sentences at all, just prints the path",
	},
	{
		ID: "whence/nothing-to-ask-about", Category: "builtins",
		Snippet: `whence; echo "st=$?"`,
		Why:     "the two shells with the builtin part company over an empty operand list: zsh says nothing and reports 1, ksh93 prints its usage line and reports 2. The quiet one is the trap — a script testing the status sees a plain miss",
	},
	// --- zsh's zstyle and bindkey (#840). Both are this shell's alone, and
	// the other five shells' command-not-found answers are the evidence of
	// that. What is recorded here is the part a real rc file depends on: that
	// a style set for a completion system survives being set, that the order
	// styles come back in is the order a lookup reads them in, and that a key
	// bound to a widget the shell has not got is accepted rather than refused.
	// `autoload` — a name defined from `$fpath` the first time it is called.
	// ksh93 has the word too, as `typeset -fu` under another name, which is
	// why its column is a `typeset` usage line rather than a not-found.
	{
		ID: "autoload/the-name-is-a-function-at-once", Category: "builtins",
		Snippet: `autoload -Uz zzfn; echo "decl=$?"; whence -w zzfn; echo "wh=$?"`,
		Why:     "the shape of the builtin, and the thing a resolve-now implementation would get wrong: the declaration reads nothing, reports 0 and the name is *already* a function — `zzfn: function`. bash has no `autoload` at all; ksh93 has one, spelled as `typeset -fu`, and refuses zsh's `-U` and `-z` with its own usage line",
	},
	{
		ID: "autoload/the-file-becomes-the-body", Category: "builtins",
		Snippet: `mkdir -p fp; printf "%s\n" 'echo "cf ran [$*]"' > fp/cfn; fpath=(fp); autoload -Uz cfn; cfn a b; echo "st=$?"`,
		Why:     "the call is what reads the file, and the file's contents are the function's *body* — so its `$*` is the call's arguments, `a b`. The case writes its own `fpath` entry, because a row that reached for a real one would be a record of the machine",
	},
	{
		ID: "autoload/a-missing-file-fails-at-the-call", Category: "builtins",
		Snippet: `fpath=(); autoload -Uz zznone; echo "decl=$?"; zznone; echo "call=$?"`,
		Why:     "and where it fails: the declaration is 0 with an empty `fpath` and the *call* is `zznone: function definition file not found` at 1. Two statuses in one row, because a shell that resolved at the declaration would report the failure in the wrong place and a script's `autoload || return` would fire when nothing was wrong yet",
	},
	{
		ID: "autoload/the-two-signs-of-x", Category: "builtins",
		Snippet: `autoload +X; echo "plus=$?"; autoload -X; echo "minus=$?"`,
		Why:     "`+X` and `-X` are two commands rather than one letter with a sign: `+X` with nothing to resolve is silence and 0, and `-X` with nothing is `bad autoload` — it means \"the function I am running inside\", so at the top level there is nothing for it to be about. zsh ends the script there and this shell reports and runs on, which the row records",
	},
	{
		ID: "autoload/a-letter-the-builtin-does-not-have", Category: "builtins",
		Snippet: `autoload -Q zz; echo "st=$?"`,
		Why:     "`bad option: -Q` and 1 — the same wording `zmodload` and `bindkey` use and not `zstyle`'s `invalid option`. `-Q` is one of the forty letters zsh's autoload does not have, against the twelve it does",
	},
	// `zmodload` — the module loader, which is the first line of a real
	// plugin manager that actually stops the file. This shell cannot load a
	// compiled module and never will, so what is recorded is the shape of
	// the answers *around* the loading: what a fresh shell has loaded, what
	// it says about a module it has not, and every option letter that
	// answers a question rather than loading something.
	//
	// The reason clause of a load failure is deliberately not a case: zsh's
	// is a dlopen error naming the module directory of the running build,
	// so a golden record holding one would be a record of this machine.
	// `-s` is the row that grades the failure without the wording.
	{
		ID: "zmodload/a-fresh-shell-has-one-module", Category: "builtins",
		Snippet: `zmodload; echo "st=$?"`,
		Why:     "what is loaded before a script asks for anything: `zsh/main` alone, one name per line and 0. Not `zsh/complete` and `zsh/zle` as well — those are linked into the binary and `zmodload -e` says 1 for both, which is why this row is the bare listing rather than a claim about what is compiled in. Nobody else has the builtin at all",
	},
	{
		ID: "zmodload/loading-the-module-a-plugin-manager-asks-for-first", Category: "builtins",
		Snippet: `zmodload zsh/zutil; echo "st=$?"; zmodload -e zsh/zutil; echo "e=$?"`,
		Why:     "the line at the top of a real plugin manager, and the one it stops at when the answer is wrong: `zmodload zsh/zutil` is silence and 0 in zsh, and the module is then loaded, which is what `-e` asking afterwards is for. Two questions rather than one because a shell that reported 0 and recorded nothing would pass the first",
	},
	{
		ID: "zmodload/the-listing-as-the-commands-that-would-make-it", Category: "builtins",
		Snippet: `zmodload -L; echo "st=$?"`,
		Why:     "`-L` writes the same set as the commands that would load it — `zmodload zsh/main` — which is the shape a script re-plays and therefore the shape worth pinning",
	},
	{
		ID: "zmodload/asking-whether-a-module-is-loaded", Category: "builtins",
		Snippet: `zmodload -e zsh/main; echo "main=$?"; zmodload -e zsh/zutil; echo "zutil=$?"`,
		Why:     "`-e` asks instead of loading, and says nothing either way: 0 for the module a fresh shell has and 1 for one it has not, with no output on either. This is the row that makes `-e` a question rather than a load, and the two answers side by side are what a shell that always answered 0 would fail",
	},
	{
		ID: "zmodload/unloading-what-was-never-loaded", Category: "builtins",
		Snippet: `zmodload -u zsh/nosuchmodule; echo "st=$?"`,
		Why:     "`no such module` here means *not loaded*, not *no such name*: `-u` on a module that was never loaded is `no such module zsh/nosuchmodule` and 1. The row that proves it is the next one, which says the same of a module zsh certainly ships",
	},
	{
		ID: "zmodload/unloading-a-real-module-that-is-not-loaded", Category: "builtins",
		Snippet: `zmodload -u zsh/mathfunc; echo "st=$?"`,
		Why:     "the same answer for a module that exists — `no such module zsh/mathfunc` and 1 — which is what settles what the sentence means. Taken together with the row above, `-u` says nothing at all about whether a name is a module, only whether it is loaded here and now",
	},
	{
		ID: "zmodload/unloading-the-module-a-fresh-shell-has", Category: "builtins",
		Snippet: `zmodload -u zsh/main; echo "st=$?"; zmodload; echo "listing=$?"`,
		Why:     "and the other side of it: `zsh/main` *is* loaded, so `-u` takes it and the listing afterwards is empty with status 0. The emptied listing has to survive being written, which is what tells a store nobody has touched apart from one somebody emptied",
	},
	{
		ID: "zmodload/a-module-that-will-not-load-is-status-one", Category: "builtins",
		Snippet: `zmodload -s zsh/nosuchmodule; echo "st=$?"`,
		Why:     "the whole of what a script can act on, without the wording: `-s` silences the complaint and the status is still 1. `zmodload zsh/zutil || return 1` is the line this row is about — a shell answering 0 here sends a plugin manager on to call a builtin the module was supposed to bring, several hundred lines further on",
	},
	{
		ID: "zmodload/the-features-of-a-module-with-none", Category: "builtins",
		Snippet: `zmodload -lF zsh/main; echo "st=$?"`,
		Why:     "`-lF` lists a module's features, and the one module a fresh shell has is the one with none: `module 'zsh/main' does not support features` and 1 — its own sentence, not the `is not yet loaded` a module that has features but is absent gets. The builtin's name is in the location here, which is where this shell puts it",
	},
	{
		ID: "zmodload/the-feature-letter-needs-the-listing-letter", Category: "builtins",
		Snippet: `zmodload -l zsh/main; echo "st=$?"`,
		Why:     "`-l` alone is refused rather than read as a listing — `-l is only allowed with -F` and 1. Worth a row because `-l` and `-L` are one letter apart and do unrelated things, so a shell that folded them would look right until a script wrote the lower-case one",
	},
	{
		ID: "zmodload/the-feature-letter-needs-a-module", Category: "builtins",
		Snippet: `zmodload -F; echo "st=$?"`,
		Why:     "and `-F` with nothing to act on is `-F requires a module name` and 1, where the bare builtin with no letters at all is a listing and 0 — so the operand is required by the letter rather than by the builtin",
	},
	// `zsh/datetime` — the clock, and the one module in the table with every
	// feature it names present rather than refused (#1154).
	{
		ID: "zmodload/loading-the-clock-module", Category: "builtins",
		Snippet: `zmodload zsh/datetime; echo "st=$?"; zmodload -e zsh/datetime; echo "e=$?"; zmodload`,
		Why:     "the line `zi.zsh` guards its whole scheduler on — `if { zmodload zsh/datetime } { … }` — and the listing after it, which is where a second loaded module first became observable: until this one there was exactly one loadable module, so nothing could tell the sorted listing from an unsorted one",
	},
	{
		ID: "datetime/the-seconds-are-a-clock-read", Category: "builtins",
		Snippet: `zmodload zsh/datetime; a=$EPOCHSECONDS; echo "$(( a > 1700000000 )) $(( EPOCHSECONDS >= a ))"`,
		Why:     "a value, not a shape: the seconds are past a date already gone and never go backwards between two reads. Asked this way because the number itself is different every time the corpus runs, and a case that pinned one would have to be rewritten daily — where a shell answering `0` for an unset parameter fails both halves",
	},
	{
		ID: "datetime/the-real-time-and-how-many-places-it-carries", Category: "builtins",
		Snippet: `zmodload zsh/datetime 2>/dev/null; d=${EPOCHREALTIME#*.}; echo "places=${#d} set=${EPOCHREALTIME:+yes}"`,
		Why:     "how many decimal places the fraction carries, which is a dialect answer and not a shape: zsh's is `typeset -F`'s ten and bash's own `$EPOCHREALTIME` — it has one, from 5.0 — is six. bash 3.2, ksh93 and dash have no such parameter and the count is of an empty string, which the `set=` half tells apart from a real zero. Counted rather than printed because the digits are a clock and a recorded value would be stale the second after",
	},
	{
		ID: "datetime/the-pair-is-seconds-and-nanoseconds", Category: "builtins",
		Snippet: `zmodload zsh/datetime; echo "n=${#epochtime[@]} $(( epochtime[1] > 1700000000 )) $(( epochtime[2] >= 0 ))"`,
		Why:     "the third parameter is the same clock with the nanoseconds beside the seconds instead of inside them — two elements, and the first of them is the same number `$EPOCHSECONDS` is",
	},
	{
		ID: "datetime/strftime-writes-an-epoch", Category: "builtins",
		Snippet: `zmodload zsh/datetime; strftime "%Y-%m-%d %H:%M:%S" 1788698096; echo "st=$?"`,
		Env:     []string{"TZ=UTC"},
		Why:     "a fixed epoch through the format language `printf '%(fmt)T'` also writes, so the two spellings of one job cannot drift. The zone is pinned because the answer is a wall clock and the record would otherwise be about the machine that made it",
	},
	{
		ID: "datetime/strftime-assigns-and-drops-the-newline", Category: "builtins",
		Snippet: `zmodload zsh/datetime; strftime -n "%Y" 1788698096; echo "|"; strftime -s v "%H:%M" 1788698096; echo "v=$v st=$?"`,
		Env:     []string{"TZ=UTC"},
		Why:     "the two letters that change where the answer goes rather than what it is — `-n` writes no newline and `-s` writes no output at all. A shell that treated `-s` as a formatting flag would print the time *and* set the variable, which the `|` and the `v=` between them catch",
	},
	{
		ID: "datetime/strftime-reads-a-header-back", Category: "builtins",
		Snippet: `zmodload zsh/datetime; strftime -rs v "%d %b %Y %H:%M:%S GMT" "06 Sep 2026 12:34:56 GMT"; echo "v=$v st=$?"`,
		Env:     []string{"TZ=UTC"},
		Why:     "the direction and the exact format `lib/zsh/install.zsh` reads an HTTP `Last-Modified` header with, `-r` and `-s` in one word as it writes them. The month is a name rather than a number, which is the part a numeric-only reading would refuse",
	},
	{
		ID: "datetime/strftime-refuses-by-name", Category: "builtins",
		Snippet: `zmodload zsh/datetime; strftime; echo "st=$?"; strftime "%Y" 1 2 3; echo "st=$?"; strftime "%Y" abc; echo "st=$?"; echo after`,
		Why:     "three usage refusals in a row, each at 1 and none of them fatal — the `after` is what says so. Two of the three are counted before any argument is looked at, so a shell checking the format first would word them differently",
	},
	{
		ID: "zmodload/a-letter-the-builtin-does-not-have", Category: "builtins",
		Snippet: `zmodload -q zsh/main; echo "st=$?"`,
		Why:     "`bad option: -q` and 1 — this builtin's wording, which is `bindkey`'s and not `zstyle`'s `invalid option`, measured in all three. `-q` is one of twenty-two letters zsh's zmodload does not have, against the seventeen it does",
	},
	{
		ID: "zmodload/autoloading-a-builtin-from-a-module", Category: "builtins",
		Snippet: `zmodload -a zsh/nosuchmodule mybuiltin; echo "st=$?"`,
		Why:     "`-a` registers a builtin to be loaded from a module on first use, and zsh takes the line with silence and 0 — it defers everything, so a module that does not exist is not its problem yet. Nothing here can autoload a builtin, so the letter is named as missing and the status differs; the row is what records that, rather than leaving a refusal graded against nothing",
	},
	{
		ID: "zmodload/loading-by-feature", Category: "builtins",
		Snippet: `zmodload -F zsh/zutil; echo "st=$?"`,
		Why:     "`-F` names the features to act on and, given none, loads the module whole: silence and 0 in zsh. A feature here is a builtin or a parameter the shell either has or has not, and `+zparseopts` cannot conjure one, so `-F` without `-l` is named as missing — the row records the difference rather than hiding it behind a 0 this shell has not earned",
	},
	// `zparseopts` and `zformat`, the two of `zsh/zutil`'s other three
	// builtins that can be learned by running the real one (#1058). What is
	// recorded is the part a caller reads by index and the part that decides
	// what a function passes on — because this builtin's failure mode is not
	// a crash but a caller handed plausible variables at status 0.
	//
	// `zregexparse` is deliberately not here. The manual gives it one
	// sentence, it needs the completion system's own state to do anything,
	// and a corpus row about a builtin that answers 2 to every invocation
	// would record nothing.
	// `zsh/parameter`'s five (#1060): this shell's own tables read through as
	// associations. Every one of them is a *view* — the answer is produced
	// when it is read — so the rows are written as read, mutate, read again
	// in one shell. A row that only defined a function and then looked would
	// pass against a snapshot taken late enough, which is the bug these
	// parameters exist to not have.
	//
	// Each snippet loads the module first, because in zsh these parameters do
	// not exist until it is loaded — and discards what that line says,
	// because it is not what the row is about. This shell has the five from
	// the start; since #1146 the module loads here too, over eighteen
	// parameters that refuse by name and ten that are empty and right to be,
	// which the two rows below grade on their own.
	{
		ID: "parameter/functions-is-a-view-of-the-function-table", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; echo "a=[${functions[g]:-ABSENT}] n=${#functions}"; g(){ :; }; echo "b=[${functions[g]:-ABSENT}] n=${#functions}"; unset -f g; echo "c=[${functions[g]:-ABSENT}] n=${#functions}"`,
		Why:     "three reads around two mutations, which is the shape a snapshot cannot satisfy at any instant: absent, then the body with its leading tab and a count of one, then absent again. A parameter filled in once and never updated answers the first pair three times and looks perfectly reasonable doing it. Nobody else has the module",
	},
	{
		ID: "parameter/a-function-body-is-what-a-listing-prints-between-the-braces", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; f(){ echo one; if [[ 1 = 1 ]]; then echo two; fi; }; printf "[%s]" "$functions[f]"`,
		Why:     "the value is the lines a listing puts between the braces — tab-indented, a nested block indented twice, no header line and no trailing newline. Every character that decides this row is whitespace, which is why it is bracketed and printed with `printf` rather than echoed",
	},
	{
		ID: "parameter/assigning-to-functions-defines-one", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; functions[h]="echo made"; h; echo "n=${#functions}"; unset "functions[h]"; echo "gone=${#functions}"`,
		Why:     "the association is written *through* to the function table in both directions: the assignment defines a function that then runs, and unsetting the element undefines it. A write that landed in an ordinary stored table would define nothing and — worse — would shadow the view from that moment, since a stored table is what a read finds first",
	},
	{
		ID: "parameter/options-is-a-view-of-the-option-namespace", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; echo "n=${#options} a=$options[extendedglob]"; setopt extendedglob; echo "b=$options[extendedglob]"; unsetopt extendedglob; echo "c=$options[extendedglob]"`,
		Why:     "197 names, and the state of one of them read on either side of a `setopt` and an `unsetopt`. The count is the part worth pinning beside the movement: it is the whole option namespace and not the couple of dozen names `set +o` writes, which is the same table under another name",
	},
	{
		ID: "parameter/the-compat-option-spellings-are-keys-of-their-own", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; echo "[$options[log]] [$options[histnofunctions]]"; setopt histnofunctions; echo "[$options[log]] [$options[histnofunctions]]"`,
		Why:     "the twelve sh and ksh compat spellings are keys here where the `setopt` and `unsetopt` listings never print them, which is what makes 197 rather than 185 — and a pair that means opposite states moves in opposite directions from one `setopt`. A table built from the listings alone is twelve keys short and cannot tell which way each of them reads",
	},
	{
		ID: "parameter/assigning-to-options-is-setopt", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; options[extendedglob]=on; [[ -o extendedglob ]] && echo on-now; options[nosuchopt]=on; echo "unknown=$?"; options[extendedglob]=maybe; echo "bad=$? still=[$options[extendedglob]]"`,
		Why:     "the assignment is `setopt` written another way, and its two complaints both leave the *command's* status at 0 — a name nobody has is `no such option` and a value that is neither on nor off is `invalid value`, with the option unmoved. The status is the surprise: a script testing `$?` after either learns nothing",
	},
	{
		ID: "parameter/builtins-drops-one-that-is-switched-off", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; echo "a=[$builtins[cd]]"; disable cd; echo "b=[${builtins[cd]:-ABSENT}]"; enable cd; echo "c=[$builtins[cd]]"`,
		Why:     "a builtin put aside with `disable` is not a key here at all — not a key with an empty value — which the default operator is what tells apart: a view that merely emptied the value would read identically to a caller using `$builtins[cd]` alone. zsh moves it to `dis_builtins`, which is one of the twenty-eight parameters of this module nothing here provides",
	},
	{
		ID: "parameter/builtins-is-readonly", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; builtins[x]=y; echo "st=$?"`,
		Why:     "`read-only variable: builtins` and a failure, where its four neighbors in this module all take an assignment. Worth a row because a produced association with no answer for a write has to have *some* answer: one that quietly stored the element would shadow the view it was written to",
	},
	{
		ID: "parameter/aliases-is-a-view-of-the-alias-table", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; alias q=ls; echo "b=[$aliases[q]]"; unalias q; echo "c=[${aliases[q]:-ABSENT}]"; aliases[zz]="echo z"; alias zz`,
		Why:     "the same table `alias` and `unalias` keep, read through and written through: an alias defined by the builtin is in the association, one removed by the builtin is gone from it, and one defined *by assignment* is what the builtin says back",
	},
	{
		ID: "parameter/commands-is-a-view-of-the-path-search", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; echo "absent=[${commands[definitelynotacommand]:-ABSENT}]"; PATH=/bin; echo "a=[${commands[ls]:-ABSENT}]"; PATH=/nonexistent; echo "b=[${commands[ls]:-ABSENT}]"`,
		Why:     "the association follows PATH, which is the property that makes it a view rather than a table filled in at startup: the same name resolves and then does not, in one shell, with nothing between the two reads but an assignment to PATH",
	},
	{
		ID: "parameter/loading-the-module-is-silent-and-zero", Category: "variables",
		Snippet: `zmodload zsh/parameter; echo "st=$?"; echo "n=${#functions}"`,
		Why:     "the line a plugin manager writes second, and the one that decides whether the rest of the file runs at all. Silence and 0 in zsh. This shell answered 1 for four merges over twenty-eight parameters the caller never touches, so the row is what pins the agreement rather than the refusal",
	},
	{
		ID: "parameter/a-parameter-of-the-module-nothing-here-provides", Category: "variables",
		Snippet: `zmodload zsh/parameter 2>/dev/null; echo "n=${#jobstates} one=[${jobstates[x]}]"; echo "after=$?"`,
		Why:     "**a recorded divergence, and the one this module's rule turns on.** zsh has the parameter and writes `n=0 one=[]`, which is the truth there — no jobs. This shell has not got it, and answering the same `0` would hand a caller an empty value for a question nobody answered, at status 0, in the spelling a plugin manager writes most (`${p[k]}` reaches no unset check at all). So it refuses by name at the expansion instead, and the row records the difference rather than leaving it to be discovered",
	},
	// The length of a one-element array, which is where `${#functions}` with
	// one function defined lands and where this implementation was wrong.
	{
		ID: "length/of-a-one-element-array", Category: "expansion",
		Snippet: `a=(hello); echo "one=${#a}"; a=(hello there); echo "two=${#a}"; a=(); echo "none=${#a}"`,
		Why:     "the shells split on what `${#a}` of an array means — one counts the elements and the others measure the scalar a bare name yields — and the split is visible at *one* element as much as at two: 1 against 5 for `a=(hello)`. A reading that skipped the question at one element on the grounds that such an array is its own element is true of the value and false of its length, and answers a plausible number at status 0",
	},
	{
		ID: "length/of-an-array-holding-one-empty-string", Category: "expansion",
		Snippet: `x=(""); typeset -p x; echo "one=${#x}"; x=("" ""); echo "two=${#x}"`,
		Why:     "the other half of the same question, and the half where both readings produce the same digit for opposite reasons: `x=(\"\")` is one element whose length is nought, so the shell that counts says 1 and the shells that measure the scalar say 0. Worth its own row because an implementation that decides *whether* a name is an array by looking at the scalar a one-element array is mirrored into cannot tell this array from an empty one — it answered 0 in the counting dialect too, which is the shell's own answer for `x=()` and no complaint anywhere (#1097). `typeset -p` is on the line to show the storage was right the whole time",
	},
	{
		ID: "zparseopts/an-argument-in-an-element-of-its-own", Category: "builtins",
		Snippet: `set -- -a val rest; zparseopts -D a:=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		Why:     "the spec grammar's load-bearing half: a single colon puts the option's argument in an element of its own, so `$x[2]` is the argument. `-D` takes what was matched out of `$@` and leaves the rest, which is the other half of what makes this usable as a function's option parser. Nobody else has the builtin",
	},
	{
		ID: "zparseopts/an-argument-joined-to-the-option", Category: "builtins",
		Snippet: `set -- -a val r; zparseopts -D a:-=x; echo "st=$? x=[${(j:|:)x}]"`,
		Why:     "and `:-` puts it in the *same* element — `(-aval)` where `a:` gives `(-a val)`. One character apart in the description and a different array shape out, which is why a parser that read the forms loosely would hand a caller the next option where it expected an argument",
	},
	{
		ID: "zparseopts/an-optional-argument-reaches-the-next-word", Category: "builtins",
		Snippet: `set -- -a rest more; zparseopts -D a::=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		Why:     "the surprise in `::`: optional does not mean `in the same word`. A following word is taken as the argument and joined on — `(-arest)` — unless it begins with a `-`. A reading that required the same word leaves `rest` in `$@`, which the script then treats as its first operand",
	},
	{
		ID: "zparseopts/a-mandatory-argument-takes-whatever-is-next", Category: "builtins",
		Snippet: `set -- -a -- z; zparseopts -D a:=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		Why:     "where mandatory and optional part company: a mandatory argument is taken from the next word whatever it looks like, `--` included, so `(-a --)` is the answer and not a stop. The pair with the `::` row above is the whole of the difference",
	},
	{
		ID: "zparseopts/a-missing-argument-changes-nothing", Category: "builtins",
		Snippet: `set -- -a; zparseopts -D a:=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		Why:     "an option without its required argument aborts the whole parse: status 1, the array untouched and `$@` untouched, rather than a half-filled array a caller would read as a successful parse. The status is the part a script can act on and the two empties are the part it cannot see",
	},
	{
		ID: "zparseopts/only-the-last-occurrence-without-a-plus", Category: "builtins",
		Snippet: `set -- -a v1 -a v2; zparseopts -D a:=x; echo "st=$? x=[${(j:|:)x}]"; set -- -a v1 -a v2; zparseopts -D a+:=x; echo "plus=[${(j:|:)x}]"`,
		Why:     "`+` is the difference between an option that accumulates and one that does not, side by side so the pair is one row: without it only the last appearance survives and with it every one does. A parser that always accumulated would give a caller two values where it asked for one, at 0",
	},
	{
		ID: "zparseopts/two-descriptions-sharing-one-array", Category: "builtins",
		Snippet: `set -- -a -b c; zparseopts -D -a arr a b; echo "st=$? arr=[${(j:|:)arr}] argv=[${(j:|:)@}]"`,
		Why:     "`-a` names the default array, and several descriptions may write into one — in command-line order. This is what says the `keep only the last` pruning is per *description* and not per array: a per-array rule answers `(-b)` here and looks perfectly reasonable doing it",
	},
	{
		ID: "zparseopts/parsing-stops-where-the-descriptions-run-out", Category: "builtins",
		Snippet: `set -- x -a y; zparseopts -D a=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"; set -- x -a y; zparseopts -D -E a=x; echo "every=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		Why:     "where a parse stops decides what a function hands on to something else: at the first word no description covers, unless `-E`. Both readings in one row, because a parser that ran to the end of `$@` would collect flags meant for a command further down the line and the passing half would look identical",
	},
	{
		ID: "zparseopts/a-double-dash-always-stops-it", Category: "builtins",
		Snippet: `set -- -a -- -b; zparseopts -D a=x b=y; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}] argv=[${(j:|:)@}]"; set -- -a -- -b; zparseopts -D -E a=x b=y; echo "every=$? y=[${(j:|:)y}] argv=[${(j:|:)@}]"`,
		Why:     "`--` stops the parse with `-E` as much as without it — the letter changes only whether `-D` removes it. `-b` is described and still not collected either way, which is the row's point: the stop is about the word and not about the descriptions",
	},
	{
		ID: "zparseopts/one-dash-is-added-to-the-name-as-written", Category: "builtins",
		Snippet: `set -- --foo=v r; zparseopts -D -- -foo:=x; echo "st=$? x=[${(j:|:)x}] argv=[${(j:|:)@}]"`,
		Why:     "a description names the option without its leading `-`, so a GNU-style `--foo` is described as `-foo`. And there is no `=` handling: the argument of `--foo=v` is the three characters `=v`, which a parser borrowing getopt's habits would silently strip",
	},
	{
		ID: "zparseopts/the-longest-name-wins-among-flags", Category: "builtins",
		Snippet: `set -- --foobar; zparseopts -D -- -foobar=y -foo=x; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}]"; set -- --foobar; zparseopts -D -- -foobar=y -foo:=x; echo "arg=$? x=[${(j:|:)x}] y=[${(j:|:)y}]"`,
		Why:     "two overlapping descriptions, and which wins depends on whether either takes an argument: among flags the longest name wins whichever order they were written in, and once one takes an argument the last one written wins instead. The two halves differ in one colon and give opposite answers",
	},
	{
		ID: "zparseopts/flags-cluster-in-one-word", Category: "builtins",
		Snippet: `set -- -ab r; zparseopts -D a=x b=y; echo "st=$? x=[${(j:|:)x}] y=[${(j:|:)y}] argv=[${(j:|:)@}]"`,
		Why:     "`-ab` is two options, each written into its own array under its own name. A parser matching whole words would leave both arrays empty and `$@` untouched, at 0",
	},
	{
		ID: "zparseopts/the-association-and-what-dash-k-keeps", Category: "builtins",
		Snippet: `typeset -A o; o[z]=1; set -- -a v; zparseopts -D -K -A o -- a:; echo "st=$? ${(kv)o}"; x=(old); set -- -b; zparseopts -D -K a=x b=y; echo "keep=$? x=[${(j:|:)x}]"; x=(old); set -- -b; zparseopts -D a=x b=y; echo "reset=$? x=[${(j:|:)x}]"`,
		Why:     "`-A` puts the options and their arguments in an association keyed by the option, and `-K` decides what a call leaves behind — an association keeps its individual elements, an array is kept whole when none of its descriptions matched and emptied otherwise. The two `x=(old)` runs differ only in the letter and are the whole of what it means",
	},
	{
		ID: "zparseopts/the-letters-do-not-stack", Category: "builtins",
		Snippet: `zparseopts -DQ a=x; echo "st=$?"; zparseopts -D a; echo "nowhere=$?"`,
		Why:     "`-DQ` is not `-D -Q`: the letters cannot be stacked, because a stack is indistinguishable from a description of the GNU-style long option `--DQ` — so the word becomes a description and complains about having nowhere to put what it finds, which is the same complaint a bare `a` gets. A builtin that stacked its letters would answer `bad option` here and look more helpful while being wrong about the syntax",
	},
	{
		ID: "zparseopts/nothing-to-parse-against", Category: "builtins",
		Snippet: `set -- -a; zparseopts -D; echo "st=$?"`,
		Why:     "no descriptions at all is `missing option descriptions` and 1, where a bare `zparseopts` doing nothing quietly would be the plausible answer. The builtin's name is in the location, which is where this shell puts it",
	},
	{
		ID: "zformat/substitution-with-a-field-width", Category: "builtins",
		Snippet: `zformat -f R "[%10c][%-10c][%5.2c]" c:hi; echo "st=$? [$R]"`,
		Why:     "the whole of `-f`'s width grammar in one string: a minimum pads to the right, a negative minimum pads to the left, and a maximum truncates before the minimum pads. Bracketed because every one of those decisions is invisible in the text and visible only in the spaces",
	},
	{
		ID: "zformat/a-percent-naming-nothing-is-text", Category: "builtins",
		Snippet: `zformat -f R "a%xb%5x%%" c:1; echo "st=$? [$R]"`,
		Why:     "the specifier set belongs to the caller, so a `%` sequence naming no specification is written back exactly as it stands, width and all — not an error and not an empty string. `%%` beside it is the one sequence this builtin owns",
	},
	{
		ID: "zformat/the-ternary-evaluates-its-value", Category: "builtins",
		Snippet: `zformat -f R "%3(c.yes.no)" c:3; echo "st=$? [$R]"; zformat -f R "%2(c.y.n)" "c:1+1"; echo "expr=$? [$R]"; zformat -f R "%2(c.y.n)" "c:x"; echo "word=$? [$R]"`,
		Why:     "`%n(c.true.false)` compares the test number with the specification's value read as an *arithmetic expression*, which `1+1` against a test of 2 is the proof of — a number parse answers the false text there. A word is worth zero, which is the shell's own reading and not a failure",
	},
	{
		ID: "zformat/dash-f-capital-asks-about-width-instead", Category: "builtins",
		Snippet: `zformat -F R "[%(c.y.n)][%3(c.y.n)][%3(d.y.n)][%-3(d.y.n)]" c:abc d:abcd; echo "st=$? [$R]"`,
		Why:     "`-F` swaps the ternary's question from `equals the test number` to `is longer than it`, and a negative test number reverses that into `no longer than`. Both boundaries are here at exactly three characters, because a strict comparison and a loose one differ nowhere else",
	},
	{
		ID: "zformat/aligning-a-list", Category: "builtins",
		Snippet: `zformat -a A " -- " "foo:" "x:y" "longer:baz" "nocolon" "a\:b"; echo "st=$?"; printf '[%s]\n' "$A[@]"`,
		Why:     "`-a` lines the separators up, and the rule worth pinning is which strings *count*: one with no colon is left alone and one whose right half is empty loses its colon, and neither takes part in deciding the column. So the separator sits at `longer`'s width and `foo` never widened it. a backslash-escaped colon is the last row and the one that says the escape is undone even where nothing split",
	},
	{
		ID: "zformat/an-expression-that-will-not-evaluate", Category: "builtins",
		Snippet: `zformat -f R "%0(c.y.n)" "c:1/0"; echo "st=$? [$R]"`,
		Why:     "a ternary's value is an expression, and one that will not *evaluate* is not a zero: zsh reports `division by zero` located as the shell rather than as the builtin — the expression failed, not the command holding it — and stops the script, so the `echo` never runs and `$R` is never written. A builtin that read it as 0 would choose the true text and hand its caller a plausible answer to a question that failed",
	},
	{
		ID: "zformat/exactly-one-option-word", Category: "builtins",
		Snippet: `zformat -f -F R "%(c.y.n)" c:; echo "st=$?"; zformat -fa R x; echo "stack=$?"; zformat -Z R x; echo "bad=$?"`,
		Why:     "the part a reading of the synopsis gets wrong: exactly one option word is read and everything after it is data. `-f -F R …` makes `-F` the parameter name and complains about the format's *specification*, `-fa` is not an option word at all, and only a lone unknown letter is `invalid option`. Three different sentences for what looks like one kind of mistake",
	},
	{
		ID: "zstyle/stores-and-lists-what-it-is-given", Category: "builtins",
		Snippet: `zstyle ':completion:*' verbose yes; echo "st=$?"; zstyle -L`,
		Why:     "the honest minimum an rc file needs: zsh takes the style, reports 0 and says it back as the command that would set it. Everyone else has no zstyle at all, which is why a config's first twenty lines die outside zsh",
	},
	{
		ID: "zstyle/more-components-sort-first", Category: "builtins",
		Snippet: `zstyle ':q:*' v 2; zstyle ':m:n:*' v 3; zstyle ':a:b:c:d:*' v 1; zstyle -L`,
		Why:     "the listing's primary key, and it is not insertion order: the four-colon pattern comes out first though it was set last. This is the same order a lookup walks, so it is what `the most specific pattern wins` actually means",
	},
	{
		ID: "zstyle/an-exact-pattern-beats-a-wildcard-one", Category: "builtins",
		Snippet: `zstyle 'a*b' v 1; zstyle 'a?b' v 2; zstyle 'ab' v 3; zstyle -L`,
		Why:     "the tie-break within one component count: the pattern with no metacharacter sorts first though it was set last, while the two that have one keep the order they were set in",
	},
	{
		ID: "zstyle/the-most-specific-pattern-is-the-one-read", Category: "builtins",
		Snippet: `zstyle ':a:b:*' v specific; zstyle ':a:*' v general; zstyle -s ':a:b:c' v out; echo "[$out]"`,
		Why:     "the ordering seen from the reading end, set in the order that would fool a first-match-wins implementation: the longer context wins whichever was set first",
	},
	{
		ID: "zstyle/the-test-form-has-three-statuses", Category: "builtins",
		Snippet: `zstyle ':a:*' v no; zstyle -t ':a:b' v; echo "false=$?"; zstyle -t ':zz' v; echo "unset=$?"`,
		Why:     "`-t` reports 1 for a style set to something false and 2 for one nobody set, which is the distinction that lets a reader tell `off` from `unsaid` — a two-status test would collapse them",
	},
	{
		ID: "zstyle/refuses-a-bad-option-its-own-way", Category: "builtins",
		Snippet: `zstyle -z ':a' v; echo "st=$?"`,
		Why:     "`invalid option: -z` at 1 — and not `bad option`, which is what bindkey says for the same mistake in the same shell. The two builtins' wordings are separate facts",
	},
	{
		ID: "bindkey/an-unknown-widget-is-accepted-in-silence", Category: "builtins",
		Snippet: `bindkey '^X^T' no-such-widget; echo "st=$?"; bindkey '^X^T'`,
		Why:     "the finding that makes a real config work: zsh stores a binding to a widget it has never heard of, reports 0, says nothing, and reads it back. A shell that refused would be stricter than zsh and would fail every config using a plugin's widget",
	},
	{
		ID: "bindkey/says-one-binding-back", Category: "builtins",
		Snippet: `bindkey '^A'; bindkey -r '^A'; bindkey '^A'; echo "st=$?"`,
		Why:     "asking about a key is a question with an answer rather than a lookup that can fail: a key nobody has bound is `undefined-key` at status 0, and that is also what removing one leaves behind",
	},
	{
		ID: "bindkey/refuses-a-bad-option-its-own-way", Category: "builtins",
		Snippet: `bindkey -q; echo "st=$?"; bindkey -r; echo "short=$?"`,
		Why:     "`bad option: -q` where zstyle says `invalid option`, and a usage complaint that names the letter that was short — `not enough arguments for -r` — where zstyle's names nothing",
	},
	{
		ID: "bindkey/names-the-keymaps", Category: "builtins",
		Snippet: `bindkey -l; echo "st=$?"; bindkey -M nosuchmap '^A' beginning-of-line; echo "bad=$?"`,
		Why:     "the keymap list a config's `bindkey -M` has to name one of, and what a name outside it costs: `no such keymap` with zsh's own backtick-and-quote spelling, at 1",
	},
	{
		ID: "print/prompt-escapes-run-after-the-backslash-ones", Category: "builtins",
		Snippet: `print -P '\045\045'; echo "st=$?"`,
		Why:     "the order of `print -P`'s two passes, put where only one order can produce the answer: `\045` is a backslash escape for `%`, so the backslash pass makes `%%` and the prompt pass then reads that as one `%`. A shell doing the prompt pass first sees no `%` at all and prints `%%`. zsh alone has `print -P`; ksh93 has `print` and refuses `-P` with its usage line, and bash and dash have no `print`",
	},
	{
		ID: "print/prompt-escapes-do-not-read-their-own-result", Category: "builtins",
		Snippet: `print -P '%%%%'; echo "st=$?"`,
		Why:     "the other half of the same rule, and the row that a second prompt pass would fail: `%%%%` never meets the backslash pass, and the two `%%` make `%%` rather than `%` — the `%` each one produced is not looked at again",
	},
	{
		ID: "print/a-trailing-percent-is-dropped", Category: "builtins",
		Snippet: `print -P 'x%'; echo "st=$?"`,
		Why:     "a `%` with nothing after it is dropped rather than written — `x`, not `x%`. Worth a row because writing it through is what an implementation that only looks at complete pairs does, and it is silently one character wrong on every prompt that ends in a percent",
	},
	{
		ID: "print/a-prompt-escape-this-shell-has-not", Category: "builtins",
		Snippet: `print -P '[%q]'; echo "st=$?"`,
		Why:     "zsh's prompt language has about forty escapes and this shell carries four of them. `%q` is one it does not, and zsh drops it silently — `[]` and 0 — so the row records the difference between dropping an escape and naming it as missing. Naming it is the choice here: a prompt quietly short of a field is the kind of wrong answer nobody reports",
	},
	{
		ID: "print/the-raw-letter-leaves-the-prompt-pass-alone", Category: "builtins",
		Snippet: `print -rP 'a\tb%%'; echo "st=$?"`,
		Why:     "`-r` suppresses the backslash pass and not the prompt one: the tab stays two characters and the `%%` still becomes one `%`. The two letters are about different passes, which is what a shell folding them into one `raw` flag would get wrong",
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
		ID: "print/l-and-n-are-the-separator-and-the-terminator", Category: "builtins",
		Snippet: `print -l a b; print -ln c d; echo .`,
		Why:     "zsh's `-l` separates the operands with newlines and `-n` withholds the terminator, so the two compose rather than canceling: `c~d.` on one line after `a~b`. ksh93 has no `-l` at all and says so twice, which is the dialect boundary inside a builtin both shells have",
	},
	{
		ID: "print/nul-separates-and-terminates", Category: "builtins",
		Snippet: `print -N a b | od -An -c | tr -s " "`,
		Why:     "`-N` is zsh's alone and moves *both* settings: a NUL between the operands and a NUL after the last one, which is what makes `print -N` the writing half of `read -d ''`",
	},
	{
		ID: "print/the-escapes-this-builtin-has-beyond-echo-s", Category: "builtins",
		Snippet: `print '\101\zx\1' | od -An -c | tr -s " "`,
		Why:     "the three ways zsh's print outruns its own echo and ksh93's print alike: a bare octal escape with no leading zero, an escape nobody knows losing its backslash rather than keeping it, and `\\1` as one byte. ksh93 prints all three as written",
	},
	{
		ID: "print/capital-r-changes-the-option-parser", Category: "builtins",
		Snippet: `print -Rl a b; print -R -l c d`,
		Why:     "`-R` is raw in both shells and stops reading options in neither the same way: zsh reads the rest of the bundle as its own letters, so `-Rl` still lists one per line, while a later `-l` word is an operand. ksh93 reads no more letters at all and prints `a b`",
	},
	{
		ID: "print/sorts-and-filters-its-operands", Category: "builtins",
		Snippet: `print -o B a C; print -O B a C; print -m 'a*' abc bcd axy`,
		Why:     "zsh's operand-list letters, none of which ksh93 has: `-o` sorts, `-O` sorts backwards, and `-m` reads the first operand as a pattern and keeps only the operands it matches. The order is byte order, which is what LC_ALL=C gets",
	},
	{
		ID: "print/the-history-and-editor-letters-write-nowhere", Category: "builtins",
		Snippet: `print -s a b; echo "s=$?"; print -z c; echo "z=$?"; print -S x y; echo "S=$?"`,
		Why:     "`-s` and `-z` aim at a history and a line editor a non-interactive shell has not got: the operands are consumed and the answer is 0 in both shells for `-s`, while `-z` and `-S` are zsh's alone and `-S` refuses more than one operand",
	},
	{
		ID: "print/e-is-echo-s-letter-in-only-one-of-them", Category: "builtins",
		Snippet: `print -e 'a\tb'; echo "st=$?"`,
		Why:     "ksh93's `-e` puts escape expansion back after a `-r`, and zsh — whose print expands by default and spells the same idea only inside `-R` — calls the letter a bad option at 1. The sharpest place the two builtins under one spelling disagree",
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
	// --- what the environment says at startup (#596). One shell in the panel
	// reads two names out of it before the first line runs; the other three
	// leave both as ordinary strings, which is what makes every row here
	// evidence rather than a coincidence.
	{
		ID: "env/an-inherited-option-list-turns-an-option-on", Category: "invocation",
		Env:     []string{"SHELLOPTS=nounset"},
		Snippet: `case $- in *u*) echo has-u ;; *) echo no-u ;; esac`,
		Why:     "the sharpest startup input a shell takes: a name in the environment changes what every command afterwards does. bash reads it and `$-` gains the letter; dash, ksh93 and zsh ignore the name entirely, which is the control",
	},
	{
		ID: "env/an-inherited-option-list-outranks-the-invocations-own-option", Category: "invocation",
		Env:     []string{"SHELLOPTS=nounset"},
		Args:    []string{"+u", "-c", ArgSnippet},
		Snippet: `case $- in *u*) echo has-u ;; *) echo no-u ;; esac`,
		Why:     "the ordering, and it is the opposite of every other startup input: the environment is read *after* the argument vector, so `+u` written out does not undo it. The three that do not read the name answer `no-u` here and `no-u` in the row above, so this row is about the order rather than about the letter",
	},
	{
		ID: "env/an-unknown-name-in-an-inherited-option-list", Category: "invocation",
		Env:     []string{"SHELLOPTS=nosuchoption:nounset"},
		Snippet: `case $- in *u*) echo has-u ;; *) echo no-u ;; esac`,
		Why:     "one bad entry costs only itself: the complaint names line 0 — nothing has been read — and the good name in the same value is still applied. The wording is the plainest of the three shapes this refusal has, with nothing standing where `set` would",
	},
	{
		ID: "env/the-option-list-follows-the-option-letters", Category: "invocation",
		Snippet: `set -u; case ":$SHELLOPTS:" in *:nounset:*) echo listed ;; *) echo not-listed ;; esac`,
		Why:     "the read direction of the binding, and the reason a stored copy would be a lie: the variable is produced when it is read, so an option set after startup is in it. Read as membership rather than as a string, for the reason `$-` is — what a shell has on by default is its own business",
	},
	{
		ID: "env/the-option-list-drops-an-option-turned-off", Category: "invocation",
		Snippet: `set -u; set +u; case ":$SHELLOPTS:" in *:nounset:*) echo listed ;; *) echo not-listed ;; esac`,
		Why:     "the other half of the same binding, and the one a copy taken at startup would fail: turning the option off takes the name back out again",
	},
	{
		ID: "env/the-option-list-uses-long-names", Category: "invocation",
		Snippet: `set -f; case ":$SHELLOPTS:" in *:noglob:*) echo long ;; *:f:*) echo letter ;; *) echo neither ;; esac`,
		Why:     "normalized rather than echoed: what goes in is a letter and what comes out is the long name, which is why nothing can usefully compare the whole string against what it exported",
	},
	{
		ID: "env/the-option-list-is-readonly", Category: "invocation",
		Snippet: `SHELLOPTS=whatever; echo after`,
		Why:     "a name whose value is produced cannot be assigned to meaningfully, and the shell that has it refuses rather than accepting quietly. The refusal is its ordinary readonly one — wording, status and whether the script survives are all the dialect's — and the other three take the assignment as the ordinary variable it is for them",
	},
	{
		ID: "env/a-file-named-for-a-non-interactive-shell-is-sourced", Category: "invocation",
		Env:     []string{"BASH_ENV=" + ArgScript},
		Args:    []string{"-c", "echo main"},
		Snippet: `echo sourced`,
		Why:     "the non-interactive counterpart of `$ENV`, and one shell's alone: bash sources the file before the command string and the other three do nothing with the name. The snippet is the *file*, which is why the argv runs something else",
	},
	{
		ID: "env/that-file-sees-the-invocations-parameters", Category: "invocation",
		Env:     []string{"BASH_ENV=" + ArgScript},
		Args:    []string{"-c", "echo main", "name", "A"},
		Snippet: `echo "[$0] n=$# [${1-}]"`,
		Why:     "it is run *by* the shell that is about to run the program and sees what that shell sees, which is what puts it after the runner is built and after the operands are named — the same shape the login profile has",
	},
	{
		ID: "env/that-file-can-end-the-shell", Category: "invocation",
		Env:     []string{"BASH_ENV=" + ArgScript},
		Args:    []string{"-c", "echo main"},
		Snippet: `echo in-file; exit 3`,
		Why:     "`exit 3` in it exits 3 and the program never runs, which is the other half of it being run by this shell rather than beside it",
	},
	{
		ID: "env/a-file-named-for-a-non-interactive-shell-that-is-not-there", Category: "invocation",
		Env:     []string{"BASH_ENV=/nonexistent-directory/nonexistent-file"},
		Args:    []string{"-c", ArgSnippet},
		Snippet: `echo main`,
		Why:     "not a failure, in the shell that reads the name or in the three that do not. Every shell starts for the first time without one, and a complaint about it would be the first thing anybody saw",
	},
	{
		ID: "env/a-file-named-for-a-non-interactive-shell-is-not-read-when-called-sh", Category: "invocation",
		Argv0:   "sh",
		Env:     []string{"BASH_ENV=" + ArgScript},
		Args:    []string{"-c", "echo main"},
		Snippet: `echo sourced`,
		Why:     "the two startup questions composed, and the reason the file is gated on the mode rather than on a second name: the shell that sources this file called by its own name sources nothing called `sh`, exactly as it sources nothing under the standard's posix option. Pair it with env/a-file-named-for-a-non-interactive-shell-is-sourced, which is the same case under the shell's own name",
	},

	// --- $ENV: the file a shell reads because there is a person -------
	{
		ID: "env/the-interactive-file-is-read-at-a-prompt", Category: "invocation",
		Env:     []string{"ENV=" + ArgScript},
		Args:    []string{"-i", "-c", "echo main"},
		Snippet: `echo sourced`,
		Why:     "`$ENV` is the standard's interactive startup file, and the panel splits over it exactly where a shell has a file of its own name: dash and ksh93 read it, bash reads `~/.bashrc` instead and zsh `~/.zshrc`, so neither of those two touches this name. `-i` rather than a terminal, because being interactive is what the file is gated on and `-i` says so on every route",
	},
	{
		ID: "env/the-interactive-file-is-read-when-called-sh", Category: "invocation",
		Argv0:   "sh",
		Env:     []string{"ENV=" + ArgScript},
		Args:    []string{"-i", "-c", "echo main"},
		Snippet: `echo sourced`,
		Why:     "the same invocation under the standard's own name, where the panel becomes unanimous: a shell in POSIX mode reads the standard's interactive file and not one of its own. That makes it the interactive half of what env/a-file-named-for-a-non-interactive-shell-is-not-read-when-called-sh records — the non-interactive file is *suppressed* in the mode and this one is *substituted*, because the standard has an interactive file and no other kind",
	},
	{
		ID: "env/the-interactive-file-is-not-read-by-a-script", Category: "invocation",
		Env:     []string{"ENV=" + ArgScript},
		Args:    []string{"-c", "echo main"},
		Snippet: `echo sourced`,
		Why:     "unanimous, and the whole point of the file: a script must not inherit what somebody wrote for their keyboard. Pair it with env/the-interactive-file-is-read-at-a-prompt, which is the same invocation with `-i`",
	},
	{
		ID: "env/the-non-interactive-file-stands-down-at-a-prompt", Category: "invocation",
		Env:     []string{"BASH_ENV=" + ArgScript},
		Args:    []string{"-i", "-c", "echo main"},
		Snippet: `echo sourced`,
		Why:     "the two files are complementary rather than alternatives: the shell that has a non-interactive startup variable reads it when there is nobody to prompt and a file of its own name when there is, and never both. So `-i` makes the one shell that reads this name read nothing here",
	},
	{
		ID: "env/the-interactive-file-sees-the-invocations-parameters", Category: "invocation",
		Argv0:   "sh",
		Env:     []string{"ENV=" + ArgScript},
		Args:    []string{"-i", "-c", "echo main", "name", "A"},
		Snippet: `echo "[$0] n=$# [${1-}]"`,
		Why:     "run *by* the shell that is about to run the program and seeing what that shell sees, which is the same shape the login profile and the non-interactive file have — and what puts it after the runner is built and after the operands are named",
	},
	{
		ID: "env/the-interactive-file-can-end-the-shell", Category: "invocation",
		Argv0:   "sh",
		Env:     []string{"ENV=" + ArgScript},
		Args:    []string{"-i", "-c", "echo main"},
		Snippet: `echo in-file; exit 3`,
		Why:     "`exit 3` in it exits 3 and the program never runs, which is the other half of it being run by this shell rather than beside it",
	},
	{
		ID: "env/the-interactive-file-that-is-not-there", Category: "invocation",
		Argv0:   "sh",
		Env:     []string{"ENV=/nonexistent-directory/nonexistent-file"},
		Args:    []string{"-i", "-c", ArgSnippet},
		Snippet: `echo main`,
		Why:     "not a failure in any of them. Every shell starts for the first time without one, and a complaint would be the first thing anybody saw — the same answer the non-interactive file gives, recorded separately because they are read on different routes",
	},

	// --- the shapes an automated caller writes (#499) ------------------
	//
	// A coding-agent harness, an editor's terminal and a CI wrapper each
	// reach a shell through a small vocabulary of invocations, and it is not
	// the vocabulary a person types. Each case here says which part of a
	// real caller it came from, because a shape nobody can re-verify is a
	// guess that has been written down.
	//
	// The provenance is one machine's own tooling, read from what it left
	// behind rather than from anyone's documentation. The harness driving
	// this checkout starts a login shell once, writes that shell's state to
	// a file, and sources the file ahead of every later command. The file
	// names its own sections — functions, shell options, aliases — opens
	// with `unalias -a 2>/dev/null || true`, spells every alias
	// `alias -- name=value`, and closes with a single `export PATH='…'`.
	// Forty-five alias lines in the copy that was measured.
	//
	// These belong in the corpus rather than only in driver's own tests for
	// the reason Case.Args exists at all: the drivers once scored 178/198
	// against a core scoring 198/198, because the corpus graded the language
	// and nothing graded the invocation.
	{
		ID: "harness/a-login-bundle-runs-the-command-string", Category: "harness invocation",
		Args:    []string{"-lc", ArgSnippet},
		Snippet: `echo ran`,
		Why:     "`$SHELL -lc '…'` is the wrapper shape — a login shell so the person's profile is in scope, a command string so nothing is interactive — and all six run the string. The letters are one word, so a front end that read `-lc` as an option called `lc` would refuse the invocation outright and a front end that stopped the bundle at `l` would open `echo ran` as a script file",
	},
	{
		ID: "harness/the-login-letter-in-dollar-dash", Category: "harness invocation",
		Args:    []string{"-lc", ArgSnippet},
		Snippet: `case $- in *l*) echo has-l ;; *) echo no-l ;; esac`,
		Why:     "four against two, so it is an axis rather than a rule: ksh93 and zsh put `l` in `$-` for a shell started as a login shell, and dash and all three bash columns do not — bash keeps the fact in `shopt login_shell` instead, which is a different question with a different answer. Membership rather than the spelling, for the reason every other `$-` case uses it: no two shells order the string alike",
	},
	{
		ID: "harness/the-login-letter-spelled-as-its-own-word", Category: "harness invocation",
		Args:    []string{"-l", "-c", ArgSnippet},
		Snippet: `case $- in *l*) echo has-l ;; *) echo no-l ;; esac`,
		Why:     "the same question with the letters unbundled, and every column answers exactly as it did bundled — so the split above belongs to the shell rather than to how the caller spelled it. Without this pair a front end that recorded the letter only for a bundle would pass the row above",
	},
	{
		ID: "harness/a-dashed-argv-zero-is-a-login-shell", Category: "harness invocation",
		Argv0:   "-shell",
		Args:    []string{"-c", ArgSnippet},
		Snippet: `case $- in *l*) echo has-l ;; *) echo no-l ;; esac`,
		Why:     "the login route no option can reach, and the one `login` and every terminal emulator's \"run as a login shell\" actually takes: a dashed `argv[0]` and nothing else. The split is exactly the one the two rows above record — ksh93 and zsh say `has-l`, dash and all three bash columns say `no-l` — which is what says the letter is about login-ness rather than about the option having been written. The name is `-shell` rather than `-sh` on purpose: a dash in front of `sh` would ask two questions at once, since that name also starts a shell in POSIX mode, and measured with `-shell` the answer is the same in all six. Paired with the row below, which is the same name undashed",
	},
	{
		ID: "harness/an-undashed-argv-zero-is-not-a-login-shell", Category: "harness invocation",
		Argv0:   "shell",
		Args:    []string{"-c", ArgSnippet},
		Snippet: `case $- in *l*) echo has-l ;; *) echo no-l ;; esac`,
		Why:     "the control the row above needs, and unanimous: one character earlier is the whole of the convention, so the same name without its dash is no login shell in any of the six. Without this a front end that put `l` in `$-` for every invocation, or that read login-ness off the name rather than off the dash, would pass the row above",
	},
	{
		ID: "harness/an-interactive-bundle-runs-the-command-string", Category: "harness invocation",
		Args:    []string{"-ic", ArgSnippet},
		Snippet: `echo ran; case $- in *i*) echo has-i ;; *) echo no-i ;; esac`,
		Why:     "`-ic` is what a caller writes when it wants the person's aliases and functions in scope, and it is the invocation with a terminal missing from it. Unanimous twice — every shell runs the string and every shell puts `i` in `$-` — and split four ways on what it says about job control first: bash 5.3 names the process group it could not set and then says there is none, bash 3.2 says only the second half, dash says it cannot reach a tty, and ksh93 and zsh say nothing at all. The process id in bash's line is masked by the harness rather than dropped, because *that it named one* is part of the complaint",
	},
	{
		ID: "harness/a-login-and-interactive-bundle", Category: "harness invocation",
		Args:    []string{"-lic", ArgSnippet},
		Snippet: `echo ran`,
		Why:     "three letters in one word, which is the shape a terminal emulator's run-this-command setting writes and the longest bundle a real caller produces. It composes: the job-control announcement is the interactive half's, unchanged by the `l` beside it, and every shell still runs the string",
	},
	{
		ID: "harness/an-option-after-the-command-string-is-an-operand", Category: "harness invocation",
		Args:    []string{"-c", ArgSnippet, "-l"},
		Snippet: `echo "0=[$0] n=$# 1=[${1-}]"`,
		Why:     "the mistake a caller assembling an argv from a list makes, and it is silent in all six: an option word written *after* the command string is not an option, it is the first operand, so `-l` becomes `$0` and the shell is not a login shell at all. Nothing complains and nothing is refused — which is why it needs a case rather than a diagnostic. The pair to invoke/options-between-c-and-its-string, where the same word one position earlier *is* an option",
	},
	{
		ID: "harness/a-command-string-that-spans-lines", Category: "harness invocation",
		Args:    []string{"-lc", ArgSnippet},
		Snippet: "cat <<EOF\none\nEOF\necho \"st=$?\"",
		Why:     "a caller hands the person's command over whole, so the command string is routinely several lines with a here-document in it — the one construct that needs the lines *after* the line it appears on. Unanimous, and it is the control the two multi-line rows below are read against: a front end that read only the first line of `-c` would print nothing and exit 0",
	},
	{
		ID: "harness/a-snapshot-file-is-sourced-then-the-command-runs", Category: "harness invocation",
		Args:    []string{"-lc", ". " + ArgScript + " && echo main"},
		Snippet: "unalias -a 2>/dev/null || true\nalias -- g='echo one'\nexport PATH=\"$PATH\"",
		Why: "the whole harness shape in one case: a login-bundled command string that sources a captured state file and then runs the command, with the file's three structural lines standing in for the real one's several thousand. " +
			"dash is the finding. It has no `--` for `alias`, so it reads the word as a name to look up, fails to find it, and writes `alias: -- not found` — once per alias line, which is forty-five times in the file that was measured, into the output the caller then shows a person as the command's own. Every other column is silent, and all six reach `main`",
	},
	{
		ID: "harness/the-snapshot-spelling-of-an-alias", Category: "harness invocation",
		Snippet: `alias -- g='echo one'; echo "st=$?"; alias g`,
		Why:     "the row above narrowed to the one line that splits, with the status shown. `alias -- name=value` is the re-inputtable spelling three shells print and only some accept as input: five take the `--` as the end of the options and exit 0, and dash takes it as an operand, diagnoses it and exits 1 — with the alias still set, which is why the failure is survivable and therefore easy to miss. A generator's file under `set -e` would stop at its first alias",
	},
	{
		ID: "harness/enumerating-every-function-at-once", Category: "harness invocation",
		Snippet: `f() { echo hi; }; g() { echo bye; }; declare -F; echo "st=$?"`,
		Why:     "how a state capture asks what functions exist. Three answers rather than two, and the middle one is the dangerous shape: bash lists both names, dash and ksh93 have no `declare` at all and say so at 127, and zsh reads `-F` as a float's precision, has nothing to say about a bare one, and exits **0** — a caller that trusted the status would record a shell with no functions and never learn it had asked the wrong question. declare/capital-f-names-a-function asks the same thing of a *named* function; this asks for the listing, which is what a generator actually runs",
	},
	{
		ID: "harness/a-function-listing-that-says-nothing-at-all", Category: "harness invocation",
		Snippet: "f() { echo hi; }\n" +
			`case $(declare -F 2>&1) in "") echo silent ;; *) echo spoke ;; esac` + "\n" +
			"declare -F >/dev/null 2>&1\n" +
			`echo "st=$?"`,
		Why: "the row above narrowed to the half a status cannot show: whether the shell said anything, on either stream. zsh is the finding and the silence is the whole of it — `-F` is a float's precision there, so it answers a bare one with **0 and not one byte**, which is the shape that lets a caller record a shell with no functions and never learn it asked the wrong question. The three bash columns speak because they list, and dash and ksh93 speak because they have no `declare`, so `spoke` covers two different reasons and the case is about the one column that does neither. Emptiness rather than a byte count: a command substitution strips trailing newlines, so this asks whether anything was written without the answer depending on how long the shell's own path is on the machine that ran it",
	},
	{
		ID: "harness/the-shells-own-functions-are-not-the-persons", Category: "harness invocation",
		Snippet: "f() { :; }\n" +
			"declare -F 2>/dev/null | grep -c \"dirs\\|popd\\|pushd\"\n" +
			"declare -F 2>/dev/null | grep -c \"^declare -f f$\"\n" +
			`echo "st=$?"`,
		Why: "the row above narrowed to two counts, because a listing grades everything already defined and this grades a slice the case sets itself. The first count is unanimous at 0 in all six: no shell in the panel names its own `dirs`, `popd` or `pushd` in a function listing, because in the two that have them at all they are builtins. The second splits with the first row's split — the three bash columns list the person's `f` and count 1 at 0, and the other three list nothing, count 0 and hand `grep`'s 1 on. Ours counted 3 on the first line: a dialect written as a prelude has those three as *functions*, and a state capture recorded them and `__dirs_rotate` as the person's own, then sourced them into a shell that already had them (#1035). The counts survive a prelude growing a function, which the whole listing does not",
	},
	{
		ID: "harness/asking-for-the-whole-option-table", Category: "harness invocation",
		Snippet: `shopt -p >/dev/null; echo "st=$?"`,
		Why:     "the shell-options half of the same capture, and the half where failing is loud: `shopt` is bash's alone, so dash, ksh93 and zsh answer 127 in three different wordings while the three bash columns write the table and exit 0. Redirected away because the table is a hundred lines of whatever this build's defaults are; what is pinned is the status and the complaint",
	},
	{
		ID: "harness/the-option-table-is-re-inputtable", Category: "harness invocation",
		Snippet: `shopt -s nullglob 2>/dev/null; shopt -p 2>/dev/null | grep -c "^shopt -s nullglob$"`,
		Why:     "the property the capture depends on and the one nothing else in the corpus asks for: what `shopt -p` prints has to be a command that sets the option back. The three bash columns write the line exactly and count 1; the other three print nothing, so `grep -c` counts 0 and the pipeline's status is grep's 1. A shell that listed the option in any other layout would count 0 here while still answering 0 to the row above",
	},
}
