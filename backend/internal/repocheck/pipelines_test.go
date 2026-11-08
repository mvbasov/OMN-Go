package repocheck

// ----------------------------------------------------------------------
// The three build pipelines must agree
// ----------------------------------------------------------------------
//
// OMN-Go has three builds. The local build, GitHub and GitLab use the one
// Dockerfile. F-Droid uses its own recipe, and
// metadata/net.basov.omngo.fdroid.yml holds a copy of it. The gomobile
// steps of the Dockerfile are in android/build_aar.sh. See
// doc/decisions/0027-build-the-android-library-with-one-script.md.
//
// The maintainer keeps the three builds as similar as possible. The
// F-Droid recipe is hard to change. The tests below read the files and
// compare the values that change the binary. These values are the Android
// API level, the Go version, the NDK, the build-tools, the Gradle version
// and the x/mobile version. A difference in the Android API level can make an APK
// that installs on Android 6 and then fails to load its library.
//
// The tests read the LAST version in the F-Droid file. An older build
// block keeps the values of its own release. When the recipe on the
// F-Droid server changes, copy it into the repository. These tests then
// show each difference.

import (
	"regexp"
	"strings"
	"testing"
)

const (
	pipeDockerfile = "Dockerfile"
	pipeScript     = "android/build_aar.sh"
	pipeGradle     = "android/app/build.gradle"
	pipeWrapper    = "android/gradle/wrapper/gradle-wrapper.properties"
)

var (
	pipeMinSdkRe     = regexp.MustCompile(`(?m)^\s*minSdk\s+(\d+)\s*$`)
	pipeNdkGradleRe  = regexp.MustCompile(`(?m)^\s*ndkVersion\s+"([^"]+)"`)
	pipeAndroidAPIRe = regexp.MustCompile(`-androidapi\s+(\d+)`)
	pipeGoImageRe    = regexp.MustCompile(`(?m)^FROM golang:([0-9.]+)-trixie\b`)
	pipeNdkDockerRe  = regexp.MustCompile(`"ndk;([^"]+)"`)
	pipeGoSrclibRe   = regexp.MustCompile(`go@go([0-9.]+)`)
	pipeNdkRecipeRe  = regexp.MustCompile(`(?m)^\s*ndk:\s*(\S+)`)
	pipeBtGradleRe   = regexp.MustCompile(`(?m)^\s*buildToolsVersion\s+"([^"]+)"`)
	pipeBtDockerRe   = regexp.MustCompile(`"build-tools;([^"]+)"`)
	pipeGradleZipRe  = regexp.MustCompile(`gradle-([0-9.]+)-bin\.zip`)
)

// pipeRead reads one file of the repository, or fails the test.
func pipeRead(t *testing.T, rel string) string {
	t.Helper()
	text, err := readRepoFile(rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return text
}

// pipeAll answers the first group of each match of re in text.
func pipeAll(re *regexp.Regexp, text string) []string {
	var out []string
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

// pipeLastRecipeBlocks answers the build blocks of the last versionName
// in the F-Droid file. Each ABI has its own block, thus there are
// several blocks for one version.
func pipeLastRecipeBlocks(t *testing.T) []string {
	t.Helper()
	recipe := pipeRead(t, "metadata/net.basov.omngo.fdroid.yml")
	builds := recipe
	if i := strings.Index(recipe, "\nBuilds:"); i >= 0 {
		builds = recipe[i:]
	}
	if i := strings.Index(builds, "\nAutoUpdateMode:"); i >= 0 {
		builds = builds[:i]
	}
	var blocks []string
	for _, part := range strings.Split(builds, "\n  - versionName:")[1:] {
		blocks = append(blocks, "versionName:"+part)
	}
	if len(blocks) == 0 {
		t.Fatal("the F-Droid file holds no build block")
	}
	name := func(b string) string {
		return strings.TrimSpace(strings.SplitN(strings.TrimPrefix(b, "versionName:"), "\n", 2)[0])
	}
	last := name(blocks[len(blocks)-1])
	var out []string
	for _, b := range blocks {
		if name(b) == last {
			out = append(out, b)
		}
	}
	return out
}

// pipeSame fails the test when the values differ from want.
func pipeSame(t *testing.T, what, where, want string, got []string) {
	t.Helper()
	if len(got) == 0 {
		t.Errorf("%s: %s names no value, want %s", what, where, want)
	}
	for _, g := range got {
		if g != want {
			t.Errorf("%s: %s says %s, want %s", what, where, g, want)
		}
	}
}

// pipeFirst answers the first group of re in the file, or fails the test.
func pipeFirst(t *testing.T, re *regexp.Regexp, rel, what string) string {
	t.Helper()
	m := re.FindStringSubmatch(pipeRead(t, rel))
	if m == nil {
		t.Fatalf("%s names no %s", rel, what)
	}
	return m[1]
}

// The Android API level of the native library must be minSdk. The
// library of a higher level can fail to load on the lowest Android that
// installs the APK. minSdk 23 is Android 6.
func TestPipelinesAgreeOnTheAndroidAPILevel(t *testing.T) {
	want := pipeFirst(t, pipeMinSdkRe, pipeGradle, "minSdk")
	pipeSame(t, "-androidapi", pipeScript, want, pipeAll(pipeAndroidAPIRe, pipeRead(t, pipeScript)))
	for _, b := range pipeLastRecipeBlocks(t) {
		pipeSame(t, "-androidapi", "the last F-Droid version", want, pipeAll(pipeAndroidAPIRe, b))
	}
}

// The Docker image and the F-Droid srclib must name the same Go version.
// A second Go version can build a different binary from the same source.
// The image must be a trixie image, because the F-Droid server runs
// Debian trixie. The regular expression thus finds no other image.
func TestPipelinesAgreeOnTheGoVersion(t *testing.T) {
	got := pipeAll(pipeGoImageRe, pipeRead(t, pipeDockerfile))
	if len(got) == 0 {
		t.Fatal("the Dockerfile names no golang:<version>-trixie image")
	}
	want := got[0]
	pipeSame(t, "the Go version", pipeDockerfile, want, got)
	for _, b := range pipeLastRecipeBlocks(t) {
		pipeSame(t, "the Go version", "the last F-Droid version", want, pipeAll(pipeGoSrclibRe, b))
	}
}

// The NDK of the Gradle file, of the Docker image and of the F-Droid
// recipe must be the same. The NDK builds the native library.
func TestPipelinesAgreeOnTheNDK(t *testing.T) {
	want := pipeFirst(t, pipeNdkGradleRe, pipeGradle, "ndkVersion")
	pipeSame(t, "the NDK", pipeDockerfile, want, pipeAll(pipeNdkDockerRe, pipeRead(t, pipeDockerfile)))
	for _, b := range pipeLastRecipeBlocks(t) {
		pipeSame(t, "the NDK", "the last F-Droid version", want, pipeAll(pipeNdkRecipeRe, b))
	}
}

// The build-tools of the Docker image must be the version that
// build.gradle pins. Without that package in the image, Gradle downloads
// it on each build. The F-Droid recipe is not compared yet: its prebuild
// line names a version that the build does not use. See
// doc/decisions/0027-build-the-android-library-with-one-script.md.
func TestPipelinesAgreeOnTheBuildTools(t *testing.T) {
	want := pipeFirst(t, pipeBtGradleRe, pipeGradle, "buildToolsVersion")
	pipeSame(t, "the build-tools", pipeDockerfile, want, pipeAll(pipeBtDockerRe, pipeRead(t, pipeDockerfile)))
}

// The Docker image installs Gradle from a zip file. F-Droid reads the
// version from gradle-wrapper.properties. The two must be the same
// version.
func TestGradleVersionsAgree(t *testing.T) {
	want := pipeFirst(t, pipeGradleZipRe, pipeWrapper, "Gradle distribution")
	pipeSame(t, "the Gradle version", pipeDockerfile, want, pipeAll(pipeGradleZipRe, pipeRead(t, pipeDockerfile)))
}

var (
	pipeBindRe     = regexp.MustCompile(`(?m)^\s*gomobile bind [^\n]*`)
	pipeLdflagsRe  = regexp.MustCompile(`-ldflags="[^"]*"`)
	pipeBindDirRe  = regexp.MustCompile(`(?m)^BIND_DIR=/[^\s$]+$`)
	pipeXMobileRe  = regexp.MustCompile(`(?m)^XMOBILE_VERSION=(v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?)$`)
	pipeGetToolRe  = regexp.MustCompile(`(?m)^\s*go get -tool`)
	pipeToolPathRe = regexp.MustCompile(`"golang\.org/x/mobile/cmd/(gobind|gomobile)@\$XMOBILE_VERSION"`)
)

// Two builds of one commit must give the same APK. gomobile writes the
// generated Go code into a new temporary directory for each run. Only the
// -trimpath flag of gomobile removes that path from libgojni.so. In
// -ldflags, -trimpath stops the build. The Android Gradle plugin encrypts
// its dependency list with a new random key for each build. Thus
// build.gradle must remove the list from the APK.
//
// Go also records the directory of the module in libgojni.so, because
// gomobile names it in a replace line. -trimpath does not remove that
// path. The script thus runs the bind step from one fixed path on each
// host.
func TestAndroidBuildIsTheSameOnEachHost(t *testing.T) {
	script := pipeRead(t, pipeScript)
	if !pipeBindDirRe.MatchString(script) || !strings.Contains(script, `cd "$BIND_DIR"`) {
		t.Errorf("%s does not run the bind step from a fixed BIND_DIR=/<path>", pipeScript)
	}
	lines := pipeBindRe.FindAllString(script, -1)
	if len(lines) != 1 {
		t.Errorf("%s has %d gomobile bind lines, want 1", pipeScript, len(lines))
	}
	for _, line := range lines {
		if !strings.Contains(pipeLdflagsRe.ReplaceAllString(line, ""), " -trimpath ") {
			t.Errorf("%s: gomobile bind has no -trimpath flag of its own: %s", pipeScript, line)
		}
	}
	gradle := pipeRead(t, pipeGradle)
	for _, want := range []string{"includeInApk = false", "includeInBundle = false"} {
		if !strings.Contains(gradle, want) {
			t.Errorf("%s has no %q in dependenciesInfo", pipeGradle, want)
		}
	}
}

// The x/mobile version has one place: XMOBILE_VERSION in the script. A
// "go get -tool" with no version takes the newest x/mobile. That can also
// raise golang.org/x/crypto, x/net and x/sys in libgojni.so, thus two
// builds of one commit on two days differ. GOTOOLCHAIN=local stops the
// build when a module asks for a newer Go, in place of a silent download.
// The Dockerfile must call the script, and it must hold no gomobile step
// of its own.
func TestXMobileHasOnePinnedVersion(t *testing.T) {
	script := pipeRead(t, pipeScript)
	if !pipeXMobileRe.MatchString(script) {
		t.Errorf("%s has no XMOBILE_VERSION=<module version> line", pipeScript)
	}
	if n, m := len(pipeGetToolRe.FindAllString(script, -1)), len(pipeToolPathRe.FindAllString(script, -1)); n != 1 || m != 2 {
		t.Errorf("%s: want one go get -tool with gobind and gomobile at $XMOBILE_VERSION, got %d lines and %d pinned paths", pipeScript, n, m)
	}
	if !strings.Contains(script, "export GOTOOLCHAIN=local") {
		t.Errorf("%s does not set GOTOOLCHAIN=local", pipeScript)
	}
	docker := pipeRead(t, pipeDockerfile)
	for _, banned := range []string{"go get -tool", "gomobile bind", "gomobile init"} {
		if strings.Contains(docker, banned) {
			t.Errorf("the Dockerfile runs %q itself. Put the step into %s", banned, pipeScript)
		}
	}
	for _, step := range []string{"tools", "init", "bind"} {
		if !strings.Contains(docker, "sh "+pipeScript+" "+step) {
			t.Errorf("the Dockerfile does not run \"sh %s %s\"", pipeScript, step)
		}
	}
}
