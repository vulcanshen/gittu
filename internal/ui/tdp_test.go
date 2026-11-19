package ui

import (
	"testing"

	terminu "github.com/vulcanshen/terminu-go"
	"github.com/vulcanshen/terminu-go/tdptest"

	"github.com/vulcanshen/gittu/internal/git"
	"github.com/vulcanshen/gittu/internal/gittest"
)

// TestFirstLaunch launches gittu as for the first time on many terminal
// sizes and icon widths, on a repository with branches, a tag, a stash and a
// remote, and walks every screen and panel: the Space menu, the global
// operation popup, the App Log and the key reference; then a terminal under
// the least size and back (tdptest.Check).
func TestFirstLaunch(t *testing.T) {
	r := gittest.Sample(t)
	r.Write("README.md", "changed\n")
	r.Write("new.txt", "new\n")
	repo, err := git.Open(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	tdptest.Check(t, tdptest.App{Name: "gittu", New: func() *terminu.App { return New(repo) }})
}
