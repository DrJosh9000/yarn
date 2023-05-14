// Copyright 2021 Josh Deprez
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package yarn

import "sync"

// VariableStorage stores values of any kind.
type VariableStorage interface {
	GetValue(name string) (value any, ok bool)
	SetValue(name string, value any)
}

// MapVariableStorage implements VariableStorage, in memory, using a map.
// In addition to the core VariableStorage functionality, there are methods for
// accessing the contents as an ordinary map[string]any.
type MapVariableStorage struct {
	mu sync.RWMutex
	m  map[string]any
}

// NewMapVariableStorage creates a new empty MapVariableStorage.
func NewMapVariableStorage() *MapVariableStorage {
	return &MapVariableStorage{
		m: make(map[string]any),
	}
}

// NewMapVariableStorageFromMap creates a new MapVariableStorage with initial
// contents copied from src. It does not keep a reference to src.
func NewMapVariableStorageFromMap(src map[string]any) *MapVariableStorage {
	return &MapVariableStorage{
		m: copyMap(src),
	}
}

// Clear empties the storage of all values.
func (m *MapVariableStorage) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name := range m.m {
		delete(m.m, name)
	}
}

// GetValue fetches a value from the storage, returning (nil, false) if not present.
func (m *MapVariableStorage) GetValue(name string) (value any, found bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, found = m.m[name]
	return value, found
}

// SetValue sets a value in the storage.
func (m *MapVariableStorage) SetValue(name string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[name] = value
}

// Delete deletes values from the storage.
func (m *MapVariableStorage) Delete(names ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, name := range names {
		delete(m.m, name)
	}
}

// Contents returns a copy of the contents of the storage, as a regular map.
// The returned map is a copy, it is not a reference to the map contained within
// the storage (to avoid accidental data races).
func (m *MapVariableStorage) Contents() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return copyMap(m.m)
}

// Clone returns a new MapVariableStorage that is a clone of the receiver.
// The new storage is a deep copy, and does not contain a reference to the
// original map inside the receiver (to avoid accidental data races).
func (m *MapVariableStorage) Clone() *MapVariableStorage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return NewMapVariableStorageFromMap(m.m)
}

// ReplaceContents replaces the contents of the storage with values from a
// regular map. ReplaceContents copies src, it does not keep a reference to src
// (to avoid accidental data races).
func (m *MapVariableStorage) ReplaceContents(src map[string]any) {
	m2 := copyMap(src)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m = m2
}

func copyMap[K comparable, V any](src map[K]V) map[K]V {
	m := make(map[K]V, len(src))
	for name, val := range src {
		m[name] = val
	}
	return m
}
