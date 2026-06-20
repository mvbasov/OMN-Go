package backend

const APP_VERSION = "1.4.0"

type Config struct {
	ServerPort    int               `json:"server_port"`
	AdminPassword string            `json:"admin_password"`
	GuestPassword string            `json:"guest_password"`
	Author        string            `json:"author"`
	UseInternalEd bool              `json:"use_internal_editor"`
	DesktopExtCmd string            `json:"desktop_ext_cmd"`
	MimeTypes     map[string]string `json:"mime_types"`
}

var (
	storageDir  string
	appConfig   Config
	activeConns int
)
