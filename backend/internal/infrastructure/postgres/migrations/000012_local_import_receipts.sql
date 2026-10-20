create table local_import_receipts (
    id uuid primary key,
    actor_id uuid not null references users(id),
    import_id uuid not null references imports(id),
    created_at timestamptz not null default clock_timestamp()
);
