package ui

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/gittu/internal/git"
)

// The groups of [1] FILES, the start of each row's ID (tdp panel.11).
const (
	inConflict = "c:"
	inStaged   = "s:"
	inUnstaged = "u:"
)

// statusScreen is Working tree: [1] the files with
// a change, [2] the change of the one under the cursor (tdp panel.13).
type statusScreen struct {
	g      *gittu
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
	// was and now are the two versions [2] draws; text, that they are text
	// to compare (tdp panel.9: what is not text has no comparison).
	was, now string
	text     bool
	// reread is the file [2] shows when its text changed on a read again,
	// for the App Log to say (task 19).
	reread string
	// failed is why [2] could not be read again, what was read before
	// staying: an Error toast to show (tdp panel.15). failedWas is the last
	// such reason, said once while it stays the same.
	failed, failedWas string
}

func newStatusScreen(g *gittu) *statusScreen {
	s := &statusScreen{g: g}
	s.files = &terminu.List{
		Columns:   []terminu.Column{{Title: "", Shrink: true, Path: true}},
		Flags:     true,
		Location:  "All changes",
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
	s.filesP = &terminu.Panel{Label: "Changes", Body: s.files, Details: s.diffP}
	s.filesP.Actions = s.fileActions()
	s.diffP.Actions = []terminu.Action{
		{Key: "Enter", Label: "Diff", Desc: "its two versions, UNIFIED or SPLIT", Scope: panel, Hint: "diff",
			Hidden: func() bool { return s.diff.Empty() || !s.text }, Do: s.openDiff},
	}
	s.screen = &terminu.Screen{Name: "Working tree", Key: "W", Desc: "the files to commit and their changes",
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

// showDiff shows in [2] the change of the row under [1]'s cursor, its two
// versions drawn as UNIFIED (tdp note.8 "In a panel"), the location row
// naming them; again, it is the same row read again (tdp note.9).
func (s *statusScreen) showDiff(again bool) {
	id, label, name := "", "Diff", ""
	item, ok := s.item()
	h, i, inHunk := s.hunk()
	if inHunk {
		id, label, name = "h:"+s.conflict+":"+strconv.Itoa(i), "Conflict", s.conflict
	} else if f, group, in := s.file(); in {
		id, name = group+f.Path, f.Path
	}
	if !again || id != s.shown {
		s.diffP.LeaveDiff() // another row: out of the list [2] went into for the last one
	}
	s.diffP.Label, s.diffP.Name, s.diffP.Fail = label, name, nil
	s.diff.EmptyFact, s.diff.EmptyHint = "No file to show", "Press [1] to go to the files"
	s.text = false
	if inHunk { // a conflict: its two sides, each under its heading
		s.diffP.Location, s.text = "", true
		lines := s.hunkLines(h)
		if again && id == s.shown {
			if !slices.Equal(lines, s.diff.Lines()) {
				s.diff.ReadAgain(lines)
			}
			return
		}
		s.shown, s.was, s.now = id, "", ""
		s.diff.SetLines(lines)
		return
	}
	if !ok {
		s.diffP.Location = ""
		s.shown, s.was, s.now = id, "", ""
		s.diff.SetLines(nil)
		return
	}
	s.diffP.Location = item.Anchor + " ↔ " + item.Other
	if item.Items != nil { // a new folder holds many: its files, each + (tdp note.8, panel.9)
		s.showItems(again, id, name, item)
		return
	}
	was, now, err := item.Text()
	if err != nil {
		lines := gitLines(err)
		why := "Can't show the change to " + name + ": " + reason(lines)
		if again && id == s.shown && (s.was != "" || s.now != "") { // what was read before stays (tdp panel.15)
			s.text = true
			if why != s.failedWas {
				s.failed, s.failedWas = why, why
				s.g.logAll(lines)
			}
			return
		}
		s.g.a.Log(terminu.Error, why)
		s.g.logAll(lines)
		s.diffP.Fail = &terminu.Failure{Fact: "Can't show the change: " + reason(lines), Hint: "Press [R] to read it again"}
		s.shown, s.was, s.now = id, "", ""
		s.diff.SetLines(nil)
		return
	}
	if s.failedWas != "" { // it failed before: one INFO, no toast (tdp note.7, note-105)
		s.g.a.Log(terminu.Info, "The change to "+name+" can be shown again")
	}
	s.failedWas = ""
	if binaryText(was) || binaryText(now) {
		s.diff.EmptyFact, s.diff.EmptyHint = "Not a text file: no lines to compare", "Press [1] to go to the files"
		s.shown, s.was, s.now = id, "", ""
		s.diff.SetLines(nil)
		return
	}
	s.text = true
	s.diff.EmptyFact, s.diff.EmptyHint = "No differences between the two", "Press [1] to go to the files"
	if again && id == s.shown && was == s.was && now == s.now {
		return
	}
	if again && id == s.shown {
		s.reread = name
	}
	s.shown, s.was, s.now = id, was, now
	s.diff.SetDiff(was, now)
}

// hunkLines are a conflicted block as [2] shows it: mine above theirs, each
// under its heading (tdp P5: their colours are the app's).
func (s *statusScreen) hunkLines(h git.Hunk) []string {
	lines := []string{terminu.Fg(teal).Bolded().Render(terminu.Escape(s.mineName(h)))}
	lines = append(lines, h.Mine...)
	lines = append(lines, "", terminu.Fg(mauve).Bolded().Render(terminu.Escape("THEIRS · "+h.TheirsLabel)))
	return append(lines, h.Theirs...)
}

// mineName names a conflict's own side: MINE · HEAD (main).
func (s *statusScreen) mineName(h git.Hunk) string {
	mine := "MINE · " + h.MineLabel
	if h.MineLabel == "HEAD" && s.g.snap != nil && s.g.snap.Head != "" {
		mine += " (" + s.g.snap.Head + ")"
	}
	return mine
}

// showItems shows in [2] the items that differ in what holds many, each
// with its mark (tdp note.8 "In a panel"): an untracked folder's files, all
// +. Enter goes into the same list (openDiff).
func (s *statusScreen) showItems(again bool, id, name string, item terminu.DiffItem) {
	items, err := item.Items()
	if err != nil {
		lines := gitLines(err)
		s.g.a.Log(terminu.Error, "Can't show the change to "+name+": "+reason(lines))
		s.g.logAll(lines)
		s.diffP.Fail = &terminu.Failure{Fact: "Can't show the change: " + reason(lines), Hint: "Press [R] to read it again"}
		s.shown, s.was, s.now = id, "", ""
		s.diff.SetLines(nil)
		return
	}
	s.text = true
	s.diff.EmptyFact, s.diff.EmptyHint = "No files in it to show", "Press [1] to go to the files"
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Name
	}
	listed := strings.Join(names, "\n")
	if again && id == s.shown && listed == s.now {
		return
	}
	if again && id == s.shown {
		s.reread = name
	}
	s.shown, s.was, s.now = id, "", listed
	s.diff.SetDiffItems(items)
}

// folderItems are the files of an untracked folder, each as added: nothing
// in the index, the file in the working tree; one git takes for binary has
// no comparison (tdp panel.9).
func (s *statusScreen) folderItems(dir string) ([]terminu.DiffItem, error) {
	repo := s.g.repo
	files, err := repo.UntrackedFiles(dir)
	if err != nil {
		return nil, err
	}
	items := make([]terminu.DiffItem, 0, len(files))
	for _, p := range files {
		it := terminu.DiffItem{Mark: terminu.DiffAdded, Name: p, Anchor: p + " @ index", Other: p}
		if !binaryFile(filepath.Join(repo.Dir, filepath.FromSlash(p))) {
			it.Text = func() (string, string, error) {
				b, err := repo.WorkFile(p)
				return "", b, err
			}
		}
		items = append(items, it)
	}
	return items, nil
}

// binaryFile reports a file git would take for binary, from its first 8000
// bytes.
func binaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 8000)
	n, _ := io.ReadFull(f, b)
	return binaryText(string(b[:n]))
}

// takeReread is the file whose text changed on the last read, once.
func (s *statusScreen) takeReread() string {
	r := s.reread
	s.reread = ""
	return r
}

// said is the Error toast of [2] read again in vain, if there is one.
func (s *statusScreen) said() tea.Cmd {
	if s.failed == "" {
		return nil
	}
	why := s.failed
	s.failed = ""
	return s.g.a.Toast(terminu.Error, why, "Press [R] to read it again")
}

// binaryText reports text git would take for binary: a NUL in its first
// 8000 bytes, as git looks.
func binaryText(t string) bool {
	return strings.IndexByte(t[:min(len(t), 8000)], 0) >= 0
}

// openDiff opens the two versions of what [2] shows, the older the anchor
// (tdp note.8; tdp K3: Enter on a panel with no items).
func (s *statusScreen) openDiff() tea.Cmd {
	item, ok := s.item()
	if !ok {
		return nil
	}
	s.g.a.Log(terminu.Info, "Compared "+item.Anchor+" with "+item.Other)
	return s.g.a.OpenDiff(s.diffP, item)
}

// item is the two versions of the row under [1]'s cursor, the older the
// anchor: a staged change HEAD against the index, an unstaged one the index
// against the working tree, a conflicted file HEAD against the working tree,
// a conflict mine against theirs. An untracked folder has none.
func (s *statusScreen) item() (terminu.DiffItem, bool) {
	repo := s.g.repo
	var item terminu.DiffItem
	if h, _, ok := s.hunk(); ok {
		mine, theirs := strings.Join(h.Mine, "\n"), strings.Join(h.Theirs, "\n")
		item = terminu.DiffItem{Anchor: s.mineName(h), Other: "THEIRS · " + h.TheirsLabel,
			Text: func() (string, string, error) { return mine, theirs, nil }}
	} else if f, group, ok := s.file(); ok {
		path := f.Path
		if f.Untracked() && repo.IsDir(path) {
			return terminu.DiffItem{Anchor: path + " @ index", Other: path,
				Items: func() ([]terminu.DiffItem, error) { return s.folderItems(path) }}, true
		}
		switch group {
		case inStaged:
			old := path
			if f.Orig != "" {
				old = f.Orig
			}
			item = terminu.DiffItem{Anchor: old + " @ HEAD", Other: path + " @ index", Text: func() (string, string, error) {
				a, err := repo.Show("HEAD", old)
				if err != nil {
					return "", "", err
				}
				b, err := repo.Show("", path)
				return a, b, err
			}}
		case inUnstaged:
			item = terminu.DiffItem{Anchor: path + " @ index", Other: path, Text: func() (string, string, error) {
				a, err := repo.Show("", path)
				if err != nil {
					return "", "", err
				}
				b, err := repo.WorkFile(path)
				return a, b, err
			}}
		default: // a conflict: what HEAD has against the file with its markers
			item = terminu.DiffItem{Anchor: path + " @ HEAD", Other: path, Text: func() (string, string, error) {
				a, err := repo.Show("HEAD", path)
				if err != nil {
					return "", "", err
				}
				b, err := repo.WorkFile(path)
				return a, b, err
			}}
		}
	} else {
		return item, false
	}
	return item, true
}

// goIn shows a conflicted file's blocks in [1] (tdp panel.10).
func (s *statusScreen) goIn(path string) tea.Cmd {
	s.conflict = path
	if err := s.readHunks(); err != nil {
		s.conflict = ""
		return s.g.a.ErrorAbout("Resolve failed", path, err.Error())
	}
	s.hunkList.Select("0")
	s.hunkList.Location = "All changes › " + path
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
// (tdp textarea): during a merge it opens with the message git prepared,
// after a hook stopped one with that message; Enter on it unchanged commits
// it (Textarea.Unchanged, tdp K3). Empty is an error, the popup kept open.
func (g *gittu) commit() tea.Cmd {
	ready := g.draft
	if g.snap.State.Op == "merge" && ready == "" {
		ready = g.repo.MergeMessage()
	}
	return g.messageFor("Commit", ready, false)
}

// amend opens the last commit's message to amend it with the staged
// changes: Enter on it unchanged keeps it (tdp K3).
func (g *gittu) amend() tea.Cmd { return g.messageFor("Amend", g.repo.LastMessage(), true) }

// messageFor opens the commit message's textarea with ready in it.
func (g *gittu) messageFor(title, ready string, amend bool) tea.Cmd {
	return g.a.Textarea(terminu.Textarea{Title: title, Glyph: glyphCommit, Name: "Message", Old: ready,
		Check: func(msg string) error {
			if strings.TrimSpace(msg) == "" {
				return terminu.ErrEmpty
			}
			return nil
		},
		Unchanged: func() tea.Cmd { return g.doCommit(ready, amend) },
		Done:      func(msg string) tea.Cmd { g.draft = msg; return g.doCommit(msg, amend) }})
}

// doCommit runs git commit; the message is kept until it goes through, to
// write again after a hook stopped it.
func (g *gittu) doCommit(msg string, amend bool) tea.Cmd {
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

// changeFlag is a working tree change's letter in its colour, in [1]'s flag
// column (tdp panel.7; tdp P5: the letters and colours are the app's).
func changeFlag(status byte) terminu.Span {
	c := terminu.Peach
	switch status {
	case 'A', 'C':
		c = terminu.Green
	case 'D', 'U':
		c = terminu.Red
	case 'R':
		c = terminu.Blue
	case '?':
		c = terminu.Overlay0
	}
	return terminu.Span{Text: string(status), Style: terminu.Fg(c)}
}
