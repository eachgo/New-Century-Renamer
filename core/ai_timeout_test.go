package core

import (
	"os"
	"testing"
	"time"
)

func TestAITimeoutDefault(t *testing.T) {
	t.Setenv("NCREAMER_AI_TIMEOUT", "")
	if got := aiTimeout(); got != defaultAITimeout {
		t.Fatalf("默认超时应为 %v，实际 %v", defaultAITimeout, got)
	}
	if defaultAITimeout < 2*time.Minute {
		t.Fatalf("默认超时 %v 对本地小模型太短", defaultAITimeout)
	}
}

func TestAITimeoutEnvOverride(t *testing.T) {
	t.Setenv("NCREAMER_AI_TIMEOUT", "600")
	if got := aiTimeout(); got != 600*time.Second {
		t.Fatalf("期望 600s，实际 %v", got)
	}
}

func TestAITimeoutBadValueFallsBack(t *testing.T) {
	t.Setenv("NCREAMER_AI_TIMEOUT", "not-a-number")
	if got := aiTimeout(); got != defaultAITimeout {
		t.Fatalf("非法值应回退到默认 %v，实际 %v", defaultAITimeout, got)
	}
	t.Setenv("NCREAMER_AI_TIMEOUT", "0")
	if got := aiTimeout(); got != defaultAITimeout {
		t.Fatalf("0 应回退到默认 %v，实际 %v", defaultAITimeout, got)
	}
}

// Guard against the env var being read from the process environment
// unexpectedly in other tests: confirm unset env yields the default.
func TestAITimeoutUnset(t *testing.T) {
	os.Unsetenv("NCREAMER_AI_TIMEOUT")
	if got := aiTimeout(); got != defaultAITimeout {
		t.Fatalf("未设置时应为默认 %v，实际 %v", defaultAITimeout, got)
	}
}
