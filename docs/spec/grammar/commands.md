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

Semantics axis: `AssignmentPrefixPersistsOnSpecialBuiltin` — dash and
ksh93 yes, bash and zsh no. Unanswered in the core. POSIX requires yes,
so the `posix` preset says yes and the two shells that ship a POSIX mode
switch to it there; recording that as a default of "no" would have
inverted the standard's own answer.

### Array assignment

An assignment whose value is a parenthesized word list makes an array,
and there are four spellings:

    a=(x y z)        a literal
    a[i]=v           one element
    a+=(y z)         append to the end
    a[i]+=v          append to one element

The parentheses are core — bash, ksh93 and zsh have them, dash does not
and calls the `(` a syntax error rather than reading it as anything
else. Grammar flag: `ArrayLiteral` (on in the core, off for `posix` and
`dash`).

**The `(` must be adjacent to the `=`, in every shell but ksh93.**

| probe | dash | bash 5 | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `a= (echo x)` | error | error | error | **accepted** | error |

Where it is accepted it is the *array literal*, not an assignment
followed by a subshell: ksh93 leaves `a` holding two elements, `echo`
and `x` (measured 2026-09-05, panel and machine as `../oracle.md`). So
the space is not significant there, and the parser cannot decide the
construct on adjacency alone — it decides *which diagnostic*, and the
non-adjacent form falls through to the ordinary rule for a `(` after a
word.

**The elements are words**, expanded, split and globbed like any other
unquoted word, and a newline inside the parentheses separates elements
rather than ending a command:

    x="p q"; a=($x)      →  2 elements in dash-family shells, 1 in zsh
    x="p q"; a=("$x")    →  1 element, unanimously
    a=(x
    y)                   →  2 elements, unanimously

The two-element answer for `a=($x)` is the `SplitParamExpansion` axis
from `semantics.md` reaching into the literal; it is one rule, not a
second one for arrays.

Three corners split the panel and each is silent:

| probe | bash 5 | ksh93 | zsh |
| --- | --- | --- | --- |
| `a=(); echo ${#a[@]}` | 0 | **1** | 0 |
| `a=(x y); a[0]+=Q` | `xQ` | `xQ` | **refused** |
| `a=([2]=c [0]=a)` | placed | placed | **refused** |

`a=()` is an *empty array* in bash and zsh. In ksh93 it declares a
compound variable instead — `typeset -p a` answers `typeset -C a=()`,
`${#a[@]}` is 1, and `${a[0]}` renders as the two-line text `(` `)`.
zsh refuses an append to a single element (`assignment to invalid
subscript range`) and refuses a subscript written inside the literal
(`bad subscript for direct array assignment`), both at status 1.

The subscript on the left of an assignment inherits the array-base axis:
`a[1]=Q` replaces the *second* element in bash and ksh93 and the first
in zsh, exactly as `${a[1]}` reads it.

An array assignment may also be an **operand** of a declaration utility
— `typeset a=(x y)`, `local a=(x y)`, `readonly a=(p q)` — which is a
separate grammar flag, because it is reached by the word after a command
name rather than by an assignment prefix (`DeclarationUtilities`;
measured: `decl/an-array-assignment-as-an-operand`,
`decl/a-local-array-stays-local`, `decl/readonly-takes-its-array-first`).

Corpus: `core/append-to-an-array`, `core/array-star-joins`,
`array/a-subscript-past-the-end`, `array/removing-one-element`,
`pat/an-array-literal-is-not-a-group`.

**Two of the rows above are ahead of the implementation**, and are
recorded so the gap is a known one: a subscript inside a literal is kept
as literal text here rather than placed (`a=([2]=c)` leaves one element
reading `[2]=c`), and `a[i]+=v` replaces the element instead of
appending to it. Both are unanimous in the two shells that answer them.

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

## C-style `for ((init; cond; post))`

A loop on a condition rather than over a list. Core — bash, ksh93 and
zsh have it and dash does not, and dash's refusal blames the *loop
variable* rather than the parenthesis, which is the tell that it read
`for` and then failed to find a name:

    dash: Syntax error: Bad for loop variable

Grammar flag: `CStyleFor` (on in the core, off for `posix` and `dash`).
Measured: `core/c-style-for`.

**The header is arithmetic, not a word list.** The three parts are the
expressions of `arithmetic.md`, so a bare name in the condition is a
variable rather than a word — `n=2; for ((i=0;i<n;i++))` iterates twice
in all three, with no `$` anywhere. The comma operator is available and
lets each part carry more than one expression:
`for ((i=0,j=9; i<3; i++,j--))` walks both, unanimously.

**Any of the three may be omitted, and an omitted condition is true.**
That is what makes the endless loop spell as it does:

    for ((;;))         →  runs until something breaks
    for ((i=0;;i++))   →  the same, with an initialization and a step
    for ((;i<3;))      →  a while loop written this way

All measured unanimous across bash 5.3, bash 3.2, ksh93 and zsh. The
consequence for an AST is that "omitted" and "the expression `0`" are
different: a missing condition loops forever and a false one runs the
body zero times and exits 0.

**The loop variable is an ordinary variable and survives the loop** —
`for ((i=0;i<3;i++)); do :; done` leaves `i` at 3 — which follows from
the header being arithmetic in the current scope.

Two shapes are specific to this loop and neither holds for any other:

- **No separator is required before `do`.** `for ((i=0;i<2;i++)) do …
  done` is accepted by all four, where the same document's rule above
  says a `;` or newline must precede `do`. The `))` has already ended
  the header, so there is nothing for a separator to delimit.
- **The body may be a brace group instead of `do … done`.**
  `for ((i=0;i<2;i++)) { …; }` runs in bash 5.3, bash 3.2, ksh93 and
  zsh alike. It is not a general loop syntax: `for i in a b { …; }` and
  `while cond { …; }` are parse errors in all four, except that zsh
  takes the brace body on `while`. **This implementation accepts only
  `do … done`**, and the brace form is a known gap rather than a
  decision.

The header is also a *lexer* fact rather than a parser one: `(( … ))`
arrives whole, because what is inside is arithmetic and not a command
list, so the three parts are cut on the semicolons afterwards.

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

### An operator where a pattern belongs

    case a in & ) echo hit;; *) echo miss;; esac

dash **parses and runs** this, and prints `miss`. bash 5.3, bash 3.2,
ksh93 and zsh all refuse it as a syntax error at the `&`. The arm the
operator opens matches nothing at all — not `&`, not the empty string —
and what dash accepts is exactly one operator where the pattern list
would start: `&a )` then fails at the word and `a& )` at the `&`.

It is a grammar flag, `CasePatternAcceptsOperator` (dash only), and it is
measured rather than inferred from the diagnostic, because the
diagnostic is not the whole of it — a shell that merely worded the error
differently would still not *run* the `esac`. It also changes where an
unrelated construct is blamed: `;;&` in a dialect without that terminator
gets a different diagnosis under dash, which is already past the `&` and
complaining about the `esac` where the `)` should be.

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

### When the definition is committed to

Some shells decide they are reading a function definition as soon as a
name is followed by `(`; others wait for the `()` pair. Nothing about a
well-formed definition depends on this — it decides **which token a
malformed one is blamed on**:

    f ( x ) { echo hi; }

    bash 5.3, bash 3.2, dash   the error is at `x`  — already inside a
                               definition, looking for `)`
    ksh93                      the error is at `(`  — never entered one
    zsh                        the error is at `}`

Grammar flag: `FuncDefAtParen`, on for bash and dash. It is reached most
often through a construct a dialect does not have: `[[ ( -n x ) ]]` is a
definition of a function called `[[` to a shell without `[[`, and the
blame lands accordingly. Committing at the paren also accepts more names
than `FunctionNamePunctuation` does on its own — `f+x()`, `@weird()` —
which is why the two flags are separate.

## `times` is a reserved word in one shell

    times extra

runs in bash, bash 3.2 and dash, which print the four times and ignore
the operand, and in zsh, which complains at run time (`times: too many
arguments`, status 1). **ksh93 makes it a syntax error** — `` `extra'
unexpected `` — because `times` is a reserved word there rather than a
builtin, so a word after it cannot be an argument.

Grammar flag: `TimesIsReserved`, ksh only. It is the only place in the
panel where **which builtin a shell has changes what parses**, which is
why it is a grammar flag at all: everywhere else the set of builtins is
purely a runtime question (`../semantics.md`).

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
    for-arith   :  'for' '((' [ expr ] ';' [ expr ] ';' [ expr ] '))'
                   [ sep ] ( 'do' list sep 'done' | '{' list sep '}' )
    select      :  'select' name [ [ 'in' word* ] sep ] 'do' list sep 'done'
    case        :  'case' word 'in' { case-item } 'esac'
    case-item   :  [ '(' ] pattern { '|' pattern } ')' [ list ] terminator
    terminator  :  ';;' | ';&' | ';;&'
    coproc      :  'coproc' ( command | name compound )

`for-arith` and `coproc` are not in XCU; each has its own section below,
and `coproc` is the one line here that is a dialect's rather than the
core's.

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

Grammar flags: `DoubleBracket` and `ArithCommand` — core: on for both;
`posix` and `dash`: off for both. As with `&>`, turning them off does not
make the text invalid — it makes it mean something else.

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

## `coproc`

A command run in the background with a pipe on each of its standard
streams, and the near ends kept where the script can reach them.

**It is a keyword in two shells and a different feature in each.**
Measured with `type coproc`:

| shell | `coproc` is | how the near ends are reached |
| --- | --- | --- |
| dash | not found | — |
| bash 3.2 | not found | — |
| bash 5.3 | a shell keyword | the array `COPROC`, pid in `COPROC_PID` |
| ksh93 | not found | `cmd \|&`, then `print -p` and `read -p` |
| zsh | a reserved word | `coproc cmd`, then `print -p` and `read -p` |

So the word `coproc` is shared by bash and zsh and the *model* is not.
bash names a coprocess and hands back a two-element array of file
descriptors; zsh has one anonymous coprocess addressed as `>&p` and
`<&p`, and no name may be written at all — `coproc MY { cat; }` is a
parse error there. ksh93 has the zsh model under a different spelling,
`cat |&`, with no `coproc` word. Only bash's is this construct.

Grammar flag: `Coproc` (off in the core, on for `bash`). It is not core
even though two shells have the keyword, because a switch that made the
word parse would still leave two incompatible ways to talk to what it
started.

Measured in bash 5.3 (2026-09-05):

    coproc cat
    echo hi >&"${COPROC[1]}"
    read -r l <&"${COPROC[0]}"      →  hi

**A name may be written only before a compound command.** This is the
whole of the grammar and it is where an implementation guesses wrong:

    coproc cat            →  runs cat; the array is COPROC
    coproc MY { cat; }    →  runs cat; the array is MY
    coproc MY ( cat )     →  the same — a subshell is compound too
    coproc MY cat         →  runs *MY* with the argument cat

The last row is the point: bash reports `MY: command not found` from the
background job, having taken the first word of a simple command as the
command. There is no ambiguity to resolve at run time, because the shape
of what follows decides it while parsing. Named coprocesses coexist —
`coproc A { cat; }; coproc B { cat; }` gives two independent pairs.

`coproc` is a **bash 4** feature, and dates rather than vetoes nothing:
bash 3.2 has no such word, so `coproc cat` there is a command lookup
that fails with 127 and `coproc MY { cat; }` is a syntax error at the
`}`. The same rule `;&` follows in the `case` table above, from
`../core.md`.

Corpus: `commands/coproc-is-one-dialect-s-keyword`. The name-before-a-
compound rule is measured here and is **not yet pinned by a case**.
