package domain

import (
	"strings"
	"time"
)

type Investigation struct {
	TransitCaseID      string            `json:"transit_case_id"`
	CauseCategory      string            `json:"cause_category"`
	RootCause          string            `json:"root_cause"`
	ImpactAnalysis     string            `json:"impact_analysis"`
	TriggerImpacts     map[string]string `json:"trigger_impacts"`
	Disposition        string            `json:"disposition"`
	NeedsCorrection    bool              `json:"needs_correction"`
	ReviewReason       string            `json:"review_reason,omitempty"`
	AcceptabilityBasis string            `json:"acceptability_basis,omitempty"`
	ReviewedBy         string            `json:"reviewed_by"`
	ReviewedAt         time.Time         `json:"reviewed_at"`
}

type DeadlineStatus string

const (
	DeadlineNotDue          DeadlineStatus = "not_due"
	DeadlineDueSoon         DeadlineStatus = "due_soon"
	DeadlineOverdue         DeadlineStatus = "overdue"
	DeadlineSubmittedOnTime DeadlineStatus = "submitted_on_time"
	DeadlineSubmittedLate   DeadlineStatus = "submitted_late"
)

type DeadlineProjection struct {
	Status           DeadlineStatus `json:"status"`
	DueAt            time.Time      `json:"due_at"`
	RemainingMinutes int64          `json:"remaining_minutes,omitempty"`
	OverdueMinutes   int64          `json:"overdue_minutes,omitempty"`
	SubmittedAt      *time.Time     `json:"submitted_at,omitempty"`
}

type VerificationIssue struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

type IssueResolution struct {
	IssueID    string `json:"issue_id"`
	Resolution string `json:"resolution"`
}

type CorrectiveAction struct {
	ID                 string              `json:"id"`
	TransitCaseID      string              `json:"transit_case_id"`
	RootCause          string              `json:"root_cause"`
	ActionText         string              `json:"action_text"`
	CompletionNote     string              `json:"completion_note"`
	Owner              string              `json:"owner"`
	DueAt              time.Time           `json:"due_at"`
	EvidenceRefs       []string            `json:"evidence_refs"`
	SubmissionNumber   int                 `json:"submission_number"`
	PreviousVersion    int                 `json:"previous_version,omitempty"`
	IssueResolutions   []IssueResolution   `json:"issue_resolutions,omitempty"`
	SubmittedAt        time.Time           `json:"submitted_at"`
	DeadlineStatus     DeadlineStatus      `json:"deadline_status"`
	OverdueMinutes     int64               `json:"overdue_minutes,omitempty"`
	OverdueReason      string              `json:"overdue_reason,omitempty"`
	VerificationResult string              `json:"verification_result,omitempty"`
	VerificationNote   string              `json:"verification_note,omitempty"`
	VerificationIssues []VerificationIssue `json:"verification_issues,omitempty"`
	EvidenceVisible    bool                `json:"evidence_visible,omitempty"`
	VerifiedBy         string              `json:"verified_by,omitempty"`
	VerifiedAt         *time.Time          `json:"verified_at,omitempty"`
}

func ValidateInvestigation(i Investigation) error {
	if strings.TrimSpace(i.CauseCategory) == "" || strings.TrimSpace(i.RootCause) == "" || strings.TrimSpace(i.ImpactAnalysis) == "" || strings.TrimSpace(i.Disposition) == "" {
		return &FieldError{Field: "investigation", Message: "原因分类、根因、影响分析和处置结论均不能为空"}
	}
	if !i.NeedsCorrection && strings.TrimSpace(i.ReviewReason) == "" {
		return &FieldError{Field: "review_reason", Message: "无需整改时必须说明审核通过理由"}
	}
	return nil
}

var allowedCauseCategories = map[string]bool{
	"packaging": true, "equipment": true, "handling": true, "handoff": true, "transport": true, "other": true,
	"packaging_failure": true, "equipment_failure": true, "handling_error": true, "handoff_failure": true, "transport_delay": true,
	"包装": true, "设备": true, "操作": true, "交接": true, "运输": true, "其他": true,
}

var allowedDispositions = map[string]bool{
	"correction_required": true, "quarantine": true, "rework": true, "reject": true, "需要整改": true, "隔离待验证": true,
	"accepted_no_correction": true, "no_correction": true, "release": true, "accepted": true, "审核通过": true, "无需整改": true,
}

func dispositionNeedsCorrection(value string) bool {
	return value == "correction_required" || value == "quarantine" || value == "rework" || value == "reject" || value == "需要整改" || value == "隔离待验证"
}

func ValidateInvestigationAgainstAssessment(i Investigation, a DeviationAssessment, assignee string, dueAt time.Time) error {
	if err := ValidateInvestigation(i); err != nil {
		return err
	}
	if !allowedCauseCategories[i.CauseCategory] {
		return &FieldError{Field: "cause_category", Message: "不属于受控原因分类"}
	}
	if !allowedDispositions[i.Disposition] {
		return &FieldError{Field: "disposition", Message: "不属于受控处置结论"}
	}
	if i.NeedsCorrection != dispositionNeedsCorrection(i.Disposition) {
		return &InvestigationConsistencyError{Field: "disposition", Message: "处置结论与整改选择不一致"}
	}
	for _, trigger := range a.Triggers {
		if strings.TrimSpace(i.TriggerImpacts[trigger]) == "" {
			return &InvestigationConsistencyError{Field: "trigger_impacts", Message: "必须逐项回应自动判定触发项: " + trigger}
		}
	}
	if a.Severity == SeverityMajor && !i.NeedsCorrection {
		return &InvestigationConsistencyError{Field: "needs_correction", Message: "重大偏差必须整改"}
	}
	if a.Severity == SeverityGeneral && !i.NeedsCorrection && (strings.TrimSpace(i.ReviewReason) == "" || strings.TrimSpace(i.AcceptabilityBasis) == "") {
		return &InvestigationConsistencyError{Field: "acceptability_basis", Message: "一般偏差无需整改时必须提供审核理由和样本可接受性依据"}
	}
	if i.NeedsCorrection && (strings.TrimSpace(assignee) == "" || !dueAt.After(i.ReviewedAt)) {
		return &FieldError{Field: "assignee", Message: "需要整改时责任人不能为空且期限必须晚于调查时间"}
	}
	return nil
}

func ValidateCorrectiveAction(a CorrectiveAction) error {
	if strings.TrimSpace(a.ActionText) == "" || strings.TrimSpace(a.CompletionNote) == "" || strings.TrimSpace(a.Owner) == "" {
		return &FieldError{Field: "corrective_action", Message: "措施、完成说明和责任人均不能为空"}
	}
	if a.DueAt.IsZero() || len(a.EvidenceRefs) == 0 {
		return &FieldError{Field: "evidence_refs", Message: "期限和至少一项证据引用为必填项"}
	}
	for _, ref := range a.EvidenceRefs {
		if strings.TrimSpace(ref) == "" {
			return &FieldError{Field: "evidence_refs", Message: "证据引用不能为空"}
		}
	}
	if a.DeadlineStatus == DeadlineSubmittedLate && strings.TrimSpace(a.OverdueReason) == "" {
		return &FieldError{Field: "overdue_reason", Message: "逾期提交整改时必须说明原因"}
	}
	return nil
}

func ValidateIssueResolutions(previous CorrectiveAction, resolutions []IssueResolution) error {
	if previous.VerificationResult != "rejected" {
		return nil
	}
	valid := make(map[string]bool, len(previous.VerificationIssues))
	for _, issue := range previous.VerificationIssues {
		valid[issue.ID] = true
	}
	resolved := make(map[string]bool, len(resolutions))
	for _, item := range resolutions {
		if strings.TrimSpace(item.IssueID) == "" || strings.TrimSpace(item.Resolution) == "" {
			return &FieldError{Field: "issue_resolutions", Message: "问题引用和解决说明不能为空"}
		}
		if !valid[item.IssueID] {
			return &FieldError{Field: "issue_resolutions", Message: "引用了不存在的驳回问题: " + item.IssueID}
		}
		if resolved[item.IssueID] {
			return &FieldError{Field: "issue_resolutions", Message: "驳回问题不能重复回应: " + item.IssueID}
		}
		resolved[item.IssueID] = true
	}
	for _, issue := range previous.VerificationIssues {
		if !resolved[issue.ID] {
			return &FieldError{Field: "issue_resolutions", Message: "必须回应上一版本驳回问题: " + issue.ID}
		}
	}
	return nil
}
