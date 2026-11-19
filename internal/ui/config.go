package ui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"
	"gopkg.in/yaml.v3"
)

// defaultTabWidth is input_tab_width's default (tdp E3).
const defaultTabWidth = 4

// configDir is the settings directory (tdp E2, E3): GITREU__CONFIG, else
// $XDG_CONFIG_HOME/gitreu, else ~/.config/gitreu.
func configDir() string {
	if d := os.Getenv("GITREU__CONFIG"); d != "" {
		return d
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "gitreu")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gitreu")
}

// ConfigPath is the settings file, config.yaml in the settings directory.
func ConfigPath() string { return filepath.Join(configDir(), "config.yaml") }

// readTabWidth reads input_tab_width from the settings file: the default with
// no file or no key. A value that is not a whole number above 0 does not fit
// (tdp E3): bad is what it was. err is a file that can't be read.
func readTabWidth(path string) (width int, bad string, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaultTabWidth, "", nil
	}
	if err != nil {
		return defaultTabWidth, "", err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return defaultTabWidth, "", err
	}
	if len(doc.Content) == 0 {
		return defaultTabWidth, "", nil // an empty file
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return defaultTabWidth, "", errors.New("it is not a mapping of keys to values")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "input_tab_width" {
			continue
		}
		v := root.Content[i+1]
		n, err := strconv.Atoi(v.Value)
		if v.Kind != yaml.ScalarNode || err != nil || n < 1 {
			return defaultTabWidth, v.Value, nil
		}
		return n, "", nil
	}
	return defaultTabWidth, "", nil
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

// loadConfig reads the settings at the start (tdp E3): a file that can't be
// read asks to be moved aside; a value that does not fit is kept in a copy
// and written again as its default.
func (g *gitreu) loadConfig() tea.Cmd {
	path := ConfigPath()
	width, bad, err := readTabWidth(path)
	if err != nil {
		return g.a.MoveAside(path, err, func() tea.Cmd {
			g.a.InputTabWidth = defaultTabWidth
			return nil
		})
	}
	g.a.InputTabWidth = width
	if bad == "" {
		return nil
	}
	misfit := terminu.Misfit{Key: "input_tab_width", Was: bad, Now: strconv.Itoa(defaultTabWidth)}
	copied, err := terminu.KeepAside(path)
	if err != nil {
		return g.a.Notify(terminu.Notice{Kind: terminu.Error, Text: err.Error()}, terminu.MisfitNotice(path, "", misfit))
	}
	if err := writeTabWidth(path); err != nil {
		return g.a.Notify(terminu.Notice{Kind: terminu.Error, Text: "Can't write " + filepath.Base(path) + ": " + err.Error()},
			terminu.MisfitNotice(path, copied, misfit))
	}
	return g.a.Notify(terminu.MisfitNotice(path, copied, misfit))
}
