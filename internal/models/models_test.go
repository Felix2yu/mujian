package models

import (
	"encoding/json"
	"testing"
)

func TestRecordJSONRoundTrip(t *testing.T) {
	f := 123.45
	r := Record{
		ID: "test-1", Name: "测试演出", City: "上海",
		Price: &f, PriceCurrency: "CNY",
		ArtistNames: []string{"张军"}, Play: []string{"惊梦"},
		ActiveStatus: StatusNormal,
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var r2 Record
	if err := json.Unmarshal(b, &r2); err != nil {
		t.Fatal(err)
	}
	if r2.Name != "测试演出" || r2.Price == nil || *r2.Price != 123.45 {
		t.Errorf("round trip mismatch: %+v", r2)
	}
}

func TestCategoryJSON(t *testing.T) {
	c := Category{ID: "c1", Name: "昆曲", SortOrder: 2}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var c2 Category
	if err := json.Unmarshal(b, &c2); err != nil {
		t.Fatal(err)
	}
	if c2.Name != "昆曲" || c2.SortOrder != 2 {
		t.Errorf("category round trip: %+v", c2)
	}
}

func TestDramaJSON(t *testing.T) {
	d := Drama{ID: "d1", Name: "牡丹亭", ZheziCount: 3, RecordCount: 5}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var d2 Drama
	if err := json.Unmarshal(b, &d2); err != nil {
		t.Fatal(err)
	}
	if d2.Name != "牡丹亭" || d2.ZheziCount != 3 {
		t.Errorf("drama round trip: %+v", d2)
	}
}

func TestZheziJSON(t *testing.T) {
	z := Zhezi{ID: "z1", Name: "惊梦", DramaID: "d1", SortOrder: 1}
	b, err := json.Marshal(z)
	if err != nil {
		t.Fatal(err)
	}
	var z2 Zhezi
	if err := json.Unmarshal(b, &z2); err != nil {
		t.Fatal(err)
	}
	if z2.Name != "惊梦" || z2.DramaID != "d1" {
		t.Errorf("zhezi round trip: %+v", z2)
	}
}

func TestStatusConstants(t *testing.T) {
	if StatusNormal != 0 || StatusWantWatch != 1 || StatusCancelled != 2 || StatusNoShow != 3 {
		t.Error("status constants have wrong values")
	}
}

func TestExportDataJSON(t *testing.T) {
	ed := ExportData{
		Source:     "mujian",
		RecordCount: 1,
		Records:    []Record{{ID: "r1", Name: "test"}},
		Categories: []Category{{Name: "昆曲"}},
	}
	b, err := json.Marshal(ed)
	if err != nil {
		t.Fatal(err)
	}
	var ed2 ExportData
	if err := json.Unmarshal(b, &ed2); err != nil {
		t.Fatal(err)
	}
	if ed2.Source != "mujian" || ed2.RecordCount != 1 || len(ed2.Records) != 1 {
		t.Errorf("export data round trip: %+v", ed2)
	}
}
