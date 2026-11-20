package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Conflicts is the error of an operation that stopped on conflicts: not a
// failure, the user resolves them.
type Conflicts struct{ Files int }

func (c *Conflicts) Error() string { return "conflicts" }

// conflicted turns err into Conflicts when the working tree now has files in
// conflict: git merge, rebase, pull, cherry-pick, revert and stash apply all
// exit with an error then.
func (r *Repo) conflicted(err error) error {
	if err == nil {
		return nil
	}
	out, e := r.Run("diff", "--name-only", "--diff-filter=U")
	if e == nil {
		if n := len(Lines(out)); n > 0 {
			return &Conflicts{Files: n}
		}
	}
	return err
}

// Checkout checks out a branch, a tag or a commit; detach leaves HEAD on the
// commit itself.
func (r *Repo) Checkout(ref string, detach bool) error {
	if detach {
		_, err := r.Run("checkout", "--detach", ref)
		return err
	}
	_, err := r.Run("checkout", ref)
	return err
}

// CheckoutRemote checks out a remote branch as a local branch tracking it, or
// the local branch of that name when there is one.
func (r *Repo) CheckoutRemote(rb RemoteBranch, local bool) error {
	if local {
		_, err := r.Run("checkout", rb.Branch)
		return err
	}
	_, err := r.Run("checkout", "-b", rb.Branch, "--track", rb.Name)
	return err
}

// NewBranch makes a branch at ref (HEAD when "") and checks it out.
func (r *Repo) NewBranch(name, ref string) error {
	args := []string{"checkout", "-b", name}
	if ref != "" {
		args = append(args, ref)
	}
	_, err := r.Run(args...)
	return err
}

// NewTag makes a lightweight tag at ref (HEAD when "").
func (r *Repo) NewTag(name, ref string) error {
	args := []string{"tag", name}
	if ref != "" {
		args = append(args, ref)
	}
	_, err := r.Run(args...)
	return err
}

// Merge merges ref into the current branch.
func (r *Repo) Merge(ref string) error {
	_, err := r.Run("merge", "--no-edit", ref)
	return r.conflicted(err)
}

// Rebase rebases the current branch onto ref.
func (r *Repo) Rebase(ref string) error {
	_, err := r.Run("rebase", ref)
	return r.conflicted(err)
}

// CherryPick applies a commit onto the current branch; a merge commit as
// against its first parent.
func (r *Repo) CherryPick(c Commit) error {
	args := []string{"cherry-pick"}
	if len(c.Parents) > 1 {
		args = append(args, "-m", "1")
	}
	_, err := r.Run(append(args, c.SHA)...)
	return r.conflicted(err)
}

// Revert makes a commit undoing c; a merge commit as against its first parent.
func (r *Repo) Revert(c Commit) error {
	args := []string{"revert", "--no-edit"}
	if len(c.Parents) > 1 {
		args = append(args, "-m", "1")
	}
	_, err := r.Run(append(args, c.SHA)...)
	return r.conflicted(err)
}

// Reset moves the current branch to sha: mode is soft, mixed or hard.
func (r *Repo) Reset(mode, sha string) error {
	_, err := r.Run("reset", "--"+mode, sha)
	return err
}

// RenameBranch renames a local branch.
func (r *Repo) RenameBranch(from, to string) error {
	_, err := r.Run("branch", "-m", from, to)
	return err
}

// ErrNotMerged is DeleteBranch's error for a branch whose commits are not
// merged anywhere: deleting it loses them.
var ErrNotMerged = errors.New("not fully merged")

// DeleteBranch deletes a local branch; force deletes one that is not merged.
func (r *Repo) DeleteBranch(name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := r.Run("branch", flag, name)
	var e *Error
	if !force && errors.As(err, &e) && strings.Contains(strings.Join(e.Lines, " "), "not fully merged") {
		return ErrNotMerged
	}
	return err
}

// DeleteRemoteBranch deletes a branch on its remote.
func (r *Repo) DeleteRemoteBranch(rb RemoteBranch) error {
	_, err := r.Run("push", rb.Remote, "--delete", rb.Branch)
	return err
}

// DeleteTag deletes a tag here (not on a remote).
func (r *Repo) DeleteTag(name string) error {
	_, err := r.Run("tag", "-d", name)
	return err
}

// PushTag pushes a tag to remote.
func (r *Repo) PushTag(remote, name string) error {
	_, err := r.Run("push", remote, "refs/tags/"+name)
	return err
}

// PushBranch pushes a local branch to remote.
func (r *Repo) PushBranch(remote, name string) error {
	_, err := r.Run("push", remote, "refs/heads/"+name)
	return err
}

// PushRemote is the remote to push to when nothing says which: the one the
// upstream is on, else origin, else the only one; "" for none.
func (s *Snapshot) PushRemote(upstream string) string {
	remotes := s.RemoteNames
	if upstream != "" {
		if name, _, ok := strings.Cut(upstream, "/"); ok {
			for _, rm := range remotes {
				if rm == name {
					return rm
				}
			}
		}
	}
	for _, rm := range remotes {
		if rm == "origin" {
			return rm
		}
	}
	if len(remotes) == 1 {
		return remotes[0]
	}
	return ""
}

// Fetch fetches every remote.
func (r *Repo) Fetch() error {
	_, err := r.Run("fetch", "--all")
	return err
}

// Pull pulls the current branch from its upstream, as the user's git is set
// to (merge or rebase).
func (r *Repo) Pull() error {
	_, err := r.Run("pull", "--no-edit")
	return r.conflicted(err)
}

// PullAll is git pull --all: every remote fetched, then the current branch
// pulled from its upstream.
func (r *Repo) PullAll() error {
	_, err := r.Run("pull", "--all", "--no-edit")
	return r.conflicted(err)
}

// Push pushes the current branch to its upstream; with setUpstream, to remote
// and tracking it there.
func (r *Repo) Push(remote, branch string, setUpstream bool) error {
	if setUpstream {
		_, err := r.Run("push", "-u", remote, branch)
		return err
	}
	_, err := r.Run("push")
	return err
}

// Stash puts the uncommitted changes away, with a message ("" for git's own).
func (r *Repo) Stash(message string) error {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}
	_, err := r.Run(args...)
	return err
}

// StashApply applies a stash entry; pop drops it when it applied cleanly.
func (r *Repo) StashApply(ref string, pop bool) error {
	verb := "apply"
	if pop {
		verb = "pop"
	}
	_, err := r.Run("stash", verb, ref)
	return r.conflicted(err)
}

// StashDrop deletes a stash entry.
func (r *Repo) StashDrop(ref string) error {
	_, err := r.Run("stash", "drop", ref)
	return err
}

// DiscardAll throws away every change to the tracked files.
func (r *Repo) DiscardAll() error {
	_, err := r.Run("reset", "--hard", "HEAD")
	return err
}

// Stage adds these files' changes to the index.
func (r *Repo) Stage(paths ...string) error {
	_, err := r.Run(append([]string{"add", "-A", "--"}, paths...)...)
	return err
}

// Unstage takes these files' changes out of the index, keeping them in the
// working tree.
func (r *Repo) Unstage(unborn bool, paths ...string) error {
	if unborn {
		_, err := r.Run(append([]string{"rm", "-q", "--cached", "-r", "--"}, paths...)...)
		return err
	}
	_, err := r.Run(append([]string{"restore", "--staged", "--"}, paths...)...)
	return err
}

// Remove deletes these tracked files and stages their deletion: the next
// commit takes them out of the repository. A change not committed is lost.
func (r *Repo) Remove(paths ...string) error {
	_, err := r.Run(append([]string{"rm", "-q", "-f", "--"}, paths...)...)
	return err
}

// Ignore stops tracking the tracked ones, keeping the files (git ignores
// only files it does not track), then adds these files to the .gitignore at
// the top of the working tree, each once.
func (r *Repo) Ignore(paths, tracked []string) error {
	if len(tracked) > 0 {
		if _, err := r.Run(append([]string{"rm", "-q", "--cached", "--"}, tracked...)...); err != nil {
			return err
		}
	}
	name := filepath.Join(r.Dir, ".gitignore")
	old, err := os.ReadFile(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(old), "\n") {
		have[strings.TrimRight(l, "\r")] = true
	}
	var add strings.Builder
	for _, p := range paths {
		if l := ignoreLine(p); !have[l] {
			have[l] = true
			add.WriteString(l + "\n")
		}
	}
	if add.Len() > 0 {
		lines := add.String()
		if len(old) > 0 && old[len(old)-1] != '\n' {
			lines = "\n" + lines
		}
		f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if _, err := f.WriteString(lines); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

// ignoreLine is the .gitignore line that matches path and only it: anchored
// at the top with "/" (so a leading # or ! is part of the name), the
// characters a pattern reads (* ? [ \) and trailing spaces escaped.
func ignoreLine(path string) string {
	var b strings.Builder
	for _, c := range path {
		if strings.ContainsRune(`*?[\`, c) {
			b.WriteByte('\\')
		}
		b.WriteRune(c)
	}
	l := "/" + b.String()
	if t := strings.TrimRight(l, " "); t != l {
		l = t + strings.Repeat("\\ ", len(l)-len(t))
	}
	return l
}

// StageAll adds every change, untracked files too.
func (r *Repo) StageAll() error {
	_, err := r.Run("add", "-A")
	return err
}

// UnstageAll empties the index of changes.
func (r *Repo) UnstageAll(unborn bool) error {
	if unborn {
		_, err := r.Run("rm", "-q", "--cached", "-r", "--", ".")
		return err
	}
	_, err := r.Run("reset", "-q")
	return err
}

// Discard throws away a file's change: staged, its change in the index and the
// working tree; otherwise only the working tree's. An untracked file is
// deleted.
func (r *Repo) Discard(f File, staged bool) error {
	switch {
	case f.Untracked():
		return os.RemoveAll(filepath.Join(r.Dir, f.Path))
	case staged && f.X == 'A':
		_, err := r.Run("rm", "-q", "-f", "--", f.Path)
		return err
	case staged:
		paths := []string{f.Path}
		if f.Orig != "" {
			paths = append(paths, f.Orig)
		}
		_, err := r.Run(append([]string{"restore", "--staged", "--worktree", "--source=HEAD", "--"}, paths...)...)
		return err
	default:
		_, err := r.Run("restore", "--", f.Path)
		return err
	}
}

// UseSide resolves whole conflicted files with one side, ours or theirs, and
// stages them.
func (r *Repo) UseSide(theirs bool, paths ...string) error {
	side := "--ours"
	if theirs {
		side = "--theirs"
	}
	if _, err := r.Run(append([]string{"checkout", side, "--"}, paths...)...); err != nil {
		return err
	}
	return r.Stage(paths...)
}

// Commit commits the index with message; amend replaces the last commit.
func (r *Repo) Commit(message string, amend bool) error {
	args := []string{"commit", "--cleanup=strip", "-F", "-"}
	if amend {
		args = append(args, "--amend")
	}
	_, err := r.RunIn(message, args...)
	return err
}

// LastMessage is the last commit's whole message.
func (r *Repo) LastMessage() string {
	out, err := r.Run("log", "-1", "--format=%B")
	if err != nil {
		return ""
	}
	return strings.TrimRight(out, "\n")
}

// Message is a commit's whole message.
func (r *Repo) Message(sha string) (string, error) {
	out, err := r.Run("log", "-1", "--format=%B", sha)
	return strings.TrimRight(out, "\n"), err
}

// MergeMessage is the message git prepared for the merge under way, its
// comment lines left out.
func (r *Repo) MergeMessage() string {
	b, err := os.ReadFile(filepath.Join(r.GitDir, "MERGE_MSG"))
	if err != nil {
		return ""
	}
	var keep []string
	for _, l := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(l, "#") {
			keep = append(keep, l)
		}
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

// Pushed reports whether HEAD is already on its upstream: amending it means a
// force push.
func (r *Repo) Pushed() bool {
	return r.ok("merge-base", "--is-ancestor", "HEAD", "@{upstream}")
}

// Abort gives up the operation under way (State.Op).
func (r *Repo) Abort(op string) error {
	_, err := r.Run(op, "--abort")
	return err
}

// Continue goes on with the rebase, cherry-pick or revert under way, taking
// the messages as they are.
func (r *Repo) Continue(op string) error {
	_, err := r.Run("-c", "core.editor=true", op, "--continue")
	return r.conflicted(err)
}
