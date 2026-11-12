

## [v0.15.0] — 2026-11-12

- - feat(config): WithSkipUnmarshaler——严格模式对自带 UnmarshalYAML 的类型整段跳过通用递归，宿主专门裁决不再被通用文案降级、双判定面收拢（#127）；缺省行为不变，选项显式开启
- refactor(config): setScalarFromString 标量解析改引 pkg/config.SetFromString 单源，core 侧「字符串按 reflect.Kind 解析落位」平行实现删除；durationType 判定单源 pkg/config.DurationType（#129，依赖 pkg v0.46.0）；**错误文案口径统一**：default/env 值解析失败的错误由英文短句（"is not a valid bool" 等）变为 pkg/config 的中文口径（"布尔值非法 %q" 等，本包 field/source 前缀保留）——对错误文案做正则/子串匹配的调用方需要同步；[]string 逗号切分（元素去空白、丢空段）语义不变
- refactor(config): deepCopyValue 单源 pkg/reflectx.DeepCopyValue——core#102 指针环防护与 core#84 struct 深拷贝强语义随实现单源，pkg/pagination 同批收敛（#130）
- refactor(config): validateEnvNaming 与 LintEnvNaming 的判定内核收敛 envNameMismatch，lint 与硬校验口径不再分叉（#131）
- refactor(network): IsPublicIP 特殊段判定改引 pkg/urlguard.IsBlockedIP——ipv6SpecialCIDRs 列表与 IPv4 字节级 switch 删除，IANA 特殊段知识三处手工对账收拢 pkg 单源（#132）；判定结果与清单逐段一致有 golden 测试钉住
- refactor(config): isZeroValue 手写零值判定改 stdlib reflect.Value.IsZero（#133）；**行为变化**：非 nil 空切片字段不再算零值——`field: []` 不被 default/env 覆盖、退化 require 校验视为已提供（LoadBootstrap 默认接线 FieldSet，主路径不变；直调 FillDefaults/ApplyEnv/ValidateTags 不带 FieldSet 时生效）
- refactor(network): collectInterfaceIPs 单源取址，GetLanIPs/ServerIPString/ServerIP/GetLocalIP 四处遍历循环收敛为谓词 + 归约（#134）
- refactor(chain): reportIsolated 收敛 report func() 单参形态，reportIsolatedPtr 删除（#135）
- refactor(lifecycle): Shutdown 错误面 panic 收编单源 pkg/errors.Recovered——panic(err) 身份不丢（errors.Is/As 可直达原始错误），文本由 "panicked: %v" 变 "panic: %T: %v"（#136）；对文案做匹配的部署门禁需同步
- fix(config): ConfigWatcher.Reload 的 onChange 队列化投递——入队留 dispatchMu 内（保序 = Swap 序），执行在独立 dispatch goroutine（锁外）：回调内重入 Reload、跨 goroutine 等待 Reload、慢回调卡投递三类死锁/阻塞形态消除（#137）；**行为变化**：onChange 不再在 Reload 调用栈上同步执行（Get 在 Reload 返回时已见新值，回调随后按序送达）；回调 panic 就地收编为 ERROR 日志（pkg/errors.Recovered），dispatch 循环存续——历史行为是打死进程面
- feat(transport): 新增 transport/hertzbridge 子包——hertz RequestContext/RequestHeader/ResponseHeader → transport 抽象的适配单源（middleware#177 中期方案、middleware#180 落点）：NewServerTransport/NewServerContext/NewRequestHeader/NewReplyHeader，Get/Values 键匹配 EqualFold 大小写不敏感（hertz 侧 #163 的正确口径），transport 根包保持仅标准库；middleware 与 hertz 两仓 wave 3/4 改引
- 战役级：Go 地板 1.26.0（go.mod + CI go-version '1.26'，job 名不变）；新增依赖 github.com/oekit/pkg v0.46.0（errors/config/urlguard/reflectx）

本版使用的现代语言特性：reflect.TypeFor（yaml.Unmarshaler 接口类型与 DurationType 的声明）；min 内建（strict.go 编辑距离 DP，minInt 删除）；slices.Sort（严格模式合法键与 hertzbridge Keys 的排序）；range-over-int（reflectx 深拷贝循环）；泛型（ConfigWatcher[T] 的 pendingChange[T] 队列投递）。

## [v0.14.2] — 2026-11-11

- fix(network): GetLocalIP 的 POD_IP 校验补上**非回环 / 非未指定**语义判据——127.0.0.1（回环）与 0.0.0.0（未指定）不再原样返回，与形状非法（not-an-ip、带端口、IPv6）同路 WARN 后回退网卡探测（#120）：core#87 的判据只有 `net.ParseIP + To4`（只挡「不是 IPv4 形状」），而 doc 契约明写「非回环 IPv4」——`POD_IP=127.0.0.1` / `0.0.0.0` 形状合法、语义非法，此前 err=nil 原样直达注册面，与同文件 podIPFallback 对回环打 ERROR（「回环地址不可被其他节点拨号」）的口径相反，同一地址在两条路径一静默一报警；判据改 net.IP 标准方法（IsLoopback/IsUnspecified，v4-mapped 写法 `::ffff:127.0.0.1` 同判），WARN 文案改为「不是合法的非回环 IPv4」与判据同口径；顺带把网卡探测失败的 errors.New 收敛为包级哨兵 errNoLocalIP（errors.Is 可判，测试不再正则扒文案）

关联问题单：core#120。

本版使用的现代语言特性：errors.Is（errNoLocalIP 哨兵判定——测试对「网卡探测失败」走 errors.Is，替代逐字匹配错误文案）；strings.Cut（测试从 WARN 日志摘 pod_ip 属性值——`strings.Cut(out, "pod_ip=")` 切前缀取注入值原文，钉住可观测面第一现场带值）。

## [v0.14.1] — 2026-11-11

- fix(config): OverlaySource.Watch 的子源 goroutine **就地 recover**——消费方 onChange 回调 / 子源实现的 panic 收编为 ERROR 日志（值 + 栈），不再打死进程（#119）：此前每个子源的 Watch 在裸 goroutine 里执行（无 recover），而 onChange 是消费方回调（ConfigWatcher.Start → Reload → OnChange 注册的函数），回调 panic 发生在无人 recover 的 goroutine 里 ⇒ 整个进程崩溃，调用方自己 goroutine 上的 defer recover() 拦不到（recover 只对当前 goroutine 生效）；与 chain/timeout.go 的 goroutine 内 recover 纪律对齐；发生 panic 的子源监听终止、其余子源不受牵连；顺带清掉原实现里第二个裸 goroutine（wg.Wait 汇合协程——无观察者、无可 panic 路径，Watch 的契约本就是「启动监听后即返回」）
- fix(governance): Parse 对 json.Unmarshal 错误改**双 %w**——ErrInvalidConfig 哨兵与 *json.SyntaxError / *json.UnmarshalTypeError 同时在链上，errors.As 可辨、UnmarshalTypeError 自带的 Struct/Field 结构化定位不丢（#118）：此前 %v 把链拍平成纯文本，配置解析失败只能正则扒非契约文案分流（"of type map[string]int" 随版本/字段变动即断，kitx#70 同构换仓复发）；错误文案逐字不变（双 %w 与 %v 的渲染一致，只差链深度）
- fix(chain): Timeout(0) 定义为**显式无超时**——原样透传 next（不派生 ctx、不加 deadline，下游回到调用方 goroutine 上执行，Recovery 的 recover 恢复生效），负值构造期 panic（fail-loud，AGENTS.md Option 总则①）（#117）：此前 0/负值无检查地喂给 context.WithTimeout 立即到期，超时来自配置面时零值透传的服务「进程活着、全量请求瞬死」且无指向性日志；与 lifecycle 在 core#86 确立的「0 = 显式无超时」口径统一（governance.Validate 里「hertz timeout 中间件 0 立即到期」的过时注释同步勘误）
- fix(lifecycle): Manager.Shutdown 改**收敛循环**——每轮锁内取「尚未执行的尾部增量」、锁外执行，直到取空（对齐 bootstrap closeAll 范式），关停执行期间经 Add / AddTeardown 注册的 closer/teardown 不再被静默漏调（#116）：此前进入 once.Do 后一次性拷贝 jobs 快照、只执行快照内的任务，快照拷贝后追加的任务零日志零错误漏调、Errors() 恒空（nacos 延迟注册形态下 = 注册中心幽灵实例存活到心跳超时）；补齐执行保持注册序、失败与快照内任务同口径进 Errors()；Add/AddTeardown godoc 写明关停期注册契约

关联问题单：core#116、core#117、core#118、core#119。

本版使用的现代语言特性：Go 1.20 多错误包装（fmt.Errorf 双 %w，#118 修复本体）；errors.Join（#116 测试侧聚合 Errors() 后 errors.Is 逐一定位哨兵——运维对账的同款形态）；泛型辅助函数 pendingBatch[T]（#116，teardown/jobs 两个收敛循环共享同一「锁内取增量、锁外执行」原语）；for range int（Go 1.22 range-over-int，#116 测试的批量注册与 #119 测试的轮询等待）。

## [v0.14.0] — 2026-11-10

- - feat(config): WithTrimStrings/WithEnvTrimStrings——ApplyEnv/Load 字符串字段可选裁剪档，默认不裁（#104）

关联问题单：core#104。

## [v0.13.6] — 2026-11-10

- - fix(config): ApplyEnvOverlay 对未显式命中的字段不再强制 zeroable/overwrite 清零——struct 块下推合并、指针块 nil/零点不清 dst，env 显式零值形态保留（#101）
- - fix(config): deepCopyValue 按指针身份记忆已拷贝节点，指针环不再栈溢出，环内/DAG 共享保留且不别名（#102）

关联问题单：core#101、core#102。

## [v0.13.5] — 2026-11-09

- fix(network): IsPublicIP 补 0.0.0.0/8 与 fec0::/10 特殊用途段判定（#98）

关联问题单：core#98。

## [v0.13.4] — 2026-11-08

- fix(governance): 「路径前缀」策略生效——service 按「.」段前缀最长匹配（com.example 命中 com.example.svc）、path 按「/」段前缀最长匹配（/order 命中 /order/123，method 限定键 GET:/order 优先于裸 /order），LookupLimiterKey 返回**实际命中的配置键**而非恒为全局 ""（#78）：此前 lookup 只做两档精确匹配，前缀档整个缺失——按路径前缀配置的限流/超时对高基数路径静默失效，限流桶仍按原始 path 分片（正是 LookupLimiterKey 注释要避免的桶表无界增长）；段边界是硬约束（/orders 不被 /order 命中、com.examplex 不被 com.example 命中）
- fix(governance): Store 值与版本改为**单原子单元**发布——atomic.Pointer[cell{val,gen}] + CAS 推进代次，新增 LoadVersion 一次返回一致的一对（#79）：此前 val/version 是两个原子变量且「先换值、后进版本」，并发读侧在交错窗口拿到「新值配旧版本」的自相矛盾对（实测 20 万次 Update 观察到 345 次），按 Version()==last 判变化的消费方漏判、派生状态停留旧配置——与 config.Snapshot 同构范式
- fix(governance): Store.Update/NewStore 入参**入库前深拷贝**——发布方 Update 后复用/改写自己的 cfg 不再波及已发布快照（#80）：此前只存指针，快照被发布方静默改写而版本不前进（消费方按版本判「没变化」永久用旧派生状态），并发下还构成 map 读写数据竞争；「不可变快照」承诺对发布方与消费方同时成立
- fix(config): ConfigWatcher 并发 Reload 按**读取代号裁决**——迟到结果丢弃，「最新读到的配置」最终胜出（#81）：dispatchMu 此前只护 Swap+回调，Read/parse 在锁外——先发起的重载若解析慢，会在后发起的重载（已 Swap 新值）之后完成 Swap，把旧配置盖回新配置之上（热更后配置回退）；代号取在 Read 返回时刻（谁读到的源内容更新谁赢），非发起时刻
- fix(config): 严格模式放行 `yaml:",inline"` map 吸收的任意键（#82）：此前 inline map 字段不登记任何键，它吸收的键被上层结构体判成「未知配置键」直接拒载——合法 YAML 起不来（非严格模式正常）；inline map 的元素类型继续下降校验（元素是结构体时内部未知键照样拒载）；嵌套 inline（struct 内 struct 内 map）不上抛豁免——实测 yaml.v3 只在直接解码层兑现 inline map，嵌套层的键会被静默丢弃，严格模式判未知键正是拦「配置写了但哪儿都没去」
- fix(config): expandEnv 损坏拦截清单补**反斜杠**（#83）——双引号标量里 env 值的 `\n`/`\t`/`\\` 被 YAML 重新解释、值被静默改写（Windows 路径/正则/密钥常含 `\`），现 fail-closed 且错误带变量名；yaml 解析错误发生在 env 替换值上时（如注入 `"` 破坏双引号配对）用变量名包装报错（`wrapEnvParseError`：原文与展开一致/原文本身语法坏/严格模式键面错误除外），排查现场与配置现场对得上
- fix(config): OverlayMerge 的 merge:"overwrite" 结构体分支**深拷贝**（#84，同族英文重报单 #90 一并收口）：整体 Set 是 Go 值语义的浅拷贝，struct 里的 map/slice 字段仍与 src 共享引用——改 dst 反向污染 src；deepCopyValue 补 reflect.Struct 分支（先整块值拷贝保未导出字段、可 Set 的字段逐一递归替换）与 reflect.Array 分支（数组元素是 struct 时整体赋值同样共享），merge.go 数组字段分支同步深拷贝
- fix(config): ApplyEnvOverlay 支持**切片元素字段**的 env（#85）：overlay 此前是空结构体、切片全 nil，ApplyEnv 遍历 0 个元素——元素上的 env tag 与覆盖表条目（items.limit，yaml 键路径不编号）静默失效，与 ApplyEnv（yaml 优先路径元素存在、env 生效）行为相反；现按 dst 切片长度给 overlay 构造零值元素（[]struct/[]*struct），合并侧对带元素级命中路径的切片逐元素下推——env 命中字段覆盖 dst（含非零值）、未命中字段保持 dst 值；无命中时构造元素回缩为 nil，不触发整体替换清零
- fix(lifecycle): New(0)/Add(...,0)/负数 timeout 定义为**显式无超时**——closer 的 ctx 不带 deadline（#86）：此前 0 被直接喂给 context.WithTimeout 立即到期，每个 closer 拿到的都是已取消的 context，资源实际未关却对外打「shutdown completed」；文档对齐（New/Add 注释），SetDrainTimeout 的「负数不限时」口径同一
- fix(network): GetLocalIP 对 POD_IP 做 net.ParseIP 校验且要求 IPv4（#87）：注入串写错（not-an-ip、带端口、IPv6）此前原样返回且 err=nil——错误地址直达注册面，与同文件其余路径 fail-closed 口径相反；现非法值打 WARN（component=network.podip）后忽略、回退网卡探测
- fix(config): env 值首尾空白**字符串字段保留原值**（#88）：判定「是否未设置」用裁剪值（纯空白视同未设置），写入字符串字段用原始值——口令/token 两端空格是合法内容，此前被静默裁剪且与 ${VAR} 文本替换路径（本就不裁剪）给出不同结果；非字符串类型解析前仍裁剪（数值/布尔/duration 文法不容空白）
- docs(config): strict.go yamlFields 注释重写——原注释含 U+FFFD 乱码（匿名内嵌写成乱码三连）且宣称「匿名内嵌结构体同样按 inline 处理」与实现（yaml.v3 不拍平无 tag 内嵌）正好相反（#89）；全仓扫描 U+FFFD 无其余命中
- docs(changelog): v0.13.3 段首条文案勘误——tag 名补全（v0.13.2）、「新的地板」误写为上一版号更正（core 无内部依赖，无依赖地板变更）（#94）；GitHub Release 正文同步订正

关联问题单：core#77（验证收口——#77 是 #73/v0.13.2 的重报单：其全部样本地址（64:ff9b::7f00:1、64:ff9b::a00:1、2002:7f00:1::1、2001:db8::1、100::1 等）在 main 上复跑全部被拒，PR #74 已补的 IANA IPv6 特殊用途段清单覆盖无遗漏，本版补回归测试钉住样本清单，不重复修）、core#78、core#79、core#80、core#81、core#82、core#83、core#84、core#85、core#86、core#87、core#88、core#89、core#90、core#94。

## [v0.13.3] — 2026-11-08

- chore: 补齐本版版本节——上一个已发布 tag（v0.13.2）的 CHANGELOG 快照缺本版段（tag 快照不可变），本版段于 v0.13.3 发布后补记（core 无内部依赖，无依赖地板变更；勘误 core#94：原句 tag 名缺失、「新的地板」误写为上一版号）

## [v0.13.2] — 2026-11-08

- fix(network): IsPublicIP 拒绝 IANA IPv6 特殊用途段——To4() 为 nil 不再整族放行（#73）：IPv4 侧特殊段逐一拒绝，但 ip.To4() 对 IPv6 返回 nil，整个 IPv6 绕过段判定直接 return true（文档段 2001:db8::/32、discard-only 100::/64、benchmarking 2001:2::/48、ORCHID 2001:10::/28、Teredo 2001::/32、6to4 2002::/16、NAT64 64:ff9b::/96、SRv6 5f00::/16 等全部被当公网放行——::127.0.0.1 写法绕过回环门禁）；修法：To4() 为 nil 分支用一张 package-level []*net.IPNet 固定清单逐段 Contains 判定，命中即 false，IPv4 侧逻辑不动
- docs: AGENTS.md DESC 行对齐矩阵真相源（refresh_agents_desc）
- docs: 版本引用对齐最新 tag（refresh_versions）

关联问题单：core#73。

## [v0.13.1] — 2026-11-07

- fix(network): IsPublicIP 重写——net.IP 标准判定 + IPv4 特殊用途段拒绝（CGNAT/TEST-NET/保留段）+ IPv6 全球单播正确放行，此前 IPv6 恒 false、组播/保留段恒 true（#53）
- fix(network): ServerIP 内外网归类改用 IsPrivate——172.16/12 私网回归内网、字符串前缀死分支删除，IPv4/IPv6 同一条判定线（#54）
- docs(config): parseInterval 括号口径修正——只接受 [/( 开头、]/) 结尾，删除「方向不敏感」的错误声明（#58）
- fix(config): expandEnv 对 YAML 上下文危险值（换行/回车/注释序列）fail-closed 拒载——env 值静默损坏面关闭，错误带变量名不回显值（#59）
- feat(config): WithLenientEnv 宽容档——不可解析 env 从拒载改为 WARN+跳过、default 可兜底，缺省仍 fail-closed（#51）
- feat(config): ApplyEnvOverlay env 命中即覆盖（含零值）——显式 env 关布尔成为可能；OverlayMerge 本体语义不变，新增 OverlayMergeExplicit（#50）
- fix(network): GetPodIP 回退 127.0.0.1 前打 ERROR（component=network.podip）——回环注册从静默 fail-open 转可观测，签名与返回值不变（oekit#141 core 侧）

关联问题单：core#50、core#51、core#53、core#54、core#58、core#59。

## [v0.13.0] — 2026-11-07

### feat(config): `Snapshot[T]` 热更快照——原子发布 / Get 永不返 nil / 值与代次一致一对（[#49](https://github.com/oekit/core/issues/49)）
- Nacos 配置热更的标准消费形态是「`Listen` 回调里 `atomic.Pointer.Store` 新配置，
  读取方每次 `Load` + nil 判空兜底」，而本包此前只有 `ConfigWatcher[T]`——它管的是
  **来源**（Read/Watch/解析/重载节奏），且必须先 `Reload` 一次才有值，`Get` 在那之前
  返 nil。**不实现 `ConfigSource` 的热更入口这一格是空的**：Nacos 的
  `Listen(func(*T))` 回调、K8s informer、管理端点的配置 POST——它们只是「把一份已
  解析好的值交过来」，没有 Read/Watch 可实现。
- 手搓那套每次都要重新回答三个问题，而每个服务的答案都不一样：
  - **未发布过时读到什么**——零值？包级默认？panic？答案是「零值」时，服务会在
    配置还没到的那几百毫秒里用**空配置**正常提供服务（端口 0、超时 0 意味着不超时），
    失效方向是静默的；
  - **读取方要不要判空**——只要 Get 可能返 nil，判空样板就随消费点线性扩散
    （消费方实证：单个 handler 内两次 `GetConfig()` 两次判空），漏写一处就是
    nil 解引用 panic；
  - **「配置换过没有」**——指针比对（`if c != last`）回答的是「指针不同」而不是
    「发布过 n 次」：Nacos 长轮询拿到**相同内容**也会回调，A→B→A 的 ABA 序列更是
    直接骗过它，而症状是「配置换过了但派生态没重建」。
- 新增 `Snapshot[T]` 与五个入口：`NewSnapshot(defaults)` / `Get()` / `Load()` /
  `Publish(v)` / `Version()`。五条契约：
  - **Get 永不返 nil**：默认值在构造期注入，判空样板不再随消费点扩散；
    `NewSnapshot(nil)` 直接 panic——允许 nil 默认值等于让契约在运行期静默失效。
    想表达「还没有配置」看 `Version() > 0`，不要用 nil。
  - **值与代次绑在同一个发布单元里**：`Load()` 拿到的是**一致的一对**。把值与代次
    拆成两个原子变量的实现，读者会在跨字段的交错窗口里读到「新值配旧代次」——
    症状是「配置换过了但读到旧值」，比崩溃难查得多（同族教训：netguard 曾出现
    「快照 gen=6、全局 gen=5」自相矛盾的一对）。
  - **代次是发布单元的投影，不是第二个计数器**：每次成功发布从**观察到的**上一代
    推导下一代（CAS 循环），并发发布者各自重试 ⇒ 代次唯一、单调、不回卷。
    默认值代次 0。非原子的读-改-写会让两个并发发布者拿到同一个代次（实测注入：
    8000 次并发发布只推进到 3123 代），而代次重复意味着「换过没有」给不出答案。
  - **发布出去的值视为不可变**：`Get` 返回的就是发布的那个指针（零拷贝），发布者
    在 Publish 之后不得再改它的字段——就地修改会让读取方看到撕裂状态，而这类数据
    竞争不会稳定复现。
  - **`Publish(nil)` 被忽略**：保持上一份好值、代次不前进。这是刻意的取舍——
    Publish 必须**无返回值**才能直接当 `Listen(func(*T))` 回调传（多一层闭包正是
    「忘了判 nil」的高发点），所以「拒绝了」无法经返回值报告；而能发出 nil 的只有
    调用方自己的解码器或第三方 SDK 的兜底回调，Publish 最常被调用的位置是**别人
    SDK 的 goroutine**——那里没有我们的 recover，一次 panic 就是进程级退出。
    两种失效方向不对称：保留上一份好值 ⇒ 配置停在旧版本（服务继续跑、可观测）；
    panic ⇒ 进程死掉。选前者，并让它可观测（看 `Version()` 有没有前进）。
- `config/doc.go` 增「热更快照（Snapshot）」节，写明与 `ConfigWatcher` 的**分工**
  （Watcher 管来源、Snapshot 管值，需要源驱动的自动重载就在 `OnChange` 里
  `snap.Publish`），免得后人以为同一包里有第二份实现。`String()` 只打印类型与代次，
  不打印配置内容（配置里常有密钥，直接丢进日志的形态很常见）。
- 测试：nil 默认值 panic / 零值不可用 / 未发布返默认值且代次 0 / 发布后可见并推进
  代次 / `Publish` 的签名可直接当回调（类型层断言）/ nil 发布保留上一份好值且不推进
  代次 / 单发布者 + 四读者的「值与代次同代」自证（2 万轮）/ 四发布者并发
  「代次不丢」（8000 次）/ `-race` 下读写混跑 / `String()` 不含配置内容。
  `snapshot.go` 逐函数覆盖率 100%，`config` 包 86.7%。
- **反向验证四处**（注入「能编译但语义坏」的实现 ⇒ 用例必红，均已还原）：值与代次
  拆两个原子变量 ⇒ 「值与代次来自不同代」红；代次改回非原子读-改-写 ⇒ 8000 次发布
  只推进 3123 代红，**同时**「一致一对」用例仍绿（说明两条判据各管一件事，不是重复
  断言）；不忽略 nil 发布 ⇒ 红；允许 nil 默认值 ⇒ 红。

## [v0.12.2] — 2026-11-05

- fix(config): `WithStrict` 拒绝 YAML merge key（`<<`）⇒ 合法锚点合并配置无法加载（[#47](https://github.com/oekit/core/issues/47)）——`<<` 是 YAML 规范内的合法合并语法、yaml.v3 解码层原生支持，严格模式的职责是拦**拼错的键**，不该拦**合法语法**；修复前多环境 base+override 这类公共段复用配置在严格模式下一律拒载，错误信息还把 `<<` 列为「是否想写别的」的拼错候选，误导排查方向（下游 bianque 因此被迫自叉一份严格递归）。修法：`walkStrictMapping` 对 `<<` 放行键名校验，merge 值按**本层类型**继续走严格递归（比「整段跳过」更严丝合缝——merge 展开后的未知键依然拒载）；覆盖 yaml 规范的两种形态：单映射（含锚点别名，AliasNode 自动解引）与映射序列（多锚点按序合并）。锚点挂在未知键上（`_base: &base ...` 的顶层草稿键）仍按未知键拒载——那是真未知键，不是语法。补 6 个回归用例（merge 放行/内容校验/别名/序列/草稿键仍拒载）

关联问题单：core#47。

# Changelog — oekit/core

由 ekit v0.34.0（git.enjoye.top/enjoydream/ekit，冻结维护）拆分而来。所有 notable 变更记录如下。

## [v0.12.1] — 2026-10-28

- fix(chain): `Timeout` 把下游放到新 goroutine 执行，而 `Recovery` 的 recover 装在**调用方** goroutine 上 ⇒ 下游 panic 根本到不了 Recovery，会打死整个进程（canonical 组合 `chain.Finalize(h, chain.Recovery(), chain.Timeout(d))` 正是这个顺序 ⇒ Recovery 的 godoc 在组合使用时是假的）。修法：在 Timeout 的 goroutine 内 recover 并返回 `*PanicError`；`Recovery` 对下游以 `*PanicError` **形式返回**的情况也走上报钩子
- fix(lifecycle): `Listen` 补 `defer signal.Stop(ch)`——`signal.Notify` 会关掉信号的默认行为（终止进程），不 Stop 的话 Listen 返回后第二次 Ctrl-C / 容器 SIGTERM 不再能终止进程，编排器只能升级到 SIGKILL 才能收尸

## [v0.12.0] — 2026-10-22

### feat(config): env 命名策略——全局前缀 + 逐字段覆盖（[#32](https://github.com/oekit/core/issues/32)）
- 字段的 env 变量名此前只能由结构体作者逐字段写死（`env:"NAME"` tag），
  「约定名」（由 yaml 键推导的 SCREAMING_SNAKE）只被 `LintEnvNaming` /
  `WithEnvNamingLint` 用来**检查**名字是否一致，并不能**决定**实际读的变量名。
  于是两类宿主接不上：用统一前缀的 12-factor 宿主（`PB_` / `APP_` / `SVC_`）
  必须给每个字段手写 tag，且**无法**让没打 tag 的字段也按约定参与 env 读取；
  历史 env 名与 yaml 键对不上、需保留兼容的宿主则只能改结构体 tag——
  结构体不是自己的就改不动。
- 新增 `EnvNaming` 策略（前缀 + 逐字段覆盖）与三个入口（#32）：
  - `WithEnvPrefix(prefix string) LoadOption`——全局前缀，加在**本包推导的名字**
    上（env tag 名与路径派生名），覆盖表里的名字是绝对名不受影响；
    `WithEnvPrefix("PB_")` 后 yaml 键 `upload.local_import.enabled` 读
    `PB_UPLOAD_LOCAL_IMPORT_ENABLED`。
  - `WithEnvOverrides(m map[string]string) LoadOption`——逐字段指定 env 变量名，
    键为 **yaml 键路径**（与严格模式诊断路径同口径：嵌套用 `.` 连接、切片元素
    不编号、`yaml:",inline"` 的子字段拍平到父级），值为空串表示**该字段不走
    env**（显式豁免，压过 env tag 与前缀）。
  - `WithEnvNaming(n EnvNaming) PassOption`——给单独调用 `ApplyEnv` /
    `ValidateTags` / `ApplyEnvOverlay` / `ApplyTagPasses` 的场景（12-factor 的
    env 优先接线走 `ApplyEnvOverlay`）。
- 名字解析顺序：覆盖表 → env tag → 路径派生名。**未启用策略时行为与历史逐字
  一致**：只有打了 env tag 的字段读 env。加了前缀后多一层能力——没打 env tag
  的字段也按路径派生名参与 env 读取（「前缀即约定」，宿主不必逐字段写 tag）。
- 命名一致性检查（`WithEnvNamingLint` / `LintEnvNaming`）在新策略下同步调整：
  覆盖表命中的字段豁免（最终名由表决定）；设了前缀时约定名改用**路径级**
  派生名（与运行时实际读的变量名同口径），前缀相同的两条路径派生名撞车
  （`a.b` 与 `a_b` 都推出 `A_B`）会被判为重名。
- env 卫生检查改为校验**最终生效的名字**（覆盖表 / 前缀 / tag 解析之后），
  而非声明名——校验声明名而不校验生效名，等于放行了一个永远 miss 的变量名；
  非法前缀与非法覆盖名在 `ValidateTags` 阶段 fail-closed（否则就是「配了没
  生效」的静默失效）。重名判定按 yaml 键路径去重：同一逻辑字段的多个实例
  （切片元素）不再被误判为重名。
- `require` 字段缺失时的错误提示改为指向**真正生效**的变量名（加前缀后不再
  指向那个不生效的旧名）。
- `ApplyEnvOverlay` / `ApplyTagPasses` / `LintEnvNaming` 新增可选策略参数
  （变参形式，既有调用方零改动）。

## [v0.11.0] — 2026-10-21

### feat(config): 加载入口按来源拆开——`LoadFile`（任意路径）/ `Unmarshal`（字节源）（[#25](https://github.com/oekit/core/issues/25)）
- v0.10.0 的严格模式只有**一个**入口 `LoadBootstrap[T]`，而它的**查找路径写死**
  在 `etc/config.yaml` 约定上——严格模式、`${VAR}` 展开、tag 接线全封在这条固定
  路径的加载线里。**不遵循 etc/ 约定的宿主吃不上**：CLI `-config` 任意路径、
  容器内镜像内固定挂载位（如 `/app/plugins.yaml`）、配置来自 API 下发 / 模板渲染
  产物 / 测试夹具。
  实锤：argus（v3.159.0）评估后接不上，退回 `yaml.v3` 的 `KnownFields(true)`——
  拒载语义有了，但**嵌套路径、合法键清单、拼写建议这三样最有价值的诊断全丢了**，
  等于 #20 的核心收益对这类宿主不可达。
- 新增两个入口（#25）：
  - `LoadFile(path string, dst any, opts ...LoadOption) error`——**任意路径**；
  - `Unmarshal(data []byte, dst any, opts ...LoadOption) error`——**字节源**。
- **三个入口共享同一条解码线**（`unmarshalInto`：展开 `${VAR}` → 可选严格判定 →
  解码 → tag 接线 ApplyEnv → FillDefaults → ValidateTags）。因此严格模式的诊断面
  **与来源无关**——不会因为「不是从 etc/ 约定路径读的」就退化成一条
  “field X not found in type Y”。
- `LoadBootstrap` 行为**逐字不变**：历史错误文案（`parse config file X: ...` /
  `apply struct tags for config file X: ...`）保持原样（内部用 `parseErr`/`tagErr`
  区分两类失败以保住文案），既有调用方零改动。
- 测试：两入口的 `*UnknownKeyError` 诊断（路径/合法键清单/拼写建议）逐项一致；
  任意路径加载；字节级入口同样走 tag 接线（env 优先于 default）；字节级入口同样
  展开 `${VAR}`；非严格模式仍忽略未知键；`LoadBootstrap` 错误文案不变。

## [v0.10.0] — 2026-10-20

### feat(config)：严格模式——未知键拒载 + 合法键清单 + 嵌套路径（[#20](https://github.com/oekit/core/issues/20)）
- `LoadBootstrap[T](envFlag string, opts ...LoadOption)` 新增可选参数（向后兼容）：
  传 `config.WithStrict()` 开启严格模式，**未知键直接拒载**，不再静默忽略。
- 错误信息（`*config.UnknownKeyError`）给出三件事：
  1. **嵌套路径**（YAML 键形式，如 `llm.base_ur`）；
  2. **该层合法键清单**（超 12 个时只列前若干并注明总数——200+ 字段全列是噪音）；
  3. **近似拼写建议**（编辑距离，如「是否想写 "base_url"？」）。
- 为何不用 yaml.v3 自带的 `Decoder.KnownFields(true)`：它只说
  「field X not found in type Y」——不告诉用户这一层有哪些合法键，也不给嵌套路径；
  对 200+ 字段的结构体，用户既不知道自己在哪一层，也不知道正确拼法。
- 合法键判定**与 yaml.v3 字段语义逐条对齐**（yaml tag 优先 / 无 tag 取小写字段名 /
  显式 `,inline` 拍平 / 内嵌未导出类型同样处理 / `yaml:"-"` 跳过）——不一致会把
  合法键误报成未知键。已用测试钉死（`TestStrictMatchesYAMLv3FieldSemantics`）。
- 默认行为不变（未知键仍静默忽略），严格模式是显式开启——它会让「之前一直能跑
  （因为键拼错被忽略）」的配置开始报错，切换前需确认现有配置无误。

## [v0.9.2] — 2026-10-19

### Fixed
- **breaking(检查行为)**：移除 `ValidateTags` 中依赖进程环境的 env「拼写冒烟检查」——会误报 `PATH`/`HOME` 等系统变量（合法配置在 CI/宿主机启动失败，如字段 `Path` + `env:"LOG_PATH"` 撞上系统 `PATH`），且漏掉无人设置的真拼写错误（[#8](https://github.com/oekit/core/issues/8)）

### Added
- `LintEnvNaming(v)`：纯静态命名一致性诊断（声明 env 名 ≠ 由 yaml key/字段名推导的约定名），返回诊断列表、不读环境、不 fail-closed；`WithEnvNamingLint()` 使 ValidateTags 对同一判定 fail-closed；字段 `ignorederived:"true"` 豁免有意自定义名
- `ValidateTags` 保留的 env 检查仅限静态项：变量名格式、同名校验重复

## [v0.9.1] — 2026-10-18

### Fixed
- `FieldSetFromYAML` / `ApplyTagPasses` 支持 `yaml:",inline"` 字段：通用 map 解码会把
  inline 子字段拍平到父级映射，而 inline 字段按字段名小写取 key、在父映射里永远查不到——
  inline 叶子的显式零值（`region: ""` / `shards: 0`）仍被 default 覆盖，恰是 FieldSet
  要消灭的那类静默错（v0.9.0 引入的缺口）。修复：以**同一映射**继续下推 inline 结构体
  （含 nil 的 `*struct` 按类型下推，不写入），叶子路径正常标记；3 例回归测试钉住

## [v0.9.0] — 2026-10-18

### Added
- `config.FieldSet` / `FieldSetFromYAML` / `WithFieldSet` / `ApplyTagPasses`：显式提供集合——YAML key 出现即算「调用方给了值」，`port: 0` 不再被 default/env 覆盖（[#5](https://github.com/oekit/core/issues/5)）。`LoadBootstrap` 默认接线；单独 `FillDefaults`/`ApplyEnv` 不带集合时保持历史行为
- `config` struct tag `require`：字段必须由 yaml/env 显式提供，default 不算（FillDefaults 永不填充 require 字段；ValidateTags 缺失时 fail-closed 并指出应设的 env 名）
- `ValidateTags` env 卫生：变量名格式（`[A-Za-z_][A-Za-z0-9_]*`）、同名校验重复；**拼写冒烟检查**——声明的 env 未设置而推导约定名（`backup_archive_format` → `BACKUP_ARCHIVE_FORMAT`）已设置时报错，拦住「tag 手误 → env 静默失效」

### Docs
- doc.go 补「零值歧义与 FieldSet」「require」「env 卫生」三节；明确全量 `*T` 指针化不是默认解，优先 FieldSet

## [v0.8.0] — 2026-10-18

- feat(config): `ApplyEnvOverlay[T](dst *T)`——「env 优先于配置文件」的 12-factor 形态（[#2](https://github.com/oekit/core/issues/2)：ApplyEnv 只填零值、yaml 优先，两者语义相反且此前未文档化）；LoadBootstrap 文档补优先级警告

## [v0.7.0] — 2026-10-17

### Added（go-kratos 对齐）
- `transport` 新包：协议无关传输层上下文抽象——`Transporter`（Kind/Endpoint/Operation/RequestHeader/ReplyHeader）、`Header`、`MapHeader`/`NoopHeader`、Server/Client ctx 存取；仅标准库，中间件/日志/trace 透传可一份实现跨 hertz/kitex 复用
- `chain` 新包：transport 无关中间件链——`Handler`/`Middleware`/`Chain`/`Finalize` + `Recovery`（PanicError + 可选上报钩子）/ `Timeout`；仅标准库，与 nns-endpoint.Endpoint、go-kratos Handler 同形
- `config.OverlaySource`：多 ConfigSource 组合（按序 JSON 深合并，后者覆盖前者；任一子源变化即触发 onChange），配合 `ConfigWatcher[T]` 实现多源热更新

## [v0.6.0] — 2026-10-17

### Added
- `lifecycle.Manager`：优雅关停资源管理器（自 [oekit/logx](https://github.com/oekit/logx) `logx/shutdown`
  迁入，与 `lifecycle.App` 契约同域）。注册 closer/teardown、信号监听、
  Drain 引用计数、per-job 超时、panic 隔离、`Errors()` 观测；`WithLogger`
  注入宿主 `*slog.Logger`。默认日志走 stdlib slog（component=shutdown），
  **lifecycle 包零外部依赖**
  （[oekit#1](https://github.com/oekit/oekit/issues/1)）

### Changed
- **breaking**：`config.ConfigWatcher` 日志接口改为 `*slog.Logger`（原 `logx.Logger`），
  `SetLogger(*slog.Logger)`。core 模块自此**不再依赖 logx**，消除 L0→L1 反向依赖
  （[oekit#1](https://github.com/oekit/oekit/issues/1)）

## [v0.5.0] — 2026-10-16

### Added
- `config` struct tag 增强（additive，不改现有行为；不做 go-zero 全套 mapping 引擎）：
  独立 tag `default:"8080"` / `env:"MYAPP_PORT"` / `range:"(0,100]"` / `options:"dev,prod"`，
  三个导出入口 `FillDefaults` / `ApplyEnv` / `ValidateTags`（共享单个反射 walker）；
  `LoadBootstrap` unmarshal 后自动接线 ApplyEnv → FillDefaults → ValidateTags，
  非结构体配置类型（如 map）跳过以保持旧行为
- 优先级（写死）：yaml 显式值 > env tag（env 有值且字段零值时才填）> default tag；
  零值歧义（yaml 显式 `port: 0` 与未写不可区分、会被 default 覆盖）见 config/doc.go
- 支持类型：标量（string/int*/uint*/float*/bool）、time.Duration（time.ParseDuration
  文法）、[]string（default/env 逗号切分）；range 仅数值字段（Duration 边界用
  duration 文法如 `(0,30s]`；四种区间 + 单端 `[a,)`/`(,b]` + 小数边界），
  options 仅 string 字段；tag 非法/值越界 fail-closed 报错且携带字段路径
  （嵌套 `Server.Port`、切片 `Items[1].Qty`）
- 遍历：嵌套 struct、*struct（nil 按需分配）、[]struct / []*struct；不遍历 map；
  跳过非导出字段。`ConfigWatcher` 不自动执行这些 pass（parser 由调用方提供，
  可在 parser 里自行调用）

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

## [v0.1.0] — 2026-10-15

自 ekit v0.34.0（git.enjoye.top/enjoydream/ekit，冻结维护）拆分而来的初始公开版本，完成拆分稳定化：模块路径统一为 `github.com/oekit/core`，多轮拆分/合并收敛至当前模块边界，组织基建（共享 CI workflow、golangci v2、Dependabot）就位。
