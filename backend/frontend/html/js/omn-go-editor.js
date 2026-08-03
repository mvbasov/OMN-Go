// --- OMN-Go standalone note editor ---
//
// The editor loads the Markdown source from /api/note when it opens, so a
// rendered page never carries a hidden second copy of its own text. Saving
// posts to /api/save. The server template defines OMN_EDIT_NAME,
// OMN_EDIT_EXT and OMN_EDIT_VIEW.

(function () {
    'use strict';

    var NAME = (typeof OMN_EDIT_NAME !== 'undefined') ? OMN_EDIT_NAME : 'Welcome';
    var VIEW = (typeof OMN_EDIT_VIEW !== 'undefined' && OMN_EDIT_VIEW) ? OMN_EDIT_VIEW : '/';

    // Jump target from a clicked console error. "find" matches by line
    // content, which survives the Markdown to HTML line shift. "line" is
    // valid only for verbatim assets.
    var JUMP_FIND = null, JUMP_LINE = 0;
    try {
        var _q = new URLSearchParams(window.location.search);
        JUMP_FIND = _q.get('find');
        JUMP_LINE = parseInt(_q.get('line') || '0', 10) || 0;
    } catch (e) { /* skip the jump */ }

    var ta = null;
    var statusEl = null;
    var dotEl = null;      // footer dot: green saved, red unsaved
    var gutterEl = null;
    var wrapBtn = null, lnBtn = null;
    var loaded = false;
    var dirty = false;
    var wrapOn = true;
    var lnOn = false;

    var lineCycleStage = 0;          // 0 = idle, 1/2/3 = stage last applied
    var lineCycleAnchor = -1;        // char offset of the anchored line start
    var lineCycleAppliedStart = -1;  // the selection this tool set last
    var lineCycleAppliedEnd = -1;

    // Each entry becomes a toolbar button, left to right.
    var TOOLS = [
        { icon: 'code', title: 'Expand Emmet abbreviation (Tab)', action: function () { expandEmmetAtCursor(); } },
        { icon: 'format_line_spacing', title: 'Select line (click again: to end of file, then to after header, then repeats)', action: function () { selectCurrentLine(); } },
        { id: 'toolWrap', icon: 'wrap_text', title: 'Toggle word wrap', action: function () { toggleWrap(); } },
        { id: 'toolLn', icon: 'format_list_numbered', title: 'Toggle line numbers (off while wrapping)', action: function () { toggleLineNumbers(); } },
        { id: 'toolFind', icon: 'search', title: 'Find / replace (Ctrl+F, Ctrl+H)', action: function () { openFind(false); } }
    ];

    // Offline Emmet-style expander. It supports > + ( ) *N #id .cls
    // [attrs] {text} and $ numbering. Anything else, including "^",
    // yields no expansion, so Tab inserts a literal tab.
    var VOID_TAGS = {
        area: 1, base: 1, br: 1, col: 1, embed: 1, hr: 1, img: 1, input: 1,
        link: 1, meta: 1, param: 1, source: 1, track: 1, wbr: 1
    };
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

    // Returns a node forest, or null if str is not an abbreviation.
    function parseAbbr(str) {
        var pos = 0;
        var n = str.length;

        function peek() { return str[pos]; }

        function parseSiblings() {
            var nodes = [];
            var prev = null;
            while (pos < n) {
                var ch = str[pos];
                if (ch === ')') break;
                if (ch === '^') {
                    // A climb-up ends this sibling run. The parent resumes.
                    break;
                }
                var node = parseSingle();
                if (!node) return null;
                nodes.push(node);
                prev = node;
                if (pos < n) {
                    var op = str[pos];
                    if (op === '+') { pos++; continue; }
                    if (op === '>') {
                        pos++;
                        var kids = parseSiblingsUntilClimbOrClose();
                        if (kids === null) return null;
                        applyChildren(prev, kids.nodes);
                        var climb = kids.climb;
                        while (climb > 0 && pos <= n) {
                            // A climb attaches later siblings to an ancestor.
                            climb--;
                            if (climb === 0) break;
                        }
                        if (kids.climb > 0) {
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

        // Children after '>'. A trailing '^' count re-parents later siblings.
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
            var implied = IMPLICIT_CHILD[node.tag];
            for (var i = 0; i < kids.length; i++) {
                if (!kids[i].tag && !kids[i].group && implied) kids[i].tag = implied;
                if (!kids[i].tag && !kids[i].group) kids[i].tag = 'div';
            }
            node.children = node.children.concat(kids);
        }

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
            // Clamp the multiplier so a stray "*999999" cannot lock up the tab.
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
            // An empty tag name is legal. The parent supplies the implicit tag.
            while (pos < n && /[A-Za-z0-9:\-$]/.test(str[pos])) { node.tag += str[pos]; pos++; }
            var sawSuffix = false;
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

    // $ / $$ becomes the 1-based repeat index, zero-padded.
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

    // Returns unindented HTML for abbr, or null if abbr is not recognized.
    function expandEmmet(abbr) {
        abbr = (abbr || '').trim();
        if (!abbr) return null;
        // A bare word expands only when it is tag-shaped, so prose survives.
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
    // Exposed for unit tests under Node.
    if (typeof window !== 'undefined') window.OMN_expandEmmet = expandEmmet;

    // --- Textarea helpers ---
    function lineBounds(value, caret) {
        var start = value.lastIndexOf('\n', caret - 1) + 1;
        var end = value.indexOf('\n', caret);
        if (end === -1) end = value.length;
        return { start: start, end: end };
    }

    // Each press widens the scope: the line, the line to end of file, the
    // line to the header block boundary, then stage 1 again. The cycle
    // continues only while the selection matches the one set last.
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
            start = b.start; end = b.end;
        } else if (lineCycleStage === 2) {
            start = b.start; end = ta.value.length;
        } else {
            // The current line is always fully inside the range. Math.min
            // and Math.max on the raw offsets would drop it below the
            // header block.
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

    // Expands the abbreviation from the first non-space to the caret.
    function expandEmmetAtCursor() {
        if (!ta) return false;
        var caret = ta.selectionStart;
        if (caret !== ta.selectionEnd) return false; // no expanding over a selection
        var b = lineBounds(ta.value, caret);
        var lineToCaret = ta.value.substring(b.start, caret);
        var lead = lineToCaret.match(/^\s*/)[0];
        var abbr = lineToCaret.slice(lead.length);
        if (!abbr) return false;
        var expanded = expandEmmet(abbr);
        if (expanded == null) return false;
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

    // --- Load / save ---
    // The footer keeps the file name on the left. setStatus shows transient
    // messages. setDot shows the unsaved state.
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
        setStatus(NAME);
        setDot('clean');
        renderGutter();
        // Land on the error line, else on the first line after the header
        // block.
        if (!jumpToTarget()) {
            ta.focus();
            var pos = firstLineAfterHeader(ta.value);
            ta.setSelectionRange(pos, pos);
            scrollToOffset(pos);
        }
    }

    // Port of the Go isHeaderFirstLine (backend/frontmatter.go). Keep the
    // two in sync.
    function isHeaderFirstLine(line) {
        if (line.charAt(line.length - 1) === '\r') line = line.slice(0, -1);
        if (line.indexOf(':') === -1) return false;
        var c = line.charAt(0);
        return c !== ' ' && c !== '#' && c !== '<';
    }

    // Returns the offset of the first line after the header block. It
    // mirrors splitFrontMatter (backend/frontmatter.go). No header block
    // gives 0. A header block with no blank line gives the end of the file.
    function firstLineAfterHeader(text) {
        var nl = text.indexOf('\n');
        var firstLine = nl === -1 ? text : text.slice(0, nl);
        if (!isHeaderFirstLine(firstLine)) return 0;
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
                // .replace() drops the editor from the back-stack. With
                // .href, Back after a save returns to the editor.
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
        // Leaving without saving must not leave the editor in the back-stack.
        window.location.replace(VIEW);
    }

    // ==================================================================
    // Jump to a line
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
        ta.scrollTop = Math.max(0, (lineNo - 3) * lineHeightPx());
        syncGutter();
    }

    // jumpToTarget selects the target line. False when nothing matched.
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
    // Line numbers and word wrap
    // ==================================================================
    function loadPrefs() {
        try {
            var w = window.localStorage.getItem('omngo_editor_wrap');
            var l = window.localStorage.getItem('omngo_editor_ln');
            if (w !== null) wrapOn = w === '1';
            if (l !== null) lnOn = l === '1';
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

    // Line numbers need word wrap off: a wrapped line fills several rows, so
    // the gutter cannot stay in step.
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
        if (wrapOn) {
            ta.classList.remove('nowrap');
            ta.setAttribute('wrap', 'soft');
        } else {
            ta.classList.add('nowrap');
            ta.setAttribute('wrap', 'off');
        }
        // The mirror must wrap like the textarea, or marks land on wrong rows.
        if (mirrorEl) mirrorEl.classList.toggle('nowrap', !wrapOn);
        document.body.classList.toggle('ln-on', lineNumbersActive());
        renderGutter();
        updateToggleButtons();
    }
    function updateToggleButtons() {
        if (wrapBtn) wrapBtn.classList.toggle('active', wrapOn);
        if (lnBtn) {
            lnBtn.classList.toggle('active', lineNumbersActive());
            lnBtn.disabled = wrapOn;
        }
    }
    function toggleWrap() {
        wrapOn = !wrapOn;
        savePrefs();
        applyState();
    }
    function toggleLineNumbers() {
        if (wrapOn) return;
        lnOn = !lnOn;
        savePrefs();
        applyState();
    }


    // ==================================================================
    // Find / replace
    // ==================================================================
    // The find bar searches the Markdown source, so header block lines and
    // code blocks match like any other text. Matching is exact, not fuzzy.
    // Chromium draws no selection in an unfocused textarea, so renderMirror
    // paints the hits underneath.

    var findEl = null, findInput = null, replaceInput = null,
        findCountEl = null, findReplaceRow = null, findChevron = null,
        findCaseBtn = null, findWordBtn = null, findRegexBtn = null;

    var mirrorEl = null;
    var findOpen = false, replaceOpen = false;
    var matchCase = false, wholeWord = false, useRegex = false;
    var findMatches = [];     // [{start, end}] in the current text
    var findIndex = -1;       // the selected match
    var findInvalid = false;  // bad pattern: field red, counter says so
    var findTimer = null;

    // Above this cap the counter shows "1000+".
    var FIND_MAX = 1000;

    // JavaScript \b covers [A-Za-z0-9_] only, so whole-word matching uses
    // property escapes and works in every alphabet. Their "u" flag goes on
    // only with whole-word, because "u" rejects some otherwise legal patterns.
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

    // buildFindRegex returns a global RegExp, or null on empty or bad input.
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

    // Nothing is cached: the user can type with the find bar open.
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
            // The lastIndex nudge stops "a*" spinning this loop forever.
            if (m[0].length === 0) {
                re.lastIndex++;
                continue;
            }
            findMatches.push({ start: m.index, end: m.index + m[0].length });
            if (findMatches.length >= FIND_MAX) break;
        }
    }


    // renderMirror builds the highlight layer from text nodes and <mark>
    // elements. innerHTML would turn every "<" in the note into a tag.
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
        // A div collapses the final empty line, and marks below drift up.
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

    function selectMatch(i) {
        if (i < 0 || i >= findMatches.length) return;
        findIndex = i;
        var m = findMatches[i];
        ta.setSelectionRange(m.start, m.end);
        scrollToOffset(m.start);
        renderMirror();
        renderFindCount();
    }

    // findStep wraps around, and starts from the caret, not the last index.
    function findStep(dir) {
        collectMatches();
        if (!findMatches.length) {
            renderMirror();
            renderFindCount();
            return;
        }
        // One end for both directions makes Next stick on the current match.
        var i;
        if (dir > 0) {
            var after = ta.selectionEnd;
            for (i = 0; i < findMatches.length; i++) {
                if (findMatches[i].start >= after) break;
            }
            if (i >= findMatches.length) i = 0;
        } else {
            var before = ta.selectionStart;
            for (i = findMatches.length - 1; i >= 0; i--) {
                if (findMatches[i].end <= before) break;
            }
            if (i < 0) i = findMatches.length - 1;
        }
        selectMatch(i);
    }

    // expandReplacement gives $1, $& and $$ meaning in regex mode only. In
    // literal mode "$5" is text. Replace and Replace all share these rules.
    function expandReplacement(rep, args) {
        if (!useRegex || rep.indexOf('$') === -1) return rep;
        var last = args.length - 1;
        // Named groups append an object. Drop it.
        if (typeof args[last] === 'object' && args[last] !== null) last--;
        var groups = last - 2;  // args: match, p1..pN, offset, string
        return rep.replace(/\$(\$|&|\d{1,2})/g, function (whole, tok) {
            if (tok === '$') return '$';
            if (tok === '&') return args[0];
            var n = parseInt(tok, 10);
            if (n >= 1 && n <= groups) return args[n] === undefined ? '' : args[n];
            return whole;   // "$7" with six groups is text
        });
    }

    // applyReplace counts as collectMatches does. onlyIndex >= 0 replaces
    // that match, -1 replaces all.
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

    // setTextRange writes through execCommand, so a replace is one undo step.
    // Assigning .value clears undo, and is the fallback.
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
        // Replace must not rewrite a match that is off-screen.
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
        // Put the caret after the new text, or replacing "a" with "ab" finds
        // its own output.
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
        // One write, so Replace all is one undo step.
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
        renderMirror();   // empties the layer
        // Esc leaves the caret at the last match.
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
        // Tab expands an Emmet abbreviation or inserts a tab. Focus stays.
        if (e.key === 'Tab' && !e.shiftKey) {
            if (expandEmmetAtCursor()) {
                e.preventDefault();
                return;
            }
            e.preventDefault();
            insertAtCaret('\t');
            return;
        }
        if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) {
            e.preventDefault();
            save(true);
            return;
        }
    }

    // Find shortcuts live on the document: Ctrl+F must work from the find
    // field, and Escape from any focus. The browser's own find cannot help.
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
            // .json goes to the JSON upload endpoint: user_json/, and a plain
            // link. The extension is checked too: a dragged file often
            // arrives with a generic MIME type.
            var isJSON = /\.json$/i.test(file.name) || file.type === 'application/json';
            var uploadURL = isJSON ? '/api/upload_json' : '/api/upload';
            var fieldName = isJSON ? 'file' : 'image';
            var fd = new FormData();
            fd.append(fieldName, file);
            // Progress and failures go to the status bar. An overlay would
            // cover the textarea mid-edit. The image embed returned carries
            // omn-imported-image, which caps the rendered width.
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
            // The dot must leave 'loading' either way.
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
            // The visible counter must not go stale while you type.
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
        applyState();   // apply saved prefs before content loads
        updateFindButtons();   // loadPrefs ran after wireFind
        loadContent();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
