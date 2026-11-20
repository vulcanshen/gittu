package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionAndHelp(t *testing.T) {
	for _, arg := range []string{"version", "-v", "--version"} {
		var out, errOut bytes.Buffer
		if code := run([]string{arg}, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != "dev" {
			t.Errorf("%s: exit %d, %q", arg, code, out.String())
		}
	}
	for _, arg := range []string{"help", "-h", "--help"} {
		var out, errOut bytes.Buffer
		code := run([]string{arg}, &out, &errOut)
		if code != 0 || !strings.Contains(out.String(), "gittu <path>") || !strings.Contains(out.String(), "./version") {
			t.Errorf("%s: exit %d, %q", arg, code, out.String())
		}
	}
}

// TestUpdateFromSource: a build from source (version dev) says to update it
// there, exit 1 (tdp E1), looking nothing up.
func TestUpdateFromSource(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"update"}, &out, &errOut)
	if code != 1 || !strings.HasPrefix(errOut.String(), "gittu: This gittu is built from source: update it there (`git pull && make install`)") {
		t.Errorf("exit %d, %q", code, errOut.String())
	}
	out.Reset()
	run([]string{"help"}, &out, &errOut)
	if !strings.Contains(out.String(), "gittu update") {
		t.Errorf("help has no update: %q", out.String())
	}
}

func TestNotARepository(t *testing.T) {
	var out, errOut bytes.Buffer
	dir := t.TempDir()
	if code := run([]string{dir}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "gittu: "+dir+" is not in a git repository") {
		t.Errorf("exit %d, %q", code, errOut.String())
	}
	errOut.Reset()
	if code := run([]string{filepath.Join(dir, "gone")}, &out, &errOut); code != 1 || !strings.HasPrefix(errOut.String(), "gittu: ") {
		t.Errorf("exit %d, %q", code, errOut.String())
	}
}

func TestNoGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var out, errOut bytes.Buffer
	if code := run([]string{os.TempDir()}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "git not found in PATH") {
		t.Errorf("exit %d, %q", code, errOut.String())
	}
}

func TestBadArguments(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--nope"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "Unknown flag") {
		t.Errorf("exit %d, %q", code, errOut.String())
	}
	errOut.Reset()
	if code := run([]string{"a", "b"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "One path at most") {
		t.Errorf("exit %d, %q", code, errOut.String())
	}
}
