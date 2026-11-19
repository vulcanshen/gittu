package ui

import (
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/gitreu/internal/git"
)

// The groups of [1] FILES, the start of each row's ID (tdp panel.11).
const (
	inConflict = "c:"
	inStaged   = "s:"
	inUnstaged = "u:"
)

// statusScreen is File status (SourceTree's): [1] the working tree's files
// with a change, [2] the change of the one under the cursor (tdp panel.13).
type statusScreen struct {
	g      *gitreu
	screen *terminu.Screen

	filesP *terminu.Panel
	files  *terminu.List
	// conflict is the conflicted file [1] has gone into, "" when it shows the
	// files (tdp panel.10); hunks are its conflicted blocks.
	conflict string
	hunks    []git.Hunk
	hunkList *terminu.List

	diffP *terminu.Panel
	diff  *terminu.Pager
	// shown is what [2] shows, the row's ID it is of, to tell new content
	// from the same read again (tdp note.9).
	shown string
}

func newStatusScreen(g *gitreu) *statusScreen {
	s := &statusScreen{g: g}
	s.files = &terminu.List{
		Columns:   []terminu.Column{{Title: "", Shrink: true, Path: true}},
		Flags:     true,
		Location:  "Working tree",
		EmptyFact: "Nothing to commit",
		EmptyHint: "Press [H] to see the history",
		OnMove:    func(terminu.Row) tea.Cmd { s.showDiff(false); return nil },
	}
	s.hunkList = &terminu.List{
		Columns: []terminu.Column{{Title: "", Shrink: true}},
		OnMove:  func(terminu.Row) tea.Cmd { s.showDiff(false); return nil },
	}
	s.diff = &terminu.Pager{Follow: true, EmptyFact: "No file to show", EmptyHint: "Press [1] to go to the files"}
	s.diffP = &terminu.Panel{Label: "Diff", Body: s.diff}
	s.filesP = &terminu.Panel{Label: "Files", Body: s.files, Details: s.diffP}
	s.filesP.Actions = s.fileActions()
	s.diffP.Actions = []terminu.Action{
		{Key: "Enter", Label: "Open", Desc: "this in a viewer, to search and copy", Scope: panel, Hint: "open",
			Hidden: func() bool { return s.diff.Empty() }, Do: s.openDiff},
	}
	s.screen = &terminu.Screen{Name: "File status", Key: "F", Desc: "the files to commit and their changes",
		Panels: []*terminu.Panel{s.filesP, s.diffP}, Layout: oneToTwo}
	return s
}

// show puts a read of the repository on the screen.
func (s *statusScreen) show(snap *git.Snapshot) {
	s.files.SetRows(fileRows(snap.Files))
	if s.conflict != "" {
		if !slices.ContainsFunc(snap.Files, func(f git.File) bool { return f.Path == s.conflict && f.Conflicted() }) {
			s.goOut()
		} else {
			s.readHunks()
		}
	}
	s.showDiff(true)
}

// fileRows are [1]'s rows in three groups: a file staged in part is in two.
func fileRows(files []git.File) []terminu.Row {
	var conflicts, staged, unstaged []terminu.Row
	for _, f := range files {
		path := f.Path
		if f.Orig != "" {
			path = f.Orig + " → " + f.Path
		}
		switch {
		case f.Conflicted():
			conflicts = append(conflicts, terminu.Row{ID: inConflict + f.Path, Group: "Conflicts", Cells: []string{path}, Flag: changeFlag('U')})
			continue
		case f.Untracked():
			unstaged = append(unstaged, terminu.Row{ID: inUnstaged + f.Path, Group: "Unstaged", Cells: []string{f.Path}, Flag: changeFlag('?')})
			continue
		}
		if f.Staged() {
			staged = append(staged, terminu.Row{ID: inStaged + f.Path, Group: "Staged", Cells: []string{path}, Flag: changeFlag(f.X)})
		}
		if f.Unstaged() {
			unstaged = append(unstaged, terminu.Row{ID: inUnstaged + f.Path, Group: "Unstaged", Cells: []string{f.Path}, Flag: changeFlag(f.Y)})
		}
	}
	return slices.Concat(conflicts, staged, unstaged)
}

// file is the file under [1]'s cursor and its group.
func (s *statusScreen) file() (git.File, string, bool) {
	if s.conflict != "" || s.g.snap == nil {
		return git.File{}, "", false
	}
	r, ok := s.files.Current()
	if !ok {
		return git.File{}, "", false
	}
	group, path := r.ID[:2], r.ID[2:]
	i := slices.IndexFunc(s.g.snap.Files, func(f git.File) bool { return f.Path == path })
	if i < 0 {
		return git.File{}, "", false
	}
	return s.g.snap.Files[i], group, true
}

func (s *statusScreen) in(group string) func() bool {
	return func() bool { _, g, ok := s.file(); return !ok || g != group }
}

// hunk is the conflicted block under the cursor, and its place.
func (s *statusScreen) hunk() (git.Hunk, int, bool) {
	if s.conflict == "" {
		return git.Hunk{}, 0, false
	}
	r, ok := s.hunkList.Current()
	if !ok {
		return git.Hunk{}, 0, false
	}
	i, err := strconv.Atoi(r.ID)
	if err != nil || i >= len(s.hunks) {
		return git.Hunk{}, 0, false
	}
	return s.hunks[i], i, true
}

// fileActions are [1]'s operations: on a file by its group, on a conflicted
// block, and on the whole panel (tdp P3, M2).
func (s *statusScreen) fileActions() []terminu.Action {
	g := s.g
	off := func() bool { return !g.idle() }
	path := func() string { f, _, _ := s.file(); return f.Path }
	noHunk := func() bool { _, _, ok := s.hunk(); return !ok }
	inFiles := func() bool { return s.conflict != "" }
	return []terminu.Action{
		{Key: "Enter", Label: "Stage", Desc: "this file's change", Scope: item, Hint: "stage",
			Hidden: s.in(inUnstaged), Disabled: off, Do: func() tea.Cmd {
				p := path()
				return g.run("Staging…", "Stage", func() error { return g.repo.Stage(p) },
					func() tea.Cmd { g.a.Log(terminu.Info, "Staged "+p); return nil })
			}},
		{Key: "Enter", Label: "Unstage", Desc: "this file's change, keeping it", Scope: item, Hint: "unstage",
			Hidden: s.in(inStaged), Disabled: off, Do: func() tea.Cmd {
				p := path()
				unborn := g.snap.HeadSHA == ""
				return g.run("Unstaging…", "Unstage", func() error { return g.repo.Unstage(p, unborn) },
					func() tea.Cmd { g.a.Log(terminu.Info, "Unstaged "+p); return nil })
			}},
		{Key: "Enter", Label: "Resolve", Desc: "its conflicts one by one", Scope: item, Hint: "resolve",
			Hidden: s.in(inConflict), Do: func() tea.Cmd { return s.goIn(path()) }},
		{Key: "m", Label: "Use mine", Desc: "for the whole file, and stage it", Scope: item,
			Hidden: s.in(inConflict), Disabled: off, Do: func() tea.Cmd { return s.useSide(path(), false) }},
		{Key: "t", Label: "Use theirs", Desc: "for the whole file, and stage it", Scope: item,
			Hidden: s.in(inConflict), Disabled: off, Do: func() tea.Cmd { return s.useSide(path(), true) }},
		{Key: "D", Label: "Discard", Desc: "this file's change", Scope: item,
			Hidden: func() bool { _, gr, ok := s.file(); return !ok || gr == inConflict }, Disabled: off,
			AskFor: func() *terminu.Ask {
				f, gr, _ := s.file()
				q, d := "Discard the change to "+f.Path+"?", "The change is lost: it can't be undone."
				switch {
				case f.Untracked():
					q, d = "Delete "+f.Path+"?", "git does not track it: it is deleted, and can't be undone."
				case gr == inStaged:
					d = "Its staged change and its change in the working tree are lost: it can't be undone."
				}
				return &terminu.Ask{Title: "Discard", Subject: f.Path, Question: q, Verb: "discard", Details: []string{d}, Warn: []int{0}}
			},
			Do: func() tea.Cmd {
				f, gr, _ := s.file()
				return g.run("Discarding…", "Discard", func() error { return g.repo.Discard(f, gr == inStaged) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Discarded the change to "+f.Path) })
			}},
		// A conflicted block.
		{Key: "Enter", Label: "Read", Desc: "it whole in the panel beside", Scope: item, Hint: "read",
			Hidden: noHunk, ToDetails: func() bool { return true }, Do: func() tea.Cmd { return nil }},
		{Key: "m", Label: "Use mine", Desc: "for this conflict", Scope: item, Hint: "mine",
			Hidden: noHunk, Disabled: off, Do: func() tea.Cmd { return s.resolve(git.Mine) }},
		{Key: "t", Label: "Use theirs", Desc: "for this conflict", Scope: item, Hint: "theirs",
			Hidden: noHunk, Disabled: off, Do: func() tea.Cmd { return s.resolve(git.Theirs) }},
		{Key: "b", Label: "Use both", Desc: "mine first, then theirs", Scope: item, Hint: "both",
			Hidden: noHunk, Disabled: off, Do: func() tea.Cmd { return s.resolve(git.Both) }},
		// The panel.
		{Key: "a", Label: "Stage all", Desc: "every change, untracked files too", Scope: panel,
			Hidden: inFiles, Disabled: func() bool {
				return off() || !slices.ContainsFunc(g.snap.Files, func(f git.File) bool { return f.Unstaged() })
			},
			Do: func() tea.Cmd {
				return g.run("Staging…", "Stage", g.repo.StageAll, func() tea.Cmd { g.a.Log(terminu.Info, "Staged every change"); return nil })
			}},
		{Label: "Unstage all", Desc: "every staged change, keeping them", Scope: panel,
			Hidden: inFiles, Disabled: func() bool {
				return off() || !slices.ContainsFunc(g.snap.Files, func(f git.File) bool { return f.Staged() })
			},
			Do: func() tea.Cmd {
				unborn := g.snap.HeadSHA == ""
				return g.run("Unstaging…", "Unstage", func() error { return g.repo.UnstageAll(unborn) },
					func() tea.Cmd { g.a.Log(terminu.Info, "Unstaged every change"); return nil })
			}},
	}
}

// showDiff shows in [2] the change of the row under [1]'s cursor; again, it
// is the same row read again (tdp note.9).
func (s *statusScreen) showDiff(again bool) {
	id, label, name, lines := "", "Diff", "", []string(nil)
	if h, i, ok := s.hunk(); ok {
		id, label, name, lines = "h:"+s.conflict+":"+strconv.Itoa(i), "Conflict", s.conflict, s.hunkLines(h)
	} else if f, group, ok := s.file(); ok {
		id, name = group+f.Path, f.Path
		var err error
		lines, err = s.g.repo.WorkDiff(f, group == inStaged)
		if err != nil {
			lines = append([]string{"Can't show the change:"}, errLines(err)...)
		}
		lines = terminu.Syntax("diff", lines)
	}
	s.diffP.Label, s.diffP.Name = label, name
	if again && id == s.shown {
		if !slices.Equal(lines, s.diff.Lines()) {
			s.diff.ReadAgain(lines)
		}
		return
	}
	s.shown = id
	s.diff.SetLines(lines)
}

// hunkLines are a conflicted block as [2] shows it: mine above theirs, each
// under its heading (tdp P5: their colours are the app's).
func (s *statusScreen) hunkLines(h git.Hunk) []string {
	mine := "MINE · " + h.MineLabel
	if h.MineLabel == "HEAD" && s.g.snap != nil && s.g.snap.Head != "" {
		mine += " (" + s.g.snap.Head + ")"
	}
	lines := []string{terminu.Fg(teal).Bolded().Render(terminu.Escape(mine))}
	lines = append(lines, h.Mine...)
	lines = append(lines, "", terminu.Fg(mauve).Bolded().Render(terminu.Escape("THEIRS · "+h.TheirsLabel)))
	return append(lines, h.Theirs...)
}

// openDiff opens what [2] shows in a viewer, to search and copy (tdp K3: Enter
// on a panel with no items).
func (s *statusScreen) openDiff() tea.Cmd {
	lines := s.diff.Lines()
	title := "Diff"
	if s.conflict != "" {
		title = "Conflict"
	}
	s.g.a.Log(terminu.Info, "Opened the "+strings.ToLower(title)+" of "+s.diffP.Name)
	return s.g.a.Viewer(&terminu.Viewer{Title: title, Subject: s.diffP.Name, Glyph: glyphDiff, Lines: lines, NoNumbers: true})
}

// goIn shows a conflicted file's blocks in [1] (tdp panel.10).
func (s *statusScreen) goIn(path string) tea.Cmd {
	s.conflict = path
	if err := s.readHunks(); err != nil {
		s.conflict = ""
		return s.g.a.ErrorAbout("Resolve failed", path, err.Error())
	}
	s.hunkList.Select("0")
	s.hunkList.Location = "Working tree › " + path
	s.filesP.Body = s.hunkList
	s.filesP.OnEsc = func() tea.Cmd { s.goOut(); s.showDiff(false); return nil }
	s.showDiff(false)
	return nil
}

// goOut goes back to the files, the cursor on the conflicted file (tdp K4).
func (s *statusScreen) goOut() {
	if s.conflict == "" {
		return
	}
	s.files.Select(inConflict + s.conflict)
	s.conflict, s.hunks = "", nil
	s.filesP.Body = s.files
	s.filesP.OnEsc = nil
}

// readHunks reads the conflicted file's blocks again.
func (s *statusScreen) readHunks() error {
	hs, err := s.g.repo.Conflicts(s.conflict)
	if err != nil {
		return err
	}
	s.hunks = hs
	rows := make([]terminu.Row, len(hs))
	for i, h := range hs {
		rows[i] = terminu.Row{ID: strconv.Itoa(i), Cells: []string{"Conflict " + strconv.Itoa(i+1) + " · lines " + strconv.Itoa(h.Start) + "–" + strconv.Itoa(h.End)}}
	}
	s.hunkList.SetRows(rows)
	return nil
}

// resolve makes the conflicted block under the cursor what choice says; the
// last one done, the file is staged and [1] goes back to the files.
func (s *statusScreen) resolve(choice git.Choice) tea.Cmd {
	g := s.g
	_, i, ok := s.hunk()
	if !ok {
		return nil
	}
	path := s.conflict
	left, err := g.repo.Resolve(path, i, choice)
	if err != nil {
		return g.a.ErrorAbout("Resolve failed", path, err.Error())
	}
	word := map[git.Choice]string{git.Mine: "mine", git.Theirs: "theirs", git.Both: "both"}[choice]
	g.a.Log(terminu.Info, "Used "+word+" in conflict "+strconv.Itoa(i+1)+" of "+path)
	if left > 0 {
		if err := s.readHunks(); err != nil {
			return g.a.ErrorAbout("Resolve failed", path, err.Error())
		}
		s.hunkList.Select(strconv.Itoa(min(i, left-1)))
		s.showDiff(false)
		return nil
	}
	return g.run("Staging…", "Stage", func() error { return g.repo.Stage(path) },
		func() tea.Cmd {
			s.goOut()
			s.files.Select(inStaged + path)
			s.showDiff(false)
			return g.a.Toast(terminu.Success, path+" resolved and staged")
		})
}

// useSide resolves a whole conflicted file with one side and stages it.
func (s *statusScreen) useSide(path string, theirs bool) tea.Cmd {
	g := s.g
	word := "mine"
	if theirs {
		word = "theirs"
	}
	return g.run("Resolving…", "Resolve", func() error { return g.repo.UseSide(path, theirs) },
		func() tea.Cmd {
			s.files.Select(inStaged + path)
			return g.a.Toast(terminu.Success, path+" resolved with "+word+" and staged")
		})
}

// commit commits the staged changes with a message the user writes
// (tdp textarea); during a merge, with the message git prepared.
func (g *gitreu) commit() tea.Cmd {
	if g.snap.State.Op == "merge" {
		msg := g.repo.MergeMessage()
		return g.a.Confirm(terminu.Ask{Title: "Commit", Question: "Commit the merge?", Verb: "commit", Details: []string{firstLine(msg)}},
			func() tea.Cmd { return g.doCommit(msg, false) })
	}
	// TODO(tgo): an empty textarea taken with Enter untouched closes with no
	// error row, where tdp K3 has a value that may not be empty say so; the
	// Check here only stops a message emptied after typing.
	return g.a.Textarea(terminu.Textarea{Title: "Commit", Glyph: glyphCommit, Name: "Message", Old: g.draft,
		Check: func(msg string) error {
			if strings.TrimSpace(msg) == "" {
				return terminu.ErrEmpty
			}
			return nil
		},
		Done: func(msg string) tea.Cmd { g.draft = msg; return g.doCommit(msg, false) }})
}

// doCommit runs git commit; the message is kept until it goes through, to
// write again after a hook stopped it.
func (g *gitreu) doCommit(msg string, amend bool) tea.Cmd {
	verb := "Commit"
	if amend {
		verb = "Amend"
	}
	return g.run("Committing…", verb, func() error { return g.repo.Commit(msg, amend) },
		func() tea.Cmd {
			g.draft = ""
			sha := ""
			if g.snap != nil {
				sha = git.Short(g.snap.HeadSHA)
			}
			return g.a.Toast(terminu.Success, "Committed "+sha+" "+firstLine(msg))
		})
}
