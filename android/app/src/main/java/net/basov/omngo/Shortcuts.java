package net.basov.omngo;

// Shortcuts puts a note on the home screen. WebViewSetup calls
// createNoteShortcut for an "omngo://shortcut" address. MainActivity calls
// register and unregister, and it reads EXTRA_SHORTCUT_NOTE from the
// intent of a pressed shortcut.
//
// A member here is not private. See the banner of ShareIn for the reason.
final class Shortcuts {
    final MainActivity a;

    Shortcuts(MainActivity activity) {
        a = activity;
    }

    // Intent extra carrying the note name a pinned home-screen shortcut
    // should open (see createNoteShortcut() and the omngo://shortcut
    // interception in WebViewSetup). MainActivity reads it back in onCreate
    // and onNewIntent, and it sends the WebView straight to that note.
    static final String EXTRA_SHORTCUT_NOTE = "omngo_shortcut_note";

    // Own-package broadcast. createNoteShortcut() asks ShortcutManager to
    // fire it once the launcher finishes the pin of a shortcut. A
    // dismissal of the confirmation fires nothing. We can thus toast a
    // clear "done", and the detour to the home screen does not stay
    // unconfirmed. See the comment in createNoteShortcut() for why that
    // detour happens at all and cannot be skipped.
    private static final String ACTION_SHORTCUT_PINNED = "net.basov.omngo.SHORTCUT_PINNED";
    private static final String EXTRA_SHORTCUT_PINNED_LABEL = "label";
    private android.content.BroadcastReceiver shortcutPinnedReceiver;

    // register starts to listen for the broadcast of a pinned shortcut.
    void register() {
        shortcutPinnedReceiver = new android.content.BroadcastReceiver() {
            @Override
            public void onReceive(android.content.Context context, android.content.Intent intent) {
                String label = intent.getStringExtra(EXTRA_SHORTCUT_PINNED_LABEL);
                a.showToast("\"" + (label != null ? label : "Shortcut") + "\" added to your Home screen.");
            }
        };
        android.content.IntentFilter shortcutPinnedFilter = new android.content.IntentFilter(ACTION_SHORTCUT_PINNED);
        if (android.os.Build.VERSION.SDK_INT >= 33) {
            a.registerReceiver(shortcutPinnedReceiver, shortcutPinnedFilter, android.content.Context.RECEIVER_NOT_EXPORTED);
        } else {
            a.registerReceiver(shortcutPinnedReceiver, shortcutPinnedFilter);
        }
    }

    void unregister() {
        if (shortcutPinnedReceiver != null) {
            try {
                a.unregisterReceiver(shortcutPinnedReceiver);
            } catch (Exception e) {
                // Already unregistered, or never registered at all. Either
                // way there is nothing left to clean up.
            }
        }
    }

    // ----------------------------------------------------------------------
    // Home-screen shortcuts ("add to home screen" for the current note)
    // ----------------------------------------------------------------------
    //
    // Triggered by the .omn-android-only button in index.html (createNoteShortcut()
    // in omn-go-core.js), which navigates to omngo://shortcut?name=... -
    // intercepted in shouldOverrideUrlLoading of WebViewSetup, same pattern
    // as omngo://edit.
    //
    // Deliberately built on the plain platform SDK only. There is no
    // androidx.core and no appcompat, which matches the size-conscious
    // approach of this project. See the empty libs/ fileTree in
    // build.gradle. android.content.pm.ShortcutManager and
    // android.graphics.drawable.Icon cover API 26+. Both are first-party
    // android.jar classes, and neither needs an extra dependency. The
    // classic "com.android.launcher.action.INSTALL_SHORTCUT" broadcast
    // covers API 24-25, where ShortcutManager.requestPinShortcut does not
    // exist yet. This project used that same approach before OMN-Go, see
    // https://stackoverflow.com/a/16873257.
    //
    // The shortcut icon is composited the same way as the own launcher icon
    // of the app. See res/mipmap-anydpi-v26/ic_launcher.xml. The shared
    // ic_launcher_background is painted first, and
    // ic_launcher_shortcut_foreground is drawn on top of it. The shortcut
    // foreground alone left a pinned shortcut that looked like a plain
    // white card. It did not match the real icon of the app.
    void createNoteShortcut(final String name, final String title) {
        if (name == null || name.isEmpty()) return;
        final String label = (title != null && !title.isEmpty()) ? title : name;

        final android.graphics.Bitmap icon = renderDrawableToBitmap(
            R.drawable.ic_launcher_background, R.drawable.ic_launcher_shortcut_foreground);

        final android.content.Intent shortcutIntent = new android.content.Intent(a, MainActivity.class);
        shortcutIntent.setAction(android.content.Intent.ACTION_VIEW);
        shortcutIntent.addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK | android.content.Intent.FLAG_ACTIVITY_CLEAR_TOP);
        shortcutIntent.putExtra(EXTRA_SHORTCUT_NOTE, name);
        // Gives the shortcut intent of each note its own Uri data. The OS
        // then never takes the shortcuts of two different notes for the
        // same intent. The data itself is otherwise ignored.
        // EXTRA_SHORTCUT_NOTE above is what onCreate and onNewIntent read.
        shortcutIntent.setData(android.net.Uri.parse("omngo-shortcut://note/" + android.net.Uri.encode(name)));

        if (android.os.Build.VERSION.SDK_INT >= 26) {
            // Two ways to place a shortcut on API 26+, with a real
            // trade-off between them. See the per-method comments below.
            // No single answer is best for everyone, thus the code asks
            // each time and hardcodes neither. Below API 26,
            // ShortcutManager does not exist, and there is nothing to
            // choose. The legacy broadcast is the only option there, see
            // the else branch.
            new android.app.AlertDialog.Builder(a)
                .setTitle("Add \"" + label + "\" to Home screen")
                .setMessage("Reliable always works with the correct OMN-Go icon, but takes you to your "
                    + "Home screen to confirm.\n\n"
                    + "Quick tries to add it without leaving OMN-Go, but on Android 8+ it often shows a "
                    + "generic icon instead of OMN-Go's, and on some launchers it may silently do nothing at "
                    + "all - if the icon looks wrong or nothing appears on your Home screen, use Reliable "
                    + "instead.")
                .setPositiveButton("Reliable", (d, w) ->
                    pinShortcutViaShortcutManager(shortcutIntent, icon, label, name))
                .setNegativeButton("Quick", (d, w) ->
                    pinShortcutViaLegacyBroadcast(shortcutIntent, icon, label))
                .setNeutralButton("Cancel", null)
                .show();
        } else {
            // Pre-Oreo (API 24-25). ShortcutManager does not exist yet,
            // thus the legacy broadcast is the only option. Nothing to ask.
            pinShortcutViaLegacyBroadcast(shortcutIntent, icon, label);
        }
    }

    // "Reliable" is the official ShortcutManager API (API 26+). It always
    // works on a modern launcher. requestPinShortcut hands off to the own
    // "Add to Home screen?" confirmation UI of the LAUNCHER. That is a
    // separate app and process, and the OS deliberately puts it in front of
    // us, thus no app can silently plant a shortcut. Most launchers then
    // drop the user on the Home screen, to show where the new icon landed.
    //
    // That hand-off cannot be suppressed from here, because it is the
    // screen of the launcher and not ours. OMN-Go itself is only
    // backgrounded, which is paused or stopped. It is never finished and
    // never killed. A switch back through Recents or the app icon returns
    // to this exact page. The new shortcut is not that way back.
    //
    // The toast below makes that expected detour explicit, and the app then
    // does not look like it vanished. The pinned-callback toast, through
    // shortcutPinnedReceiver, confirms once the launcher finishes the add.
    void pinShortcutViaShortcutManager(android.content.Intent shortcutIntent,
            android.graphics.Bitmap icon, String label, String name) {
        android.content.pm.ShortcutManager shortcutManager =
            (android.content.pm.ShortcutManager) a.getSystemService(android.content.Context.SHORTCUT_SERVICE);
        if (shortcutManager == null || !shortcutManager.isRequestPinShortcutSupported()) {
            a.showToast("Your home screen doesn't support pinned shortcuts.");
            return;
        }

        android.graphics.drawable.Icon shortcutIcon = icon != null
            ? android.graphics.drawable.Icon.createWithAdaptiveBitmap(icon)
            : android.graphics.drawable.Icon.createWithResource(a, R.mipmap.ic_launcher);

        // Shortcut id is per-note (not random), so re-adding a shortcut for
        // the same note updates/re-pins the existing one instead of piling
        // up duplicates.
        String shortcutId = "note_" + name;
        android.content.pm.ShortcutInfo shortcut =
            new android.content.pm.ShortcutInfo.Builder(a, shortcutId)
                .setShortLabel(label)
                .setLongLabel("Open \"" + label + "\" in OMN-Go")
                .setIcon(shortcutIcon)
                .setIntent(shortcutIntent)
                .build();

        a.showToast("Confirm \"" + label + "\" on your Home screen - OMN-Go stays open in the background.");

        android.content.Intent callbackIntent = new android.content.Intent(ACTION_SHORTCUT_PINNED);
        callbackIntent.setPackage(a.getPackageName());
        callbackIntent.putExtra(EXTRA_SHORTCUT_PINNED_LABEL, label);
        android.app.PendingIntent callback = android.app.PendingIntent.getBroadcast(
            a, shortcutId.hashCode(), callbackIntent,
            android.app.PendingIntent.FLAG_UPDATE_CURRENT | android.app.PendingIntent.FLAG_IMMUTABLE);

        shortcutManager.requestPinShortcut(shortcut, callback.getIntentSender());
    }

    // "Quick" is the pre-Oreo launcher broadcast. It is still the only
    // option below API 26, see the else branch in createNoteShortcut().
    // There is no confirmation UI and no Home-screen jump. On API 26+ there
    // are two separate deprecation effects, and not one. Both are confirmed
    // on a real Android 14 device.
    //   - Many current launchers have stopped to listen for this broadcast
    //     at all, since the move to ShortcutManager. The stock Pixel
    //     launcher and recent Nova are among them. The broadcast can thus
    //     silently do nothing. There is no broadcast result to check, thus
    //     we cannot detect that and warn.
    //   - A launcher can still honor the broadcast enough to create a
    //     shortcut. Even there, EXTRA_SHORTCUT_ICON and
    //     EXTRA_SHORTCUT_ICON_RESOURCE are frequently ignored by the own
    //     compatibility handling of the OS for this frozen API. The
    //     shortcut then lands with a generic default icon, and not with the
    //     one built here. That is an OS-level restriction on the deprecated
    //     broadcast itself. A change to what we pass it cannot fix it, and
    //     a larger or differently formatted bitmap makes no difference.
    //     ShortcutManager, which is "Reliable", is the only path that
    //     carries a custom icon on modern Android.
    // The manifest permission declared beside it is a no-op on a launcher
    // that does not check it.
    void pinShortcutViaLegacyBroadcast(android.content.Intent shortcutIntent,
            android.graphics.Bitmap icon, String label) {
        android.content.Intent installIntent = new android.content.Intent();
        installIntent.putExtra(android.content.Intent.EXTRA_SHORTCUT_INTENT, shortcutIntent);
        installIntent.putExtra(android.content.Intent.EXTRA_SHORTCUT_NAME, label);
        if (icon != null) {
            installIntent.putExtra(android.content.Intent.EXTRA_SHORTCUT_ICON, icon);
        } else {
            installIntent.putExtra(android.content.Intent.EXTRA_SHORTCUT_ICON_RESOURCE,
                android.content.Intent.ShortcutIconResource.fromContext(a, R.mipmap.ic_launcher));
        }
        installIntent.setAction("com.android.launcher.action.INSTALL_SHORTCUT");
        a.sendBroadcast(installIntent);
        a.showToast("Shortcut requested - check your Home screen (icon and behavior vary by launcher).");
    }

    // Rasterizes one or more drawable resources, vector or otherwise, onto
    // a single square bitmap. The bitmap is sized for an adaptive icon, at
    // 108dp. That matches the declared width and height of both
    // ic_launcher_background.xml and ic_launcher_shortcut_foreground.xml.
    // The resources are painted in the given order, thus a later resId
    // layers on top of an earlier one. The OS itself layers the own
    // launcher icon of the app the same way, background and then
    // foreground.
    //
    // Returns null when any layer fails to resolve, and the caller then
    // falls back to the plain app icon. That is better than a pinned
    // shortcut with only some of its layers drawn. getDrawable(int) is a
    // plain Context method (API 21+), thus no compat library is needed to
    // resolve a vector drawable resource.
    android.graphics.Bitmap renderDrawableToBitmap(int... resIds) {
        try {
            int size = Math.round(108 * a.getResources().getDisplayMetrics().density);
            android.graphics.Bitmap bitmap = android.graphics.Bitmap.createBitmap(
                size, size, android.graphics.Bitmap.Config.ARGB_8888);
            android.graphics.Canvas canvas = new android.graphics.Canvas(bitmap);
            for (int resId : resIds) {
                android.graphics.drawable.Drawable drawable = a.getDrawable(resId);
                if (drawable == null) return null;
                drawable.setBounds(0, 0, canvas.getWidth(), canvas.getHeight());
                drawable.draw(canvas);
            }
            return bitmap;
        } catch (Exception e) {
            e.printStackTrace();
            return null;
        }
    }
}
