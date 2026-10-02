package de.raindancer118.naknak;

import android.content.Context;
import android.webkit.CookieManager;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import org.json.JSONException;
import org.json.JSONObject;

/**
 * Talks to the naknak server with the WebView's cookies (Authentik session
 * plus naknak session), so background work needs no credentials of its own.
 */
final class Api {
    /** Signals that the session expired: the user must open the app once. */
    static final class LoginRequired extends IOException {
        LoginRequired() {
            super("login required");
        }
    }

    private final String server;

    Api(Context c) {
        server = Prefs.server(c);
    }

    boolean configured() {
        return server != null;
    }

    JSONObject get(String path) throws IOException {
        return request("GET", path, null);
    }

    /** Calls a naknak tool; the result object (body.result) is returned. */
    JSONObject tool(String name, JSONObject args) throws IOException {
        JSONObject body = request("POST", "/api/tools/" + name, args.toString());
        JSONObject res = body.optJSONObject("result");
        if (res == null) throw new IOException("unexpected answer from " + name);
        return res;
    }

    private JSONObject request(String method, String path, String json) throws IOException {
        if (server == null) throw new IOException("no server configured");
        String url = server + path;
        HttpURLConnection c = (HttpURLConnection) new URL(url).openConnection();
        c.setInstanceFollowRedirects(false); // a redirect means "go log in", not data
        c.setConnectTimeout(15000);
        c.setReadTimeout(60000);
        c.setRequestMethod(method);
        c.setRequestProperty("User-Agent", "naknak-android");
        c.setRequestProperty("Accept", "application/json");
        String cookies = CookieManager.getInstance().getCookie(url);
        if (cookies != null) c.setRequestProperty("Cookie", cookies);
        if (json != null) {
            c.setDoOutput(true);
            c.setRequestProperty("Content-Type", "application/json");
            try (OutputStream o = c.getOutputStream()) {
                o.write(json.getBytes(StandardCharsets.UTF_8));
            }
        }
        int code = c.getResponseCode();
        if (code == 401 || (code >= 300 && code < 400)) {
            c.disconnect();
            throw new LoginRequired();
        }
        InputStream in = code < 400 ? c.getInputStream() : c.getErrorStream();
        String text = read(in);
        c.disconnect();
        if (code >= 400) throw new IOException("HTTP " + code + " for " + path);
        try {
            return new JSONObject(text);
        } catch (JSONException e) {
            throw new IOException("not JSON from " + path, e);
        }
    }

    private static String read(InputStream in) throws IOException {
        if (in == null) return "";
        ByteArrayOutputStream b = new ByteArrayOutputStream();
        byte[] buf = new byte[8192];
        int n;
        while ((n = in.read(buf)) > 0) b.write(buf, 0, n);
        in.close();
        return b.toString(StandardCharsets.UTF_8.name());
    }
}
