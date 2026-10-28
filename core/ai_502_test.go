package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A 502 from the gateway must be reported promptly, not waited on for the full
// timeout. This measures how long aiGenerate takes against a server that
// returns 502 immediately.
func TestAI502FailsFast(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":{"message":"Bad gateway"}}`))
	}))
	defer srv.Close()
	t.Setenv("NCREAMER_AI_BASE", srv.URL+"/v1")
	t.Setenv("NCREAMER_AI_KEY", "k")
	t.Setenv("NCREAMER_AI_MODEL", "m")
	t.Setenv("NCREAMER_AI_TIMEOUT", "300")

	start := time.Now()
	_, err := aiGenerate("x.txt", "命名")
	el := time.Since(start)
	if err == nil {
		t.Fatal("502 应报错")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("错误应包含状态码 502,实际 %v", err)
	}
	if el > 5*time.Second {
		t.Fatalf("502 立刻返回却等了 %v(应快速失败)", el)
	}
	t.Logf("502 surfaced in %v", el)
}

// A server that accepts the connection but never replies must be cut off by the
// single user-set timeout, not hang forever. Uses a short budget so the test is
// fast.
func TestAIHangingServerTimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(200)
		w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	defer srv.Close()
	t.Setenv("NCREAMER_AI_BASE", srv.URL+"/v1")
	t.Setenv("NCREAMER_AI_KEY", "k")
	t.Setenv("NCREAMER_AI_MODEL", "m")
	t.Setenv("NCREAMER_AI_TIMEOUT", "1") // 1s budget

	start := time.Now()
	_, err := aiGenerate("x.txt", "命名")
	el := time.Since(start)
	if err == nil {
		t.Fatal("挂住的服务器应超时报错")
	}
	if el > 2500*time.Millisecond {
		t.Fatalf("应约 1s 超时,实际等了 %v", el)
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("超时错误应可读(含 超时),实际 %v", err)
	}
	t.Logf("hung server cut off in %v (err=%v)", el, err)
}
