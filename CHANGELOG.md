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

## [0.0.42](https://github.com/blairham/sh/compare/v0.0.41...v0.0.42) (2026-11-04)


### Performance

* **interp:** run a plain builtin's line by a route beside simpleCommand ([#7409](https://github.com/blairham/sh/issues/7409)) ([d02a95b](https://github.com/blairham/sh/commit/d02a95b6cebdd18859ffe406695702e672d849e1))
* **interp:** take a plain declaration line without the utility's setup ([#7410](https://github.com/blairham/sh/issues/7410)) ([a02bd72](https://github.com/blairham/sh/commit/a02bd727ea7a5edb0ba21aebdb85285adc785dfe)), closes [#6373](https://github.com/blairham/sh/issues/6373)

## [0.0.41](https://github.com/blairham/sh/compare/v0.0.40...v0.0.41) (2026-11-04)


### Performance

* **interp:** leave out a declaration operand's hold when nothing can read it ([#7406](https://github.com/blairham/sh/issues/7406)) ([8b18994](https://github.com/blairham/sh/commit/8b189948b57d1ead4fe56a8da48e071ec9dc2ab2))

## [0.0.40](https://github.com/blairham/sh/compare/v0.0.39...v0.0.40) (2026-11-04)


### Fixed

* **driver:** a run of MainArgs takes its fatal-signal watch down as it returns ([#7372](https://github.com/blairham/sh/issues/7372)) ([28e2638](https://github.com/blairham/sh/commit/28e2638b9c5cb89128a10db5c3265753d14cc80c))
* **driver:** take a stop aimed at the shell before kill returns ([#7300](https://github.com/blairham/sh/issues/7300)) ([d94dd5f](https://github.com/blairham/sh/commit/d94dd5f040859c9e85c709521c269e3cd56248be))
* **interp/zsh:** a pipeline element keeps the parent's traps working in zsh ([#7346](https://github.com/blairham/sh/issues/7346)) ([8c89231](https://github.com/blairham/sh/commit/8c892312de95e996a64640252c7b87bc4aa7406a))
* **interp:** a bare array literal that fails to expand still makes the name an array ([#7315](https://github.com/blairham/sh/issues/7315)) ([83808f0](https://github.com/blairham/sh/commit/83808f0ba9dd54c159590723f8770149d11380fd)), closes [#7078](https://github.com/blairham/sh/issues/7078)
* **interp:** a bash declaration keeps the literals it placed when its utility refuses ([#7373](https://github.com/blairham/sh/issues/7373)) ([89bde19](https://github.com/blairham/sh/commit/89bde191f2b8cddcef5322b412812874e916d9da))
* **interp:** a bash prefix is refused word by word in written order ([#7340](https://github.com/blairham/sh/issues/7340)) ([e6b2478](https://github.com/blairham/sh/commit/e6b2478a3d8bcf308ea6a52bb1f4652f0c10b41a)), closes [#7095](https://github.com/blairham/sh/issues/7095)
* **interp:** a copy's untrapped fatal signal ends a body's wait at once ([#7386](https://github.com/blairham/sh/issues/7386)) ([f2b5175](https://github.com/blairham/sh/commit/f2b5175d1c6c863596ea5081e2f753e4110622ae)), closes [#7383](https://github.com/blairham/sh/issues/7383)
* **interp:** a declaration expands its array-literal operand where it is written ([#7312](https://github.com/blairham/sh/issues/7312)) ([d9059f9](https://github.com/blairham/sh/commit/d9059f9f19845de9c34bd7a3ee59bb5c030c53c0)), closes [#7075](https://github.com/blairham/sh/issues/7075)
* **interp:** a declaration under set -a takes the export attribute ([#7303](https://github.com/blairham/sh/issues/7303)) ([424f4f8](https://github.com/blairham/sh/commit/424f4f8fb2c155193863916178a9afa7a0ab4351))
* **interp:** a failed bare literal lists and keeps the name as the shell left it ([#7325](https://github.com/blairham/sh/issues/7325)) ([bb6a33f](https://github.com/blairham/sh/commit/bb6a33f458aacc93cb0e38f64e0ec51f62926c64)), closes [#7316](https://github.com/blairham/sh/issues/7316) [#7317](https://github.com/blairham/sh/issues/7317)
* **interp:** a group kill at a forked body's own pid is no such process without the monitor ([#7305](https://github.com/blairham/sh/issues/7305)) ([75dce60](https://github.com/blairham/sh/commit/75dce600e58877db5dc6598c2ca3625b16900ad6))
* **interp:** a ksh append to an array emptied by unset lands where ksh93 puts it ([#7394](https://github.com/blairham/sh/issues/7394)) ([13de311](https://github.com/blairham/sh/commit/13de3112019ae6bd11812adfb0ccae6fb4aedcbd))
* **interp:** a ksh call to a traced function works its prefix through again inside the body ([#7336](https://github.com/blairham/sh/issues/7336)) ([b4d38f6](https://github.com/blairham/sh/commit/b4d38f63433f52968e1afebff38ae51a2fcacf4a))
* **interp:** a ksh declaration operand through a reference is traced under the variable it lands in ([#7351](https://github.com/blairham/sh/issues/7351)) ([c60c921](https://github.com/blairham/sh/commit/c60c92168a8503974a0a612e953579a42bee287f)), closes [#7102](https://github.com/blairham/sh/issues/7102)
* **interp:** a ksh declaration over a function's own reference stores through it ([#7356](https://github.com/blairham/sh/issues/7356)) ([6b35e6c](https://github.com/blairham/sh/commit/6b35e6cb44b41d23590cc7da3ed5851642896ed9)), closes [#7352](https://github.com/blairham/sh/issues/7352)
* **interp:** a ksh element write through an element reference makes the element an array ([#7363](https://github.com/blairham/sh/issues/7363)) ([d3aa913](https://github.com/blairham/sh/commit/d3aa91357bbb30d3178ef98b45c026d291010b43))
* **interp:** a ksh frozen prefix entry is refused before the declaration's operands expand ([#7338](https://github.com/blairham/sh/issues/7338)) ([81bbb29](https://github.com/blairham/sh/commit/81bbb299ff2a832ce30d1a1c94b140e92150905c))
* **interp:** a ksh keyed literal assigned to an element nests a table ([#7396](https://github.com/blairham/sh/issues/7396)) ([98ac724](https://github.com/blairham/sh/commit/98ac72417ab10a79227f3ef3de0a4e602d1331c3))
* **interp:** a ksh name reference aimed at a non-name is an invalid variable name ([#7330](https://github.com/blairham/sh/issues/7330)) ([2b02c9f](https://github.com/blairham/sh/commit/2b02c9f927a2065a1f393390cca4402d67fd883b))
* **interp:** a ksh persisting prefix is stored ahead of a redirection that fails ([#7399](https://github.com/blairham/sh/issues/7399)) ([627ca04](https://github.com/blairham/sh/commit/627ca0461474620acbfbd599a0932f1d4b3a4332))
* **interp:** a ksh persisting prefix is stored ahead of the redirections ([627ca04](https://github.com/blairham/sh/commit/627ca0461474620acbfbd599a0932f1d4b3a4332)), closes [#6986](https://github.com/blairham/sh/issues/6986)
* **interp:** a ksh read through an element reference reads the nested element ([#7369](https://github.com/blairham/sh/issues/7369)) ([16295fb](https://github.com/blairham/sh/commit/16295fb246b7db47f5b0118b170400439267b707))
* **interp:** a ksh reference operation resolves through the reference already there ([#7359](https://github.com/blairham/sh/issues/7359)) ([dc9a828](https://github.com/blairham/sh/commit/dc9a82821c6161cdb2d5dcf1d247fcc8a0d22335))
* **interp:** a ksh subscripted prefix in a command's own scope is a fresh indexed array ([#7397](https://github.com/blairham/sh/issues/7397)) ([b7dbba5](https://github.com/blairham/sh/commit/b7dbba5bf9a007ee9d837fda48301534d22cd46c)), closes [#6982](https://github.com/blairham/sh/issues/6982)
* **interp:** a refused bash element write through an element reference gives up the line ([#7371](https://github.com/blairham/sh/issues/7371)) ([1c9f00a](https://github.com/blairham/sh/commit/1c9f00a81a8d2d613d9c35a1039e14242c1ac0c0)), closes [#7367](https://github.com/blairham/sh/issues/7367)
* **interp:** a trapped signal cuts a body's wait short, and a job's early signal lands after the shell goes on ([#7384](https://github.com/blairham/sh/issues/7384)) ([52532c4](https://github.com/blairham/sh/commit/52532c4b8453e8595d4df82f702fa5388c4d6175)), closes [#7086](https://github.com/blairham/sh/issues/7086)
* **interp:** a zsh appending literal with a subscripted element stores from the first position ([#7320](https://github.com/blairham/sh/issues/7320)) ([7a06f95](https://github.com/blairham/sh/commit/7a06f95128d7bc82693a1d788e91299b2db4972a))
* **interp:** a zsh subscripted literal places an empty element and refuses a negative subscript ([#7385](https://github.com/blairham/sh/issues/7385)) ([4f43692](https://github.com/blairham/sh/commit/4f43692d4549da2914eefdb55b651f677de54841))
* **interp:** an element's death by SIGPIPE leaves the outer PIPE handler alone ([#7314](https://github.com/blairham/sh/issues/7314)) ([9b52f23](https://github.com/blairham/sh/commit/9b52f23e2363794c9d59f462a152ed7930f4fa57))
* **interp:** an interactive ksh keeps a persisting prefix's entries ahead of a failed one ([#7349](https://github.com/blairham/sh/issues/7349)) ([251fa88](https://github.com/blairham/sh/commit/251fa88b48950a95e3dd963f4c48d321f23e5976)), closes [#7098](https://github.com/blairham/sh/issues/7098)
* **interp:** answer a ( ) or $( ) copy's signal before the list goes on ([#7360](https://github.com/blairham/sh/issues/7360)) ([170356f](https://github.com/blairham/sh/commit/170356f8cd5e8aa74c8b7d5fe6b66fe1f55e17bb))
* **interp:** answer a pipeline copy's signal to its body as the wait ends ([#7357](https://github.com/blairham/sh/issues/7357)) ([f3078eb](https://github.com/blairham/sh/commit/f3078eb35e48482b8d49335b1af55cc835f8820c))
* **interp:** answer a signal a child sent before it exited as the pipeline ends ([#7381](https://github.com/blairham/sh/issues/7381)) ([77667df](https://github.com/blairham/sh/commit/77667df282f2060ad17e56a1713a3f830b1fddbc))
* **interp:** answer a signal from another process as the pipeline ends ([#7376](https://github.com/blairham/sh/issues/7376)) ([a78013a](https://github.com/blairham/sh/commit/a78013a8083cd4df63f528da781ab895c1540982))
* **interp:** answer a signal from another process before an and-or list goes on ([#7368](https://github.com/blairham/sh/issues/7368)) ([d56112a](https://github.com/blairham/sh/commit/d56112a45eb675952ce20d958e3c90deb8755251))
* **interp:** answer a substitution's own kill $$ as its command ends ([#7374](https://github.com/blairham/sh/issues/7374)) ([8fa2f9e](https://github.com/blairham/sh/commit/8fa2f9e28005471b907cad4b7d9dfecbbc4a8dae))
* **interp:** bash's posix mode ends the shell on a refused subscripted prefix or reference aim ([#7344](https://github.com/blairham/sh/issues/7344)) ([355cba9](https://github.com/blairham/sh/commit/355cba93ce793ff004ac6c138b4eb8626a2cb346)), closes [#7343](https://github.com/blairham/sh/issues/7343)
* **interp:** every body around a subshell that signals the group dies of it ([#7313](https://github.com/blairham/sh/issues/7313)) ([139a027](https://github.com/blairham/sh/commit/139a027ccf11fa1be5a33a356891c0f84c5e1b73)), closes [#7245](https://github.com/blairham/sh/issues/7245)
* **interp:** ksh reads an appending subscripted literal over an indexed array that holds no element ([#7390](https://github.com/blairham/sh/issues/7390)) ([d6febc8](https://github.com/blairham/sh/commit/d6febc8e0d1bd3b6fd0fdd6331d1dbac2ae8f951))
* **interp:** move a gated walk's descriptors out of a park's way ([#7350](https://github.com/blairham/sh/issues/7350)) ([4d90b32](https://github.com/blairham/sh/commit/4d90b328ebf4d8c5148e6f4e60f35e5e5eef23a2))
* **interp:** name the signal that ended a body whose EXIT trap ran ([#7404](https://github.com/blairham/sh/issues/7404)) ([90d6808](https://github.com/blairham/sh/commit/90d6808cb231c4b475d2b598b76008842c08f79a))
* **interp:** open a regular file out of a park's way ([#7342](https://github.com/blairham/sh/issues/7342)) ([1ee6a31](https://github.com/blairham/sh/commit/1ee6a31c06d192d8927e4216ebb1bd9eb23d3ffe))
* **interp:** open a substitution's own pipe out of a park's way ([#7334](https://github.com/blairham/sh/issues/7334)) ([e730a82](https://github.com/blairham/sh/commit/e730a8272944c86149f6ea3e9d02eeb484879968)), closes [#7035](https://github.com/blairham/sh/issues/7035)
* **interp:** run a body's own handler as its kill returns ([#7353](https://github.com/blairham/sh/issues/7353)) ([1fd939a](https://github.com/blairham/sh/commit/1fd939af22dfa5f14f10ae180b3cfe11723ca708))
* **interp:** under set -m a $( … )'s own number names no group ([#7328](https://github.com/blairham/sh/issues/7328)) ([31c64b8](https://github.com/blairham/sh/commit/31c64b81f30c08b207269585cc3b841900fee021))
* **interp:** under set -m a body forked inside a forked body leads no group ([#7333](https://github.com/blairham/sh/issues/7333)) ([ccfadf9](https://github.com/blairham/sh/commit/ccfadf9eb95b2f5f50d2e62f2dca35666f8c3ad0))
* **interp:** under set -m a pipeline element and a &lt;(…) are the groups each dialect makes them ([#7326](https://github.com/blairham/sh/issues/7326)) ([267d65e](https://github.com/blairham/sh/commit/267d65e5e94edb8e135599871ea455394f4c1562))
* **interp:** under set -m a subshell's group kill reaches its own job, not the shell ([#7319](https://github.com/blairham/sh/issues/7319)) ([8eeb3b2](https://github.com/blairham/sh/commit/8eeb3b2be00d41dc9b5eb39eb08bb5b8d17fd2da))
* **interp:** zsh matches each word behind a declaration's assignment operand as it is expanded ([#7382](https://github.com/blairham/sh/issues/7382)) ([858c7a7](https://github.com/blairham/sh/commit/858c7a74a7886e047c2d9e7c4fa75b8bd7761da7))
* **interp:** zsh traces an arithmetic command's open line, a body's declaration and an empty for initializer as zsh does ([#7375](https://github.com/blairham/sh/issues/7375)) ([bf3f5aa](https://github.com/blairham/sh/commit/bf3f5aa9ce9fc9648cc9b225c3e79753cb96007c)), closes [#7108](https://github.com/blairham/sh/issues/7108) [#7117](https://github.com/blairham/sh/issues/7117) [#7118](https://github.com/blairham/sh/issues/7118)
* **interp:** zsh writes no trace line for a modifier that runs nothing ([#7348](https://github.com/blairham/sh/issues/7348)) ([ffe42b5](https://github.com/blairham/sh/commit/ffe42b57e8636386e6cb76f4b535ffad1df6a5d2))
* **interp:** zsh's declaration words outrank a same-named function, and command local declares under posixbuiltins ([#7379](https://github.com/blairham/sh/issues/7379)) ([65ca149](https://github.com/blairham/sh/commit/65ca1492ee810da0177d021e620f646ef584cb3f)), closes [#7127](https://github.com/blairham/sh/issues/7127) [#7128](https://github.com/blairham/sh/issues/7128)


### Performance

* **driver:** open darwin's signal pipe without the signal-mask thread ([#7400](https://github.com/blairham/sh/issues/7400)) ([735a212](https://github.com/blairham/sh/commit/735a2126307ea36fd83063930b1a1f8956547d46)), closes [#6491](https://github.com/blairham/sh/issues/6491)
* **driver:** the start no longer waits for the watch to take the signals the runtime drops ([#7264](https://github.com/blairham/sh/issues/7264)) ([59ff1e9](https://github.com/blairham/sh/commit/59ff1e9e8d7f36cadf7ee9bcba586d980599d36a))
* **interp:** put an empty shadow over a still-plain name back in one step ([#7405](https://github.com/blairham/sh/issues/7405)) ([0be21bd](https://github.com/blairham/sh/commit/0be21bd158e52da343148274e925bc1241a1bb67))

## [0.0.39](https://github.com/blairham/sh/releases/tag/v0.0.39) and earlier

Releases up to `v0.0.39` predate this file. Each one's notes are on its
GitHub release page.
