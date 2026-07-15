Title: Test/OMN-Go/SetStatus
Date: 2026-06-24 14:37:29
Category: Test
Author: Mikhail Basov
Tags: Test
Modified: 2026-07-09 22:01:54

**see the status at the bottom of the page**

<style>
  .small-circle {
    width: 1rem;
    height: 1rem;
    background-color: #3498db;
    border-radius: 50%;
  }
</style>

<script type="module">
  let s = document.querySelector('#status');
  let d = document.createElement('span');
  d.textContent = '@';
  d.classList.add('small-circle');
  s.insertBefore(d, s.firstChild);
</script>

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>

