# Changelog

All notable changes to this project will be documented in this file.

## [v0.5.0](https://github.com/somaz94/helm-chart-kit/compare/v0.4.0...v0.5.0) (2026-08-21)

### Features

- add the info severity, and report the storage class no chart can create ([44bfa8a](https://github.com/somaz94/helm-chart-kit/commit/44bfa8ad8385bc80bc6dbf8fb87d1c8215f743d0))
- add the SecretStore every platform overlay said the chart does not create ([28845bd](https://github.com/somaz94/helm-chart-kit/commit/28845bd58b688778d945f71d73ee678ebecac9b8))

### Contributors

- somaz

<br/>

## [v0.4.0](https://github.com/somaz94/helm-chart-kit/compare/v0.3.0...v0.4.0) (2026-08-21)

### Features

- give list and sync a --format json, and take the last text parser out of CI ([49577e6](https://github.com/somaz94/helm-chart-kit/commit/49577e611dcdb28d9e0c6b09859cb5613e16e873))
- add the platform axis, and the four GKE resources an overlay could not express ([352478b](https://github.com/somaz94/helm-chart-kit/commit/352478bdda8993a1e7a9d3615f6a825b68c52c15))
- group the catalog by purpose, add the base presets, and stop refusing a second workload ([a126c15](https://github.com/somaz94/helm-chart-kit/commit/a126c15537389d2b1308b121f12db6b14dda9859))

### Documentation

- bring the README preset list up to date, and pin it with a test ([533858c](https://github.com/somaz94/helm-chart-kit/commit/533858c9d0c79952a2445494243ae917169fc1f6))

### Continuous Integration

- assert on --format json, and fix the pipes that closed early ([9ad5b39](https://github.com/somaz94/helm-chart-kit/commit/9ad5b3933c3d511f605b93301896547a12108bb8))

### Contributors

- somaz

<br/>

## [v0.3.0](https://github.com/somaz94/helm-chart-kit/compare/v0.2.0...v0.3.0) (2026-08-20)

### Features

- add six presets, and an escape hatch for the refusals that had none ([23ba247](https://github.com/somaz94/helm-chart-kit/commit/23ba24776399ffaaee54518a815e36ba4751dc4e))
- report a disruption budget that blocks every node drain ([103ee54](https://github.com/somaz94/helm-chart-kit/commit/103ee543c5c3c80bd5ce0f0fef57bb7b33881fb1))
- report an unused Issuer and a Service port nothing declares ([437e6ee](https://github.com/somaz94/helm-chart-kit/commit/437e6ee132c93648139ad8606ec2930a38a2824e))
- add Istio VirtualService, DestinationRule and AuthorizationPolicy ([b13a88c](https://github.com/somaz94/helm-chart-kit/commit/b13a88c522b8b237b4063125f22bdaddceb3e5ca))
- add hck remove and hck sync ([f52639e](https://github.com/somaz94/helm-chart-kit/commit/f52639e4044bb9f09a12e3d9a23ad8223409bfd4))
- make the check rules a registry a chart can configure ([4396b22](https://github.com/somaz94/helm-chart-kit/commit/4396b22cb692fe501029d9be3d9248d181666cc8))
- group commands in help and lead with the three-command path ([2636a3d](https://github.com/somaz94/helm-chart-kit/commit/2636a3d8405168c509c9b069a2ca8c911fc32abf))

### Bug Fixes

- point a chart's scalers at the workload it actually renders ([6b375b1](https://github.com/somaz94/helm-chart-kit/commit/6b375b17040b3a2d2df5b1f097bc810d737183a6))
- compare the chart skeleton in hck sync ([b5c39c0](https://github.com/somaz94/helm-chart-kit/commit/b5c39c04afe1b25c06a6189276e7de812508f10a))

### Code Refactoring

- collapse the two overlay axes into one catalog type ([c276d77](https://github.com/somaz94/helm-chart-kit/commit/c276d77266eab7559c0a72b5b1cd1377c9f817a1))

### Documentation

- add Korean translations as -ko pairs ([ba1aaa1](https://github.com/somaz94/helm-chart-kit/commit/ba1aaa1a6c2d054df9c5ed76da4729aed44eda15))

### Continuous Integration

- assert the renamed skipped-rules line, and cover the new escape hatches ([00fa688](https://github.com/somaz94/helm-chart-kit/commit/00fa688a42f4d28e704281c95211bb056376ef26))

### Contributors

- somaz

<br/>

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

