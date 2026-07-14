package continuity

import (
	"bytes"
	"testing"
)

func TestLegacyMigrationSourceBudgetFailsBeforeUnboundedRetention(t *testing.T) {
	plan := LegacyMigration{}
	chunk := bytes.Repeat([]byte{'x'}, 1<<20)
	for index := 0; index < maxMigrationData/(1<<20); index++ {
		if err := (&Module{}).addLegacySourceFile(&plan, migrationBudgetPath(index), chunk); err != nil {
			t.Fatalf("add source %d: %v", index, err)
		}
	}
	if err := (&Module{}).addLegacySourceFile(&plan, migrationBudgetPath(99), []byte{'x'}); ErrorCode(err) != CodeMigrationTooLarge {
		t.Fatalf("error=%v code=%q, want %q", err, ErrorCode(err), CodeMigrationTooLarge)
	}
}

func TestContinuitySecretGateCoversCanonicalEnginePatterns(t *testing.T) {
	for _, value := range []string{
		"-----BEGIN RSA PRIVATE KEY-----",
		"sk_live_1234567890abcdef",
		"AIza12345678901234567890123456789012345",
	} {
		if !containsContinuitySecret(value) {
			t.Fatalf("secret pattern was not detected: %q", value)
		}
	}
}

func migrationBudgetPath(index int) string {
	const digits = "0123456789"
	return ".agent/ledger/facts/task_budget_" + string(digits[index/10%10]) + string(digits[index%10]) + ".jsonl"
}
