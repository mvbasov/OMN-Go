// --- The Log page ---
//
// The whole page of /OMNGoLogs.html. Only templates/logs_page.html names
// this file, and a note page never loads it. That is the same arrangement
// as omn-go-config.js.
//
// IT READS TWO ADDRESSES. /api/logs/history answers the lines that the
// ring holds, oldest first. /api/logs carries each new line as the server
// writes it. This file reaches the second one through
// window.omnGoOnServerLog of omn-go-sse.js, thus the page opens NO second
// connection to the stream.
//
// Both addresses are admin only since 26.09.59. serveLogsPage answers a
// guest with an explanation and not with this page.
//
// THE HEADER OF THE PAGE ALREADY CARRIES THE SYNC BUTTONS. Every compiled
// page shares one shell, and Download and Upload sit in it. A reader can
// thus start a sync here and watch each line arrive. That is the answer to
// "how does a person see a sync on one Android screen".
//
// WHY THE FILTER IS IN THE PAGE AND NOT ON THE SERVER. The ring holds every
// line, whatever the switches of the Config page say. A reader who looks
// for one fault narrows the view here, and stdout keeps what it had. See
// the banner of logger.go.
//
// A line reads "<stamp> [tag] (level) message". window.omnParseLogLine in
// omn-go-sse.js splits it, and that function is the one authority for the
// shape of a line. See rule 7 of CLAUDE.md section 1.

'use strict';

(function () {

    // The three levels of log_levels.go. A level that this list omits can
    // never be hidden, which is the safe direction.
    var LOG_LEVELS = ['error', 'info', 'debug'];

    // The lines that the page holds, in arrival order. The filter reads
    // this array and draws again. It never asks the server a second time.
    var lines = [];

    // The tag of each line that has arrived, in the order of first
    // arrival. The page builds a checkbox for each one.
    var tagsSeen = [];

    // A level or a tag with the value true is HIDDEN. An unknown name is
    // thus shown, and a tag that arrives later needs no default.
    var hiddenLevels = {};
    var hiddenTags = {};

    // logLineShows answers whether one line passes the filter.
    //
    // parts is the answer of window.omnParseLogLine, or null. A line that
    // does not parse ALWAYS shows. Such a line comes from one of the three
    // log.Printf call sites that can reach no application, and each one is
    // a fault. To hide a fault behind a filter is the wrong direction.
    function logLineShows(parts, levels, tags) {
        if (!parts) { return true; }
        if (levels[parts.level]) { return false; }
        if (tags[parts.tag]) { return false; }
        return true;
    }

    function parseLine(msg) {
        if (typeof window.omnParseLogLine !== 'function') { return null; }
        return window.omnParseLogLine(msg);
    }

    function el(id) { return document.getElementById(id); }

    // A row of the log. It is built with textContent and never with
    // innerHTML, thus a note name or a path in a line cannot inject
    // markup. A log line carries text that a person wrote.
    function makeRow(msg, parts) {
        var row = document.createElement('div');
        row.className = 'lg-row';
        if (parts) { row.classList.add('lg-' + parts.level); }
        row.textContent = msg.replace(/\n+$/, '');
        return row;
    }

    // draw fills the body with each line that the filter allows.
    //
    // It rebuilds the whole body. A log of 500 lines is small, and a
    // rebuild keeps one code path for the first draw, for a filter change
    // and for a reload.
    function draw() {
        var body = el('lgBody');
        var count = el('lgCount');
        if (!body) { return; }
        var atEnd = body.scrollTop + body.clientHeight >= body.scrollHeight - 4;
        body.textContent = '';
        var shown = 0;
        for (var i = 0; i < lines.length; i++) {
            var parts = parseLine(lines[i]);
            if (!logLineShows(parts, hiddenLevels, hiddenTags)) { continue; }
            body.appendChild(makeRow(lines[i], parts));
            shown++;
        }
        if (shown === 0) {
            var none = document.createElement('div');
            none.className = 'lg-none';
            none.textContent = lines.length === 0
                ? 'The log holds no line yet.'
                : 'Every line of the log is hidden by the filter.';
            body.appendChild(none);
        }
        if (count) {
            count.textContent = shown + ' of ' + lines.length;
        }
        // A reader who scrolled up is reading something. Only a reader who
        // sits at the end wants to follow the newest line.
        if (atEnd) { body.scrollTop = body.scrollHeight; }
    }

    // A checkbox for one level or for one tag.
    function makeBox(name, hidden, onChange) {
        var label = document.createElement('label');
        label.className = 'lg-box';
        var input = document.createElement('input');
        input.type = 'checkbox';
        input.checked = !hidden[name];
        input.addEventListener('change', function () {
            if (input.checked) { delete hidden[name]; } else { hidden[name] = true; }
            onChange();
        });
        label.appendChild(input);
        var text = document.createElement('span');
        text.textContent = name;
        label.appendChild(text);
        return label;
    }

    function drawFilter() {
        var levels = el('lgLevels');
        var tags = el('lgTags');
        if (levels) {
            levels.textContent = '';
            for (var i = 0; i < LOG_LEVELS.length; i++) {
                levels.appendChild(makeBox(LOG_LEVELS[i], hiddenLevels, draw));
            }
        }
        if (tags) {
            tags.textContent = '';
            for (var j = 0; j < tagsSeen.length; j++) {
                tags.appendChild(makeBox(tagsSeen[j], hiddenTags, draw));
            }
        }
    }

    // noteTag records a tag that this page has not met yet.
    //
    // The list comes from the lines and not from OMN_LOG_TAGS. A tag that
    // wrote no line is a checkbox that does nothing, and a tag that a later
    // version adds needs no change here.
    function noteTag(msg) {
        var parts = parseLine(msg);
        if (!parts) { return false; }
        if (tagsSeen.indexOf(parts.tag) !== -1) { return false; }
        tagsSeen.push(parts.tag);
        tagsSeen.sort();
        return true;
    }

    function addLine(msg) {
        lines.push(msg);
        if (noteTag(msg)) { drawFilter(); }
        draw();
    }

    // load reads the ring. It replaces every line that the page holds,
    // thus Reload after a long time gives the newest 500 and no duplicate.
    function load() {
        var body = el('lgBody');
        if (body) { body.textContent = 'Loading…'; }
        fetch('/api/logs/history').then(function (res) {
            if (res.status === 401 || res.status === 403) {
                throw new Error('This page is for the admin of this device.');
            }
            if (!res.ok) { throw new Error('HTTP ' + res.status); }
            return res.json();
        }).then(function (data) {
            lines = (data && data.lines) || [];
            tagsSeen = [];
            for (var i = 0; i < lines.length; i++) { noteTag(lines[i]); }
            drawFilter();
            draw();
        }).catch(function (e) {
            if (body) { body.textContent = 'Cannot read the log: ' + e.message; }
        });
    }

    // The text that Copy hands over. It is what the reader SEES, thus a
    // hidden line stays out of it. A person who copies a filtered view
    // wants the filtered view.
    function shownText() {
        var out = [];
        for (var i = 0; i < lines.length; i++) {
            var parts = parseLine(lines[i]);
            if (!logLineShows(parts, hiddenLevels, hiddenTags)) { continue; }
            out.push(lines[i].replace(/\n+$/, ''));
        }
        return out.join('\n');
    }

    // Copy hands the SHOWN text over.
    //
    // window.omnGoCopyText of omn-go-core.js is the only clipboard writer
    // of this application, and TestClipboardHasOneAuthority holds that
    // rule. It knows what the Android WebView does with the Clipboard API,
    // and this page must not learn that a second time.
    function copyShown() {
        var btn = el('lgCopy');
        var said = function (word) {
            if (!btn) { return; }
            if (typeof btn.dataset.omnLabel === 'undefined') {
                btn.dataset.omnLabel = btn.textContent;
            }
            btn.textContent = word;
            clearTimeout(btn._omnCopyTimer);
            btn._omnCopyTimer = setTimeout(function () {
                btn.textContent = btn.dataset.omnLabel;
            }, 1200);
        };
        if (typeof window.omnGoCopyText !== 'function') {
            said('Cannot copy');
            return;
        }
        window.omnGoCopyText(shownText()).then(function () {
            said('Copied');
        }, function () {
            said('Cannot copy');
        });
    }

    function start() {
        var reload = el('lgReload');
        if (reload) { reload.addEventListener('click', load); }
        var copy = el('lgCopy');
        if (copy) { copy.addEventListener('click', copyShown); }
        var toggle = el('lgFilterToggle');
        var filter = el('lgFilter');
        if (toggle && filter) {
            toggle.addEventListener('click', function () {
                filter.classList.toggle('hidden');
            });
        }
        load();
        if (typeof window.omnGoOnServerLog === 'function') {
            window.omnGoOnServerLog(addLine);
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', start);
    } else {
        start();
    }

    // A test outside a browser reads this file with a Node require. The
    // check of module keeps the export out of a page, where module does not
    // exist. See backend/frontend/test/, which no build ships.
    if (typeof module !== 'undefined' && module.exports) {
        module.exports = { logLineShows: logLineShows };
    }
})();
