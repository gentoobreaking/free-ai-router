package models

import (
	"encoding/json"
	"strings"
)

var TagVocabulary = []string{"coding", "reasoning", "general", "fast", "agentic"}

type TagManager struct {
	builtIn map[string][]string
	user    map[string][]string
}

func NewTagManager() *TagManager {
	return &TagManager{
		builtIn: make(map[string][]string),
		user:    make(map[string][]string),
	}
}

func (t *TagManager) LoadBuiltIn(path string) error {
	data, err := readFile(path)
	if err != nil {
		return err
	}
	var tags map[string][]string
	if err := json.Unmarshal(data, &tags); err != nil {
		return err
	}
	for k, v := range tags {
		t.builtIn[k] = normalizeTags(v)
	}
	return nil
}

func (t *TagManager) LoadUser(userTags map[string][]string) {
	t.user = make(map[string][]string)
	for k, v := range userTags {
		t.user[k] = normalizeTags(v)
	}
}

func (t *TagManager) GetModelTags(modelID string) []string {
	result := make([]string, 0)
	seen := make(map[string]bool)

	for _, tag := range t.builtIn[modelID] {
		if !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}
	for _, tag := range t.user[modelID] {
		if !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}
	return result
}

func (t *TagManager) SetModelTags(modelID string, tags []string) {
	t.user[modelID] = normalizeTags(tags)
}

func (t *TagManager) UserTags() map[string][]string {
	return t.user
}

func (t *TagManager) HasCodingTag(modelID string) bool {
	for _, tag := range t.GetModelTags(modelID) {
		if tag == "coding" {
			return true
		}
	}
	return false
}

func normalizeTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool)
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag != "" && !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}
	return result
}

func ValidateTag(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for _, v := range TagVocabulary {
		if v == tag {
			return true
		}
	}
	return false
}
