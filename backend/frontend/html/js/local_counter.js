(async function() {
    const db = omnGoOpenDatabase('local-page_counters');
    await db.exec(`CREATE TABLE IF NOT EXISTS hits (
        page TEXT PRIMARY KEY,
        count INTEGER NOT NULL DEFAULT 0
    )`);

    // Atomic upsert - safe even if two tabs load this page at once.
    await db.exec(
        `INSERT INTO hits (page, count) VALUES (?, 1)
         ON CONFLICT(page) DO UPDATE SET count = count + 1`,
        [PageName]
    );

    const r = await db.exec('SELECT count FROM hits WHERE page = ?', [PageName]);
    document.getElementById('local_counter').textContent =
        'Viewed ' + r.rows._array[0].count + ' time(s)';
})();
