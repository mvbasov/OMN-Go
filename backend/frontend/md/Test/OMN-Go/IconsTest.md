Title: Icons test
Date: 2026-08-06 12:00:00
Category: Test
Tags: JavaScript, CSS, Material icons, Test, OMN-Go, OMN-Go user

Bundled Material Icons (no network needed) — `<i class="material-icons">name</i>`.

<style>
.icons-test-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(6rem, 1fr));
  gap: 0.75rem;
  text-align: center;
}
.icons-test-item {
  background: var(--bg-surface);
  border: 1px solid var(--border-card);
  border-radius: 8px;
  padding: 0.6rem 0.25rem;
}
.icons-test-item .material-icons { 
  font-size: 1.8rem;
  color: var(--accent);
}
.icons-test-item .icons-test-name {
  display: block;
  font-size: 0.75em;
  color: var(--text-muted);
  margin-top: 0.25rem;
}
</style>

<div id="iconsGrid" class="icons-test-grid">...</div>

<script type="module">
{
  const names = ['add', 'delete', 'edit', 'check_circle', 'radio_button_unchecked',
    'bookmark_add', 'insert_comment', 'refresh', 'search', 'expand_more',
    'today', 'local_fire_department', 'home'];
  document.querySelector('#iconsGrid').innerHTML = names.map((n) =>
    `<div class="icons-test-item"><i class="material-icons">${n}</i><span class="icons-test-name">${n}</span></div>`
  ).join('');
}
</script>

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>
