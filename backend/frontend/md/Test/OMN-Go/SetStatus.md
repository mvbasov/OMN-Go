Title: Set status
Date: 2026-06-24 14:37:29
Category: Test
Tags: JavaScript, CSS, Test, OMN-Go, OMN-Go user

**see the status at the bottom of the page**

<style>
.setstatus-dot {
  display: inline-block;
  width: 1rem;
  height: 1rem;
  background-color: var(--info);
  border-radius: 50%;
}
</style>

<script type="module">
{
  const s = document.querySelector('#status');
  // Idempotent: the script re-runs on every view of the cached page, so
  // without this check a second view would stack up a second dot.
  if (s && !s.querySelector('.setstatus-dot')) {
    const d = document.createElement('span');
    d.classList.add('setstatus-dot');
    s.insertBefore(d, s.firstChild);
  }
}
</script>

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>

