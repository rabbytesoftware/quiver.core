package domain

import "testing"

func TestArrow_VersionFor(t *testing.T) {
	version := ArrowManifest{
		ArrowMeta:    ArrowMeta{Version: "v1.0.0"},
		InstalledRef: "v1.0.0",
	}
	latest := ArrowManifest{
		ArrowMeta:    ArrowMeta{Version: "v2.0.0"},
		InstalledRef: "",
	}

	arrow := &Arrow{
		Namespace: Namespace("github.com/valve/steamcmd"),
		Versions: map[string]ArrowManifest{
			"v1.0.0": version,
			"latest": latest,
		},
	}

	t.Run("empty ref resolves to latest key", func(t *testing.T) {
		v, ok := arrow.VersionFor("")
		if !ok {
			t.Fatal("expected ok=true for empty ref")
		}
		if v.Version != "v2.0.0" {
			t.Errorf("expected version v2.0.0, got %q", v.Version)
		}
	})

	t.Run("exact ref found", func(t *testing.T) {
		v, ok := arrow.VersionFor("v1.0.0")
		if !ok {
			t.Fatal("expected ok=true for existing ref")
		}
		if v.Version != "v1.0.0" {
			t.Errorf("expected version v1.0.0, got %q", v.Version)
		}
	})

	t.Run("ref not found returns false", func(t *testing.T) {
		_, ok := arrow.VersionFor("v9.9.9")
		if ok {
			t.Error("expected ok=false for missing ref")
		}
	})

	t.Run("nil arrow returns false", func(t *testing.T) {
		var a *Arrow
		_, ok := a.VersionFor("v1.0.0")
		if ok {
			t.Error("expected ok=false for nil Arrow")
		}
	})
}
