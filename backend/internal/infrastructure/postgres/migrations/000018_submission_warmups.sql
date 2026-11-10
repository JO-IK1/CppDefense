alter table runner_jobs
 alter column defense_id drop not null,
 add column submission_version_id uuid references submission_versions(id) on delete restrict,
 add column preparation_result jsonb,
 drop constraint runner_jobs_check,
 add constraint runner_jobs_payload_check check (
   (kind='warm_submission' and defense_id is null and check_attempt_id is null and submission_version_id is not null)
   or (kind='prepare_defense' and defense_id is not null and check_attempt_id is null and submission_version_id is null)
   or (kind='check_attempt' and defense_id is not null and check_attempt_id is not null and submission_version_id is null)
 );
create unique index runner_jobs_warm_submission_idx on runner_jobs(submission_version_id) where kind='warm_submission';
