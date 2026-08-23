package media

import (
	"bytes"
	"errors"
	"strings"
)

type Kind struct {
	// ContentType is what the object is stored and served as. The claim made
	// by the uploader is never trusted.
	ContentType string
	// Category is the attachment type the frontend switches on.
	Category string
	// Folder is the storage prefix.
	Folder string
	// Inline is false for anything a browser should download rather than render.
	Inline bool
}

var (
	ErrUnrecognised = errors.New("unrecognised file type")
	ErrMismatch     = errors.New("file content does not match its content type")
)

var (
	image = func(ct string) Kind { return Kind{ContentType: ct, Category: "image", Folder: "images", Inline: true} }
	audio = func(ct string) Kind { return Kind{ContentType: ct, Category: "audio", Folder: "audio", Inline: true} }
	video = func(ct string) Kind { return Kind{ContentType: ct, Category: "video", Folder: "video", Inline: true} }
	file  = func(ct string) Kind { return Kind{ContentType: ct, Category: "file", Folder: "files", Inline: false} }
)

// Detect identifies a payload from its leading bytes. Anything not on this
// list is rejected: the bucket is served from the app's own origin, so a file
// a browser might execute as markup would be stored XSS.
func Detect(data []byte) (Kind, bool) {
	switch {
	case len(data) < 12:
		return Kind{}, false

	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return image("image/jpeg"), true
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return image("image/png"), true
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return image("image/gif"), true
	case bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return image("image/webp"), true
	case bytes.HasPrefix(data, []byte("BM")):
		return image("image/bmp"), true

	case bytes.HasPrefix(data, []byte("ID3")):
		return audio("audio/mpeg"), true
	case data[0] == 0xFF && (data[1] == 0xFB || data[1] == 0xFA || data[1] == 0xF3 || data[1] == 0xF2):
		return audio("audio/mpeg"), true
	case bytes.HasPrefix(data, []byte("fLaC")):
		return audio("audio/flac"), true
	case bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WAVE")):
		return audio("audio/wav"), true
	case bytes.HasPrefix(data, []byte("OggS")):
		return oggKind(data), true

	case bytes.HasPrefix(data, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return video("video/webm"), true
	case bytes.Equal(data[4:8], []byte("ftyp")):
		return ftypKind(data), true

	case bytes.HasPrefix(data, []byte("%PDF-")):
		return file("application/pdf"), true
	case bytes.HasPrefix(data, []byte("PK\x03\x04")),
		bytes.HasPrefix(data, []byte("PK\x05\x06")),
		bytes.HasPrefix(data, []byte("PK\x07\x08")):
		return file("application/zip"), true
	}
	return Kind{}, false
}

// oggKind separates Ogg video from Ogg audio by the codec identifier that
// follows the page header.
func oggKind(data []byte) Kind {
	head := data
	if len(head) > 128 {
		head = head[:128]
	}
	if bytes.Contains(head, []byte("theora")) || bytes.Contains(head, []byte("\x80theora")) {
		return video("video/ogg")
	}
	return audio("audio/ogg")
}

// ftypKind distinguishes the ISO base media brands we accept.
func ftypKind(data []byte) Kind {
	brand := ""
	if len(data) >= 12 {
		brand = strings.ToLower(strings.TrimSpace(string(data[8:12])))
	}
	switch brand {
	case "m4a ", "m4a", "m4b", "m4p":
		return audio("audio/mp4")
	case "avif", "avis":
		return image("image/avif")
	case "heic", "heix", "heim", "heis", "hevc", "hevx":
		return image("image/heic")
	default:
		return video("video/mp4")
	}
}

// Validate sniffs the payload and confirms the uploader's claimed content type
// agrees on the broad category. The returned Kind is what should be stored.
func Validate(data []byte, claimedContentType string) (Kind, error) {
	kind, ok := Detect(data)
	if !ok {
		return Kind{}, ErrUnrecognised
	}
	claimed := normalise(claimedContentType)
	if claimed == "" || claimed == "application/octet-stream" {
		return kind, nil
	}
	if categoryOf(claimed) != categoryOf(kind.ContentType) {
		return Kind{}, ErrMismatch
	}
	return kind, nil
}

func normalise(ct string) string {
	ct, _, _ = strings.Cut(ct, ";")
	return strings.ToLower(strings.TrimSpace(ct))
}

func categoryOf(ct string) string {
	cat, _, _ := strings.Cut(ct, "/")
	return cat
}
