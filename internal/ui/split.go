package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"
	"gopkg.in/yaml.v3"
)

// The line between the left, [1] and [2], and the right (task 23): < moves
// it left and > right, whichever panel has the focus (the user, 2026-11-23),
// and gittu keeps where it is for the terminal's width in its state, to use
// whenever the terminal is that wide again; other widths have a third.

const (
	// rightLeast is the right's least width: what it has at 60 columns,
	// [3], [4] and [5] still laid out (tdp L1).
	rightLeast = 40
	// leastWidth is the least size's width (tdp L1): the left and the right
	// at their least.
	leastWidth = leftLeast + rightLeast
	// splitStep is how far < and > move the line.
	splitStep = 2
)

// leftOf is the left's width on a terminal w wide: as the user set it for
// that width, else a third (task 1). A third is between the left's least
// and the right's from 60 columns on.
func (g *gittu) leftOf(w int) int {
	if n, ok := g.lefts[w]; ok {
		return n
	}
	return w / 3
}

// splitActions are the global operations that move the line (tdp P3: they
// act on the screen, not on a panel; M4: pressed on the panels too), off
// while zoomed and at their end (tdp M6).
func (g *gittu) splitActions() []terminu.Action {
	off := func(atEnd func(w, left int) bool) func() bool {
		return func() bool {
			w, _ := g.a.Size()
			return g.history.zoomed || atEnd(w, g.leftOf(w))
		}
	}
	return []terminu.Action{
		{Key: "<", Label: "Narrow the left", Desc: "by 2 columns, kept for this terminal width",
			Disabled: off(func(_, left int) bool { return left <= leftLeast }),
			Do:       func() tea.Cmd { return g.moveSplit(-splitStep) }},
		{Key: ">", Label: "Widen the left", Desc: "by 2 columns, kept for this terminal width",
			Disabled: off(func(w, left int) bool { return left >= w-rightLeast }),
			Do:       func() tea.Cmd { return g.moveSplit(splitStep) }},
	}
}

// moveSplit moves the line by columns, keeps it for the terminal's width and
// says so in the App Log (tdp note.7).
func (g *gittu) moveSplit(by int) tea.Cmd {
	w, _ := g.a.Size()
	n := min(max(g.leftOf(w)+by, leftLeast), w-rightLeast)
	g.lefts[w] = n
	g.a.Log(terminu.Info, fmt.Sprintf("Left %d of %d columns", n, w))
	path := statePath()
	if err := writeLeft(path, w, n); err != nil {
		return g.a.Toast(terminu.Error, "Can't write "+filepath.Base(path)+": "+err.Error())
	}
	return nil
}

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

// leftFits reads one width of left, the terminal's width and the left's: ok
// when gittu can use them (tdp E3); else the misfit, using a third for a
// width it can read, taken out otherwise.
func leftFits(k, v *yaml.Node) (w, n int, m terminu.Misfit, ok bool) {
	m = terminu.Misfit{Key: "left." + k.Value, Was: v.Value}
	w, err := strconv.Atoi(k.Value)
	if k.Kind != yaml.ScalarNode || err != nil || w < leastWidth {
		return 0, 0, m, false
	}
	m.Now = strconv.Itoa(w / 3)
	n, err = strconv.Atoi(v.Value)
	if v.Kind != yaml.ScalarNode || err != nil || n < leftLeast || n > w-rightLeast {
		return 0, 0, m, false
	}
	return w, n, m, true
}

// readState reads the left's widths from the state file: none with no file or
// no key. misfits are what gittu can't use (tdp E3); unread the other keys,
// which gittu never read; err a file that can't be read.
func readState(path string) (lefts map[int]int, misfits []terminu.Misfit, unread []string, err error) {
	lefts = map[int]int{}
	doc, err := readYAML(path)
	if doc == nil || err != nil {
		return lefts, nil, nil, err
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		if k.Value != "left" {
			unread = append(unread, k.Value)
			continue
		}
		if v.Kind != yaml.MappingNode {
			misfits = append(misfits, terminu.Misfit{Key: "left", Was: v.Value}) // taken out
			continue
		}
		for j := 0; j+1 < len(v.Content); j += 2 {
			w, n, m, ok := leftFits(v.Content[j], v.Content[j+1])
			if !ok {
				misfits = append(misfits, m)
				continue
			}
			lefts[w] = n
		}
	}
	return lefts, misfits, unread, nil
}

// writeFits writes the state file again without what gittu can't use, the
// rest as it was (tdp E3).
func writeFits(path string) error {
	doc, err := readYAML(path)
	if doc == nil || err != nil {
		return err
	}
	root := doc.Content[0]
	kept := root.Content[:0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		if k.Value == "left" {
			if v.Kind != yaml.MappingNode {
				continue
			}
			fit := v.Content[:0]
			for j := 0; j+1 < len(v.Content); j += 2 {
				if _, _, _, ok := leftFits(v.Content[j], v.Content[j+1]); ok {
					fit = append(fit, v.Content[j], v.Content[j+1])
				}
			}
			v.Content = fit
		}
		kept = append(kept, k, v)
	}
	root.Content = kept
	return writeYAML(path, doc)
}

// writeLeft keeps the left's width n for a terminal w wide in the state file,
// the rest of the file as it was. A file that can't be read is not written
// over: what the user wrote in it would be lost.
func writeLeft(path string, w, n int) error {
	doc, err := readYAML(path)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	var left *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "left" {
			left = root.Content[i+1]
		}
	}
	if left == nil {
		left = &yaml.Node{Kind: yaml.MappingNode}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "left"}, left)
	}
	if left.Kind != yaml.MappingNode {
		return errors.New("left in it is not a mapping of widths")
	}
	key, val := strconv.Itoa(w), &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(n)}
	for j := 0; j+1 < len(left.Content); j += 2 {
		if left.Content[j].Value == key {
			left.Content[j+1] = val
			return writeYAML(path, doc)
		}
	}
	left.Content = append(left.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: key}, val)
	return writeYAML(path, doc)
}

// loadState reads the left's widths at the start (tdp E3): a file that can't
// be read asks to be moved aside, and gittu goes on with none; what gittu
// can't use is kept in a copy and taken out of the file; a key gittu never
// read is left in the file, an INFO at every start.
func (g *gittu) loadState() loaded {
	path := statePath()
	lefts, misfits, unread, err := readState(path)
	if err != nil {
		return loaded{path: path, err: err}
	}
	g.lefts = lefts
	for _, k := range unread {
		g.a.Log(terminu.Info, "Not read: "+k+" in "+homeShort(path)+"; delete it if you don't need it")
	}
	if len(misfits) == 0 {
		return loaded{}
	}
	copied, err := terminu.KeepAside(path)
	if err != nil {
		return loaded{notices: []terminu.Notice{{Kind: terminu.Error, Text: err.Error()}, terminu.MisfitNotice(path, "", misfits...)}}
	}
	if err := writeFits(path); err != nil {
		return loaded{notices: []terminu.Notice{{Kind: terminu.Error, Text: "Can't write " + filepath.Base(path) + ": " + err.Error()},
			terminu.MisfitNotice(path, copied, misfits...)}}
	}
	return loaded{notices: []terminu.Notice{terminu.MisfitNotice(path, copied, misfits...)}}
}
