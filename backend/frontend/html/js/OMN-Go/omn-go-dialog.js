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
// A note page and the Config page load this file on demand. omnLazy in
// omn-go-api.js writes a stub for each of the four names. The editor page
// has no omn-go-api.js, thus editor.html loads this file with a script
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

    // omnDialog shows one dialog. opts holds:
    //
    //   kind     A name for the tests: "alert", "confirm", "prompt", "choice" or "form".
    //   title    The heading. It is optional.
    //   text     The message. A line break in the text shows as a line break.
    //   input    The start value of one text field. It is optional.
    //   fields   [{label, value}]: two or more text fields, each with a label.
    //            It is optional, and it replaces "input".
    //   onInput  A function. Each change of a field calls it with the list
    //            of the field elements and the index of the changed field.
    //            It can change the value of another field.
    //   buttons  [{label, value, cancel, default}]. One button has "default":
    //            Enter presses it, and it gets the focus when the dialog has
    //            no field. One button can have "cancel": Escape presses it.
    //            Without it, Escape presses the default button.
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
            return;
        }
        busy = true;
        show(item.opts, function (answer) {
            item.resolve(answer);
            showNext();
        });
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
        // .omn-overlay has display: none, and a script shows it.
        overlay.style.display = 'flex';

        const box = document.createElement('div');
        box.className = 'omn-modal';
        if (opts.title) {
            const title = document.createElement('h3');
            title.className = 'omn-dialog-title';
            title.textContent = opts.title;
            box.appendChild(title);
        }
        const text = document.createElement('p');
        text.className = 'omn-dialog-text';
        text.textContent = opts.text || '';
        box.appendChild(text);

        let fields = opts.fields || [];
        if (!opts.fields && opts.input !== undefined && opts.input !== null) {
            fields = [{ value: opts.input }];
        }
        const inputs = fields.map(function (f, index) {
            if (f.label) {
                const label = document.createElement('label');
                label.className = 'omn-dialog-label';
                label.textContent = f.label;
                box.appendChild(label);
            }
            const input = document.createElement('input');
            input.type = 'text';
            input.className = 'omn-dialog-input';
            input.value = f.value === undefined || f.value === null ? '' : String(f.value);
            if (typeof opts.onInput === 'function') {
                input.addEventListener('input', function () { opts.onInput(inputs, index); });
            }
            box.appendChild(input);
            return input;
        });

        const buttons = opts.buttons || [{ label: 'OK', value: true, default: true }];
        const defaultButton = buttons.find(function (b) { return b.default; }) || buttons[0];
        const cancelButton = buttons.find(function (b) { return b.cancel; }) || defaultButton;

        let closed = false;
        function finish(value) {
            if (closed) return;
            closed = true;
            overlay.remove();
            if (before && typeof before.focus === 'function') before.focus();
            const texts = inputs.map(function (el) { return el.value; });
            done({ value: value, input: texts.length ? texts[0] : null, inputs: texts });
        }

        const row = document.createElement('div');
        row.className = 'omn-modal-buttons-row';
        let focusTarget = null;
        buttons.forEach(function (b) {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.textContent = b.label;
            if (b.cancel) {
                btn.className = 'omn-btn-cancel';
                btn.setAttribute('data-omn-cancel', '');
            }
            if (b === defaultButton) {
                btn.setAttribute('data-omn-default', '');
                focusTarget = btn;
            }
            btn.addEventListener('click', function () { finish(b.value); });
            row.appendChild(btn);
        });
        box.appendChild(row);
        overlay.appendChild(box);

        // The dialog takes Escape and Enter, thus a key handler of the page
        // under it does not see them. Enter on a button is a click of that
        // button, thus the handler leaves it to the browser.
        overlay.addEventListener('keydown', function (e) {
            if (e.key === 'Escape') {
                e.preventDefault();
                e.stopPropagation();
                finish(cancelButton.value);
            } else if (e.key === 'Enter' && !(e.target && e.target.tagName === 'BUTTON')) {
                e.preventDefault();
                e.stopPropagation();
                finish(defaultButton.value);
            }
        });

        document.body.appendChild(overlay);
        if (inputs.length) {
            inputs[0].focus();
            inputs[0].select();
        } else if (focusTarget) {
            focusTarget.focus();
        }
    }

    window.omnDialog = omnDialog;

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
