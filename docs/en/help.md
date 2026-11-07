# Help { #help }

Where to look when something does not work, and where to ask.

| You have | Go to |
|---|---|
| a symptom: every request reaches the database, old data, a hang | [Troubleshooting](troubleshooting.md) |
| an error and want to know what returns it | [Errors](errors.md) |
| a question about how something works | the [Concepts](concepts/setup.md) and the [API reference](reference.md) |
| an AI assistant to ask about gashe | give it [llms.txt](llms.txt), the index of these pages, or [llms-full.txt](llms-full.txt), every page in one file |
| a bug | [an issue on GitHub](https://github.com/eugeneliukindev/gashe/issues/new) |
| a security problem | [a private report on GitHub](https://github.com/eugeneliukindev/gashe/security/advisories/new), not a public issue |

## Reporting a bug { #reporting-a-bug }

An issue is fixed fastest when it holds:

- the versions: gashe and its modules (`go list -m all | grep gashe`), Go, Redis or Valkey;
- the backend and the policy you use, as Go code;
- a short program that shows the problem, ideally on the memory backend;
- what you expected, and what happened, with the error or the `Result` from `Options.Hooks.OnResult`.
