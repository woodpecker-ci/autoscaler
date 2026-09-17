# Autoscaler reconciliation tests

These tests run the real autoscaler through its public API with in-memory
Woodpecker and provider clients. They exercise complete reconcile cycles without
cloud credentials or a live server. Provider SDKs, HTTP transport, and agent
boot scripts are outside this suite.

## Case map

`TestAutoscaler` groups the scenarios by behavior. Each scenario has a descriptive
`t.Run` name; table rows become named subtests too.

| Group | Cases |
| --- | --- |
| [Task routing](routing_test.go) | Platform and backend selection; first matching capability; normal labels missing, matching, mismatching, or wildcard; mandatory labels missing, empty, mismatching, matching, or literal wildcard; repository restrictions; unknown labels; internal and empty labels; mixed supported and unsupported demand; running work attributed by agent ID; aggregate worker counts; work waiting on dependencies. |
| [Pool limits](limits_test.go) | MaxAgents; busiest bucket and provider-order ties; WorkflowsPerAgent exact and rounded division, zero and negative fallback; running work plus backlog; MinAgents with no demand, with demand, and during scale-down; external running work; busy draining agents occupying capacity. |
| [Agent lifecycle](lifecycle_test.go) | Provision, boot, connect, run, drain, and remove; wrong-capability replacement at capacity; stale-label replacement; boot demand accounting across repeated cycles and restart; creation timeout with spare capacity and at MaxAgents; vanished boot registrations; custom labels after connection; reactivation at capacity; completion of draining work. |
| [Billing](billing_test.go) | Per-second idle timeout for schedulable and draining agents; hourly retention and teardown; unknown or future creation times; first and subsequent hour windows; a window covering an entire hour; recent work during hourly teardown; busy agents missing a window and retaining the next paid hour. |
| [Cleanup](cleanup_test.go) | Unavailable capabilities; agents that never connect; connected agents that go silent; provider-only and server-only drift; unrelated pools and ordinary agents; busy stale and unavailable agents retained until completion. |
| [Failure recovery](recovery_test.go) | Agent-list, queue, and provider-inventory reads; registration and deployment; drain and reactivation updates; task lookup, provider removal, and server deletion for both stale and drained agents; drift cleanup in both directions. Every injected failure has a nested successful retry. |
| [Capability discovery](capabilities_test.go) | Startup error; cached capabilities; empty results preserving the existing fleet. |

## Run and select cases

Run everything with engine coverage and the race detector:

```sh
make test-e2e
```

Show every case and phase by name:

```sh
go test -v -count=1 ./e2e
```

Run one group:

```sh
go test -v -count=1 ./e2e -run '^TestAutoscaler$/^failure_recovery$'
```

A full subtest path copied from verbose output can select a single scenario or
phase. Go displays spaces in names as underscores.

## Shared setup and isolation

Each independent scenario creates its own harness. Table cases also create a
fresh harness, so they cannot leak state into one another. A lifecycle reuses its
harness across phases, with each dependent phase nested **inside** its
prerequisite. Selecting a later phase therefore still executes its setup and
assertions. Sibling subtests must never depend on a previous sibling having run.

Keep queue inputs, state changes, and expected outcomes next to each case.
`environment_test.go` contains only reusable wiring and fake API behavior. Fakes
copy agent records at API boundaries so an unsuccessful update cannot mutate
server state through a shared pointer. Faults are injected before API mutations
and cleared in the nested recovery case.

Use timestamps comfortably inside or outside timeout windows. These tests do
not sleep or depend on hitting an exact wall-clock boundary.
