Title: Test/OMN-Go/DBTest
Date: 2026-07-18 13:50:14
Category: Notes
Author: Mikhail Basov
Tags: Test
Modified: 2026-07-18 13:53:18

#### Local counters
<div id="cnt_stat">...</div>

<script>
async function localCounterStat(element) {
    const cnt_db = omnGoOpenDatabase('local-page_counters');

    const r = await cnt_db.exec(
        'SELECT page, count FROM hits ORDER BY count DESC'
    );
    const rows = r.rows._array;

    const escapeHtml = (s) =>
        String(s).replace(/[&<>"']/g, (c) => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        }[c]));

    let resTable = '<table class="local-counter-stats">' +
        '<thead><tr><th>Page</th><th>Views</th></tr></thead><tbody>';

    for (const row of rows) {
        resTable += `<tr><td>${escapeHtml(row.page)}</td><td>${row.count}</td></tr>`;
    }

    resTable += '</tbody></table>';

    element.innerHTML = resTable;
}
localCounterStat(cnt_stat);
</script>

- - -

<span id="local_counter">...</span>
<script type="module" src="/js/local_counter.js"></script>