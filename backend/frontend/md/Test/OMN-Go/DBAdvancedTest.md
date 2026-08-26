Title: DB advanced test
Date: 2026-08-06 12:00:00
Category: Test
Tags: JavaScript, DB, Test, OMN-Go, OMN-Go user

Split out of [DB test](DBTest) — the basic `db.exec` counter lives there.

#### db.batch() — one transaction, both rows or neither
<div id="batchResult">...</div>
<button id="batchBump">Bump both counters atomically</button>

<script type="module">
{
    const bdb = omnGoOpenDatabase('local-dbtest_extras');
    async function renderBatch() {
        await bdb.exec(`CREATE TABLE IF NOT EXISTS pair (
            k TEXT PRIMARY KEY, n INTEGER NOT NULL DEFAULT 0)`);
        await bdb.exec("INSERT OR IGNORE INTO pair (k, n) VALUES ('a', 0), ('b', 0)");
        const r = await bdb.exec('SELECT k, n FROM pair ORDER BY k');
        document.querySelector('#batchResult').innerHTML =
            r.rows._array.map((row) => `${row.k}=${row.n}`).join(', ');
    }
    document.querySelector('#batchBump').addEventListener('click', async () => {
        // Both statements commit together or not at all.
        await bdb.batch([
            ["UPDATE pair SET n = n + 1 WHERE k = 'a'", []],
            ["UPDATE pair SET n = n + 1 WHERE k = 'b'", []]
        ]);
        renderBatch();
    });
    renderBatch().catch((err) => {
        document.querySelector('#batchResult').innerHTML = 'Could not open the database.';
        console.error(err);
    });
}
</script>

#### WebSQL shim (`openDatabase`)
<div id="webSqlResult">...</div>

<script type="module">
{
    // Old-style OMN note scripts using window.openDatabase keep working.
    // Self-sufficient: this code creates and fills the same table as the
    // batch demo above. This section works before that demo runs.
    const wdb = openDatabase('local-dbtest_extras', '1.0', 'DB test extras', 1);
    wdb.transaction((tx) => {
        tx.executeSql('CREATE TABLE IF NOT EXISTS pair (k TEXT PRIMARY KEY, n INTEGER NOT NULL DEFAULT 0)');
        tx.executeSql("INSERT OR IGNORE INTO pair (k, n) VALUES ('a', 0), ('b', 0)");
        tx.executeSql('SELECT k, n FROM pair ORDER BY k', [], (tx, res) => {
            const rows = [];
            for (let i = 0; i < res.rows.length; i++) rows.push(res.rows.item(i));
            document.querySelector('#webSqlResult').innerHTML =
                rows.map((row) => `${row.k}=${row.n}`).join(', ') || 'No rows yet.';
        });
    }, (err) => {
        document.querySelector('#webSqlResult').innerHTML = 'Could not open the database.';
        console.error(err);
    });
}
</script>

- - -

<span id="local_counter">...</span>
<script type="module" src="/js/local_counter.js"></script>
