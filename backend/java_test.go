package backend

// ----------------------------------------------------------------------
// The Android layer, from the Go gate
// ----------------------------------------------------------------------
//
// The Java of this project had NO test until 26.09.26. The reason was
// always the same. A test of Android code needs an emulator, an emulator
// needs a test framework, and a test framework is a Gradle dependency.
// Rule 1 of CLAUDE.md section 1 allows one dependency.
//
// OmnConfig.java broke that chain in 26.09.25. It imports no android
// package, thus javac and java alone can run a test of it. The Docker
// image of the gate already holds a JDK for the Gradle build. The test
// below therefore needs no new tool and no change to any Dockerfile.
//
// THE F-DROID BUILD MUST NOT SEE ANY OF THIS. F-Droid builds the
// committed Gradle configuration on its own server. The recipe at
// metadata/net.basov.omngo.fdroid.yml is hard to change, because
// AutoUpdateMode copies the last build block for each new tag. Three
// tests below hold the rules that keep the recipe correct with no edit.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------
// The rules that protect the F-Droid build
// ----------------------------------------------------------------------

// The Android build must keep exactly one dependency.
//
// A test framework, a JSON library or a compat library each add a line to
// this block. The F-Droid build server then has to resolve it. Rule 1 of
// CLAUDE.md section 1 exists because that is how an Android project grows
// a dependency tree that no person can audit.
//
// This test is the reason OmnConfig.java holds a JSON parser by hand.
func TestAndroidGradleHasOneDependency(t *testing.T) {
	gradle, err := readRepoFile("android/app/build.gradle")
	if err != nil {
		t.Skipf("android/app/build.gradle is not in this tree: %v", err)
	}
	block := regexp.MustCompile(`(?s)\ndependencies\s*\{(.*?)\n\}`).FindStringSubmatch(gradle)
	if block == nil {
		t.Fatal("build.gradle has no dependencies block")
	}
	var lines []string
	for _, l := range strings.Split(block[1], "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "//") {
			continue
		}
		lines = append(lines, l)
	}
	if len(lines) != 1 {
		t.Fatalf("the Android build has %d dependencies, want 1:\n  %s",
			len(lines), strings.Join(lines, "\n  "))
	}
	const want = "implementation fileTree(dir: 'libs', include: ['*.jar', '*.aar'])"
	if lines[0] != want {
		t.Errorf("the one dependency changed:\n got  %s\n want %s\n"+
			"A new dependency needs a reason and the approval of the maintainer, "+
			"and the F-Droid build server has to resolve it.", lines[0], want)
	}
}

// Gradle reads each source set under android/app/src. A directory named
// test or androidTest there becomes part of the Android project, thus the
// F-Droid build server would compile it.
//
// The Java test of this project lives at android/test/, OUTSIDE the
// Gradle project. Nothing there reaches a device, and nothing there
// reaches the F-Droid build.
func TestNoAndroidTestSourceSet(t *testing.T) {
	for _, name := range []string{"test", "androidTest", "testFdroid", "testStandard"} {
		p := filepath.Join("..", "android", "app", "src", name)
		if _, err := os.Stat(p); err == nil {
			t.Errorf("android/app/src/%s exists. Gradle reads each source set there, "+
				"thus the F-Droid build would compile it. The Java test belongs at "+
				"android/test/, which is outside the Gradle project.", name)
		}
	}
}

// The F-Droid recipe names one Gradle flavor and no test task. A change of
// the flavor names in build.gradle would break each build block of the
// recipe, and the recipe is hard to change.
func TestFdroidFlavorStillExists(t *testing.T) {
	gradle, err := readRepoFile("android/app/build.gradle")
	if err != nil {
		t.Skipf("android/app/build.gradle is not in this tree: %v", err)
	}
	for _, flavor := range []string{"standard", "fdroid"} {
		if !regexp.MustCompile(`(?m)^\s*` + flavor + `\s*\{`).MatchString(gradle) {
			t.Errorf("build.gradle no longer declares the %q flavor. Each build "+
				"block of metadata/net.basov.omngo.fdroid.yml names it.", flavor)
		}
	}
}

// ----------------------------------------------------------------------
// The test itself
// ----------------------------------------------------------------------

// OmnConfig must import no Android package.
//
// org.json is part of the Android framework, and a plain Java virtual
// machine cannot load it. One such import makes this class untestable
// outside an emulator, and TestJavaUnitTests below then fails to compile
// it. That failure would read as a broken test and not as a broken rule,
// thus this test names the rule.
//
// THE DEFAULTS ARE NOT TESTED HERE. ports_test.go already compares each
// one against the Go value: TestFullscreenModeHasAJavaCopy for the three
// modes and TestUploadLimitHasAJavaCopy for the upload cap. A second
// comparison would be a second authority. See rule 7 of CLAUDE.md
// section 1.
func TestOmnConfigImportsNoAndroidPackage(t *testing.T) {
	src, err := readRepoFile("android/app/src/main/java/net/basov/omngo/OmnConfig.java")
	if err != nil {
		t.Skipf("OmnConfig.java is not in this tree: %v", err)
	}
	bad := regexp.MustCompile(`(?m)^import\s+(android|androidx|org\.json)[.;]`).
		FindAllString(src, -1)
	for _, line := range bad {
		t.Errorf("OmnConfig.java holds %q. A plain JVM cannot load it, thus the "+
			"test below cannot run and the Android layer loses its only test.",
			strings.TrimSpace(line))
	}
	// It must also stay off the Android classes by their full name, which
	// needs no import line.
	//
	// The scan reads the CODE and not the comments. The banner of that
	// file names org.json to say why the file avoids it. A scan of the
	// whole text would read that sentence as the fault that it warns of.
	for _, line := range strings.Split(src, "\n") {
		code := line
		if at := strings.Index(code, "//"); at >= 0 {
			code = code[:at]
		}
		for _, name := range []string{"org.json.", "android.content.", "android.os.", "android.view."} {
			if strings.Contains(code, name) {
				t.Errorf("OmnConfig.java names %s in code: %s\n"+
					"Keep the class free of the Android framework.",
					name, strings.TrimSpace(line))
			}
		}
	}
}

// TestJavaUnitTests compiles OmnConfig.java together with its test and
// runs the test.
//
// It SKIPS on a machine with no JDK. The Docker image of the gate has one,
// because the Gradle build needs it, thus this test runs there. The image
// needs no change and no Dockerfile changes.
//
// The test class is a plain main method with its own check helpers. No
// JUnit, thus no Gradle dependency. See the banner of OmnConfigTest.java.
func TestJavaUnitTests(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("no javac on this machine. The Docker gate has a JDK and runs this test.")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("no java on this machine")
	}

	const (
		mainSrc = "android/app/src/main/java/net/basov/omngo/OmnConfig.java"
		testSrc = "android/test/java/net/basov/omngo/OmnConfigTest.java"
	)
	for _, rel := range []string{mainSrc, testSrc} {
		if _, sErr := os.Stat(filepath.Join("..", filepath.FromSlash(rel))); sErr != nil {
			t.Fatalf("%s is missing: %v", rel, sErr)
		}
	}

	// -encoding UTF-8, because a raw javac reads a source file in the
	// encoding of the PLATFORM. The build image of the gate has no UTF-8
	// locale, thus its javac read the file as US-ASCII and refused each
	// byte above 127. Version 26.09.28 failed the build that way.
	//
	// The Gradle build never had that fault. The Android Gradle Plugin
	// sets options.encoding to UTF-8 for each JavaCompile task, thus
	// MainActivity.java has carried a character above 127 in a comment
	// for a long time. This line makes the two compilers agree.
	out := t.TempDir()
	build := exec.Command(javac, "-encoding", "UTF-8", "-d", out,
		filepath.Join("..", filepath.FromSlash(mainSrc)),
		filepath.Join("..", filepath.FromSlash(testSrc)))
	if compiled, cErr := build.CombinedOutput(); cErr != nil {
		t.Fatalf("javac failed: %v\n%s", cErr, compiled)
	}

	run := exec.Command(java, "-cp", out, "net.basov.omngo.OmnConfigTest")
	result, runErr := run.CombinedOutput()
	// javac and java write a line about JAVA_TOOL_OPTIONS on some
	// machines. It is not a fault, and the exit code is the answer.
	if runErr != nil {
		t.Errorf("the Java test failed: %v\n%s", runErr, result)
		return
	}
	t.Logf("%s", strings.TrimSpace(string(result)))
}

// THE CALL SITES OF MainActivity MUST TYPE-CHECK AGAINST OmnConfig.
//
// TestJavaUnitTests compiles OmnConfig.java and its test, and it compiles
// NEITHER caller. Version 26.09.29 therefore passed the whole Go gate with
// four call sites that could not compile: storageDir() answers a String,
// and OmnConfig took a File. Gradle found it, twenty minutes later.
//
// Compiling MainActivity itself is not the answer. It needs android.jar,
// which only the build image has, and it needs R and BuildConfig, which
// Gradle generates from the resources. A stub for those would be a pile
// of fakes that drifts away from the real ones.
//
// This test compiles the REAL call sites instead. It reads each
// OmnConfig call out of MainActivity, and it reads the return type of
// storageDir() from that same file. It then writes a small class that
// holds those lines. javac answers the question that matters: does the
// argument of each call fit the parameter that OmnConfig declares.
//
// It is a real type check by a real compiler, and it needs no Android SDK.
func TestAndroidConfigCallSitesTypeCheck(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("no javac on this machine. The Docker gate has a JDK and runs this test.")
	}
	main, err := readRepoFile("android/app/src/main/java/net/basov/omngo/MainActivity.java")
	if err != nil {
		t.Skipf("MainActivity.java is not in this tree: %v", err)
	}

	// The return type of the helper that each call site passes.
	sd := regexp.MustCompile(`(?m)^\s*private\s+(\w+)\s+storageDir\(\)`).FindStringSubmatch(main)
	if sd == nil {
		t.Fatal("MainActivity.java declares no storageDir(). This test reads its " +
			"return type, thus it cannot check the call sites without it.")
	}
	storageDirType := sd[1]

	// Each statement that calls OmnConfig, taken whole.
	calls := regexp.MustCompile(`OmnConfig\.\w+\([^;]*\);`).FindAllString(main, -1)
	if len(calls) == 0 {
		t.Fatal("MainActivity.java calls OmnConfig nowhere. Either the calls moved, " +
			"or the three readers came back. See the banner of OmnConfig.java.")
	}

	// One class that holds each call. Each call answers a value, thus each
	// one is assigned, and a numbered name keeps the assignments apart.
	var body strings.Builder
	body.WriteString("package net.basov.omngo;\n\n")
	body.WriteString("// Written by TestAndroidConfigCallSitesTypeCheck.\n")
	body.WriteString("// It is not a file of this project, and nothing ships it.\n")
	body.WriteString("final class CallSiteCheck {\n")
	body.WriteString("    private " + storageDirType + " storageDir() { return null; }\n")
	body.WriteString("    @SuppressWarnings(\"unused\")\n")
	body.WriteString("    void use(String key) {\n")
	for i, c := range calls {
		fmt.Fprintf(&body, "        Object v%d = %s\n", i, c)
	}
	body.WriteString("    }\n}\n")
	src := body.String()

	dir := t.TempDir()
	checkSrc := filepath.Join(dir, "CallSiteCheck.java")
	if err := os.WriteFile(checkSrc, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	out, cErr := exec.Command(javac, "-encoding", "UTF-8", "-d", dir,
		filepath.Join("..", filepath.FromSlash(
			"android/app/src/main/java/net/basov/omngo/OmnConfig.java")),
		checkSrc).CombinedOutput()
	if cErr != nil {
		t.Errorf("the OmnConfig call sites of MainActivity do not compile:\n%s\n"+
			"storageDir() answers %s. Each parameter of OmnConfig must take that "+
			"type, or each call site must convert.\nThe checked source was:\n%s",
			out, storageDirType, src)
	}
}

// The two files that a raw javac reads must hold no byte above 127.
//
// TestJavaUnitTests passes -encoding UTF-8, thus a byte above 127 would
// compile today. This test is the second lock, and it exists because the
// first one already failed once.
//
// Each other Java file of this project is compiled by Gradle alone, and
// the Android Gradle Plugin fixes the encoding for those. MainActivity
// holds an ellipsis in a comment for that reason. The two files below are
// compiled by BOTH, thus they must satisfy the stricter of the two.
//
// A character above 127 has a plain replacement in Java source: the
// escape \uXXXX. OmnConfigTest uses it for the value it expects, which is
// also the shape that json.MarshalIndent writes.
func TestCompiledJavaSourcesAreASCII(t *testing.T) {
	for _, rel := range []string{
		"android/app/src/main/java/net/basov/omngo/OmnConfig.java",
		"android/test/java/net/basov/omngo/OmnConfigTest.java",
	} {
		src, err := readRepoFile(rel)
		if err != nil {
			t.Skipf("%s is not in this tree: %v", rel, err)
		}
		for i, line := range strings.Split(src, "\n") {
			for _, r := range line {
				if r > 127 {
					t.Errorf("%s:%d holds the character %q. A javac with no UTF-8 "+
						"locale refuses it. Write it as the escape \\u%04X.",
						rel, i+1, r, r)
					break
				}
			}
		}
	}
}
