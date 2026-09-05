package backend

// Tests for the per-flavor default server port.
//
// The Android standard and fdroid flavors are installable side by side, which
// is the whole point of the separate applicationId - but the loopback port is
// a device-global resource. With both defaulting to 8080 whichever app starts
// second loses the bind, and its WebView then silently talks to the OTHER
// app's server. build.gradle therefore passes 8081 for the fdroid flavor.
//
// It never arrived. StartServer applied the caller's default AFTER
// loadConfig, which had already put a positive 8080 in the config and written
// it to disk - so the branch could not be reached, and the wrong port was
// persisted on first run and stuck for the life of the install.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Each test below calls loadConfig itself, thus each one starts from an
// App that loaded no configuration. newTestApp loads one since 26.09.18,
// and a second load would then read the file that the first load wrote.

func portOfConfigFile(t *testing.T, a *App) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatalf("no config.json written: %v", err)
	}
	var onDisk struct {
		ServerPort int `json:"server_port"`
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	return onDisk.ServerPort
}

// The case that was broken: a fresh install of a flavor that asked for 8081.
func TestFreshInstallUsesTheCallerSuppliedPort(t *testing.T) {
	a := newUnconfiguredApp(t)
	a.defaultPort = 8081
	a.loadConfig(a.StorageDir)

	if a.Config.ServerPort != 8081 {
		t.Errorf("in-memory port %d, want 8081", a.Config.ServerPort)
	}
	// And it must be PERSISTED. The value written on first run is the one the
	// Config page shows and the one every later start reads; writing 8080 here
	// is what made the bug outlive a fix.
	if got := portOfConfigFile(t, a); got != 8081 {
		t.Errorf("config.json carries server_port %d, want 8081", got)
	}
}

// Desktop passes 0 and must keep the historical default.
func TestFreshInstallWithNoCallerDefaultStaysOn8080(t *testing.T) {
	a := newUnconfiguredApp(t)
	a.loadConfig(a.StorageDir)

	if a.Config.ServerPort != 8080 {
		t.Errorf("port %d, want 8080", a.Config.ServerPort)
	}
	if got := portOfConfigFile(t, a); got != 8080 {
		t.Errorf("config.json carries %d, want 8080", got)
	}
}

// A port the user chose outranks the flavor's default - that is the contract
// the build.gradle comment states, and the reason the default is only a
// default.
func TestConfiguredPortBeatsTheFlavorDefault(t *testing.T) {
	a := newUnconfiguredApp(t)
	writeConfigJSON(t, a, `{"server_port":9000}`)
	a.defaultPort = 8081
	a.loadConfig(a.StorageDir)

	if a.Config.ServerPort != 9000 {
		t.Errorf("port %d, want the configured 9000", a.Config.ServerPort)
	}
}

// A config.json predating server_port, or carrying nonsense, falls back the
// same way a fresh install does - to the FLAVOR default, not to 8080.
func TestConfigWithoutAPortFallsBackToTheFlavorDefault(t *testing.T) {
	for _, src := range []string{`{}`, `{"server_port":0}`, `{"server_port":-1}`} {
		a := newUnconfiguredApp(t)
		writeConfigJSON(t, a, src)
		a.defaultPort = 8081
		a.loadConfig(a.StorageDir)

		if a.Config.ServerPort != 8081 {
			t.Errorf("%s gave port %d, want 8081", src, a.Config.ServerPort)
		}
	}
}

func TestFallbackPort(t *testing.T) {
	cases := map[int]int{0: 8080, -1: 8080, 8081: 8081, 9999: 9999}
	for in, want := range cases {
		a := &App{defaultPort: in}
		if got := a.fallbackPort(); got != want {
			t.Errorf("defaultPort %d gave %d, want %d", in, got, want)
		}
	}
}

func writeConfigJSON(t *testing.T, a *App, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(a.StorageDir, "config.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
