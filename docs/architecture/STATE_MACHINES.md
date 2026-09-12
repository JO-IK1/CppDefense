# Машины состояний CppDefense 2.0

**Версия:** 1.0  
**Источник:** требования 1.4

Каждый переход выполняется одной application-командой в транзакции PostgreSQL с optimistic version check и audit event. Неописанный переход запрещён и возвращает `409 INVALID_STATE_TRANSITION`.

## Импорт

```mermaid
stateDiagram-v2
  [*] --> receiving
  receiving --> stored
  receiving --> rejected
  receiving --> cancelled
  stored --> validating
  stored --> cancelled
  validating --> review_pending
  validating --> rejected
  validating --> failed
  validating --> cancelled
  review_pending --> approved
  review_pending --> rejected
  review_pending --> cancelled
  approved --> applying
  approved --> cancelled
  applying --> completed
  applying --> failed
  failed --> validating: retry validation
  failed --> applying: retry approved apply
  completed --> [*]
  rejected --> [*]
  cancelled --> [*]
```

| Из | Событие | В | Условия |
|---|---|---|---|
| `receiving` | upload completed | `stored` | size/hash/object committed |
| `receiving` | invalid/limit | `rejected` | safe rejection code |
| `stored` | validator starts | `validating` | lease/idempotent worker |
| `validating` | valid preview | `review_pending` | no blocking errors |
| `validating` | input violation | `rejected` | no versions created |
| `validating` | infrastructure error | `failed` | retryable code |
| `review_pending` | reviewer approves | `approved` | teacher group/admin, checklist, hash unchanged |
| `review_pending` | reviewer rejects | `rejected` | reason required |
| `approved` | apply starts | `applying` | current hash equals reviewed hash |
| `applying` | atomic commit | `completed` | all items created/skipped |
| `applying` | infrastructure rollback | `failed` | no partial versions |
| non-terminal | user cancels | `cancelled` | actor authorized |

`completed`, `rejected`, `cancelled` are terminal. Retry from `failed` resumes the failed phase; it cannot bypass review.

## Защита

```mermaid
stateDiagram-v2
  [*] --> ready
  ready --> preparing
  ready --> cancelled
  preparing --> active
  preparing --> error
  preparing --> cancelled
  active --> passed
  active --> failed
  active --> expired
  active --> cancelled
  active --> error
  error --> ready: manual recovery
  passed --> [*]
  failed --> [*]
  expired --> [*]
  cancelled --> [*]
```

| Из | Событие | В | Условия |
|---|---|---|---|
| `ready` | start confirmed | `preparing` | one active defense, version available |
| `preparing` | worker prepared | `active` | candidates + selected persisted, start/deadline set |
| `preparing` | unrecoverable preparation | `error` | structured reason |
| `active` | accepted attempt passed | `passed` | attempt accepted before deadline |
| `active` | student/teacher finishes | `failed` | no passed attempt, reason for teacher |
| `active` | deadline + no pending attempt | `expired` | no eligible pass |
| `active` | deadline + pending attempt fails | `expired` | wait for last timely attempt |
| non-terminal | cancel | `cancelled` | reason required for teacher/admin |
| `error` | authorized recovery | `ready` | new preparation run, audit required |

Terminal defenses are immutable. Reopen creates a new `ready` defense linked by `reopened_from_id`; it does not transition the old record.

## Runner job

```mermaid
stateDiagram-v2
  [*] --> queued
  queued --> leased
  queued --> cancelled
  leased --> running
  leased --> queued: lease expired before start
  leased --> cancelled
  running --> completed
  running --> retry_wait
  running --> timed_out
  running --> cancelled
  retry_wait --> queued
  retry_wait --> dead
  completed --> [*]
  timed_out --> [*]
  cancelled --> [*]
  dead --> [*]
```

| Из | Событие | В | Условия |
|---|---|---|---|
| `queued` | runner lease | `leased` | capacity, atomic lease token |
| `leased` | runner starts | `running` | current unexpired token |
| `leased` | lease expires | `queued` | retry/lease count allowed |
| `running` | worker/pipeline exits | `completed` | preparation result or attempt outcome; compile/test failure is completed |
| `running` | retryable infrastructure failure | `retry_wait` | retries remain |
| `retry_wait` | backoff elapsed | `queued` | available_at reached |
| `retry_wait` | retries exhausted | `dead` | attempt becomes error |
| `running` | wall timeout | `timed_out` | container killed and cleaned |
| eligible | cancellation | `cancelled` | no final result accepted later |

## Пользователь

```mermaid
stateDiagram-v2
  [*] --> pending
  pending --> active: auto/manual link or role assignment
  pending --> rejected
  rejected --> pending: reconsider
  active --> blocked
  pending --> blocked
  rejected --> blocked
  blocked --> active: admin unblock with valid role/identity
```

- `active` requires a role and at least one identity.
- OAuth authentication and student record claim occur atomically.
- Blocking revokes all web sessions in the same transaction.
- Removing the last identity is forbidden.

## Инварианты конкуренции

- Import apply, create attempt, lease and complete use row locks/version checks.
- A stale HTTP command returns conflict and current representation; it never overwrites state.
- A stale runner token cannot heartbeat or complete a job.
- Idempotent retry returns the first committed resource/result.
- Deadline checks use PostgreSQL/Backend UTC time, never browser time.
