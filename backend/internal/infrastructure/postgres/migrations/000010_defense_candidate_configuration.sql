alter table defenses
    add column wheel_size integer,
    add column entity_mode text,
    add constraint defenses_wheel_size_ck
        check (wheel_size is null or wheel_size between 2 and 12),
    add constraint defenses_entity_mode_ck
        check (entity_mode is null or entity_mode in ('functions_only','functions_classes_structs'));

alter table defense_candidates
    add column entity_type text not null default 'function',
    add column is_test_file boolean not null default false,
    add column wheel_rank integer,
    add constraint defense_candidates_entity_type_ck
        check (entity_type in ('function','class','struct')),
    add constraint defense_candidates_wheel_rank_ck
        check (wheel_rank is null or wheel_rank between 1 and 12);

create unique index defense_candidates_wheel_rank_uq
    on defense_candidates (defense_id, wheel_rank)
    where wheel_rank is not null;
