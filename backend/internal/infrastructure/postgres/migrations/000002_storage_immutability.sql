create function reject_submission_version_mutation() returns trigger language plpgsql as $$
begin
    raise exception 'submission_versions are immutable' using errcode = '55000';
end;
$$;

create trigger submission_versions_immutable
before update or delete on submission_versions
for each row execute function reject_submission_version_mutation();

create index submission_versions_digest_idx
on submission_versions (student_record_id, lab_id, normalized_sha256);

