# Parameter expansion forms

What the operators inside `${ … }` mean. `substitutions.md` says where the
braces end; this says what is between them.

Citation: POSIX.1-2024 XCU §2.6.2. Panel measurements as in `../oracle.md`.

## One rule explains the four conditionals

    ${x:-word}   ${x:=word}   ${x:?word}   ${x:+word}
    ${x-word}    ${x=word}    ${x?word}    ${x+word}

**The colon extends the test from "unset" to "unset or empty."** That is
the whole difference between the two rows, and it is unanimous:

| probe | unset | empty | set to `S` |
| --- | --- | --- | --- |
| `${x:-D}` | `D` | `D` | `S` |
| `${x-D}` | `D` | *(empty)* | `S` |
| `${x:+A}` | *(empty)* | *(empty)* | `A` |
| `${x+A}` | *(empty)* | `A` | `A` |

The four operators then differ only in what they do when the test fires:

| operator | when the test fires |
| --- | --- |
| `-` | substitute the word |
| `=` | assign the word to the variable, then substitute it |
| `?` | write the word as an error and exit |
| `+` | substitute the word when the test does **not** fire |

`${u:=V}` leaves `u` set to `V` afterwards, so it is an expansion with a
side effect — the only one in this document.

### Whether `$@` with nothing in it is set

The table above says the colon-less row tests "unset", and there is one
parameter the panel disagrees about the set-ness of:
`Semantics.PositionalListWithNoneIsSet`.

Measured 2026-09-12, `-c` and a script file, after `set --`:

| probe | dash, zsh 5.9.2 | bash 5.3/as-`sh`/3.2, ksh93u+ |
| --- | --- | --- |
| `"${@-word}"` | *(empty)* | `word` |
| `"${@+word}"` | `word` | *(empty)* |
| `"${*-word}"` | *(empty)* | `word` |
| `"${@?}"` | *(empty)*, status 0 | `@: parameter not set` |
| `${@=abc}` | *(empty)*, status 0 | the operator fires, and is refused |

Two columns call the empty list set and four call it unset. `$*` splits
exactly as `$@` does, so one answer decides the family rather than one
operator.

The last row is the loudest and is two questions rather than one. Whether
the assignment **fires** is this axis; what happens once it does is
unanimous — `$@` is a name no assignment can land on, so bash says `$@:
cannot assign in this way` and ksh93 `${@=abc}: bad substitution`, which
is #1541 and reached by every dialect the moment the operator fires.

The **colon** forms are unanimous and ask nothing: `${@:-word}` is `word`
and `${@:+word}` is empty in all six, because an empty value fires the
test whichever way the set-ness reads. The one place the colon form still
depends on the answer is the *sentence* `${@:?}` writes, which names the
null where the list is set and says "not set" where it is not.

`${1-word}` is `word` in every column, so this is about the list and not
about a positional parameter that is not there (#1941).

## The word is itself a word

It is expanded, not taken literally:

    unset u; d=DEF
    ${u:-$d}            →  DEF
    ${u:-$(echo sub)}   →  sub
    ${u:-a b}           →  a b

So the AST cannot store it as a string. It is a nested word, which is why
`substitutions.md` keeps the inner text raw rather than flattening it:
whatever parses `${ }` has to parse a word inside it.

### But it is not a command, so no `((` in one is arithmetic

An operand stands where an operand stands, not where a command may begin,
so the two parentheses that open an arithmetic command are ordinary
characters in one. Measured 2026-09-07, **unanimous** across bash 5.3.15,
bash 3.2.57, that build invoked as `sh`, dash, ksh93u+ and zsh 5.9.2:

| probe | result |
| --- | --- |
| `u=; x=${u:-((a))}; echo "[$x]"` | `[((a))]` |
| `u=; y=${u:-$((1+2))}; echo "[$y]"` | `[3]` |

The second row is the control: `$((` in the same position still evaluates,
so the rule is about where a *command* may begin and not about arithmetic
being unavailable in an operand.

This is one of the three positions that suspend the arithmetic command,
and the other two are in `conditions.md` and `commands.md`: inside `[[ ]]`
and at the start of a `case` arm, `((` is two grouping parentheses for the
same reason. All three are the same sentence — no command begins there.

It matters more than a curiosity because reading `((` as an arithmetic
command is **lossy**: what such a reading keeps is the expression, so the
operand comes back with the two opening parentheses and up to two closing
ones removed. This implementation read one that way and answered `[a]` for
the first row — the expression `a`, taken as a variable name and found
unset — and the same fault reached every *pattern* operand, where a
leading group is ordinary: `${v#((#s)a)}` was refused as
`bad pattern: #s)a`, both outer parens gone (#1408).

A double-quoted word operand never showed it, and that is not a smaller
case but a different route: see the section below — such a body is read as
double-quoted content, which has no command position in it at all. A
pattern operand is read as an unquoted word however it is written, so
`"${v#((a))}"` is affected where `"${u:-((a))}"` is not.

### Nor is a `#` in one a comment

The same sentence again, and the second thing an operand does not have: a
comment is a thing a command line has. A `#` in an operand is a character
of the word it stands in, wherever in the operand it stands. Measured
2026-09-11 from a script file under `env -i`, **unanimous** across zsh
5.9.2, bash 5.3.15, bash 3.2.57 and dash:

| probe | result |
| --- | --- |
| `u=; printf '[%s]' "${u:-a#b}"` | `[a#b]` |
| `u=; printf '[%s]' "${u:-#b}"` | `[#b]` |
| `v='a #b'; r=${v/a #'b'/Q}` | `Q` in the four with the operator |

The third row is the one that matters, and it matters because of the
*repair* rather than the skip. This implementation read the `#` as opening
a comment and consumed the rest of the operand; the parser then put the
consumed run back as raw text, so the characters reappeared and the
quoting inside them did not. Rows with nothing quoted behind the `#`
answered correctly throughout, which is what made it silent.

Where a dialect has `(#…)` glob flags the `#` lands in that position by
itself, because the flag group's `(` ends the word in front of it. So
every glob flag group in a substitution stopped matching as soon as
anything behind it was quoted or escaped — `${v/(#b)(*)'X'/Q}` asked for a
pattern holding two literal quote marks and `${v/(#b)(*)\X/Q}` for one
holding a live backslash. Neither matches, and `(#i)` and `(#m)` were just
as dead, so it was never about backreferences (#2074).

A `$( )`, `<( )` or `${ ;}` **inside** an operand is unaffected: its body
is a command line again, and the panel is unanimous the other way on it —
`u=; printf '[%s]' "${u:-$(echo hi # there)}"` is a parse failure in all
four, the `#` having swallowed the closing parenthesis.

### And it is a word in the quoting the expansion stands in

A `${ }` written inside double quotes has a body that is double-quoted
**content**, not a fresh unquoted word. What that changes is the single
quote: there it is an ordinary character, so it quotes nothing, it is not
removed, and what stands between two of them is still substituted.

Measured 2026-09-07, unanimous across bash 5.3.15, bash 3.2.57, that build
invoked as `sh`, dash, ksh93u+ and zsh 5.9.2, with `v=VAL`:

| probe | result |
| --- | --- |
| `"${u:-'$v'}"` | `'VAL'` |
| `"${u:-'$(echo hi)'}"` | `'hi'` |
| `"${u:-''}"` | `''` |
| `"${u:-'\$v'}"` | `'$v'` |
| `${u:-'$v'}` unquoted | `$v` |

The second row is the one that settles *how far* the shell looks: the
substitution inside those quotes is recognized and **performed**, rather
than scanned past to find where the body ends. The last row is what says
the rule belongs to the enclosing context and not to the body — the same
characters unquoted are an ordinary single-quoted run, quotes removed and
nothing substituted.

The same fact reaches the **parse**, and there the panel refuses as one:

    printf '[%s]' "${x:-'a$(b'}"

With the `'` an ordinary character there is nothing to close the `$(`, and
every shell in the panel fails the line — on six different wordings and
four different statuses, which is a diagnostics question rather than a
grammar one.

Two things are *not* this rule, and each is a row that a reading which
went further would get wrong.

A **double** quote in the same position does not follow it: unescaped, it
opens a run of its own and is removed, so `"${u:-"$v"}"` is `VAL` where
`"${u:-'$v'}"` is `'VAL'`. The two quote characters part company here.

A **pattern** operand does not follow it either. Its quotes quote, and are
removed: with `s=xay`, `"${s#'x'}"` trims the `x` and is `ay`, and
`"${s#'a$(b'}"` has no substitution in it to leave open. So it is the
*word* operands — `-`, `:-`, `=`, `:=`, `+`, `:+`, `?`, `:?` — that take
the enclosing quoting, and `#`, `##`, `%`, `%%`, `/`, the case operators
and a subscript that do not.

A **backslash** in the same position escapes one character double quotes
do not: the `}` that would close the expansion. It escapes it *and is
removed*, which is the half that matters — the brace has to survive as a
character of the result, and the backslash must not survive with it.

Measured 2026-09-10 with `u` unset:

| probe | result | panel |
| --- | --- | --- |
| `"${u-A\}B}"` | `A}B` | five of six; bash 3.2 answers `A\}B` |
| `"${u-A\{B}"` | `A\{B` | unanimous — an *opening* brace keeps it |
| `"${u-A\qB}"` | `A\qB` | unanimous — so does an ordinary character |
| `"A\}B"` | `A\}B` | unanimous — and so does the same text with no expansion around it |
| `${u-A\}B}` unquoted | `A}B` | unanimous, by the ordinary word rule |

So the set a backslash escapes inside a quoted operand is the
double-quote set — `$`, a backtick, `"`, itself — **plus `}`**, and the
last three rows are what say it is that brace and not braces, not
backslashes at large, and not double quotes generally. bash 3.2 is the
lone holdout on the first row and has no dialect here to answer for it.

It reaches both operands of a substitution, and the freed brace is a
literal on either side: `"${v/x/A\}B}"` and `"${v//x/A\}B}"` are `A}B`,
and with `w='a}b'`, `"${w/a\}b/Z}"` matches and is `Z`.

Where it *stops* is a `"` written inside the operand, which opens a run
of its own — and that is the one corner of this the panel splits on.
`"${u-"A\}B"}"` is `A\}B` in zsh and `A}B` in the other five, so the
brace is escapable inside a nested run for everyone but zsh. Not modeled
yet; see #2001.

The rule matters because text is built this way and read back. The
`${(e)}` pattern powerlevel10k assembles is a run of `${NAME-<sep>\}`
entries, and a kept backslash makes the assembled text refuse to parse —
`closing brace expected` — so the prompt never installs (#1966, #1927).

A here-document body is the same context reached by the other road: it
expands as a double-quoted string does, so `${u:-'$v'}` in one is `'VAL'`
in all six.

One consequence worth writing down, because it looked like a rule of its
own: a **process substitution** in a `${ }` operand is performed in bash
only where the expansion is unquoted. `p=${u:-<(:)}` is a path there and
`p="${u:-<(:)}"` is the five characters `<(:)`, in 5.3 and 3.2 alike —
which is not a second answer about operands but this one, since a
process substitution is no more written inside double quotes here than
anywhere else.

### A nested `${ }` in a quoted run brings its own quoting

A `"` written inside an operand opens a run of its own, and an expansion
written **inside that run** is quoted the same way a substitution is: its
own `"` are its, not the run's closing one.

Measured 2026-09-11 from a script file under `env -i`, **unanimous** across
zsh 5.9.2, bash 5.3.15, that build invoked as `sh`, bash 3.2.57, dash and
ksh93, with `u` and `w` unset:

| probe | result |
| --- | --- |
| `"${u:-"${w:-ab}z"}"` | `abz` |
| `"${u:-"${w:-"X{039}"}z"}"` | `X{039}z` |
| `"${u:-"${w:-"a}b"}z"}"` | `a}bz` |
| `"${u:-"${w:-"$( echo q )"}z"}"` | `qz` |

The first row is the control and the last is the `$( )` spelling of the
same nesting, which has its own entry above; the middle two are what the
rule is for.

Reading the run as text does not merely mispair the quotes, it moves the
**braces**. The nested `${` is consumed inside the quote and never
counted, and its `}` then lands outside and closes the *enclosing*
expansion — so `"${u:-"${w:-"X{039}"}z"}"` ended at the `}` of `X{039}`
and `"}z"}` fell out as literal characters. In a word the leftover quote
usually unbalances something and the line is refused; in a here-document
body, and in the `${(%%)…}` that re-reads a prompt, there is no enclosing
word to unbalance, so the same misreading writes the wrong text and exits
0. powerlevel10k's directory segment is
`${P9K_CONTENT::="…${:-"%B%F{039}"}…"}`, and the prompt drew the `"}`,
`}+}` and `}}}+}` tails of the expansions closed early, with the leading
path components gone (#2092).

The run is still a run once the reader is inside it, so the nested
expansion is double-quoted **content**: a bare `{` in it opens no level,
by the rule above. In a here-document body, where a leftover quote
unbalances nothing,

    [${u:-"${w:-a{b}c"}z"}]

is `[a{bcz"}]` in zsh 5.9.2, bash 5.3.15, that build as `sh`, bash 3.2.57
and dash, the nested `${w:-a{b}` having ended at the first `}`. ksh93
refuses the line — it is the panel member that balances a bare brace
hardest — and no dialect here answers for it.

What the step-over does **not** decide is what quoting means once it is
inside: a single quote in a double-quoted operand is still an ordinary
character by the rule above, so `"${x:-"${y:-'"'}"}"` is still refused
by five of the six and still refused here.

### Where a word operand is matched

A word operand is part of the word it is written in, and it is matched
there — once, at the end, with whatever surrounds it. With files `Xay`
and `Xby` present, all six give two fields for

    printf '[%s]' X${u:-[a-b]}y

and the pattern they matched is `X[a-b]y`, not `[a-b]`. Matching the
operand on its own is a different question with a different answer: it
looks for a file named `[a-b]`, finds none, and in zsh — where an
unmatched pattern is fatal — stops the command that would otherwise have
printed both files.

Quoting inside the operand is the operand's own, and it reaches that
match intact: `${u:-"X[a-b]y"}` and `${u:+"X[a-b]y"}` are the seven
characters in all six, where `${u:-X[a-b]y}` is the two files. Losing it
is how a color sequence becomes a pattern — `ESC [` opens every one of
them, so an operand carrying a color is an unterminated bracket
expression the moment its quotes stop counting, and zsh refuses that
outright rather than passing it through.

The **assigning** operators are the exception that shows the rule from
the other side. `${u:=X[a-b]y}` puts those seven characters in `u` in all
six: the operand is not matched on its way into the parameter. What the
expansion then comes to is the ordinary question about an expansion's
result — bash, bash-as-sh, bash 3.2, dash and ksh93 read the stored value
back as a pattern and print both files, zsh prints the seven characters —
and that is `GlobExpansionResults` from `semantics.md`, not a rule of the
operator's.

The word `${x?word}` complains with goes the same way as an assigning
operator's, and for a plainer reason: it is a diagnostic. With `a.b`
present, `unset u; echo ${u?a.[a-c]}` says `u: a.[a-c]` in zsh, bash,
dash and ksh93 alike.

The `${~spec}` flag overrides the first of these and not the second.
`${~u:-"X[a-b]y"}` matches despite the quotes, because the flag decides
whether the result reads as a pattern; `${~~u:-X[a-b]y}` still matches,
because the operand's own metacharacters were written rather than
substituted and were never the flag's to switch off.

## Pattern removal

    ${x#pat}   shortest matching prefix removed
    ${x##pat}  longest matching prefix removed
    ${x%pat}   shortest matching suffix removed
    ${x%%pat}  longest matching suffix removed

Measured with `p=a.b.c`, unanimous:

| probe | result |
| --- | --- |
| `${p#*.}` | `b.c` |
| `${p##*.}` | `c` |
| `${p%.*}` | `a.b` |
| `${p%%.*}` | `a` |
| `${p#x}` | `a.b.c` — a pattern that does not match removes nothing |

The patterns are **glob patterns, not regular expressions**: `${p#[ab]}`
and `${p#?}` both yield `.b.c`, because a bracket expression and a `?`
each match exactly one character — the leading `a` — and neither repeats.
Doubling the operator is what selects the longer match; there is no
greediness syntax inside the pattern.

## Length, and where it diverges

`${#x}` is the length of the value — `x=abcd` gives 4, unanimously.

**In what unit** is not unanimous, and is not a property of the shell
alone:

| probe | `LC_ALL=C` | `LC_ALL=C.UTF-8` |
| --- | --- | --- |
| `s=héllo; ${#s}` | 6 everywhere | 5, and 6 in dash |
| `s=日本語; ${#s}` | 9 everywhere | 3, and 9 in dash |

So the length is what the locale's encoding calls a character, and dash
is the one member of the panel with no multibyte decoder to consult it
with. Semantics axis `MultibyteEncodingIsHonored`; the locale itself is
runtime state read off the runner rather than a second axis, and
`docs/spec/semantics.md` has the precedence, the codesets and the
undecodable byte.

`${s:off:len}` and a subscript on a scalar count the same unit, which is
the part a partial fix gets wrong: an offset in bytes against a length in
characters lands in the middle of a character. Pattern matching does
*not* follow yet (#905).

`${#@}` and `${#*}` are not unanimous:

| probe, after `set -- p q r` | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `${#@}` | **5** | 3 | 3 | 3 |
| `${#*}` | **5** | 3 | 3 | 3 |

dash gives the length of the joined string `p q r`; the others give the
number of positional parameters. This is the first axis measured where
dash stands alone, and it is a silent one: both answers are plausible
numbers and neither errors.

Semantics axis: `LengthOfSpecialIsCount` — dash no, bash, ksh93 and zsh
yes. This is one of the two axes `CoreSemantics()` answers, and it says
yes: the core panel is bash, ksh93 and zsh, and dash is the shell the
core was drawn to exclude.

### A `#` is the prefix, or it is the parameter `$#`

The same character does both jobs, and what decides it is what stands
behind it. `${#v}` is a length and `${#=w}` is the parameter `$#` with a
default assignment on it — never firing, since `$#` is always set — so
the word is not substituted at all.

The two readings are not separated by anything as tidy as "an operator
follows", because `-`, `?`, `@`, `*`, `$` and a digit are all *names*.
What can never begin a name leaves the `#` as the parameter. Measured
2026-09-07 with `set -- p q`, so `$#` is 2:

| probe | all six | reading |
| --- | --- | --- |
| `${#=w}` | `2` | the parameter, `=w` never fires |
| `${#:=w}` | `2` | the same |
| `${#+w}` | `w` | the parameter, and its alternate does fire |
| `${#:+w}` | `w` | |
| `${#:?w}` | `2` | |
| `${##2}` | `` | the parameter, with `2` trimmed off its front |
| `${#%2}` | `` | and off its back |
| `${#/2/X}` | `X` | dash has no operator and refuses |
| `${#:0:1}` | `2` | dash has none and refuses |
| `${##}` | `1` | the **length** of `$#` |
| `${#?}` | `1` | the length of `$?` |
| `${#-}` | varies | the length of `$-`, which differs by shell |

`${##}` against `${##2}` is the pair worth holding on to: the same two
characters, resolved one way with an operand behind them and the other
way without, and unanimously. So the trim reading requires an operand,
and a bare `${#%}` is a bad substitution in five of the six for the same
reason — no operand for the trim and no name for the length.

Three shapes were open, and each is a **parse** divergence rather than a
value one:

| probe | five shells | zsh 5.9.2 |
| --- | --- | --- |
| `${#-w}` | `2` | `bad substitution` |
| `${#?w}` | `2` | `bad substitution` |
| `${#:-w}` | `2` | `1` |

zsh reads the first two as a length over `$-` and `$?` with a stray word
after them, and the third as the length of the nameless `${:-w}` — an
expansion with no name at all.

The third is now answered, by `NamelessParamExpansion`: with the nameless
form in the grammar the `#` is the length prefix and there is something
for it to be the length *of*, and without the form the `#` is the
parameter `$#` with a default that never fires. One flag, both answers,
and it is the only place the two readings of a leading `#` are separated
by nothing else — `${#:+w}` is `w` and `${#:=w}` and `${#:?w}` are `2` in
all six, so the exception is exactly one operator wide. See "An expansion
with no name at all" below.

The first two are still open. Separating them needs a **backtracking**
parse rather than a lookahead — `${#-}` is a length and `${#-w}` is the
parameter, so the reading is decided by what fails rather than by what
follows — and this implementation refuses both, which is zsh's answer.
Taking the five-shell side would replace one shell's loud refusal with a
plausible number, so it is still filed rather than guessed.

### An expansion with no name at all

zsh alone reads `${` with no parameter in front of the operator. The name
that is not there is never set and never non-empty, so the conditionals
answer from that rather than from a value. Measured 2026-09-08 across the
six-column panel, where the other five call every one of these a bad
substitution:

| probe | zsh 5.9.2 | reading |
| --- | --- | --- |
| `${:-abc}` | `abc` | the default always fires |
| `${:+abc}` | `` | and the alternate never does |
| `${}` | `` | nothing between the braces is the empty string |
| `${:-}` | `` | and so is an empty operand |
| `${%x}` | `` | a trim over the nothing in front of it |
| `${:-x${v}y}` | `xqy` | the operand is an ordinary word, with `v=q` |
| `${:-${:-a}}` | `a` | and one of these nests inside another |
| `${:=abc}` | `not an identifier: ` | nothing to assign to, and fatal |
| `${:?abc}` | `: abc` | unset, so the report always fires |

Grammar flag: `NamelessParamExpansion` — zsh yes, everyone else no. A
grammar flag rather than a semantics axis for the reason
`NestedParamExpansion` is one: without it there is no parameter at the
front of `${:-abc}` at all, so the expansion is unreadable rather than
differently read, and there is nothing for a value to switch between.

**The flag group does not decide it.** `${(%):-%x}` reading while
`${:-%x}` did not was this implementation's bug, not zsh's grammar: the
group is what renders the result — `${(U):-abc}` is `ABC`, `${(q):-a b}`
is `a\ b`, `${(s.,.):-a,b}` is two fields — and the reading is the same
with it and without it. A `~`, `=` or `^` run relaxes the name the same
way and for the same reason, and none of them is the reason the name may
be absent.

**A character that is a parameter is not a missing name.** This is the
trap the form sets, and it is where widening the guard would have gone
wrong. `-`, `?` and `#` are taken as names before the operator scan runs,
so the nameless reading never competes for them:

| probe | all six | reading |
| --- | --- | --- |
| `${-}` | the option letters | `$-`, in every shell |
| `${-x}` | `bad substitution` | `$-` with a stray word after it |
| `${?x}` | `bad substitution` | and the same over `$?` |
| `${#}` | the count | `$#`, in every shell |

`${-x}` is two characters from `${:-x}` and is nothing like it. Asking
only *whether* those error cannot tell the two readings apart, because
both error; `${-}` is where they differ, and it is silent — the parameter
reading answers the option letters and a nameless one would answer the
empty string, both at status 0.

`~/.zi/bin/zi.zsh` writes the form on the line that builds the argv of
every non-zsh plugin — `${(s: :):-${${:-${(@s: :):--o}" "${(s:
:)^ICE[opts]}}:#-o }}` — which is why it is a daily-driver blocker rather
than a corner (#1529).

**What this implementation does not match.** `${@:=w}` and `${*:=w}` are
refused by all six — four wordings and two statuses — and `${1:=w}` is
refused by five and *assigns* in zsh. This implementation lets all three
through silently, which predates the nameless form and is #1541; the
empty name goes through the same door `${::=w}` uses and is refused.

## Extensions

None of these are POSIX, and they do not all arrive together.

| form | meaning | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `${x/pat/rep}` | replace first match | **no** | yes | yes | yes |
| `${x//pat/rep}` | replace every match | **no** | yes | yes | yes |
| `${x/#pat/rep}` | replace an anchored prefix | **no** | yes | yes | yes |
| `${x/%pat/rep}` | replace an anchored suffix | **no** | yes | yes | yes |
| `${x:off:len}` | substring | **no** | yes | yes | yes |
| `${x^^}` `${x,,}` | upper- and lower-case | **no** | **yes** | **no** | **no** |
| `${!x}` | indirection | **no** | **yes** | *other* | **no** |
| `${a[i]}` | array element | **no** | yes | yes | yes |

Two rows deserve their own note.

**`${x^^}` is bash alone.** ksh93 reports a syntax error and zsh a bad
substitution. `shell-matrix.md` already recorded it as bash-4-only; this
places it in the bash dialect rather than the core.

Case change has a third direction: **`${x~}` toggles the first
character's case and `${x~~}` toggles every character's** — `aBc` to
`ABc` and `AbC` — and each operator takes a pattern saying which
characters to touch, exactly as `^` and `,` do: `${x~~[ab]}` on `aBc`
is `ABc`, the pattern matched case-sensitively against each character.
bash 5.3 alone; bash 3.2, dash and zsh call it a bad substitution and
ksh93 refuses it while reading (measured:
`param/case-toggle-is-newer-bash-still`). It rides the same dialect
flag as `^^` and `,,` — one shell, one family of operators, one flag.

**`${!x}` diverges four ways.** With `x=y` and `y=V`, bash yields `V`,
dash and zsh reject it, and **ksh93 yields `x`** — it is not an error
there, it means something else. That is the `&>` failure mode inside an
expansion, and the third measured instance of it.

`${a[i]}` inherits the array-base axis from `semantics.md`: with
`a=(p q r)`, `${a[1]}` is `q` in bash and ksh93 and `p` in zsh.

### A replacement is text, and the pattern beside it is a pattern

The two operands of `${x/pat/rep}` are read differently, and the second
one is the half with no metacharacters in it at all. Measured 2026-09-07
across bash 5.3.15, that build as `sh`, bash 3.2.57, ksh93u+ and zsh
5.9.2, in a directory holding files named `axcd` and `aQcd` so that a
pattern would have something to find — and asserting **what lands in the
variable**, so that the surrounding field does not answer first:

    x=abcd
    y=${x//b/*}     →  a*cd      in all five
    y=${x//b/[Q]}   →  a[Q]cd    in all five
    y=${x//b/?}     →  a?cd      in all five

The replacement is still *expanded* — it is a word, like every other
operand — and what an expansion produces is text rather than more
syntax: with `r="p q"`, `y=${x/b/$r}` is `ap qcd` in all five, one field
and one space.

What happens to those characters **after** they land is the ordinary rule
for an expansion's result and not the replacement's own, which is why the
unquoted uses split the panel where the assignments do not:

| probe | bash, `sh`, bash 3.2, ksh93 | zsh |
| --- | --- | --- |
| `printf "[%s]" ${x//b/*}` | `[aQcd][axcd]` | `[a*cd]` |
| `printf "[%s]" "${x//b/*}"` | `[a*cd]` | `[a*cd]` |
| `r="p q"; printf "[%s]" ${x/b/$r}` | `[ap][qcd]` | `[ap qcd]` |

Both columns follow from axes already recorded: `GlobExpansionResults`
says whether an unquoted expansion's result is re-read as a pattern, and
`SplitParamExpansion` whether it is split. Neither of them is a question
about the replacement, and the quoted row is the control that says so —
one field, four characters, everywhere.

**An assignment cannot be used to check this here**, which is worth
writing down because it is the obvious probe and it does not work: an
assignment's value is expanded with pathname expansion suspended for the
*whole word*, nested operands included, so a replacement stops globbing
in that position for a reason of its own. The rows above are facts about
the panel, where the fault does not exist; a row asking the same question
of this implementation has to use a **quoted** expansion, which switches
off the result's globbing and nothing else.

Reading the replacement as a pattern is a **silent** wrong answer in the
shape this document treats as the worst: `${x//b/*}` substituted a
directory listing into the middle of a string at status 0, and the
bracket expression that made it noticeable was the same fault reaching a
dialect where an unmatched pattern is fatal (#1337).

### The scan stops where the last match ended

`${x//pat/rep}` walks the value looking for somewhere else to match, and
**a match that ended at the end of the value leaves nowhere left**. The
empty match waiting at the final position is not a second match.

It is only visible for a pattern that can match empty at all, which is
why `*` is the shape that shows it. Measured 2026-09-07 on bash 5.3.15,
ksh93u+ and zsh 5.9.2 — dash has no such operator:

    v=abcd
    ${v//*/X}   →  X      in all three
    ${v//b*/X}  →  aX     in all three
    ${v//?/X}   →  XXXX   in all three

The second row is what says this is the end of the scan rather than the
`*` being special: the `b` means the scan reaches the end from the
middle, and one X is still the answer. The third is the control for the
other direction — a pattern that never reaches the end is replaced at
every position, so a rule that ended the scan too early would show up
there as a single X.

This implementation replaced twice for the first row, because the value's
end was offered to the matcher again after `*` had consumed the whole of
it and an empty match was found there (#1341).

The panel does not agree about every empty match, only about this one.
A pattern that can match empty *without* reaching the end parts them, and
that split is `Semantics.ReplacementEmptyMatchDeclined` — see **which
empty match a replacement declines** below. Both readings agree on the
rows above, which is the whole of what this section claims.

### The end of the value is a position, but not after an empty match

The rule above is about a match that *reached* the end. There is a second
question underneath it: whether the end of the value is a position a match
may **begin** at, once the scan has arrived there with nothing consumed.

It is, and it is not, and which one depends on the step that got there.
Measured 2026-09-12 on zsh 5.9.2 with `extended_glob` and `v=abc`, because
`(#e)` is the only spelling in the panel that can match empty at the end and
nowhere else:

    ${v//x#/-}            →  -a-b-c     and not -a-b-c-
    ${v//c#/-}            →  -a-b-      the final - took the c
    ${v//(#e)/-}          →  abc-       nothing matched before it
    ${v//(x#|(#e))/-}     →  -a-b-c     the arm that could fire does not
    ${v//((#s)|(#e))/-}   →  -abc-      arrived after a failed match

So the end is offered to the matcher when the scan reached it by stepping
over a unit **no match claimed**, and is not when it reached it by the step
an empty match takes to make progress. A rule written about the position
rather than about the step gets one of the first and third rows wrong
whichever way it is stated, and this implementation had the first: it left
`-a-b-c-`, one replacement more than the shell makes.

A value with no units at all has no preceding step, and keeps its one
match: the empty string under `${e//x#/-}` is `-`.

The `(#e)` row is one shell's answer, since no grammar in the panel but
zsh's has an end anchor. The *step* rule it illustrates is not: an
alternation with an empty arm reaches the same two positions in bash and
ksh93, and those two shells answer them differently. That is the axis in
the next section.

### Which empty match a replacement declines

Once a pattern can match empty at all, there are two positions the rules
above leave open — the end of the value stepped onto after an empty match,
and the position an earlier match ended at — and the panel splits two ways
over them. `Semantics.ReplacementEmptyMatchDeclined` is the answer.

Measured 2026-09-12 from a script file, `v=abc`, the replacement written
`<>` so that a position matched twice is visible, and the pattern "empty
or one letter" spelled `@(b|)` with `extglob` on and `b#` under
`extended_glob`:

    pattern      bash 5.3.15   zsh 5.9.2   ksh93u+
    empty-or-b   <>a<><>c      <>a<><>c    <>a<>c<>
    empty-or-x   <>a<>b<>c     <>a<>b<>c   <>a<>b<>c<>
    empty-or-c   <>a<>b<>      <>a<>b<>    <>a<>b<>
    empty-or-a   <><>b<>c      <><>b<>c    <>b<>c<>

Row one is the discriminator: ksh93 has no `<>` between `b` and `c`, where
the other two have one, and has one after `c`, where they have none. So
bash and zsh **decline the empty match at the end of the value** and take
one adjacent to the match before it; ksh93 does the opposite, which is the
classic global-replace rule — a replacement never happens twice in the same
place.

Row three is the control both readings answer alike, and it must stay that
way: `c` matches at the last unit, so the scan ends there under either rule.
It is the unanimous rule above, reached from a third direction.

The axis is asked at those two positions and nowhere else. A pattern that
cannot match empty reaches neither, which is why `${v//b/X}` needs no answer
from anyone (#1859).

bash 3.2 answers all four rows `<><><>` and is a column of its own; there is
no preset for it here.

### An empty pattern

A pattern that is *empty* is a separate question from an empty match, and
the panel answers it three ways.
`Semantics.EmptyReplacementPattern` carries them.

Measured 2026-09-12 from a script file, `v=abc` and `e=`:

                    bash 5.3.15   ksh93u+   zsh 5.9.2
    ${v///X}        abc           abc       XaXbXc
    ${v//$e/X}      abc           abc       XaXbXc
    ${v/$e/X}       abc           abc       Xabc
    ${e///X}        (empty)       X         X
    ${e//x/X}       (empty)       (empty)   (empty)

So bash declines the pattern outright, ksh93 takes it only where there is
nothing left to scan, and zsh treats it as the ordinary pattern that matches
the empty string. The second row says the answer does not depend on how the
operand was written — a pattern that arrived from an expansion is the same
pattern — and the last is the control that says the `X` above it is a match
rather than something an empty value produces on its own.

It is a question about the pattern's *text* and not about empty matches: with
`extglob` on, `${v//@(|)/<>}` — a pattern with bytes in it that matches only
the empty string — is `<>a<>b<>c` in bash and `<>a<>b<>c<>` in ksh93, so
neither shell is refusing empty matches. Those two answers are the axis above,
seen again.

The anchored spellings are their own row and ask nothing: `${v/#/X}` is
`Xabc` and `${v/%/X}` is `abcX` in bash and zsh alike, and ksh93 leaves both
alone — that shell declining an anchor it takes everywhere else, which is
recorded here and not yet answered (#1857).

## The `!` that lists names instead of following one

Two more spellings open with `!` and are not indirection:

**`${!prefix*}` and `${!prefix@}` are the *names* beginning with the
prefix**, sorted, and the two suffixes differ exactly as `$*` and `$@`
do — joined into one field on IFS's first character, or one field per
name (measured: `param/the-names-with-a-prefix` and its
`-one-field-each`, `-joined` and `-are-sorted` neighbors; a prefix
matching nothing is empty with status 0, `param/no-names-with-that-prefix`).
bash and ksh93 answer alike; dash and zsh call the whole form a bad
substitution. Two shells is not a common denominator, so the form
belongs to the bash and ksh dialects and rides `ParamIndirection` —
the flag that already admits `!` after `{`.

**`${!a[@]}` and `${!a[*]}` are an array's *subscripts*, not its
elements** — with a sparse array, the subscripts actually assigned
rather than `0..n-1`. It exists to iterate an array by index, and the
corpus case is written as that loop (`param/array-indices`): bash and
ksh93 answer, zsh rejects it as a bad substitution — its arrays are
dense, so the question does not arise there — and dash has no arrays at
all. The same dialect flag governs it.

A `!` directly before an *operator* is none of these: `${!:+set}` is the
special parameter `$!` with an operator, unanimously
(`param/bang-with-an-operator-is-the-parameter`).

## Negative subscripts

A negative subscript counts back from the end, and does **not** inherit
the base axis: with `a=(one two three)`, `${a[-1]}` is `three` in bash,
ksh93 and zsh alike — zsh's positive subscripts count from 1 and its
negative ones still match the other two. Measured (oracle run,
`param/a-negative-subscript-counts-from-the-end`); bash 3.2 predates the
form and refuses it.

The same reading holds everywhere a subscript is written:

- **assignment** — `a[-1]=X` replaces the last element, unanimous
  (`param/assigning-through-a-negative-subscript`);
- **arithmetic** — `$((a[-1]))` reads and `$((a[-2]=9))` writes the same
  positions, unanimous;
- **unset** — `unset "a[-1]"` removes the last element, unanimous.

Against a **sparse** array the end is one past the highest *subscript*,
not the element count: with subscripts 0 and 5, `${a[-1]}` is the element
at 5 and `${a[-2]}` is the unassigned 4 — nothing — in bash and ksh93
(measured; zsh's arrays are dense, so the two ends never differ there).

An **associative** subscript is a key, never a position: `m[-1]` on a
declared name stores and reads the two characters `-1`, unanimous in the
three that declare them.

Out of range past the start the shells disagree three ways — bash warns
`bad array subscript` and expands to nothing (fatally, for a write), zsh
is silent (and *prepends* on a write), ksh93 is fatal either way. This
implementation reads nothing, and refuses the write with the same
diagnostic any below-base subscript gets; the divergence is recorded
rather than modeled, because none of it is behavior a script can rely on
across shells.

## What a subscript *means* — three readings that diverge

The bracket parses the same way in every shell that has subscripts at
all. What is inside it does not mean the same thing, and the three
divergences below are all one evaluator. Measured 2026-09-05 on zsh
5.9.2, bash 5.3.15, bash 3.2.57, bash as `sh`, ksh93 and dash.

### A comma: a range, or the arithmetic operator

| probe | zsh | bash 5.3 | bash 3.2 | bash-as-sh | ksh93 | dash |
| --- | --- | --- | --- | --- | --- | --- |
| `a=(w x y z); "[${a[1,3]}]"` | `[w x y]` | `[z]` | `[z]` | `[z]` | `[z]` | no arrays |
| `"[${a[0,2]}]"` | `[w x]` | `[y]` | `[y]` | `[y]` | `[y]` | — |
| `"[${a[1,2,3]}]"` | `bad substitution` (1) | `[z]` | `[z]` | `[z]` | `[z]` | — |

A subscript is an arithmetic expression, and arithmetic has a comma
operator whose value is its right operand — so `${a[1,3]}` is `${a[3]}`
in four of the five shells that read it. In zsh the comma separates the
two ends of a **range**, and a third part is not a wider range but a bad
substitution.

Both readings answer, neither reports, and the values are different: a
script cannot tell which shell it is on except by what it gets. That is
a conflict rather than an addition, so it is the semantics axis
`SubscriptCommaIsARange` and not a grammar flag. The core answers *no*,
which is both the standard's reading — POSIX has the comma operator and
no ranges — and four of the five shells'.

The axis is asked only where the two readings differ. `${a[2,2]}` is the
second element either way and needs no answer, which is what keeps a
core that has chosen no dialect from refusing a subscript both shells
agree about.

The endpoints, measured with `a=(w x y z)` and `s=hello`:

| probe | array | string |
| --- | --- | --- |
| `[2,-1]` — a negative end | `x y z` | — |
| `[-3,-2]` — both negative | `x y` | — |
| `[0,2]` — below the first | `w x` | `he` |
| `[2,99]` — past the last | `x y z` | `llo` (from `[3,99]`) |
| `[3,2]` — backwards | empty | empty |
| `[1,0]` — an end before the first | empty | empty |
| `[-5,2]` / `[-6,2]` — a start past the start | **empty** | **`he`** |

Endpoints are subscripts, so they take the base and the count-back-from-
the-end reading a single subscript takes. The last row is the one no
symmetry predicts and it is a fact rather than a derivation: a start
counted back past the *first element* leaves an array empty, where the
same reach past the first *character* of a string is clamped to it.

#### A start outside the array names one empty element

"Empty" above is the *value*; the element **count** is not always zero.
Measured 2026-09-12 on zsh 5.9.2 over the whole grid of endpoints for
arrays of five, two, one and no elements, with `a=(1 2 3 4 5)`:

| probe | `${#a[lo,hi]}` |
| --- | --- |
| `[6,7]` — a start past the end, ends apart | `1` |
| `[6,6]` — the same start, equal ends | `0` |
| `[7,9]` / `[9,9]` — the same pair further out | `1` / `0` |
| `[-8,-8]` — a start below the array, equal ends | `1` |
| `[-6,-8]` — the same start, end below it | `0` |
| `[-6,0]` / `[0,0]` — a negative and the literal it normalizes to | `1` / `0` |
| `[6,7]` on `s=hello` — a character range | `0` |

So two blocks answer with **one empty element** rather than with no
element at all, and the same count comes back through `(@)`:
`set -- "${(@)a[6,7]}"` leaves one parameter. Above the array the ends
must differ *strictly*, and it is the strictness rather than the distance
— `[7,9]` is 1 where `[9,9]` is 0. Below it, equal ends still count, so
the two edges are not mirror images.

The two are told apart by how `lo` was **written** and not by where it
landed: `-6` and `0` normalize onto the same position on five elements
and answer differently. A character range does neither, so this is the
element reading's rule alone.

One cell is not reproduced here, and it is recorded rather than fitted:
on an array with no elements `${#a[0,0]}` is `1` there while `${#a[0,1]}`
beside it is `0` and `${#a[0,2]}` is `1` again. A rule non-monotonic in
`hi` is the shell's own arithmetic showing through rather than a
statement about ranges.

### A subscript on a plain string

| probe, `s=hello` | zsh | bash 5.3 | bash 3.2 | bash-as-sh | ksh93 | dash |
| --- | --- | --- | --- | --- | --- | --- |
| `"[${s[0]}]"` | `[]` | `[hello]` | `[hello]` | `[hello]` | `[hello]` | `Bad substitution` |
| `"[${s[1]}]"` | `[h]` | `[]` | `[]` | `[]` | `[]` | — |
| `"[${s[2]}]"` | `[e]` | `[]` | `[]` | `[]` | `[]` | — |
| `"[${s[2,4]}]"` | `[ell]` | `[]` | `[]` | `[]` | `[]` | — |
| `"[${s[-1]}]"` | `[o]` | `[]` + `s: bad array subscript` | same | same | `[]` | — |

zsh reaches into the string's characters. Everything else reads a scalar
as an array of one, so the base names the whole string and no other
subscript names anything. Axis `ScalarSubscriptIsACharacter`; the core
answers *no*.

**Empty, not an error, is the core's answer**, and it is measured rather
than defaulted: neither bash nor ksh93 says anything about `${s[2]}` and
both expand it to nothing at status 0. The one place bash speaks is a
*negative* subscript, where it warns `s: bad array subscript` and still
expands to nothing at status 0 while ksh93 is silent. The value is
unanimous and is what this implementation gives; bash's advisory is
recorded rather than modeled, the same treatment its out-of-range
subscript warning gets above.

Characters and not bytes — where the locale says a character is more
than a byte. `s=héllo; "${s[2]}"` is `é` and `"${s[2,3]}"` is `él` under
`LC_ALL=C.UTF-8`, and under `LC_ALL=C` the same two are the single byte
`\xc3` and the pair that spells `é`, measured on zsh 5.9.2. It is the
same `MultibyteEncodingIsHonored` question `${#s}` asks, so the two
agree by construction; reading runes unconditionally answered the UTF-8
column in both.

### A subscript on a parameter that is not a name

| probe | zsh | bash 5.3 | bash 3.2 | bash-as-sh | ksh93 | dash |
| --- | --- | --- | --- | --- | --- | --- |
| `set -- a b c; "[${@[1]}]"` | `[a]` | bad subst. (1) | bad subst. (1) | bad subst. (127) | ``syntax error … `[' unexpected`` (3) | `Bad substitution` (2) |
| `"[${*[2]}]"` | `[b]` | bad subst. | bad subst. | bad subst. | syntax error | Bad substitution |
| `set -- abcd; "[${1[2]}]"` | `[b]` | bad subst. | bad subst. | bad subst. | syntax error | Bad substitution |
| `"[${0[1]}]"`, `"[${?[1]}]"`, `"[${-[1]}]"`, `"[${$[1]}]"` | the value's characters | bad subst. | bad subst. | bad subst. | syntax error | Bad substitution |

Five refusals and one reading, and nobody means something else by it —
so this is the additive kind of split, grammar flag
`SpecialParamSubscript`, beside `ArraySubscript`. It is separate from
`ArraySubscript` because it is about the *name*: `${a[1]}` is read by
four of the six and `${@[1]}` by one.

Where the flag is off the bracket is simply not consumed, and the
leftover text takes the route every unreadable expansion takes —
deferred to the run in most of the panel, refused while reading by the
one grammar that refuses everything else while reading. Nothing here
words a diagnostic of its own. Expanding it to nothing at status 0, as
this implementation did, is the silent middle answer nobody gives.

`@` and `*` are the positional parameters **as a list**; every other
special parameter supplies a *value*, so a subscript reaches into it by
character under the axis above.

`#` and `!` are unreachable in the braced spelling for the reason the
brace-less form records below: `${#[1]}` is a length and `${![1]}` an
indirection, so there is nowhere to write them down.

### `argv` is the positional parameters, as an array

One dialect gives the positional parameters a *name*, and it is a real
array rather than a synonym for `@`: read, subscripted, counted,
assigned to whole, appended to, written one element at a time and
`unset`, with every one of those reaching `$@`. Measured on zsh 5.9.2,
2026-09-10, with `set -- 'a b' c`:

    $argv               a b c   the bare name, joined exactly as `"$arr"`
    ${argv[1]}          a b     subscripts count from one, like any array
    ${argv[2]}          c
    ${#argv}            2       the element count
    "${argv[@]}"        two fields, `a b` and `c`
    ${(t)argv}          array-special
    argv[1]=zz          leaves `$1` as `zz`
    argv=(n1 n2 n3)     leaves `$#` at 3
    argv+=(d)           leaves `$#` at 4 and `$4` as `d`
    argv[5]=e           leaves `$#` at 5
    unset argv          leaves `$#` at 0
    f() { print $argv[1] }; f q w   →  q, the *frame's* parameters

The bare name joining is what says it is an array and not `@`: `"$@"`
keeps its fields where `"$argv"` does not, so the axes that decide what a
bare array name means — `ArrayScalarIsTheWholeArray` and
`ArrayNameWithoutSubscriptIsTheList` — decide this too.

The panel is unanimous the other way: `set -- "a b" c; echo "[$argv]"` is
`[a b c]` in zsh 5.9.2 and `[]` in bash 5.3, bash as `sh`, bash 3.2,
ksh93 and dash alike, so it is a dialect's parameter and not a core one.

The write half is the sharp one. Without the name a script reading
`argv` sees an ordinary unset name, and a *write* to it is worse than
nothing: `argv[1]=zz` creates a stray global that every later `$argv` in
the shell then reads — including inside a function, whose own parameters
it should have been. That is `[a b][zz]` in the bash columns above
against `[zz][q]` in zsh, and it is #1633.

The implementation is a produced array with a writer, not a stored array
seeded at each `set --`: a stored array is what a read finds first, so it
would have to be kept in step with `$@` at every one of the dozen places
the parameters move. See interp.SetDynamicArrayWriter, whose whole
contract is that a write to a produced name must have somewhere to go.

### How many fields a subscripted expansion makes

Quoting, measured on zsh with `a=(x y z)` and `set -- a b c`:

| probe | fields |
| --- | --- |
| `"${a[1,2]}"` | 1, joined with the first character of IFS — as `"$a"` is |
| `"${a[@]}"` | 3 |
| `"${@[1,3]}"` | 3 |
| `"${*[1,3]}"` | 1 |
| `"${@[*]}"` | 3 |
| `"${*[@]}"` | 3 |

A range yields a list, and whether quoting joins that list is decided
the way it is decided for the bare spellings: `@` keeps its fields and
everything else joins. Either spelling of the list is enough — `@` as
the name or `[@]` as the subscript.

`${#…}` follows the same line. It is a **count** where the subscript
named several elements — `${#a[1,2]}` on `(aa bb ccc)` is 2 where the
arithmetic reading measures the three-character element it landed on —
and a **width** where it named one value, so `${#s[2,4]}` on `hello` is
3.

**A whole-array subscript on a name that holds one string is that
string**, which is the one row of this where the panel splits. Measured
2026-09-10:

    ${#a[@]} on      a=""   b=x   h="a b"
    bash 5.3.15      1      1     1
    bash as `sh`     1      1     1
    bash 3.2.57      1      1     1
    ksh93            1      1     1
    dash             bad substitution
    zsh 5.9.2        0      1     3

The three-character row is what fixes the reading: an empty scalar
answering 0 on its own would only say "no elements", and `a b` answering
3 says the subscript reached the *value*. `${a+S}` says the name is set
in both columns, so this is about how a scalar is seen as a list rather
than about whether it exists. It is the `WholeSubscriptOnAScalarMeasuresIt`
axis, asked of the **length alone** — everything else is unanimous, and
`set -- "${h[@]}"` leaves one parameter holding `a b` in every column.

### `${#…}` over an operator counts what the operator left

The length is taken over what the operator leaves, and which measurement
it is follows the shape of what was left: a **count** where the operand
was a list and a **width** where it was one string. Only zsh builds this
node — the other four call the pairing a bad substitution — and measured
there on 2026-09-10 with `a=(one two three)`, `b=(two four)`,
`s="one two three"` and `typeset -A m=(k1 v1 k2 v2)`:

    ${#a}            3    the control: no operator
    ${#a:#one}       2    the filter drops one element
    ${#a:#*}         0    one that drops every element is zero, not one
    ${#a:/one/X}     3    the replacement keeps them all
    ${#a[@]:/one/X}  3    and so does the subscripted spelling
    ${#a[*]:#one}    2
    ${#a[1,3]:#one}  2
    ${#a:|b}         2    the difference of two lists
    ${#a#o}          3    a per-element trim is still three elements
    ${#a%%e}         3
    ${#a//e/E}       3
    ${#a:-zz}        3    the test did not fire, so this is the array
    ${#m:#v1}        1    an association counts what the filter left of
    ${#m#v}          2    its values, and a trim leaves all of them
    ${#@:#q}         2    the positional parameters are a list like any
    ${#@#p}          3    other — and an element the trim emptied still
                          counts, where a command line would drop it

against 13 for every array row but the first, which is the width of
`one two three`: the value joined rather than the list the operator
left.

The rows that stay a width are the other half of the rule:

    ${#a[1]#o}        2    a subscript naming one element is one string
    ${#s:#one}       13    and so is a scalar
    unset u; ${#u:-$a}   13
    s=""; ${#s:-$a}      13

The last two are why the **name** decides and not what the expansion came
to: a substituted word holding a whole array is still measured as text.
See #1651.

One measured curiosity is recorded and not reproduced: `"${@[0]}"` is
*zero* fields in zsh where `"${@[9]}"`, `"${a[0]}"` and `"${*[0]}"` are
all one empty field. It is the only subscript that behaves that way and
no script can depend on it.

## Slicing the whole array

`${a[@]:off}` and `${a[@]:off:len}` slice the **list**: the result is
elements, one field each, not a substring of anything joined. With
`a=("a b" c d)`, `"${a[@]:0:2}"` is the two fields `a b` and `c` —
which is the whole reason this is not `${x:off:len}` over joined text
(measured: `param/array-slice`, `param/array-slice-keeps-its-fields`).

Two facts ride along, both measured in the same cases:

- **The offset counts from 0 in every shell with arrays — zsh
  included**, whose *subscripts* count from 1. Like a negative
  subscript, the slice does not inherit the base axis.
- **A negative offset counts back from the end**, and needs the space:
  `${a[@]: -2}` is the last two elements, while `${a[@]:-2}` is the
  `:-` default operator on the whole array. The space is what keeps the
  two spellings apart, and it is why the operator table earlier reads
  the colon forms first.

A negative **length** is where the agreement stops. On a scalar, bash
and zsh stop that many characters short of the end and ksh93 answers
with nothing at all (`param/a-negative-substring-length`) — the
`SubstringNegativeLengthIsEmpty` axis, which the core leaves unanswered
and the ksh dialect answers yes. On the *list* the same probe splits
three ways (measured 2026-09-04, same panel): zsh stops short of the
end, ksh93 is empty again, and bash — whose scalar answer is
count-from-the-end — **refuses it**, `substring expression < 0`. This
implementation counts a list's negative length from the end, the answer
its scalar default already gives; bash's refusal is recorded here
rather than modeled, the same treatment as its out-of-range subscript
warning above.

The scalar `${x:off:len}` itself is in the extensions table above; this
section exists because the `[@]` subject changes what the numbers index
— elements rather than characters.

## An operator over the whole array

An operator written on `${a[@]}` — a trim, a replacement, a case change —
applies to **each element**, and each stays a field of its own. Measured
with `a=(aa ab)`, unanimous in the three with arrays
(`param/an-operator-distributes-over-the-elements`):

| probe | fields |
| --- | --- |
| `${a[@]#a}` | `a` `b` |
| `${a[@]%%a*}` | two empty fields |
| `${a[@]//a/X}` | `XX` `Xb` |

The operator's word is expanded **once**, not once per element: a command
substitution in the pattern runs a single time (measured in all three).

The star form is where they split
(`param/an-operator-on-the-star-subscript-diverges`): bash and ksh93
apply the operator to each element and join what is left on the first
character of IFS (`${a[*]#a}` on `(aa ab)` is `a b`), zsh joins first and
applies the operator to the joined string once (`a ab`). The two readings
often agree — a suffix trim that stops inside the last element, a pattern
that matches nothing — so the axis is asked only where they differ.

Semantics axis: `OperatorDistributesOverStarSubscript` (bash and ksh93
yes, zsh no; unanswered in the core).

`${#a[@]}` is untouched by all of this: it is the count, and the length
question is answered before any operator runs.

## The `@` transformation family

`${x@Q}` transforms the value rather than testing or editing it. The
operator is `@` followed by **exactly one letter** from a fixed set —
`${x@}`, `${x@QQ}` and `${x@ Q}` are all bad substitutions. **bash alone**
(measured on bash 5.3): dash and zsh report a bad substitution at run
time, and so does ksh93 — `${x@Q}: bad substitution`, status 1 — even
though its *other* unrecognized operators are refused while reading. An
`@` operator in a branch never taken is silent in all four, so the `@`
family is deferred to run time everywhere, including the one shell that
otherwise refuses at parse time.

**bash checks the letter only once it has a value.** `${u@QQ}` on an
*unset* `u` is empty at status 0 — no diagnostic at all — while the same
spelling on a set name is `[${x@QQ}]: bad substitution`, naming the whole
**word** rather than the expansion. The three shells without the family
reject both spellings alike. Two consequences worth stating: a probe that
forgets to set the variable measures nothing, and a script cannot use an
invalid letter to detect whether the shell has the family, because the
answer depends on the variable rather than on the shell.

A *list* has no value when it is empty, and an empty *string* has one:
`a=(); ${a[@]@Z}` and `set --; ${@@Z}` are both quiet at status 0, while
`u=; ${u@QQ}` is refused. Measured over four spellings on 2026-09-05.

**A bad letter is a failed expansion, not an unreadable word**, and bash
draws the line itself: `bash -c 'x=a; echo "${x@QQ}"'` exits **127**,
where `bash -c 'x=a; echo "${(q)x}"'` — a bad substitution for a
different reason, found while reading the word rather than while
expanding it — exits **1** from the same invocation. The 127 is the same
number the same shell gives an unset parameter under `set -u` and a
`${x?}`, and it is a property of the *route*: both are 1 when the program
comes from a file or from standard input. `${#u@Z}`, where the length
question refuses the operator before the family is reached, exits 1 like
any other unreadable word.

Measured on bash 5.3, all with the variable set unless said otherwise; an
unset variable yields empty for every letter, with `set -u` complaining
first as it would for `$x`:

| letter | meaning | measured |
| --- | --- | --- |
| `Q` | value quoted for reuse as input | `a b'c` → `'a b'\''c'`; empty → `''` |
| `E` | backslash escapes expanded, `$'…'` rules | `a\tb` → `a<TAB>b`; `\101` → `A` |
| `P` | prompt expansion | `\u at \w` → the user and directory; plain text unchanged |
| `A` | an assignment statement that reproduces the variable | below |
| `a` | attribute letters | `declare -irx v=1` → `irx`; no attributes → empty |
| `K` | keys and quoted values | below |
| `k` | keys and values as separate words | below |
| `L` | lower-case the whole value | `abC dEf` → `abc def` |
| `U` | upper-case the whole value | `abC dEf` → `ABC DEF` |
| `u` | upper-case the first character | `abC dEf` → `AbC dEf` |

**`@Q` picks its quoting by content.** A value with no unprintable
character is single-quoted, the quote itself spelled `'\''`, backslashes
and `$` left alone: `has\backslash` → `'has\backslash'`. Any control
character (or byte that is not character-shaped) switches the whole value
to `$'…'`: `\a \b \t \n \v \f \r` by name, ESC as `\E`, `'` as `\'`,
`\` as `\\`, `"` kept plain, anything else unprintable as three-digit
octal **per byte** — DEL is `$'\177'`, U+0085 is `$'\302\205'`.

**Whether printable multibyte text survives is a question about the
locale, not about `@Q`.** In a UTF-8 locale `café` → `'café'`; under
`LC_ALL=C`, which is what the oracle runs with (`oracle.md`), the same
value is `$'caf\303\251'`, because no byte of it is character-shaped to a
shell reading one byte at a time. **This implementation does not have a
locale** and always takes the UTF-8 reading, so it answers `'café'` where
a `LC_ALL=C` bash answers with the octal. That is a deliberate divergence
rather than a gap: a Go program decoding UTF-8 is the behavior a user of
this library gets on every machine, and the alternative is a locale
database this substrate has no other reason to carry. It is also why no
corpus row pins a multibyte `@Q` — a row would record the C-locale answer
and grade the implementation against an environment it does not model.

**Transformations distribute over a whole array.** `"${a[@]@Q}"` is one
quoted word per element and `"${a[*]@Q}"` joins them; the same holds for
`"${@@Q}"` and `"${*@Q}"` over the positional parameters, and for every
letter. An empty or unset array is zero fields, status 0.

**`@A` writes the statement that would recreate the variable.** A name
with no attributes is `x='a b'` (the value `@Q`-quoted); attributes put
`declare -irx v='1'` in front, the letters in a fixed order rather than
the order they were set — measured `a A i r t x l u`, so `declare -xril`
prints `irxl` and `declare -tirx` prints `irtx`; a name
whose value is unset drops the `='…'` half — `declare -A h` for an
associative table read without a subscript. A positional parameter has no
name to write and yields empty, while `"${@@A}"` yields the words
`set -- 'one' 't w'`, one field each. On `"${a[@]@A}"` the fields are the
words of the statement: `declare` `-a` `a=([0]="1" [1]="x y")` — the
elements double-quoted, an associative table's list with a trailing space
before the `)`.

**`@K` and `@k` list keys and values; on anything but a stored array they
are `@Q`.** `"${a[@]@K}"` is a *single* field, keys bare and values
double-quoted (`\"` and `\\` escaped, `$'…'` when control characters
force it): `0 "one" 1 "t w"` — an associative table's pairs each carry a
trailing space instead of the joining space. `"${a[@]@k}"` is the same
pairs as *separate* fields, values unquoted: `0` `one` `1` `t w`. A
scalar, a scalar read as `${x[@]}`, and the positional parameters all
answer with the `@Q` quoting and no keys at all.

**bash 3.2 does not refuse the family on an `[@]` subscript — it ignores
it.** `${x@Q}` on a scalar is a bad substitution there, as expected of a
build predating the operator, but `${a[@]@Q}`, `${a[@]@U}` and
`${a[@]@K}` all yield the array's *plain elements*, status 0. The
subscript swallows the trailing `@`-letter rather than the expansion
being rejected, so a script that means to detect the missing feature by
watching for the error gets no error and quietly wrong values instead.
Recorded by `param/transform-distributes-over-an-array` and
`param/transform-keys-and-values`.

**It is bash 3.2's alone.** ksh93 and zsh both refuse `${a[@]@Q}` — a
bad substitution at status 1, exactly as they refuse the scalar spelling
— so a dialect that reproduced the swallow reproduced the wrong shell.
This implementation did: a subscript is read before the operator is, and
an expansion the grammar had already marked unreadable still carried the
`[@]`, so the array shape answered and the refusal was never reached.

Citation: oracle runs against bash 5.3.15, ksh93u+ 2012-08-01, zsh 5.9.2
and dash on 2026-09-05. The corpus rows under `param/transform-…` pin the
panel's answers for every letter — `transform-quotes-for-reuse` (`Q`),
`transform-expands-escapes` (`E`), `transform-prompt-escapes` (`P`),
`transform-writes-an-assignment` (`A` and `a`),
`transform-keys-and-values` (`K` and `k`), `transform-case-letters`
(`L`, `U`, `u`) — plus `transform-takes-exactly-one-letter` for the
one-letter rule, `transform-distributes-over-an-array` for the
distribution, and `transform-deferred-in-a-branch-never-taken` for when
the rejection happens.

Two claims above are deliberately **not** pinned, and both for the same
reason: their answer is not a property of the shell. `@P` on `\t` or
`\w` records the clock or the directory, so the row probes `\n` and `\\`
only; and multibyte `@Q` records the locale, as above.

### What this implementation answers

All ten letters are implemented from the table above. **`@P` is the
prompt transformation**, and it is the *whole* of it rather than the
escape table alone: the value goes through the dialect's table of codes
and through parameter and command expansion, in the order that dialect
draws a prompt. Measured on bash 5.3.15, 2026-09-11:

```sh
v='[\u][\h]';  "${v@P}"   →  the user and the host
d='$(echo cmd)'; "${d@P}"  →  cmd            — the expansion runs
c='\u'; v='X${c}Y'
                 "${v@P}"  →  X\uY          — and the escapes ran first
a=(x '\u' y);   "${a[@]@P}" → three fields, the middle one the user
```

The order is `PromptStyle.ExpandBeforeEscapes`, false for bash and true
for zsh, so a code a parameter *produced* is text in the one and a code
in the other. It is the same value the prompt drawer and `PS4` read, and
the same walker: one function runs both passes for all three readers, so
a fourth cannot get half the rule.

An unset name is the empty string at status 0 — there is no escape in
nothing — and a runner told no table transforms nothing at all, which is
`PromptStyle`'s zero value and the substrate's own answer.

**An escape naming a fact a Runner has not got is refused by name**, not
drawn as a plausible number: `\!` is the history number, `\#` is how many
commands the session has run, and `\l` is the terminal's name. bash
answers `1`, `1` and `tty` for those in a non-interactive shell, so this
is a deliberate difference and it is the same one `${(%)…}` already
makes — zsh answers `0` for `%!` where this shell refuses it. One promise
across both spellings of the question was worth more than making `@P` the
one place that invents a session fact; the refusal is loud, names the
escape in the dialect's own spelling (`\!`, not `%!`), and stops the
command.

The four differences #577 recorded are settled, three as answers a
dialect gives and one as core behavior:

- **What the sentence names** is `Diagnostics.BadSubstitutionNames`, with
  three values because the panel gives three. bash names the run of the
  word that shares the expansion's quoting, with the quote characters
  off; ksh93 names the whole word exactly as written; dash and zsh name
  nothing at all, so their wordings have no verb and never reach it.
- **Whether the letter is checked on a name with no value** is
  `Semantics.TransformLetterCheckedOnlyWhenValued`, reached only by a
  grammar that *has* the family. It is an axis with one measured answer
  on purpose: making an operator's validity depend on what a variable
  holds is not a rule anything should inherit by having a `@` family, so
  the shell that does it has to say so.
- **The status of a bad letter** follows
  `Diagnostics.ExpansionFailureStatusFromCommandString`, the field the
  unset-parameter route already used, on the strength of the measurement
  above: it is the same 127-under-`-c` rule, and every other unreadable
  operator keeps the ordinary fatal status.
- **Abandoning at the first** is not an axis. Every column stops at the
  first expansion it cannot answer — for a bad substitution and for
  `set -u` alike — so the word stops at its first failing span and the
  command stops at its first failing word.

The `@`-letter and the diagnostic rows are
`param/bad-substitution-names-the-word`,
`param/bad-substitution-names-only-one-quoting`,
`param/bad-substitution-stops-at-the-first` and
`param/an-unset-name-stops-at-the-first-too`.

## Parenthesized expansion flags — zsh only

`${(flags)name}` opens with a parenthesized flag group before the parameter.
zsh alone parses it; the other three treat the whole expansion as a bad
substitution — bash and dash at run time when the expansion is reached, ksh93
while reading (`` `x}' unexpected ``), which is the same split the
`BadSubstitutionAtParseTime` flag already records. Found in the wild:
`/opt/homebrew`'s `ruby-lsp-activate.sh` opens with `${(%):-%x}`, the idiom
for "the path of the file being sourced", so a parser without the construct
refuses the file.

All measurements below are of zsh 5.9.2 (Homebrew, arm64). The zsh manual
(`zshexpn(1)`, Rules) confirms the ordering observed.

### The flags in scope, measured

| flag | meaning | probe | result |
| --- | --- | --- | --- |
| `(U)` / `(L)` | upper- / lowercase every letter | `x=abC; ${(U)x}` | `ABC` |
| `(q)` | quote with backslashes | `x='a b$c'\''; ${(q)x}` | `a\ b\$c\'` |
| `(qq)` | quote in single quotes | `x="a b"; ${(qq)x}` | `'a b'` |
| `(qqq)` | quote in double quotes | `x="a b"; ${(qqq)x}` | `"a b"` |
| `(qqqq)` | quote as `$'…'` | `x="a b"; ${(qqqq)x}` | `$'a b'` |
| `(q-)` | quote only where it is needed | `x="a b"; y=p; ${(q-)x}` / `${(q-)y}` | `'a b'` / `p` |
| `(q+)` | the same, extended | `x=$'a\001b'; ${(q+)x}` | `$'a\C-Ab'` |
| `(b)` | backslash the *pattern* metacharacters | `x='a b*c'; ${(b)x}` | `a b\*c` |
| `(f)` | split at newlines | `x=$'a\nb'; ${(f)x}` | two words `a`, `b` |
| `(s:sep:)` | split at sep | `x=a:b:c; ${(s.:.)x}` | `a`, `b`, `c` |
| `(0)` | split at NUL | `v=$'a\0b'; ${(@0)v}` | two words `a`, `b` |
| `(j:sep:)` | join with sep | `a=(x y z); ${(j.,.)a}` | `x,y,z` |
| `(@)` | keep array fields in `"…"` | `a=(x "y z" ""); "${(@)a}"` | 3 fields, empty kept |
| `(P)` | value is a further name | `y=hello; x=y; ${(P)x}` | `hello` |
| `(t)` | the *type* of the name | `w=(a b); ${(t)w}` | `array` |
| `(~)` | mark the arguments of the flags behind it | `a=(? x); [[ '?' = (${(~j.|.)a}) ]]` | true |
| `(k)` | keys of an associative array | `typeset -A m=(k1 v1); ${(k)m}` | `k1` |
| `(v)` | with `(k)`: key and value pairs | `${(kv)m}` | `k1 v1` interleaved |
| `(%)` | expand prompt `%` escapes | `${(%):-%x}` | see below |
| `(M)` | substitute what the pattern took | `v=hello; ${(M)v#h*l}` | `hel` |
| `(S)` | take the shortest match, and search for it | `s=abab; ${(S)s/*b/_}` | `_ab` |
| `(o)` / `(O)` | sort a list up / down | `a=(c a b); ${(@o)a}` | `a b c` |
| `(n)` | sort by the numbers in the words | `a=(10 9 1); ${(@n)a}` | `1 9 10` |
| `(i)` | sort with case folded away | `a=(B a); ${(@i)a}` | `a B` |
| `(-)` | as `n`, with a leading minus a sign | `b=(-1 -10); ${(@-)b}` | `-10 -1` |
| `(u)` | keep the first of each repeat | `a=(b a b); ${(@u)a}` | `b a` |
| `(a)` | order by the index, not the text | `a=(c a b); ${(@a)a}` | `c a b` |
| `(Q)` | remove one level of quoting | `v="'a b'"; ${(Q)v}` | `a b` |
| `(c)` | with `${#…}`: characters, joined | `a=(abc de f); ${(c)#a}` | `8` |
| `(w)` | with `${#…}`: words | `v="a b  c"; ${(w)#v}` | `3` |
| `(W)` | with `${#…}`: words, empties too | `v="a b  c"; ${(W)#v}` | `4` |
| `(m)` | with `${#…}`: columns rather than characters | `w=$'日本'; ${(m)#w}` | `4` |
| `(p)` | read the next flags' arguments with escapes | `a=(x y); ${(pj:\n:)a}` | `x`, newline, `y` |
| `(Z:opts:)` | split a value as a command line | `v="a b  c"; ${(Z+n+)v}` | `a`, `b`, `c` |
| `(l:n::f1::f2:)` | pad a word out to a width on the left | `v=ab; ${(l:5::-:)v}` | `---ab` |
| `(r:n::f1::f2:)` | the same on the right | `v=ab; ${(r:5::-:)v}` | `ab---` |
| `(g:opts:)` | read the value's backslash escapes | `v='a\tb'; ${(g::)v}` | a real tab |
| `(e)` | read the result again as shell text | `w=zz; v='$w'; ${(e)v}` | `zz` |

Details, each measured:

- **`(S)` is which match, not what to do with it.** Against a
  substitution it is non-greedy: `s=abab` gives `${s/*b/_}` as `_` and
  `${(S)s/*b/_}` as `_ab`, and the global spelling separates a length rule
  from a position rule — `${s//*b/_}` is `_` where `${(S)s//*b/_}` is `__`.
  It reaches the anchored forms too, which a reading that only touched the
  unanchored loop would leave alone: with `v=abcabc`, `${v/#a*b/X}` is `Xc`
  and `${(S)v/#a*b/X}` is `Xcabc`, `${v/%b*c/X}` is `aX` and
  `${(S)v/%b*c/X}` is `abcaX`.

  **Against a trim it is a substring search**, which the name does not give
  away. The vendor manual states it as a position rather than a length: `#`
  and `##` take the match that starts closest to the *start* of the value,
  `%` and `%%` the one that starts closest to the end — explicitly not the
  one that *ends* closest to it — and the operator still chooses how much at
  whichever position the search settled on. With `str=aXbXc`:

  | probe | zsh 5.9.2 | without the flag |
  | --- | --- | --- |
  | `${(S)str#X*}` | `abXc` | `aXbXc` |
  | `${(S)str##X*}` | `a` | `aXbXc` |
  | `${(S)str%X*}` | `aXbc` | `aXb` |
  | `${(S)str%%X*}` | `aXb` | `a` |

  So a trim under the flag takes a piece out of the **middle** and is no
  longer a prefix or a suffix removal — `${(S)str%%X*}` answering `aXb` is a
  match that stopped short of the end. That is why this implementation's
  trim answers with a *span*: the unflagged operators are the same span with
  one end pinned, and `(M)` reads the other side of whichever one it is, so
  `${(SM)str%%X*}` is `Xc`.

  **It composes with the written-arm reading and widens where that applies.**
  A longest *prefix* trim already takes the arm that was written first;
  under the flag a longest *suffix* trim does too, where without it the
  longest match wins in either written order. Measured with `w=abc` and
  `v=abcbc`: `${(S)w%%(b|bc)}` is `ac` and `${(S)w%%(bc|b)}` is `a`, while
  `${v%%(bc|cbc)}` and `${v%%(cbc|bc)}` are both `ab`. The position is
  settled before the arm is — `${(S)v%%(bc|cbc)}` is `abc` in both written
  orders, the match nearest the end rather than the arm written first.

  **And it reaches nothing else**, measured one operator at a time rather
  than reasoned from the name. With `str=aXbXc`, `${(S)str}`, `${(S)str:1}`,
  `${(S)str:-alt}`, `${(S)str:#a*}`, `${(S)#str}` and `${(S)str[(i)X]}` are
  each what they would be with no flag at all. The exclusion is the row
  worth naming, because it *is* a pattern operator: it is a whole-value test
  rather than a search, so there is no second match to prefer.

  One measured binding is deliberately not reproduced.
  `w=aXbXc; ${(S)w%(#b)(X*)}` fills `$match[1]` with `X` and sets
  `$mbegin[1]` and `$mend[1]` to **6** — an index past the end of a
  five-character value, disagreeing with the match reported beside it. Every
  other spelling of the pair agrees and is carried; this one is recorded as a
  measurement rather than copied.

- **The ordering flags are one step, and `u` is not a sort.** `o` and `O`
  order a list up and down, `n` reads each run of digits in a word as a
  number — `x1 x9 x10`, and `a1b a2b a10b`, so the digits need not be the
  whole word — and `i` folds case, which makes ties reachable and they
  keep the order the elements were written in. `i` sorts on its own.
  `u` keeps the first of each repeat, folds nothing, and leaves the order
  alone; with a sort beside it either order of the two agrees, and only
  `u` written alone says which runs first.

  **The locale decides what `o` means.** Under `LC_ALL=C`, which is what
  the corpus runs in, it is byte order and `(B a C b)` comes back
  `B C a b`. Under a UTF-8 locale the same shell orders by the collation
  and answers `a b B C`. This implementation is byte order in both, so a
  non-C locale's collation is a **measured gap** rather than a claim.

  **`(a)` is the sort key rather than a sort.** It orders by the
  element's own position, so ascending is the order the elements were
  written in and `O` reverses it — `a=(c a b); ${(@a)a}` is `c a b` and
  `${(@aO)a}` is `b a c`, in either spelling. And it *wins* over the
  comparisons rather than composing with them: `${(oa)a}`, `${(na)a}`
  and `${(ia)a}` are all `c a b` on that data, where a reading in which
  `o` still decided would answer `a b c`. `u` still runs ahead of it.

  So `(a)` belongs with the sorts and not with `(A)`, which changes what
  an *assignment* leaves behind and the substituted value not at all. The
  two differ only in case and in nothing else, which is the mistake this
  note exists to stop.

  Where the step sits is measured rather than read off the rule numbers,
  and it is **last of the group**: after the operator
  (`a=(zb ya); ${(@o)a#z}` is `b ya`), after the case conversion
  (`a=(B a); ${(@oU)a}` is `A B`), after splitting, after joining — so
  `${(oj.-.)a}` is `c-a-b` unsorted, one word being already in order,
  and so is `"${(o)a}"`, which the quoted join has already made one word
  — and after the prompt escapes and the quoting too:

  | probe | zsh 5.9.2 | if the sort ran first |
  | --- | --- | --- |
  | `a=("%x" "*"); ${(@%o)a}` | `*` then the path | the path then `*` |
  | `a=("a b" "a!"); ${(@qo)a}` | `a!` then `a\ b` | `a\ b` then `a!` |
  | `a=("'z'" b); ${(@Qo)a}` | `b` then `z` | `z` then `b` |

  The last two are what moved it. `*` sorts ahead of a path only once
  `%x` has become one, `a!` ahead of `a\ b` only once the space has
  become a backslash, and `b` ahead of `z` only once the quotes have
  gone. This implementation had the step between the case conversion and
  the quoting, which passed every row above that does not involve `%`,
  `q` or `Q` and was wrong on all three of these.

- **`(M)` reads a match from the other side, and reaches exactly two
  operators.** On the four trims it substitutes the part the pattern *took*
  rather than the part it left, with the operator still choosing how much:
  `${(M)v#h*l}` on `hello` is `hel` and `${(M)v##h*l}` is `hell`, against
  `lo` and `o` without it. A pattern that matches nothing substitutes
  nothing — `${(M)v#zzz}` is empty where `${v#zzz}` is the whole value —
  and an empty pattern takes the empty string. On `:#` it keeps the
  elements the pattern matched instead of dropping them:
  `a=(f1 f22 f333); ${(M@)a:#f2*}` is `f22` where `${(@)a:#f2*}` is `f1
  f333`.

  Everywhere else it does nothing, and that was measured one operator at a
  time rather than reasoned from the name: a replacement, an anchored
  replacement, a substring, the conditionals, `:|`, `:*` and an expansion
  with no operator at all are all what they would have been without it.
  Written twice it is written once.

- **Order of application.** Operators run before flags: `${(U)x:-def}` on an
  unset `x` is `DEF`, `${(U)x#h}` on `hello` is `ELLO`, `${(U)u:=def}`
  assigns `def` and substitutes `DEF`. `(P)` is the exception and runs
  *first*: `${(P)x:-def}` with `x=y` and `y=val` is `val`, and with the
  target unset the default fires on the resolved value. The manual's rule
  list agrees: name replacement (4), double-quoted joining (5), modifiers
  (7), forced joining (10), splitting (11), case (12), prompt escapes (13),
  quoting (14).
- **Arrays are elementwise.** `a=(ab cd); ${(U)a}` is `AB CD`; `${(q)a}` on
  `("x y" z)` is `x\ y z`. In double quotes without `(@)` the elements are
  first joined with the first character of `$IFS` (rule 5); `"${(@U)a}"`
  and `"${(U)@}"` keep one field per element — `$@` and an `[@]` subscript
  keep their fields without needing `(@)`.
- **Splitting forces fields even inside quotes.** `"${(s.:.)x}"` on `a::b`
  is two fields; only `"${(@s.:.)x}"` keeps the empty one as a third. An
  unquoted result always drops empty words. `(f)` on an empty value is no
  field unquoted and one empty field as `"${(@f)x}"`. `(s)` on an array
  joins with `$IFS`'s first character before splitting (rule 10):
  `a=(a:b c:d); ${(s.:.)a}` is `a`, `b c`, `d` — **unless the group
  carries `(@)`**, which turns that join off so each element is split on
  its own: `a=(a b); ${(@s.:.)a}` is the two fields `a` and `b` where
  `${(s.:.)a}` is the one field `a b`, and `"${(@s.:.)a}"` is two as well.
  `(f)` is exempted with it. Three neighbors say how narrow this is: a
  `j` separator asked for by name puts the join back
  (`${(@j:-:s.:.)a}` is `a-b`), the `@` *letter* is what exempts and not
  everything else that keeps fields (`${(s.:.)a[@]}` and `${(s.:.)@}` both
  join, where the same with the letter do not), and the `=` split is not
  exempted at all — `a=(x '' '' y); "${(@)=a}"` is two fields where
  `"${(@s.:.)a}"` on the same array is four. An empty separator
  `(s::)` splits into characters. Separator delimiters may be any
  punctuation — `(s.:.)`, `(s:,:)` — or the matched pairs `()`, `[]`,
  `{}`, `<>`; the separator may be several characters.
- **`(0)` is that split with a NUL for a separator**, and the vendor
  manual gives it as a shorthand for `ps:\0:`. Every rule above holds
  for it unchanged — the join ahead of the split, the `(@)` exemption
  from that join, which empty fields survive — measured a row at a time
  with `v=$'a\0b\0c'` and `w=$'x\0\0y'`:

  | probe | zsh 5.9.2 |
  | --- | --- |
  | `${(@0)v}` | 3 fields, `a` `b` `c` |
  | `${(0)v}` | 3 as well: the `@` is about quoting, not about the split |
  | `${(@0)w}` | 2 — an empty field is dropped unquoted |
  | `"${(@0)w}"` | 3 — and kept in quotes with the fields flag |
  | `"${(0)w}"` | 2 — dropped again without it |
  | `"${(0)$'p\0'}"` | 2 — but the field at an *edge* survives |
  | `a=($'a\0b' $'c\0d'); ${(0)a}` | 3: `a`, `b c`, `d` — joined first |
  | `${(@0)a}` | 4: each element split on its own |
  | `${(0w)#w}` / `${(0W)#w}` | 2 / 3 — the word counts see it too |

  **It has no long form.** An `s` argument cannot hold the byte:
  `${(@s.\0.)w}` splits on the two characters `\` and `0` and leaves one
  field, measured. So the letter is the only way to ask for the split,
  which is why `${(pj:\0:)a}` — joining *on* a NUL — is common in the
  wild beside it and is not its inverse.

  **The letter written last decides which separator is used**, and that
  is a rule about the set rather than about this member. With
  `m=$'a\nb:c'`, `${(@fs.:.)m}` splits at the colon and `${(@s.:.f)m}`
  at the newline; with `n=$'a\nb'`, `${(@f0)n}` is one field and
  `${(@0f)n}` is two. A fixed precedence answers half of that table.
- **`(l)` and `(r)` lay each word out in a field.** `(l:expr::string1::string2:)`
  pads on the left and `(r:…)` on the right; the arguments after the
  width are optional, and neither, the first, or both may be given.
  Measured on zsh 5.9.2 with `v=ab`:

  | probe | zsh 5.9.2 | what it pins |
  | --- | --- | --- |
  | `${(l:5:)v}` | `   ab` | no fill is spaces |
  | `${(l:5::-:)v}` | `---ab` | `string1` repeats |
  | `${(l:5::-::+:)v}` | `--+ab` | `string2` goes once against the word |
  | `${(r:5::x::y:)v}` | `abyxx` | and on the other side for `r` |
  | `${(l:1:)v}` | `b` | a narrow field truncates from the far end |
  | `${(r:1:)v}` | `a` | which is the opposite end for `r` |
  | `${(l:0:)v}` | `ab` | a zero width is the flag doing nothing |
  | `${(l:7::ab:)v}` | `bababab` | the repetition keeps its *tail* |
  | `${(r:7::ab:)v}` | `abababa` | and its head on the right |
  | `${(l:3::x::yz:)v}` | `zab` | `string2` is truncated the same way |
  | `${(r:3::x::yz:)v}` | `aby` | and from the other end |

  **The width is an arithmetic expression, not a number.** `${(l:$n:)v}`,
  `${(l:n:)v}` and `${(l:2+3:)v}` are all the same four- or five-wide
  field, an unset name is zero — so the flag does nothing rather than
  failing — an empty expression is zero, and a **negative width is its
  own size**: `${(l:-3:)v}` is the three-wide ` ab`. An expression that
  will not read is the ordinary arithmetic failure and is fatal to the
  script: `${(l:x y:)v}` is `bad math expression: operator expected`.

  **A fill written empty is not a fill left out**, and only a non-default
  `$IFS` separates the two. With `IFS=.`: `${(l:5:)v}` is `   ab`, spaces,
  while `${(l:5:::)v}` is `...ab` — an argument written as two delimiters
  together takes `$IFS`'s first character. The same holds for `string2`,
  `${(l:5::x:::)v}` being `xx.ab`, and an `$IFS` with no first character
  leaves the slot contributing nothing: `IFS=; ${(l:5:::)v}` is `ab`.

  **The three slots are filled in one at a time.** A group may write the
  letter twice, and a later one sets the width always and each fill only
  where it wrote one: `${(l:3::x::y:l:5:)v}` is `xxyab` and
  `${(l:3::x::y:l:5::z:)v}` is `zzyab`.

  **Both letters in one group is a third shape.** The word is cut in
  half, the first half laid in the left field and the second in the
  right, so the result is as wide as the two fields together. With
  `(l:4::L:r:4::R:)` over the empty string, `a`, `ab`, `abc`, `abcd` and
  `abcde`: `LLLLRRRR`, `LLLLaRRR`, `LLLabRRR`, `LLLabcRR`, `LLabcdRR`,
  `LLabcdeR`. An odd word gives the extra unit to the *second* half,
  which is the manual's "the extra padding is applied on the left" read
  from the other side. A zero width takes that side out entirely rather
  than halving the word for the other: `abcd` with `(l:0::L:r:6::R:)` is
  `abcdRR`.

  **Where the step sits is measured**, and one row of it disagrees with
  the manual's rule numbers. Padding is after the length
  (`${(l:5::x:)#v}` on `abc` is `xxxx3`), after the case conversion
  (`${(Ul:4::x:)u}` is `xxAB`), after the prompt escapes, after the
  quoting (`${(ql:6::x:)q}` on `a b` is `xxa\ b`), after the unquoting,
  after the ordering (`b=(bb a); ${(@ol:3::x:)b}` is `xxa xbb`), after
  the splitting — each field is padded on its own — and after the quoted
  join, which is why `a=(one two three); "${(l:5::-:)a}"` is `three`
  rather than three padded elements. It is also applied to the set
  test's answer: `${(l:5::-:)+v}` is `----1`. But it runs **before** the
  re-reading, where the manual lists it after: with
  `e='ab$(echo XY)'`, twelve characters as written and four once read
  again, `${(el:20::x:)e}` is `xxxxxxxxabXY` — padded to twenty first
  and then re-read — where a re-reading that ran first would leave
  sixteen `x`.

  **A width past what a 32-bit count can hold is refused by that shell**
  — `${(l:2147483648:)v}` is an error in the flags there and
  `${(l:4294967296:)v}` is the value unchanged, the count having wrapped
  to zero. That is an artifact of one implementation's internal width
  rather than a behavior, so it is a **measured gap** here rather than
  something reproduced.

  **A `(~)` that marks a fill is refused by name here.** Marked, the fill
  is live where the rest of the result is escaped — `w=ab;
  ${(~l:5::*:)w}` is the pattern `***ab` in that shell where
  `${(l:5::*:)w}` is those five characters literally — so a fill would
  have to be counted as text and escaped as none. A marked *join*
  separator beside a padding flag is refused for the same reason: the
  join is the one step whose result is part escaped and part not, and
  the padding is measured against the whole of it. No script in reach
  writes either pairing.
- **`(q)` in detail.** Backslash-escapes space and `` ` ``, `$`, `"`, `'`,
  `\`, `*`, `?`, `[`, `]`, `(`, `)`, `{`, `}`, `<`, `>`, `|`, `;`, `&`,
  `#`, `^` anywhere, and `~` and `=` **only as the value's first byte**,
  where one opens a tilde expansion and the other an equals one:
  measured, `${(q)x}` on `a~b` is `a~b` and on `~x` is `\~x`, and on
  `PATH=/x` is `PATH=/x` where `=x` is `\=x`. It leaves `!`, `%`, `:`,
  `,`, `.`, `/`, `@`, `-`, `_`,
  `+` and alphanumerics alone; renders each control or non-UTF-8 byte as
  its own `$'…'` segment — `$'\n'`, `$'\t'`, `$'\a'`, `$'\b'`, `$'\f'`,
  `$'\r'`, `$'\v'` by name, anything else as three-digit octal like
  `$'\033'`. An empty value is `''`. `(qq)` wraps in single quotes with
  `'` written as `'\''`, unconditionally — `plain` becomes `'plain'`.
  `(qqq)` wraps in double quotes escaping `\`, `` ` ``, `"`, `$`.
  `(qqqq)` wraps in `$'…'` escaping `'`, `\`, `!` and control bytes as in
  `(q)`. Multibyte UTF-8 passes through every form.
- **`(b)` in detail.** The third quoting flag, and the one whose question is
  about *patterns* rather than about shell syntax: it backslashes the
  characters a pattern gives meaning to and nothing else, so that the result
  matches the value it came from. Measured byte by byte across 1..127 on zsh
  5.9.2, 2026-09-10, the set is thirteen characters —

      #  (  )  *  <  >  ?  [  \  ]  ^  |  ~

  — and everything else is written through as itself, including the space,
  the tab, the newline, `!`, `-`, `{`, `}`, `$`, `` ` ``, `"`, `'`, `%`, `;`,
  `&`, every other control byte, and every byte above `0x7f`, so multibyte
  UTF-8 and invalid bytes alike pass through untouched. `!` and `-` would be
  in the set by symmetry with a bracket expression's own metacharacters and
  are measured out: whatever would open the bracket is escaped already.

  **It does not move with the glob options.** The same 127-row table comes
  back identical with `extendedglob` set and unset, with `kshglob` set, with
  `bareglobqual` unset, with `noglob` set, and under every combination of
  those — so `#` and `^` are escaped even in the shell state where they are
  ordinary text. That is the only answer that can be right for text whose
  reader's options are not the expansion's to know, and it costs nothing: an
  escaped ordinary character is that character.

  **Nor with position**, which is where it parts company with `(q)`:
  `${(q)x}` on `a~b` is `a~b` and `${(b)x}` on the same value is `a\~b`, a
  pattern's `~` being the exclusion operator wherever it stands where a shell
  word's is a tilde expansion only at the front. The space is the other
  difference and the plainer one — `${(b)x}` on `a b*c` is `a b\*c` where
  `${(q)x}` is `a\ b\*c`. And an empty value is **empty** here where `(q)`
  writes `''`: an empty pattern is simply empty, while an empty shell word
  has to be spelled.

  The property it exists for is the round trip, and it holds for every value:
  `[[ $v == ${~${(b)v}} ]]` is true whatever `v` holds. The `${~…}` is part
  of the statement rather than an aside — a value's metacharacters are not
  live in a pattern until something asks for them, so without it the match is
  literal and the backslashes are text.

  It stands beside `(q)` in rule 14, and the compositions say so. Rule 13 has
  run: `g='a\x2ab'; ${(bg:e:)g}` is `a\*b`, where a step running first would
  mark the value's own backslash and hand `(g)` a doubled one, and
  `${(b%):-%B*}` is `ESC\[1m\*`, where one running first would leave the `[`
  the prompt escape produced unmarked. The second of those needs a `TERM` in
  the environment — with none, `%B` expands to nothing and both readings
  answer `\*` — which is why the corpus row uses the first. The unquoting
  runs behind it, so `${(bQ)v}` is the value back again; the padding counts
  the *escaped* text, so `${(bl:8:)v}` on `a*b` leaves four spaces and not
  five; and the join has already happened, so the separator the group
  inserted is marked along with everything else: `a=('x*y' 'p');
  ${(bj:|:)a}` is `x\*y\|p`.

  **`b` and `q` are one family in the grammar**, and a group carries one
  member of it once: `${(bq)v}`, `${(qb)v}`, `${(bb)v}`, `${(bUq)v}`,
  `${(q-b)v}` and `${(qqqb)v}` are all `error in flags near position N`, and
  N is always the *second* member's own character — 5, 5, 5, 6, 6 and 7 for
  those six. `Q` is not in the family, and a `-` behind a `b` is the
  signed-numeric sort flag exactly as it is anywhere a `q` did not eat it.

  The one composition this implementation refuses by name is a `(~)`-marked
  join separator beside it. The mark exempts an inserted separator from the
  escape the rest of the result gets, and this is the member of the family
  the exemption does not survive: measured, `${(~bj.|.)a}` is `x\*y\|p` with
  the bar escaped where `${(~qj.|.)a}` is `x\*y|p` with the bar live. Nothing
  in reach writes the pair.
- **A word branch that substituted nothing is not an empty value**, and
  `(q)` is the only thing in the family that can tell them apart. An empty
  value has to be written `''` because backslashes cannot spell one; a
  `${name:-word}`, `${name-word}`, `${name:+word}` or `${name+word}` whose
  branch ran and whose word came to nothing is written as **nothing**.
  Measured on zsh 5.9.2 with `y=""`, inside double quotes:

      "${(q)y}"          ''       an empty value
      "${(q)y:-}"        ''       the branch ran with no word written
      "${(q)y:-$y}"      nothing  the branch ran and the word came to nothing
      "${(q)y:-$nope x}" \        the word came to " ", which is not nothing
      "${(q)nope-$nope}" nothing  the same, without the colon
      "${(q)y:+$nope}"   nothing  y is "a": the alternate ran, and came to nothing
      "${(q)y:+}"        ''       y is "": the alternate did not run
      "${(q)nope:=$nope}" ''      the assigning form substitutes what it stored
      "${(q)y#*}"        ''       trimmed to empty is an empty value

  Two gates narrow it. Only the backslash style sees it — `(qq)`, `(qqq)`,
  `(qqqq)` and `(q-)` all write their own empty wrapper for the same
  expansion — so the answer belongs to that style rather than to the flag
  group. And only inside double quotes: written bare on the right of an
  assignment, `v=${(q)y:-$y}` stores the same two characters an ordinary
  empty value gives. The field itself survives either way —
  `set -- "${(q)y:-$y}"` leaves `$#` at 1 and `${#1}` at 0 — so this is a
  fact about the text, and neither about how many fields the expansion made
  nor about whether one exists. Measured: `param/the-quoting-flag-and-a-
  branch-that-substituted-nothing` and the three rows beside it.

  The nesting the gap was reported under is not part of it. `${(q)${x}:-$y}`
  and `${(q)y:-$y}` are the same disagreement, and `${(q)${y:-$y}}` — the
  branch inside the nested expansion rather than beside it — is `''`,
  because what the enclosing flag reads is a value (#1549).
- **`(q-)` is minimal quoting**, and `-` is a modifier the `q` in front of
  it eats rather than a flag of its own. The value is cut at each `'`; each
  run between the cuts is wrapped in single quotes if any byte in it needs
  quoting and written bare if none does; each `'` becomes `\'` outside any
  quoting. The bytes that ask for quotes are the same table `(q)` escapes,
  including the positional `~` and `=` — the position being the *value's*
  start and not each run's, so `'~x` is `\'~x` and `~'a` is `'~'\'a`. Tab
  and newline ask for quotes and then go inside them as themselves rather
  than as `$'\t'`; **every other control byte, and every byte above
  `0x7f`, asks for nothing** and is written raw — `$'a\001b'` is
  `a\001b`, where `${(q+)…}` renders it. An empty value is `''`,
  which is the one value with nothing to quote that still cannot be
  written bare. The property the flag exists for is the round trip:
  `eval "r=${(q-)v}"` leaves `r` equal to `v` for every value measured.
  Composition is the ordinary rule-14 one — per element of a list, after
  the join a quoted expansion asks for, after the case flags and after any
  split.
- **`(q+)` is the extended form** of that, and `+` is the same modifier
  slot. It is the same walk with three differences, all measured:
  1. **The quoting decision is over the whole value**, not run by run, so
     `has'quote` is `'has'\''quote'` where `q-` gives `has\'quote` —
     neither run needs quotes on its own and both get them. An empty run
     is still written as nothing, so `x'` is `'x'\'`.
  2. **The two start-only specials are special wherever they stand**, so
     `a~b` is `'a~b'` and `PATH=/x` is `'PATH=/x'` where `q-` leaves both
     bare. That is the same table asked with the position pinned to zero.
  3. **One byte that cannot be written as itself moves the whole value
     into `$'…'`**: `$'a\tb'` and `$'a\C-Ab'`, where `q-` puts the tab
     inside ordinary quotes and leaves the control byte raw. The
     vocabulary is the caret notation and not `(qqqq)`'s octal — `\t` and
     `\n` are the only names, every other byte below `0x20` and `0x7f`
     are `\C-X` with bit 6 flipped, and a byte above `0x7f` is `\M-`
     followed by its low seven bits spelled the same way. `'` and `\`
     are escaped inside; `!` is not, where `(qqqq)` writes `\!`. A byte
     above `0x7f` counts as unrenderable only under a UTF-8 locale; under
     `LC_ALL=C` the same shell writes it as itself, which is the same
     locale question `(q)` and `(qqqq)` answer and is answered the same
     way here.

  The round trip is the property, as it is for `q-`, and it is a stronger
  claim: reading `${(q+)v}` back needs the `\C-` and `\M-` escapes of
  `$'…'`, so the writer and the reader are graded against each other.
  **Two byte values do not satisfy it in zsh either**: 0xa7 is written
  `$'\M-''` and 0xdc `$'\M-\'`, each ending the quoting early. Both are
  reproduced rather than corrected.
- **Which `-` is that modifier** is settled by the parser, which keeps the
  eaten character out of the flag list, so every `-` that reaches the
  interpreter is zsh's signed-numeric sort flag `(-)`. Measured over
  `b=(-1 -10 -3 2 10)`: `${(o)b}` and `${(oq-)b}` sort lexically, so the
  `q` ate the `-`; `${(o-)b}` and `${(oq--)b}` sort signed, so a lone `-`
  and a second one are the sort flag; `${(oq+-)b}` sorts signed too, a
  `q+` having already taken the slot. `${(qU-)v}` quotes with
  backslashes, so the adjacency is literal. The modifier may be the later
  of two: `${(-q-)v}` is minimal quoting with a sort flag in front of it,
  which does nothing to a scalar.
- **Where a modifier may not stand.** `+` is no flag on its own and none
  anywhere but directly behind the group's first `q`; `-` behind any
  later `q` is not a modifier either; and a group that has taken a `q-`
  takes no further `q`. Each is an error in the flags rather than an
  unimplemented flag, deferred to when the branch is reached, and the
  position is measured — for a doubled `q` it is that `q`'s and not the
  modifier's:

  | group | answer |
  | --- | --- |
  | `${(+)v}` | error in flags near position 4 |
  | `${(U+)v}` | error in flags near position 5 |
  | `${(q-+)v}` | error in flags near position 6 |
  | `${(qq-)v}` / `${(qq+)v}` | error in flags near position 5 |
  | `${(qoq-)v}` | error in flags near position 6 |
  | `${(q-q)v}` | error in flags near position 6 |
  | `${(q-Uq)v}` | error in flags near position 7 |
  | `${(q+q)v}` / `${(q+Uq)v}` | **read** by zsh, and answered
    `'has'quote'` on `has'quote` — a spelling that does not read back, so
    it is refused by name here |
- **`(-)` is the signed-numeric sort**: `n` with a `-` in front of a digit
  run read as that run's sign rather than as text. It implies `n` rather
  than modifying one — `${(@-)b}` and `${(@n-)b}` agree — and `(a)` still
  beats it. It is not a number parse: a decimal point is not part of the
  number, so `(-1 -1.5)` stays in that order; a leading `+` is not a sign
  and is compared as the byte it is; and the sign need not stand at the
  word's start, so `(x-1 x-10 x-3)` comes back `x-10 x-3 x-1`. A `-` in
  one word only is no sign, which keeps `-1` ahead of `-y`. Numerically
  equal words fall back to byte order, the same fallback `n` has. The flag
  is invisible outside a pair of negatives, every digit sorting above the
  `-` at `0x2d`.
- **`(Q)` removes quoting and expands nothing**, and that pair is the
  whole flag. `v='"$x"'` with `x` set is `$x`, two characters, where a
  reading that handed the value to the parser would answer `hi`;
  `$(echo hi)` and a backquote stay text; a tilde and a `*` stay text.
  It makes no fields either — `${(@Q)v}` on `"'a b' c"` is one word.
  `'…'` takes its contents whole, `"…"` gives a backslash meaning before
  `"`, `` ` ``, `$` and `\` and drops a `\`-newline pair altogether while
  leaving the backslash in place anywhere else (`"a\qb"` is `a\qb`),
  `$'…'` is decoded, and a bare backslash escapes whatever follows it.
  `$"` is not a quote, so `$"x"` is `$x`.

  **An opener with no closer comes back as written** — `'unterm` is
  `'unterm` — rather than as an error, a truncation, or the contents
  without the opener. A substitution is a verbatim region for the same
  reason: `"a$(b"c")d"` is `a$(b"c")d`, so the quotes inside it neither
  closed the outer one nor were removed, and `"a$(x b"` comes back whole
  because the unclosed group takes the quote with it. `` ` ``…`` ` ``,
  `$(…)` (parentheses counted, so `$(b(c))` is one) and `${…}` all
  behave that way, inside quotes and out.

  Written beside `q` it runs **after** it, which the manual's single
  rule 14 does not say: `${(Qq)v}` and `${(qQ)v}` on `'a b'` are both
  `'a b'`, the round trip, where a `Q` that ran first would have left
  `a\ b`.
- **`(c)`, `(w)`, `(W)` and `(m)` change what a length counts**, and do
  nothing at all to a value — `${(@c)a}` is the plain expansion. The **last** of
  the three written wins: `${(cw)#v}` on `a b` is 2 and `${(wc)#v}` is 3.

  `c` is the characters of the words joined, separators included, so
  `a=(abc de f); ${(c)#a}` is 8 and not 6. **The separator it counts is
  a space and not `$IFS`'s first character** — measured, `IFS=:` leaves
  it at 8 — and a `j` argument does replace it: `${(cj.--.)#a}` is 10.

  `w` counts words and `W` counts the empty ones too, an element at a
  time with the totals added — `a=(a: :b)` with `(s.:.)` is 3 and 4,
  where a join first would have made both one lower. The separator is
  the `s` argument where one was written, a newline where `f` was
  (`${(fw)#v}` on `a\nb\n` is 3), and `$IFS` otherwise — and that last
  is a different *rule* rather than a different value:

  Each cell below is `w` / `W`, measured on the same value read three
  ways:

  | value | `$IFS` default | `IFS=:` | `(s.:.)` |
  | --- | --- | --- | --- |
  | `""` | 0 / 0 | 0 / 0 | 1 / 1 |
  | `"::"` | 1 / 1 | 3 / 3 | 1 / 3 |
  | `":a:"` | 1 / 1 | 3 / 3 | 2 / 3 |
  | `"a b  c"` | 3 / 4 | 1 / 1 | 1 / 1 |

  Under `$IFS`, `W` lets every separator delimit while `w` collapses a
  run of IFS *whitespace* and trims it from both ends — so a
  non-whitespace `IFS` makes the two agree. A trailing separator still
  opens a field for `w`, which ordinary word splitting absorbs, and it
  is the trailing *run* that decides rather than the last byte:
  `IFS=': '` counts `a: ` as 2 and `a ` as 1. With an explicit
  separator, `W` is every field the literal split makes while `w`
  collapses runs of it and drops a leading empty field but keeps a
  trailing one; an empty separator counts characters, and an empty value
  is one field where under `$IFS` it is none.

  **The length is taken before the double-quoted join and before the
  split**, which is not where the rule numbers put it: `"${(U)#a}"` on
  `(abc de f)` is 3 and not 8, `"${(Uj.-.)#a}"` is 3 as well, and
  `${(s.:.)#v}` on `:a:` is the value's 3 characters rather than the
  fields the separator would have made. This implementation answered 8
  to the first of those until #935's second change.

  **`(m)` is the fourth, and it is not one of the three.** The other
  three say *what* to count and the last one written wins; `m` says what
  a **unit** is, so it composes with them rather than displacing one.
  Measured on zsh 5.9.2 under `LC_ALL=en_US.UTF-8`, with `w=$'日本'`
  (two East Asian wide characters), `c` an `e` and a combining acute,
  `b=$'\x01'`, `a=($'日' de f)` and `s="a b  c"`:

  | probe | result | |
  | --- | --- | --- |
  | `${#w}` | `2` | two characters |
  | `${(m)#w}` | `4` | drawn in four columns |
  | `${#c}` | `2` | |
  | `${(m)#c}` | `1` | a combining mark is no column of its own |
  | `${(m)#b}` | `1` | and a control character *is* one |
  | `${(m)#s}` | `6` | all-ASCII, so the readings agree |
  | `${(m)#a}` | `3` | a list is still counted in elements |
  | `${(c)#a}` | `6` | |
  | `${(mc)#a}` | `7` | `c`'s count, taken in columns |
  | `${(mw)#s}` | `3` | a word has no width |

  **The separator `c` counts is measured in characters whatever `m`
  says.** `${(cj.日.)#a}` is 6 and `${(mcj.日.)#a}` is 7: the one column
  the flag added is the wide *element*, and the two wide separators
  counted one each in both readings. Measuring the separator the same way
  as the words answers 9, which is a plausible length for a padding
  calculation to be built on.

  Where a length counts **bytes**, `m` counts bytes. Under `LC_ALL=C`,
  and under a UTF-8 locale with `multibyte` off, `${(m)#w}` is 6 there —
  the same answer `${#w}` gives, there being no characters for one to be
  wider than another.

  The control-character row is where the flag parts company with the
  *line editor's* width, which counts a control character as no cell at
  all. The prompt language's `%N(l.…)` column agrees with `(m)` and not
  with the editor, which is measured — so this implementation shares one
  width table between the two and repl's cell width is a third reader.
  See `interp/displaywidth.go`; the two are subtracted from each other by
  powerlevel10k, which measures a segment with `${(m)#…}` and the whole
  prompt with `%N(l.…)` (#2119).

  `(m)` beside `(l)` or `(r)` — a field measured in columns — is
  **refused by name**. What that shell does when a wide character
  straddles the edge of the field is measured and is not symmetrical:
  with `w=$'日本'`, `${(ml:3:)w}` keeps what fits and `${(mr:3:)w}` keeps
  what crosses, `${(ml:1:)w}` is empty where `${(mr:1:)w}` is one
  character, and `${(ml:7::日:)w}` is empty where `${(mr:7::日:)w}` is
  four characters wide. The last is that shell's own edge rather than a
  rule, and a field of the wrong width at status 0 is a prompt drawing
  off the end of a line with nothing saying which measurement was wrong.
- **`(k)`/`(v)` order.** zsh yields hash order, which it does not promise;
  this implementation yields sorted key order, the same deterministic
  answer `${m[@]}` already gives. `(k)` on anything that is not an
  associative array is a no-op, and `(v)` matters only beside `(k)` —
  except over a subscript search, where `(v)` alone substitutes the
  matched keys' values; see "A search over an associative array".
- **`(%)` prompt escapes.** `%x` and `%N` both name the file being read:
  under `-c` the shell's own name (`zsh`), in a script the script's path,
  in a sourced file the sourced file's path. Inside a function `%N` is the
  function's name while `%x` stays the defining file. `%%` is a literal
  `%`. The escapes apply to the value — `x="%x"; ${(%)x}` expands — and
  elementwise on arrays. `%n` is the user the shell runs as. zsh
  implements its whole prompt language here (`%M` the host, `%~` the
  directory, `%D` the date); this implementation carries only `%x`, `%N`,
  `%n` and `%%`, and refuses anything else loudly rather than answering
  wrong. Louder than the shell, in fact — measured, `${(%):-%zz}` in zsh
  expands `%z` to nothing at all and carries on at 0, where this refuses
  by name at 1. Silence is the worse of the two failures, so the
  difference is deliberate.

  **`%n` is a fact about the process, not about the environment.** It is
  the login name for the shell's real uid, and it ignores `USER`,
  `LOGNAME` and `USERNAME` — assigned inside the shell or injected before
  it starts. Measured on zsh 5.9.2 and on bash's `\u`, which ignores them
  too, so this is unanimous wherever the escape exists at all. Reading one
  of those variables would make `env USER=someone-else zsh` draw the wrong
  person. `USERNAME` is not even assignable: zsh answers `failed to change
  group ID` and leaves it, because the name is bound to the uid.

  So the value is *carried in* rather than read here —
  `Runner.SetPromptUser`, filled in by the shell binaries beside the `$UID`
  they already read — which keeps the process lookup out of a package that
  may be embedded twice in one program. A runner nobody told refuses `%n`
  with the rest rather than expanding it to nothing.

  The same escape is `%n` in a prompt, and it means the same thing there:
  measured in both positions in one shell. The prompt renderer in `repl/`
  resolves it from `$USER` and `$LOGNAME` instead, which is a divergence
  from the panel that this does not close — see the note in that file.
- **An empty name is legal once flags are present.** `${(U)}` is an empty
  string, and `${(%):-%x}` — the wild idiom — has no name at all: the `:-`
  fires and the flags apply to the substituted word. `${()x}` with empty
  parens is `${x}`.
- **An unrecognized flag is a runtime error**, not a parse one: `zsh:1:
  error in flags near position 4 in '${(Y)x}'`, status 1, and the line is
  abandoned; inside a branch never taken it is never diagnosed
  (`if false; then : ${(!)x}; fi` runs clean). The position is 1-based and
  counts from the `$`. A missing closing parenthesis is diagnosed the same
  way at the first character that is not a flag: `${(Ux}` errors at
  position 5.

  **The text it quotes is the rest of the word**, from the expansion's `$`
  to the end of the word it stands in — not the expansion alone, and not
  the rest of the line. Measured on zsh 5.9.2, 2026-09-12:

  | probe | quoted text |
  | --- | --- |
  | `echo ${(!)v}` | `${(!)v}` |
  | `echo ${(!)v}rest more` | `${(!)v}rest` |
  | `echo ${(g:x:)v} tail` | `${(g:x:)v}` |
  | `print -r -- "[${(Z:x:)v}]"` | `${(Z:x:)v}]"` |
  | `echo "${(Ux}"` | `${(Ux}"` |
  | `echo "${(!)v}"$'q'tail` | `${(!)v}"$'q'tail` |
  | `echo $( echo ${(!)v} in )` | `${(!)v}` |

  Row three is the discriminating one: a line would have carried ` tail`
  with it, and the expansion alone would have carried nothing in rows two,
  four, five and six. Row seven says the word is the *inner* word where the
  group stands inside a substitution. A control character in that text is
  made visible rather than written out — a tab is `\t`, a newline `\n`,
  every other control character is a caret spelling, and a backslash is
  left alone, which is the same rendering `(V)` uses.

### A context with room for one word joins the group's words

An assignment's value, a `case` subject, a `[[ ]]` operand and a here-string
keep no fields, so the words a group came to are joined there — and every
step below the join then sees one word. Measured on zsh 5.9.2, 2026-09-12,
with `f(){ printf "b  b\na a\nc\n"; }`, `g(){ printf "b\na\nb\n"; }`,
`h(){ printf "3\n1\n2\n"; }` and `v=c,a,b`. The double space in `f` is the
instrument: a value left whole and a value split and rejoined are the same
nine characters without it.

| probe | zsh 5.9.2 | |
| --- | --- | --- |
| `x=${$(f)}` | `b b a a c` | split, then joined on IFS |
| `x=${(j:-:)$(f)}` | `b-b-a-a-c` | on the separator the group named |
| `x=${(o)$(f)}` | `b b a a c` | one word by now, so nothing to sort |
| `x=${(u)$(g)}` | `b a b` | nor to dedup |
| `x=${(s:,:)v}` | `c,a,b` | nor to split |
| `x=${(l:3::_:)$(h)}` | `1 2` | and the pad measures the joined word |
| `x=${(q)$(f)}` | `b\ b\ a\ a\ c` | which the quoting takes whole |
| `printf '[%s]' ${(o)$(f)}` | `[a][a][b][b][c]` | where a list context sorts |

The last row is every other row's refutation: the same characters on a
command line sort five fields, so this is the context and not the flag.

**Where the join sits: below the operator, above everything else.** It is
*not* rule 5's quoted join, and the discriminating pair is the same
characters in the same assignment with and without quotes:

| probe, `y=(ab ab)` | zsh 5.9.2 | |
| --- | --- | --- |
| `x=${(j:+:)y#ab}` | `+` | the trim emptied both elements first |
| `x="${(j:+:)y#ab}"` | `+ab` | rule 5 joined first, then trimmed once |
| `x=${(j:+:)y}` | `ab+ab` | the control, with no trim at all |
| `x=${(@)z:/x/Q}`, `z=(x y)` | `Q y` | the element operators run elementwise |
| `x=${(o)q:#one}`, `q=(one two)` | `two` | all of them |

**The `@` letter does not exempt it**, where in quotes it does: `y=(c a b)`
and `x=${(o@)y}` is `c a b`, so the join happened and the sort found one
word. A scalar context has nowhere to put a second field.

**A length is still asked ahead of the join**, exactly as in quotes:
`a=(abc de f); x=${(U)#a}` is 3.

**The splitting flags do not split there.** `f`, `s`, `0` and `p` follow the
rule the `=` spelling already followed. `x=${(s:,:)v}` is `c,a,b` where
`printf '[%s]' ${(s:,:)v}` is three fields and `printf '[%s]' "${(s:,:)v}"`
is three as well — that pair is what says quoting is not the thing deciding
it. The `@` letter does not turn the split back on either.

**The shell split is the exception, and is joined back afterwards.** It is
the one step below the join that can make a list out of one word, so the
join is repeated after it — on IFS, and not on the separator the group
named, which is what says the two joins are different joins. With `u='b a'`:
`x=${(oZ+n+)u}` is `b a`, `x=${(l:3::_:Z+n+)u}` is `b a`, `IFS=:
x=${(Z+n+)u}` is `b:a`, `x=${(j:+:Z+n+)u}` is `b a`, and
`printf '[%s]' ${(oZ+n+)u}` on a command line is `[a][b]`.

**The inner of a nesting is not one of these contexts**, though it reaches
the group by the same route: what it comes to is read by the operator around
it rather than by a command line. `a=(one '' two)` and
`"${(j:,:)${(@)${a[@]}}}"` is `one,,two`, three fields reaching the join.

### `(t)` is what a name *is*, in place of what it holds

The one read that separates "the value changed" from "the name is still an
array", and `[[ ${(t)x} == *array* ]]` is how a function checks what it was
handed. Measured on zsh 5.9.2, 2026-09-10:

    v=abc              scalar
    w=(a b)            array
    typeset -A m       association
    typeset -i n=1     integer
    typeset -F f=1     float          `-E` is a float too
    typeset -x e=1     scalar-export
    typeset -xa a=(1)  array-export
    local l=1          scalar-local
    $PATH              scalar-tied-export-special
    an unset name      ``             and *unset*: `${(t)u-D}` is `D`

The word is the parameter's kind followed by its attributes,
hyphen-separated, in this order — measured a pair at a time:

    kind  local  left|right_blanks|right_zeros  lower|upper  readonly
          tied  export  unique  hide  hideval  special

so `typeset -xr` is `scalar-readonly-export`, `typeset -aU -r` is
`array-readonly-unique`, `typeset -L5 -x` is `scalar-left-export`, and a
`typeset -ir` inside a call is `integer-local-readonly`. The **container
wins over the numeric attribute**: `typeset -ia ia; ia=(1 2)` is `array`
and says nothing about integers.

**The flag replaces the base with the word, and the rest of the group runs
on that word.** This is the whole of its interaction with everything else,
and it is measured rather than assumed:

    ${(Ut)v}      SCALAR   the letters transform it
    ${(t)#v}      6        the length measures it
    ${(t)v#s}     calar    an operator applies to it
    ${(t)v:-D}    scalar   whose test does not fire — the name is set
    ${(t)w[1]}    a        a subscript reads its *characters*: `array[1]`
    ${(t)w[2]}    r
    ${(t)w[1,2]}  ar
    ${(t)w[@]}    array
    ${(t)+v}      1        the set test still answers about the name
    ${(Pt)h}      array    behind a `(P)`, the name it resolved to
    s=hello; ${(t)${s}}  hello   a nested inner is a *value*, which has no
                                 name to describe, so the flag does nothing

**The vocabulary is one shell's**, so the core holds none of it: a dialect
supplies the wording through `interp.SetParameterTypeWord` and a runner
nobody told refuses the letter by name — the same rule `(p)` and `(g)`
follow, and the sharpest case of it, because a word invented here would
answer `[[ ${(t)x} == *array* ]]` at status 0 in a shell that has no such
vocabulary. It is the one-name spelling of the `$parameters` table, and
deliberately the same function, so the two cannot say different things
about one parameter.

**What this shell does not carry here.** `$@`, `$0`, `$#` and a positional
are described by zsh — `array-readonly-special`, `scalar-special`,
`integer-readonly-special` — and are refused by name here: they are not in
the table the facts come from, and answering empty would be the word for a
name the shell does not *have*, so `${(t)0-D}` would have substituted `D`
for a parameter every shell has. The `L`/`R`/`Z` padding attributes and
`hideval` are never written, and `special` is narrower than zsh's — it is
written for a parameter this shell *provides*, where zsh calls `HOME` and
`IFS` special as well. Under-reporting an attribute is the failure this can
afford; naming a kind wrongly is not, because `array*` is what a caller
switches on.

### What `(P)` reads as the name, and what it comes to

**The name is read off the front of the text the base came to, and the rest
is dropped.** A base holding more than one word is not a name with a space
in it — no such parameter can exist — so the words behind the first play no
part. Measured on zsh 5.9.2, 2026-09-10, with `x=(p q)`, `y=(r s)`,
`z=(t u)` and `set -- aa bb cc`:

    n=(x y z);  ${(P)n}      →  p q    a list resolves its first element
    s="x y";    ${(P)s}      →  p q    and a *scalar* of two words the same,
                                       which is what says this is the front
                                       of the text and not the first of a list
    v="x-y";    ${(P)v}      →  p q    any character a name cannot hold ends it
    v="x=y";    ${(P)v}      →  p q
    v="_u-z";   ${(P)v}      →  UU     `_` starts a name
    v=" x";     ${(P)v}      →  ``     so a leading space is *no* name
    e=("" x);   ${(P)e}      →  ``     and neither is an empty first element
    v="x[1] j"; ${(P)v}      →  p      a subscript is part of the name
    v="2x";     ${(P)v}      →  bb     a digit run is a positional parameter
    v="12x";    ${(P)v}      →  ``     and `$12` is nothing with three of them
    v="#x";     ${(P)v}      →  3      a one-character special name is that
    v="@x";     ${(P)v}      →  aa bb cc

**A subscript that names a list is not part of the resolution.** The split
is on the subscript's *shape* and not on what it selected:

    ${(P)n[@]}     →  p q    a whole-array subscript changes nothing
    ${(P)n[*]}     →  p q
    ${(P)n[1,2]}   →  p q    nor does a range — not one starting past the
    ${(P)n[2,3]}   →  p q    first element, naming a single one, or
    ${(P)n[3,3]}   →  p q    naming none at all
    ${(P)n[5,6]}   →  p q
    ${(P)n[2]}     →  r s    while an index naming one element *is* the name
    ${(P)n[-1]}    →  t u
    ${(P)n[4]}     →  ``     and one naming nothing is no name

**What it comes to keeps the shape of the parameter it landed on**, and the
base's `[@]` does not travel with it. With `arr=(x y z)`, `na=arr`,
`typeset -A tab=(k1 v1 k2 v2)`, `nt=tab` and `typeset -A one=(a arr)`:

    ${#${(P)na}}         →  3    the element count, not 5
    ${#${(P)nt}}         →  2    an association counts its pairs
    ${#${(vP)nt}}        →  2
    ${#${(kP)nt}}        →  2
    ${#${(P)one[a]}}     →  3    reached through a subscripted base
    ARR=(q r); h=arr; ${#${(UP)h}}  →  2   the letters rename, so this
                                           measures `ARR`
    "${(P)one[@]}"       →  one field, `x y z`
    "${(@P)one[@]}"      →  three fields — the *letter* still keeps them

The controls, which say this is about `(P)` and not about the length prefix
or about `[@]`: `${#${arr}}` is 5 and `${#${(k)tab}}` is 5 — a nested
expansion that is a *value* is measured as text — while `${#${(@)arr}}` and
`${#arr}` are 3.

The first two are one defect: joining the words and looking the join up
found nothing, so every base of more than one element substituted empty at
status 0, and a one-element base — the shape every test had — hid it. The
third is the same split the subscript path already makes (see "A subscript
on the result — and on the name a `(P)` names"): a `(P)` inner is a
parameter reference and every other inner is a value, so `${#${(P)h}}` is
`${#arr}` and not the width of the text its fields join to. See #1639,
#1638 and #1543.

**The resolved text is a full reference**, so every subscript this shell
answers on a name it answers here. Measured 2026-09-12 on zsh 5.9.2 with
`x=(p q r)`, `s=abc` and `a=(P Q)`:

    v='x[@]';    ${(P)v}   →  p q r  a whole-array subscript
    v='x[*]';    ${(P)v}   →  p q r
    v='x[1,2]';  ${(P)v}   →  p q    a range
    v='s[2]';    ${(P)v}   →  b      a character of a scalar
    v='s[2,4]';  ${(P)v}   →  bc     a range of one
    v='a[(r)Q]'; ${(P)v}   →  Q      a search
    v='a[(i)Q]'; ${(P)v}   →  2      and its index form
    v='x[1]';    ${(P)v}   →  p      the control, which already worked

The subscript is **live text and not a literal**, which is what the reading
being a parse buys: `i=2; v='x[$i]'` and `v='x[$(echo 2)]'` both read the
second element, so a substitution written into a resolved reference is
performed when the reference is read.

A quoted whole-array reference keeps its **fields**, exactly as `"${x[@]}"`
does, and the written `@` is what carries it rather than the list-ness:
`"${(P)v}"` is two fields for `v='x[@]'` and one joined field for `v='x[*]'`,
for `v='x[1,2]'` and for a `v` naming the array outright.

The nested spellings reach the same reference, with the text's own subscript
as the link *before* the outer one: `${#${(P)v}}` is 3 and `${${(P)v}[2]}` is
`q` for `v='x[@]'`.

`k` and `v` reach it too, which follows from the rule above about *which*
lookup answers them — a reference is the second lookup. With
`typeset -A tab=(k1 v1)`, `v='tab[k1]'; ${(kP)v}` is `k1` where `${(P)v}` is
`v1`, `v='tab[@]'; ${(kP)v}` is the keys, and `v='x[2]'; ${(kP)v}` is `2`,
the index an ordinary array reads that letter as. The rest of the group is
not the lookup's and acts on what came out: `${(UP)v}` is `Q`.

Taken apart by hand into a name plus one arithmetic index, every one of those
fell through to the plain-name lookup and found nothing — empty at status 0,
which is a plausible value for a real element (#1852).

**The index no element has resolves the whole base.** The base's *own*
subscript has one value that is not read as an index, and it is measured
rather than derived: with `n=(x y z)` and `x=(p q)`, `${(P)n[0]}` is `p q`
where `${n[0]}` beside it is empty, and `${(P)n[1-1]}` answers alike, so it
is the value the expression comes to and not the numeral. Every other
subscript naming nothing is no name — `${(P)n[4]}` and `${(P)n[-4]}` are
empty — and an **association** is outside it, `[0]` being a key there like
any other. A corner no script can depend on, reproduced because the
alternative is an empty at status 0.

### `(P)` beside an operator that assigns

`(P)` moves the whole expansion one step along, and that includes the
**assignment**: `${(P)x::=w}`, `${(P)x:=w}` and `${(P)x=w}` write the
parameter the base names, not the base. Measured on zsh 5.9.2, 2026-09-10:

    x=tgt; tgt=old; ${(P)x::=new}   →  new    tgt=new, x=tgt
    x=tgt; tgt=old; ${(P)x:=new}    →  old    the test is about tgt
    x=tgt; unset tgt; ${(P)x:=new}  →  new    tgt=new
    x=tgt; tgt=;    ${(P)x=new}     →  ``     `=` fires on unset alone
    x=tgt; tgt=old; ${(UP)x::=new}  →  NEW    tgt=new — the other letters
                                              still act on what is
                                              substituted, not on what is
                                              stored

An implementation reading the flag on the way out only answers `new` for
the first row too, and leaves it in `x` — plausible, at status 0.

**The resolved text is a parameter reference, not only a name.** A
subscript in it reaches one element, and the base's *own* subscript
belongs to the resolution rather than to the target:

    typeset -A m=(k old); n='m[k]'
    ${(P)n}                →  old
    ${(P)n::=new}          →  new    m[k]=new
    a=(p q); i='a[2]'; ${(P)i::=Z}   →  Z    a is (p Z)
    arr=(tgt zz); ${(P)arr[1]::=new} →  new  tgt=new, arr unchanged

**An unset base is not the empty name.** With nothing to resolve the
assignment lands on the name as written, where a set-but-empty base is a
refusal:

    unset x; ${(P)x::=new}   →  new    and leaves `typeset x=new`
    x=;      ${(P)x::=new}   →  not an identifier: ``
    x='a b'; ${(P)x:=new}    →  not an identifier: a b
    x='#';   ${(P)x::=new}   →  not an identifier: #

The refusal ends the script at 1, and it is asked of **all three**
assigning operators here — where the direct spelling asks it of `::=`
alone (see "What this implementation does not match", and #1541 for why
that one is narrow).

This is the construct a plugin manager records file times with —
`.zi-get-mtime-into` is `: ${(P)2::="$(stat …)"}` called as
`.zi-get-mtime-into "$file" 'ZI[mtime-side]'` — and writing to the base
instead stored the time in a parameter *called* `2`. Every later `$2` in
a function called with one argument then read the leftover, a
"were two components given?" test answered yes, and eighteen autoloaded
functions were looked for in a directory assembled from the wrong halves
of a plugin id (#1672).

**What this shell does not carry here.** The *read* takes the resolved text
apart as the reference it is; this side still splits it into a name and one
arithmetic subscript. Measured:

| written | zsh 5.9.2 | here |
| --- | --- | --- |
| `x=(p q); v='x[@]'; ${(P)v::=Z}` | `x` is `Z` | `bad math expression: illegal character: @` |
| `a=(p q); v='a[(r)q]'; ${(P)v::=Z}` | `a` is `p Z` | `bad math expression: operator expected at` `q` |
| `x=(p q r); v='x[1,2]'; ${(P)v::=Z}` | `x` is `Z r` | **`p Z r`**, at status 0 |
| `s=abc; v='s[2]'; ${(P)v::=Z}` | `s` is `aZc` | the same |

Two of the four refuse loudly and the range is silent, which is what makes
it worth filing rather than leaving: #2169.

### What the corpus pins

The table above is measured, and the rows that hold the panel to it are
`param/expansion-flags-are-one-dialects` (`(U)`, and the three-way split
on the shells without the construct), `-split-and-join` (`(s)`, `(j)`),
`-quote-four-ways` (`(q)` through `(qqqq)`), `-split-at-newlines`
(`(f)`), `-at-keeps-array-fields` (`(@)`, against the IFS join a quoted
array otherwise gets), `-name-indirection` (`(P)`),
`-keys-and-values` (`(k)`, `(kv)`), `-run-after-the-operator` (the
ordering rule and `(P)`'s exception to it),
`param/expansion-flags-parameter-type` (`(t)`),
`param/expansion-flags-indirection-names-the-front-of-its-base` and
`-keeps-the-resolved-shape` (the two sections above),
`param/expansion-flags-indirection-assigns-through-the-name`,
`-assigns-to-an-element` and `-refuses-what-it-resolved-to` (the three
above), and `param/prompt-percent-names-the-script` (`(%)`).

The flags that order, count and unquote have rows of their own:
`param/the-ordering-flags-sort-a-list`, `-and-case`,
`param/the-unique-flag-is-not-a-sort`,
`param/the-index-is-an-ordering-key` (`(a)`),
`param/the-length-flags-count-characters-and-words` (`(c)`, `(w)`,
`(W)`), `param/the-unquoting-flag-removes-one-level` (`(Q)`), and two
rows for where the steps sit —
`param/a-length-is-taken-before-the-joining-and-the-split` and
`param/where-the-unquoting-and-ordering-steps-sit`.

The join at the head of a split has three of its own, because the rule
and its two exceptions are each defensible alone:
`param/the-fields-flag-skips-the-join-ahead-of-a-split` (`(@s)` and
`(@f)`, quoted and not, against the flagless spelling),
`param/the-fields-flag-skips-that-join-for-the-letter-splits-only` (a
named `j` separator, an `[@]` subscript and the name `@`, none of which
exempts anything) and
`param/the-fields-flag-does-not-skip-that-join-for-an-ifs-split` (`${(@)=a}`,
on an array whose holes only survive one of the two readings).

Minimal quoting has four of its own, and each is a pair of values rather
than one, because a single value cannot tell "quote only what needs it"
from "always quote" or from "never quote":
`param/minimal-quoting-quotes-only-what-needs-it` (`q` beside `q-`, on a
value that needs quoting and one that does not),
`param/minimal-quoting-writes-a-quote-with-a-backslash` (the character
single quotes cannot hold, and the empty value),
`param/minimal-quoting-leaves-a-control-byte-bare` (which unprintable
bytes are a reason to quote, which is almost none of them), and
`param/the-quoting-flags-read-a-tilde-by-position` (the two bytes that
are special only where a word starts, asked of `(q)` and `(q-)` together
because the table is shared and the `:q` modifier reads it too).

`(k)` is pinned with a **single** pair. zsh yields hash order for more
than one and does not promise it, so a row with two keys would record a
coin flip as evidence; the sorted order this implementation yields is
stated above and asserted in its own unit test rather than against the
panel.

### `(k)` and `(v)` read through a subscript

**The subscript selects which pairs; the letters say which half of each
is substituted.** They are two independent questions, and a plugin
manager depends on it: it writes `${(kv)OPTS[@]}` rather than
`${(kv)OPTS}`. Measured on zsh 5.9.2 with `typeset -A m=(a 1 b 2)`:

    ${(kv)m[@]}   →  a 1 b 2   the whole table, both halves
    ${(k)m[@]}    →  a b       the keys
    ${(v)m[@]}    →  1 2       the values, which is also `${m[@]}`
    ${(k)m[*]}    →  a b       the joining spelling selects the same pairs
    ${(kv)m[(I)a*]} → a 1      a search still says which half it wants

Reading the subscript alone and ignoring the letters answers with the
**values** whatever was written — half a list, at status 0, and
indistinguishable from a whole one (#1509).

A subscript naming **one key** is not symmetric with the whole-table
form, which is measured rather than tidy:

    ${(k)m[b]}    →  b         the key
    ${(kv)m[b]}   →  2         the value: `v` puts the other half back
    ${(v)m[b]}    →  2         the value
    ${(k)m[zz]}   →  ``        an absent key is nothing at all

An **ordinary** array reads the same letter as its *index*, which is a
different question with a different source and was recorded separately
(#1515). Measured 2026-09-12 on zsh 5.9.2 with `x=(p q r)`:

    ${(k)x[2]}      →  2      the subscript itself
    ${(k)x[1+1]}    →  2      evaluated, not the text
    ${(k)x[-1]}     →  3      a negative counted forward from the end
    ${(k)x[-9]}     →  -5     and not clamped when it reaches past the first
    ${(k)x[9]}      →  9      nor when it reaches past the last
    ${(k)x[0]}      →  0      the subscript no element has is itself
    ${(k)x[(r)q]}   →  2      a search is where it matched
    ${(k)x[(R)zz]}  →  0      and a miss is that letter's out-of-range index
    ${(k)x[(e)2]}   →  2      a group that selects nothing is still the index
    ${(k)x[1,2]}    →  `invalid subscript`, and the line ends at 1

So the reading is the subscript arithmetic-evaluated and normalized to a
forward index, and a range — which names a span rather than one
subscript — is refused outright. The wrong answer in every row is the
*element*, which is a plausible word at status 0.

Five shapes are outside it, each measured:

- `[@]` and `[*]` are the elements, unchanged.
- a **scalar** ignores the letter: `s=hello; ${(k)s[2]}` is `e`.
- a name holding nothing is nothing rather than an index —
  `${(k)nosuch[2]}` is empty, where `x=(); ${(k)x[2]}` is `2`.
- `v` beside `k` puts the element back, as it does over a table, and a
  `(P)` moves both letters to the indirection.
- a **chain** is left alone. With `a=(pqrs t)` the shell answers empty
  for `${(k)a[1][2]}`, `2` for `${(k)a[2][1]}` and `1` for
  `${(k)a[1][-1]}`, which is not monotonic in anything and so is an
  artifact rather than a rule to write down.

`${(kP)v}` with `v="x[2]"` is `2`: the letters belong to the lookup the
indirection makes, and that lookup reads a reference. See "What `(P)` reads
as the name, and what it comes to".

### The `(A)` flag is two halves

`(A)` is the one flag whose whole job is a **side effect**. Measured on
zsh 5.9.2:

| written | zsh 5.9.2 | the same without the `A` |
| --- | --- | --- |
| `v="a b"; ${(A)#v}` | `3` | `3` |
| `v="a b"; "${(A)v}"` | one field | one field |
| `v="a\|b"; ${(As:\|:)v}` | `a b` | `a b` |
| `v="a\|b"; ${(@Akons:\|:u)v}` | `a b` | `a b` |
| `v=abc; ${(AA)v}` | `abc` | `abc` |
| `${(A)nosuch:-x y}` | `x y`, and `nosuch` still unset | the same |
| `unset u; ${(A)u=x y}` | `x y`, and `u` is `typeset -a u=( 'x y' )` | `u` is a scalar |
| `unset u; ${(A)u::=x y}` | the same array | a scalar |
| `unset u; ${(AA)u=k v}` | `bad set of key/value pairs …` | assigns a scalar |

So the flag changes nothing at all unless the expansion carries an
assignment operator — `=`, `:=` or `::=` — and where it does, it changes
what kind of parameter the assignment leaves behind rather than what the
expansion substitutes.

**Both halves are answered here, and only one of them by doing
something.** Every reading in the first block is carried by leaving the
value alone, which is what the six lines a real plugin manager writes
need; an expansion that assigns is refused by name —
`${(A)u=x y}: the (A) expansion flag is not implemented for an
assignment` — because a scalar left where the script asked for an array
is read as empty by the first `${u[2]}` and by nothing before it.

The unit test asserts the first block as *pairs* rather than as recorded
values: each row runs the expansion with the flag and without it and
requires the two to agree. A recorded value would pass for an `(A)` that
quietly did something, as long as somebody had written down what it did.

`(a)` and `(A)` differ only in case and mean unrelated things — one
orders a list by its index — which is why they are described apart.

### `(p)` is a modifier, and it modifies the flags behind it

`(p)` produces nothing of its own. It changes how the flags **after** it
read their arguments, which is the whole of what it does — so it is
listed with the flags and behaves like an option on them.

Measured 2026-09-07 on zsh 5.9.2 with `a=(x y)`:

| written | result | why |
| --- | --- | --- |
| `${(j:\n:)a}` | `x\ny` | no `p`: the argument is a backslash and an `n` |
| `${(pj:\n:)a}` | `x`, newline, `y` | the escape is read |
| `${(j:\n:p)a}` | `x\ny` | a `p` **behind** the flag modifies nothing |
| `${(ppj:\n:)a}` | `x`, newline, `y` | doubling adds nothing; it is not parity |
| `${(p)v}` | the value | `p` with no argument-taking flag is a no-op |
| `${(pj:$s:s:$s:)a}` | two fields | it reaches every such flag after it |

**The escape set is `print`'s, not `echo`'s.** The two differ inside one
shell, and the difference is measurable here:

| written | result |
| --- | --- |
| `${(pj:\101:)a}` | `xAy` — octal with no leading zero, up to three digits |
| `${(pj:\0101:)a}` | `x`, `\010`, `1`, `y` — so three digits is the limit |
| `${(pj:\x41:)a}` | `xAy` |
| `${(pj:\u0041:)a}` | `xAy` |
| `${(pj:\M-a:)a}` | the high bit set on `a` |
| `${(pj:\C-a:)a}` | `\001` |
| `${(pj:\M-\C-a:)a}` | `\201` — the pair applied in turn |
| `${(pj:\q:)a}` | `xqy` — an escape it does not know loses its backslash |
| `${(pj:x\:)a}` | `xx\y` — a trailing backslash is a backslash |

with **one exception, and it is the one `print` is asked about most**:

| written | `print` | a flag argument |
| --- | --- | --- |
| `A\cB` | `A` — the rest of the output is dropped | `AcB` — an unknown escape |
| `\\c` | `\c` | `\c` |

So `\c` is the only place the two readings part, and this implementation
takes the one decoder and tells it which of the two it is rather than
keeping a second copy of forty escapes to hold the difference.

### And `(p)` substitutes an argument that is exactly `$name`

The second half of the flag, and it is narrow. Measured, with `s=-`:

| written | result |
| --- | --- |
| `${(pj:$s:)a}` | `x-y` |
| `${(j:$s:)a}` | `x$sy` — without `p`, nothing is substituted |
| `${(pj:A$s:)a}` | `xA$sy` — not the whole argument, so not substituted |
| `${(pj:$sA:)a}` | `x$sAy` |
| `${(pj:$s $s:)a}` | `x$s $sy` |
| `${(pj:$s\t:)a}` | `x$s`, tab, `y` — the escape is read, the name is not |
| `${(pj:\$s:)a}` | `x$sy` — which fixes the *order* |
| `${(pj:${s}:)a}` | `x${s}y` — no braces |
| `${(pj:$M[k]:)a}` | `x$M[k]y` — no subscript |
| `${(pj:$(echo -):)a}` | as written — no command substitution |
| `${(pj:~:)a}` | `x~y` — no tilde |
| `${(pj:$nosuch:)a}` | `x$nosuchy` — an unset **name** stays as written |
| `${(pj:$:)a}`, `${(pj:$#:)a}`, `${(pj:$@:)a}` | as written |

The `\$s` row is what settles the order: it decodes to `$s`, so a reading
that ran the escapes first and looked for a name second would substitute
there and the shell does not. So the two halves are **alternatives**: one
`$name` that resolves, else the escapes, never both — which the value
confirms from the other side, since `s='\n'; ${(pj:$s:)a}` is `x\ny` and
the substituted value is handed over unread.

A positional is the one parameter substituted whether or not it is there.
With `set -- P Q`: `${(pj:$2:)a}` joins on `Q`, `${(pj:$9:)a}` joins on
nothing at all, and `$0` answers with the script's name — where the unset
*name* two rows above stays as the seven characters it was written with.

### Where `(p)` reaches the padding pair

`(l)` and `(r)` take arguments `(p)` reads, and both fills go through it:
`${(pl.3..\n.)}` is three newlines where `${(l.3..\n.)}` is the two
characters `\n` repeated. The rule about *position* is the join
separator's unchanged — a `p` written behind the padding letter reaches
nothing, so `${(l:4::\t:p)v}` pads with a backslash and a `t`.

The width is not read this way and needs no flag: it is an arithmetic
expression, and a `$name` in an expression is a name there already. So
`${(pl:$n::0:)v}` and `${(l:$n::0:)v}` are the same field, and the `p`
in the first is doing its work on the fill beside it.

`(z)` is not one of them and never was, though the two were listed
together while both were unbuilt: `(p)` reads *arguments* and `(z)` splits
a *value*. Building one did not move the other, and `(z)` was built from
the capital's side instead — see below.

### `(z)` and `(Z:opts:)` — split a value the way the shell splits a line

**Two spellings, one flag.** The capital is the one with an argument, and
the argument is **option letters** rather than a separator: `${(Z+Cn+)v}`,
which is what a plugin manager's message formatter writes for every
diagnostic it prints. The lower case takes no argument at all and is the
same split with no letters set — `${(z)v}` — which is the spelling a
plugin manager's *extension hooks* write, four times per hook. The
delimiters are part of the syntax and are the same set the `s` and `j`
separators take, matched pairs included — `(Z:n:)`, `(Z+n+)`, `(Z[n])`,
`(Z<n>)` all read the same option.

**Three option letters exist**, confirmed by trying the whole alphabet
against the shell one letter at a time. Every other character is an error
*in the flags*, at the letter's own position — `${(Z:x:)v}` is `error in
flags near position 6` — which is a different complaint from the by-name
refusal an unbuilt flag letter gets, and deliberately so: one says the
construct is not spelled that way and the other says this implementation
has not built it.

| letters | `a # hi` + newline + `b` splits as |
| --- | --- |
| none | `a` `#` `hi` `;` `b` |
| `c` | `a` `# hi` `;` `b` |
| `C` | `a` `;` `b` |
| `n` | `a` `#` `hi` `b` |
| `Cn` | `a` `b` |

- **`n`** makes an unquoted newline ordinary whitespace. Without it each
  newline is a word of its own and the word is `;` — the terminator's
  spelling, not the newline's — one per newline, at either edge:
  `$'a\n\n\nb'` is five words.
- **`c`** keeps a comment as one word, `#` and all, running to the end of
  the line and not taking the newline with it.
- **`C`** drops the comment entirely.
- **With neither, there are no comments at all.** `a # hi` is three words
  and `a #hi` is two — the same answer an interactive shell without
  `interactive_comments` gives, which is what "split like the shell" means
  for a *value* somebody typed. This is the lexical decision the previous
  note here left standing; it is taken now, as `syntax.CommentMode`, a
  per-call mode on the lexer rather than a `Dialect` value, because it is
  a property of *this read* and not of the language.
- **`c` wins where both are written**, and it is not the last letter that
  decides: `${(Z+cC+)v}` and `${(Z+Cc+)v}` on `a # hi` are both `a` and
  `# hi`.
- **Inside a substitution a kept comment and a skipped one behave alike**,
  and both swallow the closing parenthesis: `v='a $(b # c) d'` is two
  words under `c` and under `C`, the second being `$(b # c) d` with the
  `$(` never closed, and three words with neither letter.

**An empty option list turns the flag off**, which is the opposite of the
reading "the same split, with options" invites: `${(Z::)v}` on `a  b` is
the value unchanged, both blanks and all, where `${(z)v}` is `a b`. So for
the *capital* the letter being present is not the question — the argument
being non-empty is. The lower case has no argument to be empty and always
splits, and that is the only thing the two spellings disagree about.

**`z` takes no argument**, and a delimiter behind it is an error in the
flags at the delimiter: `${(z::)v}` and `${(z:x:)v}` are both `error in
flags near position 5`. So `${(Z::)v}` and `${(z::)v}` — the same
characters one letter apart — fail and succeed at opposite ends of the
same shape.

**A group may write both, and then the written order decides the
letters.** A `Z` argument *adds* its letters to whatever stands in front
of it; a `z` *clears* them. Measured on `$'a # h\nb'`, where `C` and `n`
each change the answer visibly:

| group | words | says |
| --- | --- | --- |
| `${(Z+C+Z+n+)v}` | `a` `b` | two arguments union |
| `${(Z+n+Z+C+)v}` | `a` `b` | in either order |
| `${(zZ+n+)v}` | `a` `#` `h` `b` | an argument behind `z` counts |
| `${(Z+n+z)v}` | `a` `#` `h` `;` `b` | a `z` behind one clears it |
| `${(Z+C+zZ+n+)v}` | `a` `#` `h` `b` | clearing only what precedes it |
| `${(Z+n+Z::)v}` | `a` `#` `h` `b` | an empty argument adds nothing |
| `${(zZ::)v}` | `a` `#` `h` `;` `b` | nor does it turn the split off |

The union is why the letters are accumulated while the flag group is
*read* rather than looked up afterwards: by the time the interpreter has
`Flags` the arguments are stripped, so where a `z` stood relative to a `Z`
argument is no longer knowable. Keeping only the last argument — which is
what one assignment per `Z` came to — answered the first row with the
comment kept, at status 0.

**The words are the source they were written as**, quotes included:
`a 'b c' d` is `a`, `'b c'`, `d`, and `a b\ c d` keeps the backslash.
Operators are words of their own, substitutions are one word each and are
never run, a here-document's body is never read (`a <<EOF` and the lines
under it are six ordinary words), and input that ends inside a quote or a
substitution is no error at all — the rest of the text is the last word.
A value that is nothing but blanks, or nothing but a comment that `C`
dropped, is one empty field in quotes and none without them, which is the
edge rule `(f)` and `(s)` already follow.

**Where the split sits** is measured from both sides and it is *not* with
the other splits at rule 11:

| probe | zsh 5.9.2 | if it split at rule 11 |
| --- | --- | --- |
| `v="a\|b"; "${(Z+n+q)v}"` | one field, `a\|b` | three fields |
| `v="'a\|b'"; "${(Z+n+Q)v}"` | three fields, `a` `\|` `b` | one field |
| `v="b  a"; "${(oZ+n+)v}"` | `a b` | — |

So it runs after the quoting flags and before the ordering step. `(f)` and
`(s)` compose with it rather than racing it — each field they make is then
read as a command line of its own — and a length is still taken before it,
so `${(Z+n+)#v}` on `a b  c` is 6 and not 3.

**An array is not joined before it**, which is where it parts company with
every other split flag. Measured with `a=('"x' 'y"')`:

| probe | zsh 5.9.2 |
| --- | --- |
| `${(Z+n+)a}` | `"x` `y"` — each element read on its own |
| `"${(Z+n+)a}"` | `"x y"` — the join quoting itself asks for |
| `"${(@Z+n+)a}"` | `"x` `y"` — that join declined again |
| `${(s.:.)b}` on `("p:q" "r:s")` | `p` `q r` `s` — joined first |

So only the *quoted* join reaches it, and the forced join a split normally
asks for does not.

**The empty field belongs to the expansion, not to any word in it.** An
element with no shell words in it contributes none — `a=('' x)` under
`"${(@Z+n+)a}"` is the single field `x` — while a result with nothing in it
anywhere is one empty field: `a=()` under the same spelling is 1, where
`"${(@)a}"` on that array is 0.

**The split is this tree's own reader and nothing else.** "Split like the
shell would" already has an answer here, and a second scanner beside it would
agree on `a b` and part company over `a"b c"d`, `$(f x)`, `a#b` and every
other place a word boundary is not a blank. `syntax.ShellWords` keeps the
source each token covers — the source rather than `Token.Text`, because
`TokArithCmd` carries the expression with its parentheses already stripped.

One boundary is spelled back rather than reported: a **one-digit** file
descriptor joins the redirection operator it was written against, so
`2>&1` is `2>&` and `1`. Measured, and one digit only — `22>&1` is `22`,
`>&`, `1`, and `{v}> f` is `{v}`, `>`, `f`. Two of those three are already
answered before the rule: an IO number is only one when it is *adjacent*,
so a spaced digit never reaches it, and a dialect without multi-digit
descriptors reads `22` as an ordinary word. The width test is carrying the
named descriptor and a multi-digit one wherever a dialect has them.

**The reader is the `syntax.Parser`, not the lexer alone**, and that is
forced by one construct. A `(` that *starts* a token belongs to the word
where an argument may stand — `a (b c) d` is three words and not six — and
command position is the whole of the difference: `(b c) d` and `a; (b c) d`
split the parenthesis off. `Lexer.inArgument` is the flag, and `parseSimple`
is what sets it, because deciding it needs to know that `a="x"` is an
assignment and that `then` is a keyword, neither of which a token stream
says.

A state machine inside the splitter is the alternative and it was measured
rather than dismissed. It answers `a="x" (b c)` and `if a; then (b c)` right
only by restating the assignment table and the reserved-word table, and even
then it has three more of the parser's rules left to restate — measured on
zsh 5.9.2, `repeat 2 (b c)`, `coproc (b c)` and `foreach x (a b)` all split
the parenthesis off, while `(a) (b)` does not. Each one is a rule that
already lives in one place.

So the splitter builds a Parser, records the tokens the lexer hands it and
**throws the tree and any error away**. Discarding both is what keeps it a
split of a *value*: text that ends inside a quote is not a failure here, and
text that is not a program at all still comes back as the words it was
written as. Where the parser stops early the rest is lexed plainly and
appended — past the point the grammar lost track there is no position to
know, and the shell with the flag is no more definite there either.

One thing the parser has to be stopped from doing: a `<<` must not claim the
lines after it. A body is input, and a splitter reading a value may not
consume it — measured, `a <<EOF`, a body line and the delimiter come back as
six separate words — so `Lexer.noHeredocBodies` drops the queue rather than
the parser being taught about it.

**Four rows still differ, and all four are that shell's splitter reporting
argument position where its own grammar has a command.** Measured 2026-09-11
on zsh 5.9.2:

| value | zsh 5.9.2 | here |
| --- | --- | --- |
| `(a) (b)` | `(` `a` `)` `(b)` | `(` `a` `)` `(` `b` `)` |
| `function f { (b c) }` | `function` `f` `{` `(b c)` `}` | … `{` `(` `b` `c` `)` `}` |
| `foreach x (a b) (c d)` | `foreach` `x` `(` `a` `b` `)` `(c d)` | … `(` `c` `d` `)` |
| `case x in (a) echo hi;; esac` | `case` `x` `in` `(a)` … | `case` `x` `in` `(` `a` `)` … |

A second subshell after the first is a command in that shell as much as the
first one was, and a `case` arm's own parenthesis is not a pattern group; the
splitter says otherwise in both. Following it would mean copying an
inconsistency rather than a rule (#1514).

### `(g:opts:)` — read the value's backslash escapes

The vendor manual gives it in five lines: process escapes like `echo` when
no option is given, `o` makes octal escapes need no leading zero, `c` reads
`^X`, `e` reads `\M-t` and its family like `print`, and in none of the
readings is `\c` interpreted. The letters are additive over a base, and the
base is *this shell's* `echo` rather than any `echo` — which matters,
because "like the echo builtin" is a claim about a builtin that differs
between shells.

Measured 2026-09-09 on zsh 5.9.2:

| value | `${(g::)v}` | `${(g:o:)v}` | `${(g:e:)v}` | `${(g:c:)v}` |
| --- | --- | --- | --- | --- |
| `X\tY` | tab | tab | tab | tab |
| `X\eY` | escape | escape | escape | escape |
| `X\EY` | `X\EY` | `X\EY` | escape | `X\EY` |
| `X\x41Y` | `XAY` | `XAY` | `XAY` | `XAY` |
| `X\u0041Y` | `XAY` | `XAY` | `XAY` | `XAY` |
| `X\101Y` | `X\101Y` | `XAY` | `X\101Y` | `X\101Y` |
| `X\0101Y` | `XAY` | backspace `1` | `XAY` | `XAY` |
| `X\cY` | `X\cY` | `X\cY` | `XcY` | `X\cY` |
| `X\M-AY` | `X\M-AY` | `X\M-AY` | `0xc1` | `X\M-AY` |
| `X\C-AY` | `X\C-AY` | `X\C-AY` | `0x01` | `X\C-AY` |
| `X\qY` | `X\qY` | `X\qY` | `XqY` | `X\qY` |
| `X^XY` | `X^XY` | `X^XY` | `X^XY` | `0x18` |

**Two rows carry the whole of `o` and they are the two an implementation
gets wrong.** `\101` is text in three readings and `A` in the fourth, and
`\0101` is `A` in three and a backspace followed by a `1` in the fourth —
because `o` does not mean "octal as well", it means the leading zero is not
part of the escape, so the same three digits are read from a different
place. A `(g)` written as "process the escapes" answers both rows the base
way and looks right on the other ten.

`\c` is the row the manual is explicit about and the row a reader would
otherwise inherit from `print`: it never ends the output here, so under `e`
it is an escape this shell does not know and loses its backslash exactly as
`\q` does, and in the other readings it is two characters of text.

The rest of the details, each measured: `\M-` and `\C-` take an optional
dash and nest (`\M-\C-A` is 0x81, `\MA` is `\M-A`); `\C-?` is delete and
every other target keeps its low five bits, so `\C-@` is NUL and `\C-a` and
`\C-A` are both 1; a caret is a target for `\M-` when `c` is given
(`\M-^A` is 0x81) and *not* for `\C-`, which takes the `^` itself
(`\C-^A` is 0x1e then an `A`); `\x` takes up to two hex digits and `\u` and
`\U` up to four and eight, and each with none at all is a NUL; an octal
value above 255 is truncated to a byte; and a trailing backslash is a
backslash.

The **argument's letters are the grammar's**, exactly as `(Z)`'s are: a
letter outside `oec` is `error in flags near position N` at the letter
rather than a refusal when the expansion is reached, and the flag with no
argument at all errors at the character that arrived instead. Two `g`
arguments **union** — `${(g:o:g::)v}` on `X\101Y` is `XAY`, so the empty
second argument takes nothing away — which is the same rule `(Z)`'s
arguments follow.

Where the reading runs is rule 13, and the manual gives the half-step too:
"first any replacements from the `(g)` flag are performed, then any
prompt-style formatting from the `(%)` family". Measured from both sides,
because a rule list is a claim to check:

| probe | zsh 5.9.2 | if the reading ran first |
| --- | --- | --- |
| `v='A\TB'; ${(Lg::)v}` | a real tab | `a\tb` |
| `v='a\tb'; ${(Ug::)v}` | `A\TB` | `A<TAB>B` |
| `v='a\tb'; ${(qg::)v}` | `a$'\t'b` | `a\\tb` |

The first two are the discriminating pair and they are a pair on purpose:
lowering `A\TB` *makes* an escape the reader can use and raising `a\tb`
*destroys* one, so a reading placed before the case conversion answers both
the other way round. Neither order of the letters in the group changes it.

The reading itself is a measurement about one shell, and that shell already
has it for `echo`, for `print` and for the `(p)` flag's arguments — so the
substrate asks rather than answers. `interp.Runner.SetExpansionEscapes`
installs it, `dialect/zsh/print.go` is the one decoder all three readings
share with a struct of four booleans saying which is which, and a runner
nobody told refuses `(g)` by name. That refusal matters more than `(p)`'s:
a `(g)` read as a no-op is *right* for every value with no backslash in it,
so it would survive the first thing anyone tried and be wrong at status 0
wherever it mattered.

Found in the wild: powerlevel10k's `_p9k_init_params` reads
`POWERLEVEL9K_BATTERY_STAGES` with `${(g::)…}`.

### `(e)` — read the result again as shell text

Rule 21: "any `(e)` flag is applied to the value, forcing it to be
re-examined for new parameter substitutions, but also for command and
arithmetic substitutions". That sentence leaves two questions open, and
both were measured on zsh 5.9.2, 2026-09-09.

**How often.** Once. With `inner='$deeper'` and `deeper=bottom`,
`v='$inner'; ${(e)v}` is `$deeper` and not `bottom`. Looping is the
plausible reading and it agrees with this one on every value that resolves
in a single step, which is nearly all of them — so the row is here rather
than assumed.

**What happens to the text around the substitutions.** Not a word
expansion. With `d=DD` and `arr=(p q r)`:

| value | `${(e)v}` | says |
| --- | --- | --- |
| `~` | `~` | no tilde expansion |
| `a*` | `a*`, one field | no filename generation |
| `{x,y}` | `{x,y}` | no brace expansion |
| `'$d'` | `'DD'` | a quote is text and does not protect |
| `\$d` | `$d` | a backslash before a `$` does |
| `\\$d` | `\DD` | and before another backslash is one backslash |
| `a\tb` | `a\tb` | before anything else it is text, both of it |
| `a\"b` | `a\"b` | the double quote included |
| `$` | `$` | a `$` with nothing usable after it is a `$` |
| `$arr` | `p q r`, three fields | an array reference yields fields |
| `$(echo a; echo b)` | two fields | a command substitution's result splits |

The backslash rule there is exactly a **here-document body's**: an escape
only in front of `$`, a backtick, another backslash and a newline. So the
implementation is the here-document reader with the substitutions marked
unquoted, rather than a second expander written to the same description.

The two halves were confirmed against the options that move them, which is
what says they are the ordinary axes rather than rules of this flag's:
under `SH_WORD_SPLIT` the scalar `sp='a b'` becomes two fields here, and
under `GLOB_SUBST` an `a*` in the *result* matches — the enclosing
expansion's question, asked in the ordinary place.

Where it runs is last of the value transformations, later even than the
ordering:

| probe | zsh 5.9.2 | if the reading ran first |
| --- | --- | --- |
| `d=DD; v='$d'; ${(eU)v}` | empty | `DD` |
| `sp='a b'; v='$sp'; ${(es: :)v}` | one field, `a b` | two fields |
| `sq=hi; v='$sq'; ${(eq)v}` | `$sq` | `hi` quoted |

The first is the sharpest: the case conversion made `$D`, which names
nothing, so the answer is empty rather than the value a reader expects.

**Rule 23's rejoin is per word**, which is the half that has to be measured
rather than read. Where the expansion must come to a single word, the
fields *one* word produced go back together with IFS's first character and
the words the pipeline already held stay apart. With `arr=(p q r)`,
`z='$arr'`, `u='$arr:$arr'` and `v='$arr $arr'`:

| probe | fields |
| --- | --- |
| `"${(e)z}"` | 1 — `p q r` |
| `"${(@e)z}"` | 3 |
| `${(e)z}` | 3 |
| `"${(es.:.)u}"` | 2 — `p q r`, `p q r` |
| `"${(e)=v}"` | 2 — the same |
| `"${(@es.:.)u}"` | 6 |
| `IFS=-; "${(e)z}"` | 1 — `p-q-r` |
| `"${(ej:-:)z}"` | 1 — `p q r` |

Rows four and five are the discriminating ones: a rejoin over the whole
result answers both with one field, and one that never joined answers the
first with three. The last row is worth keeping because rule 5's join takes
the `j` separator and this one does not.

A failure inside the re-read text is a failure of the expansion — an
expansion the dialect cannot read, and `$((1/0))` a division by zero, both
abandon the command — which needs no code of its own, the text going through
the same parser and the same evaluator.

An expansion left *unterminated* is one of them, and it was the last one to
be: `v='x${'` used to come back as the literal `x` plus an expansion whose
name was whatever followed, because `HeredocSpans` returned spans and no
error. Where the dialect refuses an empty name — bash, dash — that still
failed, loudly and for the wrong reason; where it reads `${}` as empty the
whole thing was silently nothing. The reader now reports the fact the lexer
already held, so every route through it refuses the text (#1653).

The same reader serves a here-document body, which is where the fault was
reachable with no flag at all. Measured 2026-09-10 on `cat <<EOF` / `x${` /
`EOF`:

| | body that runs out inside `${` |
| --- | --- |
| bash 5.3.15 | `unexpected EOF while looking for matching }` |
| bash 3.2.57 | `x${: bad substitution` |
| dash | `Syntax error: Missing '}'` |
| ksh93 | `syntax error at line 2: 'end of file' unexpected` |
| zsh 5.9.2 | `bad substitution` |

All five refuse and none of them writes the body; the line after the
redirection still runs, so the refusal is the command's rather than the
shell's.

**The wording is the dialect's unmatched-`${` sentence**, which is not what
zsh says on the re-read road: there it answers `bad substitution` where the
scan got no further than a name — `${`, `${a`, `${a[1]`, `${#a` — and
`closing brace expected` once it reached an operator — `${a:`, `${a-`,
`${a%`, `${a/x/y`. Ours says the second everywhere. Telling the two apart
needs the *parser's* table of operators inside the lexer, for one shell's
wording on one route, and that is a second copy of knowledge that already
lives in one place. Recorded rather than chased.

A body's quoting is not a quote. `heredocSpans` marks every span
double-quoted so that nothing it produces is field-split, and the
unmatched-`${` report used to blame that mark — `unmatched "` about a text
with no quote character in it. `Lexer.inRawBody` is what keeps the mark
saying "do not split" without letting it stand in for a character somebody
wrote.

**A value that names its own expansion is bounded here rather than
followed.** `v='${(e)v}'` re-reads text asking for the same expansion again,
and the shell does not terminate on it — measured, it spins until it is
killed. There is no answer to imitate, and the two candidates for what to do
instead are a hang and a stack overflow; the second is worse than useless in
a library, where it takes the embedding program down with it. So the depth
is bounded at the same 32 arithmetic keeps for `x=x` and the refusal says
`nested too deeply`. A nesting that means something — `l1='$x';
l2='${(e)l1}'; ${(e)l2}` — is two levels and nowhere near it.

Found in the wild: powerlevel10k's `_p9k_must_init` builds a pattern of
`$…` references and evaluates it with `${(e)_p9k__param_pat}` to make the
signature it compares against.

### What this implementation refuses

Flags zsh has and this slice does not — `(t)`, `(D)`, and the rest of the
alphabet, plus `(q+)` (#1530), the signed-numeric sort flag `(-)` — which
is every `-` that a `q` did not eat, #1531 — and
`(qqq…)` beyond four — are refused at run time naming the flag, with the
same fatal shape as an unrecognized one. Refusing loudly is the honest
answer where imitating would answer wrong, and the refusal is asserted
whole rather than assumed: `TestTheUnbuiltFlagsAreStillRefusedByName`
holds it as each letter is built, because a letter added to the
implemented set is a letter taken out of the guarantee that let this list
be enumerated exactly.

One of them is measured rather than refused, and the measurement is here so
the next change starts from it:

- **`(A)`** is carried, and the half of it that is carried is the half
  that does nothing. See "The `(A)` flag is two halves" below.
- **`(m)`** is carried for the length operator and refused beside the
  padding pair, naming the composition rather than the letter (#2119).
  See the width rows under the flag table above for both halves.

`(z)` was on this list and is built (#1547), folded into the capital rather
than answered beside it — see the split's own section above. `(e)` and
`(g:opts:)` were on it too and are built (#1620); their sections follow.

### `(~)` is a modifier too, and it is not `${~name}`

`(~)` inside the parentheses looks like the flag-group spelling of the
tilde in `${~name}` and is not one. The written tilde makes the **whole
substituted string** a pattern; `(~)` marks the **string arguments of the
flags written behind it in the same group**, so what goes live is the
separator the group *inserts* and not the value it stands between. The
vendor manual (`zshexpn(1)`, Parameter Expansion Flags) says exactly that,
and the measurements below are what fixed the reading here.

Measured 2026-09-08 on zsh 5.9.2 with `a=('?' 'x')`:

| probe | result | says |
| --- | --- | --- |
| `[[ '?' = (${(~j.|.)a}) ]]` | true | the inserted `\|` is an alternation |
| `[[ q = (${(~j.|.)a}) ]]` | false | and the element's `?` is one character |
| `[[ '?\|x' = (${(j.|.)a}) ]]` | true | unmarked, the join is text |
| `b=('a*' 'x'); [[ abc = (${(~j.|.)b}) ]]` | false | an element stays literal |
| `p='a\|b'; [[ a = (${(~)p}) ]]` | false | with nothing behind it, nothing |
| `p='a\|b'; [[ a = (${~p}) ]]` | true | which is what the *written* one does |
| `t='~/zz'; ${(~)t}` | `~/zz` | and it has no tilde half either |

The order inside the group is load-bearing and the count is parity, which
is the same reading `${~~name}` has:

| group | marked | group | marked |
| --- | --- | --- | --- |
| `(~j.\|.)` | yes | `(~~j.\|.)` | no |
| `(U~j.\|.)` | yes | `(~~~j.\|.)` | yes |
| `(j.\|.~)` | no | `(~j.\|.~)` | yes |

Two more, both of which separate it from the written tilde again:

* **Quoting does not suppress it.** `[[ '?' = ("${(~j.|.)a}") ]]` is true,
  where `"${~g}"` has no answer of its own at all.
* **The mark is per expansion and does not travel in a value.**
  `x=${(~j.|.)a}; [[ '?' = $x ]]` is false — the variable holds the three
  characters `?|x`.

zsh joins at rule 10 and marks the separator, so every later step steps
over it. That mark is what this implementation has no way to carry, so it
holds the **join** back past those steps instead — which is the same
answer wherever a step rewrites text one character at a time, and a
different one where it does not:

| probe | result | held-back join |
| --- | --- | --- |
| `d=('a b' 'c'); ${(~qj.\|.)d}` | `a\ b\|c` | agrees — `(q)` is per character |
| `${(~Uj.\|.)d}` | `A B\|C` | agrees |
| `c=(x a); ${(~oj.\|.)c}` | `x\|a` | agrees — the sort has one word |
| `e=(p p q); ${(~uj.\|.)e}` | `p\|p\|q` | agrees, for the same reason |
| `${(~qqj.\|.)d}` | `'a b\|c'` | **differs** — `'a b'\|'c'` |
| `${(~q-j.\|.)d}` | `'a b\|c'` | **differs** — `'a b'\|c` |
| `k=("'a" "b'"); ${(~Qj.\|.)k}` | `a\|b` | **differs** |

So the join is performed in front of the ordering step, and the flags
whose step is not a per-character rewrite are refused rather than held
back through. `(q-)` is one of them and is the one the count does not
catch: it is spelled with a single `q`, like the `(q)` two rows above that
*is* carried, and it wraps a whole word where that one rewrites
characters. `${(oj.|.)c}` and `${(uj.|.)e}` without the tilde are
`x|a` and `p|p|q` too — the sort and the dedup have one word either way.

**What this implementation does not carry.** The marked separator is
refused by name in the compositions where a step between the join and the
escape would have to read the joined text back:

* `${(~j.|.)a:-z}` — `the (~) expansion flag is not implemented beside an
  operator`. Every operator, because the operator runs in front of the
  steps the join is held back past.
* `${(~fj.|.)a}`, `${(~j.|.)=a}` — `… beside a split`, which takes the
  separator out again.
* `${(~qqj.|.)d}`, `${(~q-j.|.)d}`, `${(~Qj.|.)k}`, `${(~%j.|.)m}` — `…
  beside the (qq) flag`, and the same for `(q-)`, `(Q)` and `(%)`. These
  are the steps the table above says a held-back join cannot reproduce:
  one pair of quotes round the join is not one pair round each word, and
  one level taken off the join is not one level off each word. A single
  `(q)` is a per-character rewrite and is carried — which is why `(q-)`
  has to be named separately rather than left to the count of `q`
  characters, being the one member of the family that has a single `q` and
  is not per-character.
* `${(~g::j.|.)a}`, `${(~ej.|.)a}` — `… beside the (g) flag`, and the same
  for `(e)`. Rule 13's escape reading is the `(%)` case again: it rewrites
  the joined text and would read the separator along with everything else.
  Rule 21's re-reading is the furthest of the lot from a per-character
  rewrite — it reads the joined text as shell source, where a separator is
  not a separator at all.
* `${(~s.-.)v}` — `… for the (s) separator`. A marked split separator does
  not split on a pattern; it stops matching at all as soon as it holds a
  character the shell marks, and which characters those are is neither the
  metacharacters nor the tokens — `<` splits where `>`, `-` and `~` do
  not. That is one implementation's internal marking rather than a
  behavior, and no script in reach writes it.
* `${(~l:5::*:)v}`, `${(~r:5::*:)v}` — `… for the (l) fill`, and the same
  for `(r)`. A marked fill is live where the rest of the result is
  escaped, and the padding is *measured* in units of the text it is laid
  into, so the fill would have to count as text and escape as none.
  `${(l:9::x:~j.|.)a}` — a mark behind the padding letter, reaching only
  the join — is `… beside a padding flag` for the mirror of that reason:
  the join is the one step whose result is part escaped and part not, and
  a width applied to it would count the escapes. The plugin manager with
  twenty-five `(~j…)` sites writes no padding flag in any of them.

`(~f)` is carried and does nothing, which is correct rather than a stub:
`(f)`'s separator is a newline and a newline has no metacharacters to
mark.

Every probe above puts the expansion inside a `( … )`, and that is not
decoration. A live `|` **outside** a group is an alternation in zsh too
and is matched as a character here — #1497, which the written `${~name}`
and `setopt globsubst` reach by the same route and which #1331 left open
when it fixed the group. So `[[ '?' = ${(~j.|.)a} ]]` is still false here
where the grouped spelling is true. The twenty-five sites in `~/.zi/bin`
that write this flag all write the group.

### Grammar

The group is read only when the dialect's `ParamExpansionFlags` is on;
elsewhere `${(…)…}` follows the bad-substitution split above — deferred to
run time via the `Bad` node everywhere but the parse-time dialect. The
parsed node carries the flag letters in order plus the two separators
(`SplitSep`, `JoinSep`) and the `Z` flag's option letters
(`ShellSplitOpts`); the printer writes the span back raw, so the
construct round-trips. An empty `ShellSplitOpts` with a `Z` in `Flags` is
meaningful and means the flag does nothing, which is measured.

The separators are kept **as written**, escapes and all, and that is what
lets `(p)` be answered entirely in the interpreter: the letter order in
`Flags` says which flags a `p` stands in front of, and the raw text is
still there to read them with. The escape set itself is a function the
dialect installs through `interp.Runner.SetFlagArgumentEscapes`, because
"the escapes" is not one answer even inside one shell — the same shell's
`echo` and `print` disagree — and a runner nobody told refuses `(p)` by
name rather than reading it as a no-op.

## A tilde at the front: `${~spec}` — zsh only

A `~` written between the `${` and the parameter makes the *result* of the
substitution eligible for tilde expansion and filename generation, whatever
the `GLOB_SUBST` option is set to. zsh alone has it. The other four call the
whole expansion unreadable, in the same three-way split every unreadable
expansion follows: bash 5.3, bash 3.2, bash-as-sh and dash say `bad
substitution` when the expansion is reached, ksh93 refuses it while reading
(`` `~' unexpected ``) — which `BadSubstitutionAtParseTime` already records.

Found in the wild, and this is why the construct is here rather than on a
list: `~/.zi/bin/zi.zsh:141-159` is eighteen of them in a row —

    ZI[HOME_DIR]=${~ZI[HOME_DIR]}
    …
    ZPFX=${~ZPFX}

— so a shell that refuses the substitution leaves eighteen variables *empty*
and every path the plugin manager later builds is rooted at nothing. The
construct is not a stylistic corner; refusing it corrupts state rather than
merely reporting.

All measurements below are of zsh 5.9.2 (Homebrew, arm64), taken 2026-09-06
against fixtures in a scratch directory with `HOME` pointed into it.

### What it does, measured

With `g='DIR/inn*.sh'` matching three files, `t='~/zz'` a directory, and
`typeset -A A; A[h]='~/zz'`:

| written | result |
| --- | --- |
| `${g}` | `DIR/inn*.sh` — the value, unchanged |
| `${~g}` | the three matched paths, as three words |
| `"${~g}"` | `DIR/inn*.sh` — quoting suppresses it |
| `${~t}` | `HOME/zz` |
| `${~t}/sub` | `HOME/zz/sub` |
| `${~A[h]}` | `HOME/zz` — a subscript is no obstacle |
| `${~1}`, `${~@}` | the positional parameters, each one expanded |

Three behaviors carry the construct and each is asserted rather than
inferred from the absence of an error:

- **Quoting suppresses it**, exactly as quoting suppresses ordinary filename
  generation. `"${~p}"` is the value unchanged.
- **The unquoted word form splits into as many words as the pattern
  matched.** `w=( ${~g} )` is a three-element array, not one holding a
  pattern.
- **`${~~p}` turns it back off**, which is how a nested use says "not here".

### The count is parity, and it overrides the option

`GLOB_SUBST` is the option that makes *every* expansion's result eligible.
The written tildes do not toggle it — they decide the answer outright, on the
parity of how many were written, measured under the option both ways:

| written | `unsetopt globsubst` | `setopt globsubst` |
| --- | --- | --- |
| `${g}` | the value | the matches |
| `${~g}` | the matches | the matches |
| `${~~g}` | the value | the value |
| `${~~~g}` | the matches | the matches |
| `${~~~~g}` | the value | the value |

So one tilde is "yes" and two are "no" from either starting point, and only
a `spec` with no tilde at all consults the option. The same table holds for
the tilde half: `${t}` is `~/zz` with the option off and `HOME/zz` with it
on, while `${~t}` is `HOME/zz` and `${~~t}` is `~/zz` regardless.

`GLOB_SUBST` moves `GlobExpansionResults` since #1734, so the zero-tilde row
follows the option as well as the dialect. That is the whole of the fix and
the reason it is one line: the option and the flag were always the same
question, and the flag was already an override of that axis — what was
missing was anything moving the axis itself.

### Where the tilde may be written

After the parenthesized flag group and before everything else. Measured:

| written | zsh |
| --- | --- |
| `${(U)~g}` | flags then tilde: read, uppercased, then matched |
| `${~(U)g}` | `bad substitution` — the group may not follow the tilde |
| `${(U)~#g}` | the *length*, so the tilde precedes `#` as well |
| `${#~g}` | `bad substitution` |
| `${~+x}` | `${+x}`, the is-it-set count, with the tilde applied to it — `${+x}` is a gap here, so both spellings are a `bad substitution` |
| `${~!x}` | `bad substitution`, as `${!x}` is here anyway |
| `${~}` | the empty string, no error — as `${}` is in this shell |

`^` and `=` share the position and are interchangeable with it — `${^~a}`
and `${~^a}` agree, `${=~g}` and `${~=g}` agree — and neither is in this
slice.

### It applies to the value the expansion came to

The operator runs first and the tilde marks what the operator produced, which
is the same ordering the flag group has:

    v='XDIR/inn*.sh'; ${~v#X}    →  the three matched paths
    ${~undef:-DIR/inn*.sh}       →  the three matched paths
    v='~/zz'; ${~#v}             →  4, the length of `~/zz`

The last one is the ordering seen from the other side: `#` has already
reduced the value to a number, and a number holds no tilde and no
metacharacter, so the flag has nothing left to do.

Tilde expansion runs before filename generation, on the *head* of the value
and nowhere else:

    v='~/*'      →  every entry of HOME
    v='a:~/zz'   →  `a:~/zz`, the tilde is not at the head
    v='~/zz ~/qq' split   → *both* directories: the value split first and
                             every field it produced is at the head of a
                             word of its own

A list expands elementwise, and each element is the head of its own word:
`a=('~/zz' '~/qq'); ${~a[@]}` is both expanded, while `X${~a[@]}` is
`X~/zz` and `HOME/qq` — the prefix took the first element out of head
position and left the second in it. **Field splitting makes the same
list**, which is measured and was recorded here the other way round until
it was: `setopt shwordsplit; v='~/zz ~/qq'; ${~v}` is both directories, and
`${=~v}` is both without the option. Splitting runs first and the fields it
made are the heads, so the two spellings of a list agree.

`~user` and a named directory (`hash -d`) both resolve in zsh; this
implementation leaves `~user` as written, the same limit its literal tilde
expansion already has, and has no named directories at all.

### The other half is the pattern half

Marking the value a pattern is the same question `GlobExpansionResults`
already answers, so the flag reaches every place that answer is read, and
that is measured rather than assumed:

    p='a*'; [[ abc == ${~p} ]]        →  true   (${p} alone is false)
    p='a*'; case abc in ${~p})        →  matches
    v=abc; p='a*'; ${v#${~p}}         →  `bc`   (${p} alone leaves `abc`)

And the run-time option still wins over it, because the option is about the
filesystem pass and not about what the word is: `setopt noglob; ${~g}` is
the value unchanged. A pattern matching nothing is `no matches found` and
fatal, which is `GlobNoMatchIsError`; under `nullglob` the word is dropped.

### Grammar

The tilde run is read only when the dialect's `ParamTildeFlag` is on;
elsewhere a leading `~` is not a name and the expansion follows the
bad-substitution split above. The parsed node carries `TildeFlags`, the
number of tildes written, so parity is the interpreter's to take and the
printer keeps writing the span back raw — the construct round-trips as
source text.

`ParamTildeFlag` and `ParamCaseChange` never coexist in a dialect, and would
not collide if they did: bash's case-toggle `~` follows the name (`${x~}`)
and this one precedes it.

### What the corpus pins

`param/the-tilde-flag-is-one-dialects` (the three-way split),
`-quoting-suppresses-it`, `-doubled-turns-it-off`, `-expands-a-tilde`
and `-marks-a-pattern-operand`.

### What this implementation does not match

Two divergences, both inherited from where this implementation does its
literal tilde expansion — the first span of a word — rather than from the
flag:

- `""${~t}` and `${empty}${~t}` expand here, because nothing was
  accumulated in front of them, which agrees with zsh. But `x${~t}` does
  not, and neither does zsh's; the disagreement is narrower than it looks
  and no measured case is missed by it.
- `q=a:${~t}` is `a:HOME/zz` in zsh: a substituted tilde is eligible in
  every position a written one would be, and an assignment's colons are
  such a position. Here the colon rule (`expandColonTildes`) reads literal
  spans only, so the substituted tilde stays. `q=a:~/zz` written out is
  correct in both.

Separately, `${}` and `x${}y` are the empty string in zsh and a `bad
substitution` here. `${~}` is right because the tilde relaxes the empty-name
rule the way a flag group does; `${}` is a gap of its own. `${+x}` — the
is-it-set count — has its own section below, and `${~+x}` reads.

## A `+` at the front: `${+name}` — zsh only

A `+` written between the `${` and the parameter asks whether the parameter
is **set**, and substitutes `1` or `0` rather than its value. zsh alone has
it. The other four call the whole expansion unreadable, in the same
three-way split every unreadable expansion follows: bash 5.3, bash 3.2,
bash-as-sh and dash say `bad substitution` when the expansion is reached,
ksh93 refuses it while reading (`` `+' unexpected ``) — which
`BadSubstitutionAtParseTime` already records.

**It never fails**, and that is the whole reason a script writes it: it is
the guard that stands in front of everything else, so a shell that raises an
error where the answer is `0` turns every guard into a hard stop rather than
a false.

Found in the wild, and this is the scale of it: `${+` appears **138 times**
in `~/.zi/bin/zi.zsh`, 42 of them `${+functions[…]}` — an order of magnitude
more than any other single construct that file gated on. The 42 are the
spelling almost every caller uses to read `$functions`, so a shell with that
parameter and without this flag cannot be asked the question the parameter
exists to answer.

All measurements below are of zsh 5.9.2 (Homebrew, arm64), taken 2026-09-06.

### What it answers

| written | result |
| --- | --- |
| `v=1; ${+v}` | `1` |
| `${+NOPE}` | `0` |
| `v=1; unset v; ${+v}` | `0` |
| `v=; ${+v}` | `1` — **set-ness is not emptiness** |
| `typeset v` (no value) | `1` |
| `a=(x y z); ${+a}` | `1`, one field — not the element count and not the join |
| `typeset -A m; ${+m}` | `1` |
| `${+a[2]}`, `${+a[9]}` | `1`, `0` — an element rather than the name |
| `${+m[k]}`, `${+m[nope]}` | `1`, `0` |
| `${+functions[f]}` | `1` when the function is defined |
| `${+aliases[x]}`, `${+commands[ls]}`, `${+options[xtrace]}` | the same question of each special table |
| `set -- A B; ${+0} ${+1} ${+2} ${+3}` | `1 1 1 0` |
| `${+10}` | `0` with two parameters — a positional may be several digits |

`v=; ${+v}` is the row that separates this from `${v:+1}`, which fires on
the empty value, and from `${v+1}`, which agrees here and disagrees
elsewhere: an implementation that reused either would get most of the table
right.

### It does not trip `set -u`

    set -u; echo ${+NOPE}     →  0, and the shell carries on
    set -u; echo ${NOPE}      →  `NOPE: parameter not set`, fatal

Asking whether a name exists without reading it is the whole of the
construct. The same holds through a flag group: `set -u; v=nosuchvar;
${(P)+v}` is `0`.

### The name it may carry is a name or a positional, and nothing else

Measured by asking for each, and the answer is not the one "is it set"
suggests:

| written | zsh |
| --- | --- |
| `${+x}`, `${+_x1}`, `${+0}`, `${+10}` | the count |
| `${+@}` `${+*}` `${+#}` `${+?}` `${+$}` `${+!}` `${+-}` | `bad substitution` |
| `${+}` | `bad substitution` — and `${~}` is the empty string, so this flag does *not* relax the empty name the way the tilde and the group do |
| `${(U)+}`, `${~+}` | `bad substitution` as well |
| `${++x}` | `bad substitution` — there is no doubled spelling, so no parity |
| `${+${x}}` | `bad substitution` |

The refusal is **deferred to the run**, like every other unreadable
expansion in this dialect: `if false; then echo ${+?}; fi` is silent.

### Where the `+` may be written

After the parenthesized flag group and after the tilde run, and in front of
everything else:

| written | zsh |
| --- | --- |
| `${~+x}` | the count, with the tilde applied to it |
| `${+~x}` | `bad substitution` — the tilde precedes, never follows |
| `${=+x}` | the count; `${=x}`'s flag shares the tilde's slot and so precedes this one too |
| `${+=x}` | `bad substitution` |
| `${^+x}` | the count — the third character of that slot, and not a flag this implementation has |
| `${+^x}` | `bad substitution` |
| `${~=+x}`, `${=~+x}` | the count either way, since the slot's characters are interchangeable within it |
| `${(U)+x}` | the count; a value transformation has nothing to transform |
| `${+(U)x}` | `bad substitution` |
| `${+#x}` | `bad substitution` — a length may not follow it |
| `${#+x}` | **not this construct at all**: the `#` is read first, so this is `$#` with `+x` as an alternate word, and it is `x` for any set `#` |

`(P)` is the one flag that reaches the answer, because it changes *which*
parameter is being asked about rather than what its value becomes:

    w=1; v=w;         ${(P)+v}  →  1
    v=nosuchvar;      ${(P)+v}  →  0     (${(P)v} is empty, ${+v} is 1)
    v=abc;            ${(U)+v}  →  1     (not an uppercased anything)
    a=(1 2); ${(j.-.)+a}        →  1

### Beside an operator it has no effect at all

Not the answer, and not a refusal either — the expansion is exactly what it
would have been without the `+`:

    v=abc;  ${+v#a}    →  bc
    v=abc;  ${+v:-x}   →  abc
    v=abc;  ${+v+y}    →  y
            ${+nope:-D}→  D
            ${+nope#a} →  the empty string
            ${+v=W}    →  W, and v is assigned
    v=abc;  ${+v[2]#a} →  b

So the count is the answer only for a plain reference. A length cannot
co-occur — `${+#v}` is refused while reading and `${#+v}` never carries the
flag — so the operator is the whole of the test.

### Grammar

The `+` is read only when the dialect's `ParamSetTestFlag` is on; elsewhere
a leading `+` is not a name and the expansion follows the bad-substitution
split above. The parsed node carries `SetTest`, a **bool** and not a count —
which is the one place this differs from `ParamTildeFlag`, and it is
measured rather than assumed: `${++x}` is a bad substitution, so there is no
second `+` for a parity to be about. It asks a question rather than setting
a mode.

What may follow the `+` is checked where the `+` is read rather than after
the name scan, because `${+#v}` would otherwise be read as a length over a
set test — a shape the shell that has the construct does not have.

The printer writes the span back raw, so the construct round-trips as
source text.

### What the corpus pins

`param/the-set-test-flag-is-one-dialects` (the three-way split, with the
empty-but-set name in the same row), `-does-not-trip-nounset` (with the
`alive` that a shell which gave up could not print) and
`-has-no-effect-beside-an-operator`.

### What this implementation does not match

`${+00}` is `1` in zsh and `0` here, which is inherited rather than the
flag's: `${00}` is the shell's own name there and the empty string here, so
a positional written with a leading zero is a gap of its own and this
follows it.

`${#+v}` is `v` in zsh — the `$#` reading above — and a `bad substitution`
here, which is a gap this change neither opened nor closed: `${#}` taking
an alternate word is the `${}` half of the same issue.

## An equals at the front: `${=spec}` — zsh only

An `=` written between the `${` and the parameter splits the *result* of the
substitution into words on `IFS`, whatever the `SH_WORD_SPLIT` option is set
to. zsh alone has it. The other four call the whole expansion unreadable, in
the same three-way split every unreadable expansion follows: bash 5.3, bash
3.2, bash-as-sh and dash say `bad substitution` when the expansion is
reached, ksh93 refuses it while reading (`` `=' unexpected ``) — which
`BadSubstitutionAtParseTime` already records.

Found in the wild, and this is why the construct is here rather than on a
list: `is-at-least`, the version predicate in zsh's own function library,
splits both of its operands with it —

    argv1=(${=1})
    argv2=(${=2:-$ZSH_VERSION})

— so a shell that refuses the substitution leaves both version arrays
**empty**, compares nothing, and answers "at least" for *every* version, at
status 0 and with no diagnostic. `~/.zi/bin/zi.zsh:205` branches on exactly
that call. A refusal names itself; this does not, and the wrongness travels.

All measurements below are of zsh 5.9.2 (Homebrew, arm64), taken 2026-09-06.

### What it does, measured

With `v='a b c'` and a function reporting `$#` and each field:

| written | fields |
| --- | --- |
| `${v}` | 1: `a b c` — the value, unchanged |
| `${=v}` | 3: `a`, `b`, `c` |
| `"${=v}"` | 3: `a`, `b`, `c` — **quoting does not suppress it** |
| `x${=v}y` | 3: `xa`, `b`, `cy` — the fields join the text around them |
| `${=v//b/x y}` | 4 — the operator runs first and the flag splits what it left |
| `${=#v}` | 1: `5` — a length is a number, which holds no separator |
| `${=a[@]}`, `${=@}` | the elements, each split |

Three behaviors carry the construct and each is asserted rather than
inferred from the absence of an error — the bug this replaces returned a
plausible value at status 0:

- **Quoting does not suppress it.** This is where the flag parts company
  with its sibling `${~spec}`, whose whole construct quoting turns off.
- **`${==v}` turns it back off**, which is how a nested use says "not here".
- **A context that never splits is not overridden.** `x=${=v}` is the value
  unchanged, and so are `[[ ${=v} = "a b" ]]`, a `case` subject and a
  here-document body.

### The count is parity, and it overrides the option

`SH_WORD_SPLIT` is the option that makes *every* unquoted expansion's result
split. The written `=` characters do not toggle it — they decide the answer
outright, on the parity of how many were written, measured under the option
both ways:

| written | `unsetopt shwordsplit` | `setopt shwordsplit` |
| --- | --- | --- |
| `${v}` | 1 field | 3 fields |
| `${=v}` | 3 | 3 |
| `${==v}` | 1 | 1 |
| `${===v}` | 3 | 3 |
| `${====v}` | 1 | 1 |

So one `=` is "yes" and two are "no" from either starting point, and only a
`spec` with no `=` at all consults the option. The zero-`=` row is the
dialect's `SplitParamExpansion` answer.

### Where the `=` may be written

The same slot the tilde flag uses, and the two are interchangeable within
it. Measured:

| written | zsh |
| --- | --- |
| `${(U)=v}` | flags then `=`: read, uppercased, split |
| `${=(U)v}` | `bad substitution` — the group may not follow the run |
| `${=#v}` | the *length*, so the run precedes `#` as well |
| `${=~v}`, `${~=v}` | the same expansion: both flags, in either order |
| `${=}` | the empty string, no error — as `${~}` is |
| `${#=v}` | **not this flag**: `${#=word}` is `$#` with a default assigned to it, so `set -- p q` makes it `2` — unanimous across the panel, and answered; see "A `#` is the prefix, or it is the parameter `$#`" |

### It is one string that is split, not each element

A list is joined on `IFS`'s first character and the join is what splits,
which is measured and is the difference a per-element implementation gets
wrong:

    a=(' x ' y); "${=a[@]}"   → 3 fields: ``, `x`, `y`
                                (splitting each element would give 4)
    a=(); "${=a[@]}"          → one empty field, where `"${a[@]}"` is none

That is `UnquotedListJoinsOnIFS` reached from the other side: the flag says
"read this as one value and split it", and what a list comes to as one value
is already answered.

### The edges, quoted and unquoted

The split is field splitting on `IFS`, with one difference between the two
quotings — quoted, a delimiter at either end of the value still separates:

| `v` | `${=v}` | `"${=v}"` |
| --- | --- | --- |
| `a b` | 2: `a`, `b` | 2: `a`, `b` |
| `a  b` | 2 | 2 — a run of whitespace is one delimiter either way |
| `' a '` | 1: `a` | 3: ``, `a`, `` |
| `'  '` | 0 fields | 2, both empty |
| `''` | 0 fields | 1, empty |
| `a::b` with `IFS=:` | 3: `a`, ``, `b` | 3, the same |

`IFS` set and empty disables the stage entirely, as it does everywhere else:
`IFS=; ${=v}` is the value.

### Beside a flag group it is a step *inside* the group

`${(U)=v}` does not split what the group produced — it splits at the group's
own splitting rule, which runs before the ordering, the prompt escapes, the
quoting and the case conversion. Three measurements fix it there:

    v='c a b'; ${(o)=v}   →  a b c    the sort sorts fields, not one word
    v='a b';   ${(q)=v}   →  a, b     the quoting ran after the split
    v=' a ';   "${(U)=v}" →  ``, A, `` the fields survive the quotes

`(f)` and `(s)` *are* that rule with a separator of their own, so an `=`
beside either adds nothing: `v='a b'; ${(s.,.)=v}` is one field, and
`${(s.,.)==v}` still splits — the group decides and the parity is never
consulted.

### Grammar

The `=` run is read only when the dialect's `ParamSplitFlag` is on;
elsewhere a leading `=` is not a name and the expansion follows the
bad-substitution split above. The parsed node carries `SplitFlags`, the
number written, so parity is the interpreter's to take and the printer keeps
writing the span back raw — the construct round-trips as source text.

It cannot collide with the `${x=word}` assignment: that `=` follows a name
and this one precedes it, so position tells them apart before either is
read.

### What the corpus pins

`param/the-split-flag-is-one-dialects` (the three-way split),
`-reaches-through-quotes`, `-doubled-turns-it-off` and
`-does-not-reach-an-assignment`.

### What this implementation does not match

- Nothing here. `GLOB_SUBST` was recorded-and-inert, so
  `setopt globsubst; ${=g}` split and did not then match; it moves the axis
  now (#1734) and both halves follow.

## A caret at the front: `${^spec}` — zsh only

A `^` written between the `${` and the parameter makes the expansion
**distributive**: the word it stands in is produced once per element, with the
text around it on each, rather than once with the elements laid into it.
zsh alone has it. The other four call the whole expansion unreadable, in the
same three-way split every unreadable expansion follows: bash 5.3, bash 3.2,
bash-as-sh and dash say `bad substitution` when the expansion is reached,
ksh93 refuses it while reading (`` `^' unexpected ``) — which
`BadSubstitutionAtParseTime` already records.

It is the per-expansion spelling of the `RC_EXPAND_PARAM` option, the same
way `${~spec}` is `GLOB_SUBST`'s and `${=spec}` is `SH_WORD_SPLIT`'s, and it
shares their slot.

Found in the wild, and this is why the construct is here rather than on a
list: `~/.zi/bin/zi.zsh` builds the argv of every non-zsh plugin with it —

    ${(s: :):-${${:-${(@s: :):--o}" "${(s: :)^ICE[opts]}}:#-o }}

— which turns an `opts` ice of `a b` into `-o a -o b`. Refused, the load of
any plugin carrying that ice stops with `bad substitution`.

All measurements below are of zsh 5.9.2 (Homebrew, arm64), taken 2026-09-08.

### What it does, measured

With `a=(1 2)` and a function reporting `$#` and each field:

| written | fields |
| --- | --- |
| `x${a}y` | 2: `x1`, `2y` — the elements laid into one word |
| `x${^a}y` | 2: `x1y`, `x2y` — the word laid onto each element |
| `x${^a}` | 2: `x1`, `x2` |
| `${^a}y` | 2: `1y`, `2y` |
| `${^a}` | 2: `1`, `2` — with no text around it there is nothing to distribute |
| `x${^a[2,3]}y` | the elements the subscript named, each in its own word |
| `x${^#a}y` | 1: `x2` — a length is a number, which is one value |

**The field count is the same in the first two rows**, which is the whole
difficulty of asserting this construct: an implementation that ignored the
flag entirely would answer `2` as well. Only the values say which reading
ran, and every row of the corpus and of the interpreter's tests prints them.

### Two of them in one word are a cross product

This is what says the subject is the *word* and not the field:

    a=(1 2)
    ${^a}z${^a}         →  1z1  1z2  2z1  2z2
    a=(1 2); b=(A B); c=(p q)
    ${^a}${^b}${^c}     →  1Ap 1Aq 1Bp 1Bq 2Ap 2Aq 2Bp 2Bq

The open fields are the outer loop and the later expansion varies fastest.
An implementation that spread each expansion over whatever stood in front of
it would produce four words for the first line too, and the wrong four.

Mixed with an ordinary expansion, each keeps its own rule — the distribution
multiplies the open fields, and the ordinary one closes them and opens a
single new field of its own:

    a=(1 2); b=(A B);  x${^a}-${b}y   →  x1-A  x2-A  By
    a=(1 2);           x${^a}y${a}z   →  x1y1  x2y1  2z

### No elements is no word

The word is produced once per element, so an empty list produces it no times
— and a field already finished stands, because it is no longer open:

| written | fields |
| --- | --- |
| `a=(); x${^a}y` | none at all |
| `a=(); x${a}y` | 1: `xy` — the same array without the flag |
| `unset u; x${^u}y` | 1: `xy` — a name that was never a list is one empty value |
| `set --; x${^@}y` | none |
| `a=(1 2); b=(); x${a}z${^b}q` | 1: `x1` — the finished field survives |

The third row is what makes this a fact about the *list* and not about
emptiness, and it is the one an implementation is likely to get wrong: an
empty array and an unset name are the same empty string under every other
reading, and here they are one field apart.

### Quoting is not a question it asks

It looks at first as though quoting suppresses it, the way it suppresses
`${~spec}`. It does not — the quotes joined the list before the flag saw it,
and the spellings that keep their fields through quotes distribute over them.
With `a=(1 2)`, and with `set -- 1 2` for the rows written on the positionals:

| written | fields |
| --- | --- |
| `"x${^a}y"` | 1: `x1 2y` — the quotes joined the list |
| `"x${^a[*]}y"`, `"x${^*}y"` | 1: the join again, written out |
| `"x${^a[@]}y"`, `"x${^@}y"` | 2: `x1y`, `x2y` — `[@]` keeps its fields |
| `"x${(@)^a}y"` | 2 — and so does the `(@)` flag |
| `x"${^a}"y` | 1: `x1 2y` — the quoting of the *expansion* |
| `"x"${^a}"y"` | 2: `x1y`, `x2y` — and not of the word |
| `set --; "x${^@}y"` | none, where `"x${@}y"` is the one field the quotes guarantee |

So the rule is "the fields the span produced", and a guard on the quoting
would answer the first row right and the four after it wrong.

### It runs on what the span came to

Everything the expansion does happens first, and the distribution spreads the
fields that came out. That is where it parts company with `${=spec}`, which
is a step *inside* the flag group:

    a=(c a b);  x${(o)^a}y     →  xay xby xcy   the sort ran first
    a=(c a b);  x${(oj:-:)^a}y →  xc-a-by       the join left one field
    v=a,b;      x${(s.,.)^v}y  →  xay xby       and the split left two
    a=(p1 p2);  x${^a#p}y      →  x1y x2y       the operator ran first
    a=();       x${^a:-p q}y   →  xp qy         so did the default
    v="a b"; setopt shwordsplit; x${^v}y → xay xby

### The count is parity, and it overrides the option

`RC_EXPAND_PARAM` is the option that makes *every* expansion distributive.
The written `^` characters do not toggle it — they decide the answer outright,
on the parity of how many were written, measured under the option both ways
with `a=(1 2)`:

| written | `unsetopt rcexpandparam` | `setopt rcexpandparam` |
| --- | --- | --- |
| `x${a}y` | `x1`, `2y` | `x1y`, `x2y` |
| `x${^a}y` | `x1y`, `x2y` | `x1y`, `x2y` |
| `x${^^a}y` | `x1`, `2y` | `x1`, `2y` |
| `x${^^^a}y` | `x1y`, `x2y` | `x1y`, `x2y` |
| `x${^^^^a}y` | `x1`, `2y` | `x1`, `2y` |

So one `^` is "yes" and two are "no" from either starting point, and only a
`spec` with no `^` at all consults the option.

### Where the `^` may be written

The slot `${~spec}` and `${=spec}` use, and all three are interchangeable
within it. Measured:

| written | zsh |
| --- | --- |
| `${(U)^a}` | flags then `^`: read, uppercased, distributed |
| `${^(U)a}` | `bad substitution` — the group may not follow the run |
| `${^#a}` | the *length*, so the run precedes `#` as well |
| `${#^a}` | `bad substitution` — nothing behind the `#` is a flag |
| `${^=a}`, `${=^a}` | the same expansion: both flags, in either order |
| `${^~a}`, `${~^a}` | likewise |
| `${^+a}` | the set test, which stands behind the run |
| `${+^a}` | `bad substitution` — and not in front of it |
| `${^}` | the empty string, no error — as `${~}` and `${=}` are |

### Grammar

The `^` run is read only when the dialect's `ParamRcExpandFlag` is on;
elsewhere a leading `^` is not a name and the expansion follows the
bad-substitution split above. The parsed node carries `RcExpandFlags`, the
number written, so parity is the interpreter's to take and the printer keeps
writing the span back raw — the construct round-trips as source text.

It cannot collide with bash's case-conversion `^`: that one follows the name
— `${x^}` — and this one precedes it. Nor with the `^` of `EXTENDED_GLOB`'s
pattern negation, which is not inside a `${`.

### Where the distribution lives

Not on the value, which is what makes it different from both of its
slot-mates: they change what one expansion *comes to*, and this one changes
how what it came to is laid into the word. So it is one rule about the set of
fields a word still has open, applied where the fields of a word are
assembled, and a second distributive expansion in the same word finds several
open where an ordinary word has one.

A context that produces a single value has no word to produce more than once,
and is not distributed: `v=x${^a}y` is the elements joined, and so are a
`case` subject and a `[[ ]]` operand. An array literal and a `for` list are
words each, so there it does distribute.

### What the corpus pins

`param/the-rc-expand-flag-is-one-dialects` (the three-way split, with the
unflagged reading beside it because the two field counts agree),
`-crosses-two-expansions`, `-doubled-turns-it-off`,
`-spreads-the-fields-it-was-given` and `-over-no-elements-is-no-word`.

### What this implementation does not match

- `RC_EXPAND_PARAM` is recorded-and-inert, so `setopt rcexpandparam` does not
  make an unflagged expansion distributive. `GLOB_SUBST` had the same gap
  behind `${~spec}` until #1734 and no longer does; the flag is what the wild
  code writes either way.
- A redirection target is one target here. zsh's `MULTIOS` opens one file per
  field, so `: > p_${^a}` creates a file per element there; this shell has no
  `MULTIOS`, and the divergence is that option's rather than this flag's.

## A third assignment operator: `${name::=word}` — zsh only

    ${name=word}     assign when the parameter is unset
    ${name:=word}    assign when it is unset **or** empty
    ${name::=word}   assign, always

The third one asks nothing. Measured 2026-09-07 on zsh 5.9.2, over the
three states of the parameter that the two above it disagree on:

| written | unset | set and empty | set to `old` |
| --- | --- | --- | --- |
| `${v=new}` | `new` | *(empty)* | `old` |
| `${v:=new}` | `new` | `new` | `old` |
| `${v::=new}` | `new` | `new` | `new` |

and the value left behind is the value substituted, in every cell where
the assignment fired. The third column is the whole of what separates the
operator from the two beside it: an implementation that read `::=` as
`:=` would be right twice and plausible the third time.

**One shell in the panel has it, and the other four are the evidence for
the grammar flag.** Measured the same day, `v=old; ${v::=new}`:

| shell | answer |
| --- | --- |
| zsh 5.9.2 | `new`, and `v` is `new` |
| bash 5.3.15 | `v: =new: arithmetic syntax error: operand expected` |
| bash as `sh` | the same |
| bash 3.2.57 | `v: =new: syntax error: operand expected` |
| ksh93 | `:=new: arithmetic syntax error` |
| dash | `Bad substitution` |

The four refusals are all the same reading: an empty offset and a length
of `=new`. So this is additive grammar rather than one syntax meaning two
things — and reading it the substring way is not a quiet mis-answer, it is
that arithmetic error, eighteen lines of it on one real startup, from a
plugin manager whose own message formatter writes `${ZI[…]::=…}` (#1369).

### The disambiguation is one character wide

Only `=` makes the operator. Measured, with `v=old`:

| written | zsh 5.9.2 |
| --- | --- |
| `${v::=D}` | `D`, assigned |
| `${v::-D}` | *(empty)* — an offset of nothing, a length of `-D` |
| `${v::+D}` | *(empty)* — the same |
| `${v::?D}` | fails in arithmetic on the word `D` |
| `${v::}` | *(empty)* |

So the second colon opens an operator for exactly one character and an
offset for the rest, and a grammar that widened the reading by one would
break shapes every shell in the panel shares.

### What it may name

A name, a positional, or `0`. Anything else is refused, and the refusal is
fatal — measured on all three routes, `-c`, a script file and a function
body, each ending the script at status 1:

| written | zsh 5.9.2 |
| --- | --- |
| `${v::=w}` | assigns `v` |
| `${1::=w}` | assigns `$1` |
| `${0::=w}` | assigns `$0` |
| `${@::=w}` | `not an identifier: @` |
| `${*::=w}` | `not an identifier: *` |
| `${#::=w}` | `not an identifier: #` |
| `${?::=w}` | `not an identifier: ?` |
| `${-::=w}` | `not an identifier: -` |
| `${${v}::=w}` | `not an identifier: ` — an expansion where the name would be |

The refusal reaches the parameters `${@=w}` and `${*=w}` never do, because
those two are always *set* and the test never fires; this operator has no
test, so every one of them is reached on every run. The word is expanded
first: `${#::=$(echo RAN >&2)}` writes RAN and then refuses the name, so a
command substitution in the word runs even on the failing line.

### It composes the way the other operators do

Measured, all four:

    setopt nounset; unset v; ${v::=new}   new — the value is never read
    v=old; ${#v::=abcd}                   4, and v is abcd
    ${(U)v::=abc}                         ABC, and v is abc
    ${=v::=p q}                           two fields, and v is `p q`

A readonly parameter refuses through the same door every other assignment
to one goes through — `read-only variable: v`, fatal.

### Grammar

Read only when the dialect's `ParamAssignAlways` is on; elsewhere the same
characters are `ParamSubstring`, which is what the four refusals above are
evidence of. The parsed node carries `ParamAssignAlways` as its operator
and the text after the `=` as its word.

It is its own operator and not `ParamAssign` with a second colon: the
conditional assignment *asks*, and has a non-firing side on which the
parameter's own value is substituted, where this one has neither. `Colon`
has no meaning on the node.

### What the corpus pins

`param/the-always-assign-operator` (the three states, with the value left
behind), `param/the-always-assign-operator-beside-the-conditional` (the
pair, on one starting value) and
`param/only-the-equals-makes-the-always-assign` (the two near-misses).

### What this implementation does not match

Both are older than this operator and are reached by it rather than caused
by it — each has a second, simpler reproducer that has nothing to do with
`::=`:

- An assignment to a **positional** does not reach the positional list.
  `set --; ${1::=new}` leaves `$1` reading `new` and `$#` at 0 where the
  shell says 1, and `set -- p q; ${1::=new}` leaves `$1` as `p`. The same
  is true of `${1:=new}` and `${1=new}` when their test fires.
- Assigning a **scalar over an existing indexed array** leaves the array
  standing: `a=(1 2 3); ${a::=x y}` substitutes `x y` and `a` is still the
  three elements, where the shell leaves the scalar `x y`. `read a` over
  an array does the same thing, which is where this belongs.

## A subscript without braces — zsh only

Everything above is written `${ … }`. zsh also reads a subscript and a
length on a parameter written **without** braces, and the difference is
in the grammar rather than in what a shared syntax means: the same
characters are a different number of words in the two shells.

Measured 2026-09-05 on zsh 5.9.2, bash 5.3.15, bash 3.2.57 and dash,
with `a=(x y z)`:

| probe | zsh | bash 5.3 | bash 3.2 | dash |
| --- | --- | --- | --- | --- |
| `"[$a[1]]"` | `[x]` | `[x[1]]` | `[x[1]]` | no arrays |
| `[$a[1]]` unquoted | `no matches found: [x]` | `[x[1]]` | `[x[1]]` | no arrays |
| `"[$#a]"` | `[3]` | `[0a]` | `[0a]` | `[0a]` |

To zsh, `$a[1]` is one expansion of the first element and `$#a` is the
array's count. To everything else, `$a[1]` is `$a` followed by the three
characters `[1]` — a glob pattern, once the quotes come off — and `$#a`
is `$#` followed by the letter `a`.

Both halves fail quietly in the direction that matters. `$#a` is a
number in either reading, so a script that tests it against zero tests
the count in one shell and the string `0a` in the other, and nothing
says so. `$a[1]` is louder only by accident: zsh's default `nomatch`
turns the *other* direction into an error, so a bash script's
`$dir[0-9]*` read as a subscript would fail visibly, which is why the
form is behind a flag and not in the core.

### Which parameters take one

Not all of them, and the two operators stop in different places.

A **subscript** follows a name, `@` and `*`. It does not follow a
positional digit: `set -- abcd; echo "[$1[2]]"` is `[abcd[2]]` in every
shell in the panel, zsh included
(`array/a-subscript-without-braces-is-not-a-positional`). Exactly one is
read — `"[$a[1][1]]"` is `[x[1]]`, not a character of `x`
(`array/a-subscript-without-braces-is-read-once`).

zsh also subscripts the remaining specials: `$?[1]`, `$-[2]`, `$$[1]`
and `$0[2]` all index the parameter's value. The brace-less spelling
leaves those out, because the parsed form has nowhere to put them — the
inner text of a span is what `${ … }` would hold, and there `${#[1]}` is
a length and `${![1]}` an indirection. The **braced** spellings are read,
under `SpecialParamSubscript` above; it is the two the span cannot write
down that are recorded rather than modeled.

A **length** takes a name, a digit, `@` or `*`: `$#a` is a count, `$#0`
and `$#1` are the lengths of those parameters, and `$#@` and `$#*` are
the number of positional parameters. It stops before `#` and before `!`
— `$##` is the count and then a literal `#`, and `$#!` the count and then
a literal `!`, both exactly as in bash
(`array/a-length-without-braces-stops-at-two-specials`).

### Grammar

Grammar flag `BareSubscript`, consumed by the **lexer**, because the
word boundary is what changes and nothing downstream can recover it once
the spans are cut. The span it produces is the one `${ … }` would have
produced — `$a[1]` and `${a[1]}` are one node — so the parser, the
interpreter and the subscript's arithmetic are unchanged, and the
printer writes the braced spelling back.

**Whether the brackets are a subscript is not this flag's question.**
The flag says they belong to the expansion; what they *mean* is the
semantics axis `BareSubscriptIsASubscript`, because the shell with the
construct moves it at run time — `setopt ksharrays` makes `$a[1]` the
element at the base position followed by the three characters `[1]`,
while `${a[1]}` is unaffected. Measured: the answer in force when the
word **expands** is the one that decides, so a function body written
under one answer and called under the other takes the caller's, and the
same holds across `eval` and `source`. The parser therefore records both
readings — `ParamExpr.BareIndexText` is the brackets lexed as the text
they would be, in the expansion's own quoting — and the run picks. See
`docs/spec/semantics.md` for the axis and `dialect/zsh/ksharrays.go` for
the option that moves it (#1726).

Where the subscript may reach is the quoting's question rather than a
fixed set of characters. Inside double quotes a blank and a newline are
ordinary text, so `"$a[1 ]"` and `"$a[1` + newline + `]"` are both the
first element; unquoted, either one ends the word. A closing quote stops
the scan in the quoted case, which `"$a[" ]` shows: there is a `]` in
that line and no subscript.

An unquoted `[` that never closes before the word does is not a
subscript here and its characters stay literal. zsh commits to the
subscript instead and reports `invalid subscript` at run time — for the
unclosed unquoted form and for `"$a[" ]` alike; the shape fails either
way, and the difference is the wording.

## A subscript's own flag group — zsh only

A subscript may open with a parenthesized group of flags of its own,
`${a[(re)value]}`, which makes the subscript a **search** rather than a
count. It is not the `${(flags)name}` group above in a second position;
the two are different constructs that happen to share a punctuation.

Measured 2026-09-06 with `a=(alpha beta gamma beta delta)`:

| shell | `echo "[${a[(r)beta]}]"` |
| --- | --- |
| zsh 5.9.2 | `[beta]` |
| bash 5.3.15 | `(r)beta: arithmetic syntax error in expression (error token is "beta")` |
| bash-as-sh | the same, naming the invocation instead of the script |
| bash 3.2.57 | `(r)beta: syntax error in expression (error token is "beta")` |
| ksh93u+ | `(r)beta: arithmetic syntax error` |
| dash | `Syntax error: "("` — at `a=( … )`, having no arrays to subscript |

One shell has it and the rest read the same characters as arithmetic, so
it is an **additive** grammar flag rather than a semantics axis. The
evidence for "additive" is stronger than the usual four-against-one,
because the shell that has the construct *also* reads the arithmetic:
`${a[(z)2]}` is `bad math expression: operator expected at `2'` in zsh
and an arithmetic failure in each of the others. There is no text this
flag gives a second meaning to. It gives a meaning to text that had none.

### The whole flag set, measured by exhaustion

Every ASCII letter was asked twice — bare, `${a[(X)2]}`, and with an
argument, `${a[(X:1:)2]}` — and the ones that did not fall back to
arithmetic are the flag set. There are thirteen and no others:

    w f p e i I r R k K        no argument
    b n s                      an argument, in any delimiter pair

Six of them select, and they are mutually exclusive — the **last one
written** wins, `${a[(ri)be*]}` being `2` and `${a[(ir)be*]}` being
`beta`:

| written | is |
| --- | --- |
| `${a[(r)pat]}` | the first element the pattern matches, or nothing |
| `${a[(R)pat]}` | the last such element |
| `${a[(i)pat]}` | the index of the first match, or one past the last element |
| `${a[(I)pat]}` | the index of the last match, or one before the first |
| `${a[(k)pat]}` | `(r)` under another letter — see below |
| `${a[(K)pat]}` | and `(R)` under another letter |

and three modify the search:

| written | is |
| --- | --- |
| `${a[(e)…]}` | the operand is a plain string, not a pattern |
| `${a[(n:expr:)…]}` | the expr'th match rather than the first |
| `${a[(b:expr:)…]}` | the search starts at element expr rather than at the end |

`(re)` is the combination the scripts use: the first element **equal**
to the operand. With `[[ -z … ]]` in front of it that is the idiom for
"is this directory already on the path", which is what
`~/.zi/bin/zi.zsh:161-193` is doing six times.

### `(k)` and `(K)` are two readings, and the target decides which

On **everything but a table** they are `(r)` and `(R)` wearing two more
letters. Measured on zsh 5.9.2, 2026-09-12, over every surface a search
reaches — and the pairs are the point, because one row could not tell a
coincidence from a rule:

| written | is | and so is |
| --- | --- | --- |
| `a=(x y z x); ${a[(k)x]}` | `x` | `${a[(r)x]}` |
| `a=(x y z x); ${a[(K)*]}` | `x`, the last match | `${a[(R)*]}` |
| `a=(x y z); ${a[(k)q]}` | empty | `${a[(r)q]}` |
| `a=(p q p); ${a[(kn:2:)p]}` | `p`, the second match | `${a[(rn:2:)p]}` |
| `a=(p q r); a[(k)q]=Z` | `p Z r` | `a[(r)q]=Z` |
| `a=(p q r); unset 'a[(k)q]'` | `p  r` | `unset 'a[(r)q]'` |
| `a=(p q r s); ${a[(k)q,3]}` | `q r` | `${a[(r)q,3]}` |
| `a=(p q r s); ${a[1,(k)r]}` | `p q r` | `${a[1,(r)r]}` |
| `s=hello; ${s[(k)l]}` | `l` | `${s[(r)l]}` |

On a **table** they part, and they are a *lookup* rather than a search:
the key exactly as written, no pattern, no walk, no modifiers, and the
**value** is what comes back — where `(i)` over the same table answers
the key.

| written | is |
| --- | --- |
| `typeset -A m=(aa 1 bb 2); ${m[(k)aa]}` | `1` |
| `${m[(K)aa]}` | `1` — the case of the letter changes nothing |
| `${m[(k)a*]}` | empty — a pattern is not a key |
| `${m[(k)zz]}` | empty — nor is a key the table has not got |
| `${m[(kn:1:)a*]}` | empty — the modifiers move nothing here |
| `${(kv)m[(k)aa]}` | `aa 1` — the expansion's own letters still choose |
| `${#m[(k)aa]}` | `1` — the match count, as for every other search |

The pattern row is the discriminator: a search reading would answer
`${m[(k)a*]}` with the `1` under `aa`, and it does not.

Because they are the same search everywhere but the table, the letter is
normalized once rather than tested for in each of the three places that
ask which way a walk runs — see `interp.orderedSearchLetter` (#1986).

### A group that selects nothing changes nothing

`${a[()2]}` and `${a[(e)2]}` are both the second element. A group with
no `r`, `R`, `i` or `I` in it says how a search would have run and no
search was asked for, so the subscript behind it is read exactly as a
subscript without a group: arithmetic, `@`, `*`, a range, or an
associative key. `(e)` alone is the sharpest case — `${a[(e)beta]}` is
**empty**, because `beta` is still an arithmetic expression there and an
unset name is zero.

### What the operand is: the text as written, and there is no quoting

A pattern, unless `(e)`. What it is a pattern *of* is the subscript's
text with its substitutions performed and **nothing else touched** — a
subscript is not a quoting context at all, and the quote characters in
one are ordinary characters. Measured three ways, with
`b=('"beta"' beta)` — the first element's value is six characters:

| written | is | so |
| --- | --- | --- |
| `${b[(r)"beta"]}` | `"beta"` | the quotes are matched, not removed |
| `${b[(r)"$h"]}`, `h=beta` | `"beta"` | the value was substituted *inside* them |
| `${b[(re)"beta"]}` | `"beta"` | and exact matching sees them too |

That single rule explains the pattern half without an axis of its own:
a substituted value's metacharacters are simply live, because nothing
escaped them.

    g='be*'; ${a[(r)$g]}   # beta
    ${(@)a:#$g}            # removes nothing, in the same shell

The second line is the dialect's ordinary answer for the result of an
expansion. The first is not an override of it — it is a position where
the question never arises.

It is never matched against the *filesystem*: `${a[(re)be*]}` in a
directory with no `be*` in it is empty rather than `no matches found`.

**The same rule reaches an associative key**, which is the other thing a
subscript can be, and there it divides the panel rather than being one
shell's construct. Measured 2026-09-07:

| probe | bash 5.3, bash as `sh`, ksh93 | zsh 5.9.2 |
| --- | --- | --- |
| `m["k"]=W; kk='"k"'; ${m[$kk]}` | `` | `W` |
| the same, `${m[k]}` | `W` | `` |
| `q[k]=K; v=k; ${q[$v]}` | `K` | `K` |
| the same, `${q["$v"]}` | `K` | `` |
| `m[a\b]=B; ${m[ab]}` | `B` | `` |

`Semantics.SubscriptIsAQuotingContext` is that answer: where the
subscript *is* a quoting context the key is the text inside its quotes
and escapes, and where it is not the key is the text as written. bash 3.2
has no associative arrays and dash has no arrays, so the axis is absent
in both rather than false.

The third and fourth rows are the crisp form: the substitution is
performed under both answers, so what differs is only whether the two
quote characters around it are part of the key. That is what makes this a
rule about quoting and not about expansion.

Two neighbors stay unanimous and must not move with it:

- a **bare** `@` or `*` is the whole array in all three, and a *quoted*
  one is a key — `${n["@"]}` looks one up and finds nothing, and on an
  *indexed* array it is an arithmetic error, since `@` is no number. So
  the whole-array spelling is read off the subscript as written and never
  off the key quote removal produced.
- a key is **not** space-trimmed. With `p[s]=T`, `${p[ s ]}` looks up
  three characters and is empty in all three. The trimming an arithmetic
  subscript can afford — the evaluator ignores blanks anyway — is wrong
  here.

#### An empty key

A key that comes out **empty** is a third answer again, and it is
`Semantics.EmptyAssociativeKeyIsAnError`. Measured 2026-09-12 from a
script file, with the name declared and `w=`:

| probe | bash 5.3.15 | ksh93u+ | zsh 5.9.2 |
| --- | --- | --- | --- |
| `m[""]=4` | refused | stored | stored |
| `m[$w]=5` | refused | stored | stored |
| `m[" "]=7` | stored | stored | stored |
| elements afterwards | 1 | 2 | 3 |

bash names the subscript **as it was written** — `m[""]: bad array
subscript` and `m[$w]: bad array subscript`, not what either came to —
leaves the table untouched, reports 1, and gives up the rest of the
command list, so `m[""]=4; echo A` prints no `A` and the line after it
runs. That last part is not a rule of its own: it is what a refused
reassignment does in the same column.

The two columns that store are not storing the same key, and that is the
axis above rather than this one. ksh93's subscript is a quoting context,
so both spellings are the empty key and the second write replaces the
first — two elements, counting the blank. zsh's is not, so `m[""]` is a
two-character key and only `m[$w]` reaches the empty one — three.

The blank row is the control that says this is emptiness and not
whitespace: one space is an ordinary key wherever the brackets hold a key
at all.

**Reading an empty key is a fourth question** and is not this axis. It is
`Semantics.EmptyAssociativeKeyIsReportedWhenRead`, measured 2026-09-12
with `typeset -A m; m[k]=v` and `w=`:

| probe | bash 5.3.15 | ksh93u+ | zsh 5.9.2 |
| --- | --- | --- | --- |
| `${m[$w]}` | `m: bad array subscript`, then `` at 0 | `` at 0 | `` at 0 |
| `${m[$w]-none}` | the same sentence, then `none` | `none` | `none` |
| `${m[ ]}` | nothing at all | `` at 0 | `` at 0 |

One column says the subscript is bad and two say nothing, and all three
answer the same empty string at the same status — so this is a report and
not a value. The subject is the **name alone**, where the store one
paragraph up names the subscript as it was written, and the status stays
0: the sentence is written once per read and the word is finished rather
than abandoned, which is why the default in `${m[$w]-none}` still fires.

The blank row is again the control. `${m[]}` — nothing between the
brackets as written — is refused one construct earlier by
`EmptyParamSubscriptIsAnError` and never reaches this, and an indexed
name reads its subscript as an expression and asks
`EmptySubscriptTextIsAMathError` instead (#1972).

This needed the parser before it needed an axis. `m[""]=4` parsed with no
subscript at all, because the empty quoted span between the brackets was
dropped as a zero-length piece of text; the assignment then arrived as a
plain `m=4` and stored under the key `0`. An empty span that was written
*quoted* is the record that quotes were there, and it is the whole of the
difference between `m[""]` and `m[]` (#1938).

### `n` and `b`, measured

Both arguments are arithmetic expressions rather than numerals, and
neither is parameter-expanded: `(rn:1+1:)` is the second match, `(rn:k:)`
with `k` unset is the first, and `(rb:$one:)` fails in the arithmetic on
the `$`.

- `n` below one is one: `(rn:0:)` is the first match.
- `n` past the last match answers the no-match value — `(rn:9:)` empty,
  `(in:9:)` one past the end, `(In:9:)` zero.
- `b` names an element in the array's own base, and a negative one counts
  back from the end: with five elements `b:-1:` is the fifth and `b:-5:`
  the first.
- `b` **outside** the array is not clamped to its nearest end. With five
  elements `${a[(Ib:6:)*a]}` is `0` and `${a[(ib:6:)*a]}` is `6`: neither
  direction searches at all, where clamping would have found element 5.
- The two ends of *outside* are not mirrors. A start past the end leaves
  each letter the miss its own direction names, as above; a start counted
  back past the **first** element gives the forward search the *backward*
  miss. Measured 2026-09-12 with `a=(p q r p t)`: `${a[(ib:-6:)p]}` is
  `0`, not `6`, and `${a[(Ib:-6:)p]}` is `0` as it already was, while
  `${a[(rb:-6:)p]}` and `${a[(Rb:-6:)p]}` are both empty. An **array**
  only: `s="hello world"; ${s[(ib:-12:)l]}` is `12`, the ordinary forward
  miss, so the rule belongs to the element walk rather than to where a
  search begins.

  The same answer reaches the left of an assignment, where neither half
  of it is a subscript that side can write to: `b[(ib:-6:)p]=Q` on
  `b=(x y z p)` puts `Q` in front of every other element exactly as
  `b[(Ib:-6:)p]=Q` does, and `b[(rb:-6:)p]=Q` is `assignment to invalid
  subscript range` exactly as the `R` miss is. Refused by name here, as
  the `I` and `R` misses already are.
- Forward searches (`r`, `i`) run from `b` upward; reverse ones (`R`,
  `I`) run from `b` downward, so `${a[(Rb:3:)*a]}` is `gamma`.

### A name that holds nothing, and an array that holds nothing

They are different. `${nosucharray[(i)x]}` is **empty**, while a declared
but empty array answers `${b[(i)x]}` with `1` and `${b[(I)x]}` with `0`.
An unset name is not searched at all.

### On the left of an assignment

The same group names the element an assignment writes to, and the
semantics need nothing new: `(r)` and `(i)` name an *index*, which is what
a subscript on that side has always been. Measured 2026-09-07 with
`b=(x y z)`:

| probe | result |
| --- | --- |
| `b[(r)y]=Q` | `x Q z` — the element the search found |
| `b[(re)y]=Q` | `x Q z` |
| `b[(R)x]=Q` on `(x y x)` | `x y Q` — the reverse search takes the last |
| `b[(i)y]=W` | `x W z` |
| `b[(I)y]=W` | `x W z` |
| `b[(i)nomatch]=W` | `x y z W` — one past the last, so it appends |
| `b[(r)nomatch]=Q` | `x y z Q` — and so does a missed search |
| `b[(r)y]+=Q` | `x yQ z` — the element joined rather than replaced |
| `b[(e)1+1]=Q` | `x Q z` — no search flag, so an ordinary subscript |

The operand is a word of its own here as it is on the read side, so
`b[(r)$want]=Q` performs the substitution. And a group is only a group
where the *source* wrote one: `g='(r)y'; b[$g]=Q` is `bad math expression`
in the shell with the construct, because the operand behind a group is
lexed as a word and a group that arrived from an expansion has no operand
to lex.

`${b[(r)]:=V}` is the same question from a third direction and answers
the same way — with `b=(x "" z)` the search finds the empty element and
the operator writes `V` there. All three sides have to agree or the same
subscript names two different elements depending on which one wrote it.

### A search on the left over a name holding a string

The same group over a **scalar** names a character position, and the
assignment writes there. The semantics still need nothing new: the search
answers a subscript exactly as it does over an array, and
`ScalarSubscriptIsACharacter` — which already decided that `${s[2]}` is a
character — is what says what that subscript reaches. Measured 2026-09-10
on zsh 5.9.2 with `s=hello`, each write paired with the read that names
the same position:

| probe | result |
| --- | --- |
| `s[(r)l]=Q` | `heQlo`, and `${s[(i)l]}` is 3 |
| `s[(i)l]=Q` | `heQlo` — the two letters name one position |
| `s[(I)l]=Q` | `helQo`, and `${s[(I)l]}` is 4 |
| `s[(R)l]=Q` | `helQo` |
| `s[(r)l]=QQ` | `heQQlo` — a span of one, not a position overwritten |
| `s[(r)l]=` | `helo` |
| `s[(r)l]+=Q` | `helQlo` — the character joined, back where it was |
| `s[(i)zz]=Q` | `helloQ` — a forward miss is one past the last, so it appends |
| `s[(r)zz]=Q` | `helloQ` — and so is the other forward letter's |
| `s[(n:2:i)l]=Q` | `helQo` — the rest of the group reads as it does on the right |
| `s=hello world; s[(r)wor]=Q` | `hello Qorld` — the operand matches a substring |

A name holding *nothing* is not a string and is not searched: `s[(r)l]=Q`
on an unset name leaves a one-element array, which is the boundary
`spliceScalarElem` draws from the numeric side.

This side refused the whole of it until the character write existed
(#1532): the index was honest and the store it would have reached was
not, so `s[(r)b]=Z` on `abc` would have left ` Z` — a plausible wrong
string at status 0.

Three shapes are refused rather than written to a plausible wrong
element, and each is one the shell declines or answers some other way:

- a **table**: `m[(r)v]=Z` is `attempt to set slice of associative array`
  there. The letters mean something else over keys, and the read side
  refuses them for the same reason.
- `(R)` **missing**: `assignment to invalid subscript range` there.
- `(I)` **missing**: puts the value at the *front* there — `b[(I)nomatch]=W`
  on `(x y z)` gives `W x y z` with four elements — which is neither the
  index one before the first, which is what a read answers, nor anything a
  write can name. Note that this and the row above are not the same
  answer as each other, which is why neither is guessed at.

Over a **string** the last two are one answer and it is the store's, not
a refusal by name: both letters miss to the position before the first
character, which is the subscript `s[0]=Q` is refused for, so
`assignment to invalid subscript range` is what comes out. That is what
zsh says about `s=hello; s[(R)zz]=Q` too. Its `(I)` missing is not
reproduced: `s=hello; s[(I)zz]=Q` there is `hellQhello`, and
`s=abcdefgh; s[(I)zz]=Q` is `abcdefgQabcdefgh` — the string without its
last character, the value, and then the whole string — which is no rule,
and the same index written any other way is refused in that shell. An
empty string parts the same way: `${e[(i)x]}` and `${e[(I)x]}` are both 0
there, and 0 is refused for `e[(r)x]=Q` and written for `e[(i)x]=Q`,
which is the same inconsistency from the other side.

An **empty** group is `bad pattern` there and a parse error here — the
lexer takes `()` for a function definition rather than a group, so the
word ends at the parenthesis — so neither writes.

**A refusal on this side ends the line at a failing status**, which is
what the shell does for each of the three shapes above and what the read
side has always done. It did not: the refusal set the flag an *expansion*
reads and nothing on the assignment path read it, so the line carried on
and ended at whatever the command before it had left. Nothing was written,
so the value was at least not wrong — but a script testing `$?` after one
of these was told it had succeeded, which is the opposite of the point of
refusing by name (#1536).

### On `unset`'s operand

The third side of the group, and the one where the subscript arrives as a
**runtime string** rather than as a word the parser lexed — so the group had
nowhere to hang its operand and the whole of `(r)y` went to the arithmetic as
`bad math expression`. It is read as the reference it is now: one parse, from
which the group, its operand and the plain arithmetic reading all come.

Measured 2026-09-12 on zsh 5.9.2 with `b=(x y z)`:

| probe | leaves |
| --- | --- |
| `unset 'b[(r)y]'` | `x`, an empty element, `z` — three long |
| `unset 'b[(re)y]'` | the same |
| `unset 'b[(r)y*]'` | the same — the operand is a pattern |
| `unset 'b[(i)y]'` | the same — the index form names that element |
| `unset 'b[(R)x]'` on `(x y x)` | `x y`, then an empty — the last match |
| `unset 'b[(r)nomatch]'` | nothing removed: the miss is one past the last |
| `unset 'b[(e)2]'` | no selecting flag, so an ordinary subscript |
| `w=y; unset 'b[(r)$w]'` | the same as the first — the operand is expanded |
| `unset 's[(r)l]'` on `hello` | `helo` — a search over a string is a character |

What is left behind is the dialect's answer and not this construct's —
`UnsetArraySpanLeavesOneEmptyElement` — so all the group decides is *which*
element, exactly as it does on the left of an assignment.

An **association** is answered before the group is reached, and is measured
to want that: `unset 'm[(r)v]'` changes nothing there, the brackets being a
key.

A refusal on this side does **not** end the line, which is where the two
sides part company: an assignment's is fatal because nothing was written and
the line must not report the status of whatever came before it, while `unset`
writes one complaint per operand and goes on to the rest — the same shape it
already has for a subscript that will not evaluate, and what the shell does
(#1275).

### A flag group in each end of a range

Each end of a range carries a group of its own, and a search in one
answers with the **index** it matched at rather than with the element,
which is the rule `(i)` and `(I)` follow anywhere. Measured 2026-09-12 on
zsh 5.9.2 with `s="hello world"` and `a=(p q r)`:

| probe | is |
| --- | --- |
| `${s[(r)l,(r)o]}` | `llo` — the same as `${s[3,5]}` |
| `${s[3,(r)o]}` | `llo` — only the second end searches |
| `${s[(r)w,-1]}` | `world` — only the first |
| `${a[(r)q,(r)r]}` | `q r` |
| `${a[(R)p,3]}` | `p q r` — a reverse search is where it matched last |
| `${s[(r)zz,-1]}` | empty — that miss starts one past the last |
| `${s[(R)zz,-1]}` | the whole string — and that one, one before the first |
| `${a[(rn:2:)*a,-1]}` | the modifiers are read in an end too |
| `${a[(e)1,(e)2]}` | `p q` — a group that selects nothing leaves the end arithmetic |

**The four selecting letters are not interchangeable between the two
ends.** `i` and `I` are read in the second one and refused in the first:

| probe, `a=(p q r)` | is |
| --- | --- |
| `${a[1,(i)r]}` | `p q r` |
| `${a[1,(I)q]}` | `p q` |
| `${a[(i)q,2]}` | `invalid subscript`, and the line ends at 1 |
| `${a[(I)q,3]}` | the same |
| `${a[(ri)q,2]}` | the same — the last selecting letter is what counts |
| `${a[(ir)q,2]}` | `q` — and here that letter is `r` |
| `${a[(e)1,(i)r]}` | `p q r` — so it is the position and not the letter |

**The comma has to have been written**, which makes the split the
parser's rather than the run's. Two measurements say so: `a=(p "q,r" s);
${a[(r)q,r]}` is empty rather than the element whose value holds the
comma, so a comma inside what looks like an operand still separates the
ends; and `i="1,2"; ${a[$i]}` is the *first* element rather than a range,
so a comma arriving through a substitution separates nothing. The second
half is not answered here yet — a substituted comma is still read as a
range by this implementation, filed as #2160.

Read as one operand, the search looked for a literal comma and a
parenthesis, missed, and the whole range came back empty at status 0
(#1533). `syntax.SubscriptRange` is where the two ends are separated.

An end that is **no expression at all** ends the line, and the shell
names the end rather than the pair: `${a[b c,2]}` is `bad math
expression: operator expected at` `c` at status 1. It was answered empty
at status 0 here — the span reported the failure and the reading that
chooses between a range and an arithmetic comma dropped it (#2161).

### Grammar

Grammar flag `ArraySubscriptFlags`. The group is read by the **parser**,
where the subscript's text is still text — `ParamExpr.IndexFlags` carries
it and `ParamExpr.Subscript()` is the operand behind it, so every reading
of a subscript asks one accessor and none of them sees the group.
`ParamExpr.Index` keeps the subscript as written, which is what a
diagnostic naming `a[(re)x]` needs.

Malformed is not an error. A character the group cannot carry, an
argument-taking flag with no argument, an argument with no closing
delimiter and a group with no closing parenthesis all mean the
parentheses were never a group — the subscript stands as written and is
read as arithmetic. That is measured (`${a[(z)2]}`, `${a[(n)2]}` and
`${a[(n:2)x]}` are all `bad math expression`) and it is what makes the
flag purely additive: with the flag off, nothing reads differently.

The **lexer** needs it too, for the brace-less spelling: `(` ends a word,
so `$a[(r)b]` was a syntax error naming the parenthesis — which is worse
than a wrong answer, because it takes the whole file with it. A group the
grammar can read is stepped over as a unit and nothing else about where a
bare subscript ends changes.

An assignment's subscript needs no lexer work of its own, which is not
what the shape of the problem suggested: `a[(r)y]=Q` was once a parse
error blaming the parenthesis, and the diagnosis written down at the time
was that the word ended there. It does not — a `(` mid-word already opens
a pattern group where the dialect has them — so what was missing was only
the *scan*, and `Assign.IndexFlags` carries it exactly as
`ParamExpr.IndexFlags` does for a read.

### What this implementation carries, and what it refuses by name

`r R i I e n b` are carried, for an ordinary array, for an associative
array, for the positional parameters and for a scalar.

`w f p k K s` are read by the grammar and **refused by name** when the
subscript is reached — `${a[(w)x]}: the (w) subscript flag is not
implemented` — for the reason the expansion flags are: a subscript flag
answered wrong returns a plausible element at status 0, which is the one
failure this repository exists to avoid.

#### A search over a scalar

The third target, and the same four letters again: over a plain string
the search counts through the string's **characters**. Measured on zsh
5.9.2 with `s="hello world"`:

| written | zsh 5.9.2 | what it selected |
| --- | --- | --- |
| `${s[(i)l]}` | `3` | the position of the first match |
| `${s[(I)l]}` | `10` | and of the last |
| `${s[(r)l]}` | `l` | which position `r` reads the character at |
| `${s[(R)[hd]]}` | `d` | so `r` and `R` part where the characters do |
| `${s[(i)wor]}` | `7` | the operand matches a **substring** |
| `${s[(r)wor]}` | `w` | and `r` still answers one character |

So it is not the array's search over the one-element list a scalar is
otherwise read as — that would make `${s[(r)wor]}` the whole string —
and not a match against each character on its own either, which would
make every multi-character operand a miss. It is the array's walk with a
**prefix** match at each position.

`r` and `R` are the index that walk found read as an *ordinary*
subscript, which is the whole of what they add: the misses are the
array's two out-of-range indices — `${s[(i)zz]}` is `12` and
`${s[(I)zz]}` is `0` — and `${s[(r)zz]}` and `${s[(R)zz]}` are empty
because `${s[12]}` and `${s[0]}` are each no character.

`(e)`, `(n:expr:)` and `(b:expr:)` are read here as they are over an
array: `${s[(ie)lo]}` is 4, `${s[(in:2:)l]}` is 4, `${s[(ib:5:)l]}` is
10. And the character is the locale's rather than a byte, the same unit
`${s[2]}` counts: `s="héllo"; ${s[(i)l]}` is 3 under a UTF-8 locale and
4 under `LC_ALL=C`.

Two edges are measured rather than derived. **One position past the last
character is searched**, which the walk over elements has no equivalent
of because only an empty match can land there: `${s[(I)*]}` is 12 where
`${s[(I)?]}` is 11. `(b:expr:)` does not reach that position —
`${s[(Ib:12:)*]}` is 0 — so a start named explicitly is one of the
characters or nowhere at all. And an **empty string** answers `0` for
both letters, which is neither out-of-range index: `e=; ${e[(i)x]}`,
`${e[(I)x]}` and `${e[(i)*]}` are all 0, where one past the last
character would be 1. A name holding *nothing* is a third answer again —
empty, and still unset, so `${nos[(i)x]-none}` is `none`.

This is the shape a plugin manager reaches three times before it has
loaded anything, on a name that holds a string rather than the array it
reads as: `opts="-X -w"; ${opts[(r)-X]}` is `-` and `${opts[(r)-C]}` is
empty, which is exactly the present-or-absent test the caller writes.

The reading is not asked of the dialect. A subscript on a string is a
character in one grammar and the one element in another
(`ScalarSubscriptIsACharacter`), but the group that makes these
characters a search at all is the first grammar's, and it answers that
question one way — so there is no disagreement here to put an axis on.

#### A search over an associative array

The same four letters, a different construct. Measured on zsh 5.9.2 with
`typeset -A m=(a 1 b 2)`:

| written | zsh 5.9.2 | what it selected |
| --- | --- | --- |
| `${m[(i)a]}` | `a` | the first matching **key**, not an index |
| `${m[(i)*]}` | `a` | still one key |
| `${m[(I)*]}` | `a b` | **every** matching key |
| `${m[(r)2]}` | `2` | the first value whose **value** matched |
| `${m[(r)a]}` | *(nothing)* | a key is not a value |
| `${m[(R)*]}` | `1 2` | every matching value |

So the case of the letter is **how many** matches come back rather than
which end the search started from — there is no "last match" here to be
the mirror of a first — and the letter itself says which half of the pair
is searched: `i` and `I` read the keys, `r` and `R` the values.

**Nothing matched is nothing, and it is still an answer.** `${m[(I)zz]}`
and `${m[(i)zz]}` are both empty rather than the out-of-range index an
ordered array answers with, and the empty is *set*: `${m[(I)zz]-none}` is
empty where `${m[zz]-none}` is `none` and `${a[(r)zz]-none}` on an ordered
array is `none` too. A script reading a hook table cannot tell an
unimplemented flag from a table with no such hook by the value, so the
distinction that carries the difference is the diagnostic and the status —
a refusal writes a line naming the flag and exits non-zero; an answer of
no keys writes nothing and exits 0.

**Two of the modifiers are ignored**, measured rather than assumed: with
three matching keys, `${m[(in:3:)a*]}` and `${m[(ib:2:)a*]}` are both the
*first* of them, so neither `(n:expr:)` nor `(b:expr:)` moves the search.
`(e)` is read — `${m[(Ie)a*]}` finds the key spelled `a*` and not the key
`aa`.

**The search names several elements, so it is a list**, and the three
readings that ask have to agree: `${#m[(I)*]}` is the match count,
`"${m[(I)*]}"` is one field with the matches joined on IFS, `${m[(I)*]}`
unquoted is one field each with any spaces in a key intact, and
`${(@)m[(I)*]}` keeps the fields through quotes. That last shape is why
`${(on)m[(I)pat]}` sorts the matches instead of sorting one word made of
all of them, which is the reading a real plugin manager writes twenty-six
times.

**Which half of each matched pair is substituted is the expansion's own
flag group**, and the answer does not depend on which letter searched:
`${(k)m[(R)*]}` is the keys, `${(v)m[(i)*]}` the value of the one match,
and `${(kv)m[(I)*]}` key and value as two consecutive words each. Without
`k` or `v` the search's own half is what comes back.

**The order several matches come back in is the table's key order.** zsh's
is its hash's — `typeset -A m=(one 1 two 2 three 3); ${m[(I)*]}` is
`one two three`, which is neither sorted nor the order assigned — and it
promises none, so nothing here imitates it. This implementation answers in
its own key order, which is sorted, for the reason `AssocArray.keys()`
gives. The **invariant** both shells hold and the corpus may rely on is
that `${m[(I)*]}` is `${(k)m}` filtered to the matches; the sequence
itself is pinned nowhere.

`(k)` and `(K)` are still refused by name over an association, and they
are not searches there at all: `${m[(k)a]}` is the value at the key `a`
and `${m[(k)*]}` is nothing, because `*` is a key nobody assigned.

An operator written on a search follows the rule `${a[*]}` already
follows, and the two halves are opposite. Measured with three matching
keys `pa pb pc` and a trim of `#p`:

| written | zsh 5.9.2 |
| --- | --- |
| `"${m[(I)p*]#p}"` | `a pb pc` — joined, then trimmed once |
| `${m[(I)p*]#p}` | `a`, `b`, `c` — one field each, each trimmed |
| `"${(@)m[(I)p*]#p}"` | the same three, kept through the quotes |
| `"${m[(I)p*]:#pa}"` | `pa pb pc` — the filter tests the joined text |
| `"${m[(I)p*]:1}"` | `pb pc` — a slice slices the list |

Both halves are carried. The quoted one is
`OperatorDistributesOverStarSubscript` answered the way `[*]` answers it,
which is why routing the search through the same path as `[*]` was the
whole of the change rather than a special case beside it.

Which match came first decides the quoted answer, so that row is asserted
in a unit test against this implementation's own key order and is
deliberately **not** in the corpus, where a key order neither shell
promises may not be pinned.

`a[(r)y]=Q` — a flag group on the left of an **assignment** — is a
parse error here and an element replacement in zsh. The group is read
from a subscript's text and an assignment's subscript is cut by the lexer
before there is any text to read, so it needs its own change rather than
this one.

An index reported by `(i)` or `(I)` is the base plus the element's
*position*, which is the same number only while the array has no gaps.
It has none in the shell that has this construct — `a=(x); a[5]=y` there
leaves five elements and `${a[(i)y]}` is 5 — so the two readings agree
everywhere this can be written today.

### What this implementation does not match

A backslash before an **ordinary** character. zsh keeps both characters —
with an element whose value is the four characters `bet\a`,
`${a[(r)bet\a]}` finds it and does *not* find `beta` — where the pattern
language this implementation shares between `case`, `[[ ]]` and pathname
expansion reads `\a` as an escaped `a`. A backslash before a
metacharacter agrees: `${a[(r)be\*]}` finds an element whose value is
`be*` in both. The disagreement is one character wide and belongs to
patterns.md rather than to this construct, which is why it is recorded
here and not worked around here.

### What the corpus pins

`array/a-subscript-takes-its-own-flag-group` (the six-shell split),
`array/a-subscript-flag-group-searches-both-ways`, `-exact-matching`,
`-unknown-flag-is-arithmetic`, `-operand-is-text-as-written`,
`-that-selects-nothing` and `-counts-matches-and-moves-the-start`.

## More than one subscript — zsh only

A braced expansion may carry **several** subscripts, each reading what
the one before it named:

    ${m[k][2]}      the second character of the value under `k`
    ${a[2,4][1]}    the first of the three elements the range named

There is one rule behind both and it is a rule the language already has:
a subscript counts **elements** when it is handed a list and
**characters** when it is handed one string. A chain only changes where
the thing it is handed comes from.

Measured 2026-09-08 on zsh 5.9.2, with `a=(one two three four five)`:

| probe | zsh 5.9.2 |
| --- | --- |
| `${a[1][2]}` | `n` — one element, so characters |
| `${a[2,4][1]}` | `two` — three elements, so elements |
| `${a[@][2]}` | `two` |
| `${a[2,4][1,2]}` | `two three`, and two fields unquoted |
| `${a[2,4][2][3]}` | `r` — the rule again, as deep as it is written |
| `${s[2,4][2]}` on `s=abcdef` | `c` — a range over a string is a substring, which is one value |

The rest of the panel divides five ways on `${a[1][2]}`, which is what
makes this a grammar's construct and not the language's: bash 5.3.15 answers
`${a[1][2]}: bad substitution` at 1 and that binary as `sh` answers the
same words at 127, bash 3.2.57 reads the first subscript and **ignores**
the second and answers `two`, ksh93 answers empty at 0, and dash has no
arrays to subscript at all.

### The shape is the *last* subscript's

How many fields the whole expansion makes, and whether `${#…}` is a
count or a width, are questions about the final subscript rather than
the first. Measured with `a=(one two three)`:

| probe | zsh 5.9.2 |
| --- | --- |
| `set -- ${a[1,3][1,2]}; echo $#` | `2` |
| `set -- ${a[1,3][2]}; echo $#` | `1` |
| `${#a[1,3][1,2]}` | `2`, a count |
| `${#a[1,3][2]}` | `3`, a width |
| `${#a[@][2]}` | `3`, a width |

A link before the last that named nothing leaves the whole expansion
**unset**, which is what the conditionals test: with `a=(x y)`,
`${a[9][1]-none}` is `none`.

### It is not the nested spelling with the braces left out

`${${a[2,4]}[1]}` and `${a[2,4][1]}` are different expansions, and
quoting is what separates them. The nested spelling has an inner
expansion, and quoting joins what that came to before the subscript is
read: `"${${a[2,4]}[1]}"` is `t`, the first character of
`two three four`. The chain has no inner expansion for the quotes to
join, so `"${a[2,4][1]}"` is `two` — the same answer it gives unquoted.

### The chain is the braced spelling's

Written without braces this shell reads **one** subscript and leaves the
rest as ordinary text: with `a=(hello world)`, `"$a[1][2]"` is
`hello[2]`, where the four columns that answer at all say
`hello[1][2]`. So the
bare form is not a shorter way to write a chain, and
`array/a-subscript-without-braces-is-read-once` above already pinned
that half.

### Grammar

Grammar flag `ChainedSubscript`, consumed by the **parser**, which reads
the brackets in a loop rather than once. Without it the second bracket
is not consumed and the leftover text makes the expansion unreadable,
which is exactly how the grammars without the chain answer
`${a[1][2]}`.

The parsed shape keeps the **last** subscript where a single one has
always lived, in `ParamExpr.Index`, and the earlier ones in
`ParamExpr.Leading`. That is deliberate and it is why nothing that reads
a subscripted node had to be taught that chains exist: every question
anything asks of one — whether it is the whole array, whether it is a
range, how many fields it makes, whether its length is a count or a
width — is a question about the final subscript, and the leading ones
only say what that final one is read against.

### What this implementation refuses by name

A chain on a **nested** expansion — `${${a}[1][2]}`, which zsh answers
`e`. That is the two constructs at once, and the second link would have
to read what the first named through the inner expansion's shape rather
than through a name's. It is refused by name —
`a chain of subscripts on a nested expansion is not implemented` —
rather than answered with the last subscript alone, which would be a
plausible value at status 0.

A chain behind a **search over an association** — `${m[(I)*][2]}`. zsh
does not read the rest of the chain against what the search named at
all: measured with `typeset -A m=(k1 vA k2 vB)`, `${m[(I)k1][1]}` is `1`
and `${m[(I)k1][2]}` is `2` — the subscript itself, whatever the keys
and the values are — and `${m[(r)vA][1]}` is the whole of `vA` rather
than its first character. There is no rule there to model, so the shape
is refused by name rather than answered with the plausible key an
ordinary source reading would produce. A search over an *ordered* array
is a source like any other and is read: `a=(one two three)` makes
`${a[(r)two][1]}` the character `t` in both.

### What this implementation does not match

A subscript that misses is **unset** here and set-and-empty in zsh, when
the subscript counts characters: with `a=(x y)`, `${a[1][2]-none}` is
empty there and `none` here. It is not this construct's divergence — the
single-subscript spelling has it too, `s=abc; ${s[9]-none}` is empty in
zsh and `none` here, and so does the nested spelling — so the chain
inherits it rather than introducing it, and it is recorded here because
this is where a chain first reaches it.

### What the corpus pins

`subscript/a-second-subscript-counts-characters` (the five-way split),
`-on-an-association`, `-after-a-range`,
`subscript/a-range-in-the-second-position`, `subscript/a-third-subscript`,
`subscript/a-length-over-a-chain`, `subscript/a-chain-is-braced-only` and
`subscript/a-chain-whose-first-link-names-nothing`.

## Choosing elements: `:#`, `:|` and `:*` — zsh only

Three operators that change **which elements** a value has, where the
substring above changes which characters each one has.

    ${a:#pattern}   drop the elements the pattern matches
    ${a:|other}     drop the elements the array named `other` holds
    ${a:*other}     keep only those

They share the `:` with the substring, and that is the whole difficulty:
the single character after the colon decides which construct was
written, before anything can be evaluated. Grammar flag
`ParamElementSelection`, for the reason `BareSubscript` is one — the
panel does not disagree about what these characters *mean*, it cuts the
word in different places, and three shells cut it three ways:

| `v=hello` | bash 5.3 / 3.2 / as-`sh` | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `${v:#hel*}` | `operand expected`, 1 | `Bad substitution`, 2 | `lo` | *empty* |
| `${v#hel*}` | `lo` | `lo` | `lo` | `lo` |

bash reads an offset and hands `#hel*` to arithmetic. **ksh93 accepts
the same six characters and means `${v#hel*}` by them** — the colon is
simply ignored, so it is prefix removal and not exclusion. Only zsh has
the operator, and it matches the pattern against the **whole** value.
Treating `:#` as `#` with a colon in front would answer ksh93's question
inside zsh's grammar, silently.

### What the operator does

The pattern is matched whole and never against a part, which is the
entire difference from `#`. Consequences, all measured:

    a=(one two three); ${(@)a:#t*}     one
    a=(one two);       ${(@)a:#*}      no fields at all
    a=("" one);        ${(@)a:#}       one — the empty pattern matches
                                       the empty string and nothing else
    a=(One two);       ${(@)a:#one}    One two — case-sensitive

A scalar is the one-element list it is, so the answer is the value or
nothing — and *nothing* here is one empty field, where an emptied list
is no field at all. `set -- "${v:#hel*}"` leaves `$#` at 1 and
`set -- "${(@)a:#*}"` leaves it at 0.

The pattern is built from the operand's **word**, so whether a
metacharacter out of a parameter is a metacharacter is
`GlobExpansionResults` — the same axis that decides whether
`p='et*'; echo $p` expands. In the shell that has the operator that axis
is false, so `p='t*'; ${(@)a:#$p}` removes nothing.

`:|` and `:*` take the **name** of another array rather than a word, and
compare elements for **equality** rather than by pattern: an element
`t*` in the other array removes only a literal `t*`. A name nothing is
stored under is an empty set, quietly — the difference keeps everything,
the intersection keeps nothing, and neither complains.

### The disambiguation is exactly one character

Every other spelling after a colon is what it always was, and all of
them are unanimous across the panel:

    ${v:2}  ${v:2:2}  ${v: -2}  ${v:-alt}  ${v:=set}  ${v:?msg}  ${v:+set}

`${v:}` is too short to carry an operator and stays the substring with an
empty offset. A grammar that widened the rule by one character would
break the shapes every shell shares rather than the ones only zsh has.

## Replacing whole elements: `:/` — zsh only

    ${a:/pattern}              drop the elements the pattern matches whole
    ${a:/pattern/replacement}  replace them with the replacement

The fourth operator of the family above, and the only one of the four
that *writes* rather than only choosing. It is not `${a/pat/repl}` with
a colon in front: that one substitutes a matching **span** inside each
element, and this one tests the **whole** element and replaces all of
it. Measured on zsh 5.9.2:

    x=foobar; ${x:/foo/Z}    foobar   — `foo` is not the whole value
    x=foobar; ${x/foo/Z}     Zbar     — the span reading
    x=foo;    ${x:/foo/Z}    Z

The disambiguation is the same one character the three above use, and
`/` cannot begin an arithmetic expression, so it cannot take input from
the offset: `${x:3/2}` is still an offset of `3/2`, and `${x: /2}` — the
same slash one space later — is still `bad math expression: operand
expected`. `${x:2:/1}` shows the rule is the *first* colon's alone: the
length after the second colon stays arithmetic and refuses the slash.
Grammar flag `ParamWholeElementReplace`, set where
`ParamElementSelection` is and for the same reason.

### The pattern ends at the first unquoted slash

As in `${x/pat/repl}`: `${x:/foo/a/b}` on `foo` is `a/b`, so the
replacement holds the rest of the text however many slashes are in it.
Omitting the second slash is the empty replacement — `${(@)a:/b*}` and
`${(@)a:/b*/}` answer alike.

### An element replaced by nothing is dropped

Measured with `a=(foo bar baz)`:

    "${(@)a:/b*/Z}"     foo Z Z          three fields
    "${(@)a:/b*/}"      foo              one — the emptied elements go
    "${(@)a:/b*}"       foo              the same, spelled without the slash

An element that was *already* empty and does not match is kept:
`a=(foo "" bar)` under `${(@)a:/x/Y}` is still three fields. So it is
the replacement being empty that removes an element, not emptiness in
the result.

A scalar is not a list and keeps its empty value: `x=foo; ${x:/foo}` is
one empty field, where `${(@)a:/foo}` on `a=(foo)` is no field at all —
the same split `:#` has.

### No match is silent

The operator leaves a value it does not match completely alone and says
nothing: `x=/a/b/c; ${x:/b/Z}` is `/a/b/c`, status 0, and an array
without `(@)` in double quotes joins first and so matches nothing —
`a=(foo bar); "${a:/foo/Z}"` is `foo bar`. This is the half a fix that
only handles the matching case gets wrong, and it is the half three real
startup files depend on.

### Distribution and quoting follow `:#`

    a=(foo bar baz)
    "${a[@]:/ba*/Z}"    foo Z Z    one field per element
    ${a[*]:/ba*/Z}      foo Z Z    unquoted `[*]` distributes too
    "${a[*]:/ba*/Z}"    foo bar baz    quoted `[*]` joins *first*, so the
                                       pattern is tested against the join
                                       and matches nothing

### `(#b)` and `(#m)` are live in the replacement

Under `extendedglob`, and by the same rule `${x//pat/repl}` follows: the
replacement is expanded **once** for the whole operator when the pattern
reports nothing, and **again for each match** when it reports.

    a=(x y z); i=0; ${(@)a:/*/$((++i))}          1 1 1
    a=(x y z); i=0; ${(@)a:/(#m)*/$((++i))}      1 2 3
    a=(foo bar);    ${(@)a:/(#m)*/<$MATCH>}      <foo> <bar>
    a=(ab cd);      ${(@)a:/(#b)(?)(?)/<$match[2]$match[1]>}   <ba> <dc>

Without `extendedglob` there is no `(#m)` to report: the same line reads
`(#m)*` as an ordinary pattern, matches nothing, and leaves the array
alone. The pattern operand is expanded once either way — a command
substitution in it runs a single time for the whole array.

`${(M@)a:/foo/Z}` is `Z bar`: the `M` flag, which reads `:#` from the
other side, has nothing to reverse here and is ignored.

## Interleaving two lists: `:^` and `:^^` — zsh only

    ${a:^other}     pair the elements of `a` with those of the array
                    named `other`, stopping when either runs out
    ${a:^^other}    the same, but cycle the shorter of the two until
                    the longer is spent

The one operator in this family that produces a list **longer** than
either input, which is why it is not one of the four above: they choose
or rewrite elements and this one interleaves, so a reading that maps one
output per input has nowhere to put the second field. Measured on
zsh 5.9.2:

    a=(1 2 3); b=(x y z);   ${a:^b}     1 x 2 y 3 z
    a=(1 2);   b=(x y z w); ${a:^b}     1 x 2 y
    a=(1 2);   b=(x y z w); ${a:^^b}    1 x 2 y 1 z 2 w
    a=(1 2 3 4); b=(x);     ${a:^^b}    1 x 2 x 3 x 4 x

Either side may be the shorter one; `:^` stops at whichever ends first
and `:^^` cycles whichever ends first.

The operand is the **name** of another array, as `:|` and `:*` take one
and unlike the pattern operators — so there is nowhere to write a
subscript on it, and reading a bare name as its elements is
`ArrayNameWithoutSubscriptIsTheList` rather than anything this operator
decides. A scalar is the one-element list it already is, on either side:
`s=one; b=(x y); ${s:^b}` is `one x`.

A name nothing is stored under contributes no second list at all, and
the left operand comes back untouched — which is the **opposite** of
what the set operators do with the same absence, where an empty set
removes everything or keeps nothing:

    a=(1 2); ${a:^nosuch}     1 2
    a=(1 2); ${a:^^nosuch}    1 2
    a=(1 2); b=(); ${a:^b}    — no field at all
    a=(1 2); b=(); ${a:^^b}   1 2      — nothing to cycle

### The longer spelling has to be read first

`:^^` is its own operator and not `:^` with a caret in its operand.
Nothing else distinguishes them, so a parser that tries the two-character
spelling first answers the doubled one as the plain one — silently, with
half the fields missing and status 0.

### Quoting picks the operand's shape, not the result's

The left side is joined to one word **before** the zip sees it, and what
comes back is still a list:

    a=(1 2 3); b=(x y z); "${a:^b}"     two fields: `1 2 3` and `x`
    a=(1 2 3); b=(x);     "${a:^b}"     the same two fields

That falls out of the ordinary quoting rule rather than needing one of
its own — the quoted `$a` is the joined value, and the zip pairs that one
word with the other list's first element.

Quoting does guarantee what `:#` guarantees: an answer with nothing in it
is one **empty** field rather than no field.

    a=(1 2 3); b=(); set -- ${a:^b};   $# is 0
    a=(1 2 3); b=(); set -- "${a:^b}"; $# is 1, and that field is empty

Set-and-empty is not never-set, and the two are distinguished on the left
as well: an empty array quoted is one empty word, which pairs with the
other list's first element, where a name that was never set is no word to
pair with at all.

    a=();    b=(x y); set -- "${a:^b}"    2 fields: `` and `x`
    unset a; b=(x y); set -- "${a:^b}"    1 field, empty

A probe built from `printf '[%s]'` cannot see any of the three rows
above: it runs its format once even with no arguments, so `[]` is what
comes back both from an empty list and from a single empty field. The
field **count** is what tells them apart, and the missing field here read
as correct until it was counted.

Grammar flag `ParamArrayZip`, set where `ParamElementSelection` is and
for the same reason: without it the colon opens a substring and the caret
goes to the arithmetic that reads the offset, which is what the other
five shells in the panel do with the same characters.

## A process substitution in an operand — bash only

    unset u
    ${u:-<(:)}      →  /dev/fd/63   in bash
                    →  <(:)         in ksh93, zsh and dash
    v='<:'; ${v#<(:)}
                    →  ``           in ksh93 and zsh — a `<` and the group `(:)`
                    →  `<:`         in bash (the pattern is a path) and dash

Whether `<(cmd)` opens a process substitution **inside a `${…}` operand**
is a question about the position, not about the shell: ksh93 and zsh both
have the construct in an ordinary word and neither reads one here. Every
operand answers alike — the pattern of `${v#…}`, the replacement of
`${v/…/…}` and the word of `${v:-…}`.

Grammar flag: `ProcessSubstitutionInParamOperand` — bash only; false for
`core` and `posix`, on the additive rule, since it is a construct one
shell admits in a place the others do not.

It matters most where the operand is a pattern, because there the wrong
answer is silent: the text the substitution *encloses* is never the
pattern in any column, and reading it that way trimmed a bare `x` off
`${v#<(x)}` and matched `case x in <(x))` (#902).

## An expansion where a name belongs — zsh only

    v=abc
    ${${v}}              →  abc
    ${${v}#a}            →  bc
    ${${${v}#a}%c}       →  b
    ${(U)${(L)v}}        →  ABC
    ${$(echo abc)#a}     →  bc

zsh alone. bash 3.2, bash 5.3 and dash answer `bad substitution` for
every one of them and ksh93 a syntax error, so this is a grammar flag —
`NestedParamExpansion` — and not a semantics axis: without it there is no
parameter name at the front of `${${v}#a}` at all, so the characters are
unreadable rather than differently read, and there is nothing for a value
to switch between.

It is not a corner. `${${0:#$ZSH_ARGZERO}:-${(%):-%N}}` is how a plugin
manager installed on this machine finds the file being sourced, and the
form appears in two of the third-party files a real startup reads.

**The inner expansion is the whole of the name position.** Measured:
`${x${v}}` and `${${v}x}` are both a bad substitution *in zsh too*, so
text either side of the nesting is not a longer name. Every operator may
follow the inner brace — trims, replacement, substring, the four
conditionals, `:#`, a modifier — and the length prefix may sit in front
of the whole thing. Depth is not limited to one.

The inner need not be another `${…}`: a `$(…)` command substitution and a
`$((…))` arithmetic expansion stand in the same position. The backquoted
spelling does not, which is measured rather than assumed — ``${`echo x`#a}``
is a bad substitution in zsh, so the two spellings of command
substitution part company exactly here.

### A subscript on the result — and on the name a `(P)` names

    a=(x y z)
    ${${a[@]}[2]}        →  y        the element, counted from the base
    ${${a[@]}[-1]}       →  z
    ${${a[@]}[(I)y]}     →  2        the index the search found
    ${${a[@]}[(I)zz]}    →  0        and one before the base for no match
    ${${a[@]}[(i)zz]}    →  4        one past the end, the append position
    v=abc; ${${v}[2]}    →  b        a string is read by character

Both halves of that were already built — the subscript with its flag
group, and the nesting — and what was missing was applying the one to the
result of the other. It is not a corner: `add-zsh-hook` line 84 is

    if (( ${${(P)hook}[(I)$fn]} == 0 )); then

which is how essentially every zsh plugin installs a `precmd` or a
`preexec`, and the refusal left that arithmetic with an empty operand, so
the function printed its usage and gave up and a real session exited
without drawing a prompt (#1381). **`0` and empty are not
interchangeable there**, which is why the index of no match is recorded
above as an arithmetic value rather than as text.

**`${(P)h}` is a name, and every other inner is a value.** The split is
measured, and the reference reading is the one that carries a parameter's
own shape through:

    typeset -A m=(k v); h=m
    ${${(P)h}[k]}        →  v        a key, which no field position is
    a=('' y); h=a
    ${${(P)h}[1]}        →  ``       the empty element is there …
    ${${a}[1]}           →  y        … where the *fields* have dropped it
    a=(hello); h=a
    ${${(P)h}[2]}        →  ``       a list of one is still a list
    ${"${(P)h}"[2]}      →  y        for `a=(x y)`: quoting does not undo it

So a subscript after a `(P)` reads the parameter the value names, exactly
as if the name had been written out.

**The name is the inner expansion with `P` struck out of its group**, and
the letters left transform the *name* rather than what the name holds.
Measured with `ARR=(x y)`, `arr=(hello)` and `h=arr`:

    ${(UP)h}           →  HELLO    the value, uppercased, unsubscripted
    ${${(UP)h}[1]}     →  x        subscripted it is `${ARR[1]}`
    g=h; ${${(UP)${g}}[2]}  →  `${H[2]}`, through the group's own base
    ${${(P)h:-d}[2]}   →  `${arr[2]}` — the operator runs on the name

An implementation that resolved the name first and applied the remaining
letters afterwards answers `hello` and `h` for the first two, which are
plausible and wrong.

A **count** and a **set test** are names too, which is the sharp end of
"the whole pipeline". With `set -- abc def` and `hh=zz`:

    ${${(P)#hh}[1]}   →  d     `${#hh}` is 2, and `${2[1]}` is `d`
    ${${(P)+h}[1]}    →  a     `${+h}` is 1, and `${1[1]}` is `a`

A rule excluding them — on the grounds that a number is not a name —
looks obviously right and is wrong: the number *is* the name, and the
name is a positional parameter.

Everywhere else the subscript reads what the inner **came to**, and the
one question that adds is whether that result is a *list*, where the
subscript counts elements, or one *string*, where it counts characters.
The field count does not answer it — a list of one element and a string
are both one field — so the shape is the inner expansion's. Measured
with `[2]` against a result of one field:

    a=(hello);   ${${a[@]}[2]}      →  ``   a one-element list
    x=abc;       ${${(f)x}[2]}      →  b    a split that found nothing to
                                            split leaves a string
    a=(aa bb);   ${${(j.,.)a}[2]}   →  a    and a join leaves one too
    a=(hello);   ${${(U)a}[2]}      →  ``   a flag doing neither keeps the
                                            list it was given
    a=(x y z);   ${${a:+abc}[2]}    →  b    a substituted word is its own
                                            value, and this one is a string
    a=(hello);   ${${x:+$a}[2]}     →  ``   while this one is a list
    a=(x y z);   ${${#a[@]}[1]}     →  3    a count is a string
    s=hello;     ${${(A)s}[2]}      →  ``   and `(A)` says outright that
                                            what it made is an array

A missing element leaves the expansion **unset** rather than empty:
`${${a[@]}[9]-none}` is `none`.

**Quoting reaches the inner**, and it is what decides whether a result that
is a list still has fields to count. Measured with `a=(p q r)`:

    "${${a}[2]}"        →  ` `      the bare name joins, as `"$a"` does
    IFS=-; "${${a}[2]}" →  `-`      and the join is on IFS, not a space
    "${${a[@]}[2]}"     →  q        `[@]` keeps its fields in quotes
    "${${*}[2]}"        →  ` `      while `$*` joins and `$@` does not
    "${${(@)a}[2]}"     →  q        and so does the `(@)` flag
    a=(one two); "${${a}#o}" → `ne two`

That is one predicate — `@`, an `[@]` subscript, `(@)` — and it is the same
one the rest of the expansion machinery follows for `"$@"` against `"$*"`.
The last line is the same rule without a subscript: a quoted inner that
joins is a *value*, so the operator applies to it once.

### An inner that comes to a list

**Every element survives, and the outer half applies to all of them.**
A nested expansion whose inner is a list is a list, and the whole of the
expansion machinery treats it as one — which is the same statement as
saying it goes down the same path `${a[@]}` does. Measured with
`a=(x y z)`:

    ${${a[@]}}          →  [x][y][z]   one field per element
    "${${a[@]}}"        →  [x y z]     quoted, joined on IFS
    ${${a[@]}#x}        →  [y][z]      the operator applies to each …
    "${${a[@]}#x}"      →  [ y z]      … and the quotes join what it made
    ${${a[@]}:1}        →  [y][z]      a slice slices the *list*
    ${${a[@]}:#y}       →  [x][z]      a filter drops an element
    ${#${a[@]}}         →  3           and a length counts them
    ${${${a[@]}}}       →  [x][y][z]   through a further nesting
    ${(j:-:)${a[@]}}    →  [x-y-z]     a flag group joins all of them

**Quoting joins where the outer wears no subscript, and the outer's own
subscript decides it where it does** — the inner's `[@]` is what made the
result a list, not a statement about the fields the outer keeps:

    "${${a[@]}}"        →  [x y z]
    "${${a[@]}[@]}"     →  [x][y][z]
    "${${a[@]}[*]}"     →  [x y z]
    "${${a[@]}[1,2]}"   →  [x y]

**A length counts elements rather than characters**, which is where a
list of one is not the same thing as a string: `a=(hello); ${#${a[@]}}`
is 1 where `s=hello; ${#${s}}` is 5. The list-ness is the inner
expansion's shape, asked with the same predicate the subscript reading
above uses.

The headline use is a plugin manager serializing an associative array:
`${(j: :)${(qkv)ICE[@]}}` is one field per key *and* per value, joined.
An implementation that lost half of the inner still had a list, so the
join produced a plausible answer at status 0 and nothing said it was half
the spec (#1509).

### What is read and not implemented

One shape is recognized and refused **by name**, because answering it
approximately is the failure this document exists to prevent:

    ${$(cmd)[2]}        a subscript on a command substitution

It is refused for a reason that is recorded rather than guessed
at. An unquoted command substitution is a **list** in that position even
when it comes to one word — `${$(echo abc)[2]}` is empty where a string
would have answered `b`, and `${$((6*7))[1]}` is `42` where a string
would have answered `4` — and this tree does not field-split an unquoted
substitution in the name position yet (#976), so the fields a subscript
would count are not the shell's. Its quoted spelling is refused with it,
since the same gap decides what `${"$(cmd)"[2]}` came to.

An arithmetic substitution is refused in the same words and for the same
reason: `${$((6*7))[1]}`.

## Dialect flags

    ParamSubstitution      ${x/pat/rep} and its anchored forms
    ParamSubstring         ${x:off:len}
    ParamCaseChange        ${x^^} ${x,,} ${x~~} and their single forms — bash only
    ParamIndirection       ${!x}, ${!prefix*}, ${!a[@]} — parses in bash and ksh;
                           what ${!x} then *means* diverges (see above)
    ParamTransformations   ${x@Q} and its letter family — bash only
    ParamExpansionFlags    ${(U)x}           — zsh only
    ParamTildeFlag         ${~x}, the tilde-and-filename flag — zsh only
    ParamSplitFlag         ${=x}, the split-into-words flag — zsh only
    ParamSetTestFlag       ${+x}, the is-it-set count — zsh only
    ParamElementSelection  ${a:#pat} ${a:|b} ${a:*b} — zsh only
    ParamArrayZip          ${a:^b} ${a:^^b} — zsh only
    ParamWholeElementReplace
                           ${a:/pat/rep}, the same family's fourth
                           operator — zsh only
    BareSubscript          $a[1] and $#a, written without braces — zsh only
    ArraySubscriptFlags    ${a[(re)v]}, a flag group inside the brackets
                           — zsh only
    ChainedSubscript       ${m[k][2]}, a second subscript reading what the
                           first named — zsh only
    SpecialParamSubscript  ${@[1]}, ${1[2]}, ${?[1]} — a subscript on a
                           parameter that is not a name — zsh only
    ProcessSubstitutionInParamOperand
                           ${u:-<(:)} carries one — bash only
    NestedParamExpansion   ${${v}#a}, an expansion where a name belongs
                           — zsh only
    NamelessParamExpansion ${:-abc} and ${}, an expansion with no
                           parameter name at all — zsh only
    BareBraceNestsInExpansion
                           ${u:-{a,q}.z}, where a bare `{` in an *unquoted*
                           operand opens a level so the expansion ends at
                           the brace that balances it — zsh and ksh93

All false for `posix`. `ParamCaseChange`, `ParamIndirection`,
`ParamTransformations`, `ParamExpansionFlags`, `ParamTildeFlag`,
`ParamSplitFlag`, `ParamSetTestFlag`, `ParamElementSelection`,
`ParamWholeElementReplace` and `ParamArrayZip` are false for `core`:
the first and the last three are one shell's, and the `!` family is two
shells' — neither is a common denominator. `BareSubscript` is false for
both, and for the same reason as `ParamExpansionFlags`: one shell reads
those characters that way and three read them as text.
`SpecialParamSubscript` is false for both as well, and its evidence is
stronger still: every other member of the panel refuses the expansion
outright. `ArraySubscriptFlags` is false for both on evidence stronger
again: the four shells that lack it read `${a[(r)v]}` as arithmetic, and
so does the shell that has it whenever the group is not one it knows. `ChainedSubscript` is false for both because the same text
has five readings across the panel: two shells refuse it, one ignores
the second subscript, one answers empty, and one has no arrays. `NestedParamExpansion` is false for both on the same evidence:
the other five columns refuse it, in three different wordings. So is
`NamelessParamExpansion`, on evidence of exactly that shape — `${:-abc}`
is `bad substitution` in the three bashes, `Bad substitution` in dash and
`` `:' unexpected `` while reading in ksh93.

`BareBraceNestsInExpansion` is the one of these the panel splits two
against three rather than one against five, and it is false for `core` for
the ordinary reason: the wider reading takes text the other three columns
leave in the word. Measured 2026-09-10 with
`unset u; printf "[%s]" ${u:-{a,q}.z}`:

| | |
| --- | --- |
| zsh 5.9.2 | `[a.z][q.z]` |
| ksh93 | `[a.z][q.z]` |
| bash 5.3.15 | `[{a,q.z}]` |
| bash 3.2.57 | `[{a,q.z}]` |
| dash | `[{a,q.z}]` |

Two fields is the operand running to `.z` and the group being expanded
afterwards; one field is the operand stopping at the first `}` and `.z}`
arriving as literal text. Brace *expansion* is not what separates them —
`${u:-a{b}c}` has no comma in it and splits the panel the same way, and
dash has no brace expansion at all and still has an answer — so the flag
decides where the word is cut and nothing else.

In double quotes the flag is not consulted: every column ends the
expansion at the first `}` there, which is the `core/a-bare-brace-inside-a-quoted-expansion`
row and what #1586 settled. The `${ cmd;}` command form keeps its own rule
again, because its body is a program and a `{ … }` block written in one has
to balance the way that program's braces do.

## What this does not cover

The pattern-matching language itself — what `*`, `?`, `[…]` and the
bracket-expression classes match — is shared with pathname expansion and
with `case`, so it belongs in its own document rather than being described
three times.
