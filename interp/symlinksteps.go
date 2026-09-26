// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io/fs"
	"path/filepath"
)

// SymlinkSteps is where a path arrives, one symlink at a time.
//
// It answers the question a shell asks when it is told to report what a
// command *really* is: not only the end of the chain, which physicalPath
// already gives, but every path on the way to it. One dialect's `whence -s`
// wants the last of these and its `whence -S` wants all of them, and both are
// the same walk read to different depths — see dialect/zsh/whence.go, which is
// where the measurement lives.
//
// The slice does **not** hold the path it was given, so an empty result means
// "nothing here is a link" and is the one answer a caller has to distinguish:
// `-s` on a command that is a real file prints the path with no arrow after
// it, and one that is a link prints the arrow.
//
// Each step is the path with its **first** unresolved symlink component
// replaced, which is what makes the list a walk rather than a set of answers.
// Measured on zsh 5.9.2, 2026-09-26, with `/tmp` itself a link to
// `/private/tmp`:
//
//	whence -S myls   /tmp/…/myls -> /private/tmp/…/myls -> /bin/ls
//
// so a link in a *directory* component is a step of its own and comes before
// the link the last component is. A walk that resolved only the final
// component would have printed one arrow there and agreed with the reference
// on every case where the directories hold no links, which is most of them.
//
// Through r.lstat and r.readLink rather than the os package, for the reason
// physicalPath sets out: a policy that hides a subtree must hide it from this
// too, and a hidden component reads as "not a link" rather than as an error.
// A relative path is resolved against the Runner's own directory, never the
// process's, and one that cannot be made absolute has no steps.
func (r *Runner) SymlinkSteps(path string) []string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.workDir(), path)
	}
	if !filepath.IsAbs(path) {
		return nil
	}
	var steps []string
	for hops := 0; hops < maxSymlinkHops; hops++ {
		next, ok := r.oneSymlinkStep(path)
		if !ok {
			return steps
		}
		steps = append(steps, next)
		path = next
	}
	return steps
}

// oneSymlinkStep replaces the first symlink component of an absolute path,
// and reports whether there was one. The tail after that component is carried
// over unchanged, because it has not been looked at yet.
func (r *Runner) oneSymlinkStep(path string) (string, bool) {
	vol := filepath.VolumeName(path)
	comps := splitPathComponents(path[len(vol):])
	at := vol
	for i, comp := range comps {
		candidate := at + string(filepath.Separator) + comp
		info, err := r.lstat(candidate)
		if err != nil {
			return "", false
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			at = candidate
			continue
		}
		target, err := r.readLink(candidate)
		if err != nil {
			return "", false
		}
		if !filepath.IsAbs(target) {
			// A relative target is read from where its link sits, which is
			// the directory the walk has already resolved its way into.
			target = at + string(filepath.Separator) + target
		}
		rest := comps[i+1:]
		return filepath.Join(append([]string{target}, rest...)...), true
	}
	return "", false
}
