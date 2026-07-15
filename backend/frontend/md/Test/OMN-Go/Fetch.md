Title: Test/OMNGo/Fetch
Date: 2026-06-23 00:29:26
Category: Test
Author: Mikhail Basov
Modified: 2026-07-09 21:59:56
Tags: Test

### `fetch()` test
See console
<div id="fetchStatus">Waiting data ...</div>
<script type="module">
// Using async/await
async function loadJSON() {
    try {
        const response = await fetch('/json/test.json'); // Relative path to your JSON file
        if (!response.ok) throw new Error('Network response was not ok');
        const data = await response.json();
        console.log(data);
        document.querySelector('#fetchStatus').innerHTML=`test: <strong>${data.test}</strong>`;
    } catch (error) {
        console.error('Fetch error:', error);
    }
}
loadJSON();;
</script>

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>


