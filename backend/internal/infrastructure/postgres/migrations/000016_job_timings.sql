alter table check_attempts add column timings jsonb check(timings is null or jsonb_typeof(timings)='object');
alter table defenses add column preparation_timings jsonb check(preparation_timings is null or jsonb_typeof(preparation_timings)='object');
