# Sandboxing

How a policy is written, where it is loaded from, what it defaults to,
what a refusal looks like, and what an audit record contains.

`docs/design.md` establishes the seam: the interpreter asks a `Gate`
before every action that leaves its own memory, and reports every one to
a `Sink`. That is the mechanism. This document is the *policy* — the
shipped implementation that fills the seam, and the event schema its
consumers read.

Read `docs/design.md` first. In particular, read the paragraph that says
the boundary is drawn around the interpreter and not around the process
tree, because every limit in this document follows from it.

## What a sandbox here is, and what it is not

**It contains the shell, not the process tree.** A denied `open` stops
the shell reading a file. An allowed `exec` starts a process that makes
its own system calls, and nothing in this repository has any say over
them. `allow exec /bin/cat` is `allow read /**`, spelled less
obviously, and `allow exec /bin/sh` is `allow` everything.

That is not a defect to be fixed later by trying harder inside this
package. Containing a running child needs the operating system —
`seccomp`, Landlock, a sandbox profile, a container — and the substrate
does not own OS backends. What it owns is the decision point, and a
backend that wants one has it: a `Gate` implementation may consult the
kernel, prompt a person, or call out to a supervisor, because the
interface is `Allow(ctx, Action) Decision` and nothing constrains how the
answer is reached.

So the honest claim is narrow and worth stating in the words a reader
will need: **this refuses what the shell itself does, and it is the only
thing that can, because `eval` walks around anything applied from
outside.** A policy that also has to contain children composes this with
an OS sandbox; it does not replace it with one.

## The policy file

### Format: a line per rule

A policy is a line-oriented list of directives. This was the first
decision and JSON was the obvious alternative, since `encoding/json` is
in the standard library and this module has no runtime dependencies at
all. Three things decided it the other way.

**A rule is reviewed in a diff.** A policy is a security artifact, and
the thing that actually stops a bad one is a second person reading the
change. One rule per line makes a changed rule exactly one changed line.
Nested JSON makes the same edit touch brackets and indentation, and the
reviewer has to reconstruct which rule moved.

**Order is visible and comments are possible.** A policy accumulates
reasons — *this path is allowed because the build writes there* — and
the format has to hold them next to the rule. JSON cannot carry a
comment without inventing a JSON dialect, which is a worse outcome than
inventing a small format honestly.

**Paths are the whole content, and JSON escapes them.** Every value in
this file is a path or a glob. A format whose quoting rules turn `\` and
`"` into escapes is a format in which the common case is misread.

The cost is a parser we own, and it is deliberately tiny: no quoting, no
continuation, no nesting, no includes. The grammar is below in full and
it fits in a paragraph.

### Grammar

    policy   := line*
    line     := blank | comment | directive
    comment  := ws* '#' ...
    directive := version | default | rule
    version  := 'version' ws '1'
    default  := 'default' ws decision [ws selector]
    rule     := decision ws selector [ws pattern]
    decision := 'allow' | 'deny'
    selector := 'exec' | 'read' | 'write' | 'open' | 'stat' | 'list'
              | 'path' | 'signal'
    pattern  := rest of the line, trailing whitespace trimmed

The pattern is *the rest of the line* rather than a whitespace-delimited
field, so a path containing spaces is written as it stands. A path
containing a newline cannot be written; name a containing directory
instead. That is the one expressible path this format cannot hold, and
it is recorded here rather than left to be discovered.

`version 1` is **required** and must be the first directive. A parser
that met a file it does not understand has to refuse it rather than read
what it recognizes, because a policy half-understood is a policy that
allows what it was written to refuse. The same requirement is what lets
a later format change be detected instead of misread.

Every parse failure is fatal and names the line. There is no recovery
and no "unknown directive ignored": an unrecognized word is a typo in a
security file, and continuing past it would silently drop a rule.

### Selectors

A selector names a set of the action kinds `interp` defines. The
vocabulary is `interp`'s and stays `interp`'s — this is the same rule
`internal/boundary` follows — and a selector is only a convenient name
for a subset of it.

| selector | action kinds |
| --- | --- |
| `exec`   | exec |
| `read`   | open for reading, stat, read-dir |
| `write`  | open for writing |
| `open`   | open, either direction |
| `stat`   | stat |
| `list`   | read-dir |
| `path`   | exec, open either direction, stat, read-dir |
| `signal` | signal |

`read` covers the probes because reading a directory and asking whether
a file is there are both reading. Someone who writes `allow read
/srv/**` and then finds `[ -f /srv/x ]` false has been given a policy
that does not mean what it says.

`path` is every kind that carries a path, and deliberately not every
kind: a signal names a process rather than a file, so no pattern can
select one and `signal` has to be named on its own. Writing a pattern
after `signal` is a parse error rather than a silently ignored word.

`inherit` is not a selector. `ActionInherit` is recorded and never
gated, for the reason written on it in `interp/seams.go`, and naming it
in a policy is a parse error that says so — better than accepting a rule
that would never be consulted.

### Patterns

Patterns are globs over path components:

- `*` matches within one component and never crosses `/`.
- `?` matches one character that is not `/`.
- `[abc]`, `[a-z]`, `[^abc]` match one character, as `path.Match` reads
  them.
- `**` is a whole component and matches **zero or more** components, so
  `/srv/**` matches `/srv` itself and everything beneath it.

**A pattern must be absolute.** A relative pattern would mean "relative
to a working directory", and the policy does not own one — the shell's
directory moves with `cd` and is not the process's. A relative pattern
is a parse error.

Correspondingly, **a relative path in an action matches no pattern** and
falls to the default. In practice the interpreter resolves paths against
the Runner's directory before it asks — a redirect joins `r.Dir`, a glob
walks from `r.workDir()`, a PATH search resolves each candidate — so
what reaches the gate is already absolute. A relative one arriving here
means something upstream did not resolve, and denying it under a
default-deny policy is the fail-closed answer.

Incoming paths are cleaned lexically before matching, so `/srv/../etc/x`
is matched as `/etc/x` and a `..` cannot be used to walk out of a
pattern.

**Symlinks defeat a name-based rule, and this is a real limit.** The
gate matches names; it does not resolve links, and it could not do so
honestly — resolving would mean the policy performing filesystem reads
of its own, outside the boundary, and the answer would still be a
time-of-check race. So `deny read /etc/**` does not stop
`cat /tmp/link` where `/tmp/link` points into `/etc`, unless `/tmp` was
not allowed in the first place. The mitigation is the posture: allow a
subtree you control rather than deny one you do not. The general
solution is an OS backend that enforces on the resolved inode, which is
above this layer.

`cd -P` and `pwd -P` are the exception that shows the shape, and it is
already handled: those walk each component through the gate rather than
handing a path to the standard library, so the policy sees every link on
the way. See `interp/physicalpath.go`.

### Evaluation: deny wins, and order does not matter

A rule set is evaluated as:

1. if any `deny` rule matches the action, deny;
2. otherwise if any `allow` rule matches, allow;
3. otherwise the default for that action's kind.

Order-independence is the point, and the argument is composition.
Concatenating two policies has to yield a policy that allows only what
*both* allow — otherwise a policy assembled from an operator's file and
an embedder's baseline means something that depends on which was read
first, and nobody can reason about the result. Deny-overrides is the
only precedence rule with that property.

First-match-wins, the firewall convention, is the alternative and was
rejected for exactly that: it makes concatenation order-dependent, and
it makes a rule's meaning depend on lines above it that a reviewer of
the diff cannot see.

The consequence to know is that a `deny` cannot be narrowed by a later
`allow`. `deny read /srv/**` plus `allow read /srv/public/**` denies
`/srv/public` too. Carving a hole is done the other way — a broad
`allow` with narrow `deny`s inside it — which reads correctly under
deny-overrides and is the shape a default-deny policy wants anyway.

### Default posture: deny, uniformly, for every kind

**The default default is `deny`, for every action kind, and a policy
that says nothing about a path refuses it.**

The obvious reason is that a sandbox whose default is allow is not a
sandbox. The less obvious reason is the one that settles the *per-kind*
question the issue asks, and it is worth spelling out, because a
per-kind mixture looks reasonable and is not.

Suppose `open` defaults to deny and `stat` defaults to allow, on the
theory that a probe reveals less than a read. Then `[ -f /home/x/.ssh/id_rsa ]`
answers truthfully, and the script has learned that the file exists
while being unable to read it. Worse, it has learned it *by asking the
policy* — the difference between "denied" and "not there" is the whole
of the information, and the deny shape below exists precisely to keep
that difference invisible. A per-kind default that allows probes turns
the policy into an oracle for the filesystem it is hiding.

So the posture is uniform, and the file can still express a mixture:
`default allow stat` sets the default for one kind. It is supported
because an operator debugging a policy genuinely wants it, and because
refusing to represent a thing does not stop anyone doing it — they will
write `allow stat /**` instead, which is the same policy spelled longer.
It is documented as discouraged, with the leak named, rather than
prohibited.

`default allow` with no selector is the other supported shape and it is
what turns this into the existing `-deny` debug flag: an allow-all
policy with `deny` rules carving holes. That is a way to *watch* a gate
rather than a sandbox, and `docs/design.md` already draws that
distinction for `-deny`. It is the same distinction here.

### Where a policy comes from

Two routes, and the split is deliberate.

**The embedder API is the primitive.** A `Policy` is a value. A program
embedding a Runner builds one in Go, or parses one from any
`io.Reader`, and assigns it to `driver.Shell.Gate` — or writes its own
`Gate` and never touches this package at all. The seam is the interface;
this package is one implementation of it.

**The invocation flag is the reachability route.** `cmd/sh` grows
`-policy FILE`, alongside `-deny` and `-trace-events`, because a seam
nothing reaches is a seam nothing grades. Until a shipped binary can be
handed a policy, the conformance harness and the wild sweep run ungated
and a hole in the boundary looks exactly like a shell that works. That
is the same argument that put `-deny` there, and it is why this is not
"embedder API only".

It is `cmd/sh`'s own flag rather than a `driver` one. `driver` is the
shared front end for binaries that claim to *be* bash or zsh, and no
real shell has a `-policy`; adding it there would make `./bash -policy`
accept a flag bash rejects, which is the rule `AGENTS.md` states about
which flags belong where.

`-audit FILE` is the other half of the same route, and writes the event
schema below to a file — or to standard error for a lone `-`. Appended
rather than truncated, because a trail that erases the previous run on
the next one is not one, and written straight through rather than
buffered, so a shell that dies mid-script has still recorded everything
up to the action that killed it.

**More than one gate composes as an intersection.** `-policy` and
`-deny` together consult both, and any refusal refuses. That is the same
rule the file uses between its own lines, and it is what makes adding a
`-deny` to an existing policy a narrowing rather than a way around the
file. Sinks compose the other way and are fanned out to: watching a
trace by eye and keeping an audit file are different jobs.

**A policy is never discovered.** Not from an environment variable, not
from a dotfile searched for in the working directory or the home
directory, not from a path a shell variable names. A policy that can be
named by `$SH_POLICY` can be replaced by anything that can set the
environment — including the sandboxed script itself, on the way to
invoking a nested shell. The policy comes from the command line or from
the embedder, and from nowhere else. This is a rule about the *source*
of the policy and not about its content, so it cannot be relaxed by a
policy.

### The apparatus is outside the boundary

The policy file and the audit stream are opened by the front end,
before any Runner exists, and **they do not pass the gate**.

This is an exemption and it needs its reason written down, because
`docs/design.md` says the test of which side a front-end access falls on
is who chose the path, and both of these are chosen at the invocation —
which is the same place a script operand comes from, and a script
operand *is* inside the boundary.

The distinction is subject versus apparatus. The script is what the
policy is about. The policy file and the audit log are the policy's own
machinery, and gating them would let a policy refuse to be read or
refuse to record what it did — a boundary that can switch off its own
enforcement record is not a boundary. They are also read before there is
a gate to ask.

That exemption is exactly two paths, both named on the command line by
whoever installed the policy, and it is listed here so that silence is
never the record.

## What a refusal looks like

The rule is one sentence:

> **A denial is opaque to the script and transparent to the audit
> sink.**

Nothing in this initiative changes the shapes already established, and
this section records why they are what they are, because the issue asks
whether a denial should instead be policy-visible everywhere.

| action | the script sees | why |
| --- | --- | --- |
| stat, read-dir | ENOENT — the path is not there | a refusal that identifies itself is an oracle for what is hidden |
| signal | EPERM — a process this one may not signal | the errno the kernel gives for the same refusal |
| open | the failure any unopenable redirect gives | the command must not run without the stream it asked for |
| exec | `exec: refused: /path`, status 126 | a command that visibly did not run cannot honestly report otherwise |

The first two are the important ones and they are ENOENT-shaped and
EPERM-shaped on purpose. If a denied stat said "refused", then
`[ -f /secret/x ]` would distinguish *hidden* from *absent*, and a script
could enumerate the policy's deny list and, through it, the filesystem it
covers. That is the same reason a login prompt does not say which of the
user and the password was wrong.

The last two are visible, and the difference is deliberate rather than
an inconsistency:

**Existence is never leaked. Policy shape is leaked only where a command
visibly did not run.** A refused `exec` tells the script that the policy
said no; it does not tell it whether the program is there. A refused
redirect reports as an unopenable redirect, which is what it is. In both
cases the script already knows something did not happen — it has a
failing status and no output — so a wrong diagnostic would buy no
secrecy and would cost every operator debugging a policy the ability to
tell a refusal from a bug.

The complete picture is in the audit stream, which carries the kind, the
path, and `EventDenied` rather than an error. The operator sees
everything; the script sees a failure with no attribution. That
separation is the whole design, and it is why "policy-visible
everywhere" was rejected.

One caveat worth stating because it is a genuine residual: the *timing*
and the *pattern* of refusals is observable to a script that is looking,
and a determined script can map an allow list by trying things. The
defense against that is the audit trail — a script probing its own
boundary produces a very distinctive event stream — and not an attempt
to make refusals unobservable, which is not achievable.

## Policy and dialects do not interact

**The gate is dialect-blind, and anything that makes a policy answer
depend on the dialect is a design error.** This is the strongest
statement in this document and it is enforced in three ways.

**By type.** `Gate.Allow` receives a `context.Context` and an
`interp.Action`. An `Action` carries a kind, a path, an argument vector,
a write flag, a PID and a signal. There is no dialect in it, no
semantics vector, and no route to one. A gate cannot ask which shell it
is inside because nothing tells it.

**By the import graph.** `internal/policy` imports `interp` and the
standard library. It imports nothing under `dialect/`, and a test
asserts that over the real dependency list rather than by inspection.
This is the substrate rule from `AGENTS.md` — nothing under the core
names a shell — applied to a package that sits beside it.

**By behavior.** A test runs the same script under the same policy in
every dialect and asserts the same denials, in the same order. This is
the assertion that would actually fail if someone reached for the
semantics vector inside a rule, and it is cheap because `cmd/sh` already
takes `-dialect`.

The confusion worth heading off: dialects *do* change which actions
happen. A dialect with a prelude sources it, which opens files; `zsh`
and `bash` split words differently, so a glob may expand to different
paths; one dialect's `kill` builtin may take an argument another's does
not. Those are different **actions**, and the policy answers each of
them the same way it would answer any other. What must never happen is
the same action getting two answers because the shell was in a different
mode. The behavioral test above pins the second and tolerates the first
by using a script the dialects agree about.

## The event schema

The `Sink` half. This is a **shared contract**, not a sandbox feature:
the audit log reads it, an agent protocol's session updates read it, an
assistant assembling context reads it. It is designed here because the
sandbox needed it first, and it is deliberately neutral about all of
them — nothing in it names a protocol, a front end, or a dialect.

It lives in `internal/event`, per the promotion rule, and it is a
*serialization* of `interp.Event` rather than a second event type.
`interp` owns what an event is; this owns how one is written down.

### Shape

One JSON object per line — JSON Lines. A stream is appendable, tailable,
greppable, and survives truncation with the loss of one record. A single
JSON array would have to be closed to be valid, which a log that is
being written cannot be.

    {"v":1,"seq":4,"time":"2026-09-05T11:02:03.000000001Z",
     "session":"0VJ8QG7K2M4N6P8R0S2T4V","actionId":"12",
     "event":"denied","action":"open","path":"/etc/shadow",
     "write":false,"line":3,"file":"script.sh"}

(wrapped here for width; a record is one line)

| field | type | present |
| --- | --- | --- |
| `v` | integer | always — the schema version, `1` |
| `seq` | integer | always — 1, 2, 3, … within one stream |
| `time` | RFC 3339 with nanoseconds | always |
| `session` | string | when the run has an identity |
| `actionId` | string | when the action has an identity |
| `event` | string | always — `command-start`, `command-end`, `denied`, `error`, `access` |
| `action` | string | always — `exec`, `open`, `stat`, `read-dir`, `signal`, `inherit` |
| `path` | string | when the action has one |
| `args` | array of string | for an exec |
| `write` | bool | for an open |
| `pid` | integer | for a signal, always, including 0 |
| `signal` | integer | for a signal, always, including 0 — 0 is `kill -0` |
| `status` | integer | for `command-end`, always, including 0 |
| `error` | string | when the event carries a failure |
| `line` | integer | always — the source line a diagnostic would name |
| `file` | string | when the line came from a file |

`event` and `action` are the names `interp`'s `String()` methods already
produce, so the wire names and the Go names cannot drift.

### The stability rules

These are the contract, and they are what a consumer may rely on.

1. **`v` identifies the schema.** It changes only for a change that
   would break a conforming consumer. A consumer must check it and
   refuse a version it does not know.
2. **A field name means one thing forever.** A name is never reused for
   a different meaning; a field that becomes wrong is abandoned, not
   repurposed.
3. **New fields may be added within a version.** A consumer must ignore
   fields it does not recognize. This is what makes additive change
   possible without a version bump, and a consumer that fails on an
   unknown field has broken the contract rather than found a bug.
4. **New `event` and `action` values may be added within a version.** A
   consumer must not fail on a name it does not know; the useful default
   is to record it and carry on. `ActionSignal` arriving after the other
   five is the worked example of why this rule is needed — see
   `docs/design.md`.
5. **An absent field means the zero value.** Fields are omitted rather
   than written as `null`, except where the table above says *always*,
   and those are always written even when zero because zero is
   meaningful there: status 0 is success, signal 0 is the existence
   probe.

### `seq` and `time` are the encoder's, not the interpreter's

Neither is on `interp.Event`, and both are added when a record is
written. That needs justification in both directions.

`seq` gives a stream a total order of *emission*. It is not a causal
order and must not be read as one: a `Sink` is called from every
goroutine a shell has, so a background job's events interleave with the
foreground's, and two records whose `seq` differ by one may be unrelated.
What `seq` is for is replay and gap detection — a consumer that has
records 1..40 and then 42 knows it lost one.

`time` is stamped at encode time rather than at the action. The gap is
the time between the interpreter emitting and the encoder writing, and
because a `Sink` is called synchronously on the emitting goroutine, that
is a function call. It is honest to within that, and stating the
mechanism is better than implying a precision the record does not have.
The clock is injectable so tests are deterministic.

Neither was added to `interp.Event` itself. An event is a description of
what happened; a sequence number is a property of a stream, and a
timestamp is a property of an observation. Putting them on the event
would make every consumer pay for a clock read whether or not it wanted
one, and would make `interp`'s tests depend on time.

### `session` and `actionId` are identity, and neither is `seq`

These are the first change the schema took after it landed, and they
went in as *added fields* rather than a version bump — which is rule 3
being used rather than described. A consumer written against the
original fifteen fields reads a record carrying these exactly as it did
before, and a test in `internal/event` decodes into that original field
set to keep it true.

They were added because two consumers arrived at the same missing field
from opposite directions and neither could work around it. A front end
that owns a command's outer boundary knows when a command started and
ended but cannot say **which events belong to which command**, so its
record and this stream describe the same session and cannot be joined.
An agent protocol has the same problem one level down: it opens a tool
call when a command starts and closes it when the command ends, and
`EventCommandStart` and `EventCommandEnd` were matched by *ordering* —
which concurrency breaks, because a background job and each half of a
pipeline emit from their own goroutines. Matching on a fingerprint of
(kind, path, args, line, file) closes the wrong record whenever a script
runs the same command twice at once.

Doing it per consumer was the expensive path and the reason this is one
field rather than two: two private id schemes that do not agree are
worse than no id at all, because they look joinable and are not.

**`actionId` is on `interp.Action`,** so the id a `Gate` is consulted
about is the id the events for that action carry. That is the whole
promise — a permission request and the records of what was permitted are
provably the same action — and it is why the field could not live only
on the wire the way `seq` does.

**It is a counter, and `session` is what makes it unique.** A `PATH`
search stats a candidate in every directory `PATH` names and a glob
stats every entry it descends past, so this is one of the hottest things
the interpreter does; sixteen bytes of randomness per stat would be paid
by every shell that had merely asked to watch itself. The counter is
shared across a subshell rather than copied, because a cloned Runner
that numbered from its own copy would hand two different actions the
same id. The pair `(session, actionId)` is what is unique everywhere.

**`actionId` is not `seq` and the two must never be conflated.** `seq`
orders *emission* within one stream and differs on every record;
`actionId` names one action and is the *same* on every record about it.
A consumer that joined on `seq` would pair a command's start with
whatever happened to be emitted next.

**`session` comes from the front end, not from `interp`.** A Runner does
not invent identity, for the same reason it does not read a clock on an
event's behalf: what needs the identity is the thing that has more than
one account of a run to line up, and that is the front end. `driver`
makes one per invocation, in `withDefaults`, which is the single point
every route passes through before anything is built — so the Runner and
the prompt cannot end up with different answers. A caller that already
has a notion of a session, such as an agent protocol with several shells
behind one connection, sets `driver.Shell.Session` and it is left alone.
An empty `session` is a run nobody gave an identity, and the field is
then omitted rather than filled with a placeholder: a record that cannot
be joined should say so.

The id generator itself is `event.NewID`, here rather than beside any
one consumer, because one generator is the whole point. It is
`math/rand/v2` and deliberately not `crypto/rand`, and the reason is
specific to a shell: what is wanted is uniqueness rather than
unpredictability, and measured on macOS the first `crypto/rand.Read` in
a process permanently opens a descriptor. In a shell that number is not
an implementation detail — descriptor 3 is the first one a script parks
with `exec 3>f`, and the process's table is what a replacement inherits.

**The front end's own accesses are identified too, and mint their ids
rather than counting.** `internal/boundary` is the third emitter — the
script operand, `$ENV`, `HISTFILE`, the block store's own index — and
its records are the ones most likely to be joined, since one of the
files it opens *is* the record of what the shell ran. It carries the
same `session`, and stamps each access with an `event.NewID`. The
opposite choice from `interp`, for the opposite reason: there a counter
is forced by the hot path, and out here a run makes a handful of these,
so an id needing no coordination is what lets the several places that
build a `Boundary` go on building one independently. A shared counter
would have to be threaded through all of them and the first that forgot
would issue a duplicate.

## Consequences a user meets immediately

These follow from the decisions above and are listed so they are not
discovered as surprises.

**A default-deny policy must allow the script itself.** The script
operand passes the gate — `internal/boundary` put it there, on purpose —
so `sh -policy p script.sh` needs a rule that permits reading
`script.sh`. That is correct: the shell reading a file is an access, and
the person who wrote the policy is the person who named the script.

**A default-deny policy must allow the interpreter's own reachable
paths.** A dialect with a prelude sources it from a string rather than a
file, so that costs nothing, but `$ENV` and `HISTFILE` are real opens
and are inside the boundary.

**Allowing an interpreter allows everything.** `allow exec /bin/sh`,
`/usr/bin/python3`, `/bin/busybox`, and anything else that runs a
program of its own, ends the usefulness of the rest of the policy. This
is the sharpest footgun in the format and there is no way for the parser
to detect it — an allowlisted binary is a name, and what a name can do is
not knowable from here.

## What is deliberately not here

- **Network rules.** The gate has no network action kind, so a policy
  cannot express one. Adding one is an `interp` change first — an
  `ActionConnect` and the places that would raise it — and this format
  gains a selector when that exists, not before. A format that accepted
  `allow net` today would be accepting a rule nothing consults, which is
  the exact failure mode `docs/design.md` warns about.
- **Resource limits.** `umask` and the resource limits are already hooks
  that a binary fills in, and `docs/design.md` explains why they are
  outside the boundary: they name no path and perform no access.
- **Per-command policy.** Rules match actions, not the command that
  caused them. "Let `git` read `/srv` but not `curl`" is not
  expressible, because by the time an exec has happened the child is
  outside the boundary entirely and the shell's own later accesses have
  no command to attribute them to.
- **Prompting.** `Decision` has `Allow` and `Deny` and no `Ask`. A
  question needs somewhere to ask it, which a library does not have; an
  embedder that has one writes a `Gate` that blocks on its own UI, which
  the interface already permits. That is the agent-protocol case and it
  belongs to whatever owns the conversation.

## Open, and flagged for the maintainer

Two decisions here are defensible defaults chosen in the absence of an
answer, and both are cheap to reverse:

1. **`default allow <selector>` is supported at all.** The uniform
   posture is argued above and the per-kind override is documented as
   discouraged. The alternative is to refuse it and make people write
   `allow stat /**`, which is the same policy with less signal in it.
   Supported, with the leak named.
2. **`read` includes `stat` and `read-dir`.** The alternative is three
   separate selectors and no grouping, which is more explicit and more
   verbose. The grouping was chosen because the ungrouped form makes
   `[ -f x ]` fail under a policy that plainly meant to allow it, which
   is the mistake people actually make.
