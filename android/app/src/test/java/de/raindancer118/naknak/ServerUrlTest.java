package de.raindancer118.naknak;

import static org.junit.Assert.*;

import org.junit.Test;

public class ServerUrlTest {
    @Test public void normalises() {
        assertEquals("https://naknak.tstieh.de", ServerUrl.normalize(" naknak.tstieh.de/ "));
        assertEquals("https://naknak.tstieh.de", ServerUrl.normalize("https://naknak.tstieh.de/#/woche"));
        assertEquals("https://example.org:8443/naknak", ServerUrl.normalize("https://example.org:8443/naknak/"));
    }

    @Test public void onlyHttpsExceptTheEmulatorHost() {
        assertNull(ServerUrl.normalize("http://naknak.tstieh.de"));
        assertNull(ServerUrl.normalize("ftp://x"));
        assertNull(ServerUrl.normalize("javascript:alert(1)"));
        assertNull(ServerUrl.normalize(""));
        assertNull(ServerUrl.normalize("https://"));
        assertEquals("http://10.0.2.2:8091", ServerUrl.normalize("http://10.0.2.2:8091"));
        assertEquals("http://localhost:8080", ServerUrl.normalize("http://localhost:8080"));
    }
}
