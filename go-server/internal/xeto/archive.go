package xeto

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"path"
	"strings"
)

const (
	MaxArchiveBytes          = 128 << 20
	MaxArchiveEntries        = 4096
	MaxArchiveEntryBytes     = 8 << 20
	MaxArchiveTotalBytes     = 64 << 20
	MaxArchiveDirectoryBytes = 4 << 20
)

// ParseArchive reads Xeto text without extracting files. All entries, including
// non-Xeto resources, are validated and charged against decompression limits.
// A rejected archive returns no partial contents.
// Only ordinary single-disk ZIPs with a contiguous central directory are
// supported; ZIP64 and layouts requiring offset recovery are rejected.
func ParseArchive(r io.ReaderAt, size int64) (map[string]string, error) {
	if size < 0 || size > MaxArchiveBytes {
		return nil, fmt.Errorf("archive input size limit exceeded")
	}
	if err := validateArchiveDirectory(r, size); err != nil {
		return nil, err
	}
	z, err := zip.NewReader(r, size)
	if err != nil {
		return nil, err
	}
	if len(z.File) > MaxArchiveEntries {
		return nil, fmt.Errorf("too many archive entries")
	}
	entries := map[string]string{}
	seen := map[string]bool{}
	var total uint64
	for _, f := range z.File {
		name := strings.TrimSuffix(f.Name, "/")
		if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\:\x00") || name == ".." || strings.HasPrefix(name, "../") || seen[name] {
			return nil, fmt.Errorf("unsafe or duplicate archive path: %q", f.Name)
		}
		seen[name] = true
		if !f.Mode().IsRegular() && !f.Mode().IsDir() {
			return nil, fmt.Errorf("unsupported archive entry: %q", f.Name)
		}
		if f.UncompressedSize64 > MaxArchiveEntryBytes || f.UncompressedSize64 > MaxArchiveTotalBytes-total {
			return nil, fmt.Errorf("archive decompression limit exceeded")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, MaxArchiveEntryBytes+1))
		closeErr := rc.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		total += uint64(len(data))
		if len(data) > MaxArchiveEntryBytes || total > MaxArchiveTotalBytes || uint64(len(data)) != f.UncompressedSize64 {
			return nil, fmt.Errorf("archive decompression limit or size mismatch")
		}
		if !f.Mode().IsDir() && strings.HasSuffix(name, ".xeto") {
			entries[name] = string(data)
		}
	}
	return entries, nil
}

// Bound metadata before archive/zip allocates File objects. Require exact
// directory ranges so its offset-recovery logic cannot select another range.
func validateArchiveDirectory(r io.ReaderAt, size int64) error {
	if size < 22 {
		return zip.ErrFormat
	}
	tail := make([]byte, min(size, int64(22+65535)))
	if _, err := r.ReadAt(tail, size-int64(len(tail))); err != nil {
		return err
	}
	end := -1
	for i := len(tail) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(tail[i:]) == 0x06054b50 {
			end = i
			break
		}
	}
	if end < 0 || end+22+int(binary.LittleEndian.Uint16(tail[end+20:])) != len(tail) {
		return zip.ErrFormat
	}
	eocd := tail[end:]
	eocdOffset := size - int64(len(tail)) + int64(end)
	if eocdOffset >= 20 {
		var locator [20]byte
		if _, err := r.ReadAt(locator[:], eocdOffset-20); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(locator[:]) == 0x07064b50 {
			return fmt.Errorf("ZIP64 archives are unsupported")
		}
	}
	count := int(binary.LittleEndian.Uint16(eocd[10:]))
	directoryBytes := int64(binary.LittleEndian.Uint32(eocd[12:]))
	directoryOffset := int64(binary.LittleEndian.Uint32(eocd[16:]))
	if count == 0xffff || binary.LittleEndian.Uint16(eocd[8:]) == 0xffff || directoryBytes == 0xffffffff || directoryOffset == 0xffffffff {
		return fmt.Errorf("ZIP64 archives are unsupported")
	}
	if binary.LittleEndian.Uint16(eocd[4:]) != 0 || binary.LittleEndian.Uint16(eocd[6:]) != 0 || int(binary.LittleEndian.Uint16(eocd[8:])) != count {
		return fmt.Errorf("multi-disk or inconsistent ZIP directory")
	}
	if directoryBytes > MaxArchiveDirectoryBytes {
		return fmt.Errorf("archive central directory byte limit exceeded")
	}
	if directoryOffset+directoryBytes != eocdOffset {
		return fmt.Errorf("invalid or unsupported ZIP directory range")
	}
	directory := make([]byte, directoryBytes)
	if _, err := r.ReadAt(directory, directoryOffset); err != nil {
		return err
	}
	records := 0
	for len(directory) > 0 {
		if len(directory) < 46 || binary.LittleEndian.Uint32(directory) != 0x02014b50 {
			return zip.ErrFormat
		}
		records++
		if records > MaxArchiveEntries {
			return fmt.Errorf("too many archive entries")
		}
		if binary.LittleEndian.Uint32(directory[20:]) == 0xffffffff || binary.LittleEndian.Uint32(directory[24:]) == 0xffffffff || binary.LittleEndian.Uint32(directory[42:]) == 0xffffffff || binary.LittleEndian.Uint16(directory[34:]) == 0xffff {
			return fmt.Errorf("ZIP64 entries are unsupported")
		}
		if binary.LittleEndian.Uint16(directory[34:]) != 0 {
			return fmt.Errorf("multi-disk ZIP entries are unsupported")
		}
		recordBytes := 46 + int(binary.LittleEndian.Uint16(directory[28:])) + int(binary.LittleEndian.Uint16(directory[30:])) + int(binary.LittleEndian.Uint16(directory[32:]))
		if recordBytes > len(directory) {
			return zip.ErrFormat
		}
		directory = directory[recordBytes:]
	}
	if records != count {
		return fmt.Errorf("ZIP directory entry count mismatch")
	}
	return nil
}
