# Changelog

## [3.0.0](https://github.com/WebDeveloperBen/tyche/compare/v2.0.5...v3.0.0) (2026-07-12)


### ⚠ BREAKING CHANGES

* unify CLI as tyche and add tyche.json config

### Features

* add codex interface for typed errors and response parsing ([af90d80](https://github.com/WebDeveloperBen/tyche/commit/af90d801f1ffac9ca7d72b311dd0b4b641fcb0d5))
* add go modernise and pre commit hooks and then run it across repo ([68e92f7](https://github.com/WebDeveloperBen/tyche/commit/68e92f7bb1619394d09a6029c088bf84e00c0f70))
* add multipart support implementation in servergen ([f04cdcf](https://github.com/WebDeveloperBen/tyche/commit/f04cdcf0dedebebc66beb56f8f1f9b310e6c84f7))
* add streaming handler support ([81400e6](https://github.com/WebDeveloperBen/tyche/commit/81400e6b41456a3245cb10fa272646102cab1482))
* add typed multipart support ([19f1977](https://github.com/WebDeveloperBen/tyche/commit/19f197779c5cb45d0764567ccb9354fa1bd584a5))
* allow easy registration of middleware ([a952315](https://github.com/WebDeveloperBen/tyche/commit/a9523154842ef5459946853af485506264b814ec))
* clean up typegen for clientgen ([38dadc7](https://github.com/WebDeveloperBen/tyche/commit/38dadc71050ea24e1aeb2169a155166b22d8b821))
* create clientgen for other golang consumers ([79f0670](https://github.com/WebDeveloperBen/tyche/commit/79f0670f393f6ae31eb85eb4d3641fc7234dc8d5))
* create type naming strategies for client genearted code ([7e03534](https://github.com/WebDeveloperBen/tyche/commit/7e035344009072de564a18078e17a20733682bc1))
* generate request support from input fields ([4ea9505](https://github.com/WebDeveloperBen/tyche/commit/4ea950571d368ad13c732bd957acc16432ec648d))
* strengthen typed runtime of text event streams ([3adde76](https://github.com/WebDeveloperBen/tyche/commit/3adde76ca93b81bb818c221d60e17a1d398caff6))
* unify CLI as tyche and add tyche.json config ([1cb3ad8](https://github.com/WebDeveloperBen/tyche/commit/1cb3ad847d3c9d13eb0109e8fb7426c5589fa5e8))


### Bug Fixes

* all responses aren't now typed as json ([14c8019](https://github.com/WebDeveloperBen/tyche/commit/14c8019199c486da17cc643f387c6bdba76e837e))
* all types to be created in main ([d00cc61](https://github.com/WebDeveloperBen/tyche/commit/d00cc6175a80b3c3407a5d6c2ebf131524ee2fa5))
* **ci:** add lefthook to catch out of date go.mod ([a720ed7](https://github.com/WebDeveloperBen/tyche/commit/a720ed79fa09d3602c812d2430f684902be8cdf7))
* **ci:** combine race and tests to just run once ([4136e2f](https://github.com/WebDeveloperBen/tyche/commit/4136e2fc599b0662d00db9a6d54053c8379d06a9))
* **ci:** gorealeser ([a4f0758](https://github.com/WebDeveloperBen/tyche/commit/a4f075857cdf753a5bc328a364ffc4e2a8490d6e))
* **ci:** prevent cache poisoning in ci ([4d208d7](https://github.com/WebDeveloperBen/tyche/commit/4d208d7a1249f7abbb2d3e7cc06df0ade6219a61))
* **ci:** split release flow and create tag before goreleaser ([5e02027](https://github.com/WebDeveloperBen/tyche/commit/5e02027b0edfc614311b8289b30553b2f4630ae3))
* **ci:** stop goreleaser creating duplicate releases ([63b5ebb](https://github.com/WebDeveloperBen/tyche/commit/63b5ebb7a20ca2d9dbab7a34b62e5a5d4360e6ff))
* **ci:** stop goreleaser creating duplicate releases ([#18](https://github.com/WebDeveloperBen/tyche/issues/18)) ([c481cea](https://github.com/WebDeveloperBen/tyche/commit/c481cea3d61896ec064d103f1e9bb2c405522ed7))
* **ci:** tag locally for goreleaser and make release job idempotent ([58384c0](https://github.com/WebDeveloperBen/tyche/commit/58384c00a1a5a985a4c402262fc5e4537111e130))
* **ci:** use latest versions of the actions ([df5ba14](https://github.com/WebDeveloperBen/tyche/commit/df5ba14d3017177b1ce208e5921d05b2c363f4e9))
* **ci:** use ref for checkout ([f5ab9cd](https://github.com/WebDeveloperBen/tyche/commit/f5ab9cdd2397b3d07c1b84b82ff0afd26cdc4f6e))
* **ci:** zizmor expanded braces finding ([4e846c3](https://github.com/WebDeveloperBen/tyche/commit/4e846c3845e34604d12bdaa8d6f13fcabdd544f3))
* clean up plugin middleware register ([b848ff9](https://github.com/WebDeveloperBen/tyche/commit/b848ff906f5b15fa4e63c049f3eace60218f0638))
* compression scope and responsibilities ([4cd3218](https://github.com/WebDeveloperBen/tyche/commit/4cd3218c3eb4345e4fe263089cf9ec601522c694))
* contract break in generated code ([84337dc](https://github.com/WebDeveloperBen/tyche/commit/84337dc44813a5cafd0a9227aae97d1a5b4ce9dc))
* correctness issues and race conditions across server and clientgen ([afb430e](https://github.com/WebDeveloperBen/tyche/commit/afb430e57bf3b7f9f30b8a22ca1dca043d42bbd6))
* correctness linting ([4da019e](https://github.com/WebDeveloperBen/tyche/commit/4da019e1d5f72abcabdd5e3afeff53768554dbdb))
* devtooling flow ([a017348](https://github.com/WebDeveloperBen/tyche/commit/a0173485aa0380ef1a19038174076f2f7f3028f1))
* harden CLI error handling and output after refactor review ([8dd802e](https://github.com/WebDeveloperBen/tyche/commit/8dd802ef11c8a91217426b378406f833ddc9dfee))
* integration tests to be self dependant ([1f37f55](https://github.com/WebDeveloperBen/tyche/commit/1f37f552a6e2a83a0ea5efc4786e1493badbbde4))
* linting and add commands to taskfile to run easily ([f684bbc](https://github.com/WebDeveloperBen/tyche/commit/f684bbc86b28efe060e09e7f3481f48015438c96))
* performance and correctness fixes ([b52b84e](https://github.com/WebDeveloperBen/tyche/commit/b52b84eaaa514275e8524a7dd4ad22aed3e53a58))
* put tools into mise so they install in ci ([9a25019](https://github.com/WebDeveloperBen/tyche/commit/9a2501938b528056a6a58e1e9ec7d7289a42a63d))
* redirect url risk ([c362f7e](https://github.com/WebDeveloperBen/tyche/commit/c362f7ef0f17ef8688223dfaf4c49602f8cce75c))
* reg bug ([603e460](https://github.com/WebDeveloperBen/tyche/commit/603e460b23439e8a65700d89eac01a6be8979a1a))
* release please and reset it ([2dfab54](https://github.com/WebDeveloperBen/tyche/commit/2dfab54c621ad156b3f0829af3d9580a281bed94))
* security weaknesses ([f1a783d](https://github.com/WebDeveloperBen/tyche/commit/f1a783dcb0c9fa7a6bf99bb039b3481c6d03ebc2))
* typegen ([61f88ad](https://github.com/WebDeveloperBen/tyche/commit/61f88ade0ded548b91a9d803f2d4150265640166))
* update go mod ([0ec22c4](https://github.com/WebDeveloperBen/tyche/commit/0ec22c4487a039400aa2210edb915b77ccf63245))
* update memory footprint of structs ([08a82b4](https://github.com/WebDeveloperBen/tyche/commit/08a82b48d10caeeb5dfa6897b5271836947f3571))
* vuln in golang std lib bump to latest ([d079fd0](https://github.com/WebDeveloperBen/tyche/commit/d079fd012d001055c3835c4e53afe5f723c9df8e))
* ws forwarding bug ([1764584](https://github.com/WebDeveloperBen/tyche/commit/1764584310aabadebcc943761e637a0a2b32be7c))
