# Changelog

Each release names the version of terminu design (tdp) it follows.

## Unreleased

- Windows (amd64, arm64): Scoop, a PowerShell install script and `gittu update`; a conflicted file opens in Notepad there,
  where gittu cannot run your editor inside itself
- A working tree file with CRLF where git keeps LF (core.autocrlf, Git for Windows' default) shows no change on those
  line endings, as `git diff` does

## v0.1.0 — tdp v1.3.3

- First version: a git client for the terminal, one screen of five panels: the files to commit above the branches,
  remote branches, tags and stashes on the left; the commits and their graph on the right, with the message and the
  files of the commit under the cursor below them
- The history graph: one row a commit, each line of history in its own colour, the branches and tags on their
  commits; `z` puts the commits or the message on the whole screen
- A commit's files and a file's two versions side by side; two commits compared the same way
- Every git operation from the `Space` menu: check out, merge, rebase, cherry-pick, revert, reset, branch, tag,
  rename, delete, stash, fetch, pull, push, each asking what it needs before it runs
- The files to commit: `s` stages and unstages, and the right shows the diff of the file under the cursor and the
  commits that changed it; discard, commit and amend
- Conflicts resolved in the editor git uses, or one block at a time (mine, theirs or both); `r` marks a file
  resolved, asking first only while conflict markers remain; abort or continue a merge, rebase, cherry-pick or revert
- `<` and `>` set the width of the left, kept for each terminal width in `state.yaml`
- It runs the git you have: your hooks, credential helper, ssh keys and settings work as they do in the shell
- macOS and Linux (WSL too)

## v0.1.0-beta.1 — tdp v1.3.3

- First version: a git client for the terminal, one screen of five panels: the files to commit above the branches,
  remote branches, tags and stashes on the left; the commits and their graph on the right, with the message and the
  files of the commit under the cursor below them
- The history graph: one row a commit, each line of history in its own colour, the branches and tags on their
  commits; `z` puts the commits or the message on the whole screen
- A commit's files and a file's two versions side by side; two commits compared the same way
- Every git operation from the `Space` menu: check out, merge, rebase, cherry-pick, revert, reset, branch, tag,
  rename, delete, stash, fetch, pull, push, each asking what it needs before it runs
- The files to commit: `s` stages and unstages, and the right shows the diff of the file under the cursor and the
  commits that changed it; discard, commit and amend
- Conflicts resolved in the editor git uses, or one block at a time (mine, theirs or both); `r` marks a file
  resolved, asking first only while conflict markers remain; abort or continue a merge, rebase, cherry-pick or revert
- `<` and `>` set the width of the left, kept for each terminal width in `state.yaml`
- It runs the git you have: your hooks, credential helper, ssh keys and settings work as they do in the shell
- macOS and Linux (WSL too)
