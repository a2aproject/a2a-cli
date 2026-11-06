// Copyright 2026 The A2A Authors
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

// Package contacts implements the user's contact book: named A2A agents the
// user trusts, persisted in a YAML file.
package contacts

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/a2aproject/a2a-cli/internal/atomicfile"
)

const fileName = "contacts.yaml"

var (
	// ErrExists is returned when adding a contact whose name is taken.
	ErrExists = errors.New("contact already exists")
	// ErrNotFound is returned when a named contact is not in the book.
	ErrNotFound = errors.New("contact not found")
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// Contact is a named A2A agent in the contact book.
type Contact struct {
	Name string `yaml:"-" json:"name"`
	// Card is the normalized agent card reference: an origin, a card URL, or a file:// URL.
	Card    string    `yaml:"card" json:"card"`
	Note    string    `yaml:"note,omitempty" json:"note,omitempty"`
	AddedAt time.Time `yaml:"added-at" json:"addedAt"`
}

// Origin returns scheme://host of the contact's card reference, or "" for a file reference.
func (c Contact) Origin() string {
	return Origin(c.Card)
}

// Origin returns scheme://host of an http(s) URL, or "" for anything else.
func Origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// IsName reports whether s is a valid contact name. Names can't contain dots,
// colons or slashes, so they never collide with a host, URL or file path.
func IsName(s string) bool {
	return namePattern.MatchString(s) && s != "localhost"
}

// ValidateName returns a descriptive error when name is not a valid contact name.
func ValidateName(name string) error {
	if IsName(name) {
		return nil
	}
	return fmt.Errorf("invalid contact name %q: use lowercase letters, digits, '-' and '_' (max 63 characters, not \"localhost\")", name)
}

// DefaultPath returns the path of the user's contact book.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "a2a-cli", fileName), nil
}

// Book is a contact book loaded from a file.
type Book struct {
	path     string
	contacts map[string]Contact
}

type fileFormat struct {
	Contacts map[string]Contact `yaml:"contacts"`
}

// Load reads the contact book at path. A missing file is an empty book.
func Load(path string) (*Book, error) {
	book := &Book{path: path, contacts: map[string]Contact{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return book, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading contacts %q: %w", path, err)
	}
	var f fileFormat
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing contacts %q: %w", path, err)
	}
	for name, c := range f.Contacts {
		if !IsName(name) || c.Card == "" {
			continue
		}
		c.Name = name
		book.contacts[name] = c
	}
	return book, nil
}

// Path returns the file the book was loaded from.
func (b *Book) Path() string { return b.path }

// Get returns the named contact.
func (b *Book) Get(name string) (Contact, bool) {
	c, ok := b.contacts[name]
	return c, ok
}

// List returns all contacts sorted by name.
func (b *Book) List() []Contact {
	out := make([]Contact, 0, len(b.contacts))
	for _, c := range b.contacts {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Contact) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Add stores c, replacing an existing contact of the same name only when replace is set.
func (b *Book) Add(c Contact, replace bool) error {
	if err := ValidateName(c.Name); err != nil {
		return err
	}
	if c.Card == "" {
		return fmt.Errorf("contact %q: empty card reference", c.Name)
	}
	if _, ok := b.contacts[c.Name]; ok && !replace {
		return fmt.Errorf("%w: %s", ErrExists, c.Name)
	}
	b.contacts[c.Name] = c
	return nil
}

// Remove deletes the named contact.
func (b *Book) Remove(name string) error {
	if _, ok := b.contacts[name]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	delete(b.contacts, name)
	return nil
}

// Save writes the book back to its file atomically.
func (b *Book) Save() error {
	data, err := yaml.Marshal(fileFormat{Contacts: b.contacts})
	if err != nil {
		return fmt.Errorf("encoding contacts: %w", err)
	}
	if err := atomicfile.Write(b.path, data); err != nil {
		return fmt.Errorf("writing contacts %q: %w", b.path, err)
	}
	return nil
}
