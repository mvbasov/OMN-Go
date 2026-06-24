Title: Test page variables
Date: 2023-01-27 22:47:02
Modified: 2023-02-21 19:39:34
Author: Mikhail Basov
Tags: JavaScript, Test

|Variable|Value|
|--------|:---:|
|PackageName|<span id="pkg_name">-</span>|
|PageName |<span id="page_name">-</span>|
|Title|<span id="page_title">-</span>|

- - -

<script type="module">
// Fill page variable table
if (typeof PackageName !== 'undefined' && PackageName)
  document.querySelector('#pkg_name').innerHTML = PackageName;
if (typeof PageName !== 'undefined' && PageName)
  document.querySelector('#page_name').innerHTML = PageName;
if (typeof Title !== 'undefined' && Title)
  document.querySelector('#page_title').innerHTML = Title;
var statusDiv = document.querySelector('#status');
if (statusDiv != null){
  statusDiv.style.display = 'block';
  statusDiv.innerHTML += 'Status present';
}
// Create and fill HTML page meta table
var table = document.createElement('table');
var tableHead = document.createElement('thead');
var trH = document.createElement('tr')
var th1 = document.createElement('th');
th1.innerHTML = 'Meta';
trH.appendChild(th1);
var th2 = document.createElement('th');
th2.innerHTML = 'Value';
trH.appendChild(th2);
tableHead.appendChild(trH);
table.appendChild(tableHead);
var tbody = document.createElement('tbody');
const pageMetas = document.getElementsByTagName('meta');
for (let i = 0; i < pageMetas.length; i++) {
  var tr = document.createElement('tr');
  var td1 = document.createElement('td');
  var metaName = pageMetas[i].getAttribute('name') == null ? 'http-equiv*' : pageMetas[i].getAttribute('name');
  td1.innerHTML = metaName;
  tr.appendChild(td1);
  var td2 = document.createElement('td');
  td2.innerHTML = pageMetas[i].getAttribute('content');
  tr.appendChild(td2);
  tbody.appendChild(tr);
}
table.appendChild(tbody);
document.querySelector('#content').appendChild(table);

/**
// function from https://stackoverflow.com/a/7524621
function getMeta(metaName) {
  const metas = document.getElementsByTagName('meta');
  for (let i = 0; i < metas.length; i++) {
    if (metas[i].getAttribute('name') === metaName) {
      return metas[i].getAttribute('content');
    }
  }
  return '';
}
if (getMeta('generator'))
  document.querySelector('#generator').innerHTML = getMeta('generator');
if (getMeta('modified'))
  document.querySelector('#modified').innerHTML = getMeta('modified');
if (getMeta('date'))
  document.querySelector('#date').innerHTML = getMeta('date');
**/
</script>
`* http-equiv` is not meta name. It is meta property itself.
