package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/vulcanshen/gitreu/internal/git"
)

// changes are what differs between two reads of the repository, as the App
// Log says it (task 19): the files whose status changed, then the file [2]
// shows when only its text did (reread), HEAD, the branches, tags and
// stashes, and an operation under way starting or ending.
func changes(old, now *git.Snapshot, reread string) []string {
	if old == nil || now == nil {
		return nil
	}
	var out []string
	add := func(s string) {
		for _, o := range out {
			if o == s {
				return
			}
		}
		out = append(out, s)
	}

	was := map[string]git.File{}
	for _, f := range old.Files {
		was[f.Path] = f
	}
	for _, f := range now.Files {
		if w, ok := was[f.Path]; !ok || w != f {
			add(f.Path)
		}
		delete(was, f.Path)
	}
	for _, f := range old.Files {
		if _, ok := was[f.Path]; ok {
			add(f.Path) // committed, discarded or deleted outside
		}
	}
	if reread != "" {
		add(reread)
	}

	if old.Head != now.Head {
		if now.Head == "" {
			add("HEAD (detached)")
		} else {
			add("HEAD (on " + now.Head + ")")
		}
	} else if now.Head == "" && old.HeadSHA != now.HeadSHA {
		add("HEAD")
	}

	refs := func(kind string, before, after map[string]string) {
		for name, sha := range after {
			switch b, ok := before[name]; {
			case !ok:
				add(kind + " " + name + " (new)")
			case b != sha:
				add(kind + " " + name)
			}
		}
		for name := range before {
			if _, ok := after[name]; !ok {
				add(kind + " " + name + " (deleted)")
			}
		}
	}
	branches := func(s *git.Snapshot) map[string]string {
		m := map[string]string{}
		for _, b := range s.Branches {
			m[b.Name] = b.SHA
		}
		return m
	}
	remotes := func(s *git.Snapshot) map[string]string {
		m := map[string]string{}
		for _, b := range s.Remotes {
			m[b.Name] = b.SHA
		}
		return m
	}
	tags := func(s *git.Snapshot) map[string]string {
		m := map[string]string{}
		for _, t := range s.Tags {
			m[t.Name] = t.SHA
		}
		return m
	}
	n := len(out)
	refs("branch", branches(old), branches(now))
	refs("remote branch", remotes(old), remotes(now))
	refs("tag", tags(old), tags(now))
	slices.Sort(out[n:]) // map order is not the same twice

	if len(old.Stashes) != len(now.Stashes) || len(now.Stashes) > 0 && old.Stashes[0].SHA != now.Stashes[0].SHA {
		add("stashes")
	}
	if old.State.Op != now.State.Op {
		if now.State.Op != "" {
			add("a " + now.State.Op + " under way")
		} else {
			add("the " + old.State.Op + " ended")
		}
	}
	return out
}

// said is the App Log's words for what changed: the first three, and how
// many more.
func said(names []string) string {
	if len(names) <= 3 {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:3], ", ") + " and " + strconv.Itoa(len(names)-3) + " more"
}
