package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	"image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"cozycast/internal/hub"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	maxAvatarBytes = 5 << 20
	// Allow bounded multipart headers and fields in addition to the file cap.
	multipartOverhead = 64 << 10
	avatarImageError  = "Use a PNG, JPEG, GIF or WebP image."
)

var mediaNameRe = regexp.MustCompile(`^[0-9a-f]{32,64}\.(png|jpg|jpeg|gif|webp|mp4|webm)$`)

var mediaContentTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp",
	".mp4": "video/mp4", ".webm": "video/webm",
}

var errUnsupportedMedia = errors.New("unsupported media")

// readUpload streams one multipart file to disk. The body and the file have
// separate caps so multipart overhead cannot lower the advertised file limit.
// The caller closes and removes the temporary file, including on validation failure.
func (s *Server) readUpload(w http.ResponseWriter, r *http.Request, dir, field string, limit int64) *os.File {
	r.Body = http.MaxBytesReader(w, r.Body, limit+multipartOverhead)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid upload.")
		return nil
	}
	f, err := os.CreateTemp(filepath.Join(s.mediaDir, dir), ".upload-*")
	if err != nil {
		s.internalError(w, r, err)
		return nil
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	found := false
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.uploadError(w, r, err)
			return nil
		}
		if part.FormName() == field {
			if found {
				writeError(w, http.StatusBadRequest, "Invalid upload.")
				return nil
			}
			found = true
			n, err := io.Copy(f, io.LimitReader(part, limit+1))
			if n > limit {
				writeError(w, http.StatusRequestEntityTooLarge, "File is too large.")
				return nil
			}
			if err != nil {
				s.uploadError(w, r, err)
				return nil
			}
		} else if _, err := io.Copy(io.Discard, part); err != nil {
			s.uploadError(w, r, err)
			return nil
		}
	}
	// Enforce the request cap even if data follows the final multipart boundary.
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		s.uploadError(w, r, err)
		return nil
	}
	if !found {
		writeError(w, http.StatusBadRequest, "Invalid upload.")
		return nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		s.internalError(w, r, err)
		return nil
	}
	ok = true
	return f
}

func (s *Server) uploadError(w http.ResponseWriter, r *http.Request, err error) {
	var tooBig *http.MaxBytesError
	var fileError *os.PathError
	switch {
	case errors.As(err, &tooBig):
		writeError(w, http.StatusRequestEntityTooLarge, "File is too large.")
	case errors.As(err, &fileError):
		s.internalError(w, r, err)
	default:
		writeError(w, http.StatusBadRequest, "Invalid upload.")
	}
}

// decodeUpload checks dimensions before allocating pixels. Avatars use only
// the first GIF frame; chat validates every frame and retains the original file.
func decodeUpload(f *os.File, allFrames bool) (image.Image, string, error) {
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8000 || cfg.Height > 8000 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return nil, "", errUnsupportedMedia
	}
	ext := map[string]string{"png": "png", "jpeg": "jpg", "gif": "gif", "webp": "webp"}[format]
	if ext == "" {
		return nil, "", errUnsupportedMedia
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	if format == "gif" && allFrames {
		animation, err := gif.DecodeAll(f)
		if err != nil {
			return nil, "", errUnsupportedMedia
		}
		return animation.Image[0], ext, nil
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, "", errUnsupportedMedia
	}
	return img, ext, nil
}

func randomMediaName(ext string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]) + "." + ext, nil
}

func (s *Server) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	f := s.readUpload(w, r, "avatars", "avatar", maxAvatarBytes)
	if f == nil {
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	img, _, err := decodeUpload(f, false)
	if errors.Is(err, errUnsupportedMedia) {
		writeError(w, http.StatusUnsupportedMediaType, avatarImageError)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	bounds := img.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	x, y := bounds.Min.X+(bounds.Dx()-side)/2, bounds.Min.Y+(bounds.Dy()-side)/2
	avatar := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	draw.CatmullRom.Scale(avatar, avatar.Bounds(), img, image.Rect(x, y, x+side, y+side), draw.Src, nil)
	name, err := randomMediaName("png")
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out, err := os.OpenFile(filepath.Join(s.mediaDir, "avatars", name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	encodeErr := png.Encode(out, avatar)
	closeErr := out.Close()
	if err := errors.Join(encodeErr, closeErr); err != nil {
		s.removeMediaFile("avatars", name)
		s.internalError(w, r, err)
		return
	}
	if !s.saveAvatar(w, r, u.ID, name) {
		s.removeMediaFile("avatars", name)
	}
}

func (s *Server) deleteAvatar(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	s.saveAvatar(w, r, u.ID, "")
}

func (s *Server) saveAvatar(w http.ResponseWriter, r *http.Request, id int64, name string) bool {
	s.avatarMu.Lock()
	defer s.avatarMu.Unlock()
	// Re-read under the lock so concurrent replacements remove the latest file.
	u, err := s.store.UserByID(r.Context(), id)
	if err == nil {
		err = s.store.UpdateAvatar(r.Context(), id, name)
	}
	if err != nil {
		s.internalError(w, r, err)
		return false
	}
	s.removeMediaFile("avatars", u.Avatar)
	u.Avatar = name
	s.hub.UserChanged(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"user": toMe(u)})
	return true
}

func (s *Server) removeMediaFile(dir, name string) {
	// Stored names may predate the current naming scheme; still require a
	// single filename so cleanup can never escape its media directory.
	if s.mediaDir == "" || name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return
	}
	if err := os.Remove(filepath.Join(s.mediaDir, dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.log.Warn("remove media", "dir", dir, "file", name, "err", err)
	}
}

func (s *Server) mediaPostError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, hub.ErrNotPresent):
		writeError(w, http.StatusForbidden, "Join the room first.")
	case errors.Is(err, hub.ErrNotAllowed):
		writeError(w, http.StatusForbidden, "You are not allowed to post images.")
	default:
		s.internalError(w, r, err)
	}
}

func (s *Server) uploadChatMedia(w http.ResponseWriter, r *http.Request) {
	rm := s.hub.Room(r.PathValue("room"))
	if rm == nil {
		writeError(w, http.StatusNotFound, "Unknown room.")
		return
	}
	id, err := s.auth.Identify(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := rm.CanPostMedia(id.Key()); err != nil {
		s.mediaPostError(w, r, err)
		return
	}
	f := s.readUpload(w, r, "chat", "file", s.maxUploadBytes)
	if f == nil {
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	var header [12]byte
	n, err := f.Read(header[:])
	if err != nil && !errors.Is(err, io.EOF) {
		s.internalError(w, r, err)
		return
	}
	typ, ext := "video", ""
	switch {
	case n >= 8 && string(header[4:8]) == "ftyp":
		ext = "mp4"
	case n >= 4 && string(header[:4]) == "\x1a\x45\xdf\xa3":
		ext = "webm"
	default:
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			s.internalError(w, r, err)
			return
		}
		_, ext, err = decodeUpload(f, true)
		if errors.Is(err, errUnsupportedMedia) {
			writeError(w, http.StatusUnsupportedMediaType, "Unsupported file type.")
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		typ = "image"
	}
	name, err := randomMediaName(ext)
	if err == nil {
		err = f.Chmod(0o640)
	}
	if err == nil {
		err = f.Close()
	}
	if err == nil {
		err = os.Rename(f.Name(), filepath.Join(s.mediaDir, "chat", name))
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := rm.PostMedia(r.Context(), id.Key(), typ, name); err != nil {
		s.removeMediaFile("chat", name)
		s.mediaPostError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveAvatar(w http.ResponseWriter, r *http.Request) {
	s.serveMedia(w, r, "avatars", "public, max-age=31536000, immutable")
}

func (s *Server) serveChatMedia(w http.ResponseWriter, r *http.Request) {
	s.serveMedia(w, r, "chat", "private, max-age=3600")
}

// Reject malformed media paths before ServeMux redirects cleaned paths,
// so traversal attempts cannot redirect to another directory or the web UI.
func validateMediaPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/media/") {
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) != 4 || (parts[2] != "avatars" && parts[2] != "chat") || !mediaNameRe.MatchString(parts[3]) {
				http.NotFound(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request, dir, cache string) {
	name := r.PathValue("file")
	if s.mediaDir == "" || !mediaNameRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.mediaDir, dir, name))
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mediaContentTypes[filepath.Ext(name)])
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", cache)
	http.ServeContent(w, r, name, info.ModTime(), f)
}
