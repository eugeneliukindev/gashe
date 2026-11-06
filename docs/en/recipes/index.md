# Recipes { #recipes }

Task-oriented how-tos: each page is one task and the code for it, ready to copy.

| Recipe | The task |
|---|---|
| [Cache a database query](postgres.md#cache-a-database-query) | Postgres rows in Redis, right again after an `UPDATE` |
| [Cache an HTTP API](http-client.md#cache-an-http-api) | answers of an external API, the last one while it is down, `404` cached |
| [Export metrics](metrics.md#export-metrics) | hit ratio and loader latency in Prometheus or OpenTelemetry |
| [Test code that uses gashe](testing.md#test-code-that-uses-gashe) | memory backend, fake time for TTLs, a backend that fails, your own backend |
| [Change a cached type](changing-types.md#change-a-cached-type) | deploy a new version of a cached struct, and what it costs |
| [Warm up the cache](warm-up.md#warm-up-the-cache) | fill an empty cache after a deploy or in a new region, without flooding the database |
