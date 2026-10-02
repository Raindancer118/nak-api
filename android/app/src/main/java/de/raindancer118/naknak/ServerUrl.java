package de.raindancer118.naknak;

import java.net.URI;
import java.util.Locale;

/** Normalises the naknak server address the user enters. */
final class ServerUrl {
    private ServerUrl() {}

    /**
     * Returns scheme://host[:port][/path] without trailing slash or fragment,
     * or null. Only https — except plain http to the emulator's host alias and
     * localhost, for development against a local server.
     */
    static String normalize(String raw) {
        if (raw == null) return null;
        String s = raw.trim();
        if (s.isEmpty()) return null;
        if (!s.contains("://")) s = "https://" + s;
        URI u;
        try {
            u = new URI(s);
        } catch (Exception e) {
            return null;
        }
        String scheme = u.getScheme() == null ? "" : u.getScheme().toLowerCase(Locale.ROOT);
        String host = u.getHost();
        if (host == null || host.isEmpty()) return null;
        host = host.toLowerCase(Locale.ROOT);
        boolean dev = host.equals("10.0.2.2") || host.equals("localhost") || host.equals("127.0.0.1");
        if (!scheme.equals("https") && !(scheme.equals("http") && dev)) return null;
        String path = u.getRawPath() == null ? "" : u.getRawPath();
        while (path.endsWith("/")) path = path.substring(0, path.length() - 1);
        return scheme + "://" + host + (u.getPort() > 0 ? ":" + u.getPort() : "") + path;
    }
}
