# ekit-core

> Go 后端基础工具库——纯工具层，框架无关（不依赖 Hertz/Kitex/数据库），是 ekit 子项目矩阵的地基。

本仓库是 [ekit](https://git.enjoye.top/enjoydream/ekit)（`git.enjoye.top/enjoydream/ekit` v0.34.0）
按框架耦合分层拆分出的独立 Go module，属于 **yi-nology** ekit 子项目矩阵。

## 安装

```bash
go get github.com/yi-nology/ekit-core@v0.1.0
```

## 包清单

- `pkg/` — errors（Wrap/Cause/BizError）、stringx、slicex、pathx（glob 匹配）、retry（重试/轮询）、syncx（键控闸门/扇出）、cache（进程内缓存）、convert、encoding、enumx、httpjson、jsonrepair、mathx、pagination、ptrx、ratelimit（令牌桶）、timex（含 cron）、uidgen、validator
- `concurrency/` — async（GoSafe 系列 safe-go）、group、singleflight、progress（多阶段进度）
- `governance/` — 治理配置模板 schema（retry/timeout/circuit_breaker/limiter，JSON + Nacos 友好）
- `lifecycle/` — transport 生命周期契约（`App` 接口）
- `network/` — IP 工具（`GetPodIP`/`IsPublicIP`，注册地址解析的公共依赖）
- `version/` — 版本信息（ldflags 注入）
- `workflow/` 已独立为 [ekit-workflow](https://github.com/yi-nology/ekit-workflow)

## 依赖关系

子项目矩阵与依赖方向（自底向上）：

```
ekit-core                      纯工具层（零内部依赖）
  └─ ekit-observability        日志/追踪/指标门面
       └─ ekit-registry        注册发现 + 配置
            ├─ ekit-data       存储层
            ├─ ekit-network    网络客户端
            │    └─ ekit-nns   服务治理客户端
            │         └─ ekit-bootstrap  应用引导
            ├─ ekit-hertz      Hertz 生态
            ├─ ekit-kitex      Kitex 生态
            └─ ekit-workflow   流程编排
```

本仓库为地基，不依赖矩阵内其他模块。

## 与 ekit 单体仓库的关系

- 代码与 ekit v0.34.0 等价，仅 module 路径从 `git.enjoye.top/enjoydream/ekit/...` 改为 `github.com/yi-nology/ekit-core/...`
- 后续演进以各子仓库独立发版（SemVer）；ekit 单体仓库保留完整历史与架构文档（docs/PLATFORM_CONTRACT.md）
- 开发规范（命名/错误处理/并发安全/日志门面）沿用 ekit AGENTS.md 约定

## License

见 [LICENSE](LICENSE)
