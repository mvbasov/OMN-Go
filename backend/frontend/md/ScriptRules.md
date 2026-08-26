Title: JS Scripting Rules
Date: 2026-06-15 12:00:00
Category: System
Author: Mikhail Basov
Tags: Document, OMN-Go, OMN-Go app

# JavaScript Guidelines for OMN-Go

The backend compiles a note to HTML. A note script then runs on every view of that page. Unscoped variables from one note script collide with the variables of another note. They also collide with the variables of OMN-Go itself. Keep the state of each note script inside its own scope.

### How embedded scripts execute

A plain note script runs **immediately, while the browser still parses the page**. Classic OMN does the same. All OMN-Go helpers are available at that moment. These helpers are `openDatabase`, `omnGoOpenDatabase`, `executeScripts` and page variables such as `PageName`, `Title` and `currentNote`. The JS console button captures everything the note script prints or throws, including syntax errors.

A plain note script runs during parsing. The page elements *below* the note script, for example the `#status` footer, do not exist yet. If a note script needs the complete page, use one of these two methods:

* Run the note script on load with `window.onload = function() { ... };` or with `window.addEventListener('load', ...)`.
* Use `<script type="module">`. The browser defers every module script and runs it after it parses the whole page.

### Rule 1: Isolate variables using Block Scopes or IIFEs
Do not leave a `const` or a `let` in the top-level global scope. Put the note script in an anonymous block `{ ... }` or in an immediately invoked function expression (IIFE).

```javascript
{
    const myLocalVar = "Safe!";
    let counter = 0;
}
```

### Rule 2: Explicitly attach required globals to `window`
If an HTML `onclick` event needs a function, attach the function to the `window` object.

```javascript
window.doSomething = function() {
    alert("This works safely on reload!");
};
```

### Rule 3: Write no empty line inside a `<script>` block

A note is Markdown, and Markdown ends an HTML block at the first empty line. An empty line does no damage to a `<script>` block at the top level of the note. An empty line breaks a `<script>` block **inside another HTML element**. The empty line ends that element. Markdown then reads the rest of your code as text. The page loses the code and shows a part of it on the screen.

This note script is inside a `<div>`, and the empty line breaks it:

```
<div>
<script>
let a = 1;

let b = 2;
</script>
</div>
```

The page shows `let b = 2;` as text, and the browser runs nothing.

Write the same code with no empty line, and the page is correct:

```
<div>
<script>
let a = 1;
let b = 2;
</script>
</div>
```

Use a comment line, `//`, where you want to separate two parts of the code. A comment line is not an empty line.

The same rule applies to a `<style>` block. It applies to each other HTML element that holds more than one line.

### See also

On Android, a raw HTML button can also fire an Android intent. The intent can open a Settings screen, start another application, or run a Termux command. See [Android Intents & Termux](AndroidIntents).

