Title: Incoming notes
Date: %%DATE%%
Category: Notes

<details class="omn-incoming server-only" id="omnIncoming">
<summary class="omn-incoming-head">Receive a note</summary>
<p class="omn-incoming-hint">Choose one or more <code>.md</code> files, or drop them here. Each note goes under <code>incoming/</code> and gets a line in the list below. A note never writes over a note that you made.</p>
<div class="omn-incoming-row">
<input type="file" id="omnIncomingFiles" accept=".md,.markdown,text/markdown,text/plain" multiple>
<button type="button" class="omn-incoming-btn" id="omnIncomingImport"><i class="material-icons icon-sm">upload_file</i>Import</button>
</div>
<div class="omn-incoming-status" id="omnIncomingStatus"></div>
</details>

<script>
// The receive box for the desktop application. Android uses the share sheet
// and never comes through here, so on Android the box is taken off the page
// (below) rather than shown as a control that has no reason to exist there.
//
// It is a <details>, closed on load: the reason to open this note is the
// list of what arrived, and the box is only needed on the day a note comes
// in by hand. The drag-and-drop target is the whole element, so a file can
// be dropped on the closed summary as well as on the open box.
//
// Block-scoped, as every note script must be (see ScriptRules): this file is
// a note like any other, and a "var" at its top level would become a global
// on this page.
{
    const box = document.getElementById('omnIncoming');
    const input = document.getElementById('omnIncomingFiles');
    const button = document.getElementById('omnIncomingImport');
    const statusEl = document.getElementById('omnIncomingStatus');

    if (box && typeof IS_ANDROID !== 'undefined' && IS_ANDROID) {
        box.remove();
    } else if (box && input && button && statusEl) {
        const say = function (msg, bad) {
            statusEl.textContent = msg || '';
            statusEl.classList.toggle('is-error', !!bad);
        };

        // One note per request. The rules live in the backend
        // (note_exchange.go), which is the same code the Android share path
        // reaches - so a note lands in the same place whichever way it came.
        const importOne = async function (file) {
            const form = new FormData();
            form.append('file', file, file.name);
            const res = await fetch('/api/import/note', { method: 'POST', body: form });
            let data = {};
            try { data = await res.json(); } catch (e) { /* not JSON */ }
            if (res.status === 401) {
                throw new Error('log in as admin to import a note');
            }
            if (!res.ok || data.status !== 'success') {
                throw new Error(data.message || ('HTTP ' + res.status));
            }
            return data;
        };

        const run = async function (files) {
            if (!files || !files.length) {
                box.open = true;
                say('Choose a file first.', true);
                return;
            }
            box.open = true;
            button.disabled = true;
            let done = 0;
            const failed = [];
            for (let i = 0; i < files.length; i++) {
                say('Importing ' + (i + 1) + ' of ' + files.length + '…');
                try {
                    await importOne(files[i]);
                    done++;
                } catch (e) {
                    failed.push(files[i].name + ': ' + e.message);
                }
            }
            button.disabled = false;

            if (!failed.length) {
                // The list below is written by the server, so the page has to
                // come again to show what just arrived.
                say('Imported ' + done + '. Refreshing…');
                window.location.reload();
                return;
            }
            say(done + ' imported, ' + failed.length + ' failed — ' + failed.join('; '), true);
        };

        button.addEventListener('click', function () { run(input.files); });

        // Dropping a file on the box is the other way a desktop does this.
        ['dragenter', 'dragover'].forEach(function (name) {
            box.addEventListener(name, function (e) {
                e.preventDefault();
                box.classList.add('is-over');
            });
        });
        ['dragleave', 'dragend'].forEach(function (name) {
            box.addEventListener(name, function () { box.classList.remove('is-over'); });
        });
        box.addEventListener('drop', function (e) {
            e.preventDefault();
            box.classList.remove('is-over');
            run(e.dataTransfer && e.dataTransfer.files);
        });
    }
}
</script>

<!-- omn-go-incoming-list -->
