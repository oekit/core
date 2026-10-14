# ekit-core

> Go 后端基础工具库——文本/数据工具层，框架无关。ekit 子项目矩阵的地基之一。

本仓库是 [ekit](https://git.enjoye.top/enjoydream/ekit)（`git.enjoye.top/enjoydream/ekit` v0.34.0）
拆分出的独立 Go module，属于 **yi-nology** ekit 子项目矩阵（Round-2 深拆粒度）。

## 安装

```bash
go get github.com/yi-nology/ekit-core@v0.2.0
```

## 包清单

- \`pkg/\` — errors（Wrap/Cause/BizError）、stringx、slicex、pathx（glob）、cache（进程内缓存）、convert、encoding、enumx、httpjson、jsonrepair、mathx、pagination、ptrx、timex（含 cron）、uidgen、validator
- \`governance/\` — 治理配置模板 schema（retry/timeout/circuit_breaker/limiter）
- \`lifecycle/\` — transport 生命周期契约
- \`network/\` — IP 工具（GetPodIP/IsPublicIP）
- \`version/\` — 版本信息

> 并发与流控原语（async/group/singleflight/progress/syncx/ratelimit/retry）已迁 [ekit-concurrency](https://github.com/yi-nology/ekit-concurrency)。

## 依赖关系

子项目矩阵与依赖方向（自底向上，19 仓）：

```
ekit-core            文本/数据工具 + governance/lifecycle/version      （零内部依赖）
ekit-concurrency     并发与流控原语                                     （零内部依赖）
ekit-logx            日志门面 + shutdown                                （零内部依赖）
ekit-istio           Istio Proxyless                                    （零内部依赖）
  ekit-metrics       Prometheus 门面                          → core
  ekit-tracing       OTel + traceid                           → logx
  ekit-registry      注册发现 + 配置                          → core, logx
    ekit-data        缓存抽象 + 定时任务                      → core, logx, concurrency
      ekit-gorm      SQL/GORM 全家                            → core, logx
      ekit-redis     Redis 全家                               → data, core, logx
    ekit-httpx       HTTP 客户端 + 负载均衡                   → core, concurrency, logx, metrics, tracing, registry
      ekit-nns       服务治理客户端                           → httpx, registry, core, logx, tracing, metrics
        ekit-bootstrap 应用引导
    ekit-middleware  Hertz 中间件栈                           → core, concurrency, logx, metrics, tracing, registry
    ekit-handler     HTTP handler 层（CRUD/绑定/版本）        → middleware, gorm, core, tracing
    ekit-hertz       Hertz 运行时/适配/instrumentation        → nns, registry, metrics, tracing, logx, core, concurrency
    ekit-kitex       Kitex 生态                               → nns, registry, core, concurrency, logx, metrics, tracing
  ekit-workflow      流程编排                                 → concurrency, logx
ekit-kitex-examples  Kitex 示例（echo client/server）         → kitex, bootstrap, registry, logx, tracing
```

> 注：`ekit-observability` 已在 Round-2 深拆中解散——logx 迁 [ekit-logx]、metrics 迁 [ekit-metrics]、
> tracing/traceid 迁 [ekit-tracing]、shutdown 迁 [ekit-logx]；原仓库已归档。

本仓库为 L0 地基，不依赖矩阵内其他模块。

## 与 ekit 单体仓库的关系

- 代码与 ekit v0.34.0 等价，module 路径 `git.enjoye.top/enjoydream/ekit/...` → `github.com/yi-nology/ekit-core/...`
- 各子仓库独立发版（SemVer）；开发规范沿用 ekit AGENTS.md 约定

## License

见 [LICENSE](LICENSE)
