# Authentication and identity linking contract

**Status:** accepted baseline for CppDefense 2.0  
**Contract version:** 1.0  
**Source:** requirements 1.4

## 1. Identity model

- `user` is the local account and authorization subject.
- `github_identity` and `telegram_identity` are verified login methods. Neither is a role.
- `student_record` is the roster/import record that owns the student's group and laboratory visibility.
- An active user has exactly one real role: `student`, `teacher`, or `admin`.
- `pending` and `rejected` users have no working role. `blocked` overrides any assigned role.
- A user may have GitHub, Telegram, or both. At least one login identity must remain linked.

Database uniqueness is mandatory for numeric `github_user_id`, Telegram `sub`, and the active `student_record.user_id` association. A GitHub login is normalized to lowercase for initial matching, but is not a permanent identity.

## 2. GitHub registration and sign-in

1. Backend creates a one-time authorization transaction containing `state`, PKCE verifier/challenge, return target and expiry.
2. Browser is redirected to GitHub using Authorization Code flow and PKCE `S256`.
3. Callback consumes the transaction exactly once and verifies `state` before exchanging the code.
4. Backend requests only the basic profile, obtains numeric GitHub user ID and current login, and then discards the access token.
5. Existing identity is found only by numeric GitHub ID. A changed login updates the display/matching value and creates an audit event.
6. A new identity is matched to a free `student_record` only when exactly one record has the same normalized login.
7. A unique match atomically creates the identity, links the user and record, and activates the `student` role. No match or any ambiguity creates a `pending` user.

The callback never accepts a client-supplied user ID, role, group or student-record ID as proof of identity.

## 3. Telegram sign-in

Telegram OIDC is an alternative login, not a prerequisite for GitHub users.

1. Backend creates a one-time transaction with `state`, PKCE verifier/challenge, `nonce`, return target and expiry.
2. Callback verifies `state`, issuer, audience, signature, token expiry and `nonce` before reading `sub`.
3. An existing Telegram `sub` signs in to its linked local user.
4. A previously unknown `sub` creates a `pending` user. It is never matched to a student record by display name.
5. Teacher for the record's group or admin may approve a pending association manually.

## 4. Adding or removing a login method

- Only an authenticated, active user may start linking another provider.
- The user must have a recently authenticated server session and complete a new full OAuth/OIDC flow.
- Link intent is stored server-side and bound to the initiating user, provider, `state`, expiry and single use.
- If the verified external identity belongs to another user, linking fails with a conflict and an audit event.
- Unlinking requires CSRF protection and recent authentication. Removing the final login identity is forbidden.
- Linking and unlinking revoke other active web sessions when the risk policy requires it.

## 5. Manual approval

Approval is one database transaction:

1. Lock pending user, external identity and target student record.
2. Recheck reviewer authorization against the target group.
3. Recheck that identity and student record are still free.
4. Link `student_record.user_id`, set real role `student`, set user state `active` and record the reviewer.
5. Append an audit event in the same transaction.

Teacher may approve only records in assigned groups. Admin may approve any record. Reject requires a reason and is audited. Retrying the same idempotency key returns the original decision.

## 6. Server session

- OAuth/OIDC tokens are never the CppDefense browser session.
- Backend issues an opaque, rotating server-side session cookie named `cppdefense_session` with `HttpOnly`, `Secure` and `SameSite=Lax`.
- State-changing browser requests also require CSRF protection.
- Session contains the real user ID, authentication time and optional admin view mode; it never stores provider secrets.
- Blocking a user, changing a sensitive identity association or changing a real role revokes active sessions.

## 7. Admin view-as-role

`view_as_role` may be `admin`, `teacher` or `student` and exists only in an admin's server session. Teacher view also requires a selected group; student view requires a selected student record. Backend verifies the target exists and is visible to the real admin.

The mode changes navigation and presentation, not the database role or Backend authorization subject. It is read-only by default. Mutating actions are allowed only when the selected target belongs to a group marked `demo`; the audit actor remains the real admin and records the view mode and selected target. A visible banner and exit control are mandatory in every non-admin view. Every view-mode change is audited.

## 8. Observable errors

Authentication errors are returned through the common problem-details contract without exposing provider tokens or existence of accounts outside the caller's scope. Stable codes include:

- `AUTH_FLOW_EXPIRED`
- `AUTH_STATE_INVALID`
- `AUTH_NONCE_INVALID`
- `IDENTITY_ALREADY_LINKED`
- `LAST_IDENTITY_REQUIRED`
- `STUDENT_MATCH_AMBIGUOUS`
- `STUDENT_RECORD_OCCUPIED`
- `USER_BLOCKED`

Logs and audit events may contain internal user/record UUIDs and provider type, but not access tokens, authorization codes, PKCE verifiers, OIDC tokens or session cookie values.
