package de.raindancer118.naknak;

import android.annotation.SuppressLint;
import android.content.Context;
import android.os.Handler;
import android.os.Looper;
import android.webkit.CookieManager;
import android.webkit.RenderProcessGoneDetail;
import android.webkit.WebResourceRequest;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * Renews the reverse proxy's session (Authentik outpost) in the background.
 * The outpost session is short, Authentik's own login lasts longer; the
 * renewal runs through Authentik's flow page, which needs JavaScript, so it
 * happens in an invisible WebView sharing the app's cookies. Succeeds when
 * the chain ends on naknak again; fails when Authentik wants a password.
 */
final class SessionRenewer {
    private static final long TIMEOUT_MS = 30000;
    // the widget worker usually runs right after the notification worker;
    // a password prompt does not go away within minutes
    private static final long RETRY_AFTER_MS = 5 * 60000;
    private static long failedAt;
    // touched on the main thread only
    private static CountDownLatch running;
    private static boolean aborted;

    private SessionRenewer() {
    }

    /** Blocks the calling (worker) thread, one renewal at a time; never on the main thread. */
    static synchronized boolean renew(Context context, String server) {
        if (Looper.myLooper() == Looper.getMainLooper()) return false;
        if (failedAt != 0 && System.currentTimeMillis() - failedAt < RETRY_AFTER_MS) return false;
        Context app = context.getApplicationContext();
        CountDownLatch done = new CountDownLatch(1);
        AtomicBoolean ok = new AtomicBoolean(false);
        WebView[] holder = new WebView[1];
        Handler main = new Handler(Looper.getMainLooper());
        main.post(() -> {
            if (App.visible()) { // checked on the main thread, where it changes
                aborted = true;
                done.countDown();
                return;
            }
            aborted = false;
            running = done;
            try {
                holder[0] = start(app, server, ok, done);
            } catch (RuntimeException e) { // no WebView provider (updating)
                done.countDown();
            }
        });
        try {
            done.await(TIMEOUT_MS, TimeUnit.MILLISECONDS);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
        CountDownLatch cleaned = new CountDownLatch(1);
        boolean[] wasAborted = new boolean[1];
        main.post(() -> {
            wasAborted[0] = aborted;
            running = null;
            if (holder[0] != null) {
                holder[0].stopLoading();
                holder[0].destroy();
            }
            CookieManager.getInstance().flush();
            cleaned.countDown();
        });
        try {
            cleaned.await(5, TimeUnit.SECONDS);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
        boolean success = ok.get() && !wasAborted[0];
        if (!wasAborted[0]) failedAt = success ? 0 : System.currentTimeMillis();
        return success;
    }

    /** Main thread: the app came to the front, stop any renewal right away. */
    static void abort() {
        aborted = true;
        if (running != null) running.countDown();
    }

    @SuppressLint("SetJavaScriptEnabled")
    private static WebView start(Context app, String server, AtomicBoolean ok, CountDownLatch done) {
        LinkPolicy policy = new LinkPolicy(server);
        String target = server + "/api/version";
        WebView w = new WebView(app);
        w.getSettings().setJavaScriptEnabled(true);
        w.getSettings().setDomStorageEnabled(true);
        w.getSettings().setUserAgentString(w.getSettings().getUserAgentString() + " naknak-android");
        w.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView v, WebResourceRequest r) {
                if (policy.inApp(r.getUrl().toString(), r.isRedirect(), v.getUrl())) return false;
                done.countDown(); // the flow wants to leave: not a silent renewal
                return true;
            }

            @Override
            public void onPageFinished(WebView v, String url) {
                // the outpost's own pages live on naknak's host too; only
                // arriving back at the target means the chain is through
                if (target.equals(url)) {
                    ok.set(true);
                    done.countDown();
                }
                // a foreign page (Authentik) either moves on by itself or
                // shows a login form; the timeout decides
            }

            @Override
            public boolean onRenderProcessGone(WebView v, RenderProcessGoneDetail d) {
                done.countDown();
                return true; // the renewal failed; the worker must live on
            }
        });
        // healthz is the one page the proxy lets through without a session,
        // so ask for something behind it
        w.loadUrl(target);
        return w;
    }
}
