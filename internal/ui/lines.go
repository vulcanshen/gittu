package ui

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"
	"gopkg.in/yaml.v3"
)

// Where the user moved the lines between the panels (task 23; tdp screen.9).
// terminu-go moves them on a panel's > < + - and =, and hands where they are
// to App.OnLines; gittu keeps that under lines in its state file and gives it
// back in App.Lines at the start. The left key gittu wrote before tdp had the
// rule is no longer used (the user, 2026-11-26: the user sets the lines
// again): left in the file, an INFO at every start (tdp E3).

// statePath is the state file, state.yaml in GITTU__STATE, else in the
// settings directory (tdp E2, E3).
func statePath() string {
	d := os.Getenv("GITTU__STATE")
	if d == "" {
		d = configDir()
	}
	return filepath.Join(d, "state.yaml")
}

// readYAML reads a file of gittu's own: nil with no file or an empty one; err
// when it can't be read or is not a mapping of keys to values.
func readYAML(path string) (*yaml.Node, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("it is not a mapping of keys to values")
	}
	return &doc, nil
}

// writeYAML writes doc to path, indented by two, with the file's permissions
// when it is there.
func writeYAML(path string, doc *yaml.Node) error {
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	perm := fs.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), perm)
}

// readState reads where the lines were left: none with no file or no lines
// key. misfits is a lines key gittu can't read (tdp E3); unread the other
// keys, which gittu does not read; err a file that can't be read.
func readState(path string) (lines terminu.Lines, misfits []terminu.Misfit, unread []string, err error) {
	doc, err := readYAML(path)
	if doc == nil || err != nil {
		return nil, nil, nil, err
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		if k.Value != "lines" {
			unread = append(unread, k.Value)
			continue
		}
		var l terminu.Lines
		if err := v.Decode(&l); err != nil {
			misfits = append(misfits, terminu.Misfit{Key: "lines", Was: v.Value}) // taken out
			continue
		}
		lines = l
	}
	return lines, misfits, unread, nil
}

// withoutLines writes the state file again without the lines key gittu can't
// read, the rest as it was (tdp E3).
func withoutLines(path string) error {
	doc, err := readYAML(path)
	if doc == nil || err != nil {
		return err
	}
	root := doc.Content[0]
	kept := root.Content[:0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "lines" {
			kept = append(kept, root.Content[i], root.Content[i+1])
		}
	}
	root.Content = kept
	return writeYAML(path, doc)
}

// writeLines keeps l under lines in the state file, the rest of the file as
// it was; with no line kept the key goes. A file that can't be read is not
// written over: what the user wrote in it would be lost.
func writeLines(path string, l terminu.Lines) error {
	doc, err := readYAML(path)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	var val yaml.Node
	if err := val.Encode(l); err != nil {
		return err
	}
	at := -1
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "lines" {
			at = i
		}
	}
	switch {
	case len(l) == 0 && at >= 0:
		root.Content = append(root.Content[:at], root.Content[at+2:]...)
	case len(l) == 0:
		return nil
	case at >= 0:
		root.Content[at+1] = &val
	default:
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "lines"}, &val)
	}
	return writeYAML(path, doc)
}

// keepLines is App.OnLines: the lines where the user moved them, written to
// the state file; an Error toast when it can't be written.
func (g *gittu) keepLines(l terminu.Lines) tea.Cmd {
	path := statePath()
	if err := writeLines(path, l); err != nil {
		return g.a.Toast(terminu.Error, "Can't write "+filepath.Base(path)+": "+err.Error())
	}
	return nil
}

// loadState reads where the lines were left at the start (tdp E3): a file
// that can't be read asks to be moved aside, and gittu goes on with the
// default; a lines key gittu can't read is kept in a copy and taken out of
// the file; a key gittu does not read is left in the file, an INFO at every
// start.
func (g *gittu) loadState() loaded {
	path := statePath()
	lines, misfits, unread, err := readState(path)
	if err != nil {
		return loaded{path: path, err: err}
	}
	g.a.Lines = lines
	for _, k := range unread {
		said := "Not read: "
		if k == "left" { // gittu's own until 2026-11-26 (tdp E3: no longer used)
			said = "No longer used: "
		}
		g.a.Log(terminu.Info, said+k+" in "+homeShort(path)+"; delete it if you don't need it")
	}
	if len(misfits) == 0 {
		return loaded{}
	}
	copied, err := terminu.KeepAside(path)
	if err != nil {
		return loaded{notices: []terminu.Notice{{Kind: terminu.Error, Text: err.Error()}, terminu.MisfitNotice(path, "", misfits...)}}
	}
	if err := withoutLines(path); err != nil {
		return loaded{notices: []terminu.Notice{{Kind: terminu.Error, Text: "Can't write " + filepath.Base(path) + ": " + err.Error()},
			terminu.MisfitNotice(path, copied, misfits...)}}
	}
	return loaded{notices: []terminu.Notice{terminu.MisfitNotice(path, copied, misfits...)}}
}
