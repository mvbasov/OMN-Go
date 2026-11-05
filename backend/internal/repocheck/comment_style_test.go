package repocheck

// ----------------------------------------------------------------------
// The source style gate
// ----------------------------------------------------------------------
//
// Section 10 of CLAUDE.md applies Simplified Technical English to every
// word that a person reads. A code comment is such a word. Section 11
// rule 7 asks the writer to check the text before the patch arrives.
//
// A check by eye fails. A long sentence and a word from the "do not use"
// list are both hard to see and easy to count. This file counts them.
//
// It also holds the gofmt check. See TestEveryGoFileIsGofmtClean below.
// The check for a version number in a comment is at the end of the file.
//
// THE RULE IS ZERO. A file that holds one comment style fault fails.
//
// WHAT IT READS.
//
// It reads a WHOLE LINE comment in a Go file, a JavaScript file and a
// Java file. A comment at the end of a code line is short by nature, and
// the scan steps over it. In a CSS file it reads each block comment that
// starts a line. See styleSource.
//
// It skips a file with the .min.js or the .min.css suffix. A vendored file
// is not ours to write, and katex.min.js alone is 277 kilobytes.
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
// change to it would break each stored note. See note_handlers.go and
// storage.go. The scanner removes that exact string before it looks for
// a contraction.
//
// THIS FILE READS ITSELF. It must hold no fault, the same as every
// other file. A rule that the author breaks is not a rule.

import (
	"go/format"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

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
// period. A scan that joins the whole list finds no period, and it
// reports one long sentence.
//
// That is a false report. The rule of CLAUDE.md section 10 is about one
// idea in one sentence, and each item of a list is one idea. A writer
// who obeys the false report writes worse text, not better.
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
		if strings.HasSuffix(name, ".min.js") || strings.HasSuffix(name, ".min.css") {
			return
		}
		switch filepath.Ext(name) {
		case ".go", ".js", ".java", ".css":
			out = append(out, filepath.ToSlash(rel))
		}
	}

	roots, err := filepath.Glob(filepath.Join(repoRoot, "*.go"))
	if err != nil {
		t.Fatalf("the glob of the repository root failed: %v", err)
	}
	for _, p := range roots {
		take(filepath.Base(p))
	}

	for _, root := range styleScanRoots {
		base := filepath.Join(repoRoot, root)
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
			rel, relErr := filepath.Rel(repoRoot, p)
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

// cssBlockRe finds a block comment of a CSS file.
var cssBlockRe = regexp.MustCompile(`(?s)/\*(.*?)\*/`)

// styleSource answers the text of one source file for the comment scans.
//
// A CSS file has block comments only. styleSource writes each block
// comment that starts a line as whole line comments, and it puts a code
// line after each block. Thus two blocks never join into one paragraph.
// A comment after a declaration is short by nature, and
// the scan steps over it, the same as in a Go file.
func styleSource(rel string) (string, error) {
	src, err := readRepoFile(rel)
	if err != nil || filepath.Ext(rel) != ".css" {
		return src, err
	}
	return cssLineComments(src), nil
}

// cssLineComments writes each block comment of src that starts a line as
// whole line comments. It drops each other line.
func cssLineComments(src string) string {
	var b strings.Builder
	for _, m := range cssBlockRe.FindAllStringSubmatchIndex(src, -1) {
		lineStart := strings.LastIndexByte(src[:m[0]], '\n') + 1
		if strings.TrimSpace(src[lineStart:m[0]]) != "" {
			continue
		}
		for _, line := range strings.Split(src[m[2]:m[3]], "\n") {
			line = strings.TrimSpace(line)
			line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
			b.WriteString("// " + line + "\n")
		}
		b.WriteString("x\n")
	}
	return b.String()
}

// No file may hold a comment style fault.
//
// The failure names the file and the count. Repair the comment. The
// header above says what this scan reads as one paragraph, and section
// 10 of CLAUDE.md holds the four rules.
func TestNoCommentStyleFault(t *testing.T) {
	read := 0
	for _, rel := range commentStyleFiles(t) {
		src, err := styleSource(rel)
		if err != nil {
			t.Fatalf("cannot read %s: %v", rel, err)
		}
		read++
		if got := countCommentStyle(src).total(); got > 0 {
			t.Errorf("%s holds %d comment style faults.\n"+
				"  A comment breaks a rule of CLAUDE.md section 10.\n"+
				"  Repair the comment.", rel, got)
		}
	}
	if read == 0 {
		t.Fatal("no file was read, thus this test proves nothing")
	}
}

// Each Go file must be gofmt clean.
//
// go vet does not read the formatting of a file. A key deleted from a map
// literal, for example, leaves the column padding of the other keys
// behind, and go vet says nothing.
//
// It uses go/format and not the gofmt command. format.Source gives the
// canonical gofmt style, thus this test needs no binary on the path and
// it can never skip.
func TestEveryGoFileIsGofmtClean(t *testing.T) {
	read := 0
	for _, rel := range commentStyleFiles(t) {
		if filepath.Ext(rel) != ".go" {
			continue
		}
		src, err := readRepoFile(rel)
		if err != nil {
			t.Fatalf("cannot read %s: %v", rel, err)
		}
		want, fErr := format.Source([]byte(src))
		if fErr != nil {
			t.Errorf("%s does not parse: %v", rel, fErr)
			continue
		}
		read++
		if string(want) != src {
			t.Errorf("%s is not gofmt clean. Run gofmt -w on it.", rel)
		}
	}
	if read == 0 {
		t.Fatal("no Go file was read, thus this test proves nothing")
	}
}

// The scanner must find each of the four faults, and it must find no
// fault in clean text.
//
// A scanner with a broken pattern reports zero everywhere. The gate
// would then stay green forever while it guards nothing. This test holds
// each rule against a small example.
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
// The case list above names four of the ten words. Without this test, a
// word could leave the pattern and each test above would stay green. A
// pattern is a list, and a test of a list holds each entry of it.
//
// The near misses matter as much. One word of the list sits inside the
// word "adjust". A pattern without the word boundary would report a
// fault on each comment that says "adjust".
//
// This comment cannot write the banned words themselves, because the
// gate reads this file.
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
		"backend/internal/app/handlers.go",
		"backend/internal/app/handlers_test.go",
		"backend/frontend/html/js/OMN-Go/omn-go-core.js",
		"backend/frontend/html/css/OMN-Go/omn-go-core.css",
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
		if strings.HasSuffix(f, ".min.js") || strings.HasSuffix(f, ".min.css") {
			t.Errorf("the scan reached the vendored file %s", f)
		}
	}
}

// ----------------------------------------------------------------------
// No version number in a comment
// ----------------------------------------------------------------------
//
// A comment says what the code does now and why. The git log holds the
// history of each change, and doc/decisions holds each decision that
// still controls the code. A version number in a comment is history.
//
// The scan counts each whole line comment that holds a version number of
// this project. TestCommentVersionScannerFindsEachForm shows each form.
// The scan uses the same files as the style scan above. In a JavaScript
// and a Java file it also reads each line of a block comment.
//
// THE RULE IS ZERO. A file that holds one such line fails.

// commentVersionRe finds a version of this project: two digits of the
// year, the month, and a sequence number of one to three digits. It does
// not find an address such as 127.0.0.1 or a version such as v5.13.1.
var commentVersionRe = regexp.MustCompile(`\bv?2[0-9]\.(0[1-9]|1[0-2])\.[0-9]{1,3}[a-z]?\b`)

// commentBlockLineRe is a line of a block comment: the line that opens it,
// or a line that starts with a star. The scan reads it in a JavaScript and
// a Java file alone. A Go line that starts with a star is a pointer.
var commentBlockLineRe = regexp.MustCompile(`^\s*(?:/\*+|\*)\s?(.*)$`)

// countCommentVersions counts the whole line comments of src that hold a
// version number. blocks adds the lines of block comments.
func countCommentVersions(src string, blocks bool) int {
	n := 0
	for _, line := range strings.Split(src, "\n") {
		m := styleCommentLineRe.FindStringSubmatch(line)
		if m == nil && blocks {
			m = commentBlockLineRe.FindStringSubmatch(line)
		}
		if m != nil && commentVersionRe.MatchString(m[1]) {
			n++
		}
	}
	return n
}

// No comment may hold a version number of this project.
func TestNoVersionNumberInComments(t *testing.T) {
	read := 0
	for _, rel := range commentStyleFiles(t) {
		src, err := styleSource(rel)
		if err != nil {
			t.Fatalf("cannot read %s: %v", rel, err)
		}
		read++
		if got := countCommentVersions(src, filepath.Ext(rel) != ".go"); got > 0 {
			t.Errorf("%s holds %d comment lines with a version number.\n"+
				"  A comment tells no history. Remove the version number.\n"+
				"  See doc/decisions/README.md.", rel, got)
		}
	}
	if read == 0 {
		t.Fatal("no file was read, thus this test proves nothing")
	}
}

// The version scanner must find each form of a version of this project,
// and it must step over each other number.
func TestCommentVersionScannerFindsEachForm(t *testing.T) {
	cases := []struct {
		src  string
		want int
	}{
		{"// The rule holds since 26.09.57.\n", 1},
		{"// The F-Droid block of v26.08.82f uses it.\n", 1},
		{"// Until 26.09.6 the value had no signature.\n", 1},
		{"// Two versions: 26.09.1 and 26.09.2.\n", 1},
		{"// Line one names 26.09.1.\n// Line two names 26.09.2.\n", 2},
		{"// The server binds 127.0.0.1 and 0.0.0.0.\n", 0},
		{"// The text of go-git v5.13.1 and Go 1.26.0.\n", 0},
		{"// The NDK is 25.2.9519653.\n", 0},
		{"// The date is 2026-09-26.\n", 0},
		{"// No month 13: 26.13.1.\n", 0},
		{"x := 1 // 26.09.1\n", 0},
		{"s := \"26.09.1\"\n", 0},
	}
	for _, c := range cases {
		if got := countCommentVersions(c.src, false); got != c.want {
			t.Errorf("%q gave %d and the case wants %d", c.src, got, c.want)
		}
	}

	// A block comment counts in a JavaScript or a Java file, and not in a
	// Go file. A Go line that starts with a star is a pointer.
	block := "/* The notice moved in 26.08.73.\n * It was inline until 26.08.73.\n */\n"
	if got := countCommentVersions(block, true); got != 2 {
		t.Errorf("a block comment gave %d with blocks on, and the case wants 2", got)
	}
	if got := countCommentVersions(block, false); got != 0 {
		t.Errorf("a block comment gave %d with blocks off, and the case wants 0", got)
	}
	if got := countCommentVersions("\t*p = 26.09\n", true); got != 0 {
		t.Errorf("a pointer line with no full version gave %d, want 0", got)
	}
}

// ----------------------------------------------------------------------
// The comment share of the Go production code
// ----------------------------------------------------------------------
//
// A comment keeps each fact that the code cannot show, and it says it one
// time. The share of whole line comments in the Go production files shows
// whether the comments grow faster than the code. The target is 200 per
// mille, which is 20 percent. Section 3 of CLAUDE.md gives the rules.
//
// THE CEILING IS THE TARGET. The test fails when the share is above
// commentShareCeiling. A new long comment thus needs a shorter one in
// another place. A ceiling above the target must follow the share: the test
// then also fails when the share is more than 10 per mille below it.

// commentShareCeiling is the highest share, in per mille, that the Go
// production files may hold.
const commentShareCeiling = 200

// commentShareTarget is the share, in per mille, that the code must not
// pass: 20 percent.
const commentShareTarget = 200

// goCommentShare answers the whole line comments and the lines of the Go
// production files: each Go file that is not a test.
func goCommentShare(t *testing.T) (comments, lines int) {
	t.Helper()
	for _, rel := range commentStyleFiles(t) {
		if filepath.Ext(rel) != ".go" || strings.HasSuffix(rel, "_test.go") {
			continue
		}
		src, err := readRepoFile(rel)
		if err != nil {
			t.Fatalf("cannot read %s: %v", rel, err)
		}
		for _, line := range strings.Split(src, "\n") {
			lines++
			if styleCommentLineRe.MatchString(line) {
				comments++
			}
		}
	}
	return comments, lines
}

// The Go production files may not hold more comments than the ceiling
// allows.
func TestCommentShare(t *testing.T) {
	comments, lines := goCommentShare(t)
	if lines == 0 {
		t.Fatal("no Go production line was read, thus this test proves nothing")
	}
	share := comments * 1000 / lines
	t.Logf("comment share: %d of %d lines, %d per mille", comments, lines, share)
	if share > commentShareCeiling {
		t.Errorf("the Go production files hold %d per mille of comments, and the ceiling is %d.\n"+
			"  Say each fact one time. See section 3 of CLAUDE.md.", share, commentShareCeiling)
	}
	if commentShareCeiling > commentShareTarget && share < commentShareCeiling-10 {
		t.Errorf("the share is %d per mille, and the ceiling is %d. Lower commentShareCeiling to %d.",
			share, commentShareCeiling, share+1)
	}
}

// The CSS reader must give the scans each block comment that starts a
// line, and no other text.
func TestCSSCommentReaderFindsEachBlock(t *testing.T) {
	src := ".a { color: red; } /* a comment after a declaration that is far too long for one sentence of the rule, and it goes on and on and on */\n" +
		"/* One block; with a semicolon. */\n" +
		".b { margin: 0; }\n" +
		"    /* A block inside a rule, from 26.09.61. */\n" +
		"/* Two lines\n   of one block. */\n"
	got := cssLineComments(src)
	want := "// One block; with a semicolon.\nx\n" +
		"// A block inside a rule, from 26.09.61.\nx\n" +
		"// Two lines\n// of one block.\nx\n"
	if got != want {
		t.Fatalf("cssLineComments gave\n%s\nwant\n%s", got, want)
	}
	if c := countCommentStyle(got); c.semicolon != 1 || c.long != 0 {
		t.Errorf("the scan of the CSS comments counted %+v, want one semicolon", c)
	}
	if n := countCommentVersions(got, false); n != 1 {
		t.Errorf("the version scan of the CSS comments counted %d, want 1", n)
	}
}
