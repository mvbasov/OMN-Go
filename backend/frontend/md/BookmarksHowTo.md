Title: How to use Bookmarks
Date: 2026-08-03 12:00:00
Category: System
Tags: Bookmarks

<!--
  Each paragraph is ONE long line on purpose: the renderer keeps every line
break of the source (hard wraps), so a paragraph that is wrapped in the
source is also wrapped on the screen, at the wrong places.
-->

#### Your bookmarks

The [Bookmarks](Bookmarks) page keeps your links. OMN-Go keeps them in a note, not in a browser, so they stay with your notes and they come to every device that you synchronize.

#### How to add a bookmark

Press the page title to expand the header, then press <i class="material-icons">bookmark_add</i>. Fill in the address, a title, tags and notes, and press *Save*. Only the address is necessary.

Tags and notes help you to find the bookmark later. Write a comma between tags (`work, recipe`). Write a semicolon between notes.

**On Android.** Press *Share* in your browser, or in another application, and select OMN-Go. The form opens with the address and the title in it. Shared text that has no address goes to [Quick Notes](QuickNotes) instead.

**On a desktop.** Drag a link from your browser and drop it on an OMN-Go page. The form opens with the address in it.

You can also make a browser bookmark that sends the page that you read to OMN-Go. Create a bookmark in your browser and put this text in the address field. Use the port of your server:

```
javascript:window.open('http://localhost:8080/Welcome.html?share_text='+encodeURIComponent(location.href)+'&share_subject='+encodeURIComponent(document.title));
```

#### How to find a bookmark

Open the [Bookmarks](Bookmarks) page:

* Write a word in the search box and press *Search*. The page shows only the bookmarks that contain the word.
* Press a tag button to show only the bookmarks with that tag. Press *All* to show all bookmarks again.
* Press the number in the top right corner to show or hide the list of tags.
* Press **ⓘ** before a bookmark to show its tags and notes. *Expand* and *Collapse* do this for all bookmarks.

The header search (<i class="material-icons">search</i>) also finds bookmarks, together with your notes, if global search is enabled in the settings.

---

[Quick Notes](QuickNotes) · [Bookmarks](Bookmarks) · [User Manual](UserManual)
