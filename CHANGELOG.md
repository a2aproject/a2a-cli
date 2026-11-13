# Changelog

## [0.5.0](https://github.com/a2aproject/a2a-cli/compare/v0.4.0...v0.5.0) (2026-11-13)


### Features

* resolve --agent-card from AI Catalogs ([#152](https://github.com/a2aproject/a2a-cli/issues/152)) ([53f9904](https://github.com/a2aproject/a2a-cli/commit/53f9904564164e1d2a3c49a71dd39f59143876ff)), closes [#19](https://github.com/a2aproject/a2a-cli/issues/19)


### Bug Fixes

* bind the gRPC server's agent card listener to --host ([#133](https://github.com/a2aproject/a2a-cli/issues/133)) ([0d4d99e](https://github.com/a2aproject/a2a-cli/commit/0d4d99e1881401722b72a7dafa5049375fe5c23c))
* drop the agent URL's query string from error messages ([#142](https://github.com/a2aproject/a2a-cli/issues/142)) ([5c95123](https://github.com/a2aproject/a2a-cli/commit/5c95123d1c8d50a1cd6dfc2f28a14f5533a383ae)), closes [#140](https://github.com/a2aproject/a2a-cli/issues/140)
* escape local agent card paths when building the file URL ([#135](https://github.com/a2aproject/a2a-cli/issues/135)) ([40b9b79](https://github.com/a2aproject/a2a-cli/commit/40b9b795445fecc7df52391884519ced924d1b13)), closes [#134](https://github.com/a2aproject/a2a-cli/issues/134)
* let SSE keep-alives reset the --stream inactivity timeout ([#143](https://github.com/a2aproject/a2a-cli/issues/143)) ([4deb709](https://github.com/a2aproject/a2a-cli/commit/4deb709ddc03e238dfe4fcf4e13f5744bfe7e65a)), closes [#137](https://github.com/a2aproject/a2a-cli/issues/137)

## [0.4.0](https://github.com/a2aproject/a2a-cli/compare/v0.3.1...v0.4.0) (2026-11-04)


### Features

* add --ca-cert to trust a custom CA ([#119](https://github.com/a2aproject/a2a-cli/issues/119)) ([b8309d0](https://github.com/a2aproject/a2a-cli/commit/b8309d04db0e73f66f36f653b931b3db625a28d6))
* add --unix-socket and credential helpers ([#105](https://github.com/a2aproject/a2a-cli/issues/105)) ([0f6200c](https://github.com/a2aproject/a2a-cli/commit/0f6200ce076263e9b6f7ba3b9161d649feeeb1fa))
* add an interactive chat command ([#86](https://github.com/a2aproject/a2a-cli/issues/86)) ([c65d313](https://github.com/a2aproject/a2a-cli/commit/c65d3131f39f8ca7a176c0aca1a7229362188680))
* add install script for macOS and Linux ([#120](https://github.com/a2aproject/a2a-cli/issues/120)) ([0234a8f](https://github.com/a2aproject/a2a-cli/commit/0234a8fb507783813baa59f6231e6f548fe18daa))


### Bug Fixes

* honor --timeout on card resolution and document wait/polling flags ([#94](https://github.com/a2aproject/a2a-cli/issues/94)) ([4c17dd2](https://github.com/a2aproject/a2a-cli/commit/4c17dd21364856c3ade9e5e89c8dbe9147fe6a5b))
* make error hints match the error class ([#107](https://github.com/a2aproject/a2a-cli/issues/107)) ([f50a110](https://github.com/a2aproject/a2a-cli/commit/f50a11081d8ee44c705fe3eb27d0009f9c04b5e9))
* print flag and argument errors as JSON under -o json ([#117](https://github.com/a2aproject/a2a-cli/issues/117)) ([b90755a](https://github.com/a2aproject/a2a-cli/commit/b90755a94699079562bceabaf02eaf60c85af662)), closes [#115](https://github.com/a2aproject/a2a-cli/issues/115)
* report a missing --file-part or --config file as a usage error ([#97](https://github.com/a2aproject/a2a-cli/issues/97)) ([7229469](https://github.com/a2aproject/a2a-cli/commit/72294695caa6e1952c9dd10accda292cb065b511)), closes [#96](https://github.com/a2aproject/a2a-cli/issues/96)
* report bad task list filters and missing push-config flags as usage errors ([#118](https://github.com/a2aproject/a2a-cli/issues/118)) ([a1809cb](https://github.com/a2aproject/a2a-cli/commit/a1809cbefbcd2ff22f47a9fb57a2062a5c3155c3)), closes [#116](https://github.com/a2aproject/a2a-cli/issues/116)
* report invalid --data-part JSON as a usage error ([#110](https://github.com/a2aproject/a2a-cli/issues/110)) ([c26bb76](https://github.com/a2aproject/a2a-cli/commit/c26bb7611631dd2f753853ac9e93983bdfe0f5b3)), closes [#109](https://github.com/a2aproject/a2a-cli/issues/109)
* send gRPC through --unix-socket ([#122](https://github.com/a2aproject/a2a-cli/issues/122)) ([c44eb01](https://github.com/a2aproject/a2a-cli/commit/c44eb0171384abf4b76dafc2d5d9b45f2dd2ed5f)), closes [#108](https://github.com/a2aproject/a2a-cli/issues/108)
* use TLS by default for built-in gRPC connections ([#114](https://github.com/a2aproject/a2a-cli/issues/114)) ([7510bd5](https://github.com/a2aproject/a2a-cli/commit/7510bd51b5379bad40be19f3feab7fc40ce28bad))

## [0.3.1](https://github.com/a2aproject/a2a-cli/compare/v0.3.0...v0.3.1) (2026-10-16)


### Bug Fixes

* keep the polling error cause in the --stream fallback ([#80](https://github.com/a2aproject/a2a-cli/issues/80)) ([4e9651c](https://github.com/a2aproject/a2a-cli/commit/4e9651c02ff35ce6105f39bb6958db094c960b77)), closes [#78](https://github.com/a2aproject/a2a-cli/issues/78)
* let --timeout bound HTTP transport requests ([#87](https://github.com/a2aproject/a2a-cli/issues/87)) ([f902547](https://github.com/a2aproject/a2a-cli/commit/f9025472336290f71448190c7ce8ba29b618885e)), closes [#84](https://github.com/a2aproject/a2a-cli/issues/84)
* **output:** show protocol version in card Interfaces ([#89](https://github.com/a2aproject/a2a-cli/issues/89)) ([1e1c175](https://github.com/a2aproject/a2a-cli/commit/1e1c175229ba4c0a11724caad4bfb08b21ae7517))
* print structured errors in -o jsonl mode ([#79](https://github.com/a2aproject/a2a-cli/issues/79)) ([a7f7bff](https://github.com/a2aproject/a2a-cli/commit/a7f7bff99c4b428e438c8b30f360cf664f96d139)), closes [#77](https://github.com/a2aproject/a2a-cli/issues/77)
* printing artifact text in streaming mode ([#90](https://github.com/a2aproject/a2a-cli/issues/90)) ([2252d75](https://github.com/a2aproject/a2a-cli/commit/2252d75ce86a6f27afed2d7412957cf404da2082))
* redact --svc-param values in config show ([#83](https://github.com/a2aproject/a2a-cli/issues/83)) ([4277703](https://github.com/a2aproject/a2a-cli/commit/42777034785e9e23ea5e9f5ba7a6350de4789208))

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
