package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// selectBranch puts [1] on the tab of a row and its cursor on the row, by its
// ID.
func selectBranch(d *tdptest.Driver, g *gitreu, id string) {
	d.Do(func() tea.Cmd {
		i := slices.Index(kinds, id[:2])
		g.history.branchP.Tab, g.history.branchP.Body = i, g.history.tabs[i]
		g.history.tabs[i].Select(id)
		return nil
	})
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
	g, d := start(t, r, 120, 32)
	has(t, d, "Merging: feature")
	// work in progress: Green (tdp screen.4)
	if !strings.Contains(strings.Join(d.Styled(), "\n"), "38;2;166;227;161m  Merging: feature") {
		t.Error("Merging: feature is not Green")
	}
	// one last field (tdp L2): what git does takes its place while it runs
	d.Do(func() tea.Cmd { g.busy = "Committing…"; return nil })
	has(t, d, "Committing…")
	hasNot(t, d, "Merging: feature")
	d.Do(func() tea.Cmd { g.busy = ""; return nil })
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
	d.Press("c")
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
		{"checkout confirm", []string{"Tab", "c"}},
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

func TestBranchTabs(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("commit", "-q", "--allow-empty", "-m", "Ahead of origin")
	g, d := start(t, r, 160, 32)
	has(t, d, "BRANCHES", "REMOTES", "TAGS", "STASHES", "main ↑1", "feature", "Enter:show h|l:tab")
	hasNot(t, d, "stash@{0}")
	styled := strings.Join(d.Styled(), "\n")
	if !strings.Contains(styled, "●") {
		t.Error("no flag on the current branch")
	}
	for _, step := range []struct {
		key  string
		tab  int
		rows string
	}{
		{"l", 1, "origin/main"},
		{"l", 2, "v0.1.0"},
		{"l", 3, "stash@{0}: On main: Half a thought"},
		{"l", 0, "feature"}, // around to the first tab
		{"h", 3, "stash@{0}: On main: Half a thought"},
	} {
		d.Press(step.key)
		if g.history.branchP.Tab != step.tab {
			t.Fatalf("after %s, tab %d, want %d", step.key, g.history.branchP.Tab, step.tab)
		}
		var ids []string
		for _, r := range g.history.cur().Rows() {
			ids = append(ids, r.Cells[0])
		}
		if !slices.ContainsFunc(ids, func(c string) bool { return strings.HasPrefix(c, step.rows) }) || len(ids) > 3 {
			t.Errorf("after %s, [1] lists %v", step.key, ids)
		}
		has(t, d, step.rows)
	}
}

func TestBranchTabsEmpty(t *testing.T) {
	r := gittest.New(t)
	r.Commit("a.txt", "a\n", "One")
	_, d := start(t, r, 160, 32)
	d.Press("l")
	has(t, d, "No remote branches", "Press [f] to fetch every remote")
	d.Press("l")
	has(t, d, "No tags", "Press [t] to tag HEAD")
	d.Press("l")
	has(t, d, "No stashes", "Press [S] to stash the changes")
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
	has(t, d, "topic "+glyphGone+" upstream gone")
	if !strings.Contains(strings.Join(d.Styled(), "\n"), "38;2;250;179;135m "+glyphGone+" upstream gone") {
		t.Error("upstream gone and its glyph are not Peach")
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
	d.Press("!")
	hasNot(t, d, "Checked out")
	d.Press("Esc")

	// [2] gone into the files of a commit: Enter on a branch comes back to
	// the graph, on that branch's commit (LeaveDiff, tdp note.8).
	d.Press("Enter")
	has(t, d, " ↔ ")
	d.Press("1")
	selectBranch(d, g, kindBranch+"topic")
	d.Press("Enter")
	hasNot(t, d, " ↔ ")
	if cur, _ := g.history.commits.Current(); cur.ID != strings.TrimSpace(r.Git("rev-parse", "topic")) {
		t.Errorf("[2] is on %s, want topic", cur.ID)
	}
	// moving on [1] comes back to the graph too (tdp panel.13)
	d.Press("Enter")
	has(t, d, " ↔ ")
	d.Press("1", "k")
	hasNot(t, d, " ↔ ")
}

func TestEnterOnARemoteBranchHidden(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("push", "-q", "origin", "topic")
	r.Git("branch", "-q", "-D", "topic")
	r.Git("fetch", "-q", "origin")
	g, d := start(t, r, 120, 32)
	selectBranch(d, g, kindRemote+"origin/topic")
	d.Press("Enter")
	has(t, d, "origin/topic is not in the graph", "Press [r] on [2] to show remote branches")
	if g.a.Screen().Focused() != g.history.branchP {
		t.Error("the focus left [1]")
	}
	d.Press("Tab", "r", "1")
	selectBranch(d, g, kindRemote+"origin/topic")
	d.Press("Enter")
	if g.a.Screen().Focused() != g.history.histP {
		t.Error("with the remote branches drawn, Enter did not go to [2]")
	}
}

func TestRemoteBranchesToggle(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("push", "-q", "origin", "topic")
	r.Git("branch", "-q", "-D", "topic")
	r.Git("fetch", "-q", "origin")
	g, d := start(t, r, 160, 32)
	d.Press("Tab")
	has(t, d, "All branches")
	hasNot(t, d, "origin/main", "Try a topic", "with remote branches")
	d.Press("Space")
	has(t, d, "Show [r]emote branches")
	d.Press("Esc", "r")
	has(t, d, "All branches · with remote branches", "origin/main", "origin/topic", "Try a topic")
	if !g.remote {
		t.Error("remote still off")
	}
	d.Press("r")
	hasNot(t, d, "origin/topic", "Try a topic")
	d.Press("!")
	has(t, d, "Showing remote branches", "Hiding remote branches")
}

// --- task 4: [2] COMMITS ---

func TestGraphRows(t *testing.T) {
	r := gittest.Sample(t)
	_, d := start(t, r, 160, 32)
	has(t, d, "GRAPH", "HASH", "DESCRIPTION", "Merge branch 'feature'", "─╮", glyphBranch+" main Say even more",
		glyphRefTag+" v0.1.0 Merge branch 'feature'", string(git.Node)+"─┴─╮")
	hasNot(t, d, "origin/main") // the remote branches are hidden at first
}

// TestGraphWithWideIcons is tdp E5: whatever cells an icon takes, the
// lines and the columns after the graph stay where they are on every row.
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

// TestAllOrCurrentBranch: both draw every branch, the main line on the left
// is main for All and HEAD for Current (SourceTree's All and Current).
func TestAllOrCurrentBranch(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Git("checkout", "-q", "topic")
	g, d := start(t, r, 120, 32)
	nodeAt := func(subject string) int {
		for _, row := range d.Screen() {
			if strings.Contains(row, subject) {
				// The node in [2]: the last one before the subject, in cells.
				i := strings.LastIndex(row[:strings.Index(row, subject)], string(git.Node))
				return terminu.Width(row[:i])
			}
		}
		return -1
	}
	d.Press("Tab")
	has(t, d, "All branches", "Try a topic", "Say even more")
	allMain, allTopic := nodeAt("Say even more"), nodeAt("Try a topic")
	if allMain >= allTopic {
		t.Errorf("All: main's commit at %d is not left of topic's at %d", allMain, allTopic)
	}
	d.Press("a")
	has(t, d, "Current branch", "Try a topic", "Say even more")
	if g.all {
		t.Error("still all")
	}
	curMain, curTopic := nodeAt("Say even more"), nodeAt("Try a topic")
	if curTopic >= curMain {
		t.Errorf("Current: HEAD's commit at %d is not left of main's at %d", curTopic, curMain)
	}
	d.Press("a")
	has(t, d, "All branches")
	d.Press("!")
	has(t, d, "Showing the current branch on the left", "Showing all branches, main on the left")
}

// --- task 6: a commit's files and their diffs ---

// TestCommitFilesAndDiff is task 6 on tdp note.8 and panel.9: Enter on a
// commit goes into the files that differ from its first parent, each with its
// mark; Enter on one compares its two versions, UNIFIED and SPLIT; Esc goes
// back up a level at a time.
func TestCommitFilesAndDiff(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Git("mv", "feature.txt", "renamed.txt")
	r.Write("README.md", "# demo\n")
	r.Write("added.txt", "new\n")
	r.Git("add", "-A")
	r.Git("commit", "-q", "-m", "Move, add and change")
	g, d := start(t, r, 120, 32)
	c := selectCommit(t, d, g, "Move, add and change")
	parent := c.Parents[0]
	d.Press("Tab", "Enter")
	has(t, d, git.Short(parent)+" ↔ "+git.Short(c.SHA), "+ added.txt", "~ README.md", "~ feature.txt → renamed.txt", "Enter:compare")
	d.Press("Enter")
	has(t, d, "DIFF — README.md @ "+git.Short(parent)+" ↔ README.md @ "+git.Short(c.SHA), "UNIFIED", "SPLIT")
	d.Press("Esc")
	hasNot(t, d, "DIFF — ")
	// A rename with nothing else changed: its two versions are the same.
	d.Press("j", "j", "Enter")
	has(t, d, "No differences between feature.txt @ "+git.Short(parent)+" and renamed.txt @ "+git.Short(c.SHA))
	d.Press("Esc", "Esc")
	has(t, d, "GRAPH")
	if cur, _ := g.history.commits.Current(); cur.ID != c.SHA {
		t.Error("back on the graph, the cursor is not on the commit gone into")
	}
	d.Press("!")
	has(t, d, "Opened the changes of "+git.Short(c.SHA))
}

func TestFirstCommitFiles(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 120, 32)
	selectCommit(t, d, g, "Start the demo")
	d.Press("Tab", "Enter")
	has(t, d, "empty ↔ ", "+ README.md")
	d.Press("Enter")
	has(t, d, "DIFF — README.md @ empty ↔ README.md @ ", "+# demo")
}

// --- task 7: two commits compared ---

// TestCompareTwoCommits is task 7 on tdp panel.9: C on a commit makes it the
// anchor of the compare mode, Enter on another goes into the files that differ,
// the anchor the base; Esc goes back up to the mode, the anchor kept.
func TestCompareTwoCommits(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 120, 32)
	d.Press("Tab")
	a := selectCommit(t, d, g, "Start the demo")
	d.Press("Space")
	has(t, d, "[C]ompare")
	d.Press("Esc", "C")
	has(t, d, "Compare")
	b := selectCommit(t, d, g, "Say even more")
	d.Press("Enter")
	has(t, d, git.Short(a.SHA)+" ↔ "+git.Short(b.SHA), "+ feature.txt", "~ README.md")
	d.Press("Enter")
	has(t, d, "DIFF — README.md @ "+git.Short(a.SHA)+" ↔ README.md @ "+git.Short(b.SHA))
	d.Press("Esc", "Esc")
	if g.history.histP.InMode() == nil {
		t.Error("Esc out of the files did not go back to the compare mode")
	}
	d.Press("Esc")
	if g.history.histP.InMode() != nil {
		t.Error("Esc did not leave the compare mode")
	}
	has(t, d, "GRAPH")
}

// --- task 8: [1]'s operations ---

func TestCheckoutBranch(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindBranch+"feature")
	d.Press("c")
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
	d.Press("Space")
	has(t, d, "Re[b]ase")
	d.Press("Esc", "b")
	has(t, d, "Rebase topic onto feature?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Rebased topic onto feature")
}

func TestRenameBranch(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindBranch+"topic")
	d.Press("r")
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
	d.Press("c")
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
	d.Press("Esc", "c")
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
	has(t, d, "base of stash@{0} ↔ stash@{0}", "~ README.md")
	d.Press("Enter")
	has(t, d, "DIFF — README.md @ base of stash@{0} ↔ README.md @ stash@{0}", "+Stashed.")
	d.Press("Esc", "Esc", "a")
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
	d.Press("c")
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
	// the line that says why, git's whole text in the App Log (tdp components.5)
	has(t, d, "PUSH FAILED", "does not appear to be a git repository")
	hasNot(t, d, "Please make sure you have the correct access rights")
	d.Press("Esc", "!")
	has(t, d, "Please make sure you have the correct access rights")
}

// TestPushWithoutRemote: with no remote, every Push is there but disabled
// (tdp M6).
func TestPushWithoutRemote(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Git("remote", "remove", "origin")
	g, d := start(t, r, 100, 32)
	disabled := func(acts []terminu.Action, label string) {
		t.Helper()
		for _, a := range acts {
			if a.Label == label && (a.Hidden == nil || !a.Hidden()) {
				if a.Disabled == nil || !a.Disabled() {
					t.Errorf("%s is not disabled", label)
				}
				return
			}
		}
		t.Errorf("no %s", label)
	}
	disabled(g.a.Global, "Push")
	disabled(g.a.Global, "Pull") // nothing to pull from
	selectBranch(d, g, kindBranch+"topic")
	disabled(g.history.branchP.Actions, "Push")
	selectBranch(d, g, kindTag+"v0.1.0")
	disabled(g.history.branchP.Actions, "Push")
	d.Press("P")
	hasNot(t, d, "PUSH FAILED", "Push topic")
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
	// git's own message is the suggested value: Enter untouched takes it
	// (tdp input.6)
	own := "WIP on main: " + git.Short(g.snap.HeadSHA)
	has(t, d, "STASH", own)
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Stashed the changes")
	if n := len(g.snap.Stashes); n != 2 {
		t.Errorf("%d stashes", n)
	}
	if m := g.snap.Stashes[0].Message; !strings.HasPrefix(m, own) {
		t.Errorf("stash message %q, want git's own", m)
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

// TestRefreshAndReadingAgain is task 19: every 3 seconds the repository is
// read again, and what changed outside gitreu is logged; R reads it at once.
func TestRefreshAndReadingAgain(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 100, 32)
	r.Git("branch", "later")
	d.Advance(refreshEvery + time.Second)
	d.WaitFor(func() bool { return !g.reading && d.Has("later") }, 5*time.Second)
	d.Press("!")
	has(t, d, "INFO Changed outside gitreu: branch later (new)")
	d.Press("Esc")
	r.Git("branch", "later2")
	d.Press("R")
	d.WaitFor(func() bool { return d.Has("later2") }, 5*time.Second)
	has(t, d, "Read ")
}

// TestRefreshLogsOnlyChanges: a read again that finds nothing new, what git
// ignores, and gitreu's own operations are not logged (task 19).
func TestRefreshLogsOnlyChanges(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Commit(".gitignore", "build/\n", "Ignore build")
	g, d := start(t, r, 120, 32)
	refresh := func() {
		t.Helper()
		d.Advance(refreshEvery + time.Second)
		d.WaitFor(func() bool { return !g.reading }, 5*time.Second)
	}
	refresh() // nothing changed
	r.Write("build/new.o", "y\n")
	refresh() // only what git ignores
	r.Write("README.md", "outside\n")
	refresh()
	d.Press("W", "Enter") // stage README.md: gitreu's own change
	done(t, g, d)
	refresh()
	d.Press("!")
	has(t, d, "Changed outside gitreu: README.md", "Staged README.md")
	screen := strings.Join(d.Screen(), "\n")
	if n := strings.Count(screen, "Changed outside gitreu"); n != 1 {
		t.Errorf("%d entries of changes, want 1:\n%s", n, screen)
	}
	hasNot(t, d, "build/")
}

// TestChanges: what two reads differ in, as the App Log says it.
func TestChanges(t *testing.T) {
	old := &git.Snapshot{Head: "main", HeadSHA: "a",
		Branches: []git.Branch{{Name: "main", SHA: "a"}, {Name: "gone", SHA: "g"}},
		Tags:     []git.Tag{{Name: "v1", SHA: "a"}},
		Files:    []git.File{{Path: "kept.txt", X: ' ', Y: 'M'}, {Path: "done.txt", X: 'M', Y: ' '}}}
	now := &git.Snapshot{Head: "feature", HeadSHA: "b",
		Branches: []git.Branch{{Name: "main", SHA: "b"}, {Name: "feature", SHA: "b"}},
		Tags:     []git.Tag{{Name: "v1", SHA: "a"}},
		Files:    []git.File{{Path: "kept.txt", X: ' ', Y: 'M'}, {Path: "new.txt", X: '?', Y: '?'}},
		State:    git.State{Op: "merge", Other: "x"}}
	got := changes(old, now, "kept.txt")
	want := []string{"new.txt", "done.txt", "kept.txt", "HEAD (on feature)", "branch feature (new)", "branch gone (deleted)", "branch main", "a merge under way"}
	if !slices.Equal(got, want) {
		t.Errorf("changes:\n got %q\nwant %q", got, want)
	}
	if s := said(want); s != "new.txt, done.txt, kept.txt and 5 more" {
		t.Errorf("said %q", s)
	}
	if got := changes(old, old, ""); len(got) != 0 {
		t.Errorf("the same read twice: %q", got)
	}
}

// TestFirstReadFails: with nothing read yet the panels say why, and the
// App Log has it (tdp panel.15, F5).
func TestFirstReadFails(t *testing.T) {
	r := gittest.Sample(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo, err := git.Open(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.Dir, ".git", "HEAD"), []byte("broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := newGitreu(repo)
	d := tdptest.Start(t, g.a, 120, 32)
	d.WaitFor(func() bool { return d.Has("Can't read") }, 5*time.Second)
	has(t, d, "Can't read", "Press [R] to read it again")
	d.Press("!")
	has(t, d, "ERR  Can't read")
}

// TestNoCommitYet: before the first commit there is no HEAD to go back to or
// to tag: Discard all and Tag are off (tdp M6).
func TestNoCommitYet(t *testing.T) {
	r := gittest.New(t)
	r.Write("a.txt", "a\n")
	r.Git("add", "a.txt")
	g, d := start(t, r, 100, 32)
	d.WaitFor(func() bool { return g.hasChanges() }, 5*time.Second)
	for _, label := range []string{"Discard all", "Tag", "Pull", "Push", "Stash", "Merge", "Branch"} {
		for _, a := range g.a.Global {
			if a.Label == label && (a.Disabled == nil || !a.Disabled()) {
				t.Errorf("%s is not disabled before the first commit", label)
			}
		}
	}
}

// TestRefreshFailureKeepsContent: a read that fails leaves what was read
// before and says why in an Error toast (tdp panel.15).
func TestRefreshFailureKeepsContent(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 120, 32)
	head := filepath.Join(r.Dir, ".git", "HEAD")
	was, err := os.ReadFile(head)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(head, []byte("broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d.Press("R")
	d.WaitFor(func() bool { return d.Has("Can't read") }, 5*time.Second)
	has(t, d, "Can't read", "feature", "topic")
	hasNot(t, d, "READ FAILED")
	if g.snap == nil || g.history.branchP.Fail != nil {
		t.Error("the old content is gone")
	}
	if err := os.WriteFile(head, was, 0o644); err != nil {
		t.Fatal(err)
	}
	d.Press("Esc", "R")
	d.WaitFor(func() bool { return d.Has("Read ") }, 5*time.Second)
	if g.readErr != "" {
		t.Errorf("readErr %q after a good read", g.readErr)
	}
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
	d.Press("!")
	has(t, d, "Changed outside gitreu: new.txt") // its status the same, its text not
	has(t, d, "+read again")
}

// TestDiffDrawnAsUnified is task 11 on tdp note.8 "In a panel": [2] DIFF
// draws the two versions as UNIFIED does, both line numbers and the
// separators, never git's own output; the location row names the two.
func TestDiffDrawnAsUnified(t *testing.T) {
	r := gittest.Sample(t)
	var was, now strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&was, "line %d\n", i)
		if i == 5 || i == 25 {
			fmt.Fprintf(&now, "changed %d\n", i)
		} else {
			fmt.Fprintf(&now, "line %d\n", i)
		}
	}
	r.Commit("long.txt", was.String(), "Add a long file")
	r.Write("long.txt", now.String())
	g, d := start(t, r, 120, 40)
	d.Press("W")
	d.Do(func() tea.Cmd { g.status.files.Select(inUnstaged + "long.txt"); return nil })
	d.Press("j", "k")
	has(t, d, "DIFF: long.txt", "long.txt @ index ↔ long.txt", "-line 5", "+changed 5", "-line 25", "+changed 25", "────")
	hasNot(t, d, "@@", "diff --git", "line 15")
	if !strings.Contains(strings.Join(d.Screen(), "\n"), " 5    -line 5") {
		t.Errorf("no line numbers:\n%s", strings.Join(d.Screen(), "\n"))
	}
}

// TestDiffNotText: a binary file has no lines to compare (tdp panel.9): [2]
// says so and has no Enter; an untracked folder lists its files.
func TestDiffNotText(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("blob.bin", "a\x00b")
	r.Write("newdir/one.txt", "1\n")
	r.Write("newdir/two.txt", "2\n")
	g, d := start(t, r, 120, 32)
	d.Press("W")
	d.Do(func() tea.Cmd { g.status.files.Select(inUnstaged + "blob.bin"); return nil })
	d.Press("j", "k")
	has(t, d, "DIFF: blob.bin", "Not a text file", "Press [1] to go to the files")
	hasNot(t, d, "Enter:compare")
	d.Do(func() tea.Cmd { g.status.files.Select(inUnstaged + "newdir/"); return nil })
	d.Press("j", "k")
	has(t, d, "DIFF: newdir/", "newdir/one.txt", "newdir/two.txt")
	hasNot(t, d, "Enter:compare")
}

// TestDiffComparesVersions is task 11 on tdp note.8: Enter on [2] DIFF compares
// the file's two versions: the index against the working tree when unstaged,
// HEAD against the index when staged.
func TestDiffComparesVersions(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Write("README.md", "changed\n")
	_, d := start(t, r, 120, 32)
	d.Press("W", "Tab")
	has(t, d, "Enter:compare")
	d.Press("Enter")
	has(t, d, "DIFF — README.md @ index ↔ README.md", "UNIFIED", "+changed")
	d.Press("Esc", "1", "Enter", "Tab", "Enter")
	has(t, d, "DIFF — README.md @ HEAD ↔ README.md @ index")
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
	d.Press("W", "c")
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
	d.Press("W", "c")
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
	d.Press("W", "c")
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

// TestConfigUnreadKey: a key gitreu never read stays in the file, no toast,
// an INFO at every start (tdp E3).
func TestConfigUnreadKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITREU__CONFIG", dir)
	path := filepath.Join(dir, "config.yaml")
	want := []byte("refesh: 5\ninput_tab_width: 2\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		repo, _ := git.Open(gittest.Sample(t).Dir)
		g := newGitreu(repo)
		d := tdptest.Start(t, g.a, 160, 32)
		hasNot(t, d, "did not fit")
		d.Press("!")
		has(t, d, "Not read: refesh in ")
		if g.a.InputTabWidth != 2 {
			t.Errorf("tab width %d", g.a.InputTabWidth)
		}
		if b, _ := os.ReadFile(path); !bytes.Equal(b, want) {
			t.Errorf("config.yaml:\n%s", b)
		}
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

// --- task 17: zoom ---

// leftWidth is [1]'s width on the screen: where [2]'s frame starts, 0 when
// [1] is not drawn.
func leftWidth(d *tdptest.Driver) int {
	row := d.Screen()[2]
	if strings.HasPrefix(row, "┏") && !strings.Contains(row, "┐") && !strings.Contains(row, "┓┌") && strings.Count(row, "┏") == 1 {
		return 0 // one panel across the whole area
	}
	for _, corner := range []string{"┓┌", "┐┏"} {
		if i := strings.Index(row, corner); i >= 0 {
			return terminu.Width(row[:i]) + 1
		}
	}
	return -1
}

func TestZoomTwoSteps(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 120, 32)
	if w := leftWidth(d); w != 40 {
		t.Fatalf("[1] is %d wide before zooming", w)
	}
	d.Press("Tab", "z")
	if w := leftWidth(d); w != leftLeast {
		t.Fatalf("first step: [1] is %d wide, want %d", w, leftLeast)
	}
	has(t, d, "z:zoom")
	d.Press("z")
	if w := leftWidth(d); w != 0 || !d.Has("[2] COMMITS") || d.Has("[1] BRANCHES") {
		t.Fatalf("second step: [1] still drawn (%d):\n%s", w, strings.Join(d.Screen(), "\n"))
	}
	has(t, d, "z:unzoom")
	d.Press("Tab")
	if !d.Has("[1] BRANCHES") || d.Has("[2] COMMITS") {
		t.Fatalf("Tab on the whole screen does not show [1] alone:\n%s", strings.Join(d.Screen(), "\n"))
	}
	d.Press("z")
	if w := leftWidth(d); w != 40 {
		t.Fatalf("z on [1] did not put both panels back: [1] is %d wide", w)
	}
	if g.history.zoom != 0 {
		t.Errorf("zoom %d", g.history.zoom)
	}
	d.Press("!")
	has(t, d, "Zoomed [2] COMMITS", "Zoomed [2] COMMITS to the whole screen", "Back to both panels")
}

func TestZoomNarrowSkipsTheFirstStep(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 60, 32)
	d.Press("Tab", "z")
	if g.history.zoom != 2 || d.Has("[1] BRANCHES") {
		t.Fatalf("at 60 columns z went to step %d:\n%s", g.history.zoom, strings.Join(d.Screen(), "\n"))
	}
	d.Press("z")
	if g.history.zoom != 0 {
		t.Errorf("z did not put it back: %d", g.history.zoom)
	}
}
