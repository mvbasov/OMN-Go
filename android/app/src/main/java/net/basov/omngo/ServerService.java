package net.basov.omngo;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.graphics.drawable.Icon;
import android.os.Build;
import android.os.IBinder;
import android.os.PowerManager;

import net.basov.omngo.backend.Backend;

/**
 * Foreground service that owns the Go HTTP server for the lifetime of the
 * process.
 *
 * WHY THIS EXISTS: the server used to be started from MainActivity, which
 * ties its fate to the Activity's process priority. The moment the Activity
 * leaves the screen (home button, split-screen focus loss, screen lock),
 * the process drops to the cached priority bucket, and modern Android -
 * especially 12+ with the cached-app freezer - freezes it outright: every
 * Go goroutine stops mid-instruction and open sockets go unanswered. That
 * is exactly the "server only answers while the app is visible" symptom.
 * The old PARTIAL_WAKE_LOCK never helped with this: a wake lock keeps the
 * CPU from sleeping, but has no effect on process priority or freezing,
 * and Doze ignores wake locks entirely anyway.
 *
 * A started foreground service with a visible notification keeps the
 * process in the perceptible priority class: it is exempt from the cached
 * freezer and from being routinely killed, which is what actually keeps
 * the server answering in split-screen, in the background, and behind a
 * locked screen. The wake lock is still held (now owned here, not by the
 * Activity) so the CPU can service requests while the screen is off; the
 * battery-optimization exemption MainActivity requests once covers the
 * remaining Doze network restriction on long screen-off periods.
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
     * the same process (Stop button then reopening the app, START_STICKY
     * restarts), so the guard must be static - it resets exactly when the
     * process (and with it the Go runtime and its socket) actually dies.
     */
    private static boolean backendStarted = false;

    private PowerManager.WakeLock wakeLock;

    @Override
    public void onCreate() {
        super.onCreate();
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationChannel ch = new NotificationChannel(
                    CHANNEL_ID, "OMN-Go Server", NotificationManager.IMPORTANCE_LOW);
            ch.setDescription("Keeps the local note server reachable while the app is in the background");
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
            // OS freezes/kills it in due course, which is the practical
            // equivalent of stopping. Reopening the app starts the
            // service again (and, if the process did die, the backend).
            stopForeground(true);
            stopSelf();
            return START_NOT_STICKY;
        }

        // Must be called promptly after startForegroundService() or the
        // system kills the app - so this comes before any other work.
        Notification n = buildNotification();
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTIFICATION_ID, n,
                    android.content.pm.ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
        } else {
            startForeground(NOTIFICATION_ID, n);
        }

        // Ensure Android OS mounts scoped storage directories for native
        // C/Go access BEFORE the Go server first touches them (moved here
        // from MainActivity so the single owner of the backend also owns
        // its precondition).
        java.io.File[] mediaDirs = getExternalMediaDirs();
        if (mediaDirs != null && mediaDirs.length > 0 && mediaDirs[0] != null) {
            mediaDirs[0].mkdirs();
        }

        if (!backendStarted) {
            Backend.startServer();
            backendStarted = true;
        }

        // Keep the CPU available for the Go server while the screen is
        // off. Non-reference-counted so repeated onStartCommand calls
        // (app reopened, sticky restart) never stack acquisitions.
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

        // If the system does reclaim the service under memory pressure,
        // recreate it (and re-enter foreground state) when possible.
        return START_STICKY;
    }

    private Notification buildNotification() {
        // Tapping the notification brings the existing UI back.
        Intent open = new Intent(this, MainActivity.class);
        open.setFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        // FLAG_IMMUTABLE is mandatory with targetSdk 31+; minSdk 24 >= 23
        // so the flag constant is always available.
        PendingIntent contentPI = PendingIntent.getActivity(
                this, 0, open, PendingIntent.FLAG_IMMUTABLE);

        Intent stop = new Intent(this, ServerService.class).setAction(ACTION_STOP);
        PendingIntent stopPI = PendingIntent.getService(
                this, 1, stop, PendingIntent.FLAG_IMMUTABLE);

        Notification.Builder b;
        if (Build.VERSION.SDK_INT >= 26) {
            b = new Notification.Builder(this, CHANNEL_ID);
        } else {
            b = new Notification.Builder(this);
            b.setPriority(Notification.PRIORITY_LOW);
        }
        b.setContentTitle("OMN-Go server running")
                .setContentText("Local note server is active")
                .setSmallIcon(Icon.createWithResource(this, android.R.drawable.stat_notify_sync))
                .setOngoing(true)
                .setContentIntent(contentPI)
                .addAction(new Notification.Action.Builder(
                        Icon.createWithResource(this, android.R.drawable.ic_menu_close_clear_cancel),
                        "Stop", stopPI).build());
        return b.build();
    }

    @Override
    public void onDestroy() {
        if (wakeLock != null && wakeLock.isHeld()) {
            wakeLock.release();
        }
        wakeLock = null;
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null; // started service only, no binding
    }
}
