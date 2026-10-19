package tg

import "testing"

func TestMergeDialogsFolders(t *testing.T) {
	// parseDialogPage sets Archived from each row's folder_id before the merge:
	// archived chats carry folder_id 1 (in BOTH the main and archive queries),
	// while pinned and main chats carry no folder id.
	main := []Dialog{
		{PeerID: 1, Title: "A"},                 // main only
		{PeerID: 2, Title: "B", Pinned: true},   // pinned -> comes back in both
		{PeerID: 3, Title: "C", Archived: true}, // archived, also in main query
	}
	archived := []Dialog{
		{PeerID: 2, Title: "B2", Pinned: true},   // pinned copy from archive query
		{PeerID: 3, Title: "C2", Archived: true}, // archived copy
		{PeerID: 4, Title: "D", Archived: true},  // archive only
	}

	out := mergeDialogs(main, archived)
	if len(out) != 4 {
		t.Fatalf("want 4 unique dialogs, got %d", len(out))
	}

	byID := map[int64]Dialog{}
	for _, d := range out {
		byID[d.PeerID] = d
	}
	if byID[1].Archived {
		t.Fatal("chat 1 is main, should not be archived")
	}
	if byID[2].Archived {
		t.Fatal("chat 2 is pinned (no folder id), must stay in All")
	}
	if byID[2].Title != "B" {
		t.Fatalf("chat 2 should keep the main copy, got %q", byID[2].Title)
	}
	if !byID[3].Archived {
		t.Fatal("chat 3 has folder_id 1, must be archived even though it was in the main query")
	}
	if !byID[4].Archived {
		t.Fatal("chat 4 is archive-only, should be archived")
	}
}
