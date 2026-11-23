package basic

import (
	"encoding/json"
	"testing"
)

type embeddedAlertAnalysisInput struct {
	EventID string `json:"event_id"`
	RuleID  string `json:"rule_id"`
}

type embeddedPreliminaryLLMInput struct {
	embeddedAlertAnalysisInput
	IsMeasurement bool `json:"is_measurement"`
}

func TestAnonymousEmbeddedFieldInit(t *testing.T) {
	input := embeddedAlertAnalysisInput{
		EventID: "event-1",
		RuleID:  "rule-1",
	}

	// 匿名嵌入字段在初始化时，要用它的隐式字段名(类型名的未限定部分)
	modelInput := embeddedPreliminaryLLMInput{
		embeddedAlertAnalysisInput: input,
		IsMeasurement:              true,
	}

	if modelInput.embeddedAlertAnalysisInput.EventID != "event-1" {
		t.Fatalf("unexpected embedded field value: %+v", modelInput.embeddedAlertAnalysisInput)
	}
	if modelInput.EventID != "event-1" {
		t.Fatalf("promoted field should be accessible directly, got %q", modelInput.EventID)
	}
}

func TestAnonymousEmbeddedFieldJSONFlatten(t *testing.T) {
	modelInput := embeddedPreliminaryLLMInput{
		embeddedAlertAnalysisInput: embeddedAlertAnalysisInput{
			EventID: "event-1",
			RuleID:  "rule-1",
		},
		IsMeasurement: true,
	}

	data, err := json.Marshal(modelInput)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if out["event_id"] != "event-1" {
		t.Fatalf("expected flattened event_id, got %#v", out)
	}
	if out["rule_id"] != "rule-1" {
		t.Fatalf("expected flattened rule_id, got %#v", out)
	}
	if out["is_measurement"] != true {
		t.Fatalf("expected outer field to stay at top level, got %#v", out)
	}
	if _, ok := out["embeddedAlertAnalysisInput"]; ok {
		t.Fatalf("anonymous embedded field should be flattened in JSON, got %#v", out)
	}
}
