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
		Description: "把某位演员参与的所有演出的剧团(company)统一改成指定名称。dry_run=true 时只预览将受影响的记录而不修改。",
	}, s.handleBatchUpdateCompanyByArtist)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "batch_merge_venues",
		Description: "合并场馆：把 source_address 的所有演出记录的地址改为 target_address，并可选地把 target 地址已有的坐标同步给这些记录。dry_run=true 时只预览不修改。",
	}, s.handleBatchMergeVenues)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "batch_update_records",
		Description: "通用批量更新演出记录（按 ID 列表）。标量字段直接赋值；数组字段（drama_ids/zhezi_ids/artist_names/play/guest/tag_ids）支持 set/append/remove 三种操作。",
	}, s.handleBatchUpdateRecords)

	// ---- 折子管理 ----
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "batch_create_zhezis",
		Description: "为剧目批量创建折子（自动跳过该剧目下已存在的同名折子）。配合网络搜索到的「常演折子」清单一次性写入。返回新建与跳过的清单。",
	}, s.handleBatchCreateZhezis)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "update_zhezi",
		Description: "更新折子的名称/别名/备注。",
	}, s.handleUpdateZhezi)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "delete_zhezi",
		Description: "删除折子（同时解除与所有演出记录的关联）。",
	}, s.handleDeleteZhezi)
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
