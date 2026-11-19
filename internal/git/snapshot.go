package git

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Snapshot is the repository as it was read at one moment: what the screens
// show.
type Snapshot struct {
	// Head is the branch HEAD is on, "" when HEAD is detached or unborn.
	Head string
	// HeadSHA is the commit HEAD is at, "" before the first commit.
	HeadSHA string
	// Upstream is the current branch's upstream ("origin/main"), "" for none.
	Upstream string
	// Ahead and Behind are the current branch's commits ahead of and behind
	// its upstream.
	Ahead, Behind int

	Branches []Branch
	Remotes  []RemoteBranch
	Tags     []Tag
	Stashes  []Stash

	// All reports whether Commits is every branch's history or only what HEAD
	// reaches.
	All     bool
	Commits []Commit
	// More reports that the history goes on past Commits (Limit).
	More bool

	Files []File
	State State
}

// Branch is a local branch.
type Branch struct {
	Name, SHA string
	// Upstream is the branch it tracks ("origin/main"), "" for none.
	Upstream      string
	Ahead, Behind int
	// Gone reports an upstream set but no longer there.
	Gone    bool
	Current bool
}

// RemoteBranch is a remote-tracking branch, "origin/main".
type RemoteBranch struct {
	Name, Remote, Branch, SHA string
}

// Tag is a tag and the commit it names.
type Tag struct{ Name, SHA string }

// Stash is one entry of the stash.
type Stash struct {
	Index   int
	Ref     string // stash@{0}
	SHA     string
	Message string
}

// Commit is one commit of the history.
type Commit struct {
	SHA     string
	Parents []string
	Author  string
	Date    time.Time
	Subject string
}

// File is a file of the working tree with a change, as git status has it.
type File struct {
	Path string
	// Orig is the path it was renamed from, "" when it was not.
	Orig string
	// X is the index's status, Y the working tree's (git status --porcelain).
	X, Y byte
}

// Untracked reports a file git does not track.
func (f File) Untracked() bool { return f.X == '?' }

// Conflicted reports a file with an unmerged conflict.
func (f File) Conflicted() bool {
	return f.X == 'U' || f.Y == 'U' || (f.X == 'A' && f.Y == 'A') || (f.X == 'D' && f.Y == 'D')
}

// Staged reports a change in the index.
func (f File) Staged() bool { return !f.Untracked() && !f.Conflicted() && f.X != ' ' }

// Unstaged reports a change in the working tree not in the index, an untracked
// file among them.
func (f File) Unstaged() bool { return f.Untracked() || (!f.Conflicted() && f.Y != ' ') }

// State is an operation stopped half way, waiting for the user.
type State struct {
	// Op is "merge", "rebase", "cherry-pick", "revert", or "" for none.
	Op string
	// Other is what is being merged, picked or rebased onto, as git names it.
	Other string
}

// Limit is how many commits the history reads at most.
const Limit = 1000

// Read reads the repository: all, every branch's history; otherwise what HEAD
// reaches.
func (r *Repo) Read(all bool) (*Snapshot, error) {
	s := &Snapshot{All: all}
	if out, err := r.Run("symbolic-ref", "-q", "--short", "HEAD"); err == nil {
		s.Head = strings.TrimSpace(out)
	}
	if out, err := r.Run("rev-parse", "-q", "--verify", "HEAD^{commit}"); err == nil {
		s.HeadSHA = strings.TrimSpace(out)
	}
	if err := r.readRefs(s); err != nil {
		return nil, err
	}
	if err := r.readStashes(s); err != nil {
		return nil, err
	}
	if err := r.readLog(s); err != nil {
		return nil, err
	}
	if err := r.readStatus(s); err != nil {
		return nil, err
	}
	s.State = r.state()
	return s, nil
}

func (r *Repo) readRefs(s *Snapshot) error {
	out, err := r.Run("for-each-ref", "--format=%(refname)%00%(objectname)%00%(*objectname)%00%(upstream:short)%00%(upstream:track,nobracket)%00%(HEAD)",
		"refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return err
	}
	for _, line := range Lines(out) {
		f := strings.Split(line, "\x00")
		if len(f) < 6 {
			continue
		}
		ref, sha, peeled, up, track, head := f[0], f[1], f[2], f[3], f[4], f[5]
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			b := Branch{Name: strings.TrimPrefix(ref, "refs/heads/"), SHA: sha, Upstream: up, Current: head == "*"}
			b.Ahead, b.Behind, b.Gone = parseTrack(track)
			s.Branches = append(s.Branches, b)
			if b.Current {
				s.Upstream, s.Ahead, s.Behind = b.Upstream, b.Ahead, b.Behind
			}
		case strings.HasPrefix(ref, "refs/remotes/"):
			name := strings.TrimPrefix(ref, "refs/remotes/")
			remote, branch, _ := strings.Cut(name, "/")
			if branch == "HEAD" {
				continue // origin/HEAD only says which branch is the remote's default
			}
			s.Remotes = append(s.Remotes, RemoteBranch{Name: name, Remote: remote, Branch: branch, SHA: sha})
		case strings.HasPrefix(ref, "refs/tags/"):
			if peeled != "" {
				sha = peeled // an annotated tag names the commit it was made on
			}
			s.Tags = append(s.Tags, Tag{Name: strings.TrimPrefix(ref, "refs/tags/"), SHA: sha})
		}
	}
	slices.SortStableFunc(s.Branches, func(a, b Branch) int { return strings.Compare(a.Name, b.Name) })
	slices.SortStableFunc(s.Remotes, func(a, b RemoteBranch) int { return strings.Compare(a.Name, b.Name) })
	slices.SortStableFunc(s.Tags, func(a, b Tag) int { return VersionCompare(a.Name, b.Name) })
	return nil
}

// parseTrack reads %(upstream:track,nobracket): "ahead 1, behind 2", "gone".
func parseTrack(t string) (ahead, behind int, gone bool) {
	if t == "gone" {
		return 0, 0, true
	}
	for _, part := range strings.Split(t, ", ") {
		k, v, _ := strings.Cut(part, " ")
		n, _ := strconv.Atoi(v)
		switch k {
		case "ahead":
			ahead = n
		case "behind":
			behind = n
		}
	}
	return ahead, behind, false
}

// VersionCompare orders names with their numbers by value: v1.9 before v1.10.
func VersionCompare(a, b string) int {
	for a != "" && b != "" {
		da, db := digits(a), digits(b)
		if da > 0 && db > 0 {
			na, nb := strings.TrimLeft(a[:da], "0"), strings.TrimLeft(b[:db], "0")
			if len(na) != len(nb) {
				return len(na) - len(nb)
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			a, b = a[da:], b[db:]
			continue
		}
		if a[0] != b[0] {
			return int(a[0]) - int(b[0])
		}
		a, b = a[1:], b[1:]
	}
	return len(a) - len(b)
}

func digits(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}

func (r *Repo) readStashes(s *Snapshot) error {
	out, err := r.Run("stash", "list", "--format=%gd%x00%H%x00%gs")
	if err != nil {
		return nil // no stash at all is not a failure to read
	}
	for i, line := range Lines(out) {
		f := strings.Split(line, "\x00")
		if len(f) < 3 {
			continue
		}
		s.Stashes = append(s.Stashes, Stash{Index: i, Ref: f[0], SHA: f[1], Message: f[2]})
	}
	return nil
}

func (r *Repo) readLog(s *Snapshot) error {
	revs := []string{"HEAD"}
	if s.All {
		revs = []string{"--branches", "--remotes", "--tags"}
		if s.HeadSHA != "" {
			revs = append(revs, "HEAD")
		}
	} else if s.HeadSHA == "" {
		return nil
	}
	if s.All && s.HeadSHA == "" && len(s.Branches) == 0 && len(s.Remotes) == 0 && len(s.Tags) == 0 {
		return nil // nothing has been committed yet
	}
	args := append([]string{"log", "--topo-order", "-z", "-n", strconv.Itoa(Limit + 1),
		"--format=%H%x1f%P%x1f%an%x1f%at%x1f%s"}, revs...)
	args = append(args, "--")
	out, err := r.Run(args...)
	if err != nil {
		return err
	}
	for _, rec := range strings.Split(out, "\x00") {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		f := strings.Split(rec, "\x1f")
		if len(f) < 5 {
			continue
		}
		at, _ := strconv.ParseInt(f[3], 10, 64)
		c := Commit{SHA: f[0], Author: f[2], Date: time.Unix(at, 0), Subject: f[4]}
		if f[1] != "" {
			c.Parents = strings.Fields(f[1])
		}
		s.Commits = append(s.Commits, c)
	}
	if len(s.Commits) > Limit {
		s.Commits, s.More = s.Commits[:Limit], true
	}
	return nil
}

func (r *Repo) readStatus(s *Snapshot) error {
	out, err := r.Run("status", "--porcelain=v1", "-z")
	if err != nil {
		return err
	}
	recs := strings.Split(out, "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if len(rec) < 4 {
			continue
		}
		f := File{X: rec[0], Y: rec[1], Path: rec[3:]}
		if f.X == 'R' || f.X == 'C' || f.Y == 'R' || f.Y == 'C' {
			if i+1 < len(recs) {
				f.Orig = recs[i+1]
				i++
			}
		}
		s.Files = append(s.Files, f)
	}
	slices.SortStableFunc(s.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return nil
}

// state finds an operation stopped half way, from the files git leaves in its
// directory while one is.
func (r *Repo) state() State {
	has := func(name string) bool { _, err := os.Stat(filepath.Join(r.GitDir, name)); return err == nil }
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(r.GitDir, name))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	switch {
	case has("rebase-merge"), has("rebase-apply"):
		onto := read("rebase-merge/onto")
		if onto == "" {
			onto = read("rebase-apply/onto")
		}
		return State{Op: "rebase", Other: r.nameOf(onto)}
	case has("MERGE_HEAD"):
		return State{Op: "merge", Other: r.mergeOther()}
	case has("CHERRY_PICK_HEAD"):
		return State{Op: "cherry-pick", Other: Short(read("CHERRY_PICK_HEAD"))}
	case has("REVERT_HEAD"):
		return State{Op: "revert", Other: Short(read("REVERT_HEAD"))}
	}
	return State{}
}

// mergeOther is what a merge under way merges in, as its message names it
// ("Merge branch 'feature'" is feature).
func (r *Repo) mergeOther() string {
	b, err := os.ReadFile(filepath.Join(r.GitDir, "MERGE_MSG"))
	if err == nil {
		first, _, _ := strings.Cut(string(b), "\n")
		if _, rest, ok := strings.Cut(first, "'"); ok {
			if name, _, ok := strings.Cut(rest, "'"); ok {
				return name
			}
		}
	}
	head, _ := os.ReadFile(filepath.Join(r.GitDir, "MERGE_HEAD"))
	return r.nameOf(strings.TrimSpace(string(head)))
}

// nameOf is a branch naming sha, or sha cut short.
func (r *Repo) nameOf(sha string) string {
	if sha == "" {
		return ""
	}
	if out, err := r.Run("name-rev", "--name-only", "--no-undefined", sha); err == nil {
		return strings.TrimSpace(out)
	}
	return Short(sha)
}
