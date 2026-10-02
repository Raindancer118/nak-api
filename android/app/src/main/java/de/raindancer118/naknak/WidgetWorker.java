package de.raindancer118.naknak;

import android.content.Context;
import androidx.annotation.NonNull;
import androidx.work.Worker;
import androidx.work.WorkerParameters;
import org.json.JSONObject;

/** Refreshes the home screen widget from nak_agenda + nak_deadlines. */
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
            cache.put("at", System.currentTimeMillis());
            Prefs.of(c).edit().putString(Prefs.WIDGET_JSON, cache.toString()).apply();
        } catch (Api.LoginRequired e) {
            Prefs.of(c).edit().putString(Prefs.WIDGET_JSON, "{\"login\":true}").apply();
        } catch (Exception e) {
            NextUpWidget.refreshAll(c);
            return Result.retry();
        }
        NextUpWidget.refreshAll(c);
        return Result.success();
    }
}
