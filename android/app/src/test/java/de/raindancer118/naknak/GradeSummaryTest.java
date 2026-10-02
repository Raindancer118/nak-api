package de.raindancer118.naknak;

import static org.junit.Assert.*;

import org.json.JSONObject;
import org.junit.Test;

public class GradeSummaryTest {
    private static JSONObject grades(String average) throws Exception {
        return new JSONObject("{\"overview\":{\"average\":\"" + average + "\",\"credits_total\":\"126\",\"modules\":[]}}");
    }

    @Test public void averageAndCredits() throws Exception {
        GradeSummary s = GradeSummary.from(grades(" 2,1 "));
        assertEquals("2,1", s.average);
        assertEquals("126", s.credits);
    }

    @Test public void noGradesYet() throws Exception {
        assertEquals("–", GradeSummary.from(grades("")).average);
        assertEquals("–", GradeSummary.from(new JSONObject()).average);
    }

    @Test public void averageInGermanNotation() throws Exception {
        assertEquals("2,3", GradeSummary.from(grades("2.3")).average);
    }
}
