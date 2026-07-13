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

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

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
                    // intent://, whatsapp:, etc.) is something the WebView
                    // has no renderer for - it fails with
                    // ERR_UNKNOWN_URL_SCHEME if we don't intercept it here.
                    // Hand it off to the OS so the matching app (Dialer,
                    // Maps, Email, Messaging...) can handle it instead.
                    try {
                        android.content.Intent intent;
                        if (url.startsWith("intent://")) {
                            // "intent:" links pack extra info (target
                            // package, fallback URL, etc.) into a special
                            // URI format that needs Intent.parseUri rather
                            // than a plain Uri.parse + ACTION_VIEW.
                            intent = android.content.Intent.parseUri(url, android.content.Intent.URI_INTENT_SCHEME);
                        } else {
                            intent = new android.content.Intent(android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url));
                        }
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

    // JSON extensions this app accepts via share - kept in sync with
    // jsonUploadExtensions in backend/handlers.go (there's only one, but
    // written as a set for parallelism with the image list below and in
    // case that ever changes).
    private static final java.util.Set<String> SHARED_JSON_EXT =
        new java.util.HashSet<>(java.util.Arrays.asList(".json"));
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
                    boolean isJson = "application/json".equals(mimeType)
                        || (displayName != null && displayName.toLowerCase(java.util.Locale.ROOT).endsWith(".json"));

                    java.util.Set<String> allowedExt = new java.util.HashSet<>(isJson
                        ? java.util.Arrays.asList(".json")
                        : java.util.Arrays.asList(".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"));

                    String filename = sanitizeSharedFilename(displayName, isJson);
                    String ext = filename.substring(filename.lastIndexOf('.')).toLowerCase(java.util.Locale.ROOT);
                    if (!allowedExt.contains(ext)) {
                        showToast("Not saved: only images or .json files can be shared into OMN-Go.");
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
        String label = (title != null && !title.isEmpty()) ? title : name;

        android.graphics.Bitmap icon = renderDrawableToBitmap(
            R.drawable.ic_launcher_background, R.drawable.ic_launcher_shortcut_foreground);

        android.content.Intent shortcutIntent = new android.content.Intent(this, MainActivity.class);
        shortcutIntent.setAction(android.content.Intent.ACTION_VIEW);
        shortcutIntent.addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK | android.content.Intent.FLAG_ACTIVITY_CLEAR_TOP);
        shortcutIntent.putExtra(EXTRA_SHORTCUT_NOTE, name);
        // Gives each per-note shortcut intent distinct Uri data so the OS
        // never mistakes two different notes' shortcuts for the same
        // intent (data itself is otherwise ignored - EXTRA_SHORTCUT_NOTE
        // above is what onCreate/onNewIntent actually read).
        shortcutIntent.setData(android.net.Uri.parse("omngo-shortcut://note/" + android.net.Uri.encode(name)));

        if (android.os.Build.VERSION.SDK_INT >= 26) {
            android.content.pm.ShortcutManager shortcutManager =
                (android.content.pm.ShortcutManager) getSystemService(android.content.Context.SHORTCUT_SERVICE);
            if (shortcutManager == null || !shortcutManager.isRequestPinShortcutSupported()) {
                showToast("Your home screen doesn't support pinned shortcuts.");
                return;
            }

            android.graphics.drawable.Icon shortcutIcon = icon != null
                ? android.graphics.drawable.Icon.createWithAdaptiveBitmap(icon)
                : android.graphics.drawable.Icon.createWithResource(this, R.mipmap.ic_launcher);

            // Shortcut id is per-note (not random), so re-adding a shortcut
            // for the same note updates/re-pins the existing one instead of
            // piling up duplicates.
            String shortcutId = "note_" + name;
            android.content.pm.ShortcutInfo shortcut =
                new android.content.pm.ShortcutInfo.Builder(this, shortcutId)
                    .setShortLabel(label)
                    .setLongLabel("Open \"" + label + "\" in OMN-Go")
                    .setIcon(shortcutIcon)
                    .setIntent(shortcutIntent)
                    .build();

            shortcutManager.requestPinShortcut(shortcut, null);
        } else {
            // Pre-Oreo (API 24-25): ShortcutManager doesn't exist yet, so
            // fall back to the classic launcher broadcast. Most launchers
            // from this era honor it without any permission prompt; the
            // manifest permission declared alongside is a no-op on
            // launchers that don't check it.
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
            showToast("Shortcut requested - check your home screen.");
        }
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
