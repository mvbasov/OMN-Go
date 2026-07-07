Title: Databases in Notes
Date: 2026-07-07 14:00:00
Category: System
Tags: database, sqlite, advanced

# Databases in Notes

OMN-Go gives a note's own `<script>` block access to a real SQL database,
server-side, replacing the browsers' now-removed WebSQL API
(`window.openDatabase`). This page shows both the modern API and a
drop-in shim for old WebSQL-style code, with runnable examples.

## Where the data lives — read this first

Each database is a plain SQLite file at `db/<name>.sqlite` inside this
device's storage directory, created the first time a note uses that name.

- **Shared** between everything currently talking to *this one running
  server* — two browser tabs open to the same desktop instance, or your
  phone and laptop both connected to one device via
  [LAN sharing](UserManual#sharing-on-the-lan), all see the same rows
  immediately.
- **Not synced** between separate installs. `db/` is deliberately excluded
  from git sync (it is in `.gitignore`), the same way `config.json` is —
  so your Android app's own server and your desktop app's own server each
  keep their *own* `db/` folder. If you want a database's contents to
  travel with your notes across devices, export/import it explicitly (see
  [Backing up or moving a database](#backing-up-or-moving-a-database)
  below) rather than assuming it syncs like Markdown does.
- **Admin-only.** A guest connected over LAN cannot call the database API,
  even though they can read pages.

## Quick start

```html
<script>
(async function() {
    const db = omnGoOpenDatabase('todo');

    await db.exec(`CREATE TABLE IF NOT EXISTS items (
        id INTEGER PRIMARY KEY,
        text TEXT NOT NULL,
        done INTEGER DEFAULT 0
    )`);

    const result = await db.exec('SELECT * FROM items ORDER BY id');
    result.rows._array.forEach(row => {
        console.log(row.id, row.text, row.done ? '[done]' : '');
    });
})();
</script>
```

`omnGoOpenDatabase(name)` returns a handle immediately (no connection
setup to await); every method on it returns a `Promise`. `name` may only
contain letters, digits, `_` and `-` (max 64 characters) — it becomes part
of a filename.

## Modern API

### `db.exec(sql, args)`

Runs one statement. `args` (optional) are bound as positional `?`
placeholders — always prefer this over string-concatenating values into
the SQL, the same reason you'd avoid it in any language.

```js
const db = omnGoOpenDatabase('todo');

// INSERT / UPDATE / DELETE / CREATE - no rows come back
const ins = await db.exec('INSERT INTO items (text) VALUES (?)', ['Buy milk']);
console.log('new row id:', ins.insertId, 'rows changed:', ins.rowsAffected);

// SELECT - rows come back
const sel = await db.exec('SELECT id, text, done FROM items WHERE done = ?', [0]);
console.log(sel.rows.length, 'open items');
for (let i = 0; i < sel.rows.length; i++) {
    console.log(sel.rows.item(i).text);
}
// or, more convenient in modern code:
sel.rows._array.forEach(row => console.log(row.text));
```

### `db.batch(statements)` — atomic multi-statement writes

Every statement in the array runs inside **one transaction**: if any of
them fails, none of them take effect. Use this whenever two or more
writes must succeed or fail together (moving an item between tables,
maintaining a counter alongside a row, ...).

```js
const db = omnGoOpenDatabase('todo');

await db.batch([
    ['UPDATE items SET done = 1 WHERE id = ?', [3]],
    ['INSERT INTO log (message) VALUES (?)', ['Completed item 3']]
]);
// Either both rows changed, or (e.g. if the "log" table doesn't
// exist) neither did - the UPDATE above is rolled back too.
```

`db.exec()` on its own is also atomic (it's a batch of one), so a single
`INSERT` or `UPDATE` never needs wrapping in `batch()` for that reason
alone — reach for `batch()` specifically when you need *multiple*
statements to rise or fall together.

### Reading results

Every result (from `exec` or each entry from `batch`) has this shape:

```js
{
    insertId:     7,           // last INSERT's rowid (0 for non-INSERT)
    rowsAffected: 1,           // rows changed by INSERT/UPDATE/DELETE
    rows: {
        length: 2,
        item: (i) => ({...}), // WebSQL-style accessor
        _array: [ {...}, {...} ]  // plain array, easiest for forEach/map
    }
}
```

## WebSQL-compatible shim

If you're pasting in an old note script written against the original
`window.openDatabase`, it should work unchanged:

```html
<script>
var db = openDatabase('todo', '1.0', 'Todo list', 2 * 1024 * 1024);

db.transaction(function(tx) {
    tx.executeSql('CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, text TEXT)');
    tx.executeSql('INSERT INTO items (text) VALUES (?)', ['Water the plants']);
    tx.executeSql('SELECT * FROM items', [], function(tx, results) {
        for (let i = 0; i < results.rows.length; i++) {
            console.log(results.rows.item(i).text);
        }
    }, function(tx, error) {
        console.error('query failed:', error);
        return false;
    });
});
</script>
```

`version`, `displayName` and `size` are accepted (for call-shape
compatibility) and ignored — there is no version negotiation or storage
quota to configure server-side.

**One behavioral difference from real WebSQL**, worth knowing if a script
does something clever: statements queued synchronously inside the
`transaction()` callback run as one atomic server-side transaction, same
as real WebSQL. But if a statement is queued *from inside a success
callback* (`tx.executeSql(...)` called from within another statement's
`okCb`), that new statement runs as a **separate**, follow-up transaction
rather than joining the original one. In practice this only matters for
scripts that build a chain of dependent inserts entirely from within
callbacks and expect the whole chain to roll back together; if you need
that guarantee, use [`db.batch()`](#dbbatchstatements--atomic-multi-statement-writes)
instead, with all statements listed up front.

## A complete example: a note-local counter

A minimal but realistic pattern — a page view counter stored in its own
database, rendered into the page on load:

```html
<div id="counter">...</div>
<script>
(async function() {
    const db = omnGoOpenDatabase('page_counters');
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
    document.getElementById('counter').textContent =
        'Viewed ' + r.rows._array[0].count + ' time(s)';
})();
</script>
```

`PageName` here is the global the page shell already exposes (see
[Buttons and shortcuts inside a page](UserManual#buttons-and-shortcuts-inside-a-page)
for other globals available to note scripts) — using it as the primary
key means every page can share one `page_counters` database instead of
each page needing its own.

## Backing up or moving a database

Because `db/*.sqlite` files aren't synced, moving one to another device is
a manual file operation:

1. Locate it in the storage directory (see
   [First run and where your data lives](UserManual#first-run-and-where-your-data-lives))
   under `db/<name>.sqlite`.
2. Copy that single file to the same relative path on the other device.
3. Restart the app (or just reopen the note) so the copied file is picked
   up on next access.

There is no in-app export/import UI for this yet — it's a plain file copy
today.

## Limits and errors

- Requests are capped at **1 MB** of JSON body and **500 statements** per
  call — comfortably enough for interactive note scripts; if a script
  needs more, split the work across a few calls rather than one giant
  batch.
- A rejected batch reports which statement failed:

  ```js
  try {
      await db.batch([...]);
  } catch (e) {
      console.error(e.message); // includes "(statement #N)" when applicable
  }
  ```

- SQL runs with the same privileges as any other admin action in
  OMN-Go — a database is only as trustworthy as the note script that
  writes to it. Don't paste database code from notes you don't trust.

## See also

- [User Manual](UserManual) — general page authoring, links, and the
  other globals (`PageName`, `PAGE_EXT`, ...) available to note scripts.
- [Raw HTML and JavaScript in pages](UserManual#raw-html-and-javascript-in-pages)
  and [ScriptRules](ScriptRules) — the scoping rules that keep one page's
  `<script>` block from colliding with another's.
