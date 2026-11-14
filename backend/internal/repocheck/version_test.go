package repocheck

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// The version of each file that a bump changes. Each pattern starts at the
// line start, thus versionNameSuffix and a comment never match.
var (
	versionGoRe      = regexp.MustCompile(`(?m)^const APP_VERSION = "(\d+)\.(\d+)\.(\d+)"$`)
	versionNameRe    = regexp.MustCompile(`(?m)^\s*versionName "([^"]+)"\s*$`)
	versionCodeRe    = regexp.MustCompile(`(?m)^\s*versionCode (\d+)\s*$`)
	versionCodeMapRe = regexp.MustCompile(`(?m)^State of the tree: version ([0-9.]+)\.`)
)

// Each commit types the version in three files by hand. A bump that misses
// one file gives an APK or a document with the version of an older commit.
//
// The Docker build has no doc/CODE_MAP.md, and a tree can have no android/.
// The test then checks the files that are present.
func TestVersionIsTheSameInEachFile(t *testing.T) {
	one := func(rel string, re *regexp.Regexp) []string {
		t.Helper()
		src, err := readRepoFile(rel)
		if os.IsNotExist(err) && rel != "backend/version.go" {
			t.Logf("%s is not in this tree", rel)
			return nil
		}
		if err != nil {
			t.Fatalf("cannot read %s: %v", rel, err)
		}
		all := re.FindAllStringSubmatch(src, -1)
		if len(all) != 1 {
			t.Fatalf("%s: the pattern %s matches %d times, want 1", rel, re, len(all))
		}
		return all[0]
	}

	goVersion := one("backend/version.go", versionGoRe)
	version := goVersion[1] + "." + goVersion[2] + "." + goVersion[3]
	var parts [3]int
	for i := range parts {
		n, err := strconv.Atoi(goVersion[i+1])
		if err != nil {
			t.Fatalf("APP_VERSION %s: %v", version, err)
		}
		parts[i] = n
	}

	if m := one("android/app/build.gradle", versionNameRe); m != nil && m[1] != version {
		t.Errorf("android/app/build.gradle: versionName is %s, want %s", m[1], version)
	}
	wantCode := strconv.Itoa(parts[0]*100000 + parts[1]*1000 + parts[2])
	if m := one("android/app/build.gradle", versionCodeRe); m != nil && m[1] != wantCode {
		t.Errorf("android/app/build.gradle: versionCode is %s, want %s for %s", m[1], wantCode, version)
	}
	if m := one("doc/CODE_MAP.md", versionCodeMapRe); m != nil && m[1] != version {
		t.Errorf("doc/CODE_MAP.md: the state of the tree is version %s, want %s", m[1], version)
	}
}
