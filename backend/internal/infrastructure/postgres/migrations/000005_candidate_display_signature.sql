alter table defense_candidates
add column signature text not null default ''
check (length(signature) <= 16384);
