package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetReadyCondition(t *testing.T) {
	t.Run("appends Ready condition when none exists", func(t *testing.T) {
		out := setReadyCondition(nil, true, "Ready", "all good")
		if len(out) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(out))
		}
		if out[0].Type != ReadyConditionType {
			t.Errorf("expected condition type %q, got %q", ReadyConditionType, out[0].Type)
		}
		if out[0].Status != metav1.ConditionTrue {
			t.Errorf("expected ConditionTrue, got %q", out[0].Status)
		}
		if out[0].Reason != "Ready" {
			t.Errorf("expected reason %q, got %q", "Ready", out[0].Reason)
		}
		if out[0].Message != "all good" {
			t.Errorf("expected message %q, got %q", "all good", out[0].Message)
		}
		if out[0].LastTransitionTime.IsZero() {
			t.Error("expected LastTransitionTime to be set")
		}
	})

	t.Run("sets ConditionFalse when not ready", func(t *testing.T) {
		out := setReadyCondition(nil, false, "InvalidSpec", "missing field")
		if out[0].Status != metav1.ConditionFalse {
			t.Errorf("expected ConditionFalse, got %q", out[0].Status)
		}
		if out[0].Reason != "InvalidSpec" {
			t.Errorf("expected reason %q, got %q", "InvalidSpec", out[0].Reason)
		}
	})

	t.Run("replaces existing Ready condition in place", func(t *testing.T) {
		existing := []metav1.Condition{
			{Type: ReadyConditionType, Status: metav1.ConditionFalse, Reason: "Pending", Message: "old"},
			{Type: "OtherCondition", Status: metav1.ConditionTrue, Reason: "X", Message: "untouched"},
		}
		out := setReadyCondition(existing, true, "Ready", "now ok")
		if len(out) != 2 {
			t.Fatalf("expected 2 conditions, got %d", len(out))
		}
		if out[0].Status != metav1.ConditionTrue || out[0].Reason != "Ready" || out[0].Message != "now ok" {
			t.Errorf("Ready condition was not updated correctly: %+v", out[0])
		}
		if out[1].Type != "OtherCondition" || out[1].Reason != "X" || out[1].Message != "untouched" {
			t.Errorf("non-Ready condition should be left alone, got %+v", out[1])
		}
	})

	t.Run("preserves order when Ready is not first", func(t *testing.T) {
		existing := []metav1.Condition{
			{Type: "OtherCondition", Status: metav1.ConditionTrue, Reason: "X", Message: "untouched"},
			{Type: ReadyConditionType, Status: metav1.ConditionFalse, Reason: "Pending", Message: "old"},
		}
		out := setReadyCondition(existing, true, "Ready", "ok")
		if len(out) != 2 {
			t.Fatalf("expected 2 conditions, got %d", len(out))
		}
		if out[0].Type != "OtherCondition" {
			t.Errorf("expected first condition to remain OtherCondition, got %q", out[0].Type)
		}
		if out[1].Type != ReadyConditionType || out[1].Status != metav1.ConditionTrue {
			t.Errorf("expected Ready condition at index 1 to be true, got %+v", out[1])
		}
	})

	// Regression guard for issue #121: refreshing LastTransitionTime on an
	// unchanged condition makes every reconcile rewrite status, which
	// re-triggers the controller's own watch and produces a hot loop.
	t.Run("preserves LastTransitionTime when the status does not flip", func(t *testing.T) {
		original := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
		existing := []metav1.Condition{{
			Type:               ReadyConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             "Ready",
			Message:            "synchronized",
			LastTransitionTime: original,
		}}

		out := setReadyCondition(existing, true, "Ready", "synchronized")

		if !out[0].LastTransitionTime.Equal(&original) {
			t.Errorf("LastTransitionTime changed on a no-op update: got %v, want %v",
				out[0].LastTransitionTime, original)
		}
	})

	t.Run("updates reason and message without moving LastTransitionTime", func(t *testing.T) {
		original := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
		existing := []metav1.Condition{{
			Type:               ReadyConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             "Ready",
			Message:            "old message",
			LastTransitionTime: original,
		}}

		out := setReadyCondition(existing, true, "Ready", "new message")

		if out[0].Message != "new message" {
			t.Errorf("expected message to be updated, got %q", out[0].Message)
		}
		if !out[0].LastTransitionTime.Equal(&original) {
			t.Error("LastTransitionTime must not move when only the message changes")
		}
	})

	t.Run("moves LastTransitionTime when the status flips", func(t *testing.T) {
		original := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
		existing := []metav1.Condition{{
			Type:               ReadyConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             "Ready",
			Message:            "synchronized",
			LastTransitionTime: original,
		}}

		out := setReadyCondition(existing, false, "SyncFailed", "boom")

		if out[0].Status != metav1.ConditionFalse {
			t.Errorf("expected status False, got %q", out[0].Status)
		}
		if out[0].LastTransitionTime.Equal(&original) {
			t.Error("LastTransitionTime must advance when the condition status flips")
		}
	})
}
