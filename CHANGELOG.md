# Changelog

All notable changes to this project will be documented in this file.

## [v0.2.0](https://github.com/somaz94/helm-chart-kit/compare/v0.1.0...v0.2.0) (2026-08-20)

### Features

- add hck init to scaffold a chart by answering questions ([6842597](https://github.com/somaz94/helm-chart-kit/commit/68425978999280f6b17cf8882057d8bcf4b1f1aa))
- add dev, staging and prod environment overlays ([322fdcc](https://github.com/somaz94/helm-chart-kit/commit/322fdcc0c2bcf7721034ffdbb921af670449b7c8))
- add sealedsecret, issuer, referencegrant, scaledjob and grafanadashboard ([a83b76b](https://github.com/somaz94/helm-chart-kit/commit/a83b76b0d25c854a66197bc6debf7d4ebe26cf69))
- add platform values overlays for aws, gcp, azure and onprem ([e017b02](https://github.com/somaz94/helm-chart-kit/commit/e017b023ac3facb79d2af2a72c73807a1b4677ab))

### Bug Fixes

- resolve overlay key collisions and unrendered resource defects ([b832d15](https://github.com/somaz94/helm-chart-kit/commit/b832d15950f00a3a1bd660a1a5d1bc2ac49ac90c))

### Tests

- make the render coverage check assert what its name claims ([4fed7eb](https://github.com/somaz94/helm-chart-kit/commit/4fed7eb48c0f96b87cd786bee7cca4cc4cc1ae09))

### Continuous Integration

- retry mirror pushes on transient remote failures ([b1d0626](https://github.com/somaz94/helm-chart-kit/commit/b1d0626bcc81a02be2495435a7e26f628019accf))
- drop the dead issue-close trigger from changelog generation ([b7875fd](https://github.com/somaz94/helm-chart-kit/commit/b7875fd80a4c05008135ff769097e9199dba7efc))

### Contributors

- somaz

<br/>

## [v0.1.0](https://github.com/somaz94/helm-chart-kit/releases/tag/v0.1.0) (2026-08-20)

### Features

- add hck docs to generate the chart values table ([7aa33ab](https://github.com/somaz94/helm-chart-kit/commit/7aa33abdf0fa4edae0063f2a632551c737a191c2))
- add certificate, podmonitor, scaledobject and vpa resources ([cc811ea](https://github.com/somaz94/helm-chart-kit/commit/cc811eab983e72812f7868d0639c1c164342d2c3))
- add the daemon preset for DaemonSet node agents ([a64cc6f](https://github.com/somaz94/helm-chart-kit/commit/a64cc6fe9a6b03fab67db8253b57620f853685d9))
- generate values.schema.json with the new hck schema command ([3fd6148](https://github.com/somaz94/helm-chart-kit/commit/3fd614889c580400f5479d85d14c4fdaf3771e31))
- scaffold hck CLI with new, add and check commands ([af65488](https://github.com/somaz94/helm-chart-kit/commit/af65488256b0626b2766ca595e1b20a1ef596dd1))

### Bug Fixes

- refuse a second workload in hck new and report it in hck check ([c64938d](https://github.com/somaz94/helm-chart-kit/commit/c64938d6fdfaafa125afe42e5205f977b22393aa))
- make hck schema --check print a command that fixes the failure ([1882169](https://github.com/somaz94/helm-chart-kit/commit/188216932838198d3ef697d2295a8116929ab169))
- correct catalog ValuesKeys to match the values templates ([f46f73c](https://github.com/somaz94/helm-chart-kit/commit/f46f73c7668b80123e185f116dab21e9a5419e67))

### Code Refactoring

- dedupe schema resource sets and report contested keys ([63d87ae](https://github.com/somaz94/helm-chart-kit/commit/63d87ae6b9faa52d427ae1778845e15bee12d47b))

### Builds

- **deps:** bump azure/setup-helm from 4 to 5 (#1) ([#1](https://github.com/somaz94/helm-chart-kit/pull/1)) ([f033dc8](https://github.com/somaz94/helm-chart-kit/commit/f033dc880050c53a69e700645d591f8e4ccbe476))

### Contributors

- somaz

<br/>

