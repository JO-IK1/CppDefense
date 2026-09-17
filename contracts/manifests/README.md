# ZIP manifest contracts

Canonical filename inside every import archive: `cppdefense-manifest.json`.

- `group-v1.schema.json` validates one group archive with multiple student/lab projects.
- `lab-v1.schema.json` validates exactly one project of one student and one lab.

JSON Schema validation is only the first validation layer. Backend must additionally enforce:

- UTF-8 and manifest size <= 1 MiB;
- Unicode NFC and case-insensitive path uniqueness;
- uniqueness of student/lab and project paths;
- no overlapping project directories;
- no ZIP Slip, links, devices, encrypted entries or nested ZIP;
- configured compressed/uncompressed/file-count/ratio limits;
- group, lab and teacher permissions;
- root `CMakeLists.txt` and at least one C/C++ source;
- immutable storage and atomic review/apply rules described in the architecture.

`$id` uses a reserved example hostname until the canonical public project domain is chosen. Release CI may replace it only through an explicit contract version update.

The `examples/` directory contains schema-valid manifests. `../tests/invalid-lab-zip-slip.json` is intentionally invalid and must remain rejected. CI validates schema documents, positive examples and this negative security fixture on every contract change.
