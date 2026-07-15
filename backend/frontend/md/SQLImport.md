Title: SQL Import
Category: Settings

# SQL Import

Import an SQL dump into a server-side database. Two dialects are understood:

- **sqlite3 `.dump`** output — executed as-is (transaction wrappers and PRAGMAs are stripped; `X'...'` BLOB literals pass through).
- **websqldump.js** output (the old WebSQL export helper) — its `INSERT ... VALUES ("...")` lines use unescaped double quotes, so they are **re-parsed into parameterized statements** instead of being executed as raw SQL. Rows that contained a literal `"` character were already damaged at dump time and are reported, not guessed at.

The dump is applied through the normal `/api/sql` endpoint in batches, each batch one transaction. After a successful import, go to the <a href="/db_backups">Database Backups</a> page and press **Backup now** — the import itself does not create a backup.

<div class="config-panel">
    <div class="config-field">
        <label class="config-label">Target database name</label>
        <input type="text" id="sqlImpDb" class="config-input" placeholder="mydata" />
        <span class="config-hint">Letters, digits, '_' and '-', max 64 chars. The database is created if it does not exist. Importing into an existing database ADDS to it (or fails on conflicts) — it does not clear it first.</span>
    </div>
    <div class="config-field">
        <label class="config-label">SQL dump</label>
        <textarea id="sqlImpText" class="config-input" rows="12" placeholder="Paste the dump here, or load a file below" spellcheck="false"></textarea>
        <input type="file" id="sqlImpFile" accept=".sql,.txt,text/*" />
    </div>
    <div class="config-field">
        <label class="config-label">Dialect</label>
        <select id="sqlImpDialect" class="config-input">
            <option value="auto" selected>Auto-detect</option>
            <option value="sqlite3">sqlite3 .dump (raw SQL)</option>
            <option value="websql">websqldump.js (re-parsed)</option>
        </select>
    </div>
    <div class="config-field config-checkbox-row">
        <input type="checkbox" id="sqlImpNull" checked />
        <label class="config-label">websqldump: treat the value "null" as SQL NULL</label>
    </div>
    <div class="config-field config-save-row">
        <button type="button" class="config-save-btn" onclick="sqlImportRun()">Import</button>
    </div>
    <div class="config-field">
        <pre id="sqlImpLog" style="white-space:pre-wrap;max-height:40vh;overflow:auto;"></pre>
    </div>
</div>

<script>
(function() {
    'use strict';
    var logEl = null;
    function log(msg) {
        if (!logEl) logEl = document.getElementById('sqlImpLog');
        logEl.textContent += msg + '\n';
        logEl.scrollTop = logEl.scrollHeight;
    }

    document.addEventListener('change', function(ev) {
        if (ev.target && ev.target.id === 'sqlImpFile' && ev.target.files[0]) {
            var reader = new FileReader();
            reader.onload = function() {
                document.getElementById('sqlImpText').value = reader.result;
            };
            reader.readAsText(ev.target.files[0]); // local read only - nothing is uploaded
        }
    });

    // Split SQL text into statements on ';', respecting single-quoted
    // strings ('' escape), double-quoted identifiers/strings, X'..' hex
    // literals and "-- " line comments. Good enough for both dump
    // dialects (neither emits /* */ comments).
    function splitStatements(text) {
        var out = [], cur = '', i = 0, n = text.length;
        var inS = false, inD = false, inComment = false;
        while (i < n) {
            var c = text[i];
            if (inComment) {
                if (c === '\n') inComment = false;
                i++; continue;
            }
            if (inS) {
                cur += c;
                if (c === "'") {
                    if (text[i + 1] === "'") { cur += "'"; i++; } else inS = false;
                }
                i++; continue;
            }
            if (inD) {
                cur += c;
                if (c === '"') inD = false;
                i++; continue;
            }
            if (c === '-' && text[i + 1] === '-') { inComment = true; i += 2; continue; }
            if (c === "'") { inS = true; cur += c; i++; continue; }
            if (c === '"') { inD = true; cur += c; i++; continue; }
            if (c === ';') {
                if (cur.trim()) out.push(cur.trim());
                cur = ''; i++; continue;
            }
            cur += c; i++;
        }
        if (cur.trim()) out.push(cur.trim());
        return out;
    }

    function isNoise(stmt) {
        var u = stmt.toUpperCase();
        return u.startsWith('BEGIN') || u.startsWith('COMMIT') ||
               u.startsWith('END') || u.startsWith('PRAGMA');
    }

    function detectDialect(statements) {
        // websqldump renders EVERY value double-quoted: VALUES ("1","x").
        // A sqlite3 dump never puts a double quote right after VALUES(.
        for (var i = 0; i < statements.length; i++) {
            var s = statements[i];
            if (/^INSERT\s/i.test(s)) {
                return /VALUES\s*\(\s*"/i.test(s) ? 'websql' : 'sqlite3';
            }
        }
        return 'sqlite3';
    }

    // Parse one websqldump INSERT into {sql, args}. Values are split on
    // top-level commas; each value is either "..." (no escaping existed
    // at dump time - an embedded '"' makes the row unparseable) or a
    // bare token.
    function parseWebSQLInsert(stmt, nullAsNull) {
        var m = stmt.match(/^INSERT\s+INTO\s+([A-Za-z0-9_"]+)\s*\(([^)]*)\)\s*VALUES\s*\((.*)\)\s*$/is);
        if (!m) throw new Error('unrecognized INSERT shape');
        var table = m[1].replace(/"/g, '');
        var cols = m[2].split(',').map(function(c) { return c.trim().replace(/"/g, ''); });
        var body = m[3];
        var args = [], i = 0, n = body.length;
        while (i < n) {
            while (i < n && (body[i] === ' ' || body[i] === ',')) i++;
            if (i >= n) break;
            if (body[i] === '"') {
                var j = body.indexOf('"', i + 1);
                if (j < 0) throw new Error('unterminated quoted value');
                // A '"' inside the value produced broken output at dump
                // time; the next char after the closing quote must be a
                // separator, otherwise the row is damaged.
                if (j + 1 < n && body[j + 1] !== ',' && body[j + 1] !== ' ') {
                    throw new Error('value contains an unescaped double quote (damaged at dump time)');
                }
                var val = body.slice(i + 1, j);
                args.push(nullAsNull && val === 'null' ? null : val);
                i = j + 1;
            } else {
                var k = body.indexOf(',', i);
                if (k < 0) k = n;
                var tok = body.slice(i, k).trim();
                args.push(tok.toUpperCase() === 'NULL' ? null : tok);
                i = k;
            }
        }
        if (args.length !== cols.length) {
            throw new Error('parsed ' + args.length + ' values for ' + cols.length + ' columns');
        }
        var q = cols.map(function(c) { return '"' + c + '"'; }).join(',');
        var ph = cols.map(function() { return '?'; }).join(',');
        return { sql: 'INSERT INTO "' + table + '" (' + q + ') VALUES (' + ph + ')', args: args };
    }

    async function postBatch(db, statements) {
        var res = await fetch('/api/sql', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ db: db, statements: statements })
        });
        var data = await res.json();
        if (data.status !== 'success') {
            var idx = (data.failed_statement !== undefined && data.failed_statement !== null)
                ? data.failed_statement : -1;
            var e = new Error(data.message || 'SQL error');
            e.failedIndex = idx;
            throw e;
        }
    }

    window.sqlImportRun = async function() {
        logEl = document.getElementById('sqlImpLog');
        logEl.textContent = '';
        var db = document.getElementById('sqlImpDb').value.trim();
        if (!/^[A-Za-z0-9_-]{1,64}$/.test(db)) { log('Invalid database name.'); return; }
        var text = document.getElementById('sqlImpText').value;
        if (!text.trim()) { log('Nothing to import.'); return; }
        var nullAsNull = document.getElementById('sqlImpNull').checked;

        var raw = splitStatements(text).filter(function(s) { return !isNoise(s); });
        if (raw.length === 0) { log('No usable statements found.'); return; }

        var dialect = document.getElementById('sqlImpDialect').value;
        if (dialect === 'auto') {
            dialect = detectDialect(raw);
            log('Detected dialect: ' + dialect);
        }

        var statements = [], skipped = 0;
        for (var i = 0; i < raw.length; i++) {
            var s = raw[i];
            if (dialect === 'websql' && /^INSERT\s/i.test(s)) {
                try {
                    statements.push(parseWebSQLInsert(s, nullAsNull));
                } catch (e) {
                    skipped++;
                    log('SKIPPED damaged row (statement ' + (i + 1) + '): ' + e.message);
                }
            } else {
                statements.push({ sql: s, args: [] });
            }
        }
        log('Importing ' + statements.length + ' statements into "' + db + '"' +
            (skipped ? ' (' + skipped + ' damaged rows skipped)' : '') + ' ...');

        // 400 per batch stays under both /api/sql caps (500 statements,
        // 1 MB body) for typical rows; halve on "too large" style errors.
        var BATCH = 400, done = 0;
        try {
            for (var off = 0; off < statements.length; off += BATCH) {
                var chunk = statements.slice(off, off + BATCH);
                await postBatch(db, chunk);
                done += chunk.length;
                log('  ' + done + ' / ' + statements.length);
            }
        } catch (e) {
            var at = (e.failedIndex >= 0) ? ' at statement ' + (done + e.failedIndex + 1) : '';
            log('FAILED' + at + ': ' + e.message);
            log('Everything in the failed batch was rolled back; earlier batches are already applied.');
            return;
        }
        log('Done. Now open /db_backups and press "Backup now" for "' + db + '".');
    };
})();
</script>
