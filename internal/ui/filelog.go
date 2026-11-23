package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/gittu/internal/git"
)

// newLog makes [4] HISTORY, shown while [1] leads (screen.go): the commits
// that changed the file under [1]'s cursor (the user), following it (tdp
// panel.13), the newest first, as [3]'s commits are written but with no
// graph.
func (s *statusScreen) newLog() {
	s.logList = &terminu.List{
		Columns: []terminu.Column{
			{Title: "Hash"},
			{Title: "Description", Shrink: true, MinWidth: 12, Fill: true},
			{Title: "Author", Shrink: true, MinWidth: 6, Drop: 2},
			{Title: "Date", Drop: 1},
		},
		Spread:     terminu.SpreadFill, // as [3]'s commits (tdp panel.5, panel-184)
		Filterable: true,
	}
	s.logP = &terminu.Panel{Label: "History", Body: s.logList}
	s.logP.Actions = append([]terminu.Action{
		{Key: "Enter", Label: "Diff", Desc: "this commit's change to the file, UNIFIED or SPLIT", Scope: item, Hint: "diff",
			// binary: no text to compare, so no Diff (tdp panel.9, M6)
			Hidden: func() bool { c, ok := s.logCommit(); return !ok || c.Binary },
			Do:     s.openLogged},
		s.g.unzoom(),
	}, s.logList.Actions()...)
}

// logPath is the file [4] is the history of: the conflicted file [1] went
// into, else the one under its cursor (a rename's old path: the new one has
// no commits yet); "" for none.
func (s *statusScreen) logPath() string {
	if s.conflict != "" {
		return s.conflict
	}
	f, _, ok := s.file()
	switch {
	case !ok:
		return ""
	case f.Orig != "":
		return f.Orig
	}
	return f.Path
}

// showLog puts in [4] the history of the file under [1]'s cursor, read again
// when the file or HEAD changes (tdp panel.15 on a read that failed).
func (s *statusScreen) showLog() {
	path, snap := s.logPath(), s.g.snap
	key := path
	if snap != nil {
		key += "@" + snap.HeadSHA
	}
	if key == s.logOf {
		return
	}
	s.logOf, s.logged, s.logP.Fail = key, nil, nil
	s.logList.ClearFilter() // another file's history in it (tdp panel.14)
	s.logP.Name = path
	s.logList.EmptyHint = "Press [1] to go to the files"
	switch {
	case path == "":
		s.logList.EmptyFact = "No file to show"
		if s.files.Empty() { // as [3]: nothing changed, so where [1] sends you (the user)
			s.logList.EmptyHint = "Press [2] to see the history"
		}
		s.logList.SetRows(nil)
		return
	case snap == nil || snap.HeadSHA == "":
		s.logList.EmptyFact = "No commits yet"
		s.logList.SetRows(nil)
		return
	}
	cs, err := s.g.repo.FileLog(path)
	if err != nil {
		s.logP.Fail = &terminu.Failure{Fact: "Can't read the history of " + path + ": " + reason(gitLines(err)), Hint: "Press [R] to read it again"}
		s.logOf = "" // tried again on the next read
		s.logList.SetRows(nil)
		return
	}
	s.logList.EmptyFact = "Not in any commit yet"
	s.logged = map[string]git.FileCommit{}
	rows := make([]terminu.Row, len(cs))
	for i, c := range cs {
		date := c.Date.Format("2006-01-02")
		rows[i] = terminu.Row{ID: c.SHA, Cells: []string{git.Short(c.SHA), c.Subject, c.Author, date},
			Styled: [][]terminu.Span{nil, nil, nil, {{Text: date, Style: terminu.Fg(terminu.Overlay0)}}}}
		s.logged[c.SHA] = c
	}
	s.logList.SetRows(rows)
}

// logCommit is the commit under [4]'s cursor.
func (s *statusScreen) logCommit() (git.FileCommit, bool) {
	r, ok := s.logList.Current()
	if !ok {
		return git.FileCommit{}, false
	}
	c, ok := s.logged[r.ID]
	return c, ok
}

// openLogged opens the two versions of the file in the commit under [4]'s
// cursor (tdp note.8): <path> @ <parent> ↔ <path> @ <commit>, the first
// commit's against nothing.
func (s *statusScreen) openLogged() tea.Cmd {
	c, ok := s.logCommit()
	if !ok || c.Binary {
		return nil
	}
	path, repo := s.logPath(), s.g.repo
	from, fromName := "", "empty"
	if len(c.Parents) > 0 {
		from, fromName = c.Parents[0], git.Short(c.Parents[0])
	}
	it := terminu.DiffItem{Name: path, Mark: terminu.DiffChanged,
		Anchor: path + " @ " + fromName, Other: path + " @ " + git.Short(c.SHA),
		Text: func() (string, string, error) {
			var a, b string
			var err error
			if from != "" {
				if a, err = repo.Show(from, path); err != nil {
					return "", "", err
				}
			}
			if b, err = repo.Show(c.SHA, path); err != nil {
				return "", "", err
			}
			return a, b, nil
		}}
	s.g.a.Log(terminu.Info, "Compared "+it.Anchor+" with "+it.Other)
	return s.g.a.OpenDiff(s.logP, it)
}
