package storage

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestMigrateLocalToS3(t *testing.T) {
	local := newLocal(t, "webp")
	fs := newFakeS3()
	ts := httptest.NewServer(fs)
	defer ts.Close()
	remote := newS3Store(t, ts.URL)

	k1, _, err := local.SaveCoverBytes(jpegBytes(t), ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	k2, _, err := local.SaveCoverBytes(pngBytes(t), ".png")
	if err != nil {
		t.Fatal(err)
	}
	// A thumbnail-style file must migrate too, keeping its exact name.
	thumb := "covers/" + k1[len("covers/"):len(k1)-len(".jpg")] + ".thumb.400.webp"
	if err := os.WriteFile(filepath.Join(local.uploadDir, thumb), jpegBytes(t), 0644); err != nil {
		t.Fatal(err)
	}

	var calls int
	stats, err := MigrateLocalToS3(local, remote, func(done, total int) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 3 || stats.Migrated != 3 || stats.Skipped != 0 || stats.Failed != 0 {
		t.Fatalf("first run stats: %+v", stats)
	}
	if calls != 3 {
		t.Errorf("emit should fire once per file, got %d", calls)
	}

	remoteKeys, err := remote.listKeys("covers/")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{k1, k2, thumb} {
		if !slices.Contains(remoteKeys, want) {
			t.Errorf("remote missing key %q, have %v", want, remoteKeys)
		}
		got, err := remote.ReadCover(want)
		if err != nil {
			t.Fatalf("read %s: %v", want, err)
		}
		loc, _ := local.ReadCover(want)
		if string(got) != string(loc) {
			t.Errorf("content mismatch for %s", want)
		}
	}

	// Second run is a no-op: everything already exists remotely.
	stats2, err := MigrateLocalToS3(local, remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats2.Migrated != 0 || stats2.Skipped != 3 || stats2.Failed != 0 {
		t.Fatalf("second run should skip all: %+v", stats2)
	}
}

func TestMigrateLocalToS3EmptyDir(t *testing.T) {
	local := newLocal(t, "avif")
	fs := newFakeS3()
	ts := httptest.NewServer(fs)
	defer ts.Close()
	stats, err := MigrateLocalToS3(local, newS3Store(t, ts.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 0 {
		t.Fatalf("expected empty migration: %+v", stats)
	}
}
