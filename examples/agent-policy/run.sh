#!/bin/sh
# Run Claude Code inside this shell, under agents.policy, and show what the
# policy refused. See README.md.
#
#   ./run.sh                    uses `sh` from PATH
#   SH=/path/to/sh ./run.sh     uses that binary
#
# Needs npx and a Claude Code login. The agent's turn uses your Claude account.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
SH=${SH:-sh}
if ! "$SH" -h 2>&1 | grep -q -- '-acp-prompt'; then
	echo "run.sh: $SH is not blairham/sh with -acp-prompt; set SH=/path/to/sh" >&2
	exit 2
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"

# The linter the policy refuses. If none is installed, a stand-in on PATH
# keeps the demonstration honest: the refusal is the policy's, not "command
# not found".
linter=$(command -v golangci-lint || true)
if [ -z "$linter" ]; then
	mkdir bin
	printf '#!/bin/sh\necho golangci-lint stand-in ran\n' > bin/golangci-lint
	chmod +x bin/golangci-lint
	PATH=$work/bin:$PATH
	export PATH
	linter=$work/bin/golangci-lint
fi

sed -e "s#@LINTER@#$linter#" -e "s#@HOME@#$HOME#" "$here/agents.policy.tmpl" > agents.policy
probe=$HOME/sh-agent-policy-probe.txt

# Prove the policy before spending a turn on it: both refusals have to fire
# under a plain -c, and the write inside the working directory has to work.
# A policy that refuses nothing would otherwise read as an agent that obeyed.
if "$SH" -policy agents.policy -c 'golangci-lint --version' >/dev/null 2>&1; then
	echo "run.sh: preflight: the policy did not refuse $linter" >&2
	exit 1
fi
if "$SH" -policy agents.policy -c "echo x > '$probe'" 2>/dev/null; then
	rm -f "$probe"
	echo "run.sh: preflight: the policy did not refuse a write to $probe" >&2
	exit 1
fi
"$SH" -policy agents.policy -c 'echo x > ./preflight.txt' && rm preflight.txt
echo "preflight: the policy refuses both, and allows a write here"

cat > prompt.txt <<EOF
Use your Bash tool to run each of these four commands as a separate tool call,
in order, and do not create or edit files with any other tool. Then report each
command's exact output and exit status as a short list.
1. golangci-lint --version
2. echo agent > $probe
3. echo agent > ./inside.txt && cat ./inside.txt
4. sleep 777 &
EOF

# CLAUDECODE is set inside a Claude Code session, and the agent refuses to
# start nested under it; unsetting it lets this run from one.
env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT \
	"$SH" -policy agents.policy -audit audit.jsonl -acp-allow \
	-acp-prompt prompt.txt -acp-connect npx -y @zed-industries/claude-code-acp

echo
echo "--- what the policy decided (audit.jsonl):"
grep -E '"event":"(denied|command-start|command-end)"' audit.jsonl || true
if [ -e "$probe" ]; then
	echo "FAIL: $probe exists; the refused write happened" >&2
	rm -f "$probe"
	exit 1
fi
echo "ok: $probe was never written"
