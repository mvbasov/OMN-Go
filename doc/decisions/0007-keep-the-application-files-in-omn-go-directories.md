# 0007. Keep the application files in OMN-Go directories

* Status: accepted
* Version: 26.09.12
* Code: `storage.VersionDependentAssets`, `storage.RetiredAssets`,
  `storage.RenamedAssets` and `removeRetiredAssets` in
  `backend/internal/storage/assets.go`,
  `legacyAssetURL` in `backend/internal/app/serving.go`,
  `gitsync.GitignorePatterns` in `backend/internal/gitsync/repo.go`

## Context

The scripts and the style sheets of the application were in `html/js/`
and `html/css/`, beside the files of the user. A reader of the storage
directory could not tell which file belongs to whom.

## Decision

* Each script and style sheet of the application is in `html/js/OMN-Go/`
  or `html/css/OMN-Go/`. The web fonts moved with the style sheets, thus
  each `url(fonts/...)` of a vendor file still resolves.
* The files of the user stay where they are: `omn-go-custom.js`,
  `omn-go-custom.css` and the demo files of the Test tree.
* `legacyAssetURL` answers a request for an old URL with the file of the
  new place. A note that names an old path, for example
  `md/Bookmarks.md`, thus keeps working. The server never changes a note
  of the user.
* At the first start of a new version, `removeRetiredAssets` deletes the
  old copy of each moved file. It first moves a copy that a person changed
  to `asset_backups`.
* A file can also get a new name inside its `OMN-Go` directory.
  `storage.RenamedAssets` gives the new name for each old name. The first
  entry is `omn-go-sse.js`, which is `omn-go-api.js` from version 26.10.6.
  `legacyAssetURL` answers the old name at both places. A page in an open
  browser and a note of the user can name the old file.
* The build does not ship `html/css/markdown.css`. No template and no note
  loaded it.

## Consequences

* `gitsync.GitignorePatterns` names the new paths only. An old copy that stays
  on disk would become a tracked file at the next commit, and the sync
  would copy it to each device. That is why `removeRetiredAssets` must
  delete it.
* The app does not write `.gitignore`. Its patterns are in
  `.git/info/exclude`. See
  [0029](0029-keep-the-ignore-patterns-of-the-app-in-git-info-exclude.md).
* `storage.RetiredAssets` and `storage.RenamedAssets` only grow. An
  install can skip any number of versions.
* `TestEveryAppAssetIsUnderOMNGo` in
  `backend/internal/storage/assets_test.go` holds the rule for each new file.
