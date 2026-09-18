package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [DEST]",
	Short: "Bootstraps a Hugo site with the Hextra docs theme",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		siteDir := args[0]
		bootstrapSite(siteDir)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func bootstrapSite(siteDir string) {
	fmt.Printf("[INFO] Initializing new Hugo site in %s\n", siteDir)
	now := time.Now()

	hugoInit := exec.Command("hugo", "new", "site", siteDir)
	hugoInit.Stderr = os.Stderr
	if err := hugoInit.Run(); err != nil {
		log.Fatalf("Failed to create Hugo site (ensure folder doesn't exist or is empty): %v", err)
	}

	modInit := exec.Command("hugo", "mod", "init", "github.com/local/docs-preview")
	modInit.Dir = siteDir
	modInit.Stderr = os.Stderr
	if err := modInit.Run(); err != nil {
		log.Fatalf("Failed to initialize Hugo module: %v", err)
	}

	configPath := filepath.Join(siteDir, "hugo.toml")
	hextraConfig := `baseURL = 'https://example.org/'
locale = 'en-us'
title = 'Stencil Preview'
enableGitInfo = false

[module]
  [[module.imports]]
    path = "github.com/imfing/hextra"
  [[module.mounts]]
    source = 'content'
    target = 'content'
  [[module.mounts]]
    source = 'content'
    target = 'static'
    files = ['!**/*.md', '!**/*.markdown']

[markup]
  [markup.goldmark]
    [markup.goldmark.renderer]
      unsafe = true
  [markup.tableOfContents]
    endLevel = 4
    startLevel = 2
`
	err := os.WriteFile(configPath, []byte(hextraConfig), 0644)
	if err != nil {
		log.Fatalf("Failed to write config file: %v", err)
	}

	contentDir := filepath.Join(siteDir, "content")
	if err := os.MkdirAll(contentDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create content directory: %v", err)
	}

	homeIndexMD := `---
title: Home
---

Click <a href="/docs">here</a> to view your imported Outline collections.  
You can edit this file in ` + "`" + `/content/_index.md` + "`" + `
`
	err = os.WriteFile(filepath.Join(contentDir, "_index.md"), []byte(homeIndexMD), 0644)
	if err != nil {
		log.Fatalf("Failed to write home _index.md: %v", err)
	}

	docsDir := filepath.Join(contentDir, "docs")
	if err := os.MkdirAll(docsDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create content/docs directory: %v", err)
	}

	docsIndexMD := `---
title: Docs
---

Welcome to your Stencil Preview 🎉  
You can view your imported collections on the left  
You can also edit this file in ` + "`" + `/content/docs/_index.md` + "`" + `
`
	err = os.WriteFile(filepath.Join(docsDir, "_index.md"), []byte(docsIndexMD), 0644)
	if err != nil {
		log.Fatalf("Failed to write docs _index.md: %v", err)
	}

	duration := time.Since(now).Round(time.Microsecond)
	fmt.Printf("[OK]   Hugo site with Hextra theme is ready (%v)\n", duration)
	fmt.Printf("[INFO] Proceed by running these commands:\n 1. stencil clean ./my-export.zip %s/content/docs\n 2. cd %s\n 3. hugo server\n", siteDir, siteDir)
}
