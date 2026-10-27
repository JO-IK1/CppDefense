create function notify_runner_jobs() returns trigger language plpgsql as $$
begin
 if new.state in ('queued','retry_wait') then
  perform pg_notify('cppdefense_runner_jobs','ready');
 end if;
 return new;
end $$;
create trigger runner_jobs_notify after insert or update of state,available_at
 on runner_jobs for each row execute function notify_runner_jobs();
