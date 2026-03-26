# Model-Scoped Channel Circuit Breaker Design

## Summary

This design adds model-scoped channel routing, circuit breaking, recovery probing, and admin visibility for the `new-api` relay.

Current behavior selects a channel from `Ability(group, model, channel)` and retries by channel priority tiers. The new behavior adds a higher-priority global model policy layer:

- Each model can define a global ordered list of channels.
- A request for one model retries to the next channel in that model's ordered list immediately after a retryable failure, even if the failed `model@channel` pair has not reached its disable threshold.
- Circuit breaking is tracked at `model@channel` granularity.
- Existing channel-level disable logic remains in place.
- If all models on one channel are unavailable, or the error is channel-wide such as invalid key, existing channel-level disable behavior still applies.

The design preserves current `group` behavior by intersecting global model policy with group-scoped `Ability` availability at runtime.

## Goals

- Support global per-model channel ordering independent of `group`.
- Retry `model@channel` failures along the next configured channel path in the same request.
- Auto-disable only after configurable consecutive failures.
- Probe disabled `model@channel` pairs in the background and auto-recover when a real probe succeeds.
- Keep existing channel-level auto-disable and manual disable behavior.
- Provide an admin UI to manage order, inspect state, and manually enable or disable one `model@channel`.
- Record the retry path for each request so operators can see the exact channel switching sequence.

## Non-Goals

- No change to project branding, package names, or protected identifiers.
- No removal or replacement of existing `Ability`-based group filtering.
- No JSON-column-only design that would reduce cross-database compatibility.
- No per-group model policy in this phase.

## Current Constraints

- Database compatibility must hold across SQLite, MySQL >= 5.7.8, and PostgreSQL >= 9.6.
- Business JSON operations must use wrappers in `common/json.go`.
- Relay selection currently starts from `controller/relay.go` and `service/channel_select.go`.
- Existing channel auto-disable and auto-enable logic already exists and must remain compatible.

## User-Approved Decisions

- Model policy is global, not group-specific.
- Recovery supports both automatic probing and manual admin actions.
- Both model-level and channel-level circuit breaking remain active.
- Model-level failures such as `429`, `model not found`, or model unsupported errors should disable `model@channel`, not the whole channel.
- Channel-wide failures such as invalid key should still disable the whole channel.
- Model-level disable threshold is configurable, default `3`.
- Background probe interval is configurable, default `5` minutes.
- Recovery probes must be real `model@channel` requests.
- A failed request must immediately retry the next channel in the model's order even if the failed pair has not yet reached the disable threshold.
- Request logs must record the retry path such as `a -> b -> c -> d`.

## Data Model

### Table 1: `model_channel_policies`

Purpose: static admin configuration for global model routing order and manual enable state.

Suggested fields:

- `id`
- `model` varchar(255), not null
- `channel_id` int, not null
- `priority` bigint, not null, default `0`
- `manual_enabled` bool, not null, default `true`
- `created_at`
- `updated_at`

Constraints and indexes:

- Unique key on `(model, channel_id)`
- Index on `model`
- Index on `priority`
- Foreign-key behavior should follow existing project style; if explicit FK is avoided elsewhere, keep the same style here.

Semantics:

- `manual_enabled=false` means the admin intentionally disables this `model@channel` pair.
- Manual disable at this layer excludes the pair from routing and from automatic recovery.

### Table 2: `model_channel_states`

Purpose: runtime state for circuit breaking and probing.

Suggested fields:

- `id`
- `model` varchar(255), not null
- `channel_id` int, not null
- `status` varchar(32), not null, default `enabled`
- `reason_type` varchar(64), not null, default `""`
- `reason` text
- `consecutive_failures` int, not null, default `0`
- `last_failed_at` bigint, not null, default `0`
- `last_succeeded_at` bigint, not null, default `0`
- `last_probe_at` bigint, not null, default `0`
- `next_probe_at` bigint, not null, default `0`
- `last_probe_status` varchar(32), not null, default `""`
- `last_probe_message` text
- `created_at`
- `updated_at`

Constraints and indexes:

- Unique key on `(model, channel_id)`
- Composite index on `(status, next_probe_at)`
- Index on `model`
- Index on `channel_id`

Status values:

- `enabled`
- `auto_disabled`
- `manual_disabled`

Semantics:

- `manual_disabled` is a per-pair admin disable and never auto-recovers.
- `auto_disabled` is runtime disable and is eligible for probe-based recovery.
- `enabled` means runtime state allows routing, but the pair may still be excluded by channel-level disable, `Ability`, or policy manual disable.

## Runtime Selection Model

### Selection Inputs

For each request, selection must respect all of the following filters:

1. Current `group` must allow the model through existing `Ability`.
2. The channel itself must not be channel-level disabled.
3. If the model has policy rows, only configured channels are eligible.
4. Policy row must have `manual_enabled=true`.
5. Runtime state must not be `auto_disabled` or `manual_disabled`.

### Ordered Retry Sequence

For a request model such as `gpt5.4`, the system builds a concrete ordered candidate list:

1. Load the model's global policy rows ordered by `priority desc`.
2. Intersect them with eligible channels from `Ability(group, model)`.
3. Remove pairs blocked by policy or runtime state.
4. Iterate the remaining channels one by one in order.

If the model has no policy rows, fallback to the current `Ability.priority + weight` selection behavior unchanged.

### Immediate Request Retry

Immediate retry and circuit-breaking threshold are separate concerns:

- If one `model@channel` request fails with a retryable error, the current request immediately switches to the next channel in the ordered list.
- This happens even when `consecutive_failures` is still below the disable threshold.
- The threshold only controls whether the pair transitions to `auto_disabled`.

Example:

- `gpt5.4` ordered channels: `a -> b -> c -> d`
- Request path:
  - `a` fails with retryable model-level error
  - increment `a` failure count
  - retry `b` immediately
  - `b` succeeds
- Future requests still start from `a` unless `a` has reached its disable threshold or has otherwise become unavailable.

## Failure Classification

### Channel-Level Failures

These continue to use existing channel disable behavior:

- invalid key
- authentication failure
- account deactivated
- billing arrears
- channel-wide forbidden conditions
- whole-channel unavailability

If all models on a channel become unavailable due to accumulated pair states, the implementation may also escalate to channel-level disable after recomputing channel coverage.

### Model-Level Failures

These affect only the specific `model@channel` pair:

- `429` limit or capacity errors scoped to one model
- `model not found`
- `model not supported`
- provider-specific errors that clearly reject only the requested model

This requires a new classifier such as `ShouldDisableModelChannel(...)`, while preserving `ShouldDisableChannel(...)`.

### Retryability

The same error may both:

- cause immediate retry to the next channel for the current request
- increment failure counters for the current pair

Only some errors additionally transition the pair to `auto_disabled` after threshold is reached.

## State Transitions

### On Request Success

For the successful `model@channel` pair:

- reset `consecutive_failures` to `0`
- set `last_succeeded_at`
- keep `status=enabled`

For previously failed pairs attempted earlier in the same request:

- their failure increments remain
- they are not auto-reset just because a later channel succeeded

### On Request Failure For One Pair

When one attempt on `model@channel` fails with a model-level circuit-breaker error:

- increment `consecutive_failures`
- set `last_failed_at`
- store `reason_type` and `reason`
- if threshold reached, transition `status -> auto_disabled`
- set `next_probe_at = now + probe_interval`

### On Channel-Wide Failure

Existing channel disable flow remains the source of truth for channel-wide state.

The request should still record that the pair failed on the path before the whole channel is disabled.

### On Manual Admin Disable

Two manual operations exist:

- policy manual disable through `model_channel_policies.manual_enabled=false`
- runtime manual disable through `model_channel_states.status=manual_disabled`

Either excludes the pair from routing. Neither auto-recovers.

## Background Probe Recovery

### Scheduler

Run a new background probe loop only on the master node, similar to existing automatic channel testing.

Default behavior:

- interval between scheduler runs: `5` minutes unless configured otherwise
- scan only rows where:
  - `status = auto_disabled`
  - `next_probe_at <= now`

### Probe Mechanics

Each probe must:

- target a real `model@channel` pair
- use the actual model id in the upstream request
- bypass ordinary selection and bind directly to the target channel
- use a lightweight but valid provider request shape compatible with the channel type

If probe succeeds:

- set `status = enabled`
- set `consecutive_failures = 0`
- set `last_probe_at`
- set `last_probe_status = success`
- clear or update `last_probe_message`
- set `last_succeeded_at`

If probe fails:

- keep `status = auto_disabled`
- set `last_probe_at`
- set `last_probe_status = failed`
- set `last_probe_message`
- set `next_probe_at = now + probe_interval`

Manual-disabled pairs are skipped by the probe loop.

## Admin UI

### Route

Add a dedicated admin page, recommended route:

- `/console/model-channel-circuit`

Reason: this feature is operational routing control, not marketplace display configuration.

### Page Structure

#### List View

One row per model with aggregated status:

- `model`
- `configured_channels`
- `enabled_channels`
- `auto_disabled_channels`
- `manual_disabled_channels`
- `last_failed_at`
- quick action to open details

#### Detail View

For one model, show ordered channel rows with:

- drag or numeric priority editing
- `channel_id`
- `channel_name`
- `channel_type`
- policy manual enable state
- runtime status
- disable reason type
- disable reason
- consecutive failures
- last failed time
- last success time
- last probe time
- next probe time

Actions:

- save order
- manual enable one pair
- manual disable one pair
- trigger immediate probe for one pair
- batch save policy changes

### UX Rules

- The page must make it obvious whether a row is blocked by policy, runtime pair state, or channel-level status.
- If the underlying channel itself is disabled, show that separately from pair status.
- If a model has no policy rows yet, the UI should allow bootstrapping rows from current `Ability` mappings.

## API Surface

Recommended new admin endpoints:

- `GET /api/model_channel_circuit/models`
- `GET /api/model_channel_circuit/models/:model`
- `PUT /api/model_channel_circuit/models/:model/policies`
- `POST /api/model_channel_circuit/models/:model/channel/:channel_id/enable`
- `POST /api/model_channel_circuit/models/:model/channel/:channel_id/disable`
- `POST /api/model_channel_circuit/models/:model/channel/:channel_id/probe`

Optional system setting endpoints or reuse existing option-setting flow for:

- pair failure threshold
- probe interval minutes

Response payloads should include both policy and runtime state so the UI does not need to stitch multiple calls.

## Logging And Observability

### Retry Path

Each relay request should persist the actual attempt sequence, for example:

- `retry_path = [12, 18, 24, 31]`
- `retry_path_text = "12 -> 18 -> 24 -> 31"`
- `attempt_count = 4`
- `final_channel_id = 31`

The path should represent actual attempted channels, not just all configured channels.

### Skip Visibility

Optional but useful:

- `skipped_model_channel_ids` for pairs skipped because they were already auto-disabled or manually disabled
- `skip_reasons` map for operator debugging

### Existing Log Integration

Current `use_channel` context already records attempted channel ids in-memory. This should be formalized into request/error log fields rather than only runtime log lines.

## Architecture Changes

### Model Layer

Add:

- new GORM models for policy and state tables
- query helpers to load policy rows by model
- query helpers to upsert runtime state
- query helpers to aggregate model status for admin pages

### Cache Layer

Add an in-memory cache keyed by `model` for ordered policy rows if memory cache is enabled.

The cache must be refreshed together with or alongside channel cache updates, without replacing channel-level cache semantics.

Suggested structures:

- `model2policyChannels map[string][]PolicyEntry`
- `modelChannelStateMap map[string]map[int]*StateEntry`

Cache invalidation points:

- policy update
- state transition
- channel status change
- periodic cache rebuild

### Service Layer

Add:

- model-pair failure classifier
- ordered channel candidate builder
- runtime failure counter updater
- probe runner
- channel-level coverage recomputation for optional escalation

### Controller Layer

Adjust:

- relay controller to use ordered per-model candidate iteration
- admin controller endpoints for policy/state management
- logging payload generation to include retry path

## Error Handling

- If policy data is missing for one model, fallback to current selection logic instead of failing requests.
- If state row is missing, treat it as implicit `enabled` and create it lazily on first update.
- If policy references a deleted channel, ignore that row in routing and surface it in admin UI for cleanup.
- If background probe cannot build a valid provider test request for one channel type, mark probe failure in state and keep the pair disabled.
- If a request exhausts all eligible channels, return request failure and preserve full retry path in logs.

## Testing Strategy

### Unit Tests

- ordered candidate generation with policy rows
- fallback to legacy behavior when no policy exists
- request-level retry progression independent of disable threshold
- model-level threshold transition after 1, 2, 3 failures
- reset on success
- state filtering for `auto_disabled` and `manual_disabled`
- channel-level filtering precedence over pair-level state
- retry path accumulation

### Integration Tests

- relay retries `a -> b -> c` and succeeds on later channel
- pair fails below threshold but request still switches to next channel
- pair reaches threshold and is excluded from subsequent requests
- background probe restores one `auto_disabled` pair
- manual-disabled pair is not restored by probe
- no policy rows still uses existing `Ability` logic

### Cross-Database Tests

- migrations for SQLite, MySQL, PostgreSQL
- upsert behavior for unique `(model, channel_id)` records
- pagination and ordering queries used by the admin UI

## Rollout Plan

1. Add tables and migrations.
2. Add model and cache helpers.
3. Add runtime selection changes behind a guarded feature flag if needed.
4. Add failure classification and state updates.
5. Add probe scheduler.
6. Add admin APIs.
7. Add admin page.
8. Add logging fields for retry path.
9. Verify on all supported databases and with representative provider types.

## Risks

- Misclassifying provider errors could disable pairs too aggressively or too weakly.
- Probe requests vary by provider type and may need channel-type-specific minimal payloads.
- Global model order plus group-local `Ability` means the UI must clearly explain why a configured channel is still not selectable for some requests.
- Logging retry path increases payload size slightly but is operationally valuable.

## Open Implementation Choices

The following are intentionally narrowed to one direction to avoid ambiguity during implementation:

- Use dedicated tables instead of JSON blobs.
- Keep per-group `Ability` filtering unchanged.
- Separate immediate request retry from disable-threshold state transition.
- Keep channel-level disable logic as an independent, higher-scope mechanism.
- Put the feature in a dedicated admin page instead of reusing marketplace model management.
