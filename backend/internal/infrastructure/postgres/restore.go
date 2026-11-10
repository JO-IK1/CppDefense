package postgres

import (
	"context"
	"errors"
	"fmt"
)

var ErrRestoreSelection = errors.New("select the requested number of non-overlapping elements; a containing class and its method cannot both be hidden")

// RestoreOptions is optional to keep existing single-body clients compatible.
type RestoreOptions struct {
	HiddenCount       int      `json:"hidden_count"`
	SignaturesVisible bool     `json:"signatures_visible"`
	CandidateIDs      []string `json:"candidate_ids"`
}

type RestoreTarget struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Signature string `json:"-"`
	BeginLine int    `json:"-"`
	EndLine   int    `json:"-"`
	SelectedFunction
}

func (r *DefenseRepository) restoreTargets(ctx context.Context, defense string) ([]RestoreTarget, bool, error) {
	var visible bool
	if err := r.db.pool.QueryRow(ctx, `select signatures_visible from defenses where id=$1`, defense).Scan(&visible); err != nil {
		return nil, false, err
	}
	rows, err := r.db.pool.Query(ctx, `select c.id,t.ordinal,c.function_name,c.entity_type,c.file_path,c.signature_begin_offset,c.body_start_offset,c.body_end_offset,c.source_sha256,c.signature,c.start_line,c.end_line from defense_restore_targets t join defense_candidates c on c.id=t.candidate_id and c.defense_id=t.defense_id where t.defense_id=$1 order by t.ordinal`, defense)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var targets []RestoreTarget
	for rows.Next() {
		var t RestoreTarget
		var ordinal int
		var digest []byte
		if err := rows.Scan(&t.ID, &ordinal, &t.FunctionName, &t.EntityType, &t.FilePath, &t.SignatureBegin, &t.BodyBegin, &t.BodyEnd, &digest, &t.Signature, &t.BeginLine, &t.EndLine); err != nil {
			return nil, false, err
		}
		t.Label = fmt.Sprintf("func_%d", ordinal)
		t.SourceSHA256 = fmt.Sprintf("%x", digest)
		targets = append(targets, t)
	}
	return targets, visible, rows.Err()
}

func nonOverlapping(a, b WheelCandidate) bool {
	return a.FilePath != b.FilePath || a.BodyEnd <= b.SignatureBegin || b.BodyEnd <= a.SignatureBegin
}

func selectRestoreTargets(catalog []WheelCandidate, mode, candidateID, seed string, size int, options RestoreOptions) ([]WheelCandidate, []WheelCandidate, bool) {
	if options.HiddenCount < 1 || options.HiddenCount > 5 || options.HiddenCount > size {
		return nil, nil, false
	}
	if mode == "manual" {
		ids := options.CandidateIDs
		if len(ids) == 0 && candidateID != "" {
			ids = []string{candidateID}
		}
		if len(ids) != options.HiddenCount {
			return nil, nil, false
		}
		var selected []WheelCandidate
		for _, id := range ids {
			found := false
			for _, c := range catalog {
				if c.ID == id {
					for _, other := range selected {
						if !nonOverlapping(c, other) {
							return nil, nil, false
						}
					}
					selected = append(selected, c)
					found = true
					break
				}
			}
			if !found {
				return nil, nil, false
			}
		}
		pool := make([]WheelCandidate, 0)
		for _, c := range catalog {
			include := !c.IsTestFile
			for _, s := range selected {
				if s.ID == c.ID {
					include = false
				}
			}
			if include {
				pool = append(pool, c)
			}
		}
		fillers, ok := mixedCandidates(pool, size-len(selected), seed)
		wheel := append(fillers, selected...)
		deterministicShuffle(wheel, seed)
		return wheel, selected, ok
	}
	wheel, _, ok := buildWheel(catalog, mode, "", seed, size)
	if !ok {
		return nil, nil, false
	}
	// Backtracking avoids failing merely because a containing class was visited first.
	var chosen []WheelCandidate
	var choose func(int) bool
	choose = func(index int) bool {
		if len(chosen) == options.HiddenCount {
			return true
		}
		for i := index; i < len(wheel); i++ {
			c := wheel[i]
			valid := true
			for _, s := range chosen {
				if !nonOverlapping(c, s) {
					valid = false
				}
			}
			if valid {
				chosen = append(chosen, c)
				if choose(i + 1) {
					return true
				}
				chosen = chosen[:len(chosen)-1]
			}
		}
		return false
	}
	if !choose(0) {
		return nil, nil, false
	}
	return wheel, chosen, true
}

func (r *DefenseRepository) decorateRestore(ctx context.Context, actor string, v *Defense) error {
	targets, visible, err := r.restoreTargets(ctx, v.ID)
	if err != nil {
		return err
	}
	v.SignaturesVisible = visible
	if len(targets) == 0 {
		return nil
	}
	var student bool
	if err = r.db.pool.QueryRow(ctx, `select role='student' from users where id=$1`, actor).Scan(&student); err != nil {
		return err
	}
	v.HiddenCount = len(targets)
	if student && !visible {
		for i := range v.WheelCandidates {
			v.WheelCandidates[i].Signature = ""
			v.WheelCandidates[i].FunctionName = "Элемент"
			for _, t := range targets {
				if v.WheelCandidates[i].ID == t.ID {
					v.WheelCandidates[i].FunctionName = t.Label
				}
			}
		}
	}
	// Wait for the existing preparation/verification transaction to activate the task.
	if v.Challenge == nil {
		return nil
	}
	for _, t := range targets {
		c := Challenge{ID: t.ID, Label: t.Label, FunctionName: t.FunctionName, EntityType: t.EntityType, FilePath: t.FilePath, Signature: t.Signature, BeginLine: t.BeginLine, EndLine: t.EndLine}
		if student && !visible {
			c.Signature = ""
			c.FunctionName = t.Label
		}
		v.Challenges = append(v.Challenges, c)
	}
	v.Challenge = &v.Challenges[0]
	return nil
}
