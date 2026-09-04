package xlsx_test

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ceymard/swl-go/handler/xlsx"
)

func biff12WriteID(buf *bytes.Buffer, id int) {
	for {
		b := id & 0xFF
		id >>= 8
		if id > 0 {
			buf.WriteByte(byte(b) | 0x80)
		} else {
			buf.WriteByte(byte(b) &^ 0x80)
			break
		}
	}
}

func biff12WriteLen(buf *bytes.Buffer, n int) {
	for {
		b := n & 0x7F
		n >>= 7
		if n > 0 {
			buf.WriteByte(byte(b) | 0x80)
		} else {
			buf.WriteByte(byte(b))
			break
		}
	}
}

func biff12WriteRec(buf *bytes.Buffer, id int, payload []byte) {
	biff12WriteID(buf, id)
	biff12WriteLen(buf, len(payload))
	buf.Write(payload)
}

func biff12EncStr(s string) []byte {
	runes := []rune(s)
	var sb bytes.Buffer
	_ = binary.Write(&sb, binary.LittleEndian, uint32(len(runes)))
	for _, r := range runes {
		_ = binary.Write(&sb, binary.LittleEndian, uint16(r))
	}
	return sb.Bytes()
}

func biff12Le32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func biff12Float64(v float64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, math.Float64bits(v))
	return b
}

func zipAdd(t *testing.T, zw *zip.Writer, name string, data []byte) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
}

func buildSST(strs []string) []byte {
	var buf bytes.Buffer
	sstPayload := make([]byte, 8)
	binary.LittleEndian.PutUint32(sstPayload[0:], uint32(len(strs)))
	binary.LittleEndian.PutUint32(sstPayload[4:], uint32(len(strs)))
	biff12WriteRec(&buf, 0x019F, sstPayload)
	for _, s := range strs {
		payload := append([]byte{0x00}, biff12EncStr(s)...)
		biff12WriteRec(&buf, 0x0013, payload)
	}
	biff12WriteRec(&buf, 0x01A0, nil)
	return buf.Bytes()
}

func writeSheetRec(name, relID string, sheetID uint32) []byte {
	var rec bytes.Buffer
	rec.Write(biff12Le32(0))
	rec.Write(biff12Le32(sheetID))
	rec.Write(biff12EncStr(relID))
	rec.Write(biff12EncStr(name))
	return rec.Bytes()
}

func writeDimension(r1, r2, c1, c2 uint32) []byte {
	var dim bytes.Buffer
	dim.Write(biff12Le32(r1))
	dim.Write(biff12Le32(r2))
	dim.Write(biff12Le32(c1))
	dim.Write(biff12Le32(c2))
	return dim.Bytes()
}

func writeStringCell(col, sstIndex uint32) []byte {
	var cell bytes.Buffer
	cell.Write(biff12Le32(col))
	cell.Write(biff12Le32(0))
	cell.Write(biff12Le32(sstIndex))
	return cell.Bytes()
}

func writeFloatCell(col uint32, v float64) []byte {
	var cell bytes.Buffer
	cell.Write(biff12Le32(col))
	cell.Write(biff12Le32(0))
	cell.Write(biff12Float64(v))
	return cell.Bytes()
}

// buildSimpleXLSB mirrors testdata/xlsx/simple.xlsx (Sheet1 + Sheet2).
func buildSimpleXLSB(t *testing.T) []byte {
	t.Helper()

	sst := buildSST([]string{
		"id", "name", ".secret", "_skip", "alice", "hidden", "nope", "bob", "x",
	})

	var wb bytes.Buffer
	biff12WriteRec(&wb, 0x0183, nil)
	biff12WriteRec(&wb, 0x018F, nil)
	biff12WriteRec(&wb, 0x019C, writeSheetRec("Sheet1", "rId1", 1))
	biff12WriteRec(&wb, 0x019C, writeSheetRec("Sheet2", "rId2", 2))
	biff12WriteRec(&wb, 0x0190, nil)
	biff12WriteRec(&wb, 0x0184, nil)

	var sheet1 bytes.Buffer
	biff12WriteRec(&sheet1, 0x0181, nil)
	biff12WriteRec(&sheet1, 0x0194, writeDimension(0, 2, 0, 3))
	biff12WriteRec(&sheet1, 0x0191, nil)

	biff12WriteRec(&sheet1, 0x0000, biff12Le32(0))
	biff12WriteRec(&sheet1, 0x0007, writeStringCell(0, 0))
	biff12WriteRec(&sheet1, 0x0007, writeStringCell(1, 1))
	biff12WriteRec(&sheet1, 0x0007, writeStringCell(2, 2))
	biff12WriteRec(&sheet1, 0x0007, writeStringCell(3, 3))

	biff12WriteRec(&sheet1, 0x0000, biff12Le32(1))
	biff12WriteRec(&sheet1, 0x0005, writeFloatCell(0, 1))
	biff12WriteRec(&sheet1, 0x0007, writeStringCell(1, 4))
	biff12WriteRec(&sheet1, 0x0007, writeStringCell(2, 5))
	biff12WriteRec(&sheet1, 0x0007, writeStringCell(3, 6))

	biff12WriteRec(&sheet1, 0x0000, biff12Le32(2))
	biff12WriteRec(&sheet1, 0x0005, writeFloatCell(0, 2))
	biff12WriteRec(&sheet1, 0x0007, writeStringCell(1, 7))

	biff12WriteRec(&sheet1, 0x0192, nil)
	biff12WriteRec(&sheet1, 0x0182, nil)

	var sheet2 bytes.Buffer
	biff12WriteRec(&sheet2, 0x0181, nil)
	biff12WriteRec(&sheet2, 0x0194, writeDimension(0, 1, 0, 0))
	biff12WriteRec(&sheet2, 0x0191, nil)
	biff12WriteRec(&sheet2, 0x0000, biff12Le32(0))
	biff12WriteRec(&sheet2, 0x0007, writeStringCell(0, 8))
	biff12WriteRec(&sheet2, 0x0000, biff12Le32(1))
	biff12WriteRec(&sheet2, 0x0005, writeFloatCell(0, 9))
	biff12WriteRec(&sheet2, 0x0192, nil)
	biff12WriteRec(&sheet2, 0x0182, nil)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	rels := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="worksheet" Target="worksheets/sheet1.bin"/>` +
		`<Relationship Id="rId2" Type="worksheet" Target="worksheets/sheet2.bin"/>` +
		`</Relationships>`
	zipAdd(t, zw, "xl/_rels/workbook.bin.rels", []byte(rels))
	zipAdd(t, zw, "xl/workbook.bin", wb.Bytes())
	zipAdd(t, zw, "xl/sharedStrings.bin", sst)
	zipAdd(t, zw, "xl/worksheets/sheet1.bin", sheet1.Bytes())
	zipAdd(t, zw, "xl/worksheets/sheet2.bin", sheet2.Bytes())
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return zipBuf.Bytes()
}

func TestWriteSimpleXLSBFixture(t *testing.T) {
	if os.Getenv("SWL_WRITE_XLSB_FIXTURE") == "" {
		t.Skip("set SWL_WRITE_XLSB_FIXTURE=1 to regenerate testdata/xlsx/simple.xlsb")
	}
	data := buildSimpleXLSB(t)
	path := filepath.Join("..", "..", "testdata", "xlsx", "simple.xlsb")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSourceFixtureXLSB(t *testing.T) {
	path := xlsbFixturePath(t)
	snaps := collectSource(t, path, xlsx.SrcOpts{File: path})
	if len(snaps) != 2 {
		t.Fatalf("collections %d", len(snaps))
	}
	byName := indexSnapshots(snaps)
	if byName["Sheet1"].Cell(0, "name") != "alice" || byName["Sheet1"].Cell(1, "name") != "bob" {
		t.Fatalf("Sheet1 rows %+v", byName["Sheet1"].Rows)
	}
	if byName["Sheet2"].Cell(0, "x") != int64(9) {
		t.Fatalf("Sheet2 %+v", byName["Sheet2"].Rows[0])
	}
}

func xlsbFixturePath(t *testing.T) string {
	t.Helper()
	committed := filepath.Join("..", "..", "testdata", "xlsx", "simple.xlsb")
	if _, err := os.Stat(committed); err == nil {
		return committed
	}
	path := filepath.Join(t.TempDir(), "simple.xlsb")
	if err := os.WriteFile(path, buildSimpleXLSB(t), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
