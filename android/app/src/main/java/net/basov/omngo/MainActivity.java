package net.basov.omngo;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.os.Handler;
import android.os.Looper;
import net.basov.omngo.backend.Backend;

public class MainActivity extends Activity {
    private WebView webView;
    private String currentEditingName;

    /**
     * The cache of the WebView is cleared a maximum of one time in the
     * life of the process. onCreate operates again after a recreate of
     * the activity, but Backend.assetsRefreshed() keeps its answer until
     * the process stops, thus the guard must be static.
     */
    private static boolean assetCacheCleared = false;

    // Storage dir and server port both used to be hardcoded here, as
    // "net.basov.omngo" and "8080". That broke on the fdroid flavor,
    // which has a different applicationId and thus a different external
    // media directory. See the productFlavors block of build.gradle. It
    // also broke on any install where a person changed the Server Port of
    // the Config page away from the default.
    //
    // Both are now resolved live. storageDir() defers to
    // ServerService.storageDir(), which is the same helper that
    // Backend.startServer() is started with. See
    // ServerService.onStartCommand. serverBase() reads the configured port
    // with ServerService.serverPort(), and it assumes no 8080.

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

    // Own-package broadcast. createNoteShortcut() asks ShortcutManager to
    // fire it once the launcher finishes the pin of a shortcut. A
    // dismissal of the confirmation fires nothing. We can thus toast a
    // clear "done", and the detour to the home screen does not stay
    // unconfirmed. See the comment in createNoteShortcut() for why that
    // detour happens at all and cannot be skipped.
    private static final String ACTION_SHORTCUT_PINNED = "net.basov.omngo.SHORTCUT_PINNED";
    private static final String EXTRA_SHORTCUT_PINNED_LABEL = "label";
    private android.content.BroadcastReceiver shortcutPinnedReceiver;

    // True when the intent came from the QuickNoteAlias activity-alias.
    // That is the second "OMN-Go Quick Note" app-drawer icon, see the
    // manifest. The normal MainActivity launcher entry is the other one.
    //
    // Android resolves the alias to MainActivity to run it, and it leaves
    // the ORIGINAL alias component name on the Intent that the activity
    // receives. It does not rewrite getComponent() to the own name of
    // MainActivity. That is what makes the two entry points different here
    // at all.
    private boolean isQuickNoteAliasLaunch(android.content.Intent intent) {
        return intent != null && intent.getComponent() != null
            && intent.getComponent().getClassName().endsWith(".QuickNoteAlias");
    }

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // Restore the "which result extra were we waiting for" marker as
        // early as possible, before onActivityResult can fire. The process
        // can be killed while a capture activity is in the foreground, for
        // example the barcode scanner. See launchCaptureIntent and
        // handleCaptureResult, and the onSaveInstanceState override below.
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

        // Receiver for Termux command results. See the capture path of
        // launchTermuxIntent. It uses the same self-package, NOT_EXPORTED
        // pattern as shortcutPinnedReceiver above. It is dynamic, and thus
        // activity-scoped. When the OS kills this process while a long
        // command still runs, the result cannot be delivered. The command
        // still runs, and only the paste-back dialog is lost. Accepted for
        // v1, because a typical captured command finishes in well under a
        // second.
        termuxResultReceiver = new android.content.BroadcastReceiver() {
            @Override
            public void onReceive(android.content.Context context, android.content.Intent intent) {
                handleTermuxResult(intent);
            }
        };
        android.content.IntentFilter termuxResultFilter = new android.content.IntentFilter(ACTION_TERMUX_RESULT);
        if (android.os.Build.VERSION.SDK_INT >= 33) {
            registerReceiver(termuxResultReceiver, termuxResultFilter, android.content.Context.RECEIVER_NOT_EXPORTED);
        } else {
            registerReceiver(termuxResultReceiver, termuxResultFilter);
        }

        // The Go server, and the storage-dir setup with it, is owned by
        // ServerService. It is started with plain startService(), and NOT
        // with startForegroundService(). That is on purpose. The service
        // itself reads config.json and decides what to do. It promotes to
        // the foreground when LAN sharing is on. It stays a plain
        // background service when sharing is off. startForegroundService()
        // would impose the 5-second "must call startForeground" obligation
        // even in the sharing-off case, where no notification is wanted. A
        // mismatch between how the service was started and what it did made
        // real bugs. The notification then did not match the sharing state.
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

            // Deep Doze suspends the network for an app, and a wake lock
            // does not change that. Deep Doze starts after a long period
            // with the screen off. The battery-optimization exemption is
            // what keeps LAN requests answered with the screen locked.
            // Asked at most once. A person who declines can grant it later
            // in system Settings > Battery.
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
            // The same spinner that covers the initial server-start wait is
            // reused for every later navigation. Several pages are built by
            // the Go server at request time. OMNGoTags rescans every note.
            // A note whose .md is newer than its cached .html is recompiled
            // on the first view, which is every changed note after a pull.
            // A tap could otherwise sit on the old screen with no feedback
            // for seconds. This work sits here and not in JS, thus it also
            // covers the hardware Back button and a shortcut launch. No
            // in-page click handler can observe those.
            @Override
            public void onPageStarted(WebView view, String url, android.graphics.Bitmap favicon) {
                progressBar.setVisibility(android.view.View.VISIBLE);
                super.onPageStarted(view, url, favicon);
            }
            @Override
            public void onPageFinished(WebView view, String url) {
                progressBar.setVisibility(android.view.View.GONE);
                // A save on the Config page reloads it. That is what makes
                // a "Fullscreen mode" change apply at once, and not at the
                // next app start.
                applyFullscreenMode();
                super.onPageFinished(view, url);
            }
            // A failed load may never reach onPageFinished, and the spinner
            // would then stay on screen. Clear it here too.
            @Override
            public void onReceivedError(WebView view, android.webkit.WebResourceRequest request,
                                        android.webkit.WebResourceError error) {
                progressBar.setVisibility(android.view.View.GONE);
                super.onReceivedError(view, request, error);
            }
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                // Sending a note out. The frontend's Send control navigates
                // here (see omnGoSendNote in omn-go-core.js) because a
                // WebView cannot open a share sheet by itself.
                if (url != null && url.startsWith("omngo://share")) {
                    MainActivity.this.handleShareOut(android.net.Uri.parse(url));
                    return true;
                }

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
                        
                        MainActivity.allowFileUriHandoff();

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
                    // Android intent-URI links authored in notes. There are
                    // two forms, the bare "intent:#Intent;...;end" and
                    // "intent://host/...#Intent;...;end". Both share the
                    // "intent:" prefix. Gated behind the enable_intent_uri
                    // config toggle, which is off by default. The code reads
                    // it live from config.json, thus a Settings change
                    // applies with no app restart. That is the same
                    // native-read pattern that readMaxUploadSizeMB() uses.
                    // The Termux RUN_COMMAND convention is a note that runs
                    // a shell command. It is gated and confirmed as well.
                    // See handleIntentUri() and the block comment above it.
                    // The generic "any other scheme" branch below thus needs
                    // no intent:// special-case of its own.
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
                    // renderer for. It fails with ERR_UNKNOWN_URL_SCHEME
                    // when we do not intercept it here. Hand it off to the
                    // OS, and the matching app (Dialer, Maps, Email,
                    // Messaging...) handles it instead. The "intent:" scheme
                    // is fully handled in its own branch above, with
                    // handleIntentUri(), thus it never reaches here. That
                    // covers the bare "intent:#Intent;...;end" form and the
                    // "intent://..." form.
                    try {
                        android.content.Intent intent = new android.content.Intent(
                            android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url));
                        if (intent.resolveActivity(view.getContext().getPackageManager()) != null) {
                            view.getContext().startActivity(intent);
                        }
                    } catch (Exception e) {
                        // No app installed to handle this scheme, or a
                        // malformed URI. There is nothing sensible to do
                        // with it. Swallow it, and do not crash and do not
                        // let the WebView throw ERR_UNKNOWN_URL_SCHEME.
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

        // Applied before the first frame is drawn. A user who turned
        // fullscreen off then does not see the hidden status bar of the
        // manifest theme appear and slide back in.
        applyFullscreenMode();

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
                    // Tapped the second "OMN-Go Quick Note" app-drawer icon.
                    // See the QuickNoteAlias activity-alias in the manifest.
                    // It still loads Welcome.html, thus the app has a normal
                    // page underneath. It adds a query flag, and the load
                    // handler of omn-go-core.js reads that flag and pops the
                    // Quick Note panel open at once. The share_text and
                    // share_subject flags below do the same for shared text.
                    startUrl += "?quicknote=1";
                } else if (MainActivity.this.isSharedNoteIntent(intent)) {
                    // A note arrived as a FILE. Imported natively, like the
                    // image/JSON branch below - startUrl stays Welcome.html,
                    // and openImportedNote takes the WebView to the note when
                    // the import answers.
                    MainActivity.this.importSharedNote(MainActivity.this.sharedNoteUri(intent));
                } else if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())
                        && intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM) == null) {
                    String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
                    String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
                    if (MainActivity.this.looksLikeSharedNote(sharedText)) {
                        // A note sent AS TEXT. It does not go through the
                        // URL. A whole note is the wrong size for a query
                        // string, and the note box is not where it belongs.
                        MainActivity.this.importSharedText(sharedText);
                    } else {
                        startUrl += "?share_text=" + (sharedText != null ? android.net.Uri.encode(sharedText) : "") +
                                    "&share_subject=" + (sharedSubject != null ? android.net.Uri.encode(sharedSubject) : "");
                    }
                } else if (isSharedFileIntent(intent)) {
                    // Handled entirely natively (see handleSharedFile) -
                    // startUrl is deliberately left alone; this cold start
                    // still lands on Welcome.html like any other launch.
                    android.net.Uri sharedUri = (android.net.Uri) intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM);
                    if (sharedUri != null) {
                        handleSharedFile(sharedUri, intent.getType());
                    }
                }

                // An update of the application writes the shipped
                // scripts and style sheets again (refreshEmbeddedAssets
                // in backend/assets.go). The WebView can hold the
                // previous copy of those files in its disk cache, and
                // then some new pages do not operate correctly. The Go
                // side tells if it wrote such a file at this start, thus
                // the cache goes away only at the start after an update.
                //
                // The server is up at this point. startService above sends
                // onStartCommand to this same main thread, and
                // Backend.startServer completes the work with the assets
                // before it returns. See initStorage in backend/storage.go.
                // This runnable comes 1 second later.
                if (!assetCacheCleared && Backend.assetsRefreshed()) {
                    assetCacheCleared = true;
                    webView.clearCache(true);
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
        // Persist which result extra the code waits for. A capture, for
        // example a barcode scan, still pastes into Quick Notes. That holds
        // when the OS killed this process while the scanner was in the
        // foreground. Restored in onCreate.
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
            // was tapped. The cold-start case is onCreate, which reloads
            // Welcome.html with ?quicknote=1. This branch only pops the
            // panel open on whatever page is already showing. A reload here
            // would throw away the current page, which is what the
            // shared-text branch below avoids for a warm start. The `p &&`
            // guard makes a silent no-op when the current page has no
            // #quickPanel at all, for example mid-edit on editor.html. That
            // mirrors the caveat noted for window.handleShare.
            if (webView != null) {
                webView.evaluateJavascript(
                    "javascript:(function(){ var p=document.getElementById('quickPanel'); if(p) p.classList.remove('hidden'); })();",
                    null);
            }
        } else if (isSharedNoteIntent(intent)) {
            // A note as a FILE, warm. Native, like the image/JSON branch
            // below. openImportedNote decides whether the WebView may move.
            importSharedNote(sharedNoteUri(intent));
        } else if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())
                && intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM) == null
                && looksLikeSharedNote(intent.getStringExtra(android.content.Intent.EXTRA_TEXT))) {
            // A note as TEXT, warm. Straight to the importer, and never
            // through window.handleShare. That call fills the note box for
            // review, which is right for a thought. It is wrong for a note
            // that already has a name and a home.
            importSharedText(intent.getStringExtra(android.content.Intent.EXTRA_TEXT));
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
            // webView at all. See the block comment above
            // handleSharedFile() for why a warm-start share cannot safely
            // assume anything about what the WebView shows at that moment.
            // It could be mid-edit of another note on editor.html, which
            // does not even define window.handleShare.
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
    // Fullscreen / system bars (Config page -> "Fullscreen mode")
    // ----------------------------------------------------------------------
    //
    // The app used to be unconditionally fullscreen, through
    // Theme.NoTitleBar.Fullscreen in AndroidManifest.xml. That theme is
    // still declared. It is what makes the DEFAULT case, with the status
    // bar hidden, draw correctly from the very first frame. No visible bar
    // flashes away once this code runs. applyFullscreenMode() then takes
    // over as the single authority for what is shown.
    //
    // The mode is read from config.json on each call, and that mirrors
    // readConfigFlag and readMaxUploadSizeMB. There is no HTTP round-trip.
    // A Settings change applies as soon as the Config page saves and
    // reloads, see the onPageFinished hook, and it needs no app restart.
    // config.json is a couple of KB, thus a re-read on navigation is
    // cheaper than a cache. That cache would have to stay coherent with an
    // edit made inside the WebView.
    //
    // Deliberately platform-only. There is no
    // androidx.core.view.WindowInsetsController compat wrapper, which
    // matches the no-AndroidX constraint of this project. The API 30+ path
    // uses android.view.WindowInsetsController directly. The older path
    // uses View.setSystemUiVisibility, which is deprecated from API 30. It
    // still works, and it is the only platform option on API 24-29. The
    // minSdk of this app is 24.
    //
    // The three mode names. OmnConfig holds the values, thus this file and
    // the reader can never disagree about what "immersive" is called. The
    // aliases stay, because applyFullscreenMode below reads better with a
    // short name.
    private static final String FULLSCREEN_OFF = OmnConfig.FULLSCREEN_OFF;
    private static final String FULLSCREEN_ON = OmnConfig.FULLSCREEN_ON;
    private static final String FULLSCREEN_IMMERSIVE = OmnConfig.FULLSCREEN_IMMERSIVE;

    // OmnConfig.fullscreenMode mirrors normalizeFullscreen in
    // backend/config.go. An unknown or absent value means "fullscreen",
    // thus a config.json written before this setting existed keeps the
    // behavior that install already had. See the banner of OmnConfig.
    private String readFullscreenMode() {
        return OmnConfig.fullscreenMode(storageDir());
    }

    private void applyFullscreenMode() {
        String mode = readFullscreenMode();
        android.view.Window window = getWindow();
        if (window == null) return;

        // The manifest theme sets FLAG_FULLSCREEN. On API 30+ that legacy
        // flag overrides WindowInsetsController. While the flag was set,
        // "off" could never show the status bar. Clear it unconditionally,
        // and let the branches below be the only authority.
        window.clearFlags(android.view.WindowManager.LayoutParams.FLAG_FULLSCREEN);

        if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.R) {
            android.view.WindowInsetsController controller = window.getInsetsController();
            if (controller == null) return;
            int status = android.view.WindowInsets.Type.statusBars();
            int nav = android.view.WindowInsets.Type.navigationBars();
            if (FULLSCREEN_OFF.equals(mode)) {
                controller.show(status | nav);
            } else if (FULLSCREEN_IMMERSIVE.equals(mode)) {
                // Swiping from an edge reveals the bars briefly, then they
                // hide again - without this they would stay up for good after
                // the first swipe.
                controller.setSystemBarsBehavior(
                        android.view.WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
                controller.hide(status | nav);
            } else {
                controller.show(nav);
                controller.hide(status);
            }
            return;
        }

        android.view.View decor = window.getDecorView();
        if (FULLSCREEN_OFF.equals(mode)) {
            decor.setSystemUiVisibility(0);
        } else if (FULLSCREEN_IMMERSIVE.equals(mode)) {
            decor.setSystemUiVisibility(
                    android.view.View.SYSTEM_UI_FLAG_FULLSCREEN
                            | android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                            // STICKY, not plain IMMERSIVE: the bars come back
                            // for a moment on a swipe and then re-hide by
                            // themselves, matching the API 30+ behaviour above.
                            | android.view.View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY);
        } else {
            decor.setSystemUiVisibility(android.view.View.SYSTEM_UI_FLAG_FULLSCREEN);
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        // Covers coming back from another app, and a mode change made on
        // another device that arrived via git sync.
        applyFullscreenMode();
    }

    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        // A return of the focus clears the hidden-bar state on some builds.
        // That happens when the keyboard closes, when a dialog closes, and
        // when the Termux confirmation returns. Assert the state again, and
        // do not fall back to a half-visible bar.
        if (hasFocus) applyFullscreenMode();
    }

    // Reads a string value out of config.json. OmnConfig holds the reader
    // and the default, and this method is the one line that remains.
    private String readConfigString(String key) {
        return OmnConfig.string(storageDir(), key);
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        if (shortcutPinnedReceiver != null) {
            try {
                unregisterReceiver(shortcutPinnedReceiver);
            } catch (Exception e) {
                // Already unregistered, or never registered at all. Either
                // way there is nothing left to clean up.
            }
        }
        if (termuxResultReceiver != null) {
            try {
                unregisterReceiver(termuxResultReceiver);
            } catch (Exception e) {
                // As above - nothing left to clean up.
            }
        }
    }

    // ----------------------------------------------------------------------
    // Shared file handling (images / JSON via Android's "Share to" chooser)
    // ----------------------------------------------------------------------
    //
    // Shared TEXT is handled with window.handleShare in JS. See the
    // text/plain branches above. That works whatever the app state is,
    // because it always targets a page-independent modal, which is Quick
    // Note or Bookmark. Such a modal exists on every ordinary view page.
    //
    // A shared FILE has no equivalent safe target. The WebView cannot read
    // a content:// Uri without a JS bridge. There is also no guarantee that
    // the app shows a page that defines window.handleShare. It could be on
    // editor.html, mid-edit of an unrelated note, which loads no
    // omn-go-core.js at all.
    //
    // So this is handled entirely natively, and it is independent of
    // whatever the WebView does.
    //   1. Validate the shared file and copy it straight onto the same
    //      on-disk tree that the Go server serves from, which is
    //      storageDir()/html/images or .../user_json. It enforces the same
    //      extension whitelist and max-size limit that saveUploadedFile
    //      enforces on the server for the own drag-and-drop upload of the
    //      editor. The limit comes from max_upload_size_mb in config.json.
    //      See backend/handlers.go. Keep the whitelist here in step with
    //      imageUploadExtensions and jsonUploadExtensions there if either
    //      one changes.
    //   2. Build the same snippet format that those Go handlers return.
    //      An image gets an HTML <img class="omn-imported-image"> tag, and
    //      JSON gets [name](/user_json/name) markdown link syntax. POST it
    //      as a Quick Note with the existing /api/quick endpoint. The
    //      QuickNotes.md append and compile logic of the server is thus
    //      reused and not duplicated here. See handleQuickNote. A loopback
    //      request bypasses authMiddleware entirely, see
    //      backend/middleware.go, thus no session or cookie handling is
    //      needed.
    // This runs entirely on a background thread and never touches webView,
    // thus it is safe whatever page is loaded. Only a single-file share is
    // handled, which is ACTION_SEND and not ACTION_SEND_MULTIPLE. That
    // matches the scope of the text/plain share handling above.

    // JSON and image extensions this app accepts via share - kept in sync
    // with jsonUploadExtensions / imageUploadExtensions in
    // backend/handlers.go. These two sets are the single source within this
    // file: both isSharedFileIntent and handleSharedFile use them, so the
    // lists are never re-typed inline.
    private static final java.util.Set<String> SHARED_JSON_EXT =
        new java.util.HashSet<>(java.util.Arrays.asList(".json", ".jsonl"));
    private static final java.util.Set<String> SHARED_IMAGE_EXT =
        new java.util.HashSet<>(java.util.Arrays.asList(".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"));

    // ----------------------------------------------------------------------
    // Receiving a note (note exchange, phase 5)
    // ----------------------------------------------------------------------
    //
    // A note arrives from Telegram, a mail client, LocalSend or a file
    // manager. It comes as a FILE through the share sheet, as a FILE
    // through ACTION_VIEW, or as TEXT in the message body.
    //
    // Every one of them ends at the same place, POST /api/import/note. The
    // rules live in Go, in backend/note_exchange.go. Those rules say where
    // a note lands, what its name becomes and how a collision is numbered.
    // This side repeats none of them. The block comment above
    // handleSharedFile below documents that trap, where the native image
    // path had to be kept in step with handleUpload by hand.

    /** Names a shared note by extension. ".txt" is not one: v1 sends notes. */
    private static final java.util.Set<String> SHARED_NOTE_EXT =
        new java.util.HashSet<>(java.util.Arrays.asList(".md", ".markdown"));

    private static boolean isMarkdownType(String type) {
        return "text/markdown".equals(type) || "text/x-markdown".equals(type);
    }

    private static boolean hasNoteExtension(String name) {
        if (name == null) return false;
        String lower = name.toLowerCase(java.util.Locale.ROOT);
        int dot = lower.lastIndexOf('.');
        return dot >= 0 && SHARED_NOTE_EXT.contains(lower.substring(dot));
    }

    /**
     * A shared or opened FILE that is a note.
     *
     * Checked before isSharedFileIntent, which owns images and JSON. The
     * declared type is not trusted on its own: Telegram and several mail
     * clients label a .md attachment "application/octet-stream", which is
     * why that type is in the manifest at all - so the file's own name
     * decides when the type says nothing useful.
     */
    private boolean isSharedNoteIntent(android.content.Intent intent) {
        return sharedNoteUri(intent) != null;
    }

    /** The URI of a shared or opened note, or null. */
    private android.net.Uri sharedNoteUri(android.content.Intent intent) {
        String action = intent.getAction();
        android.net.Uri uri = null;
        if (android.content.Intent.ACTION_SEND.equals(action)) {
            uri = intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM);
        } else if (android.content.Intent.ACTION_VIEW.equals(action)) {
            uri = intent.getData();
        }
        if (uri == null) return null;
        if (isMarkdownType(intent.getType())) return uri;
        return hasNoteExtension(queryDisplayName(uri)) ? uri : null;
    }

    /**
     * Whether shared TEXT is a note rather than a thought or a link.
     *
     * A ROUTING hint and nothing more: it decides which of two paths the
     * text takes, and the server validates whatever arrives. The rule is the
     * one splitFrontMatter uses in Go - a header block of "Key: value" lines
     * that does not begin with a space, "#" or "<" - and the question asked
     * of it is whether that block carries a FileName: line.
     *
     * Without this a note sent as text would land in the note box as a
     * quick note, header block and all.
     */
    private boolean looksLikeSharedNote(String text) {
        if (text == null || text.isEmpty()) return false;
        String[] lines = text.split("\n");
        for (int i = 0; i < lines.length && i < 32; i++) {
            String line = lines[i];
            if (line.endsWith("\r")) {
                line = line.substring(0, line.length() - 1);
            }
            if (line.trim().isEmpty()) return false;          // the block ended
            if (line.startsWith(" ") || line.startsWith("#") || line.startsWith("<")) return false;
            int colon = line.indexOf(':');
            if (colon <= 0) return false;                      // not a header line
            if ("filename".equals(line.substring(0, colon).trim()
                    .toLowerCase(java.util.Locale.ROOT))) {
                return true;
            }
        }
        return false;
    }

    /** Reads a shared file and imports it. */
    private void importSharedNote(final android.net.Uri uri) {
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    String displayName = queryDisplayName(uri);
                    long limit = (long) readMaxUploadSizeMB() * 1024 * 1024;
                    byte[] body = readUriCapped(uri, limit);
                    if (body == null) {
                        showToast("Not imported: the note is larger than the upload limit.");
                        return;
                    }
                    postImportedNote(body, displayName);
                } catch (Exception e) {
                    e.printStackTrace();
                    showToast("Could not import the note: " + e.getMessage());
                }
            }
        }).start();
    }

    /** Imports shared TEXT that looksLikeSharedNote accepted. */
    private void importSharedText(final String text) {
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    postImportedNote(text.getBytes("UTF-8"), null);
                } catch (Exception e) {
                    e.printStackTrace();
                    showToast("Could not import the note: " + e.getMessage());
                }
            }
        }).start();
    }

    /**
     * POSTs the note to the server and acts on the answer.
     *
     * The endpoint is admin only and this request comes from 127.0.0.1,
     * which bypasses that - the device is not a remote caller to its own
     * server. The retry mirrors postQuickNoteWithRetry: a share can arrive
     * during a cold start, while the Go side is still coming up.
     */
    private void postImportedNote(byte[] body, String displayName) throws java.io.IOException {
        String answer;
        try {
            answer = postImport(body, displayName);
        } catch (java.io.IOException first) {
            try {
                Thread.sleep(1500);
            } catch (InterruptedException ignored) {
                Thread.currentThread().interrupt();
            }
            answer = postImport(body, displayName);
        }

        String url = null;
        String base = null;
        try {
            org.json.JSONObject json = new org.json.JSONObject(answer);
            url = json.optString("url", null);
            base = json.optString("base", null);
        } catch (org.json.JSONException e) {
            // A 200 that is not JSON should not happen, and is not worth
            // losing the import over: the note is written either way.
            e.printStackTrace();
        }

        showToast(base != null ? ("Note imported: " + base) : "Note imported.");
        if (url != null && !url.isEmpty()) {
            openImportedNote(url);
        }
    }

    private String postImport(byte[] body, String displayName) throws java.io.IOException {
        String target = serverBase() + "/api/import/note";
        if (displayName != null && !displayName.isEmpty()) {
            target += "?name=" + java.net.URLEncoder.encode(displayName, "UTF-8");
        }
        java.net.HttpURLConnection conn =
            (java.net.HttpURLConnection) new java.net.URL(target).openConnection();
        try {
            conn.setRequestMethod("POST");
            conn.setDoOutput(true);
            conn.setRequestProperty("Content-Type", "text/markdown; charset=utf-8");
            conn.setFixedLengthStreamingMode(body.length);
            java.io.OutputStream os = conn.getOutputStream();
            os.write(body);
            os.close();

            int code = conn.getResponseCode();
            java.io.InputStream in = (code >= 400) ? conn.getErrorStream() : conn.getInputStream();
            String text = in == null ? "" : readAllUtf8(in);
            if (code != 200) {
                throw new java.io.IOException(importErrorMessage(text, code));
            }
            return text;
        } finally {
            conn.disconnect();
        }
    }

    /** The server's own words when it refuses, rather than a status code. */
    private String importErrorMessage(String body, int code) {
        try {
            String message = new org.json.JSONObject(body).optString("message", "");
            if (!message.isEmpty()) return message;
        } catch (org.json.JSONException ignored) {
            // fall through to the code
        }
        return "the server answered HTTP " + code;
    }

    /**
     * Opens the note that just arrived - EXCEPT when the editor is open.
     *
     * A share arrives while the user is in the middle of something. The
     * editor page holds text that is not saved, and a note that came from
     * Telegram is not worth throwing it away. The toast has already named
     * the note, and the incoming index lists it when the user is ready.
     *
     * The editor is recognised by its URL: ?edit=true is served by the
     * standalone editor page (see serveEditor in backend/handlers.go), and
     * the query stays in the address.
     */
    private void openImportedNote(final String url) {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                if (webView == null) return;
                String current = webView.getUrl();
                if (current != null && current.contains("edit=true")) return;
                webView.loadUrl(serverBase() + url);
            }
        });
    }

    /**
     * Reads a content:// URI into memory, or null when it is over the limit.
     *
     * Refuses rather than truncates: half a note is not an import. This is
     * the same choice readImportBody makes on the Go side, and the reason
     * neither of them is search.go's readCapped.
     */
    private byte[] readUriCapped(android.net.Uri uri, long maxBytes) throws java.io.IOException {
        java.io.InputStream in = getContentResolver().openInputStream(uri);
        if (in == null) throw new java.io.IOException("cannot open the shared file");
        try {
            java.io.ByteArrayOutputStream out = new java.io.ByteArrayOutputStream();
            byte[] chunk = new byte[8192];
            long total = 0;
            int read;
            while ((read = in.read(chunk)) != -1) {
                total += read;
                if (maxBytes > 0 && total > maxBytes) return null;
                out.write(chunk, 0, read);
            }
            return out.toByteArray();
        } finally {
            in.close();
        }
    }

    private boolean isSharedFileIntent(android.content.Intent intent) {
        if (!android.content.Intent.ACTION_SEND.equals(intent.getAction())) return false;
        android.net.Uri stream = intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM);
        if (stream == null) return false;
        String type = intent.getType();
        if (type != null && (type.startsWith("image/") || "application/json".equals(type))) {
            return true;
        }
        // Many senders hand a JSON share over with a generic or wrong MIME
        // type. File managers, chat apps and "Files" do this, and an image
        // share sometimes as well. The type is application/octet-stream,
        // text/plain, or no type at all, and not "application/json" or
        // "image/*". That is exactly why JSON sharing "did nothing" in
        // practice. The type check above never matched, thus
        // isSharedFileIntent returned false and the whole share was
        // silently dropped, and the file itself was perfectly fine. Fall
        // back to the own display name and extension of the shared file,
        // and do not trust the declared type.
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

                    // Same format that handleUpload and handleUploadJSON
                    // in backend/handlers.go produce. Keep these in step by
                    // hand if either one changes.
                    //
                    // Images went from markdown image syntax to an HTML
                    // <img> tag, with the .omn-imported-image class. See
                    // omn-go-core.css. A dropped image thus gets a sensible
                    // default size, and it does not render at full native
                    // resolution. This native share path builds its own
                    // snippet, independent of the Go server, see the block
                    // comment above. It still emitted the old markdown form
                    // here. An image shared into a fresh Android install
                    // then rendered without the class that desktop
                    // drag-and-drop already got.
                    //
                    // JSON stays markdown link syntax. It now carries the
                    // same leading and trailing newline that
                    // handleUploadJSON wraps it in, thus a shared file
                    // lands on its own line.
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

    // Falls back to a generated name when the content provider supplies
    // none. It also strips each path separator that a provider can smuggle
    // into DISPLAY_NAME, thus this can never write outside destDir.
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

    // Reads max_upload_size_mb straight out of config.json. This path
    // writes the shared file directly to disk, and it does not go through
    // /api/upload or /api/upload_json of the Go server. It thus cannot use
    // a.maxUploadBytes() on the server. It repeats the same default,
    // defaultMaxUploadSizeMB in backend/config.go, when config.json is
    // missing or unreadable.
    private int readMaxUploadSizeMB() {
        return OmnConfig.maxUploadMB(storageDir());
    }

    // Copies the bytes of uri to destFile. It aborts once the stream goes
    // over maxBytes, and it then returns -1 and leaves the partial file
    // for the caller to delete. There is no multipart header with a
    // declared size here, unlike saveUploadedFile on the server. The limit
    // is thus enforced during the stream, and not checked up front.
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
            // The Go server may still be in its start. That is the same
            // race that the 1s postDelayed in onCreate and loadUrl already
            // covers. One short retry covers a cold start that is a little
            // slower than usual, and the note is not lost.
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
    // Reached from shouldOverrideUrlLoading when the URL of a note link
    // starts with "intent:". It reproduces the behavior of the pre-OMN-Go
    // app, mvbasov/OMN, and the Termux argument-packing convention of that
    // app with it. An intent: link in a note authored for that app thus
    // keeps working here unchanged.
    //
    // Two independent config toggles gate this. Both are off by default,
    // and readConfigFlag reads them live from config.json at tap time. A
    // Settings change thus applies on the next tap, with no app restart:
    //   - enable_intent_uri   : master switch. Off => no intent link launches.
    //   - enable_termux_intent : additionally allows the Termux RUN_COMMAND
    //                            path (a note starting a shell command via
    //                            com.termux/.app.RunCommandService).
    //
    // The Termux path is deliberately hardened beyond old OMN. FOUR
    // independent consents come before a note can run a shell command.
    // The master toggle is on, the Termux toggle is on, the RUN_COMMAND
    // permission is granted, and a per-tap confirmation dialog is
    // answered.
    //
    // The reason is that an OMN-Go note is not necessarily self-authored.
    // Git sync can put third-party content in front of this WebView. So
    // can another device that edits over the network while LAN sharing is
    // on. A single tap must never silently run a shell command.

    private static final int REQ_TERMUX_PERMISSION = 1003;
    private static final String TERMUX_PACKAGE = "com.termux";
    private static final String TERMUX_PERMISSION = "com.termux.permission.RUN_COMMAND";
    private static final String TERMUX_RUN_COMMAND_PATH = "com.termux.RUN_COMMAND_PATH";
    private static final String TERMUX_RUN_COMMAND_ARGS = "com.termux.RUN_COMMAND_ARGUMENTS";
    private static final String TERMUX_RUN_COMMAND_LABEL = "com.termux.RUN_COMMAND_LABEL";

    // Activity-result capture, for example a barcode scan pasted into
    // Quick Notes. OMNGO_CAPTURE_EXTRA is the private marker of OMN-Go on
    // an intent: URI, and its value names the result extra to read back.
    // REQ_CAPTURE_RESULT tags the startActivityForResult call, thus
    // onActivityResult can recognize it. pendingCaptureExtra holds that
    // result-extra name across the launch. It is round-tripped through
    // onSaveInstanceState as STATE_PENDING_CAPTURE_EXTRA, thus it survives
    // a kill of the process while the launched activity is in the
    // foreground.
    private static final int REQ_CAPTURE_RESULT = 1004;
    private static final String OMNGO_CAPTURE_EXTRA = "omngo_capture_extra";
    private static final String STATE_PENDING_CAPTURE_EXTRA = "omngo_pending_capture_extra";
    private String pendingCaptureExtra;

    // Termux command-output capture (a note's shell command whose stdout/stderr
    // is pasted back into a review dialog). Opt-in per note via
    // OMNGO_CAPTURE_OUTPUT, whose value names the stream(s): "stdout" (default),
    // "stderr", or "both". Termux returns the result asynchronously through a
    // PendingIntent we supply (RUN_COMMAND_PENDING_INTENT), NOT via an activity
    // result - hence a broadcast receiver rather than onActivityResult. The
    // TERMUX_RESULT_* keys are the literal values from Termux's TermuxConstants
    // (verified against termux-app master): the result Bundle lives under the
    // "result" extra and carries stdout/stderr/exitCode/errmsg. OMNGO_STREAM /
    // OMNGO_LABEL ride on our own PendingIntent's base intent so the receiver
    // gets them back alongside Termux's bundle - no cross-call state to hold.
    private static final String OMNGO_CAPTURE_OUTPUT = "omngo_capture_output";
    private static final String TERMUX_RUN_COMMAND_BACKGROUND = "com.termux.RUN_COMMAND_BACKGROUND";
    private static final String TERMUX_RUN_COMMAND_PENDING_INTENT = "com.termux.RUN_COMMAND_PENDING_INTENT";
    private static final String TERMUX_RESULT_BUNDLE = "result";
    private static final String TERMUX_RESULT_STDOUT = "stdout";
    private static final String TERMUX_RESULT_STDERR = "stderr";
    private static final String TERMUX_RESULT_EXIT_CODE = "exitCode";
    private static final String ACTION_TERMUX_RESULT = "net.basov.omngo.TERMUX_RESULT";
    private static final String OMNGO_STREAM = "omngo_stream";
    private static final String OMNGO_LABEL = "omngo_label";
    private android.content.BroadcastReceiver termuxResultReceiver;
    // A distinct request code for each capture launch. The result
    // PendingIntents of two concurrent commands then do not collide under
    // FLAG_UPDATE_CURRENT.
    private int termuxResultRequestCounter = 5000;

    private void handleIntentUri(final String url) {
        if (!readConfigFlag("enable_intent_uri")) {
            showToast("Intent links are turned off. Enable them in Settings.");
            return;
        }

        final android.content.Intent intentApp;
        try {
            // parseUri() handles the bare "intent:#Intent;...;end" form and
            // the "intent://...#Intent;...;end" form. It also
            // percent-decodes a string extra value. A "%20" inside a
            // packed Termux argument thus becomes a space here, and the
            // packing convention below depends on that.
            intentApp = android.content.Intent.parseUri(url, android.content.Intent.URI_INTENT_SCHEME);
        } catch (Exception e) {
            e.printStackTrace();
            showToast("This intent link is malformed.");
            return;
        }

        // The presence of the RUN_COMMAND_PATH extra marks this as a Termux
        // "run a shell command" intent, and not an ordinary one. It is
        // checked first, because the output of a Termux command comes back
        // by a different mechanism than an activity result. That mechanism
        // is a future feature, and it is not the omngo_capture_extra path
        // below.
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

    // An ordinary intent, and not a Termux one. Hand it to the OS as an
    // activity, for example an android.settings.* screen or a third-party
    // app deep link. resolveActivity() is deliberately NOT used as a
    // pre-check here. Under API 30+ package visibility it can return null
    // even for an action that the system itself would handle, and some
    // android.settings.* screens are among those. A try and catch around
    // startActivity is the reliable form. This honors the standard
    // S.browser_fallback_url extra, which loads in the WebView, when no
    // installed app can handle the intent.
    /**
     * Lets a "file://" URI leave this process.
     *
     * An application that targets API 24 or later gets a VmPolicy with
     * penaltyDeathOnFileUriExposure, so startActivity() on an intent whose
     * data is a file:// URI throws FileUriExposedException before the other
     * application is ever asked. Clearing the policy is the documented way
     * out for an application that hands a real path to another one on
     * purpose, which is what both callers of this do.
     *
     * WHAT IT COSTS. StrictMode is a development aid, not a permission
     * boundary: nothing here widens what OMN-Go may read or write. It only
     * stops the platform from killing this process for passing a path. The
     * receiving application still needs its own storage permission to open
     * the file, and a file it may not read stays a file it may not read.
     *
     * The policy is process-wide and there is no per-call form of it, so
     * this is called at the point of use rather than at startup: on a run
     * that never opens an external editor and never follows a file: intent
     * link, the default policy stays in force.
     */
    static void allowFileUriHandoff() {
        android.os.StrictMode.setVmPolicy(new android.os.StrictMode.VmPolicy.Builder().build());
    }

    private void launchGenericIntent(final android.content.Intent intentApp) {
        String fallbackUrl = intentApp.getStringExtra("browser_fallback_url");
        // Do not leave the fallback URL in the extras of the launched
        // intent, where the target activity can misread it.
        intentApp.removeExtra("browser_fallback_url");
        // A note may point at a real path, for example a photo in DCIM or
        // a PDF in Documents. That is a file:// URI by the time parseUri is
        // finished with it. See allowFileUriHandoff for why this is
        // necessary and what it does not change.
        android.net.Uri data = intentApp.getData();
        if (data != null && "file".equals(data.getScheme())) {
            allowFileUriHandoff();
        }
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
        // Do not leak the private marker of OMN-Go to the target app.
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
    // from onActivityResult. Every "no result" path is handled with care,
    // and those are a cancel, no data, and an absent requested extra.
    // Android delivers this callback even when the launched activity set no
    // result at all, with resultCode == RESULT_CANCELED and data == null.
    // There is thus nothing to crash on. On success the text goes to
    // insertCapturedText, which shows it in a pre-filled review dialog and
    // does not save it silently.
    private void handleCaptureResult(int resultCode, android.content.Intent data) {
        final String extraName = pendingCaptureExtra;
        pendingCaptureExtra = null; // consume it either way
        if (extraName == null) {
            // A capture callback with no remembered extra name. The process
            // was killed, for example, and the onSaveInstanceState state was
            // not restored. Nothing actionable.
            return;
        }
        if (resultCode != RESULT_OK || data == null) {
            // User backed out of the scanner, or the activity returned
            // nothing. Deliberately silent, because a canceled scan is not
            // an error.
            return;
        }
        final String value = extractResultText(data, extraName);
        if (value == null || value.isEmpty()) {
            // The app returned OK but not the extra this note asked for.
            showToast("No \"" + extraName + "\" result was returned.");
            return;
        }
        // No label for a scan. The decoded text alone goes to the user for
        // review.
        insertCapturedText(value, null);
    }

    // Reads the named result extra out of a returned Intent. A barcode scan
    // returns a single String (SCAN_RESULT). The reader also accepts a
    // String-ArrayList extra, and it joins the entries of that list. The
    // same path thus works for any result-returning app, with no special
    // case and at no extra cost.
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

    // Receives a Termux command's result (see launchTermuxIntent's capture
    // path). intent carries our own OMNGO_STREAM / OMNGO_LABEL (from the
    // PendingIntent's base intent) plus Termux's result Bundle. Extracts the
    // requested stream(s) and hands the text to the same review dialog the
    // barcode path uses.
    private void handleTermuxResult(android.content.Intent intent) {
        if (intent == null) {
            return;
        }
        String stream = intent.getStringExtra(OMNGO_STREAM);
        String label = intent.getStringExtra(OMNGO_LABEL);
        android.os.Bundle result = intent.getBundleExtra(TERMUX_RESULT_BUNDLE);
        if (result == null) {
            showToast("Termux returned no result.");
            return;
        }
        String stdout = result.getString(TERMUX_RESULT_STDOUT);
        String stderr = result.getString(TERMUX_RESULT_STDERR);
        int exitCode = result.getInt(TERMUX_RESULT_EXIT_CODE, 0);
        String text = buildCaptureText(stream, stdout, stderr, exitCode);
        if (text == null || text.isEmpty()) {
            showToast("Command produced no output.");
            return;
        }
        insertCapturedText(text, (label != null && !label.isEmpty()) ? label : null);
    }

    // Assembles the text to paste from a Termux result according to the
    // requested stream ("stdout" default / "stderr" / "both"). An exit-code
    // line is appended only when the command failed (non-zero), so a normal
    // success stays clean.
    private String buildCaptureText(String stream, String stdout, String stderr, int exitCode) {
        if (stdout == null) stdout = "";
        if (stderr == null) stderr = "";
        StringBuilder sb = new StringBuilder();
        if ("stderr".equals(stream)) {
            sb.append(stderr);
        } else if ("both".equals(stream)) {
            sb.append(stdout);
            if (!stderr.isEmpty()) {
                if (sb.length() > 0) sb.append('\n');
                sb.append(stderr);
            }
        } else { // "stdout" (default)
            sb.append(stdout);
        }
        if (exitCode != 0) {
            if (sb.length() > 0) sb.append('\n');
            sb.append("exit code: ").append(exitCode);
        }
        return sb.toString().trim();
    }

    // Shared "paste a captured result" entry point for both the barcode
    // path (onActivityResult) and the Termux-output path (broadcast). It is
    // never silent. It pre-fills the in-app Quick Note panel
    // (window.omnGoInsertCapture) for the user to review and save. When
    // that panel is not available on the current page, it falls back to a
    // native dialog with the text pre-filled and editable. A page mid-edit
    // on editor.html is such a page, because it loads no omn-go-sse.js.
    private void insertCapturedText(final String text, final String label) {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                if (webView == null) {
                    showNativeCaptureDialog(text, label);
                    return;
                }
                // URI-encode the values, and decodeURIComponent them in JS.
                // The share handling of onNewIntent uses the same safe
                // transport. Arbitrary text, such as a quote or a newline,
                // then cannot break the JS string.
                String encText = android.net.Uri.encode(text != null ? text : "");
                String encLabel = android.net.Uri.encode(label != null ? label : "");
                String js = "(function(){ try{ return (typeof window.omnGoInsertCapture==='function' && "
                    + "window.omnGoInsertCapture(decodeURIComponent('" + encText + "'), "
                    + "decodeURIComponent('" + encLabel + "'))===true); }catch(e){ return false; } })();";
                webView.evaluateJavascript(js, new android.webkit.ValueCallback<String>() {
                    @Override
                    public void onReceiveValue(String value) {
                        // evaluateJavascript returns the JS value JSON-encoded,
                        // so a boolean true arrives as the literal "true".
                        if (!"true".equals(value)) {
                            showNativeCaptureDialog(text, label);
                        }
                    }
                });
            }
        });
    }

    // Native fallback review dialog: the captured text pre-filled in an
    // editable field, saved to Quick Notes only if the user confirms.
    private void showNativeCaptureDialog(final String text, final String label) {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                final android.widget.EditText input = new android.widget.EditText(MainActivity.this);
                String initial = (label != null && !label.isEmpty())
                    ? (label + "\n\n" + (text != null ? text : ""))
                    : (text != null ? text : "");
                input.setText(initial);
                new android.app.AlertDialog.Builder(MainActivity.this)
                    .setTitle("Add to Quick Notes")
                    .setView(input)
                    .setPositiveButton("Save", (d, w) -> {
                        final String note = input.getText().toString();
                        new Thread(new Runnable() {
                            @Override
                            public void run() {
                                try {
                                    postQuickNoteWithRetry("\n" + note + "\n");
                                    showToast("Added to Quick Notes");
                                    runOnUiThread(new Runnable() {
                                        @Override
                                        public void run() {
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
                                    showToast("Couldn't save to Quick Notes.");
                                }
                            }
                        }).start();
                    })
                    .setNegativeButton("Cancel", null)
                    .show();
            }
        });
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
            // Ask now. The user grants it and taps the link again. It stays
            // a re-tap, and it is not an auto-retry through
            // onRequestPermissionsResult. There is thus no cross-callback
            // state to hold for this rare path.
            requestPermissions(new String[]{ TERMUX_PERMISSION }, REQ_TERMUX_PERMISSION);
            showToast("Grant Termux the RUN_COMMAND permission, then tap the link again.");
            return;
        }

        // Old OMN packing convention. An intent URI cannot carry a String[]
        // extra, thus the arguments are packed into RUN_COMMAND_PATH as
        // "path?arg1&arg2&...". A space inside an argument is %20 there,
        // and parseUri above already decoded it. Unpack into the real
        // RUN_COMMAND_PATH and RUN_COMMAND_ARGUMENTS[] that Termux expects.
        //
        // The boundary between the path and the arguments is split on the
        // FIRST '?' only, with limit 2. That is a safe superset of the
        // unlimited split of old OMN. Every note that worked there had no
        // '?' inside an argument, or it was already broken. The result is
        // thus the same for an existing note, and it is correct as well
        // when an argument itself contains '?'. Arguments are still
        // separated on every '&', as old OMN does. An argument therefore
        // cannot contain a literal '&', which is a documented limitation of
        // the packing.
        String cmdPath = intentApp.getStringExtra(TERMUX_RUN_COMMAND_PATH);
        if (cmdPath != null && cmdPath.contains("?")) {
            String[] cmdParts = cmdPath.split("\\?", 2);
            intentApp.putExtra(TERMUX_RUN_COMMAND_PATH, cmdParts[0].trim());
            if (cmdParts.length > 1 && !cmdParts[1].isEmpty()) {
                intentApp.putExtra(TERMUX_RUN_COMMAND_ARGS, cmdParts[1].split("&"));
            }
        }

        // Opt-in output capture. When the note carries omngo_capture_output,
        // wire up a result PendingIntent. The stdout and stderr of the
        // command then come back to handleTermuxResult, and they are pasted
        // into the review dialog. Left alone, fire-and-forget as before,
        // when the marker is absent.
        if (intentApp.hasExtra(OMNGO_CAPTURE_OUTPUT)) {
            String stream = intentApp.getStringExtra(OMNGO_CAPTURE_OUTPUT);
            if (stream == null || stream.isEmpty()) {
                stream = "stdout"; // default stream when the value is bare
            }
            // Do not leak the private marker of OMN-Go to Termux.
            intentApp.removeExtra(OMNGO_CAPTURE_OUTPUT);
            // Separate stdout and stderr are captured in background mode
            // only. A capture URI that says nothing else defaults to
            // background, thus capture works with no extra word. The note
            // can still force a visible terminal session with
            // B.com.termux.RUN_COMMAND_BACKGROUND=false.
            if (!intentApp.hasExtra(TERMUX_RUN_COMMAND_BACKGROUND)) {
                intentApp.putExtra(TERMUX_RUN_COMMAND_BACKGROUND, true);
            }
            // Base intent for the own broadcast of OMN-Go. It carries the
            // stream selection and the label, thus handleTermuxResult gets
            // them back beside the result bundle of Termux. Termux adds
            // that bundle into this same intent, and the PendingIntent must
            // therefore be mutable on API 31+.
            android.content.Intent resultIntent = new android.content.Intent(ACTION_TERMUX_RESULT);
            resultIntent.setPackage(getPackageName());
            resultIntent.putExtra(OMNGO_STREAM, stream);
            String capLabel = intentApp.getStringExtra(TERMUX_RUN_COMMAND_LABEL);
            resultIntent.putExtra(OMNGO_LABEL, capLabel != null ? capLabel : "");
            int piFlags = android.app.PendingIntent.FLAG_UPDATE_CURRENT;
            if (android.os.Build.VERSION.SDK_INT >= 31) {
                piFlags |= android.app.PendingIntent.FLAG_MUTABLE;
            }
            android.app.PendingIntent resultPI = android.app.PendingIntent.getBroadcast(
                this, termuxResultRequestCounter++, resultIntent, piFlags);
            intentApp.putExtra(TERMUX_RUN_COMMAND_PENDING_INTENT, resultPI);
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

    // Starts com.termux/.app.RunCommandService. On API 26+ the service must
    // be started with startForegroundService(). RunCommandService promotes
    // itself to the foreground with a notification, and a plain
    // startService() there can throw once Termux calls startForeground
    // late. Below 26, use startService(). A SecurityException here almost
    // always means the allow-external-apps setting of Termux is not set,
    // thus the message points the user at it.
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

    // Reads a boolean flag out of config.json. The default is false when
    // the file or the key is missing or unreadable. Same native-read
    // approach as readMaxUploadSizeMB(). These Android-consumed toggles
    // never go through the Go HTTP server. A fresh read on each call means
    // that a Settings change applies on the next tap, with no app restart.
    private boolean readConfigFlag(String key) {
        return OmnConfig.flag(storageDir(), key);
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
    private void createNoteShortcut(final String name, final String title) {
        if (name == null || name.isEmpty()) return;
        final String label = (title != null && !title.isEmpty()) ? title : name;

        final android.graphics.Bitmap icon = renderDrawableToBitmap(
            R.drawable.ic_launcher_background, R.drawable.ic_launcher_shortcut_foreground);

        final android.content.Intent shortcutIntent = new android.content.Intent(this, MainActivity.class);
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

    // ----------------------------------------------------------------------
    // Sending a note out (note exchange, phase 4)
    // ----------------------------------------------------------------------
    //
    // The share sheet is the whole feature. It reaches Telegram, e-mail,
    // LocalSend, Bluetooth, Nearby Share and every other application on the
    // device, so OMN-Go integrates with none of them by name.
    //
    // The bytes come from the SERVER, and not from the note file.
    // /api/export/note adds the "FileName:" line that carries the path of
    // the note. That rule lives in Go, thus the desktop and this side
    // cannot disagree about what a sent note looks like. See
    // backend/note_exchange.go. The endpoint is admin-only, and a local
    // connection bypasses that. This request comes from 127.0.0.1, which IS
    // the device.

    /**
     * Answers "omngo://share?name=<note>" and "…&as=text".
     *
     * The work is on a thread because it is an HTTP request; the chooser goes
     * back to the UI thread in startShareChooser.
     */
    private void handleShareOut(final android.net.Uri request) {
        final String note = request.getQueryParameter("name");
        final boolean asText = "text".equals(request.getQueryParameter("as"));
        if (note == null || note.isEmpty()) {
            showToast("No note to send.");
            return;
        }

        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    java.net.URL url = new java.net.URL(serverBase() + "/api/export/note?name="
                            + java.net.URLEncoder.encode(note, "UTF-8"));
                    java.net.HttpURLConnection conn =
                            (java.net.HttpURLConnection) url.openConnection();
                    String body;
                    String filename;
                    String description;
                    try {
                        conn.setRequestMethod("GET");
                        int code = conn.getResponseCode();
                        if (code != 200) {
                            throw new java.io.IOException("the server answered HTTP " + code);
                        }
                        filename = exportFilename(conn.getHeaderField("Content-Disposition"));
                        description = exportDescription(conn.getHeaderField("X-OMN-Description"));
                        body = readAllUtf8(conn.getInputStream());
                    } finally {
                        conn.disconnect();
                    }

                    if (asText) {
                        android.content.Intent send =
                                new android.content.Intent(android.content.Intent.ACTION_SEND);
                        send.setType("text/plain");
                        // No description extra here: a text send carries the
                        // whole note, and the description block is inside it.
                        send.putExtra(android.content.Intent.EXTRA_TEXT, body);
                        send.putExtra(android.content.Intent.EXTRA_SUBJECT, note);
                        startShareChooser(send, "Send note as text");
                        return;
                    }
                    startShareChooser(shareFileIntent(filename, body, description), "Send note");
                } catch (Exception e) {
                    e.printStackTrace();
                    showToast("Could not send the note: " + e.getMessage());
                }
            }
        }).start();
    }

    /**
     * Writes the note into the export cache and builds the ACTION_SEND for it.
     *
     * The read grant is what lets the chosen application open the URI:
     * ExportProvider is exported="false", so nothing reaches it without one.
     * ClipData carries the same URI because several applications take the
     * grant from there rather than from EXTRA_STREAM.
     */
    private android.content.Intent shareFileIntent(String filename, String body, String description)
            throws java.io.IOException {
        java.io.File dir = ExportProvider.exportDir(this);

        // Sweep by age, and not by "everything but this one". A share that
        // is still in flight has not been read yet. Some applications open
        // the URI when the user presses Send, and not when the sheet
        // appears. A delete of its file would then send an empty
        // attachment. An hour is long after any of them has finished, and
        // it is short enough that the cache cannot grow without bound.
        java.io.File[] previous = dir.listFiles();
        if (previous != null) {
            long cutoff = System.currentTimeMillis() - 60L * 60L * 1000L;
            for (java.io.File old : previous) {
                if (old.lastModified() < cutoff) {
                    old.delete();
                }
            }
        }

        java.io.File out = new java.io.File(dir, filename);
        java.io.OutputStream os = new java.io.FileOutputStream(out);
        try {
            os.write(body.getBytes("UTF-8"));
        } finally {
            os.close();
        }

        android.net.Uri uri = ExportProvider.uriFor(this, out);
        android.content.Intent send =
                new android.content.Intent(android.content.Intent.ACTION_SEND);
        send.setType("text/markdown");
        send.putExtra(android.content.Intent.EXTRA_STREAM, uri);
        send.putExtra(android.content.Intent.EXTRA_SUBJECT, filename);
        // The description block of the note, as the MESSAGE that goes with
        // the file. Telegram makes it the caption, and a mail client makes
        // it the body. An application that has no place for text beside a
        // file ignores the extra. This is thus safe to send to every target
        // in the sheet, and not to a chosen few.
        //
        // Only when the note HAS one. An empty EXTRA_TEXT is not the same
        // as no EXTRA_TEXT. Some clients open an empty message body and
        // wait, where they would attach and send without the extra.
        if (description != null && !description.isEmpty()) {
            send.putExtra(android.content.Intent.EXTRA_TEXT, description);
        }
        send.addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION);
        send.setClipData(android.content.ClipData.newUri(getContentResolver(), filename, uri));
        return send;
    }

    /**
     * Decodes the X-OMN-Description header (see headerDescription in
     * backend/note_exchange.go).
     *
     * Base64 of UTF-8, because the description is a paragraph: it can hold a
     * newline, which would end the header field, and it can hold Cyrillic or
     * an accented letter, which an HTTP header field cannot carry as it
     * stands.
     *
     * A damaged or absent header is not an error. The note goes without a
     * message rather than not at all.
     */
    private String exportDescription(String header) {
        if (header == null || header.isEmpty()) {
            return "";
        }
        try {
            byte[] raw = android.util.Base64.decode(header, android.util.Base64.DEFAULT);
            return new String(raw, "UTF-8");
        } catch (Exception e) {
            e.printStackTrace();
            return "";
        }
    }

    /** Shows the chooser. Called from a worker thread, so it hops back. */
    private void startShareChooser(final android.content.Intent send, final String title) {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                try {
                    android.content.Intent chooser =
                            android.content.Intent.createChooser(send, title);
                    // The chooser is the activity that starts next, so the
                    // grant has to be on it as well as on the intent inside.
                    chooser.addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION);
                    startActivity(chooser);
                } catch (Exception e) {
                    e.printStackTrace();
                    showToast("No application on this device can send a note.");
                }
            }
        });
    }

    /**
     * The file name for the export, from the server's Content-Disposition.
     *
     * Sanitized even though OMN-Go's own server wrote it: this string becomes
     * a path under the cache directory, and a name that arrives over a socket
     * is not a name to join onto a path untouched. flattenExportName in
     * backend/note_exchange.go already restricts it to the same set, so a
     * well-formed answer passes through unchanged.
     */
    private String exportFilename(String contentDisposition) {
        String raw = "";
        if (contentDisposition != null) {
            int at = contentDisposition.indexOf("filename=\"");
            if (at >= 0) {
                int end = contentDisposition.indexOf('"', at + 10);
                if (end > at) {
                    raw = contentDisposition.substring(at + 10, end);
                }
            }
        }
        StringBuilder kept = new StringBuilder();
        for (int i = 0; i < raw.length(); i++) {
            char c = raw.charAt(i);
            boolean ok = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
                    || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-';
            if (ok) {
                kept.append(c);
            }
        }
        String name = kept.toString();
        while (name.startsWith(".")) {
            name = name.substring(1);
        }
        if (name.isEmpty()) {
            name = "note.md";
        }
        if (!name.toLowerCase(java.util.Locale.ROOT).endsWith(".md")) {
            name = name + ".md";
        }
        return name;
    }

    /** Reads a whole response as UTF-8. A note is small enough to hold. */
    private String readAllUtf8(java.io.InputStream in) throws java.io.IOException {
        java.io.ByteArrayOutputStream buffer = new java.io.ByteArrayOutputStream();
        byte[] chunk = new byte[8192];
        int read;
        while ((read = in.read(chunk)) != -1) {
            buffer.write(chunk, 0, read);
        }
        return new String(buffer.toByteArray(), "UTF-8");
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
