# Where a comment is

The parser discards comments — nothing that *runs* a script needs them
— and `cmd/shfmt` may not, because a formatter that loses a comment has
lost source. So the rules for where a comment lexically begins and ends
are written down here, measured, and `internal/fmt/comments` is
implemented from this page alone.

The recovery works by subtracting node extents from the source, which
needs no lexer support: see the last section for why that is sufficient
rather than merely convenient.

## The construct

A `#` opens a comment when it stands where a new token could begin —
after a blank, a newline, or an operator. The comment runs up to, and
not through, the next newline. POSIX XCU 2.3 (Token Recognition) states
it as: the `#` and everything after it up to but excluding the newline
are discarded.

## The core answer

Five rules, each measured below:

1. `#` mid-word is a literal. `echo a#b` prints `a#b`.
2. `#` at token start opens a comment. `echo a #b` prints `a`.
3. Quoting defeats it everywhere: `"a #b"` is one word. A `#` inside
   any quoted span is never a comment.
4. A comment ends at the newline, and the newline is not part of it. A
   backslash before that newline is inside the comment and continues
   nothing: the next line is a new command.
5. Here-document bodies take no comments: every line up to the
   delimiter is body, `#` included. `$#` and `${#var}` are parameter
   forms, not comments.

## Measured behavior

Probes run 2026-09-07 on this machine against dash, bash 5.3.15,
bash 3.2.57 (`/bin/bash`), ksh93u+ 2012-08-01, and zsh 5.9.2. All five
agree on every probe — there is no dialect split to record:

    echo a#b        → a#b        (all five)
    echo a #b       → a          (all five)
    echo "a #b"     → a #b       (all five)
    set -- x y; echo $#          → 2       (all five)
    v=hello; echo ${#v}          → 5       (all five)
    echo one # c \  <newline> echo two     → one, two (all five;
                                  the backslash extended nothing)
    cat <<EOF … line # not a comment … EOF → body intact (all five)

zsh's `INTERACTIVE_COMMENTS` option governs interactive input only; a
zsh *script* recognizes comments unconditionally, as measured above.
The formatter reads scripts, so the option does not reach it.

## What the recovery may therefore assume

Given a parse tree whose words and here-document bodies carry exact
source extents, every comment in the file is exactly: a `#` in the
source that lies **outside every word span and every heredoc span**,
extended to the end of its line. Rules 1, 3 and 5 are what make the
span test sufficient — a `#` inside a word, a quoted span, or a heredoc
body is inside a node extent and is excluded; rule 2 is why everything
left over is a comment; rule 4 is where it stops.
