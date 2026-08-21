package model

import (
	"testing"
)

func TestPlanStep_RiskLevels(t *testing.T) {
	tests := []struct {
		name string
		risk string
		want string
	}{
		{"safe", "safe", "safe"},
		{"moderate", "moderate", "moderate"},
		{"dangerous", "dangerous", "dangerous"},
		{"unknown defaults to moderate", "", "moderate"},
		{"invalid defaults to moderate", "extreme", "moderate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := PlanStep{Risk: tt.risk}
			if got := s.RiskLevel(); got != tt.want {
				t.Errorf("RiskLevel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlanStep_IsDangerous(t *testing.T) {
	s := PlanStep{Risk: "dangerous"}
	if !s.IsDangerous() {
		t.Error("dangerous step should be dangerous")
	}
	s2 := PlanStep{Risk: "safe"}
	if s2.IsDangerous() {
		t.Error("safe step should not be dangerous")
	}
}

func TestPlanStep_NeedsConfirm(t *testing.T) {
	tests := []struct {
		name        string
		risk        string
		autoApprove bool
		want        bool
	}{
		{"dangerous always confirm", "dangerous", true, true},
		{"dangerous no auto", "dangerous", false, true},
		{"safe auto", "safe", true, false},
		{"safe no auto", "safe", false, false},
		{"moderate auto", "moderate", true, false},
		{"moderate no auto", "moderate", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := PlanStep{Risk: tt.risk}
			if got := s.NeedsConfirm(tt.autoApprove); got != tt.want {
				t.Errorf("NeedsConfirm(%v) = %v, want %v", tt.autoApprove, got, tt.want)
			}
		})
	}
}

func TestPlanResponse_Validate(t *testing.T) {
	t.Run("empty steps fails", func(t *testing.T) {
		p := PlanResponse{Steps: []PlanStep{}}
		if err := p.Validate(); err == nil {
			t.Error("empty plan should fail")
		}
	})
	t.Run("valid plan", func(t *testing.T) {
		p := PlanResponse{Steps: []PlanStep{{Action: "read", Risk: "safe"}}}
		if err := p.Validate(); err != nil {
			t.Errorf("valid plan failed: %v", err)
		}
	})
	t.Run("empty action fails", func(t *testing.T) {
		p := PlanResponse{Steps: []PlanStep{{Action: "", Risk: "safe"}}}
		if err := p.Validate(); err == nil {
			t.Error("empty action should fail")
		}
	})
}

func TestPlanResponse_HasDangerousSteps(t *testing.T) {
	p1 := PlanResponse{Steps: []PlanStep{{Risk: "safe"}}}
	if p1.HasDangerousSteps() {
		t.Error("safe plan should not have dangerous steps")
	}
	p2 := PlanResponse{Steps: []PlanStep{{Risk: "safe"}, {Risk: "dangerous"}}}
	if !p2.HasDangerousSteps() {
		t.Error("plan with dangerous step should return true")
	}
}

func TestPlanExecutor_ApproveAll(t *testing.T) {
	pe := NewPlanExecutor()
	pe.ApproveAll()
	if !pe.IsApproved() {
		t.Error("should be approved after ApproveAll")
	}
}

func TestPlanExecutor_ApproveStep(t *testing.T) {
	pe := NewPlanExecutor()
	pe.ApproveStep()
	// ApproveStep increments from 0 to 1
	if pe.CurrentStep() != 1 {
		t.Errorf("after ApproveStep, CurrentStep = %d, want 1", pe.CurrentStep())
	}
}

func TestPlanExecutor_Reject(t *testing.T) {
	pe := NewPlanExecutor()
	pe.ApproveAll()
	pe.Reject()
	if pe.IsApproved() {
		t.Error("should not be approved after Reject")
	}
}

func TestPlanExecutor_HasNext(t *testing.T) {
	plan := &PlanResponse{Steps: []PlanStep{
		{Action: "read", Risk: "safe"},
		{Action: "edit", Risk: "moderate"},
	}}
	pe := NewPlanExecutor()
	// stepIdx starts at 0
	if !pe.HasNext(plan) {
		t.Error("should have next at index 0")
	}
	pe.Advance() // stepIdx = 1
	if !pe.HasNext(plan) {
		t.Error("should have next at index 1")
	}
	pe.Advance() // stepIdx = 2
	if pe.HasNext(plan) {
		t.Error("should not have next at end")
	}
}

func TestPlanExecutor_CurrentStepData(t *testing.T) {
	plan := &PlanResponse{Steps: []PlanStep{
		{Action: "read", Target: "a.go", Risk: "safe"},
	}}
	pe := NewPlanExecutor()
	// stepIdx starts at 0 — should return first step
	step := pe.CurrentStepData(plan)
	if step == nil || step.Action != "read" {
		t.Error("should return read step at index 0")
	}
	pe.Advance()
	if pe.CurrentStepData(plan) != nil {
		t.Error("should return nil at end")
	}
}

func TestParsePlanResponse(t *testing.T) {
	jsonData := `{"steps":[{"action":"read","target":"main.go","risk":"safe"}],"summary":"test"}`
	plan, err := ParsePlanResponse([]byte(jsonData))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].Action != "read" {
		t.Errorf("action = %s, want read", plan.Steps[0].Action)
	}
}
