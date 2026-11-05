const bVersion = '0.19g 2026-06-24 14:59:01';
const duplicateTag = '!Duplicates';
const noTagTag = '!NoTag';
const config = {};
const embedCont = document.querySelector('#omn-preview');
// The one key that holds the settings of this page in localStorage.
//
// THE KEY HAS NO PREFIX OF THE APPLICATION ID. Such a prefix would keep
// the settings of the standard build apart from the settings of the
// F-Droid build. Two rules already do that:
//
//   * localStorage belongs to one origin, and an origin holds the port.
//     The standard build serves 8080 and the F-Droid build serves 8081.
//     See DEFAULT_SERVER_PORT in android/app/build.gradle.
//   * On Android each flavor is a separate application with a storage
//     directory of its own, thus the WebView data is separate as well.
//
// The one name that the page could have used is PackageName, which index.html
// injects. That value is the literal "net.basov.omngo" for each build. See
// render.Renderer.CompilePageWithBody in backend/internal/render/markdown.go. A
// prefix from it would therefore be the same text on both flavors.
//
// The name below is therefore the whole rule. It does not change, and a
// change of it makes each reader lose the settings of this page. See
// TestBookmarkerConfigKeyIsWellFormed.
const configKey = 'OMNBookmarkerConfigG';
let index = {};
let durls = [];
// Remove si= reference from YouTube url
function sYURL(url){
  if (url.toUpperCase().includes("YOUTUBE")) {
    return url.replace(/\&si=.*$/,'');
  }
  return url;
}
function analyzeDuplicates(bm){
  let sUrl = sYURL(bm.url);
  if(sUrl in index) {
    index[sUrl].push(bm);
  } else {
    index[sUrl] = [bm];
  }
}
function compactDuplicates(){
  for(const url in index){
    if(index[url].length < 2)
      delete index[url];
  }
  durls = Object.keys(index);
}
// linkAddress answers the address that the link of a bookmark can open, or
// an empty text for an address that would run a script.
//
// A VALUE OF A BOOKMARK IS TEXT FROM A FOREIGN PLACE. The title is the title
// of a page of a different site, and the address comes from a share. This
// page thus writes each value as text, with textContent. It never gives one
// to innerHTML, which would run the markup of that value with the session of
// the reader.
//
// The address also goes to the href of a link. A press on a link of the
// scheme "javascript:" runs its text as a script. A browser drops each space
// and each control character of a scheme, thus this function drops them
// first. The item of such a bookmark keeps its text and gets no link.
function linkAddress(url) {
  const compact = String(url).replace(/[\u0000-\u0020]/g, '');
  if (/^(javascript|data|vbscript):/i.test(compact)) return '';
  return url;
}
function showBookmarks(onlyTag = '', search = '', duplicates = false) {
  // sort bookmarks by date (newest upper)
  bookmarks.sort((a, b) => { if (a.date > b.date) { return -1; } });
  // tag display priority on search
  if (onlyTag !== '' || search === '') {
    search = '';
    let searchF = document.querySelector('#omn-bm-search-input');
    searchF.value = '';
  }
  let counter = 0;
  let allTags = [];
  let ul = document.createElement('ul');
  ul.style.height = '100%;';
  index = {};
  for (const bm of bookmarks) {
    if (bm.url !== '') {
      if (!display(bm, onlyTag, search, duplicates)) continue;
      counter++;
      analyzeDuplicates(bm);
      let li = document.createElement('li');
      let a = document.createElement('a');
      const href = linkAddress(bm.url);
      if (href !== '') a.setAttribute('href', href);
      // Set title or url as title if empty
      if (bm.title && bm.title !== '')
        a.textContent = bm.title;
      else
        a.textContent = bm.url;
      a.onmouseover = () => {setTimeout( () => { navigator.clipboard.writeText(bm.url); }, 500 );};
      li.appendChild(a);
      let span = document.createElement('span');
      span.appendChild(document.createElement('br'));
      span.appendChild(document.createTextNode(bm.url));
      li.appendChild(span);
      if (bm.date && bm.date !== '') {
        li.setAttribute('id', bm.date.replaceAll(':','').replaceAll(' ','-'));
        li.appendChild(document.createElement('br'));
        let spanDate = document.createElement('span')
        txt = document.createTextNode('');
        txt.textContent =  bm.date;
        spanDate.appendChild(txt);
        spanDate.className = 'omn-span-date';
        li.appendChild(spanDate);
      }
      // find tags if exists   
      let ulin = document.createElement('ul');
      let liint = document.createElement('li');
      let divt = document.createElement('div');
      txt = document.createTextNode('');
      let btnt = document.createElement('button');
      if (bm.tags && bm.tags.length > 0){
        for (_tag of bm.tags) {
          btnt = document.createElement('button');
          btnt.innerText = _tag;
          btnt.className = 'omn-tag';
          btnt.onclick = function(){showBookmarks(this.innerText);};
          divt.appendChild(btnt);
          if (!allTags.includes(_tag)) {
            allTags.push(_tag);
          }
          first = false;
        }
        liint.appendChild(divt);
        ulin.appendChild(liint);
      }
      // find notes if exists
      txt = document.createTextNode('')
      var first = true;
      if (bm.notes && bm.notes.length > 0){
        const liinn = document.createElement('li');
        for (note of bm.notes) {
          if (!first) txt.textContent += ', '  
          txt.textContent += '"'+note+'"';
          first = false;
        }
        liinn.appendChild(txt);
        ulin.appendChild(liinn);
      }
      // add sub-ul with tags and notes if not empty
      if (ulin.firstChild) {
        var txt = document.createTextNode('');
        txt.textContent = 'ⓘ';
        var btn = document.createElement('button');
        btn.onclick=function() {
          if(this.parentNode) {
            var infoBlock = this.parentNode.getElementsByTagName('ul')[0];
            if(infoBlock.style.display == 'none') {
              infoBlock.style.display='block';
            } else {
              infoBlock.style.display='none';
            }
          }
        }
        btn.appendChild(txt);
        btn.className = 'omn-details';
        li.insertBefore(btn, li.firstChild);
        li.appendChild(ulin);
      }
      // add boormark to list
      ul.appendChild(li);
    }
  }
  compactDuplicates();
  // display list of bookmarks
  document.querySelector('#omn-bm-list').replaceChildren(ul);
  // collapse all details
  colexpall('collapse');
  // display counter
  var counterSpan = document.querySelector('#omn-bm-counter');
  counterSpan.innerHTML = counter;
  counterSpan.onclick = toggleTagsCloud;
  // clear and display tags cloud
  var tagsDiv = document.querySelector('#omn-bm-tags-cloud');
  tagsDiv.innerHTML = "";
  if (allTags.length > 0) {
    allTags = allTags.sort();
    allTags.unshift(duplicateTag);
    allTags.unshift(noTagTag);
    for (const tagC of allTags) {
      var tagB = document.createElement('button');
      if (tagC == onlyTag)
        tagB.disabled = true;
      if (tagC.startsWith('!')) {
        tagB.textContent = tagC.substring(1);
        tagB.classList.add('omn-tag-special');
      } else {
        tagB.textContent = tagC;
      }
      tagB.classList.add('omn-tag');
      var tagHandler = function(){showBookmarks(tagC);};
      tagB.onclick = tagHandler;
      tagsDiv.appendChild(tagB);
    }
  }
}

function toggleTagsCloud(){
  var tagsCloud = document.querySelector('#omn-bm-tags-cloud');
  if (window.getComputedStyle(tagsCloud).display === 'none')
    tagsCloud.style.display = 'block';
  else
    tagsCloud.style.display = 'none';
}

// Check bookmark need to be displayed or not
function display(bm, tag, search, duplicates) {
  //serch in notes, url, tags, date and title
  if (search != '') {
    const lsearch = stripAccents(search.trim());
    var roptions = 'mu';
    if (config['ignoreCase']) roptions += 'i';
    var patt = new RegExp(lsearch, roptions);
    if (sYURL(bm.url).search(patt) !== -1) {
      return true;
    }
    if (typeof bm.notes !== 'undefined' && bm.notes.length > 0) {
      for (note of bm.notes) {
        if (note != '' && stripAccents(note).search(patt) !== -1)
          return true;
      }
    }
    if (typeof bm.tags !== 'undefined' && bm.tags.length > 0) {
      for (tag of bm.tags) {
        if (tag != '' && stripAccents(tag).search(patt) !== -1)
          return true;
      }
    }
    if (typeof bm.title !== 'undefined' && bm.title != '')
      if (stripAccents(bm.title).search(patt) !== -1)
        return true; 
    if (typeof bm.date !== 'undefined' && bm.date != '')
      if (bm.date.search(patt) !== -1)
        return true; 
    return false;
  }
  // show without tags
  if ((tag == -1 || tag == noTagTag) && typeof bm.tags !== 'undefined' && bm.tags.length > 0) {
    return false;
  }
 /* Show only duplicates */
  if (tag == duplicateTag && durls.includes(sYURL(bm.url))){
    return true;
  }
  // show only with specified tag
  if (tag != '' && tag != -1 && tag != noTagTag) {
    if (typeof bm.tags === 'undefined' 
        || (typeof bm.tags !== 'undefined' && bm.tags.indexOf(tag) < 0)
    )        
    return false;
  }
  if (duplicates && durls.includes(sYURL(bm.url))){
    return true;
  }
  return true;
}

function colexpall(state) {
  var allButtons =
      embedCont
      .getElementsByTagName('ul')[0]
      .getElementsByClassName('omn-details');
  for(var x = 0; x < allButtons.length; x++) {
    // collapse/expand all details
    var sub = allButtons[x].parentNode.getElementsByTagName('ul')[0];
    if(sub && sub != null){
      if(state == 'collapse') {
        sub.style.display = 'none';
      } else {
        sub.style.display = 'block';
      }
    }
  }
}

function createControlHeader() {
/*
<div
  id="omn-bm-search-box"
  style="width: 95%; padding-top: 0.8em;">
  <input
    id="omn-bm-search-input"
    type"text">
  </input>
  <button 
    id="omn-bm-search-btn"
    class="omn-colexp" 
    onClick="
      showBookmarks('', document.querySelector('#omn-bm-search-input').value);
    ">
Search
  </button>
  <span id="omn-bm-counter" style="float: right;">?</span>
</div>
*/
  // create search box
  var searchD = document.createElement('div');
  searchD.id = 'omn-bm-search-box';
  var searchI = document.createElement('input');
  searchI.id = 'omn-bm-search-input';
  searchI.type = 'search';
  searchI.placeholder = 'Enter search pattern…';
  searchD.appendChild(searchI);

  var searchB = document.createElement('button');
  searchB.id = 'omn-bm-search-btn';
  searchB.className = 'omn-colexp';
  searchB.innerHTML = 'Search';
  searchB.onclick = function(){
    showBookmarks('', document.querySelector('#omn-bm-search-input').value);
  };
  searchD.appendChild(searchB);
  // Enable 'Enter' on keyboard to perform search
  //document.querySelector('#omn-bm-search-input')
  searchI.addEventListener('keyup', function(event) {
      event.preventDefault();
      if (event.keyCode === 13) {
        document.querySelector('#omn-bm-search-btn').click();
      }
  });

  var searchC = document.createElement('span');
  searchC.id = 'omn-bm-counter';
  searchC.style = 'float: right;';
  searchD.appendChild(searchC);

  embedCont.insertBefore(searchD, embedCont.firstChild);

/*
<div
  id="buttons
  style="width: 95%; padding-top: 0.8em;">
<p><button
  class="omn-colexp"
  onClick="colexpall('expand')">
Expand
</button>
<button
  class="omn-colexp"
  onClick="colexpall('collapse')">
Collapse
</button>
<button
  class="omn-colexp"
  onClick="showBookmarks('');colexp('collapse');">
All
</button>
<button
  class="omn-colexp"
  onClick="showBookmarks(-1)">
No tags
</button></p>
</div>
*/
  // create control buttons
  var buttonsD = document.createElement('div');
  buttonsD.id = 'omn-bm-buttons';

  var buttonsBexpand = document.createElement('button');
  buttonsBexpand.className = 'omn-colexp';
  buttonsBexpand.innerHTML = 'Expand';
  buttonsBexpand.onclick = function(){
    colexpall('expand');
  };
  buttonsD.appendChild(buttonsBexpand);

  var buttonsBcollapse = document.createElement('button');
  buttonsBcollapse.className = 'omn-colexp';
  buttonsBcollapse.innerHTML = 'Collapse';
  buttonsBcollapse.onclick = function(){
    colexpall('collapse');
  };
  buttonsD.appendChild(buttonsBcollapse);

  var buttonsBall = document.createElement('button');
  buttonsBall.className = 'omn-colexp';
  buttonsBall.innerHTML = 'All';
  buttonsBall.onclick = function(){
    showBookmarks('');
    colexpall('collapse');
  };
  buttonsD.appendChild(buttonsBall);

  searchD.after(buttonsD);
/*
<div
  id="omn-bm-tags-cloud">
</div>
<div
    id="omn-bm-list">
</div>
*/

  // create Tags cloud
  var tagsCloudD = document.createElement('div');
  tagsCloudD.id = 'omn-bm-tags-cloud';
  buttonsD.after(tagsCloudD);

  // create main bookmark list
  var bmlistD = document.createElement('div');
  bmlistD.id = 'omn-bm-list';
  tagsCloudD.after(bmlistD);

/*
<div
  style="font-size:0.3em;"
  id="omn-bm-version">
Version 0.4 (2023-01-16 05:19:40)
</div>
*/
  // Display version at the botom
  var versionS = document.createElement('span');
  versionS.innerHTML = bVersion;
  versionS.id = 'omn-bm-version'
  document.querySelector('#omn-bm-list').after(versionS);
}

function urlParams() {
  createControlHeader();
  // process url parameters
  // one of two parameters 'tag' or 'search' processed
  // The 'tag' has higher priority then 'search'.
  var urlQuery = window.location.search.substring(1);
  var searchF = document.querySelector('#omn-bm-search-input');
  if(urlQuery.length > 0){
    const urlParams = new URLSearchParams(urlQuery);
    var configP = urlParams.get('config', '');
    if (configP) {
      configuration(configP);
    } else {
      configuration();
    }
    var tagP = urlParams.get('tag', '');
    if (tagP) {
      searchF.value = '';
      showBookmarks(tagP);
      return;
    }
    var searchP = urlParams.get('search','');
    if (searchP) {
      searchF.value = searchP;
      showBookmarks('', searchP);
      return;
    }
  }
  configuration();
  showBookmarks();
  //showBookmarks(-1); // without tags
  //showBookmarks('Tag 2'); // with 'Tag 2'
  //showBookmarks('', 'google.com'); // search 'google.com'
}

// Strip accents 
function stripAccents(str) {
  var r = str;
  if (!config['stripAccents']) return r; 
  r = r.replace(new RegExp('[àáâãäå]', 'g'),'a');
  r = r.replace(new RegExp('[ÀÁÂÃÄÅ]', 'g'),'A');
  r = r.replace(new RegExp('æ', 'g'),'ae');
  r = r.replace(new RegExp('Æ', 'g'),'Ae');
  r = r.replace(new RegExp('ç', 'g'),'c');
  r = r.replace(new RegExp('Ç', 'g'),'C');
  r = r.replace(new RegExp('[èéê]', 'g'),'e');
  r = r.replace(new RegExp('[ÈÉÊ]', 'g'),'E');
  r = r.replace(new RegExp('[ё]', 'g'),'е'); // replace Cyrillic ё то Cyrillic е
  r = r.replace(new RegExp('[Ё]', 'g'),'Е'); // replace Cyrillic Ё то Cyrillic Е
  r = r.replace(new RegExp('[ìíîï]', 'g'),'i');
  r = r.replace(new RegExp('[ÌÍÎÏ]', 'g'),'I');
  r = r.replace(new RegExp('ñ', 'g'),'n');
  r = r.replace(new RegExp('Ñ', 'g'),'N');
  r = r.replace(new RegExp('[òóôõö]', 'g'),'o');
  r = r.replace(new RegExp('[ÒÓÔÕÖ]', 'g'),'O');
  r = r.replace(new RegExp('œ', 'g'),'oe');
  r = r.replace(new RegExp('Œ', 'g'),'Oe');
  r = r.replace(new RegExp('[ùúûü]', 'g'),'u');
  r = r.replace(new RegExp('[ÙÚÛÜ]', 'g'),'U');
  r = r.replace(new RegExp('[ýÿ]', 'g'),'y');
  r = r.replace(new RegExp('[ÝŸ]', 'g'),'Y');
  return r;
};

function configuration(params = '') {
  var configDefault = {
    "stripAccents": true,
    "ignoreCase": true,
    "storage": false
  };
  configDefault['configVersion'] = bVersion;
  var configURI = {};
  if (params !== '') {
    alert('URI params: ' + params);
    try {
      if (params != 'show') 
        configURI = JSON.parse(params);
    } catch (e) {
      alert('URI Config parsing error: ' + e);
    }
  }
  var configStorage = {};
  if (localStorage != null) {
    if (localStorage[configKey]) {
      try {
        configStorage = JSON.parse(localStorage[configKey]);
        configStorage['storage'] = true;
      } catch (e) {
        alert('Storage Config parsing error: ' + e);
        configStorage['storage'] = false;
      }
    } else {
       localStorage[configKey] = JSON.stringify(configDefault);
    } 
  }
  for (const p in configDefault) {
    if (configURI.hasOwnProperty(p))
      config[p] = configURI[p];
    else if (configStorage.hasOwnProperty(p))
      config[p] = configStorage[p];
    else 
      config[p] = configDefault[p];
  }
  config['configVersion'] = bVersion;
  if (localStorage != null)
    localStorage[configKey]= JSON.stringify(config);
  if (params !== '') {
    alert(
      'Configuration\n'
    + '—'.repeat(16) +'\n'
    + 'configKey: ' + configKey + '\n'
    + 'Default: ' + JSON.stringify(configDefault, null, 2) + '\n'
    + 'Storage: '  + JSON.stringify(configStorage, null, 2) + '\n'
    + 'URI: ' + JSON.stringify(configURI, null, 2) + '\n'
    + 'Effective: '  + JSON.stringify(config, null, 2)
    );
  }
}

// --- Arriving from a search result -------------------------------------
//
// A search hit inside a bookmark links to "/Bookmarks.html#<date-anchor>".
// That is the same id that showBookmarks() puts on each <li>, see the
// setAttribute above. Usually nothing more is needed. urlParams() renders
// the full, unfiltered list synchronously while the document is still
// parsing, thus the element exists before the browser performs its fragment
// scroll.
//
// The exception is a URL that also carries ?tag= or ?search=, which renders
// a FILTERED list. The entry that the link names may not be in that list.
// Drop the filter and render everything, rather than leave the reader on a
// page that silently does not contain what they clicked.
//
// The flash is not decoration. Several entries look alike, and the anchor
// scroll gives no feedback about which one it landed on. This is the same
// treatment that a match gets everywhere else in the app.
function revealHashedBookmark() {
  var id = decodeURIComponent((window.location.hash || '').replace(/^#/, ''));
  if (!id) return;
  var li = document.getElementById(id);
  if (!li) {
    var searchF = document.querySelector('#omn-bm-search-input');
    if (searchF) searchF.value = '';
    showBookmarks();
    li = document.getElementById(id);
    if (!li) return;   // deleted, or never existed: leave the page alone
  }
  if (li.scrollIntoView) li.scrollIntoView({ block: 'center' });
  li.classList.add('omn-search-hit-current');
}

urlParams();
revealHashedBookmark();
