Title: JS Scripting Rules
Date: 2026-06-15
Category: System

# JavaScript Guidelines for OMN-Go

Because OMN-Go is rendered server-side, standard scripts running on layout structures should scope state securely.

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
