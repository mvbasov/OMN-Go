# Color scheme tokens

This document shows the CSS custom properties (the tokens) that a color scheme sets. For each token, it shows the part of an OMN-Go page that the token colors.

## How a scheme works

Section 1 of `backend/frontend/html/css/OMN-Go/omn-go-core.css` holds the palette. The palette is the one place that holds a color value. Each token has a light value `--omn-light-<name>` and a dark value `--omn-dark-<name>`.

The theme maps below the palette give each token `--omn-<name>` a palette value. The light map is in `:root`. The dark map is in `:root[data-theme="dark"]`, and the media query `prefers-color-scheme: dark` holds a copy of it for the theme "auto". A rule of OMN-Go uses `var(--omn-<name>)`, and never a color value. `TestColorValuesLiveOnlyInThePalette` and `TestDarkMapsAreTheSame` hold this.

A scheme is one marked block in `css/omn-go-custom.css`. Each page loads this file last. The block is one `:root` rule that sets new palette values. The theme maps read the palette, thus the block needs no dark rule and no media query:

```css
/* ====OMN-GO-SCHEME-BEGIN==== */
/* OMN-Go scheme: Purple
   The ColorSchemes page writes this block. Change the scheme on that page. */
:root {
  --omn-light-accent: #8e44ad;
  --omn-dark-accent: #c39bd3;
  --omn-font-size: 18px;
}
body {
  font-size: var(--omn-font-size);
}
/* ====OMN-GO-SCHEME-END==== */
```

The ColorSchemes note makes this block. It is a separate page, and it is not part of the application. It reads the palette from `omn-go-core.css` when it starts, thus its editor shows each token of the installed version. A scheme keeps only the values that differ from the default palette. The note keeps the schemes in `user_json/omn-go-schemes.json`.

A rule of your own in `css/omn-go-custom.css` can also set a palette value. Write the rule outside the marked block, because the next scheme replaces the block.

The editor page does not load `css/omn-go-custom.css`. Thus a scheme does not change the editor. The editor keeps the values of `omn-go-core.css`. This document names the editor parts that use a token, but a scheme does not change them.

## The pictures

- Each picture is a part of a real OMN-Go page on a phone screen, 412 CSS pixels wide.
- Each picture shows the default palette of OMN-Go, with no scheme. Each token has one picture for the light palette and one for the dark palette.
- A pink dashed frame marks the part that the token colors.
- The strip above the picture gives the token, its default value with a sample of the color, and the palette.
- A test set each token to a different color and compared the page before and after. For each color token, the framed part changed. For `--omn-bg`, the frame marks the content area, and the page body behind it changed.

A token that ends in `-hover` colors a control under a mouse pointer. Its picture shows that state.

Some tokens have no picture. [Tokens with no picture](#tokens-with-no-picture) gives the part of the page that each of them colors.

## Summary

| Group | Token | Light | Dark |
| --- | --- | --- | --- |
| Surfaces | [`--omn-bg`](#--omn-bg) | `#f9f9f9` | `#1b1d1e` |
| Surfaces | [`--omn-bg-surface`](#--omn-bg-surface) | `#ffffff` | `#25282a` |
| Surfaces | [`--omn-bg-band`](#--omn-bg-band) | `#e8e8e8` | `#2a2d2f` |
| Surfaces | [`--omn-bg-inset`](#--omn-bg-inset) | `#f5f5f5` | `#303336` |
| Surfaces | [`--omn-bg-hover`](#--omn-bg-hover) | `#d0d0d0` | `#3a3e41` |
| Surfaces | [`--omn-bg-metadata`](#--omn-bg-metadata) | `#e9ecef` | `#2e3235` |
| Surfaces | [`--omn-bg-toc`](#--omn-bg-toc) | `#fafafa` | `#232628` |
| Text | [`--omn-text`](#--omn-text) | `#333333` | `#d4d4d4` |
| Text | [`--omn-text-strong`](#--omn-text-strong) | `#1a1a1a` | `#e8eaed` |
| Text | [`--omn-text-heading`](#--omn-text-heading) | `#444444` | `#c4c7cb` |
| Text | [`--omn-text-muted`](#--omn-text-muted) | `#555555` | `#9aa0a6` |
| Text | [`--omn-text-faint`](#--omn-text-faint) | `#666666` | `#8a9096` |
| Borders | [`--omn-border`](#--omn-border) | `#cccccc` | `#45494d` |
| Borders | [`--omn-border-soft`](#--omn-border-soft) | `#dddddd` | `#3a3e41` |
| Borders | [`--omn-border-card`](#--omn-border-card) | `#e1e4e8` | `#3a3e41` |
| Borders | [`--omn-border-strong`](#--omn-border-strong) | `#999999` | `#5a5f63` |
| Accent and state | [`--omn-accent`](#--omn-accent) | `#0056b3` | `#4d9fff` |
| Accent and state | [`--omn-accent-hover`](#--omn-accent-hover) | `#004494` | `#6cb0ff` |
| Accent and state | [`--omn-success`](#--omn-success) | `#28a745` | `#28a745` |
| Accent and state | [`--omn-success-hover`](#--omn-success-hover) | `#218838` | `#218838` |
| Accent and state | [`--omn-danger`](#--omn-danger) | `#dc3545` | `#dc3545` |
| Accent and state | [`--omn-info`](#--omn-info) | `#17a2b8` | `#17a2b8` |
| Accent and state | [`--omn-secondary`](#--omn-secondary) | `#6c757d` | `#6c757d` |
| Tags | [`--omn-tag-bg`](#--omn-tag-bg) | `#e7e7e7` | `#3a3d40` |
| Tags | [`--omn-tag-border`](#--omn-tag-border) | `#505050` | `#808080` |
| Tags | [`--omn-tag-selected`](#--omn-tag-selected) | `#888888` | `#5a5f63` |
| Search hits | [`--omn-search-hit-bg`](#--omn-search-hit-bg) | `#ffe08a` | `#7a5c14` |
| Search hits | [`--omn-search-hit-text`](#--omn-search-hit-text) | `#1a1a1a` | `#ffeab0` |
| Tables | [`--omn-table-border`](#--omn-table-border) | `#4d6bfe` | `#4d6bfe` |
| Tables | [`--omn-table-th-bg`](#--omn-table-th-bg) | `#cccccc` | `#3a3d40` |
| Tables | [`--omn-table-th-text`](#--omn-table-th-text) | `#000000` | `#e8eaed` |
| Tables | [`--omn-table-row-odd`](#--omn-table-row-odd) | `#eeeeee` | `#2a2d2f` |
| Tables | [`--omn-table-row-even`](#--omn-table-row-even) | `#ffffff` | `#25282a` |
| Tables | [`--omn-table-text`](#--omn-table-text) | `#000000` | `#d4d4d4` |
| Code blocks | [`--omn-code-bg`](#--omn-code-bg) | `#f3f3f3` | `#303336` |
| Code blocks | [`--omn-code-text`](#--omn-code-text) | `#444` | `#c9d1d9` |
| Code blocks | [`--omn-code-comment`](#--omn-code-comment) | `#697070` | `#8b949e` |
| Code blocks | [`--omn-code-punctuation`](#--omn-code-punctuation) | `#444a` | `#c9d1d966` |
| Code blocks | [`--omn-code-string`](#--omn-code-string) | `#800` | `#a5d6ff` |
| Code blocks | [`--omn-code-variable`](#--omn-code-variable) | `#ab5656` | `#ffa657` |
| Code blocks | [`--omn-code-literal`](#--omn-code-literal) | `#695` | `#79c0ff` |
| Code blocks | [`--omn-code-built-in`](#--omn-code-built-in) | `#397300` | `#7ee787` |
| Code blocks | [`--omn-code-meta`](#--omn-code-meta) | `#1f7199` | `#79c0ff` |
| Code blocks | [`--omn-code-meta-string`](#--omn-code-meta-string) | `#38a` | `#a5d6ff` |
| Code blocks | [`--omn-code-tag-name`](#--omn-code-tag-name) | `#444444` | `#7ee787` |
| Code blocks | [`--omn-code-tag-attr`](#--omn-code-tag-attr) | `#444444` | `#79c0ff` |
| Text | [`--omn-text-dim`](#tokens-with-no-picture) | `#888888` | `#7a8086` |
| Borders | [`--omn-border-title`](#tokens-with-no-picture) | `#eaecef` | `#3a3e41` |
| Accent and state | [`--omn-on-accent`](#tokens-with-no-picture) | `#ffffff` | `#ffffff` |
| Links | [`--omn-link`](#tokens-with-no-picture) | `#0000ee` | `#9e9eff` |
| Links | [`--omn-link-visited`](#tokens-with-no-picture) | `#551a8b` | `#d0adf0` |
| Links | [`--omn-link-hover`](#tokens-with-no-picture) | `#0000ee` | `#9e9eff` |
| Files page | [`--omn-file-app`](#tokens-with-no-picture) | `#9a5b00` | `#f0a13c` |
| Files page | [`--omn-file-alert`](#tokens-with-no-picture) | `#b3261e` | `#ff8a80` |
| Files page | [`--omn-file-keep`](#tokens-with-no-picture) | `#1f7a35` | `#5ec97a` |
| Files page | [`--omn-file-derived`](#tokens-with-no-picture) | `#12798a` | `#4fc3d7` |
| Files page | [`--omn-file-plain`](#tokens-with-no-picture) | `#6b6b6b` | `#9aa0a6` |
| Files page | [`--omn-files-source-bg`](#tokens-with-no-picture) | `#4c6b8a` | `#3f5a75` |
| Start page | [`--omn-start-notes-bg`](#tokens-with-no-picture) | `#0056b3` | `#1c5ea8` |
| Start page | [`--omn-start-bookmarks-bg`](#tokens-with-no-picture) | `#17a2b8` | `#14707e` |
| Start page | [`--omn-start-btn-text`](#tokens-with-no-picture) | `#ffffff` | `#eef3f8` |
| State dots | [`--omn-state-ok`](#tokens-with-no-picture) | `#3fb950` | `#3fb950` |
| State dots | [`--omn-state-warn`](#tokens-with-no-picture) | `#d29922` | `#d29922` |
| State dots | [`--omn-state-info`](#tokens-with-no-picture) | `#4d9fff` | `#4d9fff` |
| State dots | [`--omn-state-off`](#tokens-with-no-picture) | `#8b949e` | `#8b949e` |
| State dots | [`--omn-state-error`](#tokens-with-no-picture) | `#f85149` | `#f85149` |
| State dots | [`--omn-state-alert`](#tokens-with-no-picture) | `#ff5555` | `#ff5555` |
| JS console | [`--omn-console-bg`](#tokens-with-no-picture) | `#1e1e1e` | `#1e1e1e` |
| JS console | [`--omn-console-text`](#tokens-with-no-picture) | `#00ff00` | `#00ff00` |
| JS console | [`--omn-console-line`](#tokens-with-no-picture) | `#f8f8f2` | `#f8f8f2` |
| JS console | [`--omn-console-warn`](#tokens-with-no-picture) | `#ffb86c` | `#ffb86c` |
| JS console | [`--omn-console-error`](#tokens-with-no-picture) | `#ff5555` | `#ff5555` |
| JS console | [`--omn-console-border`](#tokens-with-no-picture) | `#555555` | `#555555` |
| JS console | [`--omn-console-head-bg`](#tokens-with-no-picture) | `#333333` | `#333333` |
| JS console | [`--omn-console-head-text`](#tokens-with-no-picture) | `#ffffff` | `#ffffff` |
| JS console | [`--omn-console-clear`](#tokens-with-no-picture) | `#888888` | `#888888` |
| JS console | [`--omn-console-button`](#tokens-with-no-picture) | `#ff9800` | `#ff9800` |
| JS console | [`--omn-console-shadow`](#tokens-with-no-picture) | `rgba(0, 0, 0, 0.5)` | `rgba(0, 0, 0, 0.5)` |
| Bookmarks page | [`--omn-bookmark-special-bg`](#tokens-with-no-picture) | `#ff000022` | `#ff000022` |
| Bookmarks page | [`--omn-bookmarks-bg`](#tokens-with-no-picture) | `#fffff0` | `#1b1d1e` |
| Color marks | [`--omn-mark-yellow`](#tokens-with-no-picture) | `#ffff00` | `#ffff00` |
| Color marks | [`--omn-mark-aqua`](#tokens-with-no-picture) | `#00ffff` | `#00ffff` |
| Color marks | [`--omn-mark-red`](#tokens-with-no-picture) | `#ff0000` | `#ff0000` |
| Color marks | [`--omn-mark-green`](#tokens-with-no-picture) | `#008000` | `#008000` |
| Color marks | [`--omn-mark-ink`](#tokens-with-no-picture) | `#000000` | `#000000` |
| Shadows and overlay | [`--omn-overlay-bg`](#tokens-with-no-picture) | `rgba(0, 0, 0, 0.7)` | `rgba(0, 0, 0, 0.7)` |
| Shadows and overlay | [`--omn-shadow`](#tokens-with-no-picture) | `rgba(0, 0, 0, 0.1)` | `rgba(0, 0, 0, 0.4)` |
| Shadows and overlay | [`--omn-shadow-soft`](#tokens-with-no-picture) | `rgba(0, 0, 0, 0.05)` | `rgba(0, 0, 0, 0.25)` |
| Shadows and overlay | [`--omn-shadow-strong`](#tokens-with-no-picture) | `rgba(0, 0, 0, 0.2)` | `rgba(0, 0, 0, 0.6)` |

## Surfaces

### --omn-bg

The background of the page.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-bg, light palette](images/scheme-bg-light.png) | ![--omn-bg, dark palette](images/scheme-bg-dark.png) |

### --omn-bg-surface

The background of a panel, a dialog, a card and an input field. The picture shows the Quick Note panel. The search dialog, the Config card and the editor tool buttons also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-bg-surface, light palette](images/scheme-bg-surface-light.png) | ![--omn-bg-surface, dark palette](images/scheme-bg-surface-dark.png) |

### --omn-bg-band

The background of the page header and of the page footer. In the editor, it is the background of the tool bar, the find bar, the line numbers and the status line. A button of a note uses it as its background.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-bg-band, light palette](images/scheme-bg-band-light.png) | ![--omn-bg-band, dark palette](images/scheme-bg-band-dark.png) |

### --omn-bg-inset

The background of an inset box. The picture shows the "Receive a note" box of the Incoming page. The title arrow, the Config menu items and the buttons of the Status and Log pages also use it. A text field, a list and a text area of a note use it as their background.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-bg-inset, light palette](images/scheme-bg-inset-light.png) | ![--omn-bg-inset, dark palette](images/scheme-bg-inset-dark.png) |

### --omn-bg-hover

The background of a control under the pointer, and of the selected line of a list. The picture shows the selected result line of the search dialog. The header buttons, the tag marks and the buttons of a note use it under the pointer.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-bg-hover, light palette](images/scheme-bg-hover-light.png) | ![--omn-bg-hover, dark palette](images/scheme-bg-hover-dark.png) |

### --omn-bg-metadata

The background of the metadata panel. The info button of the page header opens this panel.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-bg-metadata, light palette](images/scheme-bg-metadata-light.png) | ![--omn-bg-metadata, dark palette](images/scheme-bg-metadata-dark.png) |

### --omn-bg-toc

The background of the table of contents box. A note makes this box with `<div id="omn-toc">`.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-bg-toc, light palette](images/scheme-bg-toc-light.png) | ![--omn-bg-toc, dark palette](images/scheme-bg-toc-dark.png) |

## Text

### --omn-text

The body text of a note and the text of most controls. It also sets the color of the border of the table of contents box.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-text, light palette](images/scheme-text-light.png) | ![--omn-text, dark palette](images/scheme-text-dark.png) |

### --omn-text-strong

The page title. The titles of the search results, of the Config cards and of the dialogs also use it. The text of a button in a note also uses it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-text-strong, light palette](images/scheme-text-strong-light.png) | ![--omn-text-strong, dark palette](images/scheme-text-strong-dark.png) |

### --omn-text-heading

The labels of the Config page. The title arrow, the group titles of the Search page and the title of the "Receive a note" box also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-text-heading, light palette](images/scheme-text-heading-light.png) | ![--omn-text-heading, dark palette](images/scheme-text-heading-dark.png) |

### --omn-text-muted

Hints and descriptions. The picture shows the hint of the Incoming page. The search chips that are not active and the counters of the Log page also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-text-muted, light palette](images/scheme-text-muted-light.png) | ![--omn-text-muted, dark palette](images/scheme-text-muted-dark.png) |

### --omn-text-faint

Small hints. The picture shows the note name and the status line of the search dialog. The page footer, the Config hints and the status line of the editor also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-text-faint, light palette](images/scheme-text-faint-light.png) | ![--omn-text-faint, dark palette](images/scheme-text-faint-dark.png) |

## Borders

### --omn-border

The border of an input field and of a plain button. The picture shows the Config page. The Quick Note panel, the metadata panel and the line below the editor tool bar also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-border, light palette](images/scheme-border-light.png) | ![--omn-border, dark palette](images/scheme-border-dark.png) |

### --omn-border-soft

Thin separator lines. The picture shows the line below the input row and the line below the scope row of the search dialog. The lines between the search results and between the rows of the Files page also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-border-soft, light palette](images/scheme-border-soft-light.png) | ![--omn-border-soft, dark palette](images/scheme-border-soft-dark.png) |

### --omn-border-card

The border of a large card. The picture shows the Config card. The progress card and the card of the external editor also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-border-card, light palette](images/scheme-border-card-light.png) | ![--omn-border-card, dark palette](images/scheme-border-card-dark.png) |

### --omn-border-strong

The border of a button of the page header. The search dialog, the "Receive a note" box and the editor tool buttons also use it. The buttons and the fields of a note also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-border-strong, light palette](images/scheme-border-strong-light.png) | ![--omn-border-strong, dark palette](images/scheme-border-strong-dark.png) |

## Accent and state

### --omn-accent

The main accent color. The picture shows the search button and the active chip of the search dialog. The progress bar, the Config menu icons, the editor Save button and the "File:" line of the metadata panel also use it. The links in a note do not use it. They use the `--omn-link` tokens. A field of a note uses it for its focus frame, and a check box and a radio button of a note use it for their mark.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-accent, light palette](images/scheme-accent-light.png) | ![--omn-accent, dark palette](images/scheme-accent-dark.png) |

### --omn-accent-hover

An accent button under the pointer. The picture shows the search button of the search dialog under the pointer. The text of a button in a note also uses it under the pointer.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-accent-hover, light palette](images/scheme-accent-hover-light.png) | ![--omn-accent-hover, dark palette](images/scheme-accent-hover-dark.png) |

### --omn-success

The Save button and the Copy button of a panel. The Save Configuration button and the mark of a Config page with no change also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-success, light palette](images/scheme-success-light.png) | ![--omn-success, dark palette](images/scheme-success-dark.png) |

### --omn-success-hover

The Save Configuration button under the pointer.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-success-hover, light palette](images/scheme-success-hover-light.png) | ![--omn-success-hover, dark palette](images/scheme-success-hover-dark.png) |

### --omn-danger

The mark of a Config page with a change that is not saved. The error texts of the Status, Log and Incoming pages also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-danger, light palette](images/scheme-danger-light.png) | ![--omn-danger, dark palette](images/scheme-danger-dark.png) |

### --omn-info

The three create buttons of the page header: new note, quick note and bookmark. On Android, the button that adds the page to the home screen also uses it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-info, light palette](images/scheme-info-light.png) | ![--omn-info, dark palette](images/scheme-info-dark.png) |

### --omn-secondary

The Refresh button and the Config button of the page header.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-secondary, light palette](images/scheme-secondary-light.png) | ![--omn-secondary, dark palette](images/scheme-secondary-dark.png) |

## Tags

### --omn-tag-bg

The background of a tag mark. The page header, the Tags page and the Search page show tag marks.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-tag-bg, light palette](images/scheme-tag-bg-light.png) | ![--omn-tag-bg, dark palette](images/scheme-tag-bg-dark.png) |

### --omn-tag-border

The border of a tag mark.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-tag-border, light palette](images/scheme-tag-border-light.png) | ![--omn-tag-border, dark palette](images/scheme-tag-border-dark.png) |

### --omn-tag-selected

The background of a tag mark with the class `omn-tagmarkselected`. The text of that mark has the color `--omn-bg-surface`. No OMN-Go page sets this class now. For the picture, a script added the class to the tag mark "Theme". A note script can use the class.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-tag-selected, light palette](images/scheme-tag-selected-light.png) | ![--omn-tag-selected, dark palette](images/scheme-tag-selected-dark.png) |

## Search hits

### --omn-search-hit-bg

The background of a found word. The picture shows a word that a search marks in the note. The search results and the find bar of the editor also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-search-hit-bg, light palette](images/scheme-search-hit-bg-light.png) | ![--omn-search-hit-bg, dark palette](images/scheme-search-hit-bg-dark.png) |

### --omn-search-hit-text

The text of a found word in the note and in the search results.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-search-hit-text, light palette](images/scheme-search-hit-text-light.png) | ![--omn-search-hit-text, dark palette](images/scheme-search-hit-text-dark.png) |

## Tables

### --omn-table-border

The border of a table and of each cell. The value is the same in the two palettes.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-table-border, light palette](images/scheme-table-border-light.png) | ![--omn-table-border, dark palette](images/scheme-table-border-dark.png) |

### --omn-table-th-bg

The background of a header cell.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-table-th-bg, light palette](images/scheme-table-th-bg-light.png) | ![--omn-table-th-bg, dark palette](images/scheme-table-th-bg-dark.png) |

### --omn-table-th-text

The text of a header cell.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-table-th-text, light palette](images/scheme-table-th-text-light.png) | ![--omn-table-th-text, dark palette](images/scheme-table-th-text-dark.png) |

### --omn-table-row-odd

The background of the odd rows of a table.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-table-row-odd, light palette](images/scheme-table-row-odd-light.png) | ![--omn-table-row-odd, dark palette](images/scheme-table-row-odd-dark.png) |

### --omn-table-row-even

The background of the even rows of a table.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-table-row-even, light palette](images/scheme-table-row-even-light.png) | ![--omn-table-row-even, dark palette](images/scheme-table-row-even-dark.png) |

### --omn-table-text

The text of a data cell.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-table-text, light palette](images/scheme-table-text-light.png) | ![--omn-table-text, dark palette](images/scheme-table-text-dark.png) |

## Code blocks

### --omn-code-bg

The background of a code block.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-bg, light palette](images/scheme-code-bg-light.png) | ![--omn-code-bg, dark palette](images/scheme-code-bg-dark.png) |

### --omn-code-text

The base text of a code block. Keywords such as `const` and `if` have this color in bold.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-text, light palette](images/scheme-code-text-light.png) | ![--omn-code-text, dark palette](images/scheme-code-text-dark.png) |

### --omn-code-comment

A comment.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-comment, light palette](images/scheme-code-comment-light.png) | ![--omn-code-comment, dark palette](images/scheme-code-comment-dark.png) |

### --omn-code-punctuation

Punctuation and the marks around an HTML tag: `<`, `=`, `>` and `</`.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-punctuation, light palette](images/scheme-code-punctuation-light.png) | ![--omn-code-punctuation, dark palette](images/scheme-code-punctuation-dark.png) |

### --omn-code-string

A string, a number, a type and a deleted line of a diff. A function name and a section title also use it, in bold.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-string, light palette](images/scheme-code-string-light.png) | ![--omn-code-string, dark palette](images/scheme-code-string-dark.png) |

### --omn-code-variable

A variable of the language, for example `document` and `console`. A link, an operator, a regular expression and a template variable also use it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-variable, light palette](images/scheme-code-variable-light.png) | ![--omn-code-variable, dark palette](images/scheme-code-variable-dark.png) |

### --omn-code-literal

A literal value, for example `null` and `true`.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-literal, light palette](images/scheme-code-literal-light.png) | ![--omn-code-literal, dark palette](images/scheme-code-literal-dark.png) |

### --omn-code-built-in

A built-in name, a list mark and an added line of a diff. The picture shows an added line.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-built-in, light palette](images/scheme-code-built-in-light.png) | ![--omn-code-built-in, dark palette](images/scheme-code-built-in-dark.png) |

### --omn-code-meta

A meta item, for example `<!DOCTYPE html>` and `#include`.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-meta, light palette](images/scheme-code-meta-light.png) | ![--omn-code-meta, dark palette](images/scheme-code-meta-dark.png) |

### --omn-code-meta-string

A string inside a meta item, for example `"notes.h"` in `#include "notes.h"`.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-meta-string, light palette](images/scheme-code-meta-string-light.png) | ![--omn-code-meta-string, dark palette](images/scheme-code-meta-string-dark.png) |

### --omn-code-tag-name

The name of an HTML tag. The highlight theme gives a tag name a fixed color. The core stylesheet gives it this token, thus a scheme can change it.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-tag-name, light palette](images/scheme-code-tag-name-light.png) | ![--omn-code-tag-name, dark palette](images/scheme-code-tag-name-dark.png) |

### --omn-code-tag-attr

The name of an HTML attribute. The core stylesheet gives it this token, the same as `--omn-code-tag-name`.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-tag-attr, light palette](images/scheme-code-tag-attr-light.png) | ![--omn-code-tag-attr, dark palette](images/scheme-code-tag-attr-dark.png) |

## Typography

The scheme block can also set the main font and the code font, each with a size. The block writes the values as variables in `:root`: `--omn-font`, `--omn-font-size`, `--omn-code-font` and `--omn-code-size`. Two rules of the block read them. `omn-go-core.css` does not read these variables. A scheme with no font sets no variable and no rule.

The pictures show the fonts of OMN-Go with no scheme.

### --omn-font

The main font and its size. The scheme block sets them on `body` of each page that loads the block. Without a scheme, a note uses `sans-serif` at 16 px.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-font, light palette](images/scheme-omn-font-light.png) | ![--omn-font, dark palette](images/scheme-omn-font-dark.png) |

### --omn-code-font

The code font and its size. The scheme block sets them on `#omn-preview pre` and `#omn-preview code`. Without a scheme, a code block uses `monospace` at 13 px.

| Light palette | Dark palette |
| --- | --- |
| ![--omn-code-font, light palette](images/scheme-omn-code-font-light.png) | ![--omn-code-font, dark palette](images/scheme-omn-code-font-dark.png) |

## Tokens with no picture

A scheme sets these tokens the same as the other tokens. The table gives the part of the page that each token colors.

| Token | Light | Dark | Part of the page |
| --- | --- | --- | --- |
| `--omn-text-dim` | `#888888` | `#7a8086` | Small quiet text: the note path and the line number in the search dialog, the counts of the Search page, the facts of the Files page and the line numbers of the editor. |
| `--omn-border-title` | `#eaecef` | `#3a3e41` | The line below the title and below each subtitle of the Config page. |
| `--omn-on-accent` | `#ffffff` | `#ffffff` | The text and the icon on a filled button, for example the search button, the Save button of a changed Config page and an active button of the editor. |
| `--omn-link` | `#0000ee` | `#9e9eff` | A link in a note or on a panel page. A link with a class of its own keeps its own color. |
| `--omn-link-visited` | `#551a8b` | `#d0adf0` | A visited link. |
| `--omn-link-hover` | `#0000ee` | `#9e9eff` | A link under the mouse pointer. The default is the same as `--omn-link`, thus the color does not change. |
| `--omn-file-app` | `#9a5b00` | `#f0a13c` | The state word of a file that an update of OMN-Go replaces. |
| `--omn-file-alert` | `#b3261e` | `#ff8a80` | The state word of a file that an update replaces, when the copy on the device differs. |
| `--omn-file-keep` | `#1f7a35` | `#5ec97a` | The state word of a file that OMN-Go keeps. |
| `--omn-file-derived` | `#12798a` | `#4fc3d7` | The state word of a file that OMN-Go makes again. |
| `--omn-file-plain` | `#6b6b6b` | `#9aa0a6` | The state word of a file with no risk. |
| `--omn-files-source-bg` | `#4c6b8a` | `#3f5a75` | The background of the Source button on the first screen of the Files page. |
| `--omn-start-notes-bg` | `#0056b3` | `#1c5ea8` | The background of the "My Quick Notes" button of the start page. The buttons of the Files page also use it. |
| `--omn-start-bookmarks-bg` | `#17a2b8` | `#14707e` | The background of the "My Bookmarks" button of the start page. The Served button of the Files page also uses it. |
| `--omn-start-btn-text` | `#ffffff` | `#eef3f8` | The text of the two start buttons. |
| `--omn-state-ok` | `#3fb950` | `#3fb950` | The dot of a database that is in sync with its backup. The dot of the editor when the note is saved. |
| `--omn-state-warn` | `#d29922` | `#d29922` | The dot of a database with changes after its last backup. |
| `--omn-state-info` | `#4d9fff` | `#4d9fff` | The dot of a database with a newer backup, or with a missing file. |
| `--omn-state-off` | `#8b949e` | `#8b949e` | The dot of a database with no backup. |
| `--omn-state-error` | `#f85149` | `#f85149` | The dot of a damaged backup. |
| `--omn-state-alert` | `#ff5555` | `#ff5555` | The dot and the status text of the editor when the note is not saved, or after an error. |
| `--omn-console-bg` | `#1e1e1e` | `#1e1e1e` | The background of the JS console. |
| `--omn-console-text` | `#00ff00` | `#00ff00` | The default text of the JS console. |
| `--omn-console-line` | `#f8f8f2` | `#f8f8f2` | A log line of the JS console. |
| `--omn-console-warn` | `#ffb86c` | `#ffb86c` | A warning line of the JS console. |
| `--omn-console-error` | `#ff5555` | `#ff5555` | An error line of the JS console, and the Close button. |
| `--omn-console-border` | `#555555` | `#555555` | The border of the JS console. |
| `--omn-console-head-bg` | `#333333` | `#333333` | The background of the head of the JS console. |
| `--omn-console-head-text` | `#ffffff` | `#ffffff` | The text of the head of the JS console. |
| `--omn-console-clear` | `#888888` | `#888888` | The Clear button of the JS console. |
| `--omn-console-button` | `#ff9800` | `#ff9800` | The console button of the page header, and the console dot of the page footer. |
| `--omn-console-shadow` | `rgba(0, 0, 0, 0.5)` | `rgba(0, 0, 0, 0.5)` | The shadow of the JS console. |
| `--omn-bookmark-special-bg` | `#ff000022` | `#ff000022` | The background of a special tag of the Bookmarks page, for example "Duplicates". |
| `--omn-bookmarks-bg` | `#fffff0` | `#1b1d1e` | The background of the page of the Bookmarks note. |
| `--omn-mark-yellow` | `#ffff00` | `#ffff00` | The background of the mark `omn-bg-yellow` that a note can use. |
| `--omn-mark-aqua` | `#00ffff` | `#00ffff` | The background of the mark `omn-bg-aqua`. |
| `--omn-mark-red` | `#ff0000` | `#ff0000` | The text of the mark `omn-fg-red`. |
| `--omn-mark-green` | `#008000` | `#008000` | The text of the mark `omn-fg-green`. |
| `--omn-mark-ink` | `#000000` | `#000000` | The text on the marks `omn-bg-yellow` and `omn-bg-aqua`. |
| `--omn-overlay-bg` | `rgba(0, 0, 0, 0.7)` | `rgba(0, 0, 0, 0.7)` | The dark layer behind a dialog. |
| `--omn-shadow` | `rgba(0, 0, 0, 0.1)` | `rgba(0, 0, 0, 0.4)` | The shadow of a card, a panel, a dialog and a start button. |
| `--omn-shadow-soft` | `rgba(0, 0, 0, 0.05)` | `rgba(0, 0, 0, 0.25)` | The shadow of a git server card of the Config page, and of a button in a note. |
| `--omn-shadow-strong` | `rgba(0, 0, 0, 0.2)` | `rgba(0, 0, 0, 0.6)` | A dark shadow: the search dialog, the progress card and the backup dialog. |
