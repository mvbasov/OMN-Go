Title: The Editor
Date: 2026-07-10 12:00:00
Category: System
Author: Mikhail Basov
Tags: Document, OMN-Go, OMN-Go app

# The OMN-Go Editor

Press <i class="material-icons">edit</i> on any page to open the built-in editor. You can also open any file with `?edit=true`. See the [User Manual](UserManual#edit-links-for-non-page-files).

The editor is a page of its own. It loads the Markdown source of the note when it opens, and it writes the Markdown source back when you save. A rendered note therefore never carries a hidden second copy of its own text.

The cursor starts directly after the `Title:`/`Date:`/… header block of the note. You start in the body of the note, and not at the end of the file. If you arrive from a console error that you pressed, the cursor starts on the line with the error instead.

## The toolbar

Left to right:

- <i class="material-icons">code</i> **Expand Emmet abbreviation** — expand the abbreviation on the current line into HTML. This button does the same as the **Tab** key. See below.
- <i class="material-icons">format_line_spacing</i> **Cycle selection** — press this button again and again to select more of the note. The cycle has seven selections. An eighth press starts the cycle again:
  1. The current line.
  2. From the cursor to the end of the line.
  3. From the start of the line to the cursor.
  4. From the current line to the end of the file.
  5. From the current line to the header block.
  6. The body of the note, without the header block.
  7. The whole note, with the header block.

  The editor keeps the position of the cursor from the start of the cycle. All seven selections use that position, and not the ends of the selection before them.

  For the fifth selection, the start of the selection is the position that comes first in the file. The fifth selection therefore also works when the cursor is inside the header block. The fifth selection always contains the full current line.

  A note without a header block has no boundary between the header block and the body. For such a note, the sixth selection is the same as the seventh.

  If you move the cursor or make a different selection, the editor resets the cycle. The next press selects the current line again.
- <i class="material-icons">wrap_text</i> **Word wrap** — wrap long lines to the width of the window. With word wrap off, a long line continues past the edge of the window and you scroll sideways.
- <i class="material-icons">format_list_numbered</i> **Line numbers** — show a numbered gutter at the left. This button is available only with word wrap *off*. A wrapped line fills several rows on the screen, but it stays one line. The numbers and the text therefore cannot stay in step.
- <i class="material-icons">search</i> **Find / replace** — open the find bar. See [below](#find-and-replace).
- <i class="material-icons">save</i> **Save** — save the note and return to the rendered page. Keyboard shortcut: **Ctrl/Cmd + S**.
- <i class="material-icons">close</i> **Cancel** — leave the editor without saving. If you have unsaved changes, the editor asks you first.

The editor shows the name of the note at its bottom ("Editing …"). Drag an image file onto the text area to upload the image. The editor inserts an `<img>` tag at the cursor. The tag has the `omn-imported-image` class, which limits the width of the image. The image therefore does not render at its full native resolution.

## Find and replace

Press <i class="material-icons">search</i>, or press **Ctrl/Cmd + F**. **Ctrl/Cmd + H** opens the find bar with the replace field already shown. The find bar appears between the toolbar and the text. It pushes the note down and does not cover it, so you see the text while you change it. If you selected text before you opened the find bar, that text is already the query.

The find bar highlights every match. It also puts a ring around the match that the counter points to. The counter reads **3 / 17**, or *no matches*, or *bad pattern*. In a very large note the counter stops at 1000 matches and reads **1000+**. It does not show an exact count above 1000.

### Moving between matches

| Key | Does |
| --- | --- |
| **Enter** | move to the next match |
| **Shift + Enter** | move to the previous match |
| **F3** / **Ctrl + G** | move to the next match, from anywhere |
| **Shift + F3** | move to the previous match |
| **Esc** | close the find bar and keep the cursor in the text where you were |

After the last match, the find bar continues from the first match.

### The three switches

The three switches are at the right of the query field:

- **Aa** — match case. This switch is off by default, so `note` finds `Note` and `NOTE`.
- **ab** — whole word. When you enable it, `note` no longer matches inside `notes` or `footnote`. The switch understands every alphabet, and not only the Latin alphabet. It works the same way on Russian text.
- **.\*** — regular expression. In this mode the query is a pattern, and not literal text. In the replacement, `$1`, `$2` … stand for the bracketed groups of the pattern, and `$&` stands for the whole match. If a pattern does not compile, the find bar turns the field red and the counter reads *bad pattern*. The find bar never reports zero matches for such a pattern.

  Outside this mode the find bar treats every character literally. A search
  for `a.b` finds `a.b` and does not find `axb`. A `$` in the replacement is a
  dollar sign.

The editor remembers the three switches for each device. If you work with regular expressions, the find bar stays in that mode.

### Replacing

The chevron at the left of the find bar shows and hides the replace field.

- **Replace** changes the current match and moves to the next match. Press it again and again to move forward through the note.
- **All** changes every match and reports the number of changes.

Both buttons make one step for **Ctrl + Z**. One undo returns the note to its previous state, even after you change hundreds of matches.

### What is being searched

The find bar searches the **Markdown source** of the note. This is the same text that you edit, and not the rendered page. The find bar therefore searches the header block at the top of the file like any other line. It also searches the text inside a `<script>` block or a fenced code block.

The search panel in the page header is a different search. It searches your notes as OMN-Go *published* them. See [Searching](UserManual#searching) in the User Manual for the search panel.

## Tab and Emmet

The **Tab** key does one of two things:

- If the text on the current line, up to the cursor, is an Emmet abbreviation, Tab **expands** the abbreviation into HTML.
- If it is not an Emmet abbreviation, Tab inserts a normal tab character.

You can type a compact abbreviation and press Tab to get the full markup. For example, type

```
ul>li*3
```

and press Tab. The editor produces:

```
<ul>
  <li></li>
  <li></li>
  <li></li>
</ul>
```

### Emmet in a nutshell

An abbreviation is a tag name with any of these parts:

- `#name` — an id, `.name` — a class (repeatable): `div#main.box.wide`
- `[attr=value]` — attributes: `a[href=# title="Go home"]`
- `{text}` — text content: `p{Hello}`
- `>` child, `+` sibling, `*N` repeat, `( … )` group: `nav>ul>li*2>a`
- `$` — the editor replaces this with the item number inside a repeat (`$$` adds leading zeros): `li.item$*3`

In this worked example, the abbreviation

```
div.card>h3{Title}+p{Body}
```

expands to:

```
<div class="card">
  <h3>Title</h3>
  <p>Body</p>
</div>
```

The children of a few container tags get an implied tag name. For example, `ul>.item` becomes `<ul><li class="item">…`, and `table>tr>td` also works. Other container tags behave in the same way.

**Limits.** The editor supports a compact subset of Emmet, and not full Emmet. It does not support the climb-up operator `^`. It does not support text generators such as `lorem`. If the editor does not recognize an abbreviation, Tab inserts a tab character instead. The editor also limits the repeat count, so a large number cannot lock the editor. This page lists the full supported set.

---

See the [User Manual](UserManual) for everything else. See [Scripting Rules](ScriptRules) for the rules about a note script.
