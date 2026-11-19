package ui

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/gitreu/internal/git"
)

const (
	item  = terminu.ItemScope
	panel = terminu.PanelScope
)

// rowKind is the kind and the name of the row under [1]'s cursor; "" when
// there is none.
func (h *historyScreen) rowKind() (kind, name string) {
	r, ok := h.cur().Current()
	if !ok || len(r.ID) < 2 {
		return "", ""
	}
	return r.ID[:2], r.ID[2:]
}

func (h *historyScreen) is(kind string) func() bool {
	return func() bool { k, _ := h.rowKind(); return k != kind }
}

func (h *historyScreen) branch() (git.Branch, bool) {
	k, name := h.rowKind()
	if k != kindBranch || h.g.snap == nil {
		return git.Branch{}, false
	}
	i := slices.IndexFunc(h.g.snap.Branches, func(b git.Branch) bool { return b.Name == name })
	if i < 0 {
		return git.Branch{}, false
	}
	return h.g.snap.Branches[i], true
}

func (h *historyScreen) remote() (git.RemoteBranch, bool) {
	k, name := h.rowKind()
	if k != kindRemote || h.g.snap == nil {
		return git.RemoteBranch{}, false
	}
	i := slices.IndexFunc(h.g.snap.Remotes, func(b git.RemoteBranch) bool { return b.Name == name })
	if i < 0 {
		return git.RemoteBranch{}, false
	}
	return h.g.snap.Remotes[i], true
}

func (h *historyScreen) stash() (git.Stash, bool) {
	k, name := h.rowKind()
	if k != kindStash || h.g.snap == nil {
		return git.Stash{}, false
	}
	i := slices.IndexFunc(h.g.snap.Stashes, func(s git.Stash) bool { return s.Ref == name })
	if i < 0 {
		return git.Stash{}, false
	}
	return h.g.snap.Stashes[i], true
}

// onCurrent reports the branch under the cursor is the one checked out: what
// works on another branch is off on it (tdp M6).
func (h *historyScreen) onCurrent() bool {
	b, ok := h.branch()
	return !ok || b.Current || !h.g.idle()
}

func (h *historyScreen) busyOr(f func() bool) func() bool {
	return func() bool { return !h.g.idle() || (f != nil && f()) }
}

// branchActions are [1]'s operations, by the kind of row (tdp P3, M2): a
// branch, a remote branch, a tag, a stash; and the panel's.
func (h *historyScreen) branchActions() []terminu.Action {
	g := h.g
	name := func() string { _, n := h.rowKind(); return n }
	off := h.busyOr(nil)
	return []terminu.Action{
		// A branch, a remote branch, a tag: Enter shows its commit in [2].
		{Key: "Enter", Label: "Show", Desc: "its commit in [2] COMMITS", Scope: item, Hint: "show",
			Hidden: func() bool { k, _ := h.rowKind(); return k == "" || k == kindStash }, Do: h.showRef},
		// A branch.
		{Key: "c", Label: "Check out", Desc: "this branch", Scope: item,
			Hidden: h.is(kindBranch), Disabled: h.onCurrent, Do: func() tea.Cmd {
				n := name()
				return g.run("Checking out…", "Checkout", func() error { return g.repo.Checkout(n, false) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Checked out "+n) })
			}},
		{Key: "M", Label: "Merge", Desc: "it into the current branch", Scope: item,
			Hidden: h.is(kindBranch), Disabled: h.onCurrent, AskFor: func() *terminu.Ask { return h.g.askMerge(name()) },
			Do: func() tea.Cmd { return g.merge(name()) }},
		{Key: "b", Label: "Rebase", Desc: "the current branch onto it", Scope: item,
			Hidden: h.is(kindBranch), Disabled: h.onCurrent, AskFor: func() *terminu.Ask { return g.askRebase(name()) },
			Do: func() tea.Cmd { return g.rebase(name()) }},
		{Key: "r", Label: "Rename", Desc: "this branch", Scope: item,
			Hidden: h.is(kindBranch), Disabled: off, Do: func() tea.Cmd { return h.rename(name()) }},
		{Key: "D", Label: "Delete", Desc: "this branch", Scope: item,
			Hidden: h.is(kindBranch), Disabled: h.onCurrent,
			AskFor: func() *terminu.Ask {
				return &terminu.Ask{Title: "Delete", Subject: name(), Question: "Delete " + name() + "?", Verb: "delete"}
			},
			Do: func() tea.Cmd { return h.deleteBranch(name()) }},
		{Label: "Push", Desc: "this branch to its remote", Scope: item,
			Hidden: h.is(kindBranch), Disabled: off, Do: func() tea.Cmd {
				b, _ := h.branch()
				remote := g.repo.Remote(b.Upstream)
				if remote == "" {
					return g.a.ErrorAbout("Push failed", b.Name, "This repository has no remote to push to")
				}
				return g.run("Pushing…", "Push", func() error { return g.repo.PushBranch(remote, b.Name) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Pushed "+b.Name+" to "+remote) })
			}},
		// A remote branch.
		{Key: "c", Label: "Check out", Desc: "as a local branch tracking it", Scope: item,
			Hidden: h.is(kindRemote), Disabled: off,
			AskFor: func() *terminu.Ask {
				rb, _ := h.remote()
				if h.hasLocal(rb.Branch) {
					return &terminu.Ask{Title: "Check out", Subject: rb.Name, Question: "Check out " + rb.Branch + "?", Verb: "check out",
						Details: []string{"There is a local branch " + rb.Branch + " already: that one is checked out."}}
				}
				return &terminu.Ask{Title: "Check out", Subject: rb.Name, Question: "Check out " + rb.Name + " as " + rb.Branch + "?", Verb: "check out",
					Details: []string{"A local branch " + rb.Branch + " is made, tracking " + rb.Name + "."}}
			},
			Do: func() tea.Cmd {
				rb, _ := h.remote()
				local := h.hasLocal(rb.Branch)
				return g.run("Checking out…", "Checkout", func() error { return g.repo.CheckoutRemote(rb, local) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Checked out "+rb.Branch) })
			}},
		{Key: "M", Label: "Merge", Desc: "it into the current branch", Scope: item,
			Hidden: h.is(kindRemote), Disabled: off, AskFor: func() *terminu.Ask { return g.askMerge(name()) },
			Do: func() tea.Cmd { return g.merge(name()) }},
		{Key: "b", Label: "Rebase", Desc: "the current branch onto it", Scope: item,
			Hidden: h.is(kindRemote), Disabled: off, AskFor: func() *terminu.Ask { return g.askRebase(name()) },
			Do: func() tea.Cmd { return g.rebase(name()) }},
		{Key: "D", Label: "Delete", Desc: "this branch on its remote", Scope: item,
			Hidden: h.is(kindRemote), Disabled: off,
			AskFor: func() *terminu.Ask {
				rb, _ := h.remote()
				return &terminu.Ask{Title: "Delete", Subject: rb.Name, Question: "Delete " + rb.Branch + " on " + rb.Remote + "?", Verb: "delete",
					Details: []string{"It is deleted for everyone using " + rb.Remote + ": it can't be undone here."}, Warn: []int{0}}
			},
			Do: func() tea.Cmd {
				rb, _ := h.remote()
				return g.run("Deleting…", "Delete", func() error { return g.repo.DeleteRemoteBranch(rb) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Deleted "+rb.Branch+" on "+rb.Remote) })
			}},
		// A tag.
		{Key: "c", Label: "Check out", Desc: "this tag, HEAD detached", Scope: item,
			Hidden: h.is(kindTag), Disabled: off,
			AskFor: func() *terminu.Ask {
				return &terminu.Ask{Title: "Check out", Subject: name(), Question: "Check out " + name() + "?", Verb: "check out",
					Details: []string{"HEAD is left detached: commits made there are on no branch."}, Warn: []int{0}}
			},
			Do: func() tea.Cmd {
				n := name()
				return g.run("Checking out…", "Checkout", func() error { return g.repo.Checkout("refs/tags/"+n, true) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Checked out "+n) })
			}},
		{Key: "D", Label: "Delete", Desc: "this tag here, not on a remote", Scope: item,
			Hidden: h.is(kindTag), Disabled: off,
			AskFor: func() *terminu.Ask {
				return &terminu.Ask{Title: "Delete", Subject: name(), Question: "Delete the tag " + name() + "?", Verb: "delete"}
			},
			Do: func() tea.Cmd {
				n := name()
				return g.run("Deleting…", "Delete", func() error { return g.repo.DeleteTag(n) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Deleted the tag "+n) })
			}},
		{Label: "Push", Desc: "this tag to its remote", Scope: item,
			Hidden: h.is(kindTag), Disabled: off, Do: func() tea.Cmd {
				n := name()
				remote := g.repo.Remote(g.snap.Upstream)
				if remote == "" {
					return g.a.ErrorAbout("Push failed", n, "This repository has no remote to push to")
				}
				return g.run("Pushing…", "Push", func() error { return g.repo.PushTag(remote, n) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Pushed the tag "+n+" to "+remote) })
			}},
		// A stash.
		{Key: "Enter", Label: "Show", Desc: "what it keeps", Scope: item, Hint: "show",
			Hidden: h.is(kindStash), Do: func() tea.Cmd {
				st, _ := h.stash()
				lines, err := g.repo.StashDiff(st.Ref)
				if err != nil {
					return g.a.ErrorAbout("Show failed", st.Ref, errLines(err)...)
				}
				g.a.Log(terminu.Info, "Opened "+st.Ref)
				return g.a.Viewer(&terminu.Viewer{Title: "Stash", Subject: st.Ref, Glyph: glyphStash, Lines: lines, Lang: "diff", NoNumbers: true})
			}},
		{Key: "a", Label: "Apply", Desc: "it to the working tree, keeping it", Scope: item,
			Hidden: h.is(kindStash), Disabled: off, Do: func() tea.Cmd { return h.applyStash(false) }},
		{Key: "o", Label: "Pop", Desc: "apply it, then drop it", Scope: item,
			Hidden: h.is(kindStash), Disabled: off, Do: func() tea.Cmd { return h.applyStash(true) }},
		{Key: "D", Label: "Drop", Desc: "delete it", Scope: item,
			Hidden: h.is(kindStash), Disabled: off,
			AskFor: func() *terminu.Ask {
				st, _ := h.stash()
				return &terminu.Ask{Title: "Drop", Subject: st.Ref, Question: "Drop " + st.Ref + "?", Verb: "drop",
					Details: []string{st.Message, "What it keeps is lost: it can't be undone here."}, Warn: []int{1}}
			},
			Do: func() tea.Cmd {
				st, _ := h.stash()
				return g.run("Dropping…", "Drop", func() error { return g.repo.StashDrop(st.Ref) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Dropped "+st.Ref) })
			}},
		// The panel: [2] zoomed can be put back from here too (tdp screen.6).
		{Key: "z", Label: "Unzoom", Desc: "back to both panels", Scope: panel,
			Hidden: func() bool { return h.zoom == 0 }, Do: func() tea.Cmd { return h.zoomTo(0) }},
	}
}

// showRef puts the focus on [2] with its cursor on the commit the row under
// [1]'s cursor points at, scrolled into sight (tdp K3: the user pressing
// Enter on a branch expects the graph). [2] gone into a commit comes back to
// the graph first.
func (h *historyScreen) showRef() tea.Cmd {
	r, ok := h.cur().Current()
	if !ok {
		return nil
	}
	name := r.ID[2:]
	h.goOut()
	sha := h.shaOf(r.ID)
	if sha != "" && !h.commits.Select(sha) && h.commits.Filtering() {
		h.commits.ClearFilter() // the filter hides it: show it all the same
		h.histP.Body = h.commits
	}
	if sha == "" || !h.commits.Select(sha) {
		if !h.g.all {
			return h.g.a.Toast(terminu.Info, name+" is not in the current branch's history", "Press [a] on [2] for all branches")
		}
		return h.g.a.Toast(terminu.Info, name+" is not in the graph's "+strconv.Itoa(git.Limit)+" newest commits")
	}
	return h.g.a.Focus(h.histP)
}

func (h *historyScreen) hasLocal(name string) bool {
	return h.g.snap != nil && slices.ContainsFunc(h.g.snap.Branches, func(b git.Branch) bool { return b.Name == name })
}

func (h *historyScreen) applyStash(pop bool) tea.Cmd {
	g := h.g
	st, _ := h.stash()
	verb, done := "Apply", "Applied "
	if pop {
		verb, done = "Pop", "Popped "
	}
	return g.runWith(doneMsg{title: verb, ok: func() tea.Cmd { return g.a.Toast(terminu.Success, done+st.Ref) },
		conflicts: g.conflictsSaid(verb)}, verb+"ing…", func() error { return g.repo.StashApply(st.Ref, pop) })
}

// rename asks for a branch's new name (tdp text).
func (h *historyScreen) rename(old string) tea.Cmd {
	g := h.g
	return g.a.Text(terminu.TextInput{Title: "Rename", Subject: old, Glyph: glyphBranch, Prompt: "New name for " + old,
		Verb: "rename", Name: "Name", Old: old, Check: g.checkBranch,
		Done: func(n string) tea.Cmd {
			return g.run("Renaming…", "Rename", func() error { return g.repo.RenameBranch(old, n) },
				func() tea.Cmd {
					h.tabs[0].Select(kindBranch + n)
					return g.a.Toast(terminu.Success, "Renamed "+old+" to "+n)
				})
		}})
}

// deleteBranch deletes a branch; one not merged asks again, as deleting it
// loses its commits.
func (h *historyScreen) deleteBranch(name string) tea.Cmd {
	g := h.g
	return g.runWith(doneMsg{title: "Delete",
		ok: func() tea.Cmd { return g.a.Toast(terminu.Success, "Deleted "+name) },
		failed: func(err error) tea.Cmd {
			if !errors.Is(err, git.ErrNotMerged) {
				return g.a.ErrorAbout("Delete failed", name, errLines(err)...)
			}
			return g.a.Confirm(terminu.Ask{Title: "Delete", Subject: name, Question: "Delete " + name + " anyway?", Verb: "delete",
				Details: []string{name + " is not merged: its commits that are on no other branch are lost."}, Warn: []int{0}},
				func() tea.Cmd {
					return g.run("Deleting…", "Delete", func() error { return g.repo.DeleteBranch(name, true) },
						func() tea.Cmd { return g.a.Toast(terminu.Success, "Deleted "+name) })
				})
		}}, "Deleting…", func() error { return g.repo.DeleteBranch(name, false) })
}

// checkBranch is a new branch name's rule: git check-ref-format's.
func (g *gitreu) checkBranch(name string) error {
	if !g.repo.ValidBranch(name) {
		return errors.New("Not a valid branch name")
	}
	if g.snap != nil && slices.ContainsFunc(g.snap.Branches, func(b git.Branch) bool { return b.Name == name }) {
		return errors.New("There is a branch " + name + " already")
	}
	return nil
}

func (g *gitreu) checkTag(name string) error {
	if !g.repo.ValidTag(name) {
		return errors.New("Not a valid tag name")
	}
	if g.snap != nil && slices.ContainsFunc(g.snap.Tags, func(t git.Tag) bool { return t.Name == name }) {
		return errors.New("There is a tag " + name + " already")
	}
	return nil
}

func (g *gitreu) currentName() string {
	if g.snap != nil && g.snap.Head != "" {
		return g.snap.Head
	}
	return "HEAD"
}

func (g *gitreu) askMerge(what string) *terminu.Ask {
	return &terminu.Ask{Title: "Merge", Subject: what, Question: "Merge " + what + " into " + g.currentName() + "?", Verb: "merge"}
}

func (g *gitreu) askRebase(onto string) *terminu.Ask {
	return &terminu.Ask{Title: "Rebase", Subject: onto, Question: "Rebase " + g.currentName() + " onto " + onto + "?", Verb: "rebase",
		Details: []string{"Its commits are made again on top of " + onto + "."}}
}

func (g *gitreu) merge(what string) tea.Cmd {
	into := g.currentName()
	return g.runWith(doneMsg{title: "Merge",
		ok:        func() tea.Cmd { return g.a.Toast(terminu.Success, "Merged "+what+" into "+into) },
		conflicts: g.conflictsSaid("Merge")}, "Merging…", func() error { return g.repo.Merge(what) })
}

func (g *gitreu) rebase(onto string) tea.Cmd {
	cur := g.currentName()
	return g.runWith(doneMsg{title: "Rebase",
		ok:        func() tea.Cmd { return g.a.Toast(terminu.Success, "Rebased "+cur+" onto "+onto) },
		conflicts: g.conflictsSaid("Rebase")}, "Rebasing…", func() error { return g.repo.Rebase(onto) })
}

// commitActions are [2]'s operations: on a commit of the graph, or on a file
// of the level gone into; and the panel's (tdp P3, M2).
func (h *historyScreen) commitActions() []terminu.Action {
	g := h.g
	onGraph := func() bool { _, ok := h.current(); return !ok }
	inLevel := func() bool { _, ok := h.change(); return !ok }
	off := h.busyOr(nil)
	short := func() string { c, _ := h.current(); return git.Short(c.SHA) }
	return []terminu.Action{
		{Key: "Enter", Label: "Files", Desc: "this commit changed", Scope: item, Hint: "files",
			Hidden: onGraph, Do: func() tea.Cmd {
				c, _ := h.current()
				from := ""
				if len(c.Parents) > 0 {
					from = c.Parents[0]
				}
				return h.goIn(from, c.SHA, git.Short(c.SHA)+" "+c.Subject)
			}},
		{Key: "Enter", Label: "Diff", Desc: "of this file, before and after", Scope: item, Hint: "diff",
			Hidden: inLevel, Do: h.openDiff},
		{Key: "m", Label: "Mark", Desc: "it to compare, or take the mark off", Scope: item, Hint: "mark",
			Hidden: onGraph, Do: func() tea.Cmd { h.commits.ToggleMark(); return nil }},
		{Key: "c", Label: "Check out", Desc: "this commit, HEAD detached", Scope: item,
			Hidden: onGraph, Disabled: off,
			AskFor: func() *terminu.Ask {
				return &terminu.Ask{Title: "Check out", Subject: short(), Question: "Check out " + short() + "?", Verb: "check out",
					Details: []string{"HEAD is left detached: commits made there are on no branch."}, Warn: []int{0}}
			},
			Do: func() tea.Cmd {
				c, _ := h.current()
				return g.run("Checking out…", "Checkout", func() error { return g.repo.Checkout(c.SHA, true) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Checked out "+git.Short(c.SHA)) })
			}},
		{Key: "b", Label: "Branch", Desc: "a new one here, and check it out", Scope: item,
			Hidden: onGraph, Disabled: off, Do: func() tea.Cmd { c, _ := h.current(); return g.newBranch(c.SHA) }},
		{Key: "t", Label: "Tag", Desc: "this commit", Scope: item,
			Hidden: onGraph, Disabled: off, Do: func() tea.Cmd { c, _ := h.current(); return g.newTag(c.SHA) }},
		{Key: "M", Label: "Merge", Desc: "it into the current branch", Scope: item,
			Hidden: onGraph, Disabled: off, AskFor: func() *terminu.Ask { return g.askMerge(short()) },
			Do: func() tea.Cmd { c, _ := h.current(); return g.merge(c.SHA) }},
		{Key: "e", Label: "Rebase", Desc: "the current branch onto it", Scope: item,
			Hidden: onGraph, Disabled: off, AskFor: func() *terminu.Ask { return g.askRebase(short()) },
			Do: func() tea.Cmd { c, _ := h.current(); return g.rebase(c.SHA) }},
		{Label: "Cherry-pick", Desc: "it onto the current branch", Scope: item,
			Hidden: onGraph, Disabled: off,
			AskFor: func() *terminu.Ask {
				return &terminu.Ask{Title: "Cherry-pick", Subject: short(), Question: "Cherry-pick " + short() + " onto " + g.currentName() + "?", Verb: "cherry-pick"}
			},
			Do: func() tea.Cmd {
				c, _ := h.current()
				return g.runWith(doneMsg{title: "Cherry-pick",
					ok:        func() tea.Cmd { return g.a.Toast(terminu.Success, "Cherry-picked "+git.Short(c.SHA)) },
					conflicts: g.conflictsSaid("Cherry-pick")}, "Cherry-picking…", func() error { return g.repo.CherryPick(c) })
			}},
		{Label: "Revert", Desc: "it with a new commit", Scope: item,
			Hidden: onGraph, Disabled: off,
			AskFor: func() *terminu.Ask {
				return &terminu.Ask{Title: "Revert", Subject: short(), Question: "Revert " + short() + "?", Verb: "revert",
					Details: []string{"A new commit on " + g.currentName() + " undoes what it did."}}
			},
			Do: func() tea.Cmd {
				c, _ := h.current()
				return g.runWith(doneMsg{title: "Revert",
					ok:        func() tea.Cmd { return g.a.Toast(terminu.Success, "Reverted "+git.Short(c.SHA)) },
					conflicts: g.conflictsSaid("Revert")}, "Reverting…", func() error { return g.repo.Revert(c) })
			}},
		{Key: "s", Label: "Reset", Desc: "the current branch to here", Scope: item,
			Hidden: onGraph, Disabled: h.busyOr(func() bool { return g.snap == nil || g.snap.HeadSHA == "" }), Do: h.reset},
		{Key: "y", Label: "Copy hash", Desc: "the whole hash, to the clipboard", Scope: item,
			Hidden: onGraph, Do: func() tea.Cmd {
				c, _ := h.current()
				terminu.Clipboard(c.SHA)
				return g.a.Toast(terminu.Success, "Copied "+git.Short(c.SHA))
			}},
		{Key: "a", Label: "Current branch", Desc: "only what HEAD reaches", Scope: panel,
			Hidden: func() bool { return h.in != nil || !g.all }, Do: func() tea.Cmd { return h.setAll(false) }},
		{Key: "a", Label: "All branches", Desc: "every branch, remote branch and tag", Scope: panel,
			Hidden: func() bool { return h.in != nil || g.all }, Do: func() tea.Cmd { return h.setAll(true) }},
		{Key: "z", Label: "Zoom", Desc: "wider, [1] at its least", Scope: panel, Hint: "zoom",
			Hidden: func() bool { return h.zoom != 0 || h.narrowZoom() }, Do: func() tea.Cmd { return h.zoomTo(1) }},
		{Key: "z", Label: "Zoom", Desc: "to the whole screen", Scope: panel, Hint: "zoom",
			Hidden: func() bool { return !(h.zoom == 1 || h.zoom == 0 && h.narrowZoom()) }, Do: func() tea.Cmd { return h.zoomTo(2) }},
		{Key: "z", Label: "Unzoom", Desc: "back to both panels", Scope: panel, Hint: "unzoom",
			Hidden: func() bool { return h.zoom != 2 }, Do: func() tea.Cmd { return h.zoomTo(0) }},
		{Key: "C", Label: "Compare", Desc: "the two marked commits", Scope: panel,
			Hidden: func() bool { return h.in != nil }, Disabled: func() bool { return len(h.commits.Marked()) != 2 },
			Do: h.compare},
	}
}

// setAll switches the graph between every branch and the current one (tdp
// P3: the panel's operation).
func (h *historyScreen) setAll(all bool) tea.Cmd {
	g := h.g
	g.all = all
	snap, err := g.repo.Read(all)
	g.show(snap, err)
	if all {
		g.a.Log(terminu.Info, "Showing all branches")
	} else {
		g.a.Log(terminu.Info, "Showing the current branch")
	}
	return nil
}

// compare goes into the files that differ between the two marked commits,
// the older one first (tdp panel.7, panel.10).
//
// TODO(tdp #20): tdp compares two texts only (panel.9, note.8); comparing two
// commits, a list of files and then each file, is not in it yet. Meanwhile the
// two commits are marked and compared as a batch (tdp P3).
func (h *historyScreen) compare() tea.Cmd {
	marked := h.commits.Marked()
	if len(marked) != 2 {
		return nil
	}
	// The graph lists newer commits first: the later row is the older one.
	ia := slices.IndexFunc(h.g.snap.Commits, func(c git.Commit) bool { return c.SHA == marked[0] })
	ib := slices.IndexFunc(h.g.snap.Commits, func(c git.Commit) bool { return c.SHA == marked[1] })
	older, newer := marked[0], marked[1]
	if ia < ib {
		older, newer = newer, older
	}
	return h.goIn(older, newer, git.Short(older)+" ↔ "+git.Short(newer))
}

// reset asks how to reset the current branch to the commit under the cursor:
// soft, mixed, or hard after a confirm (tdp select, F6).
func (h *historyScreen) reset() tea.Cmd {
	g := h.g
	c, _ := h.current()
	modes := []string{"soft", "mixed", "hard"}
	options := []string{
		"Soft: keep the changes, staged",
		"Mixed: keep the changes, unstaged",
		"Hard: throw the changes away",
	}
	do := func(mode string) tea.Cmd {
		return g.run("Resetting…", "Reset", func() error { return g.repo.Reset(mode, c.SHA) },
			func() tea.Cmd {
				return g.a.Toast(terminu.Success, "Reset "+g.currentName()+" to "+git.Short(c.SHA)+" ("+mode+")")
			})
	}
	return g.a.Select(terminu.Select{Title: "Reset", Subject: git.Short(c.SHA), Glyph: glyphCommit, Options: options,
		// h is a movement key (tdp K12): Hard takes H, which is also harder to
		// press by mistake.
		Keys: []string{"s", "m", "H"}, Selected: -1,
		Done: func(i int) tea.Cmd {
			if modes[i] != "hard" {
				return do(modes[i])
			}
			return g.a.Confirm(terminu.Ask{Title: "Reset", Subject: git.Short(c.SHA), Verb: "reset",
				Question: "Reset " + g.currentName() + " to " + git.Short(c.SHA) + " and throw the changes away?",
				Details:  []string{"Every change not committed is lost: it can't be undone."}, Warn: []int{0}},
				func() tea.Cmd { return do("hard") })
		}})
}

// newBranch asks for the name of a branch to make at ref ("" for HEAD) and
// checks it out.
func (g *gitreu) newBranch(ref string) tea.Cmd {
	at := "HEAD"
	if ref != "" {
		at = git.Short(ref)
	}
	return g.a.Text(terminu.TextInput{Title: "New branch", Glyph: glyphBranch, Prompt: "A branch at " + at + ", checked out",
		Verb: "make", Name: "Name", Check: g.checkBranch,
		Done: func(n string) tea.Cmd {
			return g.run("Branching…", "Branch", func() error { return g.repo.NewBranch(n, ref) },
				func() tea.Cmd {
					g.history.tabs[0].Select(kindBranch + n)
					return g.a.Toast(terminu.Success, "Made and checked out "+n)
				})
		}})
}

// newTag asks for the name of a tag to make at ref ("" for HEAD).
func (g *gitreu) newTag(ref string) tea.Cmd {
	at := "HEAD"
	if ref != "" {
		at = git.Short(ref)
	}
	return g.a.Text(terminu.TextInput{Title: "New tag", Glyph: glyphTag, Prompt: "A tag at " + at,
		Verb: "tag", Name: "Name", Check: g.checkTag,
		Done: func(n string) tea.Cmd {
			return g.run("Tagging…", "Tag", func() error { return g.repo.NewTag(n, ref) },
				func() tea.Cmd {
					g.history.tabs[2].Select(kindTag + n)
					return g.a.Toast(terminu.Success, "Tagged "+at+" "+n)
				})
		}})
}

// globalActions are what SourceTree's toolbar does, for the whole repository
// (tdp P3, M4).
func (g *gitreu) globalActions() []terminu.Action {
	off := func() bool { return !g.idle() }
	state := func(op string) func() bool {
		return func() bool { return g.snap == nil || g.snap.State.Op != op }
	}
	acts := []terminu.Action{
		{Key: "c", Label: "Commit", Desc: "the staged changes", Disabled: func() bool { return off() || !g.hasStaged() }, Do: g.commit},
		{Label: "Amend", Desc: "the last commit with the staged changes, its message kept",
			Disabled: func() bool { return off() || g.snap == nil || g.snap.HeadSHA == "" },
			AskFor: func() *terminu.Ask {
				a := &terminu.Ask{Title: "Amend", Question: "Amend the last commit, keeping its message?", Verb: "amend",
					Details: []string{firstLine(g.repo.LastMessage())}}
				if g.repo.Pushed() {
					a.Details = append(a.Details, "It is on "+g.snap.Upstream+" already: pushing it again takes a force push.")
					a.Warn = []int{1}
				}
				return a
			},
			Do: func() tea.Cmd { return g.doCommit(g.repo.LastMessage(), true) }},
		{Key: "p", Label: "Pull", Desc: "the current branch from its upstream", Disabled: off, Do: g.pull},
		{Key: "P", Label: "Push", Desc: "the current branch to its upstream", Disabled: func() bool { return off() || g.snap == nil || g.snap.Head == "" }, Do: g.push},
		{Key: "f", Label: "Fetch", Desc: "every remote", Disabled: off, Do: func() tea.Cmd {
			return g.run("Fetching…", "Fetch", g.repo.Fetch, func() tea.Cmd { return g.a.Toast(terminu.Success, "Fetched every remote") })
		}},
		{Key: "b", Label: "Branch", Desc: "a new one at HEAD, and check it out", Disabled: func() bool { return off() || g.snap == nil || g.snap.HeadSHA == "" },
			Do: func() tea.Cmd { return g.newBranch("") }},
		{Key: "M", Label: "Merge", Desc: "a branch into the current one", Disabled: off, Do: g.pickMerge},
		{Key: "t", Label: "Tag", Desc: "HEAD", Disabled: func() bool { return off() || g.snap == nil || g.snap.HeadSHA == "" },
			Do: func() tea.Cmd { return g.newTag("") }},
		{Key: "S", Label: "Stash", Desc: "the uncommitted changes", Disabled: func() bool { return off() || !g.hasChanges() }, Do: g.stash},
		{Label: "Discard all", Desc: "every change to the tracked files", Disabled: func() bool { return off() || !g.hasChanges() },
			Ask: &terminu.Ask{Title: "Discard all", Question: "Discard every change to the tracked files?", Verb: "discard",
				Details: []string{"What is not committed is lost: it can't be undone. Untracked files stay."}, Warn: []int{0}},
			Do: func() tea.Cmd {
				return g.run("Discarding…", "Discard", g.repo.DiscardAll, func() tea.Cmd { return g.a.Toast(terminu.Success, "Discarded every change") })
			}},
		{Key: "R", Label: "Refresh", Desc: "read the repository again", Do: func() tea.Cmd { return g.read(true) }},
	}
	for _, op := range []string{"merge", "rebase", "cherry-pick", "revert"} {
		acts = append(acts, terminu.Action{Key: "A", Label: "Abort " + op, Desc: "the " + op + " under way", Hidden: state(op), Disabled: off,
			Ask: &terminu.Ask{Title: "Abort", Question: "Abort the " + op + "?", Verb: "abort",
				Details: []string{"What it did so far is undone, conflicts resolved too."}, Warn: []int{0}},
			Do: func() tea.Cmd {
				return g.run("Aborting…", "Abort", func() error { return g.repo.Abort(op) },
					func() tea.Cmd { return g.a.Toast(terminu.Success, "Aborted the "+op) })
			}})
	}
	for _, op := range []string{"rebase", "cherry-pick", "revert"} {
		acts = append(acts, terminu.Action{Label: "Continue " + op, Desc: "with the conflicts resolved", Hidden: state(op),
			Disabled: func() bool { return off() || g.hasConflicts() },
			Do: func() tea.Cmd {
				return g.runWith(doneMsg{title: "Continue",
					ok:        func() tea.Cmd { return g.a.Toast(terminu.Success, "Went on with the "+op) },
					conflicts: g.conflictsSaid(strings.ToUpper(op[:1]) + op[1:])}, "Continuing…", func() error { return g.repo.Continue(op) })
			}})
	}
	return acts
}

func (g *gitreu) hasStaged() bool {
	return g.snap != nil && slices.ContainsFunc(g.snap.Files, func(f git.File) bool { return f.Staged() }) ||
		g.snap != nil && g.snap.State.Op == "merge" && !g.hasConflicts()
}

func (g *gitreu) hasChanges() bool {
	return g.snap != nil && slices.ContainsFunc(g.snap.Files, func(f git.File) bool { return !f.Untracked() })
}

func (g *gitreu) hasConflicts() bool {
	return g.snap != nil && slices.ContainsFunc(g.snap.Files, func(f git.File) bool { return f.Conflicted() })
}

func (g *gitreu) pull() tea.Cmd {
	return g.runWith(doneMsg{title: "Pull",
		ok:        func() tea.Cmd { return g.a.Toast(terminu.Success, "Pulled "+g.currentName()) },
		conflicts: g.conflictsSaid("Pull")}, "Pulling…", g.repo.Pull)
}

// push pushes the current branch; one with no upstream asks first to push it
// to the remote and track it there.
func (g *gitreu) push() tea.Cmd {
	branch := g.snap.Head
	if g.snap.Upstream != "" {
		return g.run("Pushing…", "Push", func() error { return g.repo.Push("", branch, false) },
			func() tea.Cmd { return g.a.Toast(terminu.Success, "Pushed "+branch+" to "+g.snap.Upstream) })
	}
	remote := g.repo.Remote("")
	if remote == "" {
		return g.a.ErrorAbout("Push failed", branch, "This repository has no remote to push to")
	}
	return g.a.Confirm(terminu.Ask{Title: "Push", Subject: branch, Question: "Push " + branch + " to " + remote + " and track it?", Verb: "push",
		Details: []string{branch + " has no upstream yet: it becomes " + remote + "/" + branch + "."}},
		func() tea.Cmd {
			return g.run("Pushing…", "Push", func() error { return g.repo.Push(remote, branch, true) },
				func() tea.Cmd { return g.a.Toast(terminu.Success, "Pushed "+branch+" to "+remote+"/"+branch) })
		})
}

// pickMerge asks which branch to merge into the current one (tdp select).
func (g *gitreu) pickMerge() tea.Cmd {
	var names []string
	for _, b := range g.snap.Branches {
		if !b.Current {
			names = append(names, b.Name)
		}
	}
	for _, rb := range g.snap.Remotes {
		names = append(names, rb.Name)
	}
	if len(names) == 0 {
		return g.a.Toast(terminu.Info, "No other branch to merge")
	}
	return g.a.Select(terminu.Select{Title: "Merge", Subject: g.currentName(), Glyph: glyphBranch, Options: names, Selected: -1,
		Done: func(i int) tea.Cmd {
			return g.a.Confirm(*g.askMerge(names[i]), func() tea.Cmd { return g.merge(names[i]) })
		}})
}

// stash asks for a message and puts the uncommitted changes away.
func (g *gitreu) stash() tea.Cmd {
	do := func(msg string) tea.Cmd {
		msg = strings.TrimSpace(msg)
		return g.run("Stashing…", "Stash", func() error { return g.repo.Stash(msg) },
			func() tea.Cmd { return g.a.Toast(terminu.Success, "Stashed the changes") })
	}
	return g.a.Text(terminu.TextInput{Title: "Stash", Glyph: glyphStash, Prompt: "A message, or nothing for git's own",
		Verb: "stash", Name: "Message", Done: do, Unchanged: func() tea.Cmd { return do("") }})
}
