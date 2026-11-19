// gittu is a git client laid out like SourceTree, in the terminal: the
// branches beside the history graph, every operation in the Space menu.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vulcanshen/gittu/internal/git"
	"github.com/vulcanshen/gittu/internal/ui"
	"github.com/vulcanshen/gittu/internal/version"
)

const usage = `gittu: a git client laid out like SourceTree

Usage:
  gittu            open the repository the current directory is in
  gittu <path>     open the repository <path> is in
  gittu version    print the version (-v, --version)
  gittu help       print this help (-h, --help)

A directory named version or help: write ./version.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is gittu's command line (tdp E1); it gives the exit status.
func run(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		switch a := args[0]; {
		case a == "version" || a == "-v" || a == "--version":
			fmt.Fprintln(stdout, version.Display())
			return 0
		case a == "help" || a == "-h" || a == "--help":
			fmt.Fprint(stdout, usage)
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "gittu: unknown flag %q\n\n%s", a, usage)
			return 2
		case len(args) > 1:
			fmt.Fprintf(stderr, "gittu: one path at most\n\n%s", usage)
			return 2
		default:
			dir = a
		}
	}
	repo, err := git.Open(dir)
	if err != nil {
		fmt.Fprintln(stderr, "gittu:", err)
		return 1
	}
	if err := ui.New(repo).Run(); err != nil {
		fmt.Fprintln(stderr, "gittu:", err)
		return 1
	}
	return 0
}
