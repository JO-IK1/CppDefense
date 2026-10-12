# Web roster and lab imports

The teacher uploads a versioned roster JSON, reviews a preview and confirms it.
The database update is transactional and repeated imports update existing
student records by GitHub login.

Students upload ordinary project archives named
`labwork1-USERNAME-deadline_0.zip`. The authenticated GitHub login and linked
roster record determine the owner and group. CppDefense validates the archive,
creates its internal manifest and keeps the version unavailable for defenses
until a teacher reviews and approves it.

The offline sandbox image includes CMake, CTest, GCC and GoogleTest. Labs should
prefer `find_package(GTest REQUIRED)`. Existing GoogleTest `FetchContent`
projects resolve `/usr/src/googletest` through
`FETCHCONTENT_SOURCE_DIR_GOOGLETEST`; runtime network access remains disabled.
Production deployment rebuilds the sandbox before restarting the runner.
The protected installer under `/usr/local/sbin` must be updated once by an
administrator when this repository script changes; `deploy-app.sh` verifies an
exact match and fails before changing the application otherwise.
