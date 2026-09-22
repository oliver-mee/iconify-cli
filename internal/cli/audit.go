// Copyright 2026 Oliver Mee and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/oliver-mee/iconify-cli/internal/iconindex"
)

// iconRefPattern matches the ways an Iconify icon is referenced in source:
// a bare "prefix:name" literal inside quotes, an @iconify-icons/<set> import,
// and unplugin's ~icons/<set>/<name> form.
var (
	iconRefLiteral  = regexp.MustCompile(`["'` + "`" + `]([a-z0-9]{2}[a-z0-9-]*):([a-z0-9][a-z0-9-]*)["'` + "`" + `]`)
	iconRefUnplugin = regexp.MustCompile(`~icons/([a-z0-9][a-z0-9-]*)/([a-z0-9][a-z0-9-]*)`)
	iconRefPackage  = regexp.MustCompile(`@iconify[-/]icons?[-/]([a-z0-9][a-z0-9-]*)`)
)

type auditRef struct {
	Icon      string   `json:"icon"`
	Prefix    string   `json:"prefix"`
	Name      string   `json:"name"`
	Canonical string   `json:"canonical,omitempty"`
	Status    string   `json:"status"` // ok | aliased | retired | unknown-set | missing
	Files     []string `json:"files"`
}

type auditSetUse struct {
	Prefix  string `json:"prefix"`
	Name    string `json:"name,omitempty"`
	Uses    int    `json:"uses"`
	Height  int    `json:"height,omitempty"`
	Palette bool   `json:"palette"`
}

type auditView struct {
	Root      string        `json:"root"`
	Scanned   int           `json:"files_scanned"`
	Refs      []auditRef    `json:"refs"`
	Sets      []auditSetUse `json:"sets"`
	Findings  []string      `json:"findings"`
	SetSpread int           `json:"set_spread"`
}

func newNovelAuditCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var exts []string
	var showOK bool

	cmd := &cobra.Command{
		Use:   "audit [path]",
		Short: "Scan a codebase for Iconify references and report set spread, grid mismatches, and dead names",
		Long: strings.Trim(`
Find every Iconify icon a codebase references and check it against the index.

Reports how many different icon sets are in play, whether their grid heights
and palettes disagree, and which referenced names have been aliased away or
retired upstream. Runs against the local index; run 'index' first.

Use this command to inspect icons already referenced in a codebase. Do NOT use
it to pick a target set for new work; use 'set-pick'. Do NOT use it to detect
upstream changes to a set; use 'diff'.`, "\n"),
		Example: strings.Trim(`
  iconify-pp-cli audit ./src
  iconify-pp-cli audit ./src --json
  iconify-pp-cli audit . --ext .tsx --ext .vue`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			// The path positional is a directory, not an id. Scanning the CLI's
			// own source is a real scan that always exists wherever it runs.
			"pp:happy-args": "<path>=.",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit")
			}
			root := "."
			if len(args) > 0 {
				root = args[0]
			}
			if _, statErr := os.Stat(root); statErr != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("cannot scan %s: %w", root, statErr))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			db, empty, err := openIconIndexState(ctx, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			_ = empty

			refs, scanned, err := scanForIconRefs(root, exts)
			if err != nil {
				return err
			}
			// The scan matches any quoted "word:word", which in real source also
			// catches things like "sha256:..." or "prefix:name" in prose. Check
			// candidates against the authoritative icon-set list before treating
			// them as icon references, or the report is full of noise.
			if len(refs) > 0 {
				known, kErr := iconindex.KnownPrefixes(ctx, db.DB())
				if kErr != nil {
					return kErr
				}
				if len(known) == 0 {
					api, aErr := flags.newClient()
					if aErr != nil {
						return aErr
					}
					known, kErr = iconindex.LivePrefixes(ctx, api)
					if kErr != nil {
						return kErr
					}
				}
				seen := map[string]bool{}
				var needed []string
				for key := range refs {
					prefix, _, ok := splitIconName(key)
					if !ok || !known[prefix] {
						delete(refs, key)
						continue
					}
					if !seen[prefix] {
						seen[prefix] = true
						needed = append(needed, prefix)
					}
				}
				sort.Strings(needed)
				if len(needed) > 0 {
					if err := ensureIndexed(ctx, cmd, db, flags, needed...); err != nil {
						return err
					}
				}
			}
			view := auditView{Root: root, Scanned: scanned, Refs: []auditRef{}, Sets: []auditSetUse{}, Findings: []string{}}

			prefixes := map[string]int{}
			keys := make([]string, 0, len(refs))
			for k := range refs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				parts := strings.SplitN(key, ":", 2)
				prefix, name := parts[0], parts[1]
				res, rErr := iconindex.Resolve(ctx, db.DB(), prefix, name)
				if rErr != nil {
					return rErr
				}
				r := auditRef{Icon: key, Prefix: prefix, Name: name, Files: refs[key], Status: "ok"}
				switch {
				case !res.Exists:
					known, kErr := prefixKnown(ctx, db.DB(), prefix)
					if kErr != nil {
						return kErr
					}
					if !known {
						r.Status = "unknown-set"
					} else {
						r.Status = "missing"
					}
				case res.Hidden:
					r.Status = "retired"
					r.Canonical = res.Canonical
				case res.Aliased:
					r.Status = "aliased"
					r.Canonical = res.Canonical
				}
				if r.Status != "ok" || showOK {
					view.Refs = append(view.Refs, r)
				}
				if r.Status != "unknown-set" {
					prefixes[prefix] += len(r.Files)
				}
			}

			metas, err := iconindex.SetsMeta(ctx, db.DB(), keysOf(prefixes))
			if err != nil {
				return err
			}
			byPrefix := map[string]iconindex.SetMeta{}
			for _, m := range metas {
				byPrefix[m.Prefix] = m
			}
			heights := map[int]bool{}
			palettes := map[bool]bool{}
			for p, uses := range prefixes {
				m := byPrefix[p]
				view.Sets = append(view.Sets, auditSetUse{Prefix: p, Name: m.Name, Uses: uses, Height: m.Height, Palette: m.Palette})
				if m.Height > 0 {
					heights[m.Height] = true
				}
				palettes[m.Palette] = true
			}
			sort.Slice(view.Sets, func(i, j int) bool { return view.Sets[i].Uses > view.Sets[j].Uses })
			view.SetSpread = len(prefixes)

			if view.SetSpread > 1 {
				view.Findings = append(view.Findings,
					fmt.Sprintf("%d icon sets in use; standardising on one keeps stroke weight and grid consistent", view.SetSpread))
			}
			if len(heights) > 1 {
				view.Findings = append(view.Findings,
					fmt.Sprintf("mixed grid heights across sets (%s); icons will not align optically", joinInts(heights)))
			}
			if len(palettes) > 1 {
				view.Findings = append(view.Findings,
					"mixing monotone and multicolour sets; currentColor will not tint the multicolour ones")
			}
			for _, r := range view.Refs {
				switch r.Status {
				case "aliased":
					view.Findings = append(view.Findings, fmt.Sprintf("%s resolves to %s upstream", r.Icon, r.Canonical))
				case "retired":
					view.Findings = append(view.Findings, fmt.Sprintf("%s is marked hidden upstream and may disappear", r.Icon))
				case "missing":
					view.Findings = append(view.Findings, fmt.Sprintf("%s does not exist in %s", r.Icon, r.Prefix))
				}
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(prefixes) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No Iconify references found under %s (%d files scanned).\n", root, scanned)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d files scanned, %d icon sets in use\n\n", scanned, view.SetSpread)
			for _, s := range view.Sets {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-18s %3d uses  height %-4d %s\n",
					s.Prefix, s.Uses, s.Height, paletteLabel(s.Palette))
			}
			if len(view.Findings) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nFindings:")
				for _, f := range view.Findings {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", f)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local index database")
	cmd.Flags().StringSliceVar(&exts, "ext", nil, "File extensions to scan; defaults to common web source types")
	cmd.Flags().BoolVar(&showOK, "show-ok", false, "Include references that are healthy")
	return cmd
}

func scanForIconRefs(root string, exts []string) (map[string][]string, int, error) {
	if len(exts) == 0 {
		exts = []string{".ts", ".tsx", ".js", ".jsx", ".vue", ".svelte", ".astro", ".html", ".md", ".mdx", ".json", ".yaml", ".yml", ".css"}
	}
	want := map[string]bool{}
	for _, e := range exts {
		want[strings.ToLower(e)] = true
	}
	refs := map[string][]string{}
	scanned := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped rather than failing the scan
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "dist", "build", "vendor", ".next", ".nuxt", "target":
				return fs.SkipDir
			}
			return nil
		}
		if !want[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		// #nosec G304,G122 -- reading files under the repo root the user named
		// is exactly what `audit` does; path comes from WalkDir over that root,
		// is extension-filtered, and the contents are only regex-scanned.
		body, rErr := os.ReadFile(path)
		if rErr != nil {
			return nil
		}
		scanned++
		add := func(prefix, name string) {
			key := prefix + ":" + name
			for _, existing := range refs[key] {
				if existing == path {
					return
				}
			}
			refs[key] = append(refs[key], path)
		}
		for _, m := range iconRefLiteral.FindAllStringSubmatch(string(body), -1) {
			add(m[1], m[2])
		}
		for _, m := range iconRefUnplugin.FindAllStringSubmatch(string(body), -1) {
			add(m[1], m[2])
		}
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("scanning %s: %w", root, err)
	}
	return refs, scanned, nil
}

func prefixKnown(ctx context.Context, db *sql.DB, prefix string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM icon_sets WHERE prefix = ?`, prefix).Scan(&n); err != nil {
		return false, fmt.Errorf("checking prefix %s: %w", prefix, err)
	}
	return n > 0, nil
}

func keysOf(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func joinInts(set map[int]bool) string {
	vals := make([]int, 0, len(set))
	for v := range set {
		vals = append(vals, v)
	}
	sort.Ints(vals)
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, fmt.Sprintf("%d", v))
	}
	return strings.Join(parts, ", ")
}

func paletteLabel(p bool) string {
	if p {
		return "multicolour"
	}
	return "monotone"
}
