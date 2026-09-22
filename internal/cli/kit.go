// Copyright 2026 Oliver Mee and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"iconify-pp-cli/internal/iconindex"
)

// kitManifest is the input file. A plain newline-delimited list of
// "prefix:name" is also accepted, so a manifest can start as a scratch list.
type kitManifest struct {
	Color  string   `yaml:"color"`
	Width  string   `yaml:"width"`
	Height string   `yaml:"height"`
	Flip   string   `yaml:"flip"`
	Rotate string   `yaml:"rotate"`
	Icons  []string `yaml:"icons"`
}

type kitRow struct {
	Query  string `json:"query"`
	File   string `json:"file,omitempty"`
	Status string `json:"status"` // written | unchanged | renamed | missing | error
	Note   string `json:"note,omitempty"`
	Bytes  int    `json:"bytes,omitempty"`
}

type kitView struct {
	Out       string   `json:"out"`
	Items     []kitRow `json:"items"`
	Written   int      `json:"written"`
	Unchanged int      `json:"unchanged"`
	Missing   int      `json:"missing"`
}

func newNovelKitCmd(flags *rootFlags) *cobra.Command {
	var dbPath, out, color, width, height, flip, rotate string

	cmd := &cobra.Command{
		Use:   "kit <manifest>",
		Short: "Render a directory of recoloured, uniformly sized SVGs from a manifest of icon names",
		Long: strings.Trim(`
Turn a list of icon names into a directory of SVG files at one colour and size.

Takes a YAML manifest with an 'icons' list and an optional render profile, or a
plain newline-delimited list of prefix:name. Names are resolved against the
local index first, so an icon that was renamed upstream is written under its
canonical name and reported. Re-running with an unchanged manifest rewrites
nothing.

Use this command to produce a whole directory of icons for a deck, brand kit,
or design system. Do NOT use it to fetch a single ad-hoc icon; use
'icons get' instead.`, "\n"),
		Example: strings.Trim(`
  iconify-pp-cli kit icons.yaml --out assets/
  iconify-pp-cli kit icons.txt --out assets/ --color '#404041' --width 32`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "false",
			// Writes only when --out is given; without it the command resolves
			// the manifest and prints the plan, touching nothing.
			// The manifest positional is a file path, not an id. The shipped
			// example manifest gives the live matrix a real one to render.
			"pp:happy-args": "<manifest>=examples/icons.yaml",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "kit")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a manifest path is required"))
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			raw, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("reading manifest: %w", err)
			}
			man, err := parseKitManifest(raw)
			if err != nil {
				return err
			}
			// Flags win over the manifest's profile.
			if color != "" {
				man.Color = color
			}
			if width != "" {
				man.Width = width
			}
			if height != "" {
				man.Height = height
			}
			if flip != "" {
				man.Flip = flip
			}
			if rotate != "" {
				man.Rotate = rotate
			}
			if len(man.Icons) == 0 {
				return fmt.Errorf("manifest %s lists no icons", args[0])
			}
			// #nosec G301 -- generated icon assets are checked into the caller's
			// repo and must stay world-readable like the rest of the tree.
			if out != "" {
				if err := os.MkdirAll(out, 0o755); err != nil {
					return fmt.Errorf("creating output directory: %w", err)
				}
			}

			db, err := openIconIndex(ctx, dbPath, false)
			if err != nil {
				return err
			}
			defer db.Close()
			api, err := flags.newClient()
			if err != nil {
				return err
			}

			view := kitView{Out: out, Items: make([]kitRow, 0, len(man.Icons))}
			for _, entry := range man.Icons {
				prefix, name, ok := splitIconName(entry)
				if !ok {
					view.Items = append(view.Items, kitRow{Query: entry, Status: "error", Note: "expected prefix:name"})
					view.Missing++
					continue
				}
				row := kitRow{Query: entry, Status: "written"}
				canonical := name
				if res, rErr := iconindex.Resolve(ctx, db.DB(), prefix, name); rErr == nil && res.Exists {
					canonical = res.Canonical
					if res.Aliased {
						row.Status = "renamed"
						row.Note = name + " resolves to " + res.Canonical
					}
				}
				if out == "" {
					row.Status = "planned"
					row.File = prefix + "-" + canonical + ".svg"
					if canonical != name {
						row.Note = name + " resolves to " + canonical
					}
					view.Items = append(view.Items, row)
					continue
				}
				params := map[string]string{}
				for k, v := range map[string]string{
					"color": man.Color, "width": man.Width, "height": man.Height,
					"flip": man.Flip, "rotate": man.Rotate,
				} {
					if v != "" {
						params[k] = v
					}
				}
				body, fErr := api.Get(ctx, "/"+prefix+"/"+canonical+".svg", params)
				if fErr != nil {
					row.Status, row.Note = "missing", fErr.Error()
					view.Missing++
					view.Items = append(view.Items, row)
					continue
				}
				svg := unwrapSVG(body)
				dest := filepath.Join(out, prefix+"-"+canonical+".svg")
				row.File, row.Bytes = dest, len(svg)
				// #nosec G304 -- dest is built from the user-chosen --out directory
				// and a sanitized prefix/name pair; the read only detects no-op writes.
				if existing, rErr := os.ReadFile(dest); rErr == nil && string(existing) == string(svg) {
					row.Status = "unchanged"
					view.Unchanged++
					view.Items = append(view.Items, row)
					continue
				}
				// #nosec G306 -- SVG assets are source files for the caller's repo;
				// 0600 would break every other tool that reads them.
				if wErr := os.WriteFile(dest, svg, 0o644); wErr != nil {
					row.Status, row.Note = "error", wErr.Error()
					view.Missing++
					view.Items = append(view.Items, row)
					continue
				}
				view.Written++
				view.Items = append(view.Items, row)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			for _, r := range view.Items {
				line := fmt.Sprintf("%-9s %s", r.Status, r.Query)
				if r.Note != "" {
					line += "  (" + r.Note + ")"
				}
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			if out == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d icons planned. Pass --out <dir> to render them.\n", len(view.Items))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d written, %d unchanged, %d failed -> %s\n",
				view.Written, view.Unchanged, view.Missing, out)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local index database")
	cmd.Flags().StringVar(&out, "out", "", "Directory to write SVG files into. Omit to print the plan without writing")
	cmd.Flags().StringVar(&color, "color", "", "Override the manifest colour, e.g. '#404041'")
	cmd.Flags().StringVar(&width, "width", "", "Override the manifest width")
	cmd.Flags().StringVar(&height, "height", "", "Override the manifest height")
	cmd.Flags().StringVar(&flip, "flip", "", "Flip icons: horizontal, vertical, or both")
	cmd.Flags().StringVar(&rotate, "rotate", "", "Rotate icons by 90, 180, or 270 degrees")
	return cmd
}

// parseKitManifest accepts YAML with an icons list, or a plain newline list.
func parseKitManifest(raw []byte) (kitManifest, error) {
	var man kitManifest
	if err := yaml.Unmarshal(raw, &man); err == nil && len(man.Icons) > 0 {
		return man, nil
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		man.Icons = append(man.Icons, line)
	}
	if len(man.Icons) == 0 {
		return man, fmt.Errorf("manifest lists no icons: expected YAML with an 'icons:' list, or one prefix:name per line")
	}
	return man, nil
}

// unwrapSVG strips the JSON string quoting the generated client applies to
// non-JSON bodies, so the file on disk is real SVG rather than a quoted blob.
func unwrapSVG(body []byte) []byte {
	s := string(body)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var unquoted string
		if err := yaml.Unmarshal(body, &unquoted); err == nil && strings.Contains(unquoted, "<svg") {
			return []byte(unquoted)
		}
	}
	return body
}
