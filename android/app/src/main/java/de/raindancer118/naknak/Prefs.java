package de.raindancer118.naknak;

import android.content.Context;
import android.content.SharedPreferences;
import androidx.preference.PreferenceManager;

/** App settings; the defaults live in res/xml/preferences.xml. */
final class Prefs {
    static final String SERVER = "server";
    static final String LOCK = "lock";
    static final String NOTIFY = "notify";
    static final String NOTIFY_DETAILS = "notify_details";
    static final String INBOX = "inbox_state";
    static final String WIDGET_JSON = "widget_cache";
    static final String GRADE_HIDDEN = "grade_hidden";

    private Prefs() {}

    static SharedPreferences of(Context c) {
        return PreferenceManager.getDefaultSharedPreferences(c);
    }

    static String server(Context c) {
        return of(c).getString(SERVER, null);
    }

    static boolean lock(Context c) {
        return of(c).getBoolean(LOCK, false);
    }

    static boolean notify(Context c) {
        return of(c).getBoolean(NOTIFY, true);
    }

    static boolean notifyDetails(Context c) {
        return of(c).getBoolean(NOTIFY_DETAILS, true);
    }
}
