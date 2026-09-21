// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

// The repository segment: what people navigate by, and the reason a prompt
// is ever slow.
//
// The lookup comes from the caller, for the reason the clock's format
// language and the prompt character do: this package computes nothing about
// the machine it is running on, and a segment that went and found a
// repository for itself would be a segment the fidelity harness could not
// pin. What arrives here is an answer somebody else already has.
//
// It is the first segment whose answer can change while nobody is typing —
// a branch moved in another terminal — and that is not this file's business
// either. The lookup answers what it has; whoever owns it publishes, and the
// prompt is redrawn. See repl/promptasync.go.

// Repo is what a repository segment is told.
//
// Branch, or the short commit of a detached head, and whether the repository
// is in the middle of something. Deliberately not the counts: those need an
// index-versus-working-tree walk, and a prompt with nothing attached to do
// that walk shows the branch and no counts rather than waiting.
type Repo struct {
	Branch    string
	Commit    string
	Operation string
}

// Repository draws the repository the shell is standing in, or declines where
// it is not standing in one.
//
// An operation in progress is drawn beside the branch rather than left to a
// color, because it is the state a person most needs a prompt to tell them
// about and the one they can forget they are in — and a color says nothing
// at all on a terminal that is not showing them.
func Repository(status func(dir string) (Repo, bool)) Segment {
	return SegmentFunc(func(_ *Settings, ctx *Context) (Rendered, bool) {
		if status == nil {
			return Rendered{}, false
		}
		repo, ok := status(ctx.Dir)
		if !ok {
			// Most directories are not in a repository, and the segment
			// costing no space there is what an absent thing should look
			// like.
			return Rendered{}, false
		}

		name, icon, state := repo.Branch, "VCS_BRANCH", ""
		if name == "" {
			name, icon, state = repo.Commit, "VCS_DETACHED", "detached"
		}
		if name == "" {
			// A repository this could not read the head of. Naming nothing
			// is better than naming a branch that is not the one.
			return Rendered{}, false
		}
		content := name
		if repo.Operation != "" {
			content = name + " (" + repo.Operation + ")"
			state = repo.Operation
		}
		return Rendered{
			Content: content,
			Icon:    icon,
			State:   state,
			Fields: map[string]string{
				"BRANCH":    repo.Branch,
				"COMMIT":    repo.Commit,
				"OPERATION": repo.Operation,
			},
		}, true
	})
}
