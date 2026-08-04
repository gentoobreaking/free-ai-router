package router

import (
	"net/http"
	"testing"
)

// TestCopyHeadersStripsHopByHop: single-hop headers must never be forwarded
// to the client; the Connection header may name additional ones (RFC 7230).
func TestCopyHeadersStripsHopByHop(t *testing.T) {
	src := http.Header{}
	src.Set("Content-Type", "application/json")
	src.Set("Connection", "keep-alive, X-Foo")
	src.Set("Keep-Alive", "timeout=5")
	src.Set("Transfer-Encoding", "chunked")
	src.Set("X-Foo", "bar")
	src.Set("X-Custom", "ok")

	dst := http.Header{}
	copyHeaders(dst, src)

	if got := dst.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := dst.Get("X-Custom"); got != "ok" {
		t.Errorf("X-Custom = %q, want ok", got)
	}
	for _, k := range []string{"Connection", "Keep-Alive", "Transfer-Encoding", "X-Foo"} {
		if got := dst.Get(k); got != "" {
			t.Errorf("hop-by-hop header %q must be stripped, got %q", k, got)
		}
	}
}

// TestCopyHeadersPreservesMultipleValues: non-hop-by-hop headers keep all
// values.
func TestCopyHeadersPreservesMultipleValues(t *testing.T) {
	src := http.Header{}
	src.Add("X-Multi", "a")
	src.Add("X-Multi", "b")

	dst := http.Header{}
	copyHeaders(dst, src)

	if got := dst.Values("X-Multi"); len(got) != 2 {
		t.Errorf("X-Multi values = %v, want [a b]", got)
	}
}

// TestRewriteModelRejectsNonObject: rewriteModel must reject a non-object
// body instead of silently mangling it (T041).
func TestRewriteModelRejectsNonObject(t *testing.T) {
	for _, body := range []string{`[1,2,3]`, `"just a string"`, `42`} {
		if _, err := rewriteModel([]byte(body), "upstream/model"); err == nil {
			t.Errorf("rewriteModel(%s) should fail", body)
		}
	}
}

// TestRewriteModelReplacesModelField: the model field is replaced while all
// other fields survive (spec §7.3 step 6).
func TestRewriteModelReplacesModelField(t *testing.T) {
	out, err := rewriteModel([]byte(`{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`), "upstream/model")
	if err != nil {
		t.Fatalf("rewriteModel: %v", err)
	}
	got := string(out)
	if !containsString(got, `"model":"upstream/model"`) {
		t.Errorf("model field not replaced: %s", got)
	}
	if !containsString(got, `"content":"hi"`) {
		t.Errorf("other fields must be preserved: %s", got)
	}
}

func containsString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
