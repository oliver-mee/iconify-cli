// Shared helpers for the commands that read the local Iconify corpus mirror.
// Hand-authored; kept in its own file so regeneration preserves it.

package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oliver-mee/iconify-cli/internal/store"
)

// iconifyIndexDBPath resolves the mirror path, honouring an explicit --db.
func iconifyIndexDBPath(dbPath string) string {
	if dbPath != "" {
		return dbPath
	}
	return defaultDBPath("iconify-pp-cli")
}

// openIconIndex opens the mirror read-write and guarantees the corpus schema
// exists. Callers get a clear "run index first" error rather than a SQL error
// when the mirror has never been built.
func openIconIndex(ctx context.Context, dbPath string, requireCorpus bool) (*store.Store, error) {
	db, empty, err := openIconIndexState(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	if requireCorpus && empty {
		_ = db.Close()
		return nil, errIndexEmpty{path: iconifyIndexDBPath(dbPath)}
	}
	return db, nil
}

// openIconIndexState opens the mirror and reports whether the corpus is empty,
// so a caller can emit a valid empty result rather than failing. An unbuilt
// index is an empty local-cache state, not a usage or API error.
func openIconIndexState(ctx context.Context, dbPath string) (*store.Store, bool, error) {
	db, err := store.OpenWithContext(ctx, iconifyIndexDBPath(dbPath))
	if err != nil {
		return nil, false, fmt.Errorf("opening local index: %w", err)
	}
	if err := db.EnsureIconifySchema(ctx); err != nil {
		_ = db.Close()
		return nil, false, err
	}
	sets, icons, cErr := db.IconifyIndexed(ctx)
	if cErr != nil {
		_ = db.Close()
		return nil, false, cErr
	}
	return db, sets == 0 || icons == 0, nil
}

// hintEmptyIndex tells a human caller how to populate the mirror. Machine
// output stays a valid empty result so an agent can parse it.
func hintEmptyIndex(cmd *cobra.Command, dbPath string) {
	fmt.Fprintf(cmd.ErrOrStderr(),
		"no local icon index at %s\nrun: %s index\n", iconifyIndexDBPath(dbPath), invokedName())
}

// errIndexEmpty is returned when a corpus-reading command runs before `index`.
type errIndexEmpty struct{ path string }

func (e errIndexEmpty) Error() string {
	return fmt.Sprintf("local icon index at %s is empty; run '%s index' first", e.path, invokedName())
}
