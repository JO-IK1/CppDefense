alter table defenses add column signatures_visible boolean not null default true;
create table defense_restore_targets (
  defense_id uuid not null references defenses(id) on delete restrict,
  candidate_id uuid not null references defense_candidates(id) on delete restrict,
  ordinal integer not null check (ordinal between 1 and 5),
  primary key (defense_id, ordinal),
  unique (defense_id, candidate_id)
);
