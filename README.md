# 幕间 (MuJian)

现场演出记录管理应用，支持日历视图、演出管理、数据分析和 PWA 离线使用。

## 功能特性

- **日历视图** — 按月查看演出，支持海报显示和滑动切换月份
- **演出管理** — 添加、编辑、删除演出记录，支持批量操作
- **数据分析** — 统计图表展示演出趋势、分类分布、场馆排行等
- **导入导出** — JSON 备份恢复、Excel 批量导入，自动去重
- **PWA 支持** — 可添加到主屏幕，支持离线使用
- **暗色模式** — 自动跟随系统或手动切换

## 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | SvelteKit, Svelte 4 |
| 后端 | Go, Chi Router |
| 数据库 | SQLite (modernc.org/sqlite) |
| 样式 | CSS 自定义属性 (支持暗色模式) |

## 快速开始

### 环境要求

- Node.js 18+
- Go 1.21+

### 开发模式

```bash
# 安装前端依赖
cd frontend && npm install

# 启动前端 (端口 5173)
npm run dev

# 启动后端 (端口 8080，支持热重载)
cd .. && make dev-backend
```

前端访问 http://localhost:5173，API 请求自动代理到后端。

### 生产构建

```bash
make build
```

构建产物位于 `backend/dist/`，可直接运行：

```bash
cd backend && ./mujian
```

### Docker

```bash
make docker
```

## 项目结构

```
mujian/
├── frontend/                # SvelteKit 前端
│   └── src/
│       ├── lib/
│       │   ├── components/  # 组件 (Calendar, ShowCard, ShowForm 等)
│       │   ├── api.js       # API 客户端
│       │   └── stores.js    # Svelte stores (主题)
│       └── routes/          # 页面路由
│           ├── +page.svelte           # 首页 (日历)
│           ├── shows/                 # 演出列表
│           ├── analytics/             # 数据分析
│           └── settings/              # 设置
├── backend/                 # Go 后端
│   └── internal/
│       ├── db/              # 数据库层 (SQLite)
│       ├── handlers/        # HTTP 处理器
│       ├── models/          # 数据模型
│       └── config/          # 配置管理
├── dev.sh                   # 开发热重载脚本
└── Makefile                 # 构建命令
```

## API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/shows | 获取演出列表 |
| POST | /api/shows | 创建演出 |
| PUT | /api/shows/:id | 更新演出 |
| DELETE | /api/shows/:id | 删除演出 |
| GET | /api/calendar | 获取日历数据 |
| GET | /api/stats | 获取统计信息 |
| POST | /api/shows/import | Excel 导入 |
| POST | /api/backup/restore | JSON 备份恢复 |
| GET | /api/backup/download | 导出备份 |

## 演出状态

| 状态值 | 显示名称 | 颜色 |
|--------|----------|------|
| normal | 正常 | 🟢 绿色 |
| cancelled | 已取消 | 🔴 红色 |
| pending_tickets | 待开票 | 🟠 橙色 |
| no_show | 未赴约 | ⚫ 灰色 |

## 数据备份

应用支持 JSON 格式的备份和恢复：

- **导出**：设置页面点击"下载备份"
- **导入**：设置页面点击"恢复备份"，支持自动去重
- **Excel 导入**：演出列表页面支持 Excel 批量导入
