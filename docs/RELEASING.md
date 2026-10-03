# Releasing CppDefense

**English** | [Русский](ru/RELEASING.md)

1. Start from a clean branch based on current `main`.
2. Update `project(CppDefense VERSION ...)` and every image/application version.
3. Update both English and Russian documentation; English is canonical.
4. Run the full local gate:

   ```sh
   ./deploy/release-check.sh deploy/production.env
   ```

5. Confirm the contract workflow validates JSON Schema, negative fixtures,
   OpenAPI, and a live worker.
6. Open a pull request and require all protected-branch checks.
7. Merge without bypassing checks, deploy `main`, and run public live/ready plus
   one synthetic end-to-end defense.
8. Create and push the matching annotated tag:

   ```sh
   git tag -a v2.1.0 -m "CppDefense 2.1.0"
   git push origin v2.1.0
   ```

The tag publishes platform archives through GitHub Actions. CMake install rules
include the CLI, worker, README, license, and documentation. Production deploy
from `main` updates backend/frontend only; if C++, Runner Agent, service config,
or sandbox changed, follow the manual runner upgrade in the deployment guide.

Before a production release, create an off-host backup and verify rollback. Do
not tag a revision whose database migration has not been exercised on a copy of
production data.
