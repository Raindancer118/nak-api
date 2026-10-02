package de.raindancer118.naknak;

import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import org.json.JSONArray;
import org.json.JSONObject;

/**
 * Remembers which naknak notifications the phone has already shown. The
 * first run only learns what exists, like the server's watcher does.
 */
final class Inbox {
    static final class Item {
        final String id, kind, title, body, url;

        Item(String id, String kind, String title, String body, String url) {
            this.id = id;
            this.kind = kind;
            this.title = title;
            this.body = body;
            this.url = url;
        }
    }

    private final Set<String> seen = new HashSet<>();
    private boolean baselined;

    Inbox(String state) {
        // "" is a real state: baselined while the server had nothing to report
        if (state == null) return;
        baselined = true;
        for (String id : state.split(",")) if (!id.isEmpty()) seen.add(id);
    }

    String state() {
        return String.join(",", seen);
    }

    List<Item> update(JSONObject res) {
        List<Item> fresh = new ArrayList<>();
        JSONArray list = res.optJSONArray("notifications");
        if (list == null) return fresh;
        Set<String> now = new HashSet<>();
        for (int i = 0; i < list.length(); i++) {
            JSONObject n = list.optJSONObject(i);
            if (n == null) continue;
            String id = n.optString("id");
            if (id.isEmpty()) continue;
            now.add(id);
            if (baselined && !seen.contains(id) && !n.optBoolean("read")) {
                fresh.add(new Item(id, n.optString("kind"), n.optString("title"), n.optString("body"), n.optString("url", "#/")));
            }
        }
        // keep only ids the server still lists, so the state stays small
        seen.clear();
        seen.addAll(now);
        baselined = true;
        return fresh;
    }
}
