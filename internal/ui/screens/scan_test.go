package screens

import (
	"context"
	"errors"
	"go-s/internal/install"
	"go-s/internal/model"
	"go-s/internal/task"
	"testing"
)

func TestScanSuggestedSelectionAndPossibleOptIn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewScan(ctx, ".", Services{Scan: func(context.Context, string) (model.ScanResult, error) {
		return model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{
			{Skill: model.SkillRef{Source: "org/local", Name: "one"}, Bucket: "suggested"},
			{Skill: model.SkillRef{Source: "org/local", Name: "two"}, Bucket: "possible"},
		}}}, nil
	}})
	msg := s.Init()()
	updated, _ := s.Update(msg)
	s = updated.(*Scan)
	if !s.selected[0] || s.selected[1] {
		t.Fatalf("selection defaults must be suggested-only: %#v", s.selected)
	}
}

func TestScanRejectsStaleResult(t *testing.T) {
	s := NewScan(context.Background(), ".", Services{})
	s.operationID = "current"
	s.loading = true
	updated, _ := s.Update(task.DoneMsg[model.ScanResult]{Label: "scan", OperationID: "old"})
	if !updated.(*Scan).loading {
		t.Fatal("stale result changed loading state")
	}
}

func TestScanInstallFailureClearsLoading(t *testing.T) {
	s := NewScan(context.Background(), ".", Services{
		PlanInstall: func(string, model.ScanResult, []model.SkillRef, install.Options) (install.Plan, error) {
			return install.Plan{}, nil
		},
		Install: func(context.Context, install.Plan) error { return errors.New("failed") },
	})
	s.result = model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{{Skill: model.SkillRef{Source: "org/x", Name: "a"}, Bucket: "suggested"}}}}
	s.selected = []bool{true}
	s.operationID = "install"
	s.loading = true
	updated, cmd := s.startInstall()
	if !updated.(*Scan).loading {
		t.Fatal("install should be loading")
	}
	msg := cmd()
	updated, _ = s.Update(msg)
	if updated.(*Scan).loading {
		t.Fatal("install failure left spinner active")
	}
}
