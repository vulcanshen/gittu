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

	"github.com/vulcanshen/gittu/internal/git"
	"github.com/vulcanshen/gittu/internal/gittest"
)

// start opens gittu on the test's repository in a w × h terminal.
func start(t *testing.T, r *gittest.Repo, w, h int) (*gittu, *tdptest.Driver) {
	t.Helper()
	return startWithState(t, r, "", w, h)
}

// startWithState starts gittu with its state in dir: the settings directory
// when "", a new one for each test.
func startWithState(t *testing.T, r *gittest.Repo, dir string, w, h int) (*gittu, *tdptest.Driver) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GITTU__STATE", dir)
	repo, err := git.Open(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	g := newGittu(repo)
	return g, tdptest.Start(t, g.a, w, h)
}

// done waits for the git gittu runs to end.
func done(t *testing.T, g *gittu, d *tdptest.Driver) {
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
func selectBranch(d *tdptest.Driver, g *gittu, id string) {
	d.Do(func() tea.Cmd {
		i := slices.Index(kinds, id[:2])
		g.history.branchP.Tab, g.history.branchP.Body = i, g.history.tabs[i]
		g.history.tabs[i].Select(id)
		return nil
	})
}

// selectCommit puts [2]'s cursor on the commit with this subject.
func selectCommit(t *testing.T, d *tdptest.Driver, g *gittu, subject string) git.Commit {
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

// TestOpensAt80x24: the least size is 60×24, so an 80×24 terminal shows the
// screen, and 23 rows the size it needs (the user; tdp L1).
func TestOpensAt80x24(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 80, 24)
	has(t, d, "[2] BRANCHES", "[3] CURRENT", "[4] DESCRIPTION", "[5] FILES")
	hasNot(t, d, "needs a terminal")
	d.Resize(80, 23)
	has(t, d, "needs a terminal of at least 60×24")
}

func TestHistoryLayout(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("commit", "-q", "--allow-empty", "-m", "Ahead of origin")
	_, d := start(t, r, 80, 32)
	has(t, d, "[2] BRANCHES", "● main ↑1", "[3] CURRENT", "ALL", "Space:menu ?:help !:log Tab|1–5:panels q:quit")
	// [1] and [2] a third of the width, the rest on the right (tdp L1).
	top := d.Screen()[0]
	at := strings.LastIndex(top[:max(strings.Index(top, "[3]"), 0)], "┌") // the capsule's end between
	if at < 0 || terminu.Width(top[:at]) != 26 {
		t.Errorf("[3] does not start at column 26:\n%s", top)
	}
	// [1] above [2], 1:2 (the user): 31 rows, [2] from the 11th.
	if i := slices.IndexFunc(d.Screen(), func(r string) bool { return strings.Contains(r, "[2] BRANCHES") }); i != 10 {
		t.Errorf("[2] starts on row %d, want 10:\n%s", i, strings.Join(d.Screen(), "\n"))
	}
}

func TestFileStatusScreen(t *testing.T) {
	r := gittest.Sample(t)
	_, d := start(t, r, 120, 32)
	d.Press("1")
	has(t, d, "[1] CHANGES", "[3] DIFF", "All changes", "Nothing to commit", "Press [2] to see the history")
	d.Press("2")
	has(t, d, "[2] BRANCHES")
}

// TestPanelsFollowTheLead is tdp L5-4: [3], [4] and [5] follow [1] or [2],
// whichever last had the focus, the same every time; the focus moving into
// them changes nothing; [5] keeps its number while hidden, its digit doing
// nothing, and Tab passes it by (the user).
func TestPanelsFollowTheLead(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 120, 32)
	focused := func() *terminu.Panel { return g.a.Screen().Focused() }
	has(t, d, "[3] CURRENT", "[4] DESCRIPTION", "[5] FILES", "Tab|1–5:panels")
	d.Press("1")
	has(t, d, "[3] DIFF", "[4] HISTORY", "Tab|1–5:panels")
	hasNot(t, d, "[3] CURRENT", "[4] DESCRIPTION", "[5] FILES")
	d.Press("5")
	if focused() != g.status.filesP {
		t.Error("5 moved the focus while [5] is hidden")
	}
	d.Press("3", "4") // into the followers: nothing changes
	has(t, d, "[3] DIFF", "[4] HISTORY")
	d.Press("Tab") // [5] hidden: round to [1]
	if focused() != g.status.filesP {
		t.Error("Tab from [4] did not pass the hidden [5] by")
	}
	d.Press("Tab")
	if focused() != g.history.branchP {
		t.Fatal("Tab from [1] is not on [2]")
	}
	has(t, d, "[3] CURRENT", "[4] DESCRIPTION", "[5] FILES")
	hasNot(t, d, "[3] DIFF", "[4] HISTORY")
	d.Press("1", "2") // the same every time
	has(t, d, "[3] CURRENT", "[5] FILES")
}

// TestNoStatusRow: no screen chips, no statusbar and no STATUS sign: [1]
// and [3] start on the first row, and [1]'s location row says All changes
// while nothing is under way (the user).
func TestNoStatusRow(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 120, 32)
	if top := d.Screen()[0]; !strings.Contains(top, "[1] CHANGES") || !strings.Contains(top, "[3] CURRENT") {
		t.Errorf("the first row is not [1] beside [3]:\n%s", top)
	}
	if row := d.Screen()[1]; !strings.Contains(row, "All changes") {
		t.Errorf("[1]'s location row does not say All changes:\n%s", row)
	}
	hasNot(t, d, "WORKING TREE", "STATUS", "Repo: ", "Branch: ")
}

// TestLocationSaysTheMerge: an operation stopped half way takes the place of
// All changes in [1]'s location row, its state's glyph first: conflicts
// left, then none; what git does takes it while it runs (the user).
func TestLocationSaysTheMerge(t *testing.T) {
	r := gittest.Conflicted(t)
	g, d := start(t, r, 120, 32)
	has(t, d, glyphConflicts+" Merging: feature")
	hasNot(t, d, "All changes")
	d.Press("1") // focused: not dimmed
	if !strings.Contains(d.Styled()[1], "38;2;180;190;254m"+glyphConflicts+" Merging: feature") {
		t.Errorf("the location row is not Lavender (tdp panel.10):\n%q", d.Styled()[1])
	}
	d.Do(func() tea.Cmd { g.busy = "Committing…"; return nil })
	has(t, d, "Committing…")
	hasNot(t, d, "Merging: feature")
	d.Do(func() tea.Cmd { g.busy = ""; return nil })
	has(t, d, "Merging: feature")
	// Into the conflicted file: the file after the state.
	d.Do(func() tea.Cmd { return g.status.goIn("notes.txt") })
	has(t, d, "Merging: feature › notes.txt")
	d.Press("Esc")
	// No conflict left: the ready glyph.
	r.Git("add", "notes.txt")
	d.Do(func() tea.Cmd { return g.read(true) })
	done(t, g, d)
	has(t, d, glyphReady+" Merging: feature")
	hasNot(t, d, glyphConflicts)
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
	has(t, d, "Quit gittu?", "git is still at work: Pushing…")
	d.Press("Enter")
	if !d.Quit() {
		t.Fatal("Enter on the confirm did not quit")
	}
}

// TestOwnPopupsEverySize opens gittu's own popups and resizes the terminal
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

// --- task 3: [2] BRANCHES ---

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
	r.Write("a.txt", "changed\n")
	_, d := start(t, r, 160, 32)
	d.Press("l") // no remote: f does nothing, so the hint names Space (tdp screen.8)
	has(t, d, "This repository has no remote", "Press [Space] to see what you can do here")
	d.Press("l")
	has(t, d, "No tags", "Press [t] to tag HEAD")
	d.Press("l")
	has(t, d, "No stashes", "Press [S] to stash the changes")
}

// TestBranchTabsEmptyHints: an empty tab names a key only while it works
// (tdp screen.8, M6).
func TestBranchTabsEmptyHints(t *testing.T) {
	r := gittest.New(t) // no commit yet, nothing changed
	r.Git("remote", "add", "origin", t.TempDir())
	g, d := start(t, r, 160, 32)
	d.Do(func() tea.Cmd { g.history.branchP.Tab, g.history.branchP.Body = 1, g.history.tabs[1]; return nil })
	has(t, d, "No remote branches", "Press [f] to fetch every remote")
	d.Do(func() tea.Cmd { g.history.branchP.Tab, g.history.branchP.Body = 2, g.history.tabs[2]; return nil })
	has(t, d, "No tags", "Press [Space] to see what you can do here")
	d.Do(func() tea.Cmd { g.history.branchP.Tab, g.history.branchP.Body = 3, g.history.tabs[3]; return nil })
	has(t, d, "No stashes", "Press [Space] to see what you can do here")
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
		d.Press("2")
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
}

// TestEnterChecksOutARemoteBranch: Enter on a remote branch no local branch
// goes with checks it out as one, asked first, then puts [2]'s cursor on it
// (the user).
func TestEnterChecksOutARemoteBranch(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("push", "-q", "origin", "topic")
	r.Git("branch", "-q", "-D", "topic")
	r.Git("fetch", "-q", "origin")
	g, d := start(t, r, 120, 32)
	selectBranch(d, g, kindRemote+"origin/topic")
	has(t, d, "Enter:check out")
	d.Press("Enter")
	has(t, d, "Check out origin/topic as topic?", "A local branch topic is made, tracking origin/topic.")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Checked out topic")
	if cur := strings.TrimSpace(r.Git("branch", "--show-current")); cur != "topic" {
		t.Fatalf("on %s, want topic", cur)
	}
	if g.a.Screen().Focused() != g.history.histP {
		t.Error("Enter did not go to [2]")
	}
	cur, _ := g.history.commits.Current()
	if want := strings.TrimSpace(r.Git("rev-parse", "topic")); cur.ID != want {
		t.Errorf("[2] is on %s, want %s", cur.ID, want)
	}
	// Now topic goes with it: Enter shows it, as on a branch.
	d.Press("2")
	selectBranch(d, g, kindRemote+"origin/topic")
	has(t, d, "Enter:show")
}

// TestGraphDrawsTrackedRemote: with the remote branches hidden, the graph
// still draws the ones a local branch goes with, and their commits ahead
// of it; not the others (the user).
func TestGraphDrawsTrackedRemote(t *testing.T) {
	r := gittest.Sample(t)
	r.Commit("ahead.txt", "a\n", "Get ahead on origin")
	r.Git("push", "-q", "origin", "main")
	r.Git("reset", "-q", "--hard", "HEAD~1")
	r.Git("push", "-q", "origin", "topic")
	r.Git("branch", "-q", "-D", "topic")
	_, d := start(t, r, 160, 32)
	has(t, d, glyphBranch+" origin/main", "Get ahead on origin")
	hasNot(t, d, "origin/topic", "Try a topic")
}

func TestRemoteBranchesToggle(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("push", "-q", "origin", "topic")
	r.Git("branch", "-q", "-D", "topic")
	r.Git("fetch", "-q", "origin")
	g, d := start(t, r, 160, 32)
	d.Press("Tab")
	has(t, d, "origin/main") // main tracks it: drawn all the same (the user)
	hasNot(t, d, "origin/topic", "Try a topic", "with remote branches")
	d.Press("Space")
	has(t, d, "Show remote branches")
	hasNot(t, d, "[r]emote") // no hotkey: not by a stray key
	d.Press("Esc", "r")
	if g.remote {
		t.Error("r drew the remote branches")
	}
	d.Do(func() tea.Cmd { return runPanel(t, g.history.histP, "Show remote branches") })
	has(t, d, "main · with remote branches", "origin/main", "origin/topic", "Try a topic")
	if !g.remote {
		t.Error("remote still off")
	}
	d.Do(func() tea.Cmd { return runPanel(t, g.history.histP, "Hide remote branches") })
	hasNot(t, d, "origin/topic", "Try a topic")
	d.Press("!")
	has(t, d, "Showing remote branches", "Hiding remote branches")
}

// --- task 4: [3] COMMITS ---

func TestGraphRows(t *testing.T) {
	r := gittest.Sample(t)
	_, d := start(t, r, 160, 32)
	has(t, d, "GRAPH", "HASH", "DESCRIPTION", "Merge branch 'feature'", "─╮", string(git.Node)+"─┴─╮")
	hasNot(t, d, "with remote branches") // hidden at first
	// A commit's branches and tags on the row under it (tdp main panel.5),
	// the graph's lines running on through it.
	g := d.Screen()
	under := func(subject, refs string) {
		t.Helper()
		for i, row := range g {
			if strings.Contains(row, subject) {
				if i+1 >= len(g) || !strings.Contains(g[i+1], refs) || strings.Contains(row, refs) {
					t.Errorf("%q is not on the row under %q:\n%s", refs, subject, strings.Join(g, "\n"))
				}
				if i+1 < len(g) && !strings.Contains(g[i+1], "│") {
					t.Errorf("the graph does not run on under %q", subject)
				}
				return
			}
		}
		t.Errorf("no %q", subject)
	}
	under("Say even more", glyphBranch+" main "+glyphBranch+" origin/main") // main tracks it (the user)
	under("Merge branch 'feature'", glyphRefTag+" v0.1.0")
}

// TestFilterFindsARef: the filter matches a commit by the row of its refs
// (tdp main panel.5).
func TestFilterFindsARef(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 160, 32)
	d.Press("Tab", "/")
	d.Type("v0.1.0")
	d.Press("Enter")
	cur, _ := g.history.commits.Current()
	if !strings.Contains(cur.Cells[2], "Merge branch") {
		t.Errorf("the filter for v0.1.0 is on %q", cur.Cells[2])
	}
}

// TestGraphWithWideIcons is tdp E5: whatever cells an icon takes, the
// lines and the columns after the graph stay where they are on every row.
// TestGraphWidensWithRoom: the graph draws as many lines as [2] has room
// for, 10 at the least (the user): zoomed to the whole screen, all 14.
func TestGraphWidensWithRoom(t *testing.T) {
	r := gittest.New(t)
	r.Commit("base.txt", "base\n", "Base")
	for i := range 14 {
		r.Git("checkout", "-q", "-b", fmt.Sprintf("b%02d", i), "main")
		r.Commit(fmt.Sprintf("f%02d.txt", i), "x\n", fmt.Sprintf("Work %02d", i))
	}
	r.Git("checkout", "-q", "main")
	g, d := start(t, r, 90, 32)
	graphHas := func(text string) bool {
		for _, row := range g.history.commits.Rows() {
			if strings.Contains(row.Cells[0], text) {
				return true
			}
		}
		return false
	}
	if !graphHas("…") {
		t.Fatalf("[2] beside [1] at 90 columns drew all 14 lines:\n%s", strings.Join(d.Screen(), "\n"))
	}
	d.Press("Tab", "z") // [3] on the whole screen
	if graphHas("…") {
		t.Errorf("[2] on the whole screen still cut the lines:\n%s", strings.Join(d.Screen(), "\n"))
	}
}

// TestDescriptionFills: on a wide [2], the room left over goes to
// DESCRIPTION, AUTHOR and DATE at the right edge (tdp panel.5, panel-184;
// the user picked Fill).
func TestDescriptionFills(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 160, 32)
	d.Press("Tab", "z")
	for _, row := range d.Screen() {
		if !strings.Contains(row, "GRAPH") {
			continue
		}
		rs := []rune(row)
		at := strings.Index(row, "DATE")
		col := len([]rune(row[:at]))
		if want := len(rs) - 2 - 10; col != want { // the frame and its margin, DATE 10 wide
			t.Errorf("DATE at column %d, want %d (the right edge):\n%s", col, want, row)
		}
		return
	}
	t.Fatalf("no header:\n%s", strings.Join(d.Screen(), "\n"))
}

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

// TestActionsClearTheFilter is tdp panel.14 (panel-180): an action of
// gittu's own that puts [2]'s cursor on a commit clears [2]'s filter first,
// whether the filter hides that commit or not; the focus leaving keeps it.
func TestActionsClearTheFilter(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 120, 32)
	commits := g.history.commits
	filter := func(text string) {
		t.Helper()
		d.Press("3", "/")
		d.Type(text)
		d.Press("Enter")
		if !commits.Filtering() {
			t.Fatalf("no filter %q", text)
		}
	}
	on := func(branch string) {
		t.Helper()
		if cur, _ := commits.Current(); cur.ID != strings.TrimSpace(r.Git("rev-parse", branch)) {
			t.Errorf("[2] is not on %s", branch)
		}
	}
	// [1] lists feature, main, topic; the cursor starts on main.
	filter("topic")
	d.Press("2")
	if !commits.Filtering() {
		t.Error("the focus leaving [2] cleared its filter")
	}
	d.Press("k") // to feature, its commit hidden by the filter
	if commits.Filtering() {
		t.Error("moving to feature kept [2]'s filter")
	}
	on("feature")
	filter("Say")
	d.Press("2", "j") // to main, its commit not hidden: cleared all the same
	if commits.Filtering() {
		t.Error("moving to main kept [2]'s filter")
	}
	on("main")
	filter("Try")
	d.Press("2", "Enter") // Enter on main
	if commits.Filtering() {
		t.Error("Enter on main kept [2]'s filter")
	}
	on("main")
}

// TestTrace is task 20: with the current branch on the left, n and N go
// down and up from commit to commit HEAD reaches (the user).
func TestTrace(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 160, 32)
	d.Press("Tab")
	on := func(subject string) {
		t.Helper()
		cur, _ := g.history.commits.Current()
		if !strings.Contains(cur.Cells[2], subject) {
			t.Errorf("[2] is on %q, want %q", cur.Cells[2], subject)
		}
	}
	selectCommit(t, d, g, "Say even more")
	d.Press("l", "n") // the All tab: no trace
	on("Say even more")
	d.Press("h") // the Current tab, main on the left
	d.Press("Space")
	has(t, d, "[n]ext commit", "[N] Previous commit")
	d.Press("Esc")
	d.Press("n") // past Try a topic, which main does not reach
	on("Merge branch 'feature'")
	d.Press("n") // a merged commit counts
	on("Grow the feature")
	d.Press("N", "N")
	on("Say even more")
	selectCommit(t, d, g, "Try a topic") // not on main: the nearest that way
	d.Press("N")
	on("Say even more")
	selectCommit(t, d, g, "Try a topic")
	d.Press("n")
	on("Merge branch 'feature'")
	selectCommit(t, d, g, "Start the demo")
	d.Press("n")
	has(t, d, "No older commit on main")
	on("Start the demo")
}

// TestCheckoutWarnsOfChanges: with changes not committed, every checkout
// asks first, saying they come along (the user); untracked files alone
// don't count.
func TestCheckoutWarnsOfChanges(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Write("scratch.txt", "untracked\n")
	g, d := start(t, r, 120, 32)
	selectBranch(d, g, kindBranch+"topic")
	d.Press("c") // only an untracked file: no question
	done(t, g, d)
	has(t, d, "Checked out topic")
	r.Write("feature.txt", "changed\n") // the same on topic and main: it can come along
	d.Press("R")
	d.WaitFor(func() bool { return g.hasChanges() }, 5*time.Second)
	d.Press("Esc")
	selectBranch(d, g, kindBranch+"main")
	d.Press("c")
	has(t, d, "Check out main?", "Changes not committed come along to main")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Checked out main")
	if got := r.Read("feature.txt"); got != "changed\n" {
		t.Errorf("the change did not come along: %q", got)
	}
	selectBranch(d, g, kindTag+"v0.1.0")
	d.Press("c")
	has(t, d, "HEAD is left detached", "Changes not committed come along to v0.1.0")
	d.Press("Esc")
	d.Press("Tab")
	selectCommit(t, d, g, "Say more")
	d.Press("c")
	has(t, d, "HEAD is left detached", "Changes not committed come along to")
	d.Press("Esc")
}

// --- task 5: all branches or the current one ---

// TestAllOrCurrentBranch: both draw every branch, the main line on the left
// is main for All and HEAD for Current.
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
	// Current first: [1]'s cursor starts on topic, the branch checked out.
	d.Press("Tab")
	if g.all {
		t.Error("[2] opens on All")
	}
	curMain, curTopic := nodeAt("Say even more"), nodeAt("Try a topic")
	if curTopic >= curMain {
		t.Errorf("Current: topic's commit at %d is not left of main's at %d", curTopic, curMain)
	}
	d.Press("l") // the All tab: main on the left
	has(t, d, "All branches", "Try a topic", "Say even more")
	allMain, allTopic := nodeAt("Say even more"), nodeAt("Try a topic")
	if allMain >= allTopic {
		t.Errorf("All: main's commit at %d is not left of topic's at %d", allMain, allTopic)
	}
	d.Press("2", "k") // [1] to main: All keeps main on the left
	if nodeAt("Say even more") >= nodeAt("Try a topic") {
		t.Error("All: main is not on the left")
	}
	d.Press("2", "j") // back to topic
	d.Press("3", "h") // Current again: the tab chosen is kept
	if g.all {
		t.Error("h did not go back to Current")
	}
	d.Press("2", "k") // [1] to main: Current follows it
	has(t, d, "main")
	if nodeAt("Say even more") >= nodeAt("Try a topic") {
		t.Errorf("Current did not follow [1] to main:\n%s", strings.Join(d.Screen(), "\n"))
	}
	d.Press("j") // and back to topic
	if nodeAt("Try a topic") >= nodeAt("Say even more") {
		t.Errorf("Current did not follow [1] to topic:\n%s", strings.Join(d.Screen(), "\n"))
	}
}

// --- task 6: a commit's files and their diffs ---

// TestCommitFilesAndDiff is task 6 on tdp panel.13, note.8 and panel.9: [3]
// lists the files the commit under [2]'s cursor changed against its first
// parent, each with its mark, and follows the cursor, the focus staying;
// Enter on the commit sends the focus to [3]; Enter on a file compares its
// two versions, UNIFIED and SPLIT.
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
	d.Press("Tab")
	has(t, d, "[4] DESCRIPTION", "Move, add and change", "[5] FILES", git.Short(parent)+" ↔ "+git.Short(c.SHA), "+ added.txt", "~ README.md", "~ feature.txt → renamed.txt", "Enter:files")
	d.Press("j") // [3] follows [2]'s cursor, the focus staying (tdp panel.13)
	hasNot(t, d, "+ added.txt")
	if g.a.Screen().Focused() != g.history.histP {
		t.Error("moving on [2] took the focus away")
	}
	d.Press("k", "Enter")
	if g.a.Screen().Focused() != g.history.filesP {
		t.Fatal("Enter on a commit did not send the focus to [3]")
	}
	d.Press("Enter")
	has(t, d, "README.md @ "+git.Short(parent)+" ↔ README.md @ "+git.Short(c.SHA), "UNIFIED", "SPLIT")
	d.Press("Esc")
	hasNot(t, d, "UNIFIED")
	// A rename with nothing else changed: its two versions are the same.
	d.Press("j", "j", "Enter")
	has(t, d, "No differences between feature.txt @ "+git.Short(parent)+" and renamed.txt @ "+git.Short(c.SHA))
	if cur, _ := g.history.commits.Current(); cur.ID != c.SHA {
		t.Error("[2]'s cursor left the commit")
	}
	d.Press("2", "k") // [1] on feature: [2] on its commit, [3] its files
	tip, before := strings.TrimSpace(r.Git("rev-parse", "feature")), strings.TrimSpace(r.Git("rev-parse", "feature^"))
	has(t, d, git.Short(before)+" ↔ "+git.Short(tip))
	d.Press("!")
	has(t, d, "Compared README.md @ "+git.Short(parent)+" with README.md @ "+git.Short(c.SHA))
}

// TestCommitDescription: [3] is the whole message of the commit under [2]'s
// cursor, a line too long for it folded at a blank (the user; tdp
// components.3), and follows the cursor.
func TestCommitDescription(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Write("README.md", "changed\n")
	r.Git("commit", "-q", "-am", "Change the readme\n\nThe body says why, in a line far longer than the panel is wide.")
	g, d := start(t, r, 120, 32)
	d.Press("Tab", "g", "g")
	has(t, d, "[4] DESCRIPTION", "Change the readme", "The body says why, in a line", "far longer than the panel is", "wide.")
	d.Press("j")
	hasNot(t, d, "The body says why")
	has(t, d, g.snap.Commits[1].Subject)
}

func TestFirstCommitFiles(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 120, 32)
	selectCommit(t, d, g, "Start the demo")
	d.Press("Tab") // a key: [3] follows after it, as after any (selectCommit sets the cursor outside one)
	has(t, d, "empty ↔ ", "+ README.md")
	d.Press("Enter", "Enter")
	has(t, d, "README.md @ empty ↔ README.md @ ", "+# demo")
}

// --- task 7: two commits compared ---

// TestDiffTwoCommits is task 7 on tdp panel.9 "With a details panel"
// (panel-185, panel-186): D on a commit makes it the anchor of the Diff mode,
// [2] and [3] both in it, the focus staying on [2]; [3] follows the cursor
// with the files that differ, the anchor the base; Enter sends the focus to
// [3], Enter there compares a file; Esc goes back the way it came: the diff,
// [3], [2] with the mode kept, out of the mode; Tab moves between the two,
// [1] waits.
func TestDiffTwoCommits(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 160, 32) // [4] wide enough for its empty state on a row
	h := g.history
	d.Press("Tab")
	a := selectCommit(t, d, g, "Start the demo")
	d.Press("Space")
	has(t, d, "[D]iff")
	d.Press("Esc", "D")
	has(t, d, "Diff", "D|Enter:diff", "Move to another commit to diff with "+git.Short(a.SHA))
	if h.histP.InMode() == nil || h.filesP.InMode() == nil || g.a.Screen().Focused() != h.histP {
		t.Fatal("D did not put [2] and [3] in the Diff mode, the focus on [2]")
	}
	hasNot(t, d, "Enter:files", "tab z:zoom") // [2]'s bottom hint is the mode's keys (tdp K11); [3], not in it, keeps its z:zoom
	b := g.snap.Commits[0]                    // gg: the top commit, [3] following
	d.Press("g", "g")
	has(t, d, git.Short(a.SHA)+" ↔ "+git.Short(b.SHA), "+ feature.txt", "~ README.md")
	d.Press("Enter")
	if g.a.Screen().Focused() != h.filesP {
		t.Fatal("Enter on [2] did not send the focus to [3]")
	}
	has(t, d, "Esc:back")
	d.Press("Enter")
	has(t, d, "README.md @ "+git.Short(a.SHA)+" ↔ README.md @ "+git.Short(b.SHA))
	d.Press("Esc")
	if g.a.Screen().Focused() != h.filesP {
		t.Error("Esc on the diff did not go back to [3]")
	}
	d.Press("Esc")
	if g.a.Screen().Focused() != h.histP || h.histP.InMode() == nil || h.filesP.InMode() == nil {
		t.Fatal("Esc on [3] did not go back to [2], the mode kept")
	}
	d.Press("Tab")
	if g.a.Screen().Focused() != h.filesP {
		t.Error("Tab in the mode did not go to [4]")
	}
	d.Press("4") // [3], the message, waits as [1] does
	has(t, d, "Press [Esc] to leave Diff first")
	if g.a.Screen().Focused() != h.filesP {
		t.Error("3 in the mode left [4]")
	}
	d.Press("Esc", "2")
	has(t, d, "Press [Esc] to leave Diff first")
	d.Press("Esc", "Esc", "Esc") // the toast, [4] back to [2], out of the mode
	if h.histP.InMode() != nil || h.filesP.InMode() != nil {
		t.Error("Esc did not leave the Diff mode")
	}
	has(t, d, "GRAPH", git.Short(b.Parents[0])+" ↔ "+git.Short(b.SHA), "Enter:files") // [3] the commit's own files again, [2]'s own hint
}

// TestDiffSameCommits: on a commit the same as the anchor (an empty commit
// and its parent) [3] says there is no difference, no toast (tdp panel.9,
// panel-185).
func TestDiffSameCommits(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("commit", "-q", "--allow-empty", "-m", "Nothing")
	g, d := start(t, r, 160, 32) // [4] wide enough for its empty state on a row
	top, parent := g.snap.Commits[0], g.snap.Commits[1]
	d.Press("Tab", "g", "g", "D", "j")
	has(t, d, "No differences between "+git.Short(top.SHA)+" and "+git.Short(parent.SHA))
	if top.Subject != "Nothing" || parent.SHA != top.Parents[0] {
		t.Fatalf("the top two commits: %q, %s", top.Subject, git.Short(parent.SHA))
	}
}

// --- task 8: [1]'s operations ---

func TestCheckoutBranch(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	g, d := start(t, r, 100, 32)
	selectBranch(d, g, kindBranch+"feature")
	d.Press("c")
	done(t, g, d)
	has(t, d, "Checked out feature", "● feature")
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
	g, d := start(t, r, 120, 32)
	selectBranch(d, g, kindBranch+"other")
	d.Press("M")
	has(t, d, "Merge other into main?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Merge stopped on conflicts in 1 file", "Press [1] to resolve them", "Merging: other")
	d.Press("1") // Working tree opens on the conflict
	if cur, _ := g.status.files.Current(); cur.ID != inConflict+"x.txt" {
		t.Errorf("Working tree opens on %q", cur.ID)
	}
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
	d.Press("x")
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
	d.Press("x")
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
	d.Press("x")
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
	has(t, d, "Checked out v0.1.0", glyphBranch+" HEAD") // the graph names HEAD detached
	selectBranch(d, g, kindTag+"v0.1.0")
	d.Press("x", "Enter")
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
	has(t, d, "README.md @ base of stash@{0} ↔ README.md @ stash@{0}", "+Stashed.")
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
	toastsGone(d, g) // stacked, they would cover the confirm's question
	d.Press("x")
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
	has(t, d, "Tagged "+git.Short(c.SHA)+" as v0.0.1")
	selectCommit(t, d, g, "Add the feature")
	d.Press("b")
	d.Type("fix")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Made and checked out fix", "● fix")
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
	toastsGone(d, g) // stacked, they would cover the confirm's question
	d.Press("c")
	has(t, d, "Check out "+git.Short(c.SHA)+"?")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, glyphBranch+" HEAD") // the graph names HEAD detached
	if got := strings.TrimSpace(r.Git("rev-parse", "HEAD")); got != c.SHA {
		t.Errorf("HEAD %s, want %s", got, c.SHA)
	}
}

// runLabel runs the action of panel p with this label, as its menu row does.
func runLabel(t *testing.T, g *gittu, p *terminu.Panel, label string) tea.Cmd {
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
	d.Do(func() tea.Cmd { return runPanel(t, g.history.histP, "Reset") })
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
	d.Press("s", "e") // no hotkeys for Reset and Rebase
	hasNot(t, d, "RESET", "REBASE")
	d.Do(func() tea.Cmd { return runPanel(t, g.history.histP, "Reset") })
	d.Press("s")
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

	// Pull all: every remote fetched, then the current branch pulled
	url := strings.TrimSpace(r.Git("remote", "get-url", "origin"))
	r.Git("remote", "add", "backup", url)
	d.Do(func() tea.Cmd { return runGlobal(t, g, "Pull all") })
	done(t, g, d)
	has(t, d, "Fetched every remote and pulled main")
	if _, err := r.Try("rev-parse", "--verify", "backup/main"); err != nil {
		t.Errorf("backup was not fetched: %v", err)
	}
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
	disabled(g.a.Global, "Pull all")
	disabled(g.a.Global, "Fetch")
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
	d.Press("1", "b")
	has(t, d, "A branch at HEAD, checked out")
	d.Type("next")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Made and checked out next")
	d.Press("t")
	d.Type("v9")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Tagged HEAD as v9")
}

func TestPickMerge(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	g, d := start(t, r, 100, 32)
	d.Press("1", "3", "M") // on [1] M is Mark all
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
	if cur, _ := g.history.tabs[3].Current(); cur.ID != kindStash+"stash@{0}" {
		t.Errorf("STASHES is on %q, not the new stash", cur.ID)
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

// runPanel runs the operation of panel p with this label, its confirm first.
func runPanel(t *testing.T, p *terminu.Panel, label string) tea.Cmd {
	t.Helper()
	for _, a := range p.Actions {
		if a.Label == label && (a.Hidden == nil || !a.Hidden()) {
			if a.Do == nil {
				t.Fatalf("%s does nothing", label)
			}
			return a.Do()
		}
	}
	t.Fatalf("no %s on the panel", label)
	return nil
}

// runGlobal runs the global operation with this label, its confirm first.
func runGlobal(t *testing.T, g *gittu, label string) tea.Cmd {
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
// read again, and what changed outside gittu is logged; R reads it at once.
func TestRefreshAndReadingAgain(t *testing.T) {
	r := gittest.Sample(t)
	g, d := start(t, r, 100, 32)
	r.Git("branch", "later")
	d.Advance(refreshEvery + time.Second)
	d.WaitFor(func() bool { return !g.reading && d.Has("later") }, 5*time.Second)
	d.Press("!")
	has(t, d, "INFO Changed outside gittu: branch later (new)")
	d.Press("Esc")
	r.Git("branch", "later2")
	d.Press("R")
	d.WaitFor(func() bool { return d.Has("later2") }, 5*time.Second)
	has(t, d, "Read ")
}

// TestRefreshLogsOnlyChanges: a read again that finds nothing new, what git
// ignores, and gittu's own operations are not logged (task 19).
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
	d.Press("1", "Enter") // stage README.md: gittu's own change
	done(t, g, d)
	refresh()
	d.Press("!")
	has(t, d, "Changed outside gittu: README.md", "Staged README.md")
	screen := strings.Join(d.Screen(), "\n")
	if n := strings.Count(screen, "Changed outside gittu"); n != 1 {
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
	g := newGittu(repo)
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
	for _, label := range []string{"Discard all", "Tag", "Pull", "Pull all", "Push", "Stash", "Merge", "Branch"} {
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
	refresh := func() {
		t.Helper()
		d.Advance(refreshEvery + time.Second)
		d.WaitFor(func() bool { return !g.reading }, 5*time.Second)
	}
	refresh()
	d.WaitFor(func() bool { return d.Has("Can't read") }, 5*time.Second)
	has(t, d, "Can't read", "feature", "topic")
	hasNot(t, d, "READ FAILED")
	if g.snap == nil || g.history.branchP.Fail != nil {
		t.Error("the old content is gone")
	}
	d.Press("Esc")
	refresh() // the same failure again: nothing more (tdp note-105)
	refresh()
	if err := os.WriteFile(head, was, 0o644); err != nil {
		t.Fatal(err)
	}
	refresh() // it reads again: one INFO, no toast
	if g.readErr != "" {
		t.Errorf("readErr %q after a good read", g.readErr)
	}
	hasNot(t, d, "can be read again") // not a toast
	d.Press("!")
	has(t, d, "INFO The repository can be read again")
	screen := strings.Join(d.Screen(), "\n")
	if n := strings.Count(screen, "ERR  Can't read"); n != 1 {
		t.Errorf("%d ERR entries of the same failure, want 1:\n%s", n, screen)
	}
}

// --- task 11: the working tree's files ---

// TestLongPathKeepsItsLetter: a path too long for [1] is cut from its front
// by gittu, the state's letter kept before it, at any width; the filter
// still matches the whole path (the user).
func TestLongPathKeepsItsLetter(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	long := "a/very/long/path/to/some/deeply/nested/folder/notes-of-the-day.txt"
	r.Commit(long, "x\n", "Add notes deep down")
	r.Write(long, "y\n")
	g, d := start(t, r, 60, 32)
	d.Press("1")
	has(t, d, "M …") // the letter, then the path cut from its front
	d.Resize(130, 32)
	has(t, d, "M …", "notes-of-the-day.txt")
	if cells := g.status.files.Rows()[0].Cells[0]; !strings.HasSuffix(cells, "a/very/long/path/to/some/deeply/nested/folder/notes-of-the-day.txt") {
		t.Errorf("the row text is not the whole path: %q", cells)
	}
}

func TestFileGroups(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "staged\n")
	r.Git("add", "README.md")
	r.Write("README.md", "staged and more\n")
	r.Write("new.txt", "new\n")
	_, d := start(t, r, 100, 32)
	d.Press("1")
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
	d.Press("1")
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
	has(t, d, "Changed outside gittu: new.txt") // its status the same, its text not
	has(t, d, "+read again")
}

// TestDiffDrawnAsUnified is task 11 on tdp note.8 "In a panel": [3] DIFF
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
	d.Press("1")
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
	d.Press("1")
	d.Do(func() tea.Cmd { g.status.files.Select(inUnstaged + "blob.bin"); return nil })
	d.Press("j", "k")
	has(t, d, "DIFF: blob.bin", "Not a text file", "Press [1] to go to the files")
	hasNot(t, d, "Enter:diff")
}

// TestNothingToCommitHints: with nothing changed, [3] and [4] send you where
// [1] does, not to [1]'s files: there are none (the user).
func TestNothingToCommitHints(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 120, 32)
	d.Press("1")
	has(t, d, "Nothing to commit", "No file to show")
	hasNot(t, d, "Press [1] to go to the files")
	if n := strings.Count(strings.Join(d.Screen(), "\n"), "Press [2] to see the history"); n != 3 {
		t.Errorf("Press [2] to see the history %d times, want 3 ([1], [3] and [4])", n)
	}
}

// TestDiffNewFile: an untracked file is not in the index, so its left side
// is written empty, as a commit's new file (the user).
func TestDiffNewFile(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("brand-new.txt", "hello\n")
	g, d := start(t, r, 120, 32)
	d.Press("1")
	d.Do(func() tea.Cmd { g.status.files.Select(inUnstaged + "brand-new.txt"); return nil })
	d.Press("j", "k")
	has(t, d, "brand-new.txt @ empty ↔ brand-new.txt", "+hello")
	hasNot(t, d, "brand-new.txt @ index")
}

// TestDiffNewFolder is task 11 on tdp note.8 (note-106): an untracked
// folder's files are listed with their marks, all +, and Enter goes into
// that list as into a commit; [1]'s cursor moving on leaves it.
func TestDiffNewFolder(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("newdir/one.txt", "1\n")
	r.Write("newdir/two.txt", "2\n")
	r.Write("newdir/blob.bin", "a\x00b")
	g, d := start(t, r, 120, 32)
	d.Press("1")
	d.Do(func() tea.Cmd { g.status.files.Select(inUnstaged + "newdir/"); return nil })
	d.Press("j", "k")
	has(t, d, "DIFF: newdir/", "newdir/ @ empty ↔ newdir/", "+ newdir/one.txt", "+ newdir/two.txt", "+ newdir/blob.bin", "Enter:diff")
	d.Press("3", "Enter") // into the list
	d.Press("Enter")      // the first file: newdir/blob.bin, not text, has no comparison
	hasNot(t, d, "blob.bin @ empty ↔")
	d.Press("j", "Enter")
	has(t, d, "newdir/one.txt @ empty ↔ newdir/one.txt", "UNIFIED")
	d.Press("Esc")
	d.Press("1", "k") // [1]'s cursor on another row: out of the list
	d.Press("j")
	has(t, d, "+ newdir/one.txt")
	if g.a.Screen().Focused() != g.status.filesP {
		t.Error("the focus left [1]")
	}
}

// TestDiffComparesVersions is task 11 on tdp note.8: Enter on [3] DIFF compares
// the file's two versions: the index against the working tree when unstaged,
// HEAD against the index when staged.
func TestDiffComparesVersions(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Write("README.md", "changed\n")
	_, d := start(t, r, 120, 32)
	d.Press("1", "3")
	has(t, d, "Enter:diff")
	d.Press("Enter")
	has(t, d, "README.md @ index ↔ README.md", "UNIFIED", "+changed")
	d.Press("Esc", "1", "Enter", "3", "Enter")
	has(t, d, "README.md @ HEAD ↔ README.md @ index")
}

// TestFileHistory: while [1] leads, [4] is the history of the file under its
// cursor, the newest first, following it; Enter compares the file in that
// commit with the commit before (the user; tdp panel.13, note.8).
func TestFileHistory(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Write("README.md", "changed\n")
	r.Write("new.txt", "new\n")
	g, d := start(t, r, 120, 32)
	d.Press("1")
	d.Do(func() tea.Cmd { g.status.files.Select(inUnstaged + "new.txt"); return nil })
	d.Press("k")
	has(t, d, "HISTORY: README.md", "Say even more", "Say more", "Start the demo")
	d.Press("j")
	has(t, d, "HISTORY: new.txt", "Not in any commit yet")
	var said, first git.Commit
	for _, c := range g.snap.Commits {
		switch c.Subject {
		case "Say more":
			said = c
		case "Start the demo":
			first = c
		}
	}
	d.Press("k", "4", "j", "Enter") // Say more
	has(t, d, "README.md @ "+git.Short(first.SHA)+" ↔ README.md @ "+git.Short(said.SHA), "UNIFIED", "+More.")
}

// --- task 12: stage, unstage, discard ---

// TestTabFollowsToTheGraph: another tab of [1] puts [2] on its row, as a
// move does; a stash's files gone into are left first.
func TestTabFollowsToTheGraph(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 120, 32)
	h := g.history
	d.Press("l") // REMOTES
	if !strings.Contains(h.commits.Location, "origin/main") {
		t.Fatalf("REMOTES: [2] on %q", h.commits.Location)
	}
	d.Press("l") // TAGS
	if !strings.Contains(h.commits.Location, "v0.1.0") {
		t.Fatalf("TAGS: [2] on %q", h.commits.Location)
	}
	d.Press("h") // back to REMOTES: [2] follows it again
	if !strings.Contains(h.commits.Location, "origin/main") {
		t.Errorf("back on REMOTES: [2] still on %q", h.commits.Location)
	}
	d.Press("l", "l", "Enter") // a stash's files gone into, then another tab
	has(t, d, "base of stash@{0} ↔ stash@{0}")
	d.Press("h")
	hasNot(t, d, "base of stash@{0} ↔ stash@{0}")
	has(t, d, "v0.1.0")
}

func TestEscOnFilesBackToTheGraph(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 120, 32)
	d.Press("Tab", "Enter")
	if g.a.Screen().Focused() != g.history.filesP {
		t.Fatal("Enter on a commit did not go to [4]")
	}
	d.Press("Esc") // back the way it came (tdp K4)
	if g.a.Screen().Focused() != g.history.histP {
		t.Error("Esc on [4] did not go back to [2]")
	}
}

// TestStageAllLeavesTheConflicts: a stages the other changes, never a
// conflicted file, whose markers Mark [r]esolved looks for first.
func TestStageAllLeavesTheConflicts(t *testing.T) {
	r := gittest.Conflicted(t)
	r.Write("other.txt", "new\n")
	g, d := start(t, r, 120, 32)
	d.Press("1", "a")
	done(t, g, d)
	has(t, d, "Staged every change but the 1 conflict")
	if out := strings.TrimSpace(r.Git("diff", "--name-only", "--diff-filter=U")); out != "notes.txt" {
		t.Errorf("unmerged after a: %q", out)
	}
	if out := strings.TrimSpace(r.Git("diff", "--cached", "--name-only", "--", "other.txt")); out != "other.txt" {
		t.Errorf("other.txt not staged: %q", out)
	}
}

// TestUnstageRenameWhole: Enter on a staged rename takes both its paths out
// of the index.
func TestUnstageRenameWhole(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Git("mv", "feature.txt", "renamed.txt")
	g, d := start(t, r, 120, 32)
	d.Press("1")
	d.Do(func() tea.Cmd { g.status.files.Select(inStaged + "renamed.txt"); return nil })
	d.Press("Enter")
	done(t, g, d)
	if out := strings.TrimSpace(r.Git("diff", "--cached", "--name-only")); out != "" {
		t.Errorf("still staged: %q", out)
	}
}

// TestStageWalksDown: after Enter stages a file the cursor is on the next
// one of its group, so Enter walks down the list.
func TestStageWalksDown(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Write("a.txt", "a\n")
	r.Write("b.txt", "b\n")
	r.Write("c.txt", "c\n")
	g, d := start(t, r, 120, 32)
	d.Press("1")
	d.Do(func() tea.Cmd { g.status.files.Select(inUnstaged + "b.txt"); return nil })
	d.Press("Enter")
	done(t, g, d)
	if cur, _ := g.status.files.Current(); cur.ID != inUnstaged+"c.txt" {
		t.Errorf("after staging b.txt the cursor is on %q", cur.ID)
	}
}

// TestMarksGoWithTheirRows: a mark on a file that left the list is dropped.
func TestMarksGoWithTheirRows(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Write("README.md", "changed\n")
	g, d := start(t, r, 120, 32)
	d.Press("1")
	markFile(d, g, inUnstaged+"README.md")
	r.Git("add", "README.md")
	d.Do(func() tea.Cmd { return g.read(true) })
	done(t, g, d)
	if m := g.status.files.Marked(); len(m) != 0 {
		t.Errorf("marks of rows gone: %v", m)
	}
}

// TestNoMergeHalfway: while a merge stops half way, Merge, Rebase,
// Cherry-pick and Revert are off (tdp M6).
func TestNoMergeHalfway(t *testing.T) {
	r := gittest.Conflicted(t)
	g, d := start(t, r, 120, 32)
	has(t, d, "Merging: feature")
	for _, a := range g.a.Global {
		if a.Label == "Merge" && (a.Disabled == nil || !a.Disabled()) {
			t.Error("the global Merge is on during a merge")
		}
	}
	for _, a := range g.history.commitActions() {
		switch a.Label {
		case "Merge", "Rebase", "Cherry-pick", "Revert":
			if a.Disabled == nil || !a.Disabled() {
				t.Errorf("%s is on during a merge", a.Label)
			}
		}
	}
}

func TestStageUnstage(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	g, d := start(t, r, 100, 32)
	d.Press("1")
	has(t, d, "Enter:stage")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "┃ STAGED", "Enter:unstage")
	d.Press("Enter")
	done(t, g, d)
	hasNot(t, d, "┃ STAGED")
	d.Press("Space") // a toggles, the key bracketed at "all" (the user; tdp main M5-1)
	has(t, d, "Stage [a]ll")
	hasNot(t, d, "St[a]ge")
	d.Press("Esc", "a")
	done(t, g, d)
	has(t, d, "┃ STAGED")
	d.Press("Space")
	has(t, d, "Unstage [a]ll")
	d.Press("Esc", "a") // again: every change unstaged
	done(t, g, d)
	hasNot(t, d, "┃ STAGED")
	d.Press("!")
	has(t, d, "Staged README.md", "Unstaged README.md", "Staged every change", "Unstaged every change")
}

// markFile puts [1]'s cursor on the row id and marks it with m.
// toastsGone lets the Info and Success toasts showing run out (2.2 s; tdp
// toast.2, stacked since v1.3.0), the read again on the way settled.
func toastsGone(d *tdptest.Driver, g *gittu) {
	d.Advance(2300 * time.Millisecond)
	d.WaitFor(func() bool { return !g.reading }, 5*time.Second)
}

func markFile(d *tdptest.Driver, g *gittu, id string) {
	d.Do(func() tea.Cmd { g.status.files.Select(id); return nil })
	d.Press("m")
}

// TestMarkFiles: m marks the file under the cursor, its tick in the mark
// column, the file's state in its row; again unmarks it; M marks them all,
// and again unmarks them all, never the global Merge; Clear marks, in the
// Space menu, unmarks some marked (the user; tdp panel.7, M4, M6).
func TestMarkFiles(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Write("new.txt", "new\n")
	g, d := start(t, r, 100, 32)
	d.Press("1")
	has(t, d, "M README.md", "? new.txt", "m:mark")
	markFile(d, g, inUnstaged+"README.md")
	if got := g.status.files.Marked(); !slices.Equal(got, []string{inUnstaged + "README.md"}) {
		t.Fatalf("marked %v", got)
	}
	has(t, d, "\U000F0132 M README.md", "m:unmark")
	d.Press("m")
	if len(g.status.files.Marked()) != 0 {
		t.Fatal("m again did not unmark")
	}
	d.Press("M")
	if len(g.status.files.Marked()) != 2 {
		t.Fatalf("M marked %v", g.status.files.Marked())
	}
	d.Press("M")
	hasNot(t, d, "MERGE")
	if len(g.status.files.Marked()) != 0 {
		t.Fatalf("M again did not unmark them all: %v", g.status.files.Marked())
	}
	markFile(d, g, inUnstaged+"README.md")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Clear marks") })
	if len(g.status.files.Marked()) != 0 {
		t.Fatal("Clear marks left some")
	}
	d.Press("!")
	has(t, d, "Marked README.md", "Unmarked README.md", "Marked every file", "Unmarked every file", "Cleared the marks")
}

// TestWorkOnTheMarked: the Space menu's operations on the marked files act
// on the ones they fit, a file in two groups once, then clear the marks.
func TestWorkOnTheMarked(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Write("a.txt", "a\n")
	r.Write("b.txt", "b\n")
	g, d := start(t, r, 100, 32)
	d.Press("1")
	markFile(d, g, inUnstaged+"README.md")
	markFile(d, g, inUnstaged+"a.txt")
	d.Press("Space")
	has(t, d, "Stage the marked", "2 files", "Discard the marked")
	d.Press("Esc")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Stage the marked") })
	done(t, g, d)
	if got := r.Git("diff", "--cached", "--name-only"); got != "README.md\na.txt\n" {
		t.Errorf("staged %q", got)
	}
	if len(g.status.files.Marked()) != 0 {
		t.Error("the marks stayed")
	}
	markFile(d, g, inStaged+"README.md")
	markFile(d, g, inStaged+"a.txt")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Unstage the marked") })
	done(t, g, d)
	if got := r.Git("diff", "--cached", "--name-only"); got != "" {
		t.Errorf("still staged %q", got)
	}
	markFile(d, g, inUnstaged+"a.txt")
	markFile(d, g, inUnstaged+"b.txt")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Discard the marked") })
	has(t, d, "Discard the changes to 2 files?", "a.txt", "b.txt", "can't be undone")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Discarded the changes to 2 files")
	if _, err := os.Stat(filepath.Join(r.Dir, "a.txt")); err == nil {
		t.Error("a.txt is still there")
	}
	if r.Read("README.md") != "changed\n" {
		t.Error("README.md, not marked, lost its change")
	}
	d.Press("!")
	has(t, d, "Staged README.md", "Staged a.txt", "Unstaged a.txt", "Deleted b.txt") // untracked: deleted
}

// TestRemoveAndIgnore: Remove deletes a tracked file and stages its
// deletion, asking first; Ignore adds a file to .gitignore, asking only when
// a tracked one is among them, which stops being tracked, the file kept.
func TestRemoveAndIgnore(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Write("feature.txt", "changed\n")
	r.Write("new*.txt", "new\n")
	g, d := start(t, r, 100, 32)
	d.Press("1")
	d.Do(func() tea.Cmd { g.status.files.Select(inUnstaged + "README.md"); return nil })
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Remove") })
	has(t, d, "Remove README.md from the repository?", "changes not committed are lost")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Removed README.md from the repository")
	if got := r.Git("status", "--porcelain", "--", "README.md"); got != "D  README.md\n" {
		t.Errorf("README.md is %q", got)
	}
	// An untracked file: no question.
	d.Do(func() tea.Cmd { g.status.files.Select(inUnstaged + "new*.txt"); return nil })
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Ignore") })
	done(t, g, d)
	has(t, d, "Added new*.txt to .gitignore")
	if got := r.Read(".gitignore"); got != "/new\\*.txt\n" {
		t.Errorf(".gitignore is %q", got)
	}
	// A tracked one, marked: asked, untracked, kept.
	markFile(d, g, inUnstaged+"feature.txt")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Ignore the marked") })
	has(t, d, "Add feature.txt to .gitignore?", "stop being tracked")
	d.Press("Enter")
	done(t, g, d)
	if got := r.Read(".gitignore"); got != "/new\\*.txt\n/feature.txt\n" {
		t.Errorf(".gitignore is %q", got)
	}
	if strings.Contains(r.Git("ls-files"), "feature.txt") {
		t.Error("feature.txt is still tracked")
	}
	if r.Read("feature.txt") != "changed\n" {
		t.Error("feature.txt was not kept")
	}
}

// TestXDiscardsMany: X discards the marked files when some are marked,
// asking first (tdp K13); with none marked it does nothing: Discard all has
// no key (the user).
func TestXDiscardsMany(t *testing.T) {
	r := gittest.Sample(t)
	r.Git("stash", "drop", "-q")
	r.Write("README.md", "changed\n")
	r.Write("feature.txt", "changed\n")
	g, d := start(t, r, 100, 32)
	d.Press("1")
	markFile(d, g, inUnstaged+"README.md")
	d.Press("X")
	has(t, d, "Discard the changes to README.md?")
	d.Press("Enter")
	done(t, g, d)
	if r.Read("feature.txt") != "changed\n" {
		t.Error("X with a mark discarded a file not marked")
	}
	d.Press("X")
	hasNot(t, d, "Discard every change to the tracked files?")
}

// TestDiscardFile: Discard, from the Space menu (no key, the user), asks
// first; on a file git does not track it deletes it.
func TestDiscardFile(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Write("new.txt", "new\n")
	g, d := start(t, r, 100, 32)
	d.Press("1", "x")
	hasNot(t, d, "Discard the change to README.md?")
	d.Press("Space")
	has(t, d, "Discard")
	hasNot(t, d, "[x] Discard")
	d.Press("Esc")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Discard") })
	has(t, d, "Discard the change to README.md?", "can't be undone")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Discarded the change to README.md")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Discard") })
	has(t, d, "Delete new.txt?")
	d.Press("Enter")
	done(t, g, d)
	if _, err := os.Stat(filepath.Join(r.Dir, "new.txt")); !os.IsNotExist(err) {
		t.Error("new.txt is still there")
	}
}

// --- task 13: commit ---

// TestCommitMessage: c opens the commit form (the user; tdp form): Title, a
// one-line input, Description, a textarea, and the Commit button; with no
// title the button says so and the form stays; the message is the title, a
// blank line, then the description.
func TestCommitMessage(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Git("add", "README.md")
	g, d := start(t, r, 100, 32)
	d.Press("1", "c")
	has(t, d, "COMMIT", "Title", "not set", "Description", "none", "Commit", "Enter:edit Tab:actions Esc:cancel")
	d.Press("Tab", "Enter") // no title: an error, the form kept (tdp form.16)
	has(t, d, "Title is required")
	d.Press("Enter") // the focus on Title: its input
	d.Type("Change the readme")
	d.Press("Enter", "j", "Enter") // Description: a textarea
	d.Type("Why it changed")
	d.Press("Esc", "Enter")
	has(t, d, "Why it changed")
	d.Press("Tab", "Enter")
	done(t, g, d)
	has(t, d, "Committed ", "Change the readme")
	if got := strings.TrimSpace(r.Git("log", "-1", "--format=%B")); got != "Change the readme\n\nWhy it changed" {
		t.Errorf("last commit %q", got)
	}
}

func TestCommitOffWithNothingStaged(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 100, 32)
	d.Press("1", "c")
	hasNot(t, d, "Tab:actions")
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
	d.Press("1", "c", "Enter")
	d.Type("Blocked")
	d.Press("Enter", "Tab", "Enter")
	done(t, g, d)
	has(t, d, "COMMIT FAILED", "no commits today")
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	d.Press("Esc", "c")
	has(t, d, "Blocked")
	d.Press("Tab", "Enter") // the message kept: it commits
	done(t, g, d)
	has(t, d, "Committed ", "Blocked")
}

func TestAmendKeepsMessage(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Git("add", "README.md")
	g, d := start(t, r, 100, 32)
	d.Do(func() tea.Cmd { return runGlobal(t, g, "Amend") })
	has(t, d, "Amend the last commit?", "Say even more", "force push") // pushed: a warning first
	d.Press("Enter")
	has(t, d, "AMEND", "Say even more", "Amend") // its message, ready
	d.Press("Tab", "Enter")                      // unchanged: kept
	done(t, g, d)
	has(t, d, "Amended ")
	if got := strings.TrimSpace(r.Git("log", "-1", "--format=%s")); got != "Say even more" {
		t.Errorf("last commit %q", got)
	}
	if got := strings.TrimSpace(r.Git("show", "-s", "--format=%H", "HEAD:README.md")); got == "" {
		t.Error("no README.md")
	}
}

// TestAmendChangesMessage: an amend of a commit not pushed opens its message
// straight away, to change.
func TestAmendChangesMessage(t *testing.T) {
	r := gittest.Sample(t)
	r.Commit("local.txt", "x\n", "A local commit")
	r.Write("local.txt", "y\n")
	r.Git("add", "local.txt")
	g, d := start(t, r, 100, 32)
	d.Press("Space") // a menu under it: it closes with the form (tdp T1)
	d.Do(func() tea.Cmd { return runGlobal(t, g, "Amend") })
	hasNot(t, d, "force push")
	has(t, d, "AMEND", "A local commit")
	d.Press("Enter", "Tab") // Title's input; Tab takes the title it had (tdp input.5)
	d.Type(" reworded")
	d.Press("Enter", "Tab", "Enter")
	done(t, g, d)
	hasNot(t, d, "PANEL OPERATION")
	if got := strings.TrimSpace(r.Git("log", "-1", "--format=%s")); got != "A local commit reworded" {
		t.Errorf("last commit %q", got)
	}
}

// --- task 14: conflicts ---

func TestResolveHunks(t *testing.T) {
	r := gittest.Conflicted(t)
	g, d := start(t, r, 100, 32)
	d.Press("1")
	has(t, d, "CONFLICTS", "U notes.txt", "Enter:show r:resolved")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Resolve one by one") }) // from the Space menu
	has(t, d, "feature › notes.txt", "Conflict 1", "Conflict 2", "MINE · HEAD (main)", "THEIRS · feature", "B from main", "B from feature",
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
	has(t, d, "COMMIT", "Merge branch 'feature'") // the message git prepared, ready
	d.Press("Tab", "Enter")                       // as it is: it commits
	done(t, g, d)
	hasNot(t, d, "Merging:")
}

// TestEditConflict: Enter on a conflicted file goes to [2], Enter there
// opens it in the editor git would open (a stand-in here) in a terminal
// popup; when it ends the file is checked: markers left are a Warning naming
// their lines, none an Info saying r marks it resolved; r on [2] stages it
// (the user; tdp terminal, textarea.1).
func TestEditConflict(t *testing.T) {
	r := gittest.Conflicted(t)
	dir := t.TempDir()
	editor, runs := filepath.Join(dir, "editor"), filepath.Join(dir, "runs")
	// The first run leaves the file as it is, the second resolves it.
	script := "#!/bin/sh\necho run >> " + runs + "\n" +
		"if [ $(wc -l < " + runs + ") -ge 2 ]; then printf 'a\\nB\\nc\\nd\\ne\\nx1\\nx2\\nx3\\nx4\\nF\\ng\\n' > \"$1\"; fi\n"
	if err := os.WriteFile(editor, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_EDITOR", editor)
	g, d := start(t, r, 100, 32)
	d.Press("1")
	has(t, d, "U notes.txt", "Enter:show r:resolved")
	d.Press("Enter")
	if g.a.Screen().Focused() != g.status.diffP {
		t.Fatal("Enter on a conflicted file did not go to [2]")
	}
	has(t, d, "Enter:edit r:resolved")
	d.Press("Enter")
	d.WaitFor(func() bool { return d.Has("still has conflicts on") }, 5*time.Second)
	has(t, d, "notes.txt still has conflicts on lines 2–6, 14–18")
	d.Press("Esc", "Enter")
	d.WaitFor(func() bool { return d.Has("no conflicts left") }, 5*time.Second)
	has(t, d, "notes.txt has no conflicts left: press [r] to mark it resolved")
	d.Press("r")
	done(t, g, d)
	has(t, d, "Marked notes.txt resolved", "STAGED")
	if out := r.Git("diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(out) != "" {
		t.Errorf("still unmerged: %s", out)
	}
	d.Press("!")
	has(t, d, "Editing notes.txt")
}

// TestEditorPausesRefresh: while gittu's editor is open the repository is not
// read again, so its swap file is not logged as changed outside gittu; nor is
// the file it saved, read when it ends (the user).
func TestEditorPausesRefresh(t *testing.T) {
	r := gittest.Conflicted(t)
	dir := t.TempDir()
	editor, goOn := filepath.Join(dir, "editor"), filepath.Join(dir, "go-on")
	swap := filepath.Join(r.Dir, ".notes.txt.swp")
	script := "#!/bin/sh\ntouch " + swap + "\necho editing\n" + // the output wakes WaitFor
		"while [ ! -f " + goOn + " ]; do sleep 0.05; done\n" +
		"rm " + swap + "\nprintf 'a\\nB\\nc\\n' > \"$1\"\n"
	if err := os.WriteFile(editor, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_EDITOR", editor)
	g, d := start(t, r, 100, 32)
	d.Press("1", "Enter", "Enter")
	d.WaitFor(func() bool { _, err := os.Stat(swap); return err == nil }, 5*time.Second)
	d.Advance(refreshEvery + time.Second)
	if g.reading {
		t.Error("read again while the editor is open")
	}
	if err := os.WriteFile(goOn, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	d.WaitFor(func() bool { return d.Has("no conflicts left") }, 5*time.Second)
	d.WaitFor(func() bool { return !g.reading }, 5*time.Second)
	d.Advance(refreshEvery + time.Second)
	d.WaitFor(func() bool { return !g.reading }, 5*time.Second)
	d.Press("!")
	has(t, d, "Editing notes.txt")
	hasNot(t, d, "Changed outside gittu")
}

// TestMarkResolvedAsks: Mark [r]esolved (so the Space menu writes it, the
// user; tdp main M5-1)
// on a file with conflict markers left asks first, naming their lines: they
// would be staged as they are.
func TestMarkResolvedAsks(t *testing.T) {
	r := gittest.Conflicted(t)
	g, d := start(t, r, 100, 32)
	d.Press("1", "Space")
	has(t, d, "Mark [r]esolved")
	hasNot(t, d, "Ma[r]k")
	d.Press("Esc", "r")
	has(t, d, "Mark notes.txt resolved?", "notes.txt: conflicts on lines 2–6, 14–18")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Marked notes.txt resolved")
}

// TestMarkTheMarkedResolved: Mark the marked resolved, from the Space menu
// (no key, the user), stages the marked conflicted files, asking first when
// one still has markers (tdp P3).
func TestMarkTheMarkedResolved(t *testing.T) {
	r := gittest.New(t)
	r.Git("commit", "-q", "--allow-empty", "-m", "Start")
	r.Commit("a.txt", "a\n", "Add a")
	r.Commit("b.txt", "b\n", "Add b")
	r.Git("checkout", "-q", "-b", "feature")
	r.Write("a.txt", "a from feature\n")
	r.Commit("b.txt", "b from feature\n", "Change both on feature")
	r.Git("checkout", "-q", "main")
	r.Write("a.txt", "a from main\n")
	r.Commit("b.txt", "b from main\n", "Change both on main")
	if _, err := r.Try("merge", "--no-edit", "feature"); err == nil {
		t.Fatal("no conflict")
	}
	r.Write("a.txt", "a resolved\n")
	g, d := start(t, r, 100, 32)
	d.Press("1")
	markFile(d, g, inConflict+"a.txt")
	markFile(d, g, inConflict+"b.txt")
	d.Press("Space")
	has(t, d, "Mark the marked resolved")
	hasNot(t, d, "[R]")
	d.Press("Esc")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Mark the marked resolved") })
	has(t, d, "Mark 2 files resolved?", "b.txt: conflicts on lines 1–5")
	hasNot(t, d, "a.txt: conflicts")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "Marked 2 files resolved")
	if out := r.Git("diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(out) != "" {
		t.Errorf("still unmerged: %s", out)
	}
	d.Press("!")
	has(t, d, "Marked a.txt resolved", "Marked b.txt resolved")
}

func TestUseSideWholeFile(t *testing.T) {
	r := gittest.Conflicted(t)
	g, d := start(t, r, 100, 32)
	d.Press("1")
	d.Press("Space")
	has(t, d, "Use mine", "Use theirs") // no keys (the user)
	hasNot(t, d, "Use [m]ine", "Use [t]heirs")
	d.Press("Esc")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Use mine") })
	done(t, g, d)
	has(t, d, "notes.txt resolved with mine and staged")
	if !strings.Contains(r.Read("notes.txt"), "B from main") {
		t.Error("not mine")
	}
}

// TestUseSideAsksWhenChanged: Use theirs on a file saved after the conflict
// asks first: what was done in it is lost (the user). One git wrote is not
// asked about (TestUseSideWholeFile).
func TestUseSideAsksWhenChanged(t *testing.T) {
	r := gittest.Conflicted(t)
	r.Write("notes.txt", strings.Replace(r.Read("notes.txt"), "<<<<<<< HEAD\nB from main", "B by hand\n<<<<<<< HEAD\nB from main", 1))
	ago := time.Now().Add(-time.Minute)
	if err := os.Chtimes(filepath.Join(r.Dir, ".git", "MERGE_HEAD"), ago, ago); err != nil {
		t.Fatal(err)
	}
	g, d := start(t, r, 100, 32)
	d.Press("1")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Use theirs") })
	has(t, d, "Use theirs for notes.txt?", "notes.txt: changed after the conflict",
		"The whole file becomes theirs: any change made to it after the conflict is lost.")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "notes.txt resolved with theirs and staged")
	if strings.Contains(r.Read("notes.txt"), "B by hand") {
		t.Error("not theirs")
	}
}

// TestUseSideAsksAfterAStash: a conflict from applying a stash leaves
// nothing to tell a change by, so Use mine for the marked asks.
func TestUseSideAsksAfterAStash(t *testing.T) {
	r := gittest.New(t)
	r.Commit("a.txt", "a\n", "Add a")
	r.Write("a.txt", "a stashed\n")
	r.Git("stash", "-q")
	r.Commit("a.txt", "a committed\n", "Change a")
	if _, err := r.Try("stash", "pop"); err == nil {
		t.Fatal("no conflict")
	}
	g, d := start(t, r, 100, 32)
	d.Press("1")
	markFile(d, g, inConflict+"a.txt")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Use mine for the marked") })
	has(t, d, "Use mine for a.txt?", "a.txt: gittu can't tell whether it was changed after the conflict")
	d.Press("Enter")
	done(t, g, d)
	has(t, d, "1 file resolved with mine and staged")
	if r.Read("a.txt") != "a committed\n" {
		t.Errorf("not mine: %q", r.Read("a.txt"))
	}
}

// TestUseSideForTheMarked: the marked conflicted files resolved with one
// side at once.
func TestUseSideForTheMarked(t *testing.T) {
	r := gittest.New(t)
	r.Git("commit", "-q", "--allow-empty", "-m", "Start")
	r.Commit("a.txt", "a\n", "Add a")
	r.Commit("b.txt", "b\n", "Add b")
	r.Git("checkout", "-q", "-b", "feature")
	r.Write("a.txt", "a from feature\n")
	r.Commit("b.txt", "b from feature\n", "Change both on feature")
	r.Git("checkout", "-q", "main")
	r.Write("a.txt", "a from main\n")
	r.Commit("b.txt", "b from main\n", "Change both on main")
	if _, err := r.Try("merge", "--no-edit", "feature"); err == nil {
		t.Fatal("no conflict")
	}
	g, d := start(t, r, 100, 32)
	d.Press("1")
	markFile(d, g, inConflict+"a.txt")
	markFile(d, g, inConflict+"b.txt")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Use theirs for the marked") })
	done(t, g, d)
	has(t, d, "2 files resolved with theirs and staged")
	if r.Read("a.txt") != "a from feature\n" || r.Read("b.txt") != "b from feature\n" {
		t.Errorf("not theirs: %q %q", r.Read("a.txt"), r.Read("b.txt"))
	}
}

func TestAbortMerge(t *testing.T) {
	r := gittest.Conflicted(t)
	g, d := start(t, r, 100, 32)
	d.Press("A")
	has(t, d, "Abort the merge?", "Everything goes back to how it was before the merge; conflicts you have")
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
	d.Press("1")
	d.Do(func() tea.Cmd { return runLabel(t, g, g.status.filesP, "Use mine") })
	done(t, g, d)
	d.Do(func() tea.Cmd { return runGlobal(t, g, "Continue rebase") })
	done(t, g, d)
	has(t, d, "Went on with the rebase")
	hasNot(t, d, "Rebasing onto")
}

// --- task 15: the setting ---

func TestTabWidthSetting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITTU__CONFIG", dir)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("input_tab_width: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, _ := git.Open(gittest.Sample(t).Dir)
	g := newGittu(repo)
	tdptest.Start(t, g.a, 80, 32)
	if g.a.InputTabWidth != 2 {
		t.Errorf("tab width %d", g.a.InputTabWidth)
	}
}

func TestConfigMisfit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITTU__CONFIG", dir)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("input_tab_width: -3\nother: kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, _ := git.Open(gittest.Sample(t).Dir)
	g := newGittu(repo)
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

// TestConfigUnreadKey: a key gittu never read stays in the file, no toast,
// an INFO at every start (tdp E3).
func TestConfigUnreadKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITTU__CONFIG", dir)
	path := filepath.Join(dir, "config.yaml")
	want := []byte("refesh: 5\ninput_tab_width: 2\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		repo, _ := git.Open(gittest.Sample(t).Dir)
		g := newGittu(repo)
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
	t.Setenv("GITTU__CONFIG", dir)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("input_tab_width: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, _ := git.Open(gittest.Sample(t).Dir)
	g := newGittu(repo)
	d := tdptest.Start(t, g.a, 100, 32)
	has(t, d, "MOVE ASIDE", "can't be read")
	d.Press("Enter")
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("no config.yaml.bak: %v", err)
	}
}

// --- task 16: the splash ---

// TestSplashIsDocsIcon is tdp E6: the splash draws docs/icon.svg, gittu's
// family mark; the copy go:embed reads is the same file.
func TestSplashIsDocsIcon(t *testing.T) {
	want, err := os.ReadFile("../../docs/icon.svg")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(icon, want) {
		t.Fatal("internal/ui/icon.svg is not docs/icon.svg: copy docs/icon.svg over it")
	}
	// gittu: G I T T inside the U.
	for _, letter := range []string{"<!-- U -->", "<!-- G -->", "<!-- I -->", "<!-- T -->"} {
		if !bytes.Contains(icon, []byte(letter)) {
			t.Errorf("the mark has no %s", letter)
		}
	}
}

func TestSplashOpens(t *testing.T) {
	_, d := start(t, gittest.Sample(t), 100, 40)
	d.Press("V")
	d.Advance(3 * time.Second)
	has(t, d, "gittu", "A git client for the terminal", "vulcan.shen.2304@gmail.com")
	d.Press("q")
	if d.Quit() {
		t.Error("a key on the splash quit")
	}
	has(t, d, "[2] BRANCHES")
}

// --- task 17: zoom ---

// leftWidth is the left's width on the screen, [1] and [2]: where [3]'s
// frame starts; 0 when they are not drawn.
func leftWidth(d *tdptest.Driver) int {
	row := d.Screen()[0]
	if strings.HasPrefix(row, "┏") && strings.Count(row, "┏") == 1 && !strings.ContainsAny(row, "┌┐") {
		return 0 // one panel across the whole area
	}
	for _, corner := range []string{"┐┌", "┐┏", "┓┌", "┓┏"} {
		if i := strings.Index(row, corner); i >= 0 {
			return terminu.Width(row[:i]) + 1
		}
	}
	return -1
}

// TestZoomToTheWholeScreen: z on [3] puts it on the whole screen at once,
// and z again back (the user, 2026-11-23: one step).
func TestZoomToTheWholeScreen(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 120, 32)
	if w := leftWidth(d); w != 40 {
		t.Fatalf("[1] is %d wide before zooming", w)
	}
	d.Press("Tab", "z")
	if w := leftWidth(d); w != 0 || !d.Has("[3] CURRENT") || d.Has("[2] BRANCHES") {
		t.Fatalf("z did not put [3] on the whole screen (%d):\n%s", w, strings.Join(d.Screen(), "\n"))
	}
	has(t, d, "z:unzoom")
	d.Press("Tab")
	if !d.Has("[4] DESCRIPTION") || d.Has("[3] CURRENT") || d.Has("[5] FILES") {
		t.Fatalf("Tab on the whole screen does not show [4] alone:\n%s", strings.Join(d.Screen(), "\n"))
	}
	has(t, d, "z:unzoom") // put back from [4] too
	d.Press("Tab")
	if !d.Has("[5] FILES") || d.Has("[3] CURRENT") || d.Has("[4] DESCRIPTION") {
		t.Fatalf("Tab on the whole screen does not show [5] alone:\n%s", strings.Join(d.Screen(), "\n"))
	}
	d.Press("Tab") // round to [1], which leads: [3] the diff
	if !d.Has("[1] CHANGES") || d.Has("[2] BRANCHES") || d.Has("[3] DIFF") {
		t.Fatalf("Tab on the whole screen does not show [1] alone:\n%s", strings.Join(d.Screen(), "\n"))
	}
	d.Press("z")
	if w := leftWidth(d); w != 40 || !d.Has("[3] DIFF") {
		t.Fatalf("z on [1] did not put the panels back: the left is %d wide", w)
	}
	if g.history.zoomed {
		t.Error("still zoomed")
	}
	d.Press("!")
	has(t, d, "Zoomed [3] to the whole screen", "Back to both panels")
}

// TestDescriptionZooms: [4] DESCRIPTION, narrower than [5] FILES (2:3, the
// user), zooms to the whole screen with z, and back.
func TestDescriptionZooms(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 100, 32)
	d.Press("4")
	has(t, d, "z:zoom")
	d.Press("z")
	has(t, d, "[4] DESCRIPTION", "z:unzoom")
	hasNot(t, d, "[3] CURRENT", "[5] FILES")
	d.Press("z", "!")
	has(t, d, "Zoomed [4] to the whole screen", "Back to both panels")
	d.Press("Esc")
	if r := g.layout(100, 30); r[placeLeft].W != 26 || r[placeRight].W != 41 {
		t.Errorf("[3] %d wide, [4] %d: not 2:3 of 67", r[2].W, r[3].W)
	}
}

// TestFilesLocationFitsAt60: at 60 columns [4] still has room for the two
// short hashes of its location row (the user).
func TestFilesLocationFitsAt60(t *testing.T) {
	g, d := start(t, gittest.Sample(t), 60, 24)
	loc := g.history.files.Location
	if loc == "" {
		t.Fatal("no location in [4]")
	}
	has(t, d, loc)
}

// --- task 23: the left's width ---

// TestSplitKeys: < and > move the line between the left and the right by
// two columns, whichever panel has the focus, and keep it in the state.
func TestSplitKeys(t *testing.T) {
	dir := t.TempDir()
	g, d := startWithState(t, gittest.Sample(t), dir, 120, 32)
	d.Press(">")
	if w := leftWidth(d); w != 42 {
		t.Fatalf("> made the left %d wide, want 42", w)
	}
	d.Press("3", "<", "<") // on the right: the same way
	if w := leftWidth(d); w != 38 {
		t.Fatalf("< < on [3] made the left %d wide, want 38", w)
	}
	if r := g.layout(120, 31); r[placeChanges].W != 38 || r[placeMain].X != 38 || r[placeLeft].W != 82*2/5 {
		t.Errorf("rects %+v", r)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "state.yaml"))
	if !bytes.Contains(b, []byte("120: 38")) {
		t.Errorf("state.yaml:\n%s", b)
	}
	d.Press("Space", "k", "Enter") // the global row, then the global operation popup
	has(t, d, "[<] Narrow the left", "[>] Widen the left")
	d.Press("Esc", "Esc", "!")
	has(t, d, "Left 42 of 120 columns", "Left 38 of 120 columns")
}

// TestSplitKeptByWidth: the width set for a terminal is used when gittu opens
// on a terminal that wide again; another width has a third (the user: A).
func TestSplitKeptByWidth(t *testing.T) {
	dir := t.TempDir()
	r := gittest.Sample(t)
	_, d := startWithState(t, r, dir, 120, 32)
	d.Press(">", ">", ">")
	_, d = startWithState(t, r, dir, 120, 32)
	if w := leftWidth(d); w != 46 {
		t.Errorf("opened again at 120 columns: the left %d wide, want 46", w)
	}
	_, d = startWithState(t, r, dir, 100, 32)
	if w := leftWidth(d); w != 33 {
		t.Errorf("at 100 columns: the left %d wide, want a third", w)
	}
}

// TestSplitFollowsTheTerminal: the terminal made wider or narrower while gittu
// is open takes the width kept for its new width.
func TestSplitFollowsTheTerminal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "state.yaml"), []byte("left:\n  100: 50\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, d := startWithState(t, gittest.Sample(t), dir, 120, 32)
	if w := leftWidth(d); w != 40 {
		t.Fatalf("at 120 columns the left is %d wide", w)
	}
	d.Resize(100, 32)
	if w := leftWidth(d); w != 50 {
		t.Errorf("at 100 columns the left is %d wide, want 50", w)
	}
	d.Resize(120, 32)
	if w := leftWidth(d); w != 40 {
		t.Errorf("back at 120 columns the left is %d wide", w)
	}
}

// TestSplitAtItsEnds: the left stops at 20 columns and the right at 40; at 60
// columns neither moves; zoomed, neither does.
func TestSplitAtItsEnds(t *testing.T) {
	dir := t.TempDir()
	_, d := startWithState(t, gittest.Sample(t), dir, 80, 32)
	d.Press("<", "<", "<", "<")
	if w := leftWidth(d); w != leftLeast {
		t.Errorf("the left went to %d", w)
	}
	for range 12 {
		d.Press(">")
	}
	if w := leftWidth(d); w != 80-rightLeast {
		t.Errorf("the right went to %d", 80-w)
	}
	_, d = startWithState(t, gittest.Sample(t), t.TempDir(), 60, 32)
	d.Press("<", ">")
	if w := leftWidth(d); w != 20 {
		t.Errorf("at 60 columns the left went to %d", w)
	}
	d.Press("Tab", "z", ">", "z")
	if w := leftWidth(d); w != 20 {
		t.Errorf("> while zoomed made the left %d wide", w)
	}
}

// TestSplitDisabledWhenZoomed: zoomed, < and > do nothing, and say so in the
// global operation popup (tdp M6).
func TestSplitDisabledWhenZoomed(t *testing.T) {
	dir := t.TempDir()
	g, d := startWithState(t, gittest.Sample(t), dir, 120, 32)
	d.Press("Tab", "z", ">", "<", "z")
	if w := leftWidth(d); w != 40 {
		t.Errorf("the left is %d wide after < and > zoomed", w)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.yaml")); err == nil {
		t.Error("state.yaml written while zoomed")
	}
	g.history.zoomed = true
	for _, a := range g.splitActions() {
		if !a.Disabled() {
			t.Errorf("%s is on while zoomed", a.Label)
		}
	}
}

func TestStateMisfit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	if err := os.WriteFile(path, []byte("left:\n  120: 50\n  100: 90\n  abc: 3\nother: kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, d := startWithState(t, gittest.Sample(t), dir, 120, 32)
	has(t, d, "did not fit", "state.yaml.bak")
	if w := leftWidth(d); w != 50 {
		t.Errorf("the left is %d wide, want 50", w)
	}
	b, _ := os.ReadFile(path)
	if !bytes.Contains(b, []byte("120: 50")) || !bytes.Contains(b, []byte("other: kept")) || bytes.Contains(b, []byte("90")) || bytes.Contains(b, []byte("abc")) {
		t.Errorf("state.yaml:\n%s", b)
	}
	if bak, _ := os.ReadFile(path + ".bak"); !bytes.Contains(bak, []byte("100: 90")) {
		t.Errorf("state.yaml.bak:\n%s", bak)
	}
	d.Press("!")
	has(t, d, `left.100 "90" did not fit; using "33"`, "left.abc did not fit; taken out")
}

// TestStateCannotBeRead: config.yaml and state.yaml both broken ask one after
// the other, config.yaml first (tdp E3).
func TestStateCannotBeRead(t *testing.T) {
	cfg, dir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(cfg, "config.yaml"), []byte("input_tab_width: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.yaml")
	if err := os.WriteFile(path, []byte("left: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITTU__CONFIG", cfg)
	t.Setenv("GITTU__STATE", dir)
	repo, _ := git.Open(gittest.Sample(t).Dir)
	g := newGittu(repo)
	d := tdptest.Start(t, g.a, 120, 32)
	has(t, d, "MOVE ASIDE", "config.yaml")
	d.Press("Enter")
	has(t, d, "MOVE ASIDE", "state.yaml")
	d.Press("Enter")
	hasNot(t, d, "MOVE ASIDE")
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("no state.yaml.bak: %v", err)
	}
	if w := leftWidth(d); w != 40 {
		t.Errorf("the left is %d wide", w)
	}
	d.Press(">")
	if b, _ := os.ReadFile(path); !bytes.Contains(b, []byte("120: 42")) {
		t.Errorf("state.yaml:\n%s", b)
	}
}

// TestStateUnreadKey: a key gittu never read stays in the file, no toast, an
// INFO at every start (tdp E3).
func TestStateUnreadKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	want := []byte("refesh: 5\nleft:\n  120: 44\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	_, d := startWithState(t, gittest.Sample(t), dir, 120, 32)
	hasNot(t, d, "did not fit")
	if w := leftWidth(d); w != 44 {
		t.Errorf("the left is %d wide, want 44", w)
	}
	d.Press(">")
	b, _ := os.ReadFile(path)
	if !bytes.Contains(b, []byte("refesh: 5")) || !bytes.Contains(b, []byte("120: 46")) {
		t.Errorf("state.yaml:\n%s", b)
	}
	d.Press("!")
	has(t, d, "Not read: refesh in ")
}

// TestStateCannotBeWritten: a state directory gittu can't write in gives an
// Error toast; the left is moved all the same.
func TestStateCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	_, d := startWithState(t, gittest.Sample(t), dir, 120, 32)
	d.Press(">")
	has(t, d, "Can't write state.yaml")
	if w := leftWidth(d); w != 42 {
		t.Errorf("the left is %d wide, want 42", w)
	}
}
