package app

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Server) makePlaceholder(prompt, size string) ([]byte, error) {
	w, h := 1024, 1024
	switch size {
	case "16:9":
		w, h = 1280, 720
	case "9:16":
		w, h = 720, 1280
	case "4:3":
		w, h = 1152, 864
	case "3:4":
		w, h = 864, 1152
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := md5.Sum([]byte(prompt))
	c1 := color.RGBA{seed[0], seed[1], seed[2], 255}
	c2 := color.RGBA{seed[3], seed[4], seed[5], 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := float64(x+y) / float64(w+h)
			img.SetRGBA(x, y, color.RGBA{uint8(float64(c1.R)*(1-t) + float64(c2.R)*t), uint8(float64(c1.G)*(1-t) + float64(c2.G)*t), uint8(float64(c1.B)*(1-t) + float64(c2.B)*t), 255})
		}
	}
	for i := 0; i < 8; i++ {
		r := image.Rect((i+1)*w/12, (i+1)*h/12, (i+4)*w/12, (i+2)*h/12)
		draw.Draw(img, r, &image.Uniform{color.RGBA{255, 255, 255, 40}}, image.Point{}, draw.Over)
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	return buf.Bytes(), err
}

func (s *Server) saveImage(r *http.Request, data []byte) (string, string, error) {
	s.cleanupOldImages()
	sum := md5.Sum(data)
	relDir := filepath.Join(time.Now().Format("2006"), time.Now().Format("01"), time.Now().Format("02"))
	name := fmt.Sprintf("%d_%x.png", time.Now().Unix(), sum)
	rel := filepath.ToSlash(filepath.Join(relDir, name))
	path := filepath.Join(s.imagesDir, relDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", "", err
	}
	return rel, s.baseURL(r) + "/images/" + rel, nil
}
func (s *Server) recordOwner(id *Identity, rel string) {
	owners := s.store.LoadOwners()
	owners[rel] = id.ID
	_ = s.store.SaveOwners(owners)
}
func (s *Server) recordPrompt(rel, prompt string, isEdit bool) {
	ps := s.store.LoadPrompts()
	ps[rel] = map[string]any{"prompt": prompt, "is_edit": isEdit, "created_at": time.Now().Unix()}
	_ = s.store.SavePrompts(ps)
}
func (s *Server) cleanupOldImages() int {
	days := s.cfg.ImageRetentionDays
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	protected := map[string]bool{}
	if s.cfg.CleanupProtectGallery {
		for _, it := range s.store.LoadGallery() {
			if rel := relClean(it.ImageRel); rel != "" {
				protected[rel] = true
			}
		}
	}
	if s.cfg.CleanupProtectUserImages {
		for rel, owner := range s.store.LoadOwners() {
			owner = strings.ToLower(strings.TrimSpace(owner))
			if rel = relClean(rel); rel != "" && owner != "" && owner != "admin" && owner != "__admin__" {
				protected[rel] = true
			}
		}
	}
	removed := 0
	_ = filepath.WalkDir(s.imagesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		st, err := d.Info()
		if err != nil || st.ModTime().After(cutoff) {
			return nil
		}
		rel, err := filepath.Rel(s.imagesDir, path)
		if err == nil && protected[filepath.ToSlash(rel)] {
			return nil
		}
		if os.Remove(path) == nil {
			removed++
		}
		return nil
	})
	// 清理空目录，深目录优先
	dirs := []string{}
	_ = filepath.WalkDir(s.imagesDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != s.imagesDir {
			dirs = append(dirs, path)
		}
		return nil
	})
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		_ = os.Remove(d)
	}
	if removed > 0 && s.logSvc != nil {
		s.logSvc.add("system", "清理旧图片", map[string]any{"removed": removed, "retention_days": days})
	}
	return removed
}

func relFromURL(u string) string {
	if i := strings.Index(u, "/images/"); i >= 0 {
		return relClean(u[i+8:])
	}
	return relClean(u)
}

func (s *Server) listImages(r *http.Request, ownerFilter string) map[string]any {
	owners := s.store.LoadOwners()
	prompts := s.store.LoadPrompts()
	tags := s.store.LoadTags()
	items := []map[string]any{}
	filepath.WalkDir(s.imagesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
			return nil
		}
		rel, _ := filepath.Rel(s.imagesDir, path)
		rel = filepath.ToSlash(rel)
		owner := owners[rel]
		if ownerFilter != "" {
			if ownerFilter == "__unowned__" && owner != "" {
				return nil
			}
			if ownerFilter != "__unowned__" && owner != ownerFilter {
				return nil
			}
		}
		st, _ := d.Info()
		pr := prompts[rel]
		items = append(items, map[string]any{"rel": rel, "path": rel, "name": d.Name(), "date": st.ModTime().Format("2006-01-02"), "size": st.Size(), "url": s.baseURL(r) + "/images/" + rel, "thumbnail_url": s.baseURL(r) + "/image-thumbnails/" + rel, "created_at": st.ModTime().Format(time.RFC3339), "tags": tags[rel], "owner_id": owner, "prompt": strAny(pr["prompt"], "")})
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return strAny(items[i]["created_at"], "") > strAny(items[j]["created_at"], "") })
	return map[string]any{"items": items}
}
func (s *Server) handleImages(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	writeJSON(w, 200, s.listImages(r, r.URL.Query().Get("owner")))
}
func (s *Server) handleMyImages(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	owner := id.ID
	if id.Role == "admin" {
		owner = "admin"
	}
	writeJSON(w, 200, s.listImages(r, owner))
}
func (s *Server) handleImageOwners(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	owners := s.store.LoadOwners()
	counts := map[string]int{}
	for _, o := range owners {
		counts[o]++
	}
	items := []map[string]any{{"id": "__admin__", "name": "管理员", "deleted": false, "count": counts["admin"]}, {"id": "__unowned__", "name": "未归属", "deleted": false, "count": 0}}
	for _, k := range s.store.LoadAuthKeys() {
		if k.Role == "user" {
			items = append(items, map[string]any{"id": k.ID, "name": k.Name, "deleted": false, "count": counts[k.ID]})
		}
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) handleImageDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	var b struct {
		Paths []string `json:"paths"`
	}
	if !readBody(w, r, &b) {
		return
	}
	owners := s.store.LoadOwners()
	removed := 0
	for _, p := range b.Paths {
		rel := relClean(p)
		if id.Role != "admin" && owners[rel] != id.ID {
			continue
		}
		if os.Remove(filepath.Join(s.imagesDir, filepath.FromSlash(rel))) == nil {
			removed++
			delete(owners, rel)
		}
	}
	_ = s.store.SaveOwners(owners)
	writeJSON(w, 200, map[string]any{"removed": removed})
}
func (s *Server) handleImageDownload(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var b struct {
		Paths []string `json:"paths"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range b.Paths {
		rel := relClean(p)
		data, err := os.ReadFile(filepath.Join(s.imagesDir, filepath.FromSlash(rel)))
		if err == nil {
			f, _ := zw.Create(filepath.Base(rel))
			_, _ = f.Write(data)
		}
	}
	_ = zw.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=images.zip")
	_, _ = w.Write(buf.Bytes())
}
func (s *Server) handleImageDownloadSingle(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/api/images/download/")
	http.ServeFile(w, r, filepath.Join(s.imagesDir, filepath.FromSlash(rel)))
}
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/image-thumbnails/")
	s.serveThumbnail(w, r, rel)
}
func (s *Server) handleImageTags(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	tags := s.store.LoadTags()
	if r.Method == http.MethodGet {
		set := map[string]bool{}
		for _, ts := range tags {
			for _, t := range ts {
				set[t] = true
			}
		}
		arr := []string{}
		for t := range set {
			arr = append(arr, t)
		}
		sort.Strings(arr)
		writeJSON(w, 200, map[string]any{"tags": arr})
		return
	}
	if r.Method == http.MethodPost {
		var b struct {
			Path string   `json:"path"`
			Tags []string `json:"tags"`
		}
		if !readBody(w, r, &b) {
			return
		}
		tags[relClean(b.Path)] = b.Tags
		_ = s.store.SaveTags(tags)
		writeJSON(w, 200, map[string]any{"ok": true, "tags": b.Tags})
		return
	}
	writeErr(w, 405, "method not allowed")
}
func (s *Server) handleImageTagDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	tag, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/images/tags/"))
	tags := s.store.LoadTags()
	n := 0
	for rel, ts := range tags {
		out := []string{}
		for _, t := range ts {
			if t == tag {
				n++
			} else {
				out = append(out, t)
			}
		}
		tags[rel] = out
	}
	_ = s.store.SaveTags(tags)
	writeJSON(w, 200, map[string]any{"ok": true, "removed_from": n})
}

var _ = base64.StdEncoding
var _ = io.Copy
