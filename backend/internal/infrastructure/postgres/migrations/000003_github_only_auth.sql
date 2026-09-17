alter table telegram_identities rename to legacy_telegram_identities;

alter trigger telegram_active_identity on legacy_telegram_identities
rename to legacy_telegram_active_identity;

comment on table legacy_telegram_identities is
'Deprecated identity data retained for safe migration. The application must not create or authenticate these identities.';

alter table auth_flows
add constraint auth_flows_github_only check (provider = 'github') not valid;

comment on column auth_flows.nonce_hash is
'Deprecated Telegram OIDC field retained for migration compatibility; GitHub OAuth leaves it null.';

create or replace function validate_active_user_identity() returns trigger
language plpgsql as $$
declare
    target_id uuid := coalesce(new.id, old.id);
begin
    if exists (select 1 from users where id = target_id and state = 'active')
       and not exists (select 1 from github_identities where user_id = target_id)
       and not exists (select 1 from legacy_telegram_identities where user_id = target_id) then
        raise exception 'active user requires a login identity';
    end if;
    return coalesce(new, old);
end;
$$;
