# Holding a coding agent to a policy

This example runs Claude Code as an agent **inside** this shell over the
Agent Client Protocol, with `sh -acp-connect`, and holds it to a policy file.
The policy enforces two rules that are usually only written down as requests:

- **Don't run the linter by hand.** The commit hook and CI already run it, and
  each run started by hand costs gigabytes of memory.
- **Don't write outside your own working directory**, and in particular not into
  another agent's worktree.

```sh
./run.sh                    # uses `sh` from PATH
SH=/path/to/sh ./run.sh     # or name the binary
```

It needs `npx` and a logged-in Claude Code, and the agent's one turn uses your
Claude account. It also needs a build with `-acp-prompt`, which was added after
0.0.36.

## What it does

1. Fills in `agents.policy.tmpl` with your `$HOME` and the path the shell
   resolves for `golangci-lint`. If none is installed, a stand-in is put first
   on `PATH`, so a refusal is the policy's work and not `command not found`.
2. **Checks the policy before spending a turn on it.** Under a plain `-c`,
   both refusals have to fire and a write in the working directory has to
   succeed. A policy that refused nothing would otherwise look like an agent
   that obeyed.
3. Asks the agent, in one turn, to run four commands with its Bash tool, under
   `-policy agents.policy -audit audit.jsonl -acp-allow`.
4. Prints the policy's decisions from the audit log, and fails if the refused
   write happened anyway.

`-acp-allow` answers yes to every permission request the agent makes. The
refusals still happen because a policy refusal is final: nobody is asked about
an action the policy has already refused.

## What a run looks like

Recorded 2026-10-18 with `run.sh` against a build of `main` at `9fc8c088c`,
`@zed-industries/claude-code-acp` 0.16.2 and macOS. It took one turn and
`sh` exited 0 by itself. Paths are shortened:

| The agent ran | Result |
| --- | --- |
| `golangci-lint --version` | **refused**: exit 126, `exec: refused: …/golangci-lint`, and the linter never started |
| `echo agent > ~/sh-agent-policy-probe.txt` | **refused**: exit 1, `open: refused: …`, and the file was never created |
| `echo agent > ./inside.txt && cat ./inside.txt` | allowed, and printed `agent` |
| `sleep 777 &` | started, then ended with status 137 when the agent released its terminal, so nothing was left running |

And the decisions from `audit.jsonl`:

```
{"event":"denied","action":"exec","path":"…/golangci-lint","args":["golangci-lint","--version"]}
{"event":"denied","action":"open","path":"~/sh-agent-policy-probe.txt","write":true}
{"event":"command-start","action":"exec","path":"/bin/cat","args":["cat","./inside.txt"]}
{"event":"command-end","action":"exec","path":"/bin/cat","args":["cat","./inside.txt"],"status":0}
{"event":"command-start","action":"exec","path":"/bin/sleep","args":["sleep","777"]}
{"event":"command-end","action":"exec","path":"/bin/sleep","args":["sleep","777"],"status":137}
```

## Why it works

Under `-acp-connect` this shell is the agent's **client**. Claude Code asks
the client to run each Bash command (`terminal/create`), and the shell
interprets the command line itself. So every exec and open in it reaches the
gate, including the ones inside `eval`, `$(…)`, a function or a subshell. See
`docs/design/acp.md`.

## What it does not cover

- **It is not a sandbox.** The boundary is around the interpreter. An allowed
  program is outside it once it starts. Containment is
  `docs/design/sandboxing.md`.
- **A policy reaches what the agent asks this shell for.** Claude Code routes
  its commands here; its built-in file edit tools write directly, and nothing
  gates those. An agent that runs commands in its own process, as Codex does
  today, passes no gate at all. After each turn, `-acp-connect` reports how
  many commands the agent announced and how many it asked this shell to run.
- **An exec rule names a program, not its arguments.** This policy can refuse
  the linter outright, but not `make check` while still allowing `make build`.
- **Rules are deny-overrides**, so a working directory can't be carved out of
  a denied subtree. `run.sh` works in a temporary directory outside `$HOME`
  for that reason.
