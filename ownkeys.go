package gashe

// The keys a Cache keeps for itself start with two underscores, so they never meet keys of yours:
// keys that start with "__" are reserved. Options.Prefix comes before them like before any key.
const (
	ownTag     = "__tag:"          // the generation counter of a tag
	ownLoad    = "__load:"         // the LockOnMiss lock of an entry
	ownRefresh = "__refresh:"      // the lock of the one reload of a stale entry
	ownSliding = "__rate:sliding:" // a SlidingWindow counter, followed by the window number
	ownFixed   = "__rate:fixed:"   // a FixedWindow counter
)
