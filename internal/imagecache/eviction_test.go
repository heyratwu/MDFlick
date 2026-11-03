package imagecache

import "testing"

func TestEvictionPolicyBoundaries(t *testing.T) {
	p := EvictionPolicy{WarmBytes: 8, WarmEntries: 64, ByteLimit: 16, EntryLimit: 128}
	for _, tc := range []struct {
		name      string
		mode      EvictionMode
		used      int64
		entries   int
		incoming  int64
		available int64
		want      bool
	}{
		{"warm inclusive bytes", TrimWarm, 8, 63, 0, 0, false},
		{"warm excess bytes", TrimWarm, 9, 63, 0, 100, true},
		{"warm exclusive entries", TrimWarm, 0, 64, 0, 100, true},
		{"warm ignores hard pressure", TrimWarm, 8, 63, 100, 0, false},
		{"entry room", AdmitEntry, 16, 127, 1, 0, false},
		{"entry full", AdmitEntry, 0, 128, 0, 100, true},
		{"bytes exact fit", ReserveBytes, 8, 128, 8, 8, false},
		{"document pressure", ReserveBytes, 9, 1, 8, 100, true},
		{"application pressure", ReserveBytes, 0, 1, 8, 7, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.NeedsEviction(tc.mode, tc.used, tc.entries, tc.incoming, tc.available); got != tc.want {
				t.Fatalf("NeedsEviction = %v, want %v", got, tc.want)
			}
		})
	}
}
