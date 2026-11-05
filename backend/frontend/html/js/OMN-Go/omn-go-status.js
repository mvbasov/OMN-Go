// --- The Status page ---
//
// The whole page of /OMNGoStatus.html. Only templates/status_page.html
// names this file, and a note page never loads it. That is the same
// arrangement as omn-go-config.js and omn-go-logs.js.
//
// The page holds no fact of its own. It reads /api/status and draws what
// comes back. That endpoint came first for this reason.
//
// IT IS A FILE AND NOT AN INLINE BLOCK. Section 4 of CLAUDE.md asks for a
// file and a src, and nothing about this code needs an exception.

'use strict';

(function () {
    // The page holds what it loaded so far. A second request for a slow
    // section adds to this object, and the page draws it again.
    var state = {};
    var loaded = ['server', 'config', 'git', 'search', 'runtime', 'android'];

    var body = document.getElementById('omn-st-body');

    // Titles for the sections, in the order the page shows them.
    var ORDER = [
        ['server', 'Server'],
        ['config', 'Configuration'],
        ['git', 'Git'],
        ['git_dirty', 'Git worktree'],
        ['search', 'Search'],
        ['runtime', 'Runtime'],
        ['android', 'Android'],
        ['storage', 'Storage'],
        ['errors', 'Errors']
    ];

    // Fields whose value is a count of bytes. The page adds a readable
    // size after the exact number, because a phone screen is small and
    // 18874368 says less than 18.0 MB.
    var BYTE_KEYS = /(^|_)(bytes|heap_alloc|sys)$/;

    function human(n) {
        if (typeof n !== 'number' || n < 1024) { return null; }
        var units = ['kB', 'MB', 'GB'], v = n / 1024, i = 0;
        while (v >= 1024 && i < units.length - 1) { v = v / 1024; i++; }
        return v.toFixed(1) + ' ' + units[i];
    }

    function valueText(key, value) {
        if (value === null || value === undefined) { return ''; }
        if (Array.isArray(value)) { return value.length ? value.join(', ') : '—'; }
        if (typeof value === 'boolean') { return value ? 'yes' : 'no'; }
        if (typeof value === 'number' && BYTE_KEYS.test(key)) {
            var h = human(value);
            return h ? h + ' (' + value + ' bytes)' : value + ' bytes';
        }
        return String(value);
    }

    // Every value goes in through textContent. A commit subject, a path
    // and a remote name are content, not markup.
    function row(table, key, value) {
        var tr = document.createElement('tr');
        var k = document.createElement('td');
        k.className = 'omn-st-key';
        k.textContent = key;
        var v = document.createElement('td');
        v.className = 'omn-st-val';
        v.textContent = value;
        tr.appendChild(k);
        tr.appendChild(v);
        table.appendChild(tr);
    }

    function rowsFor(table, prefix, obj) {
        Object.keys(obj).forEach(function (key) {
            var value = obj[key];
            var name = prefix ? prefix + '.' + key : key;
            if (value && typeof value === 'object' && !Array.isArray(value)) {
                if (typeof value.files === 'number' && typeof value.bytes === 'number') {
                    // A storage group: one line reads better than two.
                    var h = human(value.bytes);
                    row(table, name,
                        value.files + (value.files === 1 ? ' file, ' : ' files, ') +
                        (h ? h + ' (' + value.bytes + ' bytes)' : value.bytes + ' bytes'));
                    return;
                }
                rowsFor(table, name, value);
                return;
            }
            row(table, name, valueText(key, value));
        });
    }

    function render() {
        body.textContent = '';
        ORDER.forEach(function (pair) {
            var data = state[pair[0]];
            if (!data) { return; }
            var section = document.createElement('div');
            section.className = 'omn-st-section';
            var h = document.createElement('h3');
            h.textContent = pair[1];
            section.appendChild(h);
            var table = document.createElement('table');
            table.className = 'omn-st-table';
            rowsFor(table, '', data);
            section.appendChild(table);
            body.appendChild(section);
        });
        if (state.generated) {
            var p = document.createElement('p');
            p.className = 'omn-st-note';
            p.textContent = 'Generated ' + state.generated;
            body.appendChild(p);
        }
    }

    function fail(message) {
        body.textContent = '';
        var p = document.createElement('p');
        p.className = 'omn-st-error';
        p.textContent = message;
        body.appendChild(p);
    }

    async function load(sections, note) {
        var url = '/api/status';
        if (sections) { url += '?sections=' + encodeURIComponent(sections); }
        if (note && window.OMNProgress) {
            window.OMNProgress.show('Status');
            window.OMNProgress.stage(note);
        }
        try {
            var res = await fetch(url, { cache: 'no-store' });
            if (!res.ok) { throw new Error('HTTP ' + res.status); }
            var data = await res.json();
            Object.keys(data).forEach(function (key) { state[key] = data[key]; });
            render();
        } catch (e) {
            if (Object.keys(state).length === 0) {
                fail('Could not read the status: ' + e.message);
            }
        } finally {
            if (note && window.OMNProgress) { window.OMNProgress.hide(); }
        }
    }

    function ask(id, sections, note) {
        var btn = document.getElementById(id);
        btn.addEventListener('click', async function () {
            btn.disabled = true;
            if (loaded.indexOf(sections) < 0) { loaded.push(sections); }
            await load(sections, note);
            btn.disabled = false;
        });
    }

    ask('omn-st-storage', 'storage', 'Counting the files in the storage directory');
    ask('omn-st-dirty', 'git_dirty', 'Reading the git worktree state');

    document.getElementById('omn-st-reload').addEventListener('click', function () {
        state = {};
        load(loaded.join(','), null);
    });

    // Copy asks the server for the Markdown form of the sections that the
    // page holds. The text is therefore the same text that "Open as text"
    // gives, and a bug report gets each fact in one paste.
    // omnGoCopyText in omn-go-share.js is the only clipboard writer of the
    // application. A second copy here would be a second path that must
    // agree about the Android WebView.
    document.getElementById('omn-st-copy').addEventListener('click', async function () {
        var btn = this;
        var label = btn.textContent;
        try {
            var res = await fetch('/api/status?format=md&sections=' +
                encodeURIComponent(loaded.join(',')), { cache: 'no-store' });
            if (!res.ok) { throw new Error('HTTP ' + res.status); }
            await window.omnGoCopyText(await res.text());
            btn.textContent = 'Copied';
        } catch (e) {
            btn.textContent = 'Copy failed';
        }
        setTimeout(function () { btn.textContent = label; }, 1400);
    });

    load(null, null);
})();
