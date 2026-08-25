---
title: Deployment Runbook
owner: platform
reviewed: 2026-08-10
---

# Deployment Runbook

This is the document we hand to whoever is on call. It exists because the last
outage was extended by twenty minutes of somebody reading a wiki page that had
been wrong since March.

## Before you start

Check these in order. Do not skip the second one because the first looked fine.

1. The build you intend to ship is green on `main`.
2. The migration in that build has been run against staging **and** rolled back
   once, successfully.
3. Someone else is awake.

## Rolling out

Take the current revision first, because you will want it in a hurry later and
the console is slow when you are stressed:

```sh
kubectl get deploy/api -o jsonpath='{.metadata.annotations.revision}'
kubectl rollout status deploy/api --timeout=90s
```

```sh
kubectl set image deploy/api api=registry.internal/api:"$TAG"
kubectl rollout status deploy/api --timeout=180s
```

Two blocks, deliberately. The first is what you run before touching anything;
the second is the change itself. Running them as one paste is how the wrong
image got shipped in February.

### If it does not come up

The readiness probe is the thing that usually fails, and it fails for a boring
reason:

```go
func (s *Server) Ready(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		// A pool that has not warmed up yet is not the same as a pool that is
		// broken, but this handler cannot tell the difference, so it reports
		// unready and lets the orchestrator retry.
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
}
```

> If the probe is failing and the database is fine, look at the connection pool
> size before you look at anything else. It has been the pool three times out of
> four.

## Rollback

| Symptom            | Action                  | Escalate after |
| ------------------ | ----------------------- | -------------- |
| 5xx above baseline | Roll back immediately   | 0 min          |
| Latency doubled    | Roll back, then look    | 5 min          |
| One pod crashloops | Drain it, keep watching | 15 min         |

Rolling back is not an admission of anything. It is the cheap option, and it
stays cheap only while it is fast.

---

## Notes

- The runbook lives with the service, not in the wiki. See [the ADR][adr] for
  why.
- `TAG` is the immutable digest tag, never `latest`.
- Anything here that turns out to be wrong should be fixed in the same hour it
  is discovered.

[adr]: https://internal.example.com/adr/0042
