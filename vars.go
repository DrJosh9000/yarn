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

// VariableStorage stores values of any kind.
type VariableStorage interface {
	Clear()
	GetValue(name string) (value interface{}, ok bool)
	SetValue(name string, value interface{})
}

// MapVariableStorage implements VariableStorage, in memory, using a map.
type MapVariableStorage map[string]interface{}

// Clear empties the storage of all values.
func (m MapVariableStorage) Clear() {
	for name := range m {
		delete(m, name)
	}
}

// GetValue fetches a value from the map, returning (nil, false) if not present.
func (m MapVariableStorage) GetValue(name string) (value interface{}, found bool) {
	value, found = m[name]
	return value, found
}

// SetValue sets a value in the map.
func (m MapVariableStorage) SetValue(name string, value interface{}) {
	m[name] = value
}
