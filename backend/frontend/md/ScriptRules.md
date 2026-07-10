Title: JS Scripting Rules
Date: 2026-06-15 12:00:00
Category: System
Tags: Document

# JavaScript Guidelines for OMN-Go

Because OMN-Go is rendered server-side, standard scripts running on layout structures should scope state securely.

### How embedded scripts execute

Plain `<script>` blocks embedded in a note run **immediately, while the page is still being parsed** — exactly like in classic OMN. All OMN-Go helpers (`openDatabase`, `omnGoOpenDatabase`, `executeScripts`, page variables like `PageName`, `Title`, `currentNote`, ...) are already available at that moment, and everything the script prints or throws (including syntax errors) is captured by the JS console button.

Because a plain script runs during parsing, page elements *below* the script (for example the `#status` footer) do not exist yet. If a script needs the complete page, either:

* run it on load: `window.onload = function() { ... };` (or `window.addEventListener('load', ...)`), or
* use `<script type="module">` — module scripts are always deferred by the browser and run after the whole page is parsed.

### Rule 1: Isolate variables using Block Scopes or IIFEs
Never leave `const` or `let` in the top-level global scope. Wrap the script in an Anonymous Block `{ ... }` or an Immediately Invoked Function Expression (IIFE).

```javascript
{
    const myLocalVar = "Safe!";
    let counter = 0;
}
```

### Rule 2: Explicitly attach required globals to `window`
If a function is needed for an HTML `onclick` event, attach it directly to the `window` object.

```javascript
window.doSomething = function() {
    alert("This works safely on reload!");
};
```
