package main

import "testing"

// Creating a virtual user credits every asset and leaves one ledger row per
// asset, so the account reconciles against the ledger from the moment it exists.
func TestCreatingAVirtualUserCreditsEveryAsset(t *testing.T) {
	captured := captureDbMessages(t)
	e := newTestEngine(t)

	e.creditAllAssets("alice", 100000, "txn-1")

	for _, asset := range allAssets() {
		assertClose(t, asset+" available", bal(t, e, "alice", asset).Available, 100000)
	}

	entries := ledgerEntries(*captured)
	if want := len(allAssets()); len(entries) != want {
		t.Fatalf("got %d ledger entries, want %d (one per asset)", len(entries), want)
	}
	refs := map[string]bool{}
	for _, entry := range entries {
		if entry.Reason != LEDGER_DEPOSIT {
			t.Errorf("reason = %q, want %q", entry.Reason, LEDGER_DEPOSIT)
		}
		if refs[entry.RefID] {
			t.Errorf("duplicate ref_id %q - one of these credits would be deduped away", entry.RefID)
		}
		refs[entry.RefID] = true
	}
}

func TestNextUserIDSkipsIDsAlreadyInUse(t *testing.T) {
	e := newTestEngine(t)
	e.Users["1"] = "Demo user 1"
	fund(e, "5", 0, 0) // a balance with no name, as the seed script leaves behind

	if got := e.nextUserID(); got != "6" {
		t.Errorf("nextUserID() = %q, want %q", got, "6")
	}
}
