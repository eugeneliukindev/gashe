package gashe

// The keys a Cache keeps for itself start with an underscore, so they never meet keys of yours:
// keys that start with "_" are reserved. Options.Prefix comes before them like before any key.
const (
	ownTag     = "_tag:"          // the generation counter of a tag
	ownLoad    = "_load:"         // the LockOnMiss lock of an entry
	ownRefresh = "_refresh:"      // the lock of the one reload of a stale entry
	ownSliding = "_rate:sliding:" // a SlidingWindow counter, followed by the window number
	ownFixed   = "_rate:fixed:"   // a FixedWindow counter
)
