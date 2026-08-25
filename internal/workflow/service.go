package workflow

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/store"
)

type Service struct {
	repo       *store.Repository
	calculator *assessment.Calculator
	now        func() time.Time
}

func New(repo *store.Repository, calculator *assessment.Calculator) *Service {
	return &Service{repo: repo, calculator: calculator, now: time.Now}
}

func (s *Service) context(meta Metadata) (domain.ChangeContext, error) {
	if strings.TrimSpace(meta.Actor) == "" {
		return domain.ChangeContext{}, &domain.FieldError{Field: "X-Actor", Message: "请求头不能为空"}
	}
	if strings.TrimSpace(meta.RequestID) == "" {
		return domain.ChangeContext{}, &domain.FieldError{Field: "X-Request-ID", Message: "请求头不能为空"}
	}
	return domain.ChangeContext{Actor: strings.TrimSpace(meta.Actor), RequestID: strings.TrimSpace(meta.RequestID), Now: s.now().UTC()}, nil
}

func requireIdempotency(meta Metadata) error {
	if strings.TrimSpace(meta.IdempotencyKey) == "" {
		return &domain.FieldError{Field: "Idempotency-Key", Message: "请求头不能为空"}
	}
	return nil
}

func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("随机数源不可用: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func resultFrom(data store.CaseData) CaseResult {
	return CaseResult{Case: data.Case, Assessment: data.Assessment, EvidenceProgress: data.EvidenceProgress, Deadline: data.Deadline}
}

func RegistrationFingerprint(shipmentCode, containerCode, sampleCategory string, minC, maxC float64) string {
	payload := struct {
		ShipmentCode, ContainerCode, SampleCategory string
		TemperatureMinC, TemperatureMaxC            float64
	}{strings.ToUpper(strings.TrimSpace(shipmentCode)), strings.ToUpper(strings.TrimSpace(containerCode)), strings.ToLower(strings.TrimSpace(sampleCategory)), minC, maxC}
	return commandFingerprint(payload)
}

func commandFingerprint(payload any) string {
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Service) Register(cmd RegisterCommand) (CaseResult, error) {
	if err := requireIdempotency(cmd.Metadata); err != nil {
		return CaseResult{}, err
	}
	ctx, err := s.context(cmd.Metadata)
	if err != nil {
		return CaseResult{}, err
	}
	scope := "register"
	fingerprint := cmd.RequestFingerprint
	if fingerprint == "" {
		fingerprint = RegistrationFingerprint(cmd.ShipmentCode, cmd.ContainerCode, cmd.SampleCategory, cmd.TemperatureMinC, cmd.TemperatureMaxC)
	}
	var prior CaseResult
	if ok, err := s.repo.Idempotent(scope, cmd.IdempotencyKey, fingerprint, &prior); ok || err != nil {
		return prior, err
	}
	if existing, err := s.repo.FindByShipment(strings.ToUpper(strings.TrimSpace(cmd.ShipmentCode))); err == nil {
		return CaseResult{}, &domain.DuplicateShipmentError{CaseID: existing.Case.ID, Revision: existing.Case.Revision}
	} else if err != domain.ErrNotFound {
		return CaseResult{}, err
	}
	c, event, err := domain.NewTransitCase(newID("case"), domain.NewCaseInput{ShipmentCode: cmd.ShipmentCode, ContainerCode: cmd.ContainerCode,
		SampleCategory: cmd.SampleCategory, TemperatureMinC: cmd.TemperatureMinC, TemperatureMaxC: cmd.TemperatureMaxC}, ctx, newID("evt"))
	if err != nil {
		return CaseResult{}, err
	}
	result := CaseResult{Case: c}
	if err := s.repo.Create(c, []domain.AuditEvent{event}, scope, cmd.IdempotencyKey, fingerprint, result); err != nil {
		return CaseResult{}, err
	}
	return result, nil
}

func (s *Service) Get(caseID string) (store.CaseData, error) {
	data, err := s.repo.GetCase(caseID)
	if err != nil {
		return store.CaseData{}, err
	}
	progress := domain.BuildEvidenceProgress(data.Evidence, data.Readings, s.calculator.CoverageSlop())
	data.EvidenceProgress = &progress
	data.Deadline = deadlineProjection(data, s.now().UTC())
	return data, nil
}

func deadlineProjection(data store.CaseData, now time.Time) *domain.DeadlineProjection {
	if data.Case.DueAt == nil {
		return nil
	}
	due := data.Case.DueAt.UTC()
	if data.Case.State != domain.StatePendingCorrection && len(data.Actions) > 0 {
		last := data.Actions[len(data.Actions)-1]
		if !last.SubmittedAt.IsZero() {
			submitted := last.SubmittedAt.UTC()
			return &domain.DeadlineProjection{Status: last.DeadlineStatus, DueAt: due, SubmittedAt: &submitted, OverdueMinutes: last.OverdueMinutes}
		}
	}
	delta := due.Sub(now)
	if delta < 0 {
		return &domain.DeadlineProjection{Status: domain.DeadlineOverdue, DueAt: due, OverdueMinutes: roundedUpMinutes(-delta)}
	}
	status := domain.DeadlineNotDue
	if delta <= 24*time.Hour {
		status = domain.DeadlineDueSoon
	}
	return &domain.DeadlineProjection{Status: status, DueAt: due, RemainingMinutes: roundedUpMinutes(delta)}
}

func roundedUpMinutes(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return int64((d + time.Minute - 1) / time.Minute)
}
