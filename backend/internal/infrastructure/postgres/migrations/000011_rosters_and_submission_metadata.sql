create table organizations (
    id uuid primary key,
    github_login citext not null unique check (length(github_login::text) between 1 and 100),
    name text not null check (length(name) between 1 and 200),
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp()
);

alter table groups
    add column organization_id uuid references organizations(id) on delete restrict;

alter table student_records
    add column full_name text check (full_name is null or length(full_name) between 1 and 200);

alter type submission_source add value if not exists 'student_zip';

alter table imports
    add column original_filename text check (original_filename is null or length(original_filename) between 1 and 255);

alter table submission_versions
    add column version_label text not null default '' check (length(version_label) <= 100),
    add column source_reference text check (source_reference is null or length(source_reference) <= 255),
    add column commit_sha text,
    add column approved_by uuid references users(id) on delete restrict;

create index groups_organization_idx on groups(organization_id);
