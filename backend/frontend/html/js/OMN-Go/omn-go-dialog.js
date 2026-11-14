// omn-go-dialog.js - the dialogs of the application.
//
// These dialogs replace alert(), confirm() and prompt() of the browser. They
// use the same classes as the other modals of the application:
// .omn-overlay, .omn-modal and .omn-modal-buttons-row of omn-go-core.css.
//
// Each function answers a Promise:
//
//   omnAlert(text, title)                  No value. It ends when the reader closes the dialog.
//   omnConfirm(text, okLabel, cancelLabel) True for OK. False for Cancel.
//   omnPrompt(text, value, okLabel)        The text of the field. Null for Cancel.
//   omnDialog(opts)                        The button and the fields. See omnDialog below.
//
// omnCloseDialog() answers a boolean at once. omnCloseTopLayer of
// omn-go-core.js calls it for the Back key of Android and for Escape. True
// means that a dialog is open.
//
// A note page and the Config page load this file on demand. omnLazy in
// omn-go-api.js writes a stub for each of the four Promise names. The editor
// page has no omn-go-api.js, thus editor.html loads this file with a script
// element.
//
// Only one dialog shows at a time. A second call waits until the first
// dialog closes. Escape gives the value of the cancel button. Enter gives
// the value of the default button. Each text goes into the page with
// textContent, thus a text from a note or from the server adds no markup.
(function () {
    'use strict';

    // The calls that wait. Each item is {opts, resolve}.
    const queue = [];
    let busy = false;

    // The dialog on the screen: {cancel, closable, working}, or null.
    // omnCloseDialog reads it.
    let current = null;

    // omnDialog shows one dialog. opts holds:
    //
    //   kind      A name for the tests, for example "alert", "confirm" or "form".
    //   title     The heading. It is optional.
    //   text      The message. A line break in the text shows as a line break.
    //   content   A list of blocks below the text. Each block is one of:
    //               {p: text}           A paragraph.
    //               {warning: text}     A paragraph in the warning color.
    //               {table: [[k, v]]}   Two columns: a name and a value.
    //               {list: [text]}      A list, for example of file names.
    //   input     The start value of one text field. It is optional.
    //   fields    [{label, value, type, placeholder, rows, autocomplete}].
    //             type is "text" (the default), "password" or "textarea".
    //             It is optional, and it replaces "input".
    //   onInput   A function. Each change of a field calls it with the list
    //             of the field elements and the index of the changed field.
    //             It can change the value of another field.
    //   check     A function. A button that is not "cancel" calls it with the
    //             answer before the dialog closes. It answers an error text,
    //             or a Promise of one. An empty answer closes the dialog.
    //   closable  False for a dialog that only a button closes. Escape and
    //             the Back key then do nothing. The default is true.
    //   width     A CSS width of the box, for example "500px".
    //   buttons   [{label, value, cancel, default, danger, hint}].
    //             One button has "default": Enter presses it, and it gets
    //             the focus when the dialog has no field. One button can have
    //             "cancel": Escape presses it. Without it, Escape presses the
    //             default button. "danger" makes the button red. "hint" is a
    //             second, small line in the button.
    //
    // The answer is {value, input, inputs}. value is the value of the
    // pressed button. inputs is the list of the texts of the fields. input
    // is the first text, or null when the dialog has no field.
    function omnDialog(opts) {
        return new Promise(function (resolve) {
            queue.push({ opts: opts, resolve: resolve });
            if (!busy) showNext();
        });
    }

    // showNext shows the first waiting dialog. It runs at once for the
    // first call, thus the dialog is in the page when omnDialog returns.
    function showNext() {
        const item = queue.shift();
        if (!item) {
            busy = false;
            current = null;
            return;
        }
        busy = true;
        show(item.opts, function (answer) {
            item.resolve(answer);
            showNext();
        });
    }

    // makeElement makes one element with a class and a text.
    function makeElement(tag, className, text) {
        const el = document.createElement(tag);
        if (className) el.className = className;
        if (text !== undefined && text !== null) el.textContent = String(text);
        return el;
    }

    // makeBlock makes one block of opts.content. It answers null for a
    // block that it does not know, and the console names that block.
    function makeBlock(block) {
        if (block.p !== undefined) return makeElement('p', 'omn-dialog-p', block.p);
        if (block.warning !== undefined) return makeElement('p', 'omn-dialog-warning', block.warning);
        if (block.table) {
            const table = makeElement('table', 'omn-dialog-table');
            block.table.forEach(function (row) {
                const tr = makeElement('tr');
                tr.appendChild(makeElement('th', null, row[0]));
                tr.appendChild(makeElement('td', null, row[1]));
                table.appendChild(tr);
            });
            return table;
        }
        if (block.list) {
            const list = makeElement('ul', 'omn-dialog-list');
            block.list.forEach(function (item) { list.appendChild(makeElement('li', null, item)); });
            return list;
        }
        console.error('omnDialog: unknown content block', block);
        return null;
    }

    // makeField makes one field: an input or a textarea.
    function makeField(f) {
        const type = f.type || 'text';
        const el = document.createElement(type === 'textarea' ? 'textarea' : 'input');
        if (type === 'textarea') {
            el.rows = f.rows || 4;
        } else {
            el.type = type;
        }
        el.className = 'omn-dialog-input';
        if (f.placeholder) el.placeholder = f.placeholder;
        if (f.autocomplete) el.setAttribute('autocomplete', f.autocomplete);
        el.value = f.value === undefined || f.value === null ? '' : String(f.value);
        return el;
    }

    // makeButton makes one button. A button with a hint holds two lines,
    // and the label is in .omn-dialog-button-label.
    function makeButton(b) {
        const btn = document.createElement('button');
        btn.type = 'button';
        if (b.hint) {
            btn.appendChild(makeElement('span', 'omn-dialog-button-label', b.label));
            btn.appendChild(makeElement('small', 'omn-dialog-button-hint', b.hint));
        } else {
            btn.textContent = b.label;
        }
        if (b.cancel) {
            btn.classList.add('omn-btn-cancel');
            btn.setAttribute('data-omn-cancel', '');
        }
        if (b.danger) btn.classList.add('omn-btn-danger');
        return btn;
    }

    // show makes the dialog, puts it into the page, and calls done with the
    // answer when the reader presses a button or a key.
    function show(opts, done) {
        const before = document.activeElement;
        const overlay = document.createElement('div');
        overlay.className = 'omn-overlay omn-dialog';
        overlay.setAttribute('role', 'dialog');
        overlay.setAttribute('aria-modal', 'true');
        overlay.setAttribute('data-omn-dialog', opts.kind || 'choice');

        const box = makeElement('div', 'omn-modal');
        if (opts.width) box.style.width = opts.width;
        if (opts.title) box.appendChild(makeElement('h3', 'omn-dialog-title', opts.title));
        box.appendChild(makeElement('p', 'omn-dialog-text', opts.text || ''));
        (opts.content || []).forEach(function (block) {
            const el = block ? makeBlock(block) : null;
            if (el) box.appendChild(el);
        });

        let fields = opts.fields || [];
        if (!opts.fields && opts.input !== undefined && opts.input !== null) {
            fields = [{ value: opts.input }];
        }
        const inputs = fields.map(function (f, index) {
            if (f.label) box.appendChild(makeElement('label', 'omn-dialog-label', f.label));
            const input = makeField(f);
            if (typeof opts.onInput === 'function') {
                input.addEventListener('input', function () { opts.onInput(inputs, index); });
            }
            box.appendChild(input);
            return input;
        });

        // The error line of check. It is empty, and CSS hides it then.
        const error = makeElement('p', 'omn-dialog-error');
        error.setAttribute('role', 'alert');
        box.appendChild(error);

        const buttons = opts.buttons || [{ label: 'OK', value: true, default: true }];
        const defaultButton = buttons.find(function (b) { return b.default; }) || buttons[0];
        const cancelButton = buttons.find(function (b) { return b.cancel; }) || defaultButton;
        const closable = opts.closable !== false;

        let closed = false;
        const state = { closable: closable, working: false, cancel: null };
        current = state;

        function answerOf(value) {
            const texts = inputs.map(function (el) { return el.value; });
            return { value: value, input: texts.length ? texts[0] : null, inputs: texts };
        }
        function finish(value) {
            if (closed) return;
            closed = true;
            overlay.remove();
            if (before && typeof before.focus === 'function') before.focus();
            done(answerOf(value));
        }
        function setWorking(on) {
            state.working = on;
            Array.prototype.forEach.call(row.querySelectorAll('button'), function (btn) { btn.disabled = on; });
            if (on) overlay.setAttribute('aria-busy', 'true');
            else overlay.removeAttribute('aria-busy');
        }
        // press runs check for each button that is not "cancel". An error
        // text keeps the dialog open and shows the text in the error line.
        function press(b) {
            if (closed || state.working) return;
            if (b.cancel || typeof opts.check !== 'function') {
                finish(b.value);
                return;
            }
            error.textContent = '';
            setWorking(true);
            let result;
            try {
                result = opts.check(answerOf(b.value));
            } catch (e) {
                result = Promise.reject(e);
            }
            Promise.resolve(result).then(function (problem) {
                setWorking(false);
                if (problem) {
                    error.textContent = String(problem);
                    return;
                }
                finish(b.value);
            }, function (e) {
                setWorking(false);
                error.textContent = (e && e.message) ? e.message : String(e);
            });
        }
        state.cancel = function () { press(cancelButton); };

        const row = makeElement('div', 'omn-modal-buttons-row');
        // Buttons with a hint are wide, thus they stand one above the other.
        if (buttons.some(function (b) { return b.hint; })) row.classList.add('omn-dialog-buttons-column');
        let focusTarget = null;
        buttons.forEach(function (b) {
            const btn = makeButton(b);
            if (b === defaultButton) {
                btn.setAttribute('data-omn-default', '');
                focusTarget = btn;
            }
            btn.addEventListener('click', function () { press(b); });
            row.appendChild(btn);
        });
        box.appendChild(row);
        overlay.appendChild(box);

        // The dialog takes Escape, Enter and Tab, thus a key handler of the
        // page under it does not see them.
        overlay.addEventListener('keydown', function (e) {
            const target = e.target || {};
            if (e.key === 'Escape') {
                e.preventDefault();
                e.stopPropagation();
                if (closable) press(cancelButton);
            } else if (e.key === 'Enter') {
                // Enter on a button is a click of that button. Enter in a
                // textarea makes a new line, and Ctrl+Enter presses the
                // default button there.
                if (target.tagName === 'BUTTON') return;
                if (target.tagName === 'TEXTAREA' && !e.ctrlKey && !e.metaKey) {
                    e.stopPropagation();
                    return;
                }
                e.preventDefault();
                e.stopPropagation();
                press(defaultButton);
            } else if (e.key === 'Tab') {
                keepFocusInside(e);
            }
        });

        // keepFocusInside moves the focus from the last control to the first,
        // and back. Thus Tab never reaches the page under the dialog.
        function keepFocusInside(e) {
            // querySelectorAll answers a NodeList, which has no filter.
            const controls = Array.prototype.filter.call(box.querySelectorAll('input, textarea, button'), function (el) {
                return !el.disabled;
            });
            if (!controls.length) return;
            const first = controls[0];
            const last = controls[controls.length - 1];
            const active = document.activeElement;
            const inside = controls.indexOf(active) >= 0;
            if (e.shiftKey && (active === first || !inside)) {
                e.preventDefault();
                last.focus();
            } else if (!e.shiftKey && (active === last || !inside)) {
                e.preventDefault();
                first.focus();
            }
            e.stopPropagation();
        }

        document.body.appendChild(overlay);
        if (inputs.length) {
            inputs[0].focus();
            inputs[0].select();
        } else if (focusTarget) {
            focusTarget.focus();
        }
    }

    window.omnDialog = omnDialog;

    // omnCloseDialog cancels the open dialog and answers true. A dialog
    // that is not closable stays, and the answer is also true, thus the
    // page under it does not change. With no dialog it answers false.
    window.omnCloseDialog = function () {
        if (!current) return false;
        if (current.closable && !current.working) current.cancel();
        return true;
    };
    // The editor page has no omn-go-core.js, thus no omnCloseTopLayer. On
    // that page the dialog is the only layer. A page with omn-go-core.js
    // loads that file first, and its omnCloseTopLayer stays.
    if (typeof window.omnCloseTopLayer !== 'function') window.omnCloseTopLayer = window.omnCloseDialog;

    window.omnAlert = function (text, title) {
        return omnDialog({
            kind: 'alert',
            title: title,
            text: text,
            buttons: [{ label: 'OK', value: true, default: true }],
        }).then(function () {});
    };

    window.omnConfirm = function (text, okLabel, cancelLabel) {
        return omnDialog({
            kind: 'confirm',
            text: text,
            buttons: [
                { label: okLabel || 'OK', value: true, default: true },
                { label: cancelLabel || 'Cancel', value: false, cancel: true },
            ],
        }).then(function (answer) { return answer.value === true; });
    };

    window.omnPrompt = function (text, value, okLabel) {
        return omnDialog({
            kind: 'prompt',
            text: text,
            input: value === undefined || value === null ? '' : value,
            buttons: [
                { label: okLabel || 'OK', value: true, default: true },
                { label: 'Cancel', value: false, cancel: true },
            ],
        }).then(function (answer) { return answer.value === true ? answer.input : null; });
    };
})();
