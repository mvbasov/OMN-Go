package backend

// ----------------------------------------------------------------------
// The first start
// ----------------------------------------------------------------------
//
// initStorage held 0 of its statements under test before 26.09.45.
//
// It is the FIRST code that an install runs. It decides what a person
// sees when they open the application for the first time.
//
// It also carries the upgrade path. A tree of an old version has notes
// in the root and assets in six directories beside md/. initStorage
// moves each one. That code runs on every start of every device, and
// nothing held it.
//
// AND IT CARRIES RULE 8 of CLAUDE.md section 1: an upgrade never
// overwrites a user-owned asset. initDefaultPage writes a starter note
// only when the file is absent. A change of that one condition would
// write over the notes of a person at the next start, and no test would
// say a word.
//
// WHY newTestApp CANNOT DO THIS. That helper builds the directories by
// hand and calls loadConfig. It never calls initStorage, thus each test
// of this package runs against a layout that no install ever made.
//
// THE PRECOMPILE IS A GOROUTINE, AND IT OUTLIVES THE CALL.
//
// initStorage ends with `go a.precompileAllPages()`. That goroutine
// writes into the same directory for as long as it runs.
//
// The first draft of this file used t.TempDir for the storage. The tests
// then failed at random with "TempDir RemoveAll cleanup: directory not
// empty", because the cleanup of the framework raced the goroutine.
//
// A wait for the last file that the goroutine writes did not answer it
// either. Two tests below call initStorage TWICE, thus that file already
// exists when the second goroutine starts.
//
// siDir therefore owns the directory and its cleanup. The cleanup tries
// until the goroutine lets go, and it never fails a test. See its
// comment.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// siDir answers an empty directory that this file owns.
//
// IT IS NOT t.TempDir. The framework removes that one exactly once, at
// the end of the test. A write from the background precompile at that
// moment fails the test for no fault of the code.
//
// The cleanup here tries for ten seconds. A precompile of ten notes
// takes milliseconds, thus the first or the second try nearly always
// wins. It then IGNORES a failure: a directory left in the temporary
// tree of an ephemeral container is not a fault worth a red test.
func siDir(t *testing.T) string {
	t.Helper()
	base, err := os.MkdirTemp("", "omn-initstorage-")
	if err != nil {
		t.Fatalf("cannot make a temporary directory: %v", err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(10 * time.Second)
		for {
			if err := os.RemoveAll(base); err == nil {
				return
			}
			if time.Now().After(deadline) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	return filepath.Join(base, "storage")
}

// siApp answers an App whose storage directory is empty, and the path of
// that directory.
//
// It does NOT use newTestApp. The point of each test here is what
// initStorage makes, thus nothing may make it first.
func siApp(t *testing.T) (*App, string) {
	t.Helper()
	return &App{}, siDir(t)
}

// A fresh install gets the layout, the configuration and the notes.
//
// Each name below is one that a person meets in the first minute. The
// Welcome note holds the two start buttons, and the incoming index is
// where a note arrives from another device.
func TestInitStorageMakesAFreshInstall(t *testing.T) {
	a, dir := siApp(t)
	a.initStorage(dir)

	if a.StorageDir != dir {
		t.Fatalf("the storage directory is %q, want %q", a.StorageDir, dir)
	}
	for _, sub := range []string{"md", "html"} {
		st, err := os.Stat(filepath.Join(dir, sub))
		if err != nil || !st.IsDir() {
			t.Errorf("%s/ is missing after the first start", sub)
		}
	}
	for _, name := range []string{
		"Welcome.md", "ScriptRules.md", "QuickNotes.md", "Bookmarks.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, "md", name)); err != nil {
			t.Errorf("md/%s is missing after the first start", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Errorf("config.json is missing, thus loadConfig wrote nothing")
	}
	// The index struct exists from the start, and it holds nothing until
	// a person turns global search on.
	if a.search == nil {
		t.Error("a.search is nil, thus every search answers that the index is not ready")
	}
}

// Each bundled note reaches the device with the EMBEDDED text.
//
// TWO PATHS PUT A NOTE IN md/, and the test compares the CONTENT for
// that reason.
//
// refreshEmbeddedAssets installs the seven notes of
// versionDependentAssets, and it refreshes each one at a version change.
// The loop of initStorage writes the other three, which are Welcome,
// QuickNotes and Bookmarks. Those three belong to the person after the
// first start, thus no upgrade touches them again. See rule 8 of
// CLAUDE.md section 1.
//
// initDefaultPage under that loop is a FALLBACK for the same three, with
// a short text of its own. It runs when the embed gives nothing.
//
// A test that asked only whether the file exists passes with the whole
// extraction loop removed. The fallback writes the three, and the asset
// refresh writes the seven. The first draft of this test did that, and a
// probe proved it guarded nothing.
func TestInitStorageExtractsTheBundledNotes(t *testing.T) {
	a, dir := siApp(t)
	a.initStorage(dir)

	entries, err := staticFS.ReadDir("frontend/md")
	if err != nil {
		t.Fatalf("staticFS holds no frontend/md: %v", err)
	}
	want := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		want++
		embedded, eerr := staticFS.ReadFile("frontend/md/" + e.Name())
		if eerr != nil {
			t.Errorf("cannot read the embedded %s: %v", e.Name(), eerr)
			continue
		}
		onDisk, derr := os.ReadFile(filepath.Join(dir, "md", e.Name()))
		if derr != nil {
			t.Errorf("the bundled note %s did not reach md/: %v", e.Name(), derr)
			continue
		}
		if !bytes.Equal(onDisk, embedded) {
			t.Errorf("md/%s does not hold the embedded text.\n"+
				"  A person then reads the short fallback of initDefaultPage "+
				"in place of the real note.", e.Name())
		}
	}
	if want < 5 {
		t.Fatalf("frontend/md holds %d notes. This test proves little.", want)
	}
}

// RULE 8. A second start never writes over the note of a person.
//
// This is the test that matters most in this file. A person edits
// Welcome.md, restarts, and the edit must be there. initDefaultPage
// writes only when the file is absent, and that ONE condition is the
// whole of the rule.
func TestInitStorageKeepsAnEditedNote(t *testing.T) {
	a, dir := siApp(t)
	a.initStorage(dir)

	mine := "Title: Welcome\n\nMy own words, and nobody may write over them.\n"
	for _, name := range []string{"Welcome.md", "QuickNotes.md", "Bookmarks.md"} {
		if err := os.WriteFile(filepath.Join(dir, "md", name), []byte(mine), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The second start of the same device.
	b := &App{}
	b.initStorage(dir)

	for _, name := range []string{"Welcome.md", "QuickNotes.md", "Bookmarks.md"} {
		got, err := os.ReadFile(filepath.Join(dir, "md", name))
		if err != nil {
			t.Fatalf("md/%s went away at the second start: %v", name, err)
		}
		if string(got) != mine {
			t.Errorf("the second start wrote over md/%s.\n"+
				"  Rule 8 of CLAUDE.md section 1 says that an upgrade never "+
				"overwrites a user-owned asset.\n  It now holds %q", name,
				string(got)[:min(60, len(got))])
		}
	}
}

// The upgrade path. A note in the root moves into md/.
//
// A tree of an old version keeps each note beside config.json. The move
// runs on every start, thus a person who upgrades finds their notes and
// a person who never had that layout loses nothing.
func TestInitStorageMovesALegacyNoteIntoMd(t *testing.T) {
	a, dir := siApp(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "Title: An old note\n\nWritten before the md directory existed.\n"
	if err := os.WriteFile(filepath.Join(dir, "OldNote.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	a.initStorage(dir)

	got, err := os.ReadFile(filepath.Join(dir, "md", "OldNote.md"))
	if err != nil {
		t.Fatalf("md/OldNote.md is missing, thus the upgrade lost the note: %v", err)
	}
	if string(got) != body {
		t.Errorf("the moved note holds %q", string(got))
	}
	if _, err := os.Stat(filepath.Join(dir, "OldNote.md")); err == nil {
		t.Error("the note is still in the root as well, thus it moved as a copy")
	}
}

// The upgrade path. The six asset directories move under html/.
//
// The list in initStorage names images, user_json, css, js, json and
// fonts. Each one held its files in the root of an old tree, and each
// URL of this application resolves under html/ now.
func TestInitStorageMovesTheLegacyAssetDirectories(t *testing.T) {
	a, dir := siApp(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := []string{"images", "user_json", "css", "js", "json", "fonts"}
	for _, d := range legacy {
		p := filepath.Join(dir, d)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "mark.txt"), []byte(d), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a.initStorage(dir)

	for _, d := range legacy {
		got, err := os.ReadFile(filepath.Join(dir, "html", d, "mark.txt"))
		if err != nil {
			t.Errorf("html/%s/mark.txt is missing, thus the upgrade lost %s: %v", d, d, err)
			continue
		}
		if string(got) != d {
			t.Errorf("html/%s/mark.txt holds %q", d, got)
		}
		if _, err := os.Stat(filepath.Join(dir, d)); err == nil {
			t.Errorf("%s is still in the root as well", d)
		}
	}
}

// A directory that already holds html/ keeps what is in it.
//
// The move reads os.Stat first. A tree that upgraded once already has
// html/js, and a second move must not write over it.
func TestInitStorageLeavesAnUpgradedTreeAlone(t *testing.T) {
	a, dir := siApp(t)
	a.initStorage(dir)

	mine := filepath.Join(dir, "html", "js", "omn-go-custom.js")
	if err := os.MkdirAll(filepath.Dir(mine), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mine, []byte("// my own rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := &App{}
	b.initStorage(dir)

	got, err := os.ReadFile(mine)
	if err != nil {
		t.Fatalf("the custom file went away at the second start: %v", err)
	}
	if string(got) != "// my own rule\n" {
		t.Errorf("the second start wrote over the custom file. It now holds %q", got)
	}
}

// The override wins over every default.
//
// ServerService.storageDir(ctx) on Android passes the directory of the
// flavor into StartServer, because the Go package cannot read the
// applicationId. See rule 3 of CLAUDE.md section 1. An override that did
// not win would put the notes of an fdroid install in the directory of
// the standard one.
func TestInitStorageUsesTheOverride(t *testing.T) {
	a, dir := siApp(t)
	a.initStorage(dir)
	if a.StorageDir != dir {
		t.Errorf("the storage directory is %q, want the override %q", a.StorageDir, dir)
	}
}

// With no override the directory is ./data, relative to the working
// directory of the process.
//
// The desktop entry point passes no override. This test moves into a
// temporary directory first, thus it writes no data/ into the tree of
// the repository.
func TestInitStorageDefaultsToData(t *testing.T) {
	// siDir and not t.TempDir. The background precompile writes into
	// ./data of this directory. See the banner.
	t.Chdir(filepath.Dir(siDir(t)))
	a := &App{}
	a.initStorage("")
	if a.StorageDir != "./data" {
		t.Errorf("the storage directory is %q, want ./data", a.StorageDir)
	}
	if _, err := os.Stat(filepath.Join("data", "md", "Welcome.md")); err != nil {
		t.Errorf("data/md/Welcome.md is missing after a start with no override")
	}
}

// A directory that the application cannot make is a logged fault and
// never a panic.
//
// A person meets this with a storage path on a card that is not mounted.
// The application must start and say so, because a panic on a phone is a
// crash dialog and no message.
//
// The test puts a FILE where a directory must go. That fails for every
// user. A read-only parent does not, because this test runs as root in
// the build image, and root writes through a mode of 0555.
func TestInitStorageSurvivesADirectoryItCannotMake(t *testing.T) {
	base := filepath.Dir(siDir(t))
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &App{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("initStorage panicked on a directory it cannot make: %v", r)
		}
	}()
	a.initStorage(filepath.Join(blocker, "storage"))

	// It still answers a storage directory, thus a later call has a path
	// to report rather than an empty string.
	if a.StorageDir == "" {
		t.Error("the storage directory is empty after the fault")
	}
}
