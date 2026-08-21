// Package model — Plan Mode for Heretic Forge.
// «Primum non nocere. Plan before act.»
package model

import (
	"encoding/json"
	"fmt"
)

// PlanStep is a single step in a plan.
type PlanStep struct {
	Action      string `json:"action"`      // "read", "write", "edit", "bash"
	Target      string `json:"target"`      // file or command
	Description string `json:"description"` // human-readable description
	Risk        string `json:"risk"`        // "safe", "moderate", "dangerous"
}

// PlanResponse is the full plan returned by the agent.
type PlanResponse struct {
	Steps   []PlanStep `json:"steps"`
	Summary string     `json:"summary"`
	AskUser bool       `json:"ask_user"` // true = waits for approval
}

// Validate checks that the plan is well-formed.
func (p *PlanResponse) Validate() error {
	if len(p.Steps) == 0 {
		return fmt.Errorf("plan has no steps")
	}
	for i, step := range p.Steps {
		if step.Action == "" {
			return fmt.Errorf("step %d has empty action", i)
		}
		if step.Risk != "safe" && step.Risk != "moderate" && step.Risk != "dangerous" {
			return fmt.Errorf("step %d has invalid risk: %q", i, step.Risk)
		}
	}
	return nil
}

// HasDangerousSteps returns true if any step is "dangerous".
func (p *PlanResponse) HasDangerousSteps() bool {
	for _, step := range p.Steps {
		if step.Risk == "dangerous" {
			return true
		}
	}
	return false
}

// RiskLevel returns the risk classification of a step.
func (s *PlanStep) RiskLevel() string {
	switch s.Risk {
	case "safe", "moderate", "dangerous":
		return s.Risk
	default:
		return "moderate"
	}
}

// IsDangerous returns true if the step always requires manual confirmation.
func (s *PlanStep) IsDangerous() bool {
	return s.Risk == "dangerous"
}

// NeedsConfirm returns true if the step requires user confirmation.
func (s *PlanStep) NeedsConfirm(autoApprove bool) bool {
	if s.IsDangerous() {
		return true
	}
	if s.Risk == "moderate" && !autoApprove {
		return true
	}
	return false
}

// PlanExecutor executes plan steps.
type PlanExecutor struct {
	Steps       []PlanStep
	AutoApprove bool
	stepIdx     int
}

// NewPlanExecutor creates a new executor.
func NewPlanExecutor() *PlanExecutor {
	return &PlanExecutor{stepIdx: 0}
}

// ApproveAll marks all steps as approved.
func (pe *PlanExecutor) ApproveAll() {
	pe.AutoApprove = true
	pe.stepIdx = -1 // -1 means all
}

// ApproveStep approves the current step and advances.
func (pe *PlanExecutor) ApproveStep() {
	pe.stepIdx++
}

// Reject marks the plan as rejected.
func (pe *PlanExecutor) Reject() {
	pe.AutoApprove = false
	pe.stepIdx = -1
}

// IsApproved returns whether the plan is approved.
func (pe *PlanExecutor) IsApproved() bool {
	return pe.stepIdx >= 0 || pe.AutoApprove
}

// CurrentStep returns the current step index (-1 if all approved/rejected).
func (pe *PlanExecutor) CurrentStep() int {
	return pe.stepIdx
}

// HasNext returns true if there are more steps to execute.
func (pe *PlanExecutor) HasNext(plan *PlanResponse) bool {
	return pe.stepIdx >= 0 && pe.stepIdx < len(plan.Steps)
}

// Advance moves to the next step.
func (pe *PlanExecutor) Advance() {
	pe.stepIdx++
}

// CurrentStepData returns the current step from the plan.
func (pe *PlanExecutor) CurrentStepData(plan *PlanResponse) *PlanStep {
	if pe.stepIdx < 0 || pe.stepIdx >= len(plan.Steps) {
		return nil
	}
	return &plan.Steps[pe.stepIdx]
}

// ParsePlanResponse parses a JSON plan response.
func ParsePlanResponse(data []byte) (*PlanResponse, error) {
	var plan PlanResponse
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return &plan, nil
}
