#!/bin/sh
# The repositories TestVcsInfoAnswersWhatZshAnswers runs vcs_info in, one
# directory each under $1, every one in a state vcs_info reports: clean,
# changed, detached, part-way through a merge, a rebase, an am, a
# cherry-pick, a revert or a bisect. The dates, names and messages are
# fixed so that every hash is the same on every machine.
set -e
R=$1; rm -rf "$R"; mkdir -p "$R"; cd "$R"
printf '#!/bin/sh\n{ echo break; cat "$1"; } > "$1.new" && mv "$1.new" "$1"\n' > "$R/breakfirst"
chmod +x "$R/breakfirst"
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@e GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@e GIT_AUTHOR_DATE='2026-01-01T00:00:00Z' GIT_COMMITTER_DATE='2026-01-01T00:00:00Z'
g() { git -c init.defaultBranch=main -c commit.gpgsign=false -c advice.detachedHead=false "$@" >/dev/null 2>&1; }
mkdir none
mkdir clean; (cd clean && g init && echo a > f && g add f && g commit -m one && mkdir -p sub/deep && echo x > sub/deep/x && g add sub && g commit -m two)
mkdir unborn; (cd unborn && g init)
cp -R clean detached; (cd detached && g checkout HEAD~1)
cp -R clean tagged; (cd tagged && g tag v1 HEAD~1 && g checkout v1)
cp -R clean staged; (cd staged && echo b >> f && g add f)
cp -R clean unstaged; (cd unstaged && echo b >> f)
cp -R clean both; (cd both && echo b >> f && g add f && echo c >> f)
cp -R clean untracked; (cd untracked && echo n > new)
cp -R clean merge; (cd merge && g checkout -b other && echo o > f && g commit -am o && g checkout main && echo m > f && g commit -am m && g merge other || true)
cp -R clean rebasei; (cd rebasei && GIT_SEQUENCE_EDITOR='sed -i.bak -e 1s/^pick/edit/' g rebase -i HEAD~1 || true)
cp -R clean rebasem; (cd rebasem && g checkout -b other HEAD~1 && mkdir -p sub/deep && echo o > sub/deep/x && g add sub && g commit -m o && g rebase --merge main || true)
cp -R clean rebaseam; (cd rebaseam && g checkout -b other HEAD~1 && mkdir -p sub/deep && echo o > sub/deep/x && g add sub && g commit -m o && g rebase --apply main || true)
cp -R clean cherry; (cd cherry && g checkout -b other HEAD~1 && echo o > f && g commit -am o && g checkout main && echo m > f && g commit -am m && g cherry-pick other || true)
cp -R clean revert; (cd revert && echo z > f && g commit -am z && echo w > f && g commit -am w && g revert HEAD~1 || true)
cp -R clean bisect; (cd bisect && g bisect start && g bisect bad HEAD && g bisect good HEAD~1 || true)
cp -R clean am; (cd am && g format-patch -1 HEAD -o ../ampatch && g checkout -b amb HEAD~1 && mkdir -p sub/deep && echo q > sub/deep/x && g add sub && g commit -m c && g am ../ampatch/*.patch || true)
mkdir bare; (cd bare && g init --bare)
cp -R clean wt; (cd wt && g worktree add ../linked -b lb)
cp -R clean orphan; (cd orphan && g checkout --detach && g commit --allow-empty -m lone && g checkout main && g checkout 'HEAD@{1}')
mkdir unbornstaged; (cd unbornstaged && g init && echo a > f && g add f)
cp -R clean brk; (cd brk && GIT_SEQUENCE_EDITOR="$R/breakfirst" g rebase -i HEAD~1 || true)
cp -R clean multi; (cd multi && g checkout -b other HEAD~1 && echo 1 > a1 && g add a1 && g commit -m p1 && echo 2 > a2 && g add a2 && g commit -m p2 && mkdir -p sub/deep && echo o > sub/deep/x && g add sub && g commit -m p3 && echo 4 > a4 && g add a4 && g commit -m p4 && g rebase --merge main || true)
cp -R clean multiam; (cd multiam && g checkout -b other HEAD~1 && echo 1 > a1 && g add a1 && g commit -m p1 && echo 2 > a2 && g add a2 && g commit -m p2 && mkdir -p sub/deep && echo o > sub/deep/x && g add sub && g commit -m p3 && echo 4 > a4 && g add a4 && g commit -m p4 && g rebase --apply main || true)
cp -R clean cseq; (cd cseq && g checkout -b other HEAD~1 && echo 1 > a1 && g add a1 && g commit -m c1 && echo o > f && g commit -am c2 && echo 3 > a3 && g add a3 && g commit -m c3 && g checkout main && echo m > f && g commit -am m && g cherry-pick main..other || true)
cp -R clean multiapply; (cd multiapply && g checkout -b src HEAD~1 && echo 1 > a1 && g add a1 && g commit -m q1 && mkdir -p sub/deep && echo o > sub/deep/x && g add sub && g commit -m q2 && echo 3 > a3 && g add a3 && g commit -m q3 && g format-patch -3 HEAD -o ../mapatch && g checkout main && g am ../mapatch/*.patch || true)
