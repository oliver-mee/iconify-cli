// Copyright 2026 Oliver Mee and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/oliver-mee/iconify-cli/internal/iconindex"
)

type swapView struct {
	From    string              `json:"from"`
	To      string              `json:"to"`
	Items   []iconindex.SwapRow `json:"items"`
	Covered int                 `json:"covered"`
	Renamed int                 `json:"renamed"`
	Missing int                 `json:"missing"`
	// Unindexed names the requested sets that carry no icons locally. A
	// swap verdict is only meaningful when both sets are indexed, so this
	// field is how the caller learns the answer was withheld rather than
	// computed.
	Unindexed []string `json:"unindexed,omitempty"`
	Note      string   `json:"note,omitempty"`
}

func newNovelSwapCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var icons []string
	var all bool

	cmd := &cobra.Command{
		Use:   "swap <from-set> <to-set>",
		Short: "Map icons from one set to another, classifying each as covered, renamed, or missing",
		Long: strings.Trim(`
Cost out a migration between icon sets before committing to it.

Each name is matched against the target set's canonical names first, then its
aliases, so an icon that merely changed name is reported as renamed rather than
missing. Runs against the local index; run 'index' first.

Use this command when you have already chosen a target set and need to migrate
specific existing icons to it. Do NOT use it to choose the target set in the
first place; use 'set-pick' instead.`, "\n"),
		Example: strings.Trim(`
  iconify-pp-cli swap mdi lucide --icons home,account,cog,rocket
  iconify-pp-cli swap fa6-solid lucide --all --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			// Two positional set prefixes plus an explicit icon list, so the
			// happy path exercises a real cross-set join.
			"pp:happy-args": "<from-set>=mdi;<to-set>=lucide;--icons=home,cog,rocket",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 2 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("both a source set and a target set are required"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "swap")
			}
			from, to := args[0], args[1]
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			db, empty, err := openIconIndexState(ctx, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			_ = empty
			if err := ensureIndexed(ctx, cmd, db, flags, from, to); err != nil {
				return err
			}

			// CrossSet only indexes the target set, so an unindexed source
			// would silently classify every requested name against `to`
			// alone and report a migration as free. Refuse to answer until
			// both sides are actually in the index.
			unindexed, err := iconindex.UnindexedPrefixes(ctx, db.DB(), []string{from, to})
			if err != nil {
				return err
			}
			if len(unindexed) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"not in the local index: %s\nrun: iconify-pp-cli index --only %s\n",
					strings.Join(unindexed, ", "), strings.Join(unindexed, " --only "))
				withheld := swapView{
					From: from, To: to,
					Items:     make([]iconindex.SwapRow, 0),
					Unindexed: unindexed,
					Note:      "no verdict: " + strings.Join(unindexed, ", ") + " missing from the local index",
				}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), withheld, flags)
				}
				fmt.Fprintln(cmd.OutOrStdout(), withheld.Note)
				return nil
			}

			names := icons
			view := swapView{From: from, To: to}
			if all {
				names, err = iconindex.SetNames(ctx, db.DB(), from)
				if err != nil {
					return err
				}
				view.Note = fmt.Sprintf("compared every visible icon in %s", from)
			}
			if len(names) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass --icons with names, or --all to compare the whole source set"))
			}

			rows, err := iconindex.CrossSet(ctx, db.DB(), from, to, names)
			if err != nil {
				return err
			}
			view.Items = rows
			for _, r := range rows {
				switch r.Status {
				case "covered":
					view.Covered++
				case "renamed":
					view.Renamed++
				default:
					view.Missing++
				}
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No icons to compare.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ICON\tSTATUS\tTARGET")
			for _, r := range rows {
				target := r.Target
				if target == "" {
					target = "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Status, target)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d covered, %d renamed, %d missing\n", view.Covered, view.Renamed, view.Missing)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local index database")
	cmd.Flags().StringSliceVar(&icons, "icons", nil, "Icon names to map, comma separated")
	cmd.Flags().BoolVar(&all, "all", false, "Map every visible icon in the source set")
	return cmd
}
