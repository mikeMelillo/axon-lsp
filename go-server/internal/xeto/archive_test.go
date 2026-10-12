package xeto

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

func archiveBytes(t *testing.T, names, contents []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for i, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(contents[i])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseArchive(t *testing.T) {
	data := archiveBytes(t, []string{"xeto.lib/", "xeto.lib/lib.xeto", "xeto.lib/specs.xeto", "resource.txt"}, []string{"", "pragma: Lib", "Equip: Dict", "ignored"})
	entries, err := ParseArchive(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(entries) != 2 || entries["xeto.lib/specs.xeto"] != "Equip: Dict" {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
}

func TestParseArchiveRejectsInvalidInputs(t *testing.T) {
	for _, name := range []string{"../specs.xeto", "/specs.xeto", "a/../specs.xeto", "a//specs.xeto", "./specs.xeto", "C:/specs.xeto", "a\\specs.xeto", "a\x00.xeto", ".."} {
		t.Run(name, func(t *testing.T) {
			data := archiveBytes(t, []string{"good.xeto", name}, []string{"Good: Dict", "Bad: Dict"})
			if entries, err := ParseArchive(bytes.NewReader(data), int64(len(data))); err == nil || entries != nil {
				t.Fatalf("accepted unsafe path: %v, %v", entries, err)
			}
		})
	}
	for _, data := range [][]byte{[]byte("not zip"), archiveBytes(t, []string{"a.xeto", "a.xeto"}, []string{"", ""})} {
		if entries, err := ParseArchive(bytes.NewReader(data), int64(len(data))); err == nil || entries != nil {
			t.Fatalf("accepted malformed archive: %v", err)
		}
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "link.xeto"}
	h.SetMode(os.ModeSymlink | 0777)
	f, err := w.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("target"))
	w.Close()
	if _, err := ParseArchive(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err == nil {
		t.Fatal("accepted symlink")
	}
}

func TestParseArchiveLimits(t *testing.T) {
	t.Run("input", func(t *testing.T) {
		if _, err := ParseArchive(bytes.NewReader(nil), MaxArchiveBytes+1); err == nil {
			t.Fatal("accepted oversized input")
		}
	})
	t.Run("entries", func(t *testing.T) {
		names, contents := make([]string, MaxArchiveEntries+1), make([]string, MaxArchiveEntries+1)
		for i := range names {
			names[i] = strconv.Itoa(i)
		}
		data := archiveBytes(t, names, contents)
		end := len(data) - 22
		for _, declared := range []uint16{MaxArchiveEntries + 1, 0, 1, MaxArchiveEntries} {
			forged := append([]byte(nil), data...)
			binary.LittleEndian.PutUint16(forged[end+8:], declared)
			binary.LittleEndian.PutUint16(forged[end+10:], declared)
			if entries, err := ParseArchive(bytes.NewReader(forged), int64(len(forged))); err == nil || entries != nil || !strings.Contains(err.Error(), "too many archive entries") {
				t.Fatalf("did not preflight real entry count with declared=%d: %v", declared, err)
			}
		}
		boundary := archiveBytes(t, names[:MaxArchiveEntries], contents[:MaxArchiveEntries])
		if _, err := ParseArchive(bytes.NewReader(boundary), int64(len(boundary))); err != nil {
			t.Fatalf("rejected exact entry limit: %v", err)
		}
	})
	t.Run("entry bytes", func(t *testing.T) {
		data := archiveBytes(t, []string{"ignored.bin"}, []string{strings.Repeat("x", MaxArchiveEntryBytes+1)})
		if _, err := ParseArchive(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Fatal("accepted oversized resource")
		}
	})
	t.Run("total bytes", func(t *testing.T) {
		names := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}
		contents := make([]string, len(names))
		for i := range contents {
			contents[i] = strings.Repeat("x", MaxArchiveEntryBytes)
		}
		boundary := archiveBytes(t, names[:8], contents[:8])
		if _, err := ParseArchive(bytes.NewReader(boundary), int64(len(boundary))); err != nil {
			t.Fatalf("rejected exact entry/total limits: %v", err)
		}
		data := archiveBytes(t, names, contents)
		if _, err := ParseArchive(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Fatal("accepted excessive decompression")
		}
	})
}

type archiveReadCounter struct {
	io.ReaderAt
	bytesRead int
}

func (r *archiveReadCounter) ReadAt(data []byte, offset int64) (int, error) {
	n, err := r.ReaderAt.ReadAt(data, offset)
	r.bytesRead += n
	return n, err
}

func TestParseArchiveCentralDirectoryByteLimit(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	comment := strings.Repeat("c", 65535)
	for i := 0; i < 65; i++ {
		if _, err := w.CreateHeader(&zip.FileHeader{Name: strconv.Itoa(i), Comment: comment}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r := &archiveReadCounter{ReaderAt: bytes.NewReader(buf.Bytes())}
	if entries, err := ParseArchive(r, int64(buf.Len())); err == nil || entries != nil || !strings.Contains(err.Error(), "central directory byte limit") {
		t.Fatalf("did not reject oversized metadata: %v", err)
	}
	if r.bytesRead > 22+65535+20 {
		t.Fatalf("read directory before checking metadata budget: %d bytes", r.bytesRead)
	}
	buf.Reset()
	w = zip.NewWriter(&buf)
	directoryBytes := 0
	for i := 0; i < 64; i++ {
		name := strconv.Itoa(i)
		commentBytes := min(len(comment), MaxArchiveDirectoryBytes-directoryBytes-46-len(name))
		if _, err := w.CreateHeader(&zip.FileHeader{Name: name, Comment: comment[:commentBytes]}); err != nil {
			t.Fatal(err)
		}
		directoryBytes += 46 + len(name) + commentBytes
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if directoryBytes != MaxArchiveDirectoryBytes {
		t.Fatal("fixture did not reach metadata limit")
	}
	if _, err := ParseArchive(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatalf("rejected exact metadata limit: %v", err)
	}
}

func TestParseArchiveRejectsForgedDirectoryMetadata(t *testing.T) {
	data := archiveBytes(t, []string{"specs.xeto"}, []string{"Equip: Dict"})
	end := len(data) - 22
	central := int(binary.LittleEndian.Uint32(data[end+16:]))
	cases := []struct {
		name   string
		mutate func([]byte)
		want   string
	}{
		{"count mismatch", func(b []byte) {
			binary.LittleEndian.PutUint16(b[end+8:], 0)
			binary.LittleEndian.PutUint16(b[end+10:], 0)
		}, "entry count mismatch"},
		{"forged small directory", func(b []byte) { binary.LittleEndian.PutUint32(b[end+12:], 0) }, "directory range"},
		{"forged offset", func(b []byte) { binary.LittleEndian.PutUint32(b[end+16:], uint32(central+1)) }, "directory range"},
		{"recovered offset", func(b []byte) { binary.LittleEndian.PutUint32(b[end+16:], uint32(central-1)) }, "directory range"},
		{"oversized metadata declaration", func(b []byte) { binary.LittleEndian.PutUint32(b[end+12:], MaxArchiveDirectoryBytes+1) }, "central directory byte limit"},
		{"truncated record", func(b []byte) { binary.LittleEndian.PutUint16(b[central+28:], 65535) }, "zip: not a valid zip file"},
		{"bad record signature", func(b []byte) { b[central] = 0 }, "zip: not a valid zip file"},
		{"multiple disks", func(b []byte) { binary.LittleEndian.PutUint16(b[end+4:], 1) }, "multi-disk"},
		{"entry on another disk", func(b []byte) { binary.LittleEndian.PutUint16(b[central+34:], 1) }, "multi-disk"},
		{"ZIP64 count", func(b []byte) {
			binary.LittleEndian.PutUint16(b[end+8:], 0xffff)
			binary.LittleEndian.PutUint16(b[end+10:], 0xffff)
		}, "ZIP64"},
		{"ZIP64 size", func(b []byte) { binary.LittleEndian.PutUint32(b[end+12:], 0xffffffff) }, "ZIP64"},
		{"ZIP64 offset", func(b []byte) { binary.LittleEndian.PutUint32(b[end+16:], 0xffffffff) }, "ZIP64"},
		{"ZIP64 entry", func(b []byte) { binary.LittleEndian.PutUint32(b[central+24:], 0xffffffff) }, "ZIP64"},
		{"ZIP64 locator", func(b []byte) { binary.LittleEndian.PutUint32(b[end-20:], 0x07064b50) }, "ZIP64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := append([]byte(nil), data...)
			tc.mutate(broken)
			if entries, err := ParseArchive(bytes.NewReader(broken), int64(len(broken))); err == nil || entries != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
	if entries, err := ParseArchive(bytes.NewReader(append(data, 0)), int64(len(data)+1)); err == nil || entries != nil {
		t.Fatal("accepted trailing bytes after EOCD")
	}
}

func TestParseArchiveOrdinaryZIPMetadata(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	if err := w.SetComment(strings.Repeat("c", 65535)); err != nil {
		t.Fatal(err)
	}
	f, err := w.CreateHeader(&zip.FileHeader{Name: "xeto.lib/specs.xeto", Comment: "entry comment", Extra: []byte{0xfe, 0xca, 2, 0, 1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("Equip: Dict")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if entries, err := ParseArchive(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil || entries["xeto.lib/specs.xeto"] != "Equip: Dict" {
		t.Fatalf("ordinary ZIP metadata rejected: %v", err)
	}
	empty := archiveBytes(t, nil, nil)
	if entries, err := ParseArchive(bytes.NewReader(empty), int64(len(empty))); err != nil || len(entries) != 0 {
		t.Fatalf("empty ordinary ZIP rejected: %v", err)
	}
}

func TestParseArchiveRejectsCorruption(t *testing.T) {
	data := archiveBytes(t, []string{"specs.xeto"}, []string{"Equip: Dict"})
	central := bytes.Index(data, []byte{'P', 'K', 1, 2})
	for _, offset := range []int{10, 16, 24} { // Compression method, CRC, and uncompressed size.
		broken := append([]byte(nil), data...)
		binary.LittleEndian.PutUint32(broken[central+offset:], 1)
		if entries, err := ParseArchive(bytes.NewReader(broken), int64(len(broken))); err == nil || entries != nil {
			t.Fatalf("accepted corrupt metadata at %d", offset)
		}
	}
	if _, err := ParseArchive(bytes.NewReader(data[:len(data)-10]), int64(len(data)-10)); err == nil {
		t.Fatal("accepted truncated archive")
	}
}
