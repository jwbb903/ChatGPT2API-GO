package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	dir string
	mu  sync.RWMutex
}

func NewStore(dir string) *Store {
	_ = os.MkdirAll(dir, 0755)
	return &Store{dir: dir}
}

func (s *Store) path(name string) string { return filepath.Join(s.dir, name) }

func readJSONFile[T any](path string, fallback T) T {
	b, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return fallback
	}
	return out
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) LoadAccounts() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := readJSONFile(s.path("accounts.json"), []Account{})
	out := make([]Account, 0, len(items))
	for _, a := range items {
		if a.AccessToken == "" {
			continue
		}
		if a.Type == "" {
			a.Type = "free"
		}
		if a.Status == "" {
			a.Status = "正常"
		}
		if a.SourceType == "" {
			a.SourceType = "web"
		}
		if a.InitialQuota < a.Quota {
			a.InitialQuota = a.Quota
		}
		out = append(out, a)
	}
	return out
}

func (s *Store) SaveAccounts(items []Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.path("accounts.json"), items)
}

type authKeysWrap struct {
	Items []UserKey `json:"items"`
}

func (s *Store) LoadAuthKeys() []UserKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path := s.path("auth_keys.json")
	wrap := readJSONFile(path, authKeysWrap{})
	if len(wrap.Items) > 0 {
		return normalizeKeys(wrap.Items)
	}
	arr := readJSONFile(path, []UserKey{})
	return normalizeKeys(arr)
}

func (s *Store) SaveAuthKeys(items []UserKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.path("auth_keys.json"), authKeysWrap{Items: items})
}

func (s *Store) LoadGallery() []GalleryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readJSONFile(s.path("gallery.json"), []GalleryItem{})
}
func (s *Store) SaveGallery(items []GalleryItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.path("gallery.json"), items)
}
func (s *Store) LoadLogs() []LogItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readJSONFile(s.path("logs.json"), []LogItem{})
}
func (s *Store) SaveLogs(items []LogItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.path("logs.json"), items)
}
func (s *Store) LoadTasks() []ImageTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readJSONFile(s.path("image_tasks.json"), []ImageTask{})
}
func (s *Store) SaveTasks(items []ImageTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.path("image_tasks.json"), items)
}
func (s *Store) LoadOwners() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readJSONFile(s.path("image_owners.json"), map[string]string{})
}
func (s *Store) SaveOwners(items map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.path("image_owners.json"), items)
}
func (s *Store) LoadPrompts() map[string]map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readJSONFile(s.path("image_prompts.json"), map[string]map[string]any{})
}
func (s *Store) SavePrompts(items map[string]map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.path("image_prompts.json"), items)
}
func (s *Store) LoadTags() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readJSONFile(s.path("image_tags.json"), map[string][]string{})
}
func (s *Store) SaveTags(items map[string][]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.path("image_tags.json"), items)
}
func (s *Store) LoadList(name string) []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readJSONFile(s.path(name), []map[string]any{})
}
func (s *Store) SaveList(name string, v []map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.path(name), v)
}

func ensureNotDir(path string) error {
	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.IsDir() {
		return errors.New(path + " is a directory")
	}
	return nil
}

func hashKey(key string) string { h := sha256.Sum256([]byte(key)); return hex.EncodeToString(h[:]) }
