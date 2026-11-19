package git_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/vulcanshen/gitreu/internal/git"
	"github.com/vulcanshen/gitreu/internal/gittest"
)

func TestOpenFindsTheTop(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("sub/x.txt", "x\n")
	repo, err := git.Open(r.Dir + "/sub")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(repo.Dir, r.Dir[strings.LastIndex(r.Dir, "/"):]) {
		t.Errorf("Dir %q is not the top %q", repo.Dir, r.Dir)
	}
	if _, err := git.Open(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not in a git repository") {
		t.Errorf("a directory out of any repository: %v", err)
	}
}

func TestReadTheSample(t *testing.T) {
	r := gittest.Sample(t)
	repo, err := git.Open(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := repo.Read(true)
	if err != nil {
		t.Fatal(err)
	}
	if s.Head != "main" || s.HeadSHA == "" {
		t.Errorf("head %q %q", s.Head, s.HeadSHA)
	}
	var names []string
	for _, b := range s.Branches {
		names = append(names, b.Name)
		if b.Name == "main" && (!b.Current || b.Upstream != "origin/main") {
			t.Errorf("main: %+v", b)
		}
	}
	if !slices.Equal(names, []string{"feature", "main", "topic"}) {
		t.Errorf("branches %v", names)
	}
	if len(s.Remotes) != 1 || s.Remotes[0].Name != "origin/main" {
		t.Errorf("remotes %+v", s.Remotes)
	}
	if len(s.Tags) != 1 || s.Tags[0].Name != "v0.1.0" {
		t.Errorf("tags %+v", s.Tags)
	}
	if len(s.Stashes) != 1 || s.Stashes[0].Ref != "stash@{0}" || !strings.Contains(s.Stashes[0].Message, "Half a thought") {
		t.Errorf("stashes %+v", s.Stashes)
	}
	if len(s.Commits) != 7 {
		t.Errorf("%d commits, want 7 (topic's too)", len(s.Commits))
	}
	cur, err := repo.Read(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(cur.Commits) != 6 {
		t.Errorf("%d commits on main, want 6", len(cur.Commits))
	}
	if len(s.Files) != 0 {
		t.Errorf("files %+v", s.Files)
	}
}

func TestStatus(t *testing.T) {
	r := gittest.Sample(t)
	r.Commit("staged.txt", "s\n", "Add staged")
	r.Write("README.md", "changed\n")
	r.Write("new.txt", "new\n")
	r.Write("staged.txt", "s2\n")
	r.Git("add", "staged.txt")
	r.Write("staged.txt", "s3\n")
	repo, _ := git.Open(r.Dir)
	s, err := repo.Read(true)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][3]bool{}
	for _, f := range s.Files {
		got[f.Path] = [3]bool{f.Staged(), f.Unstaged(), f.Untracked()}
	}
	want := map[string][3]bool{
		"README.md":  {false, true, false},
		"new.txt":    {false, true, true},
		"staged.txt": {true, true, false},
	}
	for p, w := range want {
		if got[p] != w {
			t.Errorf("%s: staged, unstaged, untracked %v, want %v", p, got[p], w)
		}
	}
}

func TestGraphOfAMerge(t *testing.T) {
	// main: A ← C ← M(C, B) ; feature: A ← B
	cs := []git.Commit{
		{SHA: "M", Parents: []string{"C", "B"}},
		{SHA: "C", Parents: []string{"A"}},
		{SHA: "B", Parents: []string{"A"}},
		{SHA: "A"},
	}
	rows := git.Graph(cs)
	text := func(r []git.Cell) string {
		var b strings.Builder
		for _, c := range r {
			b.WriteRune(c.Rune)
		}
		return b.String()
	}
	n := string(git.Node)
	want := []string{n + "─╮", n + " │", "│ " + n, n + "─╯"}
	for i, r := range rows {
		if text(r) != want[i] {
			t.Errorf("row %d %q, want %q", i, text(r), want[i])
		}
	}
	if rows[0][0].Line == rows[0][2].Line {
		t.Error("the merged-in line has the same colour as main's")
	}
}

func TestGraphCapsTheLines(t *testing.T) {
	var cs []git.Commit
	for i := range 14 {
		cs = append(cs, git.Commit{SHA: string(rune('a' + i)), Parents: []string{"root"}})
	}
	cs = append(cs, git.Commit{SHA: "root"})
	rows := git.Graph(cs)
	for i, r := range rows {
		if len(r) > 2*git.MaxLines {
			t.Errorf("row %d is %d cells", i, len(r))
		}
	}
	last := rows[len(rows)-1]
	if last[len(last)-1].Rune != '…' {
		t.Errorf("the row joining 14 lines does not end in …: %q", string(cellRunes(last)))
	}
}

func cellRunes(r []git.Cell) []rune {
	var out []rune
	for _, c := range r {
		out = append(out, c.Rune)
	}
	return out
}

func TestConflictsResolve(t *testing.T) {
	for _, tc := range []struct {
		choice git.Choice
		want   string
	}{
		{git.Mine, "a\nB from main\nc\nd\ne\nx1\nx2\nx3\nx4\nF from main\ng\n"},
		{git.Theirs, "a\nB from feature\nc\nd\ne\nx1\nx2\nx3\nx4\nF from feature\ng\n"},
		{git.Both, "a\nB from main\nB from feature\nc\nd\ne\nx1\nx2\nx3\nx4\nF from main\nF from feature\ng\n"},
	} {
		r := gittest.Conflicted(t)
		repo, _ := git.Open(r.Dir)
		hs, err := repo.Conflicts("notes.txt")
		if err != nil {
			t.Fatal(err)
		}
		if len(hs) != 2 || hs[0].MineLabel != "HEAD" || hs[0].TheirsLabel != "feature" {
			t.Fatalf("hunks %+v", hs)
		}
		for want := 1; want >= 0; want-- {
			left, err := repo.Resolve("notes.txt", 0, tc.choice)
			if err != nil {
				t.Fatal(err)
			}
			if left != want {
				t.Errorf("%d left, want %d", left, want)
			}
		}
		if got := r.Read("notes.txt"); got != tc.want {
			t.Errorf("choice %d: %q, want %q", tc.choice, got, tc.want)
		}
	}
}

func TestMergeStopsOnConflicts(t *testing.T) {
	r := gittest.New(t)
	r.Commit("x.txt", "1\n", "One")
	r.Git("checkout", "-q", "-b", "other")
	r.Commit("x.txt", "2\n", "Two")
	r.Git("checkout", "-q", "main")
	r.Commit("x.txt", "3\n", "Three")
	repo, _ := git.Open(r.Dir)
	err := repo.Merge("other")
	var c *git.Conflicts
	if !errors.As(err, &c) || c.Files != 1 {
		t.Fatalf("merge: %v", err)
	}
	s, _ := repo.Read(true)
	if s.State.Op != "merge" || s.State.Other != "other" {
		t.Errorf("state %+v", s.State)
	}
	if err := repo.Abort("merge"); err != nil {
		t.Fatal(err)
	}
	if s, _ := repo.Read(true); s.State.Op != "" {
		t.Errorf("state after abort %+v", s.State)
	}
}

func TestVersionCompare(t *testing.T) {
	names := []string{"v1.10.0", "v1.9.0", "v1.2.0", "alpha"}
	slices.SortFunc(names, git.VersionCompare)
	if !slices.Equal(names, []string{"alpha", "v1.2.0", "v1.9.0", "v1.10.0"}) {
		t.Errorf("%v", names)
	}
}

func TestDeleteUnmergedBranch(t *testing.T) {
	r := gittest.Sample(t)
	repo, _ := git.Open(r.Dir)
	if err := repo.DeleteBranch("topic", false); !errors.Is(err, git.ErrNotMerged) {
		t.Fatalf("delete topic: %v", err)
	}
	if err := repo.DeleteBranch("topic", true); err != nil {
		t.Fatal(err)
	}
}
