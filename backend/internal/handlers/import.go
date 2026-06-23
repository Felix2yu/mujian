package handlers

import (
	"fmt"
	"mujian/internal/models"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type ImportResult struct {
	Total   int      `json:"total"`
	Success int      `json:"success"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
}

func (h *Handler) importShows(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		jsonErr(w, 400, "no file provided")
		return
	}
	defer file.Close()

	ext := strings.ToLower(header.Filename)
	if !strings.HasSuffix(ext, ".xlsx") && !strings.HasSuffix(ext, ".xls") {
		jsonErr(w, 400, "only .xlsx and .xls files are supported")
		return
	}

	f, err := excelize.OpenReader(file)
	if err != nil {
		jsonErr(w, 400, "failed to open file: "+err.Error())
		return
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	if sheetName == "" {
		jsonErr(w, 400, "no sheets found in file")
		return
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		jsonErr(w, 400, "failed to read sheet: "+err.Error())
		return
	}

	if len(rows) < 2 {
		jsonErr(w, 400, "file must have a header row and at least one data row")
		return
	}

	headerRow := rows[0]
	colMap := parseColumns(headerRow)

	result := ImportResult{Total: len(rows) - 1}

	for i, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}

		show := h.parseRowToShow(row, colMap)
		if show.Name == "" {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Row %d: name is empty", i+2))
			continue
		}

		if _, err := h.db.CreateShow(show); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Row %d: %s", i+2, err.Error()))
			continue
		}
		result.Success++
	}

	jsonResp(w, 200, result)
}

func parseColumns(header []string) map[string]int {
	m := make(map[string]int)
	aliases := map[string][]string{
		"name":        {"名称", "演出名称", "name", "title"},
		"venue":       {"场地", "场馆", "venue", "location"},
		"date":        {"日期", "时间", "date", "time"},
		"duration":    {"时长", "分钟", "duration", "minutes"},
		"status":      {"状态", "status"},
		"category":    {"分类", "类型", "category", "type"},
		"company":     {"剧团", "公司", "company"},
		"cast":        {"阵容", "演员", "cast", "actors"},
		"friends":     {"同行", "好友", "friends"},
		"rating":      {"评分", "rating"},
		"seat":        {"座位", "seat"},
		"ticket_cost": {"门票", "票价", "ticket_cost", "price"},
		"other_cost":  {"其他花费", "其他费用", "other_cost"},
		"setlist":     {"剧目", "曲目", "setlist"},
		"review":      {"剧评", "评论", "review", "comment"},
		"notes":       {"备注", "说明", "notes"},
		"poster_url":  {"海报", "图片", "poster_url", "poster", "image"},
	}

	for colIdx, colName := range header {
		colLower := strings.TrimSpace(strings.ToLower(colName))
		for field, aliasesList := range aliases {
			for _, alias := range aliasesList {
				if colLower == alias {
					m[field] = colIdx
				}
			}
		}
	}
	return m
}

func (h *Handler) parseRowToShow(row []string, colMap map[string]int) models.ShowRequest {
	get := func(field string) string {
		if idx, ok := colMap[field]; ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}

	show := models.ShowRequest{
		Name:      get("name"),
		Venue:     get("venue"),
		Cast:      get("cast"),
		Company:   get("company"),
		Friends:   get("friends"),
		Seat:      get("seat"),
		Notes:     get("notes"),
		Review:    get("review"),
		PosterURL: get("poster_url"),
		Setlist:   get("setlist"),
	}

	if d := get("date"); d != "" {
		loc := h.cfg.Location()
		formats := []string{
			"2006-01-02 15:04",
			"2006-01-02T15:04",
			"2006/01/02 15:04",
			"2006.01.02 15:04",
			"2006-01-02",
			"2006/01/02",
			"2006.01.02",
			"01/02/2006",
			"1/2/2006",
		}
		for _, format := range formats {
			if t, err := time.ParseInLocation(format, d, loc); err == nil {
				show.Date = t.Format("2006-01-02T15:04")
				break
			}
		}
		if show.Date == "" {
			show.Date = d
		}
	}

	if d := get("duration"); d != "" {
		if v, err := strconv.Atoi(d); err == nil {
			show.Duration = v
		}
	}

	if s := get("status"); s != "" {
		switch strings.ToLower(s) {
		case "已观看", "watched", "看过":
			show.Status = "watched"
		case "已取消", "cancelled", "取消":
			show.Status = "cancelled"
		default:
			show.Status = "planned"
		}
	} else {
		show.Status = "planned"
	}

	if c := get("category"); c != "" {
		if id, err := strconv.ParseInt(c, 10, 64); err == nil {
			show.CategoryID = &id
		} else {
			if cat, err := h.db.FindOrCreateCategory(c); err == nil {
				show.CategoryID = &cat.ID
			}
		}
	}

	if r := get("rating"); r != "" {
		if v, err := strconv.Atoi(r); err == nil && v >= 1 && v <= 5 {
			show.Rating = &v
		}
	}

	if c := get("ticket_cost"); c != "" {
		if v, err := strconv.ParseFloat(c, 64); err == nil {
			show.TicketCost = &v
		}
	}

	if c := get("other_cost"); c != "" {
		if v, err := strconv.ParseFloat(c, 64); err == nil {
			show.OtherCost = &v
		}
	}

	return show
}

func (h *Handler) getImportTemplate(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "演出数据"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{
		"名称", "场地", "日期", "时长", "状态", "分类",
		"剧团", "阵容", "同行", "评分", "座位",
		"门票", "其他花费", "剧目", "剧评", "备注",
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	example := []string{
		"茶馆", "国家大剧院", "2026-07-15 19:30", "180", "计划中", "话剧",
		"北京人艺", "于是之, 郑榕", "小明", "5", "3排15座",
		"280", "50", "茶馆第一幕\n茶馆第二幕", "非常精彩", "",
	}
	for i, v := range example {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(sheet, cell, v)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=mujian_import_template.xlsx")
	f.Write(w)
}

func (h *Handler) exportShows(w http.ResponseWriter, r *http.Request) {
	shows, err := h.db.ListAllShows()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	f := excelize.NewFile()
	defer f.Close()

	sheet := "演出数据"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{
		"名称", "场地", "日期", "时长", "状态", "分类",
		"剧团", "阵容", "同行", "评分", "座位",
		"门票", "其他花费", "剧目", "剧评", "备注",
	}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, header)
	}

	statusMap := map[string]string{
		"planned":   "计划中",
		"watched":   "已观看",
		"cancelled": "已取消",
	}

	for rowIdx, show := range shows {
		row := rowIdx + 2

		setCell := func(col int, val string) {
			cell, _ := excelize.CoordinatesToCellName(col, row)
			f.SetCellValue(sheet, cell, val)
		}

		setCell(1, show.Name)
		setCell(2, show.Venue)
		setCell(3, show.Date.Format("2006-01-02 15:04"))
		setCell(4, strconv.Itoa(show.Duration))
		setCell(5, statusMap[string(show.Status)])
		setCell(6, show.CategoryName)
		setCell(7, show.Company)
		setCell(8, show.Cast)
		setCell(9, show.Friends)
		if show.Rating != nil {
			setCell(10, strconv.Itoa(*show.Rating))
		}
		setCell(11, show.Seat)
		if show.TicketCost != nil {
			setCell(12, fmt.Sprintf("%.2f", *show.TicketCost))
		}
		if show.OtherCost != nil {
			setCell(13, fmt.Sprintf("%.2f", *show.OtherCost))
		}
		setCell(14, show.Setlist)
		setCell(15, show.Review)
		setCell(16, show.Notes)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=mujian_export_%s.xlsx", time.Now().Format("20060102")))
	f.Write(w)
}
