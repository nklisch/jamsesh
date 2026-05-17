---
id: bug-static-discoverer-empty-publish
created: 2026-05-17
tags: [bug, infra]
---

`staticDiscoverer.Run` never publishes an initially-empty pod set because the change-detection key for an empty healthy slice (`""`) matches the zero-value of `prev`, so `publish` is never called when all pods are unhealthy on the first probe pass. This causes `TestStaticDiscoverer_BecomesHealthy` to fail consistently at the "no initial publish within deadline" assertion. Fix: initialise `prev` to a sentinel string (e.g. `"\x00"`) or unconditionally publish on the very first probe pass before enabling change-detection suppression — located in `internal/router/discovery/static.go` around line 29.
