Title: Test/OMN-Go/SetStatus
Date: 2026-06-24 14:37:29
Category: Test
Author: Mikhail Basov
Tags: Test
Modified: 2026-06-24 15:09:56

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