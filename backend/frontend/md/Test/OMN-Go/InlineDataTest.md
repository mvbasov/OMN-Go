Title: Inline data test
Date: 2026-08-06 12:00:00
Category: Test
Tags: JavaScript, DB, Test, OMN-Go, OMN-Go user

The list below is inline data in this note (hand-editable, syncs with your
notes). The ticks are ephemeral state in SQLite (`local-inline_demo`,
per-device). This split — structure in the note, state in the database — is
often the right answer for a checklist.

<script>
window.inlineTestItems = ['Buy milk', 'Write tests', 'Ship it'];
</script>

<div id="inlineList">Loading…</div>

<script type="module">
{
  const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[c]));
  const db = omnGoOpenDatabase('local-inline_demo');
  const list = document.querySelector('#inlineList');
  const items = window.inlineTestItems || [];
  async function render() {
    const r = await db.exec('SELECT item, done FROM ticks');
    const done = new Set(r.rows._array.filter((row) => row.done).map((row) => row.item));
    list.innerHTML = items.map((item) => `
      <label style="display:block;padding:0.4rem 0;">
        <input type="checkbox" data-item="${esc(item)}" ${done.has(item) ? 'checked' : ''} />
        ${esc(item)}
      </label>`).join('');
  }
  list.addEventListener('change', async (e) => {
    const cb = e.target.closest('input[type=checkbox]');
    if (!cb) return;
    await db.exec(
      'INSERT INTO ticks (item, done) VALUES (?, ?) ON CONFLICT(item) DO UPDATE SET done = ?',
      [cb.dataset.item, cb.checked ? 1 : 0, cb.checked ? 1 : 0]
    );
  });
  db.exec('CREATE TABLE IF NOT EXISTS ticks (item TEXT PRIMARY KEY, done INTEGER NOT NULL DEFAULT 0)')
    .then(render)
    .catch((err) => {
      list.innerHTML = '<p>Could not open the database. Admin-only; a LAN guest can\'t use this page.</p>';
      console.error(err);
    });
}
</script>

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>
