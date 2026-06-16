import os

def build_project():
    print("[*] Initializing OMN-Go V1.2.0 Project Overhaul...")

    # Define target files and their absolute contents
    files = {}

    # 1. SETTINGS.GRADLE
    files["android/settings.gradle"] = """rootProject.name = "OMN-Go"
include ':app'
"""

    # 2. APP BUILD.GRADLE
    files["android/app/build.gradle"] = """plugins {
    id 'com.android.application'
}

android {
    namespace 'net.basov.goomn'
    compileSdk 34

    defaultConfig {
        applicationId "net.basov.goomn"
        minSdk 24
        targetSdk 34
        versionCode 10200
        versionName "1.2.0"
    }

    signingConfigs {
        release {
            storeFile file('goomn.keystore')
            storePassword 'goomn123'
            keyAlias 'goomn'
            keyPassword 'goomn123'
        }
    }
    buildTypes {
        release {
            signingConfig signingConfigs.release
            minifyEnabled false
            proguardFiles getDefaultProguardFile('proguard-android-optimize.txt'), 'proguard-rules.pro'
        }
    }
    compileOptions {
        sourceCompatibility JavaVersion.VERSION_17
        targetCompatibility JavaVersion.VERSION_17
    }
}

dependencies {
    implementation fileTree(dir: 'libs', include: ['*.jar', '*.aar'])
}
"""

    # 3. ANDROID MANIFEST
    files["android/app/src/main/AndroidManifest.xml"] = """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="net.basov.goomn">
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" />
    <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" />
    
    <application
        android:allowBackup="true"
        android:label="OMN-Go"
        android:usesCleartextTraffic="true"
        android:hardwareAccelerated="true"
        android:theme="@android:style/Theme.NoTitleBar.Fullscreen">
        <activity
            android:name=".MainActivity"
            android:exported="true"
            android:configChanges="orientation|keyboardHidden|screenSize">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
"""

    # 4. MAIN ACTIVITY JAVA
    files["android/app/src/main/java/net/basov/goomn/MainActivity.java"] = """package net.basov.goomn;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import net.basov.goomn.backend.Backend;
import android.os.Handler;
import android.os.Looper;

public class MainActivity extends Activity {
    private WebView webView;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

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

                        java.io.File file = new java.io.File("/storage/emulated/0/Android/media/net.basov.goomn/md/" + name + ".md");
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
"""

    # 5. DOCKERFILE
    files["Dockerfile"] = """# STAGE 1: Toolchains & Cache
FROM golang:1.25-bookworm AS builder
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \\
    openjdk-17-jdk wget unzip cmake ninja-build \\
    && rm -rf /var/lib/apt/lists/*

# Install Android CMD Line Tools
RUN wget https://dl.google.com/android/repository/commandlinetools-linux-10406996_latest.zip -O /tmp/cmd.zip && \\
    mkdir -p /opt/android/cmdline-tools && \\
    unzip /tmp/cmd.zip -d /opt/android/cmdline-tools && \\
    mv /opt/android/cmdline-tools/cmdline-tools /opt/android/cmdline-tools/latest && \\
    rm /tmp/cmd.zip

ENV ANDROID_HOME=/opt/android
ENV PATH=$PATH:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools

# Accept licenses and install platform dependencies
RUN yes | sdkmanager --licenses && \\
    sdkmanager "platforms;android-34" "build-tools;33.0.2" "ndk;25.2.9519653"

# Install Gradle
RUN wget -q https://services.gradle.org/distributions/gradle-8.5-bin.zip -O /tmp/gradle.zip && \\
    mkdir -p /opt/gradle && \\
    unzip -q /tmp/gradle.zip -d /opt/gradle && \\
    rm /tmp/gradle.zip
ENV PATH=$PATH:/opt/gradle/gradle-8.5/bin

# Install GoMobile
RUN go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init

# STAGE 2: Dependency Lock
WORKDIR /app
COPY go.mod ./
RUN go mod download || true

# STAGE 3: Build & Pack
COPY . .
RUN go get golang.org/x/mobile@latest && go mod tidy

# Desktop Binary (OMN-Go naming convention)
RUN GOOS=linux GOARCH=amd64 go build -o bin/omn-go-desktop main_desktop.go

# Android APK - Webview Wrapper via Gradle & gomobile bind (strictly zero AndroidX/AppCompat)
RUN go get -tool golang.org/x/mobile/cmd/gobind && go mod tidy && mkdir -p android/app/libs && gomobile bind -target=android -androidapi 24 -javapkg net.basov.goomn -o android/app/libs/goomn.aar ./backend

RUN cd android && if [ ! -f app/goomn.keystore ]; then keytool -genkey -v -keystore app/goomn.keystore -alias goomn -keyalg RSA -keysize 2048 -validity 10000 -storepass goomn123 -keypass goomn123 -dname "CN=OMN-Go, O=Basov"; fi && gradle assembleRelease && cp app/build/outputs/apk/release/app-release.apk ../bin/omn-go.apk
"""

    # 6. EMBEDDED LAYOUT TEMPLATE (index.html)
    files["backend/frontend/index.html"] = """<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title><!-- OMN_GO_PAGE_TITLE --></title>
    <style>
        body { font-family: sans-serif; margin: 0; padding: 0; display: flex; flex-direction: column; height: 100vh; background: #f9f9f9; color: #333; }
        .overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.7); display: flex; align-items: center; justify-content: center; z-index: 50; }
        .modal { background: #fff; padding: 20px; border-radius: 4px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); width: 300px; }
        .modal input, .modal button, .modal textarea { width: 100%; box-sizing: border-box; margin-bottom: 10px; padding: 8px; }
        .modal button { background: #0056b3; color: white; border: none; cursor: pointer; border-radius: 4px; }
        #mainUI { display: flex; flex: 1; flex-direction: column; }
        .header { background: #333; color: #fff; padding: 10px 20px; display: flex; gap: 15px; align-items: center; }
        .header a, .header button { color: #fff; text-decoration: none; cursor: pointer; background: transparent; border: 1px solid #555; padding: 5px 10px; border-radius: 4px; font-size: 14px; }
        .header a:hover, .header button:hover { background: #555; }
        .content-area { flex: 1; padding: 20px; position: relative; display: flex; flex-direction: column; }
        #editor { display: none; width: 100%; flex: 1; border: 1px solid #ccc; padding: 10px; font-family: monospace; resize: none; box-sizing: border-box; }
        #preview { width: 100%; flex: 1; background: #fff; border: 1px solid #ccc; padding: 20px; overflow-y: auto; box-sizing: border-box; line-height: 1.6; }
        .toolbar { display: flex; justify-content: flex-end; margin-bottom: 10px; gap: 10px; }
        .toolbar button { padding: 5px 15px; cursor: pointer; border: 1px solid #ccc; background: #eee; border-radius: 4px; }
        .hidden { display: none !important; }
        .panel { position: absolute; top: 50px; right: 20px; background: white; border: 1px solid #ccc; padding: 15px; box-shadow: 0 4px 8px rgba(0,0,0,0.1); width: 300px; z-index: 40; }
        .panel h3 { margin-top: 0; }
        .panel input, .panel textarea, .panel button { width: 100%; box-sizing: border-box; margin-bottom: 10px; padding: 8px; }
        .panel button { background: #28a745; color: white; border: none; cursor: pointer; border-radius: 4px; }
    </style>
    <link rel="stylesheet" href="/css/highlight.default.min.css">
    <link rel="stylesheet" href="/css/katex.min.css">
</head>
<body>
    
    <!-- Login Overlay -->
    <div id="loginOverlay" class="overlay" style="display: none;">
        <div class="modal">
            <h2>OMN-Go Login</h2>
            <input type="password" id="pwdInput" placeholder="Admin or Guest Password">
            <button onclick="login()">Enter</button>
        </div>
    </div>

    <!-- Main UI -->
    <div id="mainUI">
        <div class="header">
            <strong>OMN-Go</strong>
            <a href="/Welcome.html">Home</a>
            <a href="/Welcome.html#help">Help</a>
            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only">Quick Note</button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only">Add Bookmark</button>
            <a href="/Bookmarks.html">Bookmarks</a>
            <a href="/Config.html" style="background: #444; border-color: #666;">Config</a>
        </div>

        <div class="content-area">
            <div class="toolbar">
                <button id="metaToggleBtn" onclick="document.getElementById('metadataPanel').classList.toggle('hidden')" style="display: none; background: #17a2b8; color: white; border: none;">Metadata</button>
                <button id="saveBtn" onclick="saveNote()" class="admin-only" style="display: none; background: #28a745; color: white; border: none;">Save Note</button>
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only">Edit Mode</button>
            </div>
            <div id="metadataPanel" class="hidden" style="background: #e9ecef; padding: 15px; font-family: monospace; white-space: pre-wrap; border: 1px solid #ccc; margin-bottom: 10px; border-radius: 4px; font-size: 13px;"><!-- OMN_GO_METADATA_PANEL --></div>
            <textarea id="editor" class="admin-only" placeholder="Markdown/Code content... Drag images here to upload."><!-- OMN_GO_RAW_MD --></textarea>
            <div id="preview"><!-- OMN_GO_PREVIEW_BODY --></div>
        </div>
    </div>

    <!-- Quick Note Modal -->
    <div id="quickPanel" class="panel hidden">
        <h3>Quick Note</h3>
        <textarea id="quickText" rows="4"></textarea>
        <div style="display: flex; gap: 10px;">
            <button onclick="submitQuickNote()">Save</button>
            <button onclick="document.getElementById('quickPanel').classList.add('hidden')" style="background: #dc3545;">Cancel</button>
        </div>
    </div>

    <!-- Bookmark Modal -->
    <div id="bmPanel" class="panel hidden">
        <h3>Ingest Bookmark</h3>
        <input id="bmUrl" placeholder="URL">
        <input id="bmTitle" placeholder="Title">
        <input id="bmTags" placeholder="Tags (comma separated)">
        <textarea id="bmNotes" rows="2" placeholder="Notes"></textarea>
        <div style="display: flex; gap: 10px;">
            <button onclick="submitBookmark()">Save</button>
            <button onclick="document.getElementById('bmPanel').classList.add('hidden')" style="background: #dc3545;">Cancel</button>
        </div>
    </div>

    <script>
        /* OMN_GO_PAGE_NAME_JS */
        const APP_VERSION = "1.2.0";

        function executeScripts(container) {
            const scripts = container.querySelectorAll('script');
            scripts.forEach(oldScript => {
                const newScript = document.createElement('script');
                Array.from(oldScript.attributes).forEach(attr => newScript.setAttribute(attr.name, attr.value));
                newScript.async = false;
                if (oldScript.innerHTML) newScript.appendChild(document.createTextNode(oldScript.innerHTML));
                oldScript.parentNode.replaceChild(newScript, oldScript);
            });
        }

        // Intercept Markdown links for standard browser-side redirects
        document.getElementById('preview').addEventListener('click', (e) => {
            let target = e.target.closest('a');
            if(target) {
                const href = target.getAttribute('href');
                if (href) {
                    if (href.startsWith('http')) {
                        e.preventDefault();
                        window.open(href, '_blank');
                    } else if (!href.startsWith('javascript:') && !href.startsWith('#')) {
                        e.preventDefault();
                        let cleanHref = href;
                        if (cleanHref.endsWith('.md')) {
                            cleanHref = cleanHref.substring(0, cleanHref.length - 3) + '.html';
                        } else if (!cleanHref.includes('.')) {
                            cleanHref = cleanHref + '.html';
                        }
                        window.location.href = cleanHref;
                    }
                }
            }
        });

        let currentMode = 'view';
        async function toggleMode() {
            try {
                const res = await fetch('/api/config');
                if (res.ok) {
                    const config = await res.json();
                    if (!config.use_internal_editor) {
                        window.location.href = '/api/edit-external?name=' + encodeURIComponent(currentNote);
                        return;
                    }
                }
            } catch(e) { console.error(e); }

            const editor = document.getElementById('editor');
            const preview = document.getElementById('preview');
            const btn = document.getElementById('toggleBtn');
            
            if(currentMode === 'view') {
                editor.style.display = 'block';
                preview.style.display = 'none';
                btn.innerText = 'View Mode';
                document.getElementById('saveBtn').style.display = 'block';
                document.getElementById('metaToggleBtn').style.display = 'none';
                document.getElementById('metadataPanel').classList.add('hidden');
                currentMode = 'edit';
            } else {
                editor.style.display = 'none';
                preview.style.display = 'block';
                btn.innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                currentMode = 'view';
            }
        }

        // Image Drag & Drop
        const editor = document.getElementById('editor');
        editor.addEventListener('dragover', e => e.preventDefault());
        editor.addEventListener('drop', async e => {
            e.preventDefault();
            if(e.dataTransfer.files.length > 0) {
                const fd = new FormData();
                fd.append('image', e.dataTransfer.files[0]);
                const res = await fetch('/api/upload', { method: 'POST', body: fd });
                if(res.ok) {
                    const text = await res.text();
                    const cursor = editor.selectionStart;
                    editor.value = editor.value.substring(0, cursor) + text + editor.value.substring(cursor);
                    editor.dispatchEvent(new Event('input'));
                }
            }
        });

        async function login() {
            const pwd = document.getElementById('pwdInput').value;
            const res = await fetch('/login', {
                method: 'POST',
                headers: {'Content-Type': 'application/x-www-form-urlencoded'},
                body: 'password=' + encodeURIComponent(pwd)
            });
            if(res.ok) {
                document.getElementById('loginOverlay').style.display = 'none';
                document.getElementById('mainUI').style.display = 'flex';
                checkRole();
            } else {
                alert('Invalid Password');
            }
        }

        function checkRole() {
            if(document.cookie.includes('session_role=guest')) {
                document.querySelectorAll('.admin-only').forEach(el => {
                    if(el.tagName === 'BUTTON' || el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') el.disabled = true;
                    if(el.id === 'toggleBtn' || el.id === 'editor' || el.id === 'saveBtn') el.style.display = 'none';
                });
            }
        }

        async function saveNote() {
            let content = document.getElementById('editor').value;
            const fd = new URLSearchParams();
            fd.append('name', currentNote);
            fd.append('content', content);
            const res = await fetch('/api/save', { method: 'POST', body: fd });
            if(res.ok) {
                alert('Note saved!');
                window.location.reload();
            } else {
                alert('Failed to save!');
            }
        }

        async function submitQuickNote() {
            const fd = new URLSearchParams();
            fd.append('note', document.getElementById('quickText').value);
            const res = await fetch('/api/quick', { method: 'POST', body: fd });
            if(res.ok) {
                document.getElementById('quickText').value = '';
                document.getElementById('quickPanel').classList.add('hidden');
                alert('Saved!');
                window.location.reload();
            }
        }

        async function submitBookmark() {
            const fd = new URLSearchParams();
            fd.append('url', document.getElementById('bmUrl').value);
            fd.append('title', document.getElementById('bmTitle').value);
            fd.append('tags', document.getElementById('bmTags').value);
            fd.append('notes', document.getElementById('bmNotes').value);
            const res = await fetch('/api/bookmark', { method: 'POST', body: fd });
            if(res.ok) {
                document.getElementById('bmPanel').classList.add('hidden');
                document.querySelectorAll('#bmPanel input, #bmPanel textarea').forEach(el => el.value = '');
                alert('Saved!');
                window.location.reload();
            }
        }

        async function checkSession() {
            // Unhide UI if role cookies exist
            if (document.cookie.includes('session_role=')) {
                document.getElementById('loginOverlay').style.display = 'none';
                document.getElementById('mainUI').style.display = 'flex';
                checkRole();
            } else {
                // Check if server is configured with public role or check backend
                const test = await fetch('/api/config');
                if (test.status === 401) {
                    document.getElementById('loginOverlay').style.display = 'flex';
                    document.getElementById('mainUI').style.display = 'none';
                } else {
                    document.getElementById('loginOverlay').style.display = 'none';
                    document.getElementById('mainUI').style.display = 'flex';
                }
            }
        }

        window.onload = () => {
            checkSession();
            if (window.hljs) {
                document.querySelectorAll('#preview pre code').forEach((block) => {
                    hljs.highlightElement(block);
                });
            }
            let hash = window.location.hash;
            if (hash) {
                let el = document.getElementById(hash.substring(1));
                if (el) el.scrollIntoView();
            }
        };
    </script>

    <!-- Code & Math Formatting Assets -->
    <script src="/js/highlight.min.js"></script>
    <script src="/js/katex.min.js"></script>
    <script src="/js/auto-render.min.js"></script>
    <script>
        document.addEventListener("DOMContentLoaded", () => {
            // Setup Auto-Rendering for KaTeX via MutationObserver
            const previewNode = document.getElementById('preview') || document.body;
            let renderTimeout;
            const observer = new MutationObserver(() => {
                clearTimeout(renderTimeout);
                renderTimeout = setTimeout(() => {
                    if (window.renderMathInElement) {
                        renderMathInElement(previewNode, {
                            delimiters: [
                                {left: '$$', right: '$$', display: true},
                                {left: '$', right: '$', display: false},
                                {left: '\\(', right: '\\)', display: false},
                                {left: '\\[', right: '\\]', display: true}
                            ],
                            throwOnError: false
                        });
                    }
                }, 50);
            });
            observer.observe(previewNode, { childList: true, subtree: true });
        });
    </script>

    <!-- Small Version Footer -->
    <div id="goomn-version-footer" style="position: fixed; bottom: 4px; right: 8px; font-size: 0.75rem; color: #888; z-index: 9999; opacity: 0.7; pointer-events: none;"></div>
    <script>
        document.addEventListener("DOMContentLoaded", () => {
            const footer = document.getElementById('goomn-version-footer');
            let v = '1.2.0';
            try { if (APP_VERSION) v = APP_VERSION; } catch(e) {}
            if (footer) footer.innerText = 'OMN-Go v' + v;
        });
    </script>

    <!-- JS Console Interceptor & UI -->
    <script>
        (function() {
            const originalLog = console.log;
            const originalError = console.error;
            const originalWarn = console.warn;
            const originalInfo = console.info;

            let logs = [];
            let consoleBtn = null;
            let consoleModal = null;
            let logsContainer = null;

            function initConsoleUI() {
                if (consoleBtn) return;

                consoleModal = document.createElement('div');
                consoleModal.id = 'goomn-console-modal';
                consoleModal.style.cssText = 'display:none; position:fixed; top:10%; left:10%; width:80%; height:80%; background:#1e1e1e; color:#00ff00; z-index:10000; border:2px solid #555; border-radius:8px; flex-direction:column; font-family:monospace; box-shadow: 0 4px 12px rgba(0,0,0,0.5);';

                const header = document.createElement('div');
                header.style.cssText = 'padding:10px; background:#333; color:#fff; display:flex; justify-content:space-between; align-items:center; border-bottom:1px solid #555; font-weight:bold;';
                header.innerHTML = '<span>JS Console Output</span><div><button id="goomn-console-clear" style="background:#888; color:white; border:none; border-radius:4px; padding:4px 12px; cursor:pointer; margin-right:8px;">Clear</button><button id="goomn-console-close" style="background:#ff5555; color:white; border:none; border-radius:4px; padding:4px 12px; cursor:pointer;">Close</button></div>';

                logsContainer = document.createElement('div');
                logsContainer.style.cssText = 'flex:1; overflow-y:auto; padding:10px; white-space:pre-wrap; word-break:break-all; font-size:12px; line-height:1.4;';

                consoleModal.appendChild(header);
                consoleModal.appendChild(logsContainer);
                document.body.appendChild(consoleModal);

                document.getElementById('goomn-console-close').onclick = () => {
                    consoleModal.style.display = 'none';
                };
                let clrBtn = document.getElementById('goomn-console-clear');
                if (clrBtn) {
                    clrBtn.onclick = () => {
                        logs = [];
                        if (logsContainer) logsContainer.innerHTML = '';
                        if (consoleBtn) consoleBtn.innerText = 'Console (0)';
                    };
                }

                consoleBtn = document.createElement('button');
                consoleBtn.id = 'goomn-console-btn';
                consoleBtn.innerText = 'Console (0)';
                consoleBtn.style.cssText = 'margin-left:8px; padding:4px 8px; background:#ff9800; color:#fff; border:none; border-radius:4px; cursor:pointer; font-size:0.8rem; font-weight:bold;';
                consoleBtn.onclick = () => {
                    consoleModal.style.display = 'flex';
                };

                let metadataEl = Array.from(document.querySelectorAll('*')).find(el => {
                    if (el.children.length > 0) return false;
                    const text = (el.textContent || '').toLowerCase();
                    const id = (el.id || '').toLowerCase();
                    const cls = (el.className || '').toLowerCase();
                    return text.includes('metadata') || id.includes('metadata') || cls.includes('metadata');
                });

                if (metadataEl && metadataEl.parentNode) {
                    metadataEl.parentNode.insertBefore(consoleBtn, metadataEl.nextSibling);
                } else {
                    consoleBtn.style.position = 'fixed';
                    consoleBtn.style.bottom = '4px';
                    consoleBtn.style.left = '8px';
                    consoleBtn.style.zIndex = '9999';
                    document.body.appendChild(consoleBtn);
                }
            }

            function appendLog(type, args) {
                logs.push({type, args});
                if (!document.body) {
                    window.addEventListener('DOMContentLoaded', () => appendLog(type, args));
                    return;
                }
                if (!consoleBtn) initConsoleUI();
                consoleBtn.innerText = `Console (${logs.length})`;

                if (logsContainer) {
                    const msg = document.createElement('div');
                    msg.style.marginBottom = '4px';
                    msg.style.paddingBottom = '4px';
                    msg.style.borderBottom = '1px solid #333';
                    const color = type === 'error' ? '#ff5555' : type === 'warn' ? '#ffb86c' : '#f8f8f2';
                    msg.style.color = color;

                    const text = Array.from(args).map(a => {
                        try { return typeof a === 'object' ? JSON.stringify(a) : String(a); }
                        catch(e) { return String(a); }
                    }).join(' ');
                    
                    msg.textContent = `[${type.toUpperCase()}] ${text}`;
                    logsContainer.appendChild(msg);
                    logsContainer.scrollTop = logsContainer.scrollHeight;
                }
            }

            console.log = function(...args) {
                originalLog.apply(console, args);
                appendLog('log', args);
            };
            console.error = function(...args) {
                originalError.apply(console, args);
                appendLog('error', args);
            };
            console.warn = function(...args) {
                originalWarn.apply(console, args);
                appendLog('warn', args);
            };
            console.info = function(...args) {
                originalInfo.apply(console, args);
                appendLog('info', args);
            };
            window.addEventListener('error', function(e) {
                console.error('Uncaught Error:', e.message, 'at', e.filename, ':', e.lineno);
            });
        })();
    </script>
</body>
</html>
"""

    # 7. BACKEND SERVER.GO
    files["backend/server.go"] = """package backend

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const APP_VERSION = "1.2.0"

type Config struct {
	ServerPort    int    `json:"server_port"`
	AdminPassword string `json:"admin_password"`
	GuestPassword string `json:"guest_password"`
	UseInternalEd bool   `json:"use_internal_editor"`
	DesktopExtCmd string `json:"desktop_ext_cmd"`
}

//go:embed frontend/index.html
var frontendHTML []byte

//go:embed frontend/js frontend/css frontend/json frontend/md
var staticFS embed.FS

var (
	storageDir  string
	appConfig   Config
	activeConns int
)

func initStorage() {
	if runtime.GOOS == "android" {
		storageDir = "/storage/emulated/0/Android/media/net.basov.goomn"
	} else {
		storageDir = "./data"
	}

	// 1. Create Isolated Storage
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	mdDir := filepath.Join(storageDir, "md")
	os.MkdirAll(mdDir, 0755)

	htmlDir := filepath.Join(storageDir, "html")
	os.MkdirAll(htmlDir, 0755)

	// Migrate legacy root md files recursively
	files, _ := filepath.Glob(filepath.Join(storageDir, "*.md"))
	for _, f := range files {
		os.Rename(f, filepath.Join(mdDir, filepath.Base(f)))
	}

	// 2. Init Config
	configPath := filepath.Join(storageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		appConfig = Config{
			ServerPort:    8080,
			AdminPassword: "admin_secret_changeme",
			GuestPassword: "guest_secret_changeme",
			UseInternalEd: true,
			DesktopExtCmd: "subl",
		}
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(configPath, data, 0644)
	} else {
		data, _ := os.ReadFile(configPath)
		json.Unmarshal(data, &appConfig)
	}

	// 3. Init Default Notes from embedFS
	initDefaultPage := func(fileName, defaultContent string) {
		p := filepath.Join(mdDir, fileName)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			data, err := staticFS.ReadFile("frontend/md/" + fileName)
			if err == nil {
				os.WriteFile(p, data, 0644)
			} else {
				os.WriteFile(p, []byte(defaultContent), 0644)
			}
		}
	}

	initDefaultPage("Welcome.md", "Title: Welcome\\nDate: 2026-06-14 12:00:00\\nCategory: System\\n\\nWelcome to OMN-Go! Start editing.\\n\\n- [Help](Welcome)\\n- [Scripting Rules](ScriptRules.md)\\n- [Bookmarks](Bookmarks)\\n- [Quick Notes](QuickNotes)")
	initDefaultPage("ScriptRules.md", "Title: JS Scripting Rules\\nDate: 2026-06-15\\nCategory: System\\n\\n# JavaScript Guidelines for OMN-Go\\n\\nBecause OMN-Go is rendered server-side, keep scripts wrapped in block scopes.")
	initDefaultPage("QuickNotes.md", "Title: Quick Notes\\nDate: 2026-06-14 12:00:00\\nCategory: Log\\n\\n")
	initDefaultPage("Bookmarks.md", "Title: Incoming bookmarks\\nDate: 2026-06-15 20:00:00\\nAuthor: \\nTags: Bookmarks\\n\\n<script>bookmarks = [\\n<!-- Don't edit body below this line -->\\n];\\n</script>")

	// Precompile all notes to data/html/ at startup
	precompileAllPages()
}

func renderMarkdownToHTML(mdContent []byte) string {
	lines := strings.Split(string(mdContent), "\\n")
	var html strings.Builder
	inList := false
	inCodeBlock := false
	var codeLang string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code blocks
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				html.WriteString("</code></pre>\\n")
				inCodeBlock = false
			} else {
				codeLang = strings.TrimPrefix(trimmed, "```")
				if codeLang == "" {
					codeLang = "plaintext"
				}
				html.WriteString(fmt.Sprintf("<pre><code class=\\"language-%s\\">", codeLang))
				inCodeBlock = true
			}
			continue
		}

		if inCodeBlock {
			escaped := htmlEscape(line)
			html.WriteString(escaped + "\\n")
			continue
		}

		// Lists
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				html.WriteString("<ul>\\n")
				inList = true
			}
			content := trimmed[2:]
			html.WriteString(fmt.Sprintf("<li>%s</li>\\n", renderInlineMarkdown(content)))
			continue
		} else {
			if inList {
				html.WriteString("</ul>\\n")
				inList = false
			}
		}

		// Headings
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			if level > 0 && level < len(trimmed) && trimmed[level] == ' ' {
				content := trimmed[level+1:]
				id := strings.ToLower(strings.Join(strings.Fields(content), "-"))
				id = strings.Map(func(r rune) rune {
					if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
						return r
					}
					return -1
				}, id)
				html.WriteString(fmt.Sprintf("<h%d id=\\"%s\\">%s</h%d>\\n", level, id, renderInlineMarkdown(content), level))
				continue
			}
		}

		// Blank lines
		if trimmed == "" {
			html.WriteString("<br/>\\n")
			continue
		}

		// Horizontal rule
		if trimmed == "---" {
			html.WriteString("<hr/>\\n")
			continue
		}

		// Regular paragraph
		html.WriteString(fmt.Sprintf("<p>%s</p>\\n", renderInlineMarkdown(line)))
	}

	if inList {
		html.WriteString("</ul>\\n")
	}
	if inCodeBlock {
		html.WriteString("</code></pre>\\n")
	}

	return html.String()
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\\"", "&quot;")
	return s
}

func renderInlineMarkdown(s string) string {
	// Bold (**text** or __text__)
	s = regexp.MustCompile(`\\*\\*(.*?)\\*\\*`).ReplaceAllString(s, "<strong>$1</strong>")
	s = regexp.MustCompile(`__(.*?)__`).ReplaceAllString(s, "<strong>$1</strong>")

	// Italics (*text* or _text_)
	s = regexp.MustCompile(`\\*(.*?)\\*`).ReplaceAllString(s, "<em>$1</em>")
	s = regexp.MustCompile(`_(.*?)_`).ReplaceAllString(s, "<em>$1</em>")

	// Inline Code (`code`)
	s = regexp.MustCompile("`(.*?)`").ReplaceAllString(s, "<code>$1</code>")

	// Links [label](url)
	s = regexp.MustCompile(`\\[(.*?)\\]\\((.*?)\\)`).ReplaceAllStringFunc(s, func(m string) string {
		parts := regexp.MustCompile(`\\[(.*?)\\]\\((.*?)\\)`).FindStringSubmatch(m)
		if len(parts) == 3 {
			label := parts[1]
			url := parts[2]
			if strings.HasSuffix(url, ".md") {
				url = strings.TrimSuffix(url, ".md") + ".html"
			} else if !strings.Contains(url, ".") && !strings.HasPrefix(url, "http") && !strings.HasPrefix(url, "#") {
				url = url + ".html"
			}
			return fmt.Sprintf(`<a href="%s">%s</a>`, url, label)
		}
		return m
	})

	return s
}

func compilePage(name string, mdContent []byte) []byte {
	var headers []string
	var bodyLines []string
	inHeader := true

	lines := strings.Split(string(mdContent), "\\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inHeader {
			if trimmed == "" {
				inHeader = false
				continue
			}
			if strings.Contains(line, ":") {
				headers = append(headers, line)
			} else {
				inHeader = false
				bodyLines = append(bodyLines, line)
			}
		} else {
			bodyLines = append(bodyLines, line)
		}
	}

	renderedBody := renderMarkdownToHTML([]byte(strings.Join(bodyLines, "\\n")))
	metadataStr := strings.Join(headers, "\\n")

	layout := string(frontendHTML)

	title := "OMN-Go - " + name
	for _, h := range headers {
		if strings.HasPrefix(h, "Title:") {
			title = strings.TrimSpace(strings.TrimPrefix(h, "Title:"))
			break
		}
	}

	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PAGE_TITLE -->", title)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PREVIEW_BODY -->", renderedBody)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_RAW_MD -->", string(mdContent))
	layout = strings.ReplaceAll(layout, "/* OMN_GO_PAGE_NAME_JS */", fmt.Sprintf(`let currentNote = "%s";`, name))
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", metadataStr)

	return []byte(layout)
}

func precompileAllPages() {
	mdDir := filepath.Join(storageDir, "md")
	htmlDir := filepath.Join(storageDir, "html")
	os.MkdirAll(htmlDir, 0755)

	files, _ := filepath.Glob(filepath.Join(mdDir, "*.md"))
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err == nil {
			name := strings.TrimSuffix(filepath.Base(f), ".md")
			compiled := compilePage(name, content)
			htmlPath := filepath.Join(htmlDir, name+".html")
			os.WriteFile(htmlPath, compiled, 0644)
		}
	}
}

func getConfigPageBody() string {
	return fmt.Sprintf(`
<div style="max-width: 600px; margin: 0 auto; background: #ffffff; padding: 30px; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); border: 1px solid #e1e4e8;">
    <h2 style="margin-top: 0; color: #1a1a1a; font-size: 24px; font-weight: 700; border-bottom: 2px solid #eaecef; padding-bottom: 10px;">Configuration Dashboard</h2>
    <form id="configForm" onsubmit="saveConfig(event)" style="margin-top: 20px;">
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Server Port</label>
            <input type="number" id="cfgPort" value="%d" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" required />
        </div>
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Admin Password</label>
            <input type="password" id="cfgAdminPwd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" required />
        </div>
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Guest Password</label>
            <input type="password" id="cfgGuestPwd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" required />
        </div>
        <div style="margin-bottom: 20px; display: flex; align-items: center; gap: 10px;">
            <input type="checkbox" id="cfgUseInternal" %s style="width: 20px; height: 20px; cursor: pointer;" />
            <label for="cfgUseInternal" style="font-weight: 600; color: #444; cursor: pointer;">Use HTML Internal Editor</label>
        </div>
        <div style="margin-bottom: 25px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Desktop External Editor Command</label>
            <input type="text" id="cfgExtCmd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
            <small style="color: #666; display: block; margin-top: 5px;">Example: <code>subl</code> or <code>code</code> or <code>nano</code></small>
        </div>
        <button type="submit" style="background: #28a745; color: white; border: none; padding: 12px 20px; border-radius: 4px; font-weight: bold; cursor: pointer; width: 100%%; font-size: 16px; transition: background 0.2s;">Save Configuration</button>
    </form>
</div>
<script>
    async function saveConfig(event) {
        event.preventDefault();
        const params = new URLSearchParams();
        params.append("server_port", document.getElementById("cfgPort").value);
        params.append("admin_password", document.getElementById("cfgAdminPwd").value);
        params.append("guest_password", document.getElementById("cfgGuestPwd").value);
        params.append("use_internal_editor", document.getElementById("cfgUseInternal").checked ? "true" : "false");
        params.append("desktop_ext_cmd", document.getElementById("cfgExtCmd").value);

        const res = await fetch("/api/config", { method: "POST", body: params });
        if (res.ok) {
            alert("Configuration saved successfully! Server port changes will take effect after restarting the application.");
            window.location.reload();
        } else {
            alert("Failed to save configuration.");
        }
    }
</script>
`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword,
		func() string { if appConfig.UseInternalEd { return "checked" }; return "" }(),
		appConfig.DesktopExtCmd)
}

func getExternalEditPageBody(name string) string {
	return fmt.Sprintf(`
<div style="max-width: 600px; margin: 40px auto; background: #ffffff; padding: 40px; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); border: 1px solid #e1e4e8; text-align: center;">
    <div style="font-size: 48px; margin-bottom: 20px;">📝</div>
    <h2 style="margin-top: 0; color: #1a1a1a; font-size: 24px; font-weight: 700;">Editing Externally</h2>
    <p style="color: #555; font-size: 16px; margin-bottom: 30px; line-height: 1.5;">
        We have launched <strong>%s</strong> to edit <code>%s.md</code>. Please complete your changes in your editor, save the file, and click the button below to view the updated page.
    </p>
    <button onclick="window.location.href='/%s.html'" style="background: #0056b3; color: white; border: none; padding: 15px 30px; border-radius: 6px; font-weight: bold; cursor: pointer; font-size: 18px; transition: background 0.2s; box-shadow: 0 2px 5px rgba(0,0,0,0.2);">
        Press after edit to refresh view
    </button>
</div>
`, appConfig.DesktopExtCmd, name, name)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(appConfig)
		return
	}
	if r.Method == "POST" {
		portStr := r.FormValue("server_port")
		var port int
		fmt.Sscanf(portStr, "%d", &port)
		if port > 0 {
			appConfig.ServerPort = port
		}
		appConfig.AdminPassword = r.FormValue("admin_password")
		appConfig.GuestPassword = r.FormValue("guest_password")
		appConfig.UseInternalEd = r.FormValue("use_internal_editor") == "true"
		appConfig.DesktopExtCmd = r.FormValue("desktop_ext_cmd")

		data, _ := json.MarshalIndent(appConfig, "", "  ")
		configPath := filepath.Join(storageDir, "config.json")
		os.WriteFile(configPath, data, 0644)
		w.Write([]byte("Saved"))
		return
	}
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

func handleEditExternal(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Missing name", http.StatusBadRequest)
		return
	}

	if runtime.GOOS == "android" {
		w.Header().Set("Location", "omngo://edit?name="+name)
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	cleanName := strings.TrimSuffix(name, ".html")
	if !strings.HasSuffix(cleanName, ".md") {
		cleanName += ".md"
	}
	filePath := filepath.Join(storageDir, "md", cleanName)

	cmd := exec.Command(appConfig.DesktopExtCmd, filePath)
	err := cmd.Start()
	if err != nil {
		log.Printf("Failed to run external editor: %v", err)
	}

	w.Header().Set("Content-Type", "text/html")
	pageName := strings.TrimSuffix(cleanName, ".md")
	compiledWait := compilePage(pageName, []byte(fmt.Sprintf("Title: Refresh %s\\nDate: %s\\nCategory: Action\\n\\n", pageName, time.Now().Format("2006-01-02 15:04:05"))))
	
	waitBody := getExternalEditPageBody(pageName)
	htmlStr := strings.Replace(string(compiledWait), "<!-- OMN_GO_PREVIEW_BODY -->", waitBody, 1)
	w.Write([]byte(htmlStr))
}

// Simple connection tracker for Android WebView synchronization
func connectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activeConns++
		next.ServeHTTP(w, r)
		activeConns--
	})
}

func authMiddleware(next http.HandlerFunc, requireAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_role")
		if err != nil || (requireAdmin && cookie.Value != "admin") || (!requireAdmin && cookie.Value != "admin" && cookie.Value != "guest") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	pwd := r.FormValue("password")
	role := ""
	if pwd == appConfig.AdminPassword {
		role = "admin"
	} else if pwd == appConfig.GuestPassword {
		role = "guest"
	}

	if role != "" {
		http.SetCookie(w, &http.Cookie{Name: "session_role", Value: role, Path: "/"})
		w.Write([]byte("OK"))
	} else {
		http.Error(w, "Invalid", http.StatusUnauthorized)
	}
}

func handleQuickNote(w http.ResponseWriter, r *http.Request) {
	note := r.FormValue("note")
	if note == "" {
		return
	}
	path := filepath.Join(storageDir, "md", "QuickNotes.md")
	data, _ := os.ReadFile(path)
	lines := strings.Split(string(data), "\\n")
	
	insertIdx := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" { // Find first blank line ending Pelican header
			insertIdx = i + 1
			break
		}
	}
	
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("\\n---\\n##### %s\\n%s\\n", timestamp, note)
	
	newContent := append(lines[:insertIdx], append([]string{entry}, lines[insertIdx:]...)...)
	fullMarkdown := strings.Join(newContent, "\\n")
	os.WriteFile(path, []byte(fullMarkdown), 0644)

	// Update Dynamic Precompile instantly
	compiled := compilePage("QuickNotes", []byte(fullMarkdown))
	os.WriteFile(filepath.Join(storageDir, "html", "QuickNotes.html"), compiled, 0644)

	w.Write([]byte("Saved"))
}

func handleBookmark(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	notes := r.FormValue("notes")
	
	path := filepath.Join(storageDir, "md", "Bookmarks.md")
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	
	tagsList := []string{}
	for _, t := range strings.Split(tags, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tagsList = append(tagsList, trimmed)
		}
	}
	notesList := []string{}
	if trimmed := strings.TrimSpace(notes); trimmed != "" {
		notesList = append(notesList, trimmed)
	}
	
	type BM struct {
		Date  string   `json:"date"`
		Url   string   `json:"url"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
		Notes []string `json:"notes"`
	}
	bm := BM{Date: timestamp, Url: url, Title: title, Tags: tagsList, Notes: notesList}
	bmJson, _ := json.MarshalIndent(bm, "  ", "  ")
	entry := "  " + string(bmJson) + ",\\n"
	
	data, err := os.ReadFile(path)
	if err == nil {
		content := string(data)
		marker := "<!-- Don't edit body below this line -->"
		if strings.Contains(content, marker) {
			newContent := strings.Replace(content, marker, marker+"\\n"+entry, 1)
			os.WriteFile(path, []byte(newContent), 0644)
			// Update Dynamic Precompile instantly
			compiled := compilePage("Bookmarks", []byte(newContent))
			os.WriteFile(filepath.Join(storageDir, "html", "Bookmarks.html"), compiled, 0644)
		}
	}
	w.Write([]byte("Saved"))
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()

	imgDir := filepath.Join(storageDir, "images")
	os.MkdirAll(imgDir, 0755)
	
	destPath := filepath.Join(imgDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)
	
	w.Write([]byte(fmt.Sprintf("![%s]({filename}/images/%s)", header.Filename, header.Filename)))
}

func handleUploadJSON(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()

	jsonDir := filepath.Join(storageDir, "user_json")
	os.MkdirAll(jsonDir, 0755)
	
	destPath := filepath.Join(jsonDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)
	
	w.Write([]byte(fmt.Sprintf("[%s]({filename}/user_json/%s)", header.Filename, header.Filename)))
}

func handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Welcome"
	}
	
	var path string
	var data []byte
	var err error

	if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {
		cleanName := strings.TrimSuffix(name, ".html")
		if !strings.HasSuffix(cleanName, ".md") {
			cleanName += ".md"
		}
		path = filepath.Join(storageDir, "md", filepath.Clean(cleanName))
		data, err = os.ReadFile(path)
		if err != nil {
			embedPath := "frontend/md/" + cleanName
			data, err = staticFS.ReadFile(embedPath)
			if err != nil {
				title := strings.TrimSuffix(cleanName, ".md")
				timestamp := time.Now().Format("2006-01-02 15:04:05")
				newContent := fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes\\n\\n# %s\\n\\nStart editing this page!", title, timestamp, title)
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte(newContent), 0644)
				data = []byte(newContent)
			} else {
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, data, 0644)
			}
		}
	} else {
		path = filepath.Join(storageDir, filepath.Clean(name))
		data, err = os.ReadFile(path)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
	}
	w.Write(data)
}

func handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		return
	}

	var path string
	if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {
		cleanName := strings.TrimSuffix(name, ".html")
		if !strings.HasSuffix(cleanName, ".md") {
			cleanName += ".md"
		}
		path = filepath.Join(storageDir, "md", filepath.Clean(cleanName))
		
		// Pelican Header modification logic
		parts := strings.Split(content, "\\n\\n")
		if len(parts) > 0 && strings.Contains(parts[0], ":") {
			headerLines := strings.Split(parts[0], "\\n")
			modIdx := -1
			for i, l := range headerLines {
				if strings.HasPrefix(l, "Modified:") {
					modIdx = i
					break
				}
			}
			now := time.Now().Format("2006-01-02 15:04:05")
			if modIdx != -1 {
				headerLines[modIdx] = fmt.Sprintf("Modified: %s", now)
			} else {
				headerLines = append(headerLines, fmt.Sprintf("Modified: %s", now))
			}
			parts[0] = strings.Join(headerLines, "\\n")
			content = strings.Join(parts, "\\n\\n")
		}

		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)

		// Overwrite the dynamic server compiled HTML
		htmlPath := filepath.Join(storageDir, "html", strings.TrimSuffix(cleanName, ".md")+".html")
		os.MkdirAll(filepath.Dir(htmlPath), 0755)
		compiled := compilePage(strings.TrimSuffix(cleanName, ".md"), []byte(content))
		os.WriteFile(htmlPath, compiled, 0644)

	} else {
		path = filepath.Join(storageDir, filepath.Clean(name))
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)
	}

	w.Write([]byte("Saved"))
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" || path == "/index.html" {
		http.Redirect(w, r, "/Welcome.html", http.StatusSeeOther)
		return
	}

	if strings.HasSuffix(path, ".html") {
		name := strings.TrimSuffix(filepath.Base(path), ".html")
		
		if name == "Config" {
			w.Header().Set("Content-Type", "text/html")
			compiled := compilePage("Config", []byte("Title: Config\\nCategory: Settings\\n\\n"))
			body := getConfigPageBody()
			htmlStr := strings.Replace(string(compiled), "<!-- OMN_GO_PREVIEW_BODY -->", body, 1)
			w.Write([]byte(htmlStr))
			return
		}

		htmlPath := filepath.Join(storageDir, "html", filepath.Clean(name+".html"))

		if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
			mdPath := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))
			if _, err := os.Stat(mdPath); os.IsNotExist(err) {
				embedData, err := staticFS.ReadFile("frontend/md/" + name + ".md")
				if err == nil {
					os.MkdirAll(filepath.Dir(mdPath), 0755)
					os.WriteFile(mdPath, embedData, 0644)
				} else {
					timestamp := time.Now().Format("2006-01-02 15:04:05")
					defaultContent := fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes\\n\\n# %s\\n\\nStart editing this page!", name, timestamp, name)
					os.MkdirAll(filepath.Dir(mdPath), 0755)
					os.WriteFile(mdPath, []byte(defaultContent), 0644)
				}
			}

			mdContent, err := os.ReadFile(mdPath)
			if err == nil {
				compiled := compilePage(name, mdContent)
				os.MkdirAll(filepath.Dir(htmlPath), 0755)
				os.WriteFile(htmlPath, compiled, 0644)
			}
		}

		w.Header().Set("Content-Type", "text/html")
		http.ServeFile(w, r, htmlPath)
		return
	}

	filePath := filepath.Join(storageDir, filepath.Clean(path))
	if _, err := os.Stat(filePath); err == nil {
		http.ServeFile(w, r, filePath)
		return
	}

	http.NotFound(w, r)
}

func StartServer() {
	initStorage() // Execute synchronously to ensure config is loaded instantly
	
	// Fallback MIME types for minimal Docker containers
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".webp", "image/webp")
	mime.AddExtensionType(".png", "image/png")
	mime.AddExtensionType(".jpg", "image/jpeg")
	mime.AddExtensionType(".jpeg", "image/jpeg")
	mime.AddExtensionType(".gif", "image/gif")
	mime.AddExtensionType(".json", "application/json")
	mime.AddExtensionType(".woff", "font/woff")
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".ttf", "font/ttf")

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", serveFrontend)
		fSys, _ := embed.FS(staticFS), error(nil)
		
		serveStrict := func(ext, cType string) http.Handler {
			fsHandler := http.FileServer(http.FS(fSys))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, ext) {
					http.Error(w, "Forbidden: Invalid file extension", http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", cType)
				
				if r.URL.Path == "/js/Bookmarker.js" {
					data, err := staticFS.ReadFile("frontend/js/Bookmarker.js")
					if err == nil {
						js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
						js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
						w.Write([]byte(js))
						return
					}
				}
				
				fsHandler.ServeHTTP(w, r)
			})
		}

		mux.Handle("/js/", serveStrict(".js", "application/javascript"))
		mux.Handle("/css/fonts/", serveStrict(".woff2", "font/woff2"))
		mux.Handle("/css/", serveStrict(".css", "text/css"))
		mux.Handle("/json/", serveStrict(".json", "application/json"))
		
		// Config for files handling Content-type by served directories
		serveStorageDir := func(subDir, cType string) http.Handler {
			dirPath := filepath.Join(storageDir, subDir)
			os.MkdirAll(dirPath, 0755)
			fsHandler := http.StripPrefix("/"+subDir+"/", http.FileServer(http.Dir(dirPath)))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if cType != "" {
					w.Header().Set("Content-Type", cType)
				}
				fsHandler.ServeHTTP(w, r)
			})
		}
		
		mux.Handle("/images/", serveStorageDir("images", ""))
		mux.Handle("/user_json/", serveStorageDir("user_json", "application/json"))

		mux.HandleFunc("/login", handleLogin)
		mux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
		mux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
		mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
		mux.HandleFunc("/api/upload_json", authMiddleware(handleUploadJSON, true))
		mux.HandleFunc("/api/note", handleGetNote)
		mux.HandleFunc("/api/save", authMiddleware(handleSaveNote, true))
		mux.HandleFunc("/api/config", authMiddleware(handleConfig, true))
		mux.HandleFunc("/api/edit-external", authMiddleware(handleEditExternal, true))
		
		port := fmt.Sprintf(":%d", appConfig.ServerPort)
		log.Printf("OMN-Go Backend running on %s", port)
		http.ListenAndServe(port, connectionMiddleware(mux))
	}()
}

// GetServerPort safely exposes the configured port for frontend wrappers
func GetServerPort() int {
	return appConfig.ServerPort
}
"""

    # 8. WRITE BASELINE MARKDOWN TEMPLATES FOR STATICFS
    files["backend/frontend/md/Welcome.md"] = """Title: Welcome
Date: 2026-06-14 12:00:00
Category: System

Welcome to OMN-Go! Start editing and organizing your notes completely offline.

- [Help](Welcome)
- [Scripting Rules](ScriptRules.md)
- [Bookmarks](Bookmarks)
- [Quick Notes](QuickNotes)
"""

    files["backend/frontend/md/ScriptRules.md"] = """Title: JS Scripting Rules
Date: 2026-06-15
Category: System

# JavaScript Guidelines for OMN-Go

Because OMN-Go is rendered server-side, standard scripts running on layout structures should scope state securely.

### Rule 1: Isolate variables using Block Scopes or IIFEs
Never leave `const` or `let` in the top-level global scope. Wrap the script in an Anonymous Block `{ ... }` or an Immediately Invoked Function Expression (IIFE).

```javascript
{
    const myLocalVar = "Safe!";
    let counter = 0;
}
```

### Rule 2: Explicitly attach required globals to `window`
If a function is needed for an HTML `onclick` event, attach it directly to the `window` object.

```javascript
window.doSomething = function() {
    alert("This works safely on reload!");
};
```
"""

    files["backend/frontend/md/QuickNotes.md"] = """Title: Quick Notes
Date: 2026-06-14 12:00:00
Category: Log


"""

    files["backend/frontend/md/Bookmarks.md"] = """Title: Bookmarks
Date: 2026-06-15 20:00:00
Author: 
Tags: Bookmarks

<script>bookmarks = [
<!-- Don't edit body below this line -->
];
</script>
"""

    # Write files programmatically
    for path, content in files.items():
        dir_name = os.path.dirname(path)
        if dir_name:
            os.makedirs(dir_name, exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            f.write(content)
        print(f"  [+] Wrote {path}")

    print("\n[+] Project Overhaul Done successfully! Your project is now locked on OMN-Go Version 1.2.0.")

    commit_msg = """feat(core): dynamic server overhaul and rename to OMN-Go

- Renamed result executables globally from GoOMN to OMN-Go.
- Implemented automated server-side Markdown-to-HTML compilation.
- Precompiled and saved styled .html paths inside isolated virtual folders.
- Created special 'Config' dashboard routing layout updating config.json dynamically.
- Synchronized Android gradle version targets to target OMN-Go V1.2.0 (code 10200)."""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    build_project()