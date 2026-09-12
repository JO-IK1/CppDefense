create extension if not exists citext;

create type user_status as enum ('pending', 'active', 'rejected', 'blocked');
create type user_role as enum ('student', 'teacher', 'admin');
create type identity_provider as enum ('github', 'telegram');
create type group_status as enum ('active', 'archived');
create type student_record_status as enum ('unclaimed', 'claimed', 'archived');
create type student_record_source as enum ('manual', 'group_import');
create type lab_status as enum ('active', 'archived');
create type import_kind as enum ('group', 'lab');
create type import_state as enum ('receiving', 'stored', 'validating', 'review_pending', 'approved', 'applying', 'completed', 'rejected', 'failed', 'cancelled');
create type import_action as enum ('create_student_and_version', 'create_version', 'skip_duplicate', 'conflict');
create type submission_source as enum ('teacher_import');
create type defense_status as enum ('ready', 'preparing', 'active', 'passed', 'failed', 'expired', 'cancelled', 'error');
create type defense_mode as enum ('official');
create type attempt_outcome as enum ('pending', 'passed', 'failed', 'error');
create type runner_job_kind as enum ('prepare_defense', 'check_attempt');
create type runner_job_state as enum ('queued', 'leased', 'running', 'retry_wait', 'completed', 'timed_out', 'cancelled', 'dead');
create type runner_status as enum ('active', 'draining', 'offline', 'disabled');
create type audit_actor_kind as enum ('user', 'runner', 'system');

create table users (
    id uuid primary key,
    status user_status not null default 'pending',
    role user_role,
    display_name text,
    rejection_reason text,
    blocked_at timestamptz,
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp(),
    version bigint not null default 1 check (version >= 1),
    check (status <> 'active' or role is not null),
    check ((status = 'rejected') = (rejection_reason is not null)),
    check ((status = 'blocked') = (blocked_at is not null))
);

create table github_identities (
    id uuid primary key,
    user_id uuid not null unique references users(id) on delete restrict,
    github_user_id bigint not null unique check (github_user_id > 0),
    login citext not null check (length(login::text) between 1 and 100),
    display_name text,
    avatar_url text,
    verified_at timestamptz not null,
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp()
);
create index github_identities_login_idx on github_identities (login);

create table telegram_identities (
    id uuid primary key,
    user_id uuid not null unique references users(id) on delete restrict,
    telegram_sub text not null unique check (length(telegram_sub) between 1 and 200),
    username text,
    display_name text,
    verified_at timestamptz not null,
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp()
);

create table auth_flows (
    id uuid primary key,
    provider identity_provider not null,
    state_hash bytea not null unique check (octet_length(state_hash) = 32),
    pkce_verifier_encrypted bytea not null,
    nonce_hash bytea check (nonce_hash is null or octet_length(nonce_hash) = 32),
    return_path text not null check (return_path like '/%' and return_path not like '//%'),
    expires_at timestamptz not null,
    consumed_at timestamptz,
    created_at timestamptz not null default clock_timestamp(),
    check (expires_at > created_at),
    check (consumed_at is null or consumed_at >= created_at)
);
create index auth_flows_expiry_idx on auth_flows (expires_at) where consumed_at is null;

create table web_sessions (
    id uuid primary key,
    user_id uuid not null references users(id) on delete restrict,
    token_hash bytea not null unique check (octet_length(token_hash) = 32),
    csrf_secret_hash bytea not null check (octet_length(csrf_secret_hash) = 32),
    view_as_role user_role,
    view_as_target_id uuid,
    created_at timestamptz not null default clock_timestamp(),
    last_seen_at timestamptz not null default clock_timestamp(),
    expires_at timestamptz not null,
    revoked_at timestamptz,
    check (expires_at > created_at),
    check (last_seen_at >= created_at),
    check ((view_as_role is null) = (view_as_target_id is null))
);
create index web_sessions_active_token_idx on web_sessions (token_hash) where revoked_at is null;
create index web_sessions_user_idx on web_sessions (user_id) where revoked_at is null;

create table groups (
    id uuid primary key,
    code citext not null unique check (length(code::text) between 1 and 64),
    name text not null check (length(name) between 1 and 200),
    is_demo boolean not null default false,
    status group_status not null default 'active',
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp(),
    archived_at timestamptz,
    version bigint not null default 1 check (version >= 1),
    check ((status = 'archived') = (archived_at is not null))
);

create table group_teachers (
    group_id uuid not null references groups(id) on delete restrict,
    teacher_user_id uuid not null references users(id) on delete restrict,
    assigned_at timestamptz not null default clock_timestamp(),
    assigned_by uuid references users(id) on delete restrict,
    primary key (group_id, teacher_user_id)
);

create table student_records (
    id uuid primary key,
    group_id uuid not null references groups(id) on delete restrict,
    user_id uuid unique references users(id) on delete restrict,
    github_login_expected citext not null check (length(github_login_expected::text) between 1 and 100),
    status student_record_status not null default 'unclaimed',
    created_source student_record_source not null,
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp(),
    archived_at timestamptz,
    version bigint not null default 1 check (version >= 1),
    check ((status = 'claimed' and user_id is not null) or (status = 'unclaimed' and user_id is null) or status = 'archived'),
    check ((status = 'archived') = (archived_at is not null))
);
create unique index student_records_active_login_uq on student_records (group_id, github_login_expected) where status <> 'archived';

create table labs (
    id uuid primary key,
    group_id uuid not null references groups(id) on delete restrict,
    code citext not null check (length(code::text) between 1 and 64),
    name text not null check (length(name) between 1 and 200),
    description text,
    status lab_status not null default 'active',
    time_limit_seconds integer not null check (time_limit_seconds > 0),
    top_n integer not null check (top_n between 1 and 50),
    check_config jsonb not null default '{}'::jsonb check (jsonb_typeof(check_config) = 'object'),
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp(),
    archived_at timestamptz,
    version bigint not null default 1 check (version >= 1),
    unique (group_id, code),
    check ((status = 'archived') = (archived_at is not null))
);

create table imports (
    id uuid primary key,
    group_id uuid not null references groups(id) on delete restrict,
    kind import_kind not null,
    state import_state not null default 'receiving',
    uploaded_by uuid not null references users(id) on delete restrict,
    original_object_key text not null unique,
    original_sha256 bytea not null check (octet_length(original_sha256) = 32),
    compressed_size bigint not null check (compressed_size >= 0),
    uncompressed_size bigint check (uncompressed_size is null or uncompressed_size >= 0),
    schema_version integer not null check (schema_version > 0),
    reviewed_by uuid references users(id) on delete restrict,
    reviewed_at timestamptz,
    reviewed_sha256 bytea check (reviewed_sha256 is null or octet_length(reviewed_sha256) = 32),
    review_checklist jsonb check (review_checklist is null or jsonb_typeof(review_checklist) = 'object'),
    rejection_reason text,
    error_code text,
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp(),
    version bigint not null default 1 check (version >= 1),
    check (state not in ('approved','applying','completed') or (reviewed_by is not null and reviewed_at is not null and reviewed_sha256 is not null)),
    check (state <> 'rejected' or rejection_reason is not null),
    check (state not in ('applying','completed') or reviewed_sha256 = original_sha256)
);

create table submission_versions (
    id uuid primary key,
    student_record_id uuid not null references student_records(id) on delete restrict,
    lab_id uuid not null references labs(id) on delete restrict,
    version_no integer not null check (version_no > 0),
    source submission_source not null,
    original_import_id uuid not null references imports(id) on delete restrict,
    normalized_object_key text not null unique,
    normalized_sha256 bytea not null check (octet_length(normalized_sha256) = 32),
    source_manifest jsonb not null check (jsonb_typeof(source_manifest) = 'object'),
    created_at timestamptz not null default clock_timestamp(),
    created_by uuid not null references users(id) on delete restrict,
    unique (student_record_id, lab_id, version_no)
);
create index submission_versions_lookup_idx on submission_versions (student_record_id, lab_id, version_no desc);

create table import_items (
    id uuid primary key,
    import_id uuid not null references imports(id) on delete restrict,
    student_record_id uuid references student_records(id) on delete restrict,
    lab_id uuid references labs(id) on delete restrict,
    github_login_input citext not null,
    project_path text not null,
    action import_action not null,
    normalized_sha256 bytea check (normalized_sha256 is null or octet_length(normalized_sha256) = 32),
    file_count integer check (file_count is null or file_count >= 0),
    uncompressed_size bigint check (uncompressed_size is null or uncompressed_size >= 0),
    warnings jsonb not null default '[]'::jsonb check (jsonb_typeof(warnings) = 'array'),
    errors jsonb not null default '[]'::jsonb check (jsonb_typeof(errors) = 'array'),
    submission_version_id uuid unique references submission_versions(id) on delete restrict,
    unique (import_id, project_path),
    unique (import_id, student_record_id, lab_id),
    check (project_path <> '' and project_path not like '/%' and project_path !~ '(^|/)\.\.?(/|$)')
);

create table defenses (
    id uuid primary key,
    session_id uuid not null unique,
    submission_version_id uuid not null references submission_versions(id) on delete restrict,
    status defense_status not null default 'ready',
    mode defense_mode not null default 'official',
    seed numeric(20,0) not null check (seed >= 0 and seed <= 18446744073709551615),
    time_limit_seconds integer not null check (time_limit_seconds > 0),
    started_at timestamptz,
    deadline_at timestamptz,
    finished_at timestamptz,
    current_draft text,
    draft_version bigint not null default 1 check (draft_version >= 1),
    selected_candidate_id uuid,
    reopened_from_id uuid references defenses(id) on delete restrict,
    terminal_reason text,
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp(),
    version bigint not null default 1 check (version >= 1),
    check ((status in ('passed','failed','expired','cancelled')) = (finished_at is not null)),
    check ((started_at is null and deadline_at is null) or deadline_at = started_at + make_interval(secs => time_limit_seconds))
);
create unique index defenses_one_open_uq on defenses (submission_version_id) where status in ('ready','preparing','active');

create table defense_candidates (
    id uuid primary key,
    defense_id uuid not null references defenses(id) on delete restrict,
    rank integer not null check (rank > 0),
    function_name text not null,
    file_path text not null check (file_path <> '' and file_path not like '/%' and file_path !~ '(^|/)\.\.?(/|$)'),
    body_start_offset bigint not null check (body_start_offset >= 0),
    body_end_offset bigint not null check (body_end_offset > body_start_offset),
    start_line integer not null check (start_line > 0),
    end_line integer not null check (end_line >= start_line),
    line_count integer not null check (line_count > 0),
    source_sha256 bytea not null check (octet_length(source_sha256) = 32),
    original_body_sha256 bytea not null check (octet_length(original_body_sha256) = 32),
    is_selected boolean not null default false,
    unique (defense_id, rank)
);
create unique index defense_candidates_selected_uq on defense_candidates (defense_id) where is_selected;
alter table defenses add constraint defenses_selected_candidate_fk foreign key (selected_candidate_id) references defense_candidates(id) on delete restrict;

create table check_attempts (
    id uuid primary key,
    defense_id uuid not null references defenses(id) on delete restrict,
    attempt_no integer not null check (attempt_no > 0),
    idempotency_key uuid not null,
    answer text not null,
    answer_sha256 bytea not null check (octet_length(answer_sha256) = 32),
    accepted_at timestamptz not null default clock_timestamp(),
    outcome attempt_outcome not null default 'pending',
    configure_result jsonb,
    build_result jsonb,
    ctest_result jsonb,
    finished_at timestamptz,
    safe_log_object_key text,
    manual_decision_id uuid,
    unique (defense_id, attempt_no),
    unique (defense_id, idempotency_key),
    check ((outcome = 'pending') = (finished_at is null))
);

create table runners (
    id uuid primary key,
    name text not null unique check (length(name) between 1 and 100),
    status runner_status not null default 'offline',
    slots integer not null check (slots between 1 and 64),
    agent_version text not null,
    credential_hash bytea not null unique check (octet_length(credential_hash) = 32),
    last_heartbeat_at timestamptz,
    disabled_at timestamptz,
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp(),
    version bigint not null default 1 check (version >= 1),
    check ((status = 'disabled') = (disabled_at is not null))
);

create table runner_jobs (
    id uuid primary key,
    kind runner_job_kind not null,
    defense_id uuid not null references defenses(id) on delete restrict,
    check_attempt_id uuid unique references check_attempts(id) on delete restrict,
    state runner_job_state not null default 'queued',
    priority integer not null default 0,
    available_at timestamptz not null default clock_timestamp(),
    retry_count integer not null default 0 check (retry_count >= 0),
    max_retries integer not null default 3 check (max_retries >= 0),
    lease_owner_runner_id uuid references runners(id) on delete restrict,
    lease_token_hash bytea check (lease_token_hash is null or octet_length(lease_token_hash) = 32),
    lease_expires_at timestamptz,
    timeout_seconds integer not null check (timeout_seconds > 0),
    worker_version text,
    image_digest text,
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp(),
    version bigint not null default 1 check (version >= 1),
    check ((kind = 'prepare_defense') = (check_attempt_id is null)),
    check ((lease_owner_runner_id is null) = (lease_token_hash is null)),
    check ((lease_owner_runner_id is null) = (lease_expires_at is null))
);
create index runner_jobs_available_idx on runner_jobs (priority desc, available_at, created_at) where state in ('queued','retry_wait');

create table job_leases (
    id uuid primary key,
    job_id uuid not null references runner_jobs(id) on delete restrict,
    runner_id uuid not null references runners(id) on delete restrict,
    lease_no integer not null check (lease_no > 0),
    token_fingerprint text not null,
    leased_at timestamptz not null,
    expires_at timestamptz not null,
    released_at timestamptz,
    release_reason text,
    unique (job_id, lease_no),
    check (expires_at > leased_at),
    check ((released_at is null) = (release_reason is null))
);

create table idempotency_records (
    id uuid primary key,
    principal_id uuid not null references users(id) on delete restrict,
    http_method text not null,
    canonical_route text not null,
    idempotency_key uuid not null,
    request_hash bytea not null check (octet_length(request_hash) = 32),
    response_status integer check (response_status between 100 and 599),
    response_reference jsonb,
    created_at timestamptz not null default clock_timestamp(),
    expires_at timestamptz not null,
    unique (principal_id, http_method, canonical_route, idempotency_key),
    check (expires_at > created_at)
);
create index idempotency_records_expiry_idx on idempotency_records (expires_at);

create table audit_events (
    sequence_no bigint generated always as identity primary key,
    id uuid not null unique,
    occurred_at timestamptz not null,
    actor_user_id uuid references users(id) on delete restrict,
    actor_kind audit_actor_kind not null,
    action text not null,
    target_type text not null,
    target_id uuid,
    request_id uuid not null,
    reason text,
    metadata jsonb not null default '{}'::jsonb check (jsonb_typeof(metadata) = 'object'),
    prev_event_hash bytea check (prev_event_hash is null or octet_length(prev_event_hash) = 32),
    event_hash bytea not null unique check (octet_length(event_hash) = 32),
    check ((actor_kind = 'user') = (actor_user_id is not null))
);
create index audit_events_target_idx on audit_events (target_type, target_id, sequence_no desc);
create index audit_events_request_idx on audit_events (request_id);

create function reject_audit_mutation() returns trigger language plpgsql as $$
begin
    raise exception 'audit_events is append-only' using errcode = '55000';
end;
$$;
create trigger audit_events_append_only before update or delete on audit_events
for each row execute function reject_audit_mutation();

create function validate_import_transition() returns trigger language plpgsql as $$
begin
    if new.state = old.state then return new; end if;
    if not (
        (old.state = 'receiving' and new.state in ('stored','rejected','cancelled')) or
        (old.state = 'stored' and new.state in ('validating','cancelled')) or
        (old.state = 'validating' and new.state in ('review_pending','rejected','failed','cancelled')) or
        (old.state = 'review_pending' and new.state in ('approved','rejected','cancelled')) or
        (old.state = 'approved' and new.state in ('applying','cancelled')) or
        (old.state = 'applying' and new.state in ('completed','failed')) or
        (old.state = 'failed' and new.state in ('validating','applying'))
    ) then
        raise exception 'INVALID_STATE_TRANSITION: import % -> %', old.state, new.state using errcode = '23000';
    end if;
    new.version = old.version + 1;
    new.updated_at = clock_timestamp();
    return new;
end;
$$;
create trigger imports_state_transition before update of state on imports
for each row execute function validate_import_transition();

create function validate_defense_transition() returns trigger language plpgsql as $$
begin
    if new.status = old.status then return new; end if;
    if not (
        (old.status = 'ready' and new.status in ('preparing','cancelled')) or
        (old.status = 'preparing' and new.status in ('active','error','cancelled')) or
        (old.status = 'active' and new.status in ('passed','failed','expired','cancelled','error')) or
        (old.status = 'error' and new.status = 'ready')
    ) then
        raise exception 'INVALID_STATE_TRANSITION: defense % -> %', old.status, new.status using errcode = '23000';
    end if;
    new.version = old.version + 1;
    new.updated_at = clock_timestamp();
    return new;
end;
$$;
create trigger defenses_state_transition before update of status on defenses
for each row execute function validate_defense_transition();

create function validate_runner_job_transition() returns trigger language plpgsql as $$
begin
    if new.state = old.state then return new; end if;
    if not (
        (old.state = 'queued' and new.state in ('leased','cancelled')) or
        (old.state = 'leased' and new.state in ('running','queued','cancelled')) or
        (old.state = 'running' and new.state in ('completed','retry_wait','timed_out','cancelled')) or
        (old.state = 'retry_wait' and new.state in ('queued','dead'))
    ) then
        raise exception 'INVALID_STATE_TRANSITION: runner job % -> %', old.state, new.state using errcode = '23000';
    end if;
    new.version = old.version + 1;
    new.updated_at = clock_timestamp();
    return new;
end;
$$;
create trigger runner_jobs_state_transition before update of state on runner_jobs
for each row execute function validate_runner_job_transition();

create function validate_user_transition() returns trigger language plpgsql as $$
begin
    if new.status = old.status then return new; end if;
    if not (
        (old.status = 'pending' and new.status in ('active','rejected','blocked')) or
        (old.status = 'rejected' and new.status in ('pending','blocked')) or
        (old.status = 'active' and new.status = 'blocked') or
        (old.status = 'blocked' and new.status = 'active')
    ) then
        raise exception 'INVALID_STATE_TRANSITION: user % -> %', old.status, new.status using errcode = '23000';
    end if;
    new.version = old.version + 1;
    new.updated_at = clock_timestamp();
    return new;
end;
$$;
create trigger users_state_transition before update of status on users
for each row execute function validate_user_transition();

create function validate_active_user_identity() returns trigger language plpgsql as $$
declare target_id uuid;
begin
    if tg_table_name = 'users' then
        target_id = coalesce(new.id, old.id);
    else
        target_id = coalesce(new.user_id, old.user_id);
    end if;
    if exists (select 1 from users where id = target_id and status = 'active')
       and not exists (select 1 from github_identities where user_id = target_id)
       and not exists (select 1 from telegram_identities where user_id = target_id) then
        raise exception 'active user requires an identity' using errcode = '23514';
    end if;
    return null;
end;
$$;
create constraint trigger users_active_identity after insert or update of status on users
deferrable initially deferred for each row execute function validate_active_user_identity();
create constraint trigger github_active_identity after delete or update of user_id on github_identities
deferrable initially deferred for each row execute function validate_active_user_identity();
create constraint trigger telegram_active_identity after delete or update of user_id on telegram_identities
deferrable initially deferred for each row execute function validate_active_user_identity();
