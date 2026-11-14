// omn-go-db.js - the script of the Database Backups page.
//
// db_backups.html loads it with a script element. The page lists each
// database with its backups from GET /api/db/backups. The buttons make a
// backup (POST /api/db/backup) and restore one (POST /api/db/restore). Each
// question comes from omnDialog of omn-go-dialog.js.
//
// The list is HTML text. Each value from the server goes through esc, thus
// a name of a database or of a file adds no markup. The dialogs set each
// text with textContent.
(function () {
    'use strict';
    const list = document.getElementById('omn-db-backup-list');
    if (!list) return;
    if (window.location.protocol === 'file:') {
        list.textContent = 'Database backups need the OMN-Go server (page opened via file://).';
        return;
    }

    // cls names a .omn-dbb-dot.omn-st-* rule in omn-go-db.css. Thus the
    // colors of the dots stay in the CSS with the other colors.
    const STATE = {
        insync:       { cls: 'omn-st-insync',       label: 'in sync',        hint: 'Database matches its newest backup.' },
        dirty:        { cls: 'omn-st-dirty',        label: 'not backed up',  hint: 'Database has changes newer than any backup.' },
        backup_newer: { cls: 'omn-st-backup_newer', label: 'backup newer',   hint: 'Newest backup is newer than the database (pulled from another device?). Consider Restore.' },
        missing:      { cls: 'omn-st-missing',      label: 'no database',    hint: 'Backups exist but the database file does not (fresh device). It is restored automatically on first use, or restore manually here.' },
        none:         { cls: 'omn-st-none',         label: 'no backups',     hint: 'No backups exist yet for this database.' },
        invalid:      { cls: 'omn-st-invalid',      label: 'invalid backup', hint: 'The newest backup file cannot be parsed (damaged or conflicted?).' }
    };

    // lastData is the last answer of /api/db/backups. The dialogs read the
    // backups and the prune depth from it.
    let lastData = null;

    function esc(s) {
        return String(s).replace(/[&<>"]/g, function (c) {
            return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
        });
    }
    function kb(n) { return n >= 1024 ? (n / 1024).toFixed(1) + ' KB' : n + ' B'; }
    function ts(iso) { return iso ? iso.replace('T', ' ').replace(/(:\d\d)(\.\d+)?(Z|[+-].*)$/, '$1') : '—'; }

    async function api(url, opts) {
        const res = await fetch(url, opts || {});
        const data = await res.json();
        if (data.status !== 'success') throw new Error(data.message || 'request failed');
        return data;
    }

    function findDatabase(name) {
        return (lastData.databases || []).find(function (d) { return d.name === name; });
    }

    // runWithProgress shows the progress bar while work runs. A backup of
    // a large database takes time on a phone. The bar goes away before the
    // error dialog, thus the dialog never shows above a bar that turns.
    async function runWithProgress(title, stage, detail, work, failure) {
        let err = null;
        window.OMNProgress.show(title);
        window.OMNProgress.stage(stage);
        if (detail) window.OMNProgress.detail(detail);
        try {
            await work();
        } catch (e) {
            err = e;
        }
        window.OMNProgress.hide();
        if (err) {
            await window.omnAlert(failure + err.message);
            return;
        }
        load();
    }

    async function backupNow(name) {
        const db = findDatabase(name);
        const content = [];
        if (db && db.backups.length >= lastData.prune_depth) {
            const oldest = db.backups[db.backups.length - 1].file;
            content.push({ warning: 'This exceeds the prune depth (' + lastData.prune_depth + '): the oldest backup ' + oldest + ' will be deleted.' });
            content.push({
                p: name.indexOf('local-') === 0
                    ? 'This is a local database: its backups are not in git history. The pruned file is unrecoverable.'
                    : 'For synced databases the pruned file still exists in git history.',
            });
        }
        const answer = await window.omnDialog({
            kind: 'db-backup',
            title: 'Backup ' + name,
            text: 'Create a new backup of ' + name + '?',
            content: content,
            buttons: [
                { label: 'Create backup', value: true, default: true },
                { label: 'Cancel', value: false, cancel: true },
            ],
        });
        if (answer.value !== true) return;
        await runWithProgress('Database backup', 'Creating backup of ' + name + '…', '', function () {
            return api('/api/db/backup?db=' + encodeURIComponent(name), { method: 'POST' });
        }, 'Backup failed: ');
    }

    // restore asks before it replaces a whole database. Cancel is the
    // default button, thus Enter never replaces a database.
    async function restore(name, file) {
        const db = findDatabase(name);
        const bk = db.backups.find(function (b) { return b.file === file; });
        const current = db.sqlite_exists
            ? kb(db.size) + ', last modified ' + ts(db.mtime)
            : 'does not exist yet';
        const answer = await window.omnDialog({
            kind: 'db-restore',
            title: 'Restore ' + name,
            text: 'Replace the entire database ' + name + ' with this backup?',
            content: [
                {
                    table: [
                        ['Current database:', current],
                        ['Backup:', file],
                        ['Created:', ts(bk.created) + ' on ' + (bk.hostname || '?')],
                        ['Content:', bk.objects + ' objects, ' + bk.rows + ' rows, ' + kb(bk.size)],
                    ],
                },
                { warning: 'The current database contents will be lost. You can create a backup of the current state first.' },
            ],
            buttons: [
                { label: 'Replace database', value: true, danger: true },
                { label: 'Cancel', value: false, cancel: true, default: true },
            ],
        });
        if (answer.value !== true) return;
        await runWithProgress('Database restore', 'Restoring ' + name + '…', file, function () {
            return api('/api/db/restore?db=' + encodeURIComponent(name) +
                       '&file=' + encodeURIComponent(file), { method: 'POST' });
        }, 'Restore failed: ');
    }

    function toggleDetails(name) {
        const el = document.getElementById('omn-dbb-details-' + name);
        el.style.display = el.style.display === 'none' ? 'block' : 'none';
    }

    // The actions of this page. See OMN.action in omn-go-core.js. data-arg
    // holds the name of the database, and data-file holds the backup file.
    window.OMN.action('dbb-details', function (el) { toggleDetails(el.getAttribute('data-arg')); });
    window.OMN.action('dbb-backup', function (el) { return backupNow(el.getAttribute('data-arg')); });
    window.OMN.action('dbb-restore', function (el) {
        return restore(el.getAttribute('data-arg'), el.getAttribute('data-file'));
    });

    function render(data) {
        lastData = data;
        (data.databases || []).forEach(function (db) { if (!db.backups) db.backups = []; });
        const out = ['<div class="omn-dbb-meta" style="padding:0.3em 0.2em;">This device: <b>' +
                     esc(data.hostname) + '</b> · keeping newest ' + data.prune_depth +
                     ' backup(s) per database (Config page)</div>'];
        if (!data.databases || data.databases.length === 0) {
            out.push('<p>No databases yet. A database appears here after a note script first uses one.</p>');
        }
        (data.databases || []).forEach(function (db) {
            const st = STATE[db.state] || STATE.none;
            out.push(
                '<div class="omn-dbb-row">' +
                '<span class="omn-dbb-dot ' + st.cls + '" title="' + esc(st.hint) + '"></span>' +
                '<span class="omn-dbb-name">' + esc(db.name) + '</span>' +
                '<span class="omn-dbb-state" title="' + esc(st.hint) + '">' + st.label + '</span>' +
                '<span class="omn-dbb-meta">' + (db.sqlite_exists ? kb(db.size) + ' · ' + ts(db.mtime) : 'no .sqlite file') +
                ' · ' + db.backups.length + ' backup(s)</span>' +
                '<span style="flex:1"></span>' +
                '<button type="button" class="omn-dbb-btn" data-action="dbb-details" data-arg="' + esc(db.name) + '">Details</button>' +
                (db.sqlite_exists ? '<button type="button" class="omn-dbb-btn" data-action="dbb-backup" data-arg="' + esc(db.name) + '">Backup now</button>' : '') +
                '</div>');
            const det = ['<div class="omn-dbb-details" id="omn-dbb-details-' + esc(db.name) + '" style="display:none;">'];
            if (db.backups.length === 0) det.push('<div class="omn-dbb-meta">No backups.</div>');
            db.backups.forEach(function (bk) {
                det.push('<div class="omn-dbb-bk">' +
                    '<code class="omn-dbb-code">' + esc(bk.file) + '</code>' +
                    (bk.valid
                        ? '<span class="omn-dbb-meta">created ' + ts(bk.created) + ' on ' + esc(bk.hostname || '?') +
                          ' · ' + bk.objects + ' obj · ' + bk.rows + ' rows · ' + kb(bk.size) + '</span>'
                        : '<span class="omn-dbb-meta omn-dbb-error">invalid: ' + esc(bk.error || '?') + '</span>') +
                    '<span style="flex:1"></span>' +
                    (bk.valid ? '<button type="button" class="omn-dbb-btn" data-action="dbb-restore" data-arg="' + esc(db.name) + '" data-file="' + esc(bk.file) + '">Restore</button>' : '') +
                    '<a href="/db_backup/' + encodeURIComponent(db.name) + '/' + encodeURIComponent(bk.file) +
                    '" class="omn-dbb-meta">view</a>' +
                    '</div>');
            });
            det.push('</div>');
            out.push(det.join(''));
        });
        list.innerHTML = out.join('');
    }

    async function load() {
        try {
            render(await api('/api/db/backups'));
        } catch (e) {
            list.textContent = 'Failed to load: ' + e.message;
        }
    }
    load();
})();
