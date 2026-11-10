package net.basov.omngo;

// Fullscreen shows or hides the system bars. It has one method and no
// state, thus the method is static.
final class Fullscreen {

    private Fullscreen() {
    }

    // ----------------------------------------------------------------------
    // Fullscreen / system bars (Config page -> "Fullscreen mode")
    // ----------------------------------------------------------------------
    //
    // The app used to be unconditionally fullscreen, through
    // Theme.NoTitleBar.Fullscreen in AndroidManifest.xml. That theme is
    // still the parent of OmnGoTheme (res/values/themes.xml). It is what makes the DEFAULT case, with the status
    // bar hidden, draw correctly from the very first frame. No visible bar
    // flashes away once this code runs. applyFullscreenMode() then takes
    // over as the single authority for what is shown.
    //
    // The mode is read from config.json on each call, and that mirrors
    // readConfigFlag and readMaxUploadSizeMB. There is no HTTP round-trip.
    // A Settings change applies as soon as the Config page saves and
    // reloads, see onPageFinished in WebViewSetup. It needs no app restart.
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
    // OmnConfig holds the three mode names, thus this file and the reader
    // can never disagree about what "immersive" is called.

    static void applyFullscreenMode(MainActivity a) {
        String mode = a.readFullscreenMode();
        android.view.Window window = a.getWindow();
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
            if (OmnConfig.FULLSCREEN_OFF.equals(mode)) {
                controller.show(status | nav);
            } else if (OmnConfig.FULLSCREEN_IMMERSIVE.equals(mode)) {
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
        if (OmnConfig.FULLSCREEN_OFF.equals(mode)) {
            decor.setSystemUiVisibility(0);
        } else if (OmnConfig.FULLSCREEN_IMMERSIVE.equals(mode)) {
            decor.setSystemUiVisibility(
                    android.view.View.SYSTEM_UI_FLAG_FULLSCREEN
                            | android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                            // STICKY, not plain IMMERSIVE: the bars come back
                            // for a moment on a swipe and then re-hide by
                            // themselves, matching the API 30+ behavior above.
                            | android.view.View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY);
        } else {
            decor.setSystemUiVisibility(android.view.View.SYSTEM_UI_FLAG_FULLSCREEN);
        }
    }
}
