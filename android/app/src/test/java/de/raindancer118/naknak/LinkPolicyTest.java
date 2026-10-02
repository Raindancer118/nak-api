package de.raindancer118.naknak;

import static org.junit.Assert.*;

import org.junit.Test;

public class LinkPolicyTest {
    private final LinkPolicy p = new LinkPolicy("https://naknak.tstieh.de");

    @Test public void ownPagesStayInTheApp() {
        assertTrue(p.inApp("https://naknak.tstieh.de/#/woche", false));
        assertTrue(p.inApp("https://naknak.tstieh.de/files/a.pdf?inline=1", false));
    }

    @Test public void loginRedirectsStayButClickedForeignLinksLeave() {
        // Authentik login (redirect chain) must happen inside the WebView,
        // otherwise its cookies land in another browser
        assertTrue(p.inApp("https://portal.tstieh.de/if/flow/default-authentication-flow/", true));
        assertFalse(p.inApp("https://moodle.nordakademie.de/course/view.php?id=1", false));
        assertFalse(p.inApp("https://eduvault4.de/x", false));
    }

    @Test public void schemesOtherThanWebNeverLoad() {
        assertFalse(p.inApp("javascript:alert(1)", true));
        assertFalse(p.inApp("intent://x#Intent;end", true));
        assertFalse(p.inApp("file:///data/data/x", true));
        assertTrue(p.external("mailto:a@b.de"));
        assertFalse(p.external("javascript:alert(1)"));
    }

    @Test public void hostLookalikesAreForeign() {
        assertFalse(p.inApp("https://naknak.tstieh.de.evil.com/", false));
        assertFalse(p.inApp("https://evil.com/?https://naknak.tstieh.de", false));
    }
}
