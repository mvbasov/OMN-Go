package net.basov.omngo;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.util.HashMap;
import java.util.Map;

// ----------------------------------------------------------------------
// The Android reader of config.json
// ----------------------------------------------------------------------
//
// Four settings reach the Android layer through the file and not through
// the Go HTTP server: android_fullscreen, enable_intent_uri,
// enable_termux_intent and max_upload_size_mb. MainActivity reads each one
// at the moment it needs it. A change on the Config page therefore applies
// at the next tap, and it needs no restart of the application.
//
// MainActivity held THREE near copies of the same reader until 26.09.25.
// readConfigString, readConfigFlag and readMaxUploadSizeMB each opened the
// file, read the bytes, built an org.json.JSONObject and asked it for one
// key. Each copy carried its own default, thus a change of a default was
// a change in three places, and no test held any of them.
//
// WHY THIS CLASS IMPORTS NO android PACKAGE. org.json is part of the
// Android framework. A plain Java virtual machine does not have it, thus a
// test of a class that uses it needs an emulator. This class therefore
// reads the file with java.io and parses it with the small parser below.
// javac and java alone can then run a test of it. See
// android/test/java/net/basov/omngo/OmnConfigTest.java, which
// TestJavaUnitTests in backend/java_test.go compiles and runs.
//
// IT ADDS NO GRADLE DEPENDENCY. Rule 1 of CLAUDE.md section 1 holds: the
// one dependency of the Android build is the fileTree of libs. The F-Droid
// recipe therefore needs no change, and the F-Droid build compiles this
// file the same as each other file of src/main.
//
// EACH METHOD ANSWERS A DEFAULT AND NEVER THROWS. A missing file, a
// damaged file and a value of the wrong type each give the default. The
// Android layer has no place to show a fault about the configuration, and
// a default that matches the Go side is the right answer. Each default
// below names the line of backend/config.go that it mirrors.
final class OmnConfig {

    // The three fullscreen modes. They mirror the constants of
    // normalizeFullscreen in backend/config.go.
    static final String FULLSCREEN_OFF = "off";
    static final String FULLSCREEN_ON = "fullscreen";
    static final String FULLSCREEN_IMMERSIVE = "immersive";

    // The default upload cap. It mirrors defaultMaxUploadSizeMB in
    // backend/config.go. TestAndroidDefaultsMatchTheGoDefaults in
    // backend/java_test.go compares the two.
    static final int DEFAULT_MAX_UPLOAD_MB = 3;

    private OmnConfig() {
    }

    // read answers the top level of config.json as a map, or an empty map
    // when the file is absent or damaged. A caller reads one key from it.
    //
    // It reads the whole file each time. The file is small, and a cache
    // would have to know when the Config page wrote a new one.
    static Map<String, Object> read(File storageDir) {
        try {
            File cfgFile = new File(storageDir, "config.json");
            if (!cfgFile.exists()) return new HashMap<String, Object>();
            FileInputStream fis = new FileInputStream(cfgFile);
            ByteArrayOutputStream bos = new ByteArrayOutputStream();
            byte[] buf = new byte[4096];
            int n;
            try {
                while ((n = fis.read(buf)) != -1) bos.write(buf, 0, n);
            } finally {
                fis.close();
            }
            return parseFlat(bos.toString("UTF-8"));
        } catch (Exception e) {
            return new HashMap<String, Object>();
        }
    }

    // flag answers one boolean. An absent key, a damaged file and a value
    // that is not a boolean each give false.
    //
    // False is the correct default for each caller. enable_intent_uri and
    // enable_termux_intent are off on a fresh install, and rule 11 of
    // CLAUDE.md section 1 says that they stay off until a person turns
    // them on.
    static boolean flag(File storageDir, String key) {
        Object v = read(storageDir).get(key);
        return (v instanceof Boolean) && ((Boolean) v).booleanValue();
    }

    // string answers one string, or the empty string.
    static String string(File storageDir, String key) {
        Object v = read(storageDir).get(key);
        return (v instanceof String) ? (String) v : "";
    }

    // fullscreenMode answers one of the three modes above.
    //
    // A value that this build does not know becomes FULLSCREEN_ON. That
    // mirrors normalizeFullscreen in backend/config.go, and it keeps the
    // behavior that each install had before the setting existed. A change
    // of this default without the same change there makes the Config page
    // disagree with the window.
    static String fullscreenMode(File storageDir) {
        String mode = string(storageDir, "android_fullscreen");
        if (FULLSCREEN_OFF.equals(mode) || FULLSCREEN_IMMERSIVE.equals(mode)) {
            return mode;
        }
        return FULLSCREEN_ON;
    }

    // maxUploadMB answers the upload cap in megabytes, and never a number
    // at zero or below. A cap of zero would refuse each upload.
    static int maxUploadMB(File storageDir) {
        Object v = read(storageDir).get("max_upload_size_mb");
        if (!(v instanceof Number)) return DEFAULT_MAX_UPLOAD_MB;
        int mb = ((Number) v).intValue();
        return mb > 0 ? mb : DEFAULT_MAX_UPLOAD_MB;
    }

    // ------------------------------------------------------------------
    // The parser
    // ------------------------------------------------------------------
    //
    // It reads the TOP LEVEL of a JSON object and nothing deeper. A value
    // that is a string, a number, true, false or null becomes an entry of
    // the map. A value that is a nested object or a nested array is
    // stepped over, and the parser then reads the next key.
    //
    // config.json holds two nested values today. mime_types is an object
    // and git_servers is an array. No Android caller reads either one,
    // thus stepping over them costs nothing and keeps this parser small.
    //
    // IT NEVER THROWS. A damaged file gives whatever the parser read
    // before the damage, which is an empty map for damage near the start.
    // Each caller then answers its own default.
    //
    // The input is a file that this application wrote. A parser for a
    // stranger of a file would need more than this one has.
    static Map<String, Object> parseFlat(String json) {
        Map<String, Object> out = new HashMap<String, Object>();
        if (json == null) return out;
        int i = skipSpace(json, 0);
        if (i >= json.length() || json.charAt(i) != '{') return out;
        i++;

        while (true) {
            i = skipSpace(json, i);
            if (i >= json.length()) return out;
            char c = json.charAt(i);
            if (c == '}') return out;
            if (c == ',') {
                i++;
                continue;
            }
            if (c != '"') return out;

            StringBuilder key = new StringBuilder();
            i = readString(json, i, key);
            if (i < 0) return out;

            i = skipSpace(json, i);
            if (i >= json.length() || json.charAt(i) != ':') return out;
            i++;
            i = skipSpace(json, i);
            if (i >= json.length()) return out;

            c = json.charAt(i);
            if (c == '{' || c == '[') {
                i = skipNested(json, i);
                if (i < 0) return out;
                continue;
            }
            if (c == '"') {
                StringBuilder val = new StringBuilder();
                i = readString(json, i, val);
                if (i < 0) return out;
                out.put(key.toString(), val.toString());
                continue;
            }
            int start = i;
            while (i < json.length() && ",}".indexOf(json.charAt(i)) < 0) i++;
            String raw = json.substring(start, i).trim();
            if ("true".equals(raw)) {
                out.put(key.toString(), Boolean.TRUE);
            } else if ("false".equals(raw)) {
                out.put(key.toString(), Boolean.FALSE);
            } else if ("null".equals(raw)) {
                // A null value means "no answer recorded". The key stays
                // out of the map, thus the caller reads its default.
                continue;
            } else {
                try {
                    if (raw.indexOf('.') >= 0 || raw.indexOf('e') >= 0 || raw.indexOf('E') >= 0) {
                        out.put(key.toString(), Double.valueOf(raw));
                    } else {
                        out.put(key.toString(), Long.valueOf(raw));
                    }
                } catch (NumberFormatException e) {
                    // Not a number this build understands. The key stays
                    // out of the map.
                }
            }
        }
    }

    private static int skipSpace(String s, int i) {
        while (i < s.length() && Character.isWhitespace(s.charAt(i))) i++;
        return i;
    }

    // readString reads one quoted string that starts at i, writes it into
    // out, and answers the index after the closing quote. It answers -1
    // when the string never closes.
    //
    // It reads the six escapes that a JSON writer of Go produces, and \\u
    // with four hexadecimal digits. json.MarshalIndent writes \\u for each
    // character that it escapes, thus a device label with a non-Latin
    // letter arrives through that path.
    private static int readString(String s, int i, StringBuilder out) {
        i++; // the opening quote
        while (i < s.length()) {
            char c = s.charAt(i);
            if (c == '"') return i + 1;
            if (c != '\\') {
                out.append(c);
                i++;
                continue;
            }
            i++;
            if (i >= s.length()) return -1;
            char e = s.charAt(i);
            if (e == 'u') {
                if (i + 4 >= s.length()) return -1;
                try {
                    out.append((char) Integer.parseInt(s.substring(i + 1, i + 5), 16));
                } catch (NumberFormatException ex) {
                    return -1;
                }
                i += 5;
                continue;
            }
            if (e == 'n') out.append('\n');
            else if (e == 't') out.append('\t');
            else if (e == 'r') out.append('\r');
            else if (e == 'b') out.append('\b');
            else if (e == 'f') out.append('\f');
            else out.append(e); // \" \\ \/ and anything else
            i++;
        }
        return -1;
    }

    // skipNested steps over one object or one array, and over each object
    // and array inside it. It answers the index after the closing brace or
    // bracket, or -1 when the file ends first.
    //
    // It counts a quote, because a brace inside a string is text and not
    // structure. An SSH key inside git_servers holds no brace today, and a
    // parser that trusts that would be a parser waiting for a fault.
    private static int skipNested(String s, int i) {
        int depth = 0;
        while (i < s.length()) {
            char c = s.charAt(i);
            if (c == '"') {
                StringBuilder ignored = new StringBuilder();
                i = readString(s, i, ignored);
                if (i < 0) return -1;
                continue;
            }
            if (c == '{' || c == '[') depth++;
            if (c == '}' || c == ']') {
                depth--;
                if (depth == 0) return i + 1;
            }
            i++;
        }
        return -1;
    }
}
