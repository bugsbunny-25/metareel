// Package xlsx reads the cell text of simple .xlsx workbooks (Office Open
// XML spreadsheets): enough for data exports such as Netflix's engagement
// reports, without formulas, styles or dates as types.
package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// Sheet is a worksheet's rows of cell text, indexed by column (A = 0).
type Sheet struct {
	Name string
	Rows [][]string
}

const (
	maxPartSize = 64 << 20 // per XML part, uncompressed
	maxColumns  = 1024
)

var errTooLarge = errors.New("xlsx part too large")

// Read parses a workbook and returns its sheets in workbook order.
func Read(data []byte) ([]Sheet, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		files[f.Name] = f
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := decodePart(files, "xl/workbook.xml", &wb); err != nil {
		return nil, err
	}
	var rels struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := decodePart(files, "xl/_rels/workbook.xml.rels", &rels); err != nil {
		return nil, err
	}
	targets := map[string]string{}
	for _, r := range rels.Rels {
		t := r.Target
		if strings.HasPrefix(t, "/") {
			t = strings.TrimPrefix(t, "/")
		} else {
			t = path.Join("xl", t)
		}
		targets[r.ID] = t
	}
	shared, err := sharedStrings(files)
	if err != nil {
		return nil, err
	}
	out := make([]Sheet, 0, len(wb.Sheets))
	for _, s := range wb.Sheets {
		target, ok := targets[s.RID]
		if !ok {
			return nil, fmt.Errorf("xlsx: sheet %q has no part", s.Name)
		}
		rows, err := readSheet(files, target, shared)
		if err != nil {
			return nil, fmt.Errorf("xlsx sheet %q: %w", s.Name, err)
		}
		out = append(out, Sheet{Name: s.Name, Rows: rows})
	}
	return out, nil
}

func openPart(files map[string]*zip.File, name string) (io.ReadCloser, error) {
	f, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("xlsx: missing %s", name)
	}
	if f.UncompressedSize64 > maxPartSize {
		return nil, errTooLarge
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("xlsx: open %s: %w", name, err)
	}
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(rc, maxPartSize), rc}, nil
}

func decodePart(files map[string]*zip.File, name string, v any) error {
	rc, err := openPart(files, name)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := xml.NewDecoder(rc).Decode(v); err != nil {
		return fmt.Errorf("xlsx: decode %s: %w", name, err)
	}
	return nil
}

// sharedStrings reads the shared string table (rich text runs joined).
func sharedStrings(files map[string]*zip.File) ([]string, error) {
	if _, ok := files["xl/sharedStrings.xml"]; !ok {
		return nil, nil
	}
	var sst struct {
		Items []struct {
			T    string `xml:"t"`
			Runs []struct {
				T string `xml:"t"`
			} `xml:"r"`
		} `xml:"si"`
	}
	if err := decodePart(files, "xl/sharedStrings.xml", &sst); err != nil {
		return nil, err
	}
	out := make([]string, len(sst.Items))
	for i, it := range sst.Items {
		if len(it.Runs) == 0 {
			out[i] = it.T
			continue
		}
		var b strings.Builder
		for _, r := range it.Runs {
			b.WriteString(r.T)
		}
		out[i] = b.String()
	}
	return out, nil
}

type xmlCell struct {
	Ref    string `xml:"r,attr"`
	Type   string `xml:"t,attr"`
	Value  string `xml:"v"`
	Inline struct {
		T string `xml:"t"`
	} `xml:"is"`
}

// readSheet streams a worksheet's rows.
func readSheet(files map[string]*zip.File, name string, shared []string) ([][]string, error) {
	rc, err := openPart(files, name)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	dec := xml.NewDecoder(rc)
	var rows [][]string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "row" {
			continue
		}
		var row struct {
			Num   int       `xml:"r,attr"`
			Cells []xmlCell `xml:"c"`
		}
		if err := dec.DecodeElement(&row, &se); err != nil {
			return nil, err
		}
		idx := len(rows)
		if row.Num > 0 {
			idx = row.Num - 1
		}
		for len(rows) <= idx {
			rows = append(rows, nil)
		}
		var cells []string
		for i, c := range row.Cells {
			col := i
			if c.Ref != "" {
				col = columnIndex(c.Ref)
			}
			if col < 0 || col >= maxColumns {
				continue
			}
			for len(cells) <= col {
				cells = append(cells, "")
			}
			cells[col] = cellText(c, shared)
		}
		rows[idx] = cells
	}
}

func cellText(c xmlCell, shared []string) string {
	switch c.Type {
	case "s":
		i, err := strconv.Atoi(strings.TrimSpace(c.Value))
		if err != nil || i < 0 || i >= len(shared) {
			return ""
		}
		return shared[i]
	case "inlineStr":
		return c.Inline.T
	default:
		return c.Value
	}
}

// columnIndex turns a cell reference ("C7") into a 0-based column (2).
func columnIndex(ref string) int {
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		n = n*26 + int(r-'A'+1)
		if n > maxColumns {
			return -1
		}
	}
	return n - 1
}
