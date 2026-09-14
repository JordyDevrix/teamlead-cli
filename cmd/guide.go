package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/prompt"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var (
	guideFormat string
	guideWrite  bool
)

var guideCmd = &cobra.Command{
	Use:   "guide",
	Short: "Generate system prompts or rules for AI models to follow teamlead coordination",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil {
			cwd, _ := os.Getwd()
			root = cwd
		}

		content := ""
		filename := "TEAMLEAD.md"

		switch guideFormat {
		case "claude":
			content = prompt.GenerateClaudeMD()
			filename = "CLAUDE.md"
		case "cursor":
			content = prompt.GenerateCursorRules()
			filename = ".cursorrules"
		case "gemini":
			content = prompt.GenerateGeminiMD()
			filename = "GEMINI.md"
		default:
			content = prompt.GetAgentRulesText("general")
			filename = "TEAMLEAD.md"
		}

		if guideWrite {
			target := filepath.Join(root, filename)
			if err := os.WriteFile(target, []byte(content), 0644); err != nil {
				return err
			}
			ui.Successf("Wrote coordination instructions to %s", target)
		} else {
			cmd.Println(content)
		}

		return nil
	},
}

func init() {
	guideCmd.Flags().StringVarP(&guideFormat, "format", "f", "markdown", "Format: claude, cursor, gemini, or markdown")
	guideCmd.Flags().BoolVarP(&guideWrite, "write", "w", false, "Write rule file directly into repository root")

	rootCmd.AddCommand(guideCmd)
}
