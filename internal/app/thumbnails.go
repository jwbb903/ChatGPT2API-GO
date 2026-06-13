package app

import (
	"image"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) thumbnailPath(rel string) string {
	return filepath.Join(s.dataDir, "image_thumbnails", filepath.FromSlash(rel)+".jpg")
}

func (s *Server) serveThumbnail(w http.ResponseWriter, r *http.Request, rel string) {
	rel = relClean(rel)
	thumb := s.thumbnailPath(rel)
	if _, err := os.Stat(thumb); err == nil {
		http.ServeFile(w, r, thumb)
		return
	}
	src := filepath.Join(s.imagesDir, filepath.FromSlash(rel))
	f, err := os.Open(src)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		http.ServeFile(w, r, src)
		return
	}
	bounds := img.Bounds()
	w0, h0 := bounds.Dx(), bounds.Dy()
	if w0 <= 0 || h0 <= 0 {
		http.ServeFile(w, r, src)
		return
	}
	maxSide := 360
	nw, nh := w0, h0
	if w0 >= h0 && w0 > maxSide {
		nw = maxSide
		nh = h0 * maxSide / w0
	} else if h0 > w0 && h0 > maxSide {
		nh = maxSide
		nw = w0 * maxSide / h0
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			sx := bounds.Min.X + x*w0/nw
			sy := bounds.Min.Y + y*h0/nh
			dst.Set(x, y, img.At(sx, sy))
		}
	}
	_ = os.MkdirAll(filepath.Dir(thumb), 0755)
	out, err := os.Create(thumb)
	if err != nil {
		http.ServeFile(w, r, src)
		return
	}
	defer out.Close()
	_ = jpeg.Encode(out, dst, &jpeg.Options{Quality: 82})
	http.ServeFile(w, r, thumb)
}

func isImagePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp"
}
