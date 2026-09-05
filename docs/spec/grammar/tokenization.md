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

Vector field: `DollarDoubleQuote` (off in the core; the bash and ksh
dialects set it).

### `$'...'` decodes escapes, and this is the set

`$'...'` is single quotes with one difference: backslash escapes decode.
`shell-matrix.md` records the form as core — bash, ksh93 and zsh all
have it, and dash keeps every character literally, `$` included. The
corpus pins the load-bearing failure mode
(`core/dollar-single-expands-escapes`: the quoting was once recorded and
nothing decoded it); the full set below was measured escape by escape on
2026-09-04, same panel and machine as `shell-matrix.md`.

Unanimous across bash 3.2, bash 5.3, ksh93 and zsh, and decoded here:

| escape | value |
| --- | --- |
| `\n` `\t` `\r` `\a` `\b` `\f` `\v` | the C control characters |
| `\e`, `\E` | ESC (0x1b), both spellings |
| `\\` `\'` `\"` `\?` | the character itself |
| `\xHH` | the byte, one or two hex digits — `$'\x4'` is 0x04 |
| `\NNN` | the byte, one to three octal digits — `$'\101'` is `A` |

Two escapes are decoded here on the current shells' agreement, with
bash 3.2 keeping the text as written — dated, not vetoed, per
`../core.md`:

| escape | value | bash 3.2 |
| --- | --- | --- |
| `\uHHHH` | the code point, up to four hex digits, as UTF-8 | literal |
| `\UHHHHHHHH` | the same, up to eight digits | literal (as `sh` and as `bash` alike) |

Two decisions are recorded because the panel splits, and each follows
bash:

- **An escape with no meaning keeps both characters.** `$'\q'` is `\q`
  in bash (3.2 and 5.3); ksh93 and zsh drop the backslash and keep the
  `q`. Ours keeps both — dropping the backslash destroys information,
  and the dominant scripting target keeps it.
- **`\x` with no digit after it stays literal.** bash keeps `\xzz` as
  written; zsh reads a zero byte and keeps the `zz`; ksh93 emits a byte
  of its own. Ours is bash's answer, and the same rule covers a
  digitless `\u` and `\U`.

One escape is measured and deliberately not decoded: **`\cX`**, the
control character, which bash and ksh93 decode (`$'\cA'` is 0x01) and
zsh treats as unknown. Ours treats it as unknown too — both characters
kept — recorded here so the gap is a decision rather than an oversight;
a script that needs a control character has `\x01` in every shell that
has the form at all.

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

Vector field: `FdVariableRedirections` (on in the core, off for `posix`
and `dash`).

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

Vector field: `AmpersandRedirect` (default true; false for `posix`).

By contrast `>|`, which overrides `noclobber`, is accepted with the same
meaning by all six and is core.
