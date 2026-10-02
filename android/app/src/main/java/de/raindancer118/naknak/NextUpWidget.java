package de.raindancer118.naknak;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.view.View;
import android.widget.RemoteViews;
import androidx.work.OneTimeWorkRequest;
import androidx.work.WorkManager;
import java.util.Calendar;
import java.util.List;
import org.json.JSONObject;

/** Home screen widget: what's next today plus the next deadlines. */
public class NextUpWidget extends AppWidgetProvider {
    private static final int[] ROWS = {R.id.d1, R.id.d2, R.id.d3};

    @Override
    public void onUpdate(Context c, AppWidgetManager m, int[] ids) {
        render(c, m, ids);
        WorkManager.getInstance(c).enqueue(OneTimeWorkRequest.from(WidgetWorker.class));
    }

    static void refreshAll(Context c) {
        AppWidgetManager m = AppWidgetManager.getInstance(c);
        render(c, m, m.getAppWidgetIds(new ComponentName(c, NextUpWidget.class)));
    }

    private static void render(Context c, AppWidgetManager m, int[] ids) {
        RemoteViews v = new RemoteViews(c.getPackageName(), R.layout.widget_next_up);
        Intent open = new Intent(c, MainActivity.class).putExtra(MainActivity.EXTRA_PATH, "#/");
        v.setOnClickPendingIntent(R.id.widget_root, PendingIntent.getActivity(c, 7, open, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        try {
            String raw = Prefs.of(c).getString(Prefs.WIDGET_JSON, null);
            JSONObject cache = raw == null ? new JSONObject() : new JSONObject(raw);
            if (cache.optBoolean("login") && cache.optJSONObject("agenda") == null) {
                v.setTextViewText(R.id.title, c.getString(R.string.widget_login));
                v.setTextViewText(R.id.detail, "");
                v.setViewVisibility(R.id.badge, View.GONE);
                for (int r : ROWS) v.setViewVisibility(r, View.GONE);
            } else {
                JSONObject agenda = cache.optJSONObject("agenda");
                NextUp.Event e = agenda == null ? null : NextUp.next(agenda, Calendar.getInstance());
                if (e != null) {
                    v.setTextViewText(R.id.title, e.title);
                    v.setTextViewText(R.id.detail, e.detail());
                    v.setTextViewText(R.id.badge, e.running ? c.getString(R.string.widget_running) : e.kind);
                    v.setViewVisibility(R.id.badge, View.VISIBLE);
                } else {
                    v.setTextViewText(R.id.title, c.getString(agenda == null ? R.string.widget_loading : R.string.widget_free));
                    v.setTextViewText(R.id.detail, agenda == null && cache.has("error") ? c.getString(R.string.widget_error, cache.optString("error")) : "");
                    v.setViewVisibility(R.id.badge, View.GONE);
                }
                JSONObject dl = cache.optJSONObject("deadlines");
                List<NextUp.Deadline> list = dl == null ? java.util.Collections.emptyList() : NextUp.deadlines(dl, ROWS.length);
                for (int i = 0; i < ROWS.length; i++) {
                    if (i < list.size()) {
                        NextUp.Deadline d = list.get(i);
                        v.setTextViewText(ROWS[i], d.when() + " · " + d.what);
                        v.setTextColor(ROWS[i], c.getColor(d.urgent ? R.color.due : R.color.widget_text));
                        v.setViewVisibility(ROWS[i], View.VISIBLE);
                    } else {
                        v.setViewVisibility(ROWS[i], View.GONE);
                    }
                }
            }
        } catch (Exception ignored) {
            v.setTextViewText(R.id.title, c.getString(R.string.widget_loading));
        }
        for (int id : ids) m.updateAppWidget(id, v);
    }
}
