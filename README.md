# stencil
CLI tool to export your Outline collections.
> [!WARNING]
> Outline will quickly rate-limit when exporting all your collections, stencil will reuse exports in the last 10 mins by default to prevent this. You can force new exports or configure this cooldown.
## Get Started
### Setup
Set OUTLINE_URL and OUTLINE_TOKEN
```bash
export OUTLINE_URL=https://outline.example.com
export OUTLINE_TOKEN=hunter2
```
Your token must have at least these permissions
- collections.export_all
- fileOperations.info
- fileOperations.redirect

### Export and download your collections
Download all collections, including private collections and attachments
```bash
stencil pull --private --attachments <path>
```
You can use flags to set your Outline credentials aswell
```bash
stencil pull --url <url> --token <token> <path>
```
Change the cooldown for reusing exports (e.g. reuse an export if one in the last 15 mins)
```bash
stencil pull --private --attachments --cooldown 15 <path>
```

### Preview with Hugo Docs
You can use stencil to convert/sanitize your Outline export and bootstrap a clean Hugo docs preview site powered by [Hextra](https://github.com/imfing/hextra). 
> [!NOTE]
> In case you don't like Hextra, you can try skipping or adjusting some sanitization options to adapt for your framework's quirks, however I can't support every framework so expect some issues.

Here is every sanitization step with their respective flag
- Add H1 header into frontmatter `--skip-frontmatter`
- Remove H1 header line `--skip-header`
- Convert Convert Outline image dimension syntax to HTML <img> tags `--skip-image-fix`
- Rewrite relative asset upload paths `--skip-asset-fix`
- Custom URL prefix for asset link rewriting (default: auto-detected) `--asset-prefix`
- Section index filename (e.g. _index.md for Hugo, index.md for Starlight/Docusaurus) `--index-name`

> [!NOTE]
> Requires [Hugo](https://gohugo.io/) installed.

```bash
stencil init ./my-hugo-site

# Standard Hugo / Hextra usage (all fixes enabled by default)
stencil convert ./export.zip ./my-hugo-site/content/docs
# Target Astro Starlight or Docusaurus
stencil convert --index-name index.md ./export.zip ./src/content/docs
# Raw export without modifying image syntax into HTMl and asset paths (e.g. Obsidian)
stencil convert --skip-image-fix --skip-asset-fix ./export.zip ./dest

cd ./my-hugo-site
hugo server
```
stencil might not cover every edge case, verify your site before deploying it to production. If you find a mismatch between Outline and your converted docs, feel free to open up an issue.

> [!WARNING]
> make sure you aren't using a version of outline where docs are duplicated 

###### Built For CEDILLE






