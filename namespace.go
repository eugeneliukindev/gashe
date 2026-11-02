package gashe

// defaultNamespace is where a Cache keeps its own keys unless Options.Namespace says otherwise.
const defaultNamespace = "gashe:"

// The kinds of keys a Cache keeps under its namespace, each followed by a tag or a key.
const (
	spaceTag     = "tag:"          // the generation counter of a tag
	spaceLoad    = "load:"         // the LockOnMiss lock of an entry
	spaceRefresh = "refresh:"      // the lock of the one reload of a stale entry
	spaceSliding = "rate:sliding:" // a SlidingWindow counter, followed by the window number
	spaceFixed   = "rate:fixed:"   // a FixedWindow counter
)

// ownKey returns the key of one of the Cache's own entries: a tag counter, a lock or a counter.
func (c *Cache) ownKey(space, key string) string { return c.ns + space + key }
