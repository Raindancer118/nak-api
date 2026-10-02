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

    boolean inApp(String url, boolean redirect) {
        return inApp(url, redirect, null);
    }

    /**
     * True if the WebView should load url. naknak itself always; any https
     * page the server redirects to (the Authentik login); and while such a
     * foreign page is showing (from), every https step it takes — the login
     * flow continues with JavaScript, and in another browser its callback
     * would lack the state cookie. Links tapped on naknak pages leave.
     */
    boolean inApp(String url, boolean redirect, String from) {
        URI u = parse(url);
        if (u == null || u.getScheme() == null) return false;
        String scheme = u.getScheme().toLowerCase(Locale.ROOT);
        if (!scheme.equals("https") && !scheme.equals("http")) return false;
        if (own(u)) return true;
        if (!scheme.equals("https")) return false;
        if (redirect) return true;
        URI f = from == null ? null : parse(from);
        return f != null && f.getScheme() != null && f.getScheme().equalsIgnoreCase("https") && !own(f);
    }

    private boolean own(URI u) {
        return u.getHost() != null && u.getHost().toLowerCase(Locale.ROOT).equals(host);
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
