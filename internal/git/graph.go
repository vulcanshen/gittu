package git

// The history's graph, one row a commit: each line of history is a column two
// cells wide, the commit a Node on its line, the lines going on down │, joining it
// and leaving it with ─ ╮ ╯ ╭ ╰ ├ ┤, crossing ┼ (not git log --graph, whose
// rows of lines only would put the cursor on rows that are not commits).

// Cell is one cell of a graph row: a rune and the line it belongs to, whose
// colour it takes (-1: a blank).
type Cell struct {
	Rune rune
	Line int
}

// MaxLines is how many lines a row draws: past them it ends in "…".
const MaxLines = 10

// Node is a commit on its line. Not a Nerd Font icon: a terminal that lets an
// icon spill into a blank cell after it draws it large there and small before
// a line (─), so one commit would look bigger than the next; ◉ is drawn in its
// one cell everywhere.
const Node = '◉'

// Graph lays out the commits, in the order given (a child before its
// parents), and gives each its row of cells. main, when not "", is the commit
// whose line is the main line: it and its first parents, down to the end, are
// always the first column, which no other line takes (blank above it).
func Graph(commits []Commit, main string) [][]Cell {
	chain := mainLine(commits, main)
	var lanes []string // the commit each column waits for, "" when free
	var colour []int   // each column's line, for its colour
	next := 0
	if len(chain) > 0 {
		lanes, colour, next = []string{""}, []int{0}, 1
	}
	free := func(skip map[int]bool) int {
		for i, l := range lanes {
			if l == "" && !skip[i] && !(i == 0 && len(chain) > 0) {
				return i
			}
		}
		lanes = append(lanes, "")
		colour = append(colour, 0)
		return len(lanes) - 1
	}
	rows := make([][]Cell, len(commits))
	for n, c := range commits {
		col := -1
		if chain[c.SHA] {
			col = 0 // the main line's: other columns waiting for it join it
			lanes[0] = c.SHA
		}
		for i, l := range lanes {
			if col < 0 && l == c.SHA {
				col = i
				break
			}
		}
		if col < 0 { // a tip: no child seen
			col = free(nil)
			lanes[col], colour[col] = c.SHA, next
			next++
		}
		before := append([]string(nil), lanes...)
		var joins []int // the other columns waiting for this commit end here
		for i, l := range lanes {
			if i != col && l == c.SHA {
				joins = append(joins, i)
				lanes[i] = ""
			}
		}
		var forks []int // the columns its other parents go down
		var opened = map[int]bool{}
		if len(c.Parents) == 0 {
			lanes[col] = ""
		} else {
			lanes[col] = c.Parents[0]
			skip := map[int]bool{col: true}
			for _, j := range joins {
				skip[j] = true
			}
			for _, p := range c.Parents[1:] {
				k := -1
				for i, l := range lanes {
					if i != col && l == p {
						k = i
						break
					}
				}
				if k < 0 {
					k = free(skip)
					lanes[k], colour[k] = p, next
					next++
					opened[k] = true
				}
				skip[k] = true
				forks = append(forks, k)
			}
		}
		width := max(len(before), len(lanes))
		for width > 0 && (width-1 >= len(before) || before[width-1] == "") && (width-1 >= len(lanes) || lanes[width-1] == "") && width-1 != col {
			width--
		}
		active := func(list []string, i int) bool { return i < len(list) && list[i] != "" }
		row := make([]Cell, 2*width)
		for i := range row {
			row[i] = Cell{' ', -1}
		}
		// The horizontal runs: from the commit to each column joining or
		// forking, in that column's colour.
		lo, hi := col, col
		runColour := make([]int, width)
		for i := range runColour {
			runColour[i] = -1
		}
		mark := func(k int) {
			lo, hi = min(lo, k), max(hi, k)
			if k > col {
				for i := col; i < k; i++ {
					runColour[i] = colour[k]
				}
			} else {
				for i := k; i < col; i++ {
					runColour[i] = colour[k]
				}
			}
		}
		for _, j := range joins {
			mark(j)
		}
		for _, k := range forks {
			mark(k)
		}
		isJoin := map[int]bool{}
		for _, j := range joins {
			isJoin[j] = true
		}
		isFork := map[int]bool{}
		for _, k := range forks {
			isFork[k] = true
		}
		for i := 0; i < width; i++ {
			r, line := ' ', -1
			// through: the horizontal run goes on past this column.
			through := i > lo && i < hi
			switch {
			case i == col:
				r, line = Node, colour[col]
			case isJoin[i] && isFork[i]:
				r, line = '┼', colour[i]
			case isJoin[i] && through:
				r, line = '┴', colour[i]
			case isJoin[i]:
				if i > col {
					r = '╯'
				} else {
					r = '╰'
				}
				line = colour[i]
			case isFork[i] && opened[i] && through:
				r, line = '┬', colour[i]
			case isFork[i] && opened[i]:
				if i > col {
					r = '╮'
				} else {
					r = '╭'
				}
				line = colour[i]
			case isFork[i] && through: // into a line already going down
				r, line = '┼', colour[i]
			case isFork[i]:
				if i > col {
					r = '┤'
				} else {
					r = '├'
				}
				line = colour[i]
			case active(before, i) && active(lanes, i):
				r, line = '│', colour[i]
				if i > lo && i < hi {
					r = '┼'
				}
			case i > lo && i < hi:
				r, line = '─', runColour[i]
			}
			row[2*i] = Cell{r, line}
			if i < width-1 && i >= lo && i < hi {
				row[2*i+1] = Cell{'─', runColour[i]}
			}
		}
		if width > MaxLines {
			row = append(row[:2*MaxLines-1], Cell{'…', -1})
		}
		rows[n] = trimRow(row)
		for len(lanes) > 1 && lanes[len(lanes)-1] == "" || len(lanes) == 1 && lanes[0] == "" && len(chain) == 0 {
			lanes = lanes[:len(lanes)-1]
			colour = colour[:len(colour)-1]
		}
	}
	return rows
}

// trimRow drops the blanks at the row's end.
func trimRow(row []Cell) []Cell {
	for len(row) > 0 && row[len(row)-1].Rune == ' ' {
		row = row[:len(row)-1]
	}
	return row
}

// mainLine is the commits of the main line: main and its first parents, as
// far as the commits go; empty for no main.
func mainLine(commits []Commit, main string) map[string]bool {
	chain := map[string]bool{}
	if main == "" {
		return chain
	}
	byID := make(map[string]Commit, len(commits))
	for _, c := range commits {
		byID[c.SHA] = c
	}
	for sha := main; sha != "" && !chain[sha]; {
		c, ok := byID[sha]
		if !ok {
			break
		}
		chain[sha] = true
		if len(c.Parents) == 0 {
			break
		}
		sha = c.Parents[0]
	}
	return chain
}
