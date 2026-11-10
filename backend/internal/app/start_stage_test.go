package app

import "testing"

// The start screen of Android shows the stage text as it is. A count with a
// whole count reads "what: done of total", and a count with no whole count
// reads "what: done".
func TestStartStageText(t *testing.T) {
	t.Cleanup(func() { setStartStage("") })

	setStartStage("Reading the settings")
	if got := StartStage(); got != "Reading the settings" {
		t.Errorf("StartStage = %q", got)
	}
	startProgress("Updating the application files")(34, 120)
	if got, want := StartStage(), "Updating the application files: 34 of 120"; got != want {
		t.Errorf("StartStage = %q, want %q", got, want)
	}
	startProgress("Removing the compiled pages of the old version")(7, 0)
	if got, want := StartStage(), "Removing the compiled pages of the old version: 7"; got != want {
		t.Errorf("StartStage = %q, want %q", got, want)
	}
}
