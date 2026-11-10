package net.basov.omngo;

import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;

// WebViewSetup builds the one screen of the application: the WebView and
// the spinner above it. It also gives each address that is not a page of
// the Go server to the part that owns it. See shouldOverrideUrlLoading.
//
// It has one method and no state, thus the method is static.
final class WebViewSetup {

    private WebViewSetup() {
    }

    // build makes the layout, sets a.webView and shows the layout.
    // MainActivity.onCreate calls it one time.
    static void build(final MainActivity a) {
        // Create Native Loading Layout
        android.widget.FrameLayout rootLayout = new android.widget.FrameLayout(a);
        // The color of omn_start_background in res/values/themes.xml, the
        // window background before this layout.
        rootLayout.setBackgroundColor(android.graphics.Color.parseColor("#f9f9f9"));

        final android.widget.ProgressBar progressBar = new android.widget.ProgressBar(a);
        android.widget.FrameLayout.LayoutParams pbParams = new android.widget.FrameLayout.LayoutParams(
            android.view.ViewGroup.LayoutParams.WRAP_CONTENT,
            android.view.ViewGroup.LayoutParams.WRAP_CONTENT);
        pbParams.gravity = android.view.Gravity.CENTER;
        progressBar.setLayoutParams(pbParams);

        // The work of the start, under the spinner. See
        // MainActivity.showStartStage. The spinner stays at the center,
        // because each later page load shows it there with no text.
        float dp = a.getResources().getDisplayMetrics().density;
        a.startText = new android.widget.TextView(a);
        a.startText.setText("Starting");
        a.startText.setTextColor(android.graphics.Color.parseColor("#666666"));
        a.startText.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 14);
        a.startText.setGravity(android.view.Gravity.CENTER);
        a.startText.setPadding((int) (24 * dp), 0, (int) (24 * dp), 0);
        android.widget.FrameLayout.LayoutParams textParams = new android.widget.FrameLayout.LayoutParams(
            android.view.ViewGroup.LayoutParams.MATCH_PARENT,
            android.view.ViewGroup.LayoutParams.WRAP_CONTENT);
        textParams.gravity = android.view.Gravity.CENTER;
        textParams.topMargin = (int) (64 * dp);
        a.startText.setLayoutParams(textParams);

        // Initialize WebView
        a.webView = new WebView(a);
        a.webView.setLayoutParams(new android.widget.FrameLayout.LayoutParams(
            android.view.ViewGroup.LayoutParams.MATCH_PARENT,
            android.view.ViewGroup.LayoutParams.MATCH_PARENT));

        WebSettings webSettings = a.webView.getSettings();
        webSettings.setJavaScriptEnabled(true);
        webSettings.setDomStorageEnabled(true);

        a.webView.setWebChromeClient(new android.webkit.WebChromeClient() {
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

        a.webView.setWebViewClient(new WebViewClient() {
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
                a.startText.setVisibility(android.view.View.GONE);
                // A save on the Config page reloads it. That is what makes
                // a "Fullscreen mode" change apply at once, and not at the
                // next app start.
                Fullscreen.applyFullscreenMode(a);
                super.onPageFinished(view, url);
            }
            // A failed load may never reach onPageFinished, and the spinner
            // would then stay on screen. Clear it here too.
            @Override
            public void onReceivedError(WebView view, android.webkit.WebResourceRequest request,
                                        android.webkit.WebResourceError error) {
                progressBar.setVisibility(android.view.View.GONE);
                a.startText.setVisibility(android.view.View.GONE);
                super.onReceivedError(view, request, error);
            }
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                // Sending a note out. The frontend's Send control navigates
                // here (see omnGoSendNote in omn-go-share.js) because a
                // WebView cannot open a share sheet by itself.
                if (url != null && url.startsWith("omngo://share")) {
                    a.shareOut.handleShareOut(android.net.Uri.parse(url));
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
                        a.shortcuts.createNoteShortcut(name, title);
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
                        a.currentEditingName = name;

                        IntentBridge.allowFileUriHandoff();

                        // Determine correct subdirectory and extension.
                        java.io.File file;
                        String editStorageDir = a.storageDir();
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

                        a.startActivityForResult(android.content.Intent.createChooser(intent, "Edit Markdown File"), MainActivity.REQ_EDIT);
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
                    // See handleIntentUri() and the banner of IntentBridge.
                    // The generic "any other scheme" branch below thus needs
                    // no intent:// special-case of its own.
                    a.intents.handleIntentUri(url);
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
        rootLayout.addView(a.webView);
        rootLayout.addView(progressBar);
        rootLayout.addView(a.startText);
        a.setContentView(rootLayout);
    }
}
