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
	"mujian/internal/db"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wires the MuJian database into an MCP server over HTTP.
type Server struct {
	server *mcp.Server
	db     *db.DB
}

// New creates an MCP server bound to the given database and registers all tools.
func New(database *db.DB) *Server {
	s := &Server{
		server: mcp.NewServer(&mcp.Implementation{
			Name:    "mujian-mcp",
			Version: "1.0.0",
		}, nil),
		db: database,
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

func (s *Server) registerTools() {
	// ---- 查询 / 分析 ----
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "search_records",
		Description: "搜索演出记录。可按关键词（匹配演出名/城市/场馆/剧团/备注/演员名等）、演员、剧目、折子、城市、分类、日期范围筛选，返回记录详情列表。",
	}, s.handleSearchRecords)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_record",
		Description: "按 ID 获取单条演出记录的完整详情（含关联的剧目/折子/演员）。",
	}, s.handleGetRecord)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_artists",
		Description: "列出所有演员档案，含别名与演出次数。",
	}, s.handleListArtists)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_artist_detail",
		Description: "获取演员详情及其全部关联演出记录。支持按 ID 或姓名/别名查找。",
	}, s.handleGetArtistDetail)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_dramas",
		Description: "列出所有剧目档案，含剧种、折子数量与演出次数。",
	}, s.handleListDramas)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_drama_detail",
		Description: "获取剧目详情，包含折子列表与关联演出。支持按 ID 或名称查找。",
	}, s.handleGetDramaDetail)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_venues",
		Description: "列出场馆（按地址分组统计），含每个场馆的演出次数、城市与坐标状态。可用 query 子串过滤，用于发现同一场馆的不同写法（如「xx剧院」与「xx剧院（某某店）」）。",
	}, s.handleListVenues)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "value_counts",
		Description: "统计某个标量字段的所有取值及出现次数（支持 company/city/channel/category_name）。用于发现相似但不完全一致的写法并规划合并。",
	}, s.handleValueCounts)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_stats",
		Description: "获取整体统计概览（总场次、总消费、覆盖城市等）。",
	}, s.handleGetStats)

	// ---- 批量修改 ----
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "batch_update_company_by_artist",
		Description: "把某位演员参与的所有演出的剧团(company)统一改成指定名称。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览将受影响的记录而不修改。",
	}, s.handleBatchUpdateCompanyByArtist)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "batch_merge_venues",
		Description: "合并场馆：把 source_address 的所有演出记录的地址改为 target_address，并可选地把 target 地址已有的坐标同步给这些记录。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。",
	}, s.handleBatchMergeVenues)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "batch_update_records",
		Description: "通用批量更新演出记录（按 ID 列表）。标量字段直接赋值（name/分类/评分/状态/城市/场馆/渠道/剧团/同行/备注/座位/date_text 演出时间/coordinate 坐标/票价等金额字段）；数组字段（drama_ids/zhezi_ids/artist_names/play/guest/tag_ids/category_names 多剧种）支持 set/append/remove 三种操作。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览变更而不修改。",
	}, s.handleBatchUpdateRecords)

	// ---- 演出记录 CRUD ----
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_record",
		Description: "创建一条新的演出记录。name 必填，其余字段可选。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不创建。",
		InputSchema: toolSchema([]string{"name"}, map[string]*jsonschema.Schema{
			"name":              strProp(),
			"channel":           strProp(),
			"city":              strProp(),
			"address":           strProp(),
			"cover_file":        strProp(),
			"cover_thumb":       strProp(),
			"category_name":     strProp(),
			"category_names":    arrayProp("string"),
			"artist_ids":        arrayProp("string"),
			"artist_names":      arrayProp("string"),
			"guest":             arrayProp("string"),
			"play":              arrayProp("string"),
			"drama_ids":         arrayProp("string"),
			"zhezi_ids":         arrayProp("string"),
			"tag_ids":           arrayProp("string"),
			"date_text":         strProp(),
			"rating":            intProp(),
			"seat":              strProp(),
			"friends":           strProp(),
			"company":           strProp(),
			"remark":            strProp(),
			"active_status":     intProp(),
			"price":             numProp(),
			"price_currency":    strProp(),
			"pay_price":         numProp(),
			"pay_price_currency": strProp(),
			"other_cost":        numProp(),
			"other_cost_currency": strProp(),
			"dry_run":           boolProp(),
		}),
	}, s.handleCreateRecord)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "update_record",
		Description: "更新单条演出记录的任意字段（nil 保持不变）。支持标量字段直接赋值和数组字段 set/append/remove。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。",
	}, s.handleUpdateRecord)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "delete_record",
		Description: "删除单条演出记录。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不删除。",
	}, s.handleDeleteRecord)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "batch_delete_records",
		Description: "批量删除多条演出记录（按 ID 列表）。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不删除。",
	}, s.handleBatchDeleteRecords)

	// ---- 剧目管理 ----
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_drama",
		Description: "创建新剧目档案。name 必填，剧种默认由关联演出自动聚合。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不创建。",
		InputSchema: toolSchema([]string{"name"}, map[string]*jsonschema.Schema{
			"name":           strProp(),
			"aliases":        arrayProp("string"),
			"category_name":  strProp(),
			"category_names": arrayProp("string"),
			"remark":         strProp(),
			"dry_run":        boolProp(),
		}),
	}, s.handleCreateDrama)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "update_drama",
		Description: "更新剧目档案的名称/别名/备注/剧种。剧种默认由关联演出自动聚合；category_names 提供非空列表时手动覆盖（用于修正拼盘演出导致的聚合偏差），空数组则清除覆盖回到自动。未提供的字段保持不变。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。",
	}, s.handleUpdateDrama)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "delete_drama",
		Description: "删除剧目及其所有折子。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不删除。",
	}, s.handleDeleteDrama)

	// ---- 折子管理 ----
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "batch_create_zhezis",
		Description: "为剧目批量创建折子（自动跳过该剧目下已存在的同名折子）。配合网络搜索到的「常演折子」清单一次性写入。返回新建与跳过的清单。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不创建。",
		InputSchema: toolSchema([]string{"names"}, map[string]*jsonschema.Schema{
			"drama_id":   strProp(),
			"drama_name": strProp(),
			"names":      arrayProp("string"),
			"remark":     strProp(),
			"dry_run":    boolProp(),
		}),
	}, s.handleBatchCreateZhezis)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "update_zhezi",
		Description: "更新折子的名称/别名/备注。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。",
	}, s.handleUpdateZhezi)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "delete_zhezi",
		Description: "删除折子（同时解除与所有演出记录的关联）。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不删除。",
	}, s.handleDeleteZhezi)

	// ---- 演员管理 ----
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_artist",
		Description: "创建新演员档案。name 必填，可附带别名和简介。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不创建。",
		InputSchema: toolSchema([]string{"name"}, map[string]*jsonschema.Schema{
			"name":    strProp(),
			"aliases": arrayProp("string"),
			"remark":  strProp(),
			"bio":     strProp(),
			"dry_run": boolProp(),
		}),
	}, s.handleCreateArtist)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "update_artist",
		Description: "更新演员的名称/别名/备注/简介。未提供的字段保持不变。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。",
	}, s.handleUpdateArtist)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "delete_artist",
		Description: "删除演员档案（同时解除与演出记录的关联）。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不删除。",
	}, s.handleDeleteArtist)

	// ---- 分类管理 ----
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_categories",
		Description: "列出所有分类（剧种），含演出计数和排序。",
	}, s.handleListCategories)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_category",
		Description: "创建新分类。name 必填。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不创建。",
	}, s.handleCreateCategory)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "update_category",
		Description: "更新分类名称。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。",
	}, s.handleUpdateCategory)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "delete_category",
		Description: "删除分类。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不删除。",
	}, s.handleDeleteCategory)

	// ---- 封面管理 ----
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_covers",
		Description: "列出封面（去重），支持按文件名查询，含引用计数。",
	}, s.handleListCovers)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "cover_duplicates",
		Description: "查找内容哈希相同的重复封面分组。",
	}, s.handleCoverDuplicates)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "merge_covers",
		Description: "合并重复封面：将 sources 的引用全部指向 target，然后删除 sources。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不修改。",
	}, s.handleMergeCovers)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "cover_orphans",
		Description: "查找没有被任何演出记录引用的孤立封面文件。",
	}, s.handleCoverOrphans)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "cleanup_covers",
		Description: "清理所有孤立封面（无引用的文件）。dry_run 默认为 true（仅预览，不真正修改；显式传 dry_run=false 才执行）；预览不删除。",
	}, s.handleCleanupCovers)
}

// jsonResult renders v as pretty JSON text content for the model.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
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
