// Package mcp exposes MuJian's data layer as an MCP (Model Context Protocol)
// server so AI agents can search, bulk-edit and analyse performance records —
// e.g. unify troupe names across an actor's shows, merge near-duplicate venue
// spellings, or curate a drama's zhezi list.
//
// The server is served over the Streamable HTTP transport by the main HTTP
// process (typically at /mcp behind a reverse proxy).
package mcp

import (
	"encoding/json"
	"fmt"
	"mujian/internal/backup"
	"mujian/internal/db"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wires the MuJian database into an MCP server over HTTP.
type Server struct {
	server *mcp.Server
	db     *db.DB
	backup *backup.Manager
}

// New creates an MCP server bound to the given database and registers all tools.
func New(database *db.DB, backupMgr *backup.Manager) *Server {
	s := &Server{
		server: mcp.NewServer(&mcp.Implementation{
			Name:    "mujian-mcp",
			Version: "1.1.0",
		}, nil),
		db:     database,
		backup: backupMgr,
	}
	s.registerTools()
	return s
}

// HTTPHandler returns an http.Handler serving the MCP server over the
// Streamable HTTP transport, for the main router to mount (e.g. at "/mcp").
//
// Stateless + JSON responses keep the endpoint session-free: no
// Mcp-Session-Id state survives restarts and plain application/json avoids
// SSE streaming pitfalls through compressing proxies. Localhost/DNS-rebinding
// protection is disabled because requests arrive via a reverse proxy carrying
// a public Host header; exposure is delegated to the proxy layer.
func (s *Server) HTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.server
	}, &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		JSONResponse:               true,
		DisableLocalhostProtection: true,
	})
}

// Tool constructors. The annotations are hints (per spec clients must not
// rely on them), but they let MCP clients badge read-only vs mutating vs
// destructive tools in permission UIs.

// roTool is a read-only tool: it never mutates data.
func roTool(name, description string, schema ...*jsonschema.Schema) *mcp.Tool {
	t := &mcp.Tool{Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}
	if len(schema) > 0 {
		t.InputSchema = schema[0]
	}
	return t
}

// mutTool is a mutating tool. Mutations still default to dry_run previews;
// destructive marks the ones that delete or overwrite irrecoverably. The
// DestructiveHint is always set explicitly (spec default is true, which would
// misbadge additive tools like reorder/update).
func mutTool(name, description string, destructive bool, schema ...*jsonschema.Schema) *mcp.Tool {
	t := &mcp.Tool{Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive}}
	if len(schema) > 0 {
		t.InputSchema = schema[0]
	}
	return t
}

func (s *Server) registerTools() {
	// ---- 查询 / 分析 ----
	mcp.AddTool(s.server, roTool("search_records",
		"搜索演出记录。可按关键词（匹配演出名/城市/场馆/剧团/备注/演员名等）、演员、剧目、折子、城市、分类、日期范围、渠道、剧团、评分/票价区间、演出状态筛选；missing 参数可查任一字段为空的记录（数据卫生，如 missing=\"category,coordinate\"）；compact=true 时每条只返回核心字段；limit/offset 分页。",
		toolSchema(nil, map[string]*jsonschema.Schema{
			"query":          strProp(),
			"artist_name":    strProp(),
			"artist_id":      strProp(),
			"drama_name":     strProp(),
			"drama_id":       strProp(),
			"zhezi_id":       strProp(),
			"city":           strProp(),
			"category":       strProp(),
			"year":           intProp(),
			"month":          intProp(),
			"start":          strProp(),
			"end":            strProp(),
			"limit":          intProp(),
			"offset":         intProp(),
			"channel":        strProp(),
			"company":        strProp(),
			"rating_min":     intProp(),
			"price_min":      numProp(),
			"price_max":      numProp(),
			"active_status":  intProp(),
			"statuses":       arrayProp("integer"),
			"exact":          boolPropNoDefault(),
			"missing":        strProp(),
			"compact":        boolPropNoDefault(),
		})), s.handleSearchRecords)

	mcp.AddTool(s.server, roTool("get_record",
		"按 ID 获取单条演出记录的完整详情（含关联的剧目/折子/演员）。"), s.handleGetRecord)

	mcp.AddTool(s.server, roTool("list_artists",
		"列出所有演员档案，含别名与演出次数。可用 query 按姓名/别名子串过滤，避免全量返回。",
		toolSchema(nil, map[string]*jsonschema.Schema{"query": strProp()})), s.handleListArtists)

	mcp.AddTool(s.server, roTool("get_artist_detail",
		"获取演员详情及其全部关联演出记录。支持按 ID 或姓名/别名查找。"), s.handleGetArtistDetail)

	mcp.AddTool(s.server, roTool("list_dramas",
		"列出所有剧目档案，含剧种、折子数量与演出次数。可用 query 按名称/别名子串过滤。",
		toolSchema(nil, map[string]*jsonschema.Schema{"query": strProp()})), s.handleListDramas)

	mcp.AddTool(s.server, roTool("get_drama_detail",
		"获取剧目详情，包含折子列表与关联演出。支持按 ID 或名称查找。"), s.handleGetDramaDetail)

	mcp.AddTool(s.server, roTool("list_venues",
		"列出场馆（按地址分组统计），含每个场馆的演出次数、城市与坐标状态。可用 query 子串过滤，用于发现同一场馆的不同写法（如「xx剧院」与「xx剧院（某某店）」）。"), s.handleListVenues)

	mcp.AddTool(s.server, roTool("value_counts",
		"统计某个标量字段的所有取值及出现次数（支持 company/city/channel/category_name）。用于发现相似但不完全一致的写法并规划合并。"), s.handleValueCounts)

	mcp.AddTool(s.server, roTool("get_stats",
		"获取整体统计概览（总场次、总消费、覆盖城市等）。"), s.handleGetStats)

	mcp.AddTool(s.server, roTool("get_analytics",
		"获取深度分析数据（与网页分析页一致）：观演频率与间隔、重看统计、剧种多样性指数、票价分布、星期分布、新剧发现曲线等。"), s.handleGetAnalytics)

	mcp.AddTool(s.server, roTool("get_dashboard",
		"获取看板统计（与网页首页一致）：总场次/总消费/平均评分、近 12 个月按月与按剧种/按城市分布、成本趋势、最高评分与最近记录。"), s.handleGetDashboard)

	mcp.AddTool(s.server, roTool("search_records_by_location",
		"按坐标中心点和半径（米）搜索附近的演出记录。返回按距离排序的列表，含距离信息。建议半径不超过 10000（10公里）。",
		toolSchema([]string{"latitude", "longitude", "radius"}, map[string]*jsonschema.Schema{
			"latitude":   numProp(),
			"longitude":  numProp(),
			"radius":     numProp(),
			"limit":      intProp(),
			"category":   strProp(),
			"city":       strProp(),
			"start_date": strProp(),
			"end_date":   strProp(),
		})), s.handleSearchByLocation)

	mcp.AddTool(s.server, roTool("list_record_photos",
		"列出某条演出记录附加的照片/票根文件名与排序（不含图片内容）。",
		toolSchema([]string{"record_id"}, map[string]*jsonschema.Schema{
			"record_id": strProp(),
		})), s.handleListRecordPhotos)

	// ---- 批量修改 ----
	mcp.AddTool(s.server, mutTool("batch_update_company_by_artist",
		"把某位演员参与的所有演出的剧团(company)统一改成指定名称。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览将受影响的记录而不修改。", false), s.handleBatchUpdateCompanyByArtist)

	mcp.AddTool(s.server, mutTool("batch_merge_venues",
		"合并场馆：把 source_address 的所有演出记录的地址改为 target_address，并可选地把 target 地址已有的坐标同步给这些记录。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。", false), s.handleBatchMergeVenues)

	mcp.AddTool(s.server, mutTool("batch_update_records",
		"通用批量更新演出记录（按 ID 列表）。标量字段直接赋值（name/分类/评分/状态/城市/场馆/渠道/剧团/同行/备注/座位/date_text 演出时间/coordinate 坐标/票价等金额字段）；数组字段（drama_ids/zhezi_ids/artist_names/artist_ids/play/guest/category_names 多剧种）支持 set/append/remove 三种操作；artist_ids 直接按档案 ID 改演员关联。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览变更而不修改。", false), s.handleBatchUpdateRecords)

	// ---- 演出记录 CRUD ----
	mcp.AddTool(s.server, mutTool("create_record",
		"创建一条新的演出记录。name 必填，其余字段可选。dry_run 默认为 true（仅预览，不真正创建；显式传 dry_run=false 才执行）。", false,
		toolSchema([]string{"name"}, map[string]*jsonschema.Schema{
			"name":                strProp(),
			"channel":             strProp(),
			"city":                strProp(),
			"address":             strProp(),
			"cover_file":          strProp(),
			"cover_thumb":         strProp(),
			"category_name":       strProp(),
			"category_names":      arrayProp("string"),
			"artist_ids":          arrayProp("string"),
			"artist_names":        arrayProp("string"),
			"guest":               arrayProp("string"),
			"play":                arrayProp("string"),
			"drama_ids":           arrayProp("string"),
			"zhezi_ids":           arrayProp("string"),
			"date_text":           strProp(),
			"rating":              intProp(),
			"duration":            intProp(),
			"seat":                strProp(),
			"friends":             strProp(),
			"company":             strProp(),
			"remark":              strProp(),
			"active_status":       intProp(),
			"price":               numProp(),
			"price_currency":      strProp(),
			"pay_price":           numProp(),
			"pay_price_currency":  strProp(),
			"other_cost":          numProp(),
			"other_cost_currency": strProp(),
			"dry_run":             boolProp(),
		})), s.handleCreateRecord)

	mcp.AddTool(s.server, mutTool("update_record",
		"更新单条演出记录的任意字段（nil 保持不变）。支持标量字段直接赋值和数组字段 set/append/remove。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。", false), s.handleUpdateRecord)

	mcp.AddTool(s.server, mutTool("delete_record",
		"删除单条演出记录（进回收站，可恢复）。dry_run 默认为 true（仅预览，不真正删除；显式传 dry_run=false 才执行）。", true), s.handleDeleteRecord)

	mcp.AddTool(s.server, mutTool("batch_delete_records",
		"批量删除多条演出记录（按 ID 列表，进回收站，可恢复）。dry_run 默认为 true（仅预览，不真正删除；显式传 dry_run=false 才执行）。", true), s.handleBatchDeleteRecords)

	// ---- 回收站 ----
	mcp.AddTool(s.server, roTool("list_deleted_records",
		"列出已删除的演出记录（回收站），支持分页。"), s.handleListDeletedRecords)

	mcp.AddTool(s.server, mutTool("restore_record",
		"恢复已删除的演出记录到正常状态。dry_run 默认为 true；预览不恢复。", false), s.handleRestoreRecord)

	mcp.AddTool(s.server, mutTool("purge_record",
		"永久删除单条演出记录（不可恢复）。dry_run 默认为 true；预览不删除。", true), s.handlePurgeRecord)

	mcp.AddTool(s.server, mutTool("purge_deleted_records",
		"清空回收站（永久删除所有已删除记录，不可恢复）。dry_run 默认为 true；预览不删除。", true), s.handlePurgeDeletedRecords)

	// ---- 剧目管理 ----
	mcp.AddTool(s.server, mutTool("create_drama",
		"创建新剧目档案。name 必填，剧种默认由关联演出自动聚合。dry_run 默认为 true（仅预览，不真正创建；显式传 dry_run=false 才执行）。", false,
		toolSchema([]string{"name"}, map[string]*jsonschema.Schema{
			"name":           strProp(),
			"aliases":        arrayProp("string"),
			"category_name":  strProp(),
			"category_names": arrayProp("string"),
			"remark":         strProp(),
			"dry_run":        boolProp(),
		})), s.handleCreateDrama)

	mcp.AddTool(s.server, mutTool("update_drama",
		"更新剧目档案的名称/别名/备注/剧种。剧种默认由关联演出自动聚合；category_names 提供非空列表时手动覆盖（用于修正拼盘演出导致的聚合偏差），空数组则清除覆盖回到自动。未提供的字段保持不变。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。", false), s.handleUpdateDrama)

	mcp.AddTool(s.server, mutTool("delete_drama",
		"删除剧目及其所有折子（演出记录上的关联一并解除）。dry_run 默认为 true（仅预览，不真正删除；显式传 dry_run=false 才执行）。", true), s.handleDeleteDrama)

	// ---- 剧目/演员合并 ----
	mcp.AddTool(s.server, mutTool("merge_artists",
		"合并重复的演员档案：把 source 的所有演出关联改挂到 target，source 的姓名与别名并入 target 的别名（bio/备注/封面仅在 target 为空时补入），然后删除 source 档案。双方支持 id 或姓名定位。dry_run 默认为 true；预览不合并。", true), s.handleMergeArtists)

	mcp.AddTool(s.server, mutTool("merge_dramas",
		"合并重复的剧目档案：把 source 的演出关联与折子并入 target（与 target 同名的折子去重），姓名与别名并入 target 的别名，然后删除 source 剧目。双方支持 id 或名称定位。dry_run 默认为 true；预览不合并。", true), s.handleMergeDramas)

	// ---- 折子管理 ----
	mcp.AddTool(s.server, mutTool("batch_create_zhezis",
		"为剧目批量创建折子（自动跳过该剧目下已存在的同名折子）。配合网络搜索到的「常演折子」清单一次性写入。返回新建与跳过的清单。dry_run 默认为 true（仅预览，不真正创建；显式传 dry_run=false 才执行）。", false,
		toolSchema([]string{"names"}, map[string]*jsonschema.Schema{
			"drama_id":   strProp(),
			"drama_name": strProp(),
			"names":      arrayProp("string"),
			"remark":     strProp(),
			"dry_run":    boolProp(),
		})), s.handleBatchCreateZhezis)

	mcp.AddTool(s.server, mutTool("update_zhezi",
		"更新折子的名称/别名/备注。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。", false), s.handleUpdateZhezi)

	mcp.AddTool(s.server, mutTool("delete_zhezi",
		"删除折子（同时解除与所有演出记录的关联）。dry_run 默认为 true（仅预览，不真正删除；显式传 dry_run=false 才执行）。", true), s.handleDeleteZhezi)

	// ---- 演员管理 ----
	mcp.AddTool(s.server, mutTool("create_artist",
		"创建新演员档案。name 必填，可附带别名和简介。dry_run 默认为 true（仅预览，不真正创建；显式传 dry_run=false 才执行）。", false,
		toolSchema([]string{"name"}, map[string]*jsonschema.Schema{
			"name":    strProp(),
			"aliases": arrayProp("string"),
			"remark":  strProp(),
			"bio":     strProp(),
			"dry_run": boolProp(),
		})), s.handleCreateArtist)

	mcp.AddTool(s.server, mutTool("update_artist",
		"更新演员的名称/别名/备注/简介。未提供的字段保持不变。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。", false), s.handleUpdateArtist)

	mcp.AddTool(s.server, mutTool("delete_artist",
		"删除演员档案（同时解除与演出记录的关联）。dry_run 默认为 true（仅预览，不真正删除；显式传 dry_run=false 才执行）。", true), s.handleDeleteArtist)

	// ---- 分类管理 ----
	mcp.AddTool(s.server, roTool("list_categories",
		"列出所有分类（剧种），含演出计数和排序。"), s.handleListCategories)

	mcp.AddTool(s.server, mutTool("create_category",
		"创建新分类。name 必填。dry_run 默认为 true（仅预览，不真正创建；显式传 dry_run=false 才执行）。", false), s.handleCreateCategory)

	mcp.AddTool(s.server, mutTool("update_category",
		"更新分类名称。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。", false), s.handleUpdateCategory)

	mcp.AddTool(s.server, mutTool("delete_category",
		"删除分类。dry_run 默认为 true（仅预览，不真正删除；显式传 dry_run=false 才执行）。", true), s.handleDeleteCategory)

	// ---- 排序 ----
	mcp.AddTool(s.server, mutTool("reorder_categories",
		"按指定顺序重新排列分类。ids 为完整排序后的 ID 列表。dry_run 默认为 true；预览不修改。", false), s.handleReorderCategories)

	mcp.AddTool(s.server, mutTool("reorder_dramas",
		"按指定顺序重新排列剧目。ids 为完整排序后的 ID 列表。dry_run 默认为 true；预览不修改。", false), s.handleReorderDramas)

	mcp.AddTool(s.server, mutTool("reorder_zhezis",
		"按指定顺序重新排列剧目下的折子。需提供 drama_id 和 ids。dry_run 默认为 true；预览不修改。", false), s.handleReorderZhezis)

	mcp.AddTool(s.server, mutTool("reorder_artists",
		"按指定顺序重新排列演员。ids 为完整排序后的 ID 列表。dry_run 默认为 true；预览不修改。", false), s.handleReorderArtists)

	// ---- 封面管理 ----
	mcp.AddTool(s.server, roTool("list_covers",
		"列出封面（去重），支持按文件名查询，含引用计数。"), s.handleListCovers)

	mcp.AddTool(s.server, roTool("cover_duplicates",
		"查找内容哈希相同的重复封面分组。"), s.handleCoverDuplicates)

	mcp.AddTool(s.server, mutTool("merge_covers",
		"合并重复封面：将 sources 的引用全部指向 target，然后删除 sources。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。", true), s.handleMergeCovers)

	mcp.AddTool(s.server, roTool("cover_orphans",
		"查找没有被任何演出记录引用的孤立封面文件。"), s.handleCoverOrphans)

	mcp.AddTool(s.server, mutTool("cleanup_covers",
		"清理所有孤立封面（无引用的文件）。dry_run 默认为 true（仅预览，不真正删除；显式传 dry_run=false 才执行）。", true), s.handleCleanupCovers)

	// ---- 导入导出 ----
	mcp.AddTool(s.server, roTool("export_data",
		"导出全部数据。默认返回统计概览（计数、分类列表），不带记录正文；to_file=true 时把完整 JSON 写入备份目录（export-*.json，返回路径，可用 import_data 的 file_path 读回）；include_records=true 时才在响应中内联全部记录（数据量大时慎用，会占用大量上下文）。"), s.handleExportData)

	mcp.AddTool(s.server, mutTool("import_data",
		"从 JSON 导入演出记录，按记录 upsert：同 ID 覆盖更新，不删除未包含的现有数据。数据可经 json_data（内联字符串）或 file_path（服务器本地文件，如 export-*.json / 备份 json）提供。dry_run 默认为 true；预览不导入。", false,
		toolSchema(nil, map[string]*jsonschema.Schema{
			"json_data": strProp(),
			"file_path": strProp(),
			"dry_run":   boolProp(),
		})), s.handleImportData)

	// ---- 备份管理 ----
	mcp.AddTool(s.server, mutTool("run_backup",
		"手动触发一次备份（快照到备份目录，非破坏性操作，直接执行）。", false), s.handleRunBackup)

	mcp.AddTool(s.server, roTool("list_backups",
		"列出所有备份文件（按时间倒序）。"), s.handleListBackups)

	mcp.AddTool(s.server, mutTool("delete_backup",
		"删除指定备份文件。dry_run 默认为 true；预览不删除。", true), s.handleDeleteBackup)

	mcp.AddTool(s.server, mutTool("restore_from_backup",
		"从指定备份文件恢复数据（按记录 upsert 覆盖）。支持 .json 格式。dry_run 默认为 true；预览不恢复。", true), s.handleRestoreFromBackup)

	// ---- 地图点位 ----
	mcp.AddTool(s.server, roTool("list_map_points",
		"获取所有有坐标的演出记录（用于地图展示），支持按城市/分类过滤。"), s.handleListMapPoints)
}

// jsonResult renders v as compact JSON text content for the model — pretty
// printing would inflate token cost with no benefit to the consumer.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil, nil
}

// errResult reports a recoverable tool error back to the model (IsError=true)
// instead of failing the protocol call, letting it self-correct.
func errResult(format string, args ...any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
	}, nil, nil
}

// arrayProp returns a clean array property schema (type "array", not
// ["null","array"]) so MCP clients handle it correctly.
func arrayProp(itemType string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "array",
		Items: &jsonschema.Schema{
			Type: itemType,
		},
	}
}

// strProp returns a string property schema.
func strProp() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string"}
}

// numProp returns a number property schema.
func numProp() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "number"}
}

// intProp returns an integer property schema.
func intProp() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "integer"}
}

// boolProp returns a boolean property schema. All mutating MCP tools default
// dry_run to true, so the schema advertises that default to callers.
func boolProp() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "boolean", Default: json.RawMessage("true")}
}

// boolPropNoDefault is a boolean property for non-mutation flags (compact,
// exact): no dry_run-style default applies.
func boolPropNoDefault() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "boolean"}
}

// dryRun reports whether an operation should only preview its changes.
// dry_run defaults to true: callers must explicitly pass dry_run:false to
// actually apply a mutation. A nil *bool means "not specified" → preview only.
func dryRun(p *bool) bool {
	return p == nil || *p
}

// objProp returns an object property schema with the given properties.
func objProp(props map[string]*jsonschema.Schema) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:       "object",
		Properties: props,
	}
}

// toolSchema creates a JSON Schema for a tool with the given properties.
func toolSchema(required []string, props map[string]*jsonschema.Schema) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:       "object",
		Required:   required,
		Properties: props,
	}
}

// noInput is the input type for tools that take no parameters.
type noInput struct{}
