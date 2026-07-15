Title: Databases
Date: 2026-07-15 12:00:00
Category: System
Tags: Database, Backup, SQL

# Databases in Notes

OMN-Go gives a note's own `<script>` block access to a real SQL database,
server-side, replacing the browsers' now-removed WebSQL API
(`window.openDatabase`). This page shows both the modern API and a
drop-in shim for old WebSQL-style code, then explains how to back up,
move and restore a database with the [Database Backups](#database-backups)
page.

## Where the data lives — read this first

Each database is a plain SQLite file at `db/<name>.sqlite` inside this
device's storage directory, created the first time a note uses that name.

- **Shared** between everything currently talking to *this one running
  server* — two browser tabs open to the same desktop instance, or your
  phone and laptop both connected to one device via
  [LAN sharing](UserManual#sharing-on-the-lan), all see the same rows
  immediately.
- **The live `.sqlite` file itself is never synced** between separate
  installs — `db/` is deliberately excluded from git (it is in
  `.gitignore`), the same way `config.json` is, so your Android app's own
  server and your desktop app's own server each keep their own `db/`
  folder. To move a database's *content* between devices you take a
  **backup** and let it travel with your notes through git — see
  [Database Backups](#database-backups) below.
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

## Database Backups

Because the live `.sqlite` file is never synced, moving a database's
content between devices — or simply keeping a safety copy — is done with
**manual backups**. A backup is a single self-contained snapshot file you
create when you want one; it then travels with your notes through the same
git sync you already use for pages.

### The Database Backups page

Open the [Config](Config) page and press the **Database Backups** button
at the top. (There is no header button for it — database management is an
occasional task, so it lives one click inside Config.)

The page lists every database that has either a live `.sqlite` file or at
least one backup. Each row shows a coloured status dot:

- 🟢 **in sync** — the database matches its newest backup.
- 🟡 **not backed up** — the database has changes newer than any backup.
  Press **Backup now** to capture them.
- 🔵 **backup newer** — the newest backup is newer than the database
  (typically it was just pulled from another device). Consider **Restore**.
- 🔵 **no database** — backups exist but no `.sqlite` file does yet (a
  fresh device right after a pull). It is restored automatically the first
  time a note opens it, or you can restore it here by hand.
- ⚪ **no backups** — nothing has been backed up for this database yet.
- 🔴 **invalid backup** — the newest backup file cannot be parsed (damaged
  or left with git conflict markers).

The dot is only a hint computed from file timestamps; it never triggers
any action on its own.

Expand a row (**Details**) to see each backup's creation time, the device
(hostname) that made it, its object and row counts and size, with a
**Restore** button and a plain **download** link per backup.

### Making a backup

Press **Backup now** on a database's row. OMN-Go writes one snapshot file
to `html/db_backup/<db>/<timestamp>_<hostname>.jsonl` and refreshes the
status dot to green. Because that file lives under `html/`, it is a normal
tracked file: your next <i class="material-icons">cloud_upload</i> Upload
commits and pushes it like any note. On another device, pull as usual and
the backup appears on that device's Database Backups page, ready to
restore.

A backup captures the **whole database** in one internally-consistent
file — every table, index, view and trigger, the `AUTOINCREMENT`
counters, and every row (including `BLOB` columns, and large integers,
which are all preserved exactly). Unlike the earlier per-table export,
nothing about the schema is silently dropped.

### Restoring

Press **Restore** on the backup you want. Restore **fully replaces** the
database: it rebuilds the whole thing from the snapshot in one atomic
step, so the current contents are lost. A confirmation dialog shows what
you are replacing versus what the backup contains; you can always press
**Backup now** first to keep the current state. There is no row-level
merge — an honest full replace beats a silent, ambiguous partial one (the
same trade-off [Force Pull](UserManual#git-synchronization) makes for
notes). If a backup file is damaged (for example a git conflict marker got
written into it), the restore is refused whole and the live database is
left untouched.

### Fresh devices restore themselves

The one thing that happens automatically: if a database has backups but no
`.sqlite` file at all — exactly the state of a brand-new device right after
it clones or pulls your notes — then the first time a note opens that
database, OMN-Go restores its newest backup for you. There is nothing to
lose in this case (no local data exists yet), so no confirmation is asked.
Every other restore is manual.

### How many backups are kept

Each database keeps its most recent **Backup Prune Depth** backups
(default **3**, changeable on the [Config](Config) page). Creating a new
backup beyond that count deletes the oldest one; the page warns you before
a backup would prune. For a synced database the pruned file still exists in
git history, so it is recoverable; for a `local-` database (below) it is
not.

### `local-` databases: on-device only

Give a database a name starting with **`local-`** (e.g.
`local-scratchpad`) and its backups are written to disk normally — so you
still have a safety copy — but are excluded from git, the same way
`/md/local/` pages are. Use this for per-device data (drafts,
device-specific caches) you don't want travelling to other devices. Note
that pruning a `local-` backup is final, since nothing is in git history
behind it.

### Naming your devices

Each backup filename ends with this device's **Hostname**, set on the
[Config](Config) page, so backups from different devices never collide and
you can tell at a glance which machine made one. The default is the
operating system's hostname; on Android that is usually unhelpful, so set
a short label like `phone` or `tablet` once per device.

### Importing an existing SQL dump

To load data from somewhere else — a `sqlite3 .dump`, or the output of the
old `websqldump.js` WebSQL exporter — use the [SQL Import](SQLImport) note.
It executes the dump into a database of your choosing (creating it if
needed) via the same `/api/sql` interface note scripts use, then you press
**Backup now** on the Database Backups page to snapshot the result.

### For scripting

The page's actions are plain admin-only HTTP endpoints, if you'd rather
drive them from `curl` or another tool:

- `POST /api/db/backup?db=<name>` — create a backup.
- `GET  /api/db/backups` — list every database and its backups (JSON).
- `POST /api/db/restore?db=<name>&file=<backup-file>` — restore that backup.

## Limits and errors

- Requests to the SQL API are capped at **1 MB** of JSON body and **500
  statements** per call — comfortably enough for interactive note scripts;
  if a script needs more, split the work across a few calls rather than one
  giant batch.
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

- [SQL Import](SQLImport) — load an existing SQL dump into a database.
- [User Manual](UserManual) — general page authoring, links, and the
  other globals (`PageName`, `PAGE_EXT`, ...) available to note scripts.
- [Raw HTML and JavaScript in pages](UserManual#raw-html-and-javascript-in-pages)
  and [ScriptRules](ScriptRules) — the scoping rules that keep one page's
  `<script>` block from colliding with another's.
