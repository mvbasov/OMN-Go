Title: Theme tokens test
Date: 2026-08-06 12:00:00
Category: Test
Tags: JavaScript, CSS, Test, OMN-Go, OMN-Go user

Live swatches of the design tokens from `omn-go-core.css`. Flip the app's theme and reload to see these move — never hardcode a colour, use one of these instead.

<style>
.tt-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(7.5rem, 1fr));
  gap: 0.6rem;
} .tt-swatch {
  border: 1px solid var(--omn-border-card);
  border-radius: 8px;
  padding: 0.5rem;
  font-size: 0.8em;
} .tt-swatch code {
  display: block;
  font-size: 0.75em;
  color: var(--omn-text-muted);
  margin-top: 0.3rem;
  word-break: break-all;
}
</style>

<h4>Surfaces &amp; semantic</h4>
<div id="ttColor" class="tt-grid">...</div>

<h4>Text</h4>
<div id="ttText" class="tt-grid">...</div>

<script type="module">
{
  const colorTokens = ['--omn-bg', '--omn-bg-surface', '--omn-bg-band', '--omn-bg-inset',
    '--omn-bg-hover', '--omn-accent', '--omn-success', '--omn-danger', '--omn-info', '--omn-secondary',
    '--omn-border', '--omn-border-card', '--omn-tag-bg', '--omn-table-th-bg'];
  const textTokens = ['--omn-text', '--omn-text-strong', '--omn-text-heading',
    '--omn-text-muted', '--omn-text-faint', '--omn-text-dim'];
  document.querySelector('#ttColor').innerHTML = colorTokens.map((t) =>
    `<div class="tt-swatch" style="background:var(${t});color:var(--omn-text)">Aa<code>${t}</code></div>`
  ).join('');
  document.querySelector('#ttText').innerHTML = textTokens.map((t) =>
    `<div class="tt-swatch" style="background:var(--omn-bg-surface);color:var(${t})">Sample text<code>${t}</code></div>`
  ).join('');
}
</script>

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>
