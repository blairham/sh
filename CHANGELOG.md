# Changelog

All notable changes to sh are recorded here by
[release-please](https://github.com/googleapis/release-please), from the
Conventional Commit title of each squash-merged pull request — see
`AGENTS.md`, *Pull request titles*. Do not edit it by hand: release-please
writes each version's section in its release pull request, and that section
is the release's notes on GitHub. The release workflow fails a tag that has
none.

Pre-stable releases (`v0.x.y`) make no compatibility promise. Until `v1.0.0`,
`feat` and `fix` both bump the patch version and a breaking change bumps the
minor.

## [0.0.40](https://github.com/blairham/sh/compare/v0.0.39...v0.0.40) (2026-11-02)


### Fixed

* **driver:** take a stop aimed at the shell before kill returns ([#7300](https://github.com/blairham/sh/issues/7300)) ([d94dd5f](https://github.com/blairham/sh/commit/d94dd5f040859c9e85c709521c269e3cd56248be))
* **interp:** a bare array literal that fails to expand still makes the name an array ([#7315](https://github.com/blairham/sh/issues/7315)) ([83808f0](https://github.com/blairham/sh/commit/83808f0ba9dd54c159590723f8770149d11380fd)), closes [#7078](https://github.com/blairham/sh/issues/7078)
* **interp:** a declaration expands its array-literal operand where it is written ([#7312](https://github.com/blairham/sh/issues/7312)) ([d9059f9](https://github.com/blairham/sh/commit/d9059f9f19845de9c34bd7a3ee59bb5c030c53c0)), closes [#7075](https://github.com/blairham/sh/issues/7075)
* **interp:** a declaration under set -a takes the export attribute ([#7303](https://github.com/blairham/sh/issues/7303)) ([424f4f8](https://github.com/blairham/sh/commit/424f4f8fb2c155193863916178a9afa7a0ab4351))
* **interp:** a failed bare literal lists and keeps the name as the shell left it ([#7325](https://github.com/blairham/sh/issues/7325)) ([bb6a33f](https://github.com/blairham/sh/commit/bb6a33f458aacc93cb0e38f64e0ec51f62926c64)), closes [#7316](https://github.com/blairham/sh/issues/7316) [#7317](https://github.com/blairham/sh/issues/7317)
* **interp:** a group kill at a forked body's own pid is no such process without the monitor ([#7305](https://github.com/blairham/sh/issues/7305)) ([75dce60](https://github.com/blairham/sh/commit/75dce600e58877db5dc6598c2ca3625b16900ad6))
* **interp:** a ksh call to a traced function works its prefix through again inside the body ([#7336](https://github.com/blairham/sh/issues/7336)) ([b4d38f6](https://github.com/blairham/sh/commit/b4d38f63433f52968e1afebff38ae51a2fcacf4a))
* **interp:** a ksh name reference aimed at a non-name is an invalid variable name ([#7330](https://github.com/blairham/sh/issues/7330)) ([2b02c9f](https://github.com/blairham/sh/commit/2b02c9f927a2065a1f393390cca4402d67fd883b))
* **interp:** a zsh appending literal with a subscripted element stores from the first position ([#7320](https://github.com/blairham/sh/issues/7320)) ([7a06f95](https://github.com/blairham/sh/commit/7a06f95128d7bc82693a1d788e91299b2db4972a))
* **interp:** an element's death by SIGPIPE leaves the outer PIPE handler alone ([#7314](https://github.com/blairham/sh/issues/7314)) ([9b52f23](https://github.com/blairham/sh/commit/9b52f23e2363794c9d59f462a152ed7930f4fa57))
* **interp:** every body around a subshell that signals the group dies of it ([#7313](https://github.com/blairham/sh/issues/7313)) ([139a027](https://github.com/blairham/sh/commit/139a027ccf11fa1be5a33a356891c0f84c5e1b73)), closes [#7245](https://github.com/blairham/sh/issues/7245)
* **interp:** open a substitution's own pipe out of a park's way ([#7334](https://github.com/blairham/sh/issues/7334)) ([e730a82](https://github.com/blairham/sh/commit/e730a8272944c86149f6ea3e9d02eeb484879968)), closes [#7035](https://github.com/blairham/sh/issues/7035)
* **interp:** under set -m a $( … )'s own number names no group ([#7328](https://github.com/blairham/sh/issues/7328)) ([31c64b8](https://github.com/blairham/sh/commit/31c64b81f30c08b207269585cc3b841900fee021))
* **interp:** under set -m a body forked inside a forked body leads no group ([#7333](https://github.com/blairham/sh/issues/7333)) ([ccfadf9](https://github.com/blairham/sh/commit/ccfadf9eb95b2f5f50d2e62f2dca35666f8c3ad0))
* **interp:** under set -m a pipeline element and a &lt;(…) are the groups each dialect makes them ([#7326](https://github.com/blairham/sh/issues/7326)) ([267d65e](https://github.com/blairham/sh/commit/267d65e5e94edb8e135599871ea455394f4c1562))
* **interp:** under set -m a subshell's group kill reaches its own job, not the shell ([#7319](https://github.com/blairham/sh/issues/7319)) ([8eeb3b2](https://github.com/blairham/sh/commit/8eeb3b2be00d41dc9b5eb39eb08bb5b8d17fd2da))

## [0.0.39](https://github.com/blairham/sh/releases/tag/v0.0.39) and earlier

Releases up to `v0.0.39` predate this file. Each one's notes are on its
GitHub release page.
