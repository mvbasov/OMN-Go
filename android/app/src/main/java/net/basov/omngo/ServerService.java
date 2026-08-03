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
 * Owns the backend for the lifetime of the process. The notification, the
 * wake lock and the foreground promotion exist only while LAN sharing is
 * enabled in config.json. Every start re-reads that file, so the
 * notification cannot disagree with the address the backend binds.
 */
public class ServerService extends Service {
    public static final String ACTION_STOP = "net.basov.omngo.action.STOP_SERVER";

    private static final String CHANNEL_ID = "omngo_server";
    private static final int NOTIFICATION_ID = 1;

    /**
     * Backend.startServer binds the TCP port, so it must run once per OS
     * process. Static because the service object is recreated in-process.
     */
    private static boolean backendStarted = false;

    private PowerManager.WakeLock wakeLock;

    /**
     * getExternalMediaDirs resolves per the running applicationId, so the
     * storage directory is right for both flavors without a package literal.
     */
    public static String storageDir(Context ctx) {
        java.io.File[] dirs = ctx.getExternalMediaDirs();
        if (dirs != null && dirs.length > 0 && dirs[0] != null) {
            return dirs[0].getAbsolutePath();
        }
        return "/storage/emulated/0/Android/media/" + ctx.getPackageName();
    }

    /**
     * @return null when config.json is missing or unparsable. Callers read
     *         null as sharing off with default values.
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

    public static boolean isLanSharingEnabled(Context ctx) {
        org.json.JSONObject cfg = readConfig(ctx);
        return cfg != null && cfg.optBoolean("share_lan", false);
    }

    /**
     * The fallback must stay BuildConfig.DEFAULT_SERVER_PORT. A fresh
     * install has no config.json, and the WebView polls what the backend binds.
     */
    public static int serverPort(Context ctx) {
        org.json.JSONObject cfg = readConfig(ctx);
        int def = BuildConfig.DEFAULT_SERVER_PORT;
        int port = cfg != null ? cfg.optInt("server_port", def) : def;
        return port > 0 ? port : def;
    }

    /**
     * @return the site-local IPv4 address other devices reach, or 0.0.0.0
     *         when no network is up and LAN sharing is unreachable.
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
            // The backend has no stop endpoint. Dropping foreground state
            // returns the process to cached priority, which is the stop.
            stopForeground(true);
            stopSelf();
            return START_NOT_STICKY;
        }

        // config.json also gives the backend its bind address, so both agree.
        boolean lan = isLanSharingEnabled(this);

        if (lan) {
            Notification n = buildNotification();
            if (Build.VERSION.SDK_INT >= 34) {
                startForeground(NOTIFICATION_ID, n,
                        android.content.pm.ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
            } else {
                startForeground(NOTIFICATION_ID, n);
            }
            // The wake lock exempts the process from the cached-app freezer.
            // A locked screen stops the backend without a battery optimization exemption.
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
            // Clear foreground state left by a start with sharing enabled.
            stopForeground(true);
            releaseWakeLock();
        }

        // Android must mount scoped storage before the backend touches it.
        java.io.File[] mediaDirs = getExternalMediaDirs();
        if (mediaDirs != null && mediaDirs.length > 0 && mediaDirs[0] != null) {
            mediaDirs[0].mkdirs();
        }

        if (!backendStarted) {
            // Per-flavor default port, so side-by-side installs do not collide.
            Backend.startServer(storageDir(this), BuildConfig.DEFAULT_SERVER_PORT);
            backendStarted = true;
        }

        // START_STICKY makes Android recreate the service after the process
        // exits, which is what makes a self-restart work.
        return START_STICKY;
    }

    // Android 13 and later need notification permission. LAN sharing works
    // without it, but shows no address and no Stop button.
    private Notification buildNotification() {
        Intent open = new Intent(this, MainActivity.class);
        open.setFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent contentPI = PendingIntent.getActivity(
                this, 0, open, PendingIntent.FLAG_IMMUTABLE);

        Intent stop = new Intent(this, ServerService.class).setAction(ACTION_STOP);
        PendingIntent stopPI = PendingIntent.getService(
                this, 1, stop, PendingIntent.FLAG_IMMUTABLE);

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
