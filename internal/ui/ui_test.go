package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"
	"github.com/vulcanshen/terminu-go/tdptest"

	"github.com/vulcanshen/gitreu/internal/git"
	"github.com/vulcanshen/gitreu/internal/gittest"
)

// start opens gitreu on the test's repository in a w × h terminal.
func start(t *testing.T, r *gittest.Repo, w, h int) (*gitreu, *tdptest.Driver) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo, err := git.Open(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	g := newGitreu(repo)
	return g, tdptest.Start(t, g.a, w, h)
}

// done waits for the git gitreu runs to end.
func done(t *testing.T, g *gitreu, d *tdptest.Driver) {
	t.Helper()
	d.WaitFor(func() bool { return g.idle() }, 20*time.Second)
	if !g.idle() {
		t.Fatalf("git still runs: %s", g.busy)
	}
}

// has fails the test when the screen does not show text.
func has(t *testing.T, d *tdptest.Driver, text ...string) {
	t.Helper()
	for _, s := range text {
		if !d.Has(s) {
			t.Fatalf("the screen has no %q:\n%s", s, strings.Join(d.Screen(), "\n"))
		}
	}
}

func hasNot(t *testing.T, d *tdptest.Driver, text ...string) {
	t.Helper()
	for _, s := range text {
		if d.Has(s) {
			t.Fatalf("the screen has %q:\n%s", s, strings.Join(d.Screen(), "\n"))
		}
	}
}

// selectBranch puts [1]'s cursor on a row by its ID.
func selectBranch(d *tdptest.Driver, g *gitreu, id string) {
	d.Do(func() tea.Cmd { g.history.branches.Select(id); return nil })
}

// selectCommit puts [2]'s cursor on the commit with this subject.
func selectCommit(t *testing.T, d *tdptest.Driver, g *gitreu, subject string) git.Commit {
	t.Helper()
	for _, c := range g.snap.Commits {
		if c.Subject == subject {
			d.Do(func() tea.Cmd { g.history.commits.Select(c.SHA); return nil })
			return c
		}
	}
	t.Fatalf("no commit %q", subject)
	return git.Commit{}
}

// --- task 1: the screens ---

func TestHistoryLayout(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("commit", "-q", "--allow-empty", "-m", "Ahead of origin")
	_, d := start(t, r, 80, 32)
	has(t, d, "[1] BRANCHES", "[2] COMMITS", "Repo: ", "Branch: main ↑1", "Space:menu ?:help !:log Tab|1–2:panels q:quit")
	// [1] a third of the width, [2] the rest (tdp L1).
	top := d.Screen()[2]
	at := strings.Index(top, "┌")
	if at < 0 || terminu.Width(top[:at]) != 26 {
		t.Errorf("[2] does not start at column 26:\n%s", top)
	}
}

func TestFileStatusScreen(t *testing.T) {
	r := gittest.Sample(t)
	_, d := start(t, r, 120, 32)
	d.Press("W")
	has(t, d, "[1] CHANGES", "[2] DIFF", "All changes", "Nothing to commit", "Press [H] to see the history")
	d.Press("H")
	has(t, d, "[1] BRANCHES")
}

func TestStatusbarSaysTheMerge(t *testing.T) {
	r := gittest.Conflicted(t)
	_, d := start(t, r, 120, 32)
	has(t, d, "Merging: feature")
}

// TestStatusbarKeepsItsPlace is tdp L2: Branch first, then Repo, both values
// Lavender; switching to a branch with a longer name moves nothing.
func TestStatusbarKeepsItsPlace(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Git("branch", "a-much-longer-branch-name")
	g, d := start(t, r, 120, 32)
	top := d.Screen()[0]
	branchAt, repoAt := strings.Index(top, "Branch: "), strings.Index(top, "Repo: ")
	if branchAt < 0 || repoAt < branchAt {
		t.Fatalf("Branch is not before Repo:\n%s", top)
	}
	styled := d.Styled()[0]
	lavender := "38;2;180;190;254"
	if strings.Count(styled, lavender) < 2 {
		t.Errorf("the branch and the repository are not both Lavender:\n%q", styled)
	}
	selectBranch(d, g, kindBranch+"a-much-longer-branch-name")
	d.Press("o")
	done(t, g, d)
	top2 := d.Screen()[0]
	if terminu.Width(top2[:strings.Index(top2, "Repo: ")]) != terminu.Width(top[:repoAt]) {
		t.Errorf("Repo moved:\n%s\n%s", top, top2)
	}
	if terminu.Width(top2[:strings.Index(top2, "Branch: ")]) != terminu.Width(top[:branchAt]) {
		t.Errorf("Branch moved:\n%s\n%s", top, top2)
	}
}

func TestQuitAtOnce(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 80, 32)
	d.Press("q")
	if !d.Quit() {
		t.Fatal("q did not quit")
	}
}

func TestQuitAsksWhileGitRuns(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 80, 32)
	d.Do(func() tea.Cmd { g.busy = "Pushing…"; return nil })
	has(t, d, "Pushing…")
	d.Press("q")
	if d.Quit() {
		t.Fatal("q quit while git runs")
	}
	has(t, d, "Quit gitreu?", "git is still at work: Pushing…")
	d.Press("Enter")
	if !d.Quit() {
		t.Fatal("Enter on the confirm did not quit")
	}
}

// TestOwnPopupsEverySize opens gitreu's own popups and resizes the terminal
// under each, for every icon width: each row as wide as the terminal (tdp L4,
// E5), never a crash.
func TestOwnPopupsEverySize(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	opens := []struct {
		name string
		keys []string
	}{
		{"diff viewer", []string{"Tab", "Enter", "Enter"}},
		{"new branch", []string{"b"}},
		{"reset select", []string{"Tab", "s"}},
		{"checkout confirm", []string{"Tab", "o"}},
		{"commit textarea", []string{"W", "Enter", "c"}},
		{"stash", []string{"S"}},
	}
	for _, o := range opens {
		t.Run(o.name, func(t *testing.T) {
			tdptest.EachIconWidth(t, func(t *testing.T) {
				_, d := start(t, r, 80, 32)
				for _, k := range o.keys {
					d.Press(k)
				}
				for _, sz := range [][2]int{{60, 32}, {50, 32}, {80, 12}, {50, 12}, {59, 32}, {80, 5}, {1, 1}, {80, 32}} {
					d.Resize(sz[0], sz[1])
				}
			})
		})
	}
}

// --- task 3: [1] BRANCHES ---

func TestBranchGroups(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("commit", "-q", "--allow-empty", "-m", "Ahead of origin")
	_, d := start(t, r, 160, 32)
	has(t, d, "BRANCHES", "REMOTES", "TAGS", "STASHES", "origin/main", "v0.1.0", "stash@{0}: On main: Half a thought", "main ↑1")
	styled := strings.Join(d.Styled(), "\n")
	if !strings.Contains(styled, "●") {
		t.Error("no flag on the current branch")
	}
}

func TestBranchMovesTheGraph(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 80, 32)
	d.Press("j") // from main to topic
	cur, _ := g.history.commits.Current()
	topic := strings.TrimSpace(r.Git("rev-parse", "topic"))
	if cur.ID != topic {
		t.Errorf("[2] is on %s, want topic's %s", cur.ID, topic)
	}
	if g.a.Screen().Focused() != g.history.branchP {
		t.Error("the focus left [1]")
	}
}

func TestBranchesEmpty(t *testing.T) {
	r := gittest.New(t)
	_, d := start(t, r, 80, 32)
	has(t, d, "No branches yet", "No commits yet")
}

func TestUpstreamGone(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("push", "-q", "-u", "origin", "topic")
	r.Git("push", "-q", "origin", "--delete", "topic")
	r.Git("fetch", "-q", "--prune")
	_, d := start(t, r, 120, 32)
	has(t, d, "topic upstream gone")
	if !strings.Contains(strings.Join(d.Styled(), "\n"), "38;2;250;179;135m upstream gone") {
		t.Error("upstream gone is not Peach")
	}
}

func TestEnterShowsTheCommit(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 120, 32)
	for _, ref := range []struct{ id, rev string }{
		{kindBranch + "feature", "feature"},
		{kindRemote + "origin/main", "origin/main"},
		{kindTag + "v0.1.0", "v0.1.0^{commit}"},
	} {
		d.Press("1")
		selectBranch(d, g, ref.id)
		d.Press("Enter")
		if g.a.Screen().Focused() != g.history.histP {
			t.Fatalf("%s: Enter did not go to [2]", ref.id)
		}
		cur, _ := g.history.commits.Current()
		if want := strings.TrimSpace(r.Git("rev-parse", ref.rev)); cur.ID != want {
			t.Errorf("%s: [2] is on %s, want %s", ref.id, cur.ID, want)
		}
		if strings.TrimSpace(r.Git("branch", "--show-current")) != "main" {
			t.Fatalf("%s: Enter checked out", ref.id)
		}
	}
	// From a commit gone into, Enter on a branch comes back to the graph.
	d.Press("Enter")
	has(t, d, "All branches › ")
	d.Press("1")
	selectBranch(d, g, kindBranch+"topic")
	d.Press("Enter")
	hasNot(t, d, "All branches › ")
	has(t, d, "Enter:files")
	d.Press("!")
	hasNot(t, d, "Checked out")
}

func TestEnterOnABranchNotInTheGraph(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 120, 32)
	d.Press("Tab", "a", "1")
	selectBranch(d, g, kindBranch+"topic")
	d.Press("Enter")
	has(t, d, "topic is not in the current branch's history", "Press [a] on [2] for all branches")
	if g.a.Screen().Focused() != g.history.branchP {
		t.Error("the focus left [1]")
	}
}

// --- task 4: [2] COMMITS ---

func TestGraphRows(t *testing.T) {
	r := gittest.Sample(t)
	_, d := start(t, r, 160, 32)
	has(t, d, "GRAPH", "HASH", "DESCRIPTION", "Merge branch 'feature'", "─╮", "main origin/main Say even more",
		string(git.Node), string(git.NodeTip), string(git.NodeRoot))
}

// TestGraphWithWideIcons is tdp E5: where an icon takes two cells, the
// commit's glyph takes the cell after it, and the lines stay in their
// columns on every row.
func TestGraphWithWideIcons(t *testing.T) {
	r := gittest.Sample(t)
	tdptest.EachIconWidth(t, func(t *testing.T) {
		_, d := start(t, r, 160, 32)
		var cols []int
		for _, row := range d.Screen() {
			if i := strings.Index(row, "Add the feature"); i >= 0 {
				cols = append(cols, terminu.Width(row[:i]))
			}
			if i := strings.Index(row, "Start the demo"); i >= 0 {
				cols = append(cols, terminu.Width(row[:i]))
			}
		}
		if len(cols) != 2 || cols[0] != cols[1] {
			t.Errorf("the descriptions are not in one column: %v\n%s", cols, strings.Join(d.Screen(), "\n"))
		}
	})
}

func TestGraphDropsColumnsWhenNarrow(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 60, 32)
	hasNot(t, d, "DATE")
	d.Resize(120, 32)
	has(t, d, "DATE", "AUTHOR")
}

func TestHistoryFilter(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 100, 32)
	d.Press("Tab", "/")
	d.Type("topic")
	has(t, d, "Try a topic")
	hasNot(t, d, "Say even more")
}

// --- task 5: all branches or the current one ---

func TestAllOrCurrentBranch(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 100, 32)
	d.Press("Tab")
	has(t, d, "All branches", "Try a topic")
	d.Press("a")
	has(t, d, "Current branch")
	hasNot(t, d, "Try a topic")
	if g.all {
		t.Error("still all")
	}
	d.Press("a")
	has(t, d, "All branches", "Try a topic")
	d.Press("!")
	has(t, d, "Showing the current branch", "Showing all branches")
}

// --- task 6: a commit's files and their diffs ---

func TestCommitFilesAndDiff(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 100, 32)
	c := selectCommit(t, d, g, "Grow the feature")
	d.Press("Tab", "Enter")
	has(t, d, "All branches › "+git.Short(c.SHA)+" Grow the feature", "feature.txt", "Enter:diff")
	d.Press("Enter")
	has(t, d, "DIFF — feature.txt", "+two")
	d.Press("Esc")
	hasNot(t, d, "DIFF — feature.txt")
	d.Press("Esc")
	has(t, d, "GRAPH")
	if cur, _ := g.history.commits.Current(); cur.ID != c.SHA {
		t.Error("back on the graph, the cursor is not on the commit gone into")
	}
	d.Press("!")
	has(t, d, "Opened the diff of feature.txt")
}

func TestFirstCommitFiles(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 100, 32)
	selectCommit(t, d, g, "Start the demo")
	d.Press("Tab", "Enter")
	has(t, d, "README.md")
	d.Press("Enter")
	has(t, d, "+# demo")
}

// --- task 7: two commits compared ---

func TestCompareMarked(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 100, 32)
	d.Press("Tab")
	a := selectCommit(t, d, g, "Start the demo")
	d.Press("m")
	b := selectCommit(t, d, g, "Say even more")
	d.Press("Space")
	has(t, d, "[C]ompare")
	d.Press("Esc", "m", "C")
	has(t, d, "› "+git.Short(a.SHA)+" ↔ "+git.Short(b.SHA), "feature.txt", "README.md")
	d.Press("Enter")
	has(t, d, "DIFF")
	d.Press("Esc", "Esc")
	has(t, d, "GRAPH")
}

// --- task 8: [1]'s operations ---

func TestCheckoutBranch(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindBranch+"feature")
	d.Press("o")
	done(t, g, d)
	has(t, d, "Checked out feature", "Branch: feature")
	if strings.TrimSpace(r.Git("branch", "--show-current")) != "feature" {
		t.Error("not on feature")
	}
}

func TestMergeBranchConflicts(t *testing.T) {
	r := gittest.New(t)
	r.Commit("x.txt", "1\n", "One")
	r.Git("checkout", "-q", "-b", "other")
	r.Commit("x.txt", "2\n", "Two")
	r.Git("checkout", "-q", "main")
	r.Commit("x.txt", "3\n", "Three")
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindBranch+"other")
	d.Press("M")
	has(t, d, "Merge other into main?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Merge stopped on conflicts in 1 file", "Resolve them in Working tree [W]", "Merging: other")
}

func TestMergeAndRebaseBranch(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindBranch+"topic")
	d.Press("M", "Enter")
	done(t, g, d)
	has(t, d, "Merged topic into main")
	r.Git("checkout", "-q", "topic")
	d.Press("R")
	done(t, g, d)
	d.WaitFor(func() bool { return g.snap.Head == "topic" }, 5*time.Second)
	selectBranch(d, g, kindBranch+"feature")
	d.Press("r")
	has(t, d, "Rebase topic onto feature?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Rebased topic onto feature")
}

func TestRenameBranch(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindBranch+"topic")
	d.Press("n")
	has(t, d, "RENAME: topic")
	d.Type("feature")
	d.Press("Enter")
	has(t, d, "There is a branch feature already")
	for range len("feature") {
		d.Press("Backspace")
	}
	d.Type("bad..name")
	d.Press("Enter")
	has(t, d, "Not a valid branch name")
	for range len("bad..name") {
		d.Press("Backspace")
	}
	d.Type("idea")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Renamed topic to idea")
}

func TestDeleteUnmergedAsksAgain(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindBranch+"topic")
	d.Press("D")
	has(t, d, "Delete topic?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Delete topic anyway?", "topic is not merged")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Deleted topic")
}

func TestCurrentBranchOperationsOff(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 100, 32)
	selectBranch(d, g, kindBranch+"main")
	d.Press("D")
	hasNot(t, d, "Delete main?")
}

func TestCheckoutRemoteBranch(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Git("push", "-q", "origin", "topic")
	r.Git("branch", "-q", "-D", "topic")
	r.Git("fetch", "-q", "origin")
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindRemote+"origin/topic")
	d.Press("o")
	has(t, d, "Check out origin/topic as topic?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Checked out topic")
	if up := strings.TrimSpace(r.Git("rev-parse", "--abbrev-ref", "topic@{upstream}")); up != "origin/topic" {
		t.Errorf("topic tracks %q", up)
	}
}

func TestDeleteRemoteBranch(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("push", "-q", "origin", "topic")
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindRemote+"origin/topic")
	d.Press("D")
	has(t, d, "Delete topic on origin?", "can't be undone")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Deleted topic on origin")
}

func TestTagOperations(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindTag+"v0.1.0")
	d.Press("Space")
	has(t, d, "Push", "this tag to its remote")
	d.Press("Esc", "o")
	has(t, d, "Check out v0.1.0?", "HEAD is left detached")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Checked out v0.1.0", "detached at")
	selectBranch(d, g, kindTag+"v0.1.0")
	d.Press("D", "Enter")
	done(t, g, d)
	has(t, d, "Deleted the tag v0.1.0")
}

func TestStashOperations(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindStash+"stash@{0}")
	d.Press("Enter")
	has(t, d, "STASH — stash@{0}", "+Stashed.")
	d.Press("Esc", "a")
	done(t, g, d)
	has(t, d, "Applied stash@{0}")
	r.Git("checkout", "-q", "--", ".")
	selectBranch(d, g, kindStash+"stash@{0}")
	d.Press("o")
	done(t, g, d)
	has(t, d, "Popped stash@{0}")
	r.Git("stash", "push", "-q", "-m", "Again")
	d.Press("R")
	done(t, g, d)
	d.WaitFor(func() bool { return len(g.snap.Stashes) == 1 }, 5*time.Second)
	selectBranch(d, g, kindStash+"stash@{0}")
	d.Press("D")
	has(t, d, "Drop stash@{0}?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Dropped stash@{0}")
}

func TestPushBranch(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindBranch+"topic")
	d.Do(func() tea.Cmd {
		for _, a := range g.history.branchP.Actions {
			if a.Label == "Push" && (a.Hidden == nil || !a.Hidden()) {
				return a.Do()
			}
		}
		t.Fatal("no Push on a branch")
		return nil
	})
	done(t, g, d)
	has(t, d, "Pushed topic to origin")
}

// --- task 9: a commit's operations ---

func TestCommitOperations(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	g, d := start(t, r, 160, 32)
	d.Press("Tab")
	c := selectCommit(t, d, g, "Add the feature")
	d.Press("y")
	has(t, d, "Copied "+git.Short(c.SHA))
	if clip := d.Clipboard(); len(clip) == 0 || clip[len(clip)-1] != c.SHA {
		t.Errorf("clipboard %v", clip)
	}
	d.Press("t")
	d.Type("v0.0.1")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Tagged "+git.Short(c.SHA)+" v0.0.1")
	selectCommit(t, d, g, "Add the feature")
	d.Press("b")
	d.Type("fix")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Made and checked out fix", "Branch: fix")
	c = selectCommit(t, d, g, "Try a topic")
	d.Press("Space")
	has(t, d, "Cherry-pick", "Revert")
	d.Press("Esc")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.history.histP, "Cherry-pick") })
	has(t, d, "Cherry-pick "+git.Short(c.SHA)+" onto fix?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Cherry-picked "+git.Short(c.SHA))
	selectCommit(t, d, g, "Try a topic")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.history.histP, "Revert") })
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Reverted")
	c = selectCommit(t, d, g, "Add the feature")
	d.Press("o")
	has(t, d, "Check out "+git.Short(c.SHA)+"?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "detached at "+git.Short(c.SHA))
}

// runLabel runs the action of panel p with this label, as its menu row does.
func runLabel(t *testing.T, g *gitreu, p *terminu.Panel, label string) tea.Cmd {
	for _, a := range p.Actions {
		if a.Label == label && (a.Hidden == nil || !a.Hidden()) {
			if a.AskFor != nil {
				if ask := a.AskFor(); ask != nil {
					return g.a.Confirm(*ask, a.Do)
				}
			}
			return a.Do()
		}
	}
	t.Fatalf("no %s", label)
	return nil
}

func TestResetHardAsks(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "dirty\n")
	g, d := start(t, r, 100, 32)
	d.Press("Tab")
	c := selectCommit(t, d, g, "Say more")
	d.Press("s")
	has(t, d, "RESET: "+git.Short(c.SHA), "[s]oft: keep the changes, staged", "[H]ard: throw the changes away")
	d.Press("H")
	has(t, d, "throw the changes away?", "can't be undone")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "(hard)")
	if got := strings.TrimSpace(r.Git("rev-parse", "HEAD")); got != c.SHA {
		t.Errorf("HEAD %s", got)
	}
}

func TestResetSoft(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 100, 32)
	d.Press("Tab")
	selectCommit(t, d, g, "Say more")
	d.Press("s", "s")
	done(t, g, d)
	has(t, d, "(soft)")
}

// --- task 10: the toolbar ---

func TestFetchPullPush(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Commit("ahead.txt", "a\n", "Get ahead")
	g, d := start(t, r, 100, 32)
	has(t, d, "↑1")
	d.Press("P")
	done(t, g, d)
	has(t, d, "Pushed main to origin/main")
	d.Press("f")
	done(t, g, d)
	has(t, d, "Fetched every remote")
	d.Press("p")
	done(t, g, d)
	has(t, d, "Pulled main")
}

func TestPushWithNoUpstreamAsks(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Git("checkout", "-q", "topic")
	g, d := start(t, r, 100, 32)
	d.Press("P")
	has(t, d, "Push topic to origin and track it?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Pushed topic to origin/topic")
}

func TestPushFailsWithItsLines(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Git("remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))
	r.Commit("ahead.txt", "a\n", "Get ahead")
	g, d := start(t, r, 100, 32)
	d.Press("P")
	done(t, g, d)
	has(t, d, "PUSH FAILED")
}

func TestNewBranchAndTagAtHead(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	g, d := start(t, r, 100, 32)
	d.Press("W", "b")
	has(t, d, "A branch at HEAD, checked out")
	d.Type("next")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Made and checked out next")
	d.Press("t")
	d.Type("v9")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Tagged HEAD v9")
}

func TestPickMerge(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	g, d := start(t, r, 100, 32)
	d.Press("W", "M")
	has(t, d, "MERGE: main", "topic", "origin/main")
	d.Press("/")
	d.Type("topic")
	d.Press("Enter", "Enter")
	has(t, d, "Merge topic into main?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Merged topic into main")
}

func TestStashAndDiscardAll(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "dirty\n")
	g, d := start(t, r, 100, 32)
	d.Press("S")
	has(t, d, "STASH")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Stashed the changes")
	if n := len(g.snap.Stashes); n != 2 {
		t.Errorf("%d stashes", n)
	}
	r.Write("README.md", "dirty again\n")
	d.Press("R")
	d.WaitFor(func() bool { return g.hasChanges() }, 5*time.Second)
	d.Do(func() tea.Cmd { return runGlobal(t, g, "Discard all") })
	has(t, d, "Discard every change to the tracked files?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Discarded every change")
}

// runGlobal runs the global operation with this label, its confirm first.
func runGlobal(t *testing.T, g *gitreu, label string) tea.Cmd {
	for _, a := range g.a.Global {
		if a.Label == label && (a.Hidden == nil || !a.Hidden()) {
			if a.Ask != nil {
				return g.a.Confirm(*a.Ask, a.Do)
			}
			if a.AskFor != nil {
				if ask := a.AskFor(); ask != nil {
					return g.a.Confirm(*ask, a.Do)
				}
			}
			return a.Do()
		}
	}
	t.Fatalf("no global %s", label)
	return nil
}

func TestRefreshAndReadingAgain(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 100, 32)
	r.Git("branch", "later")
	d.Advance(refreshEvery + time.Second)
	d.WaitFor(func() bool { return !g.reading }, 5*time.Second)
	has(t, d, "later")
	r.Git("branch", "later2")
	d.Press("R")
	d.WaitFor(func() bool { return d.Has("later2") }, 5*time.Second)
	has(t, d, "Read ")
}

// --- task 11: the working tree's files ---

func TestFileGroups(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "staged\n")
	r.Git("add", "README.md")
	r.Write("README.md", "staged and more\n")
	r.Write("new.txt", "new\n")
	_, d := start(t, r, 100, 32)
	d.Press("W")
	has(t, d, "All changes", "STAGED", "UNSTAGED", "? new.txt")
	screen := strings.Join(d.Screen(), "\n")
	if strings.Count(screen, "README.md") < 2 {
		t.Errorf("README.md is not in both groups:\n%s", screen)
	}
}

func TestDiffFollowsCursor(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Write("new.txt", "brand new\n")
	g, d := start(t, r, 100, 32)
	d.Press("W")
	has(t, d, "DIFF: README.md", "+changed")
	d.Press("j")
	has(t, d, "DIFF: new.txt", "+brand new")
	if g.a.Screen().Focused() != g.status.filesP {
		t.Error("the focus left [1]")
	}
	r.Write("new.txt", "brand new\nread again\n")
	d.Advance(refreshEvery + time.Second)
	d.WaitFor(func() bool { return d.Has("+read again") }, 5*time.Second)
	has(t, d, "+read again")
}

func TestDiffOpensInViewer(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	_, d := start(t, r, 100, 32)
	d.Press("W", "Tab", "Enter")
	has(t, d, "DIFF — README.md", "/:search")
}

// --- task 12: stage, unstage, discard ---

func TestStageUnstage(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	g, d := start(t, r, 100, 32)
	d.Press("W")
	has(t, d, "Enter:stage")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "┃ STAGED", "Enter:unstage")
	d.Press("Enter")
	done(t, g, d)
	hasNot(t, d, "┃ STAGED")
	d.Press("a")
	done(t, g, d)
	has(t, d, "┃ STAGED")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Unstage all") })
	done(t, g, d)
	hasNot(t, d, "┃ STAGED")
	d.Press("!")
	has(t, d, "Staged README.md", "Unstaged README.md", "Staged every change", "Unstaged every change")
}

func TestDiscardFile(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Write("new.txt", "new\n")
	g, d := start(t, r, 100, 32)
	d.Press("W", "D")
	has(t, d, "Discard the change to README.md?", "can't be undone")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Discarded the change to README.md")
	d.Press("D")
	has(t, d, "Delete new.txt?")
	d.Press("Enter")
	done(t, g, d)
	if _, err := os.Stat(filepath.Join(r.Dir, "new.txt")); !os.IsNotExist(err) {
		t.Error("new.txt is still there")
	}
}

// --- task 13: commit ---

func TestCommitMessage(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Git("add", "README.md")
	g, d := start(t, r, 100, 32)
	d.Press("c")
	has(t, d, "Esc:move")
	d.Type("Change the readme")
	d.Press("Esc", "Enter")
	done(t, g, d)
	has(t, d, "Committed ", "Change the readme")
	if got := strings.TrimSpace(r.Git("log", "-1", "--format=%s")); got != "Change the readme" {
		t.Errorf("last commit %q", got)
	}
}

func TestCommitOffWithNothingStaged(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 100, 32)
	d.Press("c")
	hasNot(t, d, "Esc:move")
}

func TestCommitHookKeepsTheMessage(t *testing.T) {
	r := gittest.Sample(t)
	hook := filepath.Join(r.Dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho no commits today >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.Write("README.md", "changed\n")
	r.Git("add", "README.md")
	g, d := start(t, r, 100, 32)
	d.Press("c")
	d.Type("Blocked")
	d.Press("Esc", "Enter")
	done(t, g, d)
	has(t, d, "COMMIT FAILED", "no commits today")
	d.Press("Esc", "c")
	has(t, d, "Blocked")
}

func TestAmendKeepsMessage(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Git("add", "README.md")
	g, d := start(t, r, 100, 32)
	d.Do(func() tea.Cmd { return runGlobal(t, g, "Amend") })
	has(t, d, "Amend the last commit, keeping its message?", "Say even more", "force push")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Committed ")
	if got := strings.TrimSpace(r.Git("log", "-1", "--format=%s")); got != "Say even more" {
		t.Errorf("last commit %q", got)
	}
}

// --- task 14: conflicts ---

func TestResolveHunks(t *testing.T) {
	r := gittest.Conflicted(t)
	g, d := start(t, r, 100, 32)
	d.Press("W")
	has(t, d, "CONFLICTS", "U notes.txt", "Enter:resolve")
	d.Press("Enter")
	has(t, d, "All changes › notes.txt", "Conflict 1", "Conflict 2", "MINE · HEAD (main)", "THEIRS · feature", "B from main", "B from feature",
		"Enter:read m:mine")
	d.Press("Enter")
	if g.a.Screen().Focused() != g.status.diffP {
		t.Error("Enter on a conflict did not go to [2]")
	}
	d.Press("1", "b")
	has(t, d, "F from main")
	d.Press("t")
	done(t, g, d)
	has(t, d, "notes.txt resolved and staged", "┃ STAGED")
	want := "a\nB from main\nB from feature\nc\nd\ne\nx1\nx2\nx3\nx4\nF from feature\ng\n"
	if got := r.Read("notes.txt"); got != want {
		t.Errorf("notes.txt %q", got)
	}
	d.Press("!")
	has(t, d, "Used both in conflict 1 of notes.txt", "Used theirs in conflict 1 of notes.txt")
	d.Advance(5 * time.Second)
	d.Press("Esc", "c")
	has(t, d, "Commit the merge?", "Merge branch 'feature'")
	d.Press("Enter")
	done(t, g, d)
	hasNot(t, d, "Merging:")
}

func TestUseSideWholeFile(t *testing.T) {
	r := gittest.Conflicted(t)
	g, d := start(t, r, 100, 32)
	d.Press("W", "m")
	done(t, g, d)
	has(t, d, "notes.txt resolved with mine and staged")
	if !strings.Contains(r.Read("notes.txt"), "B from main") {
		t.Error("not mine")
	}
}

func TestAbortMerge(t *testing.T) {
	r := gittest.Conflicted(t)
	g, d := start(t, r, 100, 32)
	d.Press("A")
	has(t, d, "Abort the merge?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Aborted the merge")
	hasNot(t, d, "Merging:")
}

func TestContinueRebase(t *testing.T) {
	r := gittest.New(t)
	r.Commit("x.txt", "1\n", "One")
	r.Git("checkout", "-q", "-b", "other")
	r.Commit("x.txt", "2\n", "Two")
	r.Git("checkout", "-q", "main")
	r.Commit("x.txt", "3\n", "Three")
	if _, err := r.Try("rebase", "other"); err == nil {
		t.Fatal("no conflict")
	}
	g, d := start(t, r, 160, 32)
	has(t, d, "Rebasing onto: other")
	d.Press("W", "m")
	done(t, g, d)
	d.Do(func() tea.Cmd { return runGlobal(t, g, "Continue rebase") })
	done(t, g, d)
	has(t, d, "Went on with the rebase")
	hasNot(t, d, "Rebasing onto")
}

// --- task 15: the setting ---

func TestTabWidthSetting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITREU__CONFIG", dir)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("input_tab_width: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, _ := git.Open(gittest.Sample(t).Dir)
	g := newGitreu(repo)
	tdptest.Start(t, g.a, 80, 32)
	if g.a.InputTabWidth != 2 {
		t.Errorf("tab width %d", g.a.InputTabWidth)
	}
}

func TestConfigMisfit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITREU__CONFIG", dir)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("input_tab_width: -3\nother: kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, _ := git.Open(gittest.Sample(t).Dir)
	g := newGitreu(repo)
	d := tdptest.Start(t, g.a, 100, 32)
	has(t, d, "did not fit", "config.yaml.bak")
	b, _ := os.ReadFile(path)
	if !bytes.Contains(b, []byte("input_tab_width: 4")) || !bytes.Contains(b, []byte("other: kept")) {
		t.Errorf("config.yaml:\n%s", b)
	}
	if bak, _ := os.ReadFile(path + ".bak"); !bytes.Contains(bak, []byte("-3")) {
		t.Errorf("config.yaml.bak:\n%s", bak)
	}
	if g.a.InputTabWidth != 4 {
		t.Errorf("tab width %d", g.a.InputTabWidth)
	}
}

func TestConfigCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITREU__CONFIG", dir)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("input_tab_width: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, _ := git.Open(gittest.Sample(t).Dir)
	g := newGitreu(repo)
	d := tdptest.Start(t, g.a, 100, 32)
	has(t, d, "MOVE ASIDE", "can't be read")
	d.Press("Enter")
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("no config.yaml.bak: %v", err)
	}
}

// --- task 16: the splash ---

// TestSplashIsDocsIcon is tdp E6: the splash draws docs/icon.svg, gitreu's
// family mark; the copy go:embed reads is the same file.
func TestSplashIsDocsIcon(t *testing.T) {
	want, err := os.ReadFile("../../docs/icon.svg")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(icon, want) {
		t.Fatal("internal/ui/icon.svg is not docs/icon.svg: copy docs/icon.svg over it")
	}
	for _, letter := range []string{"<!-- U -->", "<!-- G -->", "<!-- I -->", "<!-- T -->", "<!-- R -->", "<!-- E -->"} {
		if !bytes.Contains(icon, []byte(letter)) {
			t.Errorf("the mark has no %s", letter)
		}
	}
}

func TestSplashOpens(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 100, 40)
	d.Press("V")
	d.Advance(3 * time.Second)
	has(t, d, "gitreu", "A git client laid out like SourceTree", "vulcan.shen.2304@gmail.com")
	d.Press("q")
	if d.Quit() {
		t.Error("a key on the splash quit")
	}
	has(t, d, "[1] BRANCHES")
}
