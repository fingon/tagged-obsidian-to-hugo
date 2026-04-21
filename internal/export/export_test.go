package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"
)

func TestScanTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []byte
		want []string
	}{
		{name: "empty", in: nil, want: []string{}},
		{name: "one", in: []byte("#blog/3"), want: []string{"#blog/3"}},
		{name: "multi word", in: []byte("#blog/3/multi word#"), want: []string{"#blog/3/multi word"}},
		{name: "multiple", in: []byte("#blog/3 #blog/3/foo #other"), want: []string{"#blog/3", "#blog/3/foo", "#other"}},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assert.DeepEqual(t, scanTags(testCase.in), testCase.want)
		})
	}
}

func TestMatchingTags(t *testing.T) {
	t.Parallel()

	tags := []string{"#blog/2", "#blog/3", "#blog/3/child", "#blog/30"}
	assert.DeepEqual(t, matchingTags(tags, "#blog/3"), []string{"#blog/3", "#blog/3/child"})
}

func TestParseNoteUsesTagDateWhenFrontMatterMissing(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "Note.md")
	noteBody := "# Note\n\nBody\n\n#blog/3 #y/2026/4/21"

	assert.NilError(t, os.WriteFile(notePath, []byte(noteBody), defaultFileMode))

	exporter, err := New(Config{
		ContentDir: "content/blog",
		HugoDir:    tempDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		TimeFormat: time.RFC3339,
		VaultDir:   tempDir,
	})
	assert.NilError(t, err)

	note, shouldSkip, skipReason, err := exporter.parseNote(notePath, "Note.md")
	assert.NilError(t, err)
	assert.Assert(t, !shouldSkip)
	assert.Equal(t, skipReason, "")
	assert.Equal(t, note.Date, time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC))
	assert.Assert(t, note.LastModified != nil)
	assert.Equal(t, *note.LastModified, time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC))
	assert.Equal(t, note.Title, "Note")
	assert.Assert(t, strings.Contains(note.Body, "Body"))
	assert.Assert(t, !strings.Contains(note.Body, "#blog/3 #y/2026/4/21"))
}

func TestParseNoteReadsFrontMatterTimestamps(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "Front Matter.md")
	noteBody := `---
date created: 2026-04-20T10:00:00+03:00
date modified: 2026-04-21T11:30:00+03:00
---
# Front Matter

Body

#blog/3`

	assert.NilError(t, os.WriteFile(notePath, []byte(noteBody), defaultFileMode))

	exporter, err := New(Config{
		ContentDir: "content/blog",
		HugoDir:    tempDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		TimeFormat: time.RFC3339,
		VaultDir:   tempDir,
	})
	assert.NilError(t, err)

	note, shouldSkip, skipReason, err := exporter.parseNote(notePath, "Front Matter.md")
	assert.NilError(t, err)
	assert.Assert(t, !shouldSkip)
	assert.Equal(t, skipReason, "")
	assert.Equal(t, note.Date, time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC))
	assert.Assert(t, note.LastModified != nil)
	assert.Equal(t, *note.LastModified, time.Date(2026, 4, 21, 8, 30, 0, 0, time.UTC))
	assert.Equal(t, note.Title, "Front Matter")
	assert.Assert(t, strings.Contains(note.Body, "Body"))
	assert.Assert(t, !strings.Contains(note.Body, "date created"))
	assert.Assert(t, !strings.Contains(note.Body, "#blog/3"))
}

func TestParseNoteReadsFrontMatterAliases(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "Aliases.md")
	noteBody := `---
created: 2026-04-20T10:00:00+03:00
last modified: 2026-04-21T11:30:00+03:00
---
# Aliases

Body

#blog/3`

	assert.NilError(t, os.WriteFile(notePath, []byte(noteBody), defaultFileMode))

	exporter, err := New(Config{
		ContentDir: "content/blog",
		HugoDir:    tempDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		TimeFormat: time.RFC3339,
		VaultDir:   tempDir,
	})
	assert.NilError(t, err)

	note, shouldSkip, skipReason, err := exporter.parseNote(notePath, "Aliases.md")
	assert.NilError(t, err)
	assert.Assert(t, !shouldSkip)
	assert.Equal(t, skipReason, "")
	assert.Equal(t, note.Date, time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC))
	assert.Assert(t, note.LastModified != nil)
	assert.Equal(t, *note.LastModified, time.Date(2026, 4, 21, 8, 30, 0, 0, time.UTC))
}

func TestParseNoteReadsFrontMatterTimestampsWithoutOffsetColon(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "Front Matter Basic Offset.md")
	noteBody := `---
date created: 2026-04-20T10:00:00+0300
date modified: 2026-04-21T11:30:00+0300
---
# Front Matter Basic Offset

Body

#blog/3`

	assert.NilError(t, os.WriteFile(notePath, []byte(noteBody), defaultFileMode))

	exporter, err := New(Config{
		ContentDir: "content/blog",
		HugoDir:    tempDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		TimeFormat: time.RFC3339,
		VaultDir:   tempDir,
	})
	assert.NilError(t, err)

	note, shouldSkip, skipReason, err := exporter.parseNote(notePath, "Front Matter Basic Offset.md")
	assert.NilError(t, err)
	assert.Assert(t, !shouldSkip)
	assert.Equal(t, skipReason, "")
	assert.Equal(t, note.Date, time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC))
	assert.Assert(t, note.LastModified != nil)
	assert.Equal(t, *note.LastModified, time.Date(2026, 4, 21, 8, 30, 0, 0, time.UTC))
}

func TestParseNoteReadsFrontMatterAliasesWithoutOffsetColon(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "Aliases Basic Offset.md")
	noteBody := `---
created: 2026-04-20T10:00:00+0300
last modified: 2026-04-21T11:30:00+0300
---
# Aliases Basic Offset

Body

#blog/3`

	assert.NilError(t, os.WriteFile(notePath, []byte(noteBody), defaultFileMode))

	exporter, err := New(Config{
		ContentDir: "content/blog",
		HugoDir:    tempDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		TimeFormat: time.RFC3339,
		VaultDir:   tempDir,
	})
	assert.NilError(t, err)

	note, shouldSkip, skipReason, err := exporter.parseNote(notePath, "Aliases Basic Offset.md")
	assert.NilError(t, err)
	assert.Assert(t, !shouldSkip)
	assert.Equal(t, skipReason, "")
	assert.Equal(t, note.Date, time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC))
	assert.Assert(t, note.LastModified != nil)
	assert.Equal(t, *note.LastModified, time.Date(2026, 4, 21, 8, 30, 0, 0, time.UTC))
}

func TestParseNoteRejectsInvalidFrontMatterTimestamp(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "Broken.md")
	noteBody := `---
date created: nope
---
# Broken

Body

#blog/3`

	assert.NilError(t, os.WriteFile(notePath, []byte(noteBody), defaultFileMode))

	exporter, err := New(Config{
		ContentDir: "content/blog",
		HugoDir:    tempDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		TimeFormat: time.RFC3339,
		VaultDir:   tempDir,
	})
	assert.NilError(t, err)

	_, shouldSkip, skipReason, err := exporter.parseNote(notePath, "Broken.md")
	assert.Assert(t, !shouldSkip)
	assert.Equal(t, skipReason, "")
	assert.ErrorContains(t, err, "parse created timestamp")
	assert.ErrorContains(t, err, "one of")
}

func TestParseNoteFillsMissingTimestampFieldsFromTagDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		body             string
		wantDate         time.Time
		wantLastModified time.Time
	}{
		{
			name: "fills missing created",
			body: `---
last modified: 2026-04-21T11:30:00+03:00
---
# Missing Created

Body

#blog/3 #y/2026/4/20`,
			wantDate:         time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
			wantLastModified: time.Date(2026, 4, 21, 8, 30, 0, 0, time.UTC),
		},
		{
			name: "fills missing modified",
			body: `---
created: 2026-04-20T10:00:00+03:00
---
# Missing Modified

Body

#blog/3 #y/2026/4/21`,
			wantDate:         time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
			wantLastModified: time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC),
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			notePath := filepath.Join(tempDir, "Note.md")
			assert.NilError(t, os.WriteFile(notePath, []byte(testCase.body), defaultFileMode))

			exporter, err := New(Config{
				ContentDir: "content/blog",
				HugoDir:    tempDir,
				Tag:        "#blog/3",
				TagLine:    -1,
				TimeFormat: time.RFC3339,
				VaultDir:   tempDir,
			})
			assert.NilError(t, err)

			note, shouldSkip, skipReason, err := exporter.parseNote(notePath, "Note.md")
			assert.NilError(t, err)
			assert.Assert(t, !shouldSkip)
			assert.Equal(t, skipReason, "")
			assert.Equal(t, note.Date, testCase.wantDate)
			assert.Assert(t, note.LastModified != nil)
			assert.Equal(t, *note.LastModified, testCase.wantLastModified)
		})
	}
}

func TestParseNotePrefersOlderTagDateOverCreatedFrontMatter(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "Older Tag Date.md")
	noteBody := `---
created: 2026-04-21T10:00:00+03:00
last modified: 2026-04-22T11:30:00+03:00
---
# Older Tag Date

Body

#blog/3 #y/2026/4/20`

	assert.NilError(t, os.WriteFile(notePath, []byte(noteBody), defaultFileMode))

	exporter, err := New(Config{
		ContentDir: "content/blog",
		HugoDir:    tempDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		TimeFormat: time.RFC3339,
		VaultDir:   tempDir,
	})
	assert.NilError(t, err)

	note, shouldSkip, skipReason, err := exporter.parseNote(notePath, "Older Tag Date.md")
	assert.NilError(t, err)
	assert.Assert(t, !shouldSkip)
	assert.Equal(t, skipReason, "")
	assert.Equal(t, note.Date, time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	assert.Assert(t, note.LastModified != nil)
	assert.Equal(t, *note.LastModified, time.Date(2026, 4, 22, 8, 30, 0, 0, time.UTC))
}

func TestParseNoteSkipsExportWithoutUsableDateMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing front matter and tag date",
			body: "# Missing Date\n\nBody\n\n#blog/3",
		},
		{
			name: "invalid tag date",
			body: "# Bad Date\n\nBody\n\n#blog/3 #y/2026/2/30",
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			notePath := filepath.Join(tempDir, "Note.md")
			assert.NilError(t, os.WriteFile(notePath, []byte(testCase.body), defaultFileMode))

			exporter, err := New(Config{
				ContentDir: "content/blog",
				HugoDir:    tempDir,
				Tag:        "#blog/3",
				TagLine:    -1,
				TimeFormat: time.RFC3339,
				VaultDir:   tempDir,
			})
			assert.NilError(t, err)

			note, shouldSkip, skipReason, err := exporter.parseNote(notePath, "Note.md")
			assert.NilError(t, err)
			assert.Assert(t, shouldSkip)
			assert.Equal(t, skipReason, skipReasonDate)
			assert.Assert(t, note == nil)
		})
	}
}

func TestParseNoteAllowsNonExportNoteWithoutDateMetadata(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "Note.md")
	noteBody := "# Note\n\nBody\n\n#other"
	assert.NilError(t, os.WriteFile(notePath, []byte(noteBody), defaultFileMode))

	exporter, err := New(Config{
		ContentDir: "content/blog",
		HugoDir:    tempDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		TimeFormat: time.RFC3339,
		VaultDir:   tempDir,
	})
	assert.NilError(t, err)

	note, shouldSkip, skipReason, err := exporter.parseNote(notePath, "Note.md")
	assert.NilError(t, err)
	assert.Assert(t, !shouldSkip)
	assert.Equal(t, skipReason, "")
	assert.Assert(t, !note.Export)
	assert.Assert(t, note.Date.IsZero())
	assert.Assert(t, note.LastModified == nil)
	assert.Equal(t, note.Title, "Note")
}

func TestParseNoteSkipsShortMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		fileName string
	}{
		{
			name:     "empty file",
			body:     "",
			fileName: "Empty.md",
		},
		{
			name:     "two non-empty lines",
			body:     "# Short\n\n#blog/3",
			fileName: "Short.md",
		},
		{
			name: "front matter does not count",
			body: `---
created: 2026-04-20T10:00:00+03:00
---
# Front Matter

#blog/3`,
			fileName: "FrontMatterOnly.md",
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			notePath := filepath.Join(tempDir, testCase.fileName)
			assert.NilError(t, os.WriteFile(notePath, []byte(testCase.body), defaultFileMode))

			exporter, err := New(Config{
				ContentDir: "content/blog",
				HugoDir:    tempDir,
				Tag:        "#blog/3",
				TagLine:    -1,
				TimeFormat: time.RFC3339,
				VaultDir:   tempDir,
			})
			assert.NilError(t, err)

			note, shouldSkip, skipReason, err := exporter.parseNote(notePath, testCase.fileName)
			assert.NilError(t, err)
			assert.Assert(t, shouldSkip)
			assert.Equal(t, skipReason, skipReasonShort)
			assert.Assert(t, note == nil)
		})
	}
}

func TestRewriteWikiTargetDowngradesUnsupportedAnchors(t *testing.T) {
	t.Parallel()

	exporter, err := New(Config{
		ContentDir: "content/blog",
		HugoDir:    t.TempDir(),
		Tag:        "#blog/3",
		TimeFormat: time.RFC3339,
		VaultDir:   t.TempDir(),
	})
	assert.NilError(t, err)

	got, err := exporter.rewriteWikiTarget(&Note{Slug: "source"}, false, "other-note#heading", "", map[string]attachmentCopy{})
	assert.NilError(t, err)
	assert.Equal(t, got, "other-note")
}

func TestRunMatchesGolden(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")
	siteDir := filepath.Join(tempDir, "site")

	assert.NilError(t, copyDir(filepath.Join("testdata", "vault"), vaultDir))
	assert.NilError(t, copyDir(filepath.Join("testdata", "site"), siteDir))
	setFixtureTimes(t, vaultDir, time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC))

	err := Run(context.Background(), Config{
		Categories: true,
		ContentDir: "content/blog",
		HugoDir:    siteDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		Tags:       true,
		TimeFormat: time.RFC3339,
		VaultDir:   vaultDir,
	})
	assert.NilError(t, err)

	snapshot, err := SnapshotDir(siteDir)
	assert.NilError(t, err)
	assert.Equal(t,
		strings.TrimRight(snapshot, "\n"),
		strings.TrimRight(string(golden.Get(t, filepath.Join("golden", "basic.txt"))), "\n"),
	)
}

func TestRunDetectsAttachmentCollision(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")
	siteDir := filepath.Join(tempDir, "site")

	assert.NilError(t, os.MkdirAll(filepath.Join(vaultDir, "one"), defaultDirMode))
	assert.NilError(t, os.MkdirAll(filepath.Join(vaultDir, "two"), defaultDirMode))
	assert.NilError(t, os.MkdirAll(siteDir, defaultDirMode))

	noteBody := "# Collision\n\n![[one/file.txt]] ![[two/file.txt]]\n\n#blog/3 #y/2026/4/21"
	assert.NilError(t, os.WriteFile(filepath.Join(vaultDir, "collision.md"), []byte(noteBody), defaultFileMode))
	assert.NilError(t, os.WriteFile(filepath.Join(vaultDir, "one", "file.txt"), []byte("one"), defaultFileMode))
	assert.NilError(t, os.WriteFile(filepath.Join(vaultDir, "two", "file.txt"), []byte("two"), defaultFileMode))

	err := Run(context.Background(), Config{
		Categories: true,
		ContentDir: "content/blog",
		HugoDir:    siteDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		Tags:       false,
		TimeFormat: time.RFC3339,
		VaultDir:   vaultDir,
	})
	assert.ErrorContains(t, err, "attachment collision")
}

func TestRunSkipsShortMarkdownFiles(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")
	siteDir := filepath.Join(tempDir, "site")

	assert.NilError(t, os.MkdirAll(vaultDir, defaultDirMode))
	assert.NilError(t, os.MkdirAll(siteDir, defaultDirMode))

	validBody := "# Valid\n\nBody\n\n#blog/3 #y/2026/4/21"
	shortBody := "# Short\n\n#blog/3"
	assert.NilError(t, os.WriteFile(filepath.Join(vaultDir, "valid.md"), []byte(validBody), defaultFileMode))
	assert.NilError(t, os.WriteFile(filepath.Join(vaultDir, "short.md"), []byte(shortBody), defaultFileMode))
	assert.NilError(t, os.WriteFile(filepath.Join(vaultDir, "empty.md"), []byte(""), defaultFileMode))

	err := Run(context.Background(), Config{
		ContentDir: "content/blog",
		HugoDir:    siteDir,
		Tag:        "#blog/3",
		TagLine:    -1,
		TimeFormat: time.RFC3339,
		VaultDir:   vaultDir,
	})
	assert.NilError(t, err)

	_, err = os.Stat(filepath.Join(siteDir, "content/blog", "valid", "index.md"))
	assert.NilError(t, err)
	_, err = os.Stat(filepath.Join(siteDir, "content/blog", "short"))
	assert.Assert(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(siteDir, "content/blog", "empty"))
	assert.Assert(t, os.IsNotExist(err))
}

func copyDir(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dstDir, relativePath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, defaultDirMode)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, defaultFileMode)
	})
}

func setFixtureTimes(t *testing.T, root string, modTime time.Time) {
	t.Helper()

	err := filepath.Walk(root, func(path string, _ os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Chtimes(path, modTime, modTime)
	})
	assert.NilError(t, err)
}
