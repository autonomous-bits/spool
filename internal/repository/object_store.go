package repository

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"lukechampine.com/blake3"
)

var (
	errInvalidLooseObjectID = errors.New("invalid loose object ID")
	errLooseObjectNotFound  = errors.New("loose object not found")
	errLooseObjectCorrupt   = errors.New("corrupt loose object")
	errLooseObjectType      = errors.New("loose object type mismatch")
)

// looseObjectEnvelope binds canonical object bytes to their repository type.
// Its canonical CBOR representation is the on-disk loose-object format.
type looseObjectEnvelope struct {
	Type string `cbor:"1,keyasint"`
	Data []byte `cbor:"2,keyasint"`
}

const objectIDHexLength = 64

// looseObjectStore keeps canonical object bytes in memory and, when stateDir is
// set, durably mirrors them below .spl/objects/loose.
type looseObjectStore struct {
	looseDir string
	cache    *map[ObjectID][]byte
}

func newLooseObjectStore(stateDir string, cache *map[ObjectID][]byte) *looseObjectStore {
	store := &looseObjectStore{cache: cache}
	if stateDir != "" {
		store.looseDir = filepath.Join(stateDir, "objects", "loose")
	}
	return store
}

func (s *looseObjectStore) put(objectType string, value any) (ObjectID, error) {
	encoded, err := canonicalObjectEncoding(value)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", objectType, err)
	}
	return s.putEncoded(objectType, encoded)
}

func (s *looseObjectStore) putEncoded(objectType string, encoded []byte) (ObjectID, error) {
	if objectType == "" {
		return "", fmt.Errorf("%w: empty type", errLooseObjectCorrupt)
	}
	id := objectIDForEncoded(objectType, encoded)
	if cached, ok := (*s.cache)[id]; ok {
		if objectIDForEncoded(objectType, cached) != id {
			return "", fmt.Errorf("%w: cached %s", errLooseObjectCorrupt, id)
		}
	}
	if s.looseDir != "" {
		path, err := s.path(id)
		if err != nil {
			return "", err
		}
		if err := s.ensureDurable(path, id, objectType, encoded); err != nil {
			return "", err
		}
	}
	(*s.cache)[id] = append([]byte(nil), encoded...)
	return id, nil
}

func (s *looseObjectStore) get(id ObjectID, objectType string) ([]byte, error) {
	if objectType == "" {
		return nil, fmt.Errorf("%w: empty expected type", errLooseObjectType)
	}
	if !validLooseObjectID(id) {
		return nil, fmt.Errorf("%w: %q", errInvalidLooseObjectID, id)
	}
	if cached, ok := (*s.cache)[id]; ok {
		if objectIDForEncoded(objectType, cached) != id {
			return nil, fmt.Errorf("%w: cached %s", errLooseObjectCorrupt, id)
		}
		return append([]byte(nil), cached...), nil
	}
	if s.looseDir == "" {
		return nil, fmt.Errorf("%w: %s", errLooseObjectNotFound, id)
	}
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}
	encoded, err := readLooseObject(path, id, objectType)
	if err != nil {
		return nil, err
	}
	(*s.cache)[id] = append([]byte(nil), encoded...)
	return encoded, nil
}

func (s *looseObjectStore) ensureDurable(path string, id ObjectID, objectType string, encoded []byte) error {
	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s is not a regular file", errLooseObjectCorrupt, id)
		}
		existing, err := readLooseObject(path, id, objectType)
		if err != nil {
			return err
		}
		if !bytes.Equal(existing, encoded) {
			return fmt.Errorf("%w: payload mismatch for %s", errLooseObjectCorrupt, id)
		}
		return nil
	case !os.IsNotExist(err):
		return fmt.Errorf("inspect loose object %s: %w", id, err)
	}

	if err := ensureLooseObjectDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("create loose object directory: %w", err)
	}
	envelope, err := canonicalCBOR.Marshal(looseObjectEnvelope{Type: objectType, Data: encoded})
	if err != nil {
		return fmt.Errorf("encode loose object envelope: %w", err)
	}
	if err := writeDurableStateFile(path, envelope); err != nil {
		return fmt.Errorf("write loose object %s: %w", id, err)
	}
	return nil
}

// ensureLooseObjectDirectory makes each newly linked directory durable before
// an object file can be published below it.
func ensureLooseObjectDirectory(path string) error {
	var missing []string
	for current := path; ; current = filepath.Dir(current) {
		_, err := os.Stat(current)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("inspect loose object directory: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("find existing loose object directory parent: %w", errLooseObjectCorrupt)
		}
		missing = append(missing, current)
	}
	if len(missing) == 0 {
		return nil
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		if err := syncMergeStateDirectory(filepath.Dir(missing[index])); err != nil {
			return err
		}
	}
	return nil
}

func readLooseObject(path string, id ObjectID, objectType string) ([]byte, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", errLooseObjectNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("inspect loose object %s: %w", id, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", errLooseObjectCorrupt, id)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read loose object %s: %w", id, err)
	}
	var envelope looseObjectEnvelope
	if err := cbor.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("%w: decode %s: %v", errLooseObjectCorrupt, id, err)
	}
	canonical, err := canonicalCBOR.Marshal(envelope)
	if err != nil || !bytes.Equal(data, canonical) {
		return nil, fmt.Errorf("%w: non-canonical envelope %s", errLooseObjectCorrupt, id)
	}
	if envelope.Type != objectType {
		return nil, fmt.Errorf("%w: %s is %q, want %q", errLooseObjectType, id, envelope.Type, objectType)
	}
	if objectIDForEncoded(envelope.Type, envelope.Data) != id {
		return nil, fmt.Errorf("%w: hash mismatch for %s", errLooseObjectCorrupt, id)
	}
	return append([]byte(nil), envelope.Data...), nil
}

func (s *looseObjectStore) path(id ObjectID) (string, error) {
	if s.looseDir == "" {
		return "", errors.New("loose object store is not durable")
	}
	if !validLooseObjectID(id) {
		return "", fmt.Errorf("%w: %q", errInvalidLooseObjectID, id)
	}
	path := filepath.Join(s.looseDir, string(id[:2]), string(id[2:]))
	relative, err := filepath.Rel(s.looseDir, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: unsafe path for %q", errInvalidLooseObjectID, id)
	}
	return path, nil
}

func validLooseObjectID(id ObjectID) bool {
	if len(id) != objectIDHexLength || strings.ToLower(string(id)) != string(id) {
		return false
	}
	decoded, err := hex.DecodeString(string(id))
	return err == nil && len(decoded) == objectIDHexLength/2
}

func objectIDForEncoded(objectType string, encoded []byte) ObjectID {
	header := objectType + " " + strconv.Itoa(len(encoded)) + "\x00"
	sum := blake3.Sum256(append([]byte(header), encoded...))
	return ObjectID(hex.EncodeToString(sum[:]))
}
