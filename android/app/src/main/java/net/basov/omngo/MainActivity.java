package net.basov.omngo;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.os.Handler;
import android.os.Looper;

public class MainActivity extends Activity {
    private WebView webView;
    private String currentEditingName;

    // Storage dir and server port both used to be hardcoded here
    // ("net.basov.omngo" and "8080"), which broke on the fdroid flavor
    // (different applicationId -> different external media directory,
    // see build.gradle's productFlavors) and on any install where the
    // Config page's Server Port was changed away from the default. Both
    // are now resolved live instead: storageDir() defers to
    // ServerService.storageDir(), the same helper Backend.startServer()
    // itself is started with (see ServerService.onStartCommand), and
    // serverBase() reads the actual configured port via
    // ServerService.serverPort() rather than assuming 8080.

    private String storageDir() {
        return ServerService.storageDir(this);
    }

    private String serverBase() {
        return "http://127.0.0.1:" + ServerService.serverPort(this);
    }

    // Intent extra carrying the note name a pinned home-screen shortcut
    // should open (see createNoteShortcut() and the omngo://shortcut
    // interception below). Read back in onCreate/onNewIntent to send the
    // WebView straight to that note instead of the usual Welcome.html.
    private static final String EXTRA_SHORTCUT_NOTE = "omngo_shortcut_note";

    // Own-package broadcast createNoteShortcut() asks ShortcutManager to
    // fire once the launcher actually finishes pinning a shortcut (as
    // opposed to the user dismissing the confirmation), so we can toast a
    // clear "done" instead of leaving the detour to the home screen
    // unconfirmed. See the comment in createNoteShortcut() for why that
    // detour happens at all and can't be skipped.
    private static final String ACTION_SHORTCUT_PINNED = "net.basov.omngo.SHORTCUT_PINNED";
    private static final String EXTRA_SHORTCUT_PINNED_LABEL = "label";
    private android.content.BroadcastReceiver shortcutPinnedReceiver;

    // True when intent was launched via the QuickNoteAlias activity-alias
    // (the second "OMN-Go Quick Note" app-drawer icon - see the manifest)
    // rather than the normal MainActivity launcher entry. Android resolves
    // the alias to MainActivity to actually run it, but leaves the
    // ORIGINAL alias component name on the Intent the activity receives -
    // it does not rewrite getComponent() to MainActivity's own name - which
    // is what makes the two entry points distinguishable here at all.
    private boolean isQuickNoteAliasLaunch(android.content.Intent intent) {
        return intent != null && intent.getComponent() != null
            && intent.getComponent().getClassName().endsWith(".QuickNoteAlias");
    }

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // Restore the "which result extra were we waiting for" marker as early
        // as possible (before onActivityResult can fire), in case the process
        // was killed while a capture activity (e.g. the barcode scanner) was in
        // the foreground. See launchCaptureIntent / handleCaptureResult and the
        // onSaveInstanceState override below.
        if (savedInstanceState != null) {
            pendingCaptureExtra = savedInstanceState.getString(STATE_PENDING_CAPTURE_EXTRA);
        }

        shortcutPinnedReceiver = new android.content.BroadcastReceiver() {
            @Override
            public void onReceive(android.content.Context context, android.content.Intent intent) {
                String label = intent.getStringExtra(EXTRA_SHORTCUT_PINNED_LABEL);
                showToast("\"" + (label != null ? label : "Shortcut") + "\" added to your Home screen.");
            }
        };
        android.content.IntentFilter shortcutPinnedFilter = new android.content.IntentFilter(ACTION_SHORTCUT_PINNED);
        if (android.os.Build.VERSION.SDK_INT >= 33) {
            registerReceiver(shortcutPinnedReceiver, shortcutPinnedFilter, android.content.Context.RECEIVER_NOT_EXPORTED);
        } else {
            registerReceiver(shortcutPinnedReceiver, shortcutPinnedFilter);
        }

        // The Go server (plus storage-dir setup) is owned by ServerService.
        // It is started with plain startService() - NOT
        // startForegroundService() - on purpose: the service itself decides
        // from config.json whether to promote to foreground (LAN sharing
        // on) or stay a plain background service (sharing off), and
        // startForegroundService() would impose the 5-second "must call
        // startForeground" obligation even in the sharing-off case where
        // no notification is wanted. Mismatches between how the service
        // was started and what it did were exactly the source of the
        // "notification doesn't match sharing state" bugs.
        boolean lanSharing = ServerService.isLanSharingEnabled(this);

        // Permissions are requested ONLY when LAN sharing is actually
        // enabled - i.e. at sharing start time (first launch after the
        // ShareLAN restart), never on ordinary local-only app starts.
        if (lanSharing) {
            // Android 13+ needs runtime consent for the sharing
            // notification to be visible.
            if (android.os.Build.VERSION.SDK_INT >= 33 &&
                    checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS)
                            != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                requestPermissions(
                    new String[]{ android.Manifest.permission.POST_NOTIFICATIONS }, 1002);
            }

            // Deep Doze (long screen-off periods) suspends network for
            // apps regardless of wake locks; the battery-optimization
            // exemption is what keeps LAN requests answered with the
            // screen locked. Asked at most once - if declined, it can be
            // granted later via system Settings > Battery.
            try {
                android.os.PowerManager pm = (android.os.PowerManager) getSystemService(android.content.Context.POWER_SERVICE);
                if (pm != null && !pm.isIgnoringBatteryOptimizations(getPackageName())) {
                    android.content.SharedPreferences prefs = getSharedPreferences("omngo", MODE_PRIVATE);
                    if (!prefs.getBoolean("asked_battery_opt", false)) {
                        prefs.edit().putBoolean("asked_battery_opt", true).apply();
                        android.content.Intent bi = new android.content.Intent(
                            android.provider.Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS);
                        bi.setData(android.net.Uri.parse("package:" + getPackageName()));
                        startActivity(bi);
                    }
                }
            } catch (Exception e) {
                e.printStackTrace();
            }
        }

        startService(new android.content.Intent(this, ServerService.class));
        // Create Native Loading Layout
        android.widget.FrameLayout rootLayout = new android.widget.FrameLayout(this);
        rootLayout.setBackgroundColor(android.graphics.Color.parseColor("#f9f9f9"));
        
        final android.widget.ProgressBar progressBar = new android.widget.ProgressBar(this);
        android.widget.FrameLayout.LayoutParams pbParams = new android.widget.FrameLayout.LayoutParams(
            android.view.ViewGroup.LayoutParams.WRAP_CONTENT,
            android.view.ViewGroup.LayoutParams.WRAP_CONTENT);
        pbParams.gravity = android.view.Gravity.CENTER;
        progressBar.setLayoutParams(pbParams);

        // Initialize WebView
        webView = new WebView(this);
        webView.setLayoutParams(new android.widget.FrameLayout.LayoutParams(
            android.view.ViewGroup.LayoutParams.MATCH_PARENT,
            android.view.ViewGroup.LayoutParams.MATCH_PARENT));

        WebSettings webSettings = webView.getSettings();
        webSettings.setJavaScriptEnabled(true);
        webSettings.setDomStorageEnabled(true);

        webView.setWebChromeClient(new android.webkit.WebChromeClient() {
            @Override
            public boolean onJsAlert(android.webkit.WebView view, String url, String message, android.webkit.JsResult result) {
                new android.app.AlertDialog.Builder(view.getContext())
                    .setMessage(message)
                    .setPositiveButton("OK", (d, w) -> result.confirm())
                    .setOnCancelListener(d -> result.cancel())
                    .show();
                return true;
            }

            @Override
            public boolean onJsConfirm(android.webkit.WebView view, String url, String message, android.webkit.JsResult result) {
                new android.app.AlertDialog.Builder(view.getContext())
                    .setMessage(message)
                    .setPositiveButton("OK", (d, w) -> result.confirm())
                    .setNegativeButton("Cancel", (d, w) -> result.cancel())
                    .setOnCancelListener(d -> result.cancel())
                    .show();
                return true;
            }

            @Override
            public boolean onJsPrompt(android.webkit.WebView view, String url, String message, String defaultValue, android.webkit.JsPromptResult result) {
                android.widget.EditText input = new android.widget.EditText(view.getContext());
                input.setText(defaultValue);
                new android.app.AlertDialog.Builder(view.getContext())
                    .setMessage(message)
                    .setView(input)
                    .setPositiveButton("OK", (d, w) -> result.confirm(input.getText().toString()))
                    .setNegativeButton("Cancel", (d, w) -> result.cancel())
                    .setOnCancelListener(d -> result.cancel())
                    .show();
                return true;
            }
        });

        webView.setWebViewClient(new WebViewClient() {
            @Override
            public void onPageFinished(WebView view, String url) {
                progressBar.setVisibility(android.view.View.GONE);
                super.onPageFinished(view, url);
            }
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                if (url != null && url.startsWith("omngo://shortcut")) {
                    try {
                        String query = url.substring(url.indexOf('?') + 1);
                        String name = null;
                        String title = null;
                        for (String param : query.split("&")) {
                            int eq = param.indexOf('=');
                            if (eq < 0) continue;
                            String key = param.substring(0, eq);
                            String value = android.net.Uri.decode(param.substring(eq + 1));
                            if ("name".equals(key)) {
                                name = value;
                            } else if ("title".equals(key)) {
                                title = value;
                            }
                        }
                        MainActivity.this.createNoteShortcut(name, title);
                    } catch (Exception e) {
                        e.printStackTrace();
                    }
                    return true;
                }

                if (url != null && url.startsWith("omngo://edit")) {
                    try {
                        String name = url.substring(url.indexOf("?name=") + 6);
                        if (name.contains("&")) {
                            name = name.split("&")[0];
                        }
                        name = android.net.Uri.decode(name);
                        currentEditingName = name;
                        
                        // Disable strict mode exposed file exceptions
                        android.os.StrictMode.VmPolicy.Builder builder = new android.os.StrictMode.VmPolicy.Builder();
                        android.os.StrictMode.setVmPolicy(builder.build());

                        // Determine correct subdirectory and extension.
                        // MainActivity.this (not a bare storageDir() call)
                        // because this whole block is inside the anonymous
                        // WebViewClient below, where "this" means the
                        // WebViewClient itself.
                        java.io.File file;
                        String editStorageDir = MainActivity.this.storageDir();
                        if (name.endsWith(".md")) {
                            file = new java.io.File(editStorageDir + "/md/" + name);
                        } else {
                            file = new java.io.File(editStorageDir + "/html/" + name);
                        }
                        if (!file.exists()) {
                            file.getParentFile().mkdirs();
                            file.createNewFile();
                        }

                        android.content.Intent intent = new android.content.Intent(android.content.Intent.ACTION_EDIT);
                        intent.setDataAndType(android.net.Uri.fromFile(file), "text/plain");
                        intent.addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION | android.content.Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
                        
                        MainActivity.this.startActivityForResult(android.content.Intent.createChooser(intent, "Edit Markdown File"), 1001);
                    } catch (Exception e) {
                        e.printStackTrace();
                    }
                    return true;
                }

                if (url != null && url.startsWith("intent:")) {
                    // Android intent-URI links authored in notes - both the
                    // bare "intent:#Intent;...;end" form and the
                    // "intent://host/...#Intent;...;end" form (both share the
                    // "intent:" prefix). Gated behind the enable_intent_uri
                    // config toggle (default off), read live from config.json
                    // so a Settings change applies without an app restart -
                    // the same native-read pattern readMaxUploadSizeMB() uses.
                    // The Termux RUN_COMMAND convention (a note running a
                    // shell command) is additionally gated and confirmed; see
                    // handleIntentUri() and the block comment above it. This
                    // is why the generic "any other scheme" branch below no
                    // longer needs its own intent:// special-case.
                    handleIntentUri(url);
                    return true;
                }

                if (url != null && (url.startsWith("http://") || url.startsWith("https://"))) {
                    if (!url.contains("localhost") && !url.contains("127.0.0.1")) {
                        view.getContext().startActivity(
                            new android.content.Intent(android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url))
                        );
                        return true;
                    }
                    // Local app traffic (our own Go server) - let the WebView load it itself.
                    return false;
                }

                if (url != null) {
                    // Any other scheme (tel:, mailto:, geo:, sms:, market:,
                    // whatsapp:, etc.) is something the WebView has no
                    // renderer for - it fails with ERR_UNKNOWN_URL_SCHEME if
                    // we don't intercept it here. Hand it off to the OS so
                    // the matching app (Dialer, Maps, Email, Messaging...)
                    // can handle it instead. The "intent:" scheme (both the
                    // bare "intent:#Intent;...;end" and "intent://..." forms)
                    // is fully handled in its own branch above via
                    // handleIntentUri(), so it never reaches here.
                    try {
                        android.content.Intent intent = new android.content.Intent(
                            android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url));
                        if (intent.resolveActivity(view.getContext().getPackageManager()) != null) {
                            view.getContext().startActivity(intent);
                        }
                    } catch (Exception e) {
                        // No app installed to handle this scheme, or a
                        // malformed URI - nothing sane to do with it, so
                        // swallow it rather than crash or let the WebView
                        // throw ERR_UNKNOWN_URL_SCHEME.
                        e.printStackTrace();
                    }
                    return true;
                }
                return false;
            }
        });
        rootLayout.addView(webView);
        rootLayout.addView(progressBar);
        setContentView(rootLayout);

        // Wait for the Go server to bind before loading
        new Handler(Looper.getMainLooper()).postDelayed(new Runnable() {
            @Override
            public void run() {
                String startUrl = MainActivity.this.serverBase() + "/Welcome.html";
                android.content.Intent intent = getIntent();
                String shortcutNote = intent.getStringExtra(EXTRA_SHORTCUT_NOTE);
                if (shortcutNote != null && !shortcutNote.isEmpty()) {
                    // Tapped a pinned shortcut (see createNoteShortcut()) -
                    // go straight to that note instead of Welcome.html.
                    startUrl = MainActivity.this.serverBase() + "/" + android.net.Uri.encode(shortcutNote) + ".html";
                } else if (isQuickNoteAliasLaunch(intent)) {
                    // Tapped the second "OMN-Go Quick Note" app-drawer icon
                    // (see the QuickNoteAlias activity-alias in the
                    // manifest) - still loads Welcome.html so the app has a
                    // normal page underneath, but with a query flag
                    // omn-go-core.js's load handler uses to pop the Quick
                    // Note panel open immediately, same as the share_text/
                    // share_subject flags below do for shared text.
                    startUrl += "?quicknote=1";
                } else if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())
                        && intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM) == null) {
                    String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
                    String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
                    startUrl += "?share_text=" + (sharedText != null ? android.net.Uri.encode(sharedText) : "") +
                                "&share_subject=" + (sharedSubject != null ? android.net.Uri.encode(sharedSubject) : "");
                } else if (isSharedFileIntent(intent)) {
                    // Handled entirely natively (see handleSharedFile) -
                    // startUrl is deliberately left alone; this cold start
                    // still lands on Welcome.html like any other launch.
                    android.net.Uri sharedUri = (android.net.Uri) intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM);
                    if (sharedUri != null) {
                        handleSharedFile(sharedUri, intent.getType());
                    }
                }
                webView.loadUrl(startUrl);
            }
        }, 1000); // 1 second delay
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, android.content.Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == REQ_CAPTURE_RESULT) {
            // Result from a capture launch (e.g. a barcode scan) - paste it
            // into Quick Notes. See handleCaptureResult for all the "no
            // result" cases, which are handled gracefully.
            handleCaptureResult(resultCode, data);
            return;
        }
        if (requestCode == 1001 && webView != null) {
            if (currentEditingName != null && !currentEditingName.isEmpty()) {
                // currentEditingName already carries its extension (e.g. "Welcome.md"),
                // since it comes straight from the omngo://edit?name= URL built by the
                // frontend as currentNote + PAGE_EXT. Blindly appending ".html" here used
                // to produce "Welcome.md.html", which the server then re-suffixed into a
                // "Welcome.md.md" file on disk. Strip the existing extension first so we
                // reload the actual page name, matching handleEditExternal's viewURL logic
                // on the desktop side.
                String baseName = currentEditingName;
                int dotIdx = baseName.lastIndexOf('.');
                if (dotIdx > 0) {
                    baseName = baseName.substring(0, dotIdx);
                }
                webView.loadUrl(serverBase() + "/" + android.net.Uri.encode(baseName) + ".html");
                currentEditingName = null;
            } else {
                webView.reload(); // Refresh view when returning from external editor
            }
        }
    }

    @Override
    protected void onSaveInstanceState(android.os.Bundle outState) {
        super.onSaveInstanceState(outState);
        // Persist which result extra we're waiting for so a capture (e.g. a
        // barcode scan) still pastes into Quick Notes even if the OS killed
        // this process while the scanner was foreground. Restored in onCreate.
        if (pendingCaptureExtra != null) {
            outState.putString(STATE_PENDING_CAPTURE_EXTRA, pendingCaptureExtra);
        }
    }

    @Override
    protected void onNewIntent(android.content.Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        String shortcutNote = intent.getStringExtra(EXTRA_SHORTCUT_NOTE);
        if (shortcutNote != null && !shortcutNote.isEmpty()) {
            // App was already running (singleTask) and a pinned shortcut
            // was tapped - jump the existing WebView straight to that note.
            if (webView != null) {
                webView.loadUrl(serverBase() + "/" + android.net.Uri.encode(shortcutNote) + ".html");
            }
        } else if (isQuickNoteAliasLaunch(intent)) {
            // App was already running and the Quick Note app-drawer icon
            // was tapped. Unlike the cold-start case (onCreate, which
            // reloads Welcome.html with ?quicknote=1), this just pops the
            // panel open on whatever page is already showing - reloading
            // here would throw away the current page the same way the
            // shared-text branch below avoids doing for a warm start. A
            // silent no-op (via the `p &&` guard) if the current page
            // doesn't have #quickPanel at all (e.g. mid-edit on
            // editor.html) mirrors the same caveat noted for window.handleShare.
            if (webView != null) {
                webView.evaluateJavascript(
                    "javascript:(function(){ var p=document.getElementById('quickPanel'); if(p) p.classList.remove('hidden'); })();",
                    null);
            }
        } else if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())
                && intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM) == null) {
            String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
            String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
            if (webView != null) {
                String tText = sharedText != null ? android.net.Uri.encode(sharedText) : "";
                String tSubj = sharedSubject != null ? android.net.Uri.encode(sharedSubject) : "";
                String js = "javascript:(function(){ if(window.handleShare) window.handleShare(decodeURIComponent('" + tText + "'), decodeURIComponent('" + tSubj + "')); })();";
                webView.evaluateJavascript(js, null);
            }
        } else if (isSharedFileIntent(intent)) {
            // Unlike the text/plain branch above, this never touches
            // webView at all - see the block comment above
            // handleSharedFile() for why a warm-start share can't safely
            // assume anything about what the WebView is currently
            // showing (e.g. it could be mid-edit of some other note on
            // editor.html, which doesn't even define window.handleShare).
            android.net.Uri sharedUri = (android.net.Uri) intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM);
            if (sharedUri != null) {
                handleSharedFile(sharedUri, intent.getType());
            }
        }
    }

    @Override
    public void onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack();
        } else {
            super.onBackPressed();
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        if (shortcutPinnedReceiver != null) {
            try {
                unregisterReceiver(shortcutPinnedReceiver);
            } catch (Exception e) {
                // Already unregistered, or never successfully registered -
                // either way there's nothing left to clean up.
            }
        }
    }

    // ----------------------------------------------------------------------
    // Shared file handling (images / JSON via Android's "Share to" chooser)
    // ----------------------------------------------------------------------
    //
    // Shared TEXT is handled via window.handleShare in JS (see the
    // text/plain branches above): that works regardless of app state
    // because it always targets a page-independent modal (Quick Note /
    // Bookmark) that exists on every ordinary view page. A shared FILE
    // doesn't have an equivalent safe target: the WebView can't read a
    // content:// Uri without a JS bridge, and there's no guarantee the app
    // is even showing a page that has window.handleShare defined (e.g. it
    // could currently be on editor.html, mid-edit of some unrelated note,
    // which doesn't load omn-go-core.js at all).
    //
    // So this is handled entirely natively, independent of whatever the
    // WebView is doing:
    //   1. Validate + copy the shared file straight onto the same on-disk
    //      tree the Go server serves from (storageDir()/html/images or
    //      .../user_json), enforcing the same extension whitelist and
    //      max-size limit (read from config.json's max_upload_size_mb)
    //      that saveUploadedFile enforces server-side for the editor's
    //      own drag-and-drop upload (see backend/handlers.go). Keep the
    //      whitelist here in sync with imageUploadExtensions /
    //      jsonUploadExtensions there if either changes.
    //   2. Build the same snippet format those Go handlers return (an
    //      HTML <img class="omn-imported-image"> tag for images,
    //      [name](/user_json/name) markdown link syntax for JSON) and
    //      POST it as a Quick Note via the existing
    //      /api/quick endpoint - reusing the server's QuickNotes.md
    //      append/compile logic (handleQuickNote) rather than duplicating
    //      it here. Loopback requests bypass authMiddleware entirely (see
    //      backend/middleware.go), so no session/cookie handling is needed.
    // Runs entirely on a background thread and never touches webView, so
    // it's safe no matter what page (if any) is currently loaded. Only
    // single-file shares are handled (ACTION_SEND, not
    // ACTION_SEND_MULTIPLE) - matching the scope of the existing
    // text/plain share handling above.

    // JSON and image extensions this app accepts via share - kept in sync
    // with jsonUploadExtensions / imageUploadExtensions in
    // backend/handlers.go. These two sets are the single source within this
    // file: both isSharedFileIntent and handleSharedFile use them, so the
    // lists are never re-typed inline.
    private static final java.util.Set<String> SHARED_JSON_EXT =
        new java.util.HashSet<>(java.util.Arrays.asList(".json", ".jsonl"));
    private static final java.util.Set<String> SHARED_IMAGE_EXT =
        new java.util.HashSet<>(java.util.Arrays.asList(".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"));

    private boolean isSharedFileIntent(android.content.Intent intent) {
        if (!android.content.Intent.ACTION_SEND.equals(intent.getAction())) return false;
        android.net.Uri stream = intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM);
        if (stream == null) return false;
        String type = intent.getType();
        if (type != null && (type.startsWith("image/") || "application/json".equals(type))) {
            return true;
        }
        // Many senders (file managers, chat apps, "Files") hand a JSON (or
        // occasionally an image) share over with a generic/wrong MIME type
        // - application/octet-stream, text/plain, or no type at all -
        // rather than "application/json" or "image/*". That's exactly why
        // JSON sharing "did nothing" in practice: the type check above
        // never matched, so isSharedFileIntent returned false and the
        // whole share was silently dropped, even though the file itself
        // was perfectly fine. Fall back to sniffing the shared file's own
        // display name/extension instead of trusting the declared type.
        String name = queryDisplayName(stream);
        if (name != null) {
            String lower = name.toLowerCase(java.util.Locale.ROOT);
            int dot = lower.lastIndexOf('.');
            if (dot >= 0) {
                String ext = lower.substring(dot);
                if (SHARED_JSON_EXT.contains(ext) || SHARED_IMAGE_EXT.contains(ext)) {
                    return true;
                }
            }
        }
        return false;
    }

    private void handleSharedFile(final android.net.Uri uri, final String mimeType) {
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    String displayName = queryDisplayName(uri);
                    String lowerName = displayName == null ? "" : displayName.toLowerCase(java.util.Locale.ROOT);
                    boolean isJson = "application/json".equals(mimeType) || "application/jsonl".equals(mimeType)
                        || lowerName.endsWith(".json") || lowerName.endsWith(".jsonl");

                    java.util.Set<String> allowedExt = isJson ? SHARED_JSON_EXT : SHARED_IMAGE_EXT;

                    String filename = sanitizeSharedFilename(displayName, isJson);
                    String ext = filename.substring(filename.lastIndexOf('.')).toLowerCase(java.util.Locale.ROOT);
                    if (!allowedExt.contains(ext)) {
                        showToast("Not saved: only images, .json or .jsonl files can be shared into OMN-Go.");
                        return;
                    }

                    long maxBytes = (long) readMaxUploadSizeMB() * 1024 * 1024;

                    String subDir = isJson ? "user_json" : "images";
                    java.io.File destDir = new java.io.File(storageDir() + "/html/" + subDir);
                    destDir.mkdirs();
                    java.io.File destFile = new java.io.File(destDir, filename);

                    long copied = copyUriToFile(uri, destFile, maxBytes);
                    if (copied < 0) {
                        destFile.delete();
                        showToast("Not saved: file is larger than the configured upload limit.");
                        return;
                    }

                    // Same format handleUpload/handleUploadJSON in
                    // backend/handlers.go produce - keep these in sync by
                    // hand if either changes. Images went from markdown
                    // image syntax to an HTML <img> tag (with the
                    // .omn-imported-image class - see omn-go-core.css) so
                    // dropped images get a sane default size instead of
                    // rendering at full native resolution; this native
                    // share path builds its own snippet independently of
                    // the Go server (see the block comment above) and was
                    // still emitting the old markdown form here, so images
                    // shared into a fresh Android install rendered without
                    // the class desktop drag-and-drop already got. JSON
                    // stays markdown link syntax, now with the same
                    // leading/trailing newline handleUploadJSON already
                    // wraps it in, so a shared file lands on its own line.
                    String snippet;
                    if (isJson) {
                        snippet = "\n[" + filename + "](/user_json/" + filename + ")\n";
                    } else {
                        String escapedName = android.text.Html.escapeHtml(filename);
                        snippet = "\n<img src=\"/images/" + escapedName + "\" alt=\"" + escapedName
                            + "\" class=\"omn-imported-image\" />\n";
                    }

                    postQuickNoteWithRetry(snippet);
                    showToast(isJson ? "JSON file added to Quick Notes" : "Image added to Quick Notes");
                } catch (Exception e) {
                    e.printStackTrace();
                    showToast("Failed to save shared file: " + e.getMessage());
                }
            }
        }).start();
    }

    private String queryDisplayName(android.net.Uri uri) {
        String name = null;
        android.database.Cursor cursor = getContentResolver().query(uri, null, null, null, null);
        if (cursor != null) {
            try {
                int idx = cursor.getColumnIndex(android.provider.OpenableColumns.DISPLAY_NAME);
                if (idx >= 0 && cursor.moveToFirst()) {
                    name = cursor.getString(idx);
                }
            } finally {
                cursor.close();
            }
        }
        return name;
    }

    // Falls back to a generated name when the content provider doesn't
    // supply one, and strips any path separators a provider might smuggle
    // into DISPLAY_NAME so this can never write outside destDir.
    private String sanitizeSharedFilename(String displayName, boolean isJson) {
        String name = displayName;
        if (name == null || name.trim().isEmpty()) {
            name = "shared_" + System.currentTimeMillis() + (isJson ? ".json" : ".png");
        }
        name = name.replace('\\', '/');
        int slash = name.lastIndexOf('/');
        if (slash >= 0) name = name.substring(slash + 1);
        if (name.isEmpty() || name.lastIndexOf('.') <= 0) {
            name = name + (isJson ? ".json" : ".png");
        }
        return name;
    }

    // Reads max_upload_size_mb straight out of config.json - this path
    // writes the shared file directly to disk rather than going through
    // the Go server's /api/upload(_json), so it can't rely on
    // a.maxUploadBytes() server-side and duplicates the same default
    // (defaultMaxUploadSizeMB in backend/config.go) if config.json is
    // missing or unreadable.
    private int readMaxUploadSizeMB() {
        try {
            java.io.File cfgFile = new java.io.File(storageDir(), "config.json");
            if (!cfgFile.exists()) return 3;
            java.io.FileInputStream fis = new java.io.FileInputStream(cfgFile);
            java.io.ByteArrayOutputStream bos = new java.io.ByteArrayOutputStream();
            byte[] buf = new byte[4096];
            int n;
            while ((n = fis.read(buf)) != -1) bos.write(buf, 0, n);
            fis.close();
            org.json.JSONObject cfg = new org.json.JSONObject(bos.toString("UTF-8"));
            int mb = cfg.optInt("max_upload_size_mb", 3);
            return mb > 0 ? mb : 3;
        } catch (Exception e) {
            return 3; // matches backend/config.go's defaultMaxUploadSizeMB
        }
    }

    // Copies uri's bytes to destFile, aborting (returns -1; the partial
    // file is left for the caller to delete) once the stream exceeds
    // maxBytes. There's no multipart header with a declared size here
    // (unlike saveUploadedFile server-side), so the limit is enforced
    // while streaming instead of checked up front.
    private long copyUriToFile(android.net.Uri uri, java.io.File destFile, long maxBytes) throws java.io.IOException {
        java.io.InputStream in = getContentResolver().openInputStream(uri);
        if (in == null) throw new java.io.IOException("could not open shared file");
        try {
            // Opening the destination is inside this try/finally too, so a
            // FileOutputStream failure (e.g. permissions) still closes
            // `in` instead of leaking it.
            java.io.OutputStream out = new java.io.FileOutputStream(destFile);
            try {
                long total = 0;
                byte[] buf = new byte[8192];
                int n;
                while ((n = in.read(buf)) != -1) {
                    total += n;
                    if (total > maxBytes) {
                        return -1;
                    }
                    out.write(buf, 0, n);
                }
                return total;
            } finally {
                out.close();
            }
        } finally {
            in.close();
        }
    }

    private void postQuickNoteWithRetry(String note) throws java.io.IOException {
        try {
            postQuickNote(note);
        } catch (java.io.IOException firstErr) {
            // The Go server may still be starting up (same race the 1s
            // postDelayed in onCreate/loadUrl already accounts for) - one
            // short retry covers a cold start that's just barely slower
            // than usual instead of losing the note entirely.
            try {
                Thread.sleep(1500);
            } catch (InterruptedException ignored) {
                Thread.currentThread().interrupt();
            }
            postQuickNote(note);
        }
    }

    // POSTs note (already-built markdown) to /api/quick, appending it to
    // QuickNotes.md - see handleQuickNote in backend/handlers.go.
    private void postQuickNote(String note) throws java.io.IOException {
        java.net.URL url = new java.net.URL(serverBase() + "/api/quick");
        java.net.HttpURLConnection conn = (java.net.HttpURLConnection) url.openConnection();
        try {
            conn.setRequestMethod("POST");
            conn.setDoOutput(true);
            conn.setRequestProperty("Content-Type", "application/x-www-form-urlencoded; charset=utf-8");
            String body = "note=" + java.net.URLEncoder.encode(note, "UTF-8");
            byte[] bodyBytes = body.getBytes("UTF-8");
            conn.setFixedLengthStreamingMode(bodyBytes.length);
            java.io.OutputStream os = conn.getOutputStream();
            os.write(bodyBytes);
            os.close();
            int code = conn.getResponseCode();
            if (code != 200) {
                throw new java.io.IOException("server returned HTTP " + code);
            }
        } finally {
            conn.disconnect();
        }
    }

    // ----------------------------------------------------------------------
    // Android intent-URI links (incl. Termux RUN_COMMAND integration)
    // ----------------------------------------------------------------------
    //
    // Reached from shouldOverrideUrlLoading when a note link's URL starts with
    // "intent:". Reproduces the behavior of the pre-OMN-Go app (mvbasov/OMN),
    // including its Termux argument-packing convention, so intent: links in
    // notes authored for that app keep working here unchanged.
    //
    // Two independent config toggles gate this, both default off and read live
    // from config.json at tap time (readConfigFlag), so a Settings change
    // applies on the next tap without an app restart:
    //   - enable_intent_uri   : master switch. Off => no intent link launches.
    //   - enable_termux_intent : additionally allows the Termux RUN_COMMAND
    //                            path (a note starting a shell command via
    //                            com.termux/.app.RunCommandService).
    //
    // The Termux path is deliberately hardened beyond old OMN with FOUR
    // independent consents before a note can run a shell command: the master
    // toggle on, the Termux toggle on, the RUN_COMMAND permission granted, and
    // a per-tap confirmation dialog. That is because OMN-Go notes are not
    // necessarily self-authored - git sync and (when LAN sharing is on) other
    // devices editing over the network can put third-party content in front of
    // this WebView, and a single tap must never silently run a shell command.

    private static final int REQ_TERMUX_PERMISSION = 1003;
    private static final String TERMUX_PACKAGE = "com.termux";
    private static final String TERMUX_PERMISSION = "com.termux.permission.RUN_COMMAND";
    private static final String TERMUX_RUN_COMMAND_PATH = "com.termux.RUN_COMMAND_PATH";
    private static final String TERMUX_RUN_COMMAND_ARGS = "com.termux.RUN_COMMAND_ARGUMENTS";
    private static final String TERMUX_RUN_COMMAND_LABEL = "com.termux.RUN_COMMAND_LABEL";

    // Activity-result capture (e.g. a barcode scan pasted into Quick Notes).
    // OMNGO_CAPTURE_EXTRA is OMN-Go's private marker on an intent: URI whose
    // value names the result extra to read back; REQ_CAPTURE_RESULT tags the
    // startActivityForResult call so onActivityResult can recognize it.
    // pendingCaptureExtra holds that result-extra name across the launch, and
    // is round-tripped through onSaveInstanceState (STATE_PENDING_CAPTURE_EXTRA)
    // so it survives the process being killed while the launched activity is in
    // the foreground.
    private static final int REQ_CAPTURE_RESULT = 1004;
    private static final String OMNGO_CAPTURE_EXTRA = "omngo_capture_extra";
    private static final String STATE_PENDING_CAPTURE_EXTRA = "omngo_pending_capture_extra";
    private String pendingCaptureExtra;

    private void handleIntentUri(final String url) {
        if (!readConfigFlag("enable_intent_uri")) {
            showToast("Intent links are turned off. Enable them in Settings.");
            return;
        }

        final android.content.Intent intentApp;
        try {
            // parseUri handles both the bare "intent:#Intent;...;end" and the
            // "intent://...#Intent;...;end" forms, and percent-decodes string
            // extra values (so "%20" inside a packed Termux arg becomes a
            // space here, which the packing convention below relies on).
            intentApp = android.content.Intent.parseUri(url, android.content.Intent.URI_INTENT_SCHEME);
        } catch (Exception e) {
            e.printStackTrace();
            showToast("This intent link is malformed.");
            return;
        }

        // Presence of the RUN_COMMAND_PATH extra is what marks this as a
        // Termux "run a shell command" intent rather than an ordinary one; it
        // is checked first because a Termux command's output comes back by a
        // different mechanism than an activity result (a future feature), not
        // via the omngo_capture_extra path below.
        if (intentApp.hasExtra(TERMUX_RUN_COMMAND_PATH)) {
            launchTermuxIntent(intentApp);
            return;
        }

        // OMN-Go's own opt-in marker for "launch this for a result and paste
        // it into Quick Notes". Its value names the result extra to read back
        // (e.g. "SCAN_RESULT" for a barcode scan). Only when a note explicitly
        // carries it do we wait for a result; every other intent stays
        // fire-and-forget. See launchCaptureIntent / handleCaptureResult.
        String captureExtra = intentApp.getStringExtra(OMNGO_CAPTURE_EXTRA);
        if (captureExtra != null && !captureExtra.isEmpty()) {
            launchCaptureIntent(intentApp, captureExtra);
        } else {
            launchGenericIntent(intentApp);
        }
    }

    // Ordinary (non-Termux) intent: hand to the OS as an activity - e.g. an
    // android.settings.* screen, or a third-party app deep link.
    // resolveActivity() is deliberately NOT used as a pre-check here: under
    // API 30+ package visibility it can return null even for actions the
    // system itself would handle (some android.settings.* screens included),
    // so a try/catch around startActivity is the reliable form. Honors the
    // standard S.browser_fallback_url extra (loaded in the WebView) when no
    // installed app can handle the intent.
    private void launchGenericIntent(final android.content.Intent intentApp) {
        String fallbackUrl = intentApp.getStringExtra("browser_fallback_url");
        // Don't leave the fallback URL sitting in the launched intent's extras
        // where the target activity might misread it.
        intentApp.removeExtra("browser_fallback_url");
        try {
            startActivity(intentApp);
        } catch (android.content.ActivityNotFoundException e) {
            if (fallbackUrl != null
                    && (fallbackUrl.startsWith("http://") || fallbackUrl.startsWith("https://"))) {
                if (webView != null) {
                    webView.loadUrl(fallbackUrl);
                }
            } else {
                showToast("No app can handle this link.");
            }
        } catch (Exception e) {
            e.printStackTrace();
            showToast("Couldn't open this link.");
        }
    }

    // Capture path: launch the target app FOR A RESULT, remembering which
    // result extra to read back when it finishes. Only reached when a note
    // opted in with omngo_capture_extra (see handleIntentUri). No confirmation
    // dialog: unlike Termux, this launches an ordinary app UI (a scanner) and
    // only pastes text - it runs nothing on the device.
    private void launchCaptureIntent(final android.content.Intent intentApp, final String captureExtra) {
        // FLAG_ACTIVITY_NEW_TASK (and its NEW_DOCUMENT / MULTIPLE_TASK
        // relatives) start the target in a separate task, which severs the
        // result chain so onActivityResult would never fire. Clear them so the
        // result actually comes back to us, whatever flags the parsed URI set.
        intentApp.setFlags(intentApp.getFlags()
                & ~android.content.Intent.FLAG_ACTIVITY_NEW_TASK
                & ~android.content.Intent.FLAG_ACTIVITY_NEW_DOCUMENT
                & ~android.content.Intent.FLAG_ACTIVITY_MULTIPLE_TASK);
        // Don't leak OMN-Go's private marker to the target app.
        intentApp.removeExtra(OMNGO_CAPTURE_EXTRA);
        pendingCaptureExtra = captureExtra;
        try {
            startActivityForResult(intentApp, REQ_CAPTURE_RESULT);
        } catch (android.content.ActivityNotFoundException e) {
            pendingCaptureExtra = null;
            showToast("No app can handle this link.");
        } catch (Exception e) {
            pendingCaptureExtra = null;
            e.printStackTrace();
            showToast("Couldn't open this link.");
        }
    }

    // Handles the return from a capture launch (REQ_CAPTURE_RESULT). Called
    // from onActivityResult. Every "no result" path (canceled, no data, or the
    // requested extra absent) is handled gracefully - Android delivers this
    // callback even when the launched activity set no result at all
    // (resultCode == RESULT_CANCELED, data == null), so there's nothing to
    // crash on. On success the text is appended to Quick Notes via the same
    // /api/quick path shared-file handling already uses.
    private void handleCaptureResult(int resultCode, android.content.Intent data) {
        final String extraName = pendingCaptureExtra;
        pendingCaptureExtra = null; // consume it either way
        if (extraName == null) {
            // A capture callback with no remembered extra name - e.g. the
            // process was killed and onSaveInstanceState state wasn't restored.
            // Nothing actionable.
            return;
        }
        if (resultCode != RESULT_OK || data == null) {
            // User backed out of the scanner, or the activity returned nothing.
            // Deliberately silent - a canceled scan isn't an error.
            return;
        }
        final String value = extractResultText(data, extraName);
        if (value == null || value.isEmpty()) {
            // The app returned OK but not the extra this note asked for.
            showToast("No \"" + extraName + "\" result was returned.");
            return;
        }
        // Off the UI thread: this makes a loopback HTTP call to /api/quick.
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    // Surrounding newlines so the result lands on its own line,
                    // matching the shared-file snippet format.
                    postQuickNoteWithRetry("\n" + value + "\n");
                    showToast("Result added to Quick Notes");
                    runOnUiThread(new Runnable() {
                        @Override
                        public void run() {
                            // If Quick Notes is the page on screen, refresh so
                            // the pasted result shows immediately.
                            if (webView != null) {
                                String cur = webView.getUrl();
                                if (cur != null && cur.contains("QuickNotes")) {
                                    webView.reload();
                                }
                            }
                        }
                    });
                } catch (java.io.IOException e) {
                    e.printStackTrace();
                    showToast("Couldn't save the result to Quick Notes.");
                }
            }
        }).start();
    }

    // Reads the named result extra out of a returned Intent. A barcode scan
    // returns a single String (SCAN_RESULT); the reader also accepts a
    // String-ArrayList extra (joining its entries) so the same path works for
    // any result-returning app without special-casing, at no extra cost.
    private String extractResultText(android.content.Intent data, String extraName) {
        String s = data.getStringExtra(extraName);
        if (s != null) {
            return s;
        }
        java.util.ArrayList<String> list = data.getStringArrayListExtra(extraName);
        if (list != null && !list.isEmpty()) {
            StringBuilder sb = new StringBuilder();
            for (int i = 0; i < list.size(); i++) {
                if (i > 0) sb.append('\n');
                sb.append(list.get(i));
            }
            return sb.toString();
        }
        return null;
    }

    // Termux RUN_COMMAND path. Enforces the second toggle + Termux installed +
    // permission granted, applies old OMN's argument-packing convention, then
    // asks for explicit confirmation before starting the service.
    private void launchTermuxIntent(final android.content.Intent intentApp) {
        if (!readConfigFlag("enable_termux_intent")) {
            showToast("Termux commands are turned off. Enable them in Settings.");
            return;
        }
        if (!isPackageInstalled(TERMUX_PACKAGE)) {
            showToast("Termux is not installed.");
            return;
        }
        if (checkSelfPermission(TERMUX_PERMISSION)
                != android.content.pm.PackageManager.PERMISSION_GRANTED) {
            // Ask now; the user grants it and taps the link again. Kept as a
            // re-tap (rather than an auto-retry via onRequestPermissionsResult)
            // so there's no cross-callback state to hold for this rare path.
            requestPermissions(new String[]{ TERMUX_PERMISSION }, REQ_TERMUX_PERMISSION);
            showToast("Grant Termux the RUN_COMMAND permission, then tap the link again.");
            return;
        }

        // Old OMN packing convention: an intent URI can't carry a String[]
        // extra, so arguments are packed into RUN_COMMAND_PATH as
        // "path?arg1&arg2&..." (with %20 for a space inside an argument,
        // already decoded by parseUri above). Unpack into the real
        // RUN_COMMAND_PATH + RUN_COMMAND_ARGUMENTS[] Termux expects. The
        // path/args boundary is split on only the FIRST '?' (limit 2), a safe
        // superset of old OMN's unlimited split: every note that worked there
        // had no '?' inside an argument (or it was already broken), so results
        // are identical for existing notes and additionally correct when an
        // argument itself contains '?'. Arguments are still separated on every
        // '&', matching old OMN (an argument therefore cannot contain a
        // literal '&' - a documented limitation of the packing).
        String cmdPath = intentApp.getStringExtra(TERMUX_RUN_COMMAND_PATH);
        if (cmdPath != null && cmdPath.contains("?")) {
            String[] cmdParts = cmdPath.split("\\?", 2);
            intentApp.putExtra(TERMUX_RUN_COMMAND_PATH, cmdParts[0].trim());
            if (cmdParts.length > 1 && !cmdParts[1].isEmpty()) {
                intentApp.putExtra(TERMUX_RUN_COMMAND_ARGS, cmdParts[1].split("&"));
            }
        }

        // Human-readable summary for the confirmation dialog: the note's
        // RUN_COMMAND_LABEL if present, then the resolved command line.
        String label = intentApp.getStringExtra(TERMUX_RUN_COMMAND_LABEL);
        String path = intentApp.getStringExtra(TERMUX_RUN_COMMAND_PATH);
        String[] args = intentApp.getStringArrayExtra(TERMUX_RUN_COMMAND_ARGS);
        StringBuilder summary = new StringBuilder();
        if (label != null && !label.isEmpty()) {
            summary.append(label).append("\n\n");
        }
        summary.append(path != null ? path : "");
        if (args != null) {
            for (String arg : args) {
                summary.append(' ').append(arg);
            }
        }

        new android.app.AlertDialog.Builder(this)
            .setTitle("Run Termux command?")
            .setMessage(summary.toString().trim())
            .setPositiveButton("Run", (d, w) -> startTermuxService(intentApp))
            .setNegativeButton("Cancel", null)
            .show();
    }

    // Starts com.termux/.app.RunCommandService. On API 26+ the service must be
    // started with startForegroundService() - RunCommandService promotes
    // itself to the foreground with a notification, and a plain startService()
    // there can throw once Termux calls startForeground late. Below 26,
    // startService(). A SecurityException here almost always means Termux's
    // allow-external-apps is not set, so the message points the user at it.
    private void startTermuxService(final android.content.Intent intentApp) {
        try {
            if (android.os.Build.VERSION.SDK_INT >= 26) {
                startForegroundService(intentApp);
            } else {
                startService(intentApp);
            }
        } catch (SecurityException e) {
            showToast("Termux refused the command. In Termux, set allow-external-apps=true "
                + "in ~/.termux/termux.properties and run termux-reload-settings.");
        } catch (Exception e) {
            e.printStackTrace();
            showToast("Couldn't start the Termux command.");
        }
    }

    private boolean isPackageInstalled(String pkg) {
        try {
            getPackageManager().getPackageInfo(pkg, 0);
            return true;
        } catch (android.content.pm.PackageManager.NameNotFoundException e) {
            return false;
        }
    }

    // Reads a boolean flag out of config.json (default false when the file or
    // key is missing/unreadable). Same native-read approach as
    // readMaxUploadSizeMB(): these Android-consumed toggles never go through
    // the Go HTTP server, and reading fresh on each call means a Settings
    // change applies on the next tap without an app restart.
    private boolean readConfigFlag(String key) {
        try {
            java.io.File cfgFile = new java.io.File(storageDir(), "config.json");
            if (!cfgFile.exists()) return false;
            java.io.FileInputStream fis = new java.io.FileInputStream(cfgFile);
            java.io.ByteArrayOutputStream bos = new java.io.ByteArrayOutputStream();
            byte[] buf = new byte[4096];
            int n;
            while ((n = fis.read(buf)) != -1) bos.write(buf, 0, n);
            fis.close();
            org.json.JSONObject cfg = new org.json.JSONObject(bos.toString("UTF-8"));
            return cfg.optBoolean(key, false);
        } catch (Exception e) {
            return false;
        }
    }

    // ----------------------------------------------------------------------
    // Home-screen shortcuts ("add to home screen" for the current note)
    // ----------------------------------------------------------------------
    //
    // Triggered by the .android-only button in index.html (createNoteShortcut()
    // in omn-go-core.js), which navigates to omngo://shortcut?name=... -
    // intercepted in shouldOverrideUrlLoading above, same pattern as
    // omngo://edit.
    //
    // Deliberately built on the plain platform SDK only - no androidx.core/
    // appcompat - matching this project's size-conscious approach (see the
    // empty libs/ fileTree in build.gradle): android.content.pm.ShortcutManager
    // + android.graphics.drawable.Icon (both first-party android.jar classes,
    // no extra dependency) cover API 26+, and the classic
    // "com.android.launcher.action.INSTALL_SHORTCUT" broadcast - the same
    // approach this project used pre-OMN-Go (see
    // https://stackoverflow.com/a/16873257) - covers API 24-25, where
    // ShortcutManager.requestPinShortcut doesn't exist yet.
    //
    // The shortcut icon is composited the same way the app's own launcher
    // icon is (see res/mipmap-anydpi-v26/ic_launcher.xml): the shared
    // ic_launcher_background painted first, then ic_launcher_shortcut_foreground
    // drawn on top - rather than the shortcut foreground alone, which left
    // pinned shortcuts looking like a plain white card instead of matching
    // the app's actual icon.
    private void createNoteShortcut(final String name, final String title) {
        if (name == null || name.isEmpty()) return;
        final String label = (title != null && !title.isEmpty()) ? title : name;

        final android.graphics.Bitmap icon = renderDrawableToBitmap(
            R.drawable.ic_launcher_background, R.drawable.ic_launcher_shortcut_foreground);

        final android.content.Intent shortcutIntent = new android.content.Intent(this, MainActivity.class);
        shortcutIntent.setAction(android.content.Intent.ACTION_VIEW);
        shortcutIntent.addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK | android.content.Intent.FLAG_ACTIVITY_CLEAR_TOP);
        shortcutIntent.putExtra(EXTRA_SHORTCUT_NOTE, name);
        // Gives each per-note shortcut intent distinct Uri data so the OS
        // never mistakes two different notes' shortcuts for the same
        // intent (data itself is otherwise ignored - EXTRA_SHORTCUT_NOTE
        // above is what onCreate/onNewIntent actually read).
        shortcutIntent.setData(android.net.Uri.parse("omngo-shortcut://note/" + android.net.Uri.encode(name)));

        if (android.os.Build.VERSION.SDK_INT >= 26) {
            // Two ways to place a shortcut on API 26+, with a real
            // trade-off between them (see the per-method comments below) -
            // there's no single answer that's best for everyone, so ask
            // each time rather than hardcoding one. Below API 26,
            // ShortcutManager doesn't exist, so there's nothing to choose:
            // the legacy broadcast is the only option (see the else branch).
            new android.app.AlertDialog.Builder(this)
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
            // Pre-Oreo (API 24-25): ShortcutManager doesn't exist yet, so
            // the legacy broadcast is the only option - nothing to ask.
            pinShortcutViaLegacyBroadcast(shortcutIntent, icon, label);
        }
    }

    // "Reliable": the official ShortcutManager API (API 26+). Always works
    // on modern launchers, but requestPinShortcut hands off to the
    // LAUNCHER's own "Add to Home screen?" confirmation UI - a separate
    // app/process the OS deliberately puts in front of us (so no app can
    // silently plant shortcuts) - and most launchers then drop the user on
    // the Home screen to show where the new icon landed. That hand-off
    // can't be suppressed from here - it's the launcher's screen, not
    // ours - but OMN-Go itself is only backgrounded (paused/stopped), never
    // finished or killed: switching back via Recents or the app icon (not
    // the new shortcut) returns to this exact page. The toast below makes
    // that expected detour explicit instead of it looking like the app just
    // vanished; the pinned-callback toast (via shortcutPinnedReceiver)
    // confirms once the launcher actually finishes adding it.
    private void pinShortcutViaShortcutManager(android.content.Intent shortcutIntent,
            android.graphics.Bitmap icon, String label, String name) {
        android.content.pm.ShortcutManager shortcutManager =
            (android.content.pm.ShortcutManager) getSystemService(android.content.Context.SHORTCUT_SERVICE);
        if (shortcutManager == null || !shortcutManager.isRequestPinShortcutSupported()) {
            showToast("Your home screen doesn't support pinned shortcuts.");
            return;
        }

        android.graphics.drawable.Icon shortcutIcon = icon != null
            ? android.graphics.drawable.Icon.createWithAdaptiveBitmap(icon)
            : android.graphics.drawable.Icon.createWithResource(this, R.mipmap.ic_launcher);

        // Shortcut id is per-note (not random), so re-adding a shortcut for
        // the same note updates/re-pins the existing one instead of piling
        // up duplicates.
        String shortcutId = "note_" + name;
        android.content.pm.ShortcutInfo shortcut =
            new android.content.pm.ShortcutInfo.Builder(this, shortcutId)
                .setShortLabel(label)
                .setLongLabel("Open \"" + label + "\" in OMN-Go")
                .setIcon(shortcutIcon)
                .setIntent(shortcutIntent)
                .build();

        showToast("Confirm \"" + label + "\" on your Home screen - OMN-Go stays open in the background.");

        android.content.Intent callbackIntent = new android.content.Intent(ACTION_SHORTCUT_PINNED);
        callbackIntent.setPackage(getPackageName());
        callbackIntent.putExtra(EXTRA_SHORTCUT_PINNED_LABEL, label);
        android.app.PendingIntent callback = android.app.PendingIntent.getBroadcast(
            this, shortcutId.hashCode(), callbackIntent,
            android.app.PendingIntent.FLAG_UPDATE_CURRENT | android.app.PendingIntent.FLAG_IMMUTABLE);

        shortcutManager.requestPinShortcut(shortcut, callback.getIntentSender());
    }

    // "Quick": the pre-Oreo launcher broadcast (still the only option below
    // API 26 - see the else branch in createNoteShortcut()). No confirmation
    // UI and no Home-screen jump, but two separate deprecation effects on
    // API 26+ (confirmed on a real Android 14 device), not just one:
    //   - many current launchers (stock Pixel launcher, recent Nova, etc.)
    //     have stopped listening for this broadcast entirely since migrating
    //     to ShortcutManager, so it can silently do nothing - there's no
    //     broadcast result to check, so we can't detect that and warn.
    //   - even where a launcher still honors the broadcast enough to create
    //     a shortcut, EXTRA_SHORTCUT_ICON/_RESOURCE are frequently ignored by
    //     the OS's own compatibility handling for this frozen API, so the
    //     shortcut lands with a generic default icon instead of the one
    //     built here - that's an OS-level restriction on the deprecated
    //     broadcast itself, not something fixable by changing what we pass
    //     it (a larger/differently-formatted bitmap makes no difference).
    //     ShortcutManager ("Reliable") is the only path that reliably
    //     carries a custom icon on modern Android.
    // The manifest permission declared alongside is a no-op on launchers
    // that don't check it.
    private void pinShortcutViaLegacyBroadcast(android.content.Intent shortcutIntent,
            android.graphics.Bitmap icon, String label) {
        android.content.Intent installIntent = new android.content.Intent();
        installIntent.putExtra(android.content.Intent.EXTRA_SHORTCUT_INTENT, shortcutIntent);
        installIntent.putExtra(android.content.Intent.EXTRA_SHORTCUT_NAME, label);
        if (icon != null) {
            installIntent.putExtra(android.content.Intent.EXTRA_SHORTCUT_ICON, icon);
        } else {
            installIntent.putExtra(android.content.Intent.EXTRA_SHORTCUT_ICON_RESOURCE,
                android.content.Intent.ShortcutIconResource.fromContext(this, R.mipmap.ic_launcher));
        }
        installIntent.setAction("com.android.launcher.action.INSTALL_SHORTCUT");
        sendBroadcast(installIntent);
        showToast("Shortcut requested - check your Home screen (icon and behavior vary by launcher).");
    }

    // Rasterizes one or more drawable resources (vector or otherwise) onto a
    // single square bitmap sized for an adaptive icon (108dp, matching both
    // ic_launcher_background.xml and ic_launcher_shortcut_foreground.xml's
    // declared width/height), painting them in the given order so later
    // resIds layer on top of earlier ones - same layering the OS itself does
    // for the app's own launcher icon (background then foreground). Returns
    // null (falls back to the plain app icon) if any layer fails to resolve,
    // rather than pinning a shortcut with only some of its layers drawn.
    // getDrawable(int) is a plain Context method (API 21+) - no compat
    // library needed to resolve a vector drawable resource.
    private android.graphics.Bitmap renderDrawableToBitmap(int... resIds) {
        try {
            int size = Math.round(108 * getResources().getDisplayMetrics().density);
            android.graphics.Bitmap bitmap = android.graphics.Bitmap.createBitmap(
                size, size, android.graphics.Bitmap.Config.ARGB_8888);
            android.graphics.Canvas canvas = new android.graphics.Canvas(bitmap);
            for (int resId : resIds) {
                android.graphics.drawable.Drawable drawable = getDrawable(resId);
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

    private void showToast(final String msg) {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                android.widget.Toast.makeText(MainActivity.this, msg, android.widget.Toast.LENGTH_LONG).show();
            }
        });
    }
}
