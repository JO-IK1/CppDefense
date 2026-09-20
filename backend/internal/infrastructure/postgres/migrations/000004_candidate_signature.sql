alter table defense_candidates
add column signature_begin_offset bigint not null default 0
check (signature_begin_offset >= 0 and signature_begin_offset <= body_start_offset);
