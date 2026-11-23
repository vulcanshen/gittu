package ui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"
	"gopkg.in/yaml.v3"
)

// defaultTabWidth is input_tab_width's default (tdp E3).
const defaultTabWidth = 4

// configDir is the settings directory (tdp E2, E3): GITTU__CONFIG, else
// $XDG_CONFIG_HOME/gittu, else ~/.config/gittu.
func configDir() string {
	if d := os.Getenv("GITTU__CONFIG"); d != "" {
		return d
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "gittu")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gittu")
}

// ConfigPath is the settings file, config.yaml in the settings directory.
func ConfigPath() string { return filepath.Join(configDir(), "config.yaml") }

// readTabWidth reads input_tab_width from the settings file: the default with
// no file or no key. A value that is not a whole number above 0 does not fit
// (tdp E3): bad is what it was. unread are the other keys, which gittu never
// read. err is a file that can't be read.
func readTabWidth(path string) (width int, bad string, unread []string, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaultTabWidth, "", nil, nil
	}
	if err != nil {
		return defaultTabWidth, "", nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return defaultTabWidth, "", nil, err
	}
	if len(doc.Content) == 0 {
		return defaultTabWidth, "", nil, nil // an empty file
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return defaultTabWidth, "", nil, errors.New("it is not a mapping of keys to values")
	}
	width = defaultTabWidth
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "input_tab_width" {
			unread = append(unread, root.Content[i].Value)
			continue
		}
		v := root.Content[i+1]
		n, err := strconv.Atoi(v.Value)
		if v.Kind != yaml.ScalarNode || err != nil || n < 1 {
			bad = v.Value
			continue
		}
		width = n
	}
	return width, bad, unread, nil
}

// homeShort writes a path under the home directory from ~.
func homeShort(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			return filepath.Join("~", rel)
		}
	}
	return p
}

// writeTabWidth writes input_tab_width back as its default, the rest of the
// file as it was.
func writeTabWidth(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return err
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "input_tab_width" {
			root.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(defaultTabWidth)}
		}
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, st.Mode().Perm())
}

// loaded is what reading one of gittu's own files at the start came to (tdp
// E3): the notices to give, and a file that can't be read, path and err, to
// be moved aside; fresh is going on without it.
type loaded struct {
	notices []terminu.Notice
	path    string
	err     error
	fresh   func()
}

// loadFiles reads gittu's own files at the start, config.yaml then
// state.yaml (tdp E3): one that can't be read asks to be moved aside before
// the next is read; the notices of both are given at the end, in the order
// they came, with one App.Notify.
func (g *gittu) loadFiles() tea.Cmd {
	var ns []terminu.Notice
	loads := []func() loaded{g.loadConfig, g.loadState}
	var from func(i int) tea.Cmd
	from = func(i int) tea.Cmd {
		for ; i < len(loads); i++ {
			l := loads[i]()
			ns = append(ns, l.notices...)
			if l.err != nil {
				next := i + 1
				return g.a.MoveAside(l.path, l.err, func() tea.Cmd {
					if l.fresh != nil {
						l.fresh()
					}
					return from(next)
				})
			}
		}
		return g.a.Notify(ns...)
	}
	return from(0)
}

// loadConfig reads the settings at the start (tdp E3): a file that can't be
// read asks to be moved aside; a value that does not fit is kept in a copy
// and written again as its default; a key gittu never read is left in the
// file, an INFO at every start.
func (g *gittu) loadConfig() loaded {
	path := ConfigPath()
	width, bad, unread, err := readTabWidth(path)
	if err != nil {
		return loaded{path: path, err: err, fresh: func() { g.a.InputTabWidth = defaultTabWidth }}
	}
	g.a.InputTabWidth = width
	for _, k := range unread {
		g.a.Log(terminu.Info, "Not read: "+k+" in "+homeShort(path)+"; delete it if you don't need it")
	}
	if bad == "" {
		return loaded{}
	}
	misfit := terminu.Misfit{Key: "input_tab_width", Was: bad, Now: strconv.Itoa(defaultTabWidth)}
	copied, err := terminu.KeepAside(path)
	if err != nil {
		return loaded{notices: []terminu.Notice{{Kind: terminu.Error, Text: err.Error()}, terminu.MisfitNotice(path, "", misfit)}}
	}
	if err := writeTabWidth(path); err != nil {
		return loaded{notices: []terminu.Notice{{Kind: terminu.Error, Text: "Can't write " + filepath.Base(path) + ": " + err.Error()},
			terminu.MisfitNotice(path, copied, misfit)}}
	}
	return loaded{notices: []terminu.Notice{terminu.MisfitNotice(path, copied, misfit)}}
}
