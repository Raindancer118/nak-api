package de.raindancer118.naknak;

import java.text.ParseException;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Calendar;
import java.util.Date;
import java.util.List;
import java.util.Locale;
import org.json.JSONArray;
import org.json.JSONObject;

/** What the home screen widget shows, computed from nak_agenda and nak_deadlines. */
final class NextUp {
    private NextUp() {}

    static final class Event {
        final String title, start, end, room, kind;
        final boolean running;

        Event(String title, String start, String end, String room, String kind, boolean running) {
            this.title = title;
            this.start = start;
            this.end = end;
            this.room = room;
            this.kind = kind;
            this.running = running;
        }

        String detail() {
            String t = end.isEmpty() || end.equals(start) ? start : start + "–" + end;
            return room.isEmpty() ? t : t + " · " + room;
        }
    }

    static final class Deadline {
        final String what;
        final int days;
        final boolean urgent;

        Deadline(String what, int days, boolean urgent) {
            this.what = what;
            this.days = days;
            this.urgent = urgent;
        }

        String when() {
            if (days < 0) return "überfällig";
            if (days == 0) return "heute";
            if (days == 1) return "morgen";
            return "in " + days + " T.";
        }
    }

    private static Date parse(String day, String clock) {
        String d = day.contains(" ") ? day.substring(day.indexOf(' ') + 1) : day;
        try {
            return new SimpleDateFormat("dd.MM.yyyy HH:mm", Locale.GERMANY).parse(d + " " + clock);
        } catch (ParseException e) {
            return null;
        }
    }

    /** The event running now, else the next one; null if nothing is left. */
    static Event next(JSONObject agenda, Calendar now) {
        JSONArray days = agenda.optJSONArray("days");
        if (days == null) return null;
        Date t = now.getTime();
        for (int i = 0; i < days.length(); i++) {
            JSONArray evs = days.optJSONObject(i).optJSONArray("events");
            if (evs == null) continue;
            for (int j = 0; j < evs.length(); j++) {
                JSONObject e = evs.optJSONObject(j);
                Date s = parse(e.optString("date"), e.optString("start"));
                Date en = parse(e.optString("date"), e.optString("end", e.optString("start")));
                if (s == null) continue;
                if (en == null || en.before(s)) en = s;
                if (en.before(t)) continue;
                return new Event(e.optString("title"), e.optString("start"), e.optString("end"), e.optString("room"), e.optString("kind"), !s.after(t));
            }
        }
        return null;
    }

    static List<Deadline> deadlines(JSONObject res, int limit) {
        List<Deadline> out = new ArrayList<>();
        JSONArray l = res.optJSONArray("deadlines");
        if (l == null) return out;
        for (int i = 0; i < l.length() && out.size() < limit; i++) {
            JSONObject d = l.optJSONObject(i);
            out.add(new Deadline(d.optString("what"), d.optInt("in_days"), d.optBoolean("urgent")));
        }
        return out;
    }
}
