package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestJsonStderrError_StructuredOutput(t *testing.T) {
	tests := []struct {
		name    string
		message string
		hint    string
	}{
		{"message_only", "database not found", ""},
		{"message_with_hint", "database not found", "Run 'bd init' to create one"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]interface{}{
				"schema_version": JSONSchemaVersion,
				"error":          tt.message,
			}
			if tt.hint != "" {
				obj["hint"] = tt.hint
			}

			data, err := json.Marshal(obj)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var parsed map[string]interface{}
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if parsed["schema_version"] != float64(JSONSchemaVersion) {
				t.Errorf("schema_version = %v, want %d", parsed["schema_version"], JSONSchemaVersion)
			}
			if parsed["error"] != tt.message {
				t.Errorf("error = %v, want %s", parsed["error"], tt.message)
			}
			if tt.hint != "" {
				if parsed["hint"] != tt.hint {
					t.Errorf("hint = %v, want %s", parsed["hint"], tt.hint)
				}
			} else {
				if _, ok := parsed["hint"]; ok {
					t.Errorf("hint should not be present when empty")
				}
			}
		})
	}
}

// captureStdoutDuring captures stdout output produced while fn runs. Unlike
// captureStdout (test_helpers_pure_test.go), it does not fail the test based
// on any value fn returns — needed here because HandleAmbiguousMoleculeError
// is expected to return a non-nil *exitError by design (be-myd44), and that
// must not be mistaken for a test failure.
func captureStdoutDuring(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	defer func() {
		os.Stdout = old
		_ = r.Close()
	}()
	fn()
	_ = w.Close()
	return <-done
}

// --- be-myd44: refuse ambiguous multi-molecule match instead of silently
// returning one --- (see also cmd/bd/mol_test.go for the resolver-level
// tests; these cover the JSON/text error-rendering layer, modeled directly
// on buildJSONCapabilityError/HandleProxyCapabilityError.)

func TestBuildJSONAmbiguousMoleculeError(t *testing.T) {
	e := &ambiguousMoleculeError{
		Agent: "agent-x",
		Candidates: []*ambiguousMoleculeCandidate{
			{MoleculeID: "test-1", MoleculeTitle: "Molecule A", StepID: "test-2", StepTitle: "Step A1"},
			{MoleculeID: "test-3", MoleculeTitle: "Molecule B", StepID: "test-4", StepTitle: "Step B1"},
		},
	}

	built := buildJSONAmbiguousMoleculeError(e)
	data, err := json.Marshal(built)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 || data[0] != '{' {
		t.Fatalf("buildJSONAmbiguousMoleculeError() marshaled to %q, want a leading '{' (JSON object, not array)", data)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["schema_version"] != float64(JSONSchemaVersion) {
		t.Errorf("schema_version = %v, want %d", parsed["schema_version"], JSONSchemaVersion)
	}
	if parsed["code"] != "ambiguous_molecule" {
		t.Errorf("code = %v, want %q", parsed["code"], "ambiguous_molecule")
	}
	if _, ok := parsed["error"].(string); !ok {
		t.Errorf("error field missing or not a string: %v", parsed["error"])
	}
	candidates, ok := parsed["candidates"].([]interface{})
	if !ok {
		t.Fatalf("candidates field missing or not an array: %v", parsed["candidates"])
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates has %d entries, want 2", len(candidates))
	}
	first, ok := candidates[0].(map[string]interface{})
	if !ok {
		t.Fatalf("candidates[0] is not an object: %v", candidates[0])
	}
	if first["molecule_id"] != "test-1" {
		t.Errorf("candidates[0].molecule_id = %v, want %q", first["molecule_id"], "test-1")
	}
	if first["step_id"] != "test-2" {
		t.Errorf("candidates[0].step_id = %v, want %q", first["step_id"], "test-2")
	}
}

// TestAmbiguousMoleculeErrorDistinguishableFromSliceSuccess pins the
// object-vs-array distinction a JSON consumer needs to tell an ambiguous
// refusal apart from an ordinary successful multi-molecule list: with
// BD_JSON_ENVELOPE unset, wrapWithSchemaVersion's slice branch
// (cmd/bd/output.go) returns a bare array with no schema_version key, so the
// error shape must marshal to an object ('{') never an array ('[') to stay
// distinguishable on the wire.
func TestAmbiguousMoleculeErrorDistinguishableFromSliceSuccess(t *testing.T) {
	t.Setenv("BD_JSON_ENVELOPE", "")

	success := wrapWithSchemaVersion([]*MoleculeProgress{{MoleculeID: "test-1"}})
	successData, err := json.Marshal(success)
	if err != nil {
		t.Fatalf("marshal success: %v", err)
	}
	if len(successData) == 0 || successData[0] != '[' {
		t.Fatalf("wrapWithSchemaVersion(slice) marshaled to %q, want a leading '[' (bare array when envelope is off)", successData)
	}

	failure := buildJSONAmbiguousMoleculeError(&ambiguousMoleculeError{
		Agent: "agent-x",
		Candidates: []*ambiguousMoleculeCandidate{
			{MoleculeID: "test-1"},
			{MoleculeID: "test-3"},
		},
	})
	failureData, err := json.Marshal(failure)
	if err != nil {
		t.Fatalf("marshal failure: %v", err)
	}
	if len(failureData) == 0 || failureData[0] != '{' {
		t.Fatalf("buildJSONAmbiguousMoleculeError() marshaled to %q, want a leading '{' (object, distinguishable from the slice success shape)", failureData)
	}
}

// TestHandleAmbiguousMoleculeError covers exit_contract's "non-zero exit,
// JSON object not array, name every candidate" requirement end to end
// through the same front-door convention every other proxy/capability error
// uses (HandleProxyCapabilityError): JSON to stdout when jsonOutput is set,
// text to stderr otherwise, exit code 1 either way.
func TestHandleAmbiguousMoleculeError(t *testing.T) {
	e := &ambiguousMoleculeError{
		Agent: "agent-x",
		Candidates: []*ambiguousMoleculeCandidate{
			{MoleculeID: "test-1", MoleculeTitle: "Molecule A", StepID: "test-2", StepTitle: "Step A1"},
			{MoleculeID: "test-3", MoleculeTitle: "Molecule B", StepID: "test-4", StepTitle: "Step B1"},
		},
	}

	t.Run("json mode", func(t *testing.T) {
		old := jsonOutput
		jsonOutput = true
		defer func() { jsonOutput = old }()

		var handleErr error
		stdout := captureStdoutDuring(t, func() {
			handleErr = HandleAmbiguousMoleculeError(e)
		})

		code, ok := exitCodeFromError(handleErr)
		if !ok || code != 1 {
			t.Errorf("exitCodeFromError() = (%d, %v), want (1, true)", code, ok)
		}
		if len(stdout) == 0 || stdout[0] != '{' {
			t.Fatalf("stdout = %q, want a leading '{' (JSON object)", stdout)
		}
		if !strings.Contains(stdout, `"candidates"`) {
			t.Errorf("stdout = %q, want it to contain %q", stdout, `"candidates"`)
		}
	})

	t.Run("text mode", func(t *testing.T) {
		old := jsonOutput
		jsonOutput = false
		defer func() { jsonOutput = old }()

		var handleErr error
		stderr := captureStderr(t, func() {
			handleErr = HandleAmbiguousMoleculeError(e)
		})

		code, ok := exitCodeFromError(handleErr)
		if !ok || code != 1 {
			t.Errorf("exitCodeFromError() = (%d, %v), want (1, true)", code, ok)
		}
		if !strings.Contains(stderr, "test-1") {
			t.Errorf("stderr = %q, want it to contain candidate molecule id %q", stderr, "test-1")
		}
		if !strings.Contains(stderr, "test-3") {
			t.Errorf("stderr = %q, want it to contain candidate molecule id %q", stderr, "test-3")
		}
	})
}
