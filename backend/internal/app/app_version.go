package app

// version is APP_VERSION of the build. backend/version.go holds the value,
// because the CI reads it there, and StartServer of package backend gives it
// to SetVersion before the start. A test sees "dev".
var version = "dev"

// generator is the value of the generator meta tag, "OMN-Go " and the
// version. SetVersion makes it one time, thus a compile makes no new string.
var generator = "OMN-Go dev"

// SetVersion records the version of the build. Call it before StartServer.
func SetVersion(v string) {
	version = v
	generator = "OMN-Go " + v
}

// The commit of the build. See backend/version.go.
var (
	buildCommit     string
	buildCommitTime string
)

// SetBuildCommit records the commit of the build. Call it before StartServer.
func SetBuildCommit(commit, commitTime string) {
	buildCommit = commit
	buildCommitTime = commitTime
}
