package xlsx

import (
	"archive/zip"
	"bytes"
	"testing"
)

func build(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, body := range parts {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestRead(t *testing.T) {
	data := build(t, map[string]string{
		"xl/workbook.xml": `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
			<sheets><sheet name="Shows" sheetId="1" r:id="rId1"/><sheet name="Movies" sheetId="2" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
			<Relationship Id="rId1" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Target="/xl/worksheets/sheet2.xml"/></Relationships>`,
		"xl/sharedStrings.xml": `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Title</t></si><si><r><t>Bridgerton: </t></r><r><t>Season 4</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
			<row r="2"><c r="B2" t="s"><v>0</v></c><c r="D2" t="inlineStr"><is><t>inline</t></is></c></row>
			<row r="3"><c r="B3" t="s"><v>1</v></c><c r="E3"><v>889800000</v></c><c r="F3" t="s"><v>99</v></c></row>
			</sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row><c><v>1</v></c><c><v>2</v></c></row></sheetData></worksheet>`,
	})
	sheets, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 2 || sheets[0].Name != "Shows" || sheets[1].Name != "Movies" {
		t.Fatalf("sheets = %+v", sheets)
	}
	rows := sheets[0].Rows
	if len(rows) != 3 || rows[0] != nil || rows[1][1] != "Title" || rows[1][3] != "inline" ||
		rows[2][1] != "Bridgerton: Season 4" || rows[2][4] != "889800000" || rows[2][5] != "" {
		t.Fatalf("rows = %q", rows)
	}
	if m := sheets[1].Rows; len(m) != 1 || m[0][0] != "1" || m[0][1] != "2" {
		t.Fatalf("movies = %q", m)
	}
	if _, err := Read([]byte("not a zip")); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestColumnIndex(t *testing.T) {
	for ref, want := range map[string]int{"A1": 0, "C7": 2, "Z9": 25, "AA1": 26, "AMJ1": 1023, "ZZZZ1": -1} {
		if got := columnIndex(ref); got != want {
			t.Errorf("columnIndex(%q) = %d, want %d", ref, got, want)
		}
	}
}
