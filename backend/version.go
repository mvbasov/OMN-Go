package backend

// APP_VERSION is the global application version
const APP_VERSION = "26.11.21"

// The commit of the build and its time, set with -ldflags -X. They are not
// exported, because gomobile binds each exported name. See
// doc/decisions/0028-record-the-commit-of-the-build.md.
var (
	buildCommit     string
	buildCommitTime string
)
