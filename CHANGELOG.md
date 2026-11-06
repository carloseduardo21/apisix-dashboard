# Changelog

## 0.1.0 (2026-11-06)


### ⚠ BREAKING CHANGES

* **i18n:** the "error" field of API responses is now English and clients should use "code"/"params" to localize. The ACME auto-renewal audit actor is now "acme:auto-renewal".

### Features

* **auth:** use Lintel as the TOTP issuer ([b938e2c](https://github.com/carloseduardo21/apisix-dashboard/commit/b938e2c15b407709d2c580985dd3a651d9205766))
* **i18n:** finish UI translation and enforce catalog consistency ([06d1ffb](https://github.com/carloseduardo21/apisix-dashboard/commit/06d1ffb60cbf0364443c135b27bdeacd975bca06))
* **i18n:** frontend i18n infrastructure and first screens (WIP) ([7e8365b](https://github.com/carloseduardo21/apisix-dashboard/commit/7e8365bc3f8a7858ca7f3c11c5282f0c21dda5d6))
* **i18n:** return error codes and translatable messages from the API ([79796c9](https://github.com/carloseduardo21/apisix-dashboard/commit/79796c979ae2419840a7652f2dcf89027eeb87d8))
* **i18n:** translate audit and automatic certificates screens ([28f7af3](https://github.com/carloseduardo21/apisix-dashboard/commit/28f7af3e3bb4b79d2b61e4db488d8d075fc2c9a1))
* **i18n:** translate Kong migration screens ([235e520](https://github.com/carloseduardo21/apisix-dashboard/commit/235e5207eb589c0f5173262f2af221360ab5bb48))
* **kong:** revert migrated objects from the history ([7d4ef81](https://github.com/carloseduardo21/apisix-dashboard/commit/7d4ef81f227111af023e35c9aaaf9e8c55233ac1))


### Bug Fixes

* handle ignored errors and stop relaying raw plugin schemas ([73c3fea](https://github.com/carloseduardo21/apisix-dashboard/commit/73c3fea5869aaabe735be58838bf5f5b4301fa84))
* **kong:** match regex paths on their literal prefix ([dac657e](https://github.com/carloseduardo21/apisix-dashboard/commit/dac657ecd973fd61f1cbe41070b6054ffe9dd76d))
* **kong:** write the migration summary audit detail in English ([cb7bcc7](https://github.com/carloseduardo21/apisix-dashboard/commit/cb7bcc7000453ea923fa4140eee46aa1e1540cc3))
* **lint:** resolve trivial Go lint findings and justify gosec exceptions ([a1eea2b](https://github.com/carloseduardo21/apisix-dashboard/commit/a1eea2b7c6c1ebb8dbbba1483258b86e14010d0c))
* **security:** bound login hashing and throttle authentication per IP ([b7ce392](https://github.com/carloseduardo21/apisix-dashboard/commit/b7ce392ae96df2b26b4291183819598a2b6312e4))
* **security:** mask credentials plugins do not declare as encrypted ([8dd426c](https://github.com/carloseduardo21/apisix-dashboard/commit/8dd426cabf74f763d7570837a77130f6abf655a9))
* **security:** reject dot segments in Admin API paths ([07836c5](https://github.com/carloseduardo21/apisix-dashboard/commit/07836c57fd02d04a11bd54ce6976536f91505686))
* **security:** resolve the client IP from trusted proxies only ([508976d](https://github.com/carloseduardo21/apisix-dashboard/commit/508976d169462cc2ac16df487287a11d3b2a17b3))
* **security:** restrict code-running plugins and protect masked secrets ([6c26c4d](https://github.com/carloseduardo21/apisix-dashboard/commit/6c26c4d2ed8656db2e915fe6148a238f40ddbcc5))
* **security:** stop background polling from extending idle sessions ([b72c70f](https://github.com/carloseduardo21/apisix-dashboard/commit/b72c70f0d70fec1a1e8fbb0c1015f0b9b090764e))
* **web:** mark fire-and-forget promises and drop redundant type assertions ([0121006](https://github.com/carloseduardo21/apisix-dashboard/commit/012100633df65cece9b9fb91765cb7fc9ee3b09b))
* **web:** resolve all Oxlint findings ([e62ab6e](https://github.com/carloseduardo21/apisix-dashboard/commit/e62ab6ea53c36df2a3c2893d30b3eca5da9e5346))


### Documentation

* add licence, governance files and English README ([46ff6e3](https://github.com/carloseduardo21/apisix-dashboard/commit/46ff6e3390ff6f136181b85fb1fb60cfdef1eacd))
* **claude:** document project context, stack rules and Definition of Done ([5633a89](https://github.com/carloseduardo21/apisix-dashboard/commit/5633a898f182d55964497600592157df91cad68c))
* track open-source release preparation status ([a6d6560](https://github.com/carloseduardo21/apisix-dashboard/commit/a6d65606d08b53e8e20458dcd4f08b9d178753ec))


### Build and Dependencies

* automate signed multi-arch releases ([26569c4](https://github.com/carloseduardo21/apisix-dashboard/commit/26569c4059c04128c2ad9fd7153d50ff62791b88))
* bump Go to 1.27 and adopt the GitHub module path ([3e70c2c](https://github.com/carloseduardo21/apisix-dashboard/commit/3e70c2c6e523278bc041402ba2b2b8e89b195e1a))
* pin Go toolchain to go1.27.3 ([47cba18](https://github.com/carloseduardo21/apisix-dashboard/commit/47cba18f8cc9ae181d0c549e3507b756f4d7b36e))
* **tooling:** add linters, formatters and Makefile quality targets ([af90f7b](https://github.com/carloseduardo21/apisix-dashboard/commit/af90f7b4e2e497fc3ea99aa9dc82a74eaddaa20b))
