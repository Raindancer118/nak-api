package de.raindancer118.naknak;

import android.appwidget.AppWidgetManager;
import android.content.ComponentName;
import android.content.Intent;
import android.os.Bundle;
import android.widget.Toast;
import androidx.appcompat.app.AppCompatActivity;
import androidx.appcompat.widget.Toolbar;
import androidx.biometric.BiometricManager;
import androidx.preference.Preference;
import androidx.preference.PreferenceFragmentCompat;
import androidx.preference.SwitchPreferenceCompat;
import androidx.work.OneTimeWorkRequest;
import androidx.work.WorkInfo;
import androidx.work.WorkManager;
import org.json.JSONObject;

public class SettingsActivity extends AppCompatActivity {
    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        setContentView(R.layout.activity_settings);
        Toolbar bar = findViewById(R.id.toolbar);
        bar.setNavigationOnClickListener(v -> finish());
        if (state == null) {
            getSupportFragmentManager().beginTransaction().replace(R.id.settings_container, new Fragment()).commit();
        }
    }

    @Override
    protected void onPause() {
        super.onPause();
        // the app lock decides whether the grade widget may show numbers
        WidgetWorker.refreshAll(this);
    }

    public static class Fragment extends PreferenceFragmentCompat {
        @Override
        public void onCreatePreferences(Bundle state, String rootKey) {
            setPreferencesFromResource(R.xml.preferences, rootKey);
            Preference server = findPreference("server_change");
            if (server != null) {
                server.setSummary(Prefs.server(requireContext()));
                server.setOnPreferenceClickListener(p -> {
                    startActivity(new Intent(requireContext(), SetupActivity.class));
                    return true;
                });
            }
            SwitchPreferenceCompat lock = findPreference(Prefs.LOCK);
            if (lock != null) {
                lock.setOnPreferenceChangeListener((p, v) -> {
                    int how = BiometricManager.Authenticators.BIOMETRIC_WEAK | BiometricManager.Authenticators.DEVICE_CREDENTIAL;
                    if ((Boolean) v && BiometricManager.from(requireContext()).canAuthenticate(how) != BiometricManager.BIOMETRIC_SUCCESS) {
                        Toast.makeText(requireContext(), R.string.lock_unavailable, Toast.LENGTH_LONG).show();
                        return false;
                    }
                    return true;
                });
            }
            Preference notify = findPreference(Prefs.NOTIFY);
            if (notify != null) notify.setOnPreferenceChangeListener((p, v) -> {
                p.getPreferenceManager().getSharedPreferences().edit().putBoolean(Prefs.NOTIFY, (Boolean) v).apply();
                App.schedule(requireContext());
                return true;
            });
            Preference now = findPreference("check_now");
            if (now != null) now.setOnPreferenceClickListener(p -> {
                WorkManager wm = WorkManager.getInstance(requireContext());
                wm.enqueue(OneTimeWorkRequest.from(NotifyWorker.class));
                OneTimeWorkRequest widgets = OneTimeWorkRequest.from(WidgetWorker.class);
                wm.enqueue(widgets);
                Toast.makeText(requireContext(), R.string.checking, Toast.LENGTH_SHORT).show();
                // say what came of it: the widgets' cache records the last error
                wm.getWorkInfoByIdLiveData(widgets.getId()).observe(this, info -> {
                    if (info == null || !(info.getState().isFinished() || info.getRunAttemptCount() > 0 && info.getState() == WorkInfo.State.ENQUEUED)) return;
                    String err = null, raw = Prefs.of(requireContext()).getString(Prefs.WIDGET_JSON, "{}");
                    try {
                        JSONObject cache = new JSONObject(raw);
                        if (cache.optBoolean("login")) err = getString(R.string.widget_login);
                        else if (cache.has("error")) err = cache.optString("error");
                    } catch (Exception ignored) {
                    }
                    Toast.makeText(requireContext(), err == null ? getString(R.string.check_done) : getString(R.string.check_failed, err), Toast.LENGTH_LONG).show();
                    wm.getWorkInfoByIdLiveData(widgets.getId()).removeObservers(this);
                });
                return true;
            });
            Preference pin = findPreference("widget_pin");
            AppWidgetManager awm = AppWidgetManager.getInstance(requireContext());
            if (pin != null && !awm.isRequestPinAppWidgetSupported()) {
                Preference cat = findPreference("home_cat");
                if (cat != null) cat.setVisible(false);
            } else if (pin != null) {
                pinOnClick(pin, NextUpWidget.class);
                Preference gradePin = findPreference("grade_widget_pin");
                if (gradePin != null) pinOnClick(gradePin, GradeWidget.class);
            }
            Preference version = findPreference("version");
            if (version != null) version.setSummary(BuildConfig.VERSION_NAME);
        }

        private void pinOnClick(Preference p, Class<?> provider) {
            p.setOnPreferenceClickListener(x -> {
                AppWidgetManager.getInstance(requireContext()).requestPinAppWidget(new ComponentName(requireContext(), provider), null, null);
                WorkManager.getInstance(requireContext()).enqueue(OneTimeWorkRequest.from(WidgetWorker.class));
                return true;
            });
        }
    }
}
