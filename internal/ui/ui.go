// Package ui is gittu's screens: History (the branches and the history graph,
// as SourceTree lays them out) and File status (the working tree's files and
// their changes), every git operation in their Space menus.
package ui

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/gittu/internal/git"
	"github.com/vulcanshen/gittu/internal/version"
)

// icon is gittu's family mark (tdp S5), drawn by the splash: a copy of
// docs/icon.svg, which go:embed can't reach from here (a test holds the two
// the same, tdp E6).
//
//go:embed icon.svg
var icon []byte

// Glyphs of gittu's own popups (tdp note.2: a viewer's glyph is the app's),
// and of the refs in the graph (tdp P5: content's symbols are the app's).
const (
	glyphRefTag = "\U000F04F9" // nf-md-tag
	glyphGone   = "\U000F0165" // nf-md-cloud_off_outline: a branch whose upstream is gone
	glyphBranch = ""          // nf-dev-git_branch
	glyphTag    = ""          // nf-fa-tag
	glyphCommit = ""          // nf-dev-git_commit
	glyphStash  = "\U000F01BC" // nf-md-database
)

// The colours gittu gives content of its own (tdp P5), catppuccin-mocha.
var (
	mauve    = terminu.Hex("#cba6f7")
	teal     = terminu.Hex("#94e2d5")
	pink     = terminu.Hex("#f5c2e7")
	flamingo = terminu.Hex("#f2cdcd")
	// lineColours are the history graph's lines, one each, in turn.
	lineColours = []terminu.Color{terminu.Blue, terminu.Green, mauve, terminu.Peach, teal, pink, terminu.Sapphire, flamingo}
)

// refreshEvery is how often the repository is read again by itself, so that
// what is changed outside gittu shows (task 19; tdp K3: it moves nothing).
const refreshEvery = 3 * time.Second

// gittu is the app and the repository it shows.
type gittu struct {
	a    *terminu.App
	repo *git.Repo
	snap *git.Snapshot
	// all is the graph's main line, the first column: main (else master) when
	// set, HEAD otherwise (SourceTree's All and Current branches).
	all bool
	// remote reports the remote branches drawn in the graph too.
	remote bool
	// busy is what git is doing now, for the statusbar ("Pushing…"); "" when
	// nothing runs. Operations that change the repository wait meanwhile.
	busy string
	// reading reports a read of the repository on its way.
	reading bool
	// readErr is why the last read failed, "" when it did not.
	readErr string
	// draft is a commit message written but not committed yet (a hook
	// stopped it), to write again.
	draft string

	history *historyScreen
	status  *statusScreen
}

// New makes gittu on repo.
func New(repo *git.Repo) *terminu.App { return newGittu(repo).a }

func newGittu(repo *git.Repo) *gittu {
	g := &gittu{a: terminu.New("gittu"), repo: repo, all: true}
	g.a.Splash = splash()
	g.a.QuitFlow = g.quit
	g.a.Status = g.statusbar
	g.history = newHistoryScreen(g)
	g.status = newStatusScreen(g)
	g.a.AddScreen(g.history.screen)
	g.a.AddScreen(g.status.screen)
	g.a.Global = g.globalActions()
	g.a.OnMsg = g.onMsg
	g.a.Start = g.start
	g.a.InputTabWidth = 4
	snap, err := repo.Read(g.remote)
	g.show(snap, err)
	if err != nil {
		g.readFailed(err) // the panels say why; the App Log has it
	}
	return g
}

// splash is the family's easter egg, V on a panel (tdp S).
func splash() terminu.Splash {
	s, err := terminu.NewSplash(icon, "gittu", version.Display(), "A git client laid out like SourceTree")
	if err != nil {
		panic(err) // the embedded icon is broken: a mistake in the program
	}
	return s
}

// start reads the settings and starts reading the repository again now and
// then.
func (g *gittu) start() tea.Cmd {
	return tea.Batch(g.loadConfig(), g.tick())
}

type tickMsg struct{}

func (g *gittu) tick() tea.Cmd {
	return g.a.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

// readMsg is a read of the repository done.
type readMsg struct {
	snap *git.Snapshot
	err  error
	// asked reports a read the user asked for (Refresh), to log.
	asked bool
}

// doneMsg is an operation done, with the repository read after it.
type doneMsg struct {
	title string // the operation, for its error popup: "Push"
	err   error
	snap  *git.Snapshot
	rerr  error
	ok    func() tea.Cmd // what to say when it went well
	// conflicts is what to say when it stopped on conflicts.
	conflicts func(n int) tea.Cmd
	// failed, when set, handles the error instead of the error popup.
	failed func(error) tea.Cmd
}

func (g *gittu) onMsg(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tickMsg:
		if g.busy == "" && !g.reading {
			g.reading = true
			return tea.Batch(g.read(false), g.tick())
		}
		return g.tick()
	case readMsg:
		g.reading = false
		if g.busy != "" {
			return nil // an operation is on its way: its own read comes after it
		}
		was, old := g.readErr, g.snap
		g.show(m.snap, m.err)
		reread := g.status.takeReread()
		if !m.asked && m.err == nil {
			// What changed outside gittu since the last read: its own
			// operations read the repository as they end (task 19).
			if what := changes(old, m.snap, reread); len(what) > 0 {
				g.a.Log(terminu.Info, "Changed outside gittu: "+said(what))
			}
		}
		if m.asked && m.err == nil {
			return g.a.Toast(terminu.Info, "Read "+g.repo.Name()+" again")
		}
		if m.err != nil && (m.asked || g.readErr != was) {
			return g.readFailed(m.err)
		}
		return g.status.said()
	case doneMsg:
		g.busy = ""
		was := g.readErr
		g.show(m.snap, m.rerr)
		g.status.takeReread() // gittu's own change
		var c *git.Conflicts
		switch {
		case m.err == nil && m.rerr != nil && g.readErr != was:
			return g.readFailed(m.rerr)
		case m.err == nil:
			if m.ok != nil {
				return tea.Batch(m.ok(), g.status.said())
			}
			return g.status.said()
		case errors.As(m.err, &c) && m.conflicts != nil:
			return m.conflicts(c.Files)
		case m.failed != nil:
			return m.failed(m.err)
		default:
			return g.fail(m.title, "", m.err)
		}
	}
	return nil
}

// read reads the repository in the background.
func (g *gittu) read(asked bool) tea.Cmd {
	repo, remote := g.repo, g.remote
	return func() tea.Msg {
		snap, err := repo.Read(remote)
		return readMsg{snap: snap, err: err, asked: asked}
	}
}

// run runs an operation in the background while the statusbar says doing,
// then reads the repository and shows it. ok says how it went; title names
// it in its error popup.
func (g *gittu) run(doing, title string, op func() error, ok func() tea.Cmd) tea.Cmd {
	return g.runWith(doneMsg{title: title, ok: ok}, doing, op)
}

// runWith is run with what doneMsg says about conflicts and failures.
func (g *gittu) runWith(m doneMsg, doing string, op func() error) tea.Cmd {
	g.busy = doing
	repo, remote := g.repo, g.remote
	return func() tea.Msg {
		m.err = op()
		m.snap, m.rerr = repo.Read(remote)
		return m
	}
}

// idle reports that nothing runs: an operation that changes the repository is
// off while one does (tdp M6).
func (g *gittu) idle() bool { return g.busy == "" }

// show puts a read of the repository on every panel. A read that failed
// leaves what was read before (tdp panel.15); with nothing read yet the
// panels say why.
func (g *gittu) show(snap *git.Snapshot, err error) {
	if err != nil {
		g.readErr = reason(gitLines(err))
		if g.snap == nil {
			g.history.failed()
		}
		return
	}
	if g.readErr != "" { // it failed before: one INFO, no toast (tdp note.7, note-105)
		g.a.Log(terminu.Info, "The repository can be read again")
	}
	g.readErr = ""
	g.snap = snap
	g.history.show(snap)
	g.status.show(snap)
}

// conflictsSaid is the toast of an operation that stopped on conflicts.
func (g *gittu) conflictsSaid(what string) func(int) tea.Cmd {
	return func(n int) tea.Cmd {
		return g.a.Toast(terminu.Warning, what+" stopped on conflicts in "+plural(n, "file", "files"),
			"Resolve them in Working tree [W]")
	}
}

// statusbar is the right of the screen chips (tdp screen.4): where HEAD is,
// the repository (both Lavender: where you are), and the work in progress
// (Green): what git is doing, else an operation stopped half way.
//
// The lib draws it on the right, so a longer text moves all of it: it is
// always the w cells given, its fields from the left and blanks after them,
// the branch's field as wide as the terminal allows, not as the name is (tdp
// L2). The repository's name does not change while gittu runs; the operation
// and what git does are the last fields.
func (g *gittu) statusbar(w int) []terminu.Span {
	quiet := terminu.Fg(terminu.Overlay0)
	here := terminu.Fg(terminu.Lavender)
	repo := g.repo.Name()
	var spans []terminu.Span
	used := 0
	add := func(text string, st terminu.Style) {
		if text != "" {
			spans = append(spans, terminu.Span{Text: text, Style: st})
			used += terminu.Width(text)
		}
	}
	field := min(max((w-len("Branch: ")-len("  Repo: ")-terminu.Width(repo))/2, 8), 28)
	name, t := "", ""
	if g.snap != nil {
		name, t = g.where(), track(g.snap.Ahead, g.snap.Behind, false)
	}
	nameW := field
	if t != "" {
		nameW = field - 1 - terminu.Width(t)
		if nameW < 4 { // no room for both: the name alone
			t, nameW = "", field
		}
	}
	name = terminu.Truncate(name, nameW)
	add("Branch: ", quiet)
	add(name, here)
	if t != "" {
		add(" "+t, quiet)
	}
	add(strings.Repeat(" ", max(len("Branch: ")+field-used, 0)), terminu.Style{})
	add("  Repo: ", quiet)
	add(repo, here)
	// The work in progress is one field, the last (tdp L2, screen.4): what
	// git is doing while it runs, else an operation stopped half way.
	switch {
	case g.busy != "":
		add("  "+g.busy, terminu.Fg(terminu.Green))
	case g.snap != nil && g.snap.State.Op != "":
		st := g.snap.State
		add("  "+stateWord(st.Op)+": "+st.Other, terminu.Fg(terminu.Green))
	}
	add(strings.Repeat(" ", max(w-used, 0)), terminu.Style{})
	return spans
}

// where is the branch HEAD is on, or where it is detached.
func (g *gittu) where() string {
	switch {
	case g.snap.Head != "":
		return g.snap.Head
	case g.snap.HeadSHA != "":
		return "detached at " + git.Short(g.snap.HeadSHA)
	}
	return "no commits yet"
}

// stateWord is an operation stopped half way, as the statusbar says it.
func stateWord(op string) string {
	switch op {
	case "merge":
		return "Merging"
	case "rebase":
		return "Rebasing onto"
	case "cherry-pick":
		return "Cherry-picking"
	case "revert":
		return "Reverting"
	}
	return op
}

// track is a branch's commits ahead of and behind its upstream: ↑1 ↓2, only
// what is not 0; "upstream gone" when it is no longer there (git's [gone]).
func track(ahead, behind int, gone bool) string {
	if gone {
		return "upstream gone"
	}
	var parts []string
	if ahead > 0 {
		parts = append(parts, "↑"+strconv.Itoa(ahead))
	}
	if behind > 0 {
		parts = append(parts, "↓"+strconv.Itoa(behind))
	}
	return strings.Join(parts, " ")
}

// quit is the quit flow (tdp K9): at once, but asking first while git runs,
// as leaving would cut it short.
func (g *gittu) quit() tea.Cmd {
	if g.busy == "" {
		return tea.Quit
	}
	return g.a.Confirm(terminu.Ask{Title: "Quit", Question: "Quit gittu?", Verb: "quit",
		Details: []string{"git is still at work: " + g.busy}, Warn: []int{0}}, func() tea.Cmd { return tea.Quit })
}

// fail opens the error popup of a git operation that failed: the line that
// says why, git's whole text in the App Log (tdp components.5, F5).
func (g *gittu) fail(title, subject string, err error) tea.Cmd {
	lines := gitLines(err)
	say := []string{reason(lines)}
	if needsCredentials(lines) {
		say = append(say, "gittu can't ask for a password: use an ssh agent or a credential helper")
	}
	cmd := g.a.ErrorAbout(title+" failed", subject, say...)
	g.logAll(lines)
	return cmd
}

// readFailed reports a read that failed: with what was read before staying,
// an Error toast with the line that says why; with nothing read yet the
// panels say it. git's whole text goes in the App Log (tdp panel.15,
// components.5, F5).
func (g *gittu) readFailed(err error) tea.Cmd {
	lines := gitLines(err)
	if g.snap == nil { // nothing read yet: the panels say why (tdp panel.15)
		g.a.Log(terminu.Error, "Can't read "+g.repo.Name()+": "+reason(lines))
		g.logAll(lines)
		return nil
	}
	cmd := g.a.Toast(terminu.Error, "Can't read "+g.repo.Name()+": "+reason(lines), "Press [R] to read it again")
	g.logAll(lines)
	return cmd
}

// logAll writes git's text, more than the one line shown, in the App Log.
func (g *gittu) logAll(lines []string) {
	if len(lines) < 2 {
		return
	}
	for _, l := range lines {
		g.a.Log(terminu.Error, l)
	}
}

// gitLines are what git printed for an error, else the error's own lines.
func gitLines(err error) []string {
	var e *git.Error
	if errors.As(err, &e) && len(e.Lines) > 0 {
		return e.Lines
	}
	return git.Lines(err.Error())
}

// reason is the line of git's text that says why: the first fatal: or
// error: line, else the first one that is not a hint.
func reason(lines []string) string {
	for _, l := range lines {
		if strings.HasPrefix(l, "fatal:") || strings.HasPrefix(l, "error:") {
			return l
		}
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "hint:") && strings.TrimSpace(l) != "" {
			return l
		}
	}
	if len(lines) > 0 {
		return lines[0]
	}
	return "git failed"
}

// needsCredentials reports git failing for want of a password or a passphrase.
func needsCredentials(lines []string) bool {
	text := strings.ToLower(strings.Join(lines, " "))
	for _, s := range []string{"terminal prompts disabled", "could not read username", "could not read password", "permission denied (publickey", "authentication failed"} {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

// plural is n and the word for that many.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
