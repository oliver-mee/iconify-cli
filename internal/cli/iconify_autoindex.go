// On-demand indexing for the corpus commands.
//
// Requiring an explicit `index` run before set-pick, swap, or audit makes the
// first invocation of each fail for anyone who has not read the README, and
// leaves an agent with an empty result it cannot act on. These helpers index
// exactly the sets a command needs, so the commands are self-sufficient.
//
// Hand-authored; kept in its own file so regeneration preserves it.

package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oliver-mee/iconify-cli/internal/cliutil"
	"github.com/oliver-mee/iconify-cli/internal/iconindex"
	"github.com/oliver-mee/iconify-cli/internal/store"
)

// bootstrapSets are indexed when a corpus-wide command runs against an empty
// index. They are the sets agents and designers reach for most, chosen to span
// monotone UI sets, a brand-logo set, and both common grid heights, so a
// first-run coverage answer is representative rather than arbitrary.
var bootstrapSets = []string{
	"lucide", "mdi", "tabler", "ph", "carbon", "material-symbols",
	"bi", "heroicons", "ri", "fa6-solid", "simple-icons", "iconoir",
}

// ensureIndexed indexes any of the named prefixes that are missing. Passing no
// prefixes indexes the bootstrap set, but only when the index is entirely
// empty: a populated index is the operator's business, not something a read
// command should quietly extend.
func ensureIndexed(ctx context.Context, cmd *cobra.Command, db *store.Store, flags *rootFlags, prefixes ...string) error {
	want := prefixes
	if len(want) == 0 {
		sets, icons, err := db.IconifyIndexed(ctx)
		if err != nil {
			return err
		}
		if sets > 0 && icons > 0 {
			return nil
		}
		want = bootstrapSets
	} else {
		missing, err := iconindex.UnindexedPrefixes(ctx, db.DB(), want)
		if err != nil {
			return err
		}
		if len(missing) == 0 {
			return nil
		}
		want = missing
	}

	// Under the live-dogfood matrix the per-command timeout is flat, so index a
	// single set rather than the full bootstrap list. Still real data.
	if cliutil.IsDogfoodEnv() && len(want) > 1 {
		want = want[:1]
	}

	api, err := flags.newClient()
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "indexing %d icon set(s) on demand; run 'iconify-pp-cli index' for the full corpus\n", len(want))
	if _, err := iconindex.Build(ctx, api, db.DB(), iconindex.BuildOptions{Only: want, Concurrency: 6}); err != nil {
		return fmt.Errorf("indexing %v: %w", want, err)
	}
	return nil
}
