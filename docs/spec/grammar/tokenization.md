# Tokenization

How input becomes tokens, and how quoting is recorded on them. This is
the stage before everything in `expansion.md`, and it is what supplies
the per-span quoting that document requires.

Citation: POSIX.1-2024 XCU §2.2 (quoting), §2.3 (token recognition),
§2.10 (grammar). Panel measurements as in `../oracle.md`.

## Why this document exists

`expansion.md` asserts that the parser must record quoting **per span
within a word**, not per word. That is a requirement on this stage, and
it is the reason a word cannot be modeled as a string:

    set -- a"b c"d    →  [ab cd]   one field, in all six shells

`a` is unquoted, `b c` is quoted, `d` is unquoted, and the result is a
single word carrying three spans. Splitting and globbing later apply only
to the unquoted spans of what an expansion produced. A lexer that
discards where the quotes were makes those stages unimplementable.

## Quoting

Three forms, and they differ in what they protect.

| form | protects |
| --- | --- |
| `\c` | the single following character |
| `'…'` | everything, including backslash; no escape exists inside |
| `"…"` | everything except `$`, `` ` ``, `\`, and `"` |

Inside double quotes, backslash is an escape **only** before those four
characters and newline. Before anything else it is a literal backslash,
which is measured and unanimous:

| probe | all six |
| --- | --- |
| `printf '[%s]' "a\"b"` | `[a"b]` — escapes the quote |
| `printf '[%s]' "a\nb"` | `[a\nb]` — **backslash kept**, `n` is not special |
| `printf '[%s]' "a\qb"` | `[a\qb]` — likewise |

This is the rule most often implemented wrongly, because C-family
intuition expects `\n` to be a newline. It is not; that is `$'\n'`, a
separate form which `shell-matrix.md` records as core and absent from
dash.

Quote removal happens at the *end* of expansion, not here. This stage
records which spans were quoted and leaves the characters in place.

### `$"..."` is a plain double-quoted string to half the panel

`$"..."` marks a double-quoted string for locale translation. With no
message catalog — the only condition the panel can measure — the
shells that have the form strip the `$` and read an ordinary
double-quoted string; the shells without it leave the `$` as a literal
character in front of one. Measured 2026-09-04, same panel and machine
as `shell-matrix.md`:

| probe | dash | bash3.2 | bash5.3 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `echo $"hello"` | `$hello` | `hello` | `hello` | `hello` | `$hello` |
| `x=world; echo $"hi $x"` | `$hi world` | `hi world` | `hi world` | `hi world` | `$hi world` |

Where the form exists it is `"..."` in every detail, not a third
quoting rule. Measured in bash 5.3 and ksh93, against the plain-quote
rows above:

| probe | bash5.3 and ksh93 |
| --- | --- |
| `echo $"a\nb"` | `a\nb` — backslash kept, as in `"a\nb"` |
| `echo $"a\"b"` | `a"b` — escapes the quote, as in `"a\"b"` |
| `echo $"x$(echo y)z"` | `xyz` — substitutions expand |
| `echo a$"b"c` | `abc` — mid-word, concatenates like any quoting |

A here-document body is not a word, and `$"x"` in one stays literal in
bash — the form exists only where a quote could open.

So the `$` contributes nothing to the parsed tree: the spans of
`$"..."` are exactly the spans of `"..."`, and only *whether the form
parses that way* varies by shell. zsh is on the literal side, which
keeps it out of the core.

Grammar flag: `DollarDoubleQuote` — core: off; `bash` and `ksh`: on.

### `$'...'` decodes escapes, and this is the set

`$'...'` is single quotes with one difference: backslash escapes decode.
`shell-matrix.md` records the form as core — bash, ksh93 and zsh all
have it, and dash keeps every character literally, `$` included. The
form is core; the table it decodes is not one decision but several, and
the whole set below was measured escape by escape on 2026-09-05, same
panel and machine as `shell-matrix.md`. The corpus pins its shape: `\t`
(`core/dollar-single-expands-escapes`, the load-bearing failure mode —
the quoting was once recorded and nothing decoded it), `\e`, `\xHH`,
`\NNN`, the code-point pair, `\cX` twice, the NUL, and an escape with no
meaning.

Unanimous across **all six** columns that have the form — bash 3.2, bash
5.3, that binary as `sh`, ksh93, zsh and BusyBox ash — and decoded here:

| escape | value |
| --- | --- |
| `\n` `\t` `\r` `\a` `\b` `\f` `\v` | the C control characters |
| `\\` `\'` `\"` | the character itself |
| `\xHH` | the byte, one or two hex digits — `$'\x4'` is 0x04 |
| `\NNN` | the byte, one to three octal digits — `$'\101'` is `A` |

The list used to be longer and used to say "bash 3.2, bash 5.3, ksh93
and zsh", which was a panel written before BusyBox had a column. `\e`,
`\E` and `\?` were in it and BusyBox reads none of the three, so they
are axes now and are listed below (#3270). The same shape as #3226 and
#3248: **a table is unanimous only over the columns that were in the
room.**

The octal form is one rule and not two: `\0101` is three digits counted
from the zero onward, so it is a backspace and then a `1` rather than
0x41. A value past a byte wraps into one — `$'\400'` is a zero byte,
which is the truncation rule below and not a separate answer.

The two code-point escapes are an axis,
`DollarSingleUnicodeEscapes` — bash 5.3, that binary as `sh`, ksh93 and
zsh read both, and **bash 3.2 and BusyBox ash read neither**, keeping
the backslash and every digit. Measured 2026-09-16 by `od`:

| escape | value | bash 3.2, BusyBox ash |
| --- | --- | --- |
| `\uHHHH` | the code point, up to four hex digits, as UTF-8 | literal |
| `\UHHHHHHHH` | the same, up to eight digits | literal |

One axis for both spellings, measured rather than assumed: no column
reads one and not the other.

**What a shell does with a code point outside ASCII depends on the
locale.** Under `LC_ALL=C`, bash 5.3 prints `$'é'` as the eight
characters it was written as and zsh refuses it — `character not in
range` — where in a UTF-8 locale both encode it. The corpus case stays
inside ASCII for that reason; ours encodes as UTF-8 whatever the locale,
which is a divergence recorded rather than fixed here.

Three places the panel splits, and each is an axis on the semantics
vector rather than a decision taken here:

- **`\e` and `\E`, the escape character** — `DollarSingleEscEscape`.
  1b in bash 5.3, bash 3.2, that binary as `sh`, ksh93 and zsh; BusyBox
  ash has neither spelling, so `$'\e'` is a backslash and an `e` there
  and `DollarSingleUnknownEscape` decides the backslash. One axis for
  both letters here, where the `echo -e` site needs two — and the two
  sites **disagree inside BusyBox**: `echo -e 'a\eZ'` writes 1b there
  and `$'a\eZ'` does not (#3226, #3270). An answer carried from one
  site to the other is a guess.
- **`\?`, the question mark** — `DollarSingleQuestionEscape`. The
  question mark alone in the five columns that have it; BusyBox ash
  keeps the backslash. It is the one entry of the C set that shell
  leaves out — `\n`, `\r`, `\a`, `\b`, `\f`, `\v`, `\\`,
  `\'`, `\"` and the octal forms were measured beside it and are
  unanimous.
- **`\cX`, the control character** — `DollarSingleBackslashC`. bash and
  ksh93 decode it and zsh has no such escape at all, so `$'\cA'` is
  0x01, 0x01 and `cA`. The two that decode use *different arithmetic*,
  which is invisible over letters: both uppercase the character first,
  and then bash keeps its low five bits where ksh93 toggles bit 6. Those
  agree over `@` through `_` — every letter and six symbols — and part
  everywhere else, so `$'\c1'` is 0x11 in bash and `q` in ksh93. bash
  5.3 special-cases `\c?` as DEL; bash 3.2 has no such case and gives
  0x1f, the plain mask.
- **An escape with no meaning** — `DollarSingleUnknownEscape`. `$'\q'`
  is `\q` in bash (3.2 and 5.3) and `q` in ksh93 and zsh. This was a
  recorded decision following bash until `\cX` made it answerable: zsh
  reaches the rule *through* `\c`, so pinning `$'\cA'` for zsh pins this
  too, and one of them had to become an axis for the other to be right.
- **What a NUL does to the text around it** — `DollarSingleNul`, and the
  panel gives it **three** answers rather than two. bash and ksh93 hold a
  word as a C string, so `$'a\0b'` is `a` and `${#x}` is 1; zsh counts its
  strings and keeps all three bytes; BusyBox ash drops the byte and keeps
  the rest, `ab` at length 2. What ends, in the two that end anything, is
  the *span* and not the word — `$'a\0b'ccc` is `accc` in bash and ksh93
  and `abccc` in ash — and every road to a zero byte takes the same
  answer: `\0`, `\x00`, `\u0000`, an octal value past a byte, and `\c@`.

  It was an `Answer` until #2276, which is the shape to remember rather
  than the fix: two columns were measured, "does the NUL truncate" looked
  like the question, and the third column could then only be recorded by
  being wrong by a byte and silent about it. **"Does X happen" and "what
  happens" are different questions**, and a two-valued axis has quietly
  committed to the first.

Two more places the panel splits, both about the hexadecimal escape's
digits, and each an axis:

- **How many digits `\x` reads** — `DollarSingleHexReadsEveryDigit`.
  bash and zsh stop at two; ksh93 takes every digit that follows, and a
  run past two is a *code point* rather than a byte. Measured 2026-09-12
  under `LC_ALL=C`, by `od`:

  | written | bash 5.3, bash 3.2, zsh | ksh93 |
  | --- | --- | --- |
  | `$'\x00b'` | 00, which truncates, then `b` | 0b |
  | `$'\x414'` | 41 then `4` | d0 94, which is U+0414 |
  | `$'\xFF'` | ff | ff |
  | `$'\x00FF'` | 00, which truncates, then `FF` | c3 bf, which is U+00FF |

  The last two rows are the pair that says the *digit count* decides and
  not the value. A run past the last code point is encoded rather than
  refused, in the extended form UTF-8 has room for: `$'\x41414141'` is
  the six bytes fd 81 90 94 85 81 there, and a run too long for the value
  to hold keeps the low bits, so `$'\x41414141414141414141'` is the same
  six. The locale does not enter into it — the same bytes come back under
  `LC_ALL=C` and under a UTF-8 locale.

  The same escape in a `printf` format is `PrintfHexEscape`, which holds
  this reading as one of its four values. The two are separate fields
  because a shell answers the two sites differently — ksh93 reads `\x41`
  in a format and leaves it alone in a `%b` argument.

- **An escape with no digit after it** —
  `DollarSingleDigitlessEscapeIsAZeroByte`. bash keeps `\xzz` as written;
  ksh93 and zsh read a zero byte and carry on with the rest. The two that
  agree look different in a terminal and are the same answer: the zero
  ends the span in ksh93, which is `DollarSingleNul` and not this, so
  what is left there is nothing at all. One answer for
  `\x`, `\u` and `\U` alike — no column splits them.

**What `\c` applies to also differs, when the argument is itself an
escape**, and it follows the same axis. bash takes the raw byte —
`$'\c\t'` is control-backslash and then a `t` — with the one exception
that a doubled backslash is read as the single character it stands for.
ksh93 decodes first, so the same text is control-tab. It is the one
place `DollarSingleBackslashC` decides more than arithmetic.

The form is specified here because it is a *quoting* rule: the decoded
text is a *quoted span*, so no later stage splits or globs it —
`$'a\tb'` is one field holding a real tab, however much whitespace the
tab is. When the decoding runs is an implementation's choice; what may
never be lost is the span's quoting, which is the same requirement every
other quote form places on this stage.

POSIX §2.3 is a set of rules applied character by character. The three
that determine the lexer's shape:

**Operators delimit words with no whitespace required.**

    echo a>b     →  writes "a" to the file b, in all six shells

`a>b` is three tokens. A lexer that splits on whitespace is wrong before
it starts.

**Longest match wins.** `>>` is one operator, not two, and the rule is
that an operator is extended while the next character can continue a
valid operator.

    echo x>b; echo y>>b   →  b contains x then y

**A digit immediately before a redirection operator is an IO number**,
not a word. Unanimous, and it is the cleanest demonstration that
tokenization is context-sensitive:

    echo 1>b    →  b is empty      the 1 is a file descriptor
    echo 1 >b   →  b contains "1"  the 1 is an argument

One space changes what the digit *is*. The rule is strict adjacency: no
space between the digits and the operator, and the token is a candidate
IO number only in that position.

### How many digits, is not unanimous

One digit is everybody's. A *second* one is bash's alone (macOS,
2026-09-05, and the same answer for every width from two digits up):

    exec 10>f; echo hi >&10; cat f
      bash 5.3, bash 3.2   f holds hi — ten is a descriptor
      dash                 exec: 10: not found
      ksh93                exec: 10: not found
      zsh                  command not found: 10

    echo x 10>f; cat f
      bash 5.3, bash 3.2   x on the terminal, f empty
      dash, ksh93, zsh     f holds "x 10"

To three of the five the digits are an ordinary word and the operator is
a redirection with no number of its own, so `exec 10>f` runs a *command*
called `10` with its output in the file. It is the milder form of the
trap `AmpersandRedirect` carries: nothing is reported, and a different
command runs.

Those three still hold descriptors above nine perfectly well. They have
no way to *write* one, and `exec {v}>f` is what puts them there — ksh93
and zsh both answer 10 or 11 and use it happily — which is why this is a
question about the token and not about the table.

POSIX is why it is a flag rather than a mistake in three shells: XCU's
IO_NUMBER is one *or more* digits, so bash conforms and so does the
majority that reads one. Where the panel and the standard disagree the
panel decides the core.

Grammar flag: `MultiDigitFdNumber` — core: off; `bash`: on. Measured as
`redir/a-two-digit-descriptor-number` and
`redir/digits-before-a-redirection-that-are-not-a-number`. What happens
when a number that large is *too* large is the interpreter's question and
is in `docs/spec/semantics.md`.

### `{name}` before a redirection asks the shell to pick the descriptor

Where an IO number could stand, bash, ksh93 and zsh also accept a
variable in braces: `exec {fd}>f` opens the file on a descriptor the
shell chooses — 10 or above — and assigns the number to `fd`, so
`>&$fd` and `{fd}>&-` use and close it later. To dash the braces are
part of an ordinary word, and `exec {fd}>f` goes looking for a command
named `{fd}` (measured: `redir/the-shell-picks-the-descriptor`; the
follow-on case, `redir/a-picked-descriptor-may-outlive-its-command`,
is where the *lifetime* answers diverge, and that half belongs to the
interpreter).

The grammar is the adjacency rule again, one token earlier: `{fd}>f`
names a descriptor and `{fd} >f` is a word followed by a redirection,
exactly as `1>f` and `1 >f` differ. That is why the construct is
consumed by the lexer — the space is the whole distinction, and only
the lexer still has it.

Grammar flag: `FdVariableRedirections` — core: on; `posix` and `dash`:
off.

### The name in those braces may be an element

`exec {a[1]}>&-` closes the descriptor that element holds, and that is
the way a coprocess's feed is closed: the shell puts the near ends in an
array, so the number to close is `${NAME[1]}` and never a scalar.

The panel splits three ways rather than two (macOS, 2026-09-05, with a
descriptor parked on 3 and the element holding 3):

    exec 3>f; a[1]=3; exec {a[1]}>&-
      bash 5.3   closes it — the write afterwards fails
      ksh93      closes it
      zsh        `no matches found: {a[1]}` — the braces are a word, and
                 a word with brackets is a pattern; with globbing turned
                 off it is `command not found: {a[1]}` instead
      bash 3.2   `exec: {a[1]}: not found` — no `{name}` token at all
      dash       the same, and no arrays either

    exec {a[1]}>f; echo "${a[1]}"
      bash 5.3 and ksh93 both answer 10 and leave the file open there

Subscript 1 rather than 0 on purpose: it names the first element in every
shell that has arrays, so zsh's column is about the token rather than
about the array base — `a[0]=3` is `assignment to invalid subscript
range` there, which would have looked like a refusal of this construct
and is not one. The refusal is real, and it is the *construct's*: with
`setopt noglob` zsh still reads the word as a command name.

So this is not a consequence of having both `{name}` and subscripts —
zsh has both and refuses — and it gets a flag of its own. The core is
what bash 5.3, ksh93 and zsh agree on, which leaves this to the two that
answer yes.

The subscript is taken as written and never expanded, because the whole
token becomes one literal: `{a[i]}` and `{a[i+1]}` are read, and
`{a[$i]}` stays an ordinary word rather than quietly meaning the two
characters. bash reads that last spelling and ksh93 takes the token and
then refuses the `$` in the arithmetic, so no answer there is
everyone's. A subscript is also never empty and never nested.

Where it is read, the subscript means what it means everywhere else: a
declared associative name takes it as a key and any other takes it as an
expression, which is `${a[i+1]}`'s rule and not a second one.

Measured: `redir/the-picked-descriptors-name-may-be-an-element` and
`redir/closing-a-descriptor-through-an-element`.

Grammar flag: `FdVariableSubscript` — core: off; `bash` and `ksh`: on.

## A subscript at command position runs to its matching `]`

An associative array's subscript is text, and text may hold a blank. The
question that raises is not what the key is but where the *word* ends,
and the panel gives three answers to it.

Measured 2026-09-12, each row read back with `typeset -p m`:

    typeset -A m; m[foo bar]=qux
      bash 5.3   `declare -A m=(["foo bar"]="qux" )`
      bash as sh the same
      ksh93      `typeset -A m=(['foo bar']=qux)`
      zsh        `bad pattern: m[foo`, status 1 — nothing is stored
      bash 3.2   no `-A`, so the subscript is read as arithmetic, and
                 `foo bar: syntax error in expression` is the tell that
                 its word ran to the matching bracket too
      dash, ash  no arrays; the word ends at the blank

So once a **name** at command position is followed by `[`, four of the
seven columns read through to the matching `]`. It is the brackets and
not the blank that do this, which the rest of the rows say:

    m[foo<tab>bar]=v   a tab the same way
    m[foo\nbar]=v      and a newline: the word spans the line
    m[a; b]=v          `;`, `|`, `>` and `&&` are all key characters
    m[#c]=v            a `#` in there is not a comment
    m[a [b] c]=v       brackets nest, so it is the *matching* `]`
    m['a]b' c]=v       a quoted `]` closes nothing, nor an escaped one

Three things are required and each was measured by taking it away.

**A name.** `1m[foo bar]=v`, `m-n[foo bar]=v` and `[foo bar]=v` all still
end at the blank in every column that takes the construct.

**Command position.** An argument does not: `printf '<%s>' m[foo bar]=v`
prints two fields in every column. An assignment *prefix* is command
position, so `a=1 m[foo bar]=v` stores the element. A redirection target
is not, in bash — which is the column this follows, ksh93 spanning there
too.

**No `=`.** The assignment is not part of the condition — `m[foo bar]`
alone is quoted back whole as `m[foo bar]: command not found` — so the
word is one word before anything has looked for an assignment.

Where the bracket has **no** matching `]` the two columns diverge: bash
refuses with ``unexpected EOF while looking for matching `]'`` and ksh93
swallows the rest of the input into the word. There is no common answer,
so the word is left as a grammar without the construct reads it, and
nothing that parsed before parses differently.

Measured: `assoc/a-key-holding-a-blank-is-written`,
`assoc/a-key-holding-a-blank-is-appended-to`,
`assoc/a-key-holding-an-operator`, `assoc/a-key-holding-a-nested-bracket`
and `subscript/a-blank-in-an-argument-subscript`.

Grammar flag: `SubscriptSpansSeparators` — core: off; `bash` and `ksh`:
on.

### And at the front of a compound literal's element

The same flag answers a second position, measured 2026-09-13 because a
file of bash's own suite ended at a status bash does not end at:

    typeset -A m; m=( [one]=1 [two words]=2 )
      bash 5.3   the keys are `one` and `two words`
      bash as sh the same
      ksh93      the same
      zsh        `bad pattern: [two`, status 1 — nothing is stored
      bash 3.2   **two fields**, `[two` and `words]=2`, although the same
                 build spans at command position
      dash, ash  no literals at all

So between a compound assignment's parentheses an element whose **first
character** is an unquoted `[` runs to its matching `]`, and everything
inside is a character of the subscript on exactly the rows above — a
tab, a newline, `;`, a nested bracket, a quoted or escaped `]`.

The corpus records the run-of-blanks spelling rather than the `;` one,
and the reason is the corpus rather than the shell: every case is read
back by one dialect-neutral grammar, and `m=( [a; b]=v )` is not a parse
of anything without the flag — the `;` closes no bracket and the element
list ends at a token it cannot use. The `;` is recorded at command
position, where a grammar without the flag still reads the line, as two
commands.

bash 3.2 is why this is a position of its own rather than a consequence
of the command-position rule: one build of one shell spans in one
position and not the other, so nothing about having the first implies
the second.

The condition is narrower here, and each half was measured by taking it
away:

**Nothing in front of the bracket.** `a=( pre[1 2]=x )` and
`a=( x[1 2] )` are two fields in all three bash columns. ksh93 reaches
further — it takes `pre[1 2]=x` as a subscripted element — and that
extra reach is recorded and not implemented, because the front of the
element is the shape every column that spans agrees on.

**An unquoted bracket.** `a=( "[1 2]"=x )` is one field holding the text
and no key is written, unanimously, zsh included.

An element whose bracket never closes falls back to the ordinary
reading, for the reason the command-position rule falls back: bash
refuses the text and ksh93 does something else again, so there is no
common answer and falling back keeps the flag additive.

Measured: `assoc/a-literal-element-keyed-with-a-blank`,
`assoc/a-literal-element-keyed-with-a-run-of-blanks`,
`assoc/a-literal-element-keyed-with-a-nested-bracket`,
`array/a-name-before-a-literal-elements-bracket`,
`array/a-literal-value-holding-a-bracket` and
`array/a-quoted-bracket-at-a-literal-elements-front`.

## Comments

`#` begins a comment only where a word could begin. Mid-word it is an
ordinary character.

    echo a#b    →  a#b
    echo a #b   →  a

## Reserved words are positional

`if`, `then`, `done` and the rest are keywords only in the position where
a command name is expected. Elsewhere they are ordinary words:

    echo if then done      →  if then done
    f() { echo "$1"; }; f if   →  if

The lexer therefore cannot classify a word as a reserved word on its own.
It reports a word, and the grammar decides — which is the coupling
between this document and the one that specifies the command language.

## Line continuation

A backslash immediately before a newline removes both, before tokens are
formed. The joined text is one word:

    printf '[%s]' ab\
    cd          →  [abcd]

Because it happens before tokenization, a continuation can split an
operator or a word anywhere. It does **not** apply inside single quotes,
where backslash has no special meaning at all.

That includes the text of an arithmetic expression, which is not tokenized
as words but is still read with this rule in force. Measured 2026-09-16 from
script files, unanimous in bash 5.3, bash 3.2, zsh 5.9.2, ksh93u+ and dash
for every construct each of them has:

    $(( 1\
    2 + 1 ))                 →  13     inside a number
    $(( 1 <\
    < 3 ))                   →  8      between an operator's two characters
    $(( ab\
    c + 1 ))                 →  abc + 1
    (( 2\
    +2 )), for ((i=0; i<1\
    ; i++)), $[ 1\
    +2 ], ${((\
    2+3))}                   →  the same, on each route into an expression

It is removed inside a double-quoted part of the expression too, as in any
double-quoted string, and kept inside a single-quoted part and inside a
command substitution's program, which reads it by its own rules. A body of
an unquoted here-document reads its `$(( ))` through the same scanner.

A parameter expansion's text is read with the rule in force too: `${x\⏎y}`
is `${xy}` on the whole panel. `parameter-expansion.md` has the shapes and
the one place ksh93 refuses the pair.

The **backquoted** command substitution is the exception to "not inside
single quotes": the pair is removed from its text before that text is
parsed, in every quoting written inside it, so `` `printf %s 'a\⏎b'` ``
produces `ab` on the whole panel where the `$( )` spelling of it keeps
both characters. `substitutions.md` has the shapes.

### A continuation between the `$` and what it introduces

The pair can also stand at the construct's **front**, between the `$` and the
character that says which construct this is. The pair is removed there in
every shell; what splits is whether the `$` then **reaches** what stands
behind it, or stays as text.

Measured 2026-09-16 from script files, `env -i PATH=/usr/bin:/bin LC_ALL=C`,
stdin on /dev/null, fresh directory, with `x=5` and `set -- a b`. BusyBox ash
was not measured.

| probe | bash 5.3, 3.2 | dash | zsh 5.9.2 | ksh93u+ |
| --- | --- | --- | --- | --- |
| `$\⏎x` | `5` | `5` | `5` | `$x` |
| `$\⏎{x}` | `5` | `5` | `5` | `5` |
| `$\⏎(echo hi)` | `hi` | `hi` | `hi` | `` `(' unexpected `` |
| `$\⏎[1+2]` | `3` | — | `3` | — |
| `$\⏎'a	b'` | `a`⇥`b` | — | `a`⇥`b` | `a`⇥`b` |
| `"$\⏎x"` | `5` | `5` | `5` | `$x` |
| `"$\⏎{x}"` | `5` | `5` | `{x}` | `${x}` |
| `"$\⏎(echo hi)"` | `hi` | `hi` | `$(echo hi)` | `$(echo hi)` |
| `"$\⏎[1+2]"` | `3` | — | `$[1+2]` | — |
| `"$\⏎1"` | `a` | `a` | `a` | `$1` |

A `—` is a shell with no such form at all, so nothing there is a measurement
of this question. The ksh93 refusal row is the `$` staying as text and a `(`
then being unable to begin a word.

**A `$` reaches a *form*, not simply the next character.** `$\⏎ x` is `$ x`
and `$\⏎` at the end of a word is `$`, in every column — so the pair is not
removed and forgotten; the question is asked about what it uncovered.

So the sets are `Dialect.ContinuationStopsADollarAt` and
`…InDoubleQuotes`: zsh stops at everything but a bare parameter inside double
quotes and at nothing outside them; ksh93 stops at a bare parameter and a
parenthesis outside quotes and at every form inside them. Neither set contains
the other. `Dialect.DollarGoesWhenAContinuationStopsItAtABrace` is the second
question the brace row asks: the same stop yields `{x}` in one shell and
`${x}` in the other, so what becomes of the `$` is not derivable from whether
it was stopped.

An unquoted here-document body stops nothing, in every dialect: a body holding
`[$\⏎x][$\⏎{x}]` is `[5][5]` in bash 5.3, zsh 5.9.2, ksh93u+ and dash. That is
the same exemption ksh93's `${ }` refusal has, and for the same reason — the
body's continuations are gone before an expansion in it is scanned.

Outside quotes ksh93 **takes its own stop away when a pattern character
stands earlier in the same word**, which is
`Dialect.PatternCharacterUndoesTheContinuationStop` and was recorded here as
unmodeled until #3523. The set is five characters and was measured one at a
time, 2026-09-18, with the word written as a `printf` operand:

| prefix | ksh93u+ | prefix | ksh93u+ |
| --- | --- | --- | --- |
| `a` | `<a$x>` | `[` | `<[5>` |
| `!` | `<!$x>` | `{` | `<{5>` |
| `}` | `<}$x>` | `*` | `<*5>` |
| `]` | `<]$x>` | `?` | `<?5>` |
| `'*'` | `<*$x>` | `~` | `<~5>` |
| `"*"` | `<*$x>` | `a[b]` | `<a[b]5>` |
| `\*` | `<*$x>` | | |

The three quoted spellings are what make it a question about *unquoted*
pattern characters, and the closing `}` and `]` are what make the set five
characters rather than "the punctuation" — `~` is in it and is no glob
metacharacter at all. Whether the rule is about globbing or about that shell
taking a second pass over a word it has marked as a pattern is not decidable
from outside, and the field records what was seen. The other four columns have
no stop for it to take away, so there is no panel split here: this is one
shell against our reading of it. `echo [$\⏎x]` printing `[5]` is this, and it
is the shape a reader is most likely to write.

Two further shapes are at a construct's delimiters rather than in front of
them, and they are the next two sections: the pair standing *between* the two
characters of `$((` or of `))`, and the pair standing directly behind the `((`
of an arithmetic command.

### A continuation inside the delimiters themselves

The pair can stand between the two characters of `$((` or of `))`, and there
it decides **whether the construct is arithmetic at all** rather than what the
expression says. The panel splits, and not the same way at the two ends.

Measured 2026-09-16 from script files, `env -i PATH=/usr/bin:/bin LC_ALL=C`,
stdin on /dev/null, fresh directory. BusyBox ash has no arithmetic expansion
of this shape and was not measured.

| probe | bash 5.3, 3.2 | dash | zsh 5.9.2 | ksh93u+ |
| --- | --- | --- | --- | --- |
| `echo "[$(\⏎( 1 + 2 ))]"` | `[3]` | `[3]` | `[3]` | runs `1` |
| `echo "[$(( 1 + 2 )\⏎)]"` | `[3]` | `[3]` | runs `1` | runs `1` |

"runs `1`" is the command-substitution reading: the body is the subshell
`( 1 + 2 )`, whose first word is a command nobody has, so the diagnostic names
`1` and the expansion is empty.

zsh joins the opener and parts the closer, so a single yes/no could not be
given a value for it: `Dialect.ContinuationPartsTheArithmeticOpener` and
`…Closer` are two fields.

**A blank on either side takes the question away** and is the control:
`$( \⏎( 1 + 2 ))` and `$(( 1 + 2 ) \⏎)` are a command substitution in every
column, because the parentheses are no longer adjacent whatever becomes of the
pair.

The arithmetic **command**'s closer is not this question: `(( 1 + 2 )\⏎)` is
two groupings running `1` in bash 5.3, zsh 5.9.2, ksh93u+ and dash alike
(#3454).

### A continuation directly behind an arithmetic command's `((`

ksh93 alone opens nothing there. The two parentheses and the pair are
consumed, no command is produced, and reading starts again from what follows —
so the visible answer is usually a syntax error, and the error is at the
**leftovers** rather than at the continuation.

Measured 2026-09-16 on ksh93u+ 2012 from script files:

    ((    x = 5 )); echo "[$x]"      `)' unexpected at line 2, status 3
    if ((    1 )); then …               `)' unexpected
    for ((    i=0; i<1; i++)); do …      `i' unexpected
    ((    )); echo st=$?             `)' unexpected
    echo pre⏎((\⏎echo hi⏎echo b   pre, hi and b, status 0
    false⏎((\⏎echo "rc=$?"        rc=1 — not even the status moved

The last two rows are the whole of the reading, and every refusal above them
is the text that followed being read on its own. bash 5.3, bash 3.2 and zsh
5.9.2 read all six as an ordinary arithmetic command; dash has no `((` and
reads two groupings.

A blank in front of the backslash takes the rule away — `(( \⏎x = 5 ))` is 5
there — and so does anything at all: `((x\⏎ = 5 ))` is 5 too. It is the pair
standing *directly* behind the second parenthesis
(`Dialect.ContinuationEndsTheArithmeticCommandOpener`, #3455).

Which token the refusal then names is that parser's own: `for ((\⏎i=0; …` is
`` `i' unexpected `` there and `` `))' unexpected `` here, both at line 2 and
both at status 3, because the two parsers give up at different points in the
same leftover text.

## Aliases are expanded while the line is read

Alias expansion belongs to this document rather than to the interpreter,
and that placement is forced: **an alias may hold a keyword.**

    alias iff='if true; then'
    iff echo yes
    fi                          →  yes, in all six shells

Nothing substituting at execution time can produce that, because by then
the grammar has already been decided. (Measured from a script file: an
alias defined and used on the same *line* is not expanded, in every
shell, because the line was read before the definition ran.)

It is a **token-level** substitution rather than a textual splice, and
that is measured rather than assumed. Every diagnostic about a command
that came from a *single-line* alias names the line the alias word was
written on — bash, dash and ksh93 all report line 3 for `alias
bad=nosuchcmd` written on line 2 and used on line 3 — and `$LINENO`
inside such a body reads the same. So a spliced token carries the
position of the word it replaced, every position still points into the
real input, and there is no position map to keep.

The model has one seam, and what crosses it is below: a construct the body
**left open** is continued over the input, because that is the one thing a
body lexed between its own edges cannot be asked about.

The one place the two models are distinguishable from outside is a body
containing a **newline**, and `Dialect.AliasBodyCountsLines` is the axis.

| shell | `$LINENO` on the line after a two-line body | a failure on the body's second line |
| --- | --- | --- |
| bash | 5 (its physical line) | reported at the alias word |
| dash | 6 | one line below the alias word |
| ksh93 | 6 | one line below the alias word |
| zsh | 6 | one line below the alias word |

The three splice the body's *text*, so its newlines are lines of the
program; bash splices tokens and leaves the whole body on the alias
word's line. Three further measurements pin the shape of it, all on
2026-09-05:

- The shift is **one per newline**: a three-line body moves what follows
  by two.
- It belongs to the **expansion**, not to the definition. An alias
  defined and never used moves nothing, in all six columns; using it
  twice moves what follows twice.
- A command *inside* a multi-line body is reported on the body's own
  line, which is the half a one-line body cannot show.

Where the axis is on, this parser still substitutes tokens and gives
them lines, rather than splicing text. That is what keeps every offset
pointing into the real input — a diagnostic still quotes text that is
really there — while the numbering matches. The lines an expansion adds
are in no text at all, so a caller reading a program in pieces asks
`Parser.LineShift` for them; counting newlines cannot find them, and
without it the numbering resets at the first refill.

### Two questions, and they were one field until #2109

**The option: does this shell expand aliases with nobody having asked.**
bash is the holdout and needs `shopt -s expand_aliases`; every other shell
in the panel expands out of the box and turns it off with an option of its
own — zsh's `unsetopt aliases`. `Dialect.AliasesExpandUnlessTold`.

**The route: which routes' own program text expands, once the option has
said there is anything to expand.** `Dialect.ExpandAliasesInProgramText`.

They were one `ProgramRoutes` field, and the two readings it held were not
the same thing: bash's empty set meant "off until asked" and zsh's partial
set meant "not on this route". A front end that derived one answer from
that field got both jobs wrong at once — see the section below.

| shell | `-c` | script file | standard input |
| --- | --- | --- | --- |
| bash *(with the option on)* | yes | yes | yes |
| bash as `sh` | yes | yes | yes |
| dash | yes | yes | yes |
| ksh93 | yes | yes | yes |
| zsh | **no** | **yes** | **yes** |

Measured 2026-09-12 with the option turned on where the shell has it off
by default: `bash -c $'shopt -s expand_aliases\nalias t=echo\nt TOP'`
writes `TOP`, and so do the same three lines in a file and on standard
input. The same binary invoked as `sh` expands by every route with no
`shopt` anywhere — POSIX mode turns alias expansion on for a
non-interactive shell, which is why the panel runs bash twice. All of them
expand interactively, which the front end decides rather than the grammar.

The option is a run-time switch on the runner that starts at what
`AliasesExpandUnlessTold` says, and the two things that move it are `shopt
-s`/`-u expand_aliases` and POSIX mode; leaving the mode restores the base
rather than what was set before entering it. See
`interp.Runner.ExpandingAlias`, which is the hook the front end hands the
parser, and the `shopt/expand-aliases-*` corpus cases.

**zsh does not fit a boolean**, and this was measured rather than
inferred: the answer depends on how the program arrived, not on whether
anyone is at the keyboard. A boolean gets one of zsh's three right, and
the two it gets wrong are the ones a real script uses.

zsh is the only witness to the split, and one witness is enough: a
boolean cannot record an answer that zsh gives two ways.

Measuring this needs care, because the obvious probe answers a different
question. Writing the `-c` case as `alias a=…; a` puts the definition and
the use on one line, and aliases are applied while the line is parsed —
so the whole string is parsed before the definition takes effect, and
every shell reports `a` as not found. That measures same-line-versus-next
line, which is what `alias/not-on-the-line-that-defines-it` covers. The
route only shows itself when the `-c` string carries a real newline.

So `Dialect.ExpandAliasesInProgramText` is a **set of routes**,
`ProgramRoutes`, and the front end asks it with the route it read: a
command string, a file, or standard input. That is the same three-way
split `$0` already turns on, and for the same reason — how a program
arrived is the front end's fact and nothing else knows it. The parser
never reads *this* set; whoever knows the route hands the table of aliases
in, or leaves it nil.

**And the one cell in it is not really about aliases.** zsh reads a `-c`
string *whole* before running any of it — `Diagnostics.CommandStringParsedWhole`,
measured 2026-09-11 with `zsh -fc $'printf A\nif; then'`, which prints no
`A` at all — so the `alias` on line 1 has not run when line 2 is parsed and
nothing on the string can expand. This front end parses a command string a
line at a time, so the route set is what stands in for the reading strategy.

zsh's is the **only** cell, and that is a measurement rather than a
simplification. The `ash` column held the same value until #2338, and the
probe behind it was a one-liner — which, as the paragraph above explains, is
a probe that cannot tell the two answers apart, since dash answers it
identically while expanding on every route. Asked with two lines in the
pinned alpine image, BusyBox v1.37.0 prints `hi` for
`ash -c $'alias foo=echo\nfoo hi'`, so ash sits with dash and the reading
strategy the route set stands in for belongs to exactly one shell in the
panel.

It is no longer the only question of that shape. `CloseQuotesAtEOF` is a
set of the same routes, and the parser does read that one: ksh93 ends an
unterminated quote at the end of a command string and refuses the same
text in a file, so the route rides on `Dialect.ProgramRoute` and the
lexer asks whether it is in the set. The empty route is in no set, which
is what makes the strict answer what a parse gets when nobody said how
its text arrived.

The algorithm, four rules, each measured and unanimous across the shells
that expand aliases in scripts:

- **Only an unquoted word in command position.** `"a"` is a command name
  and not an alias — the rule that lets a script reach the real thing past
  an alias shadowing it. The lookup is by the token's source text, quotes
  and all.
- **Keep expanding while the replacement names another**, with the names
  already used in *this command* remembered. That is the whole of the
  recursion guard: `alias echo='echo x'` gives `x hi` rather than looping,
  and a cycle `a` → `b x` → `a y x` leaves the inner `a` as an ordinary
  word, which is then not found. The set is fresh per command, so
  `e yes` and `e two` on two lines both expand.
- **A value ending in a space makes the word *after* the expansion
  eligible too.** This is the rule behind `alias sudo='sudo '`. It applies
  once the expansion's own tokens are spent — the space makes the
  following word eligible, not the value's own second word.
- **A value that is empty or all blanks leaves nothing behind**, and the
  command becomes whatever followed it.

### A construct the body opens reaches the rest of the line

The token model above is right about positions and right about keywords,
and there is one question it cannot answer from inside the body: what the
body **left open**. Substitution puts the body in the input the shell is
reading, and lexing carries on over the join, so a quote the body opens is
still open when the rest of the line is read — and a quote nothing closes
runs to the end of the input and is an unterminated quote.

    alias q='echo "'
    echo one
    q hello"          →  one, ` hello`, two — 0 in all seven columns
    echo two

    alias a='echo "x'
    echo one
    a                 →  one, then a refusal in all seven
    echo two

Measured 2026-09-13 from a script file, because zsh expands no alias under
`-c`, across dash, bash 5.3, that binary as `sh`, bash 3.2, ksh93, zsh and
BusyBox ash. The first is `0` everywhere, with bash 3.2 writing the blank
twice; the second is a refusal everywhere, at 2 in five columns, 1 in zsh
and 3 in ksh93, and the line each names is the alias word's in four of them
— bash 5.3, that binary as `sh`, bash 3.2 and ksh93 — and the end of the
input in the other three.

**Not a rule about quotes.** It is the lexer crossing the seam, so it holds
for everything the lexer can be inside: both spellings of a command
substitution carry the same way, and `alias q='echo $('` used as
`q echo hi)` prints `hi` in all seven. The second direction is the reason
it matters beyond the construct — a shell that runs the second script above
is closing a quote the script never closed and running a command the author
did not write.

Two consequences of the first rule follow from it rather than being rules
of their own:

- The word a construct **swallows** is never offered to the table, even
  where the value ends in a blank. `alias q='echo "x '` used as `q b"` is
  `x  b` in all seven columns and never the expansion of `b`: the word
  after the alias word is inside the quote and was never a word at all.
  Whether the word *past* the construct is offered is the one question
  inside this seam the panel splits on, and
  `Dialect.AliasTrailingBlankReachesPastAnOpenConstruct` is the axis —
  `alias c='CEE'` beside the same `q`, used as `q b" c`, is `x  b CEE` in
  bash 5.3, that binary as `sh` and bash 3.2, and `x  b c` in dash, ksh93,
  zsh and BusyBox ash.
- A snippet like the first one **does not parse on its own**, and is right
  not to: until the `alias` line has run, the closing `"` belongs to
  nothing. The reference shells' own `-n` refuses the same text for the
  same reason, and the corpus marks such a case `ExpansionCompletes`.

**The seam is crossed at every level, and the input is the last of them.**
Where the alias word was itself a token of another body's expansion, what
follows it is that body's remaining text, then the remaining text of every
body that one was spliced into, and only then the input. The pending queue
carries that text beside its tokens, and what the joined reading takes of it
is spent by dropping the tokens it stands for. Measured 2026-09-15 from a
script file, with `alias b='echo "'` throughout and `echo one` / `echo two`
around the line — unanimous in all seven columns:

| written | result |
|---|---|
| `alias a='b x'` then `a y"` | `` x y`` — the inner body opens the quote and the input closes it |
| `alias a='b x"'` then `a` | `` x`` — the inner body opens it and the *enclosing* body closes it |
| `alias a='b x'` then `a` alone | an unterminated quote, in every column |
| `alias c='echo "'`, `alias b='c y'`, `alias a='b z'` then `a w"` | `` y z w`` — three bodies deep |

The third row is the one that says this is not a convenience: with the carry
declined one level in, a quote nothing closes became a closed one and the
line after it ran, which is the direction that runs a command the author did
not write (#2709).

The carry is performed when the token is **handed out** rather than when the
body is spliced, and the second row is why. A body's own tokenization is not
final while an alias word stands earlier in it: `b x"` lexes as a word and a
quote nothing closes, and the quote that closes it is the one `b` is about to
contribute. Carrying at splice time reads that open quote over the input and
swallows the rest of the file.

### A here-document the body opens reads its body from the body

The same textual reading reaches a here-document. A body is read from the
lines after the operator's line, and where the operator is in an alias value
holding newlines, those lines are the value's own — then the text of any
value it was spliced into, then the input after the alias word. Measured
2026-09-16 from script files, with `shopt -s expand_aliases` where bash needs
it:

| value(s) | used as | bash 5.3.20, ksh93u+, dash, BusyBox ash | zsh 5.9.2 |
| --- | --- | --- | --- |
| `hd='cat <<EOF⏎in alias⏎EOF⏎'` | `hd` | `in alias` | `in alias` |
| `hd='cat <<EOF⏎in alias⏎EOF'` | `hd` | `in alias` | body runs on: `in alias`, `EOF␠`, … |
| `hd='cat <<EOF⏎in alias⏎EOF'` | `hd; echo same` | body runs on: `in alias`, `EOF; echo same`, … | the same, `EOF ; echo same` |
| `hd='cat <<EOF⏎in alias⏎'` | `hd` ⏎ `from file` ⏎ `EOF` | `in alias`, an empty line, `from file` | `in alias`, `␠`, `from file` |
| `hd='cat <<EOF⏎in alias'` | `hd x` ⏎ `from file` ⏎ `EOF` | `in alias x`, `from file` | the same |
| `Y='cat <<\END'`, `X='Y⏎text⏎END⏎echo inX'` | `X` | `text`, `inX` | the same |
| `Y='cat <<-END⏎<tab>in Y'`, `X='Y⏎<tab>in X⏎<tab>END'` | `X` | `in Y`, `in X` | `in Y␠`, `in X` |

So the seam is crossed in both directions — a body goes on from a value into
the value around it and into the input, and a line the value ends in the
middle of is finished by what follows the alias word. The last row but two is
the one that shows it is text and not tokens: the delimiter written as the
value's last line is only the delimiter when nothing follows the alias word
on its line.

zsh is the one column that reads the seam as a **blank**, unless one is
already there: `EOF` at a value's end is `EOF␠` and ends nothing, and `in Y`
finished by a newline is `in Y␠`. It is the same blank that keeps a backslash
ending a value from joining the next line there, so
`Dialect.AliasBodyBackslashJoinsTheNextLine` answers both.

Not modeled: a body that the *input* runs out inside after crossing the seam,
which is a remark about where the input ended and is left to the input's own
route — so `hd; echo same` above, with no later `EOF`, still reads the value's
body lines as commands here. And the seam blank is inserted only at the end
of the value being expanded, not at the end of each value it was spliced
into, so zsh's `EOF␠` for a delimiter the *outer* value ends in is not
reproduced.

### The table reaches every text this shell reads, and the *option* is its only gate

Alias expansion is not a property of the *outermost* parse. Every place this
shell reads shell source is a place an alias is expanded, measured 2026-09-11
across dash, ksh93 and zsh — the three that expand aliases in a script at all:

| route | the caller's alias works inside | a definition inside reaches its own later lines |
| --- | --- | --- |
| `.` / `source` a file | all three | dash, zsh — **not** ksh93 |
| `eval` | all three | dash — **not** zsh, ksh93 |
| `$( )` and `` ` ` `` | all three | none |
| a trap body | all three | — |

The second column is not a second axis. It is `EvalRunsWhatItParsed` and
`SourcedFileRunsWhatItParsed`, which record whether that text is read through
before it runs or read as it runs, and those answers line up with this table
exactly. A text read through first has already been parsed by the time its
first line runs, so a definition on line 1 cannot reach line 2 — which is the
same sentence as "an alias defined and used on the same line does not expand",
one level up.

That correction matters because the reader question used to be asked only of
text that *failed* to parse, on the ground that parsing has no effect of its
own. It has one whenever a line changes how a later line parses, and an alias
is the plain case; a `setopt` or `shopt` that moves the grammar is the same
shape. So the question is now asked of any borrowed text with a later line in
it, and of nothing else — a text with no newline after its last command reads
the same either way.

`$( )` is the route with no question attached: every column parses a
substitution through before running it.

**The route does not reach any of them**, which is #2109 and was measured
2026-09-12 by asking the whole table under a zsh `-c` string — the one
place where the route and the option disagree:

```console
$ zsh -fc $'alias t=echo\nt TOP'          # the program's own text
zsh:2: command not found: t
$ zsh -fc 'alias t=echo; eval "t E"'       # and everything nested in it
E
$ zsh -fc $'alias t=echo\nv=$(t S)\necho "v=$v"'
v=S
$ zsh -fc 'alias t=echo; . ./f.sh'         # f.sh holds `t hi-from-file`
hi-from-file
$ zsh -fc $'alias t=echo\ntrap "t TRAP" USR1\nkill -USR1 $$\nsleep 0.2'
TRAP
```

`unsetopt aliases` turns all four off, which is what says the option is the
gate. The same shape holds in bash: nothing nested expands until `shopt -s
expand_aliases`, and everything does once it is on.

Deriving the nested texts' answer from the route is what #2109 fixed. The
front end handed the runner "the option modulated by the route", so a zsh
`-c` string — which really does expand nothing of its own — turned the alias
table off for every `eval`, `$( )`, `.` and trap body inside it. The runner's
switch is now the option alone, and the route is the front end's own and
decides one thing: whether the *program's* parser is handed a table at all.

`alias/nested-text-expands-where-the-command-string-did-not` and
`alias/a-trap-body-under-a-command-string-expands-too` are the corpus rows.
One column of the first still misses, and for an unrelated reason: dash
parses a command substitution with the line that holds it rather than when
it runs, so its `$( )` there is read before the `alias` beside it — #2357.

### Three kinds of alias, and two namespaces

zsh has two further kinds, and no other shell in the panel has either:
`alias -g` and `alias -s` are `invalid option` in bash 5.3 and 3.2,
`unknown option` in ksh93, and in dash — which reads no options for
`alias` at all — a name it cannot find. Measured 2026-09-11.
`Semantics.GlobalAliases` and `Semantics.SuffixAliases` are the axes.

**A global alias is expanded wherever a word stands**, not only where a
command word does, and it shares the table with the regular kind:

    alias -g UP='| tr a-z A-Z'
    echo hi UP                   →  HI

Measured, one at a time, on zsh 5.9.2:

| probe | expands |
| --- | --- |
| `echo 1 G`, `echo G 2` | yes — any argument |
| `G` alone | yes — command position too |
| `for x in G`, `case G in`, `a=(G)` | yes |
| `echo x > G` | yes — a redirection target |
| `cat << G` | yes — a heredoc delimiter |
| `[[ G == … ]]` | yes |
| `"G"`, `'G'`, `\G` | **no** — any quoting |
| `"x G y"` | **no** — inside a quoted word |
| `v=G` | **no** — one word, and its text is not the name |
| `$(( G + 1 ))` | **no** — arithmetic is not word text |

The value is spliced as **tokens**, exactly as a regular alias body is,
so a command separator (`;`), a redirection operator (`>`) and a reserved
word (`then`) all work from one — and a two-word value is two words.

Two recursion rules, and they are not the same rule:

- The set of spent names is **fresh per word**, where a regular alias
  keeps one per *command*: `alias -g S=x` used twice in one command
  expands twice, and a cycle `A`→`B`→`A` stops with `A` standing.
- What a word spends is spent for the **command word** as well, which is
  the half that keeps the two from fighting: `alias -g f='echo f'` run as
  a command prints `f` rather than recurring.

One table, so `alias -g dup=…` **replaces** a regular `dup`, a plain
`alias` lists the regular and the global together, `alias -g` lists the
global alone, and `unalias` has no `-g` — it is `bad option` there,
because the plain form already removes either.

**A suffix alias is keyed on a command word's extension**, and is the
second namespace:

    alias -s txt=cat
    ./x.txt                      →  the file, through `cat`

The rule is a substitution of *text*: a command word `text.name` where
`text` is non-empty and `name` names a suffix alias becomes `value
text.name`. So the value may hold a pipeline, and the filename lands
after its last command; `*.ps` is replaced before globbing and so still
globs; and a trailing space in the value is not special, because the
spliced text ends in the word.

Measured, again one at a time:

| probe | result |
| --- | --- |
| `x.txt`, `./x.txt`, `/abs/x.txt`, `a/.txt` | substituted |
| `.zsh` | **not** — `text` is empty |
| `q.sh/w` | **not** — the run after the last dot is `sh/w` |
| `./nope.txt`, `d.sh` (a directory) | substituted — the file need not exist or be a file |
| `p.sh` with `p.sh` executable on PATH | substituted — it beats the executable |
| `p.sh` with a function `p.sh` | substituted — it beats the function |
| `p.sh` with `alias p.sh=…` | the **regular alias** wins |
| `'./x.txt'`, `$f` holding the path | **not** — quoted, and not yet expanded |
| `v=1 ./x.txt` | substituted — an assignment prefix does not move the command word |

That order is not a policy; it falls out of doing this while the line is
read. Neither the function nor the PATH entry exists yet, and the regular
alias is looked for first.

The namespaces are what `unalias` shows: `unalias txt` is "no such hash
table element" with a suffix alias `txt` defined, `unalias -s txt`
removes it, `unalias -a` empties the other table and leaves this one, and
`unalias -s -a` does the reverse. `alias -g -s` asks for both at once and
is `illegal combination of options`, status 1.

zsh presents all three as parameters, which is the same statement in
another surface: `$aliases` holds the regular kind alone, `$galiases` the
global and `$saliases` the suffix.

## An unterminated quote at end of input

`echo "abc` with no closing quote is a syntax error in bash, dash and
zsh — each with its own wording — and in **ksh93 it prints `abc`**, as if
the closing mark had been there. The same for an unterminated backquote:
`echo \`echo hi` prints `hi` in ksh93 and nowhere else.

`$(` and `${` are **not** quotes and still refuse, in ksh93 as well:
`echo $(echo hi` is `` `(' unmatched `` there.

Grammar flag: `CloseQuotesAtEOF` — ksh only. It is a flag rather than a
leniency applied everywhere because the difference is visible in what a
script *does*, not only in whether it is diagnosed: a truncated file ends
up running a command under one shell and not another.

## Heredoc delimiters

Whether the delimiter is quoted decides whether the body is expanded, and
that decision belongs here, because it is a property of how the delimiter
token was written:

| probe | result |
| --- | --- |
| `cat <<EOF` | body expanded — `[VAL]` |
| `cat <<"EOF"` | body literal — `[$x]` |
| `cat <<\EOF` | body literal — `[$x]` |

Unanimous across the panel. Any quoting anywhere in the delimiter makes
the whole body literal; it is not a per-character property.

The delimiter's quoting must survive onto the heredoc token, because the
body is read later. Earlier work of ours got this wrong twice by
rewriting the parse tree instead — see
`../../lessons-carried-forward.md`.

### Where the body is

**Not after the operator — after the next newline.** The rest of the line
is ordinary input and is read first:

    cat <<EOF; echo after     →  one, then after
    one
    EOF

So the body is collected when the newline is reached, not when the
operator is seen. That is the whole reason this needs cooperation between
the lexer and the parser: the parser has the delimiter, and the lexer is
what reaches the newline.

The body ends at a line **exactly equal** to the delimiter. `EOFX` does
not end an `EOF` heredoc, and neither does a line with trailing spaces.
`EOF x` — the delimiter, a space, and a word — is body in all six, and so
is `EOF junk` inside a command substitution
(`heredoc/the-delimiter-is-the-whole-line`). Prior work of our own had
this recorded as a rule about a line that *begins* with the delimiter,
and the prefix reading is what these two disprove.

**One shape is the exception, and it is the `)` of a command
substitution.** `EOF)` on its own line inside `$( … )` ends the body and
closes the substitution in bash 5.3, bash 3.2 and ksh93; dash and zsh
refuse the construct outright, dash saying it wanted the `)` and zsh
naming the assignment. The case is
`heredoc/a-delimiter-that-closes-a-command-substitution`.

So the exception is not "a prefix ends the body" but "the substitution's
closer may follow the delimiter" — four of the six columns take it and
two refuse — and it is the only place the whole-line rule bends.

**A line reading the delimiter alone further down does not move it.**
Measured 2026-09-16, script files under `env -i`:

    x=$(cat <<EOF
    body
    EOF)
    echo "[$x]"
    EOF
    )
    echo "{$x}"

bash 5.3.20 and ksh93u+ print `[body]`, then run `EOF` as a command and
refuse the `)` under it — the body ended at `EOF)` although a later line
could have ended it, and bash warns about end of file there exactly as it
does with nothing further down. zsh 5.9.2 and dash read the body from the
whole input, so it ends at the later `EOF` and the `)` under that closes the
substitution: `{body`, `EOF)`, `echo "[]"}`. The same holds for `<<-` over
`<tab>EOF)`, a quoted delimiter, `EOF))` closing two substitutions, `EOF)x`
(the `x` joins the word the substitution is in), `EOF);cmd`, and `<(`.
The reading that only looked for the `)` once nothing further down could
end the body was wrong in bash and ksh93 whenever the file went on to hold
the delimiter again, which a file of several such documents always does.

bash reads further than ksh93 here, and it is not modeled: `EOF )`, with a
blank before the parenthesis, and `EOF X )` both end the body in bash and
are body in ksh93, zsh and dash — bash appears to end it at a line that
opens with the delimiter and holds a `)` anywhere, since `EOF "a)"` does
too. `EOF X` with no parenthesis is body in all four.

bash 5.3 also *warns* there that the document was delimited by end of
file, which is the same remark `<<-` with a space-indented delimiter
earns below. **Where that warning comes from is the interesting part**
(#785). The delimiter is present and matches nothing, so from the
here-document's point of view the input ran out — while the program
itself is complete, `v` is `a` and the status is 0. Both halves are true
because the here-document's input is *the substitution's own text,
closing parenthesis included*: `EOF)` is a body line, and the last line
of the substitution is the last line the body could have. Read the
trimmed text instead and the opposite is true — `EOF` alone is the
delimiter, there is nothing to remark on, and that is exactly why the
substitution runs and yields `a`.

Which is also what makes it the *parser's* remark rather than the
interpreter's: it is said for a substitution on the right of a `&&` that
never reaches it (`heredoc/a-substitutions-delimiter-is-remarked-on-before-it-runs`).
bash 3.2 says nothing, so the wording is a `Diagnostics` value and the
silence is an answer rather than a gap.

What decides it is that the parentheses hold **a program**, not which
sigil opened them. `<(cat <<EOF` … `EOF)` earns the same remark in bash
5.3 and the same silence in bash 3.2 and ksh93
(`heredoc/a-substitution-that-is-not-a-dollar-sign`); dash and zsh have
no process substitution and refuse the line. The counter-case draws the
line and is the reason there is one: `$(( a << b ))` is a **left shift**,
and reading it as a program would make `<<` a here-document whose
delimiter `b` never arrives — a warning about a script that has none.

Not reached: a command substitution *inside* an arithmetic one —
`$(( $(cat <<EOF` … `EOF) + 1 ))` — where bash warns and we do not. The
arithmetic text is not read as commands while the source is scanned, so
the substitution inside it is invisible to the parser and the remark has
nowhere to come from. Recorded rather than fixed; it is a question about
when nested expansions are parsed, not about here-documents.

**Several heredocs on one line are collected in operator order**:

    cat <<A <<B
    first
    A
    second
    B

`A`'s body is the lines up to `A`, then `B`'s. What the *command* then
does with two redirections of the same descriptor is the interpreter's
problem — and the panel diverges there, dash, bash and ksh93 printing
`second` while zsh prints both — but the collection order is unanimous.

### `<<-` strips tabs, and only tabs

    cat <<-EOF        cat <<-EOF
    <tab>tabbed           spaced          ← four spaces
    <tab>EOF              EOF
    →  tabbed         →  runs to end of input

Leading **tabs** are stripped from the body lines and from the delimiter
line. Spaces are not, so an indented-with-spaces delimiter never matches
and the heredoc swallows the rest of the input. bash warns about that
(`here-document delimited by end-of-file`); dash, ksh93 and zsh take it
silently. Reaching the end of input without the delimiter is therefore
**unfinished input**, not a syntax error.

**A delimiter written with a leading tab is where the panel parts three
ways.** Only a quoted delimiter can begin with one — `<<- '<tab>EOF'` — and
the stripped body lines can then never be spelled like it. Measured
2026-09-16, script files under `env -i`, the body printed by `cat`:

| delimiter written | `<tab>EOF` | `<tab>EOF` | `<tab>EOF` | `<tab><tab>EOF` |
| --- | --- | --- | --- | --- |
| end line | `<tab>EOF` | `EOF` | `<tab><tab>EOF` | `<tab><tab>EOF` |
| bash 5.3.20 | ends | runs out, warns | runs out, warns | ends |
| zsh 5.9.2 | ends | ends | ends | ends |
| ksh93u+ 2012 | ends | ends | ends | ends |
| dash 0.5.12 | runs out | runs out | runs out | runs out |

bash compares the line as written as well as the stripped line; zsh and
ksh93 strip the delimiter's leading tabs as they strip the lines'; dash
compares the stripped line against the delimiter as written, which nothing
can equal. Without the dash nothing is stripped and `<tab>EOF` ends a
`<tab>EOF` document in all four, and a tab *inside* the delimiter
(`'E<tab>OF'`) is unanimous. BusyBox ash, measured the same day through the
pinned Alpine image, runs out on all four rows as dash does.
`syntax.Dialect.StrippedHeredocDelimiter` carries it.

## `<<<` is a redirection, not a heredoc

It shares a prefix with `<<` and nothing else. There is no delimiter, no
body read from later lines, and no cooperation between the lexer and the
parser: **the operator takes exactly one word**, on the same line, and
that word becomes the input.

    cat <<< hi                    →  `hi\n` — a newline is appended
    printf '[%s]' <<< one two     →  [two] — `two` is an argument to the
                                     command, not part of the input
    x='a b'; cat <<< "$x"         →  a b
    x='a b'; cat <<< '$x'         →  $x

The word is expanded as a word — quoting decides, exactly as it does
anywhere else, and a single-quoted word is literal — and it is **not
field-split** (`word-splitting.md`, where the one dated exception is
recorded). A trailing newline is always added, so `cat <<< hi` yields
three bytes and not two.

Grammar flag: `Herestring` — core: on; `posix` and `dash`: off. dash is
the only panel member without it, and refuses at the operator
(`Syntax error: redirection unexpected`) rather than at the word.

## `<&`'s operand is a file number in one column

`<&` duplicates a descriptor, and four of the panel read its operand as an
ordinary word and open whatever it expands to. zsh reads it while **parsing**
and refuses anything that is not a file number. Measured 2026-09-18 on zsh
5.9.2 under `-f`, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`:

| written | zsh 5.9.2 | bash · ksh93 · dash · ash |
| --- | --- | --- |
| `cat <&5`, `<&55`, `<&-`, `<&p`, `3<&4` | runs | runs |
| `cat <&"5"`, `<&$'5'` | runs | runs |
| `cat <&5$v`, `<&${v}5`, `<&"5""$v"` | runs | runs |
| `cat <&$(echo 5)5`, `<&5$(echo x)` | runs | runs |
| `cat <&$v`, `<&"$v"`, `<&${v}`, `<&$(echo 5)` | `file number expected` | runs |
| `cat <&x`, `<&5x`, `<&{fd}`, ``<&`echo 5` `` | `file number expected` | runs |
| `cat <&$v5`, `<&$v$w`, `<&x$v`, `<&"x"$v` | `file number expected` | runs |
| `cat <&${v}x`, `<&""$v`, `exec {fd}<&$v` | `file number expected` | runs |
| `cat >&$v`, `>&x`, `>&p`, `2>&$v` | runs | runs |

It is the **literal text the parser already holds** rather than what the word
would come to: the spans an expansion fills are passed over, and what is left
has to be a non-empty run of digits — or the whole operand is the `-` that
closes the descriptor or the `p` that names the coprocess's.

Two pairs say that twice over. `5$v` against `$v5`: a digit the parser can see
is enough and the expansion beside it is not looked into, and `$v5` is the
*parameter* `v5`, one span with no literal text at all. `5x` against `5$v`:
literal text that is not a digit refuses whatever stands beside it.

The last row is the control and is why this is not "a duplicating
redirection's operand": `>&` also spells *send both streams to this file*, so
a word there is a path.

The refusal gives up the **line** and not the file — the line before it runs,
the line after it runs, and `$?` is 1 — which is the shape
`syntax.File.Refused` already carries for a syntax error inside a compound
assignment's parentheses. `zsh -n` reports it with the script never run, which
is how it was found: one of zsh's own shipped functions, `tcp_point`,
redirects from a descriptor held in a parameter and real `zsh -n` refuses the
file where this parser accepted it (#3144).

One row is measured and not modeled: on a line holding more than one command,
`cat <&$v; echo hi` runs the `echo` there and gives up only the command.
Giving up the line is as far as `File.Refused` reaches.

Grammar flag: `InputDuplicateOperandIsAFileNumber` — `zsh`: on; everything
else: off.

## `&>` is the dangerous one

Not every dialect difference is a construct that fails to parse. `&>` is
accepted by all six shells and **means different things**:

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `echo hi &>b` then `cat b` | `hi` then empty | `[hi]` | `hi` then empty | `[hi]` |

In bash and zsh, `&>` is one operator redirecting both streams. In dash
there is no such operator, so the same text tokenizes as `echo hi &` — a
**background command** — followed by `>b`, a redirection with no command
that truncates the file.

**ksh93 is on both sides of this, depending on build.** AJM 93u+ from
2012, which macOS ships, has no `&>` and backgrounds the command.
ksh93u+m 1.0.8 from 2024, which Debian ships, treats it as the operator
and agrees with bash. The harness found this by running the corpus on a
Linux runner after the spec had already been written from a laptop; it is
the second claim a cross-platform run has corrected, and the reason
`../oracle.md` insists a build is part of every claim.

So "ksh supports `&>`" is not a fact about ksh, and an implementation
that keys this axis on a shell name rather than on a configured value
will be wrong for half its users.

Nothing errors. The command runs, the output goes somewhere else, and the
file is emptied. This is the failure mode a compatibility layer exists to
prevent, and it argues for the lexer knowing its dialect rather than
accepting the union and letting the interpreter sort it out: the union
would silently pick one meaning for text that legitimately has two.

Grammar flag: `AmpersandRedirect` — core: on; `posix`, `dash` and `ksh`:
off. The `ksh` preset is off because it is measured against the 93u+
build described above; a deployment standardized on 93u+m would turn it
back on, which is the point of its being a configured value rather than
a shell name.

By contrast `>|`, which overrides `noclobber`, is accepted with the same
meaning by all six and is core.

### `>;` — a write that only lands if the command succeeded

ksh93 alone has a third write operator. `cmd >; file` sends the output to
a temporary file in `file`'s own directory and renames it over `file`
when the command ends at status 0; at any other status the target is left
exactly as it was, and a target that did not exist is not created.
Measured 2026-09-14 on ksh93u+ 2012-08-01:

| written | ksh93u+ | bash 5.3.15 · zsh 5.9.2 · dash |
| --- | --- | --- |
| `echo NEW >; f` over an `f` holding `old` | `f` holds `NEW`, status 0 | syntax error at the `;` |
| `{ printf X; false; } >; f` | `f` still holds `old`, status 1 | the same refusal |
| `{ printf X; false; } >; new` | `new` is not created | the same |
| `set -C; echo NEW >; f` | `f` holds `NEW` | the same |
| `chmod 741 f; echo NEW >; f` | mode still `-rwxr----x` | the same |

The mode is carried over deliberately: a rename brings the temporary
file's own permissions with it, so a replaced file would otherwise come
back with whatever the umask gave the temporary.

The `;` is part of the operator and must be tight against the `>` —
`echo x > ; f` is a syntax error in ksh93 as well — and there is no `>>;`
and no `<;`. So this is one operator rather than a marker that
generalizes, which is the distinction `ClobberOverrideMarker` above
records getting wrong once.

**This is where `<->` comes from.** `echo <->; echo done` in ksh93
reports that it cannot open `-` and then does *not* reach `done`, where
`echo <->x; echo done` reports the same thing and does. Both follow from
the operator: the `<` takes `-` as its operand — which is what the
diagnostic naming `-` rather than `->` says — and the `>;` left over
takes the word behind the `;` as its target, so `echo done` is this
command's argument. Put any character between the `>` and the `;` and
there is no `>;` at all. The other five shells refuse the text outright.

Grammar flag: `RenameOnSuccessRedirect` — core: off; `ksh`: on. Off
everywhere else, where the fallback is the reading those shells have: `>`
then `;`, and a redirection with no target.

### `<#` and `>#` — the file-position redirections

The other pair ksh93 has alone, and the only redirections in the panel
that open nothing: they move where a descriptor next reads or writes.
`exec 3<#((0))` rewinds descriptor 3; `exec 4>#((3))` puts the write
position three bytes in. The operand is an *arithmetic command* — the
same `((expr))` the grammar already reads at command position — so the
offset may be computed and may name parameters.

Measured 2026-09-16 on ksh93u+ 2012-08-01, script files under
`env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin on /dev/null:

| written | ksh93u+ | bash 5.3.20 · zsh 5.9 · dash 0.5.12 |
| --- | --- | --- |
| `exec 3< f; exec 3<#((0))` | descriptor 3 is at 0 | syntax error |
| `n=2; exec 3<#((n * 3 + 1))` | at 7 | the same |
| `exec 4<> g; exec 4>#((3))` | the next write lands at 3 | the same |
| `exec 3<#((CUR))` | the position it already had | the same |
| `exec 3<#((EOF-3))` | three bytes before the end | the same |

`CUR` and `EOF` stand for the position and the size **inside the
expression only**: `CUR=99; exec 3<#((CUR))` seeks to where the
descriptor stands and leaves `CUR` holding 99 afterwards. An assignment
the expression makes does reach the shell — `exec 3<#((zz=6))` sets `zz`
— so the two names are shadowed rather than evaluated somewhere else.

The position is the *open file's* and not the command's, so a seek
written on an ordinary command is not taken back when it ends:
`{ read -n2 v <&3; } 3<#((6))` leaves the next read of 3 at 8.

Three refusals, each its own sentence: `exec 6<#((0))` over a number
nothing is open at is `6: bad file unit number [Bad file descriptor]` —
not the `7: cannot open [Bad file descriptor]` a duplication writes —
`exec 3<#((-1))` is `-1: invalid seek offset`, and a stream with no
position is `0: not seekable`. A seek *past* the end is not an error at
all: the read after it returns nothing at status 1. Standard input on
/dev/null is the one measured row this shell does not reproduce — ksh93
answers `0: invalid seek offset` there and the seek succeeds here.

ksh93 also reads a **pattern** after `<#` and seeks to the line matching
it; that half is not claimed, and the operand is refused rather than read
as something it is not. A plain word is no guide either: `exec 3<#0`
segfaults ksh93u+ 2012-08-01 outright.

Grammar flag: `SeekRedirect` — core: off; `ksh`: on. Off everywhere else,
where the `#` opens a comment and the fallback is a redirection with no
target — the syntax error the other three report.

### `|&` — a pipe of both streams, or a coprocess

The other operator whose two bytes mean two different things. Measured
2026-09-06, from a script file with a scratch `HOME` and `ZDOTDIR` under
`env -i`, `-n` for the parse and a run for the behavior:

| shell | `-n` | `{ echo out; echo err >&2; } \|& tr a-z A-Z` |
| --- | --- | --- |
| bash 5.3.15 | accepts | `OUT` and `ERR` |
| bash as `sh` | accepts | the same — `argv[0]` takes nothing away here |
| bash 3.2.57 | refuses | ``syntax error near unexpected token `&' ``, status 2 |
| dash | refuses | `Syntax error: "&" unexpected`, status 2 |
| ksh93u+ | accepts | **a coprocess**: `err` alone, and `tr` gets nothing |
| zsh 5.9.2 | accepts | `OUT` and `ERR` |

Three of the six columns disagree with the other three, and not in one
direction. The two that refuse have no `|&` at all: they lex a bar and
then an ampersand and blame the ampersand, which is the same sentence
they give for `echo one | & echo two` — so a dialect without the operator
needs no wording of its own, only the token the grammar could not take.

**The two accepting readings are two constructs, not two meanings.** The
measurement that separates them is what may follow the operator:

| probe | bash 5.3 | ksh93 | zsh |
| --- | --- | --- | --- |
| `echo one \| ; echo two` | error at `;` | error at `;` | accepts |
| `echo one \|& ; echo two` | error at `;` | **accepts** | accepts |
| `echo one & ; echo two` | error at `;` | **accepts** | accepts |

A `|` there needs a command after it and ksh93's `|&` does not, and a
bare `&` does not either: ksh93's `|&` *terminates* a command the way `&`
does, and what it starts is a coprocess whose ends are reached with
`read -p` and `print -p`. It is a different slot in the grammar and wants
its own flag beside `Coproc`, never a second value of this one. (zsh
accepts all three because it is lenient about `;` after any of them,
which is a separate question and not evidence either way.)

The two bytes must be adjacent. `echo one | & echo two` is refused by
bash 5.3, bash 3.2, bash-as-`sh`, dash and zsh alike — five of six, in
four wordings — so reading a spaced pair as the operator would take a
background command away from every shell that spells one that way.

Where the operator *is* the pipe of both streams, it is exactly `2>&1 |`
with the redirection written **last**. Two shapes measure the order, and
both accepting shells agree on both (zsh with `nomultios`, because
`MULTIOS` tees rather than replaces and hides the question):

| probe | equal to | not equal to |
| --- | --- | --- |
| `e 2>/dev/null \|& cat` → `O`, `E` | `e 2>/dev/null 2>&1 \| cat` | `e 2>&1 2>/dev/null \| cat` → `O` |
| `e >/dev/null \|& cat` → nothing | `e >/dev/null 2>&1 \| cat` | `e 2>&1 >/dev/null \| cat` → `E` |

`$?` and `PIPESTATUS` are the same for both spellings as well, so the
operator adds no status rule of its own.

Grammar flag: `PipeBothStreams` — core: **off**; `bash` and `zsh`: on.
Off in the core because three of the six columns do not have this reading
and two of those three have no `|&` whatever: an intersection cannot
contain it. It is a version fact as much as a dialect one — `|&` arrived
in bash 4, so the `bash` preset states what bash 5.3 does and the bash
3.2 column of every measurement above is the other half of the same row.

## Two operators with no blank between them

    (:);(:)         accepted and run by all seven columns
    echo a&;b       accepted by one, refused by the rest
    if |; then      refused by all seven

Adjacency has meaning in three places already — `<(`, an IO number before
a redirection, `|&` — and this is the one place where it means nothing to
the grammar and something to a reader. One shell says so on the way past:

    $ ksh -n s.sh                       # (:);(:)
    s.sh: warning: line 1: use space or tab to separate operators ; and (

Measured 2026-09-12 on ksh93u+ 2012-08-01, from a script file with a
scratch `HOME` under `env -i`. dash, bash 5.3, bash-as-`sh`, bash 3.2,
zsh 5.9.2 and BusyBox ash read the same bytes without a word, so the
wording is one column's: `Diagnostics.OperatorsNotSeparated`, empty in
the rest.

**It is advice about layout and nothing else.** Writing the two apart
removes the line and changes nothing else about the parse — `a |; b` and
`a | ; b` are refused identically, with the same complaint and the same
status, and only the first draws the warning. That is why it is a
`syntax.Remark` and not a fact hung off an error: half the shapes it
fires on parse and run at status 0, and there is no error for them to
hang off.

### The same route restriction the backquote remark has

| route | ksh93 on `(:);(:)` |
| --- | --- |
| `ksh -n s.sh` | the warning, status 0 |
| `ksh s.sh` | nothing, status 0 |
| `ksh -c '(:);(:)'` | nothing |
| `ksh < s.sh` | nothing |

So `-n` in that shell is a **lint mode with rules** rather than a parse
check that happens to warn, and this is its second rule; the backquote
remark in `substitutions.md` is the first. Both are held back by
`interp.RemarkOnlyWhenNotRunning`, which is a property of the remark
kind — the here-document remark is said either way, which is what makes
this a question and not a rule about remarks.

That restriction is what collapsed the design. #2409 was filed as a line
that precedes a refusal and re-measured into a channel that had to
survive a *successful* parse, because two of its rows exit 0 with no
error to carry it. Both rounds were measured entirely under `-n`. The
status-0 rows do survive a successful parse — within the one route that
ever prints anything, which lives in `driver` and had the machinery
already.

### The trigger, probed a byte at a time

The operator just lexed is **exactly one character** and is one of `;`,
`|` or `&`, and the byte immediately after it is one of `;`, `|`, `&`,
`(`, `<` or `>`.

    :;|:      ; and |     :|;:      | and ;     :&;:      & and ;
    :;(:)     ; and (     :|(:)     | and (     :&(:)     & and (
    :;<f      ; and <     :;>f      ; and >     :;((1))   ; and (
    :;>>f     ; and >     :;<<E     ; and <     :;||:     ; and |

    :&&(:)    silent      :||(:)    silent      :;;:      silent
    :;&:      silent      :|&:      silent      :&|:      silent
    :;$(:)    silent      :;x       silent      :;!:      silent
    :;)       silent      :;{ :; }  silent      :;[[ x ]] silent
    a | ; b   silent      #:;(:)    silent      ":;(:)"   silent

Three things the silent rows pin down, and each rules out a simpler rule:

- **Length is the whole of the first half.** `&&(` is silent where `|(`
  remarks, and `;;`, `;&`, `|&`, `&&`, `||` and `&|` never start one,
  because the two bytes are one operator and one operator has no blank
  to be missing. So the condition is asked of the **operator table** and
  not of a list of excluded pairs — which is what keeps it right per
  dialect: `;&` is one token only where the grammar has it, and turning
  `CaseFallthrough` off makes the same two bytes remark, correctly.
- **It is the byte and not the next token.** `:;$(:)` is silent where
  `:;(:)` remarks, so the trigger is a literal `(` opening a token
  rather than "a parenthesis comes next".
- **The second operator is named by its first byte.** `:;>>f` writes
  `; and >`, not `; and >>`.

One line per occurrence, in source order, before the syntax error where
there is one. The status is never the remark's: 0 where the parse
succeeds and the refusal's where it does not.

Redirection operators can begin one too — `:<;` writes `< and ;` and
`:><` writes `> and <` — and that half is deliberately not modeled. It
is erratic in the shell it comes from: `:>;` is silent where `:<;` warns,
and `:<>X` names `<` for every `X` and names a *newline* as the second
operator when the line ends. Every such text is refused anyway, so what
would be reproduced is one shell's inconsistency in a line nobody reads.

Corpus: `core/two-operators-run-together-under-a-syntax-check`,
`core/two-operators-run-together-when-it-runs`,
`core/two-operators-run-together-before-a-refusal`,
`core/two-operators-run-together-are-refused-without-the-remark`.
