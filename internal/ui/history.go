package ui

import (
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
	// zoom is how far [2] is zoomed (tdp screen.6): 0 not, 1 with [1] at its
	// least width, 2 the focused panel on the whole panel area.
	zoom int
}

// leftLeast is [1]'s width when [2] is zoomed with [1] still beside it: what
// [1] has at 60 columns, branch names still readable (tdp L1).
const leftLeast = 20

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
		Filterable: true,
		EmptyFact:  "No commits yet",
		EmptyHint:  "Press [W] to see the files to commit",
	}
	h.histP = &terminu.Panel{Label: "Commits", Body: h.commits}
	h.histP.Actions = append(h.commitActions(), h.commits.Actions()...)
	h.screen = &terminu.Screen{Name: "History", Key: "H", Desc: "the branches and the history graph",
		Panels: []*terminu.Panel{h.branchP, h.histP}, Layout: h.layout}
	return h
}

// oneToTwo puts the panels side by side, the first a third of the width (tdp
// L1: 20 and 40 at 60 columns).
func oneToTwo(w, h int) []terminu.Rect { return beside(w/3, w, h) }

// beside puts [1] left wide on the left, [2] the rest on the right.
func beside(left, w, h int) []terminu.Rect {
	return []terminu.Rect{{X: 0, Y: 0, W: left, H: h}, {X: left, Y: 0, W: w - left, H: h}}
}

// layout is History's panels as zoomed (tdp screen.6): side by side 1:2,
// [1] at its least width, or the focused one alone on the whole area (the
// other drawn nowhere).
func (h *historyScreen) layout(w, ht int) []terminu.Rect {
	switch h.zoom {
	case 1:
		return beside(min(leftLeast, w/3), w, ht)
	case 2:
		if h.screen.Focused() == h.branchP {
			return []terminu.Rect{{X: 0, Y: 0, W: w, H: ht}, {X: w, Y: 0, W: 0, H: ht}}
		}
		return []terminu.Rect{{X: 0, Y: 0, W: 0, H: ht}, {X: 0, Y: 0, W: w, H: ht}}
	}
	return oneToTwo(w, ht)
}

// narrowZoom reports a terminal where [1] is at its least width already: the
// zoom has no step with [1] beside [2].
func (h *historyScreen) narrowZoom() bool {
	w, _ := h.g.a.Size()
	return w/3 <= leftLeast
}

// zoomTo zooms History to step z and says so in the App Log (tdp note.7).
func (h *historyScreen) zoomTo(z int) tea.Cmd {
	h.zoom = z
	switch z {
	case 0:
		h.g.a.Log(terminu.Info, "Back to both panels")
	case 1:
		h.g.a.Log(terminu.Info, "Zoomed [2] COMMITS")
	case 2:
		h.g.a.Log(terminu.Info, "Zoomed [2] COMMITS to the whole screen")
	}
	return nil
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
	h.commits.SetRows(h.commitRows(s))
	h.commits.Location = h.scope()
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

// scope is the location row of the graph: its main line, and the remote
// branches when they are drawn (tdp panel.10).
func (h *historyScreen) scope() string {
	s := "Current branch"
	if h.g.all {
		s = "All branches"
	}
	if h.g.remote {
		s += " · with remote branches"
	}
	return s
}

// mainLine is the commit the graph keeps in its first column: main (else
// master) for All, HEAD for Current.
func (h *historyScreen) mainLine(s *git.Snapshot) string {
	if h.g.all {
		return s.MainSHA()
	}
	return s.HeadSHA
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
				t = glyphGone + " " + t
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

// commitRows are the graph's rows, one a commit, the main line first.
func (h *historyScreen) commitRows(s *git.Snapshot) []terminu.Row {
	graph := git.Graph(s.Commits, h.mainLine(s))
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
	if s.Remote {
		for _, rb := range s.Remotes {
			m[rb.SHA] = append(m[rb.SHA], ref{rb.Name, kindRemote, false})
		}
	}
	for _, t := range s.Tags {
		m[t.SHA] = append(m[t.SHA], ref{t.Name, kindTag, false})
	}
	return m
}

// description is a commit's refs, then its subject: a branch glyph before a
// local branch (Green) and a remote one (Mauve), a tag glyph before a tag
// (Peach), the branch HEAD is on bold (tdp P5). Not a capsule: that is tdp's
// for a name (tdp P4; a badge of its own is asked of tdp).
func description(refs []ref, subject string) (string, []terminu.Span) {
	var b strings.Builder
	var spans []terminu.Span
	for _, r := range refs {
		st, glyph := terminu.Fg(terminu.Green), glyphBranch
		switch r.kind {
		case kindRemote:
			st = terminu.Fg(mauve)
		case kindTag:
			st, glyph = terminu.Fg(terminu.Peach), glyphRefTag
		}
		if r.head {
			st = st.Bolded()
		}
		text := glyph + " " + r.name
		b.WriteString(text + " ")
		spans = append(spans, terminu.Span{Text: text, Style: st}, terminu.Span{Text: " "})
	}
	b.WriteString(subject)
	spans = append(spans, terminu.Span{Text: subject})
	return b.String(), spans
}

// branchMoved puts [2]'s cursor on the commit the branch, remote branch or
// tag under [1]'s cursor points at, as SourceTree's sidebar does; the focus
// stays (tdp K3, panel.13). [2] gone into the files of a commit comes back
// to the graph (LeaveDiff, tdp note.8).
func (h *historyScreen) branchMoved(r terminu.Row) tea.Cmd {
	if h.g.snap == nil {
		return nil
	}
	if sha := h.shaOf(r.ID); sha != "" && h.commits.Select(sha) {
		h.histP.LeaveDiff()
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
	r, ok := h.commits.Current()
	if !ok {
		return git.Commit{}, false
	}
	return h.commit(r.ID)
}

// commitItems are the files that differ from commit from to commit to,
// fromName and toName naming the two sides (tdp panel.9): a path each, its mark
// + - ~ (a rename ~ <old> → <new>), its two versions to compare, none for a
// binary file. from "" is the empty tree: a first commit.
func (h *historyScreen) commitItems(from, to, fromName, toName string) ([]terminu.DiffItem, error) {
	repo := h.g.repo
	changes, err := repo.Changes(from, to)
	if err != nil {
		return nil, err
	}
	items := make([]terminu.DiffItem, len(changes))
	for i, c := range changes {
		old := c.Path
		if c.Orig != "" {
			old = c.Orig
		}
		it := terminu.DiffItem{Name: c.Path, Anchor: old + " @ " + fromName, Other: c.Path + " @ " + toName}
		switch {
		case c.Status == 'A' || c.Status == 'C' && from == "":
			it.Mark = terminu.DiffAdded
		case c.Status == 'D':
			it.Mark = terminu.DiffRemoved
		default:
			it.Mark = terminu.DiffChanged
		}
		if c.Status == 'C' && from != "" {
			it.Mark = terminu.DiffAdded // a copy is a new file, made from another
		}
		if c.Orig != "" && c.Status == 'R' {
			it.Name = c.Orig + " → " + c.Path
		}
		if !c.Binary {
			mark, path := it.Mark, c.Path
			it.Text = func() (string, string, error) {
				var a, b string
				var err error
				if mark != terminu.DiffAdded && from != "" {
					if a, err = repo.Show(from, old); err != nil {
						return "", "", err
					}
				}
				if mark != terminu.DiffRemoved {
					if b, err = repo.Show(to, path); err != nil {
						return "", "", err
					}
				}
				return a, b, nil
			}
		}
		items[i] = it
	}
	return items, nil
}

// showCommit goes into what the commit under the cursor changed against its
// first parent (tdp note.8, panel.9): the files that differ, each opening
// its two versions.
func (h *historyScreen) showCommit() tea.Cmd {
	c, ok := h.current()
	if !ok {
		return nil
	}
	from, fromName := "", "empty"
	if len(c.Parents) > 0 {
		from, fromName = c.Parents[0], git.Short(c.Parents[0])
	}
	h.g.a.Log(terminu.Info, "Opened the changes of "+git.Short(c.SHA)+" "+c.Subject)
	return h.g.a.OpenDiff(h.histP, terminu.DiffItem{Anchor: fromName, Other: git.Short(c.SHA),
		Items: func() ([]terminu.DiffItem, error) { return h.commitItems(from, c.SHA, fromName, git.Short(c.SHA)) }})
}

// compareMode is [2]'s compare mode (tdp panel.9): two commits hold many, so
// comparing them goes into the files that differ, the anchor the base.
func (h *historyScreen) compareMode() terminu.Action {
	return h.g.a.CompareAction(h.histP, h.commits, terminu.Compare{Key: "C",
		Many: func(terminu.Row) bool { return true },
		Name: func(r terminu.Row) string { return git.Short(r.ID) },
		Items: func(anchor, other terminu.Row) ([]terminu.DiffItem, error) {
			return h.commitItems(anchor.ID, other.ID, git.Short(anchor.ID), git.Short(other.ID))
		}})
}
