package importer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

const ManifestName = "cppdefense-manifest.json"

type Limits struct {
	MaxCompressedBytes   int64
	MaxUncompressedBytes int64
	MaxFiles             int
	MaxRatio             int64
	MaxManifestBytes     int64
}

func DefaultLimits() Limits {
	return Limits{MaxCompressedBytes: 100 << 20, MaxUncompressedBytes: 500 << 20, MaxFiles: 10_000, MaxRatio: 100, MaxManifestBytes: 1 << 20}
}

type Manifest struct {
	SchemaVersion int     `json:"schema_version"`
	Kind          string  `json:"kind"`
	GroupCode     string  `json:"group_code"`
	LabCode       string  `json:"lab_code,omitempty"`
	Student       Student `json:"student,omitempty"`
	ProjectPath   string  `json:"project_path,omitempty"`
	Items         []Item  `json:"items,omitempty"`
}

type Student struct {
	GitHubLogin string `json:"github_login"`
}
type Item struct {
	LabCode     string  `json:"lab_code"`
	Student     Student `json:"student"`
	ProjectPath string  `json:"project_path"`
}

type Project struct {
	Item      Item
	Archive   []byte
	FileCount int
	Size      int64
}

type Analysis struct {
	Manifest     Manifest
	ManifestJSON []byte
	Projects     []Project
	TotalSize    int64
}

func Analyze(data []byte, expectedKind string, limits Limits) (Analysis, error) {
	if int64(len(data)) > limits.MaxCompressedBytes {
		return Analysis{}, errors.New("archive exceeds compressed size limit")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Analysis{}, fmt.Errorf("open ZIP: %w", err)
	}
	if len(reader.File) == 0 || len(reader.File) > limits.MaxFiles {
		return Analysis{}, errors.New("archive file count is outside limits")
	}
	files := make(map[string]*zip.File, len(reader.File))
	caseNames := make(map[string]string, len(reader.File))
	var total int64
	for _, file := range reader.File {
		name, err := safeName(file.Name)
		if err != nil {
			return Analysis{}, err
		}
		folded := strings.ToLower(name)
		if previous, exists := caseNames[folded]; exists && previous != name {
			return Analysis{}, fmt.Errorf("case-insensitive path collision: %q and %q", previous, name)
		}
		caseNames[folded] = name
		if file.Mode()&(^file.Mode().Perm()) != 0 && !file.FileInfo().IsDir() {
			return Analysis{}, fmt.Errorf("unsupported ZIP entry type: %q", name)
		}
		if !file.FileInfo().IsDir() {
			if strings.EqualFold(path.Ext(name), ".zip") {
				return Analysis{}, fmt.Errorf("nested ZIP is forbidden: %q", name)
			}
			if file.UncompressedSize64 > uint64(limits.MaxUncompressedBytes) {
				return Analysis{}, fmt.Errorf("entry is too large: %q", name)
			}
			total += int64(file.UncompressedSize64)
			if total > limits.MaxUncompressedBytes {
				return Analysis{}, errors.New("archive exceeds uncompressed size limit")
			}
			compressed := int64(file.CompressedSize64)
			if compressed == 0 {
				compressed = 1
			}
			if int64(file.UncompressedSize64)/compressed > limits.MaxRatio {
				return Analysis{}, fmt.Errorf("suspicious compression ratio: %q", name)
			}
			files[name] = file
		}
	}
	manifestFile := files[ManifestName]
	if manifestFile == nil || int64(manifestFile.UncompressedSize64) > limits.MaxManifestBytes {
		return Analysis{}, errors.New("valid root manifest is required")
	}
	manifestJSON, err := readFile(manifestFile, limits.MaxManifestBytes)
	if err != nil || !utf8.Valid(manifestJSON) {
		return Analysis{}, errors.New("manifest must be valid UTF-8 within size limit")
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(manifestJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Analysis{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Kind != expectedKind || (expectedKind != "group" && expectedKind != "lab") {
		return Analysis{}, errors.New("manifest version or kind mismatch")
	}
	items := manifest.Items
	if expectedKind == "lab" {
		items = []Item{{LabCode: manifest.LabCode, Student: manifest.Student, ProjectPath: manifest.ProjectPath}}
	}
	if len(items) == 0 || len(items) > 10_000 || !validCode(manifest.GroupCode) {
		return Analysis{}, errors.New("manifest group or item count is invalid")
	}
	seenProjects := map[string]bool{}
	seenAssignments := map[string]bool{}
	projects := make([]Project, 0, len(items))
	for _, item := range items {
		root, err := safeName(strings.TrimSuffix(item.ProjectPath, "/"))
		if err != nil || !validCode(item.LabCode) || !validLogin(item.Student.GitHubLogin) {
			return Analysis{}, errors.New("manifest item is invalid")
		}
		assignment := strings.ToLower(item.Student.GitHubLogin) + "\x00" + item.LabCode
		if seenProjects[strings.ToLower(root)] || seenAssignments[assignment] {
			return Analysis{}, errors.New("duplicate project or student/lab assignment")
		}
		for existing := range seenProjects {
			folded := strings.ToLower(root)
			if strings.HasPrefix(folded+"/", existing+"/") || strings.HasPrefix(existing+"/", folded+"/") {
				return Analysis{}, errors.New("overlapping project directories")
			}
		}
		seenProjects[strings.ToLower(root)] = true
		seenAssignments[assignment] = true
		project, err := normalizeProject(files, root, item)
		if err != nil {
			return Analysis{}, err
		}
		projects = append(projects, project)
	}
	return Analysis{Manifest: manifest, ManifestJSON: manifestJSON, Projects: projects, TotalSize: total}, nil
}

func normalizeProject(files map[string]*zip.File, root string, item Item) (Project, error) {
	prefix := root + "/"
	names := make([]string, 0)
	hasCMake, hasSource := false, false
	for name := range files {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(name, prefix)
		if rel == "" {
			continue
		}
		names = append(names, name)
		if rel == "CMakeLists.txt" {
			hasCMake = true
		}
		switch strings.ToLower(path.Ext(rel)) {
		case ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx":
			hasSource = true
		}
	}
	if !hasCMake || !hasSource {
		return Project{}, fmt.Errorf("project %q requires root CMakeLists.txt and C/C++ source", root)
	}
	sort.Strings(names)
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	var size int64
	for _, name := range names {
		rel := strings.TrimPrefix(name, prefix)
		header := &zip.FileHeader{Name: rel, Method: zip.Deflate}
		header.SetMode(0o600)
		destination, err := writer.CreateHeader(header)
		if err != nil {
			return Project{}, err
		}
		source, err := files[name].Open()
		if err != nil {
			return Project{}, err
		}
		written, copyErr := io.Copy(destination, source)
		closeErr := source.Close()
		if copyErr != nil {
			return Project{}, copyErr
		}
		if closeErr != nil {
			return Project{}, closeErr
		}
		size += written
	}
	if err := writer.Close(); err != nil {
		return Project{}, err
	}
	return Project{Item: item, Archive: output.Bytes(), FileCount: len(names), Size: size}, nil
}

func readFile(file *zip.File, max int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, max+1))
}

func safeName(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.ContainsRune(name, 0) || !utf8.ValidString(name) {
		return "", fmt.Errorf("unsafe ZIP path %q", name)
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != strings.TrimSuffix(name, "/") {
		return "", fmt.Errorf("unsafe ZIP path %q", name)
	}
	return cleaned, nil
}

func validCode(value string) bool {
	if len(value) < 1 || len(value) > 64 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '-' {
			return false
		}
	}
	return true
}

func validLogin(value string) bool {
	if len(value) < 1 || len(value) > 39 || value[0] == '-' || value[len(value)-1] == '-' || strings.Contains(value, "--") {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '-' {
			return false
		}
	}
	return true
}
