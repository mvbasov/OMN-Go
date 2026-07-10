Title: The Editor
Date: 2026-07-10 12:00:00
Category: System
Tags: Document

# The OMN-Go Editor

Pressing <i class="material-icons">edit</i> on any page (or opening any file
with `?edit=true`, see the
[User Manual](UserManual#edit-links-for-non-page-files)) opens the built-in
editor. It is a page of its own: it loads the note's Markdown source when it
opens and writes it back when you save, so a rendered note never carries a
hidden second copy of its own text.

## The toolbar

Left to right:

- <i class="material-icons">code</i> **Expand Emmet abbreviation** — expand
  the abbreviation on the current line into HTML (same as pressing **Tab**;
  see below).
- <i class="material-icons">format_line_spacing</i> **Select current line** —
  select the whole line the cursor is on, ready to cut, copy or replace.
- <i class="material-icons">save</i> **Save** — write the note and return to
  the rendered view. Keyboard shortcut: **Ctrl/Cmd + S**.
- <i class="material-icons">close</i> **Cancel** — leave without saving. If
  you have unsaved changes it asks first.

The note's name is shown at the bottom of the editor ("Editing …"). Drag an
image file onto the text area to upload it and insert a Markdown image
reference at the cursor.

## Tab and Emmet

The **Tab** key does one of two things:

- if the text on the current line, up to the cursor, is an Emmet
  abbreviation, Tab **expands** it into HTML;
- otherwise Tab inserts a normal tab character.

So you can type a compact abbreviation and press Tab to get the full markup.
For example, typing

```
ul>li*3
```

and pressing Tab produces:

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
- `>` child, `+` sibling, `*N` repeat, `( … )` group:
  `nav>ul>li*2>a`
- `$` is replaced by the item number inside a repeat (`$$` zero-pads):
  `li.item$*3`

A worked example — the abbreviation

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

Children of a few container tags get an implied tag name, so `ul>.item`
becomes `<ul><li class="item">…`, `table>tr>td` works, and so on.

**Limits.** This is a compact subset, not full Emmet: the climb-up operator
`^` and text generators such as `lorem` are not supported (when an
abbreviation isn't recognized, Tab just inserts a tab instead), and a repeat
count is capped so a stray large number can't lock the editor. Everything on
this page is the supported set.

---

See the [User Manual](UserManual) for everything else, and
[Scripting Rules](ScriptRules) for embedding scripts in a note.
