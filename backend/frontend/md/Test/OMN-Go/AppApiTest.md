Title: App API test
Date: 2026-08-06 12:00:00
Category: Test
Tags: JavaScript, Test, OMN-Go, OMN-Go user

App-level JS a page can call: progress overlay, forced refresh, search-style
highlighting.

<button id="aatProgress">Show progress overlay</button>
<button id="aatRefresh">refreshPage()</button>
<br/><br/>
<input id="aatTerm" type="text" placeholder="word on this page" value="progress" />
<button id="aatHighlight">Highlight</button>
<button id="aatClear">Clear highlights</button>

<script type="module">
{
  document.querySelector('#aatProgress').addEventListener('click', () => {
    window.OMNProgress.show('App API test');
    window.OMNProgress.stage('Working…');
    setTimeout(() => window.OMNProgress.hide(), 1500);
  });
  document.querySelector('#aatRefresh').addEventListener('click', () => {
    window.refreshPage();
  });
  document.querySelector('#aatHighlight').addEventListener('click', () => {
    const term = document.querySelector('#aatTerm').value.trim();
    if (term) window.omnHighlightTerms([term]);
  });
  document.querySelector('#aatClear').addEventListener('click', () => {
    window.omnClearHighlights();
  });
}
</script>

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>
