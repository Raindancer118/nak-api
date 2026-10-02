# The JavaScript bridge is called by name from the web UI.
-keepclassmembers class de.raindancer118.naknak.AppBridge {
    @android.webkit.JavascriptInterface <methods>;
}
