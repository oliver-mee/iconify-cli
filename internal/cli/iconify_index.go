// `index` builds the local mirror of the Iconify corpus.
// pp:data-source auto
//
// The generated `sync` command handles the spec's declared resources, but the
// corpus does not fit that shape: /collections returns one object keyed by
// prefix rather than a list, and icon names exist only inside per-prefix
// /collection responses. This command owns that fan-out.

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/oliver-mee/iconify-cli/internal/cliutil"
	"github.com/oliver-mee/iconify-cli/internal/iconindex"
)

func newNovelIndexCmd(flags *rootFlags) *cobra.Command {
	var indexTimeout time.Duration
	var dbPath string
	var only []string
	var force bool
	var concurrency int

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Build the local icon index so search and analysis work offline",
		Long: strings.Trim(`
Mirror every icon set and icon name into local SQLite.

Consults /last-modified first and re-pulls only the sets that actually changed,
so a repeat run costs two requests when nothing moved upstream. Run this once
before set-pick, swap, audit, diff, or shadcn.`, "\n"),
		Example: strings.Trim(`
  iconify-pp-cli index
  iconify-pp-cli index --only lucide --only mdi
  iconify-pp-cli index --force --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "index")
			}
			// The root --timeout bounds one API request. A full index fans out
			// across every icon set and legitimately runs for minutes, so the
			// 1 minute default would abort it partway. Honour --timeout only
			// when the caller set it explicitly.
			ctx, cancel := indexCtx(cmd, flags, indexTimeout)
			defer cancel()

			db, err := openIconIndex(ctx, dbPath, false)
			if err != nil {
				return err
			}
			defer db.Close()

			api, err := flags.newClient()
			if err != nil {
				return err
			}

			human := wantsHumanTable(cmd.OutOrStdout(), flags)
			opts := iconindex.BuildOptions{Concurrency: concurrency, Only: only, Force: force}
			// A full index fans out across 236 sets and takes well over the
			// verifier's and dogfood's per-command timeout, so curtail to a
			// single small set when running under either harness. Real data,
			// bounded work: never substitute mock responses.
			if len(opts.Only) == 0 && (cliutil.IsVerifyEnv() || cliutil.IsDogfoodEnv()) {
				opts.Only = []string{"bi"}
			}
			if human {
				opts.OnProgress = func(done, total int, prefix string) {
					fmt.Fprintf(cmd.ErrOrStderr(), "\rindexing %d/%d  %-28s", done, total, prefix)
				}
			}
			res, err := iconindex.Build(ctx, api, db.DB(), opts)
			if human {
				fmt.Fprintln(cmd.ErrOrStderr())
			}
			if err != nil {
				return err
			}
			if !human {
				return printJSONFiltered(cmd.OutOrStdout(), res, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "indexed %d sets, %d icons in %dms\n", res.Sets, res.Icons, res.DurationMS)
			if res.Skipped > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%d sets unchanged upstream, skipped\n", res.Skipped)
			}
			if len(res.Failed) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%d sets failed: %s\n", len(res.Failed), strings.Join(res.Failed, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local index database")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Index only these icon set prefixes")
	cmd.Flags().BoolVar(&force, "force", false, "Reindex sets even when unchanged upstream")
	cmd.Flags().IntVar(&concurrency, "concurrency", 8, "Parallel icon set fetches")
	cmd.Flags().DurationVar(&indexTimeout, "index-timeout", 30*time.Minute, "Overall deadline for the whole index build")
	return cmd
}

// indexCtx bounds a whole index build. An explicit --timeout wins, because the
// caller asked for it; otherwise the build gets its own generous deadline.
func indexCtx(cmd *cobra.Command, flags *rootFlags, fallback time.Duration) (context.Context, context.CancelFunc) {
	if root := cmd.Root(); root != nil {
		if f := root.PersistentFlags().Lookup("timeout"); f != nil && f.Changed {
			return boundCtx(cmd.Context(), flags)
		}
	}
	if fallback <= 0 {
		fallback = 30 * time.Minute
	}
	return context.WithTimeout(cmd.Context(), fallback)
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNovelIndexCmd(flags))
	})
}
