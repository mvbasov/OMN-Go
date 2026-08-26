Title: Link test
Date: 2026-07-04 20:52:56
Category: Test
Tags: JavaScript, Test, OMN-Go, OMN-Go user

### Live query string
This page's own query string, parsed with `URLSearchParams` — open one of the links under "URL parameters" below and watch it update.
<pre id="result">
...
</pre>

- - -

See [URI schemes test](UriSchemesTest) for `tel:`/`geo:`/`sms:`/`mailto:` links and the Android intent catalog.

#### Anchor
* [LinkTest#url-parameters](LinkTest#url-parameters)

#### Relative/absolute links
* [Console](Console)
* [./Console](./Console)
* [../../Bookmarks](../../Bookmarks)
* [../../local/local](../../local/local)
* [/QuickNotes](/QuickNotes)
* [/json/test.json](/json/test.json)

#### URL parameters
* [msg=Hello%20World!%0AI%20am%20ready](/Test/OMN-Go/LinkTest?msg=Hello%20World!%0AI%20am%20ready.)
* [tags=js&tags=html&tags=css&name=note](LinkTest?tags=js&tags=html&tags=css&name=note)


<script type="module">
{
  function parseUrlParams(search) {
    const params = new URLSearchParams(search);
    const result = {};
    for (const [key, value] of params.entries()) {
      if (result.hasOwnProperty(key)) {
        // If it's already an array, push the new value
        if (Array.isArray(result[key])) {
          result[key].push(value);
        } else {
          // Convert existing single value into an array
          result[key] = [result[key], value];
        }
      } else {
        // First time seeing this key, set as a single value
        result[key] = value;
      }
    }
    return result;
  }
  document.querySelector('#result').innerHTML =
    JSON.stringify(parseUrlParams(window.location.search));
}
</script>

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>
