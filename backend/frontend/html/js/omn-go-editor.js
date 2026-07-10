// --- OMN-Go standalone note editor ---
//
// This file powers the dedicated editor page (served for any ?edit=true
// request when the internal editor is enabled). Unlike the old in-page
// toggle, the note SOURCE is never baked into the rendered view page - it
// is fetched here, once, at editing start time, via /api/note. Saving
// posts to /api/save and returns to the rendered view.
//
// The server template defines three globals before loading this file:
//   OMN_EDIT_NAME  - the name passed to /api/note and /api/save
//   OMN_EDIT_EXT   - the file extension (informational, e.g. ".md", ".js")
//   OMN_EDIT_VIEW  - the URL of the rendered page to return to
//
// Toolbar tools live in a single TOOLS registry (see below): adding a
// button later is a one-line entry, nothing else needs to change.

(function () {
    'use strict';

    var NAME = (typeof OMN_EDIT_NAME !== 'undefined') ? OMN_EDIT_NAME : 'Welcome';
    var VIEW = (typeof OMN_EDIT_VIEW !== 'undefined' && OMN_EDIT_VIEW) ? OMN_EDIT_VIEW : '/';

    var ta = null;         // the <textarea>
    var statusEl = null;   // status line
    var loaded = false;    // has the initial content arrived?
    var dirty = false;     // unsaved changes?

    // ------------------------------------------------------------------
    // Toolbar tool registry. Each entry becomes a button, left to right.
    // To add a tool later: append one { icon, title, action } object.
    //   icon   - a Material Icons ligature name
    //   title  - tooltip / accessibility label
    //   action - function(textarea) invoked on click
    // ------------------------------------------------------------------
    var TOOLS = [
        { icon: 'code', title: 'Expand Emmet abbreviation (Tab)', action: function () { expandEmmetAtCursor(); } },
        { icon: 'format_line_spacing', title: 'Select current line', action: function () { selectCurrentLine(); } }
        // Future tools go here, e.g.:
        // { icon: 'format_bold', title: 'Bold selection', action: wrapBold },
    ];

    // ==================================================================
    // Emmet-style abbreviation expander (self-contained, offline).
    //
    // Supported subset (covers everyday use; anything not recognized -
    // including the climb-up "^" operator - simply yields no expansion, so
    // Tab falls back to inserting a literal tab rather than throwing):
    //   nesting  >     siblings  +     grouping ( )
    //   multiply *N    id  #id    class .cls     attrs [a=b c="d"]
    //   text     {..}  numbering $ / $$ (zero-padded) inside a repeat
    //   implicit tags: children of ul/ol -> li, tr -> td, table -> tr, ...
    // ==================================================================
    var VOID_TAGS = {
        area: 1, base: 1, br: 1, col: 1, embed: 1, hr: 1, img: 1, input: 1,
        link: 1, meta: 1, param: 1, source: 1, track: 1, wbr: 1
    };
    // Implicit child tag given a parent tag (Emmet's "implied" names).
    var IMPLICIT_CHILD = {
        ul: 'li', ol: 'li',
        table: 'tr', tbody: 'tr', thead: 'tr', tfoot: 'tr',
        tr: 'td',
        select: 'option', optgroup: 'option',
        map: 'area',
        dl: 'dt'
    };

    function makeNode() {
        return { tag: '', id: '', classes: [], attrs: [], text: null, children: [], mult: 1, group: false };
    }

    // Parse an abbreviation string into a forest (array of sibling nodes).
    // Returns null if the string does not look like an abbreviation.
    function parseAbbr(str) {
        var pos = 0;
        var n = str.length;

        function peek() { return str[pos]; }

        // Parse a run of sibling nodes until a closing ')' or end.
        function parseSiblings() {
            var nodes = [];
            var prev = null;
            while (pos < n) {
                var ch = str[pos];
                if (ch === ')') break;
                if (ch === '^') {
                    // Climb-up: handled by the caller stack. We signal it by
                    // returning; a '^' at this level ends the current group
                    // of siblings and the parent resumes. Support multiple.
                    break;
                }
                var node = parseSingle();
                if (!node) return null;
                nodes.push(node);
                prev = node;
                // Operator between siblings.
                if (pos < n) {
                    var op = str[pos];
                    if (op === '+') { pos++; continue; }
                    if (op === '>') {
                        pos++;
                        var kids = parseSiblingsUntilClimbOrClose();
                        if (kids === null) return null;
                        // Distribute children to the last node.
                        applyChildren(prev, kids.nodes);
                        // Handle any climb-ups returned.
                        var climb = kids.climb;
                        while (climb > 0 && pos <= n) {
                            // A climb means: subsequent siblings attach to an
                            // ancestor. We approximate by breaking out so the
                            // caller (one level up) continues.
                            climb--;
                            if (climb === 0) break;
                        }
                        if (kids.climb > 0) {
                            // Reduce climb by one for this level and bubble up.
                            pendingClimb = kids.climb - 1;
                            break;
                        }
                        continue;
                    }
                    if (op === '^') { break; }
                    if (op === ')') { break; }
                }
            }
            return nodes;
        }

        // Parse children after '>', collecting a possible trailing climb-up
        // count ('^', '^^', ...) that should re-parent following siblings.
        var pendingClimb = 0;
        function parseSiblingsUntilClimbOrClose() {
            var nodes = [];
            while (pos < n) {
                var ch = str[pos];
                if (ch === ')') break;
                if (ch === '^') {
                    var c = 0;
                    while (str[pos] === '^') { c++; pos++; }
                    return { nodes: nodes, climb: c };
                }
                var node = parseSingle();
                if (!node) return null;
                nodes.push(node);
                if (pos < n) {
                    var op = str[pos];
                    if (op === '+') { pos++; continue; }
                    if (op === '>') {
                        pos++;
                        var kids = parseSiblingsUntilClimbOrClose();
                        if (kids === null) return null;
                        applyChildren(node, kids.nodes);
                        if (kids.climb > 0) {
                            return { nodes: nodes, climb: kids.climb - 1 };
                        }
                        continue;
                    }
                    if (op === ')' || op === '^') break;
                }
            }
            return { nodes: nodes, climb: 0 };
        }

        function applyChildren(node, kids) {
            // Resolve implicit tag names for children based on the parent.
            var implied = IMPLICIT_CHILD[node.tag];
            for (var i = 0; i < kids.length; i++) {
                if (!kids[i].tag && !kids[i].group && implied) kids[i].tag = implied;
                if (!kids[i].tag && !kids[i].group) kids[i].tag = 'div';
            }
            node.children = node.children.concat(kids);
        }

        // Parse a single element or a ( ... ) group, plus optional *N.
        function parseSingle() {
            var node;
            if (str[pos] === '(') {
                pos++;
                var inner = parseGroupBody();
                if (inner === null) return null;
                if (str[pos] === ')') pos++;
                node = makeNode();
                node.group = true;
                node.children = inner;
            } else {
                node = parseElement();
                if (!node) return null;
            }
            // Multiplier. Clamped to a sane maximum so a stray "*999999"
            // can't lock the tab up building a giant string.
            if (str[pos] === '*') {
                pos++;
                var num = '';
                while (pos < n && str[pos] >= '0' && str[pos] <= '9') { num += str[pos]; pos++; }
                var m = num ? parseInt(num, 10) : 1;
                if (m < 1) m = 1;
                if (m > 1000) m = 1000;
                node.mult = m;
            }
            return node;
        }

        // Body of a ( ... ) group: siblings possibly joined by > + ^.
        function parseGroupBody() {
            var nodes = [];
            while (pos < n && str[pos] !== ')') {
                var node = parseSingle();
                if (!node) return null;
                nodes.push(node);
                if (str[pos] === '+') { pos++; continue; }
                if (str[pos] === '>') {
                    pos++;
                    var kids = parseSiblingsUntilClimbOrClose();
                    if (kids === null) return null;
                    applyChildren(node, kids.nodes);
                    continue;
                }
                if (str[pos] === '^') { pos++; continue; }
            }
            return nodes;
        }

        function parseElement() {
            var node = makeNode();
            // Tag name (letters, digits, -, :, and $ for numbering like
            // "h$" -> h1/h2/...). May be empty (implicit child tag).
            while (pos < n && /[A-Za-z0-9:\-$]/.test(str[pos])) { node.tag += str[pos]; pos++; }
            var sawSuffix = false;
            // Suffixes: #id .class [attrs] {text}
            while (pos < n) {
                var ch = str[pos];
                if (ch === '#') {
                    pos++; var id = '';
                    while (pos < n && /[A-Za-z0-9_\-$]/.test(str[pos])) { id += str[pos]; pos++; }
                    node.id = id; sawSuffix = true;
                } else if (ch === '.') {
                    pos++; var cls = '';
                    while (pos < n && /[A-Za-z0-9_\-$]/.test(str[pos])) { cls += str[pos]; pos++; }
                    if (cls) node.classes.push(cls); sawSuffix = true;
                } else if (ch === '[') {
                    pos++; var raw = '';
                    while (pos < n && str[pos] !== ']') { raw += str[pos]; pos++; }
                    if (str[pos] === ']') pos++;
                    node.attrs = node.attrs.concat(parseAttrs(raw)); sawSuffix = true;
                } else if (ch === '{') {
                    pos++; var txt = ''; var depth = 1;
                    while (pos < n && depth > 0) {
                        if (str[pos] === '{') depth++;
                        else if (str[pos] === '}') { depth--; if (depth === 0) break; }
                        txt += str[pos]; pos++;
                    }
                    if (str[pos] === '}') pos++;
                    node.text = txt; sawSuffix = true;
                } else {
                    break;
                }
            }
            if (!node.tag && !sawSuffix) return null;
            return node;
        }

        function parseAttrs(raw) {
            // Split "href=# title=\"Go home\" data-x=1" into {name,value}.
            var out = [];
            var i = 0, L = raw.length;
            while (i < L) {
                while (i < L && /\s/.test(raw[i])) i++;
                if (i >= L) break;
                var name = '';
                while (i < L && !/[\s=]/.test(raw[i])) { name += raw[i]; i++; }
                var value = null;
                if (raw[i] === '=') {
                    i++;
                    if (raw[i] === '"' || raw[i] === "'") {
                        var q = raw[i]; i++; value = '';
                        while (i < L && raw[i] !== q) { value += raw[i]; i++; }
                        if (raw[i] === q) i++;
                    } else {
                        value = '';
                        while (i < L && !/\s/.test(raw[i])) { value += raw[i]; i++; }
                    }
                }
                if (name) out.push({ name: name, value: value });
            }
            return out;
        }

        var forest = parseSiblings();
        if (forest === null || forest.length === 0) return null;
        if (pos < n) return null; // trailing garbage -> not a clean abbr
        return forest;
    }

    // Replace $ / $$ numbering tokens with the 1-based repeat index.
    function applyNumbering(s, idx) {
        if (s == null) return s;
        return s.replace(/\$+/g, function (m) {
            var num = String(idx);
            while (num.length < m.length) num = '0' + num;
            return num;
        });
    }

    function cloneWithIndex(node, idx) {
        var c = {
            tag: applyNumbering(node.tag, idx),
            id: applyNumbering(node.id, idx),
            classes: node.classes.map(function (x) { return applyNumbering(x, idx); }),
            attrs: node.attrs.map(function (a) { return { name: a.name, value: applyNumbering(a.value, idx) }; }),
            text: applyNumbering(node.text, idx),
            children: node.children,
            group: node.group
        };
        return c;
    }

    function serializeForest(nodes, indent, out) {
        for (var i = 0; i < nodes.length; i++) {
            var node = nodes[i];
            var count = node.mult || 1;
            for (var r = 1; r <= count; r++) {
                var inst = cloneWithIndex(node, r);
                if (node.group) {
                    serializeForest(inst.children, indent, out);
                } else {
                    serializeNode(inst, indent, out);
                }
            }
        }
    }

    function serializeNode(node, indent, out) {
        var pad = repeat('  ', indent);
        var tag = node.tag || 'div';
        var attrStr = '';
        if (node.id) attrStr += ' id="' + node.id + '"';
        if (node.classes.length) attrStr += ' class="' + node.classes.join(' ') + '"';
        for (var i = 0; i < node.attrs.length; i++) {
            var a = node.attrs[i];
            attrStr += ' ' + a.name + (a.value === null ? '' : '="' + a.value + '"');
        }
        if (VOID_TAGS[tag]) {
            out.push(pad + '<' + tag + attrStr + ' />');
            return;
        }
        var hasChildren = node.children && node.children.length;
        if (!hasChildren) {
            out.push(pad + '<' + tag + attrStr + '>' + (node.text || '') + '</' + tag + '>');
            return;
        }
        out.push(pad + '<' + tag + attrStr + '>');
        if (node.text) out.push(repeat('  ', indent + 1) + node.text);
        serializeForest(node.children, indent + 1, out);
        out.push(pad + '</' + tag + '>');
    }

    function repeat(s, n) { var r = ''; for (var i = 0; i < n; i++) r += s; return r; }

    // Public-ish: expand an abbreviation to HTML (no surrounding indent).
    // Returns null if the string is not a recognizable abbreviation.
    function expandEmmet(abbr) {
        abbr = (abbr || '').trim();
        if (!abbr) return null;
        // A lone plain word (e.g. "note") is a valid one-tag abbreviation,
        // but expanding "the" while typing prose would be infuriating.
        // Require at least one Emmet operator/suffix OR a known-ish tag
        // shape before treating it as an abbreviation.
        if (!/[>+^*#.\[\]{}]/.test(abbr) && !/^[a-zA-Z][a-zA-Z0-9]*$/.test(abbr)) return null;
        var forest;
        try {
            forest = parseAbbr(abbr);
        } catch (e) {
            return null;
        }
        if (!forest) return null;
        var out = [];
        serializeForest(forest, 0, out);
        return out.join('\n');
    }
    // Exposed for unit testing under Node (harmless in the browser).
    if (typeof window !== 'undefined') window.OMN_expandEmmet = expandEmmet;

    // ==================================================================
    // Textarea helpers
    // ==================================================================
    function lineBounds(value, caret) {
        var start = value.lastIndexOf('\n', caret - 1) + 1;
        var end = value.indexOf('\n', caret);
        if (end === -1) end = value.length;
        return { start: start, end: end };
    }

    function selectCurrentLine() {
        if (!ta) return;
        var b = lineBounds(ta.value, ta.selectionStart);
        ta.focus();
        ta.setSelectionRange(b.start, b.end);
    }

    // Expand the abbreviation on the current line (from first non-space to
    // the caret). Returns true if something was expanded.
    function expandEmmetAtCursor() {
        if (!ta) return false;
        var caret = ta.selectionStart;
        if (caret !== ta.selectionEnd) return false; // don't expand over a selection
        var b = lineBounds(ta.value, caret);
        var lineToCaret = ta.value.substring(b.start, caret);
        var lead = lineToCaret.match(/^\s*/)[0];
        var abbr = lineToCaret.slice(lead.length);
        if (!abbr) return false;
        var expanded = expandEmmet(abbr);
        if (expanded == null) return false;
        // Re-indent every produced line by the current line's leading space.
        var indented = expanded.split('\n').map(function (l, i) {
            return i === 0 ? lead + l : lead + l;
        }).join('\n');
        var before = ta.value.substring(0, b.start) + lead;
        var after = ta.value.substring(caret);
        ta.value = before + indented.slice(lead.length) + after;
        // Place the caret at the first empty ></ pair, else after insertion.
        var insertedAt = b.start;
        var full = ta.value;
        var emptyPair = full.indexOf('></', insertedAt);
        var caretPos;
        if (emptyPair !== -1 && emptyPair < insertedAt + lead.length + indented.length) {
            caretPos = emptyPair + 1;
        } else {
            caretPos = insertedAt + indented.length;
        }
        ta.focus();
        ta.setSelectionRange(caretPos, caretPos);
        markDirty();
        return true;
    }

    // ==================================================================
    // Load / save
    // ==================================================================
    function setStatus(msg, kind) {
        if (!statusEl) return;
        statusEl.textContent = msg || '';
        statusEl.className = 'editor-status' + (kind ? ' editor-status-' + kind : '');
    }

    function markDirty() {
        if (!dirty) {
            dirty = true;
            setStatus('Unsaved changes', 'dirty');
        }
    }

    async function loadContent() {
        setStatus('Loading…');
        try {
            var res = await fetch('/api/note?name=' + encodeURIComponent(NAME), { cache: 'no-store' });
            if (!res.ok) throw new Error('HTTP ' + res.status);
            ta.value = await res.text();
        } catch (e) {
            setStatus('Could not load note: ' + e.message, 'error');
            ta.value = '';
        }
        loaded = true;
        dirty = false;
        setStatus('Editing ' + NAME);
        ta.focus();
        // Caret at end so typing continues the note.
        var len = ta.value.length;
        ta.setSelectionRange(len, len);
    }

    async function save(thenView) {
        if (!loaded) return;
        setStatus('Saving…');
        var body = new URLSearchParams();
        body.append('name', NAME);
        body.append('content', ta.value);
        try {
            var res = await fetch('/api/save', { method: 'POST', body: body });
            if (res.status === 401 || res.status === 403) {
                setStatus('Not authorized — log in as admin on the note page to save.', 'error');
                return;
            }
            if (!res.ok) throw new Error('HTTP ' + res.status);
            dirty = false;
            if (thenView) {
                window.location.href = VIEW;
            } else {
                setStatus('Saved ' + NAME, 'ok');
            }
        } catch (e) {
            setStatus('Save failed: ' + e.message, 'error');
        }
    }

    function cancel() {
        if (dirty && !window.confirm('Discard unsaved changes?')) return;
        window.location.href = VIEW;
    }

    // ==================================================================
    // Wiring
    // ==================================================================
    function buildToolbar() {
        var bar = document.getElementById('editorTools');
        if (!bar) return;
        TOOLS.forEach(function (tool) {
            var b = document.createElement('button');
            b.type = 'button';
            b.className = 'editor-tool';
            b.title = tool.title;
            b.setAttribute('aria-label', tool.title);
            b.innerHTML = '<i class="material-icons">' + tool.icon + '</i>';
            b.addEventListener('click', function () { tool.action(ta); ta.focus(); });
            bar.appendChild(b);
        });
    }

    function onKeyDown(e) {
        // Tab: expand an Emmet abbreviation if one precedes the caret,
        // otherwise insert a real tab (never move focus away).
        if (e.key === 'Tab' && !e.shiftKey) {
            if (expandEmmetAtCursor()) {
                e.preventDefault();
                return;
            }
            e.preventDefault();
            insertAtCaret('\t');
            return;
        }
        // Ctrl/Cmd+S: save and return to the view.
        if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) {
            e.preventDefault();
            save(true);
            return;
        }
    }

    function insertAtCaret(text) {
        var s = ta.selectionStart, en = ta.selectionEnd;
        ta.value = ta.value.substring(0, s) + text + ta.value.substring(en);
        var caret = s + text.length;
        ta.setSelectionRange(caret, caret);
        markDirty();
    }

    async function setupDragDrop() {
        ta.addEventListener('dragover', function (e) { e.preventDefault(); });
        ta.addEventListener('drop', async function (e) {
            if (!e.dataTransfer || !e.dataTransfer.files || !e.dataTransfer.files.length) return;
            e.preventDefault();
            var fd = new FormData();
            fd.append('image', e.dataTransfer.files[0]);
            try {
                var res = await fetch('/api/upload', { method: 'POST', body: fd });
                if (res.ok) { insertAtCaret(await res.text()); }
            } catch (_) { /* ignore */ }
        });
    }

    function init() {
        ta = document.getElementById('editor');
        statusEl = document.getElementById('editorStatus');
        if (!ta) return;
        buildToolbar();
        ta.addEventListener('keydown', onKeyDown);
        ta.addEventListener('input', markDirty);
        setupDragDrop();

        var saveBtn = document.getElementById('editorSave');
        if (saveBtn) saveBtn.addEventListener('click', function () { save(true); });
        var cancelBtn = document.getElementById('editorCancel');
        if (cancelBtn) cancelBtn.addEventListener('click', cancel);

        window.addEventListener('beforeunload', function (e) {
            if (dirty) { e.preventDefault(); e.returnValue = ''; }
        });

        loadContent();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
