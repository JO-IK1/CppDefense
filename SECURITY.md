# Security Policy

CppDefense processes untrusted C++ projects and private student data. Security reports must not include real student work, credentials or unnecessary personal data.

## Reporting a vulnerability

Use GitHub Private Vulnerability Reporting for the repository. If it is not enabled, contact the repository owner through a private channel and ask for a secure reporting method. Do not open a public issue with exploit details, tokens or student data.

Include when possible:

- affected version/commit;
- component and deployment context;
- minimal reproduction using synthetic data;
- expected and observed impact;
- suggested mitigation, if known.

## Scope

High-priority areas include:

- authentication, account linking, sessions and RBAC;
- access to another student's source/results;
- ZIP traversal or decompression abuse;
- runner lease bypass or duplicate completion;
- container/sandbox escape, network access or cross-job access;
- secret leakage through logs, public repository or backups.

Do not test destructive payloads against a live educational deployment. Use a local/demo environment and synthetic projects.

## Supported versions

Until the first stable release, only the current development branch is evaluated. After 2.0, this section will list supported release lines and security update policy.

## Handling

The owner will acknowledge valid private reports, assess severity, prepare a fix and coordinate disclosure when practical. No response-time guarantee applies before the first stable release.
