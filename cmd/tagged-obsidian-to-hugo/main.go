package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/fingon/tagged-obsidian-to-hugo/internal/export"
)

type cli struct {
	VaultDir   string `arg:"" help:"Path to the Obsidian vault." name:"vault-dir" type:"path"`
	Tag        string `help:"Tag prefix to export, including #." name:"tag" required:""`
	HugoDir    string `default:"." help:"Root directory of the Hugo site." name:"hugo-dir" type:"path"`
	ContentDir string `default:"content/blog" help:"Content directory relative to hugo-dir." name:"content-dir"`
	TagLine    int    `default:"-1" help:"Line containing hashtags. -1 means last non-empty line." name:"tag-line"`
	Categories bool   `default:"true" help:"Populate Hugo categories from matching Obsidian tags." name:"categories"`
	Tags       bool   `default:"false" help:"Populate Hugo tags from matching Obsidian tags." name:"tags"`
	TimeFormat string `default:"2006-01-02T15:04:05-07:00" help:"Time format used in front matter." name:"time-format"`
	Verbose    bool   `help:"Enable verbose logging." name:"v" short:"v"`
}

func main() {
	var commandLine cli
	parser := kong.Must(&commandLine,
		kong.Name("tagged-obsidian-to-hugo"),
		kong.Description("Export tagged Obsidian notes into Hugo page bundles."),
	)
	_, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)

	logLevel := slog.LevelInfo
	if commandLine.Verbose {
		logLevel = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	err = export.Run(context.Background(), export.Config{
		Categories: commandLine.Categories,
		ContentDir: commandLine.ContentDir,
		HugoDir:    commandLine.HugoDir,
		Tag:        commandLine.Tag,
		TagLine:    commandLine.TagLine,
		Tags:       commandLine.Tags,
		TimeFormat: commandLine.TimeFormat,
		VaultDir:   commandLine.VaultDir,
	})
	if err != nil {
		slog.Error("export failed", "error", err)
		os.Exit(1)
	}
}
