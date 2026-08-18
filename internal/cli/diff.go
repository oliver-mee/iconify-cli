// Copyright 2026 Oliver Mee and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"iconify-pp-cli/internal/iconindex"
)

type diffSet struct {
	Prefix       string   `json:"prefix"`
	Added        []string `json:"added"`
	Removed      []string `json:"removed"`
	Changed      bool     `json:"changed"`
	IndexedCount int      `json:"indexed_count"`
	UpstreamNow  int      `json:"upstream_count"`
	Note         string   `json:"note,omitempty"`
}

type diffView struct {
	Sets    []diffSet `json:"sets"`
	Checked int       `json:"checked"`
	Changed int       `json:"changed"`
}

func newNovelDiffCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var sets []string
	var allChanged bool

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show which icons an upstream set added, removed, or aliased since your last index",
		Long: strings.Trim(`
Compare an icon set upstream against your local index.

Consults /last-modified first so only sets that actually moved are fetched,
then diffs the live name list against what you indexed. Use it before bumping
an icon dependency, to see whether a name you rely on disappeared.

Use this command to see what changed upstream. Do NOT use it to find problems
in your own code; use 'audit' instead.`, "\n"),
		Example: strings.Trim(`
  iconify-pp-cli diff --set lucide
  iconify-pp-cli diff --set lucide --set mdi --json
  iconify-pp-cli diff --all-changed`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "diff")
			}
			if len(sets) == 0 && !allChanged {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass --set <prefix> at least once, or --all-changed"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			db, empty, err := openIconIndexState(ctx, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if empty {
				hintEmptyIndex(cmd, dbPath)
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), diffView{Sets: make([]diffSet, 0)}, flags)
				}
				return nil
			}

			api, err := flags.newClient()
			if err != nil {
				return err
			}

			targets := sets
			if allChanged {
				targets, err = changedPrefixes(ctx, api, db.DB())
				if err != nil {
					return err
				}
			}

			view := diffView{Sets: make([]diffSet, 0, len(targets))}
			for _, prefix := range targets {
				indexed, iErr := iconindex.SetNames(ctx, db.DB(), prefix)
				if iErr != nil {
					return iErr
				}
				raw, fErr := api.Get(ctx, "/collection", map[string]string{"prefix": prefix})
				if fErr != nil {
					return fmt.Errorf("fetching %s: %w", prefix, fErr)
				}
				rows, pErr := iconindex.ParseCollection(prefix, raw)
				if pErr != nil {
					return pErr
				}
				live := make([]string, 0, len(rows))
				for _, r := range rows {
					if !r.Hidden {
						live = append(live, r.Name)
					}
				}
				added, removed := iconindex.DiffNames(indexed, live)
				d := diffSet{
					Prefix: prefix, Added: added, Removed: removed,
					Changed:      len(added) > 0 || len(removed) > 0,
					IndexedCount: len(indexed), UpstreamNow: len(live),
				}
				if len(indexed) == 0 {
					d.Note = "not in the local index; run 'index --only " + prefix + "' to establish a baseline"
				}
				if d.Changed {
					view.Changed++
				}
				view.Sets = append(view.Sets, d)
				view.Checked++
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if view.Checked == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No icon sets changed upstream since your last index.")
				return nil
			}
			for _, d := range view.Sets {
				if !d.Changed {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: unchanged (%d icons)\n", d.Prefix, d.IndexedCount)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: +%d added, -%d removed (%d -> %d)\n",
					d.Prefix, len(d.Added), len(d.Removed), d.IndexedCount, d.UpstreamNow)
				if len(d.Removed) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "  removed: %s\n", strings.Join(capList(d.Removed, 12), ", "))
				}
				if len(d.Added) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "  added:   %s\n", strings.Join(capList(d.Added, 12), ", "))
				}
				if d.Note != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  note: %s\n", d.Note)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local index database")
	cmd.Flags().StringSliceVar(&sets, "set", nil, "Icon set prefix to diff; repeatable")
	cmd.Flags().BoolVar(&allChanged, "all-changed", false, "Diff every set whose upstream timestamp advanced")
	return cmd
}

// changedPrefixes asks /last-modified which indexed sets advanced, so a broad
// diff costs one request rather than one per set.
func changedPrefixes(ctx context.Context, api iconindex.Fetcher, db *sql.DB) ([]string, error) {
	raw, err := api.Get(ctx, "/last-modified", nil)
	if err != nil {
		return nil, fmt.Errorf("fetching last-modified: %w", err)
	}
	upstream, err := iconindex.ParseLastModified(raw)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT prefix, last_modified FROM icon_sets WHERE last_modified > 0`)
	if err != nil {
		return nil, fmt.Errorf("reading indexed timestamps: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p sql.NullString
		var lm sql.NullInt64
		if err := rows.Scan(&p, &lm); err != nil {
			return nil, fmt.Errorf("scanning indexed timestamp: %w", err)
		}
		if up, ok := upstream[p.String]; ok && up != lm.Int64 {
			out = append(out, p.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating indexed timestamps: %w", err)
	}
	sort.Strings(out)
	return out, nil
}

func capList(in []string, max int) []string {
	if len(in) <= max {
		return in
	}
	out := append([]string{}, in[:max]...)
	return append(out, fmt.Sprintf("...and %d more", len(in)-max))
}
