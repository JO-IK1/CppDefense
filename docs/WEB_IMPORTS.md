# Web roster and lab imports

The teacher uploads a versioned roster JSON, reviews a preview and confirms it.
The database update is transactional and repeated imports update existing
student records by GitHub login.

Students upload ordinary project archives named
`labwork1-USERNAME-deadline_0.zip`. The authenticated GitHub login and linked
roster record determine the owner and group. CppDefense validates the archive,
creates its internal manifest and keeps the version unavailable for defenses
until a teacher reviews and approves it.

Teachers import via a local Git clone using Git, Python 3 and their own GitHub
SSH access: `python3 deploy/clone-lab.py org/labwork1-student deadline_0 --output ./lab-export`.
The standalone script can also be downloaded from the teacher page. It clones
the branch, exports the exact commit with git archive and writes a JSON sidecar.
Upload both files after selecting the site group, student and lab. Groups stay
site-only; repositories use `<organization>/<lab-code>-<github-login>`.
The browser no longer requests a PAT. The backend validates site ownership and
queues teacher review; the uploaded SHA is provenance metadata, not independently
authenticated proof of archive contents.

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
