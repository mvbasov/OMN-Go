package backend

// ----------------------------------------------------------------------
// The global search index
// ----------------------------------------------------------------------
//
// What this holds, and - more importantly - what it does NOT.
//
// It does not hold your notes. For every indexed file it keeps the metadata a
// result needs (path, title, tags), one 64-bit character mask per line, and a
// 512-bit trigram signature. Text is read back from disk for the handful of
// documents a query actually reaches.
//
// That is the whole design decision, and it came from measuring the shape this
// plan originally had against the real target of ~10 000 notes / 25 MB:
//
//	raw + folded text + mask per line ... 2.35x corpus
//	folded text + mask per line ........  1.90x corpus
//	masks only, text read on demand ....  0.53x corpus   <- this
//
// A per-document token dictionary was dropped for the same reason. It
// scaled with VOCABULARY rather than with text, at about 44k unique tokens
// for each MB. That is a million-odd map entries at the target size. It
// existed only to serve the typo rung.
//
// The trigram signature does that job in 64 bytes for each document,
// whatever the size of the document. It narrows to the few documents worth
// tokenising at query time. See scoreTypoInDocument.
//
// The cost that moved rather than vanished is disk reads. A query now reads
// the candidate documents, and it does not consult a copy in RAM. On a
// note-sized file that is small, and after the first query the OS page
// cache serves it. The masks below exist precisely to keep the candidate
// set small.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// indexStaleCheckEvery bounds how often a query pays for the stat walk.
	// At 10 000 files that walk measured 24 ms. That is far too much to
	// repeat on every keystroke of a debounced search box. An in-process
	// write bypasses the wait entirely, see markSearchIndexDirty. This thus
	// delays one thing only. That is to notice an edit made behind the back
	// of the server, by an external editor or by a git checkout.
	indexStaleCheckEvery = 2 * time.Second

	// maxIndexBytes caps the total volume of TEXT indexed, not resident
	// bytes: resident is a predictable ~0.5x of it. Roughly 2.5x the stated
	// growth target, so hitting it means something unexpected is in the
	// notes directory.
	maxIndexBytes = 64 << 20
)

// indexedDoc is one document as the index remembers it. It holds enough to
// decide whether a query could match, and enough to describe a result. It
// does not hold the text itself.
type indexedDoc struct {
	Path      string // storage-relative, slash form
	Kind      string
	Name      string
	Title     string
	Tags      []string
	URL       string
	Bundled   bool
	ModTime   time.Time
	Size      int64
	Truncated bool

	// FieldMask covers the title, the tags, the path and the header.
	// LineMasks has one entry for each non-blank content line. They are kept
	// apart because a term may match a field OR a line, and the two
	// questions are asked separately.
	FieldMask uint64
	LineMasks []uint64

	// Tri is the 512-bit trigram signature of the whole document, used only
	// by the typo rung - the one rung a character mask cannot filter for.
	Tri [8]uint64
}

// couldMatchTerm reports whether this document could possibly satisfy one
// term, without reading a byte of it.
//
// False here means "definitely not", and that is what keeps the candidate
// set small. True means "worth reading". The mask test cannot produce a
// false negative for the substring rung or the subsequence rung, because
// both need every rune present. The trigram test covers the typo rung,
// which by definition matches text that misses some runes of the term.
func (d *indexedDoc) couldMatchTerm(t queryTerm) bool {
	if !maskRejects(t.mask, d.FieldMask) {
		return true
	}
	for _, m := range d.LineMasks {
		if !maskRejects(t.mask, m) {
			return true
		}
	}
	return d.couldMatchTypo(t.runes)
}

// couldMatchTypo applies the trigram bound. A term of length L matched
// within k edits still shares at least L-2-3k of its own trigrams with the
// text.
func (d *indexedDoc) couldMatchTypo(term []rune) bool {
	k := typoBudget(len(term))
	if k == 0 {
		return false
	}
	tris := trigrams(term)
	need := len(term) - 2 - 3*k
	if need <= 0 {
		return true // too short for the bound to say anything useful
	}
	found := 0
	for _, h := range tris {
		if d.Tri[(h>>6)&7]&(1<<(h&63)) != 0 {
			found++
			if found >= need {
				return true
			}
		}
	}
	return false
}

// trigrams hashes every 3-rune window of s. It is deliberately a cheap
// rolling hash, and not a cryptographic one. A collision costs one wasted
// document read, and the signature is only ever used to REJECT, never to
// confirm.
func trigrams(s []rune) []uint32 {
	if len(s) < 3 {
		return nil
	}
	out := make([]uint32, 0, len(s)-2)
	for i := 0; i+3 <= len(s); i++ {
		out = append(out, uint32(s[i])*961+uint32(s[i+1])*31+uint32(s[i+2]))
	}
	return out
}

// indexStamp is the cheap fingerprint of the corpus on disk. It holds how
// many files there are, the newest modification time among them AND among
// their directories, and their total size.
//
// Directory times are included on purpose. To add, delete or rename a file
// bumps the mtime of its directory, and not necessarily the mtime of any
// file that survives. A file-only scan would miss exactly those changes.
//
// The size sum is here because mtime alone is not trustworthy. Several
// filesystems round a timestamp to the second, and the external media of
// Android is one of them. Every note lives there. An edit made within a
// second of the previous one thus leaves the newest mtime unchanged.
//
// Size moves for almost any real edit, and it costs nothing to collect.
// The walk already calls stat on every file. It still cannot see an
// external edit that keeps the byte count identical within the same second.
// That waits for the next change. An edit made THROUGH the app relies on
// none of this, see ensureSearchIndex.
type indexStamp struct {
	files  int
	newest time.Time
	bytes  int64
}

type searchIndex struct {
	mu      sync.RWMutex
	docs    map[string]*indexedDoc
	stamp   indexStamp
	checked time.Time // when stamp was last verified against disk
	built   time.Time // when the current contents were assembled
	dirty   bool      // an in-process write happened; re-check without waiting
	lines   int
	bytes   int64
	kinds   string // the SearchKinds the current contents were built for
	bundled bool   // ... and the SearchBundled setting
	common  map[string]bool
}

// ----------------------------------------------------------------------
// The common words of this collection
// ----------------------------------------------------------------------
//
// A query of five words where two are "the" gave a panel of rows that
// said nothing. 26.09.39 measured it: 33 of 50 rows carried no content
// word, and the panel drew 1360 marks. See
// claude/s1-search-panel-report-2026-09-06.md.
//
// cutSnippets steps over a term of ONE rune. That rule cannot grow by
// rune count, because "cat", "dog", "git" and "log" are three runes and
// each one is a real query. The measure has to be how common the word
// is, and not how long it is.
//
// commonWordShare is the share of the collection above which a word
// carries little. It is a half.
//
// THE SET IS TINY. This corpus of 66 documents holds 4536 distinct
// words and 22 of them are above the half. The index therefore keeps
// the words above the line and no others, which costs a map of tens of
// entries and not one of thousands.
//
// A COUNT OF EVERY WORD RUNS DURING THE REBUILD, and that map is large
// for the length of the rebuild alone. The rebuild already reads each
// file and walks each rune of it for the trigram signature. See
// indexFile.
//
// THE SET IS RELATIVE TO THE COLLECTION, and that is the point. A
// person who writes only about one subject pushes the words of that
// subject above the line. cutSnippets then keeps no row and answers the
// window, which is the behavior of today. The worst case of this rule
// is the state before it.
const commonWordShare = 2 // one part in two, thus above a half

// indexWords adds each word of one folded string to a set.
//
// It is not tokenize. tokenize drops a word below minTokenLen, and
// "the", "and", "of" and "to" are exactly the words this set needs.
func indexWords(rs []rune, into map[string]bool) {
	var w []rune
	flush := func() {
		if len(w) > 0 {
			into[string(w)] = true
			w = w[:0]
		}
	}
	for _, r := range rs {
		if isWordRune(r) {
			w = append(w, r)
			continue
		}
		flush()
	}
	flush()
}

// commonWords answers the set of words that carry little in this
// collection. It answers nil when no index is built, and cutSnippets
// then behaves as it did before 26.09.40.
func (a *App) commonWords() map[string]bool {
	if a.search == nil {
		return nil
	}
	a.search.mu.RLock()
	defer a.search.mu.RUnlock()
	return a.search.common
}

// ----------------------------------------------------------------------
// Lifecycle
// ----------------------------------------------------------------------

// markSearchIndexDirty is called from renderAndCache. That is the one
// writer of compiled pages, and thus the one place that every in-process
// note change passes through. Those are a save, a quick note, a bookmark, a
// new page, a sync and a precompile. One counter here means fewer places to
// forget than a hook in each handler.
func (a *App) markSearchIndexDirty() {
	if a.search == nil {
		return
	}
	a.search.mu.Lock()
	a.search.dirty = true
	a.search.mu.Unlock()
}

// dropSearchIndex releases the index and its memory. It is called when
// global search is switched off. That is exactly what someone does when a
// device is short of memory. It must thus take effect at once, and not at
// the next restart.
func (a *App) dropSearchIndex() {
	if a.search == nil {
		return
	}
	a.search.mu.Lock()
	had := len(a.search.docs)
	a.search.docs = nil
	a.search.common = nil
	a.search.lines = 0
	a.search.bytes = 0
	a.search.built = time.Time{}
	a.search.mu.Unlock()
	if had > 0 {
		a.logInfof(logSearch, "index dropped (%d documents released)", had)
	}
}

// searchIndexBuilt reports whether there is an index to answer from. This is
// the seam globalSearchAvailable() consults, so the dialog is never offered a
// scope this server cannot serve.
func (a *App) searchIndexBuilt() bool {
	if a.search == nil {
		return false
	}
	a.search.mu.RLock()
	defer a.search.mu.RUnlock()
	return a.search.docs != nil
}

// searchIndexStatus is the line the Config page shows. A memory-sensitive
// option that will not say how much memory it is using is a setting people
// switch off blindly.
func (a *App) searchIndexStatus() string {
	cfg := a.GetConfig()
	if !cfg.SearchEnabled {
		return "Off - page search still works, and costs nothing."
	}
	if a.search == nil {
		return "Not built yet."
	}
	a.search.mu.RLock()
	defer a.search.mu.RUnlock()
	if a.search.docs == nil {
		return "Not built yet."
	}
	// ~0.5x of indexed text, the measured ratio for this doc model.
	resident := float64(a.search.bytes) / (1 << 20) * 0.53
	return fmtIndexStatus(len(a.search.docs), a.search.lines,
		float64(a.search.bytes)/(1<<20), resident, a.search.built)
}

func fmtIndexStatus(docs, lines int, mb, residentMB float64, built time.Time) string {
	var b strings.Builder
	b.WriteString(plural(docs, "document", "documents"))
	b.WriteString(", ")
	b.WriteString(plural(lines, "line", "lines"))
	b.WriteString(", ")
	b.WriteString(oneDecimal(mb))
	b.WriteString(" MB indexed, about ")
	b.WriteString(oneDecimal(residentMB))
	b.WriteString(" MB in memory")
	if !built.IsZero() {
		b.WriteString(", built ")
		b.WriteString(built.Format("15:04:05"))
	}
	b.WriteString(".")
	return b.String()
}

// ----------------------------------------------------------------------
// Building
// ----------------------------------------------------------------------

// searchRoot describes where one kind lives.
type searchRoot struct {
	kind string
	dir  string // absolute
	exts []string
}

func (a *App) searchRoots(kinds []string) []searchRoot {
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	var roots []searchRoot
	if want[SearchKindMD] || want[SearchKindBookmarks] {
		roots = append(roots, searchRoot{
			kind: SearchKindMD,
			dir:  filepath.Join(a.StorageDir, "md"),
			exts: []string{".md"},
		})
	}
	if want[SearchKindJS] {
		roots = append(roots, searchRoot{
			kind: SearchKindJS,
			dir:  filepath.Join(a.StorageDir, "html", "js"),
			exts: []string{".js"},
		})
	}
	if want[SearchKindJSON] {
		roots = append(roots, searchRoot{
			kind: SearchKindJSON,
			dir:  filepath.Join(a.StorageDir, "html", "json"),
			exts: []string{".json"},
		})
	}
	if want[SearchKindUserJSON] {
		roots = append(roots, searchRoot{
			kind: SearchKindUserJSON,
			dir:  filepath.Join(a.StorageDir, "html", "user_json"),
			exts: []string{".json", ".jsonl"},
		})
	}
	return roots
}

// rebuildSearchIndex walks the enabled roots and replaces the index contents.
//
// The new map is assembled first and swapped in under the write lock. A
// query that runs concurrently thus sees either the whole old index or the
// whole new one. It never sees a half-built one.
func (a *App) rebuildSearchIndex() {
	cfg := a.GetConfig()
	if !cfg.SearchEnabled {
		a.dropSearchIndex()
		return
	}
	if a.search == nil {
		return
	}

	started := time.Now()
	kinds := normalizeSearchKinds(cfg.SearchKinds)
	wantBookmarks := false
	wantMD := false
	for _, k := range kinds {
		switch k {
		case SearchKindBookmarks:
			wantBookmarks = true
		case SearchKindMD:
			wantMD = true
		}
	}

	docs := map[string]*indexedDoc{}
	docFreq := map[string]int{}
	var lines int
	var bytes int64
	capped := false

	for _, root := range a.searchRoots(kinds) {
		_ = filepath.WalkDir(root.dir, func(p string, e fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if e.IsDir() {
				// md/local is the gitignored scratch tree. To skip it here
				// matches buildTagIndex, which excludes it for the same
				// reason.
				if root.kind == SearchKindMD && p == filepath.Join(root.dir, "local") {
					return fs.SkipDir
				}
				return nil
			}
			if !hasExt(e.Name(), root.exts) {
				return nil
			}
			rel, err := filepath.Rel(root.dir, p)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)

			kind := root.kind
			if root.kind == SearchKindMD {
				name := strings.TrimSuffix(rel, ".md")
				switch name {
				case "OMNGoTags":
					return nil // generated FROM the notes; indexing it duplicates them
				case "Bookmarks":
					if !wantBookmarks {
						return nil
					}
					kind = SearchKindBookmarks
				default:
					if !wantMD {
						return nil
					}
				}
			}

			bundled := isBundledAsset(root.kind, rel)
			if bundled && !cfg.SearchBundled {
				return nil
			}
			if bytes >= maxIndexBytes {
				capped = true
				return nil
			}

			info, err := e.Info()
			if err != nil {
				return nil
			}
			doc := a.indexFile(root.kind, kind, rel, p, info, bundled, docFreq)
			if doc == nil {
				return nil
			}
			docs[doc.Path] = doc
			lines += len(doc.LineMasks)
			bytes += doc.Size
			return nil
		})
	}

	// Keep the words above the line and drop the rest. See the banner of
	// commonWordShare above.
	common := map[string]bool{}
	for w, n := range docFreq {
		if n*commonWordShare > len(docs) {
			common[w] = true
		}
	}

	stamp := a.searchStamp(kinds)

	a.search.mu.Lock()
	a.search.docs = docs
	a.search.common = common
	a.search.lines = lines
	a.search.bytes = bytes
	a.search.stamp = stamp
	a.search.checked = time.Now()
	a.search.built = time.Now()
	a.search.dirty = false
	a.search.kinds = strings.Join(kinds, ",")
	a.search.bundled = cfg.SearchBundled
	a.search.mu.Unlock()

	if capped {
		a.logErrf(logSearch, "index capped at %d MB of text; some files were left out", maxIndexBytes>>20)
	}
	a.logInfof(logSearch, "Indexed %d files (%d lines, %.1f MB) in %s",
		len(docs), lines, float64(bytes)/(1<<20), time.Since(started).Round(time.Millisecond))
}

// indexFile reads one file and reduces it to what the index keeps.
//
// docFreq counts the DOCUMENTS that hold each word, and not the times
// that a word appears. Each word of this file therefore adds one, and a
// word that appears twenty times still adds one.
func (a *App) indexFile(rootKind, kind, rel, path string, info fs.FileInfo, bundled bool, docFreq map[string]int) *indexedDoc {
	data, truncated, err := readCapped(path, maxIndexFileBytes)
	if err != nil || isBinary(data) {
		return nil
	}

	var doc *searchDocument
	if rootKind == SearchKindMD {
		doc = newMarkdownDocument(strings.TrimSuffix(rel, ".md"), string(data), truncated)
		doc.Kind = kind // Bookmarks.md is its own kind
	} else {
		doc = newAssetDocument(rootKind+"/"+rel, string(data), truncated)
	}

	out := &indexedDoc{
		Path:      doc.Path,
		Kind:      doc.Kind,
		Name:      doc.Name,
		Title:     doc.Title,
		Tags:      doc.Tags,
		URL:       doc.URL,
		Bundled:   bundled,
		ModTime:   info.ModTime(),
		Size:      int64(len(data)),
		Truncated: truncated,
		LineMasks: make([]uint64, 0, len(doc.lines)),
	}
	for _, f := range doc.fields {
		out.FieldMask |= runeMask(f.text)
		addTrigrams(&out.Tri, f.text)
	}
	for i := range doc.lines {
		out.LineMasks = append(out.LineMasks, doc.lines[i].mask)
		addTrigrams(&out.Tri, doc.lines[i].fold)
	}

	if docFreq != nil {
		words := map[string]bool{}
		for _, f := range doc.fields {
			indexWords(f.text, words)
		}
		for i := range doc.lines {
			indexWords(doc.lines[i].fold, words)
		}
		for w := range words {
			docFreq[w]++
		}
	}
	return out
}

func addTrigrams(sig *[8]uint64, s []rune) {
	for _, h := range trigrams(s) {
		sig[(h>>6)&7] |= 1 << (h & 63)
	}
}

// isBundledAsset reports whether a file is one that OMN-Go ships itself. It
// is derived from versionDependentAssets in assets.go. That list already
// decides what an upgrade refreshes. A new bundled file is thus excluded
// automatically, and no second list has to be kept in step by hand.
func isBundledAsset(rootKind, rel string) bool {
	if rootKind != SearchKindJS && rootKind != SearchKindJSON {
		return false
	}
	full := "html/" + rootKind + "/" + rel
	for _, v := range versionDependentAssets {
		if v == full {
			return true
		}
	}
	base := filepath.Base(rel)
	return strings.Contains(base, ".min.")
}

func hasExt(name string, exts []string) bool {
	lower := strings.ToLower(name)
	for _, e := range exts {
		if strings.HasSuffix(lower, e) {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------
// Staleness
// ----------------------------------------------------------------------

// searchStamp is the stat-only walk: no file is opened and nothing is parsed.
func (a *App) searchStamp(kinds []string) indexStamp {
	var st indexStamp
	consider := func(e fs.DirEntry, countSize bool) {
		info, err := e.Info()
		if err != nil {
			return
		}
		if info.ModTime().After(st.newest) {
			st.newest = info.ModTime()
		}
		if countSize {
			st.bytes += info.Size()
		}
	}
	for _, root := range a.searchRoots(kinds) {
		_ = filepath.WalkDir(root.dir, func(p string, e fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if e.IsDir() {
				if root.kind == SearchKindMD && p == filepath.Join(root.dir, "local") {
					return fs.SkipDir
				}
				consider(e, false) // a rename shows up here and nowhere else
				return nil
			}
			if !hasExt(e.Name(), root.exts) {
				return nil
			}
			st.files++
			consider(e, true)
			return nil
		})
	}
	return st
}

// ensureSearchIndex is what every global query calls first. It builds the
// index on demand, and re-checks the corpus at most once per
// indexStaleCheckEvery unless an in-process write already marked it dirty.
func (a *App) ensureSearchIndex() bool {
	cfg := a.GetConfig()
	if !cfg.SearchEnabled || a.search == nil {
		return false
	}
	kinds := strings.Join(normalizeSearchKinds(cfg.SearchKinds), ",")

	a.search.mu.RLock()
	built := a.search.docs != nil
	checked := a.search.checked
	dirty := a.search.dirty
	stamp := a.search.stamp
	sameSettings := a.search.kinds == kinds && a.search.bundled == cfg.SearchBundled
	a.search.mu.RUnlock()

	// A settings change is not a staleness question - what to cover changed,
	// so what is covered has to be rebuilt regardless of file times.
	if !built || !sameSettings {
		a.rebuildSearchIndex()
		return a.searchIndexBuilt()
	}
	// An in-process write means the CONTENT changed. We watched it happen.
	// A second stat call to confirm it is redundant, and it is also
	// unreliable. A file timestamp has coarse granularity on some
	// filesystems, and the external media of Android is one of them. An edit
	// and the write before it can thus share an mtime to the second. The
	// stamp below would then report "nothing changed" about a file that we
	// know we rewrote.
	if dirty {
		a.rebuildSearchIndex()
		return a.searchIndexBuilt()
	}
	if time.Since(checked) < indexStaleCheckEvery {
		return true
	}

	fresh := a.searchStamp(normalizeSearchKinds(cfg.SearchKinds))
	if fresh == stamp {
		a.search.mu.Lock()
		a.search.checked = time.Now()
		a.search.dirty = false
		a.search.mu.Unlock()
		return true
	}
	a.rebuildSearchIndex()
	return a.searchIndexBuilt()
}

// snapshotDocs returns the current documents under a read lock, so a query can
// iterate without holding the lock across file reads.
func (a *App) snapshotDocs() []*indexedDoc {
	if a.search == nil {
		return nil
	}
	a.search.mu.RLock()
	defer a.search.mu.RUnlock()
	out := make([]*indexedDoc, 0, len(a.search.docs))
	for _, d := range a.search.docs {
		out = append(out, d)
	}
	return out
}

// storagePath maps a document's storage-relative path back to disk.
func (a *App) storagePath(rel string) string {
	return filepath.Join(a.StorageDir, filepath.FromSlash(rel))
}

// reloadDocument rebuilds the full searchable form of an indexed document, by
// reading it. Returns nil when the file has gone since the index was built -
// a query does not fail because a note was deleted underneath it.
func (a *App) reloadDocument(d *indexedDoc) *searchDocument {
	data, truncated, err := readCapped(a.storagePath(d.Path), maxIndexFileBytes)
	if err != nil {
		if !os.IsNotExist(err) {
			a.logErrf(logSearch, "%s: %v", d.Path, err)
		}
		return nil
	}
	if isBinary(data) {
		return nil
	}
	if strings.HasPrefix(d.Path, "md/") {
		doc := newMarkdownDocument(strings.TrimSuffix(strings.TrimPrefix(d.Path, "md/"), ".md"),
			string(data), truncated)
		doc.Kind = d.Kind
		return doc
	}
	return newAssetDocument(strings.TrimPrefix(d.Path, "html/"), string(data), truncated)
}

// ----------------------------------------------------------------------
// Small formatting helpers (no fmt: this file is on the startup path)
// ----------------------------------------------------------------------

func plural(n int, one, many string) string {
	word := many
	if n == 1 {
		word = one
	}
	return itoa(n) + " " + word
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [24]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func oneDecimal(f float64) string {
	if f < 0 {
		return "0.0"
	}
	whole := int(f)
	frac := int((f-float64(whole))*10 + 0.5)
	if frac >= 10 {
		whole++
		frac = 0
	}
	return itoa(whole) + "." + itoa(frac)
}
