package repository

import (
	"context"
	"testing"
)

func TestMemoryRepoListsEmptyExtensionsAsEmptySlices(t *testing.T) {
	repo := NewMemoryRepo()
	tools, err := repo.ListTools(context.Background(), "user")
	if err != nil || tools == nil || len(tools) != 0 {
		t.Fatalf("tools = %#v, %v; want non-nil empty slice", tools, err)
	}
	skills, err := repo.ListSkills(context.Background(), "user")
	if err != nil || skills == nil || len(skills) != 0 {
		t.Fatalf("skills = %#v, %v; want non-nil empty slice", skills, err)
	}
}
