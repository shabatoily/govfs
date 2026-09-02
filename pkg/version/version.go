package version

var (
	version   = "dev"
	buildTime = "unknown"
)

func Version() string {
	return version
}

func BuildTime() string {
	return buildTime
}
