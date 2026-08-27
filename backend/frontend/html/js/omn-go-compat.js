/* omn-go-compat: the one script in this application written in ES5.
 *
 * WHY IT IS ES5 AND WHY IT LOADS FIRST. Every other script here uses
 * async/await, arrow functions and template literals. A WebView that cannot
 * parse those throws a SyntaxError and drops the WHOLE file. The code that
 * would report the problem is then the code that does not run.
 *
 * A <script src> element is its own parse unit. A SyntaxError in
 * omn-go-core.js therefore cannot stop this file. Two rules keep that true,
 * and TestCompatScriptIsFirstAndES5 enforces both:
 *
 *   1. This file holds nothing newer than ES5.
 *   2. index.html loads it before every other script.
 *
 * DO NOT MOVE THIS CODE INTO omn-go-core.js. That file is modern, an old
 * WebView drops all of it, and this notice would go with it.
 *
 * It was inline in index.html until 26.08.73. It moved out because the
 * compiled page of each note carried a copy, which is 3.3 KB for each note
 * on disk and in every git sync.
 *
 * THE NUMBER. 85 is the highest requirement the frontend actually has:
 * String.replaceAll, in Bookmarker.js and the editor. Below that things
 * throw rather than degrade. Chromium 44 is what Android 6 ships, and
 * Chromium 106 is what its System WebView updates to, so on that release
 * of Android this notice is the difference between a blank page and a
 * sentence saying which update is missing.
 *
 * It says nothing on a browser with no Chrome token in its user agent -
 * Firefox and Safari are not measured by this number and do not need to
 * be warned about it.
 */
(function () {
    var MIN = 85;
    var m = /Chrome\/(\d+)/.exec(navigator.userAgent || '');
    if (!m || parseInt(m[1], 10) >= MIN) { return; }
    var found = m[1];
    function show() {
        if (!document.body || document.getElementById('omnGoCompat')) { return; }
        var bar = document.createElement('div');
        bar.id = 'omnGoCompat';
        /* Inline styles on purpose: the stylesheet is built on CSS
           custom properties, which a WebView this old does not have. */
        bar.setAttribute('style', 'position:relative;z-index:99999;'
            + 'padding:10px 34px 10px 12px;margin:0;'
            + 'background:#8a1c1c;color:#ffffff;'
            + 'font:14px/1.4 sans-serif;');
        bar.appendChild(document.createTextNode(
            'This browser is too old for OMN-Go. It is Chromium ' + found
            + ' and OMN-Go needs ' + MIN + ' or newer. On Android, update'
            + ' "Android System WebView" and Chrome, then start OMN-Go'
            + ' again. Until then this page can be incomplete or empty.'));
        var x = document.createElement('span');
        x.setAttribute('style', 'position:absolute;top:6px;right:10px;'
            + 'cursor:pointer;font-weight:bold;');
        /* The escape, not the character: the server sends this file as
           application/javascript with no charset, thus every byte here
           stays ASCII. */
        x.appendChild(document.createTextNode('\u2715'));
        x.onclick = function () {
            if (bar.parentNode) { bar.parentNode.removeChild(bar); }
        };
        bar.appendChild(x);
        document.body.insertBefore(bar, document.body.firstChild);
    }
    if (document.addEventListener) {
        document.addEventListener('DOMContentLoaded', show, false);
    }
    /* A second chance: DOMContentLoaded has fired already if this page
       came from the cache, and window.onload is the older event that
       every WebView here has. */
    if (window.addEventListener) {
        window.addEventListener('load', show, false);
    } else {
        window.attachEvent('onload', show);
    }
})();
