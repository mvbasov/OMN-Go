package net.basov.omngo;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import net.basov.omngo.backend.Backend;
import android.os.Handler;
import android.os.Looper;

public class MainActivity extends Activity {
    private WebView webView;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // Ensure Android OS mounts scoped storage directories for native C/Go access
        java.io.File[] mediaDirs = getExternalMediaDirs();
        if (mediaDirs != null && mediaDirs.length > 0 && mediaDirs[0] != null) {
            mediaDirs[0].mkdirs();
        }

        // Start the Go Backend Server from the gomobile .aar
        Backend.startServer();

        // Initialize WebView
        webView = new WebView(this);
        WebSettings webSettings = webView.getSettings();
        webSettings.setJavaScriptEnabled(true);
        webSettings.setDomStorageEnabled(true);
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                if (url != null && url.startsWith("omngo://edit")) {
                    try {
                        String name = url.substring(url.indexOf("?name=") + 6);
                        if (name.contains("&")) {
                            name = name.split("&")[0];
                        }
                        
                        // Disable strict mode exposed file exceptions
                        android.os.StrictMode.VmPolicy.Builder builder = new android.os.StrictMode.VmPolicy.Builder();
                        android.os.StrictMode.setVmPolicy(builder.build());

                        java.io.File file = new java.io.File("/storage/emulated/0/Android/media/net.basov.omngo/md/" + name + ".md");
                        if (!file.exists()) {
                            file.getParentFile().mkdirs();
                            file.createNewFile();
                        }

                        android.content.Intent intent = new android.content.Intent(android.content.Intent.ACTION_EDIT);
                        intent.setDataAndType(android.net.Uri.fromFile(file), "text/plain");
                        intent.addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION | android.content.Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
                        
                        view.getContext().startActivity(android.content.Intent.createChooser(intent, "Edit Markdown File"));
                    } catch (Exception e) {
                        e.printStackTrace();
                    }
                    return true;
                }

                if (url != null && (url.startsWith("http://") || url.startsWith("https://"))) {
                    if (!url.contains("localhost")) {
                        view.getContext().startActivity(
                            new android.content.Intent(android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url))
                        );
                        return true;
                    }
                }
                return false;
            }
        });

        setContentView(webView);

        // Wait for the Go server to bind before loading
        new Handler(Looper.getMainLooper()).postDelayed(new Runnable() {
            @Override
            public void run() {
                webView.loadUrl("http://localhost:8080");
            }
        }, 1000); // 1 second delay
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
