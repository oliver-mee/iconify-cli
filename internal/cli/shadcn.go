// Copyright 2026 Oliver Mee and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"iconify-pp-cli/internal/cliutil"
	"iconify-pp-cli/internal/iconindex"
)

type shadcnRow struct {
	Query     string `json:"query"`
	Slug      string `json:"slug"`
	Canonical string `json:"canonical"`
	Exists    bool   `json:"exists"`
	Aliased   bool   `json:"aliased,omitempty"`
	Note      string `json:"note,omitempty"`
}

type shadcnView struct {
	Items   []shadcnRow `json:"items"`
	Command string      `json:"command"`
	Missing int         `json:"missing"`
}

// shadcnRunners is the allow-list of package-manager runners the --install
// path may execute. Restricting it keeps --package-manager from turning into
// an arbitrary-command flag.
var shadcnRunners = map[string][]string{
	"npx":      {"npx"},
	"pnpm dlx": {"pnpm", "dlx"},
	"pnpm":     {"pnpm", "dlx"},
	"bunx":     {"bunx"},
	"yarn dlx": {"yarn", "dlx"},
	"yarn":     {"yarn", "dlx"},
	"deno":     {"deno", "run", "-A", "npm:shadcn@latest"},
}

// shadcnInstallArgv builds the exact argv for the install run from an
// allow-listed runner plus the resolved registry slugs.
func shadcnInstallArgv(pm string, slugs []string) ([]string, error) {
	runner, ok := shadcnRunners[strings.TrimSpace(strings.ToLower(pm))]
	if !ok {
		allowed := make([]string, 0, len(shadcnRunners))
		for k := range shadcnRunners {
			allowed = append(allowed, k)
		}
		sort.Strings(allowed)
		return nil, fmt.Errorf("unsupported --package-manager %q: expected one of %s", pm, strings.Join(allowed, ", "))
	}
	argv := append([]string{}, runner...)
	if runner[0] != "deno" {
		argv = append(argv, "shadcn@latest")
	}
	argv = append(argv, "add")
	argv = append(argv, slugs...)
	return argv, nil
}

func newNovelShadcnCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var install bool
	var pm string

	cmd := &cobra.Command{
		Use:   "shadcn <prefix:name> [prefix:name...]",
		Short: "Turn Iconify icon names into shadcn.io registry slugs, and optionally install them",
		Long: strings.Trim(`
Convert Iconify icon names into shadcn.io registry items.

shadcn.io re-serves Iconify's icon sets, and the slug is derived from the icon
name by replacing the colon with a hyphen. Each name is checked against the
local index first, following aliases, so a slug is only emitted for an icon
that actually exists.

Use this command to turn an Iconify icon name into a shadcn.io registry
install. Do NOT use it to decide which icon or set to use; use 'set-pick'
first, then pass its output here.`, "\n"),
		Example: strings.Trim(`
  iconify-pp-cli shadcn lucide:rocket
  iconify-pp-cli shadcn lucide:rocket carbon:shield --json
  iconify-pp-cli shadcn lucide:rocket --install`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "shadcn")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("at least one icon name is required, as prefix:name"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			db, empty, err := openIconIndexState(ctx, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			// This command only needs the named prefixes, not the whole corpus,
			// so a cold index resolves them live rather than making the caller
			// run 'index' first. set-pick and audit genuinely need the corpus
			// and do not get this fallback.
			//
			// The same fallback applies per prefix on a warm index: a set the
			// caller never indexed would otherwise resolve to "not present in
			// <prefix>", which reports a real icon as missing.
			var live *liveResolver
			liveFor := func() (*liveResolver, error) {
				if live == nil {
					api, aErr := flags.newClient()
					if aErr != nil {
						return nil, aErr
					}
					live = newLiveResolver(api)
				}
				return live, nil
			}
			unindexed := map[string]bool{}
			if empty {
				if _, aErr := liveFor(); aErr != nil {
					return aErr
				}
			} else {
				wanted := make([]string, 0, len(args))
				for _, raw := range args {
					if prefix, _, ok := splitIconName(raw); ok {
						wanted = append(wanted, prefix)
					}
				}
				cold, uErr := iconindex.UnindexedPrefixes(ctx, db.DB(), wanted)
				if uErr != nil {
					return uErr
				}
				for _, p := range cold {
					unindexed[p] = true
				}
			}

			view := shadcnView{Items: make([]shadcnRow, 0, len(args))}
			slugs := make([]string, 0, len(args))
			for _, raw := range args {
				prefix, name, ok := splitIconName(raw)
				if !ok {
					view.Items = append(view.Items, shadcnRow{Query: raw, Note: "expected prefix:name"})
					view.Missing++
					continue
				}
				var res iconindex.Resolution
				var rErr error
				if empty || unindexed[prefix] {
					resolver, aErr := liveFor()
					if aErr != nil {
						return aErr
					}
					res, rErr = resolver.resolve(ctx, prefix, name)
				} else {
					res, rErr = iconindex.Resolve(ctx, db.DB(), prefix, name)
				}
				if rErr != nil {
					return rErr
				}
				row := shadcnRow{Query: raw, Exists: res.Exists, Aliased: res.Aliased, Canonical: res.Canonical}
				if !res.Exists {
					row.Note = "not present in " + prefix
					view.Missing++
					view.Items = append(view.Items, row)
					continue
				}
				row.Slug = "@shadcnio/" + prefix + "-" + res.Canonical
				if res.Aliased {
					row.Note = name + " resolves to " + res.Canonical
				}
				slugs = append(slugs, row.Slug)
				view.Items = append(view.Items, row)
			}
			if len(slugs) > 0 {
				argv, argvErr := shadcnInstallArgv(pm, slugs)
				if argvErr != nil {
					return argvErr
				}
				view.Command = strings.Join(argv, " ")
			}

			if install {
				if len(slugs) == 0 {
					return fmt.Errorf("nothing to install: no icon resolved")
				}
				if cliutil.IsVerifyEnv() {
					fmt.Fprintln(cmd.OutOrStdout(), "would run:", view.Command)
					return nil
				}
				argv, argvErr := shadcnInstallArgv(pm, slugs)
				if argvErr != nil {
					return argvErr
				}
				// #nosec G204 -- argv[0] comes from shadcnRunners, a fixed
				// allow-list; the remaining args are literals plus registry
				// slugs built from index-resolved icon names, and no shell
				// is involved.
				run := exec.CommandContext(ctx, argv[0], argv[1:]...)
				run.Stdout, run.Stderr, run.Stdin = cmd.OutOrStdout(), cmd.ErrOrStderr(), os.Stdin
				if runErr := run.Run(); runErr != nil {
					return fmt.Errorf("running %s: %w", view.Command, runErr)
				}
				return nil
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			for _, r := range view.Items {
				if r.Slug == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%-28s  %s\n", r.Query, r.Note)
					continue
				}
				line := fmt.Sprintf("%-28s  %s", r.Query, r.Slug)
				if r.Note != "" {
					line += "  (" + r.Note + ")"
				}
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			if view.Command != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", view.Command)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local index database")
	cmd.Flags().BoolVar(&install, "install", false, "Run the shadcn add command instead of printing it")
	cmd.Flags().StringVar(&pm, "package-manager", "npx", "Runner used to invoke shadcn: npx, pnpm dlx, bunx, yarn dlx, or deno")
	return cmd
}

// liveResolver resolves icon names straight from the API, caching each icon
// set's payload so repeated names in one invocation cost a single request.
type liveResolver struct {
	api   iconindex.Fetcher
	cache map[string]map[string]iconindex.IconRow
}

func newLiveResolver(api iconindex.Fetcher) *liveResolver {
	return &liveResolver{api: api, cache: map[string]map[string]iconindex.IconRow{}}
}

func (l *liveResolver) resolve(ctx context.Context, prefix, name string) (iconindex.Resolution, error) {
	res := iconindex.Resolution{Query: prefix + ":" + name, Prefix: prefix, Name: name, Canonical: name}
	set, ok := l.cache[prefix]
	if !ok {
		raw, err := l.api.Get(ctx, "/collection", map[string]string{"prefix": prefix})
		if err != nil {
			// An unknown prefix is a real answer here, not a transport failure.
			l.cache[prefix] = map[string]iconindex.IconRow{}
			return res, nil
		}
		rows, err := iconindex.ParseCollection(prefix, raw)
		if err != nil {
			return res, err
		}
		set = make(map[string]iconindex.IconRow, len(rows))
		for _, r := range rows {
			set[r.Name] = r
		}
		l.cache[prefix] = set
	}
	row, found := set[name]
	if !found {
		return res, nil
	}
	res.Exists = true
	res.Hidden = row.Hidden
	if row.AliasOf != "" {
		res.Aliased = true
		res.Canonical = row.AliasOf
	}
	return res, nil
}

// splitIconName parses "prefix:name". Iconify names never contain a second
// colon, so a single split is correct.
func splitIconName(s string) (prefix, name string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
