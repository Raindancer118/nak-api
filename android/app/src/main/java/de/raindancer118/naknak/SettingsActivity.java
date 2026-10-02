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
import androidx.work.WorkManager;

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
                wm.enqueue(OneTimeWorkRequest.from(WidgetWorker.class));
                Toast.makeText(requireContext(), R.string.checking, Toast.LENGTH_SHORT).show();
                return true;
            });
            Preference pin = findPreference("widget_pin");
            AppWidgetManager awm = AppWidgetManager.getInstance(requireContext());
            if (pin != null && !awm.isRequestPinAppWidgetSupported()) {
                Preference cat = findPreference("home_cat");
                if (cat != null) cat.setVisible(false);
            } else if (pin != null) {
                pin.setOnPreferenceClickListener(p -> {
                    awm.requestPinAppWidget(new ComponentName(requireContext(), NextUpWidget.class), null, null);
                    WorkManager.getInstance(requireContext()).enqueue(OneTimeWorkRequest.from(WidgetWorker.class));
                    return true;
                });
            }
            Preference version = findPreference("version");
            if (version != null) version.setSummary(BuildConfig.VERSION_NAME);
        }
    }
}
