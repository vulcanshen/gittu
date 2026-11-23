package ui

import (
	"context"
	"errors"
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

// statusScreen is the screen's working tree (screen.go): [1] the files with
// a change, and while [1] leads, [3] the change of the one under its cursor
// (tdp panel.13) and [4] that file's history.
type statusScreen struct {
	g *gittu

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
	// width is [1]'s, as laid out last: how much of a path its rows show.
	width int
	// toConflict puts [1]'s cursor on the first conflict at the next read: an
	// operation stopped on conflicts.
	toConflict bool
	// failed is why [3] could not be read again, what was read before
	// staying: an Error toast to show (tdp panel.15). failedWas is the last
	// such reason, said once while it stays the same.
	failed, failedWas string

	// logP is [4] HISTORY, the commits that changed the file under [1]'s
	// cursor (filelog.go); logOf is the path and HEAD it was read for, its
	// commits by row ID.
	logP    *terminu.Panel
	logList *terminu.List
	logOf   string
	logged  map[string]git.FileCommit
}

func newStatusScreen(g *gittu) *statusScreen {
	s := &statusScreen{g: g}
	// Marks: files to work on together, from the Space menu (the user; tdp
	// panel.7, P3). The mark column holds only the tick, so each file's
	// state is in its row, before its path.
	s.files = &terminu.List{
		Columns:   []terminu.Column{{Title: "", Shrink: true, Path: true}},
		Marks:     true,
		Location:  "All changes",
		EmptyFact: "Nothing to commit",
		EmptyHint: "Press [2] to see the history",
		OnMove:    func(terminu.Row) tea.Cmd { s.showDiff(false); return nil },
	}
	s.hunkList = &terminu.List{
		Columns: []terminu.Column{{Title: "", Shrink: true}},
		OnMove:  func(terminu.Row) tea.Cmd { s.showDiff(false); return nil },
	}
	s.diff = &terminu.Pager{Follow: true, EmptyFact: "No file to show", EmptyHint: "Press [1] to go to the files"}
	s.diffP = &terminu.Panel{Label: "Diff", Body: s.diff}
	s.filesP = &terminu.Panel{Label: "Changes", Body: s.files, Details: s.diffP}
	s.filesP.Actions = append(append(s.fileActions(), s.hunkActions()...), g.unzoom())
	s.diffP.Actions = []terminu.Action{
		// A conflicted file: Enter edits it, r marks it resolved (the user).
		{Key: "Enter", Label: "Edit", Desc: "it in your editor, to resolve its conflicts", Scope: panel, Hint: "edit",
			Hidden: s.in(inConflict), Disabled: func() bool { return !g.idle() },
			Do: func() tea.Cmd { f, _, _ := s.file(); return s.edit(f.Path) }},
		s.resolvedAction(panel),
		{Key: "Enter", Label: "Diff", Desc: "its two versions, UNIFIED or SPLIT", Scope: panel, Hint: "diff",
			Hidden: func() bool { return s.diff.Empty() || !s.text || !s.in(inConflict)() }, Do: s.openDiff},
		g.unzoom(),
	}
	s.newLog()
	return s
}

// show puts a read of the repository on the screen.
func (s *statusScreen) show(snap *git.Snapshot) {
	s.files.SetRows(fileRows(snap.Files, s.pathWidth()))
	// A mark on a row gone (a file staged, resolved…) goes with it.
	if marked := s.files.Marked(); len(marked) > 0 {
		here := map[string]bool{}
		for _, r := range s.files.Rows() {
			here[r.ID] = true
		}
		keep := slices.DeleteFunc(slices.Clone(marked), func(id string) bool { return !here[id] })
		if len(keep) != len(marked) {
			s.files.SetMarked(keep)
		}
	}
	if s.toConflict {
		s.toConflict = false
		if i := slices.IndexFunc(snap.Files, git.File.Conflicted); i >= 0 {
			s.files.ClearFilter() // gittu puts the cursor on it (tdp panel.14)
			s.files.Select(inConflict + snap.Files[i].Path)
		}
	}
	if s.conflict != "" {
		if !slices.ContainsFunc(snap.Files, func(f git.File) bool { return f.Path == s.conflict && f.Conflicted() }) {
			s.goOut()
		} else {
			s.readHunks()
		}
	}
	s.showDiff(true)
}

// laidOut takes [1]'s width as laid out (screen.go): wider or narrower, its
// rows' paths cut again.
func (s *statusScreen) laidOut(w int) {
	if w != s.width {
		s.width = w
		if s.g.snap != nil {
			s.files.SetRows(fileRows(s.g.snap.Files, s.pathWidth()))
		}
	}
}

// pathWidth is how many cells of a path [1]'s rows show: its width but the
// frame, the margins, the mark column, the state's letter and a blank, the
// cursor's arrow, and one to spare (0: not laid out yet, the path whole).
func (s *statusScreen) pathWidth() int {
	if s.width <= 0 {
		return 0
	}
	return max(s.width-9, 4)
}

// cutFront shortens a path from its front to w cells, … first, so that
// its end, the file's name, stays (0: whole).
func cutFront(path string, w int) string {
	if w <= 0 || terminu.Width(path) <= w {
		return path
	}
	rs := []rune(path)
	keep := len(rs)
	for keep > 0 && terminu.Width(string(rs[keep-1:])) <= w-1 {
		keep--
	}
	return "…" + string(rs[keep:])
}

// fileRows are [1]'s rows in three groups: a file staged in part is in two.
// w is how many cells of a path they show.
func fileRows(files []git.File, w int) []terminu.Row {
	var conflicts, staged, unstaged []terminu.Row
	for _, f := range files {
		path := f.Path
		if f.Orig != "" {
			path = f.Orig + " → " + f.Path
		}
		switch {
		case f.Conflicted():
			conflicts = append(conflicts, fileRow(inConflict, "Conflicts", f.Path, path, 'U', w))
			continue
		case f.Untracked():
			unstaged = append(unstaged, fileRow(inUnstaged, "Unstaged", f.Path, f.Path, '?', w))
			continue
		}
		if f.Staged() {
			staged = append(staged, fileRow(inStaged, "Staged", f.Path, path, f.X, w))
		}
		if f.Unstaged() {
			unstaged = append(unstaged, fileRow(inUnstaged, "Unstaged", f.Path, f.Path, f.Y, w))
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

// marked are the files of the marked rows in these groups, each once, in
// the list's order; a file in two groups (staged in part) counts once.
func (s *statusScreen) marked(groups []string, ok func(git.File) bool) []git.File {
	if s.g.snap == nil {
		return nil
	}
	var out []git.File
	seen := map[string]bool{}
	for _, id := range s.files.Marked() {
		group, path := id[:2], id[2:]
		if !slices.Contains(groups, group) || seen[path] {
			continue
		}
		i := slices.IndexFunc(s.g.snap.Files, func(f git.File) bool { return f.Path == path })
		if i < 0 || ok != nil && !ok(s.g.snap.Files[i]) {
			continue
		}
		seen[path] = true
		out = append(out, s.g.snap.Files[i])
	}
	return out
}

// batch is a panel operation on the marked files of these groups (tdp P3),
// there while some are marked, off when none it works on is: it asks with
// ask when that is set, runs do in the background, then says done and
// clears the marks.
func (s *statusScreen) batch(label, busy, title string, groups []string, ask func([]git.File) *terminu.Ask,
	do func([]git.File) error, done func([]git.File) tea.Cmd) terminu.Action {
	g := s.g
	return s.batchWith(label, groups, nil, ask, func(fs []git.File) tea.Cmd {
		return g.run(busy, title, func() error { return do(fs) }, func() tea.Cmd { return done(fs) })
	})
}

// batchWith is batch with its own run, on the marked files ok takes.
func (s *statusScreen) batchWith(label string, groups []string, ok func(git.File) bool, ask func([]git.File) *terminu.Ask,
	run func([]git.File) tea.Cmd) terminu.Action {
	n := func() int { return len(s.marked(groups, ok)) }
	a := terminu.Action{Label: label, Scope: panel,
		Live:     func() (string, string) { return label, plural(n(), "file", "files") },
		Hidden:   func() bool { return s.conflict != "" || len(s.files.Marked()) == 0 },
		Disabled: func() bool { return !s.g.idle() || n() == 0 },
		Do: func() tea.Cmd {
			fs := s.marked(groups, ok)
			s.files.ClearMarks()
			return run(fs)
		}}
	if ask != nil {
		a.AskFor = func() *terminu.Ask { return ask(s.marked(groups, ok)) }
	}
	return a
}

// withKey gives a an action key.
func withKey(key string, a terminu.Action) terminu.Action {
	a.Key = key
	return a
}

// allMarked reports every file of [1] marked, some listed.
func (s *statusScreen) allMarked() bool {
	return !s.files.Empty() && len(s.files.Marked()) == len(s.files.Rows())
}

// isMarked reports the row under [1]'s cursor marked.
func (s *statusScreen) isMarked() bool {
	r, ok := s.files.Current()
	return ok && slices.Contains(s.files.Marked(), r.ID)
}

// toggleMark marks or unmarks the file under the cursor (tdp panel.7).
func (s *statusScreen) toggleMark() tea.Cmd {
	f, _, ok := s.file()
	if !ok {
		return nil
	}
	word := "Marked "
	if s.isMarked() {
		word = "Unmarked "
	}
	s.files.ToggleMark()
	s.g.a.Log(terminu.Info, word+f.Path)
	return nil
}

// inIndex reports a file git tracks that is still in the index: not new and
// untracked, not a deletion staged already.
func inIndex(f git.File) bool { return !f.Untracked() && f.X != 'D' }

func paths(fs []git.File) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Path
	}
	return out
}

// named is what a question about these files names: the path of one, how
// many of several.
func named(fs []git.File) string {
	if len(fs) == 1 {
		return fs[0].Path
	}
	return plural(len(fs), "file", "files")
}

// listed are the paths a confirm lists, eight at the most.
func listed(fs []git.File) []string {
	var out []string
	for i, f := range fs {
		if i == 8 {
			out = append(out, "and "+plural(len(fs)-8, "more", "more"))
			break
		}
		out = append(out, f.Path)
	}
	return out
}

func (s *statusScreen) askDiscard(fs []git.File) *terminu.Ask {
	d := listed(fs)
	d = append(d, "Their changes are lost; files git does not track are deleted: it can't be undone.")
	return &terminu.Ask{Title: "Discard", Subject: named(fs), Question: "Discard the changes to " + named(fs) + "?", Verb: "discard",
		Details: d, Warn: []int{len(d) - 1}}
}

func (s *statusScreen) askRemove(fs []git.File) *terminu.Ask {
	var d []string
	if len(fs) > 1 {
		d = listed(fs)
	}
	d = append(d, "Deleted, and the next commit takes them out of the repository.")
	warn := []int{}
	if slices.ContainsFunc(fs, func(f git.File) bool { return f.Staged() || f.Unstaged() }) {
		d = append(d, "Their changes not committed are lost: it can't be undone.")
		warn = append(warn, len(d)-1)
	}
	return &terminu.Ask{Title: "Remove", Subject: named(fs), Question: "Remove " + named(fs) + " from the repository?", Verb: "remove",
		Details: d, Warn: warn}
}

// remove deletes tracked files and stages their deletion.
func (s *statusScreen) remove(fs []git.File) tea.Cmd {
	g := s.g
	return g.run("Removing…", "Remove", func() error { return g.repo.Remove(paths(fs)...) }, func() tea.Cmd {
		if len(fs) > 1 {
			for _, f := range fs {
				g.a.Log(terminu.Info, "Removed "+f.Path)
			}
		}
		return g.a.Toast(terminu.Success, "Removed "+named(fs)+" from the repository")
	})
}

// askIgnore asks only when a tracked file is among them: it stops being
// tracked.
func (s *statusScreen) askIgnore(fs []git.File) *terminu.Ask {
	if !slices.ContainsFunc(fs, inIndex) {
		return nil
	}
	var d []string
	if len(fs) > 1 {
		d = listed(fs)
	}
	d = append(d, "Tracked ones stop being tracked: the next commit takes them out of the repository, the files stay.")
	return &terminu.Ask{Title: "Ignore", Subject: named(fs), Question: "Add " + named(fs) + " to .gitignore?", Verb: "ignore",
		Details: d, Warn: []int{len(d) - 1}}
}

// ignore adds files to the .gitignore at the top, untracking the tracked
// ones (git ignores only what it does not track).
func (s *statusScreen) ignore(fs []git.File) tea.Cmd {
	g := s.g
	var tracked []string
	for _, f := range fs {
		if inIndex(f) {
			tracked = append(tracked, f.Path)
		}
	}
	return g.run("Ignoring…", "Ignore", func() error { return g.repo.Ignore(paths(fs), tracked) }, func() tea.Cmd {
		if len(fs) > 1 {
			for _, f := range fs {
				g.a.Log(terminu.Info, "Added "+f.Path+" to .gitignore")
			}
		}
		return g.a.Toast(terminu.Success, "Added "+named(fs)+" to .gitignore")
	})
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
// hunkActions are [1]'s operations on the blocks of the conflicted file gone
// into, Hidden out of it; their keys stay [1]'s there too (tdp M4-5): t and b
// on the files do nothing, never the global Tag and Branch.
func (s *statusScreen) hunkActions() []terminu.Action {
	off := func() bool { return !s.g.idle() }
	noHunk := func() bool { _, _, ok := s.hunk(); return !ok }
	return []terminu.Action{
		{Key: "Enter", Label: "Read", Desc: "it whole in the panel beside", Scope: item, Hint: "read",
			Hidden: noHunk, ToDetails: func() bool { return true }, Do: func() tea.Cmd { return nil }},
		{Key: "m", Label: "Use mine", Desc: "for this conflict", Scope: item, Hint: "mine",
			Hidden: noHunk, Disabled: off, Do: func() tea.Cmd { return s.resolve(git.Mine) }},
		{Key: "t", Label: "Use theirs", Desc: "for this conflict", Scope: item, Hint: "theirs",
			Hidden: noHunk, Disabled: off, Do: func() tea.Cmd { return s.resolve(git.Theirs) }},
		{Key: "b", Label: "Use both", Desc: "mine first, then theirs", Scope: item, Hint: "both",
			Hidden: noHunk, Disabled: off, Do: func() tea.Cmd { return s.resolve(git.Both) }},
	}
}

func (s *statusScreen) fileActions() []terminu.Action {
	g := s.g
	off := func() bool { return !g.idle() }
	path := func() string { f, _, _ := s.file(); return f.Path }
	inFiles := func() bool { return s.conflict != "" }
	return []terminu.Action{
		// Enter shows the file's change in [3], as on a conflicted file (tdp
		// K3: the details); s stages or unstages it (the user, 2026-11-23:
		// Enter staged before).
		{Key: "Enter", Label: "Show", Desc: "its change in [3]", Scope: item, Hint: "show",
			Hidden:    func() bool { _, gr, ok := s.file(); return !ok || gr == inConflict },
			ToDetails: func() bool { return true }, Do: func() tea.Cmd { return nil }},
		{Key: "s", Label: "Stage", Desc: "this file's change", Scope: item, Hint: "stage",
			Hidden: s.in(inUnstaged), Disabled: off, Do: func() tea.Cmd {
				p, next := path(), s.nextInGroup()
				return g.run("Staging…", "Stage", func() error { return g.repo.Stage(p) },
					func() tea.Cmd { s.files.Select(next); g.a.Log(terminu.Info, "Staged "+p); return nil })
			}},
		{Key: "s", Label: "Unstage", Desc: "this file's change, keeping it", Scope: item, Hint: "unstage",
			Hidden: s.in(inStaged), Disabled: off, Do: func() tea.Cmd {
				f, _, _ := s.file()
				next := s.nextInGroup()
				unborn := g.snap.HeadSHA == ""
				return g.run("Unstaging…", "Unstage", func() error { return g.repo.Unstage(unborn, bothPaths([]git.File{f})...) },
					func() tea.Cmd { s.files.Select(next); g.a.Log(terminu.Info, "Unstaged "+f.Path); return nil })
			}},
		// A conflicted file: Enter goes to [2], Enter there edits it in the
		// user's editor, r marks it resolved (the user).
		{Key: "Enter", Label: "Show", Desc: "it in [3], to edit it there", Scope: item, Hint: "show",
			Hidden: s.in(inConflict), ToDetails: func() bool { return true }, Do: func() tea.Cmd { return nil }},
		s.resolvedAction(item),
		{Label: "Resolve one by one", Desc: "pick mine, theirs or both for each conflict", Scope: item,
			Hidden: s.in(inConflict), Do: func() tea.Cmd { return s.goIn(path()) }},
		{Label: "Use mine", Desc: "for the whole file, and stage it", Scope: item, // no key (the user)
			Hidden: s.in(inConflict), Disabled: off,
			AskFor: func() *terminu.Ask { f, _, _ := s.file(); return s.askUseSide([]git.File{f}, false) },
			Do:     func() tea.Cmd { return s.useSide(path(), false) }},
		{Label: "Use theirs", Desc: "for the whole file, and stage it", Scope: item, // no key (the user)
			Hidden: s.in(inConflict), Disabled: off,
			AskFor: func() *terminu.Ask { f, _, _ := s.file(); return s.askUseSide([]git.File{f}, true) },
			Do:     func() tea.Cmd { return s.useSide(path(), true) }},
		{Label: "Remove", Desc: "this file from the repository", Scope: item,
			Hidden: func() bool { f, gr, ok := s.file(); return !ok || gr == inConflict || !inIndex(f) }, Disabled: off,
			AskFor: func() *terminu.Ask { f, _, _ := s.file(); return s.askRemove([]git.File{f}) },
			Do:     func() tea.Cmd { f, _, _ := s.file(); return s.remove([]git.File{f}) }},
		{Label: "Ignore", Desc: "add this file to .gitignore", Scope: item,
			Hidden: func() bool { _, gr, ok := s.file(); return !ok || gr == inConflict }, Disabled: off,
			AskFor: func() *terminu.Ask { f, _, _ := s.file(); return s.askIgnore([]git.File{f}) },
			Do:     func() tea.Cmd { f, _, _ := s.file(); return s.ignore([]git.File{f}) }},
		{Key: "m", Label: "Mark", Desc: "this file, to work on the marked together", Scope: item, Hint: "mark",
			Hidden: func() bool { _, _, ok := s.file(); return !ok || s.isMarked() }, Do: s.toggleMark},
		{Key: "m", Label: "Unmark", Desc: "this file", Scope: item, Hint: "unmark",
			Hidden: func() bool { _, _, ok := s.file(); return !ok || !s.isMarked() }, Do: s.toggleMark},
		{Label: "Discard", Desc: "this file's change", Scope: item, // no key (the user)
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
					func() tea.Cmd {
						if f.Untracked() {
							return g.a.Toast(terminu.Success, "Deleted "+f.Path)
						}
						return g.a.Toast(terminu.Success, "Discarded the change to "+f.Path)
					})
			}},
		// The panel: the marked files first (tdp P3).
		s.batch("Stage the marked", "Staging…", "Stage", []string{inUnstaged}, nil, func(fs []git.File) error {
			return g.repo.Stage(paths(fs)...)
		}, func(fs []git.File) tea.Cmd {
			for _, f := range fs {
				g.a.Log(terminu.Info, "Staged "+f.Path)
			}
			return nil
		}),
		s.batch("Unstage the marked", "Unstaging…", "Unstage", []string{inStaged}, nil, func(fs []git.File) error {
			return g.repo.Unstage(g.snap.HeadSHA == "", bothPaths(fs)...)
		}, func(fs []git.File) tea.Cmd {
			for _, f := range fs {
				g.a.Log(terminu.Info, "Unstaged "+f.Path)
			}
			return nil
		}),
		s.batch("Use mine for the marked", "Resolving…", "Resolve", []string{inConflict}, func(fs []git.File) *terminu.Ask {
			return s.askUseSide(fs, false)
		}, func(fs []git.File) error {
			return g.repo.UseSide(false, paths(fs)...)
		}, func(fs []git.File) tea.Cmd {
			return g.a.Toast(terminu.Success, plural(len(fs), "file", "files")+" resolved with mine and staged")
		}),
		s.batch("Use theirs for the marked", "Resolving…", "Resolve", []string{inConflict}, func(fs []git.File) *terminu.Ask {
			return s.askUseSide(fs, true)
		}, func(fs []git.File) error {
			return g.repo.UseSide(true, paths(fs)...)
		}, func(fs []git.File) tea.Cmd {
			return g.a.Toast(terminu.Success, plural(len(fs), "file", "files")+" resolved with theirs and staged")
		}),
		withKey("X", s.batch("Discard the marked", "Discarding…", "Discard", []string{inStaged, inUnstaged}, s.askDiscard, func(fs []git.File) error {
			for _, f := range fs {
				if err := g.repo.Discard(f, f.Staged()); err != nil {
					return err
				}
			}
			return nil
		}, func(fs []git.File) tea.Cmd {
			for _, f := range fs {
				if f.Untracked() {
					g.a.Log(terminu.Info, "Deleted "+f.Path)
				} else {
					g.a.Log(terminu.Info, "Discarded the change to "+f.Path)
				}
			}
			return g.a.Toast(terminu.Success, "Discarded the changes to "+plural(len(fs), "file", "files"))
		})), // X: the marked at once (tdp K13)
		s.batchWith("Mark the marked resolved", []string{inConflict}, nil, s.askResolved, s.markResolved),
		s.batchWith("Remove the marked", []string{inStaged, inUnstaged}, inIndex, s.askRemove, s.remove),
		s.batchWith("Ignore the marked", []string{inStaged, inUnstaged}, nil, s.askIgnore, s.ignore),
		// M (the user) is the panel's over the global Merge (tdp M4): Mark all,
		// and Unmark all once every file is marked, as m is on one (the user);
		// hidden when there is nothing to mark (tdp M6), M still the panel's
		// then, doing nothing (tdp M4-5, the lib).
		{Key: "M", Label: "Mark all", Desc: "every file listed", Scope: panel,
			Hidden: func() bool { return inFiles() || s.allMarked() || s.files.Empty() },
			Do: func() tea.Cmd {
				var ids []string
				for _, r := range s.files.Rows() {
					ids = append(ids, r.ID)
				}
				s.files.SetMarked(ids)
				g.a.Log(terminu.Info, "Marked every file")
				return nil
			}},
		{Key: "M", Label: "Unmark all", Desc: "every file listed", Scope: panel,
			Hidden: func() bool { return inFiles() || !s.allMarked() },
			Do:     func() tea.Cmd { s.files.ClearMarks(); g.a.Log(terminu.Info, "Unmarked every file"); return nil }},
		{Label: "Clear marks", Desc: "unmark every file", Scope: panel,
			Hidden: func() bool { return inFiles() || len(s.files.Marked()) == 0 || s.allMarked() },
			Do:     func() tea.Cmd { s.files.ClearMarks(); g.a.Log(terminu.Info, "Cleared the marks"); return nil }},
		// a toggles (the user): Stage [a]ll while some change is not staged,
		// then Unstage [a]ll; the key bracketed at "all", which the label
		// writes itself (tdp main M5-1: the app may pick the appearance; the
		// lib leaves a label showing its key as it is). With some staged and
		// some not, Unstage all is in the menu too, with no key.
		{Key: "a", Label: "Stage [a]ll", Desc: "every change but the conflicts, untracked files too", Scope: panel,
			Hidden:   func() bool { return inFiles() || !s.anyUnstaged() && s.anyStaged() },
			Disabled: func() bool { return off() || !s.anyUnstaged() }, Do: s.stageAll},
		{Key: "a", Label: "Unstage [a]ll", Desc: "every staged change, keeping them", Scope: panel,
			Hidden: func() bool { return inFiles() || s.anyUnstaged() || !s.anyStaged() }, Disabled: off, Do: s.unstageAll},
		{Label: "Unstage all", Desc: "every staged change, keeping them", Scope: panel,
			Hidden: func() bool { return inFiles() || !s.anyUnstaged() || !s.anyStaged() }, Disabled: off, Do: s.unstageAll},
	}
}

func (s *statusScreen) anyStaged() bool {
	return s.g.snap != nil && slices.ContainsFunc(s.g.snap.Files, git.File.Staged)
}

func (s *statusScreen) anyUnstaged() bool {
	return s.g.snap != nil && slices.ContainsFunc(s.g.snap.Files, git.File.Unstaged)
}

// stageAll stages every change, untracked files too, but the conflicts: a
// conflicted file is staged by Mark [r]esolved, which looks for markers left
// first, so that none is committed unseen.
func (s *statusScreen) stageAll() tea.Cmd {
	g := s.g
	var ps []string
	left := 0
	for _, f := range g.snap.Files {
		switch {
		case f.Conflicted():
			left++
		case f.Unstaged():
			ps = append(ps, f.Path)
		}
	}
	return g.run("Staging…", "Stage", func() error { return g.repo.Stage(ps...) }, func() tea.Cmd {
		if left == 0 {
			g.a.Log(terminu.Info, "Staged every change")
			return nil
		}
		return g.a.Toast(terminu.Info, "Staged every change but the "+plural(left, "conflict", "conflicts"),
			"Press [r] on each once it is resolved")
	})
}

// nextInGroup is the row after the one under [1]'s cursor in its group, or
// the one before it when that is the last: where the cursor goes once the
// file moves to another group, so that Enter walks down the list.
func (s *statusScreen) nextInGroup() string {
	cur, ok := s.files.Current()
	if !ok {
		return ""
	}
	rows := s.files.Rows()
	i := slices.IndexFunc(rows, func(r terminu.Row) bool { return r.ID == cur.ID })
	group := cur.ID[:2]
	if i+1 < len(rows) && rows[i+1].ID[:2] == group {
		return rows[i+1].ID
	}
	if i > 0 && rows[i-1].ID[:2] == group {
		return rows[i-1].ID
	}
	return cur.ID
}

// bothPaths are the files' paths, a rename's old one too: unstaging it
// takes both out of the index.
func bothPaths(fs []git.File) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Path)
		if f.Orig != "" {
			out = append(out, f.Orig)
		}
	}
	return out
}

// unstageAll unstages every staged change, keeping them in the working tree.
func (s *statusScreen) unstageAll() tea.Cmd {
	g := s.g
	unborn := g.snap.HeadSHA == ""
	return g.run("Unstaging…", "Unstage", func() error { return g.repo.UnstageAll(unborn) },
		func() tea.Cmd { g.a.Log(terminu.Info, "Unstaged every change"); return nil })
}

// showDiff shows in [2] the change of the row under [1]'s cursor, its two
// versions drawn as UNIFIED (tdp note.8 "In a panel"), the location row
// naming them; again, it is the same row read again (tdp note.9).
func (s *statusScreen) showDiff(again bool) {
	defer s.showLog() // [4] follows the same file
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
	if s.files.Empty() { // nothing changed: no file in [1] to go to either, so where [1] sends you (the user)
		s.diff.EmptyHint = "Press [2] to see the history"
	}
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
	lines := []string{terminu.Fg(teal).Render(terminu.Escape(s.mineName(h)))}
	lines = append(lines, h.Mine...)
	lines = append(lines, "", terminu.Fg(mauve).Render(terminu.Escape("THEIRS · "+h.TheirsLabel)))
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
		it := terminu.DiffItem{Mark: terminu.DiffAdded, Name: p, Anchor: p + " @ empty", Other: p}
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
			return terminu.DiffItem{Anchor: path + " @ empty", Other: path,
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
			from := path + " @ index"
			if f.Untracked() { // not in the index: nothing on the left, as a commit's new file (the user)
				from = path + " @ empty"
			}
			item = terminu.DiffItem{Anchor: from, Other: path, Text: func() (string, string, error) {
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
	s.files.ClearFilter() // going into a row (tdp panel.14, panel.10)
	s.conflict = path
	if err := s.readHunks(); err != nil {
		s.conflict = ""
		return s.g.a.ErrorAbout("Resolve failed", path, err.Error())
	}
	s.hunkList.Select("0")
	s.located()
	s.filesP.Body = s.hunkList
	s.filesP.OnEsc = func() tea.Cmd { s.goOut(); s.showDiff(false); return nil }
	s.showDiff(false)
	return nil
}

// located writes [1]'s location row: the state of the repository when there
// is one, else All changes (the user: it only holds the place), then the
// conflicted file gone into (tdp panel.10).
func (s *statusScreen) located() {
	glyph, where := s.g.state()
	work := s.g.busy // git running: Green, the loading icon turning (tdp panel.10, popup.4)
	if where == "" && work == "" {
		where = "All changes"
	}
	in := s.conflict
	if where != "" {
		in = where + " › " + s.conflict
	}
	s.files.LocationGlyph, s.files.Location, s.files.LocationWork = glyph, where, work
	s.hunkList.LocationGlyph, s.hunkList.Location, s.hunkList.LocationWork = glyph, in, work
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
			s.files.ClearFilter() // gittu puts the cursor on it (tdp panel.14)
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
	return g.run("Resolving…", "Resolve", func() error { return g.repo.UseSide(theirs, path) },
		func() tea.Cmd {
			s.files.Select(inStaged + path) // the filter kept (tdp panel-198)
			return g.a.Toast(terminu.Success, path+" resolved with "+word+" and staged")
		})
}

// askUseSide asks before conflicted files are made one side whole when one
// was changed after the conflict, or gittu can't tell (the user): the changes
// made to it are lost. Untouched since, it does not ask.
func (s *statusScreen) askUseSide(fs []git.File, theirs bool) *terminu.Ask {
	word := "mine"
	if theirs {
		word = "theirs"
	}
	var d []string
	for _, f := range fs {
		switch changed, known := s.g.repo.ChangedAfterConflict(f.Path); {
		case !known:
			d = append(d, f.Path+": gittu can't tell whether it was changed after the conflict")
		case changed:
			d = append(d, f.Path+": changed after the conflict")
		}
	}
	if len(d) == 0 {
		return nil
	}
	if len(fs) == 1 {
		d = append(d, "The whole file becomes "+word+": any change made to it after the conflict is lost.")
	} else {
		d = append(d, "Each file becomes "+word+" whole: any change made after the conflict is lost.")
	}
	warn := make([]int, len(d))
	for i := range warn {
		warn[i] = i
	}
	return &terminu.Ask{Title: "Use " + word, Subject: named(fs), Question: "Use " + word + " for " + named(fs) + "?",
		Verb: "use " + word, Details: d, Warn: warn}
}

// edit opens a conflicted file in the editor git would open, in a terminal
// popup EDIT: <path> (tdp terminal; textarea.1: a file of lines is the
// user's editor's). When it ends, the repository is read again and the file
// checked for conflict markers left (the user).
func (s *statusScreen) edit(path string) tea.Cmd {
	g := s.g
	ed, err := g.repo.Editor()
	if err != nil {
		return g.a.ErrorAbout("Edit failed", path, err.Error())
	}
	g.a.Log(terminu.Info, "Editing "+path)
	g.editing = true
	term := &terminu.Terminal{Title: "Edit", Subject: path, Dir: g.repo.Dir, Ends: true, Env: editorEnv(),
		Command: []string{"sh", "-c", ed + ` "$@"`, "sh", filepath.Join(g.repo.Dir, path)}}
	term.OnExit = func(err error) tea.Cmd { return s.edited(path, err) }
	return g.a.OpenTerminal(term)
}

// edited reads the repository again after the editor ended and says what is
// left of the file's conflicts: the lines of its markers, or none and how to
// mark it resolved.
func (s *statusScreen) edited(path string, err error) tea.Cmd {
	g := s.g
	g.editing = false
	read := g.readOwn()
	if err != nil { // the file is checked all the same
		g.a.Log(terminu.Warning, "The editor of "+path+" ended with "+err.Error())
	}
	at, rerr := g.repo.Markers(path)
	switch {
	case rerr != nil:
		return tea.Batch(read, g.a.Toast(terminu.Error, "Can't read "+path+": "+rerr.Error()))
	case len(at) > 0:
		return tea.Batch(read, g.a.Toast(terminu.Warning, path+" still has conflicts on "+lineList(at)))
	}
	return tea.Batch(read, g.a.Toast(terminu.Info, path+" has no conflicts left: press [r] to mark it resolved"))
}

// lineList names conflicts by their lines, a block as a range (the user):
// "lines 2–6, 14–18", "line 3"; eight at the most.
func lineList(at [][2]int) string {
	if len(at) == 1 && at[0][0] == at[0][1] {
		return "line " + strconv.Itoa(at[0][0])
	}
	var ns []string
	for i, r := range at {
		if i == 8 {
			ns = append(ns, "and "+strconv.Itoa(len(at)-8)+" more")
			break
		}
		n := strconv.Itoa(r[0])
		if r[1] != r[0] {
			n += "–" + strconv.Itoa(r[1])
		}
		ns = append(ns, n)
	}
	return "lines " + strings.Join(ns, ", ")
}

// editorEnv hides the outer terminal's name from the editor: it runs in the
// lib's emulator, not there (as sshu does).
func editorEnv() []string {
	var out []string
	for _, k := range []string{"TERM_PROGRAM", "TERM_PROGRAM_VERSION", "TERM_SESSION_ID", "KITTY_WINDOW_ID",
		"KITTY_PUBLIC_KEY", "ITERM_SESSION_ID", "ITERM_PROFILE", "LC_TERMINAL", "LC_TERMINAL_VERSION",
		"WEZTERM_EXECUTABLE", "WEZTERM_PANE", "GHOSTTY_RESOURCES_DIR"} {
		out = append(out, k+"=")
	}
	return out
}

// resolvedAction is Mark [r]esolved, the conflicted file under [1]'s
// cursor, on [1] and on [2] showing it (the user).
func (s *statusScreen) resolvedAction(scope terminu.Scope) terminu.Action {
	return terminu.Action{Key: "r", Label: "Mark [r]esolved", Desc: "stage it: its conflicts are done", Scope: scope, Hint: "resolved", // the key at "resolved" (the user; tdp main M5-1)
		Hidden: s.in(inConflict), Disabled: func() bool { return !s.g.idle() },
		AskFor: func() *terminu.Ask { f, _, _ := s.file(); return s.askResolved([]git.File{f}) },
		Do:     func() tea.Cmd { f, _, _ := s.file(); return s.markResolved([]git.File{f}) }}
}

// askResolved asks only when a file still has conflict markers: they are
// staged as they are.
func (s *statusScreen) askResolved(fs []git.File) *terminu.Ask {
	var d []string
	for _, f := range fs {
		if at, err := s.g.repo.Markers(f.Path); err == nil && len(at) > 0 {
			d = append(d, f.Path+": conflicts on "+lineList(at))
		}
	}
	if len(d) == 0 {
		return nil
	}
	d = append(d, "They are staged as they are, and the commit takes them.")
	warn := make([]int, len(d))
	for i := range warn {
		warn[i] = i
	}
	return &terminu.Ask{Title: "Mark resolved", Subject: named(fs), Question: "Mark " + named(fs) + " resolved?", Verb: "mark resolved",
		Details: d, Warn: warn}
}

// markResolved stages conflicted files: git takes them as resolved.
func (s *statusScreen) markResolved(fs []git.File) tea.Cmd {
	g := s.g
	return g.run("Staging…", "Mark resolved", func() error { return g.repo.Stage(paths(fs)...) }, func() tea.Cmd {
		if len(fs) > 1 {
			for _, f := range fs {
				g.a.Log(terminu.Info, "Marked "+f.Path+" resolved")
			}
		} else {
			s.files.Select(inStaged + fs[0].Path) // the filter kept (tdp panel-198)
		}
		return g.a.Toast(terminu.Success, "Marked "+named(fs)+" resolved")
	})
}

// commit commits the staged changes with a message the user writes in the
// commit form: during a merge it opens with the message git prepared.
func (g *gittu) commit() tea.Cmd {
	ready := ""
	if g.snap.State.Op == "merge" {
		ready = g.repo.MergeMessage()
	}
	return g.messageFor("Commit", ready, false)
}

// amend opens the commit form on the last commit's message, to amend it
// with the staged changes.
func (g *gittu) amend() tea.Cmd { return g.messageFor("Amend", g.repo.LastMessage(), true) }

// messageFor opens the commit form with ready in it (the user; tdp form):
// Title, the message's first line, a one-line input; Description, the rest,
// a textarea; the button commits (or amends). A title is required.
func (g *gittu) messageFor(title, ready string, amend bool) tea.Cmd {
	subject, body := splitMessage(ready)
	titleF := &terminu.FormField{Label: "Title", Stacked: true, Empty: "not set",
		Value: func() string { return subject },
		Edit: func() terminu.Input {
			return terminu.TextInput{Title: "Title", Glyph: glyphCommit, Old: subject,
				Done: func(v string) tea.Cmd { subject = v; return nil }}
		},
		Check: func() string {
			if strings.TrimSpace(subject) == "" {
				return "Title is required"
			}
			return ""
		}}
	descF := &terminu.FormField{Label: "Description", Stacked: true, Empty: "none",
		Value: func() string { return firstLineMore(body) },
		Edit: func() terminu.Input {
			return terminu.Textarea{Title: "Description", Glyph: glyphCommit, Old: body,
				Done: func(v string) tea.Cmd { body = v; return nil }}
		}}
	repo, remote := g.repo, g.remote
	return g.a.Form(&terminu.Form{Title: title, Glyph: glyphCommit, Name: "the commit message",
		Fields: []*terminu.FormField{titleF, descF}, Button: title,
		// The form stays while git commits, hooks and all: the loading icon
		// after its title, Enter on the button doing nothing (tdp form.7).
		// A commit git stops opens its error over the form, the message
		// still in it to commit again (tdp form.16); git's whole text goes
		// in the App Log (tdp F5). It runs off the update loop: the App Log
		// takes entries from any goroutine.
		SubmitLong: func(context.Context) (tea.Cmd, error) {
			g.committing.Store(true) // no reading again meanwhile
			defer g.committing.Store(false)
			msg := joinMessage(subject, body)
			if err := repo.Commit(msg, amend); err != nil {
				lines := gitLines(err)
				g.logAll(lines)
				return nil, errors.New(reason(lines))
			}
			m := doneMsg{title: title, ok: g.committed(msg, amend)}
			m.snap, m.rerr = repo.Read(remote)
			said := didCommit(amend)
			if m.snap != nil {
				said += git.Short(m.snap.HeadSHA) + " "
			}
			said += firstLine(msg)
			g.unsaid.Store(&said) // said by the doneMsg, or by the next read if the form closed first
			return func() tea.Msg { return m }, nil
		}})
}

// splitMessage cuts a commit message into its first line and the rest, the
// blank lines between them left out.
func splitMessage(msg string) (string, string) {
	subject, body, _ := strings.Cut(strings.TrimSpace(msg), "\n")
	return strings.TrimSpace(subject), strings.Trim(body, "\n")
}

// joinMessage is a commit message of a title and a description, a blank
// line between them as git writes it.
func joinMessage(subject, body string) string {
	msg := strings.TrimSpace(subject)
	if body = strings.TrimRight(body, "\n"); strings.TrimSpace(body) != "" {
		msg += "\n\n" + body
	}
	return msg
}

// firstLineMore is a text as a form shows a textarea's value: its first
// line, … when more follow (tdp input.10).
func firstLineMore(text string) string {
	first, rest, more := strings.Cut(text, "\n")
	if more && strings.TrimSpace(rest) != "" {
		return first + "…"
	}
	return first
}

// committed is what a commit gone through says, the popups the form came
// from closed too (tdp T1). A commit is what Commit is for: [3]'s cursor goes
// on it, its filter taken away first (tdp K3-15, panel.14); an amend changes
// the last one, the cursor staying.
func (g *gittu) committed(msg string, amend bool) func() tea.Cmd {
	return func() tea.Cmd {
		g.unsaid.Store(nil) // the toast says it
		sha := ""
		if g.snap != nil {
			sha = git.Short(g.snap.HeadSHA)
			if h := g.history; !amend && h.inGraph(g.snap.HeadSHA) {
				h.commits.ClearFilter()
				h.commits.Select(g.snap.HeadSHA)
			}
		}
		return tea.Batch(g.a.CloseAll(), g.a.Toast(terminu.Success, didCommit(amend)+sha+" "+firstLine(msg)))
	}
}

// didCommit is how a commit's Success starts.
func didCommit(amend bool) string {
	if amend {
		return "Amended "
	}
	return "Committed "
}

// changeFlag is a working tree change's letter in its colour, in [1]'s flag
// column (tdp panel.7; tdp P5: the letters and colours are the app's).
// fileRow is a file's row in a group: its state's letter in its colour, then
// the path as shown.
func fileRow(in, group, path, shown string, status byte, w int) terminu.Row {
	// The letter always shows: the path is cut from its front here, before the
	// lib would cut the whole row, the letter first; the filter matches the
	// whole path (Cells).
	return terminu.Row{ID: in + path, Group: group, Cells: []string{string(status) + " " + shown},
		Styled: [][]terminu.Span{{changeFlag(status), {Text: " " + cutFront(shown, w)}}}}
}

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
