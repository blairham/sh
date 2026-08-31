# What obi learned that this repository starts with

obi (`blairham/obi-shell`) spent a year finding these the expensive way,
mostly by maintaining a vendored bash substrate it did not design. They
are recorded here as **starting constraints**, not as things to
rediscover. Issue numbers refer to obi's tracker.

None of this is upstream's code or design; it is what our own tree
learned about the problem.

## Shims rot — fix it in the substrate

obi kept a file of "substrate gap" rewrites: constructs the interpreter
refused, rewritten in the tree between parse and run. Three entries lived
there and **all three had to be deleted**, because each rewrite was
equivalent only until the substrate learned a distinction the rewrite had
been flattening:

- `declare -F` was flattened to a function-table read, which could not
  see the `readonly -f` attribute the real builtin reports.
- A quoted heredoc delimiter was restated as a literal, which silently
  corrupted heredocs inside `source` and `eval`.
- `>|` was rewritten to `>`, which turned **the one redirect that must
  ignore `noclobber`** into the one it refuses.

The rule: when a construct is missing, implement it where it belongs.
A translation layer is a bug with a delay on it.

## Anything universal must live inside the interpreter

The reason the heredoc rewrite failed generalises: **`source` and `eval`
re-parse inside the interpreter**, so nothing applied to the tree the
shell parsed reaches them. This is the load-bearing argument for the
gate and event seams being interpreter-internal (`design.md`), and it
applies identically to dialect selection, sandbox policy and audit.

## Parser configuration is interpreter state

`set -o posix`, `shopt -s extglob` and alias definitions all change how
the *rest of the input parses*. obi's answer (#450, #619, #653, #708) is
that the interpreter derives parser options on demand and re-applies
them between statements. Two consequences:

- Aliases belong to the parser, because an alias is substituted while
  the line is read.
- `extglob` changes *parsing*, not globbing: with it off, `+(a|b)` is a
  syntax error, not a pattern that fails to match.

## Options: name them, and answer honestly

Two warts worth not reproducing:

- **obi's option table is positional** — constants index into it, so
  rows cannot be reordered. Option identity should be a name.
- **"Not implemented" needs to be a real answer, not a pretence** (#542,
  #575). obi distinguishes implemented, state-only (the bit is tracked;
  the behavior belongs to the layer above), and startup facts (login
  shell, restricted). Requests to move an unsupported option are refused
  and the shell continues — because applying such an option at startup
  instead made `-vc 'echo hi'` fail where bash succeeds (#426). **An
  honest refusal beats a fatal error and beats a lie.**

## The parser is on the keystroke path

obi re-parses the current line on **every keystroke** to syntax-highlight
it. An upstream parser panic was therefore a shell crash. A substrate
meant for interactive use must be fuzzed and must never panic on
malformed input — that is a correctness requirement, not hardening.

## Compatibility is measured, and the measurement is kept honest

obi's scoreboard rules, which carry over unchanged:

- The oracle is a **real binary**, never a belief about one.
- **Which build you compare against changes the answer** — macOS bash
  3.2 rejects things bash 5.3 accepts, so the version is part of every
  claim.
- **A passing case means "this snippet behaves identically", not "this
  feature is complete."** Corpus coverage is the ceiling on the claim.
- **A scoreboard that only goes up is marketing.** The corpus grows, so
  the percentage can fall without a regression; that is the point.
- Regressions fail CI against the published number.

## Licensing hygiene is deliberate

bash's own test suite is GPLv3, so obi **fetches it and never commits
it** — vendoring would relicense an MIT repo by accident. Same rule
here, and it generalises to every third-party corpus.

## Costs to pay up front

- **Real process groups, not goroutines.** obi's goroutine-backed jobs
  made `set -m` semantically narrower than bash's and left job control
  answers it could not give honestly. See `design.md`.
- **A startup budget enforced in CI** from the first commit, not
  recovered later.
- **Portability invariants and a Windows CI gate from day one**, even
  while native Windows support is sequenced later.
- **Deadlines on every call that leaves the process**, and an allowlisted
  environment with secret scrubbing wherever context is assembled for a
  model.
