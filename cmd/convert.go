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
	skipFrontmatter    bool
	skipImageFix       bool
	skipNoticeBlockFix bool
	indexName          string
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
	convertCmd.Flags().BoolVar(&skipImageFix, "skip-image-fix", false, "don't convert Outline image dimension syntax to HTML <img> tags")
	convertCmd.Flags().BoolVar(&skipNoticeBlockFix, "skip-noticeblocks-fix", false, "don't convert Outline notice blocks to callouts")
	convertCmd.Flags().StringVar(&indexName, "index-name", "_index.md", "section index filename (_index.md for Hugo, index.md for Starlight/Docusaurus)")
}

func processZip(zipPath, destFolder string) {
	// collections are folders, which need _index.md for their overview description
	// nested collections are folders within collections, they also need _index.md, empty if their sibling .md file doesn't exist
	// pages are .md files in collections and nested collections
	// assets are stored in uploads/ of the respective folder's root

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
	defer func() { _ = r.Close() }()

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
			contents = parseLineSeparator(contents)
			if !skipFrontmatter {
				title := strings.TrimSuffix(pageName, filepath.Ext(pageName))
				contents, err = addFrontmatterTitle(title, contents)
				if err != nil {
					log.Fatalf("Error adding frontmatter: %v\n", err)
				}
			}
			if !skipNoticeBlockFix {
				contents = fixNoticeBlocks(contents)
			}
			if !skipImageFix {
				contents = fixOutlineImages(contents)
			}
		}
		err = os.WriteFile(targetPath, contents, 0o644)
		if err != nil {
			log.Fatalf("Error writing zip contents: %v\n", err)
		}

		// collections and nested collections
		if !completedCollections[collectionName] {
			fmt.Printf("[INFO] Collection: %v\n", collectionName)
			completedCollections[collectionName] = true
			collectionContent = parseLineSeparator(collectionContent)
			if !skipFrontmatter {
				collectionContent, err = addFrontmatterTitle(collectionName, collectionContent)
				if err != nil {
					log.Fatalf("Error adding frontmatter: %v\n", err)
				}
			}
			if !skipNoticeBlockFix {
				collectionContent = fixNoticeBlocks(collectionContent)
			}
			if !skipImageFix {
				collectionContent = fixOutlineImages(collectionContent)
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
				newPath := filepath.Join(path, indexName)
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

// normalizes escaped newlines and removes outline's break lines
var isolatedSlashRegex = regexp.MustCompile(`(?m)^[\s\x{00a0}]*\\$`)

func parseLineSeparator(contents []byte) []byte {
	contents = bytes.ReplaceAll(contents, []byte(`\n`), []byte("\n"))
	contents = isolatedSlashRegex.ReplaceAll(contents, []byte(""))
	return contents
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

// converts the title to frontmatter
var duplicateSuffixRegex = regexp.MustCompile(`\s*\(\d+\)$`)

func addFrontmatterTitle(title string, contents []byte) ([]byte, error) {
	cleanTitle := duplicateSuffixRegex.ReplaceAllString(title, "")
	headerRegex := regexp.MustCompile(fmt.Sprintf(`(?m)^#\s+(.*?\b%s\s*)$`, regexp.QuoteMeta(cleanTitle)))
	displayTitle := title
	if match := headerRegex.FindSubmatch(contents); len(match) > 1 {
		displayTitle = strings.TrimSpace(string(match[1]))
	}

	quoted, err := json.Marshal(displayTitle)
	if err != nil {
		return nil, fmt.Errorf("error quoting frontmatter title: %w", err)
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	fmt.Fprintf(&out, "title: %s\n", quoted)
	out.WriteString("---\n")

	sc := bufio.NewScanner(bytes.NewReader(contents))
	for sc.Scan() {
		line := sc.Text()

		if !headerRegex.MatchString(line) {
			out.WriteString(line + "\n")
		}
	}

	if err := sc.Err(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// converts Outline dimension syntax into HTML <img> tags
var imageLinkRegex = regexp.MustCompile(`!\[(.*?)\]\((.+?)\s+"(?:.*?\s*=)?\s*(\d+)x(\d+)"\s*\)`)

func fixOutlineImages(contents []byte) []byte {
	return imageLinkRegex.ReplaceAllFunc(contents, func(match []byte) []byte {
		sub := imageLinkRegex.FindSubmatch(match)
		if len(sub) == 0 {
			return match
		}
		alt := string(sub[1])
		rawURL := string(sub[2])
		width := string(sub[3])
		height := string(sub[4])

		imgSrc := rawURL
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			imgSrc = "/" + strings.TrimPrefix(rawURL, "/")
		}

		if alt != "" {
			escapedAlt := strings.ReplaceAll(alt, "'", "&#39;")
			return fmt.Appendf(nil, "<img src='%s' alt='%s' width='%s' height='%s'>", imgSrc, escapedAlt, width, height)
		}
		return fmt.Appendf(nil, "<img src='%s' width='%s' height='%s'>", imgSrc, width, height)
	})
}

// converts notice blocks into github style callouts (:::tip -> > [!TIP])
var noticeBlockRegex = regexp.MustCompile(`(?s):::(\w+)\r?\n(.*?)\r?\n:::`)

func fixNoticeBlocks(contents []byte) []byte {
	return noticeBlockRegex.ReplaceAllFunc(contents, func(match []byte) []byte {
		submatches := noticeBlockRegex.FindSubmatch(match)
		if len(submatches) < 3 {
			return match
		}

		noticeType := strings.ToUpper(string(submatches[1]))
		body := strings.TrimSpace(string(submatches[2]))

		var out bytes.Buffer
		fmt.Fprintf(&out, "> [!%s]\n", noticeType)
		for i, line := range strings.Split(body, "\n") {
			if i > 0 {
				_, err := io.WriteString(&out, "\n")
				if err != nil {
					log.Fatalf("Error fixing notice blocks: %v\n", err)
				}
			}
			fmt.Fprintf(&out, "> %s", strings.TrimRight(line, "\r"))
		}

		return out.Bytes()
	})
}
