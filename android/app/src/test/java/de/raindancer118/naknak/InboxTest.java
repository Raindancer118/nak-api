package de.raindancer118.naknak;

import static org.junit.Assert.*;

import java.util.List;
import org.json.JSONObject;
import org.junit.Test;

public class InboxTest {
    private static String list(String... items) {
        return "{\"notifications\":[" + String.join(",", items) + "],\"unread\":0}";
    }

    private static String n(String id, boolean read) {
        return "{\"id\":\"" + id + "\",\"kind\":\"grade\",\"title\":\"Neue Note: X\",\"body\":\"b\",\"url\":\"#/noten\",\"read\":" + read + "}";
    }

    @Test public void firstRunOnlyRemembers() throws Exception {
        Inbox in = new Inbox(null);
        List<Inbox.Item> fresh = in.update(new JSONObject(list(n("a", false), n("b", false))));
        assertTrue(fresh.isEmpty());
        assertNotNull(in.state());
    }

    @Test public void reportsOnlyNewUnread() throws Exception {
        Inbox in = new Inbox(null);
        in.update(new JSONObject(list(n("a", false))));
        Inbox again = new Inbox(in.state()); // persisted between worker runs
        List<Inbox.Item> fresh = again.update(new JSONObject(list(n("c", false), n("b", true), n("a", false))));
        assertEquals(1, fresh.size());
        assertEquals("c", fresh.get(0).id);
        assertEquals("#/noten", fresh.get(0).url);
        assertTrue(again.update(new JSONObject(list(n("c", false), n("a", false)))).isEmpty());
    }

    @Test public void emptyBaselineStillCounts() throws Exception {
        Inbox in = new Inbox(null);
        in.update(new JSONObject(list()));
        Inbox again = new Inbox(in.state());
        List<Inbox.Item> fresh = again.update(new JSONObject(list(n("a", false))));
        assertEquals(1, fresh.size());
    }

    @Test public void persistedIdsAreSeen() throws Exception {
        List<Inbox.Item> fresh = new Inbox("a,b").update(new JSONObject(list(n("a", false), n("c", false))));
        assertEquals(1, fresh.size());
        assertEquals("c", fresh.get(0).id);
    }
}
