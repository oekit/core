# Changelog — oekit/core

由 ekit v0.34.0（git.enjoye.top/enjoydream/ekit，冻结维护）拆分而来。所有 notable 变更记录如下。

## [v0.4.0] — 2026-10-16

### Added
- `config` 子包（自 oekit/registry v0.2.x 迁入，API 不变、仅 import 路径变化）：
  `LoadBootstrap[T]`（YAML 加载 + `${VAR}`/`${VAR:default}` 占位符展开，
  etc/config.yaml → etc/config-{env}.yaml 回退）、`LoadDotEnv`、
  `Get/GetInt/GetBool/...` 环境变量系列、`ConfigWatcher[T]` + `ConfigSource`
  热更新、`OverlayMerge[T]`

### 迁移
- `github.com/oekit/registry/config` → `github.com/oekit/core/config`
  （config 只依赖 yaml+logx；需要 Nacos 配置源的继续用 registry/nacos 的
  `NewConfigSource`，它现在实现 `core/config.ConfigSource`）

## [v0.3.3] — 2026-10-15

- v0.3.3

## [v0.3.2] — 2026-10-15

- v0.3.2

## [v0.3.1] — 2026-10-15

- v0.3.1

## [v0.3.0] — 2026-10-15

- v0.3.0

## [v0.2.0] — 2026-10-15

- v0.2.0

## [v0.1.0] — 2026-10-15

- v0.1.0
