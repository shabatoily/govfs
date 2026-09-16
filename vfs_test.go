package vfs

import "testing"

func TestMetaMIME(t *testing.T) {
	cases := map[string]string{
		"md": MIMETextMarkdown, "txt": MIMETextPlain,
		"xml": MIMEApplicationXML, "html": MIMETextHTML, "htm": MIMETextHTML,
		"js": MIMETextJavaScript, "mjs": MIMETextJavaScript,
		"css": MIMETextCSS, "csv": MIMETextCsv, "json": MIMEApplicationJSON,
		"jpg": MIMEImageJpeg, "jpeg": MIMEImageJpeg, "PNG": MIMEImagePng,
		"webp": MIMEImageWebp, "gif": MIMEImageGif, "svg": MIMEImageSvg,
		"mp4": MIMEVideoMp4, "webm": MIMEVideoWebm, "avi": MIMEVideoXmsvideo,
		"mov": MIMEVideoQuicktime, "mkv": MIMEVideoXmatroska,
		"mpeg": MIMEVideoMpeg, "mpg": MIMEVideoMpeg,
		"mp3": MIMEAudioMpeg, "wav": MIMEAudioWav, "pdf": MIMEApplicationPdf,
		"go": MIMEOctetStream, "": MIMEOctetStream,
	}
	for ext, want := range cases {
		t.Run(ext, func(t *testing.T) {
			if got := (&Meta{Extension: ext}).MIME(); got != want {
				t.Fatalf("MIME() = %q, want %q", got, want)
			}
		})
	}
}
