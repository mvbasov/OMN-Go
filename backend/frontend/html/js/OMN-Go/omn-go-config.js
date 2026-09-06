// --- The Config page ---
//
// Every setting screen of OMN-Go is in this file: the navigation, the
// unsaved-changes mark, the reveal of each secret, and the save.
//
// WHY IT IS A FILE OF ITS OWN. The navigation and the dirty mark were in
// omn-go-core.js until 26.09.23, and the secrets and the save were in
// omn-go-sse.js. Each note carries a copy of the shell of index.html,
// thus each note carried a script that only ONE page runs. config_page.html
// loads this file, and no other page does.
//
// IT NEEDS NO file: GUARD, unlike omn-go-sse.js. The Config page is
// dynamic. serveConfigPage renders it for each request and writes no
// html/Config.html, thus no export and no git sync ever carries it.
// TestBaseline_ServeHTMLPageDispatch holds that rule. A page opened from
// disk therefore never loads this file.
//
// IT LOADS AFTER omn-go-core.js AND omn-go-sse.js. The script element is
// in the body of config_page.html, and those two are in the head of the
// shell. The globals of both are there before the first line below runs.

// --- Config page: menu navigation + unsaved-changes tracking ---
// The config form itself is untouched: each settings group is a
// show/hide .config-screen block inside the ONE <form>, so
// FormData(form) in saveConfig() (omn-go-sse.js) still collects every
// field no matter which screen is open. No-ops on pages without a
// #configForm.
document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById('configForm');
    if (!form) return;
    const panel = form.closest('.config-panel') || document;
    const menu = document.getElementById('configMenu');
    const screens = panel.querySelectorAll('.config-screen');

    // -- Navigation --
    // Driven by the URL hash rather than plain click handlers so that
    // Android's hardware Back button works: MainActivity.onBackPressed
    // forwards Back to webView.goBack() whenever there is history, and
    // each hash change is a history entry. Back therefore walks
    // sub-screen -> menu -> whatever page preceded Config, matching what
    // a native settings screen does. Desktop browser Back behaves the
    // same way for free.
    const HASH_PREFIX = 'cfg-';

    function currentScreen() {
        const h = (window.location.hash || '').replace(/^#/, '');
        return h.indexOf(HASH_PREFIX) === 0 ? h.slice(HASH_PREFIX.length) : '';
    }

    function applyHash() {
        const want = currentScreen();
        let matched = false;
        screens.forEach(s => {
            // The menu block has no data-screen attribute; it is the
            // fallback shown when the hash names no known sub-screen.
            const name = s.getAttribute('data-screen');
            const active = !!name && name === want;
            s.classList.toggle('active', active);
            if (active) matched = true;
        });
        if (menu) menu.classList.toggle('active', !matched);
        // A sub-screen replaces the menu at the top of the panel, so start
        // it at the top instead of inheriting the menu's scroll offset.
        window.scrollTo(0, 0);
    }

    panel.querySelectorAll('[data-goto]').forEach(btn => {
        btn.addEventListener('click', () => {
            window.location.hash = HASH_PREFIX + btn.getAttribute('data-goto');
        });
    });
    panel.querySelectorAll('[data-back]').forEach(btn => {
        // history.back() rather than clearing the hash, so returning to the
        // menu consumes the history entry instead of adding another one -
        // otherwise Back would bounce between menu and sub-screen.
        btn.addEventListener('click', () => window.history.back());
    });

    window.addEventListener('hashchange', applyHash);
    applyHash();

    // -- Unsaved-changes tracking --
    // Mirrors the dirty/clean dot in omn-go-editor.js. Any input/change
    // anywhere in the form marks dirty; beforeunload then covers every way
    // of leaving, since this is a real full-page load and not an SPA
    // (following a link, browser Back out of the page, closing the tab).
    // Moving between sub-screens only changes the hash, so it never
    // triggers the prompt. saveConfig() calls window.configMarkClean()
    // before its reload paths so a successful save doesn't prompt.
    const dots = panel.querySelectorAll('.config-dirty-dot');
    const labels = panel.querySelectorAll('.config-dirty-indicator .config-dirty-label');
    const menuBanner = document.getElementById('configMenuDirty');
    let dirty = false;

    function markDirty() {
        if (dirty) return;
        dirty = true;
        dots.forEach(d => d.classList.add('dirty'));
        labels.forEach(l => { l.textContent = 'Unsaved changes'; });
        if (menuBanner) menuBanner.hidden = false;
    }
    window.configMarkClean = function() {
        dirty = false;
        dots.forEach(d => d.classList.remove('dirty'));
        labels.forEach(l => { l.textContent = ''; });
        if (menuBanner) menuBanner.hidden = true;
    };

    form.addEventListener('input', markDirty);
    form.addEventListener('change', markDirty);

    window.addEventListener('beforeunload', function(e) {
        if (dirty) { e.preventDefault(); e.returnValue = ''; }
    });
});

// --- The secrets of the Config page ---
//
// The Config page renders each password box and each SSH key box
// EMPTY. The compiled HTML thus holds no secret, and a reader of the
// storage directory finds none there. See the banner of gitServerView
// in backend/templates.go.
//
// An empty box must therefore mean "keep the stored value", and not
// "clear the stored value". The two rules below give that meaning:
//
//   1. A box that carries data-secret gets data-dirty="1" when the
//      reader types in it.
//   2. saveConfig removes each data-secret box that is not dirty from
//      the FormData. handleConfig then sees no such field and keeps
//      the stored value. See configFieldSent.
//
// A reader who empties a box by hand makes it dirty, thus an empty
// value reaches the server and clears the stored value. That is the
// behavior that a person expects, and it is the behavior of each
// version before 26.09.7.
//
// revealSecrets fills each box from GET /api/config, which is admin
// only. It sets no dirty flag, thus a reveal alone changes nothing.

function secretFields(form) {
    return form.querySelectorAll('[data-secret]');
}

// secretValue finds one secret in the answer of GET /api/config.
// The name of the box is the key, and a git field carries its slot
// number, for example git_key_2.
function secretValue(cfg, name) {
    if (name === 'admin_password') return cfg.admin_password || '';
    if (name === 'guest_password') return cfg.guest_password || '';
    const git = /^git_(key|pass)_(\d+)$/.exec(name);
    if (git) {
        const slot = (cfg.git_servers || [])[Number(git[2])];
        if (!slot) return '';
        return (git[1] === 'key' ? slot.ssh_key_data : slot.password) || '';
    }
    return '';
}

window.omnGoRevealSecrets = async function (btn) {
    const form = document.getElementById('configForm');
    if (!form || !btn) return;
    const label = btn.querySelector('[data-reveal-label]');
    const icon = btn.querySelector('.material-icons');
    const boxes = secretFields(form);

    if (btn.dataset.shown === '1') {
        boxes.forEach(function (el) {
            if (el.dataset.dirty === '1') return;
            el.value = '';
            if (el.tagName === 'INPUT') el.type = 'password';
        });
        btn.dataset.shown = '';
        if (icon) icon.textContent = 'visibility';
        if (label) label.textContent = btn.dataset.showLabel || label.textContent;
        return;
    }

    let cfg;
    try {
        const res = await fetch('/api/config');
        if (res.status === 401 || res.status === 403) {
            alert('Log in as admin on a note page to read the passwords.');
            return;
        }
        if (!res.ok) { alert('Failed to read the configuration: ' + res.status); return; }
        cfg = await res.json();
    } catch (e) {
        alert('Network error: ' + e);
        return;
    }

    // Filling a box with JavaScript raises no input event, thus this
    // marks nothing dirty and saveConfig still sends nothing.
    boxes.forEach(function (el) {
        if (el.dataset.dirty === '1') return;
        el.value = secretValue(cfg, el.dataset.secret);
        if (el.tagName === 'INPUT') el.type = 'text';
    });
    btn.dataset.shown = '1';
    if (icon) icon.textContent = 'visibility_off';
    if (label) {
        btn.dataset.showLabel = label.textContent;
        label.textContent = 'Hide';
    }
};

document.addEventListener('DOMContentLoaded', function () {
    const form = document.getElementById('configForm');
    if (!form) return;
    secretFields(form).forEach(function (el) {
        el.addEventListener('input', function () { el.dataset.dirty = '1'; });
    });
});

window.saveConfig = async function() {
    const form = document.getElementById('configForm');
    if (!form) { alert('Config form not found'); return; }
    const fd = new FormData(form);
    // A secret box that the reader did not touch carries no meaning,
    // thus it must not reach the server at all. See the note above.
    secretFields(form).forEach(function (el) {
        if (el.dataset.dirty !== '1' && el.name) fd.delete(el.name);
    });
    try {
        const res = await fetch('/api/config', { method: 'POST', body: fd });
        if (res.ok) {
            // Config is now persisted server-side; clear the dirty flag
            // before either reload path below so the save doesn't
            // immediately re-trigger its own "leave site?" prompt.
            if (window.configMarkClean) window.configMarkClean();
            const body = await res.text();
            if (body === 'RestartRequired') {
                // ShareLAN changed: the listen socket is bound once at
                // startup, so the server must fully restart to rebind.
                alert('LAN sharing changed - the application will now restart to apply it.\n\nDesktop: this page reloads automatically in a few seconds.\nAndroid: the app will close; reopen it manually.');
                try { await fetch('/api/restart', { method: 'POST' }); } catch (e) { /* connection drops as the server exits - expected */ }
                // Desktop: the replacement process is up within ~1-3s
                // (bind retry included); reload to reconnect. On
                // Android the whole app process exits before this
                // timer matters.
                setTimeout(function(){ window.location.reload(); }, 3000);
                return;
            }
            alert('Configuration saved. Reloading...');
            window.location.reload();
        } else {
            let msg = await res.text();
            alert('Failed to save configuration: ' + msg);
        }
    } catch (e) {
        alert('Network error: ' + e);
    }
};
