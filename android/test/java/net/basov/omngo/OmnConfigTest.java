package net.basov.omngo;

import java.io.File;
import java.io.FileWriter;
import java.util.Map;

// ----------------------------------------------------------------------
// The test of OmnConfig
// ----------------------------------------------------------------------
//
// WHY IT IS HERE AND NOT UNDER android/app/src/test. Gradle reads each
// source set under android/app/src. A test source set there would become
// part of the Android project, and the F-Droid build server builds that
// project from the committed Gradle configuration. This directory is
// outside the project, thus Gradle never looks at it and the F-Droid build
// cannot see it. TestNoAndroidTestSourceSet in backend/java_test.go holds
// that rule.
//
// WHY IT USES NO JUnit. A test framework is a Gradle dependency, and rule
// 1 of CLAUDE.md section 1 allows one dependency. The check methods below
// are ten lines, and javac and java run this file with no library at all.
//
// TestJavaUnitTests in backend/java_test.go compiles this file together
// with OmnConfig.java and runs it. A failed check writes a line and sets
// the exit code, thus the Go test reports the same fault.
public final class OmnConfigTest {

    private static int failures = 0;

    public static void main(String[] args) throws Exception {
        File dir = makeTempDir();

        noFileAtAll(dir);
        theDefaults(dir);
        theValuesOfAPerson(dir);
        aDamagedFile(dir);
        aValueOfTheWrongType(dir);
        theNestedValues(dir);
        theEscapes();

        if (failures > 0) {
            System.out.println(failures + " check(s) failed");
            System.exit(1);
        }
        System.out.println("OmnConfigTest: every check passed");
    }

    // A device with no config.json yet. Each reader answers its default,
    // and none of them throws.
    private static void noFileAtAll(File dir) {
        File empty = new File(dir, "empty");
        empty.mkdirs();
        eq("no file: fullscreen", "fullscreen", OmnConfig.fullscreenMode(empty.getPath()));
        eq("no file: intent uri", false, OmnConfig.flag(empty.getPath(), "enable_intent_uri"));
        eq("no file: termux", false, OmnConfig.flag(empty.getPath(), "enable_termux_intent"));
        eq("no file: upload cap", 3, OmnConfig.maxUploadMB(empty.getPath()));
        eq("no file: a string", "", OmnConfig.string(empty.getPath(), "author"));
    }

    // The configuration that loadConfig writes on a fresh install.
    private static void theDefaults(File dir) throws Exception {
        File d = write(dir, "fresh",
            "{\n" +
            "  \"server_port\": 8080,\n" +
            "  \"author\": \"Anonymous\",\n" +
            "  \"android_fullscreen\": \"fullscreen\",\n" +
            "  \"enable_intent_uri\": false,\n" +
            "  \"enable_termux_intent\": false,\n" +
            "  \"max_upload_size_mb\": 3\n" +
            "}\n");
        eq("fresh: fullscreen", "fullscreen", OmnConfig.fullscreenMode(d.getPath()));
        eq("fresh: intent uri", false, OmnConfig.flag(d.getPath(), "enable_intent_uri"));
        eq("fresh: upload cap", 3, OmnConfig.maxUploadMB(d.getPath()));
        eq("fresh: author", "Anonymous", OmnConfig.string(d.getPath(), "author"));
    }

    // A person who turned each Android setting on.
    private static void theValuesOfAPerson(File dir) throws Exception {
        File d = write(dir, "changed",
            "{\"android_fullscreen\":\"immersive\"," +
            "\"enable_intent_uri\":true," +
            "\"enable_termux_intent\":true," +
            "\"max_upload_size_mb\":25," +
            "\"author\":\"Ann\"}");
        eq("changed: fullscreen", "immersive", OmnConfig.fullscreenMode(d.getPath()));
        eq("changed: intent uri", true, OmnConfig.flag(d.getPath(), "enable_intent_uri"));
        eq("changed: termux", true, OmnConfig.flag(d.getPath(), "enable_termux_intent"));
        eq("changed: upload cap", 25, OmnConfig.maxUploadMB(d.getPath()));
        eq("changed: author", "Ann", OmnConfig.string(d.getPath(), "author"));

        File off = write(dir, "off", "{\"android_fullscreen\":\"off\"}");
        eq("off: fullscreen", "off", OmnConfig.fullscreenMode(off.getPath()));
    }

    // A file that a crash cut in half. Each reader answers its default,
    // and the application still starts.
    private static void aDamagedFile(File dir) throws Exception {
        for (String bad : new String[]{
            "", "   ", "not json at all", "{", "{\"android_fullscreen\"",
            "{\"android_fullscreen\":", "{\"enable_intent_uri\":tr",
            "[1,2,3]", "{\"a\":\"unterminated",
        }) {
            File d = write(dir, "bad" + Math.abs(bad.hashCode()), bad);
            eq("damaged " + show(bad) + ": fullscreen", "fullscreen", OmnConfig.fullscreenMode(d.getPath()));
            eq("damaged " + show(bad) + ": flag", false, OmnConfig.flag(d.getPath(), "enable_intent_uri"));
            eq("damaged " + show(bad) + ": cap", 3, OmnConfig.maxUploadMB(d.getPath()));
        }
    }

    // A value of the wrong type reads as absent. A person who edits
    // config.json by hand can write any of these.
    private static void aValueOfTheWrongType(File dir) throws Exception {
        File d = write(dir, "types",
            "{\"enable_intent_uri\":\"true\"," +
            "\"max_upload_size_mb\":\"25\"," +
            "\"android_fullscreen\":7," +
            "\"author\":42}");
        eq("wrong type: a string is not a flag", false, OmnConfig.flag(d.getPath(), "enable_intent_uri"));
        eq("wrong type: a string is not a cap", 3, OmnConfig.maxUploadMB(d.getPath()));
        eq("wrong type: a number is not a mode", "fullscreen", OmnConfig.fullscreenMode(d.getPath()));
        eq("wrong type: a number is not a string", "", OmnConfig.string(d.getPath(), "author"));

        // A cap of zero or less refuses every upload, thus it becomes the
        // default. The Go side holds the same rule.
        File zero = write(dir, "zero", "{\"max_upload_size_mb\":0}");
        eq("cap of 0", 3, OmnConfig.maxUploadMB(zero.getPath()));
        File neg = write(dir, "neg", "{\"max_upload_size_mb\":-5}");
        eq("cap of -5", 3, OmnConfig.maxUploadMB(neg.getPath()));

        // A null value means "no answer recorded".
        File nul = write(dir, "null", "{\"enable_intent_uri\":null,\"max_upload_size_mb\":null}");
        eq("null flag", false, OmnConfig.flag(nul.getPath(), "enable_intent_uri"));
        eq("null cap", 3, OmnConfig.maxUploadMB(nul.getPath()));
    }

    // THE ONE THAT MATTERS FOR THE PARSER. config.json holds a nested
    // object (mime_types) and a nested array (git_servers). The parser
    // steps over both and reads the keys that follow them.
    //
    // THE BRACES INSIDE THE STRINGS ARE NOT BALANCED, ON PURPOSE. A
    // parser that counts braces without reading strings survives a
    // balanced pair by luck. A lone closing brace in a password ends the
    // skip early, and each Android setting then answers its default with
    // a valid file on disk. The fixture below carries a lone "}" and a
    // lone "]" for that reason.
    private static void theNestedValues(File dir) throws Exception {
        File d = write(dir, "nested",
            "{\n" +
            "  \"server_port\": 8080,\n" +
            "  \"mime_types\": {\"css\": \"text/css}\", \"js\": \"text/javascript\"},\n" +
            "  \"git_servers\": [\n" +
            "    {\"name\": \"Server 1\", \"url\": \"\", \"ssh_key_data\": \"\", \"password\": \"\"},\n" +
            "    {\"name\": \"Server 2\", \"url\": \"git@host:r.git\",\n" +
            "     \"ssh_key_data\": \"-----BEGIN-----\\nlone } brace \\\"quoted\\\"\\n-----END-----\\n\",\n" +
            "     \"password\": \"p]ss}word{\"}\n" +
            "  ],\n" +
            "  \"android_fullscreen\": \"immersive\",\n" +
            "  \"enable_intent_uri\": true,\n" +
            "  \"max_upload_size_mb\": 12\n" +
            "}\n");
        eq("nested: fullscreen after a nested object and array", "immersive", OmnConfig.fullscreenMode(d.getPath()));
        eq("nested: flag after them", true, OmnConfig.flag(d.getPath(), "enable_intent_uri"));
        eq("nested: cap after them", 12, OmnConfig.maxUploadMB(d.getPath()));
        eq("nested: the nested keys are not top level", "", OmnConfig.string(d.getPath(), "name"));

        // An empty object and an empty array are the other shapes that
        // this file carries. A nil mime_types marshals as null.
        File e = write(dir, "emptynest",
            "{\"mime_types\":{},\"git_servers\":[],\"android_fullscreen\":\"off\"}");
        eq("empty nest: fullscreen", "off", OmnConfig.fullscreenMode(e.getPath()));
        File n = write(dir, "nullnest",
            "{\"mime_types\":null,\"git_servers\":null,\"android_fullscreen\":\"off\"}");
        eq("null nest: fullscreen", "off", OmnConfig.fullscreenMode(n.getPath()));
    }

    // json.MarshalIndent escapes a character outside ASCII as \\uXXXX. A
    // device label of a person who writes Cyrillic arrives that way.
    private static void theEscapes() {
        Map<String, Object> m = OmnConfig.parseFlat(
            "{\"hostname\":\"\\u041f\\u0438\\u043a\\u0441\\u0435\\u043b\"," +
            "\"a\":\"line\\nbreak\",\"b\":\"say \\\"hi\\\"\",\"c\":\"back\\\\slash\"}");
        // The expected value is written with the SAME escapes, thus this
        // file holds no byte above 127. A javac with no UTF-8 locale
        // refuses such a byte, and version 26.09.28 failed a build on it.
        eq("escape: \\u", "\u041f\u0438\u043a\u0441\u0435\u043b", m.get("hostname"));
        eq("escape: newline", "line\nbreak", m.get("a"));
        eq("escape: quote", "say \"hi\"", m.get("b"));
        eq("escape: backslash", "back\\slash", m.get("c"));
    }

    // ------------------------------------------------------------------
    // The check methods. Ten lines in place of a test framework.
    // ------------------------------------------------------------------

    private static void eq(String what, Object want, Object got) {
        if (want == null ? got == null : want.equals(got)) return;
        failures++;
        System.out.println("FAIL " + what + ": got " + show(got) + ", want " + show(want));
    }

    private static void eq(String what, boolean want, boolean got) {
        eq(what, Boolean.valueOf(want), Boolean.valueOf(got));
    }

    private static void eq(String what, int want, int got) {
        eq(what, Integer.valueOf(want), Integer.valueOf(got));
    }

    private static String show(Object o) {
        if (o == null) return "null";
        return "\"" + o.toString().replace("\n", "\\n") + "\"";
    }

    private static File makeTempDir() throws Exception {
        File f = File.createTempFile("omnconfig", "");
        if (!f.delete() || !f.mkdirs()) throw new Exception("cannot make a temp directory");
        f.deleteOnExit();
        return f;
    }

    private static File write(File parent, String name, String body) throws Exception {
        File d = new File(parent, name);
        d.mkdirs();
        FileWriter w = new FileWriter(new File(d, "config.json"));
        try {
            w.write(body);
        } finally {
            w.close();
        }
        return d;
    }
}
