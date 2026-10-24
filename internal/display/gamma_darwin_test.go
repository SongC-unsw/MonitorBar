//go:build darwin

package display

import "testing"

// Stub the transfer-table calls: these tests never dim a real display or
// reload its color profile.
func TestGammaRestoresOnlyAfterSuccessfulChange(t *testing.T) {
	load()
	oldBase, oldSet := gammaBase, gammaSet
	oldRestore, oldWrite := cgDisplayRestoreColorSyncSettings, cgSetDisplayTransferByTable
	t.Cleanup(func() {
		gammaBase, gammaSet = oldBase, oldSet
		cgDisplayRestoreColorSyncSettings, cgSetDisplayTransferByTable = oldRestore, oldWrite
	})
	restores, writes := 0, 0
	cgDisplayRestoreColorSyncSettings = func() { restores++ }
	cgSetDisplayTransferByTable = func(uint32, uint32, *float32, *float32, *float32) int32 {
		writes++
		return 0
	}
	gammaBase, gammaSet = [3][]float32{}, false
	m := Monitor{}
	if err := m.SetGamma(1, 0); err != nil {
		t.Fatal(err)
	}
	RestoreGamma()
	if restores != 0 || writes != 0 {
		t.Fatal("neutral settings touched the display's profile")
	}
	// Supply a captured base so dimming uses only the stubbed write.
	gammaBase = [3][]float32{{0, 1}, {0, 1}, {0, 1}}
	if err := m.SetGamma(0.5, 0.2); err != nil || !gammaSet {
		t.Fatalf("a successful change wasn't tracked: %v", err)
	}
	if err := m.SetGamma(1, 0); err != nil {
		t.Fatal(err)
	}
	RestoreGamma()
	if restores != 1 || writes != 1 || gammaSet {
		t.Fatalf("restores %d, writes %d, gammaSet %v; want 1, 1, false", restores, writes, gammaSet)
	}
	cgSetDisplayTransferByTable = func(uint32, uint32, *float32, *float32, *float32) int32 { return 1 }
	if err := m.SetGamma(0.5, 0); err == nil || gammaSet {
		t.Fatal("a failed write was tracked as a changed display")
	}
	RestoreGamma()
	if restores != 1 {
		t.Fatal("a failed gamma write triggered an unnecessary profile restore")
	}
}
