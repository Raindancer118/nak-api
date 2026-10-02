package de.raindancer118.naknak;

/**
 * What naknak's notifications say (and so what the lock screen shows).
 * GENERIC: only "Neues in naknak"; KIND: what happened ("Neue Note
 * verfügbar!") without the grade or message text; FULL: everything.
 */
final class LockScreen {
    static final String GENERIC = "generic", KIND = "kind", FULL = "full";

    private LockScreen() {
    }

    /** Lock screen title for this kind, or null for the generic one. */
    static String title(String kind, String mode) {
        if (!KIND.equals(mode) && !FULL.equals(mode)) return null;
        switch (kind) {
            case "grade":
                return "Neue Note verfügbar!";
            case "message":
                return "Neue Nachricht";
            case "deadline":
                return "Frist in naknak";
            case "news":
                return "Neues in Moodle";
            case "moodle":
                return "Neue Moodle-Benachrichtigung";
            default:
                return null;
        }
    }

    static boolean showsAll(String mode) {
        return FULL.equals(mode);
    }
}
