alter table defenses add column start_idempotency_key uuid;
create unique index defenses_start_idempotency_uq
on defenses(submission_version_id,start_idempotency_key)
where start_idempotency_key is not null;

create unique index check_attempts_one_pending_uq
on check_attempts(defense_id)
where outcome='pending';
