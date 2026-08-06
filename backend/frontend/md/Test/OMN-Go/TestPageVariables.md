Title: Test page variables
Date: 2023-01-27 22:47:02
Category: Test
Tags: JavaScript, Test, OMN-Go, OMN-Go user

|Variable|Value|
|--------|:---:|
|PackageName|<span id="pkg_name">-</span>|
|PageName |<span id="page_name">-</span>|
|Title|<span id="page_title">-</span>|
|currentNote|<span id="cur_note">-</span>|

#### Page meta
<div id="metaTable">...</div>

<script type="module">
{
  const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[c]));
  // Fill page variable table
  if (typeof PackageName !== 'undefined' && PackageName)
    document.querySelector('#pkg_name').innerHTML = esc(PackageName);
  if (typeof PageName !== 'undefined' && PageName)
    document.querySelector('#page_name').innerHTML = esc(PageName);
  if (typeof Title !== 'undefined' && Title)
    document.querySelector('#page_title').innerHTML = esc(Title);
  if (typeof currentNote !== 'undefined' && currentNote)
    document.querySelector('#cur_note').innerHTML = esc(currentNote);
  const statusDiv = document.querySelector('#status');
  if (statusDiv && !statusDiv.querySelector('.tpv-status-note')) {
    statusDiv.style.display = 'block';
    const note = document.createElement('span');
    note.className = 'tpv-status-note';
    note.textContent = 'Status present';
    statusDiv.appendChild(note);
  }
  // Build the HTML page meta table - replace, never append, since this
  // script re-runs on every view of the (cached) compiled page.
  const pageMetas = document.getElementsByTagName('meta');
  const rows = Array.from(pageMetas).map((m) => {
    const metaName = m.getAttribute('name') == null ? 'http-equiv*' : m.getAttribute('name');
    return `<tr><td>${esc(metaName)}</td><td>${esc(m.getAttribute('content') || '')}</td></tr>`;
  }).join('');
  document.querySelector('#metaTable').innerHTML =
    `<table><thead><tr><th>Meta</th><th>Value</th></tr></thead><tbody>${rows}</tbody></table>` +
    '<p><code>* http-equiv</code> is not a meta name — it is the meta property itself.</p>';
}
</script>
