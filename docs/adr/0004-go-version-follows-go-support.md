# Support a wide range of Go releases, from 1.23

The modules support Go 1.23 and newer, five releases at the time of writing, because a library is
built into other projects, and many of them are not on the newest Go. 1.23 is the oldest release
with iterators, which `Keys`, `All` and `Backend.Scan` use. Go 1.27 generic methods were tried and
dropped: they would let caching be a method of `Cache`, but only for users of the newest release,
while a generic function gives the same typed call on every supported version.
