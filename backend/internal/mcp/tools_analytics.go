package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// get_analytics / get_dashboard expose the same aggregates the web UI's
// analytics and dashboard pages render, so agents can answer "how many shows
// did I see last quarter", "which month was most expensive" etc. without
// assembling them from raw records.

func (s *Server) handleGetAnalytics(ctx context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	data, err := s.db.GetAnalytics()
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	return jsonResult(data)
}

func (s *Server) handleGetDashboard(ctx context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	stats, err := s.db.GetDashboardStats()
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	return jsonResult(stats)
}
