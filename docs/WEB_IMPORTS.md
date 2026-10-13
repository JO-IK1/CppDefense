# Web roster and lab imports

The teacher uploads a versioned roster JSON, reviews a preview and confirms it.
The database update is transactional and repeated imports update existing
student records by GitHub login.

Students upload ordinary project archives named
`labwork1-USERNAME-deadline_0.zip`. The authenticated GitHub login and linked
roster record determine the owner and group. CppDefense validates the archive,
creates its internal manifest and keeps the version unavailable for defenses
until a teacher reviews and approves it.

Teachers can also import a repository directly from GitHub without disclosing
their personal token to CppDefense. Groups remain site-only metadata and point
to one GitHub organization. The browser derives the expected repository as
`<organization>/<lab-code>-<github-login>`, reads its `deadline_X` branches and
downloads the selected immutable commit archive directly from `api.github.com`.
Only the ZIP and verified source metadata are uploaded to CppDefense. The token
is kept in page memory, is never persisted, and is cleared when the tab closes.
The backend rechecks the organization, group, student, lab, branch and commit
metadata before placing the archive in the normal teacher-review queue.

The offline sandbox image includes CMake, CTest, GCC and GoogleTest. Labs should
prefer `find_package(GTest REQUIRED)`. Existing GoogleTest `FetchContent`
projects resolve `/usr/src/googletest` through
`FETCHCONTENT_SOURCE_DIR_GOOGLETEST`; runtime network access remains disabled.
Production deployment rebuilds the sandbox before restarting the runner.
The protected installer under `/usr/local/sbin` must be updated once by an
administrator when this repository script changes; `deploy-app.sh` verifies an
exact match and fails before changing the application otherwise.
