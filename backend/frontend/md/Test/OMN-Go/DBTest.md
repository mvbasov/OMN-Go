Title: DB test
Date: 2026-07-18 13:50:14
Category: Test
Tags: JavaScript, DB, Test, OMN-Go, OMN-Go user

#### Local counters
<div id="cnt_stat">Loading…</div>

<script type="module">
{
    const escapeHtml = (s) =>
        String(s).replace(/[&<>"']/g, (c) => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        }[c]));
    async function localCounterStat(element) {
        const cnt_db = omnGoOpenDatabase('local-page_counters');
        // Create the table here too - this page may be the first thing to
        // touch it on a fresh device, before local_counter.js below has run.
        await cnt_db.exec(`CREATE TABLE IF NOT EXISTS hits (
            page TEXT PRIMARY KEY,
            count INTEGER NOT NULL DEFAULT 0
        )`);
        const r = await cnt_db.exec(
            'SELECT page, count FROM hits ORDER BY count DESC'
        );
        const rows = r.rows._array;
        if (!rows.length) {
            element.innerHTML = '<p>No pages counted yet.</p>';
            return;
        }
        let resTable = '<table class="local-counter-stats">' +
            '<thead><tr><th>Page</th><th>Views</th></tr></thead><tbody>';
        for (const row of rows) {
            resTable += `<tr><td>${escapeHtml(row.page)}</td><td>${row.count}</td></tr>`;
        }
        resTable += '</tbody></table>';
        element.innerHTML = resTable;
    }
    localCounterStat(document.querySelector('#cnt_stat')).catch((err) => {
        document.querySelector('#cnt_stat').innerHTML =
            '<p>Could not open the database. Admin-only; a LAN guest can\'t use this page.</p>';
        console.error(err);
    });
}
</script>

See [DB advanced test](DBAdvancedTest) for `db.batch()` and the WebSQL shim.

- - -

<span id="local_counter">...</span>
<script type="module" src="/js/local_counter.js"></script>
