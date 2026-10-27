package runner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/JO-IK1/CppDefense/backend/internal/application/hideseek"
	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func hideSources(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	total := 0
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, e := filepath.Rel(root, file)
		if e != nil {
			return e
		}
		relative = filepath.ToSlash(relative)
		if !hideseek.Editable(relative) {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link in project")
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if info.Size() > 2<<20 {
			return nil
		}
		data, e := os.ReadFile(file)
		if e != nil {
			return e
		}
		if !utf8.Valid(data) {
			return nil
		}
		total += len(data)
		if total > 64<<20 {
			return fmt.Errorf("слишком много исходного кода для режима Прятки")
		}
		files[relative] = data
		return nil
	})
	return files, err
}

func writeHideFiles(root string, files map[string][]byte) error {
	for name, data := range files {
		if !hideseek.Editable(name) {
			return fmt.Errorf("protected path")
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (a *agent) validateHide(parent, ctx context.Context, project string, lease postgres.Lease) error {
	report := &postgres.HideValidation{}
	finish := func(message string) error {
		report.Message = message
		return a.complete(parent, lease, postgres.Completion{Outcome: "error", HideValidation: report})
	}
	original, err := hideSources(project)
	if err != nil {
		return finish(err.Error())
	}
	changed, err := hideseek.Apply(lease.HideMutations, original)
	if err != nil {
		return finish(err.Error())
	}
	image, policy, envErr := a.referenceEnvironment(ctx)
	if envErr != nil {
		image = a.config.Image
	}
	run := func() sandboxResults {
		if err := makeSandboxReadable(project); err != nil {
			failure := commandResult{ExitCode: -1, Log: err.Error()}
			return sandboxResults{Configure: failure, Build: failure, CTest: failure}
		}
		defer restoreSandboxOwnerAccess(project)
		return runSandboxCached(ctx, a.config.Runtime, image, project, lease.SessionID)
	}
	baseline := run()
	report.Baseline = baseline
	if !baseline.OK() {
		return finish("Исходная лабораторная не проходит сборку и тесты. Исправьте исходную работу до подготовки задания.")
	}
	if err = writeHideFiles(project, changed); err != nil {
		return finish(err.Error())
	}
	mutated := run()
	report.Mutated = mutated
	if !mutated.Configure.OK || !mutated.Build.OK {
		return finish("Изменения ломают сборку. Для Пряток нужны только логические ошибки.")
	}
	if mutated.CTest.TimedOut || mutated.CTest.ExitCode != 8 {
		return finish("Изменения должны приводить к обычному провалу тестов, без таймаута или сбоя инфраструктуры.")
	}
	if err = writeHideFiles(project, original); err != nil {
		return finish(err.Error())
	}
	restored := run()
	report.Restored = restored
	if !restored.OK() {
		return finish("Возврат исходного кода не восстановил успешные тесты. Проверьте стабильность тестовой среды.")
	}
	if envErr == nil {
		a.rememberReference(project, lease, image, policy)
	}
	report.Valid = true
	report.Message = "Задание проверено: оригинал проходит тесты, изменённый проект собирается и проваливает тесты."
	return a.complete(parent, lease, postgres.Completion{HideValidation: report})
}

func (a *agent) checkHide(parent, ctx context.Context, project string, lease postgres.Lease) error {
	failure := func(err error) error {
		result := commandResult{ExitCode: -1, Log: err.Error()}
		return a.complete(parent, lease, postgres.Completion{Outcome: "error", ConfigureResult: result, BuildResult: result, CTestResult: result})
	}
	original, err := hideSources(project)
	if err != nil {
		return failure(err)
	}
	changed, err := hideseek.Apply(lease.HideMutations, original)
	if err != nil {
		return failure(err)
	}
	if lease.Answer == nil {
		return failure(fmt.Errorf("нет правок проекта"))
	}
	edits, err := hideseek.DecodeEdits(*lease.Answer, original)
	if err != nil {
		return failure(err)
	}
	for name, content := range edits {
		changed[name] = []byte(content)
	}
	if sameSources(original, changed) {
		if accepted, err := a.acceptReference(parent, ctx, project, lease); accepted || err != nil {
			return err
		}
	}
	if err = writeHideFiles(project, changed); err != nil {
		return failure(err)
	}
	if err = makeSandboxReadable(project); err != nil {
		return failure(err)
	}
	defer restoreSandboxOwnerAccess(project)
	results := runSandboxCached(ctx, a.config.Runtime, a.config.Image, project, lease.SessionID)
	outcome := "failed"
	if results.OK() {
		outcome = "passed"
	} else if results.InfrastructureError {
		outcome = "error"
	}
	return a.complete(parent, lease, postgres.Completion{Outcome: outcome, ConfigureResult: results.Configure, BuildResult: results.Build, CTestResult: results.CTest})
}
