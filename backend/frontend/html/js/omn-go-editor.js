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

    // Optional jump target, set when arriving from a clicked console error
    // (see omn-go-core.js). "find" matches by line CONTENT - robust across
    // the markdown -> compiled-HTML line shift, since a note's <script> body
    // is passed through verbatim - while "line" is a direct 1-based number,
    // used for verbatim assets (.js/.css/.json) where lines map 1:1.
    var JUMP_FIND = null, JUMP_LINE = 0;
    try {
        var _q = new URLSearchParams(window.location.search);
        JUMP_FIND = _q.get('find');
        JUMP_LINE = parseInt(_q.get('line') || '0', 10) || 0;
    } catch (e) { /* no URLSearchParams / weird URL - just skip the jump */ }

    var ta = null;         // the <textarea>
    var statusEl = null;   // footer text (file name / transient status)
    var dotEl = null;      // footer state dot (green saved / red unsaved)
    var gutterEl = null;   // line-number gutter
    var wrapBtn = null, lnBtn = null; // the two toggle buttons
    var loaded = false;    // has the initial content arrived?
    var dirty = false;     // unsaved changes?
    var wrapOn = true;     // word wrap (default on, like a plain textarea)
    var lnOn = false;      // line numbers requested by the user

    // "Select current line" button cycle state - see selectCurrentLine.
    var lineCycleStage = 0;          // 0 = idle; 1/2/3 = which stage was last applied
    var lineCycleAnchor = -1;        // char offset: start of the line the cycle is anchored to
    var lineCycleAppliedStart = -1;  // selection this tool itself last set, to
    var lineCycleAppliedEnd = -1;    // detect "still cycling" vs. a fresh click

    // ------------------------------------------------------------------
    // Toolbar tool registry. Each entry becomes a button, left to right.
    // To add a tool later: append one { icon, title, action } object
    //   icon   - a Material Icons ligature name
    //   title  - tooltip / accessibility label
    //   action - function(textarea) invoked on click
    //   id     - optional element id, for stateful (toggle) buttons
    // ------------------------------------------------------------------
    var TOOLS = [
        { icon: 'code', title: 'Expand Emmet abbreviation (Tab)', action: function () { expandEmmetAtCursor(); } },
        { icon: 'format_line_spacing', title: 'Select line (click again: to end of file, then to after header, then repeats)', action: function () { selectCurrentLine(); } },
        { id: 'toolWrap', icon: 'wrap_text', title: 'Toggle word wrap', action: function () { toggleWrap(); } },
        { id: 'toolLn', icon: 'format_list_numbered', title: 'Toggle line numbers (off while wrapping)', action: function () { toggleLineNumbers(); } }
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

    // Cycles through three selection scopes each time the toolbar button
    // is clicked:
    //   1. the current line
    //   2. from the current line to the end of the file
    //   3. from the current line to the first line after the Pelican-style
    //      header (see firstLineAfterHeader) - whichever of the two is
    //      earlier in the file becomes the selection start, so this also
    //      works sensibly when the caret is inside the header itself.
    // A fourth click starts the cycle over at stage 1.
    //
    // "Continuing the cycle" is detected by comparing the textarea's
    // current selection to the one this function itself set last time: if
    // they still match, the user clicked the button again without
    // touching the selection in between, so advance to the next stage.
    // Anything else (a different line, a manual selection, a fresh click
    // after moving the caret) resets the cycle to stage 1, anchored on
    // whatever line the caret is on/selection starts at now.
    function selectCurrentLine() {
        if (!ta) return;
        var selStart = ta.selectionStart, selEnd = ta.selectionEnd;
        var continuing = lineCycleStage > 0 &&
            selStart === lineCycleAppliedStart && selEnd === lineCycleAppliedEnd;

        if (!continuing) {
            lineCycleAnchor = lineBounds(ta.value, selStart).start;
            lineCycleStage = 0;
        }
        lineCycleStage = (lineCycleStage % 3) + 1;

        var b = lineBounds(ta.value, lineCycleAnchor);
        var start, end;
        if (lineCycleStage === 1) {
            start = b.start; end = b.end;                    // current line only
        } else if (lineCycleStage === 2) {
            start = b.start; end = ta.value.length;           // line -> end of file
        } else {
            // line -> after header. The current line is always fully
            // included: below the header, select from the header boundary
            // through the END of the current line (mirroring stage 2's
            // "line -> end of file"); at or inside the header itself,
            // select from the START of the current line through the
            // header boundary. Using Math.min/max on the two raw offsets
            // instead would exclude the current line's own text on the
            // "below the header" side (b.start to b.end never entering
            // the range at all) - this branch avoids that.
            var headerEnd = firstLineAfterHeader(ta.value);
            if (b.start >= headerEnd) {
                start = headerEnd; end = b.end;
            } else {
                start = b.start; end = headerEnd;
            }
        }

        ta.focus();
        ta.setSelectionRange(start, end);
        lineCycleAppliedStart = start;
        lineCycleAppliedEnd = end;
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
    // The footer shows the file NAME persistently on the left; setStatus is
    // reused for the transient Loading/Saving/error messages. The unsaved
    // state is conveyed by the dot at the right (setDot), never by
    // overwriting the name.
    function setStatus(msg, kind) {
        if (!statusEl) return;
        statusEl.textContent = msg || '';
        statusEl.className = kind ? 'editor-status-' + kind : '';
    }

    function setDot(state) {
        if (!dotEl) return;
        dotEl.className = 'omn-editor-dot' + (state ? ' ' + state : '');
    }

    function markDirty() {
        if (!dirty) { dirty = true; setDot('dirty'); }
    }

    async function loadContent() {
        setStatus('Loading…');
        setDot('loading');
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
        setStatus(NAME);          // just the name - no "Editing" prefix
        setDot('clean');
        renderGutter();
        // Land on the error line if we arrived from a console error,
        // otherwise put the caret right after the Pelican-style header
        // (Title:/Date:/... - see ensureHeaderModified in
        // backend/markdown.go, which every note gets) so opening a note
        // drops you straight into its body instead of scrolled all the
        // way down to the end of the file.
        if (!jumpToTarget()) {
            ta.focus();
            var pos = firstLineAfterHeader(ta.value);
            ta.setSelectionRange(pos, pos);
            scrollToOffset(pos);
        }
    }

    // Returns the character offset of the first line after the note's
    // Pelican-style metadata header, i.e. right after the header's
    // terminating blank line - same "first blank line" rule
    // handleQuickNote (backend/handlers.go) and ensureHeaderModified
    // (backend/markdown.go) both use to find the end of that header.
    // Falls back to the end of the file when no blank line exists at all
    // (e.g. a one-line file with no header), matching the previous
    // "caret at end" behavior for that edge case.
    function firstLineAfterHeader(text) {
        var m = /\r?\n\r?\n/.exec(text);
        return m ? m.index + m[0].length : text.length;
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
            setDot('clean');
            if (thenView) {
                window.location.href = VIEW;
            } else {
                setStatus(NAME);
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
    // Jump to a line (arriving from a clicked console error)
    // ==================================================================
    function lineHeightPx() {
        var cs = window.getComputedStyle(ta);
        var lh = parseFloat(cs.lineHeight);
        if (!isFinite(lh)) lh = (parseFloat(cs.fontSize) || 14) * 1.5;
        return lh;
    }

    function scrollToOffset(off) {
        var before = ta.value.substring(0, off);
        var lineNo = (before.match(/\n/g) || []).length;
        // Keep a few lines of context above the target.
        ta.scrollTop = Math.max(0, (lineNo - 3) * lineHeightPx());
        syncGutter();
    }

    // Move the caret to the jump target, selecting the whole line. Returns
    // true if a target was found and applied.
    function jumpToTarget() {
        var val = ta.value, idx = -1;
        if (JUMP_FIND) {
            idx = val.indexOf(JUMP_FIND);
            if (idx === -1) {
                // Line-by-line fallback (e.g. leading indent differs).
                var lines = val.split('\n'), off = 0;
                for (var i = 0; i < lines.length; i++) {
                    if (lines[i].indexOf(JUMP_FIND) !== -1) { idx = off; break; }
                    off += lines[i].length + 1;
                }
            }
        } else if (JUMP_LINE > 0) {
            var ls = val.split('\n'), o = 0;
            for (var j = 0; j < JUMP_LINE - 1 && j < ls.length; j++) o += ls[j].length + 1;
            idx = o;
        }
        if (idx < 0) return false;
        var b = lineBounds(val, idx);
        ta.focus();
        ta.setSelectionRange(b.start, b.end);
        scrollToOffset(b.start);
        return true;
    }

    // ==================================================================
    // Line numbers + word wrap (persisted per-device in localStorage)
    // ==================================================================
    function loadPrefs() {
        try {
            var w = window.localStorage.getItem('omngo_editor_wrap');
            var l = window.localStorage.getItem('omngo_editor_ln');
            if (w !== null) wrapOn = w === '1';
            if (l !== null) lnOn = l === '1';
        } catch (e) { /* storage unavailable - use defaults */ }
    }
    function savePrefs() {
        try {
            window.localStorage.setItem('omngo_editor_wrap', wrapOn ? '1' : '0');
            window.localStorage.setItem('omngo_editor_ln', lnOn ? '1' : '0');
        } catch (e) { /* ignore */ }
    }

    // Line numbers are only shown when requested AND not wrapping.
    function lineNumbersActive() { return lnOn && !wrapOn; }

    function renderGutter() {
        if (!gutterEl) return;
        if (!lineNumbersActive()) { gutterEl.textContent = ''; gutterEl._n = -1; return; }
        var n = ta.value.split('\n').length;
        if (gutterEl._n === n) { syncGutter(); return; } // count unchanged
        gutterEl._n = n;
        var s = '';
        for (var i = 1; i <= n; i++) s += (i > 1 ? '\n' : '') + i;
        gutterEl.textContent = s;
        syncGutter();
    }
    function syncGutter() {
        if (gutterEl && lineNumbersActive()) gutterEl.scrollTop = ta.scrollTop;
    }

    function applyState() {
        // Word wrap.
        if (wrapOn) {
            ta.classList.remove('nowrap');
            ta.setAttribute('wrap', 'soft');
        } else {
            ta.classList.add('nowrap');
            ta.setAttribute('wrap', 'off');
        }
        // Line numbers (forced off while wrapping).
        document.body.classList.toggle('ln-on', lineNumbersActive());
        renderGutter();
        updateToggleButtons();
    }
    function updateToggleButtons() {
        if (wrapBtn) wrapBtn.classList.toggle('active', wrapOn);
        if (lnBtn) {
            lnBtn.classList.toggle('active', lineNumbersActive());
            lnBtn.disabled = wrapOn; // numbers make no sense while wrapping
        }
    }
    function toggleWrap() {
        wrapOn = !wrapOn;
        savePrefs();
        applyState();
    }
    function toggleLineNumbers() {
        if (wrapOn) return; // disabled while wrapping
        lnOn = !lnOn;
        savePrefs();
        applyState();
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
            if (tool.id) b.id = tool.id;
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
            var file = e.dataTransfer.files[0];
            // .json files go through the dedicated JSON upload endpoint,
            // which lands them in user_json/ (not images/) and returns a
            // plain "[name](/user_json/name)" link, not an image embed.
            // Checked by extension as well as MIME type since some OS
            // file managers hand the browser an empty/generic type for a
            // dragged file.
            var isJSON = /\.json$/i.test(file.name) || file.type === 'application/json';
            var uploadURL = isJSON ? '/api/upload_json' : '/api/upload';
            var fieldName = isJSON ? 'file' : 'image';
            var fd = new FormData();
            fd.append(fieldName, file);
            try {
                var res = await fetch(uploadURL, { method: 'POST', body: fd });
                if (res.ok) { insertAtCaret(await res.text()); }
            } catch (_) { /* ignore */ }
        });
    }

    function init() {
        ta = document.getElementById('editor');
        statusEl = document.getElementById('editorStatus');
        dotEl = document.getElementById('editorDot');
        gutterEl = document.getElementById('editorGutter');
        if (!ta) return;
        buildToolbar();
        wrapBtn = document.getElementById('toolWrap');
        lnBtn = document.getElementById('toolLn');

        ta.addEventListener('keydown', onKeyDown);
        ta.addEventListener('input', function () { markDirty(); renderGutter(); });
        ta.addEventListener('scroll', syncGutter);
        setupDragDrop();

        var saveBtn = document.getElementById('editorSave');
        if (saveBtn) saveBtn.addEventListener('click', function () { save(true); });
        var cancelBtn = document.getElementById('editorCancel');
        if (cancelBtn) cancelBtn.addEventListener('click', cancel);

        window.addEventListener('beforeunload', function (e) {
            if (dirty) { e.preventDefault(); e.returnValue = ''; }
        });

        loadPrefs();
        applyState();   // apply wrap/line-number prefs before content loads
        loadContent();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
