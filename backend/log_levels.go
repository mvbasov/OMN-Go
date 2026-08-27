package backend

// ---------------------------------------------------------------------
// Log tags and log levels
//
// Every log line the backend writes reaches three places: stdout, the
// /api/logs SSE stream, and the browser console of every open page. See
// logger.go and the EventSource block at the end of omn-go-sse.js.
//
// Before this file existed, a line carried a bracketed subsystem name that
// each call site typed by hand. 119 of 153 call sites had one, the other 34
// had none, and one carried the literal text "[a.protectGitDirs]". A person
// who opened the browser console therefore read the full detail of every
// subsystem at once, and had no way to ask for less.
//
// This file gives a line two properties instead of one:
//
//	tag    which subsystem wrote the line. One of the constants below.
//	level  how much the reader wants it. debug, info or error.
//
// The emitted text is "[tag] (level) message". a.logDebugf, a.logInfof and
// a.logErrf write the brackets and the parentheses, so a format string never
// carries them.
//
// THIS FILE IS THE ONLY AUTHORITY FOR THE TAG SET. See CLAUDE.md section 1,
// rule 7. Add a new tag to the constant block and to allLogTags together.
// The Config page builds its checkbox list from allLogTags, so a tag that is
// absent from that slice can never be switched off.
//
// The three levels mean this, and the call sites keep to it:
//
//	error  A fault. An operation failed, was refused, or is not available.
//	       The reader must know, whatever the configuration says.
//	info   The outcome of an operation a person asked for, with its result.
//	debug  One step inside an operation. Useful to find a fault, and noise
//	       at every other time.
//
// The project has no leveled logger library and no structured logger. These
// three levels are the whole of it. A level is a word in the text, not an
// object.
// ---------------------------------------------------------------------

// logTag names the subsystem that wrote a line.
type logTag string

// The tag set. The value is the text between the brackets.
const (
	log404         logTag = "404"
	logAssets      logTag = "assets"
	logConfig      logTag = "config"
	logDB          logTag = "db"
	logDBBackup    logTag = "db-backup"
	logDBBootstrap logTag = "db-bootstrap"
	logDBRestore   logTag = "db-restore"
	logEdit        logTag = "edit"
	logExchange    logTag = "exchange"
	logNoteFiles   logTag = "note-files"
	logPage        logTag = "page"
	logPrecompile  logTag = "precompile"
	logRestart     logTag = "restart"
	logSearch      logTag = "search"
	logServer      logTag = "server"
	logStatus      logTag = "status"
	logStorage     logTag = "storage"
	logSync        logTag = "sync"
	logTags        logTag = "tags"
	logTemplates   logTag = "templates"
	logUpload      logTag = "upload"
)

// allLogTags is the full tag set in the order the Config page shows it.
// normalizeLogTags in config.go whitelists against this slice, so a tag that
// a person cannot see on the page also cannot survive in config.json.
var allLogTags = []logTag{
	log404,
	logAssets,
	logConfig,
	logDB,
	logDBBackup,
	logDBBootstrap,
	logDBRestore,
	logEdit,
	logExchange,
	logNoteFiles,
	logPage,
	logPrecompile,
	logRestart,
	logSearch,
	logServer,
	logStatus,
	logStorage,
	logSync,
	logTags,
	logTemplates,
	logUpload,
}

// logLevel is how much the reader wants a line.
type logLevel string

const (
	levelDebug logLevel = "debug"
	levelInfo  logLevel = "info"
	levelError logLevel = "error"
)

// logDebugf writes one step of an operation. A reader who looks for a fault
// switches this level on. It is off on a fresh install.
func (a *App) logDebugf(tag logTag, format string, args ...any) {
	a.emitLog(levelDebug, tag, format, args...)
}

// logInfof writes the outcome of an operation, with its result. It is off on
// a fresh install.
func (a *App) logInfof(tag logTag, format string, args ...any) {
	a.emitLog(levelInfo, tag, format, args...)
}

// logErrf writes a fault. This level has no switch. A person who turns a
// level off asks for less noise, and never for fewer faults.
//
// The word "error" is already in the parentheses. Do not write "Error:" or
// "Warning:" in the message as well.
func (a *App) logErrf(tag logTag, format string, args ...any) {
	a.emitLog(levelError, tag, format, args...)
}
