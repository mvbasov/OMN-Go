package backend

// ----------------------------------------------------------------------
// The comment style gate
// ----------------------------------------------------------------------
//
// Section 10 of CLAUDE.md applies Simplified Technical English to every
// word that a person reads. A code comment is such a word. Section 11
// rule 7 asks the writer to check the text before the patch arrives.
//
// A check by eye fails. A long sentence and a word from the "do not use"
// list are both hard to see and easy to count. This test counts them.
//
// WHY A TABLE OF NUMBERS, AND NOT A DEMAND FOR ZERO.
//
// The tree carried more than one thousand faults on the day of this
// test. A test that asks for zero fails on the first run. It then stays
// red for many patches, and a red gate teaches the reader to pass the
// gate by hand. The table below records what each file owes today.
//
// The test fails in TWO directions:
//
//   - A file ABOVE its number holds a new fault. Repair the comment.
//   - A file BELOW its number had a repair, and the number is now
//     wrong. Lower the number in the same patch.
//
// The second direction matters as much as the first. It turns each
// style pass into a number that must go down, which is a check on the
// pass itself. It also keeps the table true.
//
// A file that reaches zero leaves the table. A file that is absent from
// the table must hold no fault, thus a new file starts clean.
//
// WHAT IT READS.
//
// It reads a WHOLE LINE comment in a Go file, a JavaScript file and a
// Java file. A comment at the end of a code line is short by nature, and
// the scan steps over it.
//
// It skips a file with the .min.js suffix. A vendored file is not ours
// to write, and katex.min.js alone is 277 kilobytes.
//
// It skips a directory that a build makes, for example android/app/build.
// A generated Java file is not ours to write either. The list of skipped
// names is below.
//
// THE FOUR RULES.
//
// A paragraph is a run of comment lines with no empty line and no rule
// line between them. Each paragraph gives at most one count for each of
// the last three rules below.
//
//   - long: a sentence of more than 25 words. Section 10 sets the limit.
//     A paragraph with three long sentences counts three.
//   - semicolon: a semicolon in prose. A paragraph that holds a brace, a
//     parenthesis, an equal sign or an angle bracket is code text, and
//     the rule steps over it.
//   - banned: a word from the "do not use" list of section 10.
//   - contraction: a contraction. Section 10 forbids each one.
//
// THE ONE EXCEPTION. The marker comment of the bookmark note keeps its
// contraction. It is a literal string that the note carries, and a
// change to it would break each stored note. See handlers.go and
// storage.go. The scanner removes that exact string before it looks for
// a contraction.
//
// THIS FILE READS ITSELF. It is absent from the table below, thus it
// must hold no fault. A rule that the author breaks is not a rule.

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The count of each file today.
//
// A number that goes UP is a new comment that breaks a rule. A number
// that goes DOWN is a style pass, and the commit that does the pass
// lowers the number here.
//
// The path is relative to the root of the repository.
var commentStyleDebt = map[string]int{
	"android/app/src/main/java/net/basov/omngo/MainActivity.java": 113,
	"backend/frontend/html/js/OMN-Go/omn-go-core.js":              71,
	"backend/git_sync.go":                                          61,
	"backend/templates.go":                                         49,
	"backend/handlers.go":                                          44,
	"backend/note_exchange.go":                                     45,
	"backend/frontend/html/js/OMN-Go/omn-go-search.js":             43,
	"backend/frontend/html/js/OMN-Go/omn-go-editor.js":             41,
	"backend/baseline_test.go":                                     35,
	"backend/search.go":                                            37,
	"backend/search_sections.go":                                   30,
	"backend/files_index.go":                                       29,
	"backend/search_match.go":                                      28,
	"backend/frontend/html/js/OMN-Go/omn-go-sse.js":                23,
	"backend/markdown.go":                                          21,
	"backend/files_index_test.go":                                  20,
	"backend/config.go":                                            20,
	"backend/db_backup.go":                                         20,
	"backend/git_repo.go":                                          20,
	"backend/search_index.go":                                      19,
	"backend/search_sections_test.go":                              17,
	"backend/serving.go":                                           16,
	"backend/note_exchange_test.go":                                15,
	"backend/frontend/html/js/OMN-Go/omn-go-sync.js":               14,
	"backend/note_files.go":                                        14,
	"backend/search_test.go":                                       13,
	"backend/sqlite.go":                                            13,
	"backend/handlers_test.go":                                     12,
	"backend/search_highlight_test.go":                             12,
	"backend/tags.go":                                              10,
	"backend/header_block.go":                                      9,
	"backend/markdown_test.go":                                     9,
	"backend/search_config_test.go":                                9,
	"backend/search_index_test.go":                                 9,
	"backend/assets.go":                                            8,
	"backend/frontend/html/js/OMN-Go/omn-go-config.js":             8,
	"backend/git_sync_test.go":                                     8,
	"backend/frontend/html/js/OMN-Go/omn-go-bookmark.js":           7,
	"backend/templates_test.go":                                    7,
	"android/app/src/main/java/net/basov/omngo/ServerService.java": 6,
	"backend/git_handlers.go":                                      6,
	"backend/search_page_test.go":                                  6,
	"backend/serving_test.go":                                      6,
	"backend/storage.go":                                           6,
	"backend/config_port_test.go":                                  5,
	"backend/git_repo_test.go":                                     5,
	"backend/note_files_test.go":                                   5,
	"backend/render_cache.go":                                      5,
	"backend/assets_test.go":                                       4,
	"backend/sync_errors_test.go":                                  4,
	"backend/frontend/html/js/OMN-Go/Bookmarker.js":                3,
	"backend/middleware_test.go":                                   3,
	"backend/search_match_test.go":                                 3,
	"backend/hostname.go":                                          2,
	"backend/paths.go":                                             2,
	"backend/ports_test.go":                                        2,
	"backend/render_cache_test.go":                                 2,
	"main_desktop.go":                                              2,
	"backend/db_backup_test.go":                                    1,
	"backend/header_block_test.go":                                 1,
	"backend/java_test.go":                                         1,
	"backend/logger.go":                                            1,
}

// The marker that keeps its contraction. See the header above.
const styleMarkerException = "<!-- Don't edit body below this line -->"

// The word limit of one sentence. Section 10 of CLAUDE.md sets it.
const styleMaxWordsInSentence = 25

// The "do not use" list of CLAUDE.md section 10, with the plural and the
// past form of each verb that takes one.
var styleBannedRe = regexp.MustCompile(
	`(?i)\b(seamless|robust|powerful|leverage|ensure[sd]?|delve|streamline[sd]?|simply|just|in order to)\b`)

// The contractions that this project writes by accident. The pattern
// covers each "n't" form, the common "is" forms, and the pronoun forms.
var styleContractionRe = regexp.MustCompile(
	`(?i)\b\w+n't\b|\bit's\b|\bthat's\b|\bwhat's\b|\bhere's\b|\bthere's\b|\blet's\b|\bdon't\b|\bwe'\w+|\byou'\w+|\bthey'\w+`)

// A whole line comment. The pattern is anchored, thus a comment at the
// end of a code line does not match.
var styleCommentLineRe = regexp.MustCompile(`^\s*//\s?(.*)$`)

var styleSpaceRunRe = regexp.MustCompile(`\s+`)
var styleLetterRe = regexp.MustCompile(`[A-Za-z]`)

// The start of a list item inside a comment. A banner of this project
// often carries a list, and each item is its own statement.
var styleBulletRe = regexp.MustCompile(`^([-*\x{2022}]|\(?\d+[.)])\s+`)

// A paragraph that holds one of these characters is code text. The
// semicolon rule steps over it.
var styleCodeCharRe = regexp.MustCompile(`[{}()=<>]`)

// The trees that hold source that a person wrote. The scan also takes
// each Go file in the root of the repository.
var styleScanRoots = []string{"backend", "android"}

// A directory that a build or a tool makes. It holds no comment that a
// person wrote, and it is absent from a fresh clone.
var styleSkipDirs = map[string]bool{
	".git": true, ".gradle": true, ".idea": true,
	"node_modules": true, "build": true, "bin": true,
	"gen": true, "out": true, "libs": true,
	"output-binaries": true, "data": true,
}

type styleCount struct {
	long        int
	semicolon   int
	banned      int
	contraction int
}

func (c styleCount) total() int {
	return c.long + c.semicolon + c.banned + c.contraction
}

// A comment paragraph, and the units inside it.
//
// Text is the whole paragraph. The semicolon rule, the banned word rule
// and the contraction rule each read it, and each counts one time for
// the paragraph.
//
// Units are the parts that the SENTENCE rule reads. A paragraph with no
// list holds one unit, which is the whole text. A paragraph that holds a
// list holds one unit for the text above the list, and one unit for each
// item of it.
type commentParagraph struct {
	Text  string
	Units []string
}

// commentParagraphs answers each comment paragraph of one source file.
//
// A paragraph ends at a line that is not a whole line comment. It also
// ends at an empty comment line, and at a rule line of dashes, equal
// signs or stars. A banner of dashes never joins two paragraphs.
//
// WHY A LIST ITEM IS ITS OWN UNIT. A banner of this project often ends
// with a list, and an item of a list often ends with a comma and not a
// period. The scan of 26.09.33 joined the whole list into one string,
// found no period, and reported one sentence of fifty five words.
//
// That is a false report. The rule of CLAUDE.md section 10 is about one
// idea in one sentence, and each item of a list is one idea. A writer
// who obeys the false report writes worse text, not better.
//
// 26.09.36 met the fault in a new file, and the writer changed a comma
// to a period to pass the gate. The gate must not ask for that.
func commentParagraphs(src string) []commentParagraph {
	var out []commentParagraph
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		para := commentParagraph{Text: strings.Join(cur, " ")}
		var unit []string
		endUnit := func() {
			if len(unit) > 0 {
				para.Units = append(para.Units, strings.Join(unit, " "))
				unit = nil
			}
		}
		for _, line := range cur {
			if styleBulletRe.MatchString(strings.TrimSpace(line)) {
				endUnit()
			}
			unit = append(unit, line)
		}
		endUnit()
		out = append(out, para)
		cur = nil
	}
	for _, line := range strings.Split(src, "\n") {
		m := styleCommentLineRe.FindStringSubmatch(line)
		if m == nil {
			flush()
			continue
		}
		body := strings.TrimSpace(m[1])
		if body == "" || strings.Trim(body, "-=* ") == "" {
			flush()
			continue
		}
		cur = append(cur, m[1])
	}
	flush()
	return out
}

// styleSentences splits one paragraph into sentences.
//
// The input holds single spaces only. A split point is a space after a
// period, an exclamation mark or a question mark.
func styleSentences(text string) []string {
	var out []string
	start := 0
	for i := 1; i < len(text); i++ {
		if text[i] == ' ' && strings.IndexByte(".!?", text[i-1]) >= 0 {
			out = append(out, text[start:i])
			start = i + 1
		}
	}
	return append(out, text[start:])
}

// styleWordCount counts the tokens that hold a letter. A bare number and
// a bare bullet character do not count as words.
func styleWordCount(sentence string) int {
	n := 0
	for _, w := range strings.Fields(sentence) {
		if styleLetterRe.MatchString(w) {
			n++
		}
	}
	return n
}

// countCommentStyle counts the faults of one source file.
func countCommentStyle(src string) styleCount {
	var c styleCount
	for _, para := range commentParagraphs(src) {
		text := strings.TrimSpace(styleSpaceRunRe.ReplaceAllString(para.Text, " "))
		for _, unit := range para.Units {
			flat := strings.TrimSpace(styleSpaceRunRe.ReplaceAllString(unit, " "))
			for _, s := range styleSentences(flat) {
				if styleWordCount(s) > styleMaxWordsInSentence {
					c.long++
				}
			}
		}
		if strings.Contains(text, ";") && !styleCodeCharRe.MatchString(text) {
			c.semicolon++
		}
		if styleBannedRe.MatchString(text) {
			c.banned++
		}
		if styleContractionRe.MatchString(strings.ReplaceAll(text, styleMarkerException, "")) {
			c.contraction++
		}
	}
	return c
}

// commentStyleFiles answers each source path that the scan reads. Each
// path is relative to the root of the repository.
func commentStyleFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	take := func(rel string) {
		name := filepath.Base(rel)
		if strings.HasSuffix(name, ".min.js") {
			return
		}
		switch filepath.Ext(name) {
		case ".go", ".js", ".java":
			out = append(out, filepath.ToSlash(rel))
		}
	}

	roots, err := filepath.Glob(filepath.Join("..", "*.go"))
	if err != nil {
		t.Fatalf("the glob of the repository root failed: %v", err)
	}
	for _, p := range roots {
		take(filepath.Base(p))
	}

	for _, root := range styleScanRoots {
		base := filepath.Join("..", root)
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if styleSkipDirs[d.Name()] {
					return fs.SkipDir
				}
				return nil
			}
			rel, relErr := filepath.Rel("..", p)
			if relErr != nil {
				return relErr
			}
			take(rel)
			return nil
		})
		if err != nil {
			t.Fatalf("the walk of %s failed: %v", root, err)
		}
	}
	sort.Strings(out)
	return out
}

// No file may hold more comment style faults than the table allows, and
// no file may hold fewer without a change to the table.
//
// A failure reads in one of three ways. A number went up, thus a new
// comment breaks a rule. A number went down, thus a style pass landed
// and the table needs the new number. A path left the tree, thus the
// table holds a dead line.
func TestCommentStyleDoesNotGetWorse(t *testing.T) {
	seen := make(map[string]bool)
	for _, rel := range commentStyleFiles(t) {
		src, err := readRepoFile(rel)
		if err != nil {
			t.Fatalf("cannot read %s: %v", rel, err)
		}
		seen[rel] = true
		got := countCommentStyle(src).total()
		want := commentStyleDebt[rel]
		switch {
		case got > want:
			t.Errorf("%s holds %d comment style faults and the table allows %d.\n"+
				"  A comment breaks a rule of CLAUDE.md section 10.\n"+
				"  Repair the comment. Do not raise the number in commentStyleDebt.",
				rel, got, want)
		case got < want:
			t.Errorf("%s holds %d comment style faults and the table says %d.\n"+
				"  A repair landed and the table is now wrong.\n"+
				"  Set the number to %d, or delete the line when the number is zero.",
				rel, got, want, got)
		}
	}
	for rel := range commentStyleDebt {
		if !seen[rel] {
			t.Errorf("commentStyleDebt names %s and the scan did not reach it.\n"+
				"  Delete the line, or repair the path.", rel)
		}
	}
}

// The scanner must find each of the four faults, and it must find no
// fault in clean text.
//
// A scanner with a broken pattern reports zero everywhere. The table
// above would then match, and the gate would stay green forever while
// it guards nothing. This test holds each rule against a small example.
func TestCommentStyleScannerFindsEachRule(t *testing.T) {
	longSentence := "// " + strings.Repeat("word ", 26) + "end."
	edgeSentence := "// " + strings.Repeat("word ", 24) + "end."

	cases := []struct {
		name string
		src  string
		want styleCount
	}{
		{
			name: "clean text gives no fault",
			src:  "// The server reads the file one time.\n// A cache would go stale.\n",
		},
		{
			name: "a sentence of 27 words counts one long",
			src:  longSentence + "\n",
			want: styleCount{long: 1},
		},
		{
			name: "a sentence of 25 words counts nothing",
			src:  edgeSentence + "\n",
		},
		{
			name: "two long sentences in one paragraph count two",
			src:  longSentence + "\n" + longSentence + "\n",
			want: styleCount{long: 2},
		},
		{
			name: "a semicolon in prose counts one",
			src:  "// The file is small; a cache would cost more than the read.\n",
			want: styleCount{semicolon: 1},
		},
		{
			name: "a semicolon beside code text counts nothing",
			src:  "// Call flush(); the buffer then holds nothing.\n",
		},
		{
			name: "a word of the do-not-use list counts one",
			src:  "// The parser can leverage the table above.\n",
			want: styleCount{banned: 1},
		},
		{
			name: "two banned words in one paragraph still count one",
			src:  "// It is a robust and powerful parser.\n",
			want: styleCount{banned: 1},
		},
		{
			name: "a contraction counts one",
			src:  "// The parser doesn't read a nested value.\n",
			want: styleCount{contraction: 1},
		},
		{
			name: "the bookmark marker keeps its contraction",
			src:  "// The note holds the " + styleMarkerException + " line.\n",
		},
		{
			name: "an empty comment line splits a paragraph",
			src:  "// The parser can leverage the table.\n//\n// The reader can leverage it too.\n",
			want: styleCount{banned: 2},
		},
		{
			name: "a rule line splits a paragraph",
			src:  "// The parser can leverage the table.\n// ------------\n// And so can the reader leverage it.\n",
			want: styleCount{banned: 2},
		},
		{
			name: "a code line splits a paragraph",
			src:  "// The parser can leverage the table.\nx := 1\n// The reader can leverage it too.\n",
			want: styleCount{banned: 2},
		},
		{
			name: "a comment at the end of a code line is not read",
			src:  "x := 1 // the parser can leverage the table\n",
		},
		{
			name: "a list of short items counts no long sentence",
			src: "// The handler holds three faults:\n" +
				"//   - the force checkbox, which becomes an action,\n" +
				"//   - the default action, which the request can omit,\n" +
				"//   - the map from an error to a status word.\n",
		},
		{
			name: "a list item over the limit counts one",
			src: "// The handler holds one fault:\n" +
				"//   - " + strings.Repeat("word ", 26) + "end.\n",
			want: styleCount{long: 1},
		},
		{
			name: "a numbered list splits the same way",
			src: "// Two steps:\n" +
				"// 1. read the file and keep each line that holds a colon,\n" +
				"// 2. write the lines back in the order that they arrived.\n",
		},
		{
			name: "the text above a list is its own unit",
			src: "// " + strings.Repeat("word ", 26) + "end.\n" +
				"//   - a short item.\n",
			want: styleCount{long: 1},
		},
		{
			name: "a dash inside a line starts no unit",
			src:  "// The count - and the limit - stay in one sentence here.\n",
		},
		{
			name: "a banned word in a list still counts one for the paragraph",
			src: "// Two notes:\n" +
				"//   - the parser can leverage the table,\n" +
				"//   - the reader can leverage it too.\n",
			want: styleCount{banned: 1},
		},
		{
			name: "two lines of one paragraph make one sentence",
			src:  "// " + strings.Repeat("word ", 14) + "\n// " + strings.Repeat("word ", 13) + "end.\n",
			want: styleCount{long: 1},
		},
	}

	for _, c := range cases {
		got := countCommentStyle(c.src)
		if got != c.want {
			t.Errorf("%s: the scanner answered %+v and the case wants %+v", c.name, got, c.want)
		}
	}
}

// EVERY word of the "do not use" list must fire the rule.
//
// The case list above names four of the ten words. A probe removed one
// of the other six from the pattern, and each test above stayed green.
// A pattern is a list, and a test of a list holds each entry of it.
//
// The near misses matter as much. One word of the list sits inside the
// word "adjust". A pattern without the word boundary would report a
// fault on each comment that says "adjust".
//
// This comment cannot write the banned words themselves. The gate reads
// this file, and it caught the first draft of this paragraph.
func TestCommentStyleHoldsEachBannedWord(t *testing.T) {
	// Section 10 of CLAUDE.md holds the list. Keep the two the same.
	banned := []string{
		"seamless", "robust", "powerful", "leverage", "ensure",
		"delve", "streamline", "simply", "just", "in order to",
	}
	for _, w := range banned {
		src := "// The reader can " + w + " the table.\n"
		if got := countCommentStyle(src).banned; got != 1 {
			t.Errorf("the word %q gave %d and the rule wants 1. "+
				"styleBannedRe lost an entry of CLAUDE.md section 10.", w, got)
		}
		src = "// The reader can " + strings.ToUpper(w) + " the table.\n"
		if got := countCommentStyle(src).banned; got != 1 {
			t.Errorf("the word %q in capitals gave %d and the rule wants 1", w, got)
		}
	}
	// The inflected forms that the pattern carries.
	for _, w := range []string{"ensures", "ensured", "streamlines", "streamlined"} {
		if got := countCommentStyle("// It " + w + " the read.\n").banned; got != 1 {
			t.Errorf("the form %q gave %d and the rule wants 1", w, got)
		}
	}
	// A word that holds a banned word is not a banned word.
	for _, w := range []string{"adjust", "adjusted", "insure", "robustness", "empowerment"} {
		if got := countCommentStyle("// The caller can " + w + " the value.\n").banned; got != 0 {
			t.Errorf("the word %q gave %d and the rule wants 0. "+
				"styleBannedRe lost a word boundary.", w, got)
		}
	}
}

// The table must name a real file, and the scan must reach the files
// that this project writes.
//
// A walk that reads nothing makes the gate green and empty. This test
// holds the floor: the scan reaches the Go tree, the JavaScript tree and
// the Java tree, and it steps over each vendored script.
func TestCommentStyleScanReachesEachTree(t *testing.T) {
	files := commentStyleFiles(t)
	if len(files) < 60 {
		t.Fatalf("the scan reached %d files. The project holds more than that.", len(files))
	}
	want := []string{
		"main_desktop.go",
		"backend/handlers.go",
		"backend/handlers_test.go",
		"backend/frontend/html/js/OMN-Go/omn-go-core.js",
		"backend/frontend/test/header.test.js",
		"android/app/src/main/java/net/basov/omngo/MainActivity.java",
		"android/test/java/net/basov/omngo/OmnConfigTest.java",
	}
	have := make(map[string]bool, len(files))
	for _, f := range files {
		have[f] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("the scan did not reach %s", w)
		}
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".min.js") {
			t.Errorf("the scan reached the vendored script %s", f)
		}
	}
}
