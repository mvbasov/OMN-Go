package net.basov.omngo;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebView;
import net.basov.omngo.backend.Backend;

// MainActivity is the one screen of the application. It holds the methods
// of the life cycle, and it reads each intent that starts or wakes it. Each
// other task is in a class of its own:
//
//   WebViewSetup  The WebView, the spinner, and the address of each link.
//   Fullscreen    The system bars.
//   ShareIn       A note, an image or a JSON file from another application.
//   ShareOut      A note to another application.
//   IntentBridge  An "intent:" link, a capture result and Termux.
//   Shortcuts     A note on the home screen.
//   OmnText       The text rules, which a plain JVM can test.
//   OmnConfig     The reader of config.json, which a plain JVM can test.
//
// A member that a second class reads is not private. Each class is in one
// package, and nothing outside the package reads them.
public class MainActivity extends Activity {
    WebView webView;
    // The text under the spinner of the start screen. WebViewSetup makes it,
    // showStartStage writes it, and the first loaded page hides it.
    android.widget.TextView startText;
    // The name of the note that the external editor has open. WebViewSetup
    // sets it for an "omngo://edit" address, and onActivityResult reads it.
    String currentEditingName;

    // The request code of the external editor. See WebViewSetup and
    // onActivityResult.
    static final int REQ_EDIT = 1001;

    final ShareIn shareIn = new ShareIn(this);
    final ShareOut shareOut = new ShareOut(this);
    final IntentBridge intents = new IntentBridge(this);
    final Shortcuts shortcuts = new Shortcuts(this);

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

    String storageDir() {
        return ServerService.storageDir(this);
    }

    String serverBase() {
        return "http://127.0.0.1:" + ServerService.serverPort(this);
    }

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

        // Before each other step, because onActivityResult can fire early.
        intents.restoreState(savedInstanceState);

        shortcuts.register();
        intents.register();

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

        // The layout: the WebView and the spinner above it.
        WebViewSetup.build(this);

        // Applied before the first frame is drawn. A user who turned
        // fullscreen off then does not see the hidden status bar of the
        // manifest theme appear and slide back in.
        Fullscreen.applyFullscreenMode(this);

        // ServerService starts the Go server on a worker thread. The main
        // thread thus draws the spinner of WebViewSetup at once, also while
        // the first start of a new version writes and compiles its files.
        // This thread waits until the server listens, and then the main
        // thread loads the start page. While it waits, it reads the work of
        // the start two times per second, and the text under the spinner
        // shows it. The time limit only guards against a server that never
        // starts: the load then shows the error.
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    long deadline = System.currentTimeMillis() + BACKEND_START_TIMEOUT_MS;
                    boolean returned = false;
                    while (!returned && System.currentTimeMillis() < deadline) {
                        returned = ServerService.awaitBackendReturn(START_STAGE_POLL_MS);
                        showStartStage(Backend.startStage());
                    }
                    if (returned) {
                        Backend.waitUntilReady();
                    }
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                }
                runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        if (!isFinishing() && !isDestroyed()) {
                            showStartStage("Opening the page");
                            loadStartPage();
                        }
                    }
                });
            }
        }, "omn-go-wait").start();
    }

    // The longest wait for the Go server. See onCreate.
    private static final long BACKEND_START_TIMEOUT_MS = 120000;

    // The time between two reads of the start stage: two reads per second.
    private static final long START_STAGE_POLL_MS = 500;

    // showStartStage writes the work of the start under the spinner. Any
    // thread can call it. An empty stage shows "Starting". WebViewSetup
    // hides the text when the first page is loaded.
    void showStartStage(final String stage) {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                if (startText != null) {
                    startText.setText(stage == null || stage.isEmpty() ? "Starting" : stage);
                }
            }
        });
    }

    // loadStartPage loads the first page of this start. It runs on the main
    // thread, after the Go server listens.
    private void loadStartPage() {
        String startUrl = serverBase() + "/Welcome.html";
        android.content.Intent intent = getIntent();
        String shortcutNote = intent.getStringExtra(Shortcuts.EXTRA_SHORTCUT_NOTE);
        if (shortcutNote != null && !shortcutNote.isEmpty()) {
            // Tapped a pinned shortcut (see Shortcuts) -
            // go straight to that note instead of Welcome.html.
            startUrl = serverBase() + "/" + android.net.Uri.encode(shortcutNote) + ".html";
        } else if (isQuickNoteAliasLaunch(intent)) {
            // Tapped the second "OMN-Go Quick Note" app-drawer icon.
            // See the QuickNoteAlias activity-alias in the manifest.
            // It still loads Welcome.html, thus the app has a normal
            // page underneath. It adds a query flag, and the load
            // handler of omn-go-core.js reads that flag and pops the
            // Quick Note panel open at once. The share_text and
            // share_subject flags below do the same for shared text.
            startUrl += "?quicknote=1";
        } else if (shareIn.isSharedNoteIntent(intent)) {
            // A note arrived as a FILE. Imported natively, like the
            // image/JSON branch below - startUrl stays Welcome.html,
            // and openImportedNote takes the WebView to the note when
            // the import answers.
            shareIn.importSharedNote(shareIn.sharedNoteUri(intent));
        } else if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())
                && intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM) == null) {
            String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
            String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
            if (OmnText.looksLikeSharedNote(sharedText)) {
                // A note sent AS TEXT. It does not go through the
                // URL. A whole note is the wrong size for a query
                // string, and the note box is not where it belongs.
                shareIn.importSharedText(sharedText);
            } else {
                startUrl += "?share_text=" + (sharedText != null ? android.net.Uri.encode(sharedText) : "") +
                            "&share_subject=" + (sharedSubject != null ? android.net.Uri.encode(sharedSubject) : "");
            }
        } else if (shareIn.isSharedFileIntent(intent)) {
            // Handled entirely natively (see handleSharedFile) -
            // startUrl is deliberately left alone; this cold start
            // still lands on Welcome.html like any other launch.
            android.net.Uri sharedUri = (android.net.Uri) intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM);
            if (sharedUri != null) {
                shareIn.handleSharedFile(sharedUri, intent.getType());
            }
        }

        // An update of the application writes the shipped scripts and
        // style sheets again (storage.RefreshEmbeddedAssets in
        // backend/internal/storage/assets.go). The WebView can hold the
        // previous copy of those files in its disk cache, and then some
        // new pages do not operate correctly. The Go side tells if it
        // wrote such a file at this start. The app thus clears the
        // cache one time, at the first start of a new version.
        //
        // The server listens at this point, and Backend.startServer
        // completed the work with the assets before that. See
        // initStorage in backend/internal/app/storage.go.
        if (!assetCacheCleared && Backend.assetsRefreshed()) {
            assetCacheCleared = true;
            webView.clearCache(true);
        }

        webView.loadUrl(startUrl);
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, android.content.Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == IntentBridge.REQ_CAPTURE_RESULT) {
            // Result from a capture launch (e.g. a barcode scan) - paste it
            // into Quick Notes. See handleCaptureResult for all the "no
            // result" cases, which are handled gracefully.
            intents.handleCaptureResult(resultCode, data);
            return;
        }
        if (requestCode == REQ_EDIT && webView != null) {
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
        // onCreate gives the state back with intents.restoreState.
        intents.saveState(outState);
    }

    @Override
    protected void onNewIntent(android.content.Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        String shortcutNote = intent.getStringExtra(Shortcuts.EXTRA_SHORTCUT_NOTE);
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
            // shared-text branch below avoids for a warm start. omnShowPanel
            // of omn-go-core.js opens the panel. A page without that
            // function or without the panel, for example editor.html, does
            // nothing.
            if (webView != null) {
                webView.evaluateJavascript(
                    "(function(){ if (typeof window.omnShowPanel==='function') window.omnShowPanel('omn-quick-panel'); })();",
                    null);
            }
        } else if (shareIn.isSharedNoteIntent(intent)) {
            // A note as a FILE, warm. Native, like the image/JSON branch
            // below. openImportedNote decides whether the WebView may move.
            shareIn.importSharedNote(shareIn.sharedNoteUri(intent));
        } else if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())
                && intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM) == null
                && OmnText.looksLikeSharedNote(intent.getStringExtra(android.content.Intent.EXTRA_TEXT))) {
            // A note as TEXT, warm. Straight to the importer, and never
            // through window.handleShare. That call fills the note box for
            // review, which is right for a thought. It is wrong for a note
            // that already has a name and a home.
            shareIn.importSharedText(intent.getStringExtra(android.content.Intent.EXTRA_TEXT));
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
        } else if (shareIn.isSharedFileIntent(intent)) {
            // Unlike the text/plain branch above, this never touches
            // webView at all. See the banner of ShareIn
            // for why a warm-start share cannot safely
            // assume anything about what the WebView shows at that moment.
            // It could be mid-edit of another note on editor.html, which
            // does not even define window.handleShare.
            android.net.Uri sharedUri = (android.net.Uri) intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM);
            if (sharedUri != null) {
                shareIn.handleSharedFile(sharedUri, intent.getType());
            }
        }
    }

    // The page gets the Back key first. omnCloseTopLayer of omn-go-core.js
    // closes the top layer (a dialog, the search overlay or a panel) and
    // answers true. With no open layer it answers false, and the WebView goes
    // back. The editor page has only the dialog part, in omn-go-dialog.js.
    private static final String CLOSE_TOP_LAYER_JS =
        "(function(){ try{ return typeof window.omnCloseTopLayer==='function' && "
        + "window.omnCloseTopLayer()===true; }catch(e){ return false; } })();";

    @Override
    public void onBackPressed() {
        if (webView == null) {
            super.onBackPressed();
            return;
        }
        webView.evaluateJavascript(CLOSE_TOP_LAYER_JS, value -> {
            if ("true".equals(value)) {
                return;
            }
            if (webView.canGoBack()) {
                webView.goBack();
            } else {
                super.onBackPressed();
            }
        });
    }

    @Override
    protected void onResume() {
        super.onResume();
        // Covers coming back from another app, and a mode change made on
        // another device that arrived via git sync.
        Fullscreen.applyFullscreenMode(this);
    }

    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        // A return of the focus clears the hidden-bar state on some builds.
        // That happens when the keyboard closes, when a dialog closes, and
        // when the Termux confirmation returns. Assert the state again, and
        // do not fall back to a half-visible bar.
        if (hasFocus) Fullscreen.applyFullscreenMode(this);
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        shortcuts.unregister();
        intents.unregister();
    }

    // ----------------------------------------------------------------------
    // The readers of config.json
    // ----------------------------------------------------------------------
    //
    // Each call of OmnConfig is in this file.
    // TestAndroidConfigCallSitesTypeCheck in
    // backend/internal/repocheck/java_test.go reads the calls from here and
    // gives them to javac.

    // OmnConfig.fullscreenMode mirrors config.NormalizeFullscreen in
    // backend/internal/config/config.go. An unknown or absent value means
    // "fullscreen", thus a config.json written before this setting existed
    // keeps the behavior that install already had. See the banner of OmnConfig.
    String readFullscreenMode() {
        return OmnConfig.fullscreenMode(storageDir());
    }

    // Reads max_upload_size_mb straight out of config.json. ShareIn writes
    // the shared file directly to disk, and it does not use /api/upload
    // or /api/upload_json of the Go server. It thus cannot use
    // a.maxUploadBytes() on the server. It repeats the same default,
    // config.DefaultMaxUploadSizeMB in backend/internal/config/config.go, when
    // config.json is missing or unreadable.
    int readMaxUploadSizeMB() {
        return OmnConfig.maxUploadMB(storageDir());
    }

    // Reads a boolean flag out of config.json. The default is false when
    // the file or the key is missing or unreadable. Same native-read
    // approach as readMaxUploadSizeMB(). These Android-consumed toggles
    // never go through the Go HTTP server. A fresh read on each call means
    // that a Settings change applies on the next tap, with no app restart.
    boolean readConfigFlag(String key) {
        return OmnConfig.flag(storageDir(), key);
    }

    void showToast(final String msg) {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                android.widget.Toast.makeText(MainActivity.this, msg, android.widget.Toast.LENGTH_LONG).show();
            }
        });
    }
}
