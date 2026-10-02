package de.raindancer118.naknak;

import android.Manifest;
import android.app.Notification;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import androidx.annotation.NonNull;
import androidx.core.app.NotificationCompat;
import androidx.core.app.NotificationManagerCompat;
import androidx.core.content.ContextCompat;
import androidx.work.Worker;
import androidx.work.WorkerParameters;
import java.util.List;
import org.json.JSONObject;

/**
 * Turns naknak's notifications (new grades, messages, Moodle news,
 * deadlines — found by the server's watcher) into Android notifications.
 */
public class NotifyWorker extends Worker {
    public NotifyWorker(@NonNull Context c, @NonNull WorkerParameters p) {
        super(c, p);
    }

    @NonNull
    @Override
    public Result doWork() {
        Context c = getApplicationContext();
        Api api = new Api(c);
        if (!api.configured() || !Prefs.notify(c)) return Result.success();
        JSONObject res;
        try {
            res = api.get("/api/notifications");
        } catch (Api.LoginRequired e) {
            loginHint(c);
            return Result.success();
        } catch (Exception e) {
            return Result.retry();
        }
        Prefs.of(c).edit().remove("login_hint").apply();
        Inbox inbox = new Inbox(Prefs.of(c).getString(Prefs.INBOX, null));
        List<Inbox.Item> fresh = inbox.update(res);
        Prefs.of(c).edit().putString(Prefs.INBOX, inbox.state()).apply();
        for (Inbox.Item n : fresh) post(c, n);
        return Result.success();
    }

    private static String channel(String kind) {
        switch (kind) {
            case "grade":
                return App.CH_GRADES;
            case "message":
                return App.CH_MESSAGES;
            case "deadline":
                return App.CH_DEADLINES;
            default:
                return App.CH_NEWS;
        }
    }

    private static PendingIntent open(Context c, String path, int code) {
        Intent i = new Intent(c, MainActivity.class).putExtra(MainActivity.EXTRA_PATH, path)
            .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        return PendingIntent.getActivity(c, code, i, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    private static boolean allowed(Context c) {
        return android.os.Build.VERSION.SDK_INT < 33
            || ContextCompat.checkSelfPermission(c, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED;
    }

    static void post(Context c, Inbox.Item n) {
        if (!allowed(c)) return;
        String mode = Prefs.notifyContent(c);
        String kindTitle = LockScreen.title(n.kind, LockScreen.KIND);
        String safeTitle = kindTitle != null ? kindTitle : c.getString(R.string.notify_generic);
        boolean full = LockScreen.showsAll(mode);
        // the notification itself carries only what the owner chose: Android
        // shows content on the lock screen unless hidden system-wide
        String title = full ? n.title : LockScreen.KIND.equals(mode) ? safeTitle : c.getString(R.string.notify_generic);
        String body = full ? n.body : c.getString(R.string.notify_open);
        // where the system does hide sensitive content, "what happened" is still shown
        Notification pub = new NotificationCompat.Builder(c, channel(n.kind))
            .setSmallIcon(R.drawable.ic_stat_duck)
            .setContentTitle(LockScreen.GENERIC.equals(mode) ? c.getString(R.string.notify_generic) : safeTitle)
            .setContentText(c.getString(R.string.notify_open))
            .build();
        NotificationCompat.Builder b = new NotificationCompat.Builder(c, channel(n.kind))
            .setSmallIcon(R.drawable.ic_stat_duck)
            .setColor(ContextCompat.getColor(c, R.color.moodle))
            .setContentTitle(title)
            .setContentText(body)
            .setStyle(new NotificationCompat.BigTextStyle().bigText(body))
            .setAutoCancel(true)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setPublicVersion(pub)
            .setCategory("message".equals(n.kind) ? NotificationCompat.CATEGORY_MESSAGE : NotificationCompat.CATEGORY_EVENT)
            .setContentIntent(open(c, n.url, n.id.hashCode()));
        try {
            NotificationManagerCompat.from(c).notify(n.id.hashCode(), b.build());
        } catch (SecurityException ignored) {
            // permission revoked meanwhile
        }
    }

    // once per expiry: the Authentik/naknak session ran out, background
    // checks need the app opened once
    private static void loginHint(Context c) {
        // the app is open: the user sees the login page anyway
        if (Prefs.of(c).getBoolean("login_hint", false) || !allowed(c) || App.visible()) return;
        Prefs.of(c).edit().putBoolean("login_hint", true).apply();
        try {
            NotificationManagerCompat.from(c).notify(1, new NotificationCompat.Builder(c, App.CH_NEWS)
                .setSmallIcon(R.drawable.ic_stat_duck)
                .setContentTitle(c.getString(R.string.login_expired))
                .setContentText(c.getString(R.string.login_expired_body))
                .setAutoCancel(true)
                .setContentIntent(open(c, "#/", 1))
                .build());
        } catch (SecurityException ignored) {
        }
    }
}
