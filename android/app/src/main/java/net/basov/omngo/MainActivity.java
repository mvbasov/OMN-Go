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

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // The Go server (plus its wake lock and the scoped-storage dir
        // setup) is owned by ServerService, a foreground service with a
        // persistent notification. Starting it from the Activity used to
        // tie the server's fate to the Activity's process priority: as
        // soon as this Activity left the screen (home, split-screen focus
        // loss, screen lock), the process fell into the cached bucket and
        // Android 12+'s cached-app freezer froze it - the "server only
        // answers while the app is visible" bug. See ServerService for
        // the full explanation.

        // Android 13+ requires runtime consent for the service's
        // notification to be visible. The service runs either way; this
        // just makes its notification (and Stop button) show up.
        if (android.os.Build.VERSION.SDK_INT >= 33 &&
                checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS)
                        != android.content.pm.PackageManager.PERMISSION_GRANTED) {
            requestPermissions(
                new String[]{ android.Manifest.permission.POST_NOTIFICATIONS }, 1002);
        }

        android.content.Intent svcIntent = new android.content.Intent(this, ServerService.class);
        if (android.os.Build.VERSION.SDK_INT >= 26) {
            startForegroundService(svcIntent);
        } else {
            startService(svcIntent);
        }

        // Deep Doze (long screen-off periods) suspends network for apps
        // regardless of wake locks; the battery-optimization exemption is
        // what keeps the server reachable over LAN with the screen locked.
        // Asked at most once - if the user declines, they can still grant
        // it later via system Settings > Battery.
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

                        // Determine correct subdirectory and extension
                        java.io.File file;
                        if (name.endsWith(".md")) {
                            file = new java.io.File("/storage/emulated/0/Android/media/net.basov.omngo/md/" + name);
                        } else {
                            file = new java.io.File("/storage/emulated/0/Android/media/net.basov.omngo/html/" + name);
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
                String startUrl = "http://127.0.0.1:8080/Welcome.html";
                android.content.Intent intent = getIntent();
                if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())) {
                    String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
                    String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
                    startUrl += "?share_text=" + (sharedText != null ? android.net.Uri.encode(sharedText) : "") + 
                                "&share_subject=" + (sharedSubject != null ? android.net.Uri.encode(sharedSubject) : "");
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
                webView.loadUrl("http://127.0.0.1:8080/" + android.net.Uri.encode(baseName) + ".html");
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
        if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())) {
            String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
            String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
            if (webView != null) {
                String tText = sharedText != null ? android.net.Uri.encode(sharedText) : "";
                String tSubj = sharedSubject != null ? android.net.Uri.encode(sharedSubject) : "";
                String js = "javascript:(function(){ if(window.handleShare) window.handleShare(decodeURIComponent('" + tText + "'), decodeURIComponent('" + tSubj + "')); })();";
                webView.evaluateJavascript(js, null);
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
}
