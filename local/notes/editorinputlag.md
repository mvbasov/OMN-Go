# Investigation: internal editor input lag on Android

Status: **open, not reproduced on demand.** No code changed.

Purpose: this document holds the full state of the investigation of 2026-08-26.
Read it when the fault happens again. It tells you what to capture at that
moment, what the session already ruled out, and what to test next.

Repository state at the time of the investigation: commit `622b3c2`,
`APP_VERSION` 26.08.59.

---

## 1. Symptom

The internal editor sometimes shows a delay between a key press on the virtual
keyboard and the character on screen. The delay does not happen on every key
press. The maintainer reported a possible link to long text.

The maintainer could not reproduce the fault during the session.

---

## 2. Conditions at the last occurrence

| Item | Value |
| --- | --- |
| Device | OnePlus 11 5G |
| Operating system | Android 14 |
| Build | Android APK, internal editor (`/edit`) |
| Word wrap | Off |
| Line numbers | Off |
| Find bar | Closed |
| Document size | Not large |
| Text shape | One long paragraph |
| First bad version | Unknown |
| Browser comparison | Not tested |
| Keyboard and glide typing | Not recorded |

---

## 3. What the editor does for each key press

The internal editor is the standalone page. `handlers.go` serves it when
`UseInternalEd` is true. `renderEditorPage` in `templates.go` builds it from
`frontend/templates/editor.html`.

`editor.html` loads two files only: `/css/omn-go-core.css` and
`/js/omn-go-editor.js`. The page opens no SSE stream. The page sends no request
while the user types. The buffer goes to the server only on Save.

The `input` listener (`omn-go-editor.js:1426`) does three things:

1. `markDirty()`. This is a no-op after the first change.
2. `renderGutter()`.
3. `scheduleFind()`, but only when the find bar is open.

`onKeyDown` (`omn-go-editor.js:1308`) acts on `Tab` and on `Ctrl+S` only.

`renderGutter()` (`omn-go-editor.js:763`) returns at once when
`lineNumbersActive()` is false. That function is `lnOn && !wrapOn`
(`omn-go-editor.js:761`). The defaults are `wrapOn = true` and `lnOn = false`.

**Conclusion: in the reported configuration the page runs almost no JavaScript
for each key press.** The delay comes from a layer below the page.

---

## 4. Ruled out

* **Line-number gutter.** `renderGutter()` calls `ta.value.split('\n')` for each
  input event. That call copies the full buffer and allocates one string for
  each line. It is real cost, but it needs line numbers on and word wrap off.
  Line numbers were off. The code path did not run.
* **Find highlight mirror.** `renderMirror()` (`omn-go-editor.js:933`) empties
  the layer when the find bar is closed. The find bar was closed.
* **Network and server.** The editor page makes no request while the user types.
* **Server-sent events.** The editor page does not load `omn-go-sse.js`.
* **Key handler.** `onKeyDown` does no work for a normal character key.

---

## 5. Open hypotheses

Order is by fit to the reported symptom, not by confidence.

### 5.1 Spell check at word boundaries

`editor.html:267` sets `spellcheck="true"` on the textarea. Android runs the
platform spell checker. The checker works at word boundaries, not at each
character. This matches a delay that happens sometimes and not always.

Test: set `ta.spellcheck = false` and compare.

### 5.2 Display refresh rate ramp

The OnePlus 11 panel lowers its refresh rate when the screen content is static.
The first key press after a pause waits for the panel to return to a high rate.
This matches a delay that appears after the user stops to think.

Test: no code change. Lock the refresh rate to 120 Hz in the Android display
settings and compare. Also compare with the screen kept active.

### 5.3 Long line measurement with word wrap off

`editor.html:148` sets `.omn-editor #editor.nowrap { white-space: pre;
overflow-x: auto; }`. With word wrap off, one long paragraph becomes one line
box that is thousands of pixels wide. Each key press makes the engine measure
that line again. The engine also scrolls horizontally to keep the caret in
view.

This matches "long paragraph, small document". It does not match a fast device
well. A Snapdragon 8 Gen 2 should measure one line without a visible delay.

Test: turn word wrap on and type in the same paragraph.

### 5.4 Transparent textarea above the mirror layer

`editor.html:135-148` gives `#editor` `background: transparent`,
`position: relative`, and `z-index: 1`. `.omn-editor-mirror`
(`editor.html:154-167`) is an absolutely positioned layer across the whole
pane. A transparent editable element cannot get its own opaque composited
layer. Each key press can then repaint the whole pane instead of a small area.

The mirror arrived in commit `3f5d695`, `feat(editor): highlight find`,
version 26.07.54. If the fault is older than 26.07.54, this hypothesis is dead.

Test: set an opaque background on the textarea from JavaScript and compare.

### 5.5 Input method composition

Gboard and other keyboards compose text before they commit it. Glide typing
replaces a whole word instead of appending one character. The composition path
can add delay that the page cannot see.

Test: switch to a different keyboard. Turn glide typing off. Compare.

### 5.6 Full-screen window and soft input mode

`AndroidManifest.xml:101` sets the theme to
`@android:style/Theme.NoTitleBar.Fullscreen`. The activity declares no
`android:windowSoftInputMode` (`AndroidManifest.xml:104-108`). Android ignores
`adjustResize` in a full-screen window. The keyboard then pans the window or
uses extract mode, and this changes how text reaches the WebView.

`MainActivity.java:182-184` sets only `setJavaScriptEnabled(true)` and
`setDomStorageEnabled(true)`. The WebView gets no other tuning.

Confidence is low. The hypothesis stays open because the fault is Android only.

---

## 6. Capture list for the next occurrence

Do these steps while the fault is on screen. Do not close the editor first.

1. Write down the note name and the note size in bytes.
2. Write down the paragraph length in characters, near the caret.
3. Write down the state of word wrap and of line numbers.
4. Write down whether the find bar is open.
5. Write down the app version from the page footer.
6. Write down the keyboard name and whether glide typing is on.
7. Write down the battery level and whether the device feels warm.
8. Write down how long the screen was static before the first slow key press.
9. Record the screen if you can. A video shows the delay length.
10. Type in a second note with a short paragraph. Report if the delay follows.
11. Open the same note in Chrome on the same phone, at the LAN address. Report
    if the delay follows. This step separates the WebView from the page.

---

## 7. Measurement plan

Do not patch on a guess. Measure first.

### 7.1 Why no APK rebuild is needed

`server.go:200-201` serves `/js/` and `/css/` from the asset tree that the app
extracts to the storage directory. The user can edit those files in the browser
with `?edit=true`. `refreshEmbeddedAssets` restores the shipped copy at the next
version change and keeps the edited copy in `asset_backups/<previous>/`.

So an instrumented `omn-go-editor.js` can run on the device, and the next
version bump removes it. This is a safe sandbox.

The editor CSS is inline in `editor.html`. That file is a template and the app
never extracts it. Any style experiment must run from JavaScript.

### 7.2 The instrument

Record four timestamps for each key press with `performance.now()`. Keep a
rolling median, 95th percentile, and worst case. Show the numbers in the editor
status bar.

| Bucket | From | To | A large value means |
| --- | --- | --- | --- |
| 1 | `keydown` | `beforeinput` | Input pipeline, input method, or spell check |
| 2 | `beforeinput` | `input` | The same pipeline, after the edit is known |
| 3 | `input` | next `requestAnimationFrame` | Layout and paint on the main thread |

If all three buckets stay small and the eye still sees a delay, the cause is the
compositor or the display. Hypothesis 5.2 then becomes the main one.

Keep the worst case on screen. The fault is rare, so the instrument must hold
the peak until the user reads it.

### 7.3 Style experiments from JavaScript

Add two switches, each behind a `localStorage` flag, so the maintainer can
compare on the device:

1. `ta.spellcheck = false` — tests hypothesis 5.1.
2. An opaque background on the textarea — tests hypothesis 5.4.

---

## 8. Fixes worth making anyway

These are not the reported fault. They are real cost that the code carries.

1. `renderGutter()` calls `ta.value.split('\n')` for each input event when line
   numbers are on. Count the newline characters instead. Do not allocate one
   string for each line.
2. `renderGutter()` writes `gutterEl.textContent = ''` for each input event when
   line numbers are off. The element is already empty and hidden. Return before
   the write.
3. Coalesce the gutter update into one animation frame. Fast typing then costs
   one update for each frame, not one for each key press.

---

## 9. Questions still open

1. Which application version first showed the fault? Was 26.07.54 good?
2. Does the fault follow the note into Chrome on the same phone?
3. Which keyboard, and is glide typing on?
4. Does the fault depend on paragraph length, or on document length?
