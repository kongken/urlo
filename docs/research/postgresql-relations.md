# S3 + PostgreSQL 关系数据调研

- 调研日期：2026-10-12。
- 代码基线：`a7ec7f0`；Butterfly Core `v0.0.0-20260427155459-f3485be49e45`。
- 状态：改造前的研究快照；首期 owner 索引已据此实现，DAO 后续按用户要求改用 Ent（底层仍是 pgx），Goose 迁移继续独立执行。实际配置、迁移与限制见 [storage operations](../storage.md)。
- 用户约束：短链创建、映射与解析继续使用 S3；只有部分关系数据使用 PostgreSQL，不引入 MongoDB。

本文区分调研基线的代码/官方文档事实和当时的设计建议。候选 SQL、配置、文件布局及“尚未实现”说明反映调研时点，不替代后续实现文档。

## 1. 结论与数据归属

建议采用 **S3 主记录 + PostgreSQL 关系 module**，而不是新增一个替代 S3 的 `url.Store`。

| 数据 | 权威来源 | 首期策略 |
| --- | --- | --- |
| code 唯一性、`long_url`、创建时间 | S3 | 保持当前创建及解析方式 |
| `OwnerID`、过期时间、禁用状态 | S3 | 保持现有所有者校验与解析语义 |
| owner → code 索引 | 从 S3 派生的 PostgreSQL 投影 | 第一阶段替代 S3 owner pointer |
| 标签、目录、收藏、用户设置 | PostgreSQL | 后续按实际需求逐项新增，不是可丢弃缓存 |
| 团队成员、共享权限、套餐关系 | PostgreSQL（若未来需要） | 需要单独的鉴权/事务设计，不随 owner 索引上线 |
| 点击事件、限流 | 当前 Redis 实现 | 本次不迁移；访问计数也暂不改变 |
| 第三方 URL 还原 | 不持久化 | 保持独立 |

当前 S3 `Record` 已包含 owner、过期、禁用及计数；访问事件通过独立的 `clicks.Recorder` 处理。[C1][C2][C3]

**两类 PostgreSQL 数据不能混为一谈：**

1. `owner_link_index` 是可从 S3 重建的投影。
2. 用户创建的标签、目录及关联是 PostgreSQL 权威业务数据，需要备份和恢复。不能清空索引时连带清空这些关系。

## 2. 当前代码与框架支持

### 2.1 S3 已经承担关系索引，但查询和恢复能力有限

`s3store` 创建 `<prefix>/<code>.json` 后，额外写入空对象 `<prefix>/owners/<ownerID>/<code>`。owner pointer 的创建和删除错误均被忽略。列表通过 S3 LIST 找 pointer，再逐个 GET 主记录；`Service.ListByOwner` 返回全部未过期链接，无列表分页。[C1][C2]

另外，当前 `s3store.ListByOwner` 获取主记录后不重新校验 `OwnerID`。若旧 pointer 残留，且删除后的 code 被另一个用户复用，旧用户列表可能返回新记录。新列表实现必须用 S3 当前 owner 再次过滤，不能把索引当授权依据。[C1][C2]

### 2.2 Butterfly 现有 SQL 接口不能直接接 PostgreSQL

核实锁定版本的源码：[C5]

- `internal/store/sqldb.go` 写死 `sql.Open("mysql", ...)`，DSN 为 MySQL 格式。
- `mod.DBConfig` 没有数据库 driver/连接 URL 选项。
- `store/sqldb.GetDB` 只返回上述注册的 `*sql.DB`。
- `store/gorm.NewDB` 使用 `gorm.io/driver/mysql`；`store/gorm.GetDB` 返回 `nil`。
- `app.Config.TeardownFunc` 虽已声明，`App.Run` 中没有调用它。

因此不能把 PostgreSQL 参数填进现有 `store.db`，也不能假设配置一个 teardown 回调就完成关闭。

**建议：** 首期在 urlo 内直接管理 `pgxpool`，在 `cmd/urlo` 负责构造、注入和关闭；未来多个项目需要 PostgreSQL 时，再给 Core 增加独立的 `store.postgres` 支持。不为这次调研升级框架或改造 MySQL 配置。

## 3. Go 驱动、SQL 与迁移工具

| 选择 | 核实结果 | urlo 建议 |
| --- | --- | --- |
| `pgx/v5` + `pgxpool` | PostgreSQL 原生 Go toolkit；有并发安全连接池及 `database/sql` adapter [P1][P2] | **首选运行时方案** |
| `database/sql` + pgx stdlib | pgx 提供兼容 adapter [P1] | 有依赖要求 `*sql.DB` 时再用，不必为了 Core MySQL 接口绕行 |
| sqlc | 支持 PostgreSQL，`sql_package: pgx/v5` [P3] | 可选；首期少量参数化 SQL 足够，多表关系增加时再生成类型安全查询 |
| goose/v3 | 支持 PostgreSQL、SQL 迁移、CLI；默认事务执行 [P4] | **首选 schema 迁移工具**，独立于服务启动 |
| golang-migrate/v4 | 支持 PostgreSQL、pgx/v5 和 CLI；上下迁移为独立文件 [P5] | 合理替代；仓库目前无既有迁移工具，二选一即可 |

核实候选版本 `pgx v5.11.0` 要求 Go 1.25.0，`goose v3.29.0` 要求 Go 1.26.0；本仓库为 Go 1.26.2。这里只确认声明的 Go 版本可满足，**未执行依赖解析、安装或编译验证**。[P1][P4][C4]

建议沿用部署平台已有、受支持的 PostgreSQL 主版本；本方案不依赖 PostgreSQL 18 独有功能，已有 16/17/18 均可作为后续验证目标。官方支持周期为主版本发布后五年。[P6]

## 4. 首期 schema：只做 owner 索引

以下为候选 DDL，不是可直接视为已验证的迁移：

```sql
CREATE TABLE owner_link_index (
    link_space      text COLLATE "C" NOT NULL CHECK (link_space <> ''),
    code            text COLLATE "C" NOT NULL
                    CHECK (code ~ '^[A-Za-z0-9]{1,32}$'),
    owner_id        text COLLATE "C" NOT NULL CHECK (owner_id <> ''),
    link_created_at timestamptz NOT NULL,
    indexed_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (link_space, code)
);

CREATE INDEX owner_link_index_owner_page_idx
    ON owner_link_index
       (link_space, owner_id, link_created_at DESC, code DESC);
```

设计说明：

- `link_space` 表示稳定的 S3 code 命名空间。不同 bucket/prefix 可以存在同名 code，不能在共用数据库中串数据；它不是 Butterfly 的 app namespace。配置需保证一个 space 对应一套 S3 主记录。[C1]
- code 大小写敏感。使用 `text COLLATE "C"` 保持字节序比较，不使用 `citext` 或大小写不敏感 collation；这也确定 cursor 的 code 排序规则。[C2][P7]
- owner 使用当前 Google `sub`，不使用可变的 email。首期不要求 `users` 外键，因为目前身份来自验证后的 token/session，没有持久用户表，历史回填也不能依赖用户再次登录。[C3]
- 不复制 `long_url`、`disabled`、`expires_at` 或计数。owner/创建时间只是 S3 的查询投影，不允许独立修改。
- 主键保证一个 code 在一个 space 内只有一条索引。后续同步可使用 `ON CONFLICT DO UPDATE`；**UPSERT 保证数据库内原子插入/更新，不自动解决旧事件覆盖新状态**。[P8][P9]
- `timestamptz` 内部保存 UTC 时刻，时间精度为微秒；Go/S3 创建时间可能具有纳秒精度。cursor 必须使用数据库返回的时间，不拿它作为 S3 对象代次/version 判据。[P10]

### 4.1 Cursor 分页

第一页面使用相同的 WHERE / ORDER BY，但没有 cursor 条件。后续页面候选查询：

```sql
SELECT code, owner_id, link_created_at
FROM owner_link_index
WHERE link_space = $1
  AND owner_id = $2
  AND (link_created_at, code) < ($3::timestamptz, $4::text)
ORDER BY link_created_at DESC, code DESC
LIMIT $5;
```

`$5` 为限制后的候选页大小，可取 `page_size + 1` 做 lookahead。排序加 code 唯一 tie-breaker，避免相同创建时间造成不确定分页；复合 B-tree 匹配前导等值条件和剩余排序。官方指出大 OFFSET 仍需计算被跳过的行；行构造比较支持上述元组 `<` 语义。[P11][P12][P13][P14]

列表 module 的 Interface 必须包含这些语义：

1. owner 从当前认证主体获取；cursor 校验版本、space、用户及长度，不能信任 cursor 提供的 owner。
2. PostgreSQL 只取候选 code，再以有界并发读取 S3，并保持原排序。
3. 当前 `Record.OwnerID` 必须等于调用者；过期记录不返回，禁用但未过期的记录仍保留在管理列表。[C2]
4. S3 真正 NotFound 或 owner 不匹配可跳过并安排修复；S3 超时/权限错误不能当不存在，更不能据此清理业务关系。
5. 无效候选较多时继续扫描，直到凑足一页或达到扫描预算。cursor 推进到最后已消费的候选，而不是只跟随最后返回的记录；未消费的 lookahead 不能被跳过。
6. 达到扫描预算时允许较短页面和 continuation token，避免整页脏数据导致卡住。
7. keyset 分页不是跨请求快照；同步、回填或重建改变索引时，不能承诺快照级完整性。[P15]

**收益不能夸大：** PostgreSQL 替代 LIST、支持稳定排序和分页，但每页仍需要 S3 GET。收益主要是从“读取用户全部 N 条”变为“读取当前页及有限补扫”，不是消除所有 N+1。

现有前端 `api.listMine()` 只消费一次 `{links: []}`。不能直接让服务默认只返回首页而不同时修改调用者，否则 dashboard 会静默漏链。分页切换须一起更新前端、HTTP 文档与契约测试。[C6]

## 5. 一致性、故障与同步

### 5.1 核心规则

PostgreSQL 事务只涵盖 PostgreSQL 数据；S3 不参与该事务。Amazon S3 单 key 更新是原子的，但不支持跨 key 原子更新，更不能因此推出 S3 与 SQL 共同提交。[P15][S1]

推荐 S3-first、owner 索引最终一致：

| 场景 | 建议行为 |
| --- | --- |
| S3 创建失败 | 返回失败，不创建权威 PostgreSQL 关系 |
| S3 创建成功，索引同步失败 | 短链创建仍成功；监控并修复，不做盲目 S3 回滚 |
| 新链接尚未入索引 | “我的链接”暂时不可见；这是需要明确的产品语义 |
| PostgreSQL 索引缺失/错误 | S3 仍决定解析、所有者、过期与禁用；索引可修复 |
| PostgreSQL 不可用 | 缩短和重定向继续；管理列表返回 503 或显式回退至旧 S3 pointer |
| 写标签/目录等 PostgreSQL 权威关系失败 | 关系请求失败，不能像索引写入一样吞错 |
| 涉及 PostgreSQL 的共享/角色授权失败 | 拒绝操作，不允许 fail-open |

重定向热路径不能查询 PostgreSQL。当前 `Resolve` 对过期记录会调用 `Store.Delete`，因此组合 adapter 的删除投影处理也必须非阻塞，不能在过期跳转分支同步等待数据库。[C2]

### 5.2 首期同步建议

- 主记录成功变更后，向有界 worker 提交“刷新该 key 当前索引”的任务，而不是让 HTTP handler 编排 SQL。
- worker 从 S3 重新读取当前记录，再幂等更新 owner 投影；不要把延迟到达的旧创建/删除载荷直接应用为当前状态。
- 进程内队列允许丢任务，但必须有从 S3 主记录全量/周期性对账的修复路径；不能称它为 durable queue。若业务要求严格的列表可见性时限，需先验证扫描成本或增加持久事件源。
- 回填扫描 `<prefix>/<code>.json` 主对象，排除 `owners/` 等非主记录对象；不能只迁移可能已经缺失的 owner pointers。
- 对账除了补漏，也要检查 PostgreSQL 中残留的 code。多页扫描不是整桶事务快照，需要重复对账，并对清理使用保守、条件化策略。
- 多 worker 和回填会竞争；幂等不等于排序。索引短暂落后时由 S3 hydrate 阻止跨用户泄露；持久业务关系不由这个投影 worker 清理。

仅在 PostgreSQL 事务内增加 outbox，无法捕获“S3 已成功、数据库完全不可达、未写入 outbox”的窗口。outbox 适合 PostgreSQL 权威业务数据的后续通知，不是 S3-first 双写的万能事务方案。

### 5.3 S3 事件方案：先验证存储提供商与流量

AWS S3 Event Notifications 可发送到 SQS 等目标；至少一次投递，且不保证事件按发生顺序到达。PUT/DELETE 的 `sequencer` 只能比较同一个 key，比较前需处理字符串长度差异。[S2][S3]

若采用它，必须加入去重、重试、死信与 per-key 排序状态。删除后不能直接丢掉排序 tombstone，否则迟到的旧事件可能复活索引。优先把事件当“需要刷新当前 key”的信号；重建场景仍需对账。

**urlo 特有成本：** `IncrVisit` 每次访问都会 PUT 同一个短链 JSON，因此订阅主记录 PUT 事件也会接收访问计数写入。需要合并刷新、跳过 owner 投影未变化的更新，并测量队列/worker 开销，不应直接部署一个“每次 PUT 都写 SQL”的消费者。[C1][C2]

当前支持 S3-compatible endpoint，实际部署可能不是 AWS。不能默认所有提供商都有 AWS 通知、顺序字段或相同一致性语义；这部分需在目标环境验证。[C1][C4]

## 6. 持久业务关系：第二阶段必须解决短链代次

当前主记录只含 code，没有独立不可变 `link_id`。删除后 code 可重新创建。若标签、目录或共享关系只绑定 `(link_space, code)`，旧关系可能错误附着到新短链。[C1][C2]

**在引入这些持久关联前，建议：**

- 在 S3 主记录加入每次创建新生成的不可变 `link_id`（例如 UUID），与 code 分开；修改 URL/TTL/状态不改变该 ID。
- 历史记录需要幂等、并发安全补 ID；不能靠 PostgreSQL 微秒时间、ETag 或“第一次查询时随机生成 ID”识别对象代次。
- 在 PostgreSQL 建立对应的持久 `link_refs`，关联标签等表引用 `link_id`；code 只表示当前寻址关系。
- 新建/复用 code 时读取并校验 S3 当前 ID；旧代次的删除不能清理新代次关联。
- PostgreSQL 外键只能约束 PostgreSQL 表，不能证明对应 S3 对象仍存在；跨存储生命周期仍需对账。[P8]
- **不要让业务关系外键 `ON DELETE CASCADE` 指向可重建的 `owner_link_index`。** 投影维护不能销毁用户数据。[P8]

后续最小关系布局可按需求添加：

- `users` / `user_identities`：只有需要持久 profile、设置或角色时建立；外部身份保持 issuer + subject 唯一，兼容已有 S3 Google `OwnerID`。
- `tags` + `link_tags`：多对多；用唯一约束防止重复关联，校验标签 owner/space。
- `folders` + `link_folders`：先确定一个链接可处于一个还是多个目录，再确定唯一约束。
- `user_settings`：少量配置可选 JSONB，身份和关系约束仍用显式列。

团队/计费暂不建表，避免为未确认需求预先扩张模型。

## 7. Module 与配置接入建议

建议把数据库知识和 S3/SQL 同步藏在关系 module 的 Implementation 内，保持调用方的小 Interface；不要增加覆盖整个系统的通用 DAO。

候选文件布局（尚未创建）：

```text
internal/relations/                   # owner 查询、分页、修复与错误语义
internal/relations/postgres/          # 参数化 SQL adapter，接收注入的 pool
migrations/postgres/                  # goose SQL 迁移
cmd/urlo/                            # pool、worker 的构造和生命周期
```

首期可保留 `url.Store` 的既有 adapter，以组合方式覆盖 owner 查询、观测主记录变更；分页需求再增加明确的查询 Interface。`Get`、`Exists`、`IncrVisit` 都委托 S3，不访问 SQL。组合实现必须覆盖 HTTP、gRPC 删除和 `Resolve` 的过期清理，不能仅挂在 HTTP handler 上。[C2][C4]

候选配置：

```yaml
storage:
  driver: s3
  # 保持现有 storage.s3 设置

relations:
  driver: postgres
  link_space: urlo-production
  owner_index_read_source: s3  # 初期双写/影子查询；验证后改 postgres
  postgres:
    dsn_env: URLO_POSTGRES_DSN
    max_conns: 10
    connect_timeout_seconds: 3
    query_timeout_ms: 1000
```

这些键还未加入 `ServiceConfig`，数值只是待压测的起点；未启用 relations 时保留当前行为。DSN 从部署 secret 注入，不记录密码或连接字符串。生产 TLS 按平台要求配置，不照搬本地 `sslmode=disable`。

pool 创建成功不代表连通：`pgxpool.New` 不等待建连，需有界 `Ping` 和 schema 版本检查。[P2]

可重建索引模式下，连接配置错误可 fail-fast，但 PostgreSQL 短暂故障应以管理功能降级启动，避免阻止只需要 S3 的服务启动。liveness 和核心路由 readiness 不应因为可选关系库不可用而摘掉所有重定向流量。

需要显式连接数上限、查询 deadline、连接生命周期和日志/指标。pool 默认上限与 CPU 数有关，不宜在多 Pod 部署中完全沿用默认值；总预算包含 replicas、worker 和迁移连接。[P2]

关闭时先停止接单并终止 worker、释放 rows/事务，再关闭 pool。Core 目前没有有效执行 teardown 的路径，这项不能遗漏。[C5][P2]

## 8. 迁移与上线顺序

1. **单表试点：** 增加 owner 索引 adapter、独立 goose 迁移 job 和开关；S3 pointer 读写仍保留。
2. **先同步后切读：** 部署投影 worker，扫描 S3 主记录回填；多次对账并 shadow-read，比较缺失、脏记录和 owner 不匹配，不只比较总数。
3. **分页契约：** 与前端一起修改列表读取和文档；有界 S3 hydrate，验证当前 owner，保留禁用链接可管理的行为。
4. **切换：** owner 查询改 PostgreSQL；保留回退开关与现有 pointer，观测可见性延迟、SQL/S3 错误及容量。
5. **稳定后清旧索引：** 停写并清理 S3 owner pointer；此后不能再宣称旧 pointer 是最新的回退路径。
6. **扩展业务关系：** 先完成 `link_id` 及跨存储生命周期设计，再添加真正需要的标签/目录。

迁移使用版本化 SQL，运行在独立、单执行者部署任务中，不由每个 Web Pod 自动执行。schema 迁移和大规模回填分离。应用账户用最小 DML 权限，迁移账户另行授权。goose 事务迁移能力不能被视为跨 S3 原子迁移。[P4]

首期回滚是切回读源，不是 `DROP` 数据；一旦存在 PostgreSQL 权威关系，回滚尤其不能清空用户数据。

## 9. 下一轮验证清单与未决事项

本轮仅检查源码、抓取官方文档并整理设计，没有使用 PostgreSQL 服务或连接凭据，也未运行 schema、驱动编译、集成测试或性能测试；本机未发现 `psql`、`postgres`、Docker/Podman 工具。

实施前建议验证：

- [ ] 确认目标 PostgreSQL 主版本、现有集群是否可复用、连接预算与备份/PITR。
- [ ] 确认实际 S3 提供商、通知能力、条件更新能力、bucket/prefix 与 `link_space` 映射。
- [ ] 接受“创建成功但列表短暂不可见”吗？若要求 read-your-writes，需另行设计确认/合并策略，不能仅称最终一致即可。
- [ ] 在真实 PostgreSQL 测试新库迁移、增量迁移、主键、大小写、时间精度、UPSERT 和 cursor；mock SQL 不能验证这些行为。
- [ ] 故障注入：数据库断开/超时及进程重启时，缩短和重定向仍成功；过期清理不等待 PostgreSQL。
- [ ] 脏索引、全页过期、owner 改变/不匹配、code 复用、重复/迟到事件、并发回填均不泄露或错误清理数据。
- [ ] 有代表性的用户链接数量下运行 `EXPLAIN (ANALYZE, BUFFERS)` 和端到端压测，分别量测 SQL、S3 GET、队列与修复扫描成本。
- [ ] 建立同步失败、队列丢弃、投影差异、可见性延迟、pool 等待和 S3 hydrate 指标。
- [ ] 在引入共享权限前补齐 HTTP/gRPC 一致鉴权；目前 gRPC 管理操作未执行所有权校验，不是引入数据库就能自动修复。[C7]

另外，现有 `IncrVisit` 是无 CAS 的整对象读改写，可能丢计数，并覆盖并发 URL/禁用/过期更新。引入 PostgreSQL owner 索引不解决它。后续应单独评估 S3 `If-Match` 重试或拆分计数，但不在本轮迁移计数。[C1][S4]

## 10. 一手来源

### 当前代码

- [C1] [S3 Store](../../internal/url/s3store/s3store.go)：`Create`、`ListByOwner`、`Delete`、`IncrVisit` 与 namespace。
- [C2] [Record / Store](../../internal/url/store.go) 与 [URL Service](../../internal/url/service.go)：owner、唯一创建、过期、禁用、解析与权限校验。
- [C3] [Auth User / Sessions](../../internal/auth/auth.go)、[HTTP routes](../../internal/http/routes.go)、[Click Recorder](../../internal/clicks/clicks.go)。
- [C4] [go.mod](../../go.mod)、[ServiceConfig](../../internal/config/config.go)、[配置示例](../../config.yaml)、[启动 wiring](../../cmd/urlo/main.go)。
- [C5] Butterfly 锁定模块源码：`butterfly.orx.me/core@v0.0.0-20260427155459-f3485be49e45/{internal/store/sqldb.go,store/sqldb/sqldb.go,store/gorm/gorm.go,mod/config.go,app/app.go}`；可在 `$(go env GOMODCACHE)` 下复核，不以 Core 文档示例推断 PostgreSQL 支持。
- [C6] [URL HTTP 文档](../api/urls.md)、[前端 api.listMine](../../front/src/lib/api.ts)。
- [C7] [gRPC 文档的鉴权说明](../api/grpc.md)。

### PostgreSQL 与 Go 官方项目

- [P1] pgx [v5.11.0 README](https://github.com/jackc/pgx/blob/v5.11.0/README.md) 与 [go.mod](https://github.com/jackc/pgx/blob/v5.11.0/go.mod)。
- [P2] pgxpool [包文档](https://github.com/jackc/pgx/blob/v5.11.0/pgxpool/doc.go) 与 [Config / Ping / Close 源码](https://github.com/jackc/pgx/blob/v5.11.0/pgxpool/pool.go)。
- [P3] sqlc [配置参考](https://docs.sqlc.dev/en/v1.31.1/reference/config.html#go) 与 [pgx 指南](https://github.com/sqlc-dev/sqlc/blob/main/docs/guides/using-go-and-pgx.md)。
- [P4] goose [v3.29.0 README](https://github.com/pressly/goose/blob/v3.29.0/README.md) 与 [go.mod](https://github.com/pressly/goose/blob/v3.29.0/go.mod)。
- [P5] golang-migrate [项目 README](https://github.com/golang-migrate/migrate/blob/master/README.md)。
- [P6] PostgreSQL [支持与版本周期](https://www.postgresql.org/support/versioning/)。
- [P7] PostgreSQL [Collation Support](https://www.postgresql.org/docs/18/collation.html)。
- [P8] PostgreSQL [Constraints / Foreign Keys](https://www.postgresql.org/docs/18/ddl-constraints.html)。
- [P9] PostgreSQL [INSERT / ON CONFLICT](https://www.postgresql.org/docs/18/sql-insert.html#SQL-ON-CONFLICT)。
- [P10] PostgreSQL [Date/Time Types](https://www.postgresql.org/docs/18/datatype-datetime.html)。
- [P11] PostgreSQL [Multicolumn Indexes](https://www.postgresql.org/docs/18/indexes-multicolumn.html)。
- [P12] PostgreSQL [Indexes and ORDER BY](https://www.postgresql.org/docs/18/indexes-ordering.html)。
- [P13] PostgreSQL [LIMIT and OFFSET](https://www.postgresql.org/docs/18/queries-limit.html)。
- [P14] PostgreSQL [Row Constructor Comparison](https://www.postgresql.org/docs/18/functions-comparisons.html#ROW-WISE-COMPARISON)。
- [P15] PostgreSQL [Transaction Isolation](https://www.postgresql.org/docs/18/transaction-iso.html)。

### AWS S3 官方文档（不自动适用于所有兼容存储）

- [S1] [Amazon S3 data consistency model](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Welcome.html#ConsistencyModel)。
- [S2] [Amazon S3 Event Notifications](https://docs.aws.amazon.com/AmazonS3/latest/userguide/EventNotifications.html)。
- [S3] [Event message structure / sequencer](https://docs.aws.amazon.com/AmazonS3/latest/userguide/notification-content-structure.html)。
- [S4] [Conditional writes / If-Match](https://docs.aws.amazon.com/AmazonS3/latest/userguide/conditional-writes.html)。
