import os
import re
import shutil

def update_application():
    print("Beginning Architecture Migration: NativeActivity -> WebView (Version 1.0.22)")

    # 1. Restructure directories (Idempotent)
    os.makedirs("backend", exist_ok=True)
    
    if os.path.exists("server.go") and not os.path.exists("backend/server.go"):
        shutil.move("server.go", "backend/server.go")
        print("Moved server.go -> backend/server.go")
        
    if os.path.exists("frontend") and not os.path.exists("backend/frontend"):
        shutil.move("frontend", "backend/frontend")
        print("Moved frontend -> backend/frontend")
        
    if os.path.exists("main_android.go"):
        os.remove("main_android.go")
        print("Deleted legacy main_android.go")
        
    if os.path.exists("AndroidManifest.xml"):
        os.remove("AndroidManifest.xml")
        print("Deleted legacy root AndroidManifest.xml")

    # 2. Patch backend/server.go
    if os.path.exists("backend/server.go"):
        with open("backend/server.go", "r") as f:
            content = f.read()

        content = content.replace("package main", "package backend")

        exact_old = """func runServer() {
\tinitStorage()
\t
\tmux := http.NewServeMux()
\tmux.HandleFunc("/", serveFrontend)
\tmux.HandleFunc("/login", handleLogin)
\tmux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
\tmux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
\tmux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
\tmux.HandleFunc("/api/note", handleGetNote)
\t
\tport := fmt.Sprintf(":%d", appConfig.ServerPort)
\tlog.Printf("GoOMN Backend running on %s", port)
\thttp.ListenAndServe(port, connectionMiddleware(mux))
}"""
        
        exact_new = """func StartServer() {
\tgo func() {
\t\tinitStorage()
\t\t
\t\tmux := http.NewServeMux()
\t\tmux.HandleFunc("/", serveFrontend)
\t\tmux.HandleFunc("/login", handleLogin)
\t\tmux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
\t\tmux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
\t\tmux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
\t\tmux.HandleFunc("/api/note", handleGetNote)
\t\t
\t\tport := fmt.Sprintf(":%d", appConfig.ServerPort)
\t\tlog.Printf("GoOMN Backend running on %s", port)
\t\thttp.ListenAndServe(port, connectionMiddleware(mux))
\t}()
}"""
        if exact_old in content:
            content = content.replace(exact_old, exact_new)

        # Bump versions robustly
        content = re.sub(r'APP_VERSION = "1\.0\.[0-9]+"', 'APP_VERSION = "1.0.22"', content)

        with open("backend/server.go", "w") as f:
            f.write(content)
        print("Patched backend/server.go with StartServer goroutine.")

    # 3. Patch backend/frontend/index.html
    if os.path.exists("backend/frontend/index.html"):
        with open("backend/frontend/index.html", "r") as f:
            content = f.read()
        content = re.sub(r'const APP_VERSION = "1\.0\.[0-9]+";', 'const APP_VERSION = "1.0.22";', content)
        with open("backend/frontend/index.html", "w") as f:
            f.write(content)

    # 4. Patch main_desktop.go
    if os.path.exists("main_desktop.go"):
        with open("main_desktop.go", "r") as f:
            content = f.read()
            
        if '"net.basov.goomn/backend"' not in content:
            content = content.replace('"time"\n)', '"time"\n\t"net.basov.goomn/backend"\n)')
            
        content = content.replace("go runServer()", "backend.StartServer()")
        
        with open("main_desktop.go", "w") as f:
            f.write(content)
        print("Patched main_desktop.go imports.")

    # 5. Generate Android Gradle Project
    os.makedirs("android/app/src/main/java/net/basov/goomn", exist_ok=True)

    with open("android/settings.gradle", "w") as f:
        f.write('rootProject.name = "GoOMN"\ninclude \':app\'\n')

    with open("android/build.gradle", "w") as f:
        f.write("""buildscript {
    repositories {
        google()
        mavenCentral()
    }
    dependencies {
        classpath 'com.android.tools.build:gradle:8.1.2'
    }
}
allprojects {
    repositories {
        google()
        mavenCentral()
    }
}
""")

    with open("android/app/build.gradle", "w") as f:
        f.write("""plugins {
    id 'com.android.application'
}

android {
    namespace 'net.basov.goomn'
    compileSdk 34

    defaultConfig {
        applicationId "net.basov.goomn"
        minSdk 24
        targetSdk 34
        versionCode 10022
        versionName "1.0.22"
    }

    buildTypes {
        release {
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
""")

    with open("android/app/src/main/AndroidManifest.xml", "w") as f:
        f.write("""<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="net.basov.goomn">
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" />
    <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" />
    
    <application
        android:allowBackup="true"
        android:label="GoOMN"
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
""")

    with open("android/app/src/main/java/net/basov/goomn/MainActivity.java", "w") as f:
        f.write("""package net.basov.goomn;

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
        webView.setWebViewClient(new WebViewClient());

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
""")
    print("Scaffolded native Android Gradle Project.")

    # 6. Patch Dockerfile
    if os.path.exists("Dockerfile"):
        with open("Dockerfile", "r") as f:
            content = f.read()

        # Add Gradle installation to Dockerfile
        gradle_install = """# Install Gradle
RUN wget -q https://services.gradle.org/distributions/gradle-8.5-bin.zip -O /tmp/gradle.zip && \\
    mkdir -p /opt/gradle && \\
    unzip -q /tmp/gradle.zip -d /opt/gradle && \\
    rm /tmp/gradle.zip
ENV PATH=$PATH:/opt/gradle/gradle-8.5/bin
"""
        if "gradle-8.5" not in content:
            content = content.replace("# Install GoMobile", gradle_install + "\n# Install GoMobile")

        # Fix Desktop Build line since server.go moved
        content = re.sub(r'go build -o bin/goomn-desktop.*', 'go build -o bin/goomn-desktop main_desktop.go', content)

        # Remove old Android APK build step completely
        content = re.sub(r'# Android APK.*', '', content, flags=re.DOTALL)

        # Append new WebView Android Build step
        new_android_stage = """# Android APK - Webview Wrapper via Gradle & gomobile bind
RUN mkdir -p android/app/libs && gomobile bind -target=android -androidapi 24 -javapkg net.basov.goomn -o android/app/libs/goomn.aar ./backend

RUN cd android && gradle assembleDebug && cp app/build/outputs/apk/debug/app-debug.apk ../bin/goomn.apk
"""
        content += new_android_stage

        with open("Dockerfile", "w") as f:
            f.write(content)
        print("Updated Dockerfile for Gradle WebView Build.")

    # 7. Output Standardized Git Commit Message
    commit_msg = """feat(core): eliminate 5MB constraint and migrate to WebView Architecture
    
Per explicit permission to eliminate the 5MB size constraint, this patch discards 
the restrictive, pure-Go NativeActivity (which suffered from Android 14 ANRs) 
in favor of a robust Java WebView wrapper. 

The Go server logic was refactored into a reusable `backend` module, which 
gomobile bind now compiles into a standard .aar library. A native Android Gradle 
project was scaffolded directly inside the Docker container to seamlessly wrap 
this library inside a hardware-accelerated WebView pointing to localhost:8080.

Version bumped to 1.0.22"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! Re-run your docker build command.")

if __name__ == "__main__":
    update_application()