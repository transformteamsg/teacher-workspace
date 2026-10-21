# Changelog

All notable changes to this project will be documented in this file.

## 0.0.4 (2026-10-21)

Adds a profile menu in the top right corner that shows the signed-in teacher's name, email and role, and carries the teacher's email to remote app backends as an `email` claim in the TW JWT. No configuration changes: environments running 0.0.3 deploy this version as is.

Versions stay in the `0.0.x` series while the host shell is pre-release, so this is a patch bump despite the features below.

### Features ✨

- feat: show the teacher's name, email and role in a profile menu ([#221](https://github.com/transformteamsg/teacher-workspace/pull/221)) ([0ce6d7b](https://github.com/transformteamsg/teacher-workspace/commit/0ce6d7b))
- feat(`server/proxy`): add `email` claim to the TW JWT ([#225](https://github.com/transformteamsg/teacher-workspace/pull/225)) ([d03cbd3](https://github.com/transformteamsg/teacher-workspace/commit/d03cbd3))

### Refactors ♻️

- refactor(`server/proxy`): log an incomplete session user with one check ([#229](https://github.com/transformteamsg/teacher-workspace/pull/229)) ([076dcda](https://github.com/transformteamsg/teacher-workspace/commit/076dcda))

### Chores 🧹

- chore(`deps`): bump `oxfmt` from `0.54.0` to `0.72.0` ([#232](https://github.com/transformteamsg/teacher-workspace/pull/232)) ([7c65ca5](https://github.com/transformteamsg/teacher-workspace/commit/7c65ca5))
- chore(`deps`): bump `oxlint` from `1.69.0` to `1.87.0` ([#231](https://github.com/transformteamsg/teacher-workspace/pull/231)) ([c2d6f9c](https://github.com/transformteamsg/teacher-workspace/commit/c2d6f9c))
- chore(`deps`): bump `lefthook` from `2.1.9` to `2.2.1` ([#230](https://github.com/transformteamsg/teacher-workspace/pull/230)) ([70278a0](https://github.com/transformteamsg/teacher-workspace/commit/70278a0))

## 0.0.3 (2026-10-19)

Completes Edupass sign-in end to end: `private_key_jwt` client authentication, ID token claim validation, role resolution into the session, a sign-in gate that sends signed-out page loads to `/login`, and sign-out. The proxied TW JWT now carries `sub` and role claims, and the session package is reworked to carry a CSRF token.

Configuration is regrouped by Edupass and by remote app, which renames env vars: update each deployed environment before deploying this version, using the migration table in [#194](https://github.com/transformteamsg/teacher-workspace/pull/194). The session cookie's `Secure` attribute now comes from `TW_SESSION_SECURE` (default `true`), and the remote app backend signing keys and base URLs no longer have defaults.

Versions stay in the `0.0.x` series while the host shell is pre-release, so this is a patch bump despite the features below.

### Features ✨

- feat(`auth`): sign out of Teacher Workspace ([#217](https://github.com/transformteamsg/teacher-workspace/pull/217)) ([0ffdfa0](https://github.com/transformteamsg/teacher-workspace/commit/0ffdfa0))
- feat(`server/proxy`): add `sub` claim to the TW JWT ([#215](https://github.com/transformteamsg/teacher-workspace/pull/215)) ([4b66d4e](https://github.com/transformteamsg/teacher-workspace/commit/4b66d4e))
- feat(`server`): redirect unauthenticated access to `/login` ([#203](https://github.com/transformteamsg/teacher-workspace/pull/203)) ([e8f2a51](https://github.com/transformteamsg/teacher-workspace/commit/e8f2a51))
- feat(`server/proxy`): add role claims to the TW JWT ([#212](https://github.com/transformteamsg/teacher-workspace/pull/212)) ([1fba00a](https://github.com/transformteamsg/teacher-workspace/commit/1fba00a))
- feat(`server/auth`): drop Edupass base roles at MOE Kindergarten schools before resolution ([#202](https://github.com/transformteamsg/teacher-workspace/pull/202)) ([fa12f21](https://github.com/transformteamsg/teacher-workspace/commit/fa12f21))
- feat(`server/auth`): validate Edupass ID token claims on sign-in ([#200](https://github.com/transformteamsg/teacher-workspace/pull/200)) ([ff8621c](https://github.com/transformteamsg/teacher-workspace/commit/ff8621c))
- feat(`server/auth`): resolve Edupass roles into the session role and attributes ([#199](https://github.com/transformteamsg/teacher-workspace/pull/199)) ([c3244c3](https://github.com/transformteamsg/teacher-workspace/commit/c3244c3))
- feat(`server`): add `private_key_jwt` authentication method ([#176](https://github.com/transformteamsg/teacher-workspace/pull/176)) ([5ff58a7](https://github.com/transformteamsg/teacher-workspace/commit/5ff58a7))
- feat: rework sessions and group config by Edupass and remote apps ([#194](https://github.com/transformteamsg/teacher-workspace/pull/194)) ([840057b](https://github.com/transformteamsg/teacher-workspace/commit/840057b))
- feat(`apps/mock-edupass`): add `private_key_jwt` authentication method ([#190](https://github.com/transformteamsg/teacher-workspace/pull/190)) ([c7c358e](https://github.com/transformteamsg/teacher-workspace/commit/c7c358e))
- feat(`mock-edupass`): generate signing keys on every server start ([#175](https://github.com/transformteamsg/teacher-workspace/pull/175)) ([e290e96](https://github.com/transformteamsg/teacher-workspace/commit/e290e96))
- feat(`apps/mock-edupass`): emit the Edupass roles claim ([#179](https://github.com/transformteamsg/teacher-workspace/pull/179)) ([2716817](https://github.com/transformteamsg/teacher-workspace/commit/2716817))
- feat(`login`): integration for login & Edupass end-to-end flow ([#155](https://github.com/transformteamsg/teacher-workspace/pull/155)) ([46db08c](https://github.com/transformteamsg/teacher-workspace/commit/46db08c))

### Bug Fixes 🐛

- fix(`server/auth`): attach `provider` to every Edupass auth log ([#205](https://github.com/transformteamsg/teacher-workspace/pull/205)) ([3a86380](https://github.com/transformteamsg/teacher-workspace/commit/3a86380))
- fix(`config`): drop defaults for API proxy signing keys and base URLs ([#170](https://github.com/transformteamsg/teacher-workspace/pull/170)) ([0638597](https://github.com/transformteamsg/teacher-workspace/commit/0638597))
- fix(`handler`): answer 404 for directories under `/static/` ([#165](https://github.com/transformteamsg/teacher-workspace/pull/165)) ([f15d06a](https://github.com/transformteamsg/teacher-workspace/commit/f15d06a))
- fix(deps): bump `moby/go-archive` to `0.3.3` to patch tar path traversal ([#158](https://github.com/transformteamsg/teacher-workspace/pull/158)) ([72f1e14](https://github.com/transformteamsg/teacher-workspace/commit/72f1e14))
- fix(deps): bump js-yaml to 4.3.2 to patch DoS vulnerability ([#157](https://github.com/transformteamsg/teacher-workspace/pull/157)) ([f4eb768](https://github.com/transformteamsg/teacher-workspace/commit/f4eb768))

### Refactors ♻️

- refactor(`server/auth`): simplify the Edupass token exchange and its failure logs ([#211](https://github.com/transformteamsg/teacher-workspace/pull/211)) ([77ddb3b](https://github.com/transformteamsg/teacher-workspace/commit/77ddb3b))
- refactor(`go.mod`): rename module to match the repository ([#204](https://github.com/transformteamsg/teacher-workspace/pull/204)) ([8816abc](https://github.com/transformteamsg/teacher-workspace/commit/8816abc))

### Chores 🧹

- chore(`CODEOWNERS`): add default reviewers ([#223](https://github.com/transformteamsg/teacher-workspace/pull/223)) ([abe5457](https://github.com/transformteamsg/teacher-workspace/commit/abe5457))
- chore(`host`): switch to the variable Inter font ([#222](https://github.com/transformteamsg/teacher-workspace/pull/222)) ([9a9470b](https://github.com/transformteamsg/teacher-workspace/commit/9a9470b))

### Documentation 📚

- docs: fix the `CONTRIBUTING.md` import and split out Go test conventions ([#197](https://github.com/transformteamsg/teacher-workspace/pull/197)) ([174879b](https://github.com/transformteamsg/teacher-workspace/commit/174879b))
- docs: unwrap markdown lines and enforce it with oxfmt ([#150](https://github.com/transformteamsg/teacher-workspace/pull/150)) ([290d82b](https://github.com/transformteamsg/teacher-workspace/commit/290d82b))

## 0.0.2 (2026-09-18)

Fixes the published image, which could not open a TLS connection to the session store, and carries the Edupass sign-in plumbing, runtime remote resolution and the first-login modal that landed since 0.0.1.

Versions stay in the `0.0.x` series while the host shell is pre-release, so this is a patch bump despite the features below.

### Features ✨

- feat(`server/auth`): support return_to on the Edupass sign-in route ([#140](https://github.com/transformteamsg/teacher-workspace/pull/140)) ([2a45eb9](https://github.com/transformteamsg/teacher-workspace/commit/2a45eb9))
- feat(`server`, `auth`): add the Edupass OIDC client and login routes ([#126](https://github.com/transformteamsg/teacher-workspace/pull/126)) ([090d08e](https://github.com/transformteamsg/teacher-workspace/commit/090d08e))
- feat(`host`): Welcome Modal for first login ([#129](https://github.com/transformteamsg/teacher-workspace/pull/129)) ([36f277a](https://github.com/transformteamsg/teacher-workspace/commit/36f277a))
- feat(host/remotes): resolve Module Federation remote entries at runtime (URL) ([#125](https://github.com/transformteamsg/teacher-workspace/pull/125)) ([134aa2f](https://github.com/transformteamsg/teacher-workspace/commit/134aa2f))

### Bug Fixes 🐛

- fix(`Dockerfile`): install `ca-certificates` in the production stage ([#153](https://github.com/transformteamsg/teacher-workspace/pull/153)) ([6e4c1c4](https://github.com/transformteamsg/teacher-workspace/commit/6e4c1c4))
- fix(host): use primary color token instead of hardcoded hex ([#151](https://github.com/transformteamsg/teacher-workspace/pull/151)) ([86559f1](https://github.com/transformteamsg/teacher-workspace/commit/86559f1))
- fix(`compose.yml`): password-protect Valkey and bind ports to loopback ([#136](https://github.com/transformteamsg/teacher-workspace/pull/136)) ([74ec736](https://github.com/transformteamsg/teacher-workspace/commit/74ec736))

### Chores 🧹

- chore(valkey): pin local and test to 9.1 ([#146](https://github.com/transformteamsg/teacher-workspace/pull/146)) ([de15618](https://github.com/transformteamsg/teacher-workspace/commit/de15618))
- chore(`release.yml`): trigger on push to `main` ([#138](https://github.com/transformteamsg/teacher-workspace/pull/138)) ([0f3d417](https://github.com/transformteamsg/teacher-workspace/commit/0f3d417))
- chore(`toolchain`): pin local tools with mise and lockfile ([#141](https://github.com/transformteamsg/teacher-workspace/pull/141)) ([9cdb154](https://github.com/transformteamsg/teacher-workspace/commit/9cdb154))
- chore(`compose.yml`): remove RedisInsight ([#144](https://github.com/transformteamsg/teacher-workspace/pull/144)) ([453253e](https://github.com/transformteamsg/teacher-workspace/commit/453253e))

### Documentation 📚

- docs: adr for local development ([#109](https://github.com/transformteamsg/teacher-workspace/pull/109)) ([cc059a6](https://github.com/transformteamsg/teacher-workspace/commit/cc059a6))
- docs: adr for release strategy ([#122](https://github.com/transformteamsg/teacher-workspace/pull/122)) ([0b7ae42](https://github.com/transformteamsg/teacher-workspace/commit/0b7ae42))

## 0.0.1 (2026-09-09)

### Experimental 🧪

- Initial release to validate the release workflow
