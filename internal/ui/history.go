package ui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/gittu/internal/git"
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

// historyScreen is the screen gittu opens on (tdp terms.2): [1] the branches,
// [2] the history graph beside them, [3] the files of the commit under [2]'s
// cursor below it.
type historyScreen struct {
	// width is [2]'s, as laid out last: how many of the graph's lines fit.
	width int
	// line is the [1] row the Current tab follows (its ID), "" for HEAD.
	line   string
	g      *gittu
	screen *terminu.Screen

	branchP *terminu.Panel
	// tabs are [1]'s lists, one a tab: the branches, the remote branches, the
	// tags, the stashes (kinds).
	tabs [4]*terminu.List

	histP   *terminu.Panel
	commits *terminu.List

	filesP *terminu.Panel
	files  *terminu.List
	// items are the files [3] lists, by row ID; filesOf the commit they are
	// of, "" when [3] has none to show or could not read them.
	items   map[string]terminu.DiffItem
	filesOf string
	// zoom is how far [2] is zoomed (tdp screen.6): 0 not, 1 with [1] at its
	// least width, 2 the focused panel on the whole panel area.
	zoom int
}

// leftLeast is [1]'s width when [2] is zoomed with [1] still beside it: what
// [1] has at 60 columns, branch names still readable (tdp L1).
const leftLeast = 20

func newHistoryScreen(g *gittu) *historyScreen {
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
		OnMove:     func(terminu.Row) tea.Cmd { h.showFiles(); return nil },
	}
	// [3] follows [2]'s cursor (tdp panel.13), always there (the user).
	h.files = &terminu.List{
		Columns:    []terminu.Column{{Title: "", Shrink: true, Path: true}},
		Flags:      true,
		Filterable: true,
		EmptyFact:  "No commits yet",
		EmptyHint:  "Press [W] to see the files to commit",
	}
	h.filesP = &terminu.Panel{Label: "Files", Body: h.files}
	h.filesP.Actions = append(h.fileActions(), h.files.Actions()...)
	// Two tabs (the user): Current, the branch under [1]'s cursor on the left,
	// and All, main on the left; Current first, the one chosen kept while
	// gittu runs.
	h.histP = &terminu.Panel{Label: "Current", Tabs: []string{"All"}, Body: h.commits, Details: h.filesP,
		OnTab: func(tab int) tea.Cmd { return h.setAll(tab == 1) }}
	h.histP.Actions = append(h.commitActions(), h.commits.Actions()...)
	h.screen = &terminu.Screen{Name: "History", Key: "H", Desc: "the branches and the history graph",
		Panels: []*terminu.Panel{h.branchP, h.histP, h.filesP}, Layout: h.layout}
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
// others drawn nowhere). [3] is under [2], a third of its height.
func (h *historyScreen) layout(w, ht int) []terminu.Rect {
	var rects []terminu.Rect
	switch h.zoom {
	case 1:
		rects = beside(min(leftLeast, w/3), w, ht)
	case 2:
		rects = make([]terminu.Rect, len(h.screen.Panels))
		for i, p := range h.screen.Panels {
			if p == h.screen.Focused() {
				rects[i] = terminu.Rect{X: 0, Y: 0, W: w, H: ht}
			}
		}
	default:
		rects = oneToTwo(w, ht)
	}
	if h.zoom != 2 {
		r := rects[1]
		low := r.H / 3
		rects[1].H = r.H - low
		rects = append(rects, terminu.Rect{X: r.X, Y: r.Y + r.H - low, W: r.W, H: low})
	}
	// [2] wider or narrower: the graph draws as many lines as now fit.
	if rw := rects[1].W; rw != h.width {
		h.width = rw
		if h.g.snap != nil {
			h.commits.SetRows(h.commitRows(h.g.snap))
		}
	}
	return rects
}

// mostLines is how many of the graph's lines [2] has room for (the user):
// what is left of its width once HASH, DESCRIPTION as wide as its widest
// subject, AUTHOR and DATE have theirs, two cells a line; MaxLines at the
// least, the rest drawn "…".
func (h *historyScreen) mostLines(s *git.Snapshot) int {
	if h.width <= 0 {
		return git.MaxLines
	}
	desc, author := 0, 0
	for _, c := range s.Commits {
		desc = max(desc, terminu.Width(c.Subject))
		author = max(author, terminu.Width(c.Author))
	}
	// the frame and the margins (4), each column after the graph two
	// cells away from the one before, HASH 7 and DATE 10 wide
	left := h.width - 4 - (2 + 7) - (2 + desc) - (2 + author) - (2 + 10)
	return max(git.MaxLines, (left+1)/2)
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
		h.g.a.Log(terminu.Info, "Zoomed [2]")
	case 2:
		h.g.a.Log(terminu.Info, "Zoomed [2] to the whole screen")
	}
	return nil
}

// cur is the list of [1]'s tab now.
func (h *historyScreen) cur() *terminu.List { return h.tabs[h.branchP.Tab] }

// show puts a read of the repository on the screen.
func (h *historyScreen) show(s *git.Snapshot) {
	h.branchP.Fail, h.histP.Fail = nil, nil
	h.emptyHints(s)
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
	h.showFiles()
}

// emptyHints name in the empty tabs a key that works now (tdp screen.8):
// fetch with a remote, tag with a commit, stash with changes; otherwise
// Space, to see what can be done.
func (h *historyScreen) emptyHints(s *git.Snapshot) {
	const space = "Press [Space] to see what you can do here"
	if len(s.RemoteNames) == 0 {
		h.tabs[1].EmptyFact, h.tabs[1].EmptyHint = "This repository has no remote", space
	} else {
		h.tabs[1].EmptyFact, h.tabs[1].EmptyHint = "No remote branches", "Press [f] to fetch every remote"
	}
	h.tabs[2].EmptyHint = "Press [t] to tag HEAD"
	if s.HeadSHA == "" {
		h.tabs[2].EmptyHint = space
	}
	h.tabs[3].EmptyHint = "Press [S] to stash the changes"
	if s.HeadSHA == "" || !slices.ContainsFunc(s.Files, func(f git.File) bool { return !f.Untracked() }) {
		h.tabs[3].EmptyHint = space
	}
}

// failed says the repository could not be read (tdp panel.15).
func (h *historyScreen) failed() {
	f := &terminu.Failure{Fact: "Can't read " + h.g.repo.Name() + ": " + h.g.readErr, Hint: "Press [R] to read it again"}
	h.branchP.Fail, h.histP.Fail, h.filesP.Fail = f, f, f
	for _, l := range h.tabs {
		l.SetRows(nil)
	}
	h.commits.SetRows(nil)
	h.files.SetRows(nil)
	h.items, h.filesOf = nil, ""
}

// scope is the location row of the graph: its main line, and the remote
// branches when they are drawn (tdp panel.10).
func (h *historyScreen) scope() string {
	s := "All branches"
	if !h.g.all {
		s = h.lineName()
	}
	if h.g.remote {
		s += " · with remote branches"
	}
	return s
}

// mainLine is the commit the graph keeps in its first column: main (else
// master) on the All tab; on the Current tab the branch, remote branch or tag
// under [1]'s cursor, HEAD before [1] has one (the user).
func (h *historyScreen) mainLine(s *git.Snapshot) string {
	if h.g.all {
		return s.MainSHA()
	}
	return h.lineSHA(s)
}

// lineSHA is the commit the Current tab follows: the ref under [1]'s cursor
// while it is in the graph, else HEAD.
func (h *historyScreen) lineSHA(s *git.Snapshot) string {
	if h.line != "" {
		if sha := h.shaOf(h.line); sha != "" && slices.ContainsFunc(s.Commits, func(c git.Commit) bool { return c.SHA == sha }) {
			return sha
		}
	}
	return s.HeadSHA
}

// lineName names what the Current tab follows, as the location row says it.
func (h *historyScreen) lineName() string {
	if h.line != "" && h.g.snap != nil && h.lineSHA(h.g.snap) == h.shaOf(h.line) {
		return h.line[2:]
	}
	return h.g.currentName()
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
	graph := git.Graph(s.Commits, h.mainLine(s), h.mostLines(s))
	refs := refsByCommit(s)
	rows := make([]terminu.Row, len(s.Commits))
	for i, c := range s.Commits {
		gText, gSpans := graphSpans(graph[i])
		date := c.Date.Format("2006-01-02")
		rows[i] = terminu.Row{
			ID:     c.SHA,
			Cells:  []string{gText, git.Short(c.SHA), c.Subject, c.Author, date},
			Styled: [][]terminu.Span{gSpans, nil, nil, nil, {{Text: date, Style: terminu.Fg(terminu.Overlay0)}}},
		}
		// Its branches and tags on a row of their own under it, the
		// subject keeping DESCRIPTION's width (the user; tdp main panel.5,
		// an item of several rows): the graph's lines run on through it.
		if rs := refs[c.SHA]; len(rs) > 0 {
			tText, tSpans := graphSpans(git.Through(graph[i], len(c.Parents) > 0))
			rText, rSpans := refsText(rs)
			rows[i].Below = []terminu.BelowRow{{
				Cells:  []string{tText, "", rText},
				Styled: [][]terminu.Span{tSpans, nil, rSpans},
			}}
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

// refsText is a commit's refs, for the row under it: a branch glyph before
// a local branch (Green) and a remote one (Mauve), a tag glyph before a tag
// (Peach), the branch HEAD is on bold (tdp P5). Not a capsule: that is tdp's
// for a name (tdp P4).
func refsText(refs []ref) (string, []terminu.Span) {
	var b strings.Builder
	var spans []terminu.Span
	for i, r := range refs {
		if i > 0 {
			b.WriteString(" ")
			spans = append(spans, terminu.Span{Text: " "})
		}
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
		b.WriteString(text)
		spans = append(spans, terminu.Span{Text: text, Style: st})
	}
	return b.String(), spans
}

// branchMoved puts [2]'s cursor on the commit the branch, remote branch or
// tag under [1]'s cursor points at; the focus
// stays (tdp K3, panel.13).
func (h *historyScreen) branchMoved(r terminu.Row) tea.Cmd {
	if h.g.snap == nil {
		return nil
	}
	if r.ID[:2] != kindStash { // the Current tab follows it: its line on the left
		h.line = r.ID
		if !h.g.all {
			h.commits.SetRows(h.commitRows(h.g.snap))
			h.commits.Location = h.scope()
		}
	}
	if sha := h.shaOf(r.ID); h.inGraph(sha) {
		h.commits.ClearFilter() // it puts [2]'s cursor on that commit (tdp panel.14)
		h.commits.Select(sha)
	}
	return nil
}

// trace puts [2]'s cursor on the next commit, older (down) or newer (up),
// that HEAD reaches, merged ones too; from a commit it does not reach, the
// nearest one that way (the user). None left that way: an Info toast, the
// cursor where it was. The filter is cleared first (tdp panel.14).
func (h *historyScreen) trace(older bool) tea.Cmd {
	s := h.g.snap
	reach := reached(s.Commits, h.lineSHA(s))
	at := -1
	if cur, ok := h.commits.Current(); ok {
		at = slices.IndexFunc(s.Commits, func(c git.Commit) bool { return c.SHA == cur.ID })
	}
	step := 1
	if !older {
		step = -1
	}
	for i := at + step; i >= 0 && i < len(s.Commits); i += step {
		if reach[s.Commits[i].SHA] {
			h.commits.ClearFilter()
			h.commits.Select(s.Commits[i].SHA)
			return nil
		}
	}
	word := "older"
	if !older {
		word = "newer"
	}
	return h.g.a.Toast(terminu.Info, "No "+word+" commit on "+h.lineName())
}

// reached are the commits head reaches among commits, through every parent.
func reached(commits []git.Commit, head string) map[string]bool {
	parents := make(map[string][]string, len(commits))
	for _, c := range commits {
		parents[c.SHA] = c.Parents
	}
	reach := map[string]bool{}
	todo := []string{head}
	for len(todo) > 0 {
		sha := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if sha == "" || reach[sha] {
			continue
		}
		if _, ok := parents[sha]; !ok {
			continue // past the commits read
		}
		reach[sha] = true
		todo = append(todo, parents[sha]...)
	}
	return reach
}

// inGraph reports the commit sha among the graph's rows, a filter or not.
func (h *historyScreen) inGraph(sha string) bool {
	return sha != "" && h.g.snap != nil && slices.ContainsFunc(h.g.snap.Commits, func(c git.Commit) bool { return c.SHA == sha })
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

// showFiles puts in [3] the files the commit under [2]'s cursor changed
// against its first parent, the first commit against the empty tree: each
// its mark in its colour, then its path (tdp panel.13, note.8, panel.9). Its
// location row is <parent> ↔ <commit>. Read once a commit; a read that
// failed is tried again on the next read of the repository.
func (h *historyScreen) showFiles() {
	c, ok := h.current()
	if !ok {
		h.files.EmptyFact, h.files.EmptyHint = "No commits yet", "Press [W] to see the files to commit"
		h.files.SetRows(nil)
		h.files.Location, h.items, h.filesOf, h.filesP.Fail = "", nil, "", nil
		return
	}
	if c.SHA == h.filesOf {
		return
	}
	from, fromName := "", "empty"
	if len(c.Parents) > 0 {
		from, fromName = c.Parents[0], git.Short(c.Parents[0])
	}
	h.files.Location = fromName + " ↔ " + git.Short(c.SHA)
	items, err := h.commitItems(from, c.SHA, fromName, git.Short(c.SHA))
	if err != nil {
		h.filesP.Fail = &terminu.Failure{Fact: "Can't read the files of " + git.Short(c.SHA) + ": " + reason(gitLines(err)), Hint: "Press [R] to read it again"}
		h.files.SetRows(nil)
		h.items, h.filesOf = nil, ""
		return
	}
	h.filesP.Fail = nil
	h.files.EmptyFact, h.files.EmptyHint = "No files changed", "Press [2] to go to the commits"
	h.items, h.filesOf = map[string]terminu.DiffItem{}, c.SHA
	rows := make([]terminu.Row, len(items))
	for i, it := range items {
		rows[i] = terminu.Row{ID: it.Name, Cells: []string{it.Name}, Flag: markSpan(it.Mark)}
		h.items[it.Name] = it
	}
	h.files.SetRows(rows)
}

// markSpan is a file's mark in its colour, as the lib draws it in the list
// of the items that differ (tdp panel.9): + Green, - Red, ~ Peach.
func markSpan(m terminu.DiffMark) terminu.Span {
	switch m {
	case terminu.DiffAdded:
		return terminu.Span{Text: "+", Style: terminu.Fg(terminu.Green)}
	case terminu.DiffRemoved:
		return terminu.Span{Text: "-", Style: terminu.Fg(terminu.Red)}
	}
	return terminu.Span{Text: "~", Style: terminu.Fg(terminu.Peach)}
}

// file is the file under [3]'s cursor.
func (h *historyScreen) file() (terminu.DiffItem, bool) {
	r, ok := h.files.Current()
	if !ok {
		return terminu.DiffItem{}, false
	}
	it, ok := h.items[r.ID]
	return it, ok
}

// openFile opens the two versions of the file under [3]'s cursor (tdp
// note.8): <path> @ <parent> ↔ <path> @ <commit>.
func (h *historyScreen) openFile() tea.Cmd {
	it, ok := h.file()
	if !ok || it.Text == nil {
		return nil
	}
	h.g.a.Log(terminu.Info, "Compared "+it.Anchor+" with "+it.Other)
	return h.g.a.OpenDiff(h.filesP, it)
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
