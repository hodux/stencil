# stencil

CLI tool to export and transform Outline collections.

> [!WARNING]
> Outline will quickly rate-limit when exporting all your collections, stencil reuses exports in the last 10 mins by default to prevent this. You can force new exports or configure this cooldown.

## Get Started

### Setup

In Outline settings, create a new API token with at least these permissions

- collections.export_all
- fileOperations.info
- fileOperations.redirect

Set OUTLINE_URL and OUTLINE_TOKEN

```bash
export OUTLINE_URL=https://outline.example.com
export OUTLINE_TOKEN=hunter2
```

### Exporting

Download all collections, including private collections and attachments

```bash
stencil pull --private --attachments <path>
```

Change the cooldown for reusing exports (e.g. reuse an export if one in the last 15 mins)

```bash
stencil pull --private --attachments --cooldown 15 <path>
```

### Transforming

You can use stencil to convert/sanitize your Outline exports.

> [!WARNING]
> stencil is a WIP and might not cover every edge case, verify your site before deploying it to production. If you find a mismatch between Outline and your converted docs, feel free to open up an issue.

Here is every sanitization step with their respective flag

- Add H1 header into frontmatter `--skip-frontmatter`
- Convert Outline image dimension syntax to HTML <img> tags `--skip-image`
- Convert notice blocks to Github style callouts `--skip-noticeblocks`
- Section index filename (e.g. _index.md for Hugo, index.md for Starlight/Docusaurus) `--index-name`

```bash
# Bootstrap a Hugo docs site with Hextra to quickly preview transformations.
stencil init ./my-hugo-site

# Standard Hugo / Hextra usage (all fixes enabled by default)
stencil convert ./export.zip ./my-hugo-site/content/docs
# Target Astro Starlight or Docusaurus
stencil convert --index-name index.md ./export.zip ./src/content/docs
# Raw export without modifying image syntax into HTML (e.g. Obsidian)
stencil convert --skip-image ./export.zip ./dest
```
