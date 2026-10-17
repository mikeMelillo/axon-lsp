package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type sourcePin struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Model           string   `json:"model"`
	Version         string   `json:"version"`
	Path            string   `json:"path"`
	Repository      string   `json:"repository,omitempty"`
	Revision        string   `json:"revision,omitempty"`
	AssetPrefix     string   `json:"asset_prefix,omitempty"`
	License         string   `json:"license,omitempty"`
	LicenseFile     string   `json:"license_file,omitempty"`
	PropertyFiles   []string `json:"property_files,omitempty"`
	VersionProperty string   `json:"version_property,omitempty"`
	RequiredFiles   []string `json:"required_files,omitempty"`
}

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func loadSources(repoRoot string) ([]sourceDescriptor, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, "cache_sources", "baseline.lock.json"))
	if err != nil {
		return nil, err
	}
	var lock struct {
		SchemaVersion int         `json:"schema_version"`
		Sources       []sourcePin `json:"sources"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&lock); err != nil {
		return nil, fmt.Errorf("decode baseline lock: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("baseline lock must contain one JSON object")
	}
	if lock.SchemaVersion != 1 || len(lock.Sources) == 0 {
		return nil, fmt.Errorf("unsupported or empty baseline lock")
	}
	ids, prefixes := map[string]bool{}, map[string]bool{}
	var sources []sourceDescriptor
	for _, pin := range lock.Sources {
		if pin.ID == "" || ids[pin.ID] || pin.Kind == "" || pin.Version == "" || (pin.Model != "defs" && pin.Model != "specs") || !safeRelativePath(pin.Path) {
			return nil, fmt.Errorf("invalid or duplicate baseline source %q", pin.ID)
		}
		ids[pin.ID] = true
		source, err := loadSource(repoRoot, pin)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", pin.ID, err)
		}
		if source.AssetPrefix != "" {
			for prefix := range prefixes {
				if source.AssetPrefix == prefix || strings.HasPrefix(source.AssetPrefix, prefix+"/") || strings.HasPrefix(prefix, source.AssetPrefix+"/") {
					return nil, fmt.Errorf("overlapping source asset prefixes: %s and %s", prefix, source.AssetPrefix)
				}
			}
			prefixes[source.AssetPrefix] = true
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func loadSource(repoRoot string, pin sourcePin) (sourceDescriptor, error) {
	root := filepath.Join(repoRoot, "cache_sources", filepath.FromSlash(pin.Path))
	source := sourceDescriptor{
		ID: pin.ID, Kind: pin.Kind, Model: pin.Model, Version: pin.Version,
		Root: root, Repository: pin.Repository, Revision: pin.Revision,
		AssetPrefix: pin.AssetPrefix, License: pin.License, LicenseFile: pin.LicenseFile,
		PropertyFiles: pin.PropertyFiles, Properties: map[string]string{},
	}
	if pin.Repository == "" {
		if !strings.HasSuffix(pin.Path, ".trio") || pin.Revision != "" || pin.AssetPrefix != "" || len(pin.PropertyFiles) != 0 {
			return source, fmt.Errorf("local baseline input must be a Trio file")
		}
		if _, err := readSourceFile(root); err != nil {
			return source, err
		}
		source.CoreTrioFile, source.Root = root, filepath.Dir(root)
		return source, nil
	}
	if !strings.HasPrefix(pin.Repository, "https://github.com/") || !revisionPattern.MatchString(pin.Revision) || !safeRelativePath(pin.AssetPrefix) || pin.License == "" || !safeRelativePath(pin.LicenseFile) {
		return source, fmt.Errorf("repository sources require an immutable revision, asset prefix, and license")
	}
	// Never use a parent repository as a substitute for a missing nested clone.
	top, err := gitOutput(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return source, err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return source, err
	}
	realTop, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil || filepath.Clean(realRoot) != filepath.Clean(realTop) {
		return source, fmt.Errorf("expected a dedicated clone at %s", root)
	}
	head, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return source, err
	}
	if strings.TrimSpace(string(head)) != pin.Revision {
		return source, fmt.Errorf("revision does not match lock (want %s, got %s); check out the pinned revision or intentionally update the lock", pin.Revision, strings.TrimSpace(string(head)))
	}
	status, err := gitOutput(root, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return source, err
	}
	if len(bytes.TrimSpace(status)) != 0 {
		return source, fmt.Errorf("checkout has local changes; refusing to label modified sources as revision %s", pin.Revision)
	}
	files, err := gitOutput(root, "ls-tree", "-r", "-z", pin.Revision)
	if err != nil {
		return source, err
	}
	tracked := map[string]bool{}
	source.blobs = map[string]string{}
	for _, record := range strings.Split(strings.TrimSuffix(string(files), "\x00"), "\x00") {
		metadata, file, found := strings.Cut(record, "\t")
		fields := strings.Fields(metadata)
		if !found || len(fields) != 3 || fields[1] != "blob" || !revisionPattern.MatchString(fields[2]) || !safeRelativePath(file) {
			return source, fmt.Errorf("unsafe or unsupported tracked source entry %q", record)
		}
		tracked[file] = true
		source.blobs[file] = fields[2]
		source.files = append(source.files, file)
	}
	required := append([]string{pin.LicenseFile}, pin.PropertyFiles...)
	required = append(required, pin.RequiredFiles...)
	for _, file := range required {
		if !safeRelativePath(file) || !tracked[file] {
			return source, fmt.Errorf("required input is missing or untracked: %s", file)
		}
		if _, err := source.readFile(file); err != nil {
			return source, err
		}
	}
	for _, file := range pin.PropertyFiles {
		if !strings.HasPrefix(file, "src/xeto/") {
			return source, fmt.Errorf("build properties must be under src/xeto: %s", file)
		}
		data, err := source.readFile(file)
		if err != nil {
			return source, err
		}
		for i, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, found := strings.Cut(line, "=")
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if !found || key == "" {
				return source, fmt.Errorf("unsupported build property at %s:%d", file, i+1)
			}
			if previous, found := source.Properties[key]; found && previous != value {
				return source, fmt.Errorf("conflicting build property %s", key)
			}
			source.Properties[key] = value
		}
	}
	if pin.VersionProperty != "" && source.Properties[pin.VersionProperty] != pin.Version {
		return source, fmt.Errorf("build property %s does not match locked version %s", pin.VersionProperty, pin.Version)
	}
	source.GitHubBase = strings.TrimRight(pin.Repository, "/") + "/blob/" + pin.Revision
	return source, nil
}

func safeRelativePath(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.HasPrefix(value, "../") && !strings.ContainsAny(value, "\\:\x00") &&
		!filepath.IsAbs(value) && filepath.ToSlash(filepath.Clean(filepath.FromSlash(value))) == value
}

func readSourceFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source must be a regular file, not a symlink or directory: %s", path)
	}
	return os.ReadFile(path)
}

func (source sourceDescriptor) readFile(rel string) ([]byte, error) {
	data, err := readSourceFile(filepath.Join(source.Root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	if source.blobs != nil {
		// Git can report a clean checkout after CRLF/smudge transformations. Do
		// not publish different bytes under the same pinned source revision.
		hash := sha1.New()
		fmt.Fprintf(hash, "blob %d\x00", len(data))
		hash.Write(data)
		if fmt.Sprintf("%x", hash.Sum(nil)) != source.blobs[rel] {
			return nil, fmt.Errorf("checkout filters or line endings changed source bytes: %s; use an unfiltered checkout of the locked revision", rel)
		}
	}
	return data, nil
}

func gitOutput(root string, args ...string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
