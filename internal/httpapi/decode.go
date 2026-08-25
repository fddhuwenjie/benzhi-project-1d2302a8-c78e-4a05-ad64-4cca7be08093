package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/workflow"
)

const maxBodyBytes = 1 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			return &domain.FieldError{Field: "body", Message: "请求体超过 1 MiB"}
		}
		return &domain.FieldError{Field: "body", Message: "JSON 格式无效: " + err.Error()}
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return &domain.FieldError{Field: "body", Message: "只能包含一个 JSON 对象"}
	}
	return nil
}

func metadata(r *http.Request, revisionRequired bool) (workflow.Metadata, error) {
	m := workflow.Metadata{Actor: r.Header.Get("X-Actor"), RequestID: r.Header.Get("X-Request-ID"), IdempotencyKey: r.Header.Get("Idempotency-Key")}
	if !revisionRequired {
		return m, nil
	}
	raw := strings.TrimSpace(r.Header.Get("If-Match"))
	raw = strings.Trim(raw, "\"")
	if raw == "" {
		return m, &domain.FieldError{Field: "If-Match", Message: "请求头不能为空"}
	}
	revision, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || revision < 1 {
		return m, &domain.FieldError{Field: "If-Match", Message: "必须是正整数修订号"}
	}
	m.ExpectedRevision = revision
	return m, nil
}

func queryInt(r *http.Request, name string, fallback int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("%s 必须是非负整数", name)
	}
	return v, nil
}
