# 封面去重与复用功能设计方案（cover-management）

> **状态：已实现（2026-08-20）** — 后端（内容寻址存储 + covers 表 + 7 个接口 + 导入自动哈希化/缩略图）与前端（/covers 管理页 + 表单封面复用选择器）全部落地并验证；可选增强（自动去重、S3 适配、缩略图统一生成）均已实现。
> 实测：298 条数据发现 20 组重复（67 条记录）→ 合并释放 26.4MB；迁移后 232 个旧 UUID 封面清理释放 110.3MB；目录由 298 文件收敛为 251 个纯哈希命名文件。

> 目标：减少重复封面存储、节省空间；批量合并相同封面；清理孤儿封面；新建演出时复用已有封面。
> 适用范围：mujian（幕间）个人演出管理系统 —— Go 后端 + SQLite + 本地文件存储（`<UploadDir>/covers/`），前端 SvelteKit 5。
> 关联现状：`records.cover`（原始 UUID，来自「记录现场」导出）、`records.cover_file`（相对路径，如 `covers/xxx.jpg`）、`records.cover_thumb`（base64 缩略图）；封面文件以 `<UUID>.<ext>` 命名存放。

---

## 1. 核心决策：内容寻址存储（Content-Addressed Storage）

**封面文件名改为内容 SHA-256 哈希**，而不是 UUID：

```
上传/导入封面  →  计算 sha256(文件字节)  →  文件名 = covers/<sha256>.<ext>
                                             (ext 由图片魔数判定：jpg/png/webp)
```

为什么这是最优解：

| 维度 | 按 UUID 命名（现状） | 按内容哈希命名（本方案） |
|---|---|---|
| 重复检测 | 需额外扫描比对 | **结构上不可能重复**（内容相同 ⇒ 哈希相同 ⇒ 同名） |
| 合并 | 需要改引用 + 删文件 | 只需把引用指向规范名，写文件天然幂等 |
| 引用数统计 | 逐记录扫 | `GROUP BY cover_file` 即可 |
| 下载/导出 | 无影响 | 导出 zip 按 basename 打包，往返无损 |
| 冲突 | — | SHA-256 碰撞概率可忽略（个人数据规模） |

**字段职责划分（保持导出兼容）**：
- `records.cover`：**语义引用**，保留「记录现场」原始 UUID，不参与存储寻址；
- `records.cover_file`：**物理引用**，`covers/<sha256>.<ext>`，唯一指向磁盘/对象存储上的实际文件；
- `records.cover_thumb`：base64 缩略图，随记录存 DB，与文件存储无关，不参与去重。

现有数据无需丢弃：通过一次**迁移**（见 §6）把 `<UUID>.<ext>` 重命名/复制为 `<sha256>.<ext>`，再更新 `cover_file`，之后所有新写入都走哈希命名。

---

## 2. 数据结构设计

### 2.1 records 表（变更点）

```sql
-- 仅语义/物理引用划分，无新增必填列
ALTER TABLE records ADD COLUMN cover_sha256 TEXT;   -- 冗余缓存，便于快速分组（可空）
CREATE INDEX idx_records_cover_file ON records(cover_file);   -- 引用统计/复用查询
CREATE INDEX idx_records_cover_sha ON records(cover_sha256);  -- 合并分组查询
```

### 2.2 新增 covers 元数据表

`covers` 表用于 O(1) 查询、去重扫描增量缓存、管理页展示（大小/数量/时间），**引用数不落库、由 records 派生**（避免计数器漂移）：

```sql
CREATE TABLE IF NOT EXISTS covers (
  hash       TEXT PRIMARY KEY,        -- sha256 十六进制
  file_name  TEXT NOT NULL UNIQUE,    -- <hash>.<ext>
  ext        TEXT NOT NULL,           -- jpg | png | webp
  size       INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 2.3 文件布局

```
<UploadDir>/
  covers/            -- 唯一正本，全部 <sha256>.<ext>
  covers_trash/      -- 清理时的回收目录（软删除，可撤销）
```

### 2.4 对外 API 一览

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/covers` | 去重封面列表（复用选择器数据源）：`{covers:[{file_name, ext, size, ref_count, sample_name, thumb_url}], total}`，支持 `q`、`page`、`limit` |
| GET | `/api/covers/duplicates` | 重复分组检测（合并预览）：`{groups:[{hash, ext, size, count, records:[{id,name,cover,cover_file}]}]}` |
| POST | `/api/covers/merge` | 批量合并：`{hashes:[...]}` → `{merged_groups, updated_records, freed_bytes}` |
| GET | `/api/covers/orphans` | 孤儿文件列表：`{files:[{name,size}], total_size}` |
| POST | `/api/covers/cleanup` | 清理：`{files:[...]|"all"}` → 移入 trash → `{moved, freed_bytes}` |
| POST | `/api/covers/trash/purge` | 彻底清空回收目录 → `{deleted, freed_bytes}` |
| POST | `/api/covers/trash/restore` | （可选）从回收目录恢复：`{files:[...]}` |

---

## 3. 功能一：批量合并相同封面

### 3.1 检测（`GET /api/covers/duplicates`）

1. 遍历 `covers/`，对**未在 covers 表**中的文件计算 sha256，增量写表（旧文件迁移后即入表，后续扫描几乎零开销）；
2. SQL 分组：
   ```sql
   SELECT r.cover_sha256, COUNT(*), SUM(1)
   FROM records r
   WHERE r.cover_file != ''
   GROUP BY r.cover_sha256
   HAVING COUNT(*) > 1;      -- 或按 cover_file 规范名后必然同组
   ```
3. 返回每个重复组：哈希、文件大小、组内记录（id/名称/当前 cover_file），前端据此展示。

> 说明：哈希命名后重复组在结构上会消失，本接口主要服务**迁移期**与**历史脏数据**（同名不同内容 / 同内容不同名）。对规范数据它是空集，运行成本极低。

### 3.2 预览与确认（防误合并）

- 前端「封面管理」页展示重复组：每组一张**缩略图预览**（直接用该 cover_file 的 URL）+ 组内记录列表 + 文件大小；
- 默认全选，用户可逐组取消勾选；
- 提交前二次弹窗确认「合并 N 组，预计释放 X MB」。

### 3.3 合并执行（`POST /api/covers/merge`，一致性关键）

对每个被选中的组（记规范文件 `canonical = covers/<hash>.<ext>`）：

```
1) 确保规范文件存在（幂等）
   - canonical 已存在  → 跳过（同名内容必然一致）
   - 不存在           → 从组内任一现有文件复制/重命名生成

2) 单事务批量更新引用
   BEGIN;
   UPDATE records SET cover_file = '<canonical>', cover_sha256 = '<hash>'
   WHERE id IN (组内全部记录 id);
   UPDATE covers SET updated_at = CURRENT_TIMESTAMP WHERE hash = '<hash>';
   COMMIT;

3) 提交后清理旧文件（逐文件复核）
   for each 组内旧 cover_file != canonical:
       SELECT COUNT(*) FROM records WHERE cover_file = '<旧文件>';
       if count == 0: os.Remove(旧文件)   # 或先移入 trash
       else: 跳过并告警（理论不应发生）
```

**一致性保证**：
- 先写文件后改引用 → 任何时刻不存在「记录指向缺失文件」；
- 引用更新全部在单事务内 → 要么整组切换，要么不变；
- 删除永远发生在提交之后，且删除前逐文件复核引用数为 0；
- 文件操作与 DB 事务之间用进程内互斥锁（单用户应用，`sync.Mutex` 包住 merge/cleanup）防止并发交错。

**性能**：298 条记录量级，哈希一次毫秒级、事务毫秒级；分页处理组数即可，无瓶颈。

---

## 4. 功能二：清理不再使用的封面

### 4.1 检测（`GET /api/covers/orphans`）

```sql
-- 找出所有被引用的文件集合，做差集
SELECT file_name FROM covers c
WHERE NOT EXISTS (
  SELECT 1 FROM records r WHERE r.cover_file = c.file_name
);
```
（对未入表的散落文件同样扫描目录做差集；跳过 `covers_trash/` 与隐藏文件。）

### 4.2 安全清理（`POST /api/covers/cleanup`）

两阶段「软删除」：

```
1) 前端列出孤儿文件（缩略图 + 名称 + 大小 + 合计），用户勾选/全选 → 确认
2) 后端逐文件：
   - 复核引用数 == 0（防止两次清理请求间的竞态）  → 引用数 > 0 则跳过
   - 移动到 <UploadDir>/covers_trash/  （rename 同盘符，原子且快）
3) 返回 moved 数量与 freed_bytes
4) （可选）POST /api/covers/trash/purge 彻底删除回收目录内容；
   或保留 N 天后自动清理
```

**安全边界**：
- 复核引用为 0 是删除前最后一道闸；
- 软删除让误删可恢复（`restore` 只需 rename 回 `covers/`）；
- 清理只作用于 `covers/` 目录内文件，绝不触碰上传目录其他子目录；
- 单用户低并发场景，配合互斥锁足够。

---

## 5. 功能三：新建演出时复用封面

### 5.1 数据流

新建/编辑记录的表单不再强制上传新文件。提供「从已有演出引用封面」入口，选择后**只复制引用字段**：

```
选中记录 R  →  form.cover_file = R.cover_file
             form.cover     = R.cover（或留空/新 UUID，二者皆可）
             form.cover_thumb = R.cover_thumb（可选回填，加速列表缩略图）
```
保存走现有 `POST /api/records`，无任何新文件写入 —— 新记录与源记录共享同一份文件（引用计数自然 +1）。

### 5.2 选择器数据源（`GET /api/covers`）

按**去重后的封面**返回（而不是逐记录），避免同一封面重复出现：

```sql
SELECT c.file_name, c.ext, c.size,
       (SELECT COUNT(*) FROM records r WHERE r.cover_file = c.file_name) AS ref_count,
       (SELECT r.name FROM records r WHERE r.cover_file = c.file_name LIMIT 1) AS sample_name
FROM covers c
WHERE c.file_name LIKE ?          -- q 过滤：按名称/分类命中
ORDER BY c.updated_at DESC
LIMIT ? OFFSET ?;
```

### 5.3 前端交互流程（RecordForm 变更）

```
封面区（上传按钮旁新增）
   └ 「从已有演出引用」→ 打开模态弹窗（新组件 CoverPicker.svelte）
        ├ 顶部：搜索框（演出名/分类）+ 分类下拉 + 分页
        ├ 主体：缩略图网格（图片 + 名称 + 引用数徽标）
        ├ 点击项高亮 → 底部「使用该封面」按钮
        └ 确认 → 回填 form.cover_file / form.cover / form.cover_thumb → 关闭弹窗
```

移动端：弹窗全屏化，网格 2 列；缩略图懒加载（复用 `/uploads/...` 缓存头）。

### 5.4 边界情况

- **源记录封面被替换/删除**：由于引用共享，替换只改源记录自身字段；删除记录也不删文件（见 §4 引用复核）——新记录封面不受影响；
- **同一封面被引用多次**：选择器按去重封面展示，`ref_count` 徽标提示复用热度；
- **用户选择后源记录随后被删**：封面文件因新记录仍引用而保留，安全。

---

## 6. 迁移步骤（存量数据 → 哈希命名）

一次性后台任务（启动时检测 `covers` 表为空且有旧文件时自动执行，或提供 `POST /api/covers/migrate`）：

```
1) 遍历 covers/ 下 <UUID>.<ext> 文件，逐文件计算 sha256；
2) canonical = covers/<sha256>.<ext>：
   - canonical 不存在 → rename 旧文件为 canonical
   - canonical 存在   → 删除旧文件（内容相同）
3) UPDATE records SET cover_file = '<canonical>', cover_sha256 = '<hash>'
   WHERE cover_file = '<旧名>' （按旧名分组批量，一次一个事务）
4) 将 canonical 写入 covers 表
5) 完成后重新执行 §4 清理，兜底删除遗漏孤儿
```

迁移中断安全：每文件处理独立，可断点续跑；rename 原子；引用更新事务化。

---

## 7. 数据一致性与性能设计（汇总）

| 关注点 | 设计 |
|---|---|
| 引用完整性 | 先写文件 → 事务改引用 → 提交后删旧文件；删除前逐文件复核引用数 |
| 原子性 | 引用更新全部单事务；文件 rename 原子 |
| 并发 | 单用户应用，merge/cleanup/migrate 用进程内互斥锁串行化 |
| 崩溃恢复 | 软删除（trash）+ 可重跑的迁移；covers 表增量缓存哈希 |
| 查询性能 | `idx_records_cover_file`、`idx_records_cover_sha` 索引；引用数由 GROUP BY 派生不落库 |
| 哈希成本 | 仅新文件/迁移期计算；之后 covers 表缓存，重复扫描 O(0) |
| 存储上限 | 一次上传大小限制沿用 8MB；zip 导入 512MB 不变 |

---

## 8. 前端页面变更清单

| 位置 | 变更 |
|---|---|
| 新增路由 `/covers`（导航「封面」） | 两个区块：①重复封面检测与合并（组列表 + 勾选 + 预览 + 合并按钮）②未引用封面清理（列表 + 大小合计 + 清理确认 + 回收站管理） |
| `RecordForm.svelte` | 封面区新增「从已有演出引用」按钮 + 集成 `CoverPicker.svelte` |
| 新增 `CoverPicker.svelte` | 模态弹窗：搜索/筛选/分页/缩略图网格/引用数徽标 |
| `api.js` | 新增 covers 系列接口封装 |
| 空态/加载态 | 检测中骨架屏；无重复「🎉 暂无重复封面」；清理后结果横幅 |

---

## 9. 可选增强（不在本次范围）

- 自动去重：上传时若哈希已存在，直接返回既有 cover_file（零逻辑成本，可默认开启）；
- S3 存储适配：哈希命名天然适配对象存储，`copy_object` 替代 rename、`delete_object` 替代 unlink；
- 引用计数落库（covers.ref_count）+ 触发器维护，用于大并发多用户场景（个人应用不需要）；
- 缩略图统一生成（cover_thumb 去重），进一步节省 DB 空间。

---

## 10. 验收标准

1. 迁移后 `covers/` 中每个文件仅一份，`records.cover_file` 全部为 `covers/<sha256>.<ext>`；
2. 相同内容封面 → `duplicates` 检出为组，合并后全部记录可正常显示、旧文件被回收；
3. 手动删除一条记录后，其封面仍被其他记录引用时**不会**被清理；零引用文件经确认后可清理并释放空间，回收站可恢复；
4. 新建记录引用已有封面 → 保存后详情/列表正常显示，目录文件数不增加；
5. 导出 zip → 重新导入 → 封面全部关联（往返无损）。
