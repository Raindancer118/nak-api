package de.raindancer118.naknak;

import static org.junit.Assert.*;

import java.util.Calendar;
import java.util.List;
import org.json.JSONObject;
import org.junit.Test;

public class NextUpTest {
    private static Calendar at(int d, int h, int m) {
        Calendar c = Calendar.getInstance();
        c.set(2026, Calendar.OCTOBER, d, h, m, 0);
        return c;
    }

    @Test public void picksRunningOrNextEvent() throws Exception {
        JSONObject agenda = new JSONObject("{\"days\":[{\"date\":\"Fr 02.10.2026\",\"events\":["
            + "{\"date\":\"Fr 02.10.2026\",\"start\":\"09:00\",\"end\":\"12:15\",\"kind\":\"Vorlesung\",\"title\":\"Softwaretechnik\",\"room\":\"A 101\",\"source\":\"cis_stundenplan\"},"
            + "{\"date\":\"Fr 02.10.2026\",\"start\":\"13:00\",\"end\":\"16:15\",\"kind\":\"Vorlesung\",\"title\":\"Datenbanken\",\"room\":\"B 204\",\"source\":\"cis_stundenplan\"}]}]}");
        NextUp.Event e = NextUp.next(agenda, at(2, 10, 0));
        assertEquals("Softwaretechnik", e.title);
        assertTrue(e.running);
        e = NextUp.next(agenda, at(2, 12, 30));
        assertEquals("Datenbanken", e.title);
        assertFalse(e.running);
        assertEquals("13:00–16:15 · B 204", e.detail());
        assertNull(NextUp.next(agenda, at(2, 17, 0)));
    }

    @Test public void deadlinesKeepOrderAndLimit() throws Exception {
        JSONObject dl = new JSONObject("{\"deadlines\":[{\"what\":\"A\",\"in_days\":0,\"urgent\":true},{\"what\":\"B\",\"in_days\":1},{\"what\":\"C\",\"in_days\":3},{\"what\":\"D\",\"in_days\":5}]}");
        List<NextUp.Deadline> l = NextUp.deadlines(dl, 3);
        assertEquals(3, l.size());
        assertEquals("heute", l.get(0).when());
        assertEquals("morgen", l.get(1).when());
        assertEquals("in 3 T.", l.get(2).when());
    }
}
