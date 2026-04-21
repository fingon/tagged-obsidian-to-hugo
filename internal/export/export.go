package export

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	defaultDirMode  = 0o755
	defaultFileMode = 0o644
	indexFileName   = "index.md"
	frontMatterSep  = "---"
	draftSegment    = "draft"
	markdownExt     = ".md"
	minNoteLines    = 3
)

var (
	wikiLinkPattern             = regexp.MustCompile(`(!?)\[\[([^\]]+)\]\]`)
	markdownLinkPattern         = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)]+)\)`)
	createdFrontMatterKeys      = []string{"created", "date created"}
	lastModifiedFrontMatterKeys = []string{"last modified", "date modified"}
)

type Config struct {
	Categories bool
	ContentDir string
	HugoDir    string
	Tag        string
	TagLine    int
	Tags       bool
	TimeFormat string
	VaultDir   string
}

type Exporter struct {
	cfg              Config
	attachmentByBase map[string]*Attachment
	attachmentByRel  map[string]*Attachment
	noteByRelNoExt   map[string]*Note
	noteByStem       map[string]*Note
	noteByTitle      map[string]*Note
	notes            []*Note
}

type Attachment struct {
	AbsolutePath string
	BaseName     string
	RelativePath string
}

type Note struct {
	AbsolutePath    string
	Body            string
	BodyRaw         []byte
	Date            time.Time
	Draft           bool
	Export          bool
	FrontMatterTags []string
	LastModified    *time.Time
	RelativePath    string
	Slug            string
	TagLineIndex    int
	Title           string
}

type attachmentCopy struct {
	Source *Attachment
}

func Run(ctx context.Context, cfg Config) error {
	exporter, err := New(cfg)
	if err != nil {
		return err
	}

	return exporter.Run(ctx)
}

func New(cfg Config) (*Exporter, error) {
	if cfg.Tag == "" {
		return nil, errors.New("tag is required")
	}

	if !strings.HasPrefix(cfg.Tag, "#") {
		return nil, fmt.Errorf("tag %q must start with #", cfg.Tag)
	}

	vaultDir, err := filepath.Abs(cfg.VaultDir)
	if err != nil {
		return nil, fmt.Errorf("resolve vault dir: %w", err)
	}

	hugoDir, err := filepath.Abs(cfg.HugoDir)
	if err != nil {
		return nil, fmt.Errorf("resolve hugo dir: %w", err)
	}

	cfg.VaultDir = vaultDir
	cfg.HugoDir = hugoDir

	return &Exporter{
		attachmentByBase: map[string]*Attachment{},
		attachmentByRel:  map[string]*Attachment{},
		cfg:              cfg,
		noteByRelNoExt:   map[string]*Note{},
		noteByStem:       map[string]*Note{},
		noteByTitle:      map[string]*Note{},
		notes:            []*Note{},
	}, nil
}

func (e *Exporter) Run(ctx context.Context) error {
	if err := e.scan(ctx); err != nil {
		return err
	}

	for _, note := range e.notes {
		if !note.Export {
			continue
		}

		if err := e.writeNote(ctx, note); err != nil {
			return err
		}
	}

	return nil
}

func (e *Exporter) scan(ctx context.Context) error {
	return filepath.WalkDir(e.cfg.VaultDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk vault: %w", err)
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(e.cfg.VaultDir, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", path, err)
		}

		relativePath = filepath.ToSlash(relativePath)
		if strings.EqualFold(filepath.Ext(path), markdownExt) {
			note, shouldSkip, err := e.parseNote(path, relativePath)
			if err != nil {
				return err
			}
			if shouldSkip {
				slog.Info("skipped markdown", "path", relativePath, "reason", "not enough non-empty lines")
				return nil
			}

			e.notes = append(e.notes, note)
			e.indexNote(note)
			slog.Debug("Found markdown", "path", path)
			return nil
		}

		e.indexAttachment(&Attachment{
			AbsolutePath: path,
			BaseName:     filepath.Base(path),
			RelativePath: relativePath,
		})

		return nil
	})
}

func (e *Exporter) writeNote(ctx context.Context, note *Note) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	renderedBody, copies, err := e.rewriteBody(note)
	if err != nil {
		return err
	}

	outputDir := filepath.Join(e.cfg.HugoDir, e.cfg.ContentDir, note.Slug)
	if err := os.MkdirAll(outputDir, defaultDirMode); err != nil {
		return fmt.Errorf("create output dir for %s: %w", note.Title, err)
	}

	for destinationName, copyRequest := range copies {
		targetPath := filepath.Join(outputDir, destinationName)
		data, err := os.ReadFile(copyRequest.Source.AbsolutePath)
		if err != nil {
			return fmt.Errorf("read attachment %s: %w", copyRequest.Source.AbsolutePath, err)
		}

		if err := os.WriteFile(targetPath, data, defaultFileMode); err != nil {
			return fmt.Errorf("write attachment %s: %w", targetPath, err)
		}
	}

	indexPath := filepath.Join(outputDir, indexFileName)
	if err := os.WriteFile(indexPath, []byte(renderFrontMatter(note, renderedBody, e.cfg)), defaultFileMode); err != nil {
		return fmt.Errorf("write note %s: %w", indexPath, err)
	}

	slog.Info("exported note", "title", note.Title, "slug", note.Slug, "attachments", len(copies))
	return nil
}

func (e *Exporter) parseNote(path, relativePath string) (*Note, bool, error) {
	bodyRaw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read note %s: %w", path, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, false, fmt.Errorf("stat note %s: %w", path, err)
	}

	bodyLinesWithFrontMatter := bytes.Split(bytes.ReplaceAll(bodyRaw, []byte("\r\n"), []byte("\n")), []byte("\n"))
	frontMatter, lines, err := parseSourceFrontMatter(relativePath, bodyLinesWithFrontMatter)
	if err != nil {
		return nil, false, err
	}

	if countNonEmptyLines(lines) < minNoteLines {
		return nil, true, nil
	}

	if len(lines) == 0 || len(bytes.TrimSpace(lines[0])) == 0 {
		return nil, false, fmt.Errorf("note %s has no title line", relativePath)
	}

	tagLineIndex := e.resolveTagLineIndex(lines)
	if tagLineIndex < 0 {
		tagLineIndex = len(lines)
	}

	parsedTags := scanTags(linesAt(lines, tagLineIndex))
	exportedTags := matchingTags(parsedTags, e.cfg.Tag)
	exportNote := len(exportedTags) > 0

	bodyLines := slices.Clone(lines)
	if tagLineIndex >= 0 && tagLineIndex < len(bodyLines) {
		bodyLines = slices.Delete(bodyLines, tagLineIndex, tagLineIndex+1)
	}
	if len(bodyLines) > 0 {
		bodyLines = bodyLines[1:]
	}

	title := parseTitle(string(lines[0]))
	slug := slugify(title)
	date := info.ModTime().UTC()
	if createdTime, found, timestampErr := sourceTimestamp(frontMatter, createdFrontMatterKeys); timestampErr != nil {
		return nil, false, fmt.Errorf("parse created timestamp for %s: %w", relativePath, timestampErr)
	} else if found {
		date = createdTime.UTC()
	}

	var lastModified *time.Time
	if modifiedTime, found, timestampErr := sourceTimestamp(frontMatter, lastModifiedFrontMatterKeys); timestampErr != nil {
		return nil, false, fmt.Errorf("parse modified timestamp for %s: %w", relativePath, timestampErr)
	} else if found {
		modifiedTimeUTC := modifiedTime.UTC()
		lastModified = &modifiedTimeUTC
	}

	note := &Note{
		AbsolutePath:    path,
		Body:            string(bytes.Join(bodyLines, []byte("\n"))),
		BodyRaw:         bodyRaw,
		Date:            date,
		Draft:           hasDraftTag(parsedTags),
		Export:          exportNote,
		FrontMatterTags: formatFrontMatterTags(exportedTags, e.cfg.Tag),
		LastModified:    lastModified,
		RelativePath:    relativePath,
		Slug:            slug,
		TagLineIndex:    tagLineIndex,
		Title:           title,
	}

	slog.Debug("parsed note", "path", relativePath, "export", exportNote, "tags", parsedTags)
	return note, false, nil
}

func countNonEmptyLines(lines [][]byte) int {
	count := 0
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		count++
	}

	return count
}

func (e *Exporter) rewriteBody(note *Note) (string, map[string]attachmentCopy, error) {
	copies := map[string]attachmentCopy{}

	body, err := e.rewriteMarkdownLinks(note, note.Body, copies)
	if err != nil {
		return "", nil, err
	}

	body, err = e.rewriteWikiLinks(note, body, copies)
	if err != nil {
		return "", nil, err
	}

	return body, copies, nil
}

func (e *Exporter) rewriteMarkdownLinks(note *Note, body string, copies map[string]attachmentCopy) (string, error) {
	var rewriteErr error

	rewritten := markdownLinkPattern.ReplaceAllStringFunc(body, func(match string) string {
		if rewriteErr != nil {
			return match
		}

		submatches := markdownLinkPattern.FindStringSubmatch(match)
		if len(submatches) != 4 {
			return match
		}

		isEmbed := submatches[1] == "!"
		label := submatches[2]
		target := strings.TrimSpace(submatches[3])

		replacement, err := e.rewriteMarkdownTarget(note, isEmbed, label, target, copies)
		if err != nil {
			rewriteErr = err
			return match
		}

		return replacement
	})

	if rewriteErr != nil {
		return "", rewriteErr
	}

	return rewritten, nil
}

func (e *Exporter) rewriteWikiLinks(note *Note, body string, copies map[string]attachmentCopy) (string, error) {
	var rewriteErr error

	rewritten := wikiLinkPattern.ReplaceAllStringFunc(body, func(match string) string {
		if rewriteErr != nil {
			return match
		}

		submatches := wikiLinkPattern.FindStringSubmatch(match)
		if len(submatches) != 3 {
			return match
		}

		isEmbed := submatches[1] == "!"
		rawTarget := submatches[2]
		target := rawTarget
		label := ""
		if strings.Contains(rawTarget, "|") {
			parts := strings.SplitN(rawTarget, "|", 2)
			target = parts[0]
			label = parts[1]
		}

		replacement, err := e.rewriteWikiTarget(note, isEmbed, strings.TrimSpace(target), strings.TrimSpace(label), copies)
		if err != nil {
			rewriteErr = err
			return match
		}

		return replacement
	})

	if rewriteErr != nil {
		return "", rewriteErr
	}

	return rewritten, nil
}

func (e *Exporter) rewriteMarkdownTarget(
	note *Note,
	isEmbed bool,
	label string,
	target string,
	copies map[string]attachmentCopy,
) (string, error) {
	if isExternalTarget(target) {
		return markdownReplacement(isEmbed, label, target), nil
	}

	cleanTarget, err := unescapeTarget(target)
	if err != nil {
		return "", err
	}

	if noteTarget, ok := e.resolveMarkdownNoteTarget(note, cleanTarget); ok {
		if noteTarget.Export {
			return markdownReplacement(false, displayLabel(label, noteTarget.Title), relativeNoteLink(note.Slug, noteTarget.Slug)), nil
		}

		return displayLabel(label, noteTarget.Title), nil
	}

	attachmentTarget, ok := e.resolveMarkdownAttachmentTarget(note, cleanTarget)
	if !ok {
		return markdownReplacement(isEmbed, label, target), nil
	}

	rewrittenTarget, err := registerAttachmentCopy(copies, attachmentTarget)
	if err != nil {
		return "", err
	}

	return markdownReplacement(isEmbed || isImagePath(cleanTarget), displayLabel(label, attachmentTarget.BaseName), rewrittenTarget), nil
}

func (e *Exporter) rewriteWikiTarget(
	note *Note,
	isEmbed bool,
	target string,
	label string,
	copies map[string]attachmentCopy,
) (string, error) {
	if target == "" {
		return "", nil
	}

	if hasUnsupportedAnchor(target) {
		return displayLabel(label, baseNameWithoutExt(target)), nil
	}

	if attachmentTarget, ok := e.resolveWikiAttachmentTarget(target); ok && (isEmbed || !looksLikeNotePath(target)) {
		rewrittenTarget, err := registerAttachmentCopy(copies, attachmentTarget)
		if err != nil {
			return "", err
		}

		return markdownReplacement(isEmbed || isImagePath(target), displayLabel(label, attachmentTarget.BaseName), rewrittenTarget), nil
	}

	noteTarget, ok := e.resolveWikiNoteTarget(target)
	if !ok {
		return displayLabel(label, baseNameWithoutExt(target)), nil
	}

	if !noteTarget.Export {
		return displayLabel(label, noteTarget.Title), nil
	}

	return markdownReplacement(false, displayLabel(label, noteTarget.Title), relativeNoteLink(note.Slug, noteTarget.Slug)), nil
}

func (e *Exporter) resolveMarkdownNoteTarget(note *Note, target string) (*Note, bool) {
	cleanTarget := strings.TrimSpace(target)
	if hasUnsupportedAnchor(cleanTarget) {
		return nil, false
	}

	if !looksLikeNotePath(cleanTarget) {
		return nil, false
	}

	baseDir := pathDir(note.RelativePath)
	vaultRelative := filepath.ToSlash(filepath.Clean(filepath.Join(baseDir, cleanTarget)))
	vaultRelative = strings.TrimPrefix(vaultRelative, "./")
	noteTarget, ok := e.noteByRelNoExt[strings.TrimSuffix(strings.ToLower(vaultRelative), markdownExt)]
	if ok {
		return noteTarget, true
	}

	return nil, false
}

func (e *Exporter) resolveMarkdownAttachmentTarget(note *Note, target string) (*Attachment, bool) {
	if target == "" || strings.HasPrefix(target, "#") {
		return nil, false
	}

	baseDir := pathDir(note.RelativePath)
	vaultRelative := filepath.ToSlash(filepath.Clean(filepath.Join(baseDir, target)))
	vaultRelative = strings.TrimPrefix(vaultRelative, "./")
	attachment, ok := e.attachmentByRel[strings.ToLower(vaultRelative)]
	return attachment, ok
}

func (e *Exporter) resolveWikiAttachmentTarget(target string) (*Attachment, bool) {
	cleanTarget := filepath.ToSlash(filepath.Clean(strings.TrimSpace(target)))
	cleanTarget = strings.TrimPrefix(cleanTarget, "./")

	if strings.Contains(cleanTarget, "/") {
		attachment, ok := e.attachmentByRel[strings.ToLower(cleanTarget)]
		return attachment, ok
	}

	attachment, ok := e.attachmentByBase[strings.ToLower(cleanTarget)]
	return attachment, ok
}

func (e *Exporter) resolveWikiNoteTarget(target string) (*Note, bool) {
	cleanTarget := filepath.ToSlash(filepath.Clean(strings.TrimSpace(target)))
	cleanTarget = strings.TrimPrefix(cleanTarget, "./")
	cleanTarget = strings.TrimSuffix(cleanTarget, markdownExt)
	loweredTarget := strings.ToLower(cleanTarget)

	if strings.Contains(cleanTarget, "/") {
		note, ok := e.noteByRelNoExt[loweredTarget]
		if ok {
			return note, true
		}
	}

	stem := strings.ToLower(filepath.Base(cleanTarget))
	if note, ok := e.noteByStem[stem]; ok && note != nil {
		return note, true
	}

	note, ok := e.noteByTitle[strings.ToLower(filepath.Base(cleanTarget))]
	return note, ok
}

func (e *Exporter) indexAttachment(attachment *Attachment) {
	relativeKey := strings.ToLower(attachment.RelativePath)
	baseKey := strings.ToLower(attachment.BaseName)
	e.attachmentByRel[relativeKey] = attachment
	if _, exists := e.attachmentByBase[baseKey]; !exists {
		e.attachmentByBase[baseKey] = attachment
	}
}

func (e *Exporter) indexNote(note *Note) {
	relNoExt := strings.TrimSuffix(strings.ToLower(note.RelativePath), markdownExt)
	e.noteByRelNoExt[relNoExt] = note

	stemKey := strings.ToLower(filepath.Base(relNoExt))
	if existing, exists := e.noteByStem[stemKey]; exists && existing != nil && existing.RelativePath != note.RelativePath {
		e.noteByStem[stemKey] = nil
	} else if !exists {
		e.noteByStem[stemKey] = note
	}

	titleKey := strings.ToLower(note.Title)
	if _, exists := e.noteByTitle[titleKey]; !exists {
		e.noteByTitle[titleKey] = note
	}
}

func renderFrontMatter(note *Note, body string, cfg Config) string {
	var builder strings.Builder
	builder.WriteString(frontMatterSep)
	builder.WriteString("\n")
	fmt.Fprintf(&builder, "title: %q\n", note.Title)
	fmt.Fprintf(&builder, "date: %s\n", note.Date.Format(cfg.TimeFormat))
	if note.LastModified != nil {
		fmt.Fprintf(&builder, "lastmod: %s\n", note.LastModified.Format(cfg.TimeFormat))
	}
	if cfg.Categories {
		fmt.Fprintf(&builder, "categories: %s\n", frontMatterList(note.FrontMatterTags))
	}
	if cfg.Tags {
		fmt.Fprintf(&builder, "tags: %s\n", frontMatterList(note.FrontMatterTags))
	}
	fmt.Fprintf(&builder, "draft: %t\n", note.Draft)
	builder.WriteString(frontMatterSep)
	builder.WriteString("\n")
	if body != "" {
		builder.WriteString(body)
	}

	return builder.String()
}

func parseSourceFrontMatter(relativePath string, lines [][]byte) (map[string]any, [][]byte, error) {
	if len(lines) == 0 || string(lines[0]) != frontMatterSep {
		return nil, lines, nil
	}

	endIndex := -1
	for index := 1; index < len(lines); index++ {
		if string(lines[index]) == frontMatterSep {
			endIndex = index
			break
		}
	}
	if endIndex < 0 {
		return nil, nil, fmt.Errorf("note %s has unterminated front matter", relativePath)
	}

	frontMatterLines := bytes.Join(lines[1:endIndex], []byte("\n"))
	if len(bytes.TrimSpace(frontMatterLines)) == 0 {
		return map[string]any{}, lines[endIndex+1:], nil
	}

	frontMatter := map[string]any{}
	if err := yaml.Unmarshal(frontMatterLines, &frontMatter); err != nil {
		return nil, nil, fmt.Errorf("parse front matter for %s: %w", relativePath, err)
	}

	return frontMatter, lines[endIndex+1:], nil
}

func sourceTimestamp(frontMatter map[string]any, keys []string) (time.Time, bool, error) {
	if len(frontMatter) == 0 {
		return time.Time{}, false, nil
	}

	for _, key := range keys {
		value, found := frontMatter[key]
		if !found {
			continue
		}

		parsedTime, err := parseFrontMatterTime(value)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("key %q: %w", key, err)
		}

		return parsedTime, true, nil
	}

	return time.Time{}, false, nil
}

func parseFrontMatterTime(value any) (time.Time, error) {
	switch typedValue := value.(type) {
	case time.Time:
		return typedValue, nil
	case string:
		parsedTime, err := time.Parse(time.RFC3339, typedValue)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse %q as RFC3339: %w", typedValue, err)
		}

		return parsedTime, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp value %T", value)
	}
}

func scanTags(line []byte) []string {
	start := 0
	end := 0
	inHash := false
	multiWord := false
	hashtags := []string{}
	var prev rune

	for i, r := range line {
		var peek rune
		if i < len(line)-1 {
			peek = rune(line[i+1])
		}

		switch {
		case r == '#' && (prev == ' ' || prev == 0) && !inHash:
			start = i
			inHash = true
			end = i + 1
		case prev != ' ' && r == '#':
			end = i
		case inHash && r == ' ' && peek != '#':
			end = i
			multiWord = true
		case r == ' ' && peek == '#' && inHash:
			inHash = false
			multiWord = false
			hashtags = append(hashtags, strings.TrimSpace(string(line[start:end])))
		case !multiWord:
			end = i + 1
		}

		prev = rune(r)
	}

	if inHash {
		hashtags = append(hashtags, strings.TrimSpace(string(line[start:end])))
	}

	return hashtags
}

func matchingTags(tags []string, requestedTag string) []string {
	matches := []string{}
	for _, tag := range tags {
		switch {
		case tag == requestedTag:
			matches = append(matches, tag)
		case strings.HasPrefix(tag, requestedTag+"/"):
			matches = append(matches, tag)
		}
	}

	return matches
}

func formatFrontMatterTags(tags []string, requestedTag string) []string {
	formatted := make([]string, 0, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimPrefix(tag, requestedTag)
		trimmed = strings.TrimPrefix(trimmed, "/")
		trimmed = strings.TrimSuffix(trimmed, "#")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}

		formatted = append(formatted, titleWords(strings.ReplaceAll(trimmed, "/", " ")))
	}

	return formatted
}

func hasDraftTag(tags []string) bool {
	for _, tag := range tags {
		parts := strings.Split(strings.TrimPrefix(strings.TrimSuffix(tag, "#"), "#"), "/")
		if len(parts) == 0 {
			continue
		}

		if strings.EqualFold(parts[len(parts)-1], draftSegment) {
			return true
		}
	}

	return false
}

func parseTitle(line string) string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "# ")
	trimmed = strings.TrimPrefix(trimmed, "#")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return "untitled"
	}

	return trimmed
}

func slugify(title string) string {
	var builder strings.Builder
	lastWasDash := false

	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastWasDash = false
		case !lastWasDash:
			builder.WriteByte('-')
			lastWasDash = true
		}
	}

	return strings.Trim(builder.String(), "-")
}

func linesAt(lines [][]byte, index int) []byte {
	if index < 0 || index >= len(lines) {
		return nil
	}

	return lines[index]
}

func (e *Exporter) resolveTagLineIndex(lines [][]byte) int {
	if e.cfg.TagLine >= 0 {
		if e.cfg.TagLine >= len(lines) {
			return -1
		}

		return e.cfg.TagLine
	}

	lastIndex := len(lines) - 1
	for lastIndex >= 0 && len(bytes.TrimSpace(lines[lastIndex])) == 0 {
		lastIndex--
	}

	index := lastIndex + e.cfg.TagLine + 1
	if index < 0 || index >= len(lines) {
		return -1
	}

	return index
}

func frontMatterList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}

	return "[" + strings.Join(quoted, ", ") + "]"
}

func titleWords(value string) string {
	parts := strings.Fields(value)
	for index, part := range parts {
		runes := []rune(strings.ToLower(part))
		if len(runes) == 0 {
			continue
		}

		runes[0] = unicode.ToUpper(runes[0])
		parts[index] = string(runes)
	}

	return strings.Join(parts, " ")
}

func relativeNoteLink(fromSlug, toSlug string) string {
	relativePath, err := filepath.Rel(fromSlug, toSlug)
	if err != nil {
		return "../" + toSlug + "/"
	}

	relativePath = filepath.ToSlash(relativePath)
	if !strings.HasSuffix(relativePath, "/") {
		relativePath += "/"
	}

	return relativePath
}

func markdownReplacement(isEmbed bool, label, target string) string {
	prefix := ""
	if isEmbed {
		prefix = "!"
	}

	return fmt.Sprintf("%s[%s](%s)", prefix, label, target)
}

func displayLabel(explicitLabel, fallback string) string {
	if explicitLabel != "" {
		return explicitLabel
	}

	return fallback
}

func registerAttachmentCopy(copies map[string]attachmentCopy, attachment *Attachment) (string, error) {
	destinationName := attachment.BaseName
	if existing, exists := copies[destinationName]; exists && existing.Source.AbsolutePath != attachment.AbsolutePath {
		return "", fmt.Errorf(
			"attachment collision for %s between %s and %s",
			destinationName,
			existing.Source.RelativePath,
			attachment.RelativePath,
		)
	}

	copies[destinationName] = attachmentCopy{Source: attachment}
	return destinationName, nil
}

func isExternalTarget(target string) bool {
	parsedURL, err := url.Parse(target)
	if err != nil {
		return false
	}

	return parsedURL.Scheme != "" || strings.HasPrefix(target, "//")
}

func unescapeTarget(target string) (string, error) {
	unescaped, err := url.PathUnescape(strings.Trim(target, "<>"))
	if err != nil {
		return "", fmt.Errorf("unescape target %q: %w", target, err)
	}

	return unescaped, nil
}

func hasUnsupportedAnchor(target string) bool {
	return strings.Contains(target, "#") || strings.Contains(target, "^")
}

func looksLikeNotePath(target string) bool {
	return filepath.Ext(target) == markdownExt || filepath.Ext(target) == ""
}

func isImagePath(target string) bool {
	switch strings.ToLower(filepath.Ext(target)) {
	case ".avif", ".gif", ".jpeg", ".jpg", ".png", ".svg", ".webp":
		return true
	default:
		return false
	}
}

func baseNameWithoutExt(target string) string {
	target, _, _ = strings.Cut(target, "#")
	target, _, _ = strings.Cut(target, "^")
	base := filepath.Base(target)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func pathDir(path string) string {
	dir := filepath.Dir(path)
	if dir == "." {
		return ""
	}

	return dir
}

func SnapshotDir(path string) (string, error) {
	entries := []string{}
	err := filepath.WalkDir(path, func(currentPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(path, currentPath)
		if err != nil {
			return err
		}

		relativePath = filepath.ToSlash(relativePath)
		data, err := os.ReadFile(currentPath)
		if err != nil {
			return err
		}

		entries = append(entries, fmt.Sprintf("== %s ==\n%s", relativePath, snapshotContent(data)))
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(entries)
	return strings.Join(entries, "\n"), nil
}

func snapshotContent(data []byte) string {
	if utf8Safe(data) {
		return string(data)
	}

	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum)
}

func utf8Safe(data []byte) bool {
	for len(data) > 0 {
		r, size := utf8DecodeRune(data)
		if r == unicode.ReplacementChar && size == 1 {
			return false
		}

		data = data[size:]
	}

	return true
}

func utf8DecodeRune(data []byte) (rune, int) {
	return utf8.DecodeRune(data)
}
