Title: Test/OMNGo/Fetch
Date: 2026-06-23 00:29:26
Category: Test
Author: Mikhail Basov
Modified: 2026-06-23 00:29:31
Tags: Test

### `fetch()` test
See console

<script type="module">
// Using async/await
async function loadJSON() {
    try {
        const response = await fetch('/json/test.json'); // Relative path to your JSON file
        if (!response.ok) throw new Error('Network response was not ok');
        const data = await response.json();
        console.log(data);
        // Render data to DOM here
    } catch (error) {
        console.error('Fetch error:', error);
    }
}
loadJSON();;
</script>

