package cmd

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	skipFrontmatter  bool
	skipDeleteHeader bool
	skipImageFix     bool
	skipAssetFix     bool
	assetPrefix      string
	indexName        string
)

var convertCmd = &cobra.Command{
	Use:   "convert [ZIP_SRC] [DEST]",
	Short: "Unzip your export and sanitize/convert markdown for static documentation sites",
	Long:  "Extracts Markdown from an Outline zip export, extracts H1 titles for frontmatter, converts parent pages to section index files, and copies asset files.",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		zipPath := args[0]
		outputPath := args[1]
		processZip(zipPath, outputPath)
	},
}

func init() {
	rootCmd.AddCommand(convertCmd)
	convertCmd.Flags().BoolVar(&skipFrontmatter, "skip-frontmatter", false, "don't add YAML frontmatter title")
	convertCmd.Flags().BoolVar(&skipDeleteHeader, "skip-header", false, "don't delete the top H1 header line")
	convertCmd.Flags().BoolVar(&skipImageFix, "skip-image-fix", false, "don't convert Outline image dimension syntax to HTML <img> tags")
	convertCmd.Flags().BoolVar(&skipAssetFix, "skip-asset-fix", false, "don't rewrite relative asset upload paths")
	convertCmd.Flags().StringVar(&assetPrefix, "asset-prefix", "", "custom URL prefix for asset link rewriting (default: auto-detected)")
	convertCmd.Flags().StringVar(&indexName, "index-name", "_index.md", "section index filename (_index.md for Hugo, index.md for Starlight/Docusaurus)")
}

func processZip(zipPath, destFolder string) {
	// collections are folders, which need _index.md for their overview description
	// nested collections are folders within collections, they also need _index.md, empty if their sibling .md file doesn't exist
	// pages are .md files in collections and nested collections
	// assets belong to collections and are stored in uploads/ of the respective collection's root

	var collections []Collection
	// TODO: switch to map[string]{}struct?
	completedCollections := make(map[string]bool)
	completedPages := make(map[string]bool)
	// populate collections via collections.json
	dir, _ := filepath.Split(zipPath)
	collectionsFile := filepath.Join(dir, "collections.json")
	data, err := os.ReadFile(collectionsFile)
	if err != nil {
		log.Fatalf("Error reading file: %v\n", err)
	}
	err = json.Unmarshal(data, &collections)
	if err != nil {
		log.Fatalf("Error during json unmarshal: %v\n", err)
	}

	// read zip
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		log.Fatalf("Error reading zipfile: %v\n", err)
	}
	defer r.Close()

	_ = os.MkdirAll(destFolder, 0o755)
	for _, f := range r.File {
		fmt.Printf("[INFO] File: %v\n", f.Name)
		targetPath := filepath.Join(destFolder, f.Name)

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(targetPath, 0o755)
			continue
		}

		body, err := f.Open()
		if err != nil {
			log.Fatalf("Error reading zip body: %v\n", err)
		}
		contents, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			log.Fatalf("Error reading zip contents: %v\n", err)
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			log.Fatalf("Error creating directory: %v\n", err)
		}
		err = os.WriteFile(targetPath, contents, 0o644)
		if err != nil {
			log.Fatalf("Error writing file: %v\n", err)
		}

		// convert while contents is still in-memory
		collectionName := getTopLevelDir(f.Name)
		collectionIndexPath := filepath.Join(destFolder, collectionName, indexName)
		collectionContent := getCollectionDesc(collectionName, collections)
		pageName := filepath.Base(f.Name)

		// pages
		if filepath.Ext(pageName) == ".md" && !f.FileInfo().IsDir() {
			completedPages[targetPath] = true
			if !skipFrontmatter {
				title := strings.TrimSuffix(pageName, filepath.Ext(pageName))
				contents, err = addFrontmatterTitle(title, contents, skipDeleteHeader)
				if err != nil {
					log.Fatalf("Error adding frontmatter: %v\n", err)
				}
			}
			if !skipAssetFix {
				contents = fixAssetLinks(contents, collectionName)
			}
		}
		err = os.WriteFile(targetPath, contents, 0o644)
		if err != nil {
			log.Fatalf("Error writing zip contents: %v\n", err)
		}

		// collections and sub-collections
		if !completedCollections[collectionName] {
			fmt.Printf("[INFO] Collection: %v\n", collectionName)
			completedCollections[collectionName] = true
			if !skipFrontmatter {
				collectionContent, err = addFrontmatterTitle(collectionName, collectionContent, skipDeleteHeader)
				if err != nil {
					log.Fatalf("Error adding frontmatter: %v\n", err)
				}
			}
			if !skipAssetFix {
				collectionContent = fixAssetLinks(collectionContent, collectionName)
			}
			err = os.WriteFile(collectionIndexPath, collectionContent, 0o644)
			if err != nil {
				log.Fatalf("Error writing collection index: %v\n", err)
			}
		}
	}

	// post-process step for nested collections (example.md -> example/_index.md)
	err = filepath.WalkDir(destFolder, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("Error accessing %s: %v\n", path, err)
			return err
		}
		if d.IsDir() {
			targetFile := filepath.Clean(path) + ".md"
			if completedPages[targetFile] {
				newPath := filepath.Join(path, "_index.md")
				if err := os.Rename(targetFile, newPath); err != nil {
					log.Fatalf("Error renaming file: %v\n", err)
				}

			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("Error WalkDir failed: %v\n", err)
	}

	fmt.Printf("[OK]   Completed conversion of %d pages and %d collection descriptions\n", len(completedPages), len(completedCollections))
}

// helper methods
func getTopLevelDir(p string) string {
	cleanPath := filepath.Clean(filepath.ToSlash(p))
	cleanPath = strings.TrimPrefix(cleanPath, "/")

	parts := strings.Split(cleanPath, "/")
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}

// get the overview description of a collection
func getCollectionDesc(name string, collections []Collection) []byte {
	for _, item := range collections {
		if item.Name == name {
			if item.Desc != "" {
				return []byte(item.Desc)
			}
		}
	}
	return nil
}

// skip-frontmatter
// skip-header
func addFrontmatterTitle(title string, contents []byte, skipDeleteHeader bool) ([]byte, error) {
	sc := bufio.NewScanner(bytes.NewReader(contents))
	var out bytes.Buffer
	out.WriteString("---\n")
	quoted, err := json.Marshal(title)
	if err != nil {
		log.Fatalf("Error quoting frontmatter title: %v\n", err)
	}
	fmTitle := fmt.Sprintf("title: %s\n", quoted)
	out.WriteString(fmTitle)
	out.WriteString("---\n")

	duplicateSuffixRegex := regexp.MustCompile(`\s*\(\d+\)$`)
	cleanTitle := duplicateSuffixRegex.ReplaceAllString(title, "")
	for sc.Scan() {
		line := sc.Text()

		target := fmt.Sprintf("# %s", cleanTitle)
		if strings.Contains(line, target) {
			if skipDeleteHeader {
				out.WriteString(line + "\n")
			}
		} else {
			out.WriteString(line + "\n")
		}

	}

	if err := sc.Err(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// skip-asset-fix
// asset-prefix
// converts markdown relative links into absolute paths since we can't use page bundles,
var assetLinkRegex = regexp.MustCompile(`(\()uploads/[^)\s"]+`)
func fixAssetLinks(contents []byte, collectionName string) []byte {
	return assetLinkRegex.ReplaceAllFunc(contents, func(match []byte) []byte {
		// match starts with "(" followed by the uploads link
		rawPath := string(match[1:])
		return fmt.Appendf(nil, "(/docs/%s/%s", collectionName, rawPath)
	})
}

// skip-image-fix
// asset-prefix
// converts Outline dimension syntax into HTML <img> tags
// keep in mind, collection overview descriptions can also have assets in them, so do check its index.md
func fixOutlineImages(content string) string {
	return ""
}
