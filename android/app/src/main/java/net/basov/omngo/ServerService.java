package net.basov.omngo;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.graphics.drawable.Icon;
import android.os.Build;
import android.os.IBinder;
import android.os.PowerManager;

import net.basov.omngo.backend.Backend;

/**
 * Service that owns the Go HTTP server for the lifetime of the process.
 *
 * FOREGROUND STATE IS DERIVED FROM CONFIG, NOT FROM HOW WE WERE STARTED.
 * The persistent notification, the wake lock and the foreground promotion
 * exist ONLY while LAN sharing is enabled in config.json:
 *
 *  - Sharing OFF: the only client of the server is this app's own WebView,
 *    which needs the server only while it is on screen - and while it is
 *    on screen, the process is foreground anyway. So the service runs as a
 *    plain started service: no notification, no wake lock, no permissions,
 *    and the process may be frozen in the background without anyone
 *    noticing.
 *
 *  - Sharing ON: other devices must reach the server with the app
 *    invisible or the screen locked, so the service promotes itself to
 *    foreground with a persistent notification (showing the LAN address)
 *    and holds a partial wake lock. This exempts the process from the
 *    cached-app freezer that otherwise stops every Go goroutine the moment
 *    the app leaves the screen.
 *
 * Because the decision is (re)made from config.json on every start, and a
 * ShareLAN change forces a full process restart (see /api/restart on the
 * Go side), the notification can no longer disagree with the actual
 * sharing state - both are computed from the same file at the same moment.
 */
public class ServerService extends Service {
    /** Intent action for the notification's Stop button. */
    public static final String ACTION_STOP = "net.basov.omngo.action.STOP_SERVER";

    private static final String CHANNEL_ID = "omngo_server";
    private static final int NOTIFICATION_ID = 1;

    /**
     * Backend.startServer() must run at most ONCE per OS process: the Go
     * side binds the TCP port, and a second call would collide with the
     * first bind. The service object can be destroyed and recreated within
     * the same process, so the guard must be static - it resets exactly
     * when the process (and with it the Go runtime and its socket) dies,
     * including the deliberate os.Exit(0) that /api/restart performs.
     */
    private static boolean backendStarted = false;

    private PowerManager.WakeLock wakeLock;

    // ---------------------------------------------------------------
    // Config access (shared with MainActivity via the static helpers)
    // ---------------------------------------------------------------

    /**
     * The Go backend's storage root on this device. getExternalMediaDirs()
     * is Android's own answer to "this app's external media directory",
     * which already resolves per the ACTUALLY RUNNING applicationId
     * (net.basov.omngo vs net.basov.omngo.fdroid - see the productFlavors
     * block in build.gradle) with no need to know the package name up
     * front. Passed into Backend.startServer() below so the Go side (see
     * initStorage's overrideDir in backend/storage.go) uses the same
     * directory instead of a hardcoded "net.basov.omngo" literal that was
     * wrong for the fdroid flavor. Falls back to building the same path
     * from getPackageName() if the media-dirs API returns nothing (should
     * be rare in practice), which - unlike the old literal - is still
     * correct for whichever flavor is actually running.
     */
    public static String storageDir(Context ctx) {
        java.io.File[] dirs = ctx.getExternalMediaDirs();
        if (dirs != null && dirs.length > 0 && dirs[0] != null) {
            return dirs[0].getAbsolutePath();
        }
        return "/storage/emulated/0/Android/media/" + ctx.getPackageName();
    }

    /**
     * Reads config.json from the same scoped-storage directory the Go
     * backend uses. Returns null if the file doesn't exist yet (first
     * launch) or can't be parsed - both treated as "sharing off, defaults"
     * by the callers.
     */
    private static org.json.JSONObject readConfig(Context ctx) {
        try {
            java.io.File cfg = new java.io.File(storageDir(ctx), "config.json");
            if (!cfg.exists()) return null;
            byte[] buf = new byte[(int) cfg.length()];
            java.io.FileInputStream in = new java.io.FileInputStream(cfg);
            try {
                int off = 0;
                while (off < buf.length) {
                    int n = in.read(buf, off, buf.length - off);
                    if (n < 0) break;
                    off += n;
                }
            } finally {
                in.close();
            }
            return new org.json.JSONObject(new String(buf, java.nio.charset.StandardCharsets.UTF_8));
        } catch (Exception e) {
            e.printStackTrace();
            return null;
        }
    }

    /** True when config.json says share_lan is enabled. */
    public static boolean isLanSharingEnabled(Context ctx) {
        org.json.JSONObject cfg = readConfig(ctx);
        return cfg != null && cfg.optBoolean("share_lan", false);
    }

    /** Configured server port (default 8080). */
    public static int serverPort(Context ctx) {
        org.json.JSONObject cfg = readConfig(ctx);
        int port = cfg != null ? cfg.optInt("server_port", 8080) : 8080;
        return port > 0 ? port : 8080;
    }

    /**
     * First non-loopback site-local IPv4 address - the address other LAN
     * devices use to reach this phone. Falls back to "0.0.0.0" when no
     * network is up (Wi-Fi off), which is honest: sharing is bound but
     * currently unreachable.
     */
    private static String lanAddress() {
        try {
            java.util.Enumeration<java.net.NetworkInterface> ifaces =
                    java.net.NetworkInterface.getNetworkInterfaces();
            while (ifaces != null && ifaces.hasMoreElements()) {
                java.net.NetworkInterface iface = ifaces.nextElement();
                if (!iface.isUp() || iface.isLoopback()) continue;
                java.util.Enumeration<java.net.InetAddress> addrs = iface.getInetAddresses();
                while (addrs.hasMoreElements()) {
                    java.net.InetAddress addr = addrs.nextElement();
                    if (addr instanceof java.net.Inet4Address
                            && !addr.isLoopbackAddress()
                            && addr.isSiteLocalAddress()) {
                        return addr.getHostAddress();
                    }
                }
            }
        } catch (Exception e) {
            e.printStackTrace();
        }
        return "0.0.0.0";
    }

    // ---------------------------------------------------------------
    // Service lifecycle
    // ---------------------------------------------------------------

    @Override
    public void onCreate() {
        super.onCreate();
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationChannel ch = new NotificationChannel(
                    CHANNEL_ID, "OMN-Go LAN Sharing", NotificationManager.IMPORTANCE_LOW);
            ch.setDescription("Shown while the note server is shared on the local network");
            ch.setShowBadge(false);
            NotificationManager nm = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
            if (nm != null) {
                nm.createNotificationChannel(ch);
            }
        }
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && ACTION_STOP.equals(intent.getAction())) {
            // The Go server has no stop API; leaving foreground state is
            // enough - the process drops back to cached priority and the
            // OS freezes/kills it in due course. Reopening the app starts
            // the service (and, if the process died, the backend) again.
            stopForeground(true);
            stopSelf();
            return START_NOT_STICKY;
        }

        // Decide foreground state from config.json - the same file the Go
        // side derives its bind address from, so the two cannot disagree.
        // Re-evaluated on every start, including the null-intent
        // START_STICKY restart after the deliberate /api/restart exit.
        boolean lan = isLanSharingEnabled(this);

        if (lan) {
            Notification n = buildNotification();
            if (Build.VERSION.SDK_INT >= 34) {
                startForeground(NOTIFICATION_ID, n,
                        android.content.pm.ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
            } else {
                startForeground(NOTIFICATION_ID, n);
            }
            // Keep the CPU available for LAN requests while the screen is
            // off. Non-reference-counted so repeated starts never stack.
            if (wakeLock == null) {
                try {
                    PowerManager pm = (PowerManager) getSystemService(POWER_SERVICE);
                    if (pm != null) {
                        wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "OMNGo::ServerWakeLock");
                        wakeLock.setReferenceCounted(false);
                        wakeLock.acquire();
                    }
                } catch (Exception e) {
                    e.printStackTrace();
                }
            }
        } else {
            // Sharing off: plain background service. Make sure no stale
            // foreground state or notification survives from a previous
            // configuration of this same service object.
            stopForeground(true);
            releaseWakeLock();
        }

        // Ensure Android OS mounts scoped storage directories for native
        // C/Go access BEFORE the Go server first touches them.
        java.io.File[] mediaDirs = getExternalMediaDirs();
        if (mediaDirs != null && mediaDirs.length > 0 && mediaDirs[0] != null) {
            mediaDirs[0].mkdirs();
        }

        if (!backendStarted) {
            // storageDir(this) is resolved from the actually-running
            // applicationId (see the field comment on storageDir above),
            // not a hardcoded literal - correct for both the standard and
            // fdroid flavors.
            Backend.startServer(storageDir(this));
            backendStarted = true;
        }

        // Sticky so the LAN-sharing server comes back if the system
        // reclaims it; with sharing off a restart is harmless (it comes
        // back as a plain background service).
        return START_STICKY;
    }

    private Notification buildNotification() {
        // Tapping the notification brings the existing UI back.
        Intent open = new Intent(this, MainActivity.class);
        open.setFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent contentPI = PendingIntent.getActivity(
                this, 0, open, PendingIntent.FLAG_IMMUTABLE);

        Intent stop = new Intent(this, ServerService.class).setAction(ACTION_STOP);
        PendingIntent stopPI = PendingIntent.getService(
                this, 1, stop, PendingIntent.FLAG_IMMUTABLE);

        // The address other devices should type into their browser.
        String shareUrl = "http://" + lanAddress() + ":" + serverPort(this);

        Notification.Builder b;
        if (Build.VERSION.SDK_INT >= 26) {
            b = new Notification.Builder(this, CHANNEL_ID);
        } else {
            b = new Notification.Builder(this);
            b.setPriority(Notification.PRIORITY_LOW);
        }
        b.setContentTitle("OMN-Go sharing on LAN")
                .setContentText("Serving notes at " + shareUrl)
                .setStyle(new Notification.BigTextStyle()
                        .bigText("Serving notes at " + shareUrl
                                + "\nOther devices need the admin or guest password."))
                .setSmallIcon(Icon.createWithResource(this, android.R.drawable.stat_notify_sync))
                .setOngoing(true)
                .setContentIntent(contentPI)
                .addAction(new Notification.Action.Builder(
                        Icon.createWithResource(this, android.R.drawable.ic_menu_close_clear_cancel),
                        "Stop", stopPI).build());
        return b.build();
    }

    private void releaseWakeLock() {
        if (wakeLock != null && wakeLock.isHeld()) {
            wakeLock.release();
        }
        wakeLock = null;
    }

    @Override
    public void onDestroy() {
        releaseWakeLock();
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null; // started service only, no binding
    }
}
