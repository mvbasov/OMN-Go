// --- The bookmark panel ---
//
// The capture of a shared link, the panel itself, and the autocomplete of
// the Tags box.
//
// THIS FILE ARRIVES ON DEMAND. omn-go-api.js writes a stub for
// handleShare and showBookmarkPanel, and for the bookmark-panel action. The
// first call of one of them loads this file. See omnLazy and omnLazyActions
// there.
//
// TWO PARTS STAYED IN omn-go-api.js, and each one has a reason.
// omnGoInsertCapture answers Android with a value that Android reads,
// thus it can never be a stub that answers with a Promise. The drag and
// drop listener has to listen before a person drops a link, thus it
// cannot wait for a press. That listener writes the two boxes itself and
// calls showBookmarkPanel last, which loads this file.
if (window.location.protocol !== 'file:') {

    // --- Bookmark capture UI ---
    //
    // handleShare is the Android share-to path. It, the URL drag-and-drop,
    // and the tag autocomplete all belong to the server-backed bookmark and
    // quick-note capture flow. The submit handlers of that flow already live
    // in this file.
    window.handleShare = function(text, subject) {
        text = text || '';
        subject = subject || '';

        // Regex to find the first valid URL
        const urlMatch = text.match(/(https?:\/\/[^\s]+)/) || subject.match(/(https?:\/\/[^\s]+)/);

        if (urlMatch) {
            // URL Found -> Route to Bookmark Panel
            const url = urlMatch[0];
            document.getElementById('omn-bm-url').value = url;

            let title = subject;
            if (!title || title.includes(url)) {
                title = text.replace(url, '').trim();
            }
            if (!title) title = "Shared Link";

            document.getElementById('omn-bm-title').value = title;
            // showBookmarkPanel also hides the Quick Note panel.
            window.showBookmarkPanel();
        } else {
            // No URL -> Route to Quick Note Panel
            let content = '';
            if (subject) content += subject + "\n\n";
            if (text) content += text;

            document.getElementById('omn-quick-text').value = content.trim();
            window.omnShowPanel('omn-quick-panel');
        }
    };
    // --- Bookmark "Tags" autocomplete ---
    //
    // Suggests existing tags while a person types into the #omn-bm-tags field of
    // the Ingest Bookmark modal. Tags are typed comma-separated, as in
    // "work, recipe, ita|", where the "|" marks the caret. Suggestions are
    // computed against the fragment after the last comma alone. They are
    // shown once that fragment reaches the minChars attribute of #omn-bm-tags,
    // which is 2 by default and set in index.html. To pick a suggestion
    // completes the fragment and appends ", ", thus the next tag can be
    // typed right away.
    //
    // This is plain same-origin UI sugar, and not a "server extension". The
    // sync and login calls in omn-go-api.js need a protocol guard, and this
    // does not. A failed fetch, for example on a page opened offline, is
    // read as "no suggestions" and not as an error. The field still works as
    // a plain comma-separated text input either way.
    //
    // Both the DOM wiring and the tag-list fetch are deliberately lazy. They
    // run the first time the Ingest Bookmark modal is opened, and not on
    // every page load. Most page views never touch this panel.
    //
    // window.showBookmarkPanel() and the bookmark-panel action below are the
    // only places that open #omn-bm-panel. The "add bookmark" button of the
    // header, the URL drag-and-drop handler and window.handleShare all go
    // through one of them. Each one opens the panel with omnShowPanel or
    // omnTogglePanel of omn-go-core.js, and then prepares the tag list.
    (function () {
        var tagsCache = null;    // null until prepared; array once loaded (even if empty)
        var tagsPromise = null;  // in-flight fetch, if any
        var wired = false;       // #omn-bm-tags/#omn-bm-tags-suggestions listeners attached only once

        // Fetches /json/bookmarker-tags.json at most one time for each page.
        // It is safe to call every time the modal opens. The list can be
        // already prepared, with tagsCache set, or already loading, with
        // tagsPromise set. This reuses either one, and it fires no second
        // request.
        function ensureTagsLoaded() {
            if (tagsCache) return Promise.resolve(tagsCache);
            if (tagsPromise) return tagsPromise;
            tagsPromise = fetch('/json/bookmarker-tags.json', { cache: 'no-store' })
                .then(function (res) { return res.ok ? res.json() : []; })
                .then(function (data) { return (tagsCache = Array.isArray(data) ? data : []); })
                .catch(function () { return (tagsCache = []); });
            return tagsPromise;
        }

        // "foo, bar, ba" -> { done: ["foo", "bar"], fragment: "ba" }
        function splitTags(value) {
            var parts = value.split(',');
            var fragment = parts.pop();
            var done = parts.map(function (s) { return s.trim(); }).filter(Boolean);
            return { done: done, fragment: fragment.replace(/^\s+/, '') };
        }

        // Attaches the input/keydown/click listeners to #omn-bm-tags exactly once.
        // showBookmarkPanel() and the bookmark-panel action call it each time
        // the modal opens. The `wired` guard makes a repeat call do nothing,
        // thus a second open of the panel attaches no second listener.
        function wireBookmarkTagAutocomplete() {
            if (wired) return;
            var input = document.getElementById('omn-bm-tags');
            var list = document.getElementById('omn-bm-tags-suggestions');
            if (!input || !list) return;
            wired = true;

            var minChars = parseInt(input.getAttribute('minChars'), 10);
            if (!minChars || minChars < 1) minChars = 2;

            var activeIndex = -1;

            function hide() {
                list.innerHTML = '';
                list.classList.add('omn-hidden');
                activeIndex = -1;
            }

            function setActive(idx) {
                var items = list.querySelectorAll('.omn-tag-suggestion-item');
                items.forEach(function (it, i) { it.classList.toggle('omn-active', i === idx); });
                activeIndex = idx;
            }

            function pick(tag) {
                var split = splitTags(input.value);
                var used = split.done.concat([tag]);
                // Rebuilding from scratch (rather than splicing) keeps this
                // correct even if the fragment was picked mid-string; the
                // trailing ", " primes the field for the next tag.
                input.value = used.join(', ') + ', ';
                hide();
                input.focus();
                var end = input.value.length;
                input.setSelectionRange(end, end);
            }

            function render(matches) {
                list.innerHTML = '';
                if (!matches.length) { hide(); return; }
                matches.forEach(function (tag) {
                    var li = document.createElement('li');
                    li.textContent = tag;
                    li.className = 'omn-tag-suggestion-item';
                    // mousedown (not click) fires before #omn-bm-tags's blur, so
                    // the pick survives the input losing focus.
                    li.addEventListener('mousedown', function (e) {
                        e.preventDefault();
                        pick(tag);
                    });
                    list.appendChild(li);
                });
                activeIndex = -1;
                list.classList.remove('omn-hidden');
            }

            function update() {
                var split = splitTags(input.value);
                var fragment = split.fragment;
                if (fragment.length < minChars) { hide(); return; }
                ensureTagsLoaded().then(function (tags) {
                    // The field may have moved on while this fetch/cache
                    // lookup was pending - drop a stale response.
                    if (splitTags(input.value).fragment !== fragment) return;
                    var lower = fragment.toLowerCase();
                    var used = split.done.map(function (t) { return t.toLowerCase(); });
                    var matches = tags.filter(function (t) {
                        return typeof t === 'string' &&
                            t.toLowerCase().indexOf(lower) === 0 &&
                            used.indexOf(t.toLowerCase()) === -1;
                    }).slice(0, 20);
                    render(matches);
                });
            }

            input.addEventListener('input', update);
            input.addEventListener('focus', update);

            input.addEventListener('keydown', function (e) {
                var items = list.querySelectorAll('.omn-tag-suggestion-item');
                if (list.classList.contains('omn-hidden') || !items.length) return;
                if (e.key === 'ArrowDown') {
                    e.preventDefault();
                    setActive((activeIndex + 1) % items.length);
                } else if (e.key === 'ArrowUp') {
                    e.preventDefault();
                    setActive((activeIndex - 1 + items.length) % items.length);
                } else if (e.key === 'Enter' && activeIndex >= 0) {
                    e.preventDefault();
                    pick(items[activeIndex].textContent);
                } else if (e.key === 'Escape') {
                    // The first Escape hides only the suggestions. The
                    // listener of omn-go-api.js then skips this key, and
                    // the panel stays open.
                    e.preventDefault();
                    hide();
                }
            });

            document.addEventListener('click', function (e) {
                if (e.target !== input && !list.contains(e.target)) hide();
            });
        }

        // Unconditionally shows #omn-bm-panel (used by drag-and-drop and
        // handleShare, which only ever want it open, never toggled).
        window.showBookmarkPanel = function () {
            if (!window.omnShowPanel('omn-bm-panel')) return;
            wireBookmarkTagAutocomplete();
            ensureTagsLoaded();
        };

        // Toggles #omn-bm-panel (used by the header's "add bookmark" button, which
        // both opens and closes it). Only prepares the autocomplete on the
        // transition into "visible" - closing the panel does nothing extra.
        // See OMN.action in omn-go-core.js.
        window.OMN.action('bookmark-panel', function () {
            if (window.omnTogglePanel('omn-bm-panel')) {
                wireBookmarkTagAutocomplete();
                ensureTagsLoaded();
            }
        });
    })();

} else {
    window.handleShare = function() { printDebug('handleShare'); };
    window.showBookmarkPanel = function() { printDebug('showBookmarkPanel'); };
}
