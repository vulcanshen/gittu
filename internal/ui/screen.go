package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"
)

// gittu has one screen (the user, 2026-11-22: History and Working tree made
// one, the room of the screen chips and the statusbar given back). On the
// left [1] CHANGES above [2] BRANCHES; on the right [3], with [4] and [5]
// under it. [3], [4] and [5] follow the leading panel, [1] or [2], that last
// had the focus (tdp L5, panels that follow a leading panel): after [1], [3]
// is the diff of the file under its cursor and [4] that file's history, [5]
// hidden; after [2], [3] is the commits, [4] the message and [5] the files of
// the commit under [3]'s cursor. The focus going into [3], [4] or [5]
// changes nothing. What the statusbar said is in [1]'s location row (the
// user): a merge or what git does, while there is one.

// The places of the screen's panels (Screen.Panels).
const (
	placeChanges = iota
	placeBranches
	placeMain  // [3]: the diff, or the commits
	placeLeft  // [4]: the file's history, or the message
	placeRight // [5]: the files, hidden after [1]
)

// newScreen makes the screen; [2] has the focus, the commits on the right:
// gittu opens on the branches beside the history graph (task 1).
func (g *gittu) newScreen() {
	h, s := g.history, g.status
	// TODO(tgo): the places as [1] has them, the first panel with the focus:
	// AddScreen checks that [1]'s Enter finds its details, [3] the diff,
	// among them before it shows any lead (sent to the tdp session).
	g.screen = &terminu.Screen{Name: "gittu", Layout: g.layout,
		Panels:    []*terminu.Panel{s.filesP, h.branchP, s.diffP, s.logP, h.filesP},
		Following: []int{placeMain, placeLeft, placeRight},
		Leads: []terminu.Lead{
			{Panel: h.branchP, Shows: []*terminu.Panel{h.histP, h.descP, h.filesP}},
			{Panel: s.filesP, Shows: []*terminu.Panel{s.diffP, s.logP, nil}},
		}}
	g.a.AddScreen(g.screen)
	g.a.Focus(h.branchP)
}

// commitsShown reports [2] the leading panel: [3] the commits.
func (g *gittu) commitsShown() bool { return g.screen.Panels[placeMain] == g.history.histP }

// layout is the panels as zoomed (tdp screen.6): the left a third of the
// width, [1] above [2], 1:2 (the user); at zoom 1 the left at its least
// width; at zoom 2 the focused panel alone on the whole area (the others
// drawn nowhere). On the right [3], then a third of the height for [4] and
// [5] side by side, 2:3: [5]'s paths take the room, [4] zooms to be read
// (the user); [4] the whole width while [5] is hidden. Every draw writes
// [1]'s location row again: what git does changes it.
func (g *gittu) layout(w, ht int) []terminu.Rect {
	h, s := g.history, g.status
	s.located()
	rects := make([]terminu.Rect, len(g.screen.Panels))
	if h.zoom == 2 {
		for i, p := range g.screen.Panels {
			if p == g.screen.Focused() {
				rects[i] = terminu.Rect{W: w, H: ht}
			}
		}
	} else {
		left := w / 3
		if h.zoom == 1 {
			left = min(leftLeast, w/3)
		}
		top := ht / 3
		rects[placeChanges] = terminu.Rect{X: 0, Y: 0, W: left, H: top}
		rects[placeBranches] = terminu.Rect{X: 0, Y: top, W: left, H: ht - top}
		rw := w - left
		low := ht / 3
		rects[placeMain] = terminu.Rect{X: left, Y: 0, W: rw, H: ht - low}
		lw := rw * 2 / 5
		if g.screen.Hidden(placeRight) {
			lw = rw
		} else {
			rects[placeRight] = terminu.Rect{X: left + lw, Y: ht - low, W: rw - lw, H: low}
		}
		rects[placeLeft] = terminu.Rect{X: left, Y: ht - low, W: lw, H: low}
	}
	s.laidOut(rects[placeChanges].W)
	if g.commitsShown() {
		h.laidOut(rects[placeMain].W, rects[placeLeft].W)
	}
	return rects
}

// num is p's [N] on the screen: the places are numbered in order, a hidden
// one keeping its number (tdp K1, L5).
func (g *gittu) num(p *terminu.Panel) int {
	for i, q := range g.screen.Panels {
		if q == p {
			return i + 1
		}
	}
	return 0
}

// unzoom is the panel operation that puts the zoomed screen back, on every
// panel the focus can be on while zoomed (tdp screen.6).
func (g *gittu) unzoom() terminu.Action {
	return terminu.Action{Key: "z", Label: "Unzoom", Desc: "back to both panels", Scope: panel, Hint: "unzoom",
		Hidden: func() bool { return g.history.zoom == 0 }, Do: func() tea.Cmd { return g.history.zoomTo(0) }}
}
