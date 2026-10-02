package de.raindancer118.naknak;

import android.appwidget.AppWidgetManager;
import android.content.ComponentName;
import android.content.Context;
import androidx.annotation.NonNull;
import androidx.work.ExistingWorkPolicy;
import androidx.work.OneTimeWorkRequest;
import androidx.work.WorkManager;
import androidx.work.Worker;
import androidx.work.WorkerParameters;
import org.json.JSONObject;

/** Refreshes the home screen widgets: nak_agenda + nak_deadlines, cis_grades. */
public class WidgetWorker extends Worker {
    public WidgetWorker(@NonNull Context c, @NonNull WorkerParameters p) {
        super(c, p);
    }

    private interface Fetch {
        JSONObject get() throws Exception;
    }

    /**
     * Each part on its own: a slow deadline lookup must not leave the grade
     * widget waiting. A failed part keeps its last value; the error is kept
     * for the widget to show instead of "Lädt …" forever.
     */
    @NonNull
    @Override
    public Result doWork() {
        Context c = getApplicationContext();
        Api api = new Api(c);
        if (!api.configured()) return Result.success();
        JSONObject old = old(c);
        JSONObject cache = new JSONObject();
        String[] error = {null};
        boolean[] login = {false};
        Fetch agenda = () -> api.tool("nak_agenda", new JSONObject().put("days", 3));
        Fetch deadlines = () -> api.tool("nak_deadlines", new JSONObject().put("days", 14));
        Fetch grades = () -> api.tool("cis_grades", new JSONObject()); // served from naknak's cache
        part(cache, old, "agenda", agenda, error, login);
        part(cache, old, "deadlines", deadlines, error, login);
        if (hasGradeWidget(c)) part(cache, old, "grades", grades, error, login);
        try {
            if (login[0]) cache.put("login", true);
            if (error[0] != null) cache.put("error", error[0]);
            cache.put("at", System.currentTimeMillis());
        } catch (Exception ignored) {
        }
        Prefs.of(c).edit().putString(Prefs.WIDGET_JSON, cache.toString()).apply();
        refreshAll(c);
        return error[0] != null && !login[0] ? Result.retry() : Result.success();
    }

    private static void part(JSONObject cache, JSONObject old, String key, Fetch f, String[] error, boolean[] login) {
        try {
            if (login[0]) throw new Api.LoginRequired(); // no point asking again
            cache.put(key, f.get());
            return;
        } catch (Api.LoginRequired e) {
            login[0] = true;
        } catch (Exception e) {
            error[0] = friendly(e);
        }
        try {
            if (old.has(key)) cache.put(key, old.get(key));
        } catch (Exception ignored) {
        }
    }

    static String friendly(Exception e) {
        if (e instanceof java.net.UnknownHostException || e instanceof java.net.ConnectException || e instanceof java.net.NoRouteToHostException) {
            return "Server nicht erreichbar";
        }
        if (e instanceof java.net.SocketTimeoutException) return "Zeitüberschreitung";
        String m = e.getMessage();
        if (m != null && m.startsWith("HTTP 5")) return "Serverfehler (" + m.substring(5, 8) + ")";
        return m == null ? e.getClass().getSimpleName() : m;
    }

    /** Opening the app refreshes the widgets right away. */
    static void now(Context c) {
        AppWidgetManager m = AppWidgetManager.getInstance(c);
        boolean any = m.getAppWidgetIds(new ComponentName(c, GradeWidget.class)).length > 0
            || m.getAppWidgetIds(new ComponentName(c, NextUpWidget.class)).length > 0;
        if (any) WorkManager.getInstance(c).enqueueUniqueWork("widget-now", ExistingWorkPolicy.KEEP, OneTimeWorkRequest.from(WidgetWorker.class));
    }

    static void refreshAll(Context c) {
        NextUpWidget.refreshAll(c);
        GradeWidget.refreshAll(c);
    }

    private static boolean hasGradeWidget(Context c) {
        return AppWidgetManager.getInstance(c).getAppWidgetIds(new ComponentName(c, GradeWidget.class)).length > 0;
    }

    private static JSONObject old(Context c) {
        try {
            return new JSONObject(Prefs.of(c).getString(Prefs.WIDGET_JSON, "{}"));
        } catch (Exception e) {
            return new JSONObject();
        }
    }
}
