Title: Android Intents & Termux
Date: 2026-07-17 12:00:00
Category: System
Tags: Android, Intent, Termux

# Android Intents & Termux

On Android, a link or button in a note can fire an Android **intent** — open a
system Settings screen, launch another app, run a command in **Termux**, or scan
a barcode straight into your Quick Notes. This is an Android-only feature: the
same links do nothing in the desktop app or in a LAN browser.

Everything here is **off by default** and gated on the [Config](Config) page,
because a note is not always something you wrote yourself — it can arrive by
[Git synchronization](UserManual#git-synchronization) or be edited by another
device over [LAN sharing](UserManual#sharing-on-the-lan). See
[Security](#security) below.

## Turning it on

On the [Config](Config) page, under **Android Integration**:

- **Enable intent: links** (`enable_intent_uri`) — the master switch. With it
  off, no intent link does anything.
- **Enable Termux commands** (`enable_termux_intent`) — additionally allows the
  Termux path. Requires the master switch to be on as well.

Both stay off until you turn them on, and a change applies on the next tap — no
restart needed.

## A first example: Wi-Fi settings

An intent link is just a Markdown link whose address is an `intent:` URI. This
one opens the phone's Wi-Fi settings:

```
[Open Wi-Fi settings](intent:#Intent;action=android.settings.WIFI_SETTINGS;end)
```

Try it (needs *Enable intent: links* on):
[Open Wi-Fi settings](intent:#Intent;action=android.settings.WIFI_SETTINGS;end)

Because notes render raw HTML, you can dress it up as a button too:

```
<a href="intent:#Intent;action=android.settings.WIFI_SETTINGS;end"><button>
  <i class="material-icons">wifi</i> Wi-Fi settings
</button></a>
```

<a href="intent:#Intent;action=android.settings.WIFI_SETTINGS;end"><button>
  <i class="material-icons">wifi</i> Wi-Fi settings
</button></a>

The `action=` part names what to do. `android.settings.WIFI_SETTINGS` opens
Wi-Fi directly; the broader `android.settings.WIRELESS_SETTINGS` opens the whole
network/wireless screen.

## More Settings screens

Swap the `action=` for any of these:

```
[Bluetooth](intent:#Intent;action=android.settings.BLUETOOTH_SETTINGS;end)
[NFC](intent:#Intent;action=android.settings.NFC_SETTINGS;end)
[Location](intent:#Intent;action=android.settings.LOCATION_SOURCE_SETTINGS;end)
[Device info](intent:#Intent;action=android.settings.DEVICE_INFO_SETTINGS;end)
```

## Launching apps

An `intent:` URI can also target another app. If nothing installed can handle
it, an optional `S.browser_fallback_url` extra is opened instead (its value is
percent-encoded so it doesn't break the URI):

```
[Open a page](intent://example.com#Intent;scheme=https;S.browser_fallback_url=https%3A%2F%2Fexample.com;end)
```

## Scanning a barcode into Quick Notes

A link can launch a scanner, wait for the result, and drop it into a review
dialog you can edit before saving to [Quick Notes](QuickNotes). This example uses
[Binary Eye](https://f-droid.org/en/packages/de.markusfisch.android.binaryeye/),
an open-source scanner:

```
[Scan a code](intent:#Intent;action=com.google.zxing.client.android.SCAN;package=de.markusfisch.android.binaryeye;S.omngo_capture_extra=SCAN_RESULT;end)
```

`S.omngo_capture_extra=SCAN_RESULT` is OMN-Go's own marker: it means "launch
this for a result, then paste back the extra named `SCAN_RESULT`." When the scan
finishes, the Quick Note dialog opens pre-filled with the decoded text for you to
review and save. Cancel the scan and nothing happens. Only *Enable intent:
links* is needed for this — Termux is not involved.

## Termux integration

With Termux installed and **Enable Termux commands** on, a note can run a shell
command. Because that runs code on your device, this path always shows a
**confirmation dialog** before anything runs.

### Prerequisites

1. Install [Termux](https://f-droid.org/en/packages/com.termux/) (the F-Droid
   build).
2. Let other apps send it commands: in Termux, add `allow-external-apps=true` to
   `~/.termux/termux.properties`, then run `termux-reload-settings`.
3. The first time you run a command, OMN-Go asks for Termux's **RUN_COMMAND**
   permission — grant it, then tap the link again.

### The command URI

A minimal command that runs `uname -a`:

```
[Kernel info](intent:#Intent;action=com.termux.RUN_COMMAND;component=com.termux/.app.RunCommandService;S.com.termux.RUN_COMMAND_LABEL=Kernel%20info;S.com.termux.RUN_COMMAND_PATH=$PREFIX/bin/uname?-a;end)
```

The parts:

- `action=com.termux.RUN_COMMAND` and
  `component=com.termux/.app.RunCommandService` must be exactly as shown — they
  address Termux's command service.
- `S.com.termux.RUN_COMMAND_LABEL` — a short name shown in the confirmation
  dialog (optional; spaces written as `%20`).
- `S.com.termux.RUN_COMMAND_PATH` — the program to run, plus its packed
  arguments (next).

### Passing arguments (the `?` and `&` convention)

An intent URI can't carry a list, so arguments are **packed into the path**: put
a `?` after the program, then separate arguments with `&`. A space *inside* one
argument is written as `%20`:

```
S.com.termux.RUN_COMMAND_PATH=$PREFIX/bin/bash?-c&echo%20hello%20world
```

That runs `bash -c "echo hello world"` — two arguments, `-c` and
`echo hello world`. Two rules to remember:

- The path/arguments split happens on the **first** `?` only, so an argument may
  itself contain `?`.
- Arguments are separated on **every** `&`, so an argument cannot contain a
  literal `&`.

### Foreground or background

Add Termux's own switch to choose how the command runs:

```
B.com.termux.RUN_COMMAND_BACKGROUND=true
```

- **Background** (`true`) runs silently and captures separate `stdout` and
  `stderr`.
- **Foreground** (`false`) opens a visible Termux terminal session; its output
  comes back as one combined transcript.

When you ask OMN-Go to capture output (below) and don't set this, it defaults to
**background** so capture works cleanly. Set it to `false` yourself if you want
the terminal to open.

### Capturing command output

Add OMN-Go's `S.omngo_capture_output` marker to paste a command's output into the
Quick Note review dialog. Its value chooses the stream:

- `stdout` (the default) — standard output
- `stderr` — error output (only separated in background mode)
- `both` — both streams, combined

```
[Kernel info](intent:#Intent;action=com.termux.RUN_COMMAND;component=com.termux/.app.RunCommandService;S.com.termux.RUN_COMMAND_LABEL=Kernel%20info;S.com.termux.RUN_COMMAND_PATH=$PREFIX/bin/uname?-a;S.omngo_capture_output=stdout;end)
```

Tap it, confirm, and Termux runs `uname -a` in the background; its output
pre-fills the Quick Note dialog for you to review and save. If the command
fails, an `exit code: N` line is added — a successful (zero) exit adds nothing.

Try it (needs Termux plus both switches on):

<a href="intent:#Intent;action=com.termux.RUN_COMMAND;component=com.termux/.app.RunCommandService;S.com.termux.RUN_COMMAND_LABEL=Kernel%20info;S.com.termux.RUN_COMMAND_PATH=$PREFIX/bin/uname?-a;S.omngo_capture_output=stdout;end"><button>
  <i class="material-icons">memory</i> Capture uname -a
</button></a>

## Security

Both switches are **off by default**, and the Termux path additionally needs
Termux installed, its permission granted, and a per-command confirmation — four
independent consents before a note can run anything. This matters because a note
isn't always yours: it can arrive by Git sync or be edited by another device over
the LAN. A captured result is never saved silently either — it always lands in a
dialog you review first. Leave Termux off unless you author your own notes.

## Troubleshooting

- **A link does nothing.** Check *Enable intent: links* is on. On desktop and in
  a LAN browser these links are inert by design.
- **A Termux command does nothing.** Check *Enable Termux commands* is on, Termux
  is installed, you granted the RUN_COMMAND permission, and
  `allow-external-apps=true` is set in Termux (followed by
  `termux-reload-settings`).
- **No output was pasted.** The command may have produced none, or you were
  mid-edit on the editor page when it finished — in that case a plain Android
  dialog appears with the text instead of the in-app Quick Note panel.

## See also

- [User Manual](UserManual) — everything else about authoring notes.
- [Scripting Rules](ScriptRules) — the rules for raw HTML and JavaScript in
  notes, which intent buttons rely on.
