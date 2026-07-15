Title: Link test
Date: 2026-07-04 20:52:56
Modified: 2026-07-12 10:59:11
Category: Test
Tags: Test
Author: Mikhail Basov

### URL paraneters
<pre id="result">
...
</pre>

- - -

#### URI tests
* [tel:+1-555_5555](tel:+1-555-5555)
* [geo: Eiffel Tower z=15](geo:48.8584,2.2945?z=15)
  * z=1: World view
  * z=5: Landmass / Continent level
  * z=10: City level
  * z=15: Neighborhood level
  * z=20: Individual buildings / Roofs
* [geo: Eiffel Tower z=10](geo:48.8584,2.2945?z=10)
* [sms: +1-5555?body=text](sms:+15550199?body=Hello%20there)
* [mailto: name@example.com...](mailto:name@example.com?subject=Subj.&body=Greeting!%0ABody%20here.&cc=cc@example.com&bcc=bcc@example.com)

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


<script>
function parseUrlParams(urlStr) {
  var urlQuery = window.location.search.substring(1);
  //if(urlQuery.length > 0){
    const params = new URLSearchParams(urlQuery);
  //} else {
  //  return;
  //}
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
const testUrl = 'https://example.com';
const parsed = parseUrlParams(testUrl);
document.querySelector('#result').innerHTML=JSON.stringify(parsed);
</script>

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>
