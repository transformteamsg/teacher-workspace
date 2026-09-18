# Changelog

All notable changes to this project will be documented in this file.

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
