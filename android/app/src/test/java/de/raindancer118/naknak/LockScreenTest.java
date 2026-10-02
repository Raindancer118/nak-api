package de.raindancer118.naknak;

import static org.junit.Assert.*;

import org.junit.Test;

public class LockScreenTest {
    @Test public void genericSaysNothingAboutTheKind() {
        assertNull(LockScreen.title("grade", LockScreen.GENERIC));
        assertFalse(LockScreen.showsAll(LockScreen.GENERIC));
    }

    @Test public void kindNamesWhatHappenedButNotTheGrade() {
        assertEquals("Neue Note verfügbar!", LockScreen.title("grade", LockScreen.KIND));
        assertEquals("Neue Nachricht", LockScreen.title("message", LockScreen.KIND));
        assertEquals("Frist in naknak", LockScreen.title("deadline", LockScreen.KIND));
        assertEquals("Neues in Moodle", LockScreen.title("news", LockScreen.KIND));
        assertEquals("Neue Moodle-Benachrichtigung", LockScreen.title("moodle", LockScreen.KIND));
        assertNull("unknown kinds stay generic", LockScreen.title("sonstwas", LockScreen.KIND));
        assertFalse(LockScreen.showsAll(LockScreen.KIND));
    }

    @Test public void fullShowsTheNotificationItself() {
        assertTrue(LockScreen.showsAll(LockScreen.FULL));
        assertTrue("unknown setting falls back to the safe default", !LockScreen.showsAll("xyz") && LockScreen.title("grade", "xyz") == null);
    }
}
