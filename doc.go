// Package gashe is a caching library: it stores results of expensive calls in a backend and
// protects callers from slow or failing dependencies with strategies built on top of it.
//
// A Cache turns values into bytes with a Codec and stores them in a Backend. A strategy, such
// as Cached, is created once with its options and called with a key and a loader; it decides
// whether to answer from the cache or to run the loader.
package gashe
