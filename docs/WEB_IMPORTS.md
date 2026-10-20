# Web roster and lab imports

The teacher uploads a versioned roster JSON, reviews a preview and confirms it.
The database update is transactional and repeated imports update existing
student records by GitHub login.

Students upload ordinary project archives named
`labwork1-USERNAME-deadline_0.zip`. The authenticated GitHub login and linked
roster record determine the owner and group. CppDefense validates the archive,
creates its internal manifest and keeps the version unavailable for defenses
until a teacher reviews and approves it.

Teachers select a group, multiple students/labs and a deadline branch, then
copy one generated terminal command. It downloads import-labs.py from the site,
clones via HTTPS using local Git credentials, exports ZIPs and uploads them
automatically. Git and Python 3 are required. No manual ZIP/JSON handling is needed.
The whole-group button includes every student and resolves the highest numeric
deadline_N separately in each repository (deadline_10 beats deadline_9).
Missing branches fail only that item. Signed tickets last two hours, include
at most 200 items, and authorize upload only; teacher approval stays separate.
Migration 000012 stores upload receipts, making retries of the same command
idempotent. A new command creates a new batch. The open page refreshes its queue.
The backend validates current site permissions. SHA is provenance metadata,
not independently authenticated proof of archive contents.

Podman checks reuse a separate ccache volume per defense session and image,
limited to 256 MiB each. Only compilation is cached; CMake, linking and CTest
still execute on each check. The sandbox image must be rebuilt with ccache.
If a volume cannot be created, or Docker is used, checks fall back to cold builds.
After defenses, the runner account can reclaim unused compiler caches with
`podman volume prune --filter label=cppdefense.cache=compiler`.

The offline sandbox image includes CMake, CTest, GCC and GoogleTest. Labs should
prefer `find_package(GTest REQUIRED)`. Existing GoogleTest `FetchContent`
projects resolve `/usr/src/googletest` through
`FETCHCONTENT_SOURCE_DIR_GOOGLETEST`; runtime network access remains disabled.
Production deployment rebuilds the sandbox before restarting the runner.
The protected installer under `/usr/local/sbin` must be updated once by an
administrator when this repository script changes; `deploy-app.sh` verifies an
exact match and fails before changing the application otherwise.
