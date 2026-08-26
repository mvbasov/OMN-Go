Title: Android Intents & Termux
Date: 2026-07-17 12:00:00
Category: System
Author: Mikhail Basov
Tags: Android, Intent, Termux, OMN-Go, OMN-Go app

# Android Intents & Termux

On Android, a link or a button in a note can fire an Android **intent**. An intent can open a system Settings screen or start another application. An intent can also run a command in **Termux**, or scan a barcode into the Quick Notes page. Only the Android application supports this feature. The same links do nothing in the desktop application or in a LAN browser.

Everything on this page is **off by default**. You enable it on the [Config](Config) page. You do not write every note yourself. A note can arrive by [git synchronization](UserManual#git-synchronization), or another device can edit the note over [LAN sharing](UserManual#sharing-on-the-lan). See [Security](#security) below.

## Turning it on

On the [Config](Config) page, under **Android Integration**:

- **Enable intent: links** (`enable_intent_uri`) — the master switch. With this
  switch off, no intent link does anything.
- **Enable Termux commands** (`enable_termux_intent`) — this switch also
  permits the Termux path. The master switch must be on as well.

Both switches stay off until you enable them. A change applies at the next press. You do not restart the application.

## A first example: Wi-Fi settings

An intent link is a Markdown link with an `intent:` URI as its address. This link opens the Wi-Fi settings of the device:

```
[Open Wi-Fi settings](intent:#Intent;action=android.settings.WIFI_SETTINGS;end)
```

Try it. This link needs *Enable intent: links* on: [Open Wi-Fi settings](intent:#Intent;action=android.settings.WIFI_SETTINGS;end)

A note renders raw HTML, so you can also write the link as a button:

```
<a href="intent:#Intent;action=android.settings.WIFI_SETTINGS;end"><button>
  <i class="material-icons">wifi</i> Wi-Fi settings
</button></a>
```

<a href="intent:#Intent;action=android.settings.WIFI_SETTINGS;end"><button>
  <i class="material-icons">wifi</i> Wi-Fi settings
</button></a>

The `action=` part names the task. `android.settings.WIFI_SETTINGS` opens the Wi-Fi screen directly. The wider `android.settings.WIRELESS_SETTINGS` opens the full network and wireless screen.

## More Settings screens

Replace the `action=` value with one of these values:

```
[Bluetooth](intent:#Intent;action=android.settings.BLUETOOTH_SETTINGS;end)
[NFC](intent:#Intent;action=android.settings.NFC_SETTINGS;end)
[Location](intent:#Intent;action=android.settings.LOCATION_SOURCE_SETTINGS;end)
[Device info](intent:#Intent;action=android.settings.DEVICE_INFO_SETTINGS;end)
```

Renders to:

[Bluetooth](intent:#Intent;action=android.settings.BLUETOOTH_SETTINGS;end)
[NFC](intent:#Intent;action=android.settings.NFC_SETTINGS;end)
[Location](intent:#Intent;action=android.settings.LOCATION_SOURCE_SETTINGS;end)
[Device info](intent:#Intent;action=android.settings.DEVICE_INFO_SETTINGS;end)

## Launching apps

An `intent:` URI can also address another application. If no installed application can handle the intent, Android opens the optional `S.browser_fallback_url` extra instead. Percent-encode the value of this extra, so that it does not break the URI:

```
[Open a page](intent://example.com#Intent;scheme=https;S.browser_fallback_url=https%3A%2F%2Fexample.com;end)
```

Renders to:

[Open a page](intent://example.com#Intent;scheme=https;S.browser_fallback_url=https%3A%2F%2Fexample.com;end)

## Opening a file on the device

A note can open a photo, a PDF or any other file that is already on the device. The address needs three parts:

- `intent:///` and then the absolute path. **Three slashes.** Two of them
  belong to the URI, and the third is the start of the path. With two slashes,
  `storage` becomes the host name of the address and Android looks for a file
  that is not there.
- `scheme=file`, which makes the address a `file:` address.
- `type=`, the type of the file. Without it Android does not know which
  applications can open the file, and the link finds none of them.

A photo:

```
[Open the photo](intent:///storage/emulated/0/DCIM/Camera/001.jpg#Intent;scheme=file;action=android.intent.action.VIEW;type=image/jpeg;end)
```

A PDF:

```
[Open the PDF](intent:///storage/emulated/0/DCIM/Documents/001.pdf#Intent;scheme=file;action=android.intent.action.VIEW;type=application/pdf;end)
```

Both links open the application that you use for that type of file. If you set no default application, Android shows the list of the applications that can do it.

### Such a link stays on one device

The address is a path on this device. The file at the end of it is **outside** OMN-Go, and OMN-Go carries only what is inside its own storage directory.

Git synchronization thus moves the note and not the file. On your second device the same link opens nothing, because the path is possibly not there. It is worse when the path IS there and holds a different file. The link then opens the wrong file and says nothing about it. A note that you send to a different person has the same fault.

Two results come from this:

- Keep a link like this in a note that stays on this device. A note in
  `md/local/`, or a note with a name that starts with `local-`, does not go to
  git. See [Files that stay on this device](UserManual#files-that-stay-on-this-device).
- Put the file **into** your notes when the file must travel with them. A
  `.txt` file beside a note goes to each device. See
  [Text files beside your notes](UserManual#text-files-beside-your-notes). Not
  each directory of the storage travels. `html/images/` does not, for example.

The desktop application and a LAN browser do nothing with an intent link at all. This is by design: only the Android application has a share sheet and other applications to open.

### One named application

Add `package=` to send the file to one application and to no other. This example uses [Aves Libre](https://f-droid.org/packages/deckers.thibault.aves.libre/), an open-source gallery:

```
[Open in Aves Libre](intent:///storage/emulated/0/DCIM/Camera/001.jpg#Intent;scheme=file;action=android.intent.action.VIEW;type=image/jpeg;package=deckers.thibault.aves.libre;end)
```

A link with `package=` shows *No app can handle this link* when that application is not installed, or when it is installed but does not accept a `file:` address of that type. A link without `package=` is thus the better choice for a note that you send to a different person.

### To find the name of an application

Write the link **without** `package=` and follow it one time. Android shows the list of every application that can open that file. This list is the answer: an application in it can do the work, and an application that is not in it cannot.

Then add `package=` for the one that you want. To read the name of an installed application, open *Settings*, then *Apps*, then that application. Some devices show the name at the bottom of the page of the application. A file manager also shows the name in the properties of the APK file.

### A file manager as the opener

Some applications accept only a `content:` address, which a note cannot write. A file manager is the way through in that case: send the file to the file manager, and open it from there.

These three are open-source and are in F-Droid:

```
com.ghostsq.commander       Ghost Commander
me.zhanghai.android.files   Material Files
com.amaze.filemanager       Amaze File Manager
```

```
[Open with Ghost Commander](intent:///storage/emulated/0/DCIM/Documents/001.pdf#Intent;scheme=file;action=android.intent.action.VIEW;type=application/pdf;package=com.ghostsq.commander;end)
```

Try the link without `package=` first. A file manager is only necessary when no viewer of yours takes the file directly.

### Types for other files

```
image/png    image/jpeg    image/webp
application/pdf
text/plain
audio/mpeg   video/mp4
*/*          the list of every application that opens a file
```

### What can stop this

- **The path must be exact.** `/storage/emulated/0/` is the internal storage of
  the device. A memory card has a different path. Use a file manager to read
  the path of the file.
- **The other application needs its own permission to read storage.** A gallery
  and a file manager have it. An application that does not have it opens an
  empty screen or an error.
- **A space in a path** must be written as `%20`.

## Scanning a barcode into Quick Notes

A link can start a scanner and wait for the result. OMN-Go then puts the result into a review dialog. You can edit the result before you save it to the [Quick Notes](QuickNotes) page. This example uses [Binary Eye](https://f-droid.org/en/packages/de.markusfisch.android.binaryeye/), an open-source scanner:

```
[Scan a code](intent:#Intent;action=com.google.zxing.client.android.SCAN;package=de.markusfisch.android.binaryeye;S.omngo_capture_extra=SCAN_RESULT;end)
```

`S.omngo_capture_extra=SCAN_RESULT` is the marker of OMN-Go. It tells OMN-Go to start the intent for a result, and then to paste back the extra with the name `SCAN_RESULT`. After the scan, the Quick Note dialog opens with the decoded text. You review the text and save it. If you cancel the scan, nothing happens. This example needs only *Enable intent: links* and does not use Termux.

## Termux integration

With Termux installed and **Enable Termux commands** on, a note can run a shell command. This runs code on your device. The Termux path therefore always shows a **confirmation dialog** before the command runs.

### Prerequisites

1. Install [Termux](https://f-droid.org/en/packages/com.termux/) (the F-Droid
   build).
2. Permit other applications to send commands to Termux. In Termux, add
   `allow-external-apps=true` to `~/.termux/termux.properties`. Then run
   `termux-reload-settings`.
3. At the first command, OMN-Go asks for the Termux **RUN_COMMAND** permission.
   Grant the permission, then press the link again.

### The command URI

A minimal command that runs `uname -a`:

```
[Kernel info](intent:#Intent;action=com.termux.RUN_COMMAND;component=com.termux/.app.RunCommandService;S.com.termux.RUN_COMMAND_LABEL=Kernel%20info;S.com.termux.RUN_COMMAND_PATH=$PREFIX/bin/uname?-a;end)
```

The parts:

- `action=com.termux.RUN_COMMAND` and
  `component=com.termux/.app.RunCommandService` must be exactly as shown. These
  two parts address the command service of Termux.
- `S.com.termux.RUN_COMMAND_LABEL` — a short name that the confirmation dialog
  shows. This part is optional. Write a space as `%20`.
- `S.com.termux.RUN_COMMAND_PATH` — the program to run, and its packed
  arguments. See the next section.

### Passing arguments (the `?` and `&` convention)

An intent URI cannot carry a list. Therefore you **pack the arguments into the path**. Put a `?` after the program. Separate the arguments with `&`. Write a space *inside* one argument as `%20`:

```
S.com.termux.RUN_COMMAND_PATH=$PREFIX/bin/bash?-c&echo%20hello%20world
```

This runs `bash -c "echo hello world"` with two arguments, `-c` and `echo hello world`. There are two rules:

- Only the **first** `?` splits the program from the arguments. An argument can
  therefore contain a `?`.
- **Every** `&` separates two arguments. An argument therefore cannot contain a
  literal `&`.

### Foreground or background

Add the Termux switch to select how the command runs:

```
B.com.termux.RUN_COMMAND_BACKGROUND=true
```

- **Background** (`true`) runs the command without a terminal. It captures
  `stdout` and `stderr` separately.
- **Foreground** (`false`) opens a visible Termux terminal session. Termux
  returns the output as one combined transcript.

If you ask OMN-Go to capture output (see below) and you do not set this switch, OMN-Go uses **background**. The capture then works correctly. Set the switch to `false` if you want the terminal to open.

### Capturing command output

Add the `S.omngo_capture_output` marker of OMN-Go to paste the output of a command into the Quick Note review dialog. The value of the marker selects the stream:

- `stdout` (the default) — standard output
- `stderr` — error output (separate in background mode only)
- `both` — both streams, combined

```
[Kernel info](intent:#Intent;action=com.termux.RUN_COMMAND;component=com.termux/.app.RunCommandService;S.com.termux.RUN_COMMAND_LABEL=Kernel%20info;S.com.termux.RUN_COMMAND_PATH=$PREFIX/bin/uname?-a;S.omngo_capture_output=stdout;end)
```

Press the link and confirm. Termux then runs `uname -a` in the background. The output fills the Quick Note dialog. You review the output and save it. If the command fails, OMN-Go adds an `exit code: N` line. A successful (zero) exit adds nothing.

Try it. This example needs Termux and both switches on:

<a href="intent:#Intent;action=com.termux.RUN_COMMAND;component=com.termux/.app.RunCommandService;S.com.termux.RUN_COMMAND_LABEL=Kernel%20info;S.com.termux.RUN_COMMAND_PATH=$PREFIX/bin/uname?-a;S.omngo_capture_output=stdout;end"><button>
  <i class="material-icons">memory</i> Capture uname -a
</button></a>

## Security

Both switches are **off by default**. The Termux path needs three more things. Termux must be installed. You must grant the Termux permission. You must confirm each command. The two switches, the permission and the confirmation make four independent consents before a note can run anything.

A link that opens a file gives one path to one other application. It gives no new permission to OMN-Go and none to the other application: an application that may not read a file still may not read it. What the link does is let Android pass the path instead of stopping OMN-Go for passing it.

A note is not always your own note. A note can arrive by git synchronization. Another device can edit a note over LAN sharing. OMN-Go never saves a captured result silently. A captured result always goes into a dialog that you review first. Leave Termux off unless you write all of your notes yourself.

## Troubleshooting

- **A link does nothing.** Make sure that *Enable intent: links* is on. In the
  desktop application and in a LAN browser these links do nothing by design.
- **A file does not open.** Make sure that the path is exact and that the link
  has three slashes after `intent:` and a `type=`. If the application opens and
  shows nothing, that application has no permission to read your storage.
- **A Termux command does nothing.** Make sure that *Enable Termux commands* is
  on and that Termux is installed. Make sure that you granted the RUN_COMMAND
  permission. Make sure that you set `allow-external-apps=true` in Termux, and
  that you then ran `termux-reload-settings`.
- **No output appeared.** The command possibly gave no output. You were
  possibly on the editor page when the command finished. In that case Android
  shows a plain dialog with the text, and not the Quick Note panel of the
  application.

## See also

- [User Manual](UserManual) — everything else about how to write notes.
- [Scripting Rules](ScriptRules) — the rules for raw HTML and JavaScript in a
  note. An intent button uses these rules.
