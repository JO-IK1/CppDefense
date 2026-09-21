alter table defenses
    add column suggested_candidate_rank integer,
    add column selection_mode text,
    add constraint defenses_suggested_candidate_rank_ck
        check (suggested_candidate_rank is null or suggested_candidate_rank > 0),
    add constraint defenses_selection_mode_ck
        check (selection_mode is null or selection_mode in ('automatic','manual'));

create or replace function validate_defense_transition() returns trigger language plpgsql as $$
begin
    if new.status = old.status then return new; end if;
    if not (
        (old.status = 'ready' and new.status in ('preparing','cancelled')) or
        (old.status = 'preparing' and new.status in ('ready','active','error','cancelled')) or
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
