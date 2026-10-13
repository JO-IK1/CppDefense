package importer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
)

var (
	deadlinePattern  = regexp.MustCompile(`^deadline_[0-9]+$`)
	commitSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}(?:[0-9a-f]{24})?$`)
)

type ProjectArchiveMetadata struct {
	GroupCode       string
	LabCode         string
	GitHubLogin     string
	VersionLabel    string
	Source          string
	SourceReference string
	CommitSHA       string
}

// PrepareStudentArchive accepts an ordinary GitHub/project ZIP, infers its
// metadata from the authenticated account and filename, and converts it to the
// same canonical archive format used by reviewed teacher imports.
func PrepareStudentArchive(data []byte, filename, githubLogin, groupCode string, limits Limits) ([]byte, Analysis, error) {
	labCode, versionLabel, err := studentArchiveName(filename, githubLogin)
	if err != nil {
		return nil, Analysis{}, err
	}
	return prepareProjectArchive(data, ProjectArchiveMetadata{
		GroupCode:       groupCode,
		LabCode:         labCode,
		GitHubLogin:     githubLogin,
		VersionLabel:    versionLabel,
		Source:          "student_zip",
		SourceReference: path.Base(filename),
	}, limits)
}

// PrepareGitHubArchive converts a GitHub repository zipball downloaded in the
// teacher's browser into the same canonical archive used by every other import.
// The access token is deliberately not part of this API or its metadata.
func PrepareGitHubArchive(data []byte, metadata ProjectArchiveMetadata, limits Limits) ([]byte, Analysis, error) {
	metadata.Source = "teacher_import"
	return prepareProjectArchive(data, metadata, limits)
}

func prepareProjectArchive(data []byte, metadata ProjectArchiveMetadata, limits Limits) ([]byte, Analysis, error) {
	if !validCode(metadata.GroupCode) {
		return nil, Analysis{}, errors.New("для аккаунта не найдена корректная учебная группа")
	}
	if !validCode(metadata.LabCode) || !deadlinePattern.MatchString(metadata.VersionLabel) {
		return nil, Analysis{}, errors.New("некорректны код лабораторной или ветка deadline_X")
	}
	if strings.TrimSpace(metadata.GitHubLogin) == "" || len(metadata.GitHubLogin) > 100 {
		return nil, Analysis{}, errors.New("некорректный GitHub-логин студента")
	}
	if metadata.Source != "student_zip" && metadata.Source != "teacher_import" {
		return nil, Analysis{}, errors.New("некорректный источник архива")
	}
	if len(metadata.SourceReference) == 0 || len(metadata.SourceReference) > 255 || strings.ContainsAny(metadata.SourceReference, "\x00\r\n") {
		return nil, Analysis{}, errors.New("некорректная ссылка на источник")
	}
	if metadata.CommitSHA != "" && !commitSHAPattern.MatchString(metadata.CommitSHA) {
		return nil, Analysis{}, errors.New("некорректный SHA коммита")
	}
	if int64(len(data)) > limits.MaxCompressedBytes {
		return nil, Analysis{}, errors.New("архив превышает допустимый сжатый размер")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, Analysis{}, fmt.Errorf("не удалось открыть ZIP: %w", err)
	}
	if len(reader.File) == 0 || len(reader.File) > limits.MaxFiles {
		return nil, Analysis{}, errors.New("недопустимое количество файлов в архиве")
	}
	files := make(map[string]*zip.File, len(reader.File))
	caseNames := make(map[string]string, len(reader.File))
	cmakeCandidates := make([]string, 0, 2)
	var total int64
	for _, file := range reader.File {
		name, nameErr := safeName(file.Name)
		if nameErr != nil {
			return nil, Analysis{}, nameErr
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if file.Mode()&(^file.Mode().Perm()) != 0 {
			return nil, Analysis{}, fmt.Errorf("неподдерживаемый тип ZIP-записи: %q", name)
		}
		folded := strings.ToLower(name)
		if previous, exists := caseNames[folded]; exists && previous != name {
			return nil, Analysis{}, fmt.Errorf("пути отличаются только регистром: %q и %q", previous, name)
		}
		caseNames[folded] = name
		if strings.EqualFold(path.Ext(name), ".zip") {
			return nil, Analysis{}, fmt.Errorf("вложенный ZIP запрещён: %q", name)
		}
		if file.UncompressedSize64 > uint64(limits.MaxUncompressedBytes) {
			return nil, Analysis{}, fmt.Errorf("файл слишком большой: %q", name)
		}
		total += int64(file.UncompressedSize64)
		if total > limits.MaxUncompressedBytes {
			return nil, Analysis{}, errors.New("распакованный архив превышает допустимый размер")
		}
		compressed := int64(file.CompressedSize64)
		if compressed == 0 {
			compressed = 1
		}
		if int64(file.UncompressedSize64)/compressed > limits.MaxRatio {
			return nil, Analysis{}, fmt.Errorf("подозрительная степень сжатия файла: %q", name)
		}
		files[name] = file
		if path.Base(name) == "CMakeLists.txt" {
			cmakeCandidates = append(cmakeCandidates, name)
		}
	}
	if len(cmakeCandidates) == 0 {
		return nil, Analysis{}, errors.New("в архиве не найден корневой CMakeLists.txt")
	}
	sort.Slice(cmakeCandidates, func(i, j int) bool {
		return strings.Count(cmakeCandidates[i], "/") < strings.Count(cmakeCandidates[j], "/")
	})
	depth := strings.Count(cmakeCandidates[0], "/")
	if len(cmakeCandidates) > 1 && strings.Count(cmakeCandidates[1], "/") == depth {
		return nil, Analysis{}, errors.New("в архиве найдено несколько возможных корней проекта")
	}
	rootPrefix := strings.TrimSuffix(cmakeCandidates[0], "CMakeLists.txt")
	manifest := Manifest{
		SchemaVersion:   1,
		Kind:            "lab",
		GroupCode:       metadata.GroupCode,
		LabCode:         metadata.LabCode,
		Student:         Student{GitHubLogin: metadata.GitHubLogin},
		ProjectPath:     "project",
		VersionLabel:    metadata.VersionLabel,
		Source:          metadata.Source,
		SourceReference: metadata.SourceReference,
		CommitSHA:       metadata.CommitSHA,
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, Analysis{}, err
	}
	var canonical bytes.Buffer
	writer := zip.NewWriter(&canonical)
	manifestWriter, err := writer.Create(ManifestName)
	if err != nil {
		return nil, Analysis{}, err
	}
	if _, err = manifestWriter.Write(manifestJSON); err != nil {
		return nil, Analysis{}, err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		if strings.HasPrefix(name, rootPrefix) && name != ManifestName {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		relative := strings.TrimPrefix(name, rootPrefix)
		if relative == "" || strings.HasPrefix(relative, ".git/") || strings.HasPrefix(relative, "__MACOSX/") {
			continue
		}
		header := &zip.FileHeader{Name: "project/" + relative, Method: zip.Deflate}
		header.SetMode(0o600)
		destination, createErr := writer.CreateHeader(header)
		if createErr != nil {
			return nil, Analysis{}, createErr
		}
		source, openErr := files[name].Open()
		if openErr != nil {
			return nil, Analysis{}, openErr
		}
		_, copyErr := io.Copy(destination, source)
		closeErr := source.Close()
		if copyErr != nil {
			return nil, Analysis{}, copyErr
		}
		if closeErr != nil {
			return nil, Analysis{}, closeErr
		}
	}
	if err = writer.Close(); err != nil {
		return nil, Analysis{}, err
	}
	analysis, err := Analyze(canonical.Bytes(), "lab", limits)
	if err != nil {
		return nil, Analysis{}, err
	}
	return canonical.Bytes(), analysis, nil
}

func studentArchiveName(filename, githubLogin string) (string, string, error) {
	base := path.Base(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/"))
	if len(base) == 0 || len(base) > 255 || strings.ContainsAny(base, "\x00\r\n") {
		return "", "", errors.New("имя архива некорректно или длиннее 255 символов")
	}
	if !strings.HasSuffix(strings.ToLower(base), ".zip") {
		return "", "", errors.New("файл должен иметь расширение .zip")
	}
	stem := base[:len(base)-4]
	lowerStem := strings.ToLower(stem)
	marker := "-" + strings.ToLower(githubLogin) + "-deadline_"
	position := strings.LastIndex(lowerStem, marker)
	if position <= 0 {
		return "", "", fmt.Errorf("имя архива должно иметь вид labwork1-%s-deadline_0.zip", githubLogin)
	}
	labCode := lowerStem[:position]
	versionLabel := lowerStem[position+1+len(githubLogin)+1:]
	if !validCode(labCode) || !deadlinePattern.MatchString(versionLabel) {
		return "", "", errors.New("в имени архива некорректны код лабораторной или deadline_X")
	}
	return labCode, versionLabel, nil
}
