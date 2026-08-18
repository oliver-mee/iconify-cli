// Raw-body handlers for the two endpoints that do not return JSON.
//
// /{prefix}/{name}.svg serves image/svg+xml and /{prefix}.css serves text/css.
// The generated endpoint commands run every response through a JSON assertion,
// which sees the leading '<' of an SVG and reports it as an auth failure, so
// both commands are unusable as generated. These handlers replace the RunE
// with a raw-bytes path while keeping the flags, help, and MCP surface the
// spec produced.
//
// Hand-authored in its own file so regeneration preserves it.

package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// svgRenderFlags are the /{prefix}/{name}.svg query parameters.
var svgRenderFlags = []string{"color", "width", "height", "flip", "rotate", "box"}

func rawParamsFrom(cmd *cobra.Command, names []string) map[string]string {
	params := map[string]string{}
	for _, n := range names {
		f := cmd.Flags().Lookup(n)
		if f == nil || !f.Changed {
			continue
		}
		if v := f.Value.String(); v != "" && v != "false" {
			params[n] = v
		}
	}
	return params
}

// rawAsset is the machine-readable envelope for a non-JSON body. Emitting the
// bare SVG or CSS under --json would hand an agent something it cannot parse,
// so structured callers get the content as a string field instead.
type rawAsset struct {
	Icon        string `json:"icon,omitempty"`
	Prefix      string `json:"prefix"`
	Name        string `json:"name,omitempty"`
	ContentType string `json:"content_type"`
	Bytes       int    `json:"bytes"`
	File        string `json:"file,omitempty"`
	Content     string `json:"content"`
}

// writeRaw sends the body to --output when set, otherwise to stdout. Machine
// output modes get the envelope; humans and files get the bytes themselves.
func writeRaw(cmd *cobra.Command, flags *rootFlags, body []byte, out string, asset rawAsset) error {
	asset.Bytes = len(body)
	asset.File = out
	if out != "" {
		// #nosec G306 -- --output writes a public asset (SVG/CSS/JSON) the user
		// asked for by path; 0600 would break downstream build tooling.
		if err := os.WriteFile(out, body, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", out, err)
		}
	}
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		asset.Content = string(body)
		return printJSONFiltered(cmd.OutOrStdout(), asset, flags)
	}
	if out != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s (%d bytes)\n", out, len(body))
		return nil
	}
	_, err := cmd.OutOrStdout().Write(body)
	return err
}

func installRawIconHandlers(root *cobra.Command, flags *rootFlags) {
	iconsCmd, _, err := root.Find([]string{"icons"})
	if err != nil || iconsCmd == nil {
		return
	}
	for _, sub := range iconsCmd.Commands() {
		switch sub.Name() {
		case "get":
			bindRawSVG(sub, flags)
		case "css":
			bindRawCSS(sub, flags)
		}
	}
}

func bindRawSVG(cmd *cobra.Command, flags *rootFlags) {
	var out string
	cmd.Flags().StringVarP(&out, "output", "o", "", "Write the SVG to this file instead of stdout")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && cmd.Flags().NFlag() == 0 {
			return cmd.Help()
		}
		if dryRunOK(flags) {
			return writeDryRun(cmd.OutOrStdout(), flags, "icons get")
		}
		if len(args) < 2 {
			_ = cmd.Usage()
			return usageErr(fmt.Errorf("both an icon set prefix and an icon name are required, e.g. 'icons get lucide house'"))
		}
		ctx, cancel := boundCtx(cmd.Context(), flags)
		defer cancel()
		api, err := flags.newClient()
		if err != nil {
			return err
		}
		body, err := api.Get(ctx, "/"+args[0]+"/"+args[1]+".svg", rawParamsFrom(cmd, svgRenderFlags))
		if err != nil {
			return err
		}
		return writeRaw(cmd, flags, unwrapSVG(body), out, rawAsset{
			Icon: args[0] + ":" + args[1], Prefix: args[0], Name: args[1],
			ContentType: "image/svg+xml",
		})
	}
}

func bindRawCSS(cmd *cobra.Command, flags *rootFlags) {
	var out string
	cmd.Flags().StringVarP(&out, "output", "o", "", "Write the CSS to this file instead of stdout")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && cmd.Flags().NFlag() == 0 {
			return cmd.Help()
		}
		if dryRunOK(flags) {
			return writeDryRun(cmd.OutOrStdout(), flags, "icons css")
		}
		if len(args) < 1 {
			_ = cmd.Usage()
			return usageErr(fmt.Errorf("an icon set prefix is required, e.g. 'icons css lucide --icons home,star'"))
		}
		icons := ""
		if f := cmd.Flags().Lookup("icons"); f != nil {
			icons = f.Value.String()
		}
		if strings.TrimSpace(icons) == "" {
			_ = cmd.Usage()
			return usageErr(fmt.Errorf("--icons is required, as a comma separated list of icon names"))
		}
		ctx, cancel := boundCtx(cmd.Context(), flags)
		defer cancel()
		api, err := flags.newClient()
		if err != nil {
			return err
		}
		body, err := api.Get(ctx, "/"+args[0]+".css", map[string]string{"icons": icons})
		if err != nil {
			return err
		}
		return writeRaw(cmd, flags, unwrapSVG(body), out, rawAsset{
			Prefix: args[0], ContentType: "text/css",
		})
	}
}

func init() {
	registerNovelCommand(installRawIconHandlers)
}
