package de.raindancer118.naknak;

import android.app.Activity;
import android.app.Application;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.content.Context;
import android.os.Bundle;
import androidx.annotation.NonNull;
import androidx.work.Constraints;
import androidx.work.ExistingPeriodicWorkPolicy;
import androidx.work.NetworkType;
import androidx.work.PeriodicWorkRequest;
import androidx.work.WorkManager;
import java.util.concurrent.TimeUnit;

public class App extends Application {
    private static volatile int started;
    static final String CH_GRADES = "grades";
    static final String CH_MESSAGES = "messages";
    static final String CH_NEWS = "news";
    static final String CH_DEADLINES = "deadlines";

    @Override
    public void onCreate() {
        super.onCreate();
        NotificationManager nm = getSystemService(NotificationManager.class);
        nm.createNotificationChannel(new NotificationChannel(CH_GRADES, getString(R.string.ch_grades), NotificationManager.IMPORTANCE_HIGH));
        nm.createNotificationChannel(new NotificationChannel(CH_MESSAGES, getString(R.string.ch_messages), NotificationManager.IMPORTANCE_HIGH));
        nm.createNotificationChannel(new NotificationChannel(CH_DEADLINES, getString(R.string.ch_deadlines), NotificationManager.IMPORTANCE_DEFAULT));
        nm.createNotificationChannel(new NotificationChannel(CH_NEWS, getString(R.string.ch_news), NotificationManager.IMPORTANCE_LOW));
        schedule(this);
        registerActivityLifecycleCallbacks(new ActivityLifecycleCallbacks() {
            @Override
            public void onActivityStarted(@NonNull Activity a) {
                started++;
                // the visible app logs in itself; a background renewal would
                // overwrite the outpost's state cookie and break that login
                SessionRenewer.abort();
            }

            @Override
            public void onActivityStopped(@NonNull Activity a) {
                started--;
            }

            @Override
            public void onActivityCreated(@NonNull Activity a, Bundle b) {
            }

            @Override
            public void onActivityResumed(@NonNull Activity a) {
            }

            @Override
            public void onActivityPaused(@NonNull Activity a) {
            }

            @Override
            public void onActivitySaveInstanceState(@NonNull Activity a, @NonNull Bundle b) {
            }

            @Override
            public void onActivityDestroyed(@NonNull Activity a) {
            }
        });
    }

    /** True while one of naknak's screens is visible. */
    static boolean visible() {
        return started > 0;
    }

    /**
     * Background checks: notifications every 15 min (WorkManager's minimum),
     * the widget every 30 min — both read naknak's own cache, so they cost
     * the CIS and Moodle nothing.
     */
    static void schedule(Context c) {
        WorkManager wm = WorkManager.getInstance(c);
        Constraints online = new Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build();
        if (Prefs.notify(c) && Prefs.server(c) != null) {
            wm.enqueueUniquePeriodicWork("notify", ExistingPeriodicWorkPolicy.KEEP,
                new PeriodicWorkRequest.Builder(NotifyWorker.class, 15, TimeUnit.MINUTES).setConstraints(online).build());
        } else {
            wm.cancelUniqueWork("notify");
        }
        if (Prefs.server(c) != null) {
            wm.enqueueUniquePeriodicWork("widget", ExistingPeriodicWorkPolicy.KEEP,
                new PeriodicWorkRequest.Builder(WidgetWorker.class, 30, TimeUnit.MINUTES).setConstraints(online).build());
        }
    }
}
