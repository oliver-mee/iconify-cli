// The generated `feedback` parent ships without an Examples section, so its
// help fails the dogfood help probe while its subcommands pass. Setting it from
// a novel hook keeps the fix out of the generated file, which regeneration
// would overwrite.
//
// Upstream fix belongs in the generator's feedback template.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		cmd, _, err := root.Find([]string{"feedback"})
		if err != nil || cmd == nil || strings.TrimSpace(cmd.Example) != "" {
			return
		}
		cmd.Example = strings.Trim(`
  iconify-pp-cli feedback "set-pick returned nothing for a concept I expected to match"
  iconify-pp-cli feedback list
  iconify-pp-cli feedback list --json`, "\n")
	})
}
