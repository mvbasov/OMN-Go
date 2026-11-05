// A small document, for the tests that press a control and read the page.
//
// WHY THIS FILE EXISTS. page-stub.js gives each script an element that does
// nothing. That is enough to load a script and to find a free variable. It
// is not enough for a test of behaviour. There, a press of a button must
// reach a listener, and the test must read the class or the text that the
// script wrote.
//
// The project has no package.json and no node_modules, thus no test can use
// a DOM library. See CLAUDE.md section 4. This file holds the part of a
// document that the scripts of the application use, and nothing more.
//
// WHAT IT HOLDS:
//
//	Elements and text nodes, with the tree methods.
//	Attributes, classList, dataset and style.
//	innerHTML, with a parser for the markup of the templates.
//	querySelector with tag, id, class and attribute selectors.
//	Events with the capture phase and the bubble phase.
//	A TreeWalker for text nodes.
//	FormData, which reads the controls of a form.
//
// WHAT IT DOES NOT HOLD. It has no layout, no CSS and no network. A method
// such as scrollIntoView does nothing. A test that needs a size or a
// position cannot use this file.
//
// It is not a browser. A test here proves what a script DOES with a
// document. A check in a real browser proves that the browser agrees.
//
// This file is NOT under frontend/html, thus frontend.Static does not embed
// it and no byte of it reaches a device.

'use strict';

const VOID_TAGS = { AREA: 1, BASE: 1, BR: 1, COL: 1, EMBED: 1, HR: 1, IMG: 1, INPUT: 1, LINK: 1, META: 1, SOURCE: 1, WBR: 1 };
const RAW_TEXT_TAGS = { SCRIPT: 1, STYLE: 1 };

const ENTITIES = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: ' ' };

function decodeEntities(s) {
    return s.replace(/&(#x[0-9a-f]+|#[0-9]+|[a-z]+);/gi, function (all, name) {
        if (name[0] === '#') {
            const code = name[1] === 'x' || name[1] === 'X'
                ? parseInt(name.slice(2), 16) : parseInt(name.slice(1), 10);
            return String.fromCodePoint(code);
        }
        return Object.prototype.hasOwnProperty.call(ENTITIES, name) ? ENTITIES[name] : all;
    });
}

function escapeText(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function escapeAttr(s) {
    return escapeText(s).replace(/"/g, '&quot;');
}

// ---------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------

class Event {
    constructor(type, init) {
        init = init || {};
        this.type = type;
        this.bubbles = !!init.bubbles;
        this.cancelable = init.cancelable !== false;
        this.defaultPrevented = false;
        this.target = null;
        this.currentTarget = null;
        this.button = 0;
        this._stopped = false;
        for (const key of Object.keys(init)) {
            if (key !== 'bubbles' && key !== 'cancelable') this[key] = init[key];
        }
    }
    preventDefault() { if (this.cancelable) this.defaultPrevented = true; }
    stopPropagation() { this._stopped = true; }
    stopImmediatePropagation() { this._stopped = true; this._stoppedNow = true; }
}

class EventTarget {
    constructor() { this._listeners = []; }
    addEventListener(type, fn, options) {
        const capture = options === true || !!(options && options.capture);
        const once = !!(options && options.once);
        this._listeners.push({ type: type, fn: fn, capture: capture, once: once });
    }
    removeEventListener(type, fn) {
        this._listeners = this._listeners.filter(function (l) {
            return !(l.type === type && l.fn === fn);
        });
    }
    // _run calls the listeners of this target for one phase.
    _run(event, capture) {
        event.currentTarget = this;
        for (const l of this._listeners.slice()) {
            if (l.type !== event.type || l.capture !== capture) continue;
            if (l.once) this._listeners.splice(this._listeners.indexOf(l), 1);
            l.fn.call(this, event);
            if (event._stoppedNow) return;
        }
        // The property form, for example el.onclick = fn. It runs in the
        // bubble phase.
        if (!capture && typeof this['on' + event.type] === 'function') {
            this['on' + event.type](event);
        }
    }
    // dispatchEvent sends the event down to the target and then up again,
    // the same as a browser does. The path ends at the window.
    dispatchEvent(event) {
        event.target = this;
        const path = [];
        for (let n = this; n; n = n._eventParent()) path.push(n);
        for (let i = path.length - 1; i >= 1 && !event._stopped; i--) path[i]._run(event, true);
        if (!event._stopped) {
            this._run(event, true);
            if (!event._stoppedNow) this._run(event, false);
        }
        if (event.bubbles) {
            for (let i = 1; i < path.length && !event._stopped; i++) path[i]._run(event, false);
        }
        return !event.defaultPrevented;
    }
    _eventParent() { return null; }
}

// ---------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------

class Node extends EventTarget {
    constructor(doc) {
        super();
        this.ownerDocument = doc;
        this.parentNode = null;
        this.childNodes = [];
    }
    _eventParent() {
        if (this.parentNode) return this.parentNode;
        if (this.nodeType === 9) return this.defaultView || null;
        return null;
    }
    get parentElement() { return this.parentNode && this.parentNode.nodeType === 1 ? this.parentNode : null; }
    get firstChild() { return this.childNodes[0] || null; }
    get lastChild() { return this.childNodes[this.childNodes.length - 1] || null; }
    get nextSibling() {
        if (!this.parentNode) return null;
        const s = this.parentNode.childNodes;
        return s[s.indexOf(this) + 1] || null;
    }
    get previousSibling() {
        if (!this.parentNode) return null;
        const s = this.parentNode.childNodes;
        return s[s.indexOf(this) - 1] || null;
    }
    get isConnected() {
        for (let n = this; n; n = n.parentNode) if (n.nodeType === 9) return true;
        return false;
    }
    contains(other) {
        for (let n = other; n; n = n.parentNode) if (n === this) return true;
        return false;
    }
    appendChild(child) { return this.insertBefore(child, null); }
    insertBefore(child, before) {
        if (child.nodeType === 11) {
            for (const c of child.childNodes.slice()) this.insertBefore(c, before);
            return child;
        }
        if (child.parentNode) child.parentNode.removeChild(child);
        const at = before ? this.childNodes.indexOf(before) : -1;
        if (at < 0) this.childNodes.push(child); else this.childNodes.splice(at, 0, child);
        child.parentNode = this;
        return child;
    }
    removeChild(child) {
        const at = this.childNodes.indexOf(child);
        if (at < 0) throw new Error('mini-dom: removeChild of a node that is not a child');
        this.childNodes.splice(at, 1);
        child.parentNode = null;
        return child;
    }
    replaceChild(fresh, old) {
        this.insertBefore(fresh, old);
        this.removeChild(old);
        return old;
    }
    remove() { if (this.parentNode) this.parentNode.removeChild(this); }
    // replaceChildren takes each child away and adds the new nodes.
    replaceChildren() {
        for (const c of this.childNodes.slice()) this.removeChild(c);
        for (const c of arguments) this.appendChild(c);
    }
    // after puts the new nodes behind this node, in their order.
    after() {
        const before = this.nextSibling;
        for (const c of arguments) this.parentNode.insertBefore(c, before);
    }
    get textContent() {
        return this.childNodes.map(function (c) { return c.textContent; }).join('');
    }
    set textContent(value) {
        for (const c of this.childNodes.slice()) this.removeChild(c);
        if (value !== '' && value !== null && value !== undefined) {
            this.appendChild(new Text(this.ownerDocument, String(value)));
        }
    }
}

class Text extends Node {
    constructor(doc, data) { super(doc); this.nodeType = 3; this.nodeName = '#text'; this.data = data; }
    get textContent() { return this.data; }
    set textContent(v) { this.data = String(v); }
    get nodeValue() { return this.data; }
    set nodeValue(v) { this.data = String(v); }
    get length() { return this.data.length; }
    // splitText cuts this node at offset and answers the new second node.
    splitText(offset) {
        const rest = new Text(this.ownerDocument, this.data.slice(offset));
        this.data = this.data.slice(0, offset);
        if (this.parentNode) this.parentNode.insertBefore(rest, this.nextSibling);
        return rest;
    }
}

class Comment extends Node {
    constructor(doc, data) { super(doc); this.nodeType = 8; this.nodeName = '#comment'; this.data = data; }
    get textContent() { return ''; }
    set textContent(v) { this.data = String(v); }
}

class Fragment extends Node {
    constructor(doc) { super(doc); this.nodeType = 11; this.nodeName = '#document-fragment'; }
}

// REFLECTED lists the properties that are the same as an attribute.
const REFLECTED = ['id', 'title', 'name', 'type', 'href', 'src', 'placeholder', 'rel', 'target', 'download', 'accept'];
// FLAGS lists the properties that are true when the attribute is present.
const FLAGS = ['disabled', 'hidden', 'multiple', 'readOnly', 'open'];

class Element extends Node {
    constructor(doc, tagName) {
        super(doc);
        this.nodeType = 1;
        this.tagName = tagName.toUpperCase();
        this.nodeName = this.tagName;
        this._attrs = new Map();
        this.style = {};
        const el = this;
        this.classList = {
            _list() { return (el.getAttribute('class') || '').split(/\s+/).filter(Boolean); },
            contains(c) { return this._list().indexOf(c) >= 0; },
            add() {
                const list = this._list();
                for (const c of arguments) if (list.indexOf(c) < 0) list.push(c);
                el.setAttribute('class', list.join(' '));
            },
            remove() {
                const drop = Array.prototype.slice.call(arguments);
                el.setAttribute('class', this._list().filter(function (c) { return drop.indexOf(c) < 0; }).join(' '));
            },
            toggle(c, force) {
                const want = force === undefined ? !this.contains(c) : !!force;
                if (want) this.add(c); else this.remove(c);
                return want;
            },
        };
        this.dataset = new Proxy({}, {
            get(_, key) {
                if (typeof key !== 'string') return undefined;
                const v = el.getAttribute(dataName(key));
                return v === null ? undefined : v;
            },
            set(_, key, value) { el.setAttribute(dataName(key), String(value)); return true; },
            has(_, key) { return el.hasAttribute(dataName(key)); },
            deleteProperty(_, key) { el.removeAttribute(dataName(key)); return true; },
        });
    }
    getAttribute(name) { return this._attrs.has(name) ? this._attrs.get(name) : null; }
    setAttribute(name, value) { this._attrs.set(name, String(value)); }
    hasAttribute(name) { return this._attrs.has(name); }
    removeAttribute(name) { this._attrs.delete(name); }
    get className() { return this.getAttribute('class') || ''; }
    set className(v) { this.setAttribute('class', v); }
    get children() { return this.childNodes.filter(function (c) { return c.nodeType === 1; }); }
    get firstElementChild() { return this.children[0] || null; }
    get nextElementSibling() {
        for (let n = this.nextSibling; n; n = n.nextSibling) if (n.nodeType === 1) return n;
        return null;
    }
    append() { for (const c of arguments) this.appendChild(typeof c === 'string' ? new Text(this.ownerDocument, c) : c); }
    get innerText() { return this.textContent; }
    set innerText(v) { this.textContent = v; }

    // value is a property of its own, the same as in a browser. A typed
    // value does not change the attribute.
    get value() {
        if (this._value !== undefined) return this._value;
        if (this.tagName === 'TEXTAREA') return this.textContent;
        if (this.tagName === 'SELECT') {
            const options = this.querySelectorAll('option');
            const chosen = options.find(function (o) { return o.hasAttribute('selected'); }) || options[0];
            return chosen ? (chosen.getAttribute('value') !== null ? chosen.getAttribute('value') : chosen.textContent) : '';
        }
        return this.getAttribute('value') || '';
    }
    // A write of value moves the caret to the end of the text, the same as
    // in a browser.
    set value(v) {
        this._value = String(v);
        this.selectionStart = this._value.length;
        this.selectionEnd = this._value.length;
    }
    get checked() { return this._checked !== undefined ? this._checked : this.hasAttribute('checked'); }
    set checked(v) { this._checked = !!v; }
    get form() { return this.closest('form'); }

    get innerHTML() { return this.childNodes.map(serialize).join(''); }
    set innerHTML(html) {
        for (const c of this.childNodes.slice()) this.removeChild(c);
        parseInto(this, String(html));
    }
    get outerHTML() { return serialize(this); }
    insertAdjacentHTML(where, html) {
        const holder = new Fragment(this.ownerDocument);
        parseInto(holder, String(html));
        if (where === 'beforeend') this.insertBefore(holder, null);
        else if (where === 'afterbegin') this.insertBefore(holder, this.firstChild);
        else if (where === 'beforebegin') this.parentNode.insertBefore(holder, this);
        else if (where === 'afterend') this.parentNode.insertBefore(holder, this.nextSibling);
        else throw new Error('mini-dom: insertAdjacentHTML does not know "' + where + '"');
    }

    matches(selector) { return parseSelector(selector).some((s) => matchesComplex(this, s)); }
    closest(selector) {
        for (let n = this; n && n.nodeType === 1; n = n.parentNode) if (n.matches(selector)) return n;
        return null;
    }
    querySelectorAll(selector) {
        const list = parseSelector(selector);
        const out = [];
        walkElements(this, function (el) {
            if (list.some(function (s) { return matchesComplex(el, s); })) out.push(el);
        });
        return out;
    }
    querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
    getElementsByTagName(name) { return this.querySelectorAll(name); }
    getElementsByClassName(name) { return this.querySelectorAll('.' + name); }

    click() {
        if (this.disabled) return;
        this.dispatchEvent(new Event('click', { bubbles: true }));
    }
    focus() { this.ownerDocument.activeElement = this; }
    blur() { if (this.ownerDocument.activeElement === this) this.ownerDocument.activeElement = this.ownerDocument.body; }
    select() { this._selected = true; }
    setSelectionRange(a, b) { this.selectionStart = a; this.selectionEnd = b; }
    // scrollIntoView keeps the options of each call in _scrolls, thus a
    // test can tell a scroll to the center from a plain scroll.
    scrollIntoView(options) {
        this._scrolledIntoView = true;
        this._scrolls = (this._scrolls || []).concat([options || null]);
    }
    getBoundingClientRect() { return { top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0 }; }
    // offsetParent is null for an element that a class or a style hides.
    // The scripts use it as "is this element on screen".
    get offsetParent() {
        for (let n = this; n && n.nodeType === 1; n = n.parentNode) {
            if (n.style.display === 'none' || n.classList.contains('omn-hidden') || n.hasAttribute('hidden')) return null;
        }
        return this.parentNode;
    }
}

for (const name of REFLECTED) {
    Object.defineProperty(Element.prototype, name, {
        get() { return this.getAttribute(name) || ''; },
        set(v) { this.setAttribute(name, v); },
    });
}
for (const name of FLAGS) {
    Object.defineProperty(Element.prototype, name, {
        get() { return this.hasAttribute(name.toLowerCase()); },
        set(v) { if (v) this.setAttribute(name.toLowerCase(), ''); else this.removeAttribute(name.toLowerCase()); },
    });
}

// dataName answers the attribute of a dataset key: omnLabel is
// data-omn-label.
function dataName(key) {
    return 'data-' + key.replace(/[A-Z]/g, function (c) { return '-' + c.toLowerCase(); });
}

function walkElements(root, visit) {
    for (const c of root.childNodes) {
        if (c.nodeType !== 1) continue;
        visit(c);
        walkElements(c, visit);
    }
}

// ---------------------------------------------------------------------
// The markup: parse and serialize
// ---------------------------------------------------------------------

// parseInto reads markup and adds its nodes to parent. It knows tags,
// attributes, text, comments, the void tags and the raw text of script
// and style. It closes a tag that the markup leaves open at the end.
function parseInto(parent, html) {
    const doc = parent.ownerDocument;
    const stack = [parent];
    const top = function () { return stack[stack.length - 1]; };
    let i = 0;
    while (i < html.length) {
        if (html.startsWith('<!--', i)) {
            const end = html.indexOf('-->', i + 4);
            const stop = end < 0 ? html.length : end;
            top().appendChild(new Comment(doc, html.slice(i + 4, stop)));
            i = end < 0 ? html.length : end + 3;
            continue;
        }
        if (html.startsWith('<!', i)) {
            const end = html.indexOf('>', i);
            i = end < 0 ? html.length : end + 1;
            continue;
        }
        if (html.startsWith('</', i)) {
            const end = html.indexOf('>', i);
            const name = html.slice(i + 2, end < 0 ? html.length : end).trim().toUpperCase();
            for (let k = stack.length - 1; k >= 1; k--) {
                if (stack[k].tagName === name) { stack.length = k; break; }
            }
            i = end < 0 ? html.length : end + 1;
            continue;
        }
        const open = /^<([a-zA-Z][a-zA-Z0-9-]*)/.exec(html.slice(i, i + 64));
        if (open) {
            const el = new Element(doc, open[1]);
            i += open[0].length;
            const attr = /^\s*([^\s=>/]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+)))?/;
            for (;;) {
                const rest = html.slice(i);
                const closeTag = /^\s*(\/?)>/.exec(rest);
                if (closeTag) { i += closeTag[0].length; break; }
                const m = attr.exec(rest);
                if (!m || m[0].length === 0) { i = html.length; break; }
                const raw = m[2] !== undefined ? m[2] : m[3] !== undefined ? m[3] : m[4];
                el.setAttribute(m[1], raw === undefined ? '' : decodeEntities(raw));
                i += m[0].length;
            }
            top().appendChild(el);
            if (VOID_TAGS[el.tagName]) continue;
            if (RAW_TEXT_TAGS[el.tagName]) {
                const end = html.toLowerCase().indexOf('</' + el.tagName.toLowerCase(), i);
                const stop = end < 0 ? html.length : end;
                if (stop > i) el.appendChild(new Text(doc, html.slice(i, stop)));
                const gt = end < 0 ? -1 : html.indexOf('>', end);
                i = gt < 0 ? html.length : gt + 1;
                continue;
            }
            stack.push(el);
            continue;
        }
        const next = html.indexOf('<', i + 1);
        const stop = next < 0 ? html.length : next;
        top().appendChild(new Text(doc, decodeEntities(html.slice(i, stop))));
        i = stop;
    }
}

function serialize(node) {
    if (node.nodeType === 3) return escapeText(node.data);
    if (node.nodeType === 8) return '<!--' + node.data + '-->';
    if (node.nodeType !== 1) return node.childNodes.map(serialize).join('');
    const tag = node.tagName.toLowerCase();
    let out = '<' + tag;
    for (const [name, value] of node._attrs) out += ' ' + name + '="' + escapeAttr(value) + '"';
    out += '>';
    if (VOID_TAGS[node.tagName]) return out;
    if (RAW_TEXT_TAGS[node.tagName]) return out + node.textContent + '</' + tag + '>';
    return out + node.childNodes.map(serialize).join('') + '</' + tag + '>';
}

// ---------------------------------------------------------------------
// Selectors
// ---------------------------------------------------------------------

const selectorCache = new Map();

// parseSelector answers a list of complex selectors. Each one is a list of
// {combinator, compound} from left to right. A compound is a list of
// simple tests.
function parseSelector(text) {
    if (selectorCache.has(text)) return selectorCache.get(text);
    const out = [];
    let i = 0;
    const src = text.trim();
    const fail = function (why) { throw new Error('mini-dom: selector "' + text + '": ' + why); };

    function readName() {
        const m = /^(?:\\.|[a-zA-Z0-9_-])+/.exec(src.slice(i));
        if (!m) fail('a name is absent at ' + i);
        i += m[0].length;
        return m[0].replace(/\\(.)/g, '$1');
    }
    function readCompound() {
        const tests = [];
        for (;;) {
            const c = src[i];
            if (c === '*') { i++; tests.push(function () { return true; }); }
            else if (c === '#') { i++; const id = readName(); tests.push(function (el) { return el.getAttribute('id') === id; }); }
            else if (c === '.') { i++; const cls = readName(); tests.push(function (el) { return el.classList.contains(cls); }); }
            else if (c === '[') {
                const m = /^\[\s*([a-zA-Z0-9_-]+)\s*(?:([\^$*~|]?=)\s*(?:"([^"]*)"|'([^']*)'|([^\]\s]+)))?\s*\]/.exec(src.slice(i));
                if (!m) fail('an attribute test is wrong at ' + i);
                i += m[0].length;
                const name = m[1], op = m[2];
                const want = m[3] !== undefined ? m[3] : m[4] !== undefined ? m[4] : m[5];
                tests.push(function (el) {
                    const got = el.getAttribute(name);
                    if (got === null) return false;
                    if (!op) return true;
                    if (op === '=') return got === want;
                    if (op === '^=') return got.startsWith(want);
                    if (op === '$=') return got.endsWith(want);
                    if (op === '*=') return got.indexOf(want) >= 0;
                    if (op === '~=') return got.split(/\s+/).indexOf(want) >= 0;
                    return got === want || got.startsWith(want + '-');
                });
            }
            else if (c === ':') {
                i++;
                const name = readName();
                if (name === 'not') {
                    if (src[i] !== '(') fail(':not needs an argument');
                    i++;
                    const inner = readCompound();
                    if (src[i] !== ')') fail(':not is not closed');
                    i++;
                    tests.push(function (el) { return !inner.every(function (t) { return t(el); }); });
                }
                else if (name === 'checked') tests.push(function (el) { return el.checked; });
                else if (name === 'disabled') tests.push(function (el) { return el.disabled; });
                else if (name === 'first-child') tests.push(function (el) { return el.parentNode && el.parentNode.children[0] === el; });
                else if (name === 'last-child') tests.push(function (el) { const s = el.parentNode ? el.parentNode.children : []; return s[s.length - 1] === el; });
                else fail('the pseudo-class :' + name + ' is not in mini-dom');
            }
            else if (c !== undefined && /[a-zA-Z]/.test(c)) {
                const tag = readName().toUpperCase();
                tests.push(function (el) { return el.tagName === tag; });
            }
            else break;
        }
        if (tests.length === 0) fail('a selector is empty at ' + i);
        return tests;
    }

    let current = [];
    let combinator = null;
    while (i < src.length) {
        current.push({ combinator: combinator, compound: readCompound() });
        const m = /^\s*([>+~,])\s*|^\s+/.exec(src.slice(i));
        if (!m) break;
        i += m[0].length;
        if (m[1] === ',') { out.push(current); current = []; combinator = null; }
        else combinator = m[1] || ' ';
    }
    if (i < src.length) fail('a part is not understood at ' + i);
    if (current.length) out.push(current);
    selectorCache.set(text, out);
    return out;
}

function matchesCompound(el, compound) {
    return compound.every(function (t) { return t(el); });
}

// matchesComplex tests el against one complex selector, from right to left.
function matchesComplex(el, parts) {
    function step(node, at) {
        if (!matchesCompound(node, parts[at].compound)) return false;
        if (at === 0) return true;
        const comb = parts[at].combinator;
        if (comb === '>') {
            return !!node.parentNode && node.parentNode.nodeType === 1 && step(node.parentNode, at - 1);
        }
        if (comb === '+') {
            let p = node.previousSibling;
            while (p && p.nodeType !== 1) p = p.previousSibling;
            return !!p && step(p, at - 1);
        }
        if (comb === '~') {
            for (let p = node.previousSibling; p; p = p.previousSibling) {
                if (p.nodeType === 1 && step(p, at - 1)) return true;
            }
            return false;
        }
        for (let p = node.parentNode; p && p.nodeType === 1; p = p.parentNode) {
            if (step(p, at - 1)) return true;
        }
        return false;
    }
    return step(el, parts.length - 1);
}

// ---------------------------------------------------------------------
// The document
// ---------------------------------------------------------------------

class Document extends Node {
    constructor() {
        super(null);
        this.ownerDocument = this;
        this.nodeType = 9;
        this.nodeName = '#document';
        this.readyState = 'complete';
        this.cookie = '';
        this.title = '';
        this.documentElement = new Element(this, 'html');
        this.appendChild(this.documentElement);
        this.head = new Element(this, 'head');
        this.body = new Element(this, 'body');
        this.documentElement.appendChild(this.head);
        this.documentElement.appendChild(this.body);
        this.activeElement = this.body;
        this.commands = [];
    }
    createElement(tag) { return new Element(this, tag); }
    createTextNode(text) { return new Text(this, String(text)); }
    createComment(text) { return new Comment(this, String(text)); }
    createDocumentFragment() { return new Fragment(this); }
    getElementById(id) {
        let found = null;
        walkElements(this, function (el) { if (!found && el.getAttribute('id') === id) found = el; });
        return found;
    }
    querySelectorAll(selector) { return this.documentElement.matches(selector)
        ? [this.documentElement].concat(this.documentElement.querySelectorAll(selector))
        : this.documentElement.querySelectorAll(selector); }
    querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
    getElementsByTagName(name) { return this.querySelectorAll(name); }
    // execCommand keeps the name of each command and answers true. A test
    // reads document.commands.
    execCommand(name) { this.commands.push(name); return true; }
    // createTreeWalker answers a walker over the text nodes below root,
    // in document order. whatToShow is NodeFilter.SHOW_TEXT in each caller.
    createTreeWalker(root, whatToShow, filter) {
        const nodes = [];
        (function collect(n) {
            for (const c of n.childNodes) {
                if (c.nodeType === 3) {
                    const verdict = filter && filter.acceptNode ? filter.acceptNode(c) : NodeFilter.FILTER_ACCEPT;
                    if (verdict === NodeFilter.FILTER_ACCEPT) nodes.push(c);
                } else {
                    collect(c);
                }
            }
        })(root);
        let at = -1;
        return {
            currentNode: root,
            nextNode() {
                at++;
                this.currentNode = nodes[at] || null;
                return this.currentNode;
            },
        };
    }
}

// ---------------------------------------------------------------------
// The data of a form
// ---------------------------------------------------------------------

// FormData reads the controls of a form the same way as a browser. A
// control with no name and a disabled control give nothing. A checkbox and
// a radio button give their value only when they are checked.
class FormData {
    constructor(form) {
        this._pairs = [];
        if (!form) return;
        for (const el of form.querySelectorAll('input, select, textarea')) {
            const name = el.getAttribute('name');
            if (!name || el.disabled) continue;
            const type = (el.getAttribute('type') || '').toLowerCase();
            if (type === 'file' || type === 'submit' || type === 'button') continue;
            if (type === 'checkbox' || type === 'radio') {
                if (el.checked) this._pairs.push([name, el.getAttribute('value') === null ? 'on' : el.value]);
                continue;
            }
            this._pairs.push([name, el.value]);
        }
    }
    append(name, value) { this._pairs.push([name, String(value)]); }
    delete(name) { this._pairs = this._pairs.filter(function (p) { return p[0] !== name; }); }
    has(name) { return this._pairs.some(function (p) { return p[0] === name; }); }
    get(name) {
        const pair = this._pairs.find(function (p) { return p[0] === name; });
        return pair ? pair[1] : null;
    }
    getAll(name) {
        return this._pairs.filter(function (p) { return p[0] === name; }).map(function (p) { return p[1]; });
    }
    set(name, value) { this.delete(name); this.append(name, value); }
    entries() { return this._pairs.map(function (p) { return p.slice(); })[Symbol.iterator](); }
    [Symbol.iterator]() { return this.entries(); }
    // toString gives the pairs in the form of a query, thus a test can
    // compare the whole body with one string.
    toString() { return new URLSearchParams(this._pairs).toString(); }
}

const NodeFilter = { SHOW_TEXT: 4, SHOW_ELEMENT: 1, FILTER_ACCEPT: 1, FILTER_REJECT: 2, FILTER_SKIP: 3 };

// newDocument answers a document whose body holds bodyHTML.
function newDocument(bodyHTML) {
    const doc = new Document();
    if (bodyHTML) doc.body.innerHTML = bodyHTML;
    return doc;
}

module.exports = { newDocument, Event, EventTarget, NodeFilter, FormData, Element, Text };
