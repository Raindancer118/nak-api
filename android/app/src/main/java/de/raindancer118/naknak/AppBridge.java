package de.raindancer118.naknak;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.webkit.JavascriptInterface;

/**
 * window.NaknakApp in the web UI. Only reacts when the calling page is the
 * configured naknak server (the interface is visible to every page the
 * WebView shows, the Authentik login included).
 */
final class AppBridge {
    private final Activity activity;
    private final String host;
    private volatile boolean gestureHeld;

    AppBridge(Activity a, String server) {
        activity = a;
        host = Uri.parse(server).getHost();
    }

    private boolean trusted() {
        final String[] url = new String[1];
        final Object lock = new Object();
        activity.runOnUiThread(() -> {
            android.webkit.WebView w = activity.findViewById(R.id.web);
            synchronized (lock) {
                url[0] = w == null ? null : w.getUrl();
                lock.notifyAll();
            }
        });
        synchronized (lock) {
            try {
                if (url[0] == null) lock.wait(500);
            } catch (InterruptedException ignored) {
            }
        }
        return url[0] != null && host != null && host.equalsIgnoreCase(Uri.parse(url[0]).getHost());
    }

    @JavascriptInterface
    public String version() {
        return BuildConfig.VERSION_NAME;
    }

    @JavascriptInterface
    public void openSettings() {
        if (!trusted()) return;
        activity.runOnUiThread(() -> activity.startActivity(new Intent(activity, SettingsActivity.class)));
    }

    /**
     * The page is handling a drag (e.g. reordering): pull-to-refresh must not
     * take the gesture. No host check: the worst a foreign page could do is
     * switch pull-to-refresh off while a finger is down.
     */
    @JavascriptInterface
    public void holdGesture(boolean held) {
        gestureHeld = held;
    }

    boolean gestureHeld() {
        return gestureHeld;
    }
}
