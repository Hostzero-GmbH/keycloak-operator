package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestObserve(t *testing.T) {
	current := json.RawMessage(`{"realm":"shared","enabled":true,"displayName":"Live","smtpServer":{"host":"smtp","password":"**********"}}`)

	tests := []struct {
		name        string
		desired     string
		current     json.RawMessage
		lookupErr   error
		wantReady   bool
		wantReason  string
		wantDrifted []string
	}{
		{
			name:       "404 is NotFound, not ready",
			desired:    `{}`,
			lookupErr:  errors.New(`404 Not Found: {"error":"Realm not found."}`),
			wantReason: NotFoundReason,
		},
		{
			name:       "other error is LookupFailed, not NotFound",
			desired:    `{}`,
			lookupErr:  errors.New("request failed: dial tcp: connection refused"),
			wantReason: LookupFailedReason,
		},
		{
			name:       "401 is LookupFailed",
			desired:    `{}`,
			lookupErr:  errors.New("401 Unauthorized: "),
			wantReason: LookupFailedReason,
		},
		{
			name:       "empty definition is in sync",
			desired:    `{}`,
			current:    current,
			wantReady:  true,
			wantReason: ObservedReason,
		},
		{
			name:       "matching subset is in sync",
			desired:    `{"realm":"shared","enabled":true}`,
			current:    current,
			wantReady:  true,
			wantReason: ObservedReason,
		},
		{
			name:       "masked smtp password is not drift",
			desired:    `{"smtpServer":{"host":"smtp","password":"secret"}}`,
			current:    current,
			wantReady:  true,
			wantReason: ObservedReason,
		},
		{
			name:        "differing and missing keys are drift",
			desired:     `{"enabled":true,"displayName":"Wanted","loginTheme":"custom"}`,
			current:     current,
			wantReady:   true,
			wantReason:  ObservedReason,
			wantDrifted: []string{"displayName", "loginTheme"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := observe("realm", "shared", json.RawMessage(tt.desired), tt.current, tt.lookupErr, realmDefinitionsMatch)
			if obs.Ready != tt.wantReady {
				t.Errorf("Ready = %v, want %v", obs.Ready, tt.wantReady)
			}
			if obs.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", obs.Reason, tt.wantReason)
			}
			if !reflect.DeepEqual(obs.Drifted, tt.wantDrifted) {
				t.Errorf("Drifted = %v, want %v", obs.Drifted, tt.wantDrifted)
			}
		})
	}
}

func TestObservationErrorLabel(t *testing.T) {
	if got := (observation{Reason: NotFoundReason}).errorLabel(); got != "observed_resource_not_found" {
		t.Errorf("NotFound label = %q", got)
	}
	if got := (observation{Reason: LookupFailedReason}).errorLabel(); got != "keycloak_api_error" {
		t.Errorf("LookupFailed label = %q", got)
	}
}

func TestDriftedKeysInvalidDefinition(t *testing.T) {
	got := driftedKeys(json.RawMessage(`not json`), json.RawMessage(`{}`), definitionsMatch)
	if !reflect.DeepEqual(got, []string{"definition"}) {
		t.Errorf("driftedKeys = %v, want [definition]", got)
	}
}

func TestSetDriftedCondition(t *testing.T) {
	var conditions []metav1.Condition

	setDriftedCondition(&conditions, nil)
	c := meta.FindStatusCondition(conditions, DriftedConditionType)
	if c == nil || c.Status != metav1.ConditionFalse || c.Reason != InSyncReason {
		t.Fatalf("in sync condition = %+v", c)
	}

	setDriftedCondition(&conditions, []string{"displayName"})
	c = meta.FindStatusCondition(conditions, DriftedConditionType)
	if c == nil || c.Status != metav1.ConditionTrue || c.Reason != DefinitionMismatchReason {
		t.Fatalf("drifted condition = %+v", c)
	}
	if !strings.HasSuffix(c.Message, ": displayName") {
		t.Errorf("message = %q", c.Message)
	}
	firstTransition := c.LastTransitionTime

	// The key list changes but the status does not flip: the transition time
	// must stay, so the status write does not re-trigger the controller.
	setDriftedCondition(&conditions, []string{"displayName", "loginTheme"})
	c = meta.FindStatusCondition(conditions, DriftedConditionType)
	if !c.LastTransitionTime.Equal(&firstTransition) {
		t.Errorf("LastTransitionTime moved without a status flip")
	}
	if !strings.HasSuffix(c.Message, ": displayName, loginTheme") {
		t.Errorf("message = %q", c.Message)
	}

	removeDriftedCondition(&conditions)
	if meta.FindStatusCondition(conditions, DriftedConditionType) != nil {
		t.Errorf("Drifted condition not removed")
	}
}

func TestRecordObservation(t *testing.T) {
	conditions := []metav1.Condition{{Type: DriftedConditionType, Status: metav1.ConditionTrue, Reason: DefinitionMismatchReason}}
	path := ""

	recordObservation(observation{Reason: NotFoundReason}, &conditions, &path, "/admin/realms/shared")
	if path != "" {
		t.Errorf("resource path set on a missing resource: %q", path)
	}
	if meta.FindStatusCondition(conditions, DriftedConditionType) != nil {
		t.Errorf("Drifted condition kept on a missing resource")
	}

	recordObservation(observation{Ready: true, Reason: ObservedReason}, &conditions, &path, "/admin/realms/shared")
	if path != "/admin/realms/shared" {
		t.Errorf("resource path = %q", path)
	}
	if c := meta.FindStatusCondition(conditions, DriftedConditionType); c == nil || c.Status != metav1.ConditionFalse {
		t.Errorf("Drifted condition = %+v", c)
	}
}

func TestFormatKeysTruncates(t *testing.T) {
	keys := make([]string, 12)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%02d", i)
	}
	got := formatKeys(keys)
	if !strings.HasSuffix(got, "k09 (and 2 more)") {
		t.Errorf("formatKeys = %q", got)
	}
	if strings.Contains(got, "k10") {
		t.Errorf("formatKeys reported more than %d keys: %q", maxReportedDriftKeys, got)
	}
}
