package de.raindancer118.naknak;

import android.appwidget.AppWidgetManager;
import android.content.ComponentName;
import android.content.Context;
import androidx.annotation.NonNull;
import androidx.work.Worker;
import androidx.work.WorkerParameters;
import org.json.JSONObject;

/** Refreshes the home screen widgets: nak_agenda + nak_deadlines, cis_grades. */
public class WidgetWorker extends Worker {
    public WidgetWorker(@NonNull Context c, @NonNull WorkerParameters p) {
        super(c, p);
    }

    @NonNull
    @Override
    public Result doWork() {
        Context c = getApplicationContext();
        Api api = new Api(c);
        if (!api.configured()) return Result.success();
        try {
            JSONObject cache = new JSONObject();
            cache.put("agenda", api.tool("nak_agenda", new JSONObject().put("days", 3)));
            cache.put("deadlines", api.tool("nak_deadlines", new JSONObject().put("days", 14)));
            if (hasGradeWidget(c)) {
                try { // only asked when someone placed the widget; naknak serves it from its cache
                    cache.put("grades", api.tool("cis_grades", new JSONObject()));
                } catch (Api.LoginRequired e) {
                    throw e;
                } catch (Exception e) {
                    JSONObject old = old(c);
                    if (old.has("grades")) cache.put("grades", old.getJSONObject("grades"));
                }
            }
            cache.put("at", System.currentTimeMillis());
            Prefs.of(c).edit().putString(Prefs.WIDGET_JSON, cache.toString()).apply();
        } catch (Api.LoginRequired e) {
            Prefs.of(c).edit().putString(Prefs.WIDGET_JSON, "{\"login\":true}").apply();
        } catch (Exception e) {
            refreshAll(c);
            return Result.retry();
        }
        refreshAll(c);
        return Result.success();
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
