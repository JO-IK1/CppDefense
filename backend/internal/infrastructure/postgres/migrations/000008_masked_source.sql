alter table defense_candidates
    add column masked_source text;

alter table defense_candidates
    add constraint defense_candidates_masked_source_size_ck
    check (masked_source is null or (masked_source <> '' and octet_length(masked_source) <= 4194304));
