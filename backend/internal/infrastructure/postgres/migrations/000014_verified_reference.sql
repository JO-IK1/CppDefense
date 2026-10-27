alter table check_attempts add column reference_result jsonb;
alter table check_attempts add constraint check_attempts_reference_result_check check (
 reference_result is null or (
  outcome='passed' and jsonb_typeof(reference_result)='object'
  and configure_result is null and build_result is null and ctest_result is null
 )
);
