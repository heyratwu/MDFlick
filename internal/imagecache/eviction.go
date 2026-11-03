package imagecache

// EvictionMode identifies the constraint an eviction pass is trying to satisfy.
type EvictionMode uint8

const (
	TrimWarm EvictionMode = iota
	AdmitEntry
	ReserveBytes
)

// EvictionPolicy shares cache thresholds without owning entries or locks.
// Callers protect visible and unfinished entries, and clean up their own state.
type EvictionPolicy struct {
	WarmBytes   int64
	WarmEntries int
	ByteLimit   int64
	EntryLimit  int
}

// NeedsEviction reports whether another eligible offscreen entry is needed.
// entries includes visible, pending and failed entries. WarmEntries is exclusive,
// preserving room for the next entry; WarmBytes is inclusive.
func (p EvictionPolicy) NeedsEviction(mode EvictionMode, used int64, entries int, incoming, available int64) bool {
	switch mode {
	case TrimWarm:
		return used > p.WarmBytes || entries >= p.WarmEntries
	case AdmitEntry:
		return entries >= p.EntryLimit
	case ReserveBytes:
		return incoming > p.ByteLimit-used || incoming > available
	default:
		panic("invalid cache eviction mode")
	}
}
