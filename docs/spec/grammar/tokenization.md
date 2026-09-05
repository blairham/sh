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

Unanimous across bash 3.2, bash 5.3, ksh93 and zsh, and decoded here:

| escape | value |
| --- | --- |
| `\n` `\t` `\r` `\a` `\b` `\f` `\v` | the C control characters |
| `\e`, `\E` | ESC (0x1b), both spellings |
| `\\` `\'` `\"` `\?` | the character itself |
| `\xHH` | the byte, one or two hex digits — `$'\x4'` is 0x04 |
| `\NNN` | the byte, one to three octal digits — `$'\101'` is `A` |

The octal form is one rule and not two: `\0101` is three digits counted
from the zero onward, so it is a backspace and then a `1` rather than
0x41. A value past a byte wraps into one — `$'\400'` is a zero byte,
which is the truncation rule below and not a separate answer.

Two escapes are decoded here on the current shells' agreement, with
bash 3.2 keeping the text as written — dated, not vetoed, per
`../core.md`:

| escape | value | bash 3.2 |
| --- | --- | --- |
| `\uHHHH` | the code point, up to four hex digits, as UTF-8 | literal |
| `\UHHHHHHHH` | the same, up to eight digits | literal (as `sh` and as `bash` alike) |

**What a shell does with a code point outside ASCII depends on the
locale.** Under `LC_ALL=C`, bash 5.3 prints `$'é'` as the eight
characters it was written as and zsh refuses it — `character not in
range` — where in a UTF-8 locale both encode it. The corpus case stays
inside ASCII for that reason; ours encodes as UTF-8 whatever the locale,
which is a divergence recorded rather than fixed here.

Three places the panel splits, and each is an axis on the semantics
vector rather than a decision taken here:

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
- **A NUL ends the text** — `DollarSingleNulTruncates`. bash and ksh93
  hold a word as a C string, so `$'a\0b'` is `a` and `${#x}` is 1; zsh
  counts its strings and keeps all three bytes. What ends is the *span*
  and not the word — `$'a\0b'ccc` is `accc` — and every road to a zero
  byte takes it: `\0`, `\x00`, `\u0000`, an octal value past a byte, and
  `\c@`.

One decision is still recorded rather than made an axis, because nothing
reaches it through another rule:

- **`\x` with no digit after it stays literal.** bash keeps `\xzz` as
  written; zsh reads a zero byte and keeps the `zz`; ksh93 emits a byte
  of its own. Ours is bash's answer, and the same rule covers a
  digitless `\u` and `\U`.

**What `\c` applies to also differs, when the argument is itself an
escape**, and it follows the same axis. bash takes the raw byte —
`$'\c\t'` is control-backslash and then a `t` — with the one exception
that a doubled backslash is read as the single character it stands for.
ksh93 decodes first, so the same text is control-tab. It is the one
place `DollarSingleBackslashC` decides more than arithmetic.

One measured divergence is deliberately not implemented, recorded so the
gap is a decision rather than an oversight:

- **How many hex digits `\x` reads.** bash and zsh stop at two; ksh93
  does not, so `$'\x00b'` is one byte 0x0b there and a truncating NUL in
  bash. Ours stops at two everywhere.

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

### Which shells expand in a script

| shell | `-c` | script file | standard input |
| --- | --- | --- | --- |
| bash | no | no | no |
| bash as `sh` | yes | yes | yes |
| dash | yes | yes | yes |
| ksh93 | yes | yes | yes |
| zsh | **no** | **yes** | **yes** |

bash needs `shopt -s expand_aliases` and then expands by every route; the
`no` row is bash without it. The same binary invoked as `sh` expands by
every route with no `shopt` anywhere — POSIX mode turns alias expansion
on for a non-interactive shell, which is why the panel runs bash twice.
All of them expand interactively, which the front end decides rather than
the grammar.

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

So `Dialect.ExpandAliases` is a **set of routes**, `AliasRoutes`, and the
front end asks it with the route it read: a command string, a file, or
standard input. That is the same three-way split `$0` already turns on,
and for the same reason — how a program arrived is the front end's fact
and nothing else knows it. The parser never reads the set; whoever knows
the route hands the table of aliases in, or leaves it nil.

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
two refuse — and it is the only place the whole-line rule bends. bash
also *warns* there that the document was delimited by end of file, which
is the same remark `<<-` with a space-indented delimiter earns below.

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
