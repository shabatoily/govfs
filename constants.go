package vfs

import "errors"

// 기본 디렉터리 및 권한 설정 상수입니다.
const (
	// Root는 최상위 디렉터리 경로를 나타냅니다.
	Root = "/"

	// DefaultFileMode는 파일 생성 시 사용되는 기본 권한(0644)입니다.
	DefaultFileMode = 0o644
	// DefaultDirMode는 디렉터리 생성 시 사용되는 기본 권한(0755)입니다.
	DefaultDirMode = 0o755
)

// MIME types
const (
	// text
	MIMETextXML        = "text/xml"
	MIMETextHTML       = "text/html"
	MIMETextPlain      = "text/plain"
	MIMETextJavaScript = "text/javascript"
	MIMETextCSS        = "text/css"
	MIMETextMarkdown   = "text/markdown"
	MIMETextCsv        = "text/csv"

	// image
	MIMEImagePng  = "image/png"
	MIMEImageJpeg = "image/jpeg"
	MIMEImageWebp = "image/webp"
	MIMEImageGif  = "image/gif"
	MIMEImageSvg  = "image/svg+xml"

	// video
	MIMEVideoMp4       = "video/mp4"
	MIMEVideoWebm      = "video/webm"
	MIMEVideoXmsvideo  = "video/x-msvideo"
	MIMEVideoMpeg      = "video/mpeg"
	MIMEVideoQuicktime = "video/quicktime"
	MIMEVideoXmatroska = "video/x-matroska"

	// 오디오
	MIMEAudioMpeg = "audio/mpeg"
	MIMEAudioWav  = "audio/wav"

	// application
	MIMEApplicationXML        = "application/xml"
	MIMEApplicationJSON       = "application/json"
	MIMEApplicationJavaScript = "application/javascript"
	MIMEOctetStream           = "application/octet-stream"
	MIMEApplicationPdf        = "application/pdf"
)

// VFS 작업 중 발생할 수 있는 주요 에러들입니다.
var (
	ErrNotFound         = errors.New("no such file or directory")
	ErrAlreadyExists    = errors.New("file exists")
	ErrNotDir           = errors.New("not a directory")
	ErrInvalidPath      = errors.New("invalid path")
	ErrNotSupported     = errors.New("not supported")
	ErrNotSupportedSeek = errors.New("seek not supported")
)
