// --- A note for another person, the clipboard and the metadata panel ---
//
// This file sends or copies the note on screen, copies text, and gives a
// link to the page. It also builds the metadata panel, which holds the
// controls of these functions.
//
// index.html loads it after omn-go-core.js. The User Manual promises
// omnGoCopyText, omnGoPageTitle and omnGoPageLink to a note script.
// WebViewSetup.java names omnGoSendNote.

// --- Sending this note to someone ---
//
// The controls live on the metadata panel's "File:" line and not in the
// header actions, which is full.
//
// Both fetch the SAME URL, /api/export/note. That endpoint answers the
// source of the note, and it adds a "FileName:" line to the header block.
// That line is the only place where the path of the note survives a
// transport that delivers a flat file name. The stored note does not
// change, because an export is a read.

// omnGoExportURL is the one address both controls use, and the one
// ShareOut.java fetches for the Android share sheet.
// omnGoCurrentNoteName gives the name of the note on screen in the form
// that the server resolves with no guess.
//
// The server reads the LAST extension of a name (hasKnownAssetExtension in
// internal/config/content_types.go). A bare base name is thus ambiguous when it
// ends in a real file extension. A note named "Draft.txt" sent as "Draft.txt"
// reads as the file html/Draft.txt, and a save then writes to the wrong tree.
// The same name sent as "Draft.txt.md" reads as the note, always.
//
// PageName carries the extension already when the page is a file, thus this
// function adds ".md" only for a markdown page.
//
// Each caller that sends the note on screen to /api/note, /api/save,
// /api/export/note or /api/search uses this function.
function omnGoCurrentNoteName() {
    var name = (typeof PageName !== 'undefined' && PageName) ? String(PageName) : '';
    if (!name) return '';
    var markdown = (typeof IS_MARKDOWN !== 'undefined') ? IS_MARKDOWN : false;
    return markdown ? name + '.md' : name;
}
window.omnGoCurrentNoteName = omnGoCurrentNoteName;

function omnGoExportURL(note) {
    return '/api/export/note?name=' + encodeURIComponent(note);
}

// omnGoSendNote hands the note to whatever can carry it.
//
// On Android that is the share sheet, which reaches Telegram, e-mail,
// LocalSend and everything else installed. WebViewSetup answers the
// omngo:// scheme, as it already does for omngo://edit and
// omngo://shortcut. There is no share sheet elsewhere. The browser
// downloads the file, and the user attaches it where they want.
// Content-Disposition on the endpoint is what makes it a download.
function omnGoSendNote(note) {
    if (typeof IS_ANDROID !== 'undefined' && IS_ANDROID) {
        window.location.href = 'omngo://share?name=' + encodeURIComponent(note);
        return;
    }
    window.location.href = omnGoExportURL(note);
}

// omnGoCopyText writes one string to the clipboard. It is the only
// clipboard writer of the application. A note script can call it. See the
// "Useful functions" section of the User Manual.
//
// THE FUNCTION HAS TWO WAYS. THE SECOND WAY IS NOT A LEGACY BRANCH.
//
// The Clipboard API needs a secure context. The address http://127.0.0.1
// is one. The API is thus present on the device and in the Android
// WebView. Presence is not permission. The Android WebView refuses the
// clipboard-write permission. The method writeText then rejects with
// "NotAllowedError: Write permission denied". The refusal does not go to
// WebChromeClient.onPermissionRequest. No override in WebViewSetup can
// grant it.
//
// The refusal comes from an old WebView. A new WebView does the write.
// Android 6 keeps its System WebView at Chromium 106. That version has the
// API and refuses the permission. Android 14 has a current WebView. That
// version does the write after a tap.
//
// This function must not return at the API call. The refusal on Android 6
// would then reach the reader as "Copy failed: Write permission denied",
// and the second way would not run.
//
// The second way uses a scratch textarea and execCommand. It is
// synchronous. It needs no permission. It works in the Android WebView, on
// a plain-http LAN page and in a desktop browser. A LAN page is not a
// secure context, thus the Clipboard API is absent there. The
// quick-note-copy action below uses the second way direct, because its text
// is already in a textarea.
//
// This function throws an error when both ways fail. Each caller writes
// the failure in the status text beside the button.
async function omnGoCopyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
        try {
            await navigator.clipboard.writeText(text);
            return;
        } catch (e) {
            // The API is present and refused the write. Go to the
            // second way below.
        }
    }
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    // Call focus before select. quick-note-copy and the Status page do the
    // same, and a test on Android 6 shows that both work. execCommand can
    // refuse a selection in an element that does not have the focus.
    ta.focus();
    ta.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    if (!ok) throw new Error('the browser refused the copy');
}
window.omnGoCopyText = omnGoCopyText;

// omnGoCopyNote puts the same Markdown on the clipboard, for pasting into a
// chat or a mail body.
async function omnGoCopyNote(note, say) {
    try {
        const res = await fetch(omnGoExportURL(note), { cache: 'no-store' });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        await omnGoCopyText(await res.text());
        say('Copied');
    } catch (e) {
        say('Copy failed: ' + e.message);
    }
}

// --- The Copy button of the Quick Note panel ---
//
// Copies the Quick Note text to the clipboard WITHOUT saving it. The
// captured snippet can then be pasted somewhere else. A person types
// that snippet, or shares it in from another Android app, or pushes
// it in with a barcode scan. See omnGoInsertCapture in omn-go-api.js.
// btn is the Copy button of the panel, thus the label can report the
// outcome.
//
// It is in this file and not beside the quick-note-save action in
// omn-go-api.js, because it never talks to the backend. The no-server
// branch of that file gives each of its actions a printDebug stub.
// That is right for /api/quick and wrong for a pure clipboard action.
//
// This function uses select and execCommand('copy') on purpose. It
// does not use the Clipboard API. The Clipboard API is the modern
// spelling, but an old Android WebView refuses it. The method
// writeText needs a clipboard-write permission, and the WebView
// refuses that permission. The refusal does not go to
// WebChromeClient.onPermissionRequest. No override in WebViewSetup
// can grant it. A test on the device shows this.
//
// execCommand is deprecated, but it is synchronous and needs no
// permission. It works in the WebView, on a plain-http LAN page and
// in a desktop browser. A LAN page is not a secure context, thus
// the Clipboard API is absent there. One path serves all three.
//
// omnGoCopyText above is the general form. It tries the Clipboard
// API first. This action stays direct, because its text is
// already in a textarea. The focus must stay in that textarea for
// the typing that follows.
//
// No scratch element is needed: the text already sits in a <textarea>,
// which is exactly what select() wants.
window.OMN.action('quick-note-copy', function (btn) {
    var q = document.getElementById('omn-quick-text');
    if (!q) return;

    // Restores the button's own label after a moment. The original
    // is stashed on the first use. A repeated click lands while the
    // label still reads "Copied!". It must not capture the feedback
    // text as the label to go back to.
    function feedback(msg) {
        if (!btn) return;
        if (typeof btn.dataset.omnLabel === 'undefined') {
            btn.dataset.omnLabel = btn.textContent;
        }
        btn.textContent = msg;
        clearTimeout(btn._omnCopyTimer);
        btn._omnCopyTimer = setTimeout(function () {
            btn.textContent = btn.dataset.omnLabel;
        }, 1200);
    }

    if (!q.value) {
        feedback('Empty');
        return;
    }

    var ok = false;
    q.focus();
    q.select();
    try {
        ok = document.execCommand('copy');
    } catch (e) {
        ok = false;
    }

    // Drop the selection once the copy has been taken. The panel
    // stays open afterwards. A whole note that stays highlighted
    // looks like work in progress, and the next keystroke would
    // replace the entire text. A collapse to the end keeps the
    // caret in a sensible place for more typing.
    try {
        q.setSelectionRange(q.value.length, q.value.length);
    } catch (e) { /* element does not support selection ranges */ }

    feedback(ok ? 'Copied!' : 'Copy failed');
});

// --- A link to the page on screen ---

// omnGoPageTitle gives the text of that link. The "Title:" line of the note is
// the first choice, because that is the name the reader knows. Title and
// PageName are the fallbacks, in that order, for a view with no note behind
// it - the Config page, for example.
function omnGoPageTitle() {
    var out = '';
    document.querySelectorAll('meta[name]').forEach(function (m) {
        if (out) return;
        if (m.getAttribute('name').toLowerCase() === 'title') {
            out = (m.getAttribute('content') || '').trim();
        }
    });
    if (!out && typeof Title !== 'undefined' && Title) out = String(Title).trim();
    if (!out && typeof PageName !== 'undefined' && PageName) out = String(PageName).trim();
    return out || 'link';
}

// omnGoPageLink builds a Markdown link to the page on screen, for pasting into
// another note.
//
// The target is the address of this page WITHOUT the scheme and the host.
// It is the absolute path, the query string and the fragment, as the
// address bar holds them. A path keeps its meaning on each device that
// opens the same notes. A host does not. The Android application and the
// desktop application both serve the pages at 127.0.0.1. A link that
// carries the host works on the one device that made it and nowhere else.
//
// The browser encodes the path, so a space is already %20. The two
// parentheses are the characters that the browser leaves alone. Markdown
// reads them as the end of a link, thus this function encodes them. In the
// link text, a backslash and the two brackets get a backslash in front of
// them.
//
// render.Renderer.RewriteInternalLink (backend/internal/render/markdown.go)
// cuts the query and the fragment off before it looks at the extension, and it
// leaves a name that has an extension alone. A link of this shape thus reaches
// the reader as it was written.
function omnGoPageLink() {
    var loc = window.location;
    var target = (loc.pathname + loc.search + loc.hash)
        .replace(/\(/g, '%28').replace(/\)/g, '%29');
    var text = omnGoPageTitle().replace(/([\\\[\]])/g, '\\$1');
    return '[' + text + '](' + target + ')';
}
window.omnGoPageTitle = omnGoPageTitle;
window.omnGoPageLink = omnGoPageLink;

// omnGoCopyPageLink puts that link on the clipboard.
async function omnGoCopyPageLink(say) {
    try {
        await omnGoCopyText(omnGoPageLink());
        say('Link copied');
    } catch (e) {
        say('Copy failed: ' + e.message);
    }
}

// --- Dynamic Metadata Panel Extractor ---
//
// Built from ELEMENTS, not from a string of HTML.
//
// Every value here comes from the meta tags of the note, which come from
// its header block. A string of HTML would let a note write markup into
// its own metadata panel. textContent cannot. The colors are theme
// tokens, thus the panel is legible on the dark theme.
document.addEventListener("DOMContentLoaded", () => {
    const panel = document.getElementById('omn-metadata-panel');
    if (!panel) return;

    const noteName = (typeof PageName !== 'undefined') ? PageName : '';

    panel.textContent = '';

    // The panel is one row: the metadata at the left, a column of controls at
    // the right. A control goes under the control before it, so a new control
    // makes the column longer. It does not make the "File:" line narrower,
    // which is what a third control in that line did.
    const layout = document.createElement('div');
    layout.className = 'omn-metadata-layout';
    const body = document.createElement('div');
    body.className = 'omn-metadata-body';
    const actions = document.createElement('div');
    actions.className = 'omn-metadata-actions';

    const fileRow = document.createElement('div');
    fileRow.className = 'omn-metadata-file-row';
    const fileLabel = document.createElement('span');
    fileLabel.className = 'omn-metadata-file-name';
    fileLabel.textContent = 'File: ' + noteName;
    fileRow.appendChild(fileLabel);

    // A page opened from disk (file:) has no server to ask, and no address
    // that another note can point at. Such a page gets no controls.
    const online = window.location.protocol !== 'file:';

    // The two note controls need a note. IS_MARKDOWN is off for the Config
    // dashboard, the search page and the other views that borrow this page
    // shell. Those views have no Markdown source to send. The link control
    // has no such condition, because each of those views has an address.
    const sendable = noteName &&
        (typeof IS_MARKDOWN !== 'undefined' && IS_MARKDOWN) && online;

    if (online) {
        const status = document.createElement('span');
        status.className = 'omn-metadata-send-status';
        const say = function (msg) {
            status.textContent = msg || '';
            if (msg) setTimeout(function () { status.textContent = ''; }, 4000);
        };

        const button = function (icon, label, onClick) {
            const b = document.createElement('button');
            b.type = 'button';
            b.className = 'omn-metadata-send';
            b.title = label;
            b.setAttribute('aria-label', label);
            const i = document.createElement('i');
            i.className = 'material-icons omn-icon-sm';
            i.textContent = icon;
            b.appendChild(i);
            b.addEventListener('click', onClick);
            return b;
        };

        fileRow.appendChild(status);
        if (sendable) {
            // omnGoCurrentNoteName and not noteName: the server needs the
            // unambiguous form, and noteName is what the panel displays.
            const apiName = omnGoCurrentNoteName();
            actions.appendChild(button('share', 'Send this note',
                function () { omnGoSendNote(apiName); }));
            actions.appendChild(button('content_copy', 'Copy this note as text',
                function () { omnGoCopyNote(apiName, say); }));
        }
        actions.appendChild(button('link', 'Copy a link to this page',
            function () { omnGoCopyPageLink(say); }));
    }

    body.appendChild(fileRow);

    document.querySelectorAll('meta').forEach(m => {
        const name = m.getAttribute('name');
        const content = m.getAttribute('content');
        if (!name || !content || ['viewport', 'charset'].includes(name.toLowerCase())) return;
        const row = document.createElement('div');
        row.className = 'omn-metadata-row';
        const key = document.createElement('strong');
        key.textContent = name.charAt(0).toUpperCase() + name.slice(1) + ':';
        row.appendChild(key);
        row.appendChild(document.createTextNode(' ' + content));
        body.appendChild(row);
    });

    layout.appendChild(body);
    if (actions.childNodes.length) layout.appendChild(actions);
    panel.appendChild(layout);
});
