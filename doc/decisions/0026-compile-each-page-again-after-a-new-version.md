# 0026. Compile each page again after a new version

* Status: accepted
* Version: 26.11.1
* Code: `DropCompiledPages` in `backend/internal/storage/paths.go`,
  `refreshEmbeddedAssets` in `backend/internal/app/storage_app.go`

## Context

A compiled page `html/<name>.html` holds the note and the page shell of
`templates/index.html`. The shell holds the class names and the IDs that
the style sheets and the scripts use.

The server compiled a page again only when the note was newer, when the
page was missing, or for `?refresh`. A new version did not compile a page
again. A new version that changes a class name in the shell thus left each
old page with the old name. The page then showed with no style or with a
control that did nothing, until the user changed the note.

The runtime values, for example the theme, are not the problem. The server
puts them into the page at each request. See the last rule of section 4 of
`CLAUDE.md`.

## Decision

* At a start with a new version, the server removes each compiled page
  before the first request. A compiled page is an `.html` file under
  `html/` that has the source `md/<name>.md`.
* The server compiles each page again at its next request.
* An `.html` file with no `.md` source is a file of the user. It stays.
* The trigger is the version stamp `assets_version`, and not the count of
  changed application files. A template is in the binary, and not in the
  storage directory, thus a new template changes no file there.

## Rejected alternatives

* **Keep the names of the page shell for ever.** Each later change of the
  shell would need the same work again, and old names would stay in the
  style sheets.
* **Compare the time of each page with the time of the stamp.** That adds a
  file read to each request.

## Consequences

* The first request of each page after an update is slower by one compile.
* The cache is not in git, because `.gitignore` holds `*.html`. The removal
  thus makes no change for git sync.
* `TestANewVersionCompilesEachPageAgain` and
  `TestDropCompiledPagesRemovesOnlyCompiledPages` hold the rule.
