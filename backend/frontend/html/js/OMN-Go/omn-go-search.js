// --- The search overlay ---
//
// The panel that the magnifier opens: the query box, the scope chips, the
// result rows, and the jump into a page with the words marked.
//
// THIS FILE ARRIVES ON DEMAND, and it is the largest of the three. It was
// part of omn-go-sse.js until 26.09.24, thus each note page parsed about
// 34 kilobytes of it to show nothing. The first press of the magnifier,
// of Ctrl-K, or of the slash key loads it. See omnLazy in omn-go-sse.js.
//
// THE SHORTCUTS ARE NOT HERE. They are in omn-go-sse.js, because a
// shortcut must answer before this file exists. This file exports
// omnSearchIsOpen and omnSearchClose for that listener to close the
// overlay with.
//
// THE HIGHLIGHTING IS NOT HERE EITHER. omn-go-core.js holds it, because
// arriving at a page with a hl query needs it on each page, an exported
// one included. This file is one of its callers.
if (window.location.protocol !== 'file:') {

    // --- Search overlay ---
    //
    // The everyday entry point to search: a spotlight-style panel over the
    // current page. It has two scopes and one control to pick between them:
    //
    //   - page: the open note only. There is no index and no configuration,
    //     and it is always available. This panel thus works on any device
    //     and in any state of the app, with nothing to switch on first.
    //   - all: every indexed note. It is offered only when the server says
    //     it can answer, through OMN_SEARCH_GLOBAL, thus the control never
    //     leads nowhere.
    //
    // It lives in this file rather than in a new asset, for two reasons.
    // This file is already inside the `protocol !== 'file:'` guard, thus an
    // exported page gets the stub version for free. It is also already in
    // versionDependentAssets and in gitignorePatterns, thus it needs no new
    // plumbing to ship.
    //
    // Everything the server returns is written with textContent, or into the
    // textContent of a <mark> element. Nothing from a response is ever
    // assigned to innerHTML. That is the same discipline that
    // OMNProgress.build documents in omn-go-core.js. It matters more here,
    // because the text being rendered is the own notes of the user.
    (function () {
        // THE SEARCH IS ASKED FOR, NOT GUESSED AT.
        //
        // Typing does not search. The magnifier button does, and so does
        // Enter. A query of all notes reads every note that the index holds.
        // A search for each keystroke did that work five or six times for one
        // word, and it kept only the last answer. A timer instead of a button
        // only moves the guess. Too short and it fires mid-word, too long and
        // the panel looks broken.
        //
        // The button is the same control the results page carries, so "type,
        // then press the magnifier" is one habit for both.
        var MIN_QUERY = 2;       // 1 rune matches nearly everything
        var MAX_SNIPPETS = 10;   // the API's own cap

        var overlay = null, input = null, list = null, statusEl = null, scopeEl = null;
        var progressEl = null, goEl = null;
        var seeAllEl = null;   // lives in the scope row - see renderScope
        var rows = [];
        var active = -1;
        // The query that the rows on screen belong to. Enter opens a row
        // while the field still says that. Once the field says something
        // else, Enter is a request to search for the new thing instead. See
        // onInputKey.
        var lastQuery = null;
        var inflight = null;
        var lastTerms = [];
        // It is "" until the user picks. The FIRST query deliberately sends
        // no scope, and it adopts whatever the server used. The server knows
        // the configured default, and whether global search can answer at
        // all. After that the choice is explicit, and sticky for the session.
        var scope = "";
        var scopeShown = "";

        // The page to search. index.html defines currentNote for every
        // rendered note; without it there is nothing to scope to.
        // The "on" parameter of a page-scope search. omnGoCurrentNoteName
        // (omn-go-core.js) gives the form that the server resolves with no
        // guess. See its banner: a bare name that ends in a real file
        // extension reads as a file and not as the note.
        function pageName() {
            if (typeof omnGoCurrentNoteName === 'function') {
                return omnGoCurrentNoteName();
            }
            return (typeof currentNote !== 'undefined' && currentNote) ? currentNote : '';
        }

        function build() {
            if (overlay || !document.body) return overlay;

            overlay = document.createElement('div');
            overlay.className = 'omn-search-overlay';
            overlay.hidden = true;
            // Static markup only - see the note at the top of this module.
            overlay.innerHTML =
                '<div class="omn-search-card" role="dialog" aria-label="Search">' +
                  '<div class="omn-search-head">' +
                    // Deliberately BARE. There is no spellcheck, no
                    // autocorrect, no autocapitalize, no autocomplete and no
                    // inputmode. Every one of those is a hint that the
                    // Android keyboard reads when it attaches.
                    //
                    // Several of them fold into the NO_SUGGESTIONS flag.
                    // spellcheck="false" and autocorrect="off" do so
                    // certainly, and autocomplete="off" does so in some
                    // WebView builds. That flag switches off the COMPOSING
                    // region, which is the mechanism that every non-Latin
                    // layout uses to enter text at all.
                    //
                    // None of them buys anything here. The field is not in a
                    // <form> and has no name, thus autofill never engages.
                    // Matching is case-folded, thus auto-capitalization is
                    // harmless. A red squiggle under a query is cosmetic. A
                    // search box has no reason to describe itself as anything
                    // other than a plain text field.
                    '<input type="text" class="omn-search-input" ' +
                          'placeholder="Search this page">' +
                    // The one control that searches. It stands where the
                    // submit button of the results page stands - after the
                    // field, same glyph - so the two behave alike. The
                    // decorative magnifier that used to lead this row is
                    // gone: two of them, one inert, said the wrong thing
                    // about which one to press.
                    '<button type="button" class="omn-search-go" ' +
                            'title="Search (↵)" aria-label="Search">' +
                      '<i class="material-icons icon-sm">search</i>' +
                    '</button>' +
                    '<button type="button" class="omn-search-close" aria-label="Close">' +
                      '<i class="material-icons icon-sm">close</i>' +
                    '</button>' +
                  '</div>' +
                  // The wait, on the line under the field that causes it. It
                  // reuses the shared .omn-progress-track and -fill look
                  // from omn-go-core.css. A wait is thus the same object
                  // here as in the sync overlay. Only the placement is local.
                  '<div class="omn-search-progress omn-progress-track" ' +
                       'role="progressbar" aria-label="Search progress" hidden>' +
                    '<div class="omn-progress-fill"></div>' +
                  '</div>' +
                  '<div class="omn-search-scope"></div>' +
                  '<ul class="omn-search-results"></ul>' +
                  '<div class="omn-search-status"></div>' +
                '</div>';
            document.body.appendChild(overlay);

            input = overlay.querySelector('.omn-search-input');
            list = overlay.querySelector('.omn-search-results');
            statusEl = overlay.querySelector('.omn-search-status');
            scopeEl = overlay.querySelector('.omn-search-scope');
            progressEl = overlay.querySelector('.omn-search-progress');
            goEl = overlay.querySelector('.omn-search-go');

            goEl.addEventListener('click', function () {
                // Back to the field afterwards. On a phone the tap on this
                // button closes the keyboard, and a reader usually edits the
                // query next.
                run();
                focusInput();
            });
            overlay.querySelector('.omn-search-close').addEventListener('click', close);
            // A click on the backdrop closes. A click inside the card must
            // not close.
            overlay.addEventListener('click', function (e) {
                if (e.target === overlay) close();
            });
            input.addEventListener('input', onInput);
            input.addEventListener('keydown', onInputKey);

            renderScope();
            return overlay;
        }

        // Whether the server can answer a scope=all query: the setting is on
        // AND there is an index to ask. Injected per request (see
        // injectRuntimeVars), so a page cached before the setting changed
        // still gets the current answer.
        function globalAvailable() {
            return typeof OMN_SEARCH_GLOBAL !== 'undefined' && OMN_SEARCH_GLOBAL;
        }

        // The scope row: one control, and only when there is a choice to make.
        // With global search off it stays what it was before - a statement of
        // where you are searching, not a switch that leads nowhere.
        function renderScope() {
            if (!scopeEl) return;
            scopeEl.textContent = '';
            var showing = scopeShown || (globalAvailable() ? 'all' : 'page');

            function chip(value, label) {
                var el = document.createElement('span');
                el.className = 'omn-search-chip' + (showing === value ? ' is-active' : '');
                el.textContent = label;
                if (globalAvailable()) {
                    el.setAttribute('role', 'button');
                    el.tabIndex = 0;
                    el.addEventListener('click', function () { setScope(value); });
                }
                scopeEl.appendChild(el);
            }

            if (globalAvailable()) chip('all', 'All notes');
            chip('page', 'This page');
            if (input) {
                input.placeholder = showing === 'all' ? 'Search all notes' : 'Search this page';
            }

            // Which page "this page" means. Shown only in page scope, where it
            // is the thing being searched.
            var name = pageName();
            if (showing === 'page' && name) {
                var where = document.createElement('span');
                where.className = 'omn-search-where';
                where.textContent = name;
                scopeEl.appendChild(where);
            }

            // "See all results" belongs up here with the scope, and not at
            // the foot of the list. It is a statement about WHERE to search,
            // and not one of the answers. At the bottom it moved with every
            // query, and it was reachable only after a scroll past
            // everything above it.
            //
            // A real <button> rather than a chip, thus Tab reaches it, and
            // Enter and Space work with no keydown handler of its own. That
            // matters more now that it is no longer in the arrow-key list.
            seeAllEl = null;
            if (globalAvailable()) {
                seeAllEl = document.createElement('button');
                seeAllEl.type = 'button';
                seeAllEl.className = 'omn-search-seeall';
                seeAllEl.textContent = 'See all results \u2192';
                seeAllEl.title = 'Open the full results page';
                seeAllEl.addEventListener('click', openResultsPage);
                scopeEl.appendChild(seeAllEl);
            }
            updateSeeAll();
        }

        // The results page is global-only - serveSearchPage answers 404 with
        // global search off - and an empty query would land on a bare form. So
        // the control is present only when following it would show something.
        function updateSeeAll() {
            if (!seeAllEl) return;
            var showing = scopeShown || (globalAvailable() ? 'all' : 'page');
            seeAllEl.hidden = !(showing === 'all' && input &&
                                input.value.trim().length >= MIN_QUERY);
        }

        function openResultsPage() {
            var q = input.value.trim();
            if (!q) return;
            close();
            // The results page searches every note and renders the answer
            // server-side, so this wait is in the NAVIGATION, after this
            // document is gone. The slow-navigation guard in omn-go-core.js
            // does not cover it: that one watches <a> clicks and this is an
            // assignment to location.
            //
            // Armed, and not shown. It is the same 300 ms that the guard
            // uses, and for the same reason. A fast answer must not flash an
            // overlay on the way past. The timer dies with the document when
            // the page arrives first, thus there is nothing to take down.
            if (window.OMNProgress) {
                setTimeout(function () {
                    window.OMNProgress.show('Searching');
                    window.OMNProgress.stage('Reading all notes…');
                    window.OMNProgress.detail(q);
                }, 300);
            }
            window.location.href = '/OMNGoSearch.html?q=' + encodeURIComponent(q);
        }

        // A switch to "All notes" is the slowest thing the dialog does, and
        // the one that most needs to say so. run() raises the bar before the
        // request leaves.
        function setScope(next) {
            if (scope === next && scopeShown === next) return;
            scope = next;
            scopeShown = next;
            renderScope();
            run();
        }

        function open() {
            if (!document.body) return;
            build();
            overlay.hidden = false;
            focusInput();
            if (input.value.trim().length >= MIN_QUERY) {
                run();
            } else {
                setStatus('Type at least ' + MIN_QUERY + ' characters');
            }
        }

        // focusInput hands the field to the keyboard twice, and the second time
        // is the one that matters.
        //
        // The soft keyboard attaches to whatever element has focus, and it
        // reads the configuration of that element at that instant. This
        // overlay goes from display:none to display:flex and takes focus in
        // the SAME tick. On Android the IME can thus attach to an element
        // that the browser has not laid out yet.
        //
        // The keyboard then comes up with no COMPOSING region, and that
        // region is how every non-Latin layout enters text. The field then
        // accepts Latin typing while it silently refuses Cyrillic. That is
        // why it reads as "the search box is broken" and not as "the
        // keyboard attached wrong". It is intermittent, because it depends
        // on what the browser had already laid out.
        //
        // blur() before the second focus() is what makes it a re-attach, and
        // not a no-op. focus() on the already-focused element does nothing,
        // and to do nothing is exactly the state that needs clearing. It is
        // the same reset that a user stumbles on, by opening the quick note
        // panel and closing it again.
        //
        // The first, synchronous focus stays so that a character typed straight
        // after Ctrl-K on a desktop is not dropped in the frame between.
        function focusInput() {
            input.focus();
            if (input.value) input.select();

            var reattach = function () {
                // Only if nothing else has taken over in the meantime: the user
                // may have closed the panel or tapped elsewhere already.
                if (!isOpen() || document.activeElement !== input) return;
                input.blur();
                input.focus();
                if (input.value) input.select();
            };

            if (window.requestAnimationFrame) {
                // Two frames: one to lay the overlay out, one to be sure it has
                // been through a paint before the keyboard looks at it.
                window.requestAnimationFrame(function () {
                    window.requestAnimationFrame(reattach);
                });
            } else {
                setTimeout(reattach, 32);
            }
        }

        function close() {
            showProgress(false);
            if (inflight) {
                inflight.abort();
                inflight = null;
            }
            if (overlay) overlay.hidden = true;
        }

        function isOpen() {
            return !!overlay && !overlay.hidden;
        }

        // showProgress raises and lowers the bar under the input.
        //
        // One wait, and its length is NOT known: it depends on how many notes
        // the index holds, which is why "All notes" needed this most. So the
        // bar is the indeterminate sweep - it says "working" and promises no
        // time. It is never left on screen empty, which would read as a wait
        // that is making no progress.
        function showProgress(on) {
            if (!progressEl) return;
            progressEl.hidden = !on;
            progressEl.classList.toggle('indeterminate', !!on);
        }

        // Typing changes no results. It keeps two things honest, and both
        // describe the field. The first is whether "See all results"
        // applies. The second is whether the rows below still answer what
        // the field says.
        function onInput() {
            updateSeeAll();
            var q = input.value.trim();
            if (q.length < MIN_QUERY) {
                setStatus(q ? 'Type at least ' + MIN_QUERY + ' characters' : '');
                return;
            }
            if (q !== lastQuery) {
                // Without this the panel answers a new query with the old
                // one's results and says nothing about it. It also states
                // what to press, which is the whole of the interaction.
                setStatus('Press ↵ or the magnifier to search');
            }
        }

        function setStatus(text) {
            if (statusEl) statusEl.textContent = text || '';
        }

        function clearRows() {
            rows = [];
            active = -1;
            if (list) list.textContent = '';
        }

        function run() {
            var q = input.value.trim();
            if (q.length < MIN_QUERY) {
                showProgress(false);
                clearRows();
                lastQuery = null;
                setStatus(q ? 'Type at least ' + MIN_QUERY + ' characters' : '');
                return;
            }

            // The rows about to arrive answer THIS text. onInputKey compares
            // against it to decide what Enter means. It is thus set here,
            // where the request is made, and not where the answer lands. A
            // reply that never comes must not leave Enter opening rows that
            // belong to a query the field no longer shows.
            lastQuery = q;

            // From here a request goes out, and how long it takes is the
            // business of the server. An index of every note answers slower
            // than one page. This is the wait that "All notes" makes visible.
            showProgress(true);
            setStatus('Searching…');

            // Cancel the previous request rather than racing it: on a fast
            // typist the older answer can otherwise arrive last and overwrite
            // the newer one.
            if (inflight) inflight.abort();
            var ctrl = (typeof AbortController !== 'undefined') ? new AbortController() : null;
            inflight = ctrl;

            var url = '/api/search?snippets=' + MAX_SNIPPETS +
                '&q=' + encodeURIComponent(q) +
                '&on=' + encodeURIComponent(pageName());
            if (scope) url += '&scope=' + encodeURIComponent(scope);

            var opts = { cache: 'no-store' };
            if (ctrl) opts.signal = ctrl.signal;

            fetch(url, opts)
                .then(function (r) { return r.json(); })
                .then(function (data) {
                    if (ctrl && inflight !== ctrl) return; // superseded
                    inflight = null;
                    // After the superseded check, never before it: a newer
                    // request is still running and its bar has to stay up.
                    showProgress(false);
                    if (data && data.status && data.status !== 'ok' && data.error) {
                        // The server refused this scope (global search off, or
                        // its index not ready). Say why and drop back to the
                        // scope that always works, rather than showing an
                        // empty list that would read as "nothing matched".
                        clearRows();
                        setStatus(data.error);
                        if (scope !== 'page') {
                            scope = 'page';
                            scopeShown = 'page';
                            renderScope();
                        }
                        return;
                    }
                    render(q, data);
                })
                .catch(function (err) {
                    // An abort is this dialog's own doing (a newer query, or
                    // close) and the newer owner is showing its own bar.
                    if (err && err.name === 'AbortError') return;
                    if (ctrl && inflight !== ctrl) return;
                    inflight = null;
                    showProgress(false);
                    clearRows();
                    setStatus('Search failed');
                    console.error('search: ' + err);
                });
        }

        function render(query, data) {
            clearRows();
            // The own list of the server, and not a naive split. It has
            // already dropped the field prefixes. "tag:hydro" is a search for
            // "hydro", and a mark on the literal "tag:hydro" would find
            // nothing. It has also applied the same minimum length that the
            // highlighter uses. A fall back to a split keeps this working
            // against an older server.
            lastTerms = (data && data.highlight && data.highlight.length)
                ? data.highlight
                : query.split(/\s+/).filter(function (t) { return t.length > 0; });

            // The server reports which scope it actually used. Adopting it
            // means the first query needs no guess about the configured
            // default, and the chips can never disagree with the results
            // underneath them.
            if (data && data.scope && data.scope !== scopeShown) {
                scopeShown = data.scope;
                renderScope();
            }

            var results = (data && data.results) ? data.results : [];
            if (!results.length) {
                setStatus(scopeShown === 'all' ? 'No matches in your notes' : 'No matches on this page');
                return;
            }

            if (scopeShown === 'all') {
                renderGlobal(data, results);
            } else {
                renderPage(results[0]);
            }
        }

        // Global scope. There are several documents, and each one has its own
        // snippets. A row opens the document, and the heading above it says
        // which one.
        function renderGlobal(data, results) {
            results.forEach(function (r) {
                var head = document.createElement('li');
                head.className = 'omn-search-doc';

                var title = document.createElement('span');
                title.className = 'omn-search-doc-title';
                title.textContent = r.title || r.name;
                head.appendChild(title);

                var path = document.createElement('span');
                path.className = 'omn-search-doc-path';
                path.textContent = r.name;
                head.appendChild(path);

                head.addEventListener('click', function () { openResult(r); });
                list.appendChild(head);

                // Each row is one LINE, so each row opens the document AT that
                // line. Passing only r sent every row of a result to the same
                // place - the first match in the note - whichever line the
                // reader chose. The heading row above keeps that behavior,
                // because it names the document and no line in it.
                (r.matches || []).forEach(function (m) {
                    list.appendChild(buildSnippetRow(m, function () { openResult(r, m); }));
                });
                if (r.truncated) {
                    var note = document.createElement('li');
                    note.className = 'omn-search-note';
                    note.textContent = 'only the first 500 KiB of this file was searched';
                    list.appendChild(note);
                }
            });

            var n = data.total || results.length;
            var note = n === 1 ? '1 result' : n + ' results';
            if (data.truncated && n > results.length) note += ' (showing ' + results.length + ')';
            setStatus(note + ' \u00b7 \u2191\u2193 to move \u00b7 \u21b5 to open');
            setActive(0);
        }

        // A result in global scope is a different document, thus to follow
        // it is a navigation. Page scope is different. There the answer is
        // already on screen, and the useful move is a highlight in place.
        //
        // m is the line that the reader chose. It is absent when the reader
        // chose the document itself.
        function openResult(r, m) {
            close();
            if (r && r.url) window.location.href = withHighlight(r.url, m);
        }

        // withHighlight hangs the query terms off a URL as ?hl=, thus the
        // page that opens marks them on arrival. With a line it also hangs
        // the text of that line off as ?hlt=, and that is what the page goes
        // TO.
        //
        // A result lists each matching line. To open the first match in the
        // note is right for the first line alone. These are the same two
        // parameters that the results page puts on its links, which are
        // highlightURL and snippetURL in search.go. A result thus behaves
        // identically, whichever list it came from. The receiving page strips
        // them from the address bar once applied, see omn-go-core.js.
        function withHighlight(url, m) {
            // The fragment stays last. A sectioned result arrives here as
            // "/Bookmarks.html#2026-06-15-200000". A blind append gives
            // "#2026-06-15-200000?hl=cats". That is one fragment that names
            // no element, and no query string at all, thus the page neither
            // scrolls nor highlights. This mirrors highlightURL in search.go.
            // The two build the same URL from opposite ends of the app, and
            // they have to agree.
            var frag = '';
            var hash = url.indexOf('#');
            if (hash >= 0) {
                frag = url.slice(hash);
                url = url.slice(0, hash);
            }
            // url already ends in the section of the BEST hit. This line may
            // be in another one. The fragment is what the page falls back to
            // when it cannot find the text.
            if (m && m.section && m.section.id) frag = '#' + m.section.id;

            var sep = url.indexOf('?') === -1 ? '?' : '&';
            for (var i = 0; i < lastTerms.length; i++) {
                url += sep + 'hl=' + encodeURIComponent(lastTerms[i]);
                sep = '&';
            }
            // A hit inside a <script> block gets no ?hlt=. The text is
            // indexed and never rendered. There is thus no word on the page
            // to go to, and a search for it would only find a coincidence.
            // With no terms nothing is marked, thus there is nothing to go
            // to either.
            if (lastTerms.length && m && m.text && m.context !== 'script') {
                url += sep + 'hlt=' + encodeURIComponent(m.text);
            }
            return url + frag;
        }

        function renderPage(result) {
            if (!result || !result.matches || !result.matches.length) {
                // A note can match on its title or a tag and have no
                // matching LINE. Say so, rather than show an empty list that
                // reads as "nothing found".
                setStatus('Matches this page’s title or tags, but no line in the text');
                return;
            }

            result.matches.forEach(function (m, i) {
                var idx = i;
                list.appendChild(buildSnippetRow(m, function () { choose(idx, m); }));
            });

            var n = result.matches.length;
            var note = n === 1 ? '1 matching line' : n + ' matching lines';
            if (result.truncated) note += ' · only the first 500 KiB was searched';
            setStatus(note + ' · ↑↓ to move · ↵ to highlight in the page');
            setActive(0);
        }

        // buildSnippetRow renders one match. Both scopes share it, thus a
        // line looks the same wherever it was found. Only what a follow of it
        // DOES differs, and that is the business of the caller.
        function buildSnippetRow(m, onChoose) {
            var li = document.createElement('li');
            li.className = 'omn-search-row';
            li.setAttribute('role', 'option');

            // A section label replaces the bare line number when there is
            // one. "27 Jul, 07:23" or "OMN-Go on GitHub" locates a hit
            // inside a 3 000-line QuickNotes in a way that "line 1842" does
            // not. The line number is still what the API reports, and still
            // what the editor needs. It is not what a reader looks for.
            var num = document.createElement('span');
            num.className = 'omn-search-line';
            if (m.section && m.section.label) {
                num.classList.add('omn-search-section');
                num.textContent = '\u203a ' + m.section.label;
                num.title = 'line ' + m.line;
            } else {
                num.textContent = m.line;
            }
            li.appendChild(num);

            if (m.context) {
                // A hit inside a script or a fenced block is a different kind
                // of answer from one in prose. Marked, never ranked down: code
                // in notes is a normal thing to search for.
                var ctx = document.createElement('span');
                ctx.className = 'omn-search-ctx';
                ctx.textContent = '‹/›';
                ctx.title = m.context === 'script' ? 'inside a <script> block' : 'inside a code block';
                li.appendChild(ctx);
            }

            var text = document.createElement('span');
            text.className = 'omn-search-text';
            renderHighlighted(text, m.text || '', m.spans || []);
            li.appendChild(text);

            var idx = rows.length;
            li.addEventListener('click', onChoose);
            li.addEventListener('mousemove', function () { setActive(idx); });
            rows.push(li);
            li._omnChoose = onChoose;
            return li;
        }

        // renderHighlighted writes text into node, and it wraps each span in
        // a <mark>. Spans are RUNE offsets, because the Go side works in
        // runes so that Cyrillic is not cut in half. The text is thus split
        // with Array.from, which iterates code points. text.substring would
        // use UTF-16 units, and it would drift on anything outside the BMP.
        function renderHighlighted(node, text, spans) {
            var runes = Array.from(text);
            var at = 0;
            spans.forEach(function (s) {
                var start = s[0], len = s[1];
                if (typeof start !== 'number' || typeof len !== 'number') return;
                if (start < at || start + len > runes.length) return;
                if (start > at) {
                    node.appendChild(document.createTextNode(runes.slice(at, start).join('')));
                }
                var mark = document.createElement('mark');
                mark.className = 'omn-search-hit';
                mark.textContent = runes.slice(start, start + len).join('');
                node.appendChild(mark);
                at = start + len;
            });
            if (at < runes.length) {
                node.appendChild(document.createTextNode(runes.slice(at).join('')));
            }
        }

        function setActive(i) {
            if (!rows.length) return;
            if (i < 0) i = 0;
            if (i >= rows.length) i = rows.length - 1;
            if (active >= 0 && rows[active]) rows[active].classList.remove('is-active');
            active = i;
            rows[active].classList.add('is-active');
            if (rows[active].scrollIntoView) {
                rows[active].scrollIntoView({ block: 'nearest' });
            }
        }

        // choose closes the panel and marks the query in the page itself.
        //
        // A per-LINE jump is deliberately not attempted here. The line number
        // of the result indexes the markdown SOURCE, and the page shows
        // compiled HTML. There is no reliable mapping between the two,
        // without the machinery that a later phase adds.
        //
        // To highlight every occurrence and scroll to the first is honest
        // about what it knows. It is also the answer to "where does this note
        // talk about X" either way.
        function choose(i, m) {
            setActive(i);
            close();
            var first = window.omnHighlightTerms(lastTerms);

            // Go to the occurrence that THIS row is about, and not to the
            // first one on the page. A row inside a <script> block is skipped
            // deliberately. The text is indexed and never rendered, thus
            // there is nothing on the page to scroll to, and a look would
            // only find a coincidence.
            var target = null;
            if (m && m.context !== 'script' && window.omnMarkNear) {
                target = window.omnMarkNear(m.text || '');
            }

            var el = target || first;
            if (el && el.scrollIntoView) {
                el.scrollIntoView({ block: 'center' });
                if (el.classList) el.classList.add('omn-search-hit-current');
            }
        }

        function onInputKey(e) {
            switch (e.key) {
            case 'Escape':
                e.preventDefault();
                close();
                break;
            case 'ArrowDown':
                e.preventDefault();
                setActive(active + 1);
                break;
            case 'ArrowUp':
                e.preventDefault();
                setActive(active - 1);
                break;
            case 'Enter':
                e.preventDefault();
                // Enter is the magnifier of the keyboard. It searches
                // whenever the field says something that the rows on screen
                // do not answer. Typing now searches nothing, thus that is
                // every moment between typing and asking. Only when the two
                // agree does Enter mean what it used to, which is to open
                // the row I am on.
                if (input.value.trim() !== lastQuery) {
                    run();
                    break;
                }
                if (rows.length) {
                    var row = rows[active < 0 ? 0 : active];
                    if (row && row._omnChoose) row._omnChoose();
                } else {
                    run();
                }
                break;
            }
        }

        // --- highlighting inside the rendered page ---
        //
        // The implementation lives in omn-go-core.js, and not here. An
        // arrival at a page with ?hl= needs it on every page, and a page
        // opened from disk counts. The server half of this file never runs
        // there. This module is one of its callers.

        // --- entry points ---

        window.omnSearchOpen = function () {
            if (isOpen()) {
                input.focus();
                input.select();
                return;
            }
            open();
        };


        // omn-go-sse.js holds the keyboard shortcuts, because Ctrl-K and
        // the slash key must work before this file loads. That listener
        // asks these two whether an overlay is on screen, and closes it.
        window.omnSearchIsOpen = isOpen;
        window.omnSearchClose = close;
    })();

} else {
    window.omnSearchOpen = function() { printDebug('omnSearchOpen'); };
}
