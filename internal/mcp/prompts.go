package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Prompts are server-defined workflow scripts: unlike tools (invoked by the
// model), a prompt is triggered by the user (surfaced as a slash command in
// supporting clients) and injects step-by-step instructions that guide the
// model through a multi-tool workflow. They encode the safe patterns from
// docs/mcp.md — preview first, confirm with the user, only then apply — so
// clients without workspace instructions (AGENTS.md) still follow them.
func (s *Server) registerPrompts() {
	s.server.AddPrompt(&mcp.Prompt{
		Name:        "data_checkup",
		Title:       "数据体检",
		Description: "只读巡检演出数据：发现重复写法、重复封面与空字段，输出体检报告和建议的修复动作（先预览，确认后才执行）。",
	}, s.promptDataCheckup)

	s.server.AddPrompt(&mcp.Prompt{
		Name:        "unify_company",
		Title:       "按演员统一剧团",
		Description: "把某位演员参与的所有演出的剧团(company)统一为确认过的名称，先预览再执行。",
		Arguments: []*mcp.PromptArgument{{
			Name:        "artist_name",
			Description: "演员姓名或别名；不填则先询问用户",
		}},
	}, s.promptUnifyCompany)

	s.server.AddPrompt(&mcp.Prompt{
		Name:        "merge_venues",
		Title:       "合并场馆写法",
		Description: "找出同一场馆的不同地址写法并与用户逐对确认，预览后合并（可同步坐标）。",
		Arguments: []*mcp.PromptArgument{{
			Name:        "venue_keyword",
			Description: "场馆名称关键词；不填则先询问用户",
		}},
	}, s.promptMergeVenues)

	s.server.AddPrompt(&mcp.Prompt{
		Name:        "enrich_zhezis",
		Title:       "补充剧目常演折子",
		Description: "查看剧目已有折子，经网络查证常演折子清单，用户确认后批量写入。",
		Arguments: []*mcp.PromptArgument{{
			Name:        "drama_name",
			Description: "剧目名称；不填则先询问用户",
		}},
	}, s.promptEnrichZhezis)

	s.server.AddPrompt(&mcp.Prompt{
		Name:        "backup_export",
		Title:       "备份与导出",
		Description: "引导完成快照备份、JSON 导出或从备份恢复数据（恢复前先预览并确认）。",
	}, s.promptBackupExport)
}

// promptArg returns a trimmed prompt argument, or "" if not provided.
func promptArg(req *mcp.GetPromptRequest, name string) string {
	if req == nil || req.Params == nil {
		return ""
	}
	return strings.TrimSpace(req.Params.Arguments[name])
}

func userMsg(format string, args ...any) *mcp.PromptMessage {
	return &mcp.PromptMessage{
		Role:    "user",
		Content: &mcp.TextContent{Text: fmt.Sprintf(format, args...)},
	}
}

func (s *Server) promptDataCheckup(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{userMsg(`请为幕间演出数据库做一次数据体检。体检阶段只读，不要修改任何数据。

步骤：
1. 用 value_counts 分别统计 company、city、channel、category_name 字段的取值分布，找出疑似同一实体的不同写法（如「上昆」与「上海昆剧团」）。
2. 用 cover_duplicates 查找内容重复的封面分组，用 cover_orphans 查找无引用的孤立封面。
3. 用 search_records 的 missing 参数检查关键字段为空的记录（如 missing="category,coordinate,channel"），统计数量。
4. 汇总为一份体检报告：按问题分类列出影响条数和具体示例，不要罗列全部原始数据。
5. 针对每个问题给出建议的修复工具与参数（merge_artists / merge_dramas / batch_merge_venues / batch_update_records / merge_covers / cleanup_covers 等），并先以 dry_run 预览影响范围。
6. 逐项向用户确认后，才对确认的项以 dry_run=false 执行，最后汇报每项的实际修改结果。`)}}, nil
}

func (s *Server) promptUnifyCompany(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	name := promptArg(req, "artist_name")
	target := name
	if target == "" {
		target = "（用户未指定演员：先用 list_artists 查询候选，或直接询问用户要处理哪位演员）"
	}
	return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{userMsg(`目标：把演员「%s」参与的所有演出的剧团(company)统一为正确的名称。

步骤：
1. 用 get_artist_detail（支持姓名或别名）或 search_records(artist_name=...) 查看该演员的全部演出记录，列出当前 company 的取值分布。
2. 与用户确认正确的剧团名称；若记录中已有明显主流写法可建议之，但必须经用户确认。
3. 用 batch_update_company_by_artist(artist_name=..., company=...) 先预览（dry_run 默认即为预览），向用户展示将被修改的记录清单。
4. 用户确认后以 dry_run=false 执行，并汇报实际修改的记录数。`, target)}}, nil
}

func (s *Server) promptMergeVenues(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	kw := promptArg(req, "venue_keyword")
	target := kw
	if target == "" {
		target = "（用户未指定场馆：先询问用户要整理哪个场馆，或用 list_venues 找出演出次数最多、写法有分歧的场馆候选）"
	}
	return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{userMsg(`目标：合并场馆「%s」的不同地址写法。

步骤：
1. 用 list_venues(query=...) 列出所有匹配的场馆写法，含各自的演出次数、城市与坐标状态。
2. 与用户确认哪一个是标准写法（target），哪些是要并入的写法（source）；逐对确认，不要凭相似度猜测合并。
3. 对每一对：用 batch_merge_venues(source_address=..., target_address=..., sync_coordinates=true) 先预览（dry_run 默认即为预览），向用户展示受影响的记录。
4. 用户确认后以 dry_run=false 执行，并汇报每对合并的记录数。`, target)}}, nil
}

func (s *Server) promptEnrichZhezis(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	name := promptArg(req, "drama_name")
	target := name
	if target == "" {
		target = "（用户未指定剧目：先询问用户要补充哪个剧目，或用 list_dramas 列出候选）"
	}
	return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{userMsg(`目标：为剧目「%s」补充常演折子清单。

步骤：
1. 用 get_drama_detail（支持名称查找）查看该剧目已有的折子，避免重复补充。
2. 通过网络搜索（WebSearch/WebFetch）查证该剧目的常演折子：优先参考剧团官网、维基百科等权威来源，交叉验证多个来源后再采纳，剔除不可靠或过时的信息。
3. 把整理出的折子清单（去掉与已有折子重复的）展示给用户确认。
4. 用 batch_create_zhezis(drama_id=..., names=[...]) 先预览（dry_run 默认即为预览），确认后以 dry_run=false 一次写入（同名折子会自动跳过）。
5. 汇报新建与跳过的折子。`, target)}}, nil
}

func (s *Server) promptBackupExport(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{userMsg(`帮用户完成数据备份或导出。先询问需求属于哪一类，再按对应流程操作：

- 快照备份：直接调用 run_backup（非破坏性，立即执行），然后用 list_backups 确认生成了新备份。
- JSON 导出：调用 export_data(to_file=true) 把完整数据写入备份目录（export-*.json），把返回的文件路径告诉用户；之后可用 import_data(file_path=...) 读回（先预览再执行）。
- 恢复数据：用 list_backups 列出可用备份，与用户确认文件后，用 restore_from_backup(file_path=...) 先预览，用户确认后再以 dry_run=false 执行。

注意：恢复/导入按记录 upsert 覆盖，执行前务必先预览并向用户确认。`)}}, nil
}
