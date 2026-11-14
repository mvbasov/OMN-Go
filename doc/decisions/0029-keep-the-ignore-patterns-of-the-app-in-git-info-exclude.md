# 0029. Keep the ignore patterns of the app in .git/info/exclude

* Status: accepted
* Version: 26.11.28
* Code: `backend/internal/gitsync/repo.go` (`GitignorePatterns`,
  `WriteExcludeFile`, `Worktree`, `LoadGitignoreMatcher`)

## Context

The app kept its list of ignore patterns in `.gitignore` at the top of the
storage directory. At each open of the repository it added each missing
pattern at the end of the file. Git tracks `.gitignore`, and a sync carries
it to each device.

A new version that adds a pattern thus changed one tracked file on each
device:

1. Device A gets the new version. The app adds the line, and a sync sends
   the change to the server.
2. Device B gets the new version. The app adds the same line, and that
   change is not committed.
3. A pull on device B finds a local change of a tracked file. The pull
   stops with a conflict, also when the two files hold the same text.

Each new pattern caused this conflict one time on each device after the
first.

## Decision

* `gitsync.Service.WriteExcludeFile` writes `GitignorePatterns` to
  `<StorageDir>/.git/info/exclude`. Command line git reads this file as a
  second `.gitignore`. A sync never carries it, because it is below `.git`.
* go-git refuses each path below `.git` in its worktree filesystem, thus
  its status cannot read `.git/info/exclude`. The app gives go-git the same
  list in code. `gitsync.Worktree` sets `GitignorePatterns` as the
  `Excludes` of each worktree, and the status applies them.
  `LoadGitignoreMatcher` adds them to each matcher.
* The app writes the whole file, and only when the content is different.
  Two comment lines at the top say that the app writes the file, and that
  the person writes patterns in `.gitignore`.
* The app never writes, changes or makes `.gitignore`. The file belongs to
  the person. Each device gets it through the sync, the same as a note.
* The list of the app comes after the patterns of `.gitignore`, in
  `Excludes` and in the matcher. It thus wins for each status, commit,
  push, preview and force pull, also when `.gitignore` holds a `!` line.

## Rejected alternatives

* **Resolve a conflict in `.gitignore` alone during a pull.** The pull would
  take the remote file and add the missing lines again. The file would then
  still change on each device, and it would go back and forth between two
  devices with different versions.
* **Write one fixed `.gitignore` with the same bytes on each device.** Two
  devices with different versions still write different bytes, and a
  pattern of the person would go away at each open.

## Consequences

* A new version that adds a pattern changes no tracked file. A pull after
  an update finds no conflict.
* The status of the app leaves out each file of the list with no
  `.gitignore` at all (`TestWorktreeStatusLeavesOutTheFilesOfTheList`).
* `.git/info/exclude` is for command line git on the same storage
  directory. A desktop git tool then shows the same files as the app.
* The OMN-Go lines that an older version wrote into `.gitignore` stay. They
  repeat `.git/info/exclude` and do no harm. The person can delete them.
* The app no longer deletes an obsolete line from `.gitignore`. Each such
  line names an old path that `removeRetiredAssets` deletes, thus the line
  matches no file.
* A device that runs an older version still adds lines to `.gitignore`.
  The conflict can thus occur one more time, until each device has this
  version.
* A clone of the repository outside the app has no `.git/info/exclude`.
  The files of the app are absent there, thus nothing changes for it.
