package domain

import "errors"

var (
	ErrNotFound           = errors.New("资源不存在")
	ErrConflict           = errors.New("修订号冲突")
	ErrDuplicate          = errors.New("运输编号已存在")
	ErrIdempotencyPayload = errors.New("幂等键对应的登记载荷不一致")
	ErrInvalid            = errors.New("请求数据无效")
	ErrInvalidTransition  = errors.New("当前状态不允许此操作")
	ErrEvidenceIncomplete = errors.New("运输证据不完整")
	ErrForbidden          = errors.New("操作者无权执行此操作")
	ErrSummaryIncomplete  = errors.New("关闭摘要不完整")
)

type DuplicateShipmentError struct {
	CaseID   string `json:"existing_case_id"`
	Revision int64  `json:"current_revision"`
}

func (e *DuplicateShipmentError) Error() string { return ErrDuplicate.Error() }
func (e *DuplicateShipmentError) Unwrap() error { return ErrDuplicate }

type SummaryIncompleteError struct {
	MissingItems []string `json:"missing_items"`
}

func (e *SummaryIncompleteError) Error() string { return ErrSummaryIncomplete.Error() }
func (e *SummaryIncompleteError) Unwrap() error { return ErrSummaryIncomplete }

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Message }

type InvestigationConsistencyError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *InvestigationConsistencyError) Error() string { return e.Field + ": " + e.Message }
