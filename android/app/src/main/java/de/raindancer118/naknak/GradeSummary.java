package de.raindancer118.naknak;

import org.json.JSONObject;

/** What the grade widget shows, taken from cis_grades. */
final class GradeSummary {
    final String average, credits;

    private GradeSummary(String average, String credits) {
        this.average = average;
        this.credits = credits;
    }

    static GradeSummary from(JSONObject grades) {
        JSONObject o = grades.optJSONObject("overview");
        if (o == null) o = new JSONObject();
        String avg = o.optString("average").trim();
        return new GradeSummary(avg.isEmpty() ? "–" : avg.replace('.', ','), o.optString("credits_total").trim());
    }
}
