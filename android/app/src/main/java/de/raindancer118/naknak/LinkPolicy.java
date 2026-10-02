package de.raindancer118.naknak;

import java.net.URI;
import java.util.Locale;

/** Decides what the WebView loads itself and what goes to other apps. */
final class LinkPolicy {
    private final String host;

    LinkPolicy(String server) {
        String h = null;
        try {
            h = new URI(server).getHost();
        } catch (Exception ignored) {
        }
        host = h == null ? "" : h.toLowerCase(Locale.ROOT);
    }

    private static URI parse(String url) {
        try {
            return new URI(url);
        } catch (Exception e) {
            return null;
        }
    }

    /**
     * True if the WebView should load url: naknak itself always, any https
     * page when the server redirects there (the Authentik login), nothing else.
     */
    boolean inApp(String url, boolean redirect) {
        URI u = parse(url);
        if (u == null || u.getScheme() == null) return false;
        String scheme = u.getScheme().toLowerCase(Locale.ROOT);
        if (!scheme.equals("https") && !scheme.equals("http")) return false;
        String h = u.getHost() == null ? "" : u.getHost().toLowerCase(Locale.ROOT);
        if (h.equals(host)) return true;
        return redirect && scheme.equals("https");
    }

    /** Links the user taps that leave naknak: web pages, mail, phone. */
    boolean external(String url) {
        URI u = parse(url);
        if (u == null || u.getScheme() == null) return false;
        switch (u.getScheme().toLowerCase(Locale.ROOT)) {
            case "https":
            case "http":
            case "mailto":
            case "tel":
                return true;
            default:
                return false;
        }
    }
}
