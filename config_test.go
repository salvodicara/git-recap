package main

import (
	"slices"
	"testing"
)

func TestApplyConfigFlags(t *testing.T) {
	t.Run("scalars replace fields", func(t *testing.T) {
		cfg := &Config{Profiles: map[string]Profile{"work": {}}}
		err := applyConfigFlags(cfg, map[string]string{
			"recaps-folder":   "~/j",
			"roots":           "~/a, ~/b",
			"default-profile": "work",
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.RecapsFolder != "~/j" {
			t.Errorf("recaps folder = %q", cfg.RecapsFolder)
		}
		if !slices.Equal(cfg.WorkspaceRoots, []string{"~/a", "~/b"}) {
			t.Errorf("roots = %v", cfg.WorkspaceRoots)
		}
		if cfg.DefaultProfile != "work" {
			t.Errorf("default = %q", cfg.DefaultProfile)
		}
	})

	t.Run("new profile is created and becomes default", func(t *testing.T) {
		cfg := &Config{}
		err := applyConfigFlags(cfg, map[string]string{
			"profile": "work",
			"orgs":    "acme, acme-labs",
			"emails":  "me@co.com",
		})
		if err != nil {
			t.Fatal(err)
		}
		p, ok := cfg.Profiles["work"]
		if !ok {
			t.Fatal("profile not created")
		}
		if !slices.Equal(p.Orgs, []string{"acme", "acme-labs"}) || !slices.Equal(p.Emails, []string{"me@co.com"}) {
			t.Errorf("profile = %+v", p)
		}
		if cfg.DefaultProfile != "work" {
			t.Errorf("default not auto-set, got %q", cfg.DefaultProfile)
		}
	})

	t.Run("orgs without profile errors", func(t *testing.T) {
		if err := applyConfigFlags(&Config{}, map[string]string{"orgs": "acme"}); err == nil {
			t.Error("expected error for --orgs without --profile")
		}
	})

	t.Run("delete removes profile and clears default", func(t *testing.T) {
		cfg := &Config{
			DefaultProfile: "work",
			Profiles:       map[string]Profile{"work": {}},
		}
		if err := applyConfigFlags(cfg, map[string]string{"delete-profile": "work"}); err != nil {
			t.Fatal(err)
		}
		if _, ok := cfg.Profiles["work"]; ok {
			t.Error("profile not deleted")
		}
		if cfg.DefaultProfile != "" {
			t.Errorf("default not cleared, got %q", cfg.DefaultProfile)
		}
	})

	t.Run("delete missing profile errors", func(t *testing.T) {
		if err := applyConfigFlags(&Config{}, map[string]string{"delete-profile": "nope"}); err == nil {
			t.Error("expected error deleting missing profile")
		}
	})

	t.Run("default to undefined profile errors", func(t *testing.T) {
		cfg := &Config{Profiles: map[string]Profile{"work": {}}}
		if err := applyConfigFlags(cfg, map[string]string{"default-profile": "ghost"}); err == nil {
			t.Error("expected error for undefined default profile")
		}
	})

	t.Run("bootstrap from empty default config", func(t *testing.T) {
		cfg := defaultConfig()
		err := applyConfigFlags(cfg, map[string]string{
			"roots":   "~/Work",
			"profile": "work",
			"orgs":    "acme",
			"emails":  "me@co.com",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(cfg.WorkspaceRoots, []string{"~/Work"}) || cfg.DefaultProfile != "work" {
			t.Errorf("bootstrap failed: roots=%v default=%q", cfg.WorkspaceRoots, cfg.DefaultProfile)
		}
	})
}
