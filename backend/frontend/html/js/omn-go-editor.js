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
    var loadFailed = false;// the load errored - never save over the file
    var dirty = false;     // unsaved changes?
    var wrapOn = true;     // word wrap (default on, like a plain textarea)
    var lnOn = false;      // line numbers requested by the user

    // "Cycle selection" button state - see cycleSelection.
    var SEL_CYCLE_STAGES = 7;       // the number of stages in the cycle
    var selCycleStage = 0;          // 0 = idle; 1 to 7 = the stage applied last
    var selCycleCaret = -1;         // char offset: the cursor the cycle holds
    var selCycleAppliedStart = -1;  // the selection this tool applied last, to
    var selCycleAppliedEnd = -1;    // find a continued cycle after a new press

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
        { icon: 'format_line_spacing', title: 'Cycle selection (line, line end, line start, to file end, to header, body, whole note)', action: function () { cycleSelection(); } },
        { id: 'toolWrap', icon: 'wrap_text', title: 'Toggle word wrap', action: function () { toggleWrap(); } },
        { id: 'toolLn', icon: 'format_list_numbered', title: 'Toggle line numbers (off while wrapping)', action: function () { toggleLineNumbers(); } },
        { id: 'toolFind', icon: 'search', title: 'Find / replace (Ctrl+F, Ctrl+H)', action: function () { openFind(false); } }
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

    // Each press of the toolbar button applies the next stage of one cycle.
    // The cycle has seven stages:
    //   1. the current line
    //   2. from the cursor to the end of the line
    //   3. from the start of the line to the cursor
    //   4. from the current line to the end of the file
    //   5. from the current line to the first line after the Pelican-style
    //      header block (see firstLineAfterHeader)
    //   6. the note body: all text after the header block
    //   7. the whole note, with the header block
    // The eighth press starts the cycle again at stage 1.
    //
    // Stages 2 and 3 use the cursor. Stages 1, 4 and 5 use the line that
    // holds the cursor. Stages 6 and 7 use the whole file. The tool records
    // the cursor position when the cycle starts, and it keeps that position
    // for all seven stages. Each stage thus applies to the same line, and not
    // to the line at one end of the selection that the stage before it made.
    //
    // To find a continued cycle, this function compares the selection of the
    // textarea with the selection that it applied last. If the two agree, the
    // user pressed the button again and did not change the selection between
    // the two presses. The function then goes to the next stage. All other
    // conditions (a different line, a manual selection, a press after a move
    // of the cursor) start a new cycle at stage 1, at the cursor of that
    // moment.
    function cycleSelection() {
        if (!ta) return;
        var selStart = ta.selectionStart, selEnd = ta.selectionEnd;
        var continuing = selCycleStage > 0 &&
            selStart === selCycleAppliedStart && selEnd === selCycleAppliedEnd;

        if (!continuing) {
            selCycleCaret = selStart;
            selCycleStage = 0;
        }
        selCycleStage = (selCycleStage % SEL_CYCLE_STAGES) + 1;

        // The text can get shorter between two presses of the button. Hold
        // the cursor inside the text to keep every stage in range.
        var caret = Math.min(selCycleCaret, ta.value.length);
        var b = lineBounds(ta.value, caret);
        var headerEnd;
        var start, end;
        switch (selCycleStage) {
            case 1:
                start = b.start; end = b.end;                 // the current line
                break;
            case 2:
                start = caret; end = b.end;                   // cursor -> line end
                break;
            case 3:
                start = b.start; end = caret;                 // line start -> cursor
                break;
            case 4:
                start = b.start; end = ta.value.length;       // line -> end of file
                break;
            case 5:
                // line -> after the header block. This stage always holds the
                // full current line. Below the header block, the selection
                // goes from the boundary of the header block to the END of the
                // current line, as stage 4 goes to the end of the file. In the
                // header block, the selection goes from the START of the
                // current line to that same boundary. Math.min and Math.max on
                // the two raw offsets would remove the text of the current line
                // on the side below the header block (b.start to b.end would
                // not enter the range at all). This branch prevents that.
                headerEnd = firstLineAfterHeader(ta.value);
                if (b.start >= headerEnd) {
                    start = headerEnd; end = b.end;
                } else {
                    start = b.start; end = headerEnd;
                }
                break;
            case 6:
                // The body of the note: all text after the header block. A
                // note with no header block has no boundary, and
                // firstLineAfterHeader gives 0. This stage then selects the
                // same text as stage 7.
                start = firstLineAfterHeader(ta.value); end = ta.value.length;
                break;
            default:
                start = 0; end = ta.value.length;             // the whole note
                break;
        }

        ta.focus();
        ta.setSelectionRange(start, end);
        selCycleAppliedStart = start;
        selCycleAppliedEnd = end;
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

    // A load that failed is NOT the same as a file that does not exist.
    // 404 is the documented "open a path that does not exist yet and save
    // to create it" case, so an empty buffer is correct there. Any other
    // failure means the file may hold content this editor never received,
    // and saving would replace that content with an empty buffer - so the
    // editor keeps the error on screen and refuses to save (see save()).
    async function loadContent() {
        setStatus('Loading…');
        setDot('loading');
        try {
            var res = await fetch('/api/note?name=' + encodeURIComponent(NAME), { cache: 'no-store' });
            if (res.ok) {
                ta.value = await res.text();
            } else if (res.status === 404) {
                ta.value = '';
            } else {
                throw new Error('HTTP ' + res.status);
            }
        } catch (e) {
            loadFailed = true;
            ta.value = '';
            var sb = document.getElementById('editorSave');
            if (sb) sb.disabled = true;
        }
        loaded = true;
        dirty = false;
        if (loadFailed) {
            setStatus('Could not load ' + NAME + ' — save is off', 'error');
            setDot('error');
        } else {
            setStatus(NAME);      // just the name - no "Editing" prefix
            setDot('clean');
        }
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

    // isHeaderFirstLine is a direct port of the Go isHeaderFirstLine
    // (backend/header_block.go): the FIRST line of a note is a metadata key
    // line only when it contains ':' and does not start with a space, '#',
    // or '<'. Keep the two in sync.
    function isHeaderFirstLine(line) {
        if (line.charAt(line.length - 1) === '\r') line = line.slice(0, -1);
        if (line.indexOf(':') === -1) return false;
        var c = line.charAt(0);
        return c !== ' ' && c !== '#' && c !== '<';
    }

    // Returns the character offset of the first line after the note's
    // Pelican-style metadata header. This mirrors the backend's
    // parseHeaderBlock (backend/header_block.go) so the editor caret and the
    // server agree on where the header ends: a header exists only when the
    // first line is a metadata key line (isHeaderFirstLine); the body then
    // begins right after the header's terminating blank line. With no
    // header, the body is the whole file, so the caret starts at offset 0.
    // A header with no terminating blank line (a metadata-only note) puts
    // the caret at the end of the file.
    function firstLineAfterHeader(text) {
        var nl = text.indexOf('\n');
        var firstLine = nl === -1 ? text : text.slice(0, nl);
        if (!isHeaderFirstLine(firstLine)) return 0;
        var m = /\r?\n\r?\n/.exec(text);
        return m ? m.index + m[0].length : text.length;
    }

    async function save(thenView) {
        if (!loaded || loadFailed) return;
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
                // .replace(), not .href = - swaps the editor's own history
                // entry for VIEW instead of pushing a new one on top of it.
                // With .href, the editor page stayed in the back-stack: on
                // Android especially, pressing Back after a save landed you
                // right back in the editor instead of wherever you were
                // before opening it. .replace() drops the editor entry
                // entirely, so Back skips over it.
                window.location.replace(VIEW);
            } else {
                setStatus(NAME);
            }
        } catch (e) {
            setStatus('Save failed: ' + e.message, 'error');
        }
    }

    function cancel() {
        if (dirty && !window.confirm('Discard unsaved changes?')) return;
        // Same reasoning as the save(true) branch above - leaving the
        // editor (without saving) shouldn't leave it in the back-stack
        // either.
        window.location.replace(VIEW);
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
            // Find/replace modes are per-device preferences too: someone who
            // works in regex stays in regex across notes and restarts.
            matchCase = window.localStorage.getItem('omngo_find_case') === '1';
            wholeWord = window.localStorage.getItem('omngo_find_word') === '1';
            useRegex = window.localStorage.getItem('omngo_find_regex') === '1';
            replaceOpen = window.localStorage.getItem('omngo_find_replace') === '1';
        } catch (e) { /* storage unavailable - use defaults */ }
    }
    function savePrefs() {
        try {
            window.localStorage.setItem('omngo_editor_wrap', wrapOn ? '1' : '0');
            window.localStorage.setItem('omngo_editor_ln', lnOn ? '1' : '0');
            window.localStorage.setItem('omngo_find_case', matchCase ? '1' : '0');
            window.localStorage.setItem('omngo_find_word', wholeWord ? '1' : '0');
            window.localStorage.setItem('omngo_find_regex', useRegex ? '1' : '0');
            window.localStorage.setItem('omngo_find_replace', replaceOpen ? '1' : '0');
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
        // The mirror has to wrap exactly as the textarea does, or the marks
        // land on different rows from the words they belong to.
        if (mirrorEl) mirrorEl.classList.toggle('nowrap', !wrapOn);
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
    // Find / replace
    // ==================================================================
    //
    // Exact matching, deliberately - this is nothing to do with the fuzzy
    // search in omn-go-sse.js. That one answers "where did I write about
    // this"; this one has to answer "which characters am I about to
    // overwrite", and a fuzzy match has no defensible replacement.
    //
    // A <textarea> cannot colour its own contents, and setting its selection is
    // not enough on its own: Chromium draws no selection at all in a textarea
    // that is not focused, and it never is while you are typing in the find
    // field. So the matches are painted by a mirror layer underneath it
    // (renderMirror) - same text, same metrics, invisible ink, a <mark> behind
    // each hit showing through the transparent textarea. The selection is still
    // set, because that is what puts the caret in the right place when you
    // click back into the text and what Replace checks against.

    var findEl = null, findInput = null, replaceInput = null,
        findCountEl = null, findReplaceRow = null, findChevron = null,
        findCaseBtn = null, findWordBtn = null, findRegexBtn = null;

    var mirrorEl = null;
    var findOpen = false, replaceOpen = false;
    var matchCase = false, wholeWord = false, useRegex = false;
    var findMatches = [];     // [{start, end}] in the CURRENT text
    var findIndex = -1;       // which of them is selected
    var findInvalid = false;  // the pattern would not compile
    var findTimer = null;

    // Above this the count stops being a number anyone reads and starts being
    // a memory question. Nothing is hidden: the count says "1000+".
    var FIND_MAX = 1000;

    // Unicode-aware word boundaries. JavaScript's \b is defined on
    // [A-Za-z0-9_], so on Cyrillic text it fires in all the wrong places -
    // "\bприв\b" would match inside "привет". Property escapes need the "u"
    // flag, which is only added when whole-word is actually on: under "u"
    // some otherwise-legal patterns become errors, and a mode the user did
    // not ask for must not break their regex.
    var UNICODE_BOUNDARY = (function () {
        try {
            new RegExp('(?<![\\p{L}\\p{N}_])x(?![\\p{L}\\p{N}_])', 'u');
            return true;
        } catch (e) {
            return false;
        }
    })();

    function escapeRegex(s) {
        return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }

    // buildFindRegex returns a global RegExp, or null when the query is empty
    // or (in regex mode) will not compile.
    function buildFindRegex() {
        var q = findInput ? findInput.value : '';
        if (!q) return null;

        var body = useRegex ? q : escapeRegex(q);
        var flags = 'g' + (matchCase ? '' : 'i');

        if (wholeWord) {
            if (UNICODE_BOUNDARY) {
                body = '(?<![\\p{L}\\p{N}_])(?:' + body + ')(?![\\p{L}\\p{N}_])';
                flags += 'u';
            } else {
                body = '\\b(?:' + body + ')\\b';
            }
        }
        try {
            return new RegExp(body, flags);
        } catch (e) {
            return null;
        }
    }

    // collectMatches rebuilds the match list against the text as it is NOW.
    // Called before every navigation and every replace rather than cached,
    // because the document underneath can change between them - the user can
    // type in the textarea with the bar still open.
    function collectMatches() {
        findMatches = [];
        findInvalid = false;

        var q = findInput ? findInput.value : '';
        if (!q) return;

        var re = buildFindRegex();
        if (!re) {
            findInvalid = true;
            return;
        }

        var value = ta.value, m;
        while ((m = re.exec(value)) !== null) {
            // A zero-length match cannot be selected, cannot be replaced, and
            // would spin this loop forever. Skipped, and the lastIndex nudge
            // is what makes the loop terminate on patterns like "a*".
            if (m[0].length === 0) {
                re.lastIndex++;
                continue;
            }
            findMatches.push({ start: m.index, end: m.index + m[0].length });
            if (findMatches.length >= FIND_MAX) break;
        }
    }


    // renderMirror repaints the highlight layer.
    //
    // Built from text nodes and <mark> elements rather than an HTML string:
    // the content is the user's note, and innerHTML would make every "<" in it
    // a tag. Nothing here needs escaping precisely because nothing here is
    // parsed as markup.
    //
    // Only called while the bar is open; closing it empties the layer, so a
    // note being edited normally carries no cost at all.
    function renderMirror() {
        if (!mirrorEl) return;
        mirrorEl.textContent = '';
        if (!findOpen || !findMatches.length) return;

        var value = ta.value, at = 0, frag = document.createDocumentFragment();
        for (var i = 0; i < findMatches.length; i++) {
            var m = findMatches[i];
            if (m.start > at) {
                frag.appendChild(document.createTextNode(value.slice(at, m.start)));
            }
            var mark = document.createElement('mark');
            if (i === findIndex) mark.className = 'is-current';
            mark.textContent = value.slice(m.start, m.end);
            frag.appendChild(mark);
            at = m.end;
        }
        // The tail, plus a newline: a textarea shows a final empty line that a
        // div would collapse, and without it every mark after the last
        // wrapped line drifts up by one row.
        frag.appendChild(document.createTextNode(value.slice(at) + '\n'));
        mirrorEl.appendChild(frag);
        syncMirror();
    }

    function syncMirror() {
        if (!mirrorEl) return;
        mirrorEl.scrollTop = ta.scrollTop;
        mirrorEl.scrollLeft = ta.scrollLeft;
    }

    function renderFindCount() {
        if (!findCountEl) return;
        if (findInput && findInput.classList) {
            findInput.classList.toggle('is-invalid', findInvalid);
        }
        if (findInvalid) {
            findCountEl.textContent = 'bad pattern';
            return;
        }
        if (!findInput || !findInput.value) {
            findCountEl.textContent = '';
            return;
        }
        if (!findMatches.length) {
            findCountEl.textContent = 'no matches';
            return;
        }
        var total = findMatches.length >= FIND_MAX ? FIND_MAX + '+' : String(findMatches.length);
        findCountEl.textContent = (findIndex + 1) + ' / ' + total;
    }

    // selectMatch puts the caret on a match and scrolls it into view. The
    // textarea keeps focus on the find field, so the selection is drawn in the
    // browser's "inactive" colour - which is the right signal: that text is
    // marked, not being typed into.
    function selectMatch(i) {
        if (i < 0 || i >= findMatches.length) return;
        findIndex = i;
        var m = findMatches[i];
        ta.setSelectionRange(m.start, m.end);
        scrollToOffset(m.start);
        renderMirror();
        renderFindCount();
    }

    // findStep moves to the next/previous match, wrapping around, starting
    // from wherever the caret is rather than from the last index - so editing
    // in the middle of the document and pressing Enter continues from there.
    function findStep(dir) {
        collectMatches();
        if (!findMatches.length) {
            renderMirror();
            renderFindCount();
            return;
        }
        // Forward starts past the END of the selection and backward before its
        // START. Using one end for both is how a "next" button lands on the
        // match it is already sitting on and never moves.
        var i;
        if (dir > 0) {
            var after = ta.selectionEnd;
            for (i = 0; i < findMatches.length; i++) {
                if (findMatches[i].start >= after) break;
            }
            if (i >= findMatches.length) i = 0;   // wrap to the top
        } else {
            var before = ta.selectionStart;
            for (i = findMatches.length - 1; i >= 0; i--) {
                if (findMatches[i].end <= before) break;
            }
            if (i < 0) i = findMatches.length - 1; // wrap to the bottom
        }
        selectMatch(i);
    }

    // expandReplacement gives $1, $&, $$ their usual meaning in REGEX mode
    // only. In literal mode the replacement is literal: someone replacing a
    // price with "$5" is not writing a back-reference.
    //
    // Written out rather than handed to String.replace's own expansion,
    // because the single-match Replace has to reuse the exact same rules as
    // Replace all, and only a function replacer can pick out one match.
    function expandReplacement(rep, args) {
        if (!useRegex || rep.indexOf('$') === -1) return rep;
        var last = args.length - 1;
        // Named groups append an object; drop it before counting.
        if (typeof args[last] === 'object' && args[last] !== null) last--;
        var groups = last - 2;  // args: match, p1..pN, offset, string
        return rep.replace(/\$(\$|&|\d{1,2})/g, function (whole, tok) {
            if (tok === '$') return '$';
            if (tok === '&') return args[0];
            var n = parseInt(tok, 10);
            if (n >= 1 && n <= groups) return args[n] === undefined ? '' : args[n];
            return whole;   // "$7" with six groups is text, not a reference
        });
    }

    // applyReplace rewrites value. onlyIndex >= 0 replaces just that match
    // (counting the same non-empty matches collectMatches counts, so the two
    // agree on what "the third match" means); -1 replaces all.
    function applyReplace(value, re, rep, onlyIndex) {
        var seen = 0, count = 0;
        var out = value.replace(re, function () {
            var args = arguments, whole = args[0];
            if (whole.length === 0) return whole;   // skipped by collectMatches too
            var cur = seen++;
            if (onlyIndex >= 0 && cur !== onlyIndex) return whole;
            count++;
            return expandReplacement(rep, args);
        });
        return { text: out, count: count };
    }

    // setTextRange writes through execCommand where it exists, so the change
    // joins the textarea's native undo stack instead of wiping it - assigning
    // .value clears undo entirely, and losing the whole history to one
    // Replace all is a bad trade. The assignment is the fallback.
    function setTextRange(start, end, text) {
        ta.focus();
        ta.setSelectionRange(start, end);
        var ok = false;
        try {
            ok = !!(document.execCommand && document.execCommand('insertText', false, text));
        } catch (e) {
            ok = false;
        }
        if (!ok) {
            ta.value = ta.value.slice(0, start) + text + ta.value.slice(end);
            ta.setSelectionRange(start + text.length, start + text.length);
        }
        markDirty();
        renderGutter();
        renderMirror();
    }

    function replaceCurrent() {
        collectMatches();
        if (findInvalid || !findMatches.length) {
            renderFindCount();
            return;
        }
        // Replace what is SELECTED if the selection is one of the matches,
        // otherwise step to a match first. Pressing Replace without having
        // pressed Enter should not silently rewrite something off-screen.
        var i = -1, s = ta.selectionStart, e = ta.selectionEnd;
        for (var k = 0; k < findMatches.length; k++) {
            if (findMatches[k].start === s && findMatches[k].end === e) { i = k; break; }
        }
        if (i === -1) {
            findStep(1);
            return;
        }

        var re = buildFindRegex();
        if (!re) return;
        var rep = replaceInput ? replaceInput.value : '';
        var before = ta.value;
        var res = applyReplace(before, re, rep, i);
        if (!res.count) return;

        setTextRange(0, before.length, res.text);
        // Land the caret AFTER what was just written, then advance. Otherwise
        // replacing "a" with "ab" would find its own output and Replace would
        // never move on.
        var caret = findMatches[i].end + (res.text.length - before.length);
        ta.setSelectionRange(caret, caret);
        setStatus('Replaced 1 match', 'ok');
        findStep(1);
    }

    function replaceAll() {
        collectMatches();
        if (findInvalid) {
            renderFindCount();
            return;
        }
        var re = buildFindRegex();
        if (!re) return;
        var rep = replaceInput ? replaceInput.value : '';
        var caret = ta.selectionStart;
        var res = applyReplace(ta.value, re, rep, -1);
        if (!res.count) {
            setStatus('No matches to replace');
            renderFindCount();
            return;
        }
        // One write, so one undo step puts the whole document back.
        setTextRange(0, ta.value.length, res.text);
        ta.setSelectionRange(Math.min(caret, ta.value.length), Math.min(caret, ta.value.length));
        setStatus('Replaced ' + res.count + (res.count === 1 ? ' match' : ' matches'), 'ok');
        collectMatches();
        findIndex = -1;
        renderMirror();
        renderFindCount();
    }

    function scheduleFind() {
        clearTimeout(findTimer);
        findTimer = setTimeout(function () {
            collectMatches();
            findIndex = -1;
            renderMirror();
            renderFindCount();
            if (findMatches.length) findStep(1);
        }, 120);
    }

    function setFindFlag(which) {
        if (which === 'case') matchCase = !matchCase;
        if (which === 'word') wholeWord = !wholeWord;
        if (which === 'regex') useRegex = !useRegex;
        savePrefs();
        updateFindButtons();
        collectMatches();
        findIndex = -1;
        renderMirror();
        renderFindCount();
        if (findMatches.length) findStep(1);
    }

    function updateFindButtons() {
        function set(btn, on) {
            if (!btn) return;
            btn.classList.toggle('active', on);
            btn.setAttribute('aria-pressed', on ? 'true' : 'false');
        }
        set(findCaseBtn, matchCase);
        set(findWordBtn, wholeWord);
        set(findRegexBtn, useRegex);
        if (findChevron) {
            findChevron.title = replaceOpen ? 'Hide replace' : 'Show replace';
            findChevron.innerHTML = '<i class="material-icons">' +
                (replaceOpen ? 'expand_more' : 'chevron_right') + '</i>';
        }
        if (findReplaceRow) findReplaceRow.hidden = !replaceOpen;
    }

    function openFind(withReplace) {
        if (!findEl) return;
        if (withReplace) replaceOpen = true;
        findEl.hidden = false;
        findOpen = true;
        updateFindButtons();
        savePrefs();

        // Seed from the selection, the way every editor does: select a word,
        // press Ctrl+F, and it is already the query.
        var sel = ta.value.substring(ta.selectionStart, ta.selectionEnd);
        if (sel && sel.indexOf('\n') === -1 && findInput) findInput.value = sel;

        if (findInput) {
            findInput.focus();
            findInput.select();
        }
        collectMatches();
        findIndex = -1;
        renderMirror();
        renderFindCount();
    }

    function closeFind() {
        if (!findEl) return;
        findEl.hidden = true;
        findOpen = false;
        findMatches = [];
        findIndex = -1;
        renderMirror();   // empties the layer: no cost while not searching
        // Focus goes back to the text with the caret where the last match was,
        // so Esc leaves you where you were reading rather than at the top.
        ta.focus();
    }

    function toggleReplaceRow() {
        replaceOpen = !replaceOpen;
        updateFindButtons();
        savePrefs();
        if (replaceOpen && replaceInput) replaceInput.focus();
    }

    function wireFind() {
        mirrorEl = document.getElementById('editorMirror');
        findEl = document.getElementById('editorFind');
        if (!findEl) return;
        findInput = document.getElementById('findInput');
        replaceInput = document.getElementById('replaceInput');
        findCountEl = document.getElementById('findCount');
        findReplaceRow = document.getElementById('findReplaceRow');
        findChevron = document.getElementById('findToggleReplace');
        findCaseBtn = document.getElementById('findCase');
        findWordBtn = document.getElementById('findWord');
        findRegexBtn = document.getElementById('findRegex');

        if (findInput) {
            findInput.addEventListener('input', scheduleFind);
            findInput.addEventListener('keydown', function (e) {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    findStep(e.shiftKey ? -1 : 1);
                } else if (e.key === 'Escape') {
                    e.preventDefault();
                    closeFind();
                }
            });
        }
        if (replaceInput) {
            replaceInput.addEventListener('keydown', function (e) {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    replaceCurrent();
                } else if (e.key === 'Escape') {
                    e.preventDefault();
                    closeFind();
                }
            });
        }

        var byId = {
            findPrev: function () { findStep(-1); },
            findNext: function () { findStep(1); },
            findClose: closeFind,
            findToggleReplace: toggleReplaceRow,
            findCase: function () { setFindFlag('case'); },
            findWord: function () { setFindFlag('word'); },
            findRegex: function () { setFindFlag('regex'); },
            replaceOne: replaceCurrent,
            replaceAll: replaceAll
        };
        Object.keys(byId).forEach(function (id) {
            var b = document.getElementById(id);
            if (b) b.addEventListener('click', byId[id]);
        });

        updateFindButtons();
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

    // Find shortcuts live on the document, not on the textarea: Ctrl+F has to
    // work while the caret is in the find field too, and Escape has to close
    // the bar from wherever focus happens to be.
    //
    // Ctrl+F is taken from the browser deliberately. Inside a text editor the
    // browser's own find is the wrong tool - it searches the rendered page,
    // cannot see past the textarea's scroll, and cannot replace anything.
    function onDocKeyDown(e) {
        if (e.defaultPrevented) return;
        var ctrl = e.ctrlKey || e.metaKey;

        if (ctrl && (e.key === 'f' || e.key === 'F')) {
            e.preventDefault();
            openFind(false);
            return;
        }
        if (ctrl && (e.key === 'h' || e.key === 'H')) {
            e.preventDefault();
            openFind(true);
            return;
        }
        if (!findOpen) return;

        if (e.key === 'Escape') {
            e.preventDefault();
            closeFind();
            return;
        }
        if (e.key === 'F3' || (ctrl && (e.key === 'g' || e.key === 'G'))) {
            e.preventDefault();
            findStep(e.shiftKey ? -1 : 1);
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
            // Uploads can take a while on a phone (up to Max Upload Size,
            // 3 MB by default) and used to give no sign at all that anything
            // was happening - and a failure was swallowed entirely, so a
            // rejected file looked identical to a dropped one. Report both
            // through the status bar this page already has, rather than an
            // overlay: the upload runs while the user is mid-edit, so
            // covering the textarea would interrupt the actual task.
            setStatus('Uploading ' + file.name + '…');
            setDot('loading');
            try {
                var res = await fetch(uploadURL, { method: 'POST', body: fd });
                if (res.ok) {
                    insertAtCaret(await res.text());
                    setStatus(NAME);
                } else {
                    var why = '';
                    try { why = (await res.text()).trim(); } catch (_) {}
                    setStatus('Upload failed: ' + (why || ('HTTP ' + res.status)), 'error');
                }
            } catch (e) {
                setStatus('Upload failed: ' + e.message, 'error');
            }
            // insertAtCaret marks the buffer dirty on success; on failure the
            // previous state stands. Either way the dot must leave 'loading'.
            setDot(dirty ? 'dirty' : 'clean');
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
        ta.addEventListener('input', function () {
            markDirty();
            renderGutter();
            // The match list is rebuilt before every navigation anyway, but
            // the COUNT is on screen while you type - leaving it stale would
            // be the bar quietly lying about the document underneath it.
            if (findOpen) scheduleFind();
        });
        ta.addEventListener('scroll', function () { syncGutter(); syncMirror(); });
        document.addEventListener('keydown', onDocKeyDown);
        wireFind();
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
        updateFindButtons();   // loadPrefs ran after wireFind: repaint the flags
        loadContent();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
