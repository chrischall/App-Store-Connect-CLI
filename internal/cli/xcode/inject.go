package xcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"
	"howett.net/plist"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

type xcodeInjectManifest struct {
	Values  map[string]any              `json:"values,omitempty"`
	Outputs []xcodeInjectManifestOutput `json:"outputs"`
}

type xcodeInjectManifestOutput struct {
	Type     string         `json:"type"`
	Path     string         `json:"path"`
	Source   string         `json:"source,omitempty"`
	Values   map[string]any `json:"values,omitempty"`
	Contents string         `json:"contents,omitempty"`
}

type xcodeInjectOptions struct {
	ManifestPath string
	SetValues    []string
	Overwrite    bool
	DryRun       bool
}

type xcodeInjectResult struct {
	ManifestPath string                  `json:"manifest_path"`
	DryRun       bool                    `json:"dry_run"`
	Outputs      []xcodeInjectFileResult `json:"outputs"`
}

type xcodeInjectFileResult struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Source string `json:"source,omitempty"`
	Action string `json:"action"`
	Bytes  int64  `json:"bytes,omitempty"`
}

func XcodeInjectCommand() *ffcli.Command {
	fs := flag.NewFlagSet("xcode inject", flag.ExitOnError)

	manifestPath := fs.String("manifest", "", "Path to deployment metadata manifest JSON (required)")
	overwrite := fs.Bool("overwrite", false, "Replace existing output files")
	dryRun := fs.Bool("dry-run", false, "Validate and print the files that would be generated without writing")
	var setValues shared.MultiStringFlag
	fs.Var(&setValues, "set", "Override a manifest value as key=value (repeatable)")
	output := shared.BindOutputFlags(fs)

	return &ffcli.Command{
		Name:       "inject",
		ShortUsage: "asc xcode inject --manifest FILE [flags]",
		ShortHelp:  "[experimental] Generate Xcode deployment metadata files from a manifest.",
		LongHelp: `[experimental] Generate Xcode deployment metadata files from a manifest.

The manifest is JSON with top-level "values" and "outputs" fields. Outputs can
generate plist, json, or text files, and can copy declared assets such as app
icons into asset catalog paths. Relative output paths and copy sources resolve
from the manifest directory.

String values support ${key} placeholders from manifest values and repeated
--set key=value overrides.

Example manifest:
  {
    "values": {
      "bundle_id": "com.example.app",
      "app_name": "Example",
      "version": "1.2.3",
      "build_number": "42"
    },
    "outputs": [
      {
        "type": "plist",
        "path": "Generated/Info.generated.plist",
        "values": {
          "CFBundleIdentifier": "${bundle_id}",
          "CFBundleDisplayName": "${app_name}",
          "CFBundleShortVersionString": "${version}",
          "CFBundleVersion": "${build_number}"
        }
      },
      {
        "type": "copy",
        "source": "Assets/AppIcon.appiconset/Contents.json",
        "path": "App/Assets.xcassets/AppIcon.appiconset/Contents.json"
      }
    ]
  }

Examples:
  asc xcode inject --manifest .asc/deployment.json --dry-run --output json
  asc xcode inject --manifest .asc/deployment.json --set version=1.2.4 --set build_number=43 --overwrite`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) > 0 {
				return shared.UsageErrorf("unexpected argument(s): %s", strings.Join(args, " "))
			}
			result, err := runXcodeInject(xcodeInjectOptions{
				ManifestPath: strings.TrimSpace(*manifestPath),
				SetValues:    []string(setValues),
				Overwrite:    *overwrite,
				DryRun:       *dryRun,
			})
			if err != nil {
				var usageErr xcodeInjectUsageError
				if errors.As(err, &usageErr) {
					return shared.UsageError(usageErr.Error())
				}
				return fmt.Errorf("xcode inject: %w", err)
			}

			return shared.PrintOutputWithRenderers(
				result,
				*output.Output,
				*output.Pretty,
				func() error {
					asc.RenderTable([]string{"type", "action", "path", "source"}, xcodeInjectResultRows(result))
					return nil
				},
				func() error {
					asc.RenderMarkdown([]string{"type", "action", "path", "source"}, xcodeInjectResultRows(result))
					return nil
				},
			)
		},
	}
}

type xcodeInjectUsageError struct {
	message string
}

func (e xcodeInjectUsageError) Error() string {
	return e.message
}

func newXcodeInjectUsageError(format string, args ...any) error {
	return xcodeInjectUsageError{message: fmt.Sprintf(format, args...)}
}

func runXcodeInject(opts xcodeInjectOptions) (xcodeInjectResult, error) {
	opts.ManifestPath = strings.TrimSpace(opts.ManifestPath)
	if opts.ManifestPath == "" {
		return xcodeInjectResult{}, newXcodeInjectUsageError("--manifest is required")
	}

	manifest, err := readXcodeInjectManifest(opts.ManifestPath)
	if err != nil {
		return xcodeInjectResult{}, err
	}
	if len(manifest.Outputs) == 0 {
		return xcodeInjectResult{}, newXcodeInjectUsageError("manifest outputs must include at least one entry")
	}

	values := make(map[string]any, len(manifest.Values)+len(opts.SetValues))
	for key, value := range manifest.Values {
		values[key] = value
	}
	if err := applyXcodeInjectSetValues(values, opts.SetValues); err != nil {
		return xcodeInjectResult{}, err
	}

	baseDir := filepath.Dir(opts.ManifestPath)
	result := xcodeInjectResult{
		ManifestPath: opts.ManifestPath,
		DryRun:       opts.DryRun,
		Outputs:      make([]xcodeInjectFileResult, 0, len(manifest.Outputs)),
	}

	for i, output := range manifest.Outputs {
		fileResult, err := renderXcodeInjectOutput(baseDir, values, output, opts)
		if err != nil {
			return xcodeInjectResult{}, fmt.Errorf("output %d: %w", i+1, err)
		}
		result.Outputs = append(result.Outputs, fileResult)
	}

	return result, nil
}

func readXcodeInjectManifest(path string) (xcodeInjectManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return xcodeInjectManifest{}, err
	}

	var manifest xcodeInjectManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return xcodeInjectManifest{}, newXcodeInjectUsageError("failed to parse --manifest: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return xcodeInjectManifest{}, newXcodeInjectUsageError("failed to parse --manifest: multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return xcodeInjectManifest{}, newXcodeInjectUsageError("failed to parse --manifest: multiple JSON values")
	}
	return manifest, nil
}

func applyXcodeInjectSetValues(values map[string]any, setValues []string) error {
	for _, entry := range setValues {
		key, value, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return newXcodeInjectUsageError("--set values must use key=value")
		}
		values[key] = strings.TrimSpace(value)
	}
	return nil
}

func renderXcodeInjectOutput(baseDir string, values map[string]any, output xcodeInjectManifestOutput, opts xcodeInjectOptions) (xcodeInjectFileResult, error) {
	outputType := strings.ToLower(strings.TrimSpace(output.Type))
	targetPath := resolveXcodeInjectPath(baseDir, strings.TrimSpace(output.Path))
	if targetPath == "" {
		return xcodeInjectFileResult{}, newXcodeInjectUsageError("path is required")
	}

	switch outputType {
	case "plist":
		renderedValues, err := renderXcodeInjectValue(output.Values, values)
		if err != nil {
			return xcodeInjectFileResult{}, err
		}
		payload, err := plist.Marshal(renderedValues, plist.XMLFormat)
		if err != nil {
			return xcodeInjectFileResult{}, err
		}
		return writeXcodeInjectBytes(targetPath, outputType, "", payload, opts)
	case "json":
		renderedValues, err := renderXcodeInjectValue(output.Values, values)
		if err != nil {
			return xcodeInjectFileResult{}, err
		}
		payload, err := json.MarshalIndent(renderedValues, "", "  ")
		if err != nil {
			return xcodeInjectFileResult{}, err
		}
		payload = append(payload, '\n')
		return writeXcodeInjectBytes(targetPath, outputType, "", payload, opts)
	case "text":
		renderedContents, err := renderXcodeInjectString(output.Contents, values)
		if err != nil {
			return xcodeInjectFileResult{}, err
		}
		return writeXcodeInjectBytes(targetPath, outputType, "", []byte(renderedContents), opts)
	case "copy":
		sourcePath := resolveXcodeInjectPath(baseDir, strings.TrimSpace(output.Source))
		if sourcePath == "" {
			return xcodeInjectFileResult{}, newXcodeInjectUsageError("source is required for copy outputs")
		}
		payload, err := os.ReadFile(sourcePath)
		if err != nil {
			return xcodeInjectFileResult{}, err
		}
		return writeXcodeInjectBytes(targetPath, outputType, sourcePath, payload, opts)
	default:
		return xcodeInjectFileResult{}, newXcodeInjectUsageError("type must be one of plist, json, text, copy")
	}
}

func renderXcodeInjectValue(value any, values map[string]any) (any, error) {
	switch typed := value.(type) {
	case string:
		return renderXcodeInjectString(typed, values)
	case []any:
		rendered := make([]any, 0, len(typed))
		for _, item := range typed {
			renderedItem, err := renderXcodeInjectValue(item, values)
			if err != nil {
				return nil, err
			}
			rendered = append(rendered, renderedItem)
		}
		return rendered, nil
	case map[string]any:
		rendered := make(map[string]any, len(typed))
		for key, item := range typed {
			renderedItem, err := renderXcodeInjectValue(item, values)
			if err != nil {
				return nil, err
			}
			rendered[key] = renderedItem
		}
		return rendered, nil
	default:
		return value, nil
	}
}

func renderXcodeInjectString(input string, values map[string]any) (string, error) {
	rendered := input
	for range len(values) + 1 {
		before := rendered
		for _, key := range sortedXcodeInjectValueKeys(values) {
			placeholder := "${" + key + "}"
			if !strings.Contains(rendered, placeholder) {
				continue
			}
			rendered = strings.ReplaceAll(rendered, placeholder, fmt.Sprint(values[key]))
		}
		if !strings.Contains(rendered, "${") {
			return rendered, nil
		}
		if rendered == before {
			break
		}
	}
	if start := strings.Index(rendered, "${"); start != -1 {
		if end := strings.Index(rendered[start+2:], "}"); end != -1 {
			name := rendered[start+2 : start+2+end]
			return "", newXcodeInjectUsageError("missing value for placeholder %q", name)
		}
	}
	return rendered, nil
}

func sortedXcodeInjectValueKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j]) || (len(keys[i]) == len(keys[j]) && keys[i] < keys[j])
	})
	return keys
}

func writeXcodeInjectBytes(path, outputType, source string, payload []byte, opts xcodeInjectOptions) (xcodeInjectFileResult, error) {
	result := xcodeInjectFileResult{
		Type:   outputType,
		Path:   path,
		Source: source,
		Bytes:  int64(len(payload)),
	}
	if !opts.Overwrite {
		if _, err := os.Lstat(path); err == nil {
			return xcodeInjectFileResult{}, newXcodeInjectUsageError("output path %q already exists; use --overwrite", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return xcodeInjectFileResult{}, err
		}
	}
	if opts.DryRun {
		if outputType == "copy" {
			result.Action = "would_copy"
		} else {
			result.Action = "would_write"
		}
		return result, nil
	}
	if _, err := shared.WriteFileNoSymlinkOverwrite(path, bytes.NewReader(payload), 0o644, ".asc-inject-*", ".asc-inject-backup-*"); err != nil {
		return xcodeInjectFileResult{}, err
	}
	if outputType == "copy" {
		result.Action = "copied"
	} else {
		result.Action = "written"
	}
	return result, nil
}

func resolveXcodeInjectPath(baseDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}

func xcodeInjectResultRows(result xcodeInjectResult) [][]string {
	rows := make([][]string, 0, len(result.Outputs))
	for _, output := range result.Outputs {
		rows = append(rows, []string{output.Type, output.Action, output.Path, output.Source})
	}
	if len(rows) == 0 {
		return [][]string{{"", "", "", ""}}
	}
	return rows
}
