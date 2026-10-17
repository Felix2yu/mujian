// Package holiday 提供中国节假日（含调休 / 补班）数据：
//
//   - 内置兜底：builtin.json 收录已发布年份的放假安排（依据国务院办公厅
//     通知，经 holiday-cn 汇总），启动时种子写入，离线可用；
//   - 外部刷新：每天最多一次尝试从同一数据源拉取当年与次年的最新数据，
//     拉取失败（网络 / 源站不可用）时静默保留已有数据，不影响主流程。
package holiday

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mujian/internal/db"
	"mujian/internal/models"
	"net/http"
	"strconv"
	"time"
)

// builtinJSON 是随二进制发布的内置节假日数据（来源见文件内 $comment）。
//
//go:embed builtin.json
var builtinJSON []byte

// remoteBase 为外部刷新的数据源（与内置数据同源，格式一致）。
const remoteBase = "https://raw.githubusercontent.com/NateScarlet/holiday-cn/master/"

// refreshInterval 是两次外部刷新之间的最小间隔：每天最多打扰数据源一次，
// 跨进程重启依然生效（时间戳存 DB）。
const refreshInterval = 12 * time.Hour

// remoteFile 是数据源的单年文件结构（holiday-cn 格式）。
type remoteFile struct {
	Year int              `json:"year"`
	Days []models.HolidayDay `json:"days"`
}

// httpClient 的超时刻意收紧：刷新是后台旁路，绝不能长时间占用。
var httpClient = &http.Client{Timeout: 15 * time.Second}

// BuiltinDays 解析内置数据。解析失败视为编程错误（数据随二进制发布），
// 返回 error 由调用方决定是否兜底。
func BuiltinDays() ([]models.HolidayDay, error) {
	var f remoteFile
	if err := json.Unmarshal(builtinJSON, &f); err != nil {
		return nil, fmt.Errorf("parse builtin holidays: %w", err)
	}
	return f.Days, nil
}

// Seed 把内置数据写入 DB（幂等，只补缺失日期）。失败只记日志：
// 节假日是日历页的增强信息，不应阻塞服务启动。
func Seed(database *db.DB) {
	days, err := BuiltinDays()
	if err != nil {
		slog.Warn("load builtin holidays", "err", err)
		return
	}
	if err := database.SeedHolidayDays(days, "builtin"); err != nil {
		slog.Warn("seed builtin holidays", "err", err)
	}
}

// Refresh 尝试从外部源刷新当年与次年的节假日数据。
// 距上次成功不足 refreshInterval 时直接跳过；全部年份拉取失败时不更新
// 成功时间戳（下个周期自动重试），部分成功即视为整体成功。
func Refresh(ctx context.Context, database *db.DB) error {
	if time.Since(time.Unix(database.HolidayRefreshedAt(), 0)) < refreshInterval {
		return nil
	}
	years := []int{time.Now().Year(), time.Now().Year() + 1}
	ok := false
	var lastErr error
	for _, y := range years {
		days, found, err := fetchYear(ctx, y)
		if err != nil {
			lastErr = err
			continue
		}
		if !found {
			// 次年的放假通知通常要到年底才发布：404 属正常时序，不算失败。
			continue
		}
		if err := database.ReplaceHolidayYear(y, days, "remote"); err != nil {
			lastErr = err
			continue
		}
		ok = true
	}
	if !ok {
		if lastErr != nil {
			return lastErr
		}
		return nil // 两个年份都 404：数据源尚未发布，静默等待下个周期
	}
	return database.SetHolidayRefreshedAt(time.Now().Unix())
}

// fetchYear 拉取某年数据。found=false 表示数据源尚未发布该年（404）。
func fetchYear(ctx context.Context, year int) (days []models.HolidayDay, found bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		remoteBase+strconv.Itoa(year)+".json", nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "mujian-holiday-refresh")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("fetch holidays %d: %w", year, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("fetch holidays %d: status %d", year, resp.StatusCode)
	}
	var f remoteFile
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&f); err != nil {
		return nil, false, fmt.Errorf("parse holidays %d: %w", year, err)
	}
	if f.Year != 0 && f.Year != year {
		return nil, false, fmt.Errorf("holidays %d: payload year %d mismatch", year, f.Year)
	}
	return f.Days, true, nil
}
