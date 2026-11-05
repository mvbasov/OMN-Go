package app

import (
	"fmt"
	"strings"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
)

// --- The Config page ---

var (
	configPageTmpl     = render.LoadTemplate("config_page.html")
	gitServerCardTmpl  = render.LoadTemplate("git_server_card.html")
	configMenuLinkTmpl = render.LoadTemplate("config_menu_link.html")
)

// gitServerView is one git server slot on the Config page. It holds NO SSH
// key and NO key password, and configPageView holds no user password. See
// doc/decisions/0005-keep-each-secret-out-of-the-config-page.md.
type gitServerView struct {
	Index   int
	Slot    int
	Active  bool
	Name    string
	URL     string
	HostKey string // the line of gitsync.Service.HostKeyText, or ""
}

// configPageView holds the page values. config.PageValues gives no secret.
type configPageView struct {
	Values            []config.PageValue
	SearchIndexStatus string // human-readable line for the Search screen
	GitServers        []gitServerView
	SystemPages       []systemPage // the rows with a menu line give a link
}

// logTagLabels gives the text beside each log tag checkbox. A tag with no
// entry shows its own name. logx.AllTags is the authority for the tag set.
var logTagLabels = map[logx.Tag]string{
	logx.NotFound:    "Requests for a page that does not exist",
	logx.Assets:      "Bundled asset refresh at startup",
	logx.Config:      "Reading and writing config.json",
	logx.DB:          "SQLite handles behind /api/sql",
	logx.DBBackup:    "Database backup and pruning",
	logx.DBBootstrap: "First-run restore on a new device",
	logx.DBRestore:   "Database restore from a backup",
	logx.Edit:        "The external editor",
	logx.Exchange:    "Note import and export",
	logx.NoteFiles:   "Files carried between md/ and html/",
	logx.Page:        "Reading and writing a note",
	logx.Precompile:  "Compiling notes to HTML",
	logx.Restart:     "Restarting the server process",
	logx.Search:      "The global search index",
	logx.Server:      "Startup, the listener and crashes",
	logx.Session:     "The login and the session key",
	logx.Status:      "The Status page",
	logx.Storage:     "The storage directory",
	logx.Sync:        "Git sync, the loudest subsystem",
	logx.Tags:        "The tags index",
	logx.Templates:   "The embedded page templates",
	logx.Upload:      "File uploads",
}

// renderLogTagBoxes makes one checkbox for each tag in logx.AllTags. values
// holds the LOG_TAGS_<TAG> marks of config.PageValues.
func renderLogTagBoxes(values map[string]string) string {
	var b strings.Builder
	for _, tag := range logx.AllTags {
		label, ok := logTagLabels[tag]
		if !ok {
			label = string(tag)
		}
		checked := values[config.PlaceholderName("log_tags_"+string(tag))]
		b.WriteString(`                <div class="omn-config-checkbox-row">` + "\n")
		b.WriteString(`                    <input type="checkbox" name="log_tags" value="` +
			render.EscapeHTML(string(tag)) + `" ` + checked + ` />` + "\n")
		b.WriteString(`                    <label class="omn-config-label"><code>` +
			render.EscapeHTML(string(tag)) + `</code> - ` + render.EscapeHTML(label) + `</label>` + "\n")
		b.WriteString("                </div>\n")
	}
	return b.String()
}

func renderConfigPage(v configPageView) string {
	var cards strings.Builder
	for _, gs := range v.GitServers {
		checked := ""
		if gs.Active {
			checked = "checked"
		}
		// Put no SSH_KEY and no PASSWORD here. See gitServerView.
		cards.WriteString(render.Fill(gitServerCardTmpl, map[string]string{
			"INDEX":          fmt.Sprintf("%d", gs.Index),
			"SLOT":           fmt.Sprintf("%d", gs.Slot),
			"ACTIVE_CHECKED": checked,
			"NAME":           render.EscapeHTML(gs.Name),
			"URL":            render.EscapeHTML(gs.URL),
			"HOST_KEY":       render.EscapeHTML(gs.HostKey),
		}))
	}

	values := map[string]string{}
	for _, pv := range v.Values {
		if pv.Text {
			values[pv.Name] = render.EscapeHTML(pv.Value)
		} else {
			values[pv.Name] = pv.Value
		}
	}
	// Give the names of the checkboxes of this page, from the table in
	// internal/config/fields.go. See config.CheckboxFields.
	values["CONFIG_FIELDS"] = config.CheckboxFields()
	values["SEARCH_INDEX_STATUS"] = render.EscapeHTML(v.SearchIndexStatus)
	values["LOG_TAG_BOXES"] = renderLogTagBoxes(values)
	values["GIT_SERVERS"] = cards.String()
	var links strings.Builder
	for _, p := range v.SystemPages {
		if p.menu != nil {
			links.WriteString(render.Fill(configMenuLinkTmpl, map[string]string{
				"PATH": render.EscapeHTML(p.path), "ICON": render.EscapeHTML(p.menu.icon),
				"NAME": render.EscapeHTML(p.title), "DESC": render.EscapeHTML(p.menu.desc),
			}))
		}
	}
	values["SYSTEM_PAGE_LINKS"] = links.String()
	return render.Fill(configPageTmpl, values)
}
