# SQLite 到 PostgreSQL 业务库迁移设计

## 背景

项目当前支持 SQLite、MySQL 和 PostgreSQL 作为业务数据库，并在 [`model/main.go`](/Volumes/data/code/ai/new-api/model/main.go) 中维护统一的 schema migration 逻辑。现阶段缺少一个官方、可重复执行的离线迁移方法，用于把现有 SQLite 业务库的数据完整迁移到空的 PostgreSQL 业务库。

这次设计只覆盖业务库迁移，不包含日志库，也不修改现有在线服务行为。

## 目标

- 提供一个内置于现有二进制的离线迁移命令。
- 将 SQLite 业务库中的全部业务表数据迁移到 PostgreSQL。
- 复用现有 GORM 模型和 schema migration 逻辑，避免维护额外的表结构定义。
- 保留原始主键值和关联关系，迁移后业务数据语义不变。
- 迁移完成后修正 PostgreSQL 序列，保证后续线上写入正常增长。

## 非目标

- 不迁移 `LOG_SQL_DSN` 对应的日志库。
- 不支持增量同步、双写、在线热迁移或断点续传。
- 不支持将数据导入非空的 PostgreSQL 业务库。
- 不提供通用的数据库互转框架，本次仅支持 SQLite 到 PostgreSQL。

## 用户体验

在现有启动参数中新增离线迁移模式。命令命中该模式时，不启动 HTTP 服务，只执行迁移并退出。

建议参数：

- `--sqlite-to-postgres`
- `--sqlite-path <path>`
- `--postgres-dsn <dsn>`
- `--batch-size <n>`，可选，默认 `1000`

示例：

```bash
./new-api \
  --sqlite-to-postgres \
  --sqlite-path ./one-api.db \
  --postgres-dsn 'postgres://user:pass@127.0.0.1:5432/newapi?sslmode=disable' \
  --batch-size 1000
```

参数校验规则：

- 未指定 `--sqlite-to-postgres` 时，程序保持当前启动行为。
- 指定 `--sqlite-to-postgres` 后，`--sqlite-path` 和 `--postgres-dsn` 必填。
- `--batch-size` 必须大于 `0`。
- `--postgres-dsn` 必须是 PostgreSQL DSN。

## 架构落点

- [`common/init.go`](/Volumes/data/code/ai/new-api/common/init.go)：新增离线迁移参数与参数校验。
- [`main.go`](/Volumes/data/code/ai/new-api/main.go)：在服务初始化早期识别迁移模式，执行迁移并退出。
- 新增 [`model/sqlite_to_postgres.go`](/Volumes/data/code/ai/new-api/model/sqlite_to_postgres.go)：实现迁移主流程与辅助函数。

迁移逻辑不复用全局 `model.DB`，而是分别创建源 SQLite 连接和目标 PostgreSQL 连接，避免污染正常启动路径中的数据库类型标志位与全局状态。

## 业务表范围

迁移表范围与 [`model/main.go`](/Volumes/data/code/ai/new-api/model/main.go) 中 `migrateDB()` 的业务模型集合保持一致，但显式排除 `Log`。

建议迁移模型集合：

- `Channel`
- `Token`
- `User`
- `PasskeyCredential`
- `Option`
- `Redemption`
- `Ability`
- `Midjourney`
- `TopUp`
- `QuotaData`
- `Task`
- `Model`
- `Vendor`
- `ModelChannelPolicy`
- `ModelChannelState`
- `PrefillGroup`
- `Setup`
- `TwoFA`
- `TwoFABackupCode`
- `Checkin`
- `SubscriptionOrder`
- `UserSubscription`
- `SubscriptionPreConsumeRecord`
- `CustomOAuthProvider`
- `UserOAuthBinding`
- `SubscriptionPlan`

实现时应将该列表收敛为一份共享清单，供目标库 schema migration、非空检查、数据迁移三处复用，避免后续新增模型时出现漏迁移。

## 迁移流程

### 1. 建立源和目标连接

- 源库使用 SQLite 驱动直连指定文件。
- 目标库使用 PostgreSQL 驱动直连指定 DSN。
- 两端都使用独立 `gorm.DB` 实例，不依赖全局 `DB` 和 `LOG_DB`。

### 2. 初始化目标 schema

- 在目标 PostgreSQL 上执行业务表 schema migration。
- 逻辑应与当前 `migrateDB()` 的业务表建表结果一致，但排除 `Log`。
- `SubscriptionPlan` 直接按 PostgreSQL 路径使用 `AutoMigrate`，不走 SQLite 特殊建表逻辑。

### 3. 检查目标库是否为空

- 对业务迁移表清单逐表做 `COUNT(*)` 检查。
- 任一目标表存在数据时，直接失败退出。
- 失败信息必须包含表名，明确提示目标库需要为空。

该设计明确了幂等边界：迁移命令只对空目标库生效。若中途失败，修复问题后清空目标业务表，再完整重跑。

### 4. 逐表搬运数据

- 使用固定顺序逐表迁移，顺序与业务迁移表清单保持一致。
- 源库按主键升序分页读取。
- 目标库按批次写入，每批使用 `CreateInBatches`。
- 批量大小由 `--batch-size` 控制，默认 `1000`。
- 每张表单独开启事务；该表失败时只回滚该表写入。

设计上不做跨全量迁移的大事务，避免长事务在 PostgreSQL 上占用过多资源，也避免失败时排查成本过高。

### 5. 修正 PostgreSQL 序列

- 对每张含自增主键的表，在迁移完成后执行序列重置。
- 序列值应设置为当前表 `MAX(id)` 对应的下一个值。
- 如果表为空，序列回到初始状态。

## 数据一致性约束

- 迁移必须保留原始 `id` 值。
- 迁移过程中不重写业务字段，不做数据清洗。
- 关联关系依赖原始主键保持一致，因此所有引用型表必须保留源数据中的外键值。
- 对 JSON/TEXT 类字段，不引入数据库特有类型转换，保持当前模型定义对应的通用存储方式。

## 错误处理与可观测性

- 启动阶段参数错误应直接打印清晰错误并退出。
- 连接失败应区分源库失败和目标库失败。
- schema migration 失败应直接退出，不进入数据复制阶段。
- 每张表迁移时输出开始、完成、记录数、耗时。
- 失败日志必须包含表名、批次信息和原始错误。

建议输出示例：

```text
start migrating table users
table users migrated: rows=1234 elapsed=1.2s
```

## 失败恢复策略

- 不支持断点续传。
- 若某张表迁移失败，命令立即退出。
- 恢复方式是：修复原因，清空目标 PostgreSQL 业务表，再完整重跑。
- 文档中需要明确这是一次性离线迁移工具，禁止在生产流量写入期间执行。

## 测试策略

### 单元测试

- 参数校验测试：缺少必填参数、非法 batch size、非法 PostgreSQL DSN。
- 业务迁移表清单测试：确保排除 `Log`，并覆盖所有业务模型。
- 非空目标库检查测试：任一表存在数据时返回明确错误。
- 序列修正 SQL 辅助逻辑测试。

### 集成测试

- 构造临时 SQLite 数据库，写入多张典型业务表的样本数据。
- 连接测试用 PostgreSQL，执行完整迁移。
- 校验目标库表记录数、关键字段、主键保留和关联关系。
- 校验迁移后新插入一条记录时自增序列正常工作。

如果当前 CI 环境没有 PostgreSQL，则至少保证迁移核心逻辑拆分为可测试的纯函数或小范围函数，并保留可在本地或后续 CI 环境执行的集成测试入口。

## 实现注意事项

- 业务代码中的 JSON 编解码仍需遵守项目规则，使用 [`common/json.go`](/Volumes/data/code/ai/new-api/common/json.go) 提供的方法；迁移主流程本身不应新增直接 `encoding/json` 的业务使用。
- 不修改项目受保护标识，包括 `new-api` 与 `QuantumNous` 相关信息。
- 迁移逻辑只服务于 SQLite 到 PostgreSQL，不应影响现有 SQLite、MySQL、PostgreSQL 正常运行路径。
- 实现时优先使用 GORM 抽象；只有在 PostgreSQL 序列修正等必要场景下才使用有限的原生 SQL。

## 验收标准

- 指定 SQLite 文件和空 PostgreSQL 库时，命令可成功完成迁移并退出。
- 迁移后业务表记录数与源 SQLite 一致。
- 关键关联数据可正常查询，且主键值与源库一致。
- 迁移后应用切换到该 PostgreSQL 库启动成功。
- 目标库非空时命令拒绝执行。
- 日志库数据不会被迁移。
