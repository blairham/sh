# The command language

How tokens become commands. This consumes what `tokenization.md`
produces and produces what `expansion.md` consumes.

Citation: POSIX.1-2024 XCU §2.9 (shell commands) and §2.10 (grammar).
Panel measurements as in `../oracle.md`.

## The hierarchy

    list        →  and-or  [ ; | & | newline ]  …
    and-or      →  pipeline  [ && | || ]  pipeline  …
    pipeline    →  [ ! ]  command  [ | command ]  …
    command     →  simple | compound | function definition

Each level binds tighter than the one above it. Two consequences are
measured below and are the ones implementations get wrong.

## `&&` and `||` have equal precedence and associate left

This is **not** C's rule, where `&&` binds tighter than `||`. In the
shell they are the same precedence and evaluate left to right:

| probe | all six | why it discriminates |
| --- | --- | --- |
| `true \|\| echo A && echo B` | `B` | C's rule would parse `true \|\| (echo A && echo B)`, short-circuit, and print **nothing** |
| `true && echo A \|\| echo B` | `A` | left to right: `A` runs, succeeds, `\|\|` skips |
| `false && echo A \|\| echo B` | `B` | the `&&` fails, so `\|\|` runs |

The first row is the whole test. An implementation that gives `&&` higher
precedence produces no output and is silently wrong on a construct people
write constantly.

## `!` negates the pipeline, not the command

    ! true | false ; echo $?   →  0   in all six

The pipeline's status is `false`'s, which is 1; `!` inverts it to 0. If
`!` bound to `true` alone the answer would be 1. It is a property of the
pipeline, and it appears at most once, at the front.

A pipeline's status is its **last** command's:

    false | true ; echo $?  →  0
    true | false ; echo $?  →  1

## `time` prefixes a pipeline

`time` is a reserved word, not a command. It times a **whole pipeline**
and writes its report to the **shell's** standard error, which is why

    time true 2>&1 | wc -l   →  0   in bash, ksh93 and zsh

counts nothing: the redirection belongs to an element inside the
pipeline, and the report lands outside it. dash has no such word at all —
`time` there is an ordinary name that finds `/usr/bin/time` or nothing,
and the same snippet counts 1 because the external's report went through
the pipe. (Measured 2026-09-04, bash 5.3.15 / ksh 93u+ 2012 / zsh 5.9.2 /
dash on macOS.)

Where it stands is measured, not assumed:

- **Only at the start of a pipeline** — before the elements, and on
  either side of `!`: `time ! true` and `! time true` both parse and both
  report, with status 1 from the negation. bash and ksh93 print the
  report for both; zsh prints nothing for either, because nothing forked
  (see semantics.md — its report is per element and only for elements
  that fork).
- **Not in the middle of one**: `echo hi | time wc -c` is not a parse
  error anywhere, but bash and dash resolve `time` from PATH there and
  run the external. (ksh93 and zsh read the word as the keyword even
  there, which is a divergence this grammar does not add: the common
  ground is keyword-at-the-front, ordinary-word elsewhere.)
- **After an assignment prefix it is a word**: `FOO=1 time true` runs the
  external, exactly as any reserved word stops being one once the command
  has begun.

`time -p` switches the report to the POSIX format — `real 0.00`,
`user 0.00`, `sys 0.00`, one space, two decimals, no leading blank line —
identically in bash and ksh93. zsh does not read `-p` at all: it becomes
the first word of the timed pipeline, and `time -p true` there is
`command not found: -p` with the pipeline still timed. So the flag is a
separate grammar question from the keyword and is not core.

A bare `time`, with no pipeline, parses and reports in all three shells
that have the keyword (what it reports diverges — see semantics.md), and
resets the status to 0: `false; time; echo $?` prints 0 in all three.
A bare `time` directly followed by `|` is a syntax error in bash.

The status of a timed pipeline is the pipeline's own: `time false`
reports 1 in all three.

## Simple commands

A simple command is any interleaving of three things: variable
assignments, redirections, and words. Only the *first* word is the
command name.

**Redirections are not positional.** They may precede the command name or
sit between its arguments, and they are removed wherever they appear:

    >b echo hi          →  b contains "hi"
    echo one >b two     →  b contains "one two"

Both unanimous. A parser that treats a redirection as a suffix is wrong;
it has to strip them out of the word list wherever they occur.

### Assignment prefixes are transient — except where they are not

    x=1; x=2 true; echo $x   →  1    in all six

The assignment applies to that command's environment only. But POSIX
requires assignments preceding a **special builtin** to persist, and the
panel splits:

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `x=1; x=2 export y=3; echo $x` | **2** | 1 | **2** | 1 |

dash and ksh93 follow POSIX; bash and zsh do not outside POSIX mode. The
set of special builtins is fixed and small — `break`, `:`, `continue`,
`.`, `eval`, `exec`, `exit`, `export`, `readonly`, `return`, `set`,
`shift`, `times`, `trap`, `unset` — and it also governs whether a failure
is fatal, so it is one concept with two consequences.

Vector field: `AssignmentPrefixPersistsOnSpecialBuiltin` (default false).

## Grouping: `( )` and `{ }`

`( … )` runs in a subshell; `{ …; }` runs in the current one. The
difference is observable and unanimous:

    x=1; (x=2); echo $x   →  1    state does not escape
    x=1; { x=2; }; echo $x →  2    it does

`{ }` is made of **reserved words**, not operators, so it needs the
surrounding blanks and a terminator before the closing brace. `( )` is
made of operators and needs neither:

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `{ echo a; }` | `a` | `a` | `a` | `a` |
| `{ echo a }` | error | error | error | **`a`** |
| `(echo a)` | `a` | `a` | `a` | `a` |

zsh accepts a brace group without the terminator — but the rule is not
about brace groups, and modeling it that way misses half of it. In zsh
`}` is reserved wherever a *word* may stand, which is why `echo }` is a
parse error there and prints a brace in the other three. Grammar flag:
`CloseBraceAlwaysReserved` — note the inverted sense: the flag is
**off** by default and the terminator requirement is what its absence
produces; the zsh dialect turns it on and gets both halves. The full
account, including how the wrong model was caught, is in
`../semantics.md`.

## Compound commands take redirections

A redirection after a compound command applies to everything inside it:

    { echo a; echo b; } >f            →  f contains a, b
    for i in 1 2; do echo $i; done >f →  f contains 1, 2

Unanimous. The redirection belongs to the compound command as a whole,
which means the AST node for every compound form needs a redirection
list, not just simple commands.

## Loops

`while`, `until` and `for` exit **0 when the body never runs**:

    while false; do :; done; echo $?  →  0
    until true;  do :; done; echo $?  →  0
    for i in;    do echo x; done; echo $?  →  0

Unanimous, and worth pinning because "status of the last command" is the
obvious wrong answer when there was no last command.

## `case`

The core terminator is `;;`. Two extensions exist and they are **not the
same size**:

| terminator | meaning | dash | bash 5 | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `;;` | stop | yes | yes | yes | yes | yes |
| `;&` | fall through to the next body | **no** | yes | **no** | yes | yes |
| `;;&` | keep testing later patterns | **no** | yes | **no** | **no** | **no** |

`;&` is core; `;;&` is bash-only and belongs to the bash dialect. Lumping
them together as "case extensions" would put a bash-only construct in the
core language.

`;&` is also a **bash 4** feature: bash 3.2 rejects it, so it is
unavailable through macOS's `/bin/sh`. That column only appeared when the
panel gained `bash32`; a four-shell run had reported `;&` as universally
supported outside dash. It stays core despite the refusal: the boundary
counts the current shells, and bash 3.2's column dates a construct
rather than vetoing it — the rule is stated in `../core.md`.

## `select`

The menu loop, with a for-loop's header over a different loop:

    select name [ [ 'in' word* ] sep ] 'do' list sep 'done'

The words are a menu rather than a sequence, the body runs once per
*reply* rather than once per word, and the loop ends when the input does
rather than when the list does. Every non-dash shell in the panel has
it, bash 3.2 included, which is what makes it core; to dash the header
is a syntax error at `do` (`shell-matrix.md`, and the corpus's
`select/` cases — twelve rows, cited individually below).

What the panel agrees on, each row measured:

- **The menu and the prompt go to standard error**, so a script's own
  output can be redirected without taking the menu with it
  (`select/menu-goes-to-standard-error`).
- **The variable gets the chosen item; `REPLY` gets the line as typed.**
  A reply that names no item — out of range, or not a number — leaves
  the variable empty, keeps the typed text in `REPLY`, and still runs
  the body, which is how a script detects it (`select/reply-out-of-range`,
  `select/reply-is-not-a-number`).
- **A blank reply reprints the menu and does not run the body** — the
  only way to see the menu again (`select/blank-reply-reprints-the-menu`).
- **`PS3` is the prompt and is read before each prompt**, not once at
  loop entry (`select/ps3-is-read-each-time`).
- **With `in` omitted the menu is the positional parameters**
  (`select/no-list-uses-the-positionals`), so the AST keeps the same
  "no list is not an empty list" distinction `for` requires.
- **An empty menu does not prompt**: the loop body never runs and the
  status is 0 (`select/empty-list`). bash 3.2 alone refuses to *parse*
  `select x in;` — dated, not vetoed, per `../core.md`.
- **`break` is how the loop ends on purpose**, status 0
  (`select/break-leaves-the-loop`); input ending is the other way out
  (`select/input-ends`).

Presentation is where the shells split — the menu's layout, the prompt's
spelling, the status after end-of-input, and whether an unterminated
final reply is taken (zsh) or ignored (bash, ksh93). Those are
`SelectLayout` and its neighbors on the semantics vector, measured in
`../semantics.md`; the grammar is the part above, and it is one flag:
`Select` (on in the core, off for `posix`).

Like every compound command, `select … done` takes redirections.

## Function definitions

| form | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `f() { …; }` | yes | yes | yes | yes |
| `function f { …; }` | **no** | yes | yes | yes |
| `function f() { …; }` | **no** | yes | **no** | yes |

The POSIX form is universal. The `function` keyword is core but absent
from dash. The hybrid — keyword *and* parentheses — is rejected by ksh93,
which is where the keyword originated, so it is not core.

A function body may be **any compound command**, not only a brace group,
and it carries its own redirections. Whether it *must* be one is where
the panel splits:

    f() echo hi; f     bash: syntax error near unexpected token `echo'
                       dash, ksh93, zsh: hi

bash alone requires the compound command; the other three take a simple
command as a one-command body and run it (measured:
`cmd/function-body-simple-command`). The core is the wider form, since
three of the four accept it, and the strictness is a grammar flag:
`FuncBodyMustBeCompound` (off in the core, on for `bash`). The flag is
about the POSIX form only; the keyword form's shapes are the table
above.

**A function name may carry `-` and `.`** — `f-g()`, `a.b()` — and the
panel splits by *stage* rather than by yes and no: bash and zsh define
and run the function, dash refuses the name while parsing
(`Bad function name`), and ksh93 **parses it and stops at the
definition** — `invalid function name` for the dash, and its own
sentence, `invalid discipline function`, for the dot (measured:
`cmd/function-name-with-a-dash`, `cmd/function-name-with-a-dot`).
Parsing the name is therefore common ground for every shell but dash,
and that is all the grammar claims. Grammar flag:
`FunctionNamePunctuation` (on in the core, off for `posix` and `dash`);
what a shell that parsed the name then does with it is the
interpreter's question, not this one.

## Compound command productions

The shapes, in the notation of POSIX XCU §2.10. `list` is a sequence of
and-or lists separated by `;`, `&` or newline; `sep` is any one of those.

    subshell    :  '(' list ')'
    group       :  '{' list sep '}'
    if          :  'if' list sep 'then' list sep
                   { 'elif' list sep 'then' list sep }
                   [ 'else' list sep ]
                   'fi'
    while       :  'while' list sep 'do' list sep 'done'
    until       :  'until' list sep 'do' list sep 'done'
    for         :  'for' name [ [ 'in' word* ] sep ] 'do' list sep 'done'
    select      :  'select' name [ [ 'in' word* ] sep ] 'do' list sep 'done'
    case        :  'case' word 'in' { case-item } 'esac'
    case-item   :  [ '(' ] pattern { '|' pattern } ')' [ list ] terminator
    terminator  :  ';;' | ';&' | ';;&'

Four things there are measured rather than transcribed, because each is
somewhere an implementation guesses wrong.

**A terminator is required before `then` and `do`.** All four shells
reject `if true then echo x; fi` and `while false do echo x; done`. The
keyword does not delimit the condition; the `;` or newline does. That is
why the productions above have `sep` and not merely whitespace.

**The condition is a list, and its *last* command decides.** Not a single
command, and not "any command failed":

    if false; true; then echo yes; else echo no; fi   →  yes
    if true; false; then echo yes; else echo no; fi   →  no

**`for` may omit its word list, and omitting it is not the same as an
empty one.** With the list absent the loop iterates over the positional
parameters; with `in` present and nothing after it, over nothing:

    set -- x y; for i; do ...; done       →  x, y
    set -- x y; for i in; do ...; done    →  no iterations

So the AST needs to distinguish "no list" from "empty list", which a
`[]string` field cannot do.

**A `case` pattern may carry a leading `(`.** `case x in (x) …` is
accepted everywhere, and patterns alternate with `|`. A body may be empty,
and a `case` matching nothing exits 0.

## `[[ … ]]` and `(( … ))`

Both are core — every panel shell but dash has them — and both change what
the lexer is doing, which is why they are specified here rather than left
to the parser.

**Inside `[[ … ]]`, `<` and `>` are comparison operators, not
redirections.** This is the load-bearing fact:

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `[[ a < b ]] && echo less` | `cannot open b` | `less` | `less` | `less` |

In dash, which has no `[[`, the same text is a command named `[[` with a
**redirection** — so it tries to open the file `b`. Nothing errors about
the construct; the program simply does something else, which is the `&>`
failure mode again and the second measured instance of it.

`(( … ))` behaves the same way in dash, where it is two nested subshells
running `1+1` as a command name.

`[[` is a **reserved word, not an operator**, so it needs surrounding
blanks: every shell reports `[[a: not found` for `[[a == a]]`. The same is
true of `]]`. Contrast `((`, which is punctuation.

`(( expr ))` evaluates the expression and exits **0 when it is non-zero**,
which is the reverse of the usual convention and is unanimous:

    (( 1+1 )); echo $?   →  0
    (( 0 ));   echo $?   →  1

Vector fields: `DoubleBracket` and `ArithCommand`, both default true, both
false for `posix`. As with `&>`, turning them off does not make the text
invalid — it makes it mean something else.

### Which layer handles which

They look like the same problem and are not, so the split is recorded
here rather than rediscovered.

`(( … ))` is the **lexer's**. What is inside is an arithmetic expression,
not a command list, so it is scanned as raw text: tokenizing `(( 2 > 1 ))`
as commands would turn the comparison into a redirection and lose the
program. Nothing about command position is needed, because the
distinction is textual — measured, `((echo nested))` is arithmetic in
bash, ksh93 and zsh even with no space, while `( (echo sub) )` is nested
subshells.

`[[ … ]]` is the **parser's**, despite `<` and `>` meaning something
different inside it. `[[` is only special where a command may begin —
`echo [[ a ]]` prints `[[ a ]]` — and the lexer does not know where
commands begin. Lexing `<` as an operator loses nothing: the parser knows
it is inside `[[ ]]` and reinterprets the token. A lexer mode keyed on
seeing the word `[[` would break `echo`.
