package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Manager управляет списком профилей на диске и в памяти.
type Manager struct {
	mu          sync.RWMutex
	dir         string
	profiles    []Item
	activeID    string
}

// NewManager создает новый экземпляр менеджера профилей.
func NewManager(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating profiles dir: %w", err)
	}

	m := &Manager{
		dir:      dir,
		profiles: make([]Item, 0),
	}

	if err := m.LoadAll(); err != nil {
		return nil, err
	}

	return m, nil
}

// LoadAll сканирует директорию profiles и загружает все сохраненные JSON файлы.
func (m *Manager) LoadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return fmt.Errorf("reading profiles dir: %w", err)
	}

	profiles := make([]Item, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(m.dir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var item Item
		if err := json.Unmarshal(data, &item); err == nil && item.ID != "" {
			profiles = append(profiles, item)
		}
	}

	m.profiles = profiles
	if len(m.profiles) > 0 && m.activeID == "" {
		m.activeID = m.profiles[0].ID
	}
	return nil
}

// Add добавляет или обновляет профиль, сохраняя его в файл.
func (m *Manager) Add(item Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling profile: %w", err)
	}

	filePath := filepath.Join(m.dir, item.ID+".json")
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("saving profile to disk: %w", err)
	}

	// Обновляем список в памяти
	found := false
	for i, p := range m.profiles {
		if p.ID == item.ID {
			m.profiles[i] = item
			found = true
			break
		}
	}
	if !found {
		m.profiles = append(m.profiles, item)
	}

	if m.activeID == "" {
		m.activeID = item.ID
	}
	return nil
}

// Delete удаляет профиль по ID.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	filePath := filepath.Join(m.dir, id+".json")
	_ = os.Remove(filePath)

	filtered := make([]Item, 0, len(m.profiles))
	for _, p := range m.profiles {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	m.profiles = filtered

	if m.activeID == id {
		if len(m.profiles) > 0 {
			m.activeID = m.profiles[0].ID
		} else {
			m.activeID = ""
		}
	}
	return nil
}

// List возвращает копию списка профилей.
func (m *Manager) List() []Item {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]Item, len(m.profiles))
	copy(res, m.profiles)
	return res
}

// SetActive задает активный профиль.
func (m *Manager) SetActive(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.profiles {
		if p.ID == id {
			m.activeID = id
			return nil
		}
	}
	return fmt.Errorf("profile with id %s not found", id)
}

// Active возвращает текущий активный профиль или nil.
func (m *Manager) Active() *Item {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.profiles {
		if p.ID == m.activeID {
			itemCopy := p
			return &itemCopy
		}
	}
	return nil
}
