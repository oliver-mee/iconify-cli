// Copyright 2026 Oliver Mee and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"iconify-pp-cli/internal/iconindex"
)

func newNovelSetPickCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var completeOnly bool
	var license string

	cmd := &cobra.Command{
		Use:   "set-pick <concept> [concept...]",
		Short: "Rank icon sets by how many of the concepts you need each set actually covers",
		Long: strings.Trim(`
Given the icon concepts a page or deck needs, rank icon sets by coverage.

Answers the question that decides consistency: which single set can supply
every icon here, so the result does not mix five sets at five stroke weights.
Runs entirely against the local index; run 'index' first.

Use this command to choose which single icon set to standardise on. Do NOT use
it to migrate icons you already use; use 'swap'. Do NOT use it to inspect icons
already present in a codebase; use 'audit'.`, "\n"),
		Example: strings.Trim(`
  iconify-pp-cli set-pick rocket shield handshake
  iconify-pp-cli set-pick rocket shield --complete-only --json
  iconify-pp-cli set-pick "arrow right" clock --license MIT`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			// Concepts are free text, not ids, so the live matrix cannot invent
			// them. These three exercise a real multi-concept coverage query.
			"pp:happy-args": "<concept>=rocket;<concept>=shield;<concept>=clock",
			// A concept nothing matches is a legitimate empty result, not bad
			// input: there is no way to distinguish "no icon set covers this"
			// from "this word is nonsense" without inventing semantics.
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "set-pick")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("at least one concept is required"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			db, empty, err := openIconIndexState(ctx, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if empty {
				if err := ensureIndexed(ctx, cmd, db, flags); err != nil {
					return err
				}
			}

			rows, err := iconindex.CoverageBySet(ctx, db.DB(), args, 0)
			if err != nil {
				return err
			}
			if completeOnly {
				kept := rows[:0]
				for _, r := range rows {
					if r.Complete {
						kept = append(kept, r)
					}
				}
				rows = kept
			}
			if license != "" {
				want := strings.ToLower(license)
				kept := rows[:0]
				for _, r := range rows {
					if strings.Contains(strings.ToLower(r.SPDX), want) || strings.Contains(strings.ToLower(r.License), want) {
						kept = append(kept, r)
					}
				}
				rows = kept
			}
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No icon set covers any of those concepts.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SET\tCOVERED\tHEIGHT\tPALETTE\tLICENSE\tMISSING")
			for _, r := range rows {
				palette := "mono"
				if r.Palette {
					palette = "colour"
				}
				missing := "-"
				if len(r.Missing) > 0 {
					missing = strings.Join(r.Missing, ",")
				}
				fmt.Fprintf(w, "%s\t%d/%d\t%d\t%s\t%s\t%s\n",
					r.Prefix, r.Covered, r.Total, r.Height, palette, firstNonEmpty(r.SPDX, r.License, "-"), missing)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local index database")
	cmd.Flags().IntVar(&limit, "limit", 15, "Maximum sets to return")
	cmd.Flags().BoolVar(&completeOnly, "complete-only", false, "Only sets covering every concept")
	cmd.Flags().StringVar(&license, "license", "", "Only sets whose licence matches this text, e.g. MIT or Apache")
	return cmd
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
