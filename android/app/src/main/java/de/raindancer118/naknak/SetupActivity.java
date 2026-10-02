package de.raindancer118.naknak;

import android.content.Intent;
import android.content.SharedPreferences;
import android.os.Bundle;
import android.view.View;
import android.widget.Button;
import android.widget.EditText;
import android.widget.TextView;
import androidx.appcompat.app.AppCompatActivity;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;

/** First start: which naknak server? Checked via /healthz before saving. */
public class SetupActivity extends AppCompatActivity {
    private final ExecutorService io = Executors.newSingleThreadExecutor();

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        setContentView(R.layout.activity_setup);
        EditText field = findViewById(R.id.server);
        Button go = findViewById(R.id.connect);
        TextView msg = findViewById(R.id.message);
        String current = Prefs.server(this);
        field.setText(current != null ? current : getString(R.string.default_server));
        go.setOnClickListener(v -> {
            String url = ServerUrl.normalize(field.getText().toString());
            if (url == null) {
                msg.setText(R.string.setup_invalid);
                return;
            }
            go.setEnabled(false);
            msg.setText(R.string.setup_checking);
            io.execute(() -> {
                String err = check(url);
                runOnUiThread(() -> {
                    go.setEnabled(true);
                    if (err != null) {
                        msg.setText(getString(R.string.setup_failed, err));
                        return;
                    }
                    SharedPreferences.Editor e = Prefs.of(this).edit();
                    // another server: its notification ids and agenda are not ours
                    if (!url.equals(Prefs.server(this))) e.remove(Prefs.INBOX).remove(Prefs.WIDGET_JSON);
                    e.putString(Prefs.SERVER, url).apply();
                    NextUpWidget.refreshAll(this);
                    App.schedule(this);
                    startActivity(new Intent(this, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_CLEAR_TASK | Intent.FLAG_ACTIVITY_NEW_TASK));
                    finish();
                });
            });
        });
        findViewById(R.id.demo_hint).setVisibility(View.VISIBLE);
    }

    /** null if url is a naknak server, else a short reason. */
    static String check(String url) {
        try {
            HttpURLConnection c = (HttpURLConnection) new URL(url + "/healthz").openConnection();
            c.setConnectTimeout(10000);
            c.setReadTimeout(10000);
            c.setInstanceFollowRedirects(false);
            int code = c.getResponseCode();
            if (code != 200) return "HTTP " + code;
            InputStream in = c.getInputStream();
            byte[] b = new byte[512];
            int n = in.read(b);
            in.close();
            String body = n > 0 ? new String(b, 0, n, StandardCharsets.UTF_8) : "";
            return body.contains("\"status\":\"ok\"") ? null : "kein naknak-Server";
        } catch (Exception e) {
            return e.getClass().getSimpleName();
        }
    }

    @Override
    protected void onDestroy() {
        io.shutdownNow();
        super.onDestroy();
    }
}
