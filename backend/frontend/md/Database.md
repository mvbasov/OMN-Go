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
- **The live `.sqlite` file itself is not synced** between separate
  installs — `db/` is deliberately excluded from git (it's in
  `.gitignore`), the same way `config.json` is, so your Android app's own
  server and your desktop app's own server each keep their own `db/`
  folder. Its *content* still travels with your notes through git, just
  via an automatically-maintained JSON mirror rather than the binary file
  itself — see
  [Backing up, syncing, and restoring a database via git](#backing-up-syncing-and-restoring-a-database-via-git)
  below.
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

## Backing up, syncing, and restoring a database via git

`db/*.sqlite` files themselves are never synced (as above), but OMN-Go
maintains a **git-trackable JSON mirror** of every database's schema and
data, so a database's *content* can travel with your notes through the
same git sync you already use for pages — no manual file copying needed
for the common case.

### How it works automatically

- **Backup happens right before every push.** Whenever you press
  <i class="material-icons">cloud_upload</i> Upload, OMN-Go refreshes the
  JSON mirror for every database that changed since the last push, then
  includes those files in the commit — the same "regenerate the cache
  right before it's needed" timing used for compiling `.md` to `.html`.
  You don't need to remember to export anything.
- **If you cancel the commit dialog, nothing was written.** The commit
  preview computes what *would* change without touching any file, so
  canceling leaves your JSON backups exactly as they were.
- **Restore happens automatically too**, the next time a note opens that
  database after a pull brought in newer JSON than the local `.sqlite`
  file — you don't need to do anything after pulling for the data to
  catch up.
- The mirror lives at `html/db_json/<db>_<table>_sch.json` (schema) and
  `..._data.json` (data, one compact JSON row per line so a git diff on a
  single changed row stays a single-line diff) — one pair per table, plus
  a schema-only file for each view and trigger.

### `local-` databases: backed up, never synced

Give a database a name starting with **`local-`** (e.g.
`local-scratchpad`) and its JSON mirror is still written to disk on every
push — so you still have a backup — but is excluded from git entirely, the
same way `/md/local/` pages are. Use this for per-device data (drafts,
device-specific caches) you don't want traveling to other devices.

### Restore always fully replaces — it never merges

Restoring drops and recreates each table (and its data) from the JSON
files. There is no row-level merge: any local writes made since the last
export that never made it into the JSON are lost when you restore. This
is the same risk profile [Force Pull](UserManual#git-synchronization)
already has for notes — an honest full replace beats a silent, ambiguous
partial merge (not every table has a clean way to match old rows to new
ones).

Restoring a whole database recreates, in order: all tables, then all
views, then all tables' data, then all triggers — triggers deliberately
last, so bulk-loading a table's saved rows doesn't fire its own `AFTER
INSERT` triggers as if they were new activity.

### Forcing it manually

Both directions are also available on demand — from a note's own script,
or from any HTTP client:

```js
const db = omnGoOpenDatabase('todo');

await db.exportBackup();          // export every table/view/trigger now
await db.exportBackup('items');   // export just the "items" table

await db.restoreBackup();         // restore the whole database from JSON
await db.restoreBackup('items');  // restore ONLY "items" - strict: its
                                   // triggers are not re-created by this;
                                   // omit the argument to restore everything
```

Add a button to a page for either action:

```html
<button onclick="omnGoOpenDatabase('todo').exportBackup()
    .then(f => alert('Backed up: ' + f.length + ' file(s) changed'))
    .catch(e => alert('Backup failed: ' + e.message))">
  <i class="material-icons">save</i> Backup now
</button>

<button onclick="if (confirm('Replace current data with the last backup?'))
    omnGoOpenDatabase('todo').restoreBackup()
        .then(() => alert('Restored - reload to see it'))
        .catch(e => alert('Restore failed: ' + e.message))">
  <i class="material-icons">restore</i> Restore from backup
</button>
```

These map directly to `POST /api/db/export?db=<name>[&table=<name>]` and
`POST /api/db/restore?db=<name>[&table=<name>]`, both admin-only, if you'd
rather call them from `curl` or another tool.

### What isn't backed up this way

Tables and views make the trip; **BLOB columns do not** — the JSON export
skips (and logs an error for) any table containing one, since JSON has no
binary type. If a note needs binary data backed up, base64-encode it in
JavaScript before storing it in a `TEXT` column — the JSON mirror handles
that natively, since it's already plain text as far as SQLite and this
export are concerned:

```js
const bytes = new Uint8Array([1, 2, 3]);
const b64 = btoa(String.fromCharCode(...bytes));
await db.exec('INSERT INTO files(name, content) VALUES (?, ?)', ['x.bin', b64]);
// content is a normal TEXT column - now included in the JSON backup
```

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
