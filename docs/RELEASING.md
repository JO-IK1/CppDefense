# Releasing CppDefense

1. Build and run the complete test suite on a clean working tree.
2. Set `project(CppDefense VERSION ...)` in the root `CMakeLists.txt`.
3. Update the project status in `README.md` and any affected behavior in
   `USAGE.md`, `ARCHITECTURE.md`, and `ROADMAP.md`.
4. Commit the release changes.
5. Create and push the matching annotated tag, for example:

   ```bash
   git tag -a v2.1.0 -m "CppDefense 2.1.0"
   git push origin v2.1.0
   ```

The tag starts the GitHub Actions release job. CPack and the install rules put
the executable, README, license, and documentation into the platform archive.
