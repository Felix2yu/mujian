package handlers

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mujian/internal/models"
	"mujian/internal/storage"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// importRecords ingests:
//  1. a plain data.json file (already-converted export), or
//  2. the original 记录现场 export archive JI_LU_XIAN_CHANG.android.zip
//     (contains JI_LU_XIAN_CHANG.android, a zlib/raw-deflate JSON, plus
//     covers/<uuid> files that are base64-encoded images), or
//  3. a zip containing data.json + covers/ (converted layout).
//
// Records/categories/meta are upserted; covers are decoded to binary and
// written into <UploadDir>/covers/, with each record's coverFile derived from
// the cover UUID when it is absent.
func (h *Handler) importRecords(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(512 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		jsonErr(w, 400, "no file provided")
		return
	}
	defer file.Close()

	name := strings.ToLower(header.Filename)
	switch {
	case strings.HasSuffix(name, ".json"):
		h.importJSON(w, file)
	case strings.HasSuffix(name, ".zip"):
		h.importZIP(w, file)
	default:
		jsonErr(w, 400, "仅支持 .json 文件或「记录现场」导出的 .zip 压缩包（JI_LU_XIAN_CHANG.android.zip）")
	}
}

// importJSON: plain data.json, no covers included.
func (h *Handler) importJSON(w http.ResponseWriter, file io.Reader) {
	raw, err := io.ReadAll(file)
	if err != nil {
		jsonErr(w, 400, "failed to read file: "+err.Error())
		return
	}
	data, err := parseImport(raw)
	if err != nil {
		jsonErr(w, 400, "failed to parse export: "+err.Error())
		return
	}
	if len(data.Records) == 0 && len(data.Categories) == 0 {
		jsonErr(w, 400, "no records or categories found in file")
		return
	}

	result, err := h.db.ImportData(data)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{
		"message":         "import completed",
		"records":         result.Records,
		"categories":      result.Categories,
		"covers_imported": 0,
		"covers_missing":  0,
	})
}

// importZIP: the 记录现场 export archive. Locates the data file
// (JI_LU_XIAN_CHANG.android, raw-deflate/zlib JSON; or data.json), imports the
// records, and decodes/copies cover files into <UploadDir>/covers/.
func (h *Handler) importZIP(w http.ResponseWriter, file io.Reader) {
	raw, err := io.ReadAll(file)
	if err != nil {
		jsonErr(w, 400, "failed to read archive: "+err.Error())
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		jsonErr(w, 400, "invalid zip archive: "+err.Error())
		return
	}

	var data models.ExportData
	switch entry := findZipBySuffix(zr, "ji_lu_xian_chang.android"); {
	case entry != nil:
		if err := readDecompressedJSON(entry, &data); err != nil {
			jsonErr(w, 400, "failed to parse JI_LU_XIAN_CHANG.android: "+err.Error())
			return
		}
	case findZipBySuffix(zr, "data.json") != nil:
		if err := readZipJSON(findZipBySuffix(zr, "data.json"), &data); err != nil {
			jsonErr(w, 400, "failed to parse data.json: "+err.Error())
			return
		}
	default:
		jsonErr(w, 400, "压缩包内未找到 JI_LU_XIAN_CHANG.android 或 data.json")
		return
	}

	if len(data.Records) == 0 && len(data.Categories) == 0 {
		jsonErr(w, 400, "no records or categories found in data file")
		return
	}

	// extract covers first so each record's coverFile/coverThumb are derived
	// before upsert; storage is content-addressed (auto-dedupe by hash).
	imported, missing := h.extractCovers(zr, data.Records)

	result, err := h.db.ImportData(&data)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonResp(w, 200, map[string]interface{}{
		"message":         "import completed",
		"records":         result.Records,
		"categories":      result.Categories,
		"covers_imported": imported,
		"covers_missing":  missing,
	})
}

// ---------- locating & parsing the data file ----------

func findZipBySuffix(zr *zip.Reader, suffix string) *zip.File {
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(f.Name), suffix) {
			return f
		}
	}
	return nil
}

func readZipJSON(f *zip.File, out *models.ExportData) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return json.NewDecoder(rc).Decode(out)
}

// readDecompressedJSON inflates JI_LU_XIAN_CHANG.android. 记录现场 uses a raw
// deflate stream ("raw-inflate"), so we try flate first, then zlib.
func readDecompressedJSON(f *zip.File, out *models.ExportData) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	compressed, err := io.ReadAll(rc)
	if err != nil {
		return err
	}

	data, err := inflate(compressed, true)
	if err != nil || !json.Valid(data) {
		data, err = inflate(compressed, false)
		if err != nil {
			return fmt.Errorf("inflate failed: %w", err)
		}
	}
	if !json.Valid(data) {
		return fmt.Errorf("decompressed content is not valid JSON")
	}
	parsed, err := parseImport(data)
	if err != nil {
		return err
	}
	*out = *parsed
	return nil
}

// importBundle captures both the mujian export layout (records/categories) and
// the original 记录现场 JI_LU_XIAN_CHANG layout (active/customCategory), whose
// top-level array keys differ. Categories are linked to records by
// customCategoryId, which we resolve into the categoryName field.
type importBundle struct {
	models.ExportData
	Active         []models.Record   `json:"active"`
	CustomCategory []models.Category `json:"customCategory"`
}

// parseImport decodes an export file into the canonical ExportData. It accepts
// both the mujian data.json layout and the 记录现场 JI_LU_XIAN_CHANG.android
// layout, and fills each record's categoryName from its customCategoryId.
func parseImport(raw []byte) (*models.ExportData, error) {
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid JSON")
	}
	var b importBundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	res := &models.ExportData{
		Source:       b.Source,
		ExportedAt:   b.ExportedAt,
		RecordCount:  b.RecordCount,
		CoverMissing: b.CoverMissing,
		CoverDir:     b.CoverDir,
		CoverNote:    b.CoverNote,
		Meta:         b.Meta,
		Records:      b.Records,
		Categories:   b.Categories,
	}
	if len(res.Records) == 0 {
		res.Records = b.Active
	}
	if len(res.Categories) == 0 {
		res.Categories = b.CustomCategory
	}
	if len(res.Categories) > 0 {
		nameOf := make(map[string]string, len(res.Categories))
		for _, c := range res.Categories {
			nameOf[c.ID] = c.Name
		}
		for i := range res.Records {
			if res.Records[i].CategoryName == "" && res.Records[i].CustomCategoryID != "" {
				if n, ok := nameOf[res.Records[i].CustomCategoryID]; ok {
					res.Records[i].CategoryName = n
				}
			}
			// 记录现场只给出 unix date，不给 dateText；列表/地图页依赖
			// dateText 显示日期，缺失时从 date 推导（本地时区，与前端 formatDate 一致）。
			if res.Records[i].DateText == "" && res.Records[i].Date != 0 {
				res.Records[i].DateText = time.Unix(res.Records[i].Date, 0).Format("2006-01-02 15:04:05")
			}
		}
	}
	return res, nil
}

func inflate(b []byte, raw bool) ([]byte, error) {
	var r io.ReadCloser
	if raw {
		r = flate.NewReader(bytes.NewReader(b))
	} else {
		zr, err := zlib.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		r = zr
	}
	defer r.Close()
	return io.ReadAll(r)
}

// ---------- covers ----------

type zipIndex struct {
	byName map[string]*zip.File // exact basename, lowercased
	byBase map[string]*zip.File // basename without extension, lowercased
}

func buildZipIndex(zr *zip.Reader) *zipIndex {
	idx := &zipIndex{byName: map[string]*zip.File{}, byBase: map[string]*zip.File{}}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		idx.byName[strings.ToLower(base)] = f
		idx.byBase[strings.ToLower(stripExt(base))] = f
	}
	return idx
}

func stripExt(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[:i]
	}
	return name
}

// extractCovers materializes each record's cover from the zip (decoding
// base64 when needed), stores it content-addressed via storage (auto-dedupe
// by hash), derives coverFile, generates a thumbnail, and registers cover
// metadata.
func (h *Handler) extractCovers(zr *zip.Reader, records []models.Record) (int, int) {
	idx := buildZipIndex(zr)
	imported, missing := 0, 0

	for i := range records {
		rec := &records[i]
		if rec.CoverFile == "" && rec.Cover == "" {
			continue
		}

		var entry *zip.File
		if rec.CoverFile != "" {
			base := filepath.Base(rec.CoverFile)
			entry = idx.byName[strings.ToLower(base)]
			if entry == nil {
				entry = idx.byBase[strings.ToLower(stripExt(base))]
			}
		} else {
			key := strings.ToLower(rec.Cover)
			entry = idx.byBase[key]
			if entry == nil {
				entry = idx.byName[key]
			}
		}
		if entry == nil {
			missing++
			continue
		}

		data, _, err := materializeCover(entry)
		if err != nil {
			missing++
			continue
		}

		format := h.cfg.ImageFormat
		if !isSupportedImageFormat(format) {
			format = "avif"
		}

		// Re-encode the cover to the chosen format so imported libraries honor
		// the user's encoding preference, then store it content-addressed.
		// Skip the costly re-encode when the source is already in the target
		// format — e.g. a library pre-converted to AVIF on a faster machine —
		// so import is a no-op re-encode and just preserves the original bytes.
		storeData, ext := data, storage.DetectExt(data)
		if srcFmt := storage.DetectImageFormat(data); srcFmt == format {
			ext = storage.ExtForImageFormat(format)
		} else if img, derr := storage.DecodeImage(data); derr == nil {
			if enc, eext, eerr := storage.EncodeImage(img, format); eerr == nil {
				storeData, ext = enc, eext
			}
		}

		key, _, err := h.storage.SaveCoverBytes(storeData, ext)
		if err != nil {
			missing++
			continue
		}
		rec.CoverFile = key
		if tk, terr := h.storage.MakeThumbnail(key, storeData, 400, format); terr == nil {
			rec.CoverThumb = tk
		}
		h.db.UpsertCoverMeta(storage.HashBytes(storeData), key, ext, int64(len(storeData)))
		imported++
	}
	return imported, missing
}

// materializeCover returns binary image bytes, transparently handling both
// binary files (converted exports) and base64-encoded files (the original
// JI_LU_XIAN_CHANG covers/).
func materializeCover(f *zip.File) ([]byte, string, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", err
	}

	if ext, ok := magic(b); ok {
		return b, ext, nil
	}

	// base64 text: strip data-uri prefix and whitespace, then decode
	s := strings.TrimSpace(string(b))
	if i := strings.Index(s, ","); i >= 0 && strings.HasPrefix(s, "data:") {
		s = s[i+1:]
	}
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	if dec, err := base64.StdEncoding.DecodeString(s); err == nil {
		if ext, ok := magic(dec); ok {
			return dec, ext, nil
		}
	}
	return nil, "", fmt.Errorf("unsupported cover format")
}

func magic(b []byte) (string, bool) {
	if len(b) < 4 {
		return "", false
	}
	if b[0] == 0xff && b[1] == 0xd8 {
		return ".jpg", true
	}
	if b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4e && b[3] == 0x47 {
		return ".png", true
	}
	if b[0] == 'R' && b[1] == 'I' && b[2] == 'F' && b[3] == 'F' && len(b) >= 12 && string(b[8:12]) == "WEBP" {
		return ".webp", true
	}
	return "", false
}
