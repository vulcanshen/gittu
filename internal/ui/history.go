package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/gitreu/internal/git"
)

// The kinds of row in [1] BRANCHES, the start of each row's ID.
const (
	kindBranch = "b:"
	kindRemote = "r:"
	kindTag    = "t:"
	kindStash  = "s:"
)

// kinds are [1]'s tabs, in order: each a kind of row (tdp terms.4).
var kinds = []string{kindBranch, kindRemote, kindTag, kindStash}

// historyScreen is the screen gitreu opens on (tdp terms.2): [1] the branches,
// [2] the history graph, as SourceTree lays them out.
type historyScreen struct {
	g      *gitreu
	screen *terminu.Screen

	branchP *terminu.Panel
	// tabs are [1]'s lists, one a tab: the branches, the remote branches, the
	// tags, the stashes (kinds).
	tabs [4]*terminu.List

	histP   *terminu.Panel
	commits *terminu.List
	// in is the level [2] has gone into, nil on the graph (tdp panel.10).
	in *level
	// files lists the files of the level gone into.
	files *terminu.List
}

// level is a commit, or two commits compared, that [2] has gone into: the
// files that differ from one to the other.
type level struct {
	from, to string // from "" is the empty tree: a first commit
	// commit is the row of the graph gone in from, to come back to.
	commit  string
	where   string // what the location row adds: "a1b2c3d Fix the graph"
	changes []git.Change
}

func newHistoryScreen(g *gitreu) *historyScreen {
	h := &historyScreen{g: g}
	empty := [4][2]string{
		{"No branches yet", "Press [Space] to see what you can do here"},
		{"No remote branches", "Press [f] to fetch every remote"},
		{"No tags", "Press [t] to tag HEAD"},
		{"No stashes", "Press [S] to stash the changes"},
	}
	for i := range h.tabs {
		h.tabs[i] = &terminu.List{
			Columns:    []terminu.Column{{Title: "", Shrink: true}},
			Flags:      i == 0, // the branch checked out has its flag
			Filterable: true,
			EmptyFact:  empty[i][0],
			EmptyHint:  empty[i][1],
			OnMove:     h.branchMoved,
		}
	}
	h.branchP = &terminu.Panel{Label: "Branches", Tabs: []string{"Remotes", "Tags", "Stashes"}, Body: h.tabs[0],
		OnTab: func(tab int) tea.Cmd { h.branchP.Body = h.tabs[tab]; return nil }}
	h.branchP.Actions = append(h.branchActions(), terminu.Action{Key: "/", Label: "Filter", Desc: "narrow the list as you type",
		Scope: panel, Do: func() tea.Cmd { h.cur().OpenFilter(); return nil }})
	h.commits = &terminu.List{
		Columns: []terminu.Column{
			{Title: "Graph"},
			{Title: "Hash"},
			{Title: "Description", Shrink: true, MinWidth: 12},
			{Title: "Author", Shrink: true, MinWidth: 6, Drop: 2},
			{Title: "Date", Drop: 1},
		},
		Marks:      true,
		Filterable: true,
		EmptyFact:  "No commits yet",
		EmptyHint:  "Press [W] to see the files to commit",
	}
	h.files = &terminu.List{
		Columns:   []terminu.Column{{Title: "", Shrink: true, Path: true}},
		Flags:     true,
		EmptyFact: "No files changed",
		EmptyHint: "Press [Esc] to go back to the history",
	}
	h.histP = &terminu.Panel{Label: "Commits", Body: h.commits}
	h.histP.Actions = append(h.commitActions(), h.commits.Actions()...)
	h.screen = &terminu.Screen{Name: "History", Key: "H", Desc: "the branches and the history graph",
		Panels: []*terminu.Panel{h.branchP, h.histP}, Layout: oneToTwo}
	return h
}

// oneToTwo puts the panels side by side, the first a third of the width (tdp
// L1: 20 and 40 at 60 columns).
func oneToTwo(w, h int) []terminu.Rect {
	left := w / 3
	return []terminu.Rect{{X: 0, Y: 0, W: left, H: h}, {X: left, Y: 0, W: w - left, H: h}}
}

// cur is the list of [1]'s tab now.
func (h *historyScreen) cur() *terminu.List { return h.tabs[h.branchP.Tab] }

// show puts a read of the repository on the screen.
func (h *historyScreen) show(s *git.Snapshot) {
	h.branchP.Fail, h.histP.Fail = nil, nil
	first := h.tabs[0].Empty()
	rows := branchRows(s)
	for i, kind := range kinds {
		var tab []terminu.Row
		for _, r := range rows {
			if r.ID[:2] == kind {
				tab = append(tab, r)
			}
		}
		h.tabs[i].SetRows(tab)
	}
	if first && s.Head != "" {
		// Open on the branch checked out, the graph on HEAD.
		h.tabs[0].Select(kindBranch + s.Head)
	}
	h.commits.SetRows(commitRows(s))
	h.commits.Location = h.scope()
	if h.in != nil {
		h.files.Location = h.scope() + " › " + h.in.where
	}
}

// failed says the repository could not be read (tdp panel.15).
func (h *historyScreen) failed() {
	f := &terminu.Failure{Fact: "Can't read " + h.g.repo.Name() + ": " + firstLine(h.g.readErr), Hint: "Press [R] to read it again"}
	h.branchP.Fail, h.histP.Fail = f, f
	for _, l := range h.tabs {
		l.SetRows(nil)
	}
	h.commits.SetRows(nil)
}

// scope is the location row of the graph: which history it shows (tdp
// panel.10).
func (h *historyScreen) scope() string {
	if h.g.all {
		return "All branches"
	}
	return "Current branch"
}

func firstLine(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}

// branchRows are [1]'s rows: the branches, the remote branches, the tags and
// the stashes, each kind its tab's (the start of the ID).
func branchRows(s *git.Snapshot) []terminu.Row {
	var rows []terminu.Row
	quiet := terminu.Fg(terminu.Overlay0)
	for _, b := range s.Branches {
		r := terminu.Row{ID: kindBranch + b.Name, Cells: []string{b.Name}}
		if t := track(b.Ahead, b.Behind, b.Gone); t != "" {
			st := quiet
			if b.Gone {
				st = terminu.Fg(terminu.Peach) // worth a look: most often merged, and deleted on the remote
			}
			r.Cells[0] = b.Name + " " + t
			r.Styled = [][]terminu.Span{{{Text: b.Name}, {Text: " " + t, Style: st}}}
		}
		if b.Current {
			r.Flag = terminu.Span{Text: "●", Style: terminu.Fg(terminu.Green)}
		}
		rows = append(rows, r)
	}
	for _, rb := range s.Remotes {
		rows = append(rows, terminu.Row{ID: kindRemote + rb.Name, Cells: []string{rb.Name}})
	}
	for _, t := range s.Tags {
		rows = append(rows, terminu.Row{ID: kindTag + t.Name, Cells: []string{t.Name}})
	}
	for _, st := range s.Stashes {
		rows = append(rows, terminu.Row{ID: kindStash + st.Ref, Cells: []string{st.Ref + ": " + st.Message}})
	}
	return rows
}

// commitRows are the graph's rows, one a commit.
func commitRows(s *git.Snapshot) []terminu.Row {
	graph := git.Graph(s.Commits)
	refs := refsByCommit(s)
	rows := make([]terminu.Row, len(s.Commits))
	for i, c := range s.Commits {
		gText, gSpans := graphSpans(graph[i])
		desc, descSpans := description(refs[c.SHA], c.Subject)
		date := c.Date.Format("2006-01-02")
		rows[i] = terminu.Row{
			ID:     c.SHA,
			Cells:  []string{gText, git.Short(c.SHA), desc, c.Author, date},
			Styled: [][]terminu.Span{gSpans, nil, descSpans, nil, {{Text: date, Style: terminu.Fg(terminu.Overlay0)}}},
		}
	}
	return rows
}

// graphSpans is a graph row in its lines' colours.
func graphSpans(cells []git.Cell) (string, []terminu.Span) {
	var b strings.Builder
	var spans []terminu.Span
	for _, c := range cells {
		b.WriteRune(c.Rune)
		st := terminu.Style{}
		if c.Line >= 0 {
			st = terminu.Fg(lineColours[c.Line%len(lineColours)])
		} else if c.Rune == '…' {
			st = terminu.Fg(terminu.Overlay0)
		}
		if n := len(spans); n > 0 && spans[n-1].Style == st {
			spans[n-1].Text += string(c.Rune)
			continue
		}
		spans = append(spans, terminu.Span{Text: string(c.Rune), Style: st})
	}
	return b.String(), spans
}

// ref is a name pointing at a commit, as the graph writes it.
type ref struct {
	name string
	kind string // kindBranch, kindRemote, kindTag
	head bool
}

func refsByCommit(s *git.Snapshot) map[string][]ref {
	m := map[string][]ref{}
	for _, b := range s.Branches {
		m[b.SHA] = append(m[b.SHA], ref{b.Name, kindBranch, b.Current})
	}
	if s.Head == "" && s.HeadSHA != "" {
		m[s.HeadSHA] = append([]ref{{"HEAD", kindBranch, true}}, m[s.HeadSHA]...)
	}
	for _, rb := range s.Remotes {
		m[rb.SHA] = append(m[rb.SHA], ref{rb.Name, kindRemote, false})
	}
	for _, t := range s.Tags {
		m[t.SHA] = append(m[t.SHA], ref{t.Name, kindTag, false})
	}
	return m
}

// description is a commit's refs, then its subject: local branches Green,
// remote ones Mauve, tags Peach, the branch HEAD is on bold (tdp P5).
func description(refs []ref, subject string) (string, []terminu.Span) {
	var b strings.Builder
	var spans []terminu.Span
	for _, r := range refs {
		st := terminu.Fg(terminu.Green)
		switch r.kind {
		case kindRemote:
			st = terminu.Fg(mauve)
		case kindTag:
			st = terminu.Fg(terminu.Peach)
		}
		if r.head {
			st = st.Bolded()
		}
		b.WriteString(r.name + " ")
		spans = append(spans, terminu.Span{Text: r.name, Style: st}, terminu.Span{Text: " "})
	}
	b.WriteString(subject)
	spans = append(spans, terminu.Span{Text: subject})
	return b.String(), spans
}

// branchMoved puts [2]'s cursor on the commit the branch, remote branch or
// tag under [1]'s cursor points at, as SourceTree's sidebar does; the focus
// stays (tdp K3, panel.13).
func (h *historyScreen) branchMoved(r terminu.Row) tea.Cmd {
	if h.in != nil || h.g.snap == nil {
		return nil
	}
	if sha := h.shaOf(r.ID); sha != "" {
		h.commits.Select(sha)
	}
	return nil
}

// shaOf is the commit a row of [1] points at.
func (h *historyScreen) shaOf(id string) string {
	s := h.g.snap
	kind, name := id[:2], id[2:]
	switch kind {
	case kindBranch:
		for _, b := range s.Branches {
			if b.Name == name {
				return b.SHA
			}
		}
	case kindRemote:
		for _, rb := range s.Remotes {
			if rb.Name == name {
				return rb.SHA
			}
		}
	case kindTag:
		for _, t := range s.Tags {
			if t.Name == name {
				return t.SHA
			}
		}
	case kindStash:
		for _, st := range s.Stashes {
			if st.Ref == name {
				return st.SHA
			}
		}
	}
	return ""
}

// commit is the commit of the graph's row with this ID.
func (h *historyScreen) commit(sha string) (git.Commit, bool) {
	if h.g.snap == nil {
		return git.Commit{}, false
	}
	for _, c := range h.g.snap.Commits {
		if c.SHA == sha {
			return c, true
		}
	}
	return git.Commit{}, false
}

// current is the commit under [2]'s cursor on the graph.
func (h *historyScreen) current() (git.Commit, bool) {
	if h.in != nil {
		return git.Commit{}, false
	}
	r, ok := h.commits.Current()
	if !ok {
		return git.Commit{}, false
	}
	return h.commit(r.ID)
}

// goIn shows the files that differ from one commit to another in [2] (tdp
// panel.10): from "" is the empty tree.
func (h *historyScreen) goIn(from, to, where string) tea.Cmd {
	changes, err := h.g.repo.Changes(from, to)
	if err != nil {
		return h.g.a.ErrorAbout("Files failed", git.Short(to), errLines(err)...)
	}
	r, _ := h.commits.Current()
	h.in = &level{from: from, to: to, commit: r.ID, where: where, changes: changes}
	h.files.SetRows(changeRows(changes))
	h.files.ClearFilter()
	h.files.Location = h.scope() + " › " + where
	h.histP.Body = h.files
	h.histP.OnEsc = h.goOut
	return nil
}

// goOut goes back up to the graph, the cursor on the commit gone in from (tdp
// K4, panel.10).
func (h *historyScreen) goOut() tea.Cmd {
	if h.in == nil {
		return nil
	}
	h.commits.Select(h.in.commit)
	h.in = nil
	h.histP.Body = h.commits
	h.histP.OnEsc = nil
	return nil
}

// changeRows are the files of a commit: what changed in the flag column
// (tdp panel.7), the path.
func changeRows(cs []git.Change) []terminu.Row {
	rows := make([]terminu.Row, len(cs))
	for i, c := range cs {
		path := c.Path
		if c.Orig != "" {
			path = c.Orig + " → " + c.Path
		}
		rows[i] = terminu.Row{ID: strconv.Itoa(i), Cells: []string{path}, Flag: changeFlag(c.Status)}
	}
	return rows
}

// changeFlag is a change's letter in its colour (tdp P5).
func changeFlag(status byte) terminu.Span {
	c := terminu.Peach
	switch status {
	case 'A', 'C':
		c = terminu.Green
	case 'D':
		c = terminu.Red
	case 'R':
		c = terminu.Blue
	case '?':
		c = terminu.Overlay0
	case 'U':
		c = terminu.Red
	}
	return terminu.Span{Text: string(status), Style: terminu.Fg(c)}
}

// change is the file under the cursor of the level gone into.
func (h *historyScreen) change() (git.Change, bool) {
	if h.in == nil {
		return git.Change{}, false
	}
	r, ok := h.files.Current()
	if !ok {
		return git.Change{}, false
	}
	i, err := strconv.Atoi(r.ID)
	if err != nil || i >= len(h.in.changes) {
		return git.Change{}, false
	}
	return h.in.changes[i], true
}

// openDiff opens the diff of the file under the cursor in a viewer (tdp
// note.1).
func (h *historyScreen) openDiff() tea.Cmd {
	c, ok := h.change()
	if !ok {
		return nil
	}
	lines, err := h.g.repo.Diff(h.in.from, h.in.to, c)
	if err != nil {
		return h.g.a.ErrorAbout("Diff failed", c.Path, errLines(err)...)
	}
	h.g.a.Log(terminu.Info, "Opened the diff of "+c.Path+" in "+h.in.where)
	return h.g.a.Viewer(&terminu.Viewer{Title: "Diff", Subject: c.Path, Glyph: glyphDiff, Lines: lines, Lang: "diff", NoNumbers: true})
}
