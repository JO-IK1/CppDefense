alter table defenses
 add column defense_mode text not null default 'restore' check(defense_mode in ('restore','hide_seek')),
 add column hide_mutations jsonb not null default '[]'::jsonb,
 add column hide_report jsonb,
 add column hide_error text;
