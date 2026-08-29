# CLAUDE.md

@AGENTS.md

<!--
AGENTS.md is the cross-tool source of truth and is imported above.
Durable guidance goes there so Cursor, Codex and Copilot see it too.
Only Claude-specific notes — things that live in .claude/ — belong here.
-->

## Claude Code-specific notes

- **`CLEANROOM.md` governs this repository.** Never read another shell
  implementation's source while writing code here. If a task seems to
  require it, stop and use an oracle run instead — see `docs/spec/oracle.md`.
- The permission allowlist is in `.claude/settings.json`; the parent
  tree's allowlist applies underneath it.
- Commits carry no AI-attribution trailers, per the parent tree's rules.
