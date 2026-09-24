# Changelog

## [0.3.0](https://github.com/a2aproject/a2a-cli/compare/v0.2.0...v0.3.0) (2026-09-24)


### Features

* --save-fileparts flag for get, send and subscribe ([ed5d983](https://github.com/a2aproject/a2a-cli/commit/ed5d983c5df2635052b7a3fdcd8a87c776fbe981)), closes [#41](https://github.com/a2aproject/a2a-cli/issues/41)
* a2a skill command ([#72](https://github.com/a2aproject/a2a-cli/issues/72)) ([d459480](https://github.com/a2aproject/a2a-cli/commit/d4594803b74ae17e7f16ee2a3f49d547a66c9483))
* add command plugin framework for extending a2a CLI ([#58](https://github.com/a2aproject/a2a-cli/issues/58)) ([f255bb2](https://github.com/a2aproject/a2a-cli/commit/f255bb24188fa4a288a63ad5917b823f994e7600)), closes [#57](https://github.com/a2aproject/a2a-cli/issues/57)
* add support for setting global flags with config files ([#67](https://github.com/a2aproject/a2a-cli/issues/67)) ([4ee9f74](https://github.com/a2aproject/a2a-cli/commit/4ee9f74a2367502f3eb85489b65273963ece7fe4)), closes [#66](https://github.com/a2aproject/a2a-cli/issues/66)


### Bug Fixes

* disable command plugins by default ([#71](https://github.com/a2aproject/a2a-cli/issues/71)) ([6823186](https://github.com/a2aproject/a2a-cli/commit/682318625122dff30288ad4046823a5d9db76d12)), closes [#57](https://github.com/a2aproject/a2a-cli/issues/57)
* pass values from configs to transport plugin ([#73](https://github.com/a2aproject/a2a-cli/issues/73)) ([6745b8b](https://github.com/a2aproject/a2a-cli/commit/6745b8b77f0e61d817bd457a072c38c4d433870b)), closes [#20](https://github.com/a2aproject/a2a-cli/issues/20)
* report --stream inactivity timeouts as a timeout ([#75](https://github.com/a2aproject/a2a-cli/issues/75)) ([9465221](https://github.com/a2aproject/a2a-cli/commit/946522173f7a4e65644334b56fbfe8cf9ff7cdf1)), closes [#74](https://github.com/a2aproject/a2a-cli/issues/74)
* send --svc-param headers on the agent card request ([#63](https://github.com/a2aproject/a2a-cli/issues/63)) ([d8d9493](https://github.com/a2aproject/a2a-cli/commit/d8d94933d5c98113301d9fc809e899d34a060eca)), closes [#56](https://github.com/a2aproject/a2a-cli/issues/56)

## 0.2.0 (2026-09-09)


### Features

* a2a go as a base for the official cli ([#2](https://github.com/a2aproject/a2a-cli/issues/2)) ([9ef85ca](https://github.com/a2aproject/a2a-cli/commit/9ef85ca18ba6dc475d38a1093877a4462ec11fb7))
* add a2a-cli specification v0.2 reviewed ([#1](https://github.com/a2aproject/a2a-cli/issues/1)) ([6a0906c](https://github.com/a2aproject/a2a-cli/commit/6a0906c50971071ad86a16024f30887de82dcae4))
* cli -&gt; spec reconciliation for tier 1 features ([#9](https://github.com/a2aproject/a2a-cli/issues/9)) ([f9a143b](https://github.com/a2aproject/a2a-cli/commit/f9a143b52d22269ccf817889621f8c1e92c72cdb))
* cli pluggable transport ([#24](https://github.com/a2aproject/a2a-cli/issues/24)) ([fc49c26](https://github.com/a2aproject/a2a-cli/commit/fc49c26ff39a0c611c458a8b30195fdf0a16dc13))
* cli spec reconciliation part 2 ([#23](https://github.com/a2aproject/a2a-cli/issues/23)) ([a27e1ee](https://github.com/a2aproject/a2a-cli/commit/a27e1ee6d33241c2e5f1bce67c10ac3ef7f5e0a5))
* implement agent card file path support ([#22](https://github.com/a2aproject/a2a-cli/issues/22)) ([836c373](https://github.com/a2aproject/a2a-cli/commit/836c3738b6ba52e6922d6d8aaf34d11130bba7c5))
* implement get --wait support for polling-simulated blocking ([#42](https://github.com/a2aproject/a2a-cli/issues/42)) ([cd2c340](https://github.com/a2aproject/a2a-cli/commit/cd2c34008a8cfa20ef19e0a14aec6aa55f569d20))
* mplement config loading from .env files ([#12](https://github.com/a2aproject/a2a-cli/issues/12)) ([cb1c91b](https://github.com/a2aproject/a2a-cli/commit/cb1c91b60dcac90679a997b04a4c372755fa03a0))
* structured errors ([#44](https://github.com/a2aproject/a2a-cli/issues/44)) ([069bab3](https://github.com/a2aproject/a2a-cli/commit/069bab3f532c93bff97b910d0743402f99f5fc90))
* warn on --insecure with a credential, and render file parts by name/type/size ([#37](https://github.com/a2aproject/a2a-cli/issues/37)) ([91785b0](https://github.com/a2aproject/a2a-cli/commit/91785b035f1bd78ea7c411c15da4c71a8e3abdac))


### Bug Fixes

* address cli issues discovered during code audit ([#14](https://github.com/a2aproject/a2a-cli/issues/14)) ([fdbb3aa](https://github.com/a2aproject/a2a-cli/commit/fdbb3aad6289c0070480147a29e04c3d46aa6c2f))
* auth required ignored by polling wait ([#46](https://github.com/a2aproject/a2a-cli/issues/46)) ([27135fa](https://github.com/a2aproject/a2a-cli/commit/27135fa1b20031cb43af79f532ba98a537fb049e))


### Documentation

* restructure readme and setup brew & winget ([#47](https://github.com/a2aproject/a2a-cli/issues/47)) ([5a00f76](https://github.com/a2aproject/a2a-cli/commit/5a00f769282fc439567d5a12974334e08fec76ec))
