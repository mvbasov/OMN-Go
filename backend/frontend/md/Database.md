Title: Databases
Date: 2026-07-15 12:00:00
Category: System
Author: Mikhail Basov
Tags: Database, Backup, SQL, OMN-Go, OMN-Go app

# Databases in Notes

OMN-Go gives the `<script>` block of a note access to a real SQL database on the backend. This database replaces the WebSQL API that the browsers removed (`window.openDatabase`). This page shows the modern API and a drop-in shim for old WebSQL-style code. The page then explains how to create a backup of a database, how to move it, and how to restore it. For these tasks you use the [Database Backups](#database-backups) page.

## Where the data lives — read this first

Each database is a SQLite file at `db/<name>.sqlite` in the storage directory of this device. OMN-Go creates the file the first time a note uses that name.

- **Shared.** Everything that talks to *this one running server* uses the
  same database. Two browser tabs open to the same desktop application see
  the same rows immediately. A phone and a laptop that both connect to one
  device through [LAN sharing](UserManual#sharing-on-the-lan) also see the
  same rows immediately.
- **OMN-Go never syncs the live `.sqlite` file** between separate
  installations. The `db/` directory is excluded from git on purpose,
  because it is in `.gitignore`. The `config.json` file is excluded in the
  same way. The server of the Android application and the server of the
  desktop application each keep their own `db/` directory. To move the
  *content* of a database between devices, create a **backup**. The backup
  travels with your notes through git. See
  [Database Backups](#database-backups) below.
- **Admin-only.** A guest that connects over the LAN cannot call the
  database API. A guest can still read pages.

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

`omnGoOpenDatabase(name)` returns a handle immediately. There is no connection setup to await. Every method on the handle returns a `Promise`. The `name` can contain only letters, digits, `_` and `-`, to a maximum of 64 characters. OMN-Go uses the name as part of a filename.

## Modern API

### `db.exec(sql, args)`

This method runs one statement. The optional `args` parameter binds values as positional `?` placeholders. Always use placeholders. Do not concatenate values into the SQL text. Avoid this for the same reason that you avoid it in any other language.

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

Every statement in the array runs inside **one transaction**. If one statement fails, no statement takes effect. Use this method when two or more writes must succeed together or fail together. Examples are a move of an item between tables, or an update of a counter next to a row.

```js
const db = omnGoOpenDatabase('todo');

await db.batch([
    ['UPDATE items SET done = 1 WHERE id = ?', [3]],
    ['INSERT INTO log (message) VALUES (?)', ['Completed item 3']]
]);
// Both rows change together, or no row changes. An absent "log"
// table also rolls back the UPDATE above.
```

`db.exec()` on its own is also atomic, because it is a batch of one statement. For that reason alone, a single `INSERT` or `UPDATE` never needs `batch()`. Use `batch()` when two or more statements must succeed together or fail together.

### Reading results

Every result has this shape. A result comes from `exec`, or it is one entry from `batch`.

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

You can paste in an old note script that uses the original `window.openDatabase`. The script works without changes:

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

OMN-Go accepts `version`, `displayName` and `size` for call-shape compatibility, then ignores them. The backend has no version negotiation and no storage quota to configure.

**One behavioral difference from real WebSQL** exists. A script can queue
statements synchronously inside the `transaction()` callback. Those statements run as one atomic transaction on the backend, the same as real WebSQL.

There is one exception. A script can also queue a statement *from inside a success callback*. In this case the script calls `tx.executeSql(...)` from within the `okCb` of another statement. That new statement runs as a **separate** transaction after the first one. It does not join the original transaction.

This difference matters only for one kind of script. Such a script builds a chain of dependent inserts fully from inside callbacks. It also expects the whole chain to roll back together. If you need that guarantee, use [`db.batch()`](#dbbatchstatements--atomic-multi-statement-writes) instead. List all statements up front.

## A complete example: a note-local counter

The example below is a page view counter. The counter has its own database. The page shows the count each time it loads:

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

`PageName` is a global variable that the page shell already gives to note scripts. For the other globals that note scripts can use, see [Buttons and shortcuts inside a page](UserManual#buttons-and-shortcuts-inside-a-page). The example uses `PageName` as the primary key. Every page can therefore share one `page_counters` database. No page needs its own database.

## Database Backups

OMN-Go never syncs the live `.sqlite` file. Use **manual backups** to move the content of a database between devices. Also create a backup to keep a copy of your data for safety. A backup is one self-contained file that you create when you want it. The backup then travels with your notes through the same git sync that you already use for pages.

### The Database Backups page

Open the [Config](Config) page. Press the **Database Backups** button at the top. The header has no button for this page. Database management is an occasional task, so the button stays one press inside the Config page.

The page lists every database that has a live `.sqlite` file or at least one backup. Each row shows a colored status dot:

- 🟢 **in sync** — the database matches its newest backup.
- 🟡 **not backed up** — the database has changes that are newer than any
  backup. Press **Backup now** to save them.
- 🔵 **backup newer** — the newest backup is newer than the database.
  Usually a pull from another device brought this backup. Press
  **Restore** if you want the content of the backup.
- 🔵 **no database** — backups exist, but no `.sqlite` file exists yet.
  This is the state of a fresh device directly after a pull. OMN-Go
  restores the database automatically the first time a note opens it. You
  can also restore it here by hand.
- ⚪ **no backups** — this database has no backup yet.
- 🔴 **invalid backup** — OMN-Go cannot parse the newest backup file. The
  file is damaged, or it holds git conflict markers.

The dot is only a hint. OMN-Go computes it from the file timestamps. The dot never starts an action on its own.

Press **Details** to expand a row. The row then shows the creation time of each backup, the device (hostname) that created it, its object count, its row count and its size. Each backup also has a **Restore** button and a **view** link. The view link opens the backup file as text, in the same window. On a desktop you save the file from there with the menu of your browser. Press the back button to come back to the page.

### Making a backup

Press **Backup now** on the row of the database. OMN-Go writes one backup file to `html/db_backup/<db>/<timestamp>_<hostname>.jsonl`. OMN-Go then sets the status dot to green.

The file is under `html/`, so it is a normal tracked file. Your next
<i class="material-icons">cloud_upload</i> Upload commits and pushes it
like any note. On another device, pull as usual. The backup then appears on the Database Backups page of that device, ready to restore.

A backup holds the **whole database** in one internally consistent file. It holds every table, index, view and trigger. It holds the `AUTOINCREMENT` counters and every row. `BLOB` columns and large integers keep their exact values. The earlier per-table export dropped parts of the schema without a message. A backup drops no part of the schema.

### Restoring

Press **Restore** on the backup that you want. Restore **fully replaces** the database. OMN-Go rebuilds the whole database from the backup in one atomic step. The current contents are lost.

A confirmation dialog appears. The dialog shows what you replace and what the backup holds. To keep the current state, press **Backup now** before you restore.

There is no row-level merge. An honest full replace is better than a silent, ambiguous partial one. [Force Pull](UserManual#git-synchronization) makes the same trade-off for notes.

A backup file can be damaged. One example is a git conflict marker that git wrote into the file. If a backup file is damaged, OMN-Go refuses the restore whole and leaves the live database untouched.

### Fresh devices restore themselves

One restore happens automatically. A database can have backups but no `.sqlite` file at all. This is exactly the state of a new device directly after it clones or pulls your notes. In this state, OMN-Go restores the newest backup the first time a note opens that database. No local data exists yet, so there is nothing to lose and OMN-Go asks for no confirmation. Every other restore is manual.

### How many backups are kept

Each database keeps its most recent **Backup Prune Depth** backups. The default is **3**. You can change the value on the [Config](Config) page. When you create a backup above that count, OMN-Go deletes the oldest backup. The page warns you before a backup prunes another one.

Git history still holds the pruned file of a synced database, so you can recover it. Git history holds nothing for a `local-` database, which the next section describes.

### `local-` databases: on-device only

Give a database a name that starts with **`local-`**, for example `local-scratchpad`. OMN-Go writes the backups of this database to disk in the normal way, so you still have a copy of the data. OMN-Go excludes these backups from git. Use a `local-` database for the data of one device, such as drafts and device-specific caches. This data does not travel to other devices. When OMN-Go prunes a `local-` backup, that backup is gone for good, because git history holds nothing behind it.

The `local-` name works the same way for each file and each directory, not only for a database. The [User Manual](UserManual) describes the general rule under *Git synchronization*.

### Naming your devices

Each backup filename ends with the **Hostname** of this device. You set the hostname on the [Config](Config) page. Backups from different devices therefore never collide. You can also see which device created each backup.

The default value is the hostname of the operating system. On Android that name is usually not useful. Set a short label such as `phone` or `tablet` one time on each device.

### Importing an existing SQL dump

Use the [SQL Import](SQLImport) note to load data from another source. The source can be a `sqlite3 .dump` file, or the output of the old `websqldump.js` WebSQL exporter. The note executes the dump into the database that you select, and creates that database if it does not exist. The note uses the same `/api/sql` endpoint that note scripts use. To create a backup of the result, press **Backup now** on the Database Backups page.

### For scripting

The actions of the page are plain admin-only HTTP endpoints. You can call them from `curl` or another tool:

- `POST /api/db/backup?db=<name>` — create a backup.
- `GET  /api/db/backups` — list every database and its backups (JSON).
- `POST /api/db/restore?db=<name>&file=<backup-file>` — restore that backup.

## Limits and errors

- OMN-Go limits each call to the SQL API to **1 MB** of JSON body and
  **500 statements**. This limit is sufficient for interactive note
  scripts. If a script needs more, split the work across a few calls. Do
  not send one very large batch.
- When OMN-Go rejects a batch, the error names the statement that failed:

  ```js
  try {
      await db.batch([...]);
  } catch (e) {
      console.error(e.message); // includes "(statement #N)" when applicable
  }
  ```

- SQL runs with the same privileges as any other admin action in OMN-Go.
  A database is only as trustworthy as the note script that writes to it.
  Do not paste database code from a note that you do not trust.

## See also

- [SQL Import](SQLImport) — load an existing SQL dump into a database.
- [User Manual](UserManual) — general page authoring, links, and the
  other globals (`PageName`, `PAGE_EXT`, ...) that note scripts can use.
- [Raw HTML and JavaScript in pages](UserManual#raw-html-and-javascript-in-pages)
  and [ScriptRules](ScriptRules) — the scoping rules that isolate the
  `<script>` block of each page from other pages.
