package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTagNormalization(t *testing.T) {
	tags := normalizeTags([]string{"Coding", "coding", " REASONING ", "fast", "coding"})
	if len(tags) != 3 {
		t.Fatalf("expected 3 unique tags, got %d: %v", len(tags), tags)
	}
	if tags[0] != "coding" {
		t.Errorf("first tag should be coding, got %s", tags[0])
	}
}

func TestGetModelTags(t *testing.T) {
	tm := NewTagManager()
	tm.builtIn["deepseek-ai/deepseek-v3.2"] = []string{"coding", "reasoning"}
	tm.user["deepseek-ai/deepseek-v3.2"] = []string{"my-custom"}

	tags := tm.GetModelTags("deepseek-ai/deepseek-v3.2")
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags (builtin + user), got %d", len(tags))
	}
	if !tm.HasCodingTag("deepseek-ai/deepseek-v3.2") {
		t.Error("should have coding tag")
	}
}

func TestSetModelTags(t *testing.T) {
	tm := NewTagManager()
	tm.SetModelTags("model/id", []string{"coding", "fast"})
	tags := tm.GetModelTags("model/id")
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
}

func TestLoadBuiltIn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tags.json")
	data := map[string][]string{
		"deepseek-ai/deepseek-v3.2": {"coding", "reasoning"},
		"qwen/qwen3-coder":          {"coding"},
	}
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0600)

	tm := NewTagManager()
	if err := tm.LoadBuiltIn(path); err != nil {
		t.Fatalf("LoadBuiltIn: %v", err)
	}
	if !tm.HasCodingTag("deepseek-ai/deepseek-v3.2") {
		t.Error("should have coding tag from builtin file")
	}
}

func TestValidateTag(t *testing.T) {
	if !ValidateTag("coding") {
		t.Error("coding should be valid")
	}
	if ValidateTag("not-a-tag") {
		t.Error("unknown tag should be invalid")
	}
}

func TestTagVocabulary(t *testing.T) {
	if len(TagVocabulary) != 5 {
		t.Fatalf("vocabulary should have 5 tags, got %d", len(TagVocabulary))
	}
	want := []string{"coding", "reasoning", "general", "fast", "agentic"}
	for i, w := range want {
		if TagVocabulary[i] != w {
			t.Errorf("vocabulary[%d] = %s, want %s", i, TagVocabulary[i], w)
		}
	}
}
