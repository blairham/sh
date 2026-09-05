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

## Two commands need a separator between them

The `[ ; | & | newline ]` in the list production is required and not
decoration: a command standing straight after one that has *ended itself*
is a syntax error in every shell in the panel. Measured 2026-09-05 with
`-n`, on dash, bash 3.2.57 (`/bin/bash`), bash 5.3.15
(`/opt/homebrew/bin/bash`), bash 5.3.15 invoked as `sh`, ksh93u+ and
zsh 5.9.2 (`/opt/homebrew/bin/zsh`):

| probe | dash | bash 3.2 | bash 5 | bash as sh | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `(( 1 )) echo x` | error | error | error | error | error | error |
| `{ :; } echo x` | error | error | error | error | error | error |
| `(echo a) echo b` | error | error | error | error | error | error |
| `if true; then :; fi echo z` | error | error | error | error | error | error |
| `for i in a; do :; done echo w` | error | error | error | error | error | error |
| `case a in a) :;; esac echo u` | error | error | error | error | error | error |
| `f() { :; } echo t` | error | error | error | error | error | error |
| `(echo a) (echo b)` | error | error | error | error | error | error |
| `(echo a) >/dev/null echo b` | error | error | error | error | error | error |
| `[[ -n x ]] echo y` | *n/a* | error | error | error | error | error |

Unanimous, and the `[[ ]]` row is only *n/a* in dash because there
`[[ -n x ]]` is a simple command whose words absorb `echo y`. That is the
whole reason the rule is invisible until a compound is written: a simple
command takes the next word as an argument, so `true echo x` is one
command and there is nothing to refuse. It shows on every compound,
because a compound has closed and cannot absorb anything.

The consequence is not only the refusal. Where the text is a *different
program* the wrong reading is silent — `(echo a) echo b` would run two
commands where every shell in the panel refuses the line — so a typo they
all catch at parse time would run.

Two shells name the closer they were waiting for. dash prints
`(expecting ")")` inside a subshell, `(expecting "}")` inside a brace
group, `(expecting "done")` inside a loop and `(expecting "fi")` inside
an `if`, and only when the enclosing construct had something in it:
`( echo a; fi )` is `"fi" unexpected (expecting ")")` there and `( fi )`
is `"fi" unexpected`. So the token is named where the list stopped and
the *expectation* comes from whatever was waiting for it.

The one exception is the shell with short loops, and it is the same rule
rather than a hole in it: there a loop header that has ended is followed
by its *body*, so the second command is not a second statement of the
same list. See "Short forms" below. Its body is a whole statement,
terminator included, and the loop is terminated by whatever the body
took — `for i (a b) echo $i; echo end` parses there and
`for i (a b) { echo $i; } echo end` does not, because a brace body is
closed by its own `}` and leaves nothing between the two commands.

Corpus: `core/two-commands-need-a-separator-between-them`,
`core/a-compound-does-not-absorb-the-word-after-it`,
`core/a-separator-is-needed-after-a-redirected-compound`,
`core/the-missing-separator-is-named-inside-a-group`,
`core/two-subshells-with-nothing-between-them`,
`core/a-missing-separator-inside-a-loop-body`.

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

**Two flags, not one.** `ArrayLiteral` is the parenthesized value and
nothing else; the two *subscripted* spellings are `ArraySubscript`'s,
the same flag that decides whether `${a[i]}` is read. Naming only the
first is what let a shell with no arrays store an element (#618).

| probe | dash | bash 3.2 | bash 5 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `a[0]=x; echo "[${a[0]}]"` | `a[0]=x: not found`, then `Bad substitution` | `[x]` | `[x]` | `[x]` | refuses the subscript |
| `echo "[${a[0]}]"` | `Bad substitution` | `[]` | `[]` | `[]` | `[]` |
| `a[0]=x; a[0]+=Q` | `a[0]=x: not found`, `a[0]+=Q: not found` | `[xQ]` | `[xQ]` | `[xQ]` | refuses the subscript |

Measured 2026-09-05, panel and machine as `../oracle.md`. Reading a
subscript and writing through one split the panel identically — four
shells against dash — which is why one flag covers both and a third is
not added. zsh is not a fifth answer: it *parses* `a[0]=x` and rejects
the subscript, because `0` is below its first element, which is the
array-base axis on the semantics vector and not a grammar question
(`array/a-subscript-below-the-first-element`).

Being wrong here is one-sided and silent. Where the flag is off the word
is a command name — the no-array shell reports `a[0]=x: not found` and
carries on — so accepting the shape anyway stores an element, says
nothing, and tells someone testing under that dialect on purpose that a
script is portable when it is not. The `+=` form was accidentally right
throughout, because `AppendAssign` is also off there and gated it by
another road; the plain form had nothing (corpus:
`array/a-subscript-where-there-are-no-arrays`,
`array/appending-to-an-element-where-there-are-no-arrays`).

**The `(` must be adjacent to the `=`, in every shell but ksh93.**

| probe | dash | bash 5 | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `a= (echo x)` | error | error | error | **accepted** | error |

Where it is accepted it is the *array literal*, not an assignment
followed by a subshell: ksh93 leaves `a` holding two elements, `echo`
and `x` (measured 2026-09-05, panel and machine as `../oracle.md`;
pinned by `array/a-literal-with-a-space-before-it`). So
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

One corner splits the panel and is silent:

| probe | bash 5 | ksh93 | zsh |
| --- | --- | --- | --- |
| `a=(); echo ${#a[@]}` | 0 | **1** | 0 |

`a=()` is an *empty array* in bash and zsh. In ksh93 it declares a
compound variable instead — `typeset -p a` answers `typeset -C a=()`,
`${#a[@]}` is 1, and `${a[0]}` renders as the two-line text `(` `)`.

### A subscript inside the literal

An element may name where it goes: `a=([2]=c [1]=b)` is two elements, at
subscripts 1 and 2, and the brackets are not part of either value. This
is the ordinary way to build a sparse array, and it is where the panel
splits twice — neither split being the one an earlier reading recorded,
which was that zsh refuses the form outright. Measured 2026-09-05:

| probe | bash 5 | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `a=([2]=c [1]=b)` | `b c` | `b c` | `b c` | `b c` |
| `a=([2]=c); echo ${#a[@]}` | 1 | 1 | 1 | **2** |
| `a=([2]=c [2]=d)` | `d` | `d` | `d` | `d` |
| `x="p q"; a=([2]=$x)` | one element | one element | one element | one element |
| `a=([2]=c); a+=([5]=f)` | at 2 and 5 | at 2 and 5 | at 2 and 5 | at 2 and 5 |
| `i=2; a=([1+1]=c [i]=d)` | 1 element | 1 element | **2 elements** | 1 element |
| `a=(x [3]=y z)` | `x y z` | `x y z` | **`x [3]=y z`** | `x  y z` |
| `a=([0]=p)` | placed | placed | placed | **refused** |

Placing is unanimous, and so is everything about how a value is read:
the value of a subscripted element is an **assignment's** value and is
not field-split, where a bare element in the same parentheses is a word
and is; a repeated subscript keeps the later value, so the elements are
placed in the order written; and `+=` keeps what is there while a
subscripted element in the appended literal still places rather than
landing after the end.

**zsh does not refuse the form.** It refuses subscript `0`, and for the
subscript rather than for the literal: `0` is below zsh's first
subscript, so a plain `a[0]=Q` earns the same complaint in different
words. `a=([2]=c)` is accepted there, and the two elements it then
reports are the sparse-array axis (`ArraysAreSparse`) reading the gap
below the subscript, not a second refusal. The `${#a[@]}` row is that
axis and nothing else.

**ksh93 reads the subscript as a key rather than as an expression**, and
a literal written with one declares a *keyed* array: `typeset -p a`
answers `typeset -A`, `[1+1]` and `[i]` are two different three- and
one-character keys rather than two spellings of 2, and `${a[2]}` finds
nothing that `a=([1+1]=c)` stored. bash and zsh evaluate the subscript,
so both spellings land on the same element. Semantics axis:
`ArrayLiteralSubscriptIsAKey` — ksh93 yes, bash and zsh no. Asked only
where the two readings differ: a plain decimal numeral evaluates to
itself, so `a=([2]=c)` fills the same slot either way and the core needs
no dialect for it.

The last row is a divergence that is **recorded rather than modeled**.
bash and zsh take a literal mixing bare and subscripted elements, and a
bare element after a subscripted one continues from *that* subscript —
`z` lands one past `y` — which follows the written subscript through the
array base, so the same literal fills the same positions under either
answer. ksh93 takes no such mixture: `a=(x [3]=y z)` leaves the middle
element as the five characters `[3]=y`, and `a=([1]=p q)` is a syntax
error. Two different refusals of one shape is thin ground for an axis,
and the implementation places in every dialect.

`a[i]+=v` is **not** one of the corners, which an earlier reading of
`a=(x y); a[0]+=Q` had it be: zsh refuses that line, but for the
subscript rather than for the append. `0` is below zsh's first
subscript, so the same refusal (`assignment to invalid subscript
range`) answers a plain `a[0]=Q`; write `a[1]+=Q` or `a[-1]+=Q` and zsh
joins the element like the other two. Measured 2026-09-05:

| probe | bash 5 | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `a=(x y); a[-1]+=Q` | `x yQ` | refused | `x yQ` | `x yQ` |
| `a=(x y); a[1]+=Q` | `x yQ` | `x yQ` | `x yQ` | `xQ y` |
| `a[3]+=Q; echo "${a[3]}"` | `Q` | `Q` | `Q` | `Q` |
| `typeset -A m; m[k]+=x; m[k]+=Q` | `xQ` | no `-A` | `xQ` | `xQ` |

So appending to an element is unanimous in the three shells with
arrays, and the only thing about it that is a question is *which*
element a numeral names. An element that was never assigned has nothing
to append to, and all three place the value as it stands rather than
refusing. dash has no arrays and reads every line above as a command
name.

bash 3.2 is not a fourth answer in either row it declines: it has no
negative subscripts at all (`a[-1]: bad array subscript`, the same
refusal a plain `a[-1]=Q` gets there) and no associative arrays for
`-A` to declare, so what it says is about those two features rather
than about appending, which it does exactly as bash 5 does.

The subscript on the left of an assignment inherits the array-base axis:
`a[1]=Q` replaces the *second* element in bash and ksh93 and the first
in zsh, exactly as `${a[1]}` reads it — and the second row above says
`a[1]+=Q` inherits it too, so appending is not a form with a subscript
rule of its own.

An array assignment may also be an **operand** of a declaration utility
— `typeset a=(x y)`, `local a=(x y)`, `readonly a=(p q)` — which is a
separate grammar flag, because it is reached by the word after a command
name rather than by an assignment prefix (`DeclarationUtilities`;
measured: `decl/an-array-assignment-as-an-operand`,
`decl/a-local-array-stays-local`, `decl/readonly-takes-its-array-first`).

A subscript a script assigns **through** is an arithmetic expression and
not only a numeral, wherever it is written: `a[1+1]=v`, `a[i]=v` and
`a=([1+1]=v)` all name the element a bare `2` names, in bash and zsh.
Reading one back is a separate path and does not evaluate here yet —
`${a[1+1]}` finds nothing where bash finds the element — which is
recorded so the gap is a known one.

Corpus: `core/append-to-an-array`, `core/array-star-joins`,
`array/a-subscript-past-the-end`, `array/removing-one-element`,
`array/appending-to-an-element`,
`array/appending-to-an-element-inherits-the-base`,
`array/appending-to-an-unset-element`,
`array/appending-to-an-associative-element`,
`array/a-literal-places-its-subscripts`, `array/a-literal-leaves-a-gap`,
`array/a-literal-subscript-is-an-expression`,
`array/a-literal-repeats-a-subscript`,
`array/a-literal-mixes-subscripts-and-positions`,
`array/appending-a-literal-with-a-subscript`,
`array/a-literal-value-is-an-assignment-value`,
`pat/an-array-literal-is-not-a-group`.

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

**Every** compound form, and the one that was missed is worth recording
because of how quietly it failed. The C-style `for` had no list to keep
one in, so the operator was left standing where it was and became a
statement of its own — a redirection with no command, which truncates the
file and redirects nothing:

    for ((i=0;i<2;i++)); do echo $i; done >f    the numbers on the
                                                terminal and f empty

There was no diagnostic anywhere, because both halves are legal: a
redirection with no command is a real construct, and it is exactly what
is left over when the one before it did not take its own suffix. A node
without a redirection list does not refuse a redirection — it silently
means something else. Measured 2026-09-05 on the four shells that have
the loop; dash has no C-style `for` at all:

    for ((i=0;i<2;i++)); do echo $i; done >f    f holds 0 and 1
    for ((i=0;i<2;i++)) { echo $i; } >f         the same, brace body
    for ((i=0;i<2;i++)); do read x; …; done <d  the loop reads the file
    for … done 2>f                              and the same for stderr

## Loops

`while`, `until` and `for` exit **0 when the body never runs**:

    while false; do :; done; echo $?  →  0
    until true;  do :; done; echo $?  →  0
    for i in;    do echo x; done; echo $?  →  0

Unanimous, and worth pinning because "status of the last command" is the
obvious wrong answer when there was no last command.

And they exit with **the body's last status once the body has run**. That
is the other half of the same rule, and a shell written only for the half
above loses it: the condition is evaluated once more after the final
iteration, so the status standing when the loop ends is the *condition's*
and it is not the answer. Measured 2026-09-05 on dash (`/bin/dash`), bash
3.2.57 (`/bin/bash`), bash 5.3.15 (`/opt/homebrew/bin/bash`), that same
5.3.15 binary through a symlink named `sh`, ksh93u+ (`/bin/ksh`) and zsh
5.9.2 (`/opt/homebrew/bin/zsh`) — unanimous on every row, and POSIX says
the same:

| probe | all six |
| --- | --- |
| `i=0; while [ $i -lt 1 ]; do i=1; false; done; echo $?` | 1 |
| `i=0; while [ $i -lt 1 ]; do i=1; true; done; echo $?` | **0** |
| `i=0; until [ $i -ge 1 ]; do i=1; false; done; echo $?` | 1 |
| `for i in a; do false; done; echo $?` | 1 |
| `for ((i=0;i<1;i++)); do false; done; echo $?` | 1 (dash has no such loop) |
| `i=0; while [ $i -lt 2 ]; do false; i=$((i+1)); done; echo $?` | 0 |
| `i=0; while [ $i -lt 3 ]; do i=$((i+1)); false; break; done; echo $?` | 0 |
| `i=0; while [ $i -lt 2 ]; do i=$((i+1)); false; continue; done; echo $?` | 0 |
| `false; while false; do :; done; echo $?` | 0 |

The **second row is the one that discriminates**, and without it the wrong
answer looks right: the condition that ended that loop was false and the
body's last command succeeded, so a shell reporting the condition would
say 1 where all six say 0. The first row cannot tell them apart, because
there the condition and the body agree.

`break` is not an exception. It is a command of the body and it succeeds,
so the loop answers 0 even where the command before it failed — the same
rule, not a rule about `break`.

### What `$?` is on the way in

A different question from what the loop *reports*, and one an
implementation is likely to answer with the same line. The live status
belongs to the last command that ran until the loop has something of its
own to say, so the first iteration and the condition both see what
preceded the loop:

| probe | all six |
| --- | --- |
| `false; for i in a b; do echo "it=$?"; done` | `it=1` then `it=0` |
| `false; while [ $? -eq 0 ]; do echo ran; break; done; echo end` | `end` — the body never runs |
| `false; until [ $? -ne 0 ]; do echo ran; break; done; echo end` | `end` |
| `false; while true; do echo "it=$?"; break; done` | `it=0` — the condition ran last |

The second row is the one with teeth: the answer decides whether the body
runs at all, so a shell that zeroes the status before the first test runs
a loop that no shell in the panel runs.

The item list is **not** part of this. A command substitution in it —
`false; for i in $(echo a); do echo $?; done` — leaves 1 in dash and in
bash invoked as `sh`, and 0 in bash 5, ksh93 and zsh, so there is no
common answer to write down and none is claimed here.

Corpus: `cmd/loop-status-when-body-never-runs`,
`cmd/for-status-empty-list`,
`core/a-while-loop-answers-with-its-body`,
`core/a-loop-answers-with-its-body-and-not-its-condition`,
`core/an-until-loop-answers-with-its-body`,
`core/a-loop-that-never-ran-does-not-inherit`,
`core/a-break-is-a-command-of-the-body`,
`core/a-c-style-loop-answers-with-its-body`,
`core/the-status-a-loop-body-starts-from`,
`core/a-loop-condition-sees-the-status-before-it`.

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
body zero times and exits 0. Pinned, one per shape:
`core/c-style-for-with-an-empty-header`,
`core/c-style-for-without-a-condition` and
`core/c-style-for-with-only-a-condition`.

**The loop variable is an ordinary variable and survives the loop** —
`for ((i=0;i<3;i++)); do :; done` leaves `i` at 3 — which follows from
the header being arithmetic in the current scope.

One shape is specific to this loop: **no separator is required before
`do`.** `for ((i=0;i<2;i++)) do … done` is accepted by all four, where
the same document's rule above says a `;` or newline must precede `do`.
The `))` has already ended the header, so there is nothing for a
separator to delimit.

The header is also a *lexer* fact rather than a parser one: `(( … ))`
arrives whole, because what is inside is arithmetic and not a command
list, so the three parts are cut on the semicolons afterwards.

Nothing else about it is special, and that includes the redirection
suffix every other compound command takes — see "Compound commands take
redirections", where the shape this loop got wrong is written down.

## A brace group as a loop body

**A `for` loop may take `{ …; }` where `do … done` stands**, in either
spelling of the header, and so may `select`, whose header is a
for-loop's. Measured 2026-09-05:

| probe | dash | bash 5 | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `for ((i=0;i<2;i++)) { …; }` | *n/a* | yes | yes | yes | yes |
| `for ((i=0;i<2;i++)); { …; }` | *n/a* | yes | yes | yes | yes |
| `for i in a b; { …; }` | **error** | yes | yes | yes | yes |
| `for i in a b` ⏎ `{ …; }` | **error** | yes | yes | yes | yes |
| `for i; { …; }` | **error** | yes | yes | yes | yes |
| `select x in a; { …; }` | *n/a* | yes | yes | yes | yes |
| `for i in a b { …; }` | error | error | error | error | error |
| `while true { …; }` | error | error | error | error | error |
| `while true; { …; }` | error | error | error | error | **yes** |
| `if true; { …; }` | error | error | error | error | error |
| `if [[ -n x ]] { …; }` | error | error | error | error | **runs** |
| `if (( 1 )) { …; }` | error | error | error | error | **runs** |
| `if … { … } elif … { … } else { … }` | error | error | error | error | **runs** |
| `if [[ -n x ]] echo A` | error | error | error | error | **runs** |
| `if true { …; }` | error | error | error | error | error |
| `repeat 3 { …; }`, `repeat 3 echo R` | error | error | error | error | **runs** |
| `foreach f (a b); …; end` | error | error | error | error | **runs** |
| `() { …; } p q` | error | error | error | error | **runs** |
| `function { …; }` | error | error | error | error | **runs** |

Grammar flag: `ForBraceBody` (on in the core, off for `posix` and
`dash`). dash is the only panel shell that refuses the form, which is
the usual reason a construct is core.

**The separator is required in the list form, and not for a reason
about this flag.** With nothing between, `{` is another *item* of the
list — the loop reads on and meets `}` where `do` belongs, which is
what all five then complain about. Only the C-style header ends itself,
which is why that one form takes the brace with nothing between. So
`for i in a b { …; }` failing everywhere is not evidence that the list
form lacks the production; it is evidence about where a word list ends.

**The brace body belongs to those two loops and to nothing else.**
`while`, `until` and `if` refuse it, with or without a separator. One
shell has a wider family of its own — see *Short forms* below, where the
`while` row is answered — and `if` gets nothing anywhere.

The group is the ordinary one and keeps every rule it already has: the
body needs a terminator before `}` wherever a brace group does — so
`for i in a b; { echo $i }` is refused by bash and ksh93 and accepted by
zsh, exactly as a bare `{ echo x }` is — and a redirection after the
closing brace belongs to the loop as one after `done` does. Its list is
the loop's body rather than a group nested inside it, so `break` and
`continue` reach the loop.

One wording diverges and is recorded rather than modeled: with the body
left open, bash names the `{` and ksh93 names the `for`. This
implementation names the `{`.

Corpus: `core/c-style-for-with-a-brace-body`,
`core/c-style-for-brace-body-after-a-separator`,
`core/a-list-for-with-a-brace-body`,
`core/a-list-for-brace-body-needs-a-separator`,
`core/a-brace-body-is-not-a-while-body`.

## Short forms

One shell writes a compound command's body without the words that
ordinarily open and close it, and the family is wider than the brace body
above — wider, too, than loops, which is where this section stopped
because the flag was called `ShortLoop` and `if` is not a loop (#827).
Measured 2026-09-05 (panel and machine as `../oracle.md`); the accepted
rows are that shell's alone.

**Measured from a file, not through `-c`.** A command string whose last
character is the `}` of a short body is a parse error in that shell,
where the identical text with a trailing newline parses — so `-c` reports
a grammar no script on disk has. Every case for this family runs from a
file for that reason, and the harness writes one with the newline on it.

| probe | dash | bash 3.2 | bash 5 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `while (( i<2 )) echo $i` | error | error | error | error | **runs** |
| `while (( i<2 )) { …; }` | error | error | error | error | **runs** |
| `until [[ -n $x ]] { …; }` | error | error | error | error | **runs** |
| `for i (a b) { echo $i; }` | error | error | error | error | **runs** |
| `for i (a b) echo $i` | error | error | error | error | **runs** |
| `select x (a b) { …; }` | error | error | error | error | **runs** |
| `for i in a b; echo $i` | error | error | error | error | **runs** |
| `while false` | error | error | error | error | **accepted** |
| `for i (a b)`, `for i` | error | error | error | error | **accepted** |
| `while true { …; }` | error | error | error | error | error |
| `for i in a b` | error | error | error | error | error |
| `if true; { …; }` | error | error | error | error | error |
| `if [[ -n x ]] { …; }` | error | error | error | error | **runs** |
| `if (( 1 )) { …; }` | error | error | error | error | **runs** |
| `if … { … } elif … { … } else { … }` | error | error | error | error | **runs** |
| `if [[ -n x ]] echo A` | error | error | error | error | **runs** |
| `if true { …; }` | error | error | error | error | error |
| `repeat 3 { …; }`, `repeat 3 echo R` | error | error | error | error | **runs** |
| `foreach f (a b); …; end` | error | error | error | error | **runs** |
| `() { …; } p q` | error | error | error | error | **runs** |
| `function { …; }` | error | error | error | error | **runs** |

Grammar flags: `ShortForm` — off in the core, on for `zsh` alone — plus
`Repeat`, `Foreach` and `AnonymousFunction`, which are separate because
they are separate questions: a shell could spell a `repeat` body only as
`do … done`, and the words `repeat`, `foreach`, `end` and `()` are
constructs rather than body spellings.

It was `ShortLoop`, and the name is why this section's coverage stopped
where it did. The rule it stands for says nothing about looping.

**Two productions and one flag, because they are one rule:** a loop
header that has *ended* may be followed straight by its body, and the
body may be left out. What ends a header is the whole of it —
`(( … ))` and `[[ … ]]` close, a word does not, which is why
`while true { …; }` is refused there as well as everywhere else. For a
`for`, the parenthesized list closes and so does having no `in` clause;
`for i in a b` closes on a separator and on nothing else, which is the
same rule that makes `for i in a b { …; }` read `{` as another item.

**`if` is the same rule and not a production of its own.** Its condition
is its header, so `if [[ -n x ]] { … }` and `if (( 1 )) { … }` run and
`if true { … }` does not — the same three lines that decide a `while`.
The body is one command or a brace group, exactly as a loop's is, so
`if [[ -n x ]] echo A; echo after` prints `A after` and the `;` belongs
to the enclosing list. `elif` and `else` take their bodies the same way
and the command ends where the last of them does: there is no `fi`, and
a `;` before `else` ends the whole `if` and leaves the `else` with
nothing to attach to.

**What ends a condition is a measured set rather than a tidy one.** A
group, a subshell and a `case … esac` close as well, a `!` in front
changes nothing, and a **loop does not**: `if for i in a; do true; done
{ … }` is a syntax error in the shell that takes every other row, which
is a fact about that shell rather than a rule anyone could derive. A
pipeline is judged on its **last** command: `if [[ -n x ]] | cat { … }`
is refused because `cat` does not end a header, and
`if true | [[ -n x ]] { … }` runs. That row was found by mutation — the
rule had been written as "a pipeline never ends itself", which agrees
with the first of the two for the wrong reason. Our parse error names the
`{` where that shell names the `}` on the two loop rows; everything else
agrees byte for byte.

**`while cond; { …; }` is not a short body**, and this is the reading the
shape invites and the measurement refuses. The `;` keeps the *condition
list* going, so the brace group is the last thing tested and the body is
empty:

    i=0; while (( i<2 )); { echo $i; i=$((i+1)); }     →  0 1 2 3 … forever
    i=0; while (( i<2 ))  { echo $i; i=$((i+1)); }     →  0 1

The separator is the only difference between those two lines and it
reverses which half the group is in. `while false; echo A; echo B` shows
the same thing at length — it prints `A B` forever, because all three
statements are the condition list and the last one's status is what the
loop tests. So `while true; { echo hi; break; }` printing `hi` is the
`break` leaving a loop whose body was never there, not a body running
once (`core/a-brace-body-is-not-a-while-body`,
`core/a-tested-brace-group-is-not-a-body`,
`core/a-while-condition-that-ends-itself-takes-a-body`).

**The short body is one command**, not a list: a second needs a
separator, and a separator there belongs to whatever encloses the loop.
`while (( i<2 )) echo $((i++)); echo end` prints `end` once
(`core/a-short-loop-body-is-one-command`).

**A redirection written after a short body is that body's**, and after a
closed one it is the loop's — measured with the loop variable in the
target, which is what makes the two visible: `for i (a b) > f$i` leaves
`fa` and `fb`, so it ran once per iteration with `$i` set, while
`for i (a b) { echo hi; } > f$i` leaves one `f`, expanded before the loop
started (`core/a-short-loop-redirection-is-the-bodys`). The corollary is
that a loop with *no* body and a redirection of its own has no source
form at all, since any redirection after the header becomes the body.

The tree is the ordinary one — the parenthesized list is the `in` list,
and a short body is the loop's body, so `break` and `continue` reach the
loop. **Printing follows from that**: a body that was written short is
printed as `do … done`, which parses to the same tree under any dialect;
only an *omitted* body has no long spelling — `do done` is refused
everywhere — so that one is printed short, with the parenthesized list,
because `for i in a b` with nothing after it does not parse.

### `repeat`, `foreach` and a function with no name

Three constructs rather than three body spellings, and each has a flag of
its own.

**`repeat N`** is a loop over a count. The word is expanded and read as
an arithmetic expression **once**, before the first iteration — which is
what makes it a count rather than a condition — and a word that is not a
number, or is not positive, runs the body no times and reports success
rather than failing. The header is one word and ends itself, so every
body spelling is reachable: `do … done`, a brace group, one command, and
each of those again after an optional separator, which here belongs to
the header because there is no condition list for it to continue.
`repeat` is a keyword only where a command may begin: `repeat=5` is an
ordinary assignment.

A count that is not an *expression* is a different answer again:
`repeat '1+' { … }` is the same "bad math expression" the shell gives
`$(( 1+ ))`, word for word. That shell stops the script over it and this
one reports and carries on, which is the fatality of an arithmetic
failure rather than anything about this loop, and is recorded here rather
than modeled.

**`foreach name (a b) … end`** is the `for` this section already
describes under two other words. The tree is a `for`'s, so `break` and
`continue` reach the same place and the printer writes it back as
`for … do … done`, which every dialect can read. What it adds is `end`,
and `end` is reserved **wherever a command may begin** in that shell
rather than only inside the loop — `end` alone and `end() { :; }` are
both parse errors there, while `echo end` and `end=5` are not. The
terminator belongs to the opening word: `for f (a b); …; end` is refused.

**`() { … } word …`** is a function with no name, defined and run where
it stands, with the words after the body as its positional parameters.
`function { … }` is the same thing spelled with the keyword. It is a
*function* and not a group, which `local` is the test for, and the frame
it pushes reports a name the shell invents — `(anon)`, which is what `$0`
shows. `()` where a command begins is an empty parameter list rather than
a subshell, and nothing is lost by reading it that way, because a
subshell with nothing in it is a syntax error in all five.

`()` with nothing after it is the empty subshell it has always been
rather than a function with no body: `( ); echo ok` prints `ok` in the
shell that has both readings, which is `EmptyCompoundBody` and not this
rule. So the parentheses are read as a parameter list only when a body
follows them.

Corpus: `core/a-condition-that-ended-itself-takes-its-body`,
`core/a-short-if-takes-elif-and-else-the-same-way`,
`core/a-short-if-body-is-one-command`,
`core/a-word-condition-does-not-end-itself`,
`core/a-separator-after-a-condition-ends-the-if`,
`core/a-count-loop`, `core/a-count-that-is-not-one-runs-nothing`,
`core/a-loop-that-ends-with-end`,
`core/a-function-with-no-name-runs-where-it-stands`,
`core/a-nameless-function-is-a-function`,
`core/a-tested-brace-group-is-not-a-body`,
`core/a-while-condition-that-ends-itself-takes-a-body`,
`core/a-short-loop-body-is-one-command`,
`core/a-for-over-a-parenthesized-list`,
`core/a-for-header-that-ends-itself-needs-no-body`,
`core/a-short-loop-redirection-is-the-bodys`.

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

**ksh93 has a third answer, and it is not a weaker version of bash's.**
It takes the simple command and refuses a *redirection* in it, wherever
the operator stands (measured 2026-09-05, rows
`cmd/function-body-a-redirection`,
`cmd/function-body-a-command-with-a-redirection`,
`cmd/function-body-compound-with-a-redirection`):

    f() >out; f            dash, zsh: run    bash: at `>'    ksh93: at `>'
    f() echo hi >out; f    dash, zsh: run    bash: at `echo' ksh93: at `>'
    f() x=1 >out; f        dash, zsh: run    bash: at `x=1'  ksh93: at `>'
    f() 2>&1; f            dash, zsh: run    bash: at `2>&1' ksh93: at `>&'
    f() { echo hi; } >out  all four: run

The middle rows are what separate the two refusals. bash blames the
command and never reads what follows it; ksh93 accepts the `echo` and
blames the operator, so it is not asking for a compound body — it is
refusing to redirect one that is not. Braces make the same redirection
acceptable in every shell, which says the rule is about the body rather
than about redirecting a function. Grammar flag:
`FuncBodyTakesNoRedirection`, on for `ksh` alone.

### A body that never began

    f() ;     dash   Syntax error: ";" unexpected
              bash   syntax error near unexpected token `;'
              ksh93  syntax error at line 1: `;' unexpected
              zsh    parse error near `;'

    f()       dash   Syntax error: end of file unexpected
              bash   syntax error: unexpected end of file
              ksh93  syntax error at line 1: `end of file' unexpected
              zsh    parse error near `()'

Two things here are zsh's alone. It names **`()`** where the others'
last token would be the closing paren, because its lexer reads the empty
pair as one token — grammar flag `EmptyParensAreOneToken`, and `f( )`
with a space is not that token and is not read as a definition there at
all. And it prints **no line** in the location for either of these,
`zsh:` where the same shell writes `zsh:1:` for `if true`, which is an
input that ran out just as much. That is not the kind of failure, so the
parser marks the error instead — `syntax.Error.FuncBody`, rendered by
`Diagnostics.MissingFuncBodyOmitsTheLine`.

The mark stops at a newline, because the behavior does: `f()` followed by
a newline is `zsh:1: parse error near \n`, with the line back. Which line
it then names is a further divergence and is not modeled — zsh says 1
where the offending token is on line 2.

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

Corpus: `commands/coproc-is-one-dialect-s-keyword`, and the
name-before-a-compound rule in all three of its shapes —
`commands/coproc-names-a-compound` for a brace group,
`commands/coproc-names-a-subshell` for the compound that is easiest to
forget, and `commands/coproc-does-not-name-a-simple-command` for the
side that says the rule is a rule. The last of those runs a *function*
called `MY`, because bash's `MY: command not found` arrives from a
background job whenever that job gets to it, and a case graded on a
racing diagnostic grades nothing.
