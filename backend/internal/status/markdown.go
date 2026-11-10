package status

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ----------------------------------------------------------------------
// The Markdown form (?format=md)
// ----------------------------------------------------------------------

func renderStatusMarkdown(res *statusResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# OMN-Go status\n\nGenerated: %s\n", res.Generated)

	table := func(title string, rows [][2]string) {
		fmt.Fprintf(&b, "\n## %s\n\n| Field | Value |\n| --- | --- |\n", title)
		for _, row := range rows {
			fmt.Fprintf(&b, "| %s | %s |\n", row[0], row[1])
		}
	}
	yes := func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	}

	if s := res.Server; s != nil {
		rows := [][2]string{
			{"app_version", s.AppVersion},
			{"build_commit", s.BuildCommit},
			{"build_commit_time", s.BuildCommitTime},
		}
		if s.BuildCommitURL != "" {
			rows = append(rows, [2]string{"build_commit_url", s.BuildCommitURL})
		}
		table("Server", append(rows, [][2]string{
			{"started", s.Started},
			{"uptime_s", strconv.FormatInt(s.UptimeS, 10)},
			{"bind_port", strconv.Itoa(s.BindPort)},
			{"share_lan", yes(s.ShareLAN)},
			{"lan_urls", strings.Join(s.LANURLs, ", ")},
			{"active_conns", strconv.FormatInt(s.ActiveConns, 10)},
			{"hostname", s.Hostname},
			{"goos", s.GOOS},
			{"goarch", s.GOARCH},
		}...))
	}
	if c := res.Config; c != nil {
		rows := make([][2]string, 0, len(c))
		for _, v := range c {
			text := fmt.Sprint(v.Value)
			switch x := v.Value.(type) {
			case bool:
				text = yes(x)
			case []string:
				text = strings.Join(x, ", ")
			}
			rows = append(rows, [2]string{v.Key, text})
		}
		table("Config", rows)
	}
	if g := res.Git; g != nil {
		rows := [][2]string{
			{"repo_exists", yes(g.RepoExists)},
			{"configured", yes(g.Configured)},
			{"branch", g.Branch},
		}
		if g.Head != nil {
			rows = append(rows,
				[2]string{"head_hash", g.Head.Hash},
				[2]string{"head_short", g.Head.Short},
				[2]string{"head_subject", g.Head.Subject},
				[2]string{"head_author", g.Head.Author},
				[2]string{"head_date", g.Head.Date})
		}
		if g.Remote != nil {
			rows = append(rows,
				[2]string{"remote_name", g.Remote.Name},
				[2]string{"remote_url", g.Remote.URL})
		}
		if g.RemoteRef != "" {
			rows = append(rows, [2]string{"remote_ref", g.RemoteRef})
		}
		if g.RemoteHead != nil {
			rows = append(rows,
				[2]string{"remote_head_hash", g.RemoteHead.Hash},
				[2]string{"remote_head_short", g.RemoteHead.Short},
				[2]string{"remote_head_subject", g.RemoteHead.Subject},
				[2]string{"remote_head_author", g.RemoteHead.Author},
				[2]string{"remote_head_date", g.RemoteHead.Date})
		}
		table("Git", rows)
	}
	if d := res.GitDirty; d != nil {
		table("Git worktree", [][2]string{
			{"dirty", yes(d.Dirty)},
			{"changed", strconv.Itoa(d.Changed)},
			{"untracked", strconv.Itoa(d.Untracked)},
		})
	}
	if s := res.Search; s != nil {
		table("Search", [][2]string{
			{"enabled", yes(s.Enabled)},
			{"docs", strconv.Itoa(s.Docs)},
			{"lines", strconv.Itoa(s.Lines)},
			{"bytes", strconv.FormatInt(s.Bytes, 10)},
			{"index_bytes_estimate", strconv.FormatInt(s.IndexBytesEstimate, 10)},
			{"built", s.Built},
			{"checked", s.Checked},
			{"dirty", yes(s.Dirty)},
			{"kinds", strings.Join(s.Kinds, ", ")},
			{"scope", s.Scope},
		})
	}
	if rt := res.Runtime; rt != nil {
		table("Runtime", [][2]string{
			{"go_version", rt.GoVersion},
			{"goroutines", strconv.Itoa(rt.Goroutines)},
			{"heap_alloc", strconv.FormatUint(rt.HeapAlloc, 10)},
			{"sys", strconv.FormatUint(rt.Sys, 10)},
			{"assets_version", rt.AssetsVersion},
			{"assets_refreshed", yes(rt.AssetsRefreshed)},
		})
	}
	if an := res.Android; an != nil {
		table("Android", [][2]string{
			{"package", an.Package},
			{"default_port", strconv.Itoa(an.DefaultPort)},
			{"fullscreen", an.Fullscreen},
		})
	}
	if st := res.Storage; st != nil {
		fmt.Fprintf(&b, "\n## Storage\n\nDirectory: `%s`\n\n| Group | Files | Bytes |\n| --- | --- | --- |\n", st.Dir)
		for _, g := range [][2]any{
			{"notes", st.Notes}, {"pages", st.Pages}, {"images", st.Images},
			{"user_json", st.UserJSON}, {"user_contacts", st.UserContacts},
			{"user_calendars", st.UserCalendars}, {"databases", st.Databases},
			{"backups", st.Backups}, {"asset_backups", st.AssetBackups},
			{"total", st.Total},
		} {
			grp := g[1].(statusGroup)
			fmt.Fprintf(&b, "| %s | %d | %d |\n", g[0], grp.Files, grp.Bytes)
		}
	}
	if len(res.Errors) > 0 {
		rows := [][2]string{}
		names := make([]string, 0, len(res.Errors))
		for k := range res.Errors {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			rows = append(rows, [2]string{k, res.Errors[k]})
		}
		table("Errors", rows)
	}
	return b.String()
}
