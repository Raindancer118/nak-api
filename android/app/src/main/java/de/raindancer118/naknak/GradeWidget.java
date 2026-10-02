package de.raindancer118.naknak;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.widget.RemoteViews;
import androidx.work.OneTimeWorkRequest;
import androidx.work.WorkManager;
import org.json.JSONObject;

/**
 * Home screen widget with the grade average. A home screen is public, so the
 * eye hides the numbers, and with the app lock on they never show here.
 */
public class GradeWidget extends AppWidgetProvider {
    static final String TOGGLE = "de.raindancer118.naknak.GRADE_TOGGLE";

    @Override
    public void onUpdate(Context c, AppWidgetManager m, int[] ids) {
        render(c, m, ids);
        WorkManager.getInstance(c).enqueue(OneTimeWorkRequest.from(WidgetWorker.class));
    }

    @Override
    public void onReceive(Context c, Intent i) {
        if (TOGGLE.equals(i.getAction())) {
            Prefs.of(c).edit().putBoolean(Prefs.GRADE_HIDDEN, !Prefs.of(c).getBoolean(Prefs.GRADE_HIDDEN, false)).apply();
            refreshAll(c);
            return;
        }
        super.onReceive(c, i);
    }

    static void refreshAll(Context c) {
        AppWidgetManager m = AppWidgetManager.getInstance(c);
        render(c, m, m.getAppWidgetIds(new ComponentName(c, GradeWidget.class)));
    }

    private static void render(Context c, AppWidgetManager m, int[] ids) {
        if (ids.length == 0) return;
        RemoteViews v = new RemoteViews(c.getPackageName(), R.layout.widget_grade);
        Intent open = new Intent(c, MainActivity.class).putExtra(MainActivity.EXTRA_PATH, "#/noten");
        v.setOnClickPendingIntent(R.id.grade_root, PendingIntent.getActivity(c, 8, open, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        Intent toggle = new Intent(c, GradeWidget.class).setAction(TOGGLE);
        v.setOnClickPendingIntent(R.id.grade_eye, PendingIntent.getBroadcast(c, 9, toggle, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        boolean locked = Prefs.lock(c);
        boolean hidden = locked || Prefs.of(c).getBoolean(Prefs.GRADE_HIDDEN, false);
        v.setImageViewResource(R.id.grade_eye, hidden ? R.drawable.ic_eye_off : R.drawable.ic_eye);
        try {
            String raw = Prefs.of(c).getString(Prefs.WIDGET_JSON, null);
            JSONObject cache = raw == null ? new JSONObject() : new JSONObject(raw);
            JSONObject grades = cache.optJSONObject("grades");
            if (grades == null) {
                v.setTextViewText(R.id.grade_avg, c.getString(R.string.grade_placeholder));
                v.setTextViewText(R.id.grade_credits, cache.optBoolean("login") ? c.getString(R.string.widget_login)
                    : cache.has("error") ? c.getString(R.string.widget_error, cache.optString("error")) : c.getString(R.string.widget_loading));
            } else {
                GradeSummary s = GradeSummary.from(grades);
                v.setTextViewText(R.id.grade_avg, hidden ? c.getString(R.string.grade_hidden) : s.average);
                v.setTextViewText(R.id.grade_credits, locked ? c.getString(R.string.grade_locked)
                    : s.credits.isEmpty() ? "" : c.getString(R.string.grade_credits, s.credits));
            }
        } catch (Exception ignored) {
            v.setTextViewText(R.id.grade_avg, c.getString(R.string.grade_placeholder));
        }
        for (int id : ids) m.updateAppWidget(id, v);
    }
}
