create or replace function validate_runner_job_transition() returns trigger language plpgsql as $$
begin
    if new.state = old.state then return new; end if;
    if not (
        (old.state = 'queued' and new.state in ('leased','cancelled')) or
        (old.state = 'leased' and new.state in ('running','queued','dead','cancelled')) or
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
