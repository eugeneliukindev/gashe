# One complete Backend contract

`Backend` is a single interface that holds every operation the strategies need (values, counters,
locks, sets), not a small core plus optional interfaces. Middleware and other wrappers wrap a
backend by embedding it, so optional interfaces would make every wrapper lose capabilities.
The contract was narrowed once before the first release: sliding windows and bit fields left it
when the circuit breaker and the sliding rate limit moved to plain counters and Bloom filters were
dropped. After the first release it only grows through new interfaces, never by changing this one.
